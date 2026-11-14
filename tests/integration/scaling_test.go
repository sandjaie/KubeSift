//go:build integration

/*
Purpose: Prove reviewed scaling and API-server write preconditions on both kind clusters.
Used by: Opt-in Phase 1D.1 integration verification.
Responsibilities: Exercise only the owned workloads and restricted test identities.
Important: No ambient configuration, external endpoint or other workload may be used.
*/
package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/operation"
	auto "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

func TestReviewedScaling(t *testing.T) {
	if os.Getenv("KUBESIFT_SCALING_INTEGRATION") != "1" {
		t.Skip("explicit scaling opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	service := operation.NewScaleService(manager, nil)
	defer service.Close()
	open := func(suffix, variant string) (model.ConnectionRef, kubernetes.Interface) {
		t.Helper()
		target := "kind-dev-local-" + suffix
		snapshot, err := config.Load([]string{filepath.Join(dir, suffix+variant+".kubeconfig")})
		if err != nil {
			t.Fatal(err)
		}
		if entries := snapshot.Contexts(); len(entries) != 1 || entries[0].Name != target {
			t.Fatal("unapproved context")
		}
		resolved, err := snapshot.Resolve(target)
		if err != nil {
			t.Fatal(err)
		}
		endpoint, err := url.Parse(resolved.RESTConfig().Host)
		if err != nil || endpoint.Scheme != "https" || endpoint.User != nil {
			t.Fatal("invalid endpoint")
		}
		ip := net.ParseIP(endpoint.Hostname())
		if endpoint.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			t.Fatal("external endpoint rejected")
		}
		ref, err := manager.Open(ctx, resolved)
		if err != nil {
			t.Fatal(err)
		}
		client, err := manager.Client(ref)
		if err != nil {
			t.Fatal(err)
		}
		return ref, client
	}
	const ns = "kubesift-scaling-check"
	previousUID := map[string]types.UID{}
	for _, suffix := range []string{"a", "b"} {
		t.Log("Verifying kind-dev-local-" + suffix)
		ref, client := open(suffix, "")
		fixture, err := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		if err != nil || fixture.Labels["kubesift.dev/fixture"] != "phase1d1-validation" {
			t.Fatal("fixture ownership unavailable", err)
		}
		for _, kind := range []string{"Deployment", "StatefulSet"} {
			plural, name := "deployments", "reviewed-deployment"
			if kind == "StatefulSet" {
				plural, name = "statefulsets", "reviewed-statefulset"
			}
			get := func(client kubernetes.Interface) *auto.Scale {
				t.Helper()
				var scale *auto.Scale
				var err error
				if kind == "Deployment" {
					scale, err = client.AppsV1().Deployments(ns).GetScale(ctx, name, metav1.GetOptions{})
				} else {
					scale, err = client.AppsV1().StatefulSets(ns).GetScale(ctx, name, metav1.GetOptions{})
				}
				if err != nil {
					t.Fatal(err)
				}
				return scale
			}
			wait := func(desired int32) {
				t.Helper()
				deadline := time.Now().Add(30 * time.Second)
				stableVersion := ""
				for {
					ready := false
					version := ""
					if kind == "Deployment" {
						obj, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
						if err != nil {
							t.Fatal(err)
						}
						version = obj.ResourceVersion
						ready = obj.Status.ObservedGeneration == obj.Generation && obj.Status.AvailableReplicas == desired &&
							obj.Status.ReadyReplicas == desired && obj.Status.Replicas == desired && obj.Status.UpdatedReplicas == desired
					} else {
						obj, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
						if err != nil {
							t.Fatal(err)
						}
						version = obj.ResourceVersion
						ready = obj.Status.ObservedGeneration == obj.Generation && obj.Status.ReadyReplicas == desired && obj.Status.Replicas == desired &&
							obj.Status.UpdatedReplicas == desired
					}
					if ready && stableVersion == version {
						return
					}
					stableVersion = ""
					if ready {
						stableVersion = version
					}
					if time.Now().After(deadline) {
						t.Fatal("fixture did not settle", kind, desired)
					}
					time.Sleep(300 * time.Millisecond)
				}
			}
			wait(1)
			first := get(client)
			if first.UID == "" || first.ResourceVersion == "" || first.UID == previousUID[kind] || first.Spec.Replicas != 1 {
				t.Fatal("unexpected fixture scale", kind)
			}
			previousUID[kind] = first.UID
			toRequest := func(connection model.ConnectionRef, scale *auto.Scale, count int32) operation.ScaleRequest {
				return operation.ScaleRequest{Resource: kube.ResourceRef{Connection: connection, APIVersion: "apps/v1", Kind: kind, Namespace: ns, Name: name, UID: string(scale.UID)}, SourceVersion: scale.ResourceVersion, ReplicasText: strconv.Itoa(int(count))}
			}
			for _, desired := range []int32{2, 1, 0, 1} {
				before := get(client)
				preview, err := service.PrepareScale(ctx, toRequest(ref, before, desired))
				if err != nil || preview.Token == "" || preview.Permission != "allowed" {
					t.Fatalf("%s prepare for %d: %+v %v", kind, desired, preview, err)
				}
				show := get(client)
				if show.Spec.Replicas != before.Spec.Replicas || show.ResourceVersion != before.ResourceVersion {
					t.Fatalf("%s fixture changed while preparing %d: count %d → %d, version %s → %s", kind, desired, before.Spec.Replicas, show.Spec.Replicas, before.ResourceVersion, show.ResourceVersion)
				}
				acknowledgement := ""
				if desired == 0 {
					acknowledgement = name
				}
				result, err := service.ExecuteScale(ctx, preview.Token, acknowledgement)
				if err != nil || result.State != "accepted" || result.Reported == nil || *result.Reported != desired {
					t.Fatalf("%s execute %+v %v", kind, result, err)
				}
				duplicate, err := service.ExecuteScale(ctx, preview.Token, acknowledgement)
				if err != nil || duplicate.OperationID != result.OperationID {
					t.Fatal("duplicate did not return existing result", err)
				}
				wait(desired)
			}
			current := get(client)
			// Send the production-shaped update with one mismatch at a time.
			// Using the live version for the stale-UID case makes it an independent UID proof.
			for _, which := range []string{"uid", "resourceVersion"} {
				body := auto.Scale{TypeMeta: metav1.TypeMeta{APIVersion: "autoscaling/v1", Kind: "Scale"}, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: current.UID, ResourceVersion: current.ResourceVersion}, Spec: auto.ScaleSpec{Replicas: 2}}
				if which == "uid" {
					body.UID = "recreated-uid"
				} else {
					body.ResourceVersion = first.ResourceVersion
				}
				path := "/apis/apps/v1/namespaces/" + ns + "/" + plural + "/" + name + "/scale"
				err := client.AppsV1().RESTClient().Put().AbsPath(path).SetHeader("Accept", "application/json").Body(&body).MaxRetries(0).Do(ctx).Error()
				if !apierrors.IsConflict(err) {
					t.Fatalf("%s API did not enforce %s: %v", kind, which, err)
				}
				if get(client).Spec.Replicas != 1 {
					t.Fatal("stale update changed the desired count")
				}
			}
			restricted, restrictedClient := open(suffix, "-reader")
			if _, err := service.PrepareScale(ctx, toRequest(restricted, get(restrictedClient), 2)); err == nil {
				t.Fatal("read-only identity prepared a scale")
			} else {
				var p *model.Problem
				if !errors.As(err, &p) || p.Code != "forbidden" {
					t.Fatal(err)
				}
			}
			limited, limitedClient := open(suffix, "-writer")
			limitedPreview, err := service.PrepareScale(ctx, toRequest(limited, get(limitedClient), 2))
			if err != nil || limitedPreview.Permission != "unknown" || limitedPreview.Token == "" {
				t.Fatalf("unavailable review should allow the authorized dry-run: %+v %v", limitedPreview, err)
			}
			service.RevokeScale(limitedPreview.Token)
		}
		testScaleTransport(t, ctx, manager, service, ref, client, ns, "reviewed-deployment")
	}
}

// The proxy only forwards the owned Deployment, its scale and the fixed self-review.
// It creates a late version race or drops a successful response after one real PUT.
func testScaleTransport(t *testing.T, ctx context.Context, manager *connection.Manager, service *operation.ScaleService, ref model.ConnectionRef, client kubernetes.Interface, ns, name string) {
	t.Helper()
	const permission = "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews"
	parent := "/apis/apps/v1/namespaces/" + ns + "/deployments/" + name
	cfg, err := manager.RESTConfig(ref)
	if err != nil {
		t.Fatal(err)
	}
	upstream, err := kube.BoundedHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var mode atomic.Int32
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path := r.Method, r.URL.Path
		allowed := method == "GET" && (path == parent || path == parent+"/scale") ||
			method == "POST" && path == permission || method == "PUT" && path == parent+"/scale"
		if !allowed {
			t.Error("request outside the scaling fixture")
			w.WriteHeader(400)
			return
		}
		actual := method == "PUT" && r.URL.Query().Get("dryRun") == ""
		if actual {
			writes.Add(1)
			if mode.Load() == 1 {
				patch := []byte(fmt.Sprintf(`{"metadata":{"annotations":{"kubesift.dev/scale-race":%q}}}`, model.ID()))
				if _, err := client.AppsV1().Deployments(ns).Patch(r.Context(), name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
					t.Error("cannot advance fixture version", err)
					w.WriteHeader(500)
					return
				}
			}
		}
		req, err := http.NewRequestWithContext(r.Context(), method, strings.TrimRight(cfg.Host, "/")+r.URL.RequestURI(), r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		response, err := upstream.Do(req)
		if err != nil {
			t.Error("fixture upstream failed")
			w.WriteHeader(500)
			return
		}
		defer response.Body.Close()
		if actual && mode.Load() == 2 {
			if response.StatusCode != 200 {
				t.Error("expected successful upstream write")
				w.WriteHeader(500)
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	defer server.Close()
	// The proxy kubeconfig contains only the ephemeral localhost address.
	raw := fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"proxy","cluster":{"server":%q}}],"contexts":[{"name":"fixture-proxy","context":{"cluster":"proxy","user":"proxy"}}],"users":[{"name":"proxy","user":{}}]}`, server.URL)
	path := filepath.Join(t.TempDir(), "fixture-proxy.kubeconfig")
	if err = os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := snapshot.Resolve("fixture-proxy")
	if err != nil {
		t.Fatal(err)
	}
	routed, err := manager.Open(ctx, resolved)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(routed)
	read := func() *auto.Scale {
		t.Helper()
		current, err := client.AppsV1().Deployments(ns).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return current
	}
	makeReview := func(connection model.ConnectionRef, desired string) operation.ScalePreview {
		t.Helper()
		current := read()
		p, err := service.PrepareScale(ctx, operation.ScaleRequest{
			Resource:      kube.ResourceRef{Connection: connection, APIVersion: "apps/v1", Kind: "Deployment", Namespace: ns, Name: name, UID: string(current.UID)},
			SourceVersion: current.ResourceVersion, ReplicasText: desired,
		})
		if err != nil || p.Token == "" {
			t.Fatalf("fixture cannot prepare: %+v %v", p, err)
		}
		return p
	}
	mode.Store(1)
	stale := makeReview(routed, "2")
	result, err := service.ExecuteScale(ctx, stale.Token, "")
	if err != nil || result.State != "stale" || writes.Load() != 1 || read().Spec.Replicas != 1 {
		t.Fatalf("late conflict: %+v %v", result, err)
	}
	// Let the Deployment controller finish updating status after the annotation change.
	waitScaleFixture(t, ctx, client, ns, name, 1)
	mode.Store(2)
	lost := makeReview(routed, "2")
	result, err = service.ExecuteScale(ctx, lost.Token, "")
	if err != nil || result.State != "unknown" || writes.Load() != 2 || read().Spec.Replicas != 2 {
		t.Fatalf("lost response: %+v %v", result, err)
	}
	duplicate, err := service.ExecuteScale(ctx, lost.Token, "")
	if err != nil || duplicate.State != "unknown" || duplicate.OperationID != result.OperationID || writes.Load() != 2 {
		t.Fatal("unknown write was replayed", err)
	}
	waitScaleFixture(t, ctx, client, ns, name, 2)
	restore := makeReview(ref, "1")
	result, err = service.ExecuteScale(ctx, restore.Token, "")
	if err != nil || result.State != "accepted" {
		t.Fatal("fixture restore failed", err)
	}
	waitScaleFixture(t, ctx, client, ns, name, 1)
}

func waitScaleFixture(t *testing.T, ctx context.Context, client kubernetes.Interface, ns, name string, desired int32) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	stableVersion := ""
	for {
		obj, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		ready := obj.Status.ObservedGeneration == obj.Generation && obj.Status.AvailableReplicas == desired && obj.Status.ReadyReplicas == desired && obj.Status.UpdatedReplicas == desired && obj.Status.Replicas == desired
		if ready && stableVersion == obj.ResourceVersion {
			return
		}
		stableVersion = ""
		if ready {
			stableVersion = obj.ResourceVersion
		}
		if time.Now().After(deadline) {
			t.Fatal("deployment fixture did not settle")
		}
		time.Sleep(300 * time.Millisecond)
	}
}
