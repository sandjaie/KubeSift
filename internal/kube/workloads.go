/*
Purpose: Read one namespace-scoped page of Pods or Deployments.
Used by: The workload bridge and related-Pod projection.
Responsibilities: Bind bounded cursors to query scope and preserve optional status evidence.
Important: Continuation is not authorization; every call validates its connection generation.
*/
package kube

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/sandjaie/KubeSift/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

const cursorLimit = 16 << 10

type pageCursor struct {
	Version    int                 `json:"v"`
	Connection model.ConnectionRef `json:"connection"`
	Kind       string              `json:"kind"`
	Namespace  string              `json:"namespace"`
	Query      string              `json:"query"`
	Token      string              `json:"token"`
}

func decodeCursor(encoded string, scope pageCursor) (string, error) {
	if encoded == "" {
		return "", nil
	}
	bad := model.Fail("list_changed", "List changed; refresh")
	if len(encoded) > cursorLimit {
		return "", bad
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", bad
	}
	var c pageCursor
	if json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Connection != scope.Connection || c.Kind != scope.Kind || c.Namespace != scope.Namespace || c.Query != scope.Query || c.Token == "" {
		return "", bad
	}
	return c.Token, nil
}
func encodeCursor(scope pageCursor, token string) (string, error) {
	if token == "" {
		return "", nil
	}
	scope.Version = 1
	scope.Token = token
	raw, err := json.Marshal(scope)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > cursorLimit {
		return "", model.Fail("capacity", "Page cursor exceeds its size limit")
	}
	return encoded, nil
}
func readError(ctx context.Context, err error) error {
	if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) {
		return model.Fail("list_changed", "List changed; refresh")
	}
	if apierrors.IsNotFound(err) {
		return model.Fail("not_found", "Resource not found")
	}
	return safeError(ctx, err)
}
func resourceRef(c model.ConnectionRef, kind, version string, m metav1.ObjectMeta) ResourceRef {
	return ResourceRef{Connection: c, Kind: kind, APIVersion: version, Namespace: m.Namespace, Name: m.Name, UID: string(m.UID)}
}
func created(m metav1.ObjectMeta) *time.Time {
	if m.CreationTimestamp.IsZero() {
		return nil
	}
	v := m.CreationTimestamp.Time
	return &v
}
func podRow(c model.ConnectionRef, p corev1.Pod) WorkloadRow {
	row := WorkloadRow{Resource: resourceRef(c, "Pod", "v1", p.ObjectMeta), Phase: string(p.Status.Phase), Node: p.Spec.NodeName, CreatedAt: created(p.ObjectMeta)}
	if len(p.Spec.Containers) > 0 {
		n := int32(len(p.Spec.Containers))
		row.Desired = &n
	}
	if len(p.Status.ContainerStatuses) > 0 {
		var ready, restarts int32
		for _, s := range p.Status.ContainerStatuses {
			if s.Ready {
				ready++
			}
			restarts += s.RestartCount
		}
		row.Ready = &ready
		row.Restarts = &restarts
	}
	statuses := append(append([]corev1.ContainerStatus{}, p.Status.InitContainerStatuses...), p.Status.ContainerStatuses...)
	for _, s := range statuses {
		if s.State.Waiting != nil && s.State.Waiting.Reason != "" {
			row.Reason = s.State.Waiting.Reason
			break
		}
		if s.State.Terminated != nil && s.State.Terminated.ExitCode != 0 {
			row.Reason = s.State.Terminated.Reason
			break
		}
	}
	return row
}
func deploymentRow(c model.ConnectionRef, d appsv1.Deployment) WorkloadRow {
	row := WorkloadRow{Resource: resourceRef(c, "Deployment", "apps/v1", d.ObjectMeta), Desired: d.Spec.Replicas, CreatedAt: created(d.ObjectMeta)}
	if d.Status.ObservedGeneration != 0 || len(d.Status.Conditions) > 0 {
		row.Ready = &d.Status.ReadyReplicas
		row.Updated = &d.Status.UpdatedReplicas
		row.Available = &d.Status.AvailableReplicas
	}
	return row
}
func (s *Service) ReadWorkloads(ctx context.Context, req PageRequest) (WorkloadPage, error) {
	result := WorkloadPage{Items: []WorkloadRow{}}
	if err := namespace(req.Namespace); err != nil {
		return result, err
	}
	if req.Kind != "Pod" && req.Kind != "Deployment" {
		return result, model.Fail("invalid", "Choose Pods or Deployments")
	}
	scope := pageCursor{Connection: req.Connection, Kind: req.Kind, Namespace: req.Namespace}
	token, err := decodeCursor(req.Cursor, scope)
	if err != nil {
		return result, err
	}
	client, err := s.boundedReadClient(ctx, req.Connection)
	if err != nil {
		return result, err
	}
	opts := metav1.ListOptions{Limit: 100, Continue: token}
	var meta metav1.ListMeta
	if req.Kind == "Pod" {
		list, e := client.CoreV1().Pods(req.Namespace).List(ctx, opts)
		if e != nil {
			return result, readError(ctx, e)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		meta = list.ListMeta
		for _, p := range list.Items {
			result.Items = append(result.Items, podRow(req.Connection, p))
		}
	} else {
		list, e := client.AppsV1().Deployments(req.Namespace).List(ctx, opts)
		if e != nil {
			return result, readError(ctx, e)
		}
		if len(list.Items) > 100 {
			return result, model.Fail("capacity", "Server exceeded page size")
		}
		meta = list.ListMeta
		for _, d := range list.Items {
			result.Items = append(result.Items, deploymentRow(req.Connection, d))
		}
	}
	next, err := encodeCursor(scope, meta.Continue)
	if err != nil {
		return result, err
	}
	result.Page = PageInfo{NextCursor: next, ResourceVersion: meta.ResourceVersion, FetchedAt: time.Now().UTC()}
	return result, s.connections.Validate(req.Connection)
}
