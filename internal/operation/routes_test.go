/*
Purpose: Count reviewed Route writes and inspect conditional patch fields.
Used by: OpenShift operation contract tests.
Responsibilities: Prove write-free cancellation, one dispatch and TLS-safe previews.
Important: Only synthetic loopback API responses are used.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/kube"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReviewedRoutePatch(t *testing.T) {
	const obj = `{"apiVersion":"route.openshift.io/v1","kind":"Route","metadata":{"name":"site","namespace":"default","uid":"r1","resourceVersion":"4"},"spec":{"to":{"kind":"Service","name":"old","future":"retain"},"tls":{"termination":"edge","key":"private-sentinel"}}}`
	dry, real := 0, 0
	allowed := true
	base, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/selfsubjectaccessreviews"):
			fmt.Fprintf(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":%t,"denied":%t}}`, allowed, !allowed)
		case r.Method == "GET":
			fmt.Fprint(w, obj)
		case r.Method == "PATCH":
			b, _ := io.ReadAll(r.Body)
			text := string(b)
			if !strings.Contains(text, "/spec/to/name") || !strings.Contains(text, "/metadata/resourceVersion") || !strings.Contains(text, "/metadata/uid") || strings.Contains(text, "tls") || strings.Contains(text, "future") {
				t.Errorf("unexpected patch %s", text)
			}
			if r.URL.Query().Get("dryRun") == "All" {
				dry++
			} else {
				real++
			}
			fmt.Fprint(w, strings.Replace(obj, `"name":"old"`, `"name":"new"`, 1))
		default:
			t.Errorf("unexpected %v", r.URL)
			w.WriteHeader(404)
		}
	})
	s := fixtureCreateService(t, base)
	q := kube.WorkflowQuery{Connection: req.Resource.Connection, Namespace: "default", Kind: "Route", Name: "site", UID: "r1"}
	p, err := s.reader.ReadWorkflow(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	var r struct{ Spec kube.RouteFields }
	if err := json.Unmarshal([]byte(p.Items[0].Fields), &r); err != nil {
		t.Fatal(err)
	}
	r.Spec.To.Name = "new"
	request := RouteRequest{q, "4", "edit-route", r.Spec}
	prepare := func() WorkflowPreview {
		v, e := s.PrepareRoute(context.Background(), request, "")
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(v.Before+v.Admitted, "private-sentinel") {
			t.Fatal("key disclosure")
		}
		return v
	}
	v := prepare()
	s.RevokeRoute("edit-route", v.Token)
	if _, e := s.ExecuteRoute("edit-route", v.Token, ""); e == nil {
		t.Fatal("revoked token accepted")
	}
	v = prepare()
	if real != 0 || dry != 2 {
		t.Fatal(dry, real)
	}
	res, e := s.ExecuteRoute("edit-route", v.Token, "")
	if e != nil || res.State != "accepted" {
		t.Fatal(res, e)
	}
	_, _ = s.ExecuteRoute("edit-route", v.Token, "")
	if real != 1 {
		t.Fatal(real)
	}
	v = prepare()
	allowed = false
	res, e = s.ExecuteRoute("edit-route", v.Token, "")
	if e != nil || res.State != "cancelled-before-send" || real != 1 {
		t.Fatal("lost authority dispatched", res, e, real)
	}
}
