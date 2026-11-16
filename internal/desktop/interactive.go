/*
Purpose: Bridge product terminal and port forwarding sessions to the explorer.
Used by: Generated ExplorerService bindings.
Responsibilities: Forward typed session control and cancellable setup requests.
Important: Interactive transports and payloads remain in the native service.
*/
package desktop

import (
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
)

func (s *ExplorerService) StopInteractive(ref model.SessionRef) error {
	return s.core.kube.StopInteractive(ref)
}

func (s *ExplorerService) CloseInteractive(ref model.SessionRef) error {
	return s.core.kube.CloseInteractive(ref)
}

func (s *ExplorerService) OpenTerminal(id string, req kube.TerminalRequest) (kube.InteractiveEntry, error) {
	ctx, done, err := s.beginWorkloadRead(id)
	if err != nil {
		return kube.InteractiveEntry{}, err
	}
	defer done()
	return s.core.kube.OpenTerminal(ctx, req)
}
