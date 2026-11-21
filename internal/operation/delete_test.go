/*
Purpose: Verify fixed deletion targets, mandatory preview and bounded evidence.
Used by: Native operation regression tests.
Responsibilities: Exercise the exact wire options and reject unsafe inputs.
Important: All targets and API responses are synthetic.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
)

func TestDeleteFixedTargets(t *testing.T) {
	for kind, route := range map[string]string{
		"Pod": "v1/pods", "ReplicationController": "v1/replicationcontrollers",
		"Deployment": "apps/v1/deployments", "ReplicaSet": "apps/v1/replicasets",
		"StatefulSet": "apps/v1/statefulsets", "DaemonSet": "apps/v1/daemonsets",
		"Job": "batch/v1/jobs", "CronJob": "batch/v1/cronjobs",
	} {
		api, plural, _ := strings.Cut(route, "/")
		if api != "v1" {
			api, plural = api+"/v1", strings.TrimPrefix(plural, "v1/")
		}
		prefix := "/apis/"
		if api == "v1" {
			prefix = "/api/"
		}
		ref := kube.ResourceRef{Kind: kind, APIVersion: api, Namespace: "default", Name: "web", UID: "uid"}
		if got, resource := deleteTarget(ref); got != prefix+api+"/namespaces/default/"+plural+"/web" || resource != plural {
			t.Errorf("%s: %q %q", kind, got, resource)
		}
		for _, invalid := range []kube.ResourceRef{
			{Kind: kind, APIVersion: api, Namespace: "", Name: "web", UID: "uid"},
			{Kind: kind, APIVersion: "unknown/v1", Namespace: "default", Name: "web", UID: "uid"},
			{Kind: "ConfigMap", APIVersion: "v1", Namespace: "default", Name: "web", UID: "uid"},
			{Kind: kind, APIVersion: api, Namespace: "default", Name: "../web", UID: "uid"},
		} {
			if path, _ := deleteTarget(invalid); path != "" {
				t.Errorf("accepted invalid ref %+v", invalid)
			}
		}
	}
}

func TestDeletePermissionAndPreview(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer string
		denied bool
		valid  bool
	}{
		{"missing hint", "", false, true},
		{"object", restartFixture("Deployment", "{}", "{}"), false, true},
		{"wrong status details", `{"apiVersion":"v1","kind":"Status","status":"Success","details":{"kind":"pods"}}`, false, false},
		{"contradictory success", `{"apiVersion":"v1","kind":"Status","status":"Success","reason":"Forbidden"}`, false, false},
		{"failure status", `{"apiVersion":"v1","kind":"Status","status":"Failure"}`, false, false},
		{"denied", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					fmt.Fprint(w, restartFixture("Deployment", "{}", "{}"))
				case http.MethodPost:
					var review struct {
						Spec struct{ ResourceAttributes map[string]string }
					}
					if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
						t.Error(err)
					}
					a := review.Spec.ResourceAttributes
					if a["verb"] != "delete" || a["group"] != "apps" || a["resource"] != "deployments" || a["namespace"] != "default" || a["name"] != "web" || a["subresource"] != "" {
						t.Errorf("wrong review: %+v", a)
					}
					if tc.denied {
						fmt.Fprint(w, `{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","status":{"denied":true}}`)
					} else {
						w.WriteHeader(http.StatusForbidden)
					}
				case http.MethodDelete:
					calls.Add(1)
					var options map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
						t.Error(err)
					}
					if r.URL.Path != scalePath || r.URL.RawQuery != "" || len(options) != 5 ||
						string(options["dryRun"]) != `["All"]` || string(options["preconditions"]) != `{"uid":"uid","resourceVersion":"7"}` ||
						string(options["kind"]) != `"DeleteOptions"` || string(options["apiVersion"]) != `"v1"` ||
						string(options["propagationPolicy"]) != `"Background"` {
						t.Errorf("wrong DELETE options: %v", options)
					}
					if tc.answer == "" {
						fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Success"}`)
					} else {
						fmt.Fprint(w, tc.answer)
					}
				}
			})
			service := NewDeleteService(scale.connections, scale.coordinator, nil)
			preview, err := service.PrepareDelete(context.Background(), DeleteRequest{req.Resource, req.SourceVersion})
			if (err == nil) != tc.valid || (preview.Token != "") != tc.valid || (calls.Load() == 0) != tc.denied {
				t.Fatalf("preview=%+v error=%v calls=%d", preview, err, calls.Load())
			}
		})
	}
}

func TestDeleteImpactProjection(t *testing.T) {
	owner := `{"apiVersion":"apps/v1","kind":"ReplicaSet","name":"web-owner","uid":"owner-id","controller":true}`
	finalizers, _ := json.Marshal(makeFinalizers())
	raw := fmt.Sprintf(`{"metadata":{"ownerReferences":[%s],"finalizers":%s,"annotations":{"private":"private-value"}},"spec":{"containers":[{"env":[{"value":"private-config"}]}]}}`, owner, finalizers)
	impact, err := projectDeleteImpact(json.RawMessage(raw), "Pod")
	if err != nil || impact.Owner == nil || impact.Owner.UID != "owner-id" || impact.OwnershipUnknown ||
		impact.PodGrace == nil || *impact.PodGrace != 30 || !impact.PodGraceDefault ||
		!impact.FinalizersPartial || impact.FinalizerCount != 110 || len(impact.Finalizers) >= 110 {
		t.Fatalf("unexpected impact: %+v %v", impact, err)
	}
	encoded, _ := json.Marshal(impact)
	if strings.Contains(string(encoded), "private") {
		t.Fatal("private configuration escaped projection")
	}
	for _, refs := range []string{owner + "," + owner, owner + ",{}", "null"} {
		got, err := projectDeleteImpact(json.RawMessage(`{"metadata":{"ownerReferences":[`+refs+`]}}`), "Pod")
		if err != nil || !got.OwnershipUnknown || got.Owner != nil {
			t.Fatalf("ambiguous evidence was treated as known: %+v %v", got, err)
		}
	}
	for _, tc := range []struct {
		kind, raw           string
		eligible, defaulted bool
	}{
		{"Pod", `{"metadata":{"annotations":{"kubernetes.io/config.mirror":""}}}`, false, false},
		{"StatefulSet", `{"spec":{}}`, true, true},
		{"StatefulSet", `{"spec":{"persistentVolumeClaimRetentionPolicy":{"whenScaled":"Delete","whenDeleted":"Retain"}}}`, true, false},
		{"StatefulSet", `{"spec":{"persistentVolumeClaimRetentionPolicy":{"whenDeleted":"Delete"}}}`, false, false},
		{"StatefulSet", `{"spec":{"persistentVolumeClaimRetentionPolicy":{"whenDeleted":null}}}`, false, false},
		{"StatefulSet", `{"spec":{"persistentVolumeClaimRetentionPolicy":{"whenDeleted":""}}}`, false, false},
	} {
		got, err := projectDeleteImpact(json.RawMessage(tc.raw), tc.kind)
		if (err == nil) != tc.eligible || got.RetentionDefault != tc.defaulted {
			t.Fatalf("%s: %+v %v", tc.raw, got, err)
		}
	}
}

func makeFinalizers() []string {
	names := make([]string, 110)
	for index := range names {
		names[index] = fmt.Sprintf("%s.kubesift.dev/fixture-%d", strings.Repeat("f", 60), index)
	}
	return names
}

func TestDeleteRejectsMissingStatefulSetSpecBeforeWrite(t *testing.T) {
	for _, malformed := range []string{
		restartFixture("StatefulSet", "null", "{}"),
		strings.Replace(restartFixture("StatefulSet", "{}", "{}"), `"spec":{},`, "", 1),
	} {
		var valid atomic.Bool
		var writes atomic.Int32
		scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				if valid.Load() {
					fmt.Fprint(w, restartFixture("StatefulSet", "{}", "{}"))
				} else {
					fmt.Fprint(w, malformed)
				}
			case http.MethodPost:
				w.WriteHeader(http.StatusForbidden)
			case http.MethodDelete:
				writes.Add(1)
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Success"}`)
			}
		})
		ctx := context.Background()
		service := NewDeleteService(scale.connections, scale.coordinator, nil)
		req.Resource.Kind = "StatefulSet"
		request := DeleteRequest{req.Resource, req.SourceVersion}
		_, err := service.PrepareDelete(ctx, request)
		checkCode(t, err, "invalid_response")
		if writes.Load() != 0 {
			t.Fatal("preparation reached DELETE with missing spec")
		}
		valid.Store(true)
		review, err := service.PrepareDelete(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		valid.Store(false)
		result, err := service.ExecuteDelete(ctx, review.Token, req.Resource.Name)
		if err != nil || result.State != "cancelled-before-send" || result.Problem == nil ||
			result.Problem.Code != "invalid_response" || writes.Load() != 1 {
			t.Fatalf("malformed recheck crossed the write boundary: %+v %v", result, err)
		}
	}
}
