/*
Purpose: Own cancellable product connections and truthful access snapshots.
Used by: ExplorerService and local integration tests.
Responsibilities: Serialize source changes, guard attempt generations and schedule bounded resource checks.
Important: Workers own explicit targets; cancelled or superseded results cannot restore a connection.
*/
package explorer

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/sources"
	"github.com/sandjaie/KubeSift/internal/trust"
	"sort"
	"strings"
	"sync"
	"time"
)

type ConnectRequest struct {
	SourceID   string `json:"sourceId"`
	SnapshotID string `json:"snapshotId"`
	Context    string `json:"context"`
	Namespace  string `json:"namespace"`
}
type Attempt struct {
	ID string `json:"id"`
}
type ConnectionView struct {
	AttemptID          string               `json:"attemptId"`
	SourceID           string               `json:"sourceId"`
	Context            string               `json:"context"`
	Namespace          string               `json:"namespace"`
	State              string               `json:"state"`
	Ref                *model.ConnectionRef `json:"ref"`
	Problem            *model.Problem       `json:"problem"`
	LastSuccessAt      string               `json:"lastSuccessAt"`
	Namespaces         []string             `json:"namespaces"`
	NamespacesComplete bool                 `json:"namespacesComplete"`
	Pods               []kube.PodSummary    `json:"pods"`
	PodProblem         *model.Problem       `json:"podProblem"`
	PodLoading         bool                 `json:"podLoading"`
}
type Probe interface {
	ProbeAccess(context.Context, model.ConnectionRef, string, string) (kube.AccessResult, error)
}
type work struct {
	view           ConnectionView
	identity       string
	ctx            context.Context
	cancel         context.CancelFunc
	requestCancel  context.CancelFunc
	epoch          uint64
	probe          string
	probeNamespace string
	readPods       bool
	wake           chan struct{}
	done           chan struct{}
}
type cached struct {
	view ConnectionView
	at   time.Time
}
type Coordinator struct {
	mu          sync.Mutex
	registry    *sources.Registry
	connections *connection.Manager
	probe       Probe
	active      map[string]*work
	completed   map[string]cached
	now         func() time.Time
	after       func(time.Duration) <-chan time.Time
	closed      bool
	wg          sync.WaitGroup
}

func NewCoordinator(r *sources.Registry, c *connection.Manager, p Probe) *Coordinator {
	return &Coordinator{registry: r, connections: c, probe: p, active: map[string]*work{}, completed: map[string]cached{}, now: time.Now, after: time.After}
}
func key(source, name string) string { return source + "\x00" + name }
func (c *Coordinator) ListSources() ([]sources.SourceResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registry.List(), nil
}
func (c *Coordinator) AddSource(path string) (sources.SourceResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registry.Add(path)
}
func (c *Coordinator) SetLabel(source, name, label string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registry.SetLabel(source, name, label)
}
func (c *Coordinator) reload(id, except string, invalidatePending bool) (sources.SourceResult, error) {
	result, e := c.registry.Reload(id)
	if e != nil {
		return result, e
	}
	for _, w := range c.active {
		if w.view.SourceID != id || w.view.AttemptID == except {
			continue
		}
		r, e := c.registry.Resolve(id, result.SnapshotID, w.view.Context)
		if e != nil || r.SafetyIdentity() != w.identity || (invalidatePending && w.view.State == "connecting") {
			c.halt(w, "stale", &model.Problem{Code: "stale_target", Message: "Source changed or its pending selection was refreshed. Review the source, then Retry"})
		}
	}
	return result, nil
}
func (c *Coordinator) ReloadSource(id string) (sources.SourceResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reload(id, "", true)
}
func (c *Coordinator) StartConnection(req ConnectRequest) (Attempt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return Attempt{}, model.Fail("cancelled", "Application is closing")
	}
	k := key(req.SourceID, req.Context)
	old := c.active[k]
	if old != nil && old.view.State == "connecting" {
		return Attempt{old.view.AttemptID}, nil
	}
	if old == nil && len(c.active) >= 16 {
		return Attempt{}, model.Fail("capacity", "Close a tab before opening another; at most 16 connections are supported")
	}
	expected, e := c.registry.Resolve(req.SourceID, req.SnapshotID, req.Context)
	if e != nil {
		return Attempt{}, e
	}
	if old != nil {
		c.halt(old, "not_connected", nil)
		c.remember(old.view)
		delete(c.active, k)
	}
	fresh, e := c.reload(req.SourceID, "", false)
	if e != nil {
		return Attempt{}, e
	}
	r, e := c.registry.Resolve(req.SourceID, fresh.SnapshotID, req.Context)
	if e != nil {
		return Attempt{}, e
	}
	if expected.SafetyIdentity() != r.SafetyIdentity() {
		return Attempt{}, model.Fail("stale_target", "Configuration changed. Review the refreshed source before connecting")
	}
	ns := req.Namespace
	if ns == "" {
		for _, entry := range fresh.Contexts {
			if entry.Context == req.Context {
				ns = entry.Namespace
			}
		}
	}
	if ns == "" {
		ns = "default"
	}
	owned, cancel := context.WithCancel(context.Background())
	w := &work{view: ConnectionView{AttemptID: model.ID(), SourceID: req.SourceID, Context: req.Context, Namespace: ns, State: "connecting", Namespaces: []string{}, Pods: []kube.PodSummary{}, PodLoading: true}, identity: r.SafetyIdentity(), ctx: owned, cancel: cancel, wake: make(chan struct{}, 1), done: make(chan struct{})}
	c.active[k] = w
	if r.Anonymous() {
		w.view.State = "failed"
		w.view.Problem = &model.Problem{Code: "failed", Message: "Anonymous configurations are not supported; configure credentials externally"}
		w.view.PodLoading = false
		cancel()
		close(w.done)
		return Attempt{w.view.AttemptID}, nil
	}
	ref, e := c.connections.Open(owned, r)
	if e != nil {
		w.view.Problem = kube.ClassifyError(owned, e)
		w.view.State = state(w.view.Problem)
		w.view.PodLoading = false
		cancel()
		close(w.done)
		return Attempt{w.view.AttemptID}, nil
	}
	w.view.Ref = &ref
	c.wg.Add(1)
	go c.run(w)
	return Attempt{w.view.AttemptID}, nil
}
func state(p *model.Problem) string {
	if p == nil {
		return "connected"
	}
	switch p.Code {
	case "forbidden":
		return "restricted"
	case "trust_required":
		return "review_required"
	case "cancelled":
		return "not_connected"
	case "stale_target":
		return "stale"
	default:
		return p.Code
	}
}
func terminal(p *model.Problem) bool {
	if p == nil {
		return false
	}
	switch p.Code {
	case "unreachable", "timeout", "certificate":
		return false
	default:
		return true
	}
}
func (c *Coordinator) run(w *work) {
	defer c.wg.Done()
	defer close(w.done)
	delay := 30 * time.Second
	for {
		c.mu.Lock()
		if w.ctx.Err() != nil {
			c.mu.Unlock()
			return
		}
		epoch, ns, mode, ref := w.epoch, w.view.Namespace, w.probe, *w.view.Ref
		readingPods := w.readPods
		w.readPods = false
		if readingPods {
			mode = "pods"
		} else if w.probeNamespace != "" {
			ns = w.probeNamespace
		}
		request, cancel := context.WithTimeout(w.ctx, 30*time.Second)
		w.requestCancel = cancel
		c.mu.Unlock()
		result, err := c.probe.ProbeAccess(request, ref, ns, mode)
		problem := kube.ClassifyError(request, err)
		cancel()
		c.mu.Lock()
		w.requestCancel = nil
		if w.ctx.Err() != nil || c.active[key(w.view.SourceID, w.view.Context)] != w {
			c.mu.Unlock()
			return
		}
		if epoch != w.epoch {
			select {
			case <-w.wake:
			default:
			}
			c.mu.Unlock()
			continue
		}
		w.view.PodLoading = false
		// A denied view does not revoke evidence established by a different resource path.
		if readingPods && problem != nil && problem.Code == "forbidden" && w.probe != "" {
			problem = nil
		}
		w.view.Problem = problem
		w.view.State = state(problem)
		if result.AuthorizedRead && problem == nil {
			w.view.LastSuccessAt = c.now().UTC().Format(time.RFC3339Nano)
			if w.probe == "" || !readingPods {
				w.probe = result.Probe
				w.probeNamespace = ns
			}
			delay = 30 * time.Second
		}
		if mode != "namespaces" && ns == w.view.Namespace {
			w.view.Pods = result.Pods
			w.view.PodProblem = result.PodProblem
		}
		if mode != "pods" {
			w.view.Namespaces = result.Namespaces
			w.view.NamespacesComplete = result.NamespacesComplete
		}
		stop := terminal(problem)
		if problem != nil && problem.Code != "forbidden" && stop {
			c.connections.Close(ref)
		}
		c.mu.Unlock()
		if stop {
			select {
			case <-w.ctx.Done():
				return
			case <-w.wake:
				continue
			}
		}
		select {
		case <-w.ctx.Done():
			return
		case <-w.wake:
		case <-c.after(delay):
		}
		if problem != nil {
			delay = 60 * time.Second
		}
	}
}
func (c *Coordinator) copy(w *work) ConnectionView {
	v := w.view
	v.Namespaces = append([]string{}, v.Namespaces...)
	v.Pods = append([]kube.PodSummary{}, v.Pods...)
	if v.State == "connected" {
		stamp, e := time.Parse(time.RFC3339Nano, v.LastSuccessAt)
		if e != nil || c.now().Sub(stamp) > 90*time.Second {
			v.State = "stale"
		}
	}
	return v
}
func (c *Coordinator) GetAttempt(id string) (ConnectionView, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range c.active {
		if w.view.AttemptID == id {
			return c.copy(w), nil
		}
	}
	c.prune()
	if v, ok := c.completed[id]; ok {
		return v.view, nil
	}
	return ConnectionView{}, model.Fail("stale_target", "Attempt is no longer available")
}
func (c *Coordinator) find(ref model.ConnectionRef) *work {
	for _, w := range c.active {
		if w.view.Ref != nil && *w.view.Ref == ref {
			return w
		}
	}
	return nil
}
func (c *Coordinator) GetConnection(ref model.ConnectionRef) (ConnectionView, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.find(ref)
	if w == nil {
		return ConnectionView{}, model.Fail("stale_target", "Connection was replaced or closed")
	}
	return c.copy(w), nil
}
func (c *Coordinator) halt(w *work, status string, p *model.Problem) {
	w.cancel()
	if w.requestCancel != nil {
		w.requestCancel()
	}
	if w.view.Ref != nil {
		c.connections.Close(*w.view.Ref)
	}
	w.view.State = status
	w.view.Problem = p
	w.view.PodLoading = false
}
func (c *Coordinator) prune() {
	for id, v := range c.completed {
		if c.now().Sub(v.at) >= 5*time.Minute {
			delete(c.completed, id)
		}
	}
	for len(c.completed) > 64 {
		var oldest string
		var when time.Time
		for id, v := range c.completed {
			if oldest == "" || v.at.Before(when) {
				oldest = id
				when = v.at
			}
		}
		delete(c.completed, oldest)
	}
}
func (c *Coordinator) remember(v ConnectionView) {
	c.completed[v.AttemptID] = cached{v, c.now()}
	c.prune()
}
func (c *Coordinator) CancelAttempt(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, w := range c.active {
		if w.view.AttemptID == id {
			c.halt(w, "not_connected", nil)
			c.remember(w.view)
			delete(c.active, k)
			return nil
		}
	}
	return nil
}
func (c *Coordinator) Disconnect(ref model.ConnectionRef) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.find(ref)
	if w == nil {
		return model.Fail("stale_target", "Connection was replaced or closed")
	}
	c.halt(w, "not_connected", nil)
	c.remember(w.view)
	delete(c.active, key(w.view.SourceID, w.view.Context))
	return nil
}

// ReadPods schedules the only worker for this connection, cancelling its prior read.
func (c *Coordinator) ReadPods(ref model.ConnectionRef, ns string) (ConnectionView, error) {
	return c.changeNamespace(ref, ns, true)
}

// SetWorkloadNamespace updates the connection view while the workload reader owns inventory.
func (c *Coordinator) SetWorkloadNamespace(ref model.ConnectionRef, ns string) (ConnectionView, error) {
	return c.changeNamespace(ref, ns, false)
}

func (c *Coordinator) changeNamespace(ref model.ConnectionRef, ns string, readPods bool) (ConnectionView, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.find(ref)
	if w == nil || w.ctx.Err() != nil {
		return ConnectionView{}, model.Fail("stale_target", "Retry the connection before reading")
	}
	if ns == "" {
		return ConnectionView{}, model.Fail("invalid_source", "An explicit namespace is required")
	}
	if w.view.Problem != nil && w.view.Problem.Code != "forbidden" {
		return ConnectionView{}, model.Fail("stale_target", "Retry the connection before reading")
	}
	w.epoch++
	w.view.Namespace = ns
	w.view.Pods = []kube.PodSummary{}
	w.view.PodProblem = nil
	w.view.PodLoading = readPods
	w.readPods = readPods
	if !readPods {
		return c.copy(w), nil
	}
	if w.requestCancel != nil {
		w.requestCancel()
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return c.copy(w), nil
}
func (c *Coordinator) changeSource(id string, confirmed bool, change func() error) error {
	affected := []string{}
	for _, w := range c.active {
		if w.view.SourceID == id {
			affected = append(affected, w.view.Context)
		}
	}
	sort.Strings(affected)
	if len(affected) > 0 && !confirmed {
		return model.Fail("confirmation_required", "Close affected contexts: "+strings.Join(affected, ", "))
	}
	// Persist first under the coordination lock; failed writes leave tabs intact.
	if e := change(); e != nil {
		return e
	}
	for k, w := range c.active {
		if w.view.SourceID == id {
			c.halt(w, "not_connected", nil)
			c.remember(w.view)
			delete(c.active, k)
		}
	}
	return nil
}
func (c *Coordinator) RemoveSource(id string, confirmed bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changeSource(id, confirmed, func() error { return c.registry.Remove(id) })
}
func (c *Coordinator) SetSourceEnabled(id string, enabled, confirmed bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if enabled {
		return c.registry.SetEnabled(id, true)
	}
	return c.changeSource(id, confirmed, func() error { return c.registry.SetEnabled(id, false) })
}
func (c *Coordinator) ReviewHelper(g *trust.Guard, source, snapshot, name string) (trust.PreviewRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, e := c.registry.Resolve(source, snapshot, name)
	if e != nil {
		return trust.PreviewRecord{}, e
	}
	return g.Review(r)
}
func (c *Coordinator) ApproveHelper(g *trust.Guard, source, snapshot, name, review string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	expected, e := c.registry.Resolve(source, snapshot, name)
	if e != nil {
		return e
	}
	fresh, e := c.reload(source, "", true)
	if e != nil {
		return e
	}
	r, e := c.registry.Resolve(source, fresh.SnapshotID, name)
	if e != nil {
		return e
	}
	if r.SafetyIdentity() != expected.SafetyIdentity() {
		return model.Fail("stale_target", "Source changed; review the helper again")
	}
	return g.ApprovePreview(review, r)
}
func (c *Coordinator) RevokeHelper(g *trust.Guard, fp string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := g.Revoke(fp); e != nil {
		return e
	}
	for _, w := range c.active {
		c.halt(w, "review_required", &model.Problem{Code: "trust_required", Message: "Helper trust revoked; review and explicitly reconnect"})
	}
	c.connections.CloseAll()
	return nil
}
func (c *Coordinator) Close() {
	c.mu.Lock()
	c.closed = true
	for _, w := range c.active {
		c.halt(w, "not_connected", nil)
	}
	c.mu.Unlock()
	c.wg.Wait()
}
