/*
Purpose: Verify bounded availability capture against synthetic API responses.
Used by: Availability reader tests.
Responsibilities: Preserve native generation comparisons and revision precision.
Important: The test server handles only explicit resource reads.
*/
package kube

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAvailabilityExactEvidence(t *testing.T) {
	for _, tc := range []struct{ kind, version, path, fields, observation, revision string }{
		{"HorizontalPodAutoscaler", "autoscaling/v2", "horizontalpodautoscalers", `"status":{"observedGeneration":9007199254740995,"currentReplicas":0}`, "stale", ""},
		{"PodDisruptionBudget", "policy/v1", "poddisruptionbudgets", `"spec":{"maxUnavailable":"10%"},"status":{"disruptionsAllowed":0}`, "unknown", ""},
		{"ControllerRevision", "apps/v1", "controllerrevisions", `"revision":9007199254740993,"data":{"opaque":"sentinel"}`, "", "9007199254740993"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			path := "/apis/" + tc.version + "/namespaces/test/" + tc.path
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				object := fmt.Sprintf(`{"kind":%q,"apiVersion":%q,"metadata":{"name":"example","namespace":"test","uid":"uid","resourceVersion":"4","generation":9007199254740993},%s}`, tc.kind, tc.version, tc.fields)
				if r.Method != "GET" {
					t.Error(r.Method)
				}
				switch r.URL.Path {
				case path:
					if r.URL.Query().Get("limit") != "100" {
						t.Error("unbounded list")
					}
					fmt.Fprintf(w, `{"kind":%q,"apiVersion":%q,"items":[%s]}`, tc.kind+"List", tc.version, object)
				case path + "/example":
					fmt.Fprint(w, object)
				default:
					t.Error(r.URL)
					http.NotFound(w, r)
				}
			})
			ctx := context.Background()
			page, err := s.ReadAvailability(ctx, PageRequest{Kind: tc.kind, Namespace: "test", Connection: c})
			if err != nil || len(page.Items) != 1 {
				t.Fatal(page, err)
			}
			row := page.Items[0]
			if row.Summary.Observation != tc.observation || row.Summary.Revision != tc.revision || strings.Contains(row.ObjectJSON, "sentinel") {
				t.Fatal(row)
			}
			if tc.kind == "HorizontalPodAutoscaler" && (row.Summary.Generation != "9007199254740993" || row.Summary.ObservedGeneration != "9007199254740995") {
				t.Fatal("generation precision lost", row.Summary)
			}
			got, err := s.ReadResource(ctx, DetailRequest{Resource: row.Resource})
			if err != nil || got.Availability == nil || *got.Availability != row.Summary {
				t.Fatal(got, err)
			}
			yaml, err := s.ReadResourceYAML(ctx, DetailRequest{Resource: row.Resource})
			if err != nil || !strings.Contains(yaml.WithoutManagedFields, "9007199254740993") {
				t.Fatal(yaml, err)
			}
		})
	}
}

func TestAvailabilityReadFailuresAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name, want, body string
		status           int
		detail           bool
	}{
		{"unserved list", "api_unavailable", `{"kind":"Status","reason":"NotFound","code":404}`, 404, false},
		{"missing revision", "not_found", `{"kind":"Status","reason":"NotFound","code":404}`, 404, true},
		{"denied list", "forbidden", `{"kind":"Status","reason":"Forbidden","code":403}`, 403, false},
		{"denied get", "forbidden", `{"kind":"Status","reason":"Forbidden","code":403}`, 403, true},
		{"expired cursor", "list_changed", `{"kind":"Status","reason":"Expired","code":410}`, 410, false},
		{"oversized revision", "capacity", `{"data":{"payload":"` + strings.Repeat("x", detailLimit) + `"}}`, 200, true},
		{"oversized response", "capacity", `{"ignored":"` + strings.Repeat("x", readBodyLimit) + `"}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				want := "/apis/apps/v1/namespaces/test/controllerrevisions"
				if tc.detail {
					want += "/revision"
				}
				if r.URL.Path != want || r.Method != "GET" {
					t.Error(r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			ctx := context.Background()
			var err error
			if tc.detail {
				_, err = s.ReadResource(ctx, DetailRequest{Resource: ResourceRef{Connection: c, Kind: "ControllerRevision", APIVersion: "apps/v1", Namespace: "test", Name: "revision", UID: "uid"}})
			} else {
				_, err = s.ReadAvailability(ctx, PageRequest{Connection: c, Kind: "ControllerRevision", Namespace: "test"})
			}
			if err == nil || !strings.HasPrefix(err.Error(), tc.want+":") {
				t.Fatal(tc.want, err)
			}
		})
	}
}
