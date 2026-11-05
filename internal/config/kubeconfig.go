// Package config loads read-only Kubernetes configuration without executing credentials.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	api "k8s.io/client-go/tools/clientcmd/api"
	"net/url"
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
	raw    api.Config
	source string
}
type Resolved struct {
	source, name string
	cfg          *rest.Config
}

func (r Resolved) Source() string { return r.source }
func (r Resolved) Name() string   { return r.name }
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
	return &Snapshot{*raw, hex.EncodeToString(sum[:])}, nil
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
	cfg.Timeout = 0
	return Resolved{s.source, name, rest.CopyConfig(cfg)}, nil
}
