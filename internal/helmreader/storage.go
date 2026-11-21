/*
Purpose: Read Helm storage records through a request-local SDK driver.
Used by: Release summary and revision inspection.
Responsibilities: Enforce immutable record identity, request cancellation and payload bounds.
Important: The SDK receives only a verified copy and has no storage write implementation.
*/
package helmreader

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/release"
	"helm.sh/helm/v4/pkg/storage/driver"
	core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	apply "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
)

type storageGetter[T any, L any, A any] struct {
	get func(string, metav1.GetOptions) (*T, error)
}

var readOnly = model.Fail("unsupported", "Helm storage supports Get only.")

func (s storageGetter[T, L, A]) Get(_ context.Context, name string, options metav1.GetOptions) (*T, error) {
	return s.get(name, options)
}
func (storageGetter[T, L, A]) Create(context.Context, *T, metav1.CreateOptions) (*T, error) {
	return nil, readOnly
}
func (storageGetter[T, L, A]) Update(context.Context, *T, metav1.UpdateOptions) (*T, error) {
	return nil, readOnly
}
func (storageGetter[T, L, A]) Delete(context.Context, string, metav1.DeleteOptions) error {
	return readOnly
}
func (storageGetter[T, L, A]) DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error {
	return readOnly
}
func (storageGetter[T, L, A]) List(context.Context, metav1.ListOptions) (*L, error) {
	return nil, readOnly
}
func (storageGetter[T, L, A]) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, readOnly
}
func (storageGetter[T, L, A]) Patch(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*T, error) {
	return nil, readOnly
}
func (storageGetter[T, L, A]) Apply(context.Context, *A, metav1.ApplyOptions) (*T, error) {
	return nil, readOnly
}

func revisionOK(value string) bool {
	return len(value) > 0 && len(value) <= 128 && value[0] != '0' && strings.Trim(value, "0123456789") == ""
}

func (ref RecordRef) valid() bool {
	return validReleaseName(ref.ReleaseName) && revisionOK(ref.Revision) &&
		ref.RecordName == "sh.helm.release.v1."+ref.ReleaseName+".v"+ref.Revision &&
		ref.UID != "" && len(ref.UID) <= 128 && ref.ResourceVersion != "" && len(ref.ResourceVersion) <= 128
}

func (ref RecordRef) matches(meta metav1.ObjectMeta) bool {
	return meta.Name == ref.RecordName && meta.Namespace == ref.Namespace && string(meta.UID) == ref.UID &&
		meta.ResourceVersion == ref.ResourceVersion && meta.Labels["owner"] == "helm" &&
		meta.Labels["name"] == ref.ReleaseName && meta.Labels["version"] == ref.Revision
}

func changedRecord() error {
	return model.Fail("stale_target", "This Helm record changed or disappeared. Refresh and select a revision again.")
}

func readRelease(ctx context.Context, client kubernetes.Interface, ref RecordRef, budget *decodeBudget) (release.Releaser, error) {
	if err := validateScope(ref.Scope); err != nil || !ref.valid() {
		return nil, model.Fail("invalid", "Choose a valid captured Helm revision.")
	}
	verify := func(meta metav1.ObjectMeta, name string, payload string) error {
		if name != ref.RecordName || !ref.matches(meta) {
			return changedRecord()
		}
		return budget.verify(ctx, payload)
	}
	var d driver.Driver
	switch ref.Driver {
	case "secret":
		getter := storageGetter[core.Secret, core.SecretList, apply.SecretApplyConfiguration]{get: func(name string, options metav1.GetOptions) (*core.Secret, error) {
			object, err := client.CoreV1().Secrets(ref.Namespace).Get(ctx, ref.RecordName, options)
			if err != nil {
				return nil, err
			}
			if object.Type != "helm.sh/release.v1" {
				return nil, model.Fail("invalid_response", "The Helm Secret has an unexpected type.")
			}
			if err := verify(object.ObjectMeta, name, string(object.Data["release"])); err != nil {
				return nil, err
			}
			return object, nil
		}}
		d = driver.NewSecrets(getter)
	case "configmap":
		getter := storageGetter[core.ConfigMap, core.ConfigMapList, apply.ConfigMapApplyConfiguration]{get: func(name string, options metav1.GetOptions) (*core.ConfigMap, error) {
			object, err := client.CoreV1().ConfigMaps(ref.Namespace).Get(ctx, ref.RecordName, options)
			if err != nil {
				return nil, err
			}
			if err := verify(object.ObjectMeta, name, object.Data["release"]); err != nil {
				return nil, err
			}
			return object, nil
		}}
		d = driver.NewConfigMaps(getter)
	}
	d.(interface{ SetLogger(slog.Handler) }).SetLogger(slog.DiscardHandler)
	r, err := d.Get(ref.RecordName)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return nil, changedRecord()
		}
		var problem *model.Problem
		if errors.As(err, &problem) {
			return nil, problem
		}
		if ctx.Err() != nil || apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) || apierrors.IsTimeout(err) || apierrors.IsTooManyRequests(err) {
			return nil, kube.ClassifyError(ctx, err)
		}
		return nil, model.Fail("unavailable", "The Helm record could not be read or decoded.")
	}
	if ctx.Err() != nil {
		return nil, kube.ClassifyError(ctx, ctx.Err())
	}
	accessor, err := release.NewAccessor(r)
	if err != nil || accessor.Name() != ref.ReleaseName || accessor.Namespace() != ref.Namespace || strconv.Itoa(accessor.Version()) != ref.Revision {
		return nil, model.Fail("invalid_response", "Decoded release identity does not match the Helm record.")
	}
	return r, nil
}
