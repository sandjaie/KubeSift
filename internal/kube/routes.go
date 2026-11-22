/*
Purpose: Project safe OpenShift Route fields without transmitting embedded TLS material.
Used by: Route inventory, inspection and the installed API browser.
Responsibilities: Preserve backend weights, port type and independent router observations.
Important: Projection only decodes allowlisted fields and never opens a host or resolves a Secret.
*/
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"time"
)

type RouteBackend struct {
	Kind   string `json:"kind,omitempty"`
	Name   string `json:"name"`
	Weight *int32 `json:"weight,omitempty"`
}
type RouteObservation struct {
	Router     string           `json:"routerName"`
	Host       string           `json:"host"`
	Conditions []RouteCondition `json:"conditions,omitempty"`
}
type RouteCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}
type RouteFields struct {
	Host              string         `json:"host,omitempty"`
	Path              string         `json:"path,omitempty"`
	To                RouteBackend   `json:"to"`
	AlternateBackends []RouteBackend `json:"alternateBackends,omitempty"`
	Port              *struct {
		TargetPort intstr.IntOrString `json:"targetPort"`
	} `json:"port,omitempty"`
	TLS *struct {
		Termination         string `json:"termination"`
		Policy              string `json:"insecureEdgeTerminationPolicy,omitempty"`
		ExternalCertificate *struct {
			Name string `json:"name"`
		} `json:"externalCertificate,omitempty"`
	} `json:"tls,omitempty"`
}
type routeProjection struct {
	Spec   RouteFields `json:"spec"`
	Status struct {
		Ingress []RouteObservation `json:"ingress,omitempty"`
	} `json:"status"`
}

func ProjectRoute(raw []byte) (routeProjection, error) {
	var route routeProjection
	if json.Unmarshal(raw, &route) != nil || len(route.Status.Ingress) > 500 || len(route.Spec.AlternateBackends) > 3 {
		return routeProjection{}, model.Fail("invalid", "Invalid or oversized Route evidence")
	}
	for _, r := range route.Status.Ingress {
		if len(r.Conditions) > 100 {
			return routeProjection{}, model.Fail("capacity", "Router conditions exceed the display limit")
		}
	}
	return route, nil
}

type RouteService struct {
	Resource ResourceRef `json:"resource"`
	Type     string      `json:"type"`
	Ports    []string    `json:"ports"`
}

func (s *Service) ReadRouteService(ctx context.Context, q WorkflowQuery, name string) (RouteService, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if q.Kind != "Route" || q.Name == "" || q.UID == "" || !validVersion(name) {
		return RouteService{}, model.Fail("invalid", "Select a captured Route backend")
	}
	verify := func() error {
		page, err := s.ReadWorkflow(ctx, q)
		if err != nil {
			return err
		}
		var detail routeProjection
		if len(page.Items) != 1 || json.Unmarshal([]byte(page.Items[0].Fields), &detail) != nil {
			return model.Fail("invalid", "Invalid Route detail")
		}
		for _, b := range append(detail.Spec.AlternateBackends, detail.Spec.To) {
			if (b.Kind == "Service" || b.Kind == "") && b.Name == name {
				return nil
			}
		}
		return model.Fail("stale_target", "Route no longer references this Service")
	}
	if err := verify(); err != nil {
		return RouteService{}, err
	}
	client, err := s.boundedReadClient(ctx, q.Connection)
	if err != nil {
		return RouteService{}, err
	}
	svc, err := client.CoreV1().Services(q.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return RouteService{}, readError(ctx, err)
	}
	if svc.Name != name || svc.Namespace != q.Namespace || svc.UID == "" {
		return RouteService{}, model.Fail("invalid", "Invalid Service identity")
	}
	out := RouteService{Resource: ResourceRef{Connection: q.Connection, Namespace: q.Namespace, Name: name, UID: string(svc.UID), Kind: "Service", APIVersion: "v1"}, Type: string(svc.Spec.Type)}
	for _, p := range svc.Spec.Ports {
		out.Ports = append(out.Ports, fmt.Sprintf("%s %d/%s", p.Name, p.Port, p.Protocol))
	}
	if err := checkPayload(out); err != nil {
		return RouteService{}, err
	}
	return out, verify()
}
