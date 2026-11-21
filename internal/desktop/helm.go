/*
Purpose: Expose cancellable Helm inspection to the desktop.
Used by: Generated ExplorerService bindings.
Responsibilities: Forward explicit release and resource identities to the native readers.
Important: The bridge does not own storage or establish release identity.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/helmreader"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *ExplorerService) ListHelmReleases(id string, req helmreader.ListRequest) (helmreader.ReleasePage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return helmreader.ReleasePage{}, err
	}
	defer done()
	return s.core.helm.ListReleases(ctx, req)
}
func (s *ExplorerService) ReadHelmHistory(id string, req helmreader.HistoryRequest) (helmreader.HistoryPage, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return helmreader.HistoryPage{}, err
	}
	defer done()
	return s.core.helm.ReadHistory(ctx, req)
}
func (s *ExplorerService) ReadHelmSection(id string, req helmreader.DetailRequest) (helmreader.ReleaseSection, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return helmreader.ReleaseSection{}, err
	}
	defer done()
	return s.core.helm.ReadSection(ctx, req)
}
func (s *ExplorerService) OpenHelmResource(id string, req helmreader.OpenResourceRequest) (helmreader.CurrentResource, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return helmreader.CurrentResource{}, err
	}
	defer done()
	return s.core.helm.OpenResource(ctx, req)
}
func (s *ExplorerService) ReadHelmCurrentDetail(id string, req helmreader.OpenResourceRequest, uid string) (kube.GenericResourceDetail, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.GenericResourceDetail{}, err
	}
	defer done()
	return s.core.helm.ReadCurrentDetail(ctx, req, uid)
}
func (s *ExplorerService) CloseHelmScope(ref model.ConnectionRef, namespace, driver string) {
	s.core.helm.CloseScope(ref, namespace, driver)
}
