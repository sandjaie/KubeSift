/*
Purpose: Verify the guarded apply wire contract.
Used by: Native operation transport tests.
Responsibilities: Check exact media, options and one dispatch per request.
Important: Real UID and version enforcement is also checked against both kind servers.
*/
package operation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
	"k8s.io/client-go/rest"
)

func TestApplyPatchProtocol(t *testing.T) {
	const body = `{"metadata":{"uid":"original","resourceVersion":"10"}}`
	for _, tc := range []struct {
		name       string
		dry, force bool
		status     int
	}{
		{"review", true, false, 200},
		{"takeover review", true, true, 200},
		{"confirm", false, true, 200},
		{"unexpected creation", false, false, 201},
		{"backpressure", false, true, 429},
		{"unavailable", false, true, 503},
		{"unauthorized", false, false, 401},
		{"redirect", false, true, 307},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				q := r.URL.Query()
				if r.Method != "PATCH" || r.URL.Path != "/api/v1/configmaps/a" || r.Header.Get("Content-Type") != "application/apply-patch+yaml" ||
					q.Get("fieldManager") != applyManager || q.Get("fieldValidation") != "Strict" ||
					(q.Get("dryRun") == "All") != tc.dry || (q.Get("force") == "true") != tc.force {
					t.Errorf("incorrect apply request: %s %s", r.Method, r.URL)
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil || string(raw) != body {
					t.Errorf("guarded payload changed: %q (%v)", raw, err)
				}
				w.Header().Set("Retry-After", "0")
				w.Header().Set("Location", "/other")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			base, _ := url.Parse(server.URL)
			bounded, err := kube.BoundedHTTPClient(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			client := operationClient{base: base, client: bounded}
			var out json.RawMessage
			_, err = client.applyPatch(context.Background(), "/api/v1/configmaps/a", tc.dry, tc.force, []byte(body), &out, nil, nil)
			if (tc.status == 200 && (err != nil || string(out) != body)) ||
				(tc.status != 200 && err != operationHTTPStatus(tc.status)) || attempts.Load() != 1 {
				t.Fatalf("status=%d returned %v, response=%s, attempts=%d", tc.status, err, out, attempts.Load())
			}
		})
	}
}
