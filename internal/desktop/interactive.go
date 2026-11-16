/*
Purpose: Bridge product terminal and port forwarding sessions to the explorer.
Used by: Generated ExplorerService bindings.
Responsibilities: Forward typed session control and cancellable setup requests.
Important: Interactive transports and payloads remain in the native service.
*/
package desktop

import "github.com/sandjaie/KubeSift/internal/model"

func (s *ExplorerService) StopInteractive(ref model.SessionRef) error {
	return s.core.kube.StopInteractive(ref)
}

func (s *ExplorerService) CloseInteractive(ref model.SessionRef) error {
	return s.core.kube.CloseInteractive(ref)
}
