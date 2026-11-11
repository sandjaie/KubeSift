/*
Purpose: Read Kubernetes configuration without changing it or executing credentials.
Used by: The desktop source picker, connection manager and helper review.
Responsibilities: Resolve named contexts, preserve Kubernetes extensions and return private configuration snapshots.
Important: A preview is not permission to execute a helper; resolved configurations stay in Go.
*/
// Package config loads read-only Kubernetes configuration without executing credentials.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	api "k8s.io/client-go/tools/clientcmd/api"
	"net/url"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type ContextSummary struct {
	SnapshotID string `json:"snapshotId"`
	Source     string `json:"source"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Endpoint   string `json:"endpoint"`
}
type Snapshot struct {
	raw     api.Config
	source  string
	sources string
}
type Resolved struct {
	source, name        string
	sources, helperPath string
	identity            string
	cfg                 *rest.Config
}

func (r Resolved) Source() string        { return r.source }
func (r Resolved) SourceDisplay() string { return r.sources }
func (r Resolved) HelperPath() string    { return r.helperPath }
func (r Resolved) FreezeHelper() (Resolved, error) {
	if !r.HasHelper() {
		return r, nil
	}
	path, err := exec.LookPath(r.cfg.ExecProvider.Command)
	if err != nil {
		return r, model.Fail("helper_missing", "Credential helper executable is missing")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return r, model.Fail("invalid", "Invalid helper path")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return r, model.Fail("invalid", "Cannot resolve helper executable")
	}
	r.helperPath = path
	return r, nil
}
func (r Resolved) Name() string { return r.name }
func (r Resolved) RESTConfig() *rest.Config {
	copy := rest.CopyConfig(r.cfg)
	if copy.ExecProvider != nil {
		copy.ExecProvider = copy.ExecProvider.DeepCopy()
	}
	copy.CAData = append([]byte(nil), copy.CAData...)
	copy.CertData = append([]byte(nil), copy.CertData...)
	copy.KeyData = append([]byte(nil), copy.KeyData...)
	copy.NextProtos = append([]string(nil), copy.NextProtos...)
	return copy
}
func (r Resolved) HasHelper() bool { return r.cfg != nil && r.cfg.ExecProvider != nil }
func Load(paths []string) (*Snapshot, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if len(paths) > 0 {
		rules.Precedence = append([]string(nil), paths...)
	}
	raw, err := rules.Load()
	if err != nil {
		return nil, model.Fail("invalid", "Cannot load kubeconfig")
	}
	sources := rules.GetLoadingPrecedence()
	for i, p := range sources {
		sources[i], err = filepath.Abs(p)
		if err != nil {
			return nil, model.Fail("invalid", "Invalid source path")
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(sources, "\x00")))
	return &Snapshot{raw: *raw, source: hex.EncodeToString(sum[:]), sources: strings.Join(sources, ", ")}, nil
}
func (s *Snapshot) Contexts() []ContextSummary {
	out := []ContextSummary{}
	for name, c := range s.raw.Contexts {
		if c == nil {
			continue
		}
		endpoint := ""
		if cluster := s.raw.Clusters[c.Cluster]; cluster != nil {
			u, e := url.Parse(cluster.Server)
			if e == nil {
				endpoint = u.Scheme + "://" + u.Host
			}
		}
		ns := c.Namespace
		if ns == "" {
			ns = "default"
		}
		out = append(out, ContextSummary{Source: s.source, Name: name, Namespace: ns, Endpoint: endpoint})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (s *Snapshot) Resolve(name string) (Resolved, error) {
	c, ok := s.raw.Contexts[name]
	if !ok || c == nil {
		return Resolved{}, model.Fail("invalid", "Unknown context")
	}
	if s.raw.Clusters[c.Cluster] == nil || (c.AuthInfo != "" && s.raw.AuthInfos[c.AuthInfo] == nil) {
		return Resolved{}, model.Fail("invalid", "Unresolved context reference")
	}
	cfg, err := clientcmd.NewNonInteractiveClientConfig(s.raw, name, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return Resolved{}, model.Fail("invalid", "Invalid context configuration")
	}
	if cfg.AuthProvider != nil {
		return Resolved{}, model.Fail("invalid", "Legacy authentication provider unsupported")
	}
	if err := rest.LoadTLSFiles(cfg); err != nil {
		return Resolved{}, model.Fail("invalid", "Cannot read TLS files for selected context")
	}
	cfg.CAFile = ""
	cfg.CertFile = ""
	cfg.KeyFile = ""
	cfg.Timeout = 0
	// Hash serializable kubeconfig fields, including proxy/helper settings and loaded TLS files.
	// rest.Config contains function fields and cannot be JSON encoded.
	data, err := json.Marshal(struct {
		Cluster              *api.Cluster
		User                 *api.AuthInfo
		Namespace            string
		CA, Certificate, Key []byte
	}{s.raw.Clusters[c.Cluster], s.raw.AuthInfos[c.AuthInfo], c.Namespace, cfg.CAData, cfg.CertData, cfg.KeyData})
	if err != nil {
		return Resolved{}, model.Fail("invalid", "Cannot resolve configuration identity")
	}
	sum := sha256.Sum256(data)
	return Resolved{source: s.source, name: name, cfg: rest.CopyConfig(cfg), sources: s.sources, identity: hex.EncodeToString(sum[:])}, nil
}

// WithSource gives independently configured groups distinct connection/trust ownership.
func (r Resolved) WithSource(id string) Resolved { r.source = id; return r }
func (r Resolved) Anonymous() bool {
	c := r.cfg
	return c.ExecProvider == nil && c.BearerToken == "" && c.BearerTokenFile == "" && len(c.CertData) == 0 && c.Username == ""
}

// SafetyIdentity stays native and includes credential changes; never persist it as a label.
func (r Resolved) SafetyIdentity() string {
	return r.identity
}
func (s *Snapshot) SourceDisplay() string { return s.sources }

// LabelIdentity excludes credentials and helper values; a label describes endpoint/user identity.
func (s *Snapshot) LabelIdentity(name string) string {
	c := s.raw.Contexts[name]
	if c == nil {
		return ""
	}
	cluster := s.raw.Clusters[c.Cluster]
	data, _ := json.Marshal(struct {
		Cluster *api.Cluster
		User    string
	}{cluster, c.AuthInfo})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
