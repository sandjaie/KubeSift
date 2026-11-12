/*
Purpose: Expose cancellable workload reads through the existing explorer service.
Used by: Generated workload bindings.
Responsibilities: Bound concurrent requests and delegate typed reads to the native reader.
Important: Request IDs own cancellation independently; credentials never cross the bridge.
*/
package desktop

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"time"
)

type workloadRead struct{ cancel context.CancelFunc }

func (s *ExplorerService) beginWorkloadRead(id string) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" || len(id) > 128 {
		return nil, nil, model.Fail("invalid", "Unique request ID required")
	}
	if s.reads == nil {
		s.reads = map[string]*workloadRead{}
	}
	if s.reads[id] != nil || len(s.reads) >= 16 {
		return nil, nil, model.Fail("busy", "Too many active workload reads")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	entry := &workloadRead{cancel: cancel}
	s.reads[id] = entry
	done := func() {
		cancel()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.reads[id] == entry {
			delete(s.reads, id)
		}
	}
	return ctx, done, nil
}
func (s *ExplorerService) CancelWorkloadRead(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.reads[id]; entry != nil {
		entry.cancel()
		delete(s.reads, id)
	}
	return nil
}
func (s *ExplorerService) ReadWorkloads(id string, req kube.PageRequest) (kube.WorkloadPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.WorkloadPage{}, err
	}
	defer done()
	return s.core.kube.ReadWorkloads(ctx, req)
}

func (s *ExplorerService) ReadResource(id string, req kube.DetailRequest) (kube.ResourceDetail, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ResourceDetail{}, err
	}
	defer done()
	return s.core.kube.ReadResource(ctx, req)
}

func (s *ExplorerService) ReadResourceYAML(id string, req kube.DetailRequest) (kube.YAMLDocument, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.YAMLDocument{}, err
	}
	defer done()
	return s.core.kube.ReadResourceYAML(ctx, req)
}

func (s *ExplorerService) ReadRelatedPods(id string, req kube.RelatedRequest) (kube.RelatedPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.RelatedPage{}, err
	}
	defer done()
	return s.core.kube.ReadRelatedPods(ctx, req)
}

func (s *ExplorerService) ReadResourceEvents(id string, req kube.EventRequest) (kube.EventPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.EventPage{}, err
	}
	defer done()
	return s.core.kube.ReadResourceEvents(ctx, req)
}
