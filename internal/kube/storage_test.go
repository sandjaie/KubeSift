/*
Purpose: Verify storage reads against synthetic HTTP responses.
Used by: Explorer reader regression tests.
Responsibilities: Check scope, pagination, identity, and optional storage evidence.
Important: No cluster or storage operation is involved.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStorageInventoryAndDetail(t *testing.T) {
	for _, tc := range []struct{ kind, version, ns, path string }{
		{"PersistentVolumeClaim", "v1", "test", "/api/v1/namespaces/test/persistentvolumeclaims"},
		{"PersistentVolume", "v1", "", "/api/v1/persistentvolumes"},
		{"StorageClass", "storage.k8s.io/v1", "", "/apis/storage.k8s.io/v1/storageclasses"},
		{"VolumeAttachment", "storage.k8s.io/v1", "", "/apis/storage.k8s.io/v1/volumeattachments"},
		{"CSIDriver", "storage.k8s.io/v1", "", "/apis/storage.k8s.io/v1/csidrivers"},
		{"CSINode", "storage.k8s.io/v1", "", "/apis/storage.k8s.io/v1/csinodes"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var calls atomic.Int32
			object := fmt.Sprintf(`{"apiVersion":%q,"kind":%q,"metadata":{"name":"example","namespace":%q,"uid":"uid","resourceVersion":"1","labels":{"sentinel":"omit"}}}`, tc.version, tc.kind, tc.ns)
			s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case tc.path:
					if r.URL.Query().Get("limit") != "100" {
						t.Error("unbounded list")
					}
					if r.URL.Query().Get("continue") == "next" {
						fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"continue":"last"},"items":[]}`, tc.kind+"List", tc.version)
						return
					}
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"metadata":{"continue":"next"},"items":[%s]}`, tc.kind+"List", tc.version, object)
				case tc.path + "/example":
					fmt.Fprint(w, object)
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			})
			ctx := context.Background()
			req := PageRequest{Connection: connection, Kind: tc.kind, Namespace: tc.ns}
			page, err := s.ReadStorage(ctx, req)
			if err != nil || len(page.Items) != 1 || page.Page.NextCursor == "" {
				t.Fatal(page, err)
			}
			row := page.Items[0]
			if strings.Contains(row.ObjectJSON, "sentinel") {
				t.Fatal(row.ObjectJSON)
			}
			ref := row.Resource
			if ref.APIVersion != tc.version || ref.Namespace != tc.ns {
				t.Fatal(ref)
			}
			doc, err := s.ReadResourceYAML(ctx, DetailRequest{Resource: ref})
			if err != nil || !strings.Contains(doc.WithoutManagedFields, "kind: "+tc.kind) {
				t.Fatal(doc, err)
			}
			req.Cursor = page.Page.NextCursor
			continued, err := s.ReadStorage(ctx, req)
			if err != nil || len(continued.Items) != 0 || continued.Page.NextCursor == "" {
				t.Fatal(continued, err)
			}
			before := calls.Load()
			bad := req
			bad.Connection.Generation++
			if _, err := s.ReadStorage(ctx, bad); err == nil {
				t.Fatal("cross-generation cursor accepted")
			}
			bad = req
			if tc.ns == "" {
				bad.Namespace = "test"
				if _, err := s.ReadResourceEvents(ctx, EventRequest{Resource: ref}); err == nil {
					t.Fatal("cluster Events accepted")
				}
			} else {
				bad.Namespace = ""
			}
			if _, err := s.ReadStorage(ctx, bad); err == nil {
				t.Fatal("invalid scope accepted")
			}
			ref.UID = ""
			if _, err := s.ReadResource(ctx, DetailRequest{Resource: ref}); err == nil || calls.Load() != before {
				t.Fatal("invalid read reached API")
			}
		})
	}
}

func TestStorageAttachmentStatusPresence(t *testing.T) {
	for _, status := range []string{"", `,"status":{}`, `,"status":{"attached":false}`, `,"status":{"attached":true}`} {
		t.Run(status, func(t *testing.T) {
			object := `{"metadata":{"name":"attachment","uid":"uid"},"spec":{"attacher":"driver","source":{"persistentVolumeName":"pv"},"nodeName":"node"}` + status + "}"
			s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/attachment") {
					fmt.Fprint(w, object)
				} else {
					fmt.Fprintf(w, `{"items":[%s]}`, object)
				}
			})
			ctx := context.Background()
			page, err := s.ReadStorage(ctx, PageRequest{Connection: connection, Kind: "VolumeAttachment"})
			if err != nil || len(page.Items) != 1 {
				t.Fatal(page, err)
			}
			detail, err := s.ReadResource(ctx, DetailRequest{Resource: page.Items[0].Resource})
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range []string{page.Items[0].ObjectJSON, detail.ObjectJSON} {
				for _, field := range []string{`"attached":true`, `"attached":false`} {
					if strings.Contains(raw, field) != strings.Contains(status, field) {
						t.Fatal(raw)
					}
				}
			}
		})
	}
}

func TestStorageReadFailuresAndCaps(t *testing.T) {
	const listPath = "/apis/storage.k8s.io/v1/storageclasses"
	for _, tc := range []struct {
		name, want, body string
		status           int
		detail           bool
	}{
		{"absent collection", "api_unavailable", `{"kind":"Status","reason":"NotFound","code":404}`, 404, false},
		{"missing target", "not_found", `{"kind":"Status","reason":"NotFound","code":404}`, 404, true},
		{"denied list", "forbidden", `{"kind":"Status","reason":"Forbidden","code":403}`, 403, false},
		{"denied GET", "forbidden", `{"kind":"Status","reason":"Forbidden","code":403}`, 403, true},
		{"expired", "list_changed", `{"kind":"Status","reason":"Expired","code":410}`, 410, false},
		{"wrong identity", "invalid", `{"metadata":{"name":"elsewhere","uid":"uid"}}`, 200, true},
		{"page cap", "capacity", `{"items":[{"metadata":{"name":"example","uid":"uid"},"provisioner":"` + strings.Repeat("x", detailLimit) + `"}]}`, 200, false},
		{"detail cap", "capacity", `{"metadata":{"name":"example","uid":"uid"},"parameters":{"value":"` + strings.Repeat("x", detailLimit) + `"}}`, 200, true},
		{"body cap", "capacity", `{"ignored":"` + strings.Repeat("x", readBodyLimit) + `","items":[]}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				path := listPath
				if tc.detail {
					path += "/example"
				}
				if r.Method != "GET" || r.URL.Path != path {
					t.Error("unexpected request", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			ctx := context.Background()
			var err error
			if tc.detail {
				_, err = s.ReadResource(ctx, DetailRequest{Resource: ResourceRef{
					Connection: connection, Kind: "StorageClass", APIVersion: "storage.k8s.io/v1", Name: "example", UID: "uid",
				}})
			} else {
				_, err = s.ReadStorage(ctx, PageRequest{Connection: connection, Kind: "StorageClass"})
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.want+":") {
				t.Fatal("expected", tc.want, "got", err)
			}
		})
	}
}

func TestStorageClaimQuantitiesAndEvents(t *testing.T) {
	const path = "/api/v1/namespaces/test/persistentvolumeclaims"
	const claim = `{"metadata":{"namespace":"test","name":"claim","uid":"uid"},"spec":{"storageClassName":"","resources":{"requests":{"storage":"1.50Gi"}}},"status":{"phase":"Lost","capacity":{"storage":"0"}}}`
	s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case path:
			fmt.Fprintf(w, `{"items":[%s]}`, claim)
		case path + "/claim":
			fmt.Fprint(w, claim)
		case "/apis/events.k8s.io/v1/namespaces/test/events":
			if got := r.URL.Query().Get("fieldSelector"); got != "regarding.uid=uid" {
				t.Error("missing PVC UID selector", got)
			}
			fmt.Fprint(w, `{"kind":"EventList","apiVersion":"events.k8s.io/v1","items":[]}`)
		default:
			t.Error("unexpected request", r.URL)
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	page, err := s.ReadStorage(ctx, PageRequest{Connection: connection, Kind: "PersistentVolumeClaim", Namespace: "test"})
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	row := page.Items[0]
	for _, text := range []string{`"requested":"1.50Gi"`, `"capacity":"0"`, `"class":""`, `"phase":"Lost"`} {
		if !strings.Contains(row.ObjectJSON, text) {
			t.Fatal(row.ObjectJSON)
		}
	}
	if _, err := s.ReadResourceEvents(ctx, EventRequest{Resource: row.Resource}); err != nil {
		t.Fatal(err)
	}
}
