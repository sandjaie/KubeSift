/*
Purpose: Own native ephemeral Helm charts and source credentials.
Used by: Desktop management requests.
Responsibilities: Reserve bounded chart slots and expire sensitive snapshots independently of UI navigation.
Important: Close cancels native work and clears credential and chart references.
*/
package helmmanager

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/helmreader"
	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/chart"
)

type chartData struct {
	ChartSnapshot
	raw          []byte
	values       string
	valueOffsets map[int]bool
	source       string
	generation   uint64
}
type sourceData struct {
	ctx         context.Context
	cancel      context.CancelFunc
	source      Source
	credentials SourceCredentials
	generation  uint64
}
type indexData struct {
	id         string
	entries    []ChartEntry
	expires    time.Time
	generation uint64
}
type Service struct {
	mu               sync.Mutex
	connections      *connection.Manager
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	sources          map[string]sourceData
	charts           map[string]*chartData
	indexes          map[string]indexData
	closed           bool
	sourceGeneration uint64
	reviews          map[string]*reviewData
	configure        func(context.Context, Target, bool) (*configuration, error)
	perform          func(context.Context, *reviewData) (*helmreader.ReleaseSummary, bool, error)
	executions       map[string]*execution
	tokens           map[string]string
	locks            map[Target]bool
	running          int
	invalidate       func(Target)
}

func NewService(connections *connection.Manager) *Service {
	suppressHelmLogs()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{connections: connections, ctx: ctx, cancel: cancel, sources: map[string]sourceData{}, charts: map[string]*chartData{}, indexes: map[string]indexData{}, reviews: map[string]*reviewData{}, executions: map[string]*execution{}, tokens: map[string]string{}, locks: map[Target]bool{}}
	if connections != nil {
		connections.AddCleanup(func(ref model.ConnectionRef) {
			s.mu.Lock()
			defer s.mu.Unlock()
			for id, r := range s.reviews {
				if r.Target.Connection == ref {
					if r.cancel != nil {
						r.cancel()
					}
					delete(s.reviews, id)
				}
			}
		})
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.Lock()
				s.expireLocked()
				s.mu.Unlock()
			}
		}
	}()
	return s
}
func (s *Service) expireLocked() {
	now := time.Now()
	for id, r := range s.reviews {
		if now.After(r.ExpiresAt) {
			if r := s.reviews[id]; r != nil && r.cancel != nil {
				r.cancel()
			}
			delete(s.reviews, id)
		}
	}
	for id, c := range s.charts {
		if now.After(c.ExpiresAt) {
			delete(s.charts, id)
		}
	}
	for id, v := range s.indexes {
		if now.After(v.expires) {
			delete(s.indexes, id)
		}
	}
}
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.sources = map[string]sourceData{}
	s.charts = map[string]*chartData{}
	s.indexes = map[string]indexData{}
	s.reviews = map[string]*reviewData{}
	s.mu.Unlock()
	s.wg.Wait()
}
func (s *Service) ListSources() []Source {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Source{}
	for _, v := range s.sources {
		out = append(out, v.source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
func (s *Service) reserve(source string, generation uint64) (*chartData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	if s.closed {
		return nil, model.Fail("cancelled", "Helm manager is closed.")
	}
	if len(s.charts)+len(s.reviews) >= 4 {
		return nil, model.Fail("capacity", "Four Helm drafts are already retained. Close a draft first.")
	}
	c := &chartData{ChartSnapshot: ChartSnapshot{ID: model.ID(), ExpiresAt: time.Now().Add(reviewTTL)}, source: source, generation: generation}
	s.charts[c.ID] = c
	return c, nil
}
func (s *Service) Discard(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.charts, id)
	if r := s.reviews[id]; r != nil && r.cancel != nil {
		r.cancel()
	}
	delete(s.reviews, id)
}
func (s *Service) ImportChart(path string) (string, error) {
	ctx, done := s.owned(context.Background(), 120*time.Second)
	defer done()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", model.Fail("invalid", "Choose a regular packaged chart file.")
	}
	if filepath.Ext(path) != ".tgz" {
		return "", model.Fail("invalid", "Choose a packaged .tgz chart.")
	}
	c, err := s.reserve("", 0)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		s.Discard(c.ID)
		return "", model.Fail("unavailable", "Cannot read the selected chart.")
	}
	defer f.Close()
	raw, err := readBounded(f, archiveLimit)
	if err == nil {
		err = s.finishChart(ctx, c, raw)
	}
	if err != nil {
		s.Discard(c.ID)
		return "", err
	}
	return c.ID, nil
}
func (s *Service) finishChart(ctx context.Context, c *chartData, raw []byte) error {
	ch, err := loadChart(ctx, raw)
	if err != nil {
		return err
	}
	a, err := chart.NewAccessor(ch)
	if err != nil {
		return err
	}
	meta := a.MetadataAsMap()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.charts[c.ID] != c || ctx.Err() != nil {
		return model.Fail("cancelled", "Chart resolution was cancelled.")
	}
	if c.source != "" && s.sources[c.source].generation != c.generation {
		return model.Fail("stale_target", "Chart source changed. Resolve again.")
	}
	c.raw = raw
	c.Name = a.Name()
	c.Version, _ = meta["Version"].(string)
	if len(c.Name) > 256 || len(c.Version) > 256 {
		return model.Fail("capacity", "Chart name or version exceeds the display limit.")
	}
	c.Digest = chartHash(raw)
	return nil
}
func readBounded(r io.Reader, limit int) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if len(raw) > limit {
		return nil, model.Fail("capacity", "Helm input exceeds its size limit.")
	}
	return raw, err
}
func (s *Service) owned(ctx context.Context, timeout time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return ctx, func() {}
	}
	s.wg.Add(1)
	s.mu.Unlock()
	stop := context.AfterFunc(s.ctx, cancel)
	return ctx, func() { stop(); cancel(); s.wg.Done() }
}
