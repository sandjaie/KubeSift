/*
Purpose: Resolve a captured Service TCP port to a selectable fixed Pod.
Used by: The product forward picker and native start validation.
Responsibilities: Check the Service version, selector, Pod UID and numeric or named target port.
Important: Candidates describe selector matches, not health or Service routing.
*/
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

func tcpPort(protocol core.Protocol) bool {
	return protocol == "" || protocol == core.ProtocolTCP
}

func validPort(port int32) bool { return port > 0 && port <= 65535 }

func resolveForwardPort(port core.ServicePort, pod *core.Pod) (uint16, error) {
	if !tcpPort(port.Protocol) || !validPort(port.Port) {
		return 0, model.Fail("unsupported", "Only TCP Service ports can be forwarded")
	}
	if port.TargetPort.Type == intstr.Int {
		target := port.TargetPort.IntVal
		if target == 0 {
			target = port.Port
		}
		if validPort(target) {
			return uint16(target), nil
		}
	}
	if port.TargetPort.Type == intstr.String && port.TargetPort.StrVal != "" {
		var resolved int32
		for _, group := range [][]core.Container{pod.Spec.Containers, pod.Spec.InitContainers} {
			for _, container := range group {
				for _, declared := range container.Ports {
					if declared.Name != port.TargetPort.StrVal || !tcpPort(declared.Protocol) {
						continue
					}
					if !validPort(declared.ContainerPort) || resolved != 0 && resolved != declared.ContainerPort {
						return 0, model.Fail("ambiguous_port", "The selected Pod has conflicting named TCP ports")
					}
					resolved = declared.ContainerPort
				}
			}
		}
		if resolved != 0 {
			return uint16(resolved), nil
		}
	}
	return 0, model.Fail("unavailable_port", "The selected Pod does not declare the Service's named TCP port")
}

func readForwardService(ctx context.Context, client kubernetes.Interface, selection ServiceForwardSelection) (labels.Selector, core.ServicePort, error) {
	ref := selection.Resource
	if ref.Kind != "Service" || ref.APIVersion != "v1" || selection.ResourceVersion == "" || len(selection.ResourceVersion) > 128 ||
		!validPort(selection.Port) || len(selection.PortName) > 63 {
		return nil, core.ServicePort{}, model.Fail("invalid", "Select a captured Service TCP port")
	}
	object, err := checkedObject(ctx, client, ref)
	if err != nil {
		return nil, core.ServicePort{}, err
	}
	svc := object.(*core.Service)
	if svc.ResourceVersion != selection.ResourceVersion || svc.DeletionTimestamp != nil {
		return nil, core.ServicePort{}, model.Fail("stale_target", "Service changed. Refresh it before starting a forward.")
	}
	if svc.Spec.Type == core.ServiceTypeExternalName || len(svc.Spec.Selector) == 0 {
		return nil, core.ServicePort{}, model.Fail("unsupported", "This Service has no selectable Pod. Forward a Pod directly instead.")
	}
	selector := labels.SelectorFromSet(svc.Spec.Selector)
	for _, port := range svc.Spec.Ports {
		if port.Name == selection.PortName && port.Port == selection.Port && tcpPort(port.Protocol) {
			return selector, port, nil
		}
	}
	return nil, core.ServicePort{}, model.Fail("unsupported", "Only a captured TCP Service port can be forwarded")
}

func (s *Service) ReadForwardCandidates(ctx context.Context, req ForwardCandidateRequest) (ForwardCandidatePage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ref := req.Service
	client, err := s.boundedReadClient(ctx, ref.Connection)
	if err != nil {
		return ForwardCandidatePage{}, err
	}
	selection := ServiceForwardSelection{Resource: ref, ResourceVersion: req.ServiceVersion, PortName: req.PortName, Port: req.Port}
	selector, port, err := readForwardService(ctx, client, selection)
	if err != nil {
		return ForwardCandidatePage{}, err
	}
	scope := pageCursor{Connection: ref.Connection, Namespace: ref.Namespace, Kind: "Service/Forward",
		Query: fmt.Sprintf("%q/%q/%q/%d/%q", ref.UID, req.ServiceVersion, req.PortName, req.Port, selector.String())}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return ForwardCandidatePage{}, err
	}
	api := apiResource{"", "v1", "pods", "Pod", true}
	list, err := listJSON(ctx, api.read(client, ref.Namespace, "").Param("labelSelector", selector.String()), api, token, true)
	if err != nil {
		return ForwardCandidatePage{}, readError(ctx, err)
	}
	cursor, err := encodeCursor(scope, list.Metadata.Continue)
	if err != nil {
		return ForwardCandidatePage{}, err
	}
	result := ForwardCandidatePage{Items: []ForwardCandidate{}, Complete: cursor == "",
		Page: PageInfo{FetchedAt: time.Now().UTC(), ResourceVersion: list.Metadata.ResourceVersion, NextCursor: cursor}}
	for _, raw := range list.Items {
		if _, err := objectIdentity(raw, api, ref.Namespace, true); err != nil {
			return ForwardCandidatePage{}, err
		}
		var pod core.Pod
		if err := json.Unmarshal(raw, &pod); err != nil {
			return ForwardCandidatePage{}, model.Fail("invalid_response", "Kubernetes returned an invalid Pod")
		}
		if pod.Status.Phase != core.PodRunning || pod.DeletionTimestamp != nil || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		backend := resourceRef(ref.Connection, "Pod", "v1", pod.ObjectMeta)
		if err := validResource(backend); err != nil || pod.ResourceVersion == "" || len(pod.ResourceVersion) > 128 {
			return ForwardCandidatePage{}, model.Fail("invalid_response", "Kubernetes returned an invalid Pod identity")
		}
		if remote, err := resolveForwardPort(port, &pod); err == nil {
			result.Items = append(result.Items, ForwardCandidate{Pod: backend, PodVersion: pod.ResourceVersion, RemotePort: remote})
		}
	}
	return result, s.finishAvailability(ctx, ref.Connection, result)
}

func (s *Service) verifyForwardMapping(ctx context.Context, req ForwardRequest) error {
	if req.RemotePort == 0 {
		return model.Fail("invalid", "An explicit numeric remote TCP port is required")
	}
	pod, err := s.interactivePod(ctx, req.Target, false)
	if err != nil || req.Service == nil {
		return err
	}
	ref := req.Target.Resource
	if req.Service.Resource.Connection != ref.Connection || req.Service.Resource.Namespace != ref.Namespace {
		return model.Fail("invalid", "Select a Pod in the Service namespace and connection")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := s.boundedReadClient(ctx, ref.Connection)
	if err != nil {
		return err
	}
	selector, port, err := readForwardService(ctx, client, *req.Service)
	if err != nil {
		return err
	}
	if !selector.Matches(labels.Set(pod.Labels)) {
		return model.Fail("stale_target", "The selected Pod no longer matches the Service selector")
	}
	resolved, err := resolveForwardPort(port, pod)
	if err != nil {
		return err
	}
	if resolved != req.RemotePort {
		return model.Fail("stale_target", "The selected Pod TCP port changed. Select a current candidate.")
	}
	return nil
}

func (s *Service) OpenForward(ctx context.Context, req ForwardRequest) (InteractiveEntry, error) {
	if req.Service != nil {
		captured := *req.Service
		req.Service = &captured
	}
	if err := s.verifyForwardMapping(ctx, req); err != nil {
		return InteractiveEntry{}, err
	}
	result := InteractiveEntry{Kind: "forward", Target: req.Target, Service: req.Service, RemotePort: req.RemotePort}
	return s.reserveInteractive(ctx, result, func(ctx context.Context, _ *session.Writer, ready func(uint16) error) error {
		// Recheck on activation. A Service may change while this session is pending.
		if err := s.verifyForwardMapping(ctx, req); err != nil {
			return err
		}
		verified := func(uint16) error {
			_, err := s.interactivePod(ctx, req.Target, false)
			return err
		}
		return s.runForward(ctx, transportTarget(req.Target), req.LocalPort, req.RemotePort, verified, ready)
	})
}
