/*
Purpose: Test selector and ownership boundaries with a synthetic HTTP API.
Used by: Availability relationship regression tests.
Responsibilities: Verify null and empty selectors, continuation and the revision scan budget.
Important: No permission probes or mutations are made.
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

func TestSelectedPodsSelectorAndContinuation(t *testing.T) {
	for _, tc := range []struct {
		selector string
		list     bool
		query    string
	}{
		{"null", false, ""},
		{"{}", true, ""},
		{`{"matchExpressions":[{"key":"app","operator":"In","values":["owned"]}]}`, true, "app in (owned)"},
		{`{"matchExpressions":[{"key":"app","operator":"Invalid"}]}`, false, ""},
	} {
		t.Run(tc.selector, func(t *testing.T) {
			var reads atomic.Int32
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/apis/policy/v1/namespaces/test/poddisruptionbudgets/budget":
					fmt.Fprintf(w, `{"kind":"PodDisruptionBudget","apiVersion":"policy/v1","metadata":{"name":"budget","namespace":"test","uid":"budget-uid","resourceVersion":"7"},"spec":{"selector":%s}}`, tc.selector)
				case "/api/v1/namespaces/test/pods":
					reads.Add(1)
					if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("labelSelector") != tc.query {
						t.Error(r.URL)
					}
					fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","metadata":{"continue":"next"},"items":[{"metadata":{"name":"wrong-ns","namespace":"elsewhere","uid":"pod","labels":{"app":"owned"}}},{"metadata":{"name":"orphan","namespace":"test","uid":"orphan","labels":{"app":"other"}}}]}`)
				default:
					t.Error(r.URL)
					http.NotFound(w, r)
				}
			})
			req := RelatedRequest{Resource: ResourceRef{Connection: c, Kind: "PodDisruptionBudget", APIVersion: "policy/v1", Namespace: "test", Name: "budget", UID: "budget-uid"}}
			ctx := context.Background()
			got, err := s.ReadPDBSelectedPods(ctx, req)
			if strings.Contains(tc.selector, "Invalid") {
				if err == nil || reads.Load() != 0 {
					t.Fatal(got, err)
				}
				return
			}
			if err != nil || got.SourceVersion != "7" {
				t.Fatal(got, err)
			}
			if !tc.list {
				if !got.Complete || reads.Load() != 0 {
					t.Fatal(got)
				}
				req.Cursor = "invalid"
				if _, err := s.ReadPDBSelectedPods(ctx, req); err == nil {
					t.Fatal("ignored cursor")
				}
				return
			}
			want := 0
			if tc.query == "" {
				want = 1
			}
			if reads.Load() != 1 || len(got.Items) != want || got.Page.NextCursor == "" || got.Complete {
				t.Fatal(got)
			}
			req.Cursor = got.Page.NextCursor
			req.Resource.Name = "budget"
			req.Resource.Connection.Generation++
			if _, err := s.ReadPDBSelectedPods(ctx, req); err == nil || reads.Load() != 1 {
				t.Fatal("mixed scope accepted")
			}
		})
	}
}

func TestRevisionScanBudgetAndOwnerFields(t *testing.T) {
	var reads atomic.Int32
	s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/test/statefulsets/set":
			fmt.Fprint(w, `{"kind":"StatefulSet","apiVersion":"apps/v1","metadata":{"name":"set","namespace":"test","uid":"parent","resourceVersion":"7"},"status":{"currentRevision":"old","updateRevision":"new"}}`)
		case "/apis/apps/v1/namespaces/test/controllerrevisions":
			n := reads.Add(1)
			if r.URL.Query().Get("limit") != "100" {
				t.Error(r.URL)
			}
			if n <= 8 {
				fmt.Fprintf(w, `{"metadata":{"continue":%q},"items":[{"metadata":{"name":"unrelated","namespace":"test","uid":"ref","ownerReferences":[{"kind":"StatefulSet","apiVersion":"apps/v1","name":"wrong","uid":"parent","controller":true}]}}]}`, fmt.Sprint(n))
				return
			}
			if r.URL.Query().Get("continue") != "8" {
				t.Error(r.URL)
			}
			fmt.Fprint(w, `{"items":[{"revision":9007199254740993,"metadata":{"name":"new","namespace":"test","uid":"ref","ownerReferences":[{"kind":"StatefulSet","apiVersion":"apps/v1","name":"set","uid":"parent","controller":true}]}}]}`)
		default:
			t.Error(r.URL)
			http.NotFound(w, r)
		}
	})
	req := RelatedRequest{Resource: ResourceRef{Connection: c, Kind: "StatefulSet", APIVersion: "apps/v1", Namespace: "test", Name: "set", UID: "parent"}}
	ctx := context.Background()
	first, err := s.ReadControllerRevisions(ctx, req)
	if err != nil || reads.Load() != 8 || len(first.Items) != 0 || first.Complete || first.Page.NextCursor == "" || first.SourceVersion != "7" {
		t.Fatal(first, err)
	}
	req.Cursor = first.Page.NextCursor
	last, err := s.ReadControllerRevisions(ctx, req)
	if err != nil || !last.Complete || len(last.Items) != 1 || last.Items[0].Summary.Revision != "9007199254740993" || last.CurrentRevision != "old" || last.UpdateRevision != "new" {
		t.Fatal(last, err)
	}
}

func TestReplicationControllerOwnershipAndZeroStatus(t *testing.T) {
	for _, selector := range []string{`{"app":"owned"}`, `{}`} {
		t.Run(selector, func(t *testing.T) {
			var lists atomic.Int32
			s, c := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				const root = "/api/v1/namespaces/test/"
				meta := `"metadata":{"name":"fixture","namespace":"test","uid":"parent","resourceVersion":"7"}`
				base := fmt.Sprintf(`{%s,"spec":{"selector":%s,"replicas":0},"status":{"observedGeneration":1,"replicas":0}}`, meta, selector)
				switch r.URL.Path {
				case root + "replicationcontrollers":
					fmt.Fprintf(w, `{"kind":"ReplicationControllerList","apiVersion":"v1","items":[%s,{"metadata":{"name":"waiting","namespace":"test","uid":"other"}}]}`, base)
				case root + "replicationcontrollers/fixture":
					fmt.Fprintf(w, `{"kind":"ReplicationController","apiVersion":"v1",%s`, base[1:])
				case root + "pods":
					lists.Add(1)
					if r.URL.Query().Get("labelSelector") != "app=owned" {
						t.Error(r.URL)
					}
					fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","items":[
					{"metadata":{"name":"owned","uid":"owned","namespace":"test","labels":{"app":"owned"},"ownerReferences":[{"apiVersion":"v1","kind":"ReplicationController","name":"fixture","uid":"parent","controller":true}]}},
					{"metadata":{"name":"orphan","uid":"orphan","namespace":"test","labels":{"app":"owned"}}},
					{"metadata":{"name":"different","uid":"diff","namespace":"test","labels":{"app":"owned"},"ownerReferences":[{"apiVersion":"v1","kind":"ReplicationController","name":"fixture","uid":"other","controller":true}]}}]}`)
				default:
					t.Error(r.URL)
					http.NotFound(w, r)
				}
			})
			ctx := context.Background()
			page, err := s.ReadWorkloads(ctx, PageRequest{Connection: c, Kind: "ReplicationController", Namespace: "test"})
			if err != nil || len(page.Items) != 2 || page.Items[0].Desired == nil || page.Items[0].Current == nil || *page.Items[0].Current != 0 ||
				page.Items[1].Current != nil {
				t.Fatal(page, err)
			}
			related, err := s.ReadRelatedPods(ctx, RelatedRequest{Resource: page.Items[0].Resource})
			if selector == "{}" {
				if err == nil || lists.Load() != 0 {
					t.Fatal("unrestricted scan", related, err)
				}
			} else if err != nil || len(related.Items) != 1 || related.Items[0].Pod.Resource.Name != "owned" {
				t.Fatal(related, err)
			}
		})
	}
}
