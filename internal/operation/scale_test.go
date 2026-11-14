/*
Purpose: Verify reviewed scaling against a synthetic API server.
Used by: Native operation regression tests.
Responsibilities: Check fixed routes, preconditions, permission hints, dry-run and single dispatch.
Important: Requests use localhost and synthetic targets only.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
)

func scaleServer(t *testing.T, handler http.HandlerFunc) (*ScaleService, ScaleRequest) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "config")
	data := fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"test","cluster":{"server":%q}}],"contexts":[{"name":"test","context":{"cluster":"test","user":"test"}}],"users":[{"name":"test","user":{}}]}`, server.URL)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := snapshot.Resolve("test")
	if err != nil {
		t.Fatal(err)
	}
	manager := connection.NewManager(nil)
	t.Cleanup(manager.CloseAll)
	c, err := manager.Open(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	s := NewScaleService(manager, nil)
	t.Cleanup(s.Close)
	return s, ScaleRequest{Resource: kube.ResourceRef{Connection: c, Kind: "Deployment", APIVersion: "apps/v1", Name: "web", Namespace: "default", UID: "uid"}, SourceVersion: "7", ReplicasText: "2"}
}

const scalePath = "/apis/apps/v1/namespaces/default/deployments/web"

func testScaleResponse(w http.ResponseWriter, replicas int, version string) {
	fmt.Fprintf(w, `{"kind":"Scale","apiVersion":"autoscaling/v1","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":%q},"spec":{"replicas":%d}}`, version, replicas)
}

func TestScaleRejectsInvalidTargetBeforeIO(t *testing.T) {
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid target reached API")
		w.WriteHeader(500)
	})
	for _, value := range []string{"", "-1", "1.2", " 2", "1e2", "2147483648", "٢", "+3"} {
		invalid := req
		invalid.ReplicasText = value
		if _, err := s.PrepareScale(context.Background(), invalid); err == nil {
			t.Errorf("accepted count %q", value)
		}
	}
	for _, edit := range []func(*ScaleRequest){
		func(r *ScaleRequest) { r.Resource.Kind = "Pod" }, func(r *ScaleRequest) { r.Resource.Namespace = "" },
		func(r *ScaleRequest) { r.Resource.Name = "../other" }, func(r *ScaleRequest) { r.Resource.UID = "" }, func(r *ScaleRequest) { r.SourceVersion = "" },
	} {
		invalid := req
		edit(&invalid)
		if _, err := s.PrepareScale(context.Background(), invalid); err == nil {
			t.Error("accepted invalid identity")
		}
	}
}
func TestScalePrepareAndSingleDispatch(t *testing.T) {
	var dry, writes, reviews atomic.Int32
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET " + scalePath:
			fmt.Fprint(w, `{"kind":"Deployment","apiVersion":"apps/v1","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"spec":{"replicas":1}}`)
		case "GET " + scalePath + "/scale":
			testScaleResponse(w, 1, "7")
		case "POST /apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
			reviews.Add(1)
			w.WriteHeader(403)
		case "PUT " + scalePath + "/scale":
			var body struct {
				Kind, APIVersion string
				Metadata         map[string]any
				Spec             map[string]int
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Kind != "Scale" || body.APIVersion != "autoscaling/v1" || body.Metadata["uid"] != "uid" || body.Metadata["resourceVersion"] != "7" || body.Spec["replicas"] != 2 {
				t.Errorf("incorrect write: %+v", body)
			}
			if r.URL.Query().Get("dryRun") == "All" {
				dry.Add(1)
				testScaleResponse(w, 2, "8")
				return
			}
			writes.Add(1)
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(503)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	noChange := req
	noChange.ReplicasText = "1"
	noop, err := s.PrepareScale(ctx, noChange)
	if err != nil || !noop.NoChange || noop.Token != "" || reviews.Load() != 0 || dry.Load() != 0 {
		t.Fatalf("no-op: %+v %v", noop, err)
	}
	preview, err := s.PrepareScale(ctx, req)
	if err != nil || preview.Token == "" || preview.Before != 1 || preview.After != 2 || preview.Permission != "unknown" || dry.Load() != 1 {
		t.Fatalf("preview %+v %v", preview, err)
	}
	result, err := s.ExecuteScale(ctx, preview.Token, "")
	if err != nil || result.State != "unknown" || writes.Load() != 1 {
		t.Fatalf("result %+v %v", result, err)
	}
	again, err := s.ExecuteScale(ctx, preview.Token, "")
	if err != nil || again.OperationID != result.OperationID || writes.Load() != 1 {
		t.Fatalf("duplicate %+v %v", again, err)
	}
	recovered, err := s.GetScaleResult(result.OperationID)
	if err != nil || recovered.State != "unknown" {
		t.Fatalf("recovered %+v %v", recovered, err)
	}
}

func TestScalePermissionAndDryRun(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		httpCode     int
		dryStatus    int
		dryCount     int
		want         string
	}{
		{"denied", `{"allowed":false,"denied":true}`, 200, 200, 2, "forbidden"},
		{"unknown", `{"allowed":false,"denied":false}`, 200, 200, 2, ""},
		{"contradiction", `{"allowed":true,"denied":true}`, 200, 200, 2, ""},
		{"evaluation error", `{"allowed":true,"evaluationError":"internal"}`, 200, 200, 2, ""},
		{"denied review endpoint", "", 403, 200, 2, ""},
		{"admission changed replicas", `{"allowed":true}`, 200, 200, 3, "invalid_response"},
		{"server rejects preview", `{"allowed":true}`, 200, 422, 2, "dry_run_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var dry atomic.Int32
			s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case "POST":
					if r.URL.Path != "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews" {
						t.Error("wrong permission endpoint")
					}
					w.WriteHeader(tc.httpCode)
					if tc.httpCode == 200 {
						fmt.Fprintf(w, `{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","status":%s}`, tc.status)
					}
				case "PUT":
					dry.Add(1)
					if r.URL.Path != scalePath+"/scale" || r.URL.Query().Get("dryRun") != "All" {
						t.Error("unexpected write")
					}
					w.Header().Add("Warning", `299 kube "Test dry-run warning"`)
					w.WriteHeader(tc.dryStatus)
					if tc.dryStatus == 200 {
						testScaleResponse(w, tc.dryCount, "unpersisted-version")
					}
				default:
					if !scaleFixtureReply(w, r) {
						t.Error("unexpected read")
						w.WriteHeader(404)
					}
				}
			})
			p, err := s.PrepareScale(context.Background(), req)
			checkCode(t, err, tc.want)
			if tc.want == "" && (p.SourceVersion != req.SourceVersion || p.Permission != "unknown" || p.Token == "" || len(p.Warnings) != 1) {
				t.Fatalf("preview %+v", p)
			}
			if tc.want != "" && p.Token != "" {
				t.Fatal("failed preview issued a token")
			}
			wantDry := int32(1)
			if tc.want == "forbidden" {
				wantDry = 0
			}
			if dry.Load() != wantDry {
				t.Fatalf("dry-runs %d want %d", dry.Load(), wantDry)
			}
		})
	}
}

func TestScaleZeroOmittedByServer(t *testing.T) {
	var count atomic.Int32
	s, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			if !scaleFixtureReply(w, r) {
				w.WriteHeader(404)
			}
			return
		}
		if r.URL.Query().Get("dryRun") != "All" {
			count.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"kind":"Scale","apiVersion":"autoscaling/v1","metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"spec":{}}`)
	})
	req.ReplicasText = "0"
	ctx := context.Background()
	p, err := s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExecuteScale(ctx, p.Token, "Web"); err == nil || count.Load() != 0 {
		t.Fatal("wrong zero acknowledgement")
	}
	if _, err := s.ExecuteScale(ctx, p.Token, "web"); err == nil {
		t.Fatal("failed acknowledgement did not consume review")
	}
	p, err = s.PrepareScale(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ExecuteScale(ctx, p.Token, "web")
	if err != nil || result.State != "accepted" || result.Reported == nil || *result.Reported != 0 || count.Load() != 1 {
		t.Fatalf("zero result %+v %v", result, err)
	}
}
