/*
Purpose: Exercise Node inventory, reviewed scheduling and drain failure boundaries.
Used by: Go unit and race tests with scripted API responses.
Responsibilities: Assert preconditions, denied and uncertain writes, cancellation and final observation.
Important: Only loopback transports and synthetic objects are used here.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sampleNodePod() core.Pod {
	return core.Pod{TypeMeta: meta.TypeMeta{Kind: "Pod", APIVersion: "v1"}, ObjectMeta: meta.ObjectMeta{Name: "pod", UID: "one", Namespace: "ns", ResourceVersion: "8", OwnerReferences: []meta.OwnerReference{{Kind: "ReplicaSet", APIVersion: "apps/v1", UID: "owner", Name: "rs", Controller: ptr.To(true)}}}, Spec: core.PodSpec{NodeName: "node"}}
}
func TestNodeReviewInventory(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		change      func(*core.Pod)
	}{
		{"eligible", "candidate", func(*core.Pod) {}},
		{"mirror", "skipped", func(p *core.Pod) {
			p.Annotations = map[string]string{core.MirrorPodAnnotationKey: "hash"}
			p.OwnerReferences = nil
		}},
		{"live daemon", "skipped", func(p *core.Pod) { p.OwnerReferences[0].Kind = "DaemonSet" }},
		{"orphan daemon", "blocked", func(p *core.Pod) { p.OwnerReferences[0].Kind = "DaemonSet"; p.OwnerReferences[0].UID = "different" }},
		{"terminating", "terminating", func(p *core.Pod) {
			p.DeletionTimestamp = &meta.Time{Time: time.Now()}
			p.Finalizers = []string{"fixture/finalizer"}
			p.OwnerReferences = nil
		}},
		{"bare", "blocked", func(p *core.Pod) { p.OwnerReferences = nil }}, {"unknown", "blocked", func(p *core.Pod) { p.OwnerReferences[0].Kind = "Unknown" }},
		{"multiple", "blocked", func(p *core.Pod) { p.OwnerReferences = append(p.OwnerReferences, p.OwnerReferences[0]) }},
		{"new owner", "blocked", func(p *core.Pod) { p.OwnerReferences[0].UID = "different" }},
		{"emptyDir", "blocked", func(p *core.Pod) {
			p.Spec.Volumes = []core.Volume{{VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{Medium: core.StorageMediumMemory}}}}
		}},
		{"hostPath", "blocked", func(p *core.Pod) {
			p.Spec.Volumes = []core.Volume{{VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{}}}}
		}},
		{"generic ephemeral", "blocked", func(p *core.Pod) {
			p.Spec.Volumes = []core.Volume{{VolumeSource: core.VolumeSource{Ephemeral: &core.EphemeralVolumeSource{}}}}
		}},
		{"inline CSI", "blocked", func(p *core.Pod) {
			p.Spec.Volumes = []core.Volume{{VolumeSource: core.VolumeSource{CSI: &core.CSIVolumeSource{}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleNodePod()
			tc.change(&p)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				kind := "ReplicaSet"
				if strings.Contains(r.URL.Path, "daemonsets") {
					kind = "DaemonSet"
				}
				fmt.Fprintf(w, `{"apiVersion":"apps/v1","kind":%q,"metadata":{"name":"rs","namespace":"ns","uid":"owner"}}`, kind)
			}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			row, e := (operationClient{server.Client(), u}).classify(context.Background(), p, map[string]bool{})
			if e != nil || row.State != tc.state {
				t.Fatalf("%+v %v", row, e)
			}
		})
	}
}
func TestNodeReviewPages(t *testing.T) {
	for _, mode := range []string{"complete", "duplicate", "expired", "too many", "inconsistent"} {
		t.Run(mode, func(t *testing.T) {
			pages := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/v1/pods" || r.URL.Query().Get("fieldSelector") != "spec.nodeName=node" || r.URL.Query().Get("limit") != "100" {
					t.Errorf("unexpected request %s", r.URL)
				}
				pages++
				if pages == 2 && mode == "expired" {
					w.WriteHeader(410)
					return
				}
				list := core.PodList{TypeMeta: meta.TypeMeta{Kind: "PodList", APIVersion: "v1"}, ListMeta: meta.ListMeta{ResourceVersion: "7"}}
				count := 1
				if mode == "too many" {
					count = 100
				}
				for j := 0; j < count; j++ {
					p := sampleNodePod()
					p.OwnerReferences = nil
					p.Name = fmt.Sprintf("p-%d-%d", pages, j)
					p.UID = types.UID(p.Name)
					p.Namespace = fmt.Sprintf("ns-%d", pages)
					if mode == "duplicate" {
						p.UID = "one"
					}
					list.Items = append(list.Items, p)
				}
				if pages == 1 || mode == "too many" {
					list.Continue = fmt.Sprint(pages)
				}
				if pages == 2 && mode == "inconsistent" {
					list.ResourceVersion = "8"
				}
				_ = json.NewEncoder(w).Encode(list)
			}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			rows, e := (operationClient{server.Client(), u}).inventory(context.Background(), "node")
			if mode == "complete" {
				if e != nil || len(rows) != 2 || rows[0].Namespace == rows[1].Namespace {
					t.Fatal(rows, e)
				}
			} else if e == nil {
				t.Fatal("unsafe inventory accepted")
			}
		})
	}
}

func TestDrainExecution(t *testing.T) {
	for _, tc := range []struct {
		mode, state string
		posts       int32
	}{
		{"complete", "completed", 1}, {"same-name replacement", "partial", 1}, {"final new Pod", "partial", 1}, {"lost response", "unknown", 1}, {"forbidden", "partial", 1}, {"conflict", "partial", 1}, {"throttle", "completed", 2}, {"long retry", "timeout", 1}, {"cancel read", "cancelled", 0}, {"budget cancel", "cancelled", 1}, {"disconnect", "cancelled", 1}, {"shutdown", "cancelled", 1}, {"owner changed", "partial", 0}, {"externally uncordoned", "partial", 0}, {"impact changed", "blocked", 0}, {"denied list", "prepare rejected", 0}, {"no Pod GET", "blocked review", 0}, {"already cordoned", "completed", 1},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var posts, patches, lists, nodeGets, owners atomic.Int32
			var cordoned atomic.Bool
			cordoned.Store(tc.mode == "already cordoned")
			waiting := make(chan struct{}, 2)
			s, base := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					t.Error("unexpected DELETE")
					w.WriteHeader(405)
					return
				}
				object := sampleNodePod()
				switch r.URL.Path {
				case "/api/v1/nodes/node":
					state := cordoned.Load()
					if r.Method == "PATCH" {
						var ops []struct {
							Op, Path string
							Value    any
						}
						_ = json.NewDecoder(r.Body).Decode(&ops)
						if len(ops) != 3 || ops[0].Value != "uid" || ops[1].Value != "7" || ops[2].Path != "/spec/unschedulable" {
							t.Error("unguarded patch")
						}
						state = true
						if r.URL.Query().Get("dryRun") != "All" {
							patches.Add(1)
							cordoned.Store(true)
						}
					} else if nodeGets.Add(1) >= 4 && tc.mode == "externally uncordoned" {
						state = false
					}
					fmt.Fprintf(w, `{"kind":"Node","apiVersion":"v1","metadata":{"name":"node","uid":"uid","resourceVersion":"7"},"spec":{"unschedulable":%t}}`, state)
				case "/api/v1":
					fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"pods/eviction","group":"policy","version":"v1","kind":"Eviction","verbs":["create"]}]}`)
				case "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
					var req struct {
						Spec struct {
							ResourceAttributes struct{ Name, Namespace, Verb, Resource string }
						}
					}
					_ = json.NewDecoder(r.Body).Decode(&req)
					attrs := req.Spec.ResourceAttributes
					if tc.mode == "no Pod GET" && attrs.Name == "pod" && attrs.Namespace == "ns" && attrs.Verb == "get" && attrs.Resource == "pods" {
						fmt.Fprint(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"denied":true}}`)
						return
					}
					fmt.Fprint(w, `{"kind":"SelfSubjectAccessReview","apiVersion":"authorization.k8s.io/v1","status":{"allowed":true}}`)
				case "/api/v1/pods":
					if tc.mode == "denied list" {
						w.WriteHeader(403)
						return
					}
					num := lists.Add(1)
					items := []core.Pod{object}
					if num == 2 && tc.mode == "impact changed" {
						items[0].OwnerReferences = nil
					}
					if num >= 3 {
						items = nil
						if tc.mode == "final new Pod" || tc.mode == "same-name replacement" {
							object.UID = "new"
							items = []core.Pod{object}
						}
					}
					_ = json.NewEncoder(w).Encode(core.PodList{TypeMeta: meta.TypeMeta{Kind: "PodList", APIVersion: "v1"}, ListMeta: meta.ListMeta{ResourceVersion: "7"}, Items: items})
				case "/apis/apps/v1/namespaces/ns/replicasets/rs":
					uid := "owner"
					if owners.Add(1) >= 3 && tc.mode == "owner changed" {
						uid = "new"
					}
					fmt.Fprintf(w, `{"kind":"ReplicaSet","apiVersion":"apps/v1","metadata":{"name":"rs","namespace":"ns","uid":%q}}`, uid)
				case "/api/v1/namespaces/ns/pods/pod":
					if tc.mode == "cancel read" {
						waiting <- struct{}{}
						<-r.Context().Done()
						return
					}
					if posts.Load() > 0 && (tc.mode != "throttle" || posts.Load() > 1) {
						if tc.mode == "same-name replacement" {
							object.UID = "new"
						} else {
							w.WriteHeader(404)
							fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"NotFound","code":404,"details":{"name":"pod","kind":"pods"}}`)
							return
						}
					}
					_ = json.NewEncoder(w).Encode(object)
				case "/api/v1/namespaces/ns/pods/pod/eviction":
					var body struct {
						Kind, APIVersion string
						DeleteOptions    meta.DeleteOptions
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					if r.Method != "POST" || body.APIVersion != "policy/v1" || body.Kind != "Eviction" || body.DeleteOptions.Preconditions == nil || *body.DeleteOptions.Preconditions.UID != "one" || *body.DeleteOptions.Preconditions.ResourceVersion != "8" || body.DeleteOptions.GracePeriodSeconds != nil {
						t.Error("invalid eviction")
					}
					n := posts.Add(1)
					switch tc.mode {
					case "lost response":
						w.WriteHeader(500)
					case "forbidden":
						w.WriteHeader(403)
					case "conflict":
						w.WriteHeader(409)
					case "budget cancel", "disconnect", "shutdown":
						w.WriteHeader(429)
						fmt.Fprint(w, `{"message":"would violate the pod disruption budget"}`)
						waiting <- struct{}{}
					case "long retry":
						w.Header().Set("Retry-After", "999")
						w.WriteHeader(429)
					case "throttle":
						if n == 1 {
							w.Header().Set("Retry-After", "1")
							w.WriteHeader(429)
							return
						}
						fallthrough
					default:
						fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Success"}`)
					}
				default:
					t.Errorf("unexpected %s", r.URL)
					w.WriteHeader(404)
				}
			})
			c := s.coordinator
			r := kube.ResourceRef{Kind: "Node", APIVersion: "v1", Name: "node", UID: "uid", Connection: base.Resource.Connection}
			p, e := c.PrepareNodeAction(context.Background(), NodeRequest{r, "7", "drain", 2}, "Development")
			if tc.state == "blocked review" {
				if e != nil || p.Token != "" || p.Counts["blocked"] != 1 {
					t.Fatal("did not block missing named GET")
				}
				return
			}
			if tc.state == "prepare rejected" {
				if e == nil {
					t.Fatal("denied inventory accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			result, e := c.ExecuteNodeAction(p.Token, "node")
			if e != nil {
				t.Fatal(e)
			}
			again, e := c.ExecuteNodeAction(p.Token, "node")
			if e != nil || again.OperationID != result.OperationID {
				t.Fatal("replay failed")
			}
			switch tc.mode {
			case "cancel read", "budget cancel", "disconnect", "shutdown":
				select {
				case <-waiting:
				case <-time.After(3 * time.Second):
					t.Fatal("never reached budget")
				}
				switch tc.mode {
				case "disconnect":
					c.CloseConnection(r.Connection)
				case "shutdown":
					c.Close()
				default:
					c.CancelDrain("unrelated")
					c.CancelDrain(result.OperationID)
				}
			}
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				result, e = c.GetOperationResult(result.OperationID)
				if e != nil {
					t.Fatal(e)
				}
				if result.FinishedAt != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if result.State != tc.state || posts.Load() != tc.posts {
				t.Fatalf("state %s (%s), posts %d", result.State, result.Node.Reason, posts.Load())
			}
			if (tc.mode == "impact changed" || tc.mode == "already cordoned") && patches.Load() != 0 {
				t.Fatal("unexpected real scheduling patch")
			}
			if tc.mode == "final new Pod" || tc.mode == "same-name replacement" {
				if len(result.Node.Rows) != 2 || result.Node.Counts["gone"] != 1 || result.Node.Counts["unreviewed"] != 1 {
					t.Fatalf("lost final evidence: %+v", result.Node)
				}
			}
			if tc.mode == "cancel read" && (result.Node.Cordoned == nil || !*result.Node.Cordoned) {
				t.Fatal("lost observed cordon on read cancellation")
			}
			if len(c.ListOperationResults()[0].Node.Rows) != 0 {
				t.Fatal("list exposed detailed rows")
			}
		})
	}
}

func TestNodeReviewScheduling(t *testing.T) {
	for _, mode := range []string{"production", "no-op", "stale version", "wrong UID", "namespace", "wrong version", "access denied", "access unknown", "dry drift", "metadata drift", "dry denied", "actual denied", "ambiguous"} {
		t.Run(mode, func(t *testing.T) {
			var real, dry atomic.Int32
			s, base := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews":
					if mode == "access unknown" {
						w.WriteHeader(500)
						return
					}
					fmt.Fprintf(w, `{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","status":{"allowed":%t,"denied":%t}}`, mode != "access denied", mode == "access denied")
				case r.URL.Path == "/api/v1/nodes/node":
					metadata := map[string]any{"name": "node", "uid": "uid", "resourceVersion": "7"}
					spec := map[string]any{}
					if mode == "no-op" {
						spec["unschedulable"] = true
					}
					if r.Method == "PATCH" {
						spec["unschedulable"] = true
						if r.URL.Query().Get("dryRun") == "All" {
							dry.Add(1)
							if mode == "metadata drift" {
								metadata["labels"] = map[string]string{"unreviewed": "true"}
							}
							if mode == "dry drift" {
								spec["taints"] = []core.Taint{{Key: "unreviewed", Effect: core.TaintEffectNoSchedule}}
							}
							if mode == "dry denied" {
								w.WriteHeader(403)
								return
							}
						} else {
							real.Add(1)
							if mode == "actual denied" {
								w.WriteHeader(403)
								return
							}
							if mode == "ambiguous" {
								w.WriteHeader(504)
								return
							}
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Node", "metadata": metadata, "spec": spec})
				default:
					t.Error("unexpected path")
				}
			})
			req := NodeRequest{kube.ResourceRef{Connection: base.Resource.Connection, Kind: "Node", APIVersion: "v1", Name: "node", UID: "uid"}, "7", "cordon", 5}
			switch mode {
			case "stale version":
				req.SourceVersion = "6"
			case "wrong UID":
				req.Resource.UID = "different"
			case "namespace":
				req.Resource.Namespace = "default"
			case "wrong version":
				req.Resource.APIVersion = "v2"
			}
			p, e := s.coordinator.PrepareNodeAction(context.Background(), req, "Production")
			switch mode {
			case "no-op":
				if e != nil || !p.NoChange || p.Token != "" || dry.Load() != 0 {
					t.Fatal(p, e)
				}
			case "production":
				if e != nil || !p.Typed {
					t.Fatal(p, e)
				}
				_, e = s.coordinator.ExecuteNodeAction(p.Token, "")
				checkCode(t, e, "acknowledgement")
			case "actual denied", "ambiguous":
				if e != nil {
					t.Fatal(e)
				}
				r, e := s.coordinator.ExecuteNodeAction(p.Token, "node")
				if e != nil {
					t.Fatal(e)
				}
				for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && r.FinishedAt == nil; {
					r, _ = s.coordinator.GetOperationResult(r.OperationID)
					time.Sleep(time.Millisecond)
				}
				if (mode == "actual denied" && r.State != "blocked") || (mode == "ambiguous" && r.State != "unknown") || real.Load() != 1 {
					t.Fatal(r)
				}
				return
			default:
				if e == nil {
					t.Fatal("unsafe review accepted")
				}
			}
			if real.Load() != 0 {
				t.Fatal("unexpected real write")
			}
		})
	}
}
func TestNodeReviewCanonicalLock(t *testing.T) {
	r := kube.ResourceRef{Kind: "Node", APIVersion: "v1", Name: "node", UID: "uid"}
	node := operationKey(OperationResult{Resource: r, Node: &NodeProgress{}})
	edit := operationKey(OperationResult{Resource: r, Edit: &kube.EditTarget{Resource: "nodes"}})
	apply := operationKey(OperationResult{Resource: r, Apply: &ApplyResultData{Target: kube.EditTarget{Resource: "nodes"}}})
	if node != edit || node != apply {
		t.Fatal("separate locks for same Node")
	}
}

func TestNodeReviewDrainCapacity(t *testing.T) {
	c := NewCoordinator(nil, nil)
	defer c.Close()
	for i, tc := range []struct {
		connection string
		busy       bool
	}{{"a", false}, {"a", true}, {"b", false}, {"c", true}} {
		r := kube.ResourceRef{Connection: model.ConnectionRef{ID: tc.connection}, Kind: "Node", APIVersion: "v1", Name: fmt.Sprintf("node-%d", i)}
		token, _, err := c.save(context.Background(), pendingReview{result: OperationResult{Resource: r, Action: "node", Node: &NodeProgress{Action: "drain"}}, payload: NodePreview{NodeRequest: NodeRequest{WaitPreset: 2}}})
		if err != nil {
			t.Fatal(err)
		}
		_, _, wrong := c.begin("edit", token, "")
		checkCode(t, wrong, "invalid_token")
		_, res, err := c.begin("node", token, "")
		if tc.busy {
			checkCode(t, err, "busy")
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		defer func() { c.results[res.OperationID].cancel(); c.finish(res); c.nodeWorkers.Done() }()
	}
}
