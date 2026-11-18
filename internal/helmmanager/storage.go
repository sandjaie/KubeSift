/*
Purpose: Bound Helm SDK storage reads without changing the inspection adapter.
Used by: Native Helm action configurations.
Responsibilities: Page storage queries and preflight release expansion before SDK decoding.
Important: Embedded clients keep normal SDK write semantics; the owned transport controls cancellation and preview writes.
*/
package helmmanager

import (
	"context"
	"log/slog"

	"github.com/sandjaie/KubeSift/internal/helmreader"
	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/storage/driver"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typed "k8s.io/client-go/kubernetes/typed/core/v1"
)

type boundedSecrets struct {
	typed.SecretInterface
	ctx context.Context
}

func (c boundedSecrets) Get(_ context.Context, name string, options metav1.GetOptions) (*core.Secret, error) {
	object, err := c.SecretInterface.Get(c.ctx, name, options)
	if err == nil {
		err = helmreader.VerifyPayload(c.ctx, object.ObjectMeta, string(object.Data["release"]))
	}
	return object, err
}
func (c boundedSecrets) List(_ context.Context, options metav1.ListOptions) (*core.SecretList, error) {
	out := &core.SecretList{}
	verify := helmreader.PayloadVerifier()
	options.Limit = 100
	seen := map[string]bool{}
	for {
		page, err := c.SecretInterface.List(c.ctx, options)
		if err != nil {
			return nil, err
		}
		if len(out.Items)+len(page.Items) > 1000 {
			return nil, model.Fail("capacity", "Helm history exceeds the management record limit.")
		}
		for _, object := range page.Items {
			if err := verify(c.ctx, object.ObjectMeta, string(object.Data["release"])); err != nil {
				return nil, err
			}
		}
		out.Items = append(out.Items, page.Items...)
		options.Continue = page.Continue
		if options.Continue == "" {
			return out, nil
		}
		if seen[options.Continue] {
			return nil, model.Fail("invalid_response", "Helm storage repeated a page.")
		}
		seen[options.Continue] = true
	}
}

type boundedConfigMaps struct {
	typed.ConfigMapInterface
	ctx context.Context
}

func (c boundedConfigMaps) Get(_ context.Context, name string, options metav1.GetOptions) (*core.ConfigMap, error) {
	object, err := c.ConfigMapInterface.Get(c.ctx, name, options)
	if err == nil {
		err = helmreader.VerifyPayload(c.ctx, object.ObjectMeta, object.Data["release"])
	}
	return object, err
}
func (c boundedConfigMaps) List(_ context.Context, options metav1.ListOptions) (*core.ConfigMapList, error) {
	out := &core.ConfigMapList{}
	verify := helmreader.PayloadVerifier()
	options.Limit = 100
	seen := map[string]bool{}
	for {
		page, err := c.ConfigMapInterface.List(c.ctx, options)
		if err != nil {
			return nil, err
		}
		if len(out.Items)+len(page.Items) > 1000 {
			return nil, model.Fail("capacity", "Helm history exceeds the management record limit.")
		}
		for _, object := range page.Items {
			if err := verify(c.ctx, object.ObjectMeta, object.Data["release"]); err != nil {
				return nil, err
			}
		}
		out.Items = append(out.Items, page.Items...)
		options.Continue = page.Continue
		if options.Continue == "" {
			return out, nil
		}
		if seen[options.Continue] {
			return nil, model.Fail("invalid_response", "Helm storage repeated a page.")
		}
		seen[options.Continue] = true
	}
}

func newStorageDriver(ctx context.Context, client kubernetes.Interface, target Target, _ bool) driver.Driver {
	var d driver.Driver
	if target.Driver == "configmap" {
		d = driver.NewConfigMaps(boundedConfigMaps{client.CoreV1().ConfigMaps(target.Namespace), ctx})
	} else {
		d = driver.NewSecrets(boundedSecrets{client.CoreV1().Secrets(target.Namespace), ctx})
	}
	d.(interface{ SetLogger(slog.Handler) }).SetLogger(slog.DiscardHandler)
	return d
}
