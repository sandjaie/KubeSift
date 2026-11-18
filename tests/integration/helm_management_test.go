//go:build integration

/*
Purpose: Exercise Helm management against the two approved local kind clusters.
Used by: Opt-in Phase 1E.2 acceptance verification.
Responsibilities: Check source authentication, real SDK mutations, identity, failure and cleanup.
Important: Only loopback kind contexts and newly created task-owned namespaces are accepted.
*/
package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/helmmanager"
	"github.com/sandjaie/KubeSift/internal/helmreader"
	auth "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apiext "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
)

func managementChart(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	root := "../fixtures/helm/management"
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if entry.Name() == "Chart.yaml" {
			data = bytes.ReplaceAll(data, []byte("version: 1.0.0"), []byte("version: "+version))
		}
		rel, _ := filepath.Rel(root, path)
		if err := tw.WriteHeader(&tar.Header{Name: "kubesift-management/" + filepath.ToSlash(rel), Mode: 0600, Size: int64(len(data))}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestHelmManagement(t *testing.T) {
	if os.Getenv("KUBESIFT_HELM_INTEGRATION") != "1" {
		t.Skip("explicit Helm management opt-in required")
	}
	dir := os.Getenv("KUBESIFT_EXPLORER_CONFIG_DIR")
	if !filepath.IsAbs(dir) {
		t.Fatal("isolated kubeconfig directory required")
	}
	helm3 := os.Getenv("KUBESIFT_HELM3")
	if !filepath.IsAbs(helm3) {
		t.Fatal("explicit Helm 3 fixture binary required")
	}
	version, err := exec.Command(helm3, "version", "--short").Output()
	if err != nil || !strings.HasPrefix(string(version), "v3.") {
		t.Fatal("Helm 3 fixture binary not verified")
	}
	t.Log("compatibility fixture", strings.TrimSpace(string(version)))
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	manager := connection.NewManager(nil)
	defer manager.CloseAll()
	service := helmmanager.NewService(manager)
	defer service.Close()
	raw1, raw2 := managementChart(t, "1.0.0"), managementChart(t, "1.1.0")
	chartPath := filepath.Join(t.TempDir(), "management.tgz")
	if err := os.WriteFile(chartPath, raw1, 0600); err != nil {
		t.Fatal(err)
	}
	digest := func(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
	manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": map[string]any{"mediaType": "application/vnd.cncf.helm.config.v1+json", "digest": digest([]byte("{}")), "size": 2}, "layers": []map[string]any{{"mediaType": "application/vnd.cncf.helm.chart.content.v1.tar+gzip", "digest": digest(raw2), "size": len(raw2)}}})
	repo := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, _ := r.BasicAuth()
		if user != "fixture" || password != "synthetic-helm-password" {
			w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
			w.WriteHeader(401)
			return
		}
		var data []byte
		switch {
		case r.URL.Path == "/index.yaml":
			data = []byte(fmt.Sprintf("apiVersion: v1\nentries:\n  kubesift-management:\n    - name: kubesift-management\n      version: 1.1.0\n      digest: %s\n      urls: [management.tgz]\n", strings.TrimPrefix(digest(raw2), "sha256:")))
		case r.URL.Path == "/management.tgz":
			data = raw2
		case strings.Contains(r.URL.Path, "/manifests/"):
			data = manifest
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Header().Set("Docker-Content-Digest", digest(manifest))
		case strings.HasSuffix(r.URL.Path, digest(raw2)):
			data = raw2
		default:
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		if r.Method != "HEAD" {
			w.Write(data)
		}
	}))
	defer repo.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: repo.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"https", "oci"} {
		url := repo.URL
		if kind == "oci" {
			url = strings.Replace(url, "https:", "oci:", 1)
		}
		if err := service.SaveSource(helmmanager.Source{ID: kind, Name: kind, Kind: kind, URL: url, CAFile: ca}); err != nil {
			t.Fatal(err)
		}
		if err := service.SetCredentials(kind, helmmanager.SourceCredentials{Username: "fixture", Password: "synthetic-helm-password"}); err != nil {
			t.Fatal(err)
		}
	}
	rootTest := t
	ns := "kubesift-helm-management-" + fmt.Sprint(time.Now().UnixNano())
	clients := map[string]kubernetes.Interface{}
	cleanups := []func(){}
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()
	for _, suffix := range []string{"a", "b"} {
		t.Run(suffix, func(t *testing.T) {
			wait := func(id string) helmmanager.Result {
				t.Helper()
				deadline := time.Now().Add(90 * time.Second)
				for time.Now().Before(deadline) {
					for _, result := range service.ListResults() {
						if result.ID == id && result.State != "running" {
							return result
						}
					}
					time.Sleep(100 * time.Millisecond)
				}
				t.Fatal("native worker did not terminate")
				return helmmanager.Result{}
			}
			prepare := func(req helmmanager.PrepareRequest) helmmanager.Review {
				t.Helper()
				review, err := service.Prepare(ctx, req)
				if err != nil {
					t.Fatalf("%s prepare: %v", req.Action, err)
				}
				return review
			}
			execute := func(review helmmanager.Review, want string) helmmanager.Result {
				t.Helper()
				result, err := service.Execute(review.Token)
				if err != nil {
					t.Fatal(err)
				}
				duplicate, err := service.Execute(review.Token)
				if err != nil || duplicate.ID != result.ID {
					t.Fatal("duplicate execution not idempotent")
				}
				result = wait(result.ID)
				if result.State != want {
					t.Fatalf("%s: state=%s problem=%+v", review.Action, result.State, result.Problem)
				}
				return result
			}
			local := func() string {
				t.Helper()
				id, err := service.ImportChart(chartPath)
				if err != nil {
					t.Fatal(err)
				}
				return id
			}

			ref, client := openLocalOperationFixture(t, ctx, manager, dir, suffix, "")
			clients[suffix] = client
			namespace, err := client.CoreV1().Namespaces().Create(ctx, &core.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns, Labels: map[string]string{"kubesift.dev/fixture": "phase1e2"}}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := manager.RESTConfig(ref)
			if err != nil {
				t.Fatal(err)
			}
			crds, err := apiext.NewForConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := crds.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, "fixtures.management.kubesift.test", metav1.GetOptions{}); err == nil {
				t.Fatal("fixture CRD already exists; refusing to claim ownership")
			}
			cleanups = append(cleanups, func() {
				cleanup, done := context.WithTimeout(context.Background(), time.Minute)
				defer done()
				if err := client.CoreV1().Namespaces().Delete(cleanup, ns, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &namespace.UID}}); err != nil {
					rootTest.Error("namespace cleanup", err)
				}
				crd, err := crds.ApiextensionsV1().CustomResourceDefinitions().Get(cleanup, "fixtures.management.kubesift.test", metav1.GetOptions{})
				if err == nil {
					if crd.Labels["kubesift.dev/fixture"] != "phase1e2" {
						rootTest.Error("CRD ownership changed")
					} else if err := crds.ApiextensionsV1().CustomResourceDefinitions().Delete(cleanup, crd.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &crd.UID}}); err != nil {
						rootTest.Error(err)
					}
				}
				for cleanup.Err() == nil {
					_, namespaceError := client.CoreV1().Namespaces().Get(cleanup, ns, metav1.GetOptions{})
					_, crdError := crds.ApiextensionsV1().CustomResourceDefinitions().Get(cleanup, "fixtures.management.kubesift.test", metav1.GetOptions{})
					if apierrors.IsNotFound(namespaceError) && apierrors.IsNotFound(crdError) {
						rootTest.Log("owned namespace and CRD cleanup verified", suffix)
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
				rootTest.Error("fixture cleanup did not finish", suffix)
			})
			refreshRecord := func(record *helmreader.RecordRef) {
				t.Helper()
				var meta metav1.Object
				var err error
				if record.Driver == "secret" {
					meta, err = client.CoreV1().Secrets(ns).Get(ctx, record.RecordName, metav1.GetOptions{})
				} else {
					meta, err = client.CoreV1().ConfigMaps(ns).Get(ctx, record.RecordName, metav1.GetOptions{})
				}
				if err != nil {
					t.Fatal(err)
				}
				if string(meta.GetUID()) != record.UID {
					t.Fatal("historical record was replaced")
				}
				record.ResourceVersion = meta.GetResourceVersion()
			}
			driver := "secret"
			if suffix == "b" {
				driver = "configmap"
			}
			target := helmmanager.Target{Connection: ref, Namespace: ns, Driver: driver, ReleaseName: "same-name"}
			req := helmmanager.PrepareRequest{Action: "install", Target: target, ChartID: local(), ValuesYAML: "marker: one\n"}
			review := prepare(req)
			for _, section := range []string{"hooks", "crds", "resources"} {
				part, err := service.ReadReview(ctx, review.Token, section, 0)
				if err != nil || part.Text == "" {
					t.Fatal("missing impact", section, err)
				}
			}
			execute(review, "succeeded")
			cm, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "same-name", metav1.GetOptions{})
			if err != nil || cm.Data["marker"] != "one" {
				t.Fatal("local install", err)
			}
			first, _, err := helmreader.CaptureHead(ctx, client, target)
			if err != nil || first == nil {
				t.Fatal(err)
			}
			chart, err := service.ResolveChart(ctx, helmmanager.ChartRequest{SourceID: "https", Chart: "kubesift-management", Version: "1.1.0"})
			if err != nil {
				t.Fatal(err)
			}
			execute(prepare(helmmanager.PrepareRequest{Action: "upgrade", Target: target, ChartID: chart.ID, ValuesYAML: "marker: two\ncustomResource: true\n"}), "succeeded")
			cm, err = client.CoreV1().ConfigMaps(ns).Get(ctx, "same-name", metav1.GetOptions{})
			if err != nil || cm.Data["marker"] != "two" {
				t.Fatal("HTTPS upgrade", err)
			}
			refreshRecord(first)
			execute(prepare(helmmanager.PrepareRequest{Action: "rollback", Target: target, RollbackRecord: first}), "succeeded")
			cm, err = client.CoreV1().ConfigMaps(ns).Get(ctx, "same-name", metav1.GetOptions{})
			if err != nil || cm.Data["marker"] != "one" {
				t.Fatal("rollback", err)
			}
			execute(prepare(helmmanager.PrepareRequest{Action: "uninstall", Target: target}), "succeeded")
			if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "same-name-keep", metav1.GetOptions{}); err != nil {
				t.Fatal("keep policy lost", err)
			}
			if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "same-name", metav1.GetOptions{}); err == nil {
				t.Fatal("ordinary resource retained")
			}
			target.ReleaseName = "oci"
			chart, err = service.ResolveChart(ctx, helmmanager.ChartRequest{SourceID: "oci", Chart: "kubesift-management", Digest: digest(manifest)})
			if err != nil {
				t.Fatal(err)
			}
			execute(prepare(helmmanager.PrepareRequest{Action: "install", Target: target, ChartID: chart.ID}), "succeeded")
			execute(prepare(helmmanager.PrepareRequest{Action: "uninstall", Target: target}), "succeeded")
			target.ReleaseName = "helm3"
			cmd := exec.CommandContext(ctx, helm3, "install", target.ReleaseName, chartPath, "--kubeconfig", filepath.Join(dir, suffix+".kubeconfig"), "--kube-context", "kind-dev-local-"+suffix, "--namespace", ns, "--no-hooks", "--wait", "--timeout", "60s")
			cmd.Env = append(os.Environ(), "HELM_DRIVER="+driver)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("Helm3 fixture: %v %s", err, out)
			}
			execute(prepare(helmmanager.PrepareRequest{Action: "upgrade", Target: target, ChartID: local(), ValuesYAML: "marker: SDK4\n"}), "succeeded")
			execute(prepare(helmmanager.PrepareRequest{Action: "uninstall", Target: target}), "succeeded")
			target.ReleaseName = "failing-hook"
			execute(prepare(helmmanager.PrepareRequest{Action: "install", Target: target, ChartID: local(), ValuesYAML: "hookMode: fail\n"}), "failed")
			for _, action := range []string{"install", "rollback", "uninstall"} {
				target.ReleaseName = "cancel-" + action
				event := "pre-" + action
				if action == "uninstall" {
					event = "pre-delete"
				}
				values := "hookMode: wait\nhookEvents: " + event + "\n"
				req := helmmanager.PrepareRequest{Action: "install", Target: target, ChartID: local(), ValuesYAML: values}
				if action != "install" {
					execute(prepare(req), "succeeded")
					first, _, err := helmreader.CaptureHead(ctx, client, target)
					if err != nil {
						t.Fatal(err)
					}
					if action == "rollback" {
						execute(prepare(helmmanager.PrepareRequest{Action: "upgrade", Target: target, ChartID: local(), ValuesYAML: values + "marker: revision2\n"}), "succeeded")
					}
					req = helmmanager.PrepareRequest{Action: action, Target: target}
					if action == "rollback" {
						refreshRecord(first)
						req.RollbackRecord = first
					}
				}
				review := prepare(req)
				result, err := service.Execute(review.Token)
				if err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(30 * time.Second)
				for {
					if _, err := client.BatchV1().Jobs(ns).Get(ctx, target.ReleaseName+"-hook", metav1.GetOptions{}); err == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("hook never started")
					}
					time.Sleep(100 * time.Millisecond)
				}
				if err := service.Cancel(result.ID); err != nil {
					t.Fatal(err)
				}
				result = wait(result.ID)
				if result.State != "unknown" {
					t.Fatalf("cancel %s: %+v", action, result)
				}
				t.Log(action, "cancelled and worker joined")
			}
			// A synthetic reader can prepare, but the actual API denies storage and resource writes.
			_, err = client.CoreV1().ServiceAccounts(ns).Create(ctx, &core.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "reader"}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.RbacV1().Roles(ns).Create(ctx, &rbac.Role{ObjectMeta: metav1.ObjectMeta{Name: "reader"}, Rules: []rbac.PolicyRule{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"get", "list", "watch"}}}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.RbacV1().RoleBindings(ns).Create(ctx, &rbac.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "reader"}, RoleRef: rbac.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "reader"}, Subjects: []rbac.Subject{{Kind: "ServiceAccount", Name: "reader", Namespace: ns}}}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			token, err := client.CoreV1().ServiceAccounts(ns).CreateToken(ctx, "reader", &auth.TokenRequest{}, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			isolated, err := clientcmd.LoadFromFile(filepath.Join(dir, suffix+".kubeconfig"))
			if err != nil {
				t.Fatal(err)
			}
			isolated.AuthInfos = map[string]*clientapi.AuthInfo{"reader": {Token: token.Status.Token}}
			isolated.Contexts["kind-dev-local-"+suffix].AuthInfo = "reader"
			deniedPath := filepath.Join(dir, suffix+"-management-denied.kubeconfig")
			if err := clientcmd.WriteToFile(*isolated, deniedPath); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(deniedPath)
			denied, _ := openLocalOperationFixture(t, ctx, manager, dir, suffix, "-management-denied")
			target.Connection = denied
			target.ReleaseName = "denied"
			// Existing CRDs would require cluster-scoped access even during prepare, so use the simple inspection chart for this case.
			deniedChart := filepath.Join(t.TempDir(), "denied.tgz")
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			tw := tar.NewWriter(gz)
			for name, data := range map[string]string{"Chart.yaml": "apiVersion: v2\nname: denied\nversion: 1.0.0\n", "templates/cm.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: denied\n"} {
				if err := tw.WriteHeader(&tar.Header{Name: "denied/" + name, Mode: 0600, Size: int64(len(data))}); err != nil {
					t.Fatal(err)
				}
				tw.Write([]byte(data))
			}
			tw.Close()
			gz.Close()
			os.WriteFile(deniedChart, b.Bytes(), 0600)
			id, err := service.ImportChart(deniedChart)
			if err != nil {
				t.Fatal(err)
			}
			execute(prepare(helmmanager.PrepareRequest{Action: "install", Target: target, ChartID: id}), "failed")
			if _, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "denied", metav1.GetOptions{}); err == nil {
				t.Fatal("denied resource created")
			}
			target.Connection, target.ReleaseName = ref, "isolation"
			execute(prepare(helmmanager.PrepareRequest{Action: "install", Target: target, ChartID: local(), ValuesYAML: "marker: " + suffix + "\n"}), "succeeded")
			t.Log("local/HTTPS/OCI, four actions, Helm3", driver, "compatibility, hook failure, denied mutation verified")
		})
	}
	for suffix, client := range clients {
		cm, err := client.CoreV1().ConfigMaps(ns).Get(ctx, "isolation", metav1.GetOptions{})
		if err != nil || cm.Data["marker"] != suffix {
			t.Fatalf("same namespace/release crossed clusters: %s %v", suffix, err)
		}
	}
	t.Log("simultaneous same-namespace, same-release isolation verified")

}
