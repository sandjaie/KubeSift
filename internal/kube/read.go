/*
Purpose: Read only the Kubernetes resources explicitly requested by a caller.
Used by: The desktop adapter and local integration scenario.
Responsibilities: Validate targets, build connection-bound clients and project safe Pod summaries.
Important: Legacy summaries omit full specs; investigation reads separately cap local payloads. Credentials stay native.
*/
package kube

import (
	"context"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"sync"
)

type Service struct {
	connections *connection.Manager
	sessions    *session.Manager
	mu          sync.Mutex
	terminals   map[string]*terminal
	productLogs map[string]model.SessionRef
}

func NewService(c *connection.Manager, s *session.Manager) *Service {
	s.SetValidate(func(t model.Target) error { return c.Validate(t.Connection) })
	service := &Service{connections: c, sessions: s, terminals: map[string]*terminal{}, productLogs: map[string]model.SessionRef{}}
	c.SetCleanup(service.closeConnectionLogs)
	return service
}

type PodSummary struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	UID        string   `json:"uid"`
	Phase      string   `json:"phase"`
	Ready      bool     `json:"ready"`
	Containers []string `json:"containers"`
	Marker     string   `json:"marker"`
	Annotation string   `json:"annotation"`
}

func limited(text string) string {
	if len(text) > 4096 {
		return text[:4096] + " [truncated]"
	}
	return text
}
func Summarize(pod corev1.Pod) PodSummary {
	result := PodSummary{Name: pod.Name, Namespace: pod.Namespace, UID: string(pod.UID), Phase: string(pod.Status.Phase), Containers: []string{}, Marker: limited(pod.Annotations["kubesift.dev/marker"]), Annotation: limited(pod.Annotations["kubesift.dev/hostile"])}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			result.Ready = condition.Status == corev1.ConditionTrue
		}
	}
	for _, container := range pod.Spec.Containers {
		if len(result.Containers) >= 32 {
			break
		}
		result.Containers = append(result.Containers, container.Name)
	}
	return result
}
func namespace(name string) error {
	if len(validation.IsDNS1123Label(name)) != 0 {
		return model.Fail("invalid", "Explicit valid namespace required")
	}
	return nil
}
func target(t model.Target, container bool) error {
	if err := namespace(t.Namespace); err != nil {
		return err
	}
	if len(validation.IsDNS1123Subdomain(t.Name)) != 0 {
		return model.Fail("invalid", "Explicit valid Pod name required")
	}
	if container && len(validation.IsDNS1123Label(t.Container)) != 0 {
		return model.Fail("invalid", "Explicit valid container required")
	}
	return nil
}
func safeError(ctx context.Context, err error) error {
	if problem := ClassifyError(ctx, err); problem != nil {
		return problem
	}
	return nil
}
func (s *Service) Pods(ctx context.Context, ref model.ConnectionRef, ns string) ([]corev1.Pod, error) {
	if err := namespace(ns); err != nil {
		return nil, err
	}
	client, err := s.connections.Client(ref)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{Limit: 100})
	if err != nil {
		return nil, safeError(ctx, err)
	}
	if list.Continue != "" {
		return nil, model.Fail("capacity", "Pod lists above 100 resources are not supported yet")
	}
	return list.Items, nil
}
func (s *Service) streamClient(ctx context.Context, ref model.ConnectionRef) (kubernetes.Interface, error) {
	cfg, err := s.connections.StreamConfig(ctx, ref)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot create streaming client")
	}
	return client, nil
}
