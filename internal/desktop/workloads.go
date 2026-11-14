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

func (s *ExplorerService) ReadRelatedReplicaSets(id string, req kube.RelatedRequest) (kube.RelatedReplicaSetPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.RelatedReplicaSetPage{}, err
	}
	defer done()
	return s.core.kube.ReadRelatedReplicaSets(ctx, req)
}

func (s *ExplorerService) ReadRelatedJobs(id string, req kube.RelatedRequest) (kube.RelatedJobPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.RelatedJobPage{}, err
	}
	defer done()
	return s.core.kube.ReadRelatedJobs(ctx, req)
}

func (s *ExplorerService) ReadResourceEvents(id string, req kube.EventRequest) (kube.EventPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.EventPage{}, err
	}
	defer done()
	return s.core.kube.ReadResourceEvents(ctx, req)
}

func (s *ExplorerService) OpenPodLogs(req kube.LogRequest) (model.SessionRef, error) {
	return s.core.kube.OpenPodLogs(s.core.ctx, req)
}
func (s *ExplorerService) ClosePodLogs(ref model.SessionRef) error {
	return s.core.kube.ClosePodLogs(ref)
}

func (s *ExplorerService) ReadNetworking(id string, req kube.PageRequest) (kube.NetworkPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.NetworkPage{}, err
	}
	defer done()
	return s.core.kube.ReadNetworking(ctx, req)
}

func (s *ExplorerService) ReadServiceEndpointSlices(id string, req kube.RelatedRequest) (kube.ServiceSlicePage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ServiceSlicePage{}, err
	}
	defer done()
	return s.core.kube.ReadServiceEndpointSlices(ctx, req)
}

func (s *ExplorerService) ResolveNetworkReference(id string, req kube.NetworkReferenceRequest) (kube.ResolvedNetworkReference, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ResolvedNetworkReference{}, err
	}
	defer done()
	return s.core.kube.ResolveNetworkReference(ctx, req)
}

func (s *ExplorerService) ReadConfiguration(id string, req kube.PageRequest) (kube.ConfigurationPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ConfigurationPage{}, err
	}
	defer done()
	return s.core.kube.ReadConfiguration(ctx, req)
}

func (s *ExplorerService) ResolveConfigurationReference(id string, req kube.ConfigurationReferenceRequest) (kube.ResolvedConfigurationReference, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ResolvedConfigurationReference{}, err
	}
	defer done()
	return s.core.kube.ResolveConfigurationReference(ctx, req)
}

func (s *ExplorerService) ReadStorage(id string, req kube.PageRequest) (kube.StoragePage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.StoragePage{}, err
	}
	defer done()
	return s.core.kube.ReadStorage(ctx, req)
}

func (s *ExplorerService) ResolveStorageReference(id string, req kube.StorageReferenceRequest) (kube.ResolvedStorageReference, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ResolvedStorageReference{}, err
	}
	defer done()
	return s.core.kube.ResolveStorageReference(ctx, req)
}

func (s *ExplorerService) ReadAccessControl(id string, req kube.PageRequest) (kube.AccessControlPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.AccessControlPage{}, err
	}
	defer done()
	return s.core.kube.ReadAccessControl(ctx, req)
}

func (s *ExplorerService) ResolveAccessControlReference(id string, req kube.AccessControlReferenceRequest) (kube.ResolvedAccessControlReference, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ResolvedAccessControlReference{}, err
	}
	defer done()
	return s.core.kube.ResolveAccessControlReference(ctx, req)
}

func (s *ExplorerService) ReadAvailability(id string, req kube.PageRequest) (kube.AvailabilityPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.AvailabilityPage{}, err
	}
	defer done()
	return s.core.kube.ReadAvailability(ctx, req)
}

func (s *ExplorerService) ReadPDBSelectedPods(id string, req kube.RelatedRequest) (kube.SelectedPodPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.SelectedPodPage{}, err
	}
	defer done()
	return s.core.kube.ReadPDBSelectedPods(ctx, req)
}

func (s *ExplorerService) ReadControllerRevisions(id string, req kube.RelatedRequest) (kube.ControllerRevisionPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ControllerRevisionPage{}, err
	}
	defer done()
	return s.core.kube.ReadControllerRevisions(ctx, req)
}

func (s *ExplorerService) ResolveAvailabilityReference(id string, req kube.AvailabilityReferenceRequest) (kube.ResolvedAvailabilityReference, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ResolvedAvailabilityReference{}, err
	}
	defer done()
	return s.core.kube.ResolveAvailabilityReference(ctx, req)
}

func (s *ExplorerService) ReadClusterResources(id string, req kube.PageRequest) (kube.ClusterResourcePage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ClusterResourcePage{}, err
	}
	defer done()
	return s.core.kube.ReadClusterResources(ctx, req)
}

func (s *ExplorerService) ReadEventInventory(id string, req kube.PageRequest) (kube.EventInventoryPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.EventInventoryPage{}, err
	}
	defer done()
	return s.core.kube.ReadEventInventory(ctx, req)
}

func (s *ExplorerService) ResolveEventTarget(id string, req kube.EventTargetRequest) (kube.ResolvedEventTarget, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.ResolvedEventTarget{}, err
	}
	defer done()
	return s.core.kube.ResolveEventTarget(ctx, req)
}

func (s *ExplorerService) ReadAPIGroups(id string, req kube.APIGroupRequest) (kube.APIGroupCatalog, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.APIGroupCatalog{}, err
	}
	defer done()
	return s.core.kube.ReadAPIGroups(ctx, req.Connection, req.Refresh)
}

func (s *ExplorerService) ReadAPIResources(id string, req kube.APIResourceRequest) (kube.APIResourceCatalog, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.APIResourceCatalog{}, err
	}
	defer done()
	return s.core.kube.ReadAPIResources(ctx, req.Connection, req.Group, req.Version, req.Refresh)
}

func (s *ExplorerService) ReadGenericResources(id string, req kube.GenericPageRequest) (kube.GenericResourcePage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.GenericResourcePage{}, err
	}
	defer done()
	return s.core.kube.ReadGenericResources(ctx, req)
}

func (s *ExplorerService) ReadGenericDetail(id string, req kube.GenericDetailRequest) (kube.GenericResourceDetail, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.GenericResourceDetail{}, err
	}
	defer done()
	return s.core.kube.ReadGenericDetail(ctx, req)
}
