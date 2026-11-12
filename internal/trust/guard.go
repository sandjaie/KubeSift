/*
Purpose: Check trust at the exact moment a credential helper starts.
Used by: Connection authentication, stream startup and desktop helper review.
Responsibilities: Resolve executable identity, redact previews, consume preview IDs and run a bounded cancellable protocol exchange.
Important: Native client-go refresh stays disabled; revocation and process start share the same authorization lock.
*/
package trust

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/model"
	"io"
	"k8s.io/client-go/rest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type PreviewRecord struct {
	ID          string   `json:"id"`
	Fingerprint string   `json:"fingerprint"`
	Source      string   `json:"source"`
	Context     string   `json:"context"`
	Executable  string   `json:"executable"`
	Args        []string `json:"args"`
	Env         []string `json:"env"`
}
type pending struct {
	preview PreviewRecord
	expires time.Time
}
type Guard struct {
	store   *Store
	mu      sync.Mutex
	pending map[string]pending
	now     func() time.Time
	runner  func(context.Context, string, []string, []string) ([]byte, error)
}

func NewGuard(store *Store) *Guard {
	return &Guard{store: store, pending: map[string]pending{}, now: time.Now}
}

func Preview(r config.Resolved) (PreviewRecord, error) {
	if !r.HasHelper() {
		return PreviewRecord{}, model.Fail("invalid", "Context has no credential helper")
	}
	cfg := r.RESTConfig()
	helper := cfg.ExecProvider
	path, err := exec.LookPath(helper.Command)
	if err != nil {
		return PreviewRecord{}, model.Fail("helper_missing", "Credential helper executable is missing")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return PreviewRecord{}, model.Fail("invalid", "Invalid helper path")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return PreviewRecord{}, model.Fail("invalid", "Cannot resolve helper executable")
	}
	if r.HelperPath() != "" && r.HelperPath() != path {
		return PreviewRecord{}, model.Fail("trust_required", "Helper executable resolution changed")
	}
	helper.Command = path
	sort.Slice(helper.Env, func(i, j int) bool { return helper.Env[i].Name < helper.Env[j].Name })
	for i, e := range helper.Env {
		if e.Name == "" || strings.ContainsAny(e.Name, "=\x00") || (i > 0 && helper.Env[i-1].Name == e.Name) {
			return PreviewRecord{}, model.Fail("invalid", "Invalid helper environment")
		}
	}
	var cluster any
	if helper.ProvideClusterInfo {
		value, e := rest.ConfigToExecCluster(cfg)
		if e != nil {
			return PreviewRecord{}, model.Fail("invalid", "Invalid helper cluster information")
		}
		cluster = value
	}
	encoded, err := json.Marshal(struct {
		Source  string
		Name    string
		Helper  any
		Cluster any
	}{r.Source(), r.Name(), helper, cluster})
	if err != nil {
		return PreviewRecord{}, model.Fail("invalid", "Invalid helper configuration")
	}
	hash := sha256.Sum256(encoded)
	p := PreviewRecord{ID: model.ID(), Fingerprint: hex.EncodeToString(hash[:]), Source: r.SourceDisplay(), Context: r.Name(), Executable: path, Args: []string{}, Env: []string{}}
	for range helper.Args {
		p.Args = append(p.Args, "[redacted]")
	}
	for _, e := range helper.Env {
		p.Env = append(p.Env, e.Name+"=[redacted]")
	}
	return p, nil
}
func (g *Guard) Review(r config.Resolved) (PreviewRecord, error) {
	p, err := Preview(r)
	if err != nil {
		return p, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for key, v := range g.pending {
		if !g.now().Before(v.expires) {
			delete(g.pending, key)
		}
	}
	if len(g.pending) >= 128 {
		return PreviewRecord{}, model.Fail("invalid", "Too many helper previews")
	}
	g.pending[p.ID] = pending{p, g.now().Add(5 * time.Minute)}
	return p, nil
}
func (g *Guard) ApprovePreview(id string, current config.Resolved) error {
	g.mu.Lock()
	p, ok := g.pending[id]
	delete(g.pending, id)
	g.mu.Unlock()
	if !ok || !g.now().Before(p.expires) {
		return model.Fail("expired", "Helper preview expired")
	}
	latest, err := Preview(current)
	if err != nil {
		return err
	}
	if latest.Fingerprint != p.preview.Fingerprint {
		return model.Fail("stale_target", "Helper configuration changed")
	}
	return g.store.approve(latest.Fingerprint, filepath.Base(latest.Executable)+" · "+latest.Context)
}
func (g *Guard) Check(r config.Resolved) error {
	if !r.HasHelper() {
		return nil
	}
	p, err := Preview(r)
	if err != nil {
		return err
	}
	if g.store == nil {
		return model.Fail("trust_required", "Review credential helper")
	}
	g.store.mu.Lock()
	defer g.store.mu.Unlock()
	if !g.store.allowedLocked(p.Fingerprint) {
		return model.Fail("trust_required", "Review credential helper")
	}
	return nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, io.ErrShortBuffer
	}
	return b.Buffer.Write(p)
}
func (g *Guard) Authenticate(ctx context.Context, r config.Resolved) (*rest.Config, error) {
	cfg := r.RESTConfig()
	if !r.HasHelper() {
		return cfg, nil
	}
	p, err := Preview(r)
	if err != nil {
		return nil, err
	}
	helper := cfg.ExecProvider
	if helper.APIVersion != "client.authentication.k8s.io/v1" && helper.APIVersion != "client.authentication.k8s.io/v1beta1" {
		return nil, model.Fail("invalid", "Unsupported credential protocol")
	}
	if helper.InteractiveMode == "Always" {
		return nil, model.Fail("external_login_required", "Refresh credentials in your terminal, then Retry; interactive helpers are unsupported")
	}
	info := map[string]any{"apiVersion": helper.APIVersion, "kind": "ExecCredential", "spec": map[string]any{"interactive": false}}
	if helper.ProvideClusterInfo {
		cluster, e := rest.ConfigToExecCluster(cfg)
		if e != nil {
			return nil, model.Fail("invalid", "Invalid helper cluster information")
		}
		info["spec"].(map[string]any)["cluster"] = cluster
	}
	encoded, e := json.Marshal(info)
	if e != nil {
		return nil, model.Fail("invalid", "Invalid helper request")
	}
	environment := map[string]string{}
	for _, v := range os.Environ() {
		if k, value, ok := strings.Cut(v, "="); ok {
			environment[k] = value
		}
	}
	for _, v := range helper.Env {
		environment[v.Name] = v.Value
	}
	environment["KUBERNETES_EXEC_INFO"] = string(encoded)
	env := make([]string, 0, len(environment))
	for k, v := range environment {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var data []byte
	// Serialize authorization with process start: revocation blocks every future invocation.
	if g.store == nil {
		return nil, model.Fail("trust_required", "Review credential helper")
	}
	g.store.mu.Lock()
	if !g.store.allowedLocked(p.Fingerprint) {
		g.store.mu.Unlock()
		return nil, model.Fail("trust_required", "Review credential helper")
	}
	if runCtx.Err() != nil {
		g.store.mu.Unlock()
		return nil, model.Fail("cancelled", "Credential request cancelled")
	}
	if g.runner != nil {
		data, err = g.runner(runCtx, p.Executable, helper.Args, env)
		g.store.mu.Unlock()
	} else {
		command := exec.CommandContext(runCtx, p.Executable, helper.Args...)
		command.Env = env
		command.Stdin = nil
		command.Stderr = io.Discard
		command.WaitDelay = time.Second
		output := &boundedOutput{}
		command.Stdout = output
		err = command.Start()
		g.store.mu.Unlock()
		if err == nil {
			err = command.Wait()
		}
		data = output.Bytes()
	}
	if err != nil {
		if runCtx.Err() != nil {
			return nil, model.Fail("cancelled", "Credential request cancelled or timed out")
		}
		return nil, model.Fail("external_login_required", "Credential helper failed. Refresh credentials in your terminal, then Retry")
	}
	if len(data) > 1<<20 {
		return nil, model.Fail("invalid", "Credential response too large")
	}
	var result struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Status     *struct {
			Token      string     `json:"token"`
			Expiration *time.Time `json:"expirationTimestamp"`
			Cert       string     `json:"clientCertificateData"`
			Key        string     `json:"clientKeyData"`
		} `json:"status"`
	}
	if json.Unmarshal(data, &result) != nil || result.APIVersion != helper.APIVersion || result.Kind != "ExecCredential" || result.Status == nil {
		return nil, model.Fail("invalid", "Invalid credential response")
	}
	status := result.Status
	if status.Expiration != nil && !g.now().Before(*status.Expiration) {
		return nil, model.Fail("expired", "Credential helper returned expired authentication")
	}
	if status.Token == "" && (status.Cert == "" || status.Key == "") {
		return nil, model.Fail("invalid", "Credential helper returned no authentication")
	}
	if status.Cert != "" || status.Key != "" {
		if _, err = tls.X509KeyPair([]byte(status.Cert), []byte(status.Key)); err != nil {
			return nil, model.Fail("invalid", "Invalid helper certificate")
		}
		cfg.CertData = []byte(status.Cert)
		cfg.KeyData = []byte(status.Key)
		cfg.CertFile = ""
		cfg.KeyFile = ""
	}
	cfg.BearerToken = status.Token
	cfg.BearerTokenFile = ""
	cfg.ExecProvider = nil
	return cfg, nil
}

func (g *Guard) Entries() []Entry                { return g.store.List() }
func (g *Guard) Revoke(fingerprint string) error { return g.store.Revoke(fingerprint) }
