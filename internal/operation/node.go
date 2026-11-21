/*
Purpose: Prepare scheduling and drain reviews bound to a captured Node.
Used by: The desktop Node operation adapter and integration tests.
Responsibilities: Check identity, scoped access, complete impact and conditional scheduling patches.
Important: Review never evicts or changes persistent scheduling state.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"slices"
	"strings"
	"time"
)

type evictionRejection struct {
	budget bool
	delay  int
}

func (c operationClient) node(ctx context.Context, r kube.ResourceRef, version string) (core.Node, error) {
	var n core.Node
	_, e := c.call(ctx, "GET", "/api/v1/nodes/"+r.Name, false, nil, &n)
	if e != nil {
		return n, nodeProblem(e)
	}
	if n.APIVersion != "v1" || n.Kind != "Node" || n.Name != r.Name || string(n.UID) != r.UID || n.Namespace != "" || n.DeletionTimestamp != nil || n.ResourceVersion == "" || (version != "" && n.ResourceVersion != version) {
		return n, model.Fail("stale_target", "Node changed. Refresh and review again.")
	}
	return n, nil
}
func (c operationClient) scheduling(ctx context.Context, n core.Node, want, dry bool) error {
	patch, e := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": n.UID}, {"op": "test", "path": "/metadata/resourceVersion", "value": n.ResourceVersion}, {"op": "add", "path": "/spec/unschedulable", "value": want}})
	if e != nil {
		return e
	}
	var after core.Node
	if _, e = c.patch(ctx, "/api/v1/nodes/"+n.Name, dry, patch, &after); e != nil {
		return e
	}
	n.Spec.Unschedulable = want
	if after.UID != n.UID || after.Name != n.Name || after.ResourceVersion == "" || after.Kind != "Node" || !reflect.DeepEqual(after.Spec, n.Spec) {
		return model.Fail("invalid_response", "Scheduling response changed beyond the reviewed field")
	}
	return nil
}
func (s *Coordinator) PrepareNodeAction(ctx context.Context, req NodeRequest, environment string) (NodePreview, error) {
	p := NodePreview{Environment: environment, NodeRequest: req, Rows: []NodePod{}}
	r := req.Resource
	if r.Kind != "Node" || r.APIVersion != "v1" || r.Namespace != "" || !safeNodeName(r.Name) || r.UID == "" || len(r.UID) > 128 || len(req.SourceVersion) > 128 || req.SourceVersion == "" || (req.Action != "cordon" && req.Action != "uncordon" && req.Action != "drain") || (req.WaitPreset != 2 && req.WaitPreset != 5 && req.WaitPreset != 10) {
		return p, model.Fail("invalid", "Select a current Node and supported action and wait time")
	}
	ctx, release, e := s.prepare(ctx)
	if e != nil {
		return p, e
	}
	defer release()
	c, e := newOperationClient(s.connections, r.Connection)
	if e != nil {
		return p, e
	}
	c.client.Timeout = 10 * time.Second
	n, e := c.node(ctx, r, req.SourceVersion)
	if e != nil {
		return p, e
	}
	p.Cluster, e = s.connections.ContextName(r.Connection)
	if e != nil {
		return p, e
	}
	p.Cordoned = n.Spec.Unschedulable
	want := req.Action != "uncordon"
	if p.Cordoned != want {
		if permissionOf(ctx, c, r, "nodes", "patch", "") != "allowed" {
			return p, model.Fail("forbidden", "Cannot verify permission to patch this Node")
		}
		if e = c.scheduling(ctx, n, want, true); e != nil {
			return p, nodeProblem(e)
		}
	} else if req.Action != "drain" {
		p.NoChange = true
		return p, nil
	}
	if req.Action == "drain" {
		var discovery meta.APIResourceList
		if _, e = c.call(ctx, "GET", "/api/v1", false, nil, &discovery); e != nil {
			return p, nodeProblem(e)
		}
		supported := false
		for _, v := range discovery.APIResources {
			if v.Name == "pods/eviction" && v.Kind == "Eviction" && v.Group == "policy" && v.Version == "v1" && slices.Contains(v.Verbs, "create") {
				supported = true
			}
		}
		if !supported {
			return p, model.Fail("unsupported", "The server does not advertise policy/v1 Pod eviction")
		}
		p.Rows, e = c.inventory(ctx, r.Name)
		if e != nil {
			return p, nodeProblem(e)
		}
		access := map[string]string{}
		for i, row := range p.Rows {
			if row.State != "candidate" {
				continue
			}
			if access[row.Namespace] == "" {
				access[row.Namespace] = permissionOf(ctx, c, kube.ResourceRef{APIVersion: "v1", Namespace: row.Namespace}, "pods", "create", "eviction")
			}
			if access[row.Namespace] != "allowed" {
				p.Rows[i].State, p.Rows[i].Reason = "blocked", "Cannot verify eviction permission in this namespace"
			}
		}
	}
	p.Counts = nodeCounts(p.Rows)
	if p.Counts["blocked"] > 0 {
		return p, nil
	}
	progress := &NodeProgress{Action: req.Action, Cordoned: &p.Cordoned, Rows: p.Rows, Counts: p.Counts}
	review := pendingReview{result: OperationResult{Action: "node", Cluster: p.Cluster, Resource: r, Node: progress}, payload: p}
	p.Typed = req.Action == "drain" || strings.EqualFold(environment, "production") || strings.EqualFold(environment, "prod")
	review.payload = p
	if p.Typed {
		review.acknowledgement = r.Name
	}
	p.Token, p.ExpiresAt, e = s.save(ctx, review)
	return p, e
}

func nodeProblem(err error) error {
	var p *model.Problem
	if errors.As(err, &p) {
		return p
	}
	var code operationHTTPStatus
	if errors.As(err, &code) {
		switch code {
		case 401, 403:
			return model.Fail("forbidden", "Kubernetes denied the Node operation review")
		case 404, 409, 410, 422:
			return model.Fail("stale_target", "Node or Pod inventory changed. Refresh and review again.")
		}
	}
	return model.Fail("unavailable", "Cannot verify the Node operation with Kubernetes")
}
