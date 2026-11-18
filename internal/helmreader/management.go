/*
Purpose: Share bounded stored evidence with native release management.
Used by: Helm review and execution identity checks.
Responsibilities: Capture a complete current head and read immutable revision records.
Important: These helpers reuse the read-only adapter; they never enable writes.
*/
package helmreader

import (
	"context"
	"encoding/json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"

	"github.com/sandjaie/KubeSift/internal/model"
	stored "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/client-go/kubernetes"
)

func CaptureHead(ctx context.Context, client kubernetes.Interface, scope Scope) (*RecordRef, *stored.Release, error) {
	if err := validateScope(scope); err != nil || !validReleaseName(scope.ReleaseName) {
		return nil, nil, model.Fail("invalid", "Choose a valid namespace, driver and release name.")
	}
	items, err := scan(ctx, client, scope)
	if err != nil {
		return nil, nil, err
	}
	s := &snapshot{scope: scope}
	s.collect(items)
	if len(s.latest) == 0 {
		return nil, nil, nil
	}
	row := s.latest[0]
	if row.Problem != nil || len(s.latest) != 1 {
		return nil, nil, model.Fail("invalid_response", "Cannot establish an unambiguous current release.")
	}
	r, err := ReadCaptured(ctx, client, row.Record)
	return &row.Record, r, err
}

func ReadCaptured(ctx context.Context, client kubernetes.Interface, ref RecordRef) (*stored.Release, error) {
	r, err := readRelease(ctx, client, ref, newBudget())
	if err != nil {
		return nil, err
	}
	v, ok := r.(*stored.Release)
	if !ok {
		return nil, model.Fail("unsupported", "Unsupported stored release format.")
	}
	return v, nil
}

// VerifyPayload checks expansion and identity before a management driver decodes a record.
func VerifyPayload(ctx context.Context, meta metav1.ObjectMeta, payload string) error {
	return PayloadVerifier()(ctx, meta, payload)
}

// PayloadVerifier shares the inspection aggregate budget across one SDK query.
func PayloadVerifier() func(context.Context, metav1.ObjectMeta, string) error {
	budget := newBudget()
	return func(ctx context.Context, meta metav1.ObjectMeta, payload string) error {
		ref := RecordRef{Scope: Scope{Namespace: meta.Namespace, ReleaseName: meta.Labels["name"]}, RecordName: meta.Name, Revision: meta.Labels["version"], UID: string(meta.UID), ResourceVersion: meta.ResourceVersion}
		if !ref.valid() || !ref.matches(meta) {
			return corruptRow()
		}
		raw, err := budget.decode(ctx, payload)
		if err != nil {
			return err
		}
		var identity struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Version   int    `json:"version"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Name != ref.ReleaseName || identity.Namespace != ref.Namespace || strconv.Itoa(identity.Version) != ref.Revision {
			return model.Fail("invalid_response", "Stored release identity does not match its record.")
		}
		return nil
	}
}
