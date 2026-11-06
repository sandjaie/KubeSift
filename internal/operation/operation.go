/*
Purpose: Bind the validation ConfigMap write to an immutable reviewed target.
Used by: The desktop adapter and the opt-in local integration scenario.
Responsibilities: Read identity, issue bounded five-minute tokens, consume once and write with UID/version checks.
Important: Every request uses guarded connections; an uncertain write is never retried automatically.
*/
// Package operation owns prepare/review/execute for the Phase 1A marker experiment.
package operation

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/model"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

type Preview struct {
	Token           string       `json:"token"`
	Target          model.Target `json:"target"`
	OldValue        string       `json:"oldValue"`
	NewValue        string       `json:"newValue"`
	UID             string       `json:"uid"`
	ResourceVersion string       `json:"resourceVersion"`
	ExpiresAt       time.Time    `json:"expiresAt"`
}

type Service struct {
	mu      sync.Mutex
	pending map[string]Preview
	now     func() time.Time
	client  func(model.ConnectionRef) (kubernetes.Interface, error)
	write   func(context.Context, kubernetes.Interface, model.Target, *corev1.ConfigMap) error
}

func NewService(connections *connection.Manager, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{pending: map[string]Preview{}, now: now, client: connections.Client, write: writeOnce}
}
func (s *Service) Prepare(ctx context.Context, target model.Target, value string) (Preview, error) {
	if target.Name != "ks-marker" || target.Container != "" || len(validation.IsDNS1123Label(target.Namespace)) != 0 || len(value) > 256 {
		return Preview{}, model.Fail("invalid", "Select the validation marker and a value of at most 256 bytes")
	}
	client, err := s.client(target.Connection)
	if err != nil {
		return Preview{}, err
	}
	cm, err := client.CoreV1().ConfigMaps(target.Namespace).Get(ctx, target.Name, metav1.GetOptions{})
	if err != nil {
		return Preview{}, safeError(err, false)
	}
	if cm.UID == "" || cm.ResourceVersion == "" || len(cm.Data["marker"]) > 256 {
		return Preview{}, model.Fail("invalid", "Marker identity or value is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for token, p := range s.pending {
		if !now.Before(p.ExpiresAt) {
			delete(s.pending, token)
		}
	}
	if len(s.pending) >= 128 {
		return Preview{}, model.Fail("busy", "Too many pending reviews; wait for expiry")
	}
	preview := Preview{Token: model.ID(), Target: target, OldValue: cm.Data["marker"], NewValue: value, UID: string(cm.UID), ResourceVersion: cm.ResourceVersion, ExpiresAt: now.Add(5 * time.Minute)}
	s.pending[preview.Token] = preview
	return preview, nil
}
func (s *Service) Execute(ctx context.Context, token string) error {
	s.mu.Lock()
	preview, ok := s.pending[token]
	delete(s.pending, token)
	now := s.now()
	s.mu.Unlock()
	if !ok {
		return model.Fail("invalid_token", "Prepare a new review; token is unknown or already used")
	}
	if !now.Before(preview.ExpiresAt) {
		return model.Fail("expired", "Review expired; prepare again")
	}
	client, err := s.client(preview.Target.Connection)
	if err != nil {
		return err
	}
	cm, err := client.CoreV1().ConfigMaps(preview.Target.Namespace).Get(ctx, preview.Target.Name, metav1.GetOptions{})
	if err != nil {
		return safeError(err, false)
	}
	if string(cm.UID) != preview.UID || cm.ResourceVersion != preview.ResourceVersion {
		return model.Fail("stale_target", "Marker changed; prepare a new review")
	}
	if ctx.Err() != nil {
		return model.Fail("cancelled", "Write cancelled before execution")
	}
	updated := cm.DeepCopy()
	if updated.Data == nil {
		updated.Data = map[string]string{}
	}
	updated.Data["marker"] = preview.NewValue
	return safeError(s.write(ctx, client, preview.Target, updated), true)
}
func writeOnce(ctx context.Context, client kubernetes.Interface, target model.Target, cm *corev1.ConfigMap) error {
	// Keep both metadata preconditions in the update; suppress client-go's HTTP retries.
	return client.CoreV1().RESTClient().Put().Namespace(target.Namespace).Resource("configmaps").Name(target.Name).Body(cm).MaxRetries(0).Do(ctx).Error()
}
func safeError(err error, writing bool) error {
	if err == nil {
		return nil
	}
	var safe *model.Error
	if errors.As(err, &safe) {
		return safe
	}
	switch {
	case apierrors.IsConflict(err), apierrors.IsNotFound(err):
		return model.Fail("stale_target", "Marker changed or disappeared; prepare again")
	case apierrors.IsTooManyRequests(err):
		return model.Fail("busy", "Kubernetes rejected the request due to rate limiting; prepare again")
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return model.Fail("forbidden", "Kubernetes denied the marker request")
	case apierrors.IsInvalid(err), apierrors.IsBadRequest(err), apierrors.IsMethodNotSupported(err):
		return model.Fail("invalid", "Kubernetes rejected the marker request")
	}
	if writing {
		return model.Fail("uncertain", "Write outcome is uncertain; inspect the marker and prepare again")
	}
	return model.Fail("disconnected", "Cannot read the marker from the selected connection")
}
