/*
Purpose: Prove prepared writes keep their reviewed identity and never repeat uncertain writes.
Used by: The focused mutation safety test and repository checks.
Responsibilities: Exercise immutable targets, stale identities, expiry, concurrent consumption and response loss with local fakes.
Important: No cluster or credential is used; injected boundaries replace only Kubernetes network work.
*/
package operation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// Dropping any identity check, consuming after the write, or retrying a failed
// write would either change the wrong object or make one of these cases fail.
func TestPreparedMutationSafety(t *testing.T) {
	for _, name := range []string{"tab-switch", "uid", "version", "generation", "trust", "reused", "expired", "response-loss", "concurrent"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
			ref := model.ConnectionRef{ID: "a", Generation: 1}
			original := model.Target{Connection: ref, Namespace: "payments", Name: "ks-marker"}
			cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "ks-marker", UID: types.UID("uid-a"), ResourceVersion: "10"}, Data: map[string]string{"marker": "cluster-a", "keep": "untouched"}}
			client := fake.NewClientset(cm)
			var gate error
			service := NewService(nil, func() time.Time { return now })
			service.client = func(got model.ConnectionRef) (kubernetes.Interface, error) {
				if got != ref {
					t.Errorf("wrong connection: %+v", got)
				}
				return client, gate
			}
			var writes atomic.Int32
			service.write = func(ctx context.Context, got kubernetes.Interface, target model.Target, updated *corev1.ConfigMap) error {
				writes.Add(1)
				if target != original || updated.UID != "uid-a" || updated.ResourceVersion != "10" || updated.Data["marker"] != "reviewed" || updated.Data["keep"] != "untouched" {
					t.Errorf("write escaped review: %+v %+v", target, updated)
				}
				if name == "response-loss" {
					return errors.New("response lost with secret credential")
				}
				_, err := got.CoreV1().ConfigMaps(target.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
				return err
			}
			preview, err := service.Prepare(ctx, original, "reviewed")
			if err != nil {
				t.Fatal(err)
			}
			if preview.Target != original || preview.OldValue != "cluster-a" || preview.NewValue != "reviewed" || preview.UID != "uid-a" || preview.ResourceVersion != "10" || !preview.ExpiresAt.Equal(now.Add(5*time.Minute)) {
				t.Fatalf("incomplete review: %+v", preview)
			}
			want := ""
			switch name {
			case "tab-switch":
				// Both caller-owned values can change; Execute accepts only the token.
				original.Name = "other"
				preview.Target.Connection = model.ConnectionRef{ID: "b", Generation: 7}
				preview.NewValue = "changed"
				original = model.Target{Connection: ref, Namespace: "payments", Name: "ks-marker"}
			case "uid", "version":
				changed := cm.DeepCopy()
				if name == "uid" {
					changed.UID = "recreated"
				} else {
					changed.ResourceVersion = "11"
				}
				if err = client.Tracker().Update(corev1.SchemeGroupVersion.WithResource("configmaps"), runtime.Object(changed), "payments"); err != nil {
					t.Fatal(err)
				}
				want = "stale_target"
			case "generation":
				gate = model.Fail("stale_target", "Connection replaced")
				want = "stale_target"
			case "trust":
				gate = model.Fail("trust_required", "Trust revoked")
				want = "trust_required"
			case "expired":
				now = now.Add(5 * time.Minute)
				want = "expired"
			case "response-loss":
				want = "uncertain"
			}
			if name == "concurrent" {
				var wg sync.WaitGroup
				var successful atomic.Int32
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						if service.Execute(ctx, preview.Token) == nil {
							successful.Add(1)
						}
					}()
				}
				wg.Wait()
				if successful.Load() != 1 || writes.Load() != 1 {
					t.Fatalf("concurrent confirmations: successes=%d writes=%d", successful.Load(), writes.Load())
				}
			} else {
				err = service.Execute(ctx, preview.Token)
				checkCode(t, err, want)
				if name == "response-loss" && err.Error() != "uncertain: Write outcome is uncertain; inspect the marker and prepare again" {
					t.Fatalf("unsafe error: %v", err)
				}
				expected := int32(1)
				if want != "" && want != "uncertain" {
					expected = 0
				}
				if writes.Load() != expected {
					t.Fatalf("writes=%d want %d", writes.Load(), expected)
				}
			}
			checkCode(t, service.Execute(ctx, preview.Token), "invalid_token")
			stored, err := client.CoreV1().ConfigMaps("payments").Get(ctx, "ks-marker", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if want == "" && stored.Data["marker"] != "reviewed" {
				t.Fatal("reviewed marker not written")
			}
		})
	}
}
func checkCode(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var safe *model.Error
	if !errors.As(err, &safe) || safe.Code != want {
		t.Fatalf("got %v, want %s", err, want)
	}
}

// Exercise the actual REST writer: client-go otherwise retries Retry-After responses.
func TestMutationHTTPAttemptOnce(t *testing.T) {
	var writes atomic.Int32
	cm := corev1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: metav1.ObjectMeta{Namespace: "payments", Name: "ks-marker", UID: "uid-a", ResourceVersion: "10"}, Data: map[string]string{"marker": "cluster-a"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			writes.Add(1)
			var updated corev1.ConfigMap
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				t.Error(err)
			}
			if updated.UID != "uid-a" || updated.ResourceVersion != "10" || updated.Data["marker"] != "reviewed" || r.URL.Path != "/api/v1/namespaces/payments/configmaps/ks-marker" {
				t.Errorf("write lost preconditions or target: %+v %s", updated, r.URL.Path)
			}
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"TooManyRequests","code":429}`))
			return
		}
		_ = json.NewEncoder(w).Encode(cm)
	}))
	defer server.Close()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(nil, time.Now)
	service.client = func(model.ConnectionRef) (kubernetes.Interface, error) { return client, nil }
	preview, err := service.Prepare(context.Background(), model.Target{Connection: model.ConnectionRef{ID: "a", Generation: 1}, Namespace: "payments", Name: "ks-marker"}, "reviewed")
	if err != nil {
		t.Fatal(err)
	}
	checkCode(t, service.Execute(context.Background(), preview.Token), "busy")
	if writes.Load() != 1 {
		t.Fatalf("mutation HTTP attempts=%d, want one", writes.Load())
	}
	checkCode(t, service.Execute(context.Background(), preview.Token), "invalid_token")
}
