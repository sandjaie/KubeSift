/*
Purpose: Verify Helm SDK storage behavior through an owned HTTP transport.
Used by: Helm reader unit tests.
Responsibilities: Check scope, identity and release expansion limits.
Important: The HTTP server serves synthetic records only.
*/
package helmreader

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestBoundedStorage(t *testing.T) {
	ref := RecordRef{Scope: Scope{Namespace: "owned", Driver: "configmap", ReleaseName: "demo"}, Revision: "1", RecordName: "sh.helm.release.v1.demo.v1", UID: "original", ResourceVersion: "9"}
	for _, mode := range []string{"valid", "forbidden", "replacement", "oversize", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/v1/namespaces/owned/configmaps/"+ref.RecordName {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "forbidden" {
					w.WriteHeader(403)
					return
				}
				if mode == "cancelled" {
					cancel()
					<-r.Context().Done()
					return
				}
				uid := ref.UID
				if mode == "replacement" {
					uid = "replacement"
				}
				payload := `{"name":"demo","namespace":"owned","version":1}`
				encoded := base64.StdEncoding.EncodeToString([]byte(payload))
				if mode == "oversize" {
					var compressed bytes.Buffer
					writer := gzip.NewWriter(&compressed)
					if _, err := writer.Write([]byte(strings.Repeat("x", (8<<20)+1))); err != nil {
						t.Error(err)
					}
					if err := writer.Close(); err != nil {
						t.Error(err)
					}
					encoded = base64.StdEncoding.EncodeToString(compressed.Bytes())
				}
				fmt.Fprintf(w, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":%q,"namespace":"owned","uid":%q,"resourceVersion":"9","labels":{"owner":"helm","name":"demo","version":"1"}},"data":{"release":%q}}`, ref.RecordName, uid, encoded)
			}))
			defer server.Close()
			cfg := &rest.Config{Host: server.URL}
			httpClient, err := kube.BoundedHTTPClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			client, err := kubernetes.NewForConfigAndClient(cfg, httpClient)
			if err != nil {
				t.Fatal(err)
			}
			release, err := readRelease(ctx, client, ref, newBudget())
			if mode == "valid" {
				if err != nil || release == nil {
					t.Fatalf("expected a decoded release, got %v", err)
				}
			} else if err == nil {
				t.Fatal("unsafe storage read succeeded")
			}
		})
	}
}
