/*
Purpose: Verify safe restart eligibility, patch construction and mandatory preview.
Used by: Native operation regression tests.
Responsibilities: Check exact template changes, bounded projections and fixture API contracts.
Important: Test objects and HTTP credentials are synthetic.
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
	"sync"
	"testing"
	"time"
)

const fixtureTime = "2026-11-15T00:00:00Z"

func restartFixture(kind, spec, status string) string {
	return fmt.Sprintf(`{"apiVersion":"apps/v1","kind":%q,"metadata":{"name":"web","namespace":"default","uid":"uid","resourceVersion":"7"},"spec":%s,"status":%s}`, kind, spec, status)
}

const templateFixture = `"template":{"metadata":{"annotations":{"sibling":"secret-fixture"},"labels":{"app":"web"}},"spec":{"containers":[{"name":"web","image":"fixture"}],"future":9007199254740993}}`

func TestRestartEligibility(t *testing.T) {
	for _, tc := range []struct {
		kind, extra string
		allowed     bool
	}{
		{"Deployment", "", true},
		{"Deployment", `"paused":true,`, false},
		{"Deployment", `"strategy":{"type":"Recreate"},`, true},
		{"Deployment", `"strategy":{"type":"Future"},`, false},
		{"StatefulSet", `"updateStrategy":{"type":"OnDelete"},`, false},
		{"StatefulSet", `"updateStrategy":{"rollingUpdate":{"partition":2}},`, false},
		{"DaemonSet", `"updateStrategy":{"type":"RollingUpdate"},`, true},
		{"DaemonSet", `"updateStrategy":{"type":"OnDelete"},`, false},
	} {
		t.Run(tc.kind+tc.extra, func(t *testing.T) {
			p, err := parseRestartObject(json.RawMessage(restartFixture(tc.kind, "{"+tc.extra+templateFixture+"}", "{}")))
			if err != nil {
				t.Fatal(err)
			}
			impact, err := restartImpact(tc.kind, p.spec, p.status)
			if (err == nil) != tc.allowed {
				t.Fatalf("impact=%+v error=%v", impact, err)
			}
			if tc.allowed && (impact.Ready != nil || (tc.kind == "DaemonSet" && impact.Desired != nil)) {
				t.Fatalf("invented observations: %+v", impact)
			}
		})
	}
}

func TestRestartPatchPreservesTemplate(t *testing.T) {
	for _, meta := range []string{`{}`, `{"annotations":null}`, `{"annotations":{"sibling":"literal"}}`} {
		original := `{"template":{"metadata":` + meta + `,"spec":{"future":9007199254740993}}}`
		p, err := parseRestartObject(json.RawMessage(restartFixture("Deployment", original, "{}")))
		if err != nil {
			t.Fatal(err)
		}
		_, exists := p.annotations["sibling"]
		patch, hash, err := p.restartPatch("uid", "7", fixtureTime)
		if err != nil {
			t.Fatal(err)
		}
		var ops []patchOperation
		if err = json.Unmarshal(patch, &ops); err != nil || len(ops) != 3 || ops[0].Path != "/metadata/uid" || ops[1].Path != "/metadata/resourceVersion" {
			t.Fatalf("bad patch %s %v", patch, err)
		}
		if strings.HasSuffix(ops[2].Path, "~1restartedAt") != exists {
			t.Fatalf("wrong annotation path: %s", patch)
		}
		comparable := strings.ReplaceAll(original, `"metadata":`+meta, `"metadata":{"annotations":{"kubectl.kubernetes.io/restartedAt":"`+fixtureTime+`"`+map[bool]string{true: `,"sibling":"literal"`}[exists]+`}}`)
		response, err := parseRestartObject(json.RawMessage(restartFixture("Deployment", comparable, "{}")))
		if err != nil {
			t.Fatal(err)
		}
		same, err := specHash(response.spec)
		if err != nil || same != hash {
			t.Fatal("sibling data or exact spec changed", err)
		}
		other, err := parseRestartObject(json.RawMessage(restartFixture("Deployment", strings.ReplaceAll(comparable, "9007199254740993", "9007199254740992"), "{}")))
		if err != nil {
			t.Fatal(err)
		}
		different, err := specHash(other.spec)
		if err != nil || different == hash {
			t.Fatal("distinct large numbers matched", err)
		}
	}
}

func TestRestartDryRunAndPermission(t *testing.T) {
	for _, caseName := range []string{"unknown", "denied", "other-spec", "wrong-marker"} {
		t.Run(caseName, func(t *testing.T) {
			var mu sync.Mutex
			var calls []string
			scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls = append(calls, r.Method)
				mu.Unlock()
				switch r.Method {
				case http.MethodGet:
					fmt.Fprint(w, restartFixture("Deployment", `{"replicas":0,`+templateFixture+`}`, "{}"))
				case http.MethodPost:
					body, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(body), `"verb":"patch"`) || strings.Contains(string(body), "subresource") {
						t.Errorf("bad access review %s", body)
					}
					if caseName == "denied" {
						fmt.Fprint(w, `{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectAccessReview","status":{"denied":true}}`)
						return
					}
					w.WriteHeader(403)
				case http.MethodPatch:
					if r.URL.Path != scalePath || r.URL.RawQuery != "dryRun=All" || r.Header.Get("Content-Type") != "application/json-patch+json" {
						t.Error("invalid dry run")
					}
					tmpl := strings.Replace(templateFixture, `"sibling":"secret-fixture"`, `"sibling":"secret-fixture","kubectl.kubernetes.io/restartedAt":"`+fixtureTime+`"`, 1)
					if caseName == "wrong-marker" {
						tmpl = strings.ReplaceAll(tmpl, fixtureTime, "previous")
					}
					if caseName == "other-spec" {
						tmpl = strings.ReplaceAll(tmpl, "9007199254740993", "9007199254740992")
					}
					fmt.Fprint(w, strings.Replace(restartFixture("Deployment", `{"replicas":0,`+tmpl+`}`, `{"readyReplicas":5}`), `"resourceVersion":"7"`, `"resourceVersion":"8"`, 1))
				}
			})
			restart := NewRestartService(scale.connections, scale.coordinator, func() time.Time { now, _ := time.Parse(time.RFC3339, fixtureTime); return now })
			p, err := restart.PrepareRestart(context.Background(), RestartRequest{Resource: req.Resource, SourceVersion: req.SourceVersion})
			if caseName == "unknown" {
				if err != nil || p.Permission != "unknown" || p.SourceVersion != "7" || !p.Impact.ZeroPods || p.Token == "" || p.RestartAt != fixtureTime {
					t.Fatalf("bad preview %+v %v", p, err)
				}
				raw, _ := json.Marshal(p)
				if strings.Contains(string(raw), "secret-fixture") {
					t.Fatal("raw annotation leaked")
				}
			} else if err == nil || p.Token != "" {
				t.Fatalf("unexpected review %+v %v", p, err)
			}
			expected := "GET,POST,PATCH"
			if caseName == "denied" {
				expected = "GET,POST"
			}
			mu.Lock()
			defer mu.Unlock()
			if strings.Join(calls, ",") != expected {
				t.Errorf("unexpected calls %v", calls)
			}
		})
	}
}

func TestRestartTargetAndMalformedEvidence(t *testing.T) {
	for kind, plural := range map[string]string{"Deployment": "deployments", "StatefulSet": "statefulsets", "DaemonSet": "daemonsets", "Pod": ""} {
		ref := kube.ResourceRef{Kind: kind, APIVersion: "apps/v1", Namespace: "default", Name: "web", UID: "uid"}
		route, got := workloadTarget(ref)
		if got != plural || (route != "") == (plural == "") {
			t.Fatalf("bad route for %s: %s", kind, route)
		}
	}
	for _, meta := range []string{`null`, `{"annotations":[]}`, `{"annotations":{"sibling":1}}`} {
		if _, err := parseRestartObject(json.RawMessage(restartFixture("Deployment", `{"template":{"metadata":`+meta+`,"spec":{}}}`, "{}"))); err == nil {
			t.Fatalf("accepted malformed metadata: %s", meta)
		}
	}
	shown, truncated := escapedPrevious("\n\x1b" + strings.Repeat("é", 1024))
	if !truncated || len(shown) > 1024 || !strings.HasPrefix(shown, `\n\x1b`) {
		t.Fatalf("unsafe prior annotation projection (%d bytes)", len(shown))
	}
	one, _ := canonicalDecimal("9007199254740993.00")
	other, _ := canonicalDecimal("900719925474099300e-2")
	if one != other || one != "9007199254740993" {
		t.Fatalf("incorrect exact number normalization: %s %s", one, other)
	}
	p, err := parseRestartObject(json.RawMessage(restartFixture("Deployment", "{"+templateFixture+"}", "{}")))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.restartPatch("uid", strings.Repeat("x", patchLimit), fixtureTime); err == nil {
		t.Fatal("oversized patch accepted")
	}
}

func TestRestartPreviewAdvancesIdenticalMarker(t *testing.T) {
	old := strings.Replace(templateFixture, `"sibling":"secret-fixture"`, `"sibling":"secret-fixture","kubectl.kubernetes.io/restartedAt":"`+fixtureTime+`"`, 1)
	clock, err := time.Parse(time.RFC3339, fixtureTime)
	if err != nil {
		t.Fatal(err)
	}
	proposed := clock.Add(time.Nanosecond).Format(time.RFC3339Nano)
	scale, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			fmt.Fprint(w, restartFixture("Deployment", "{"+old+"}", "{}"))
		case http.MethodPost:
			w.WriteHeader(403)
		case http.MethodPatch:
			fmt.Fprint(w, restartFixture("Deployment", "{"+strings.Replace(old, fixtureTime, proposed, 1)+"}", "{}"))
		}
	})
	service := NewRestartService(scale.connections, scale.coordinator, func() time.Time { return clock.In(time.FixedZone("Fixture timezone", 3600)) })
	p, err := service.PrepareRestart(context.Background(), RestartRequest{Resource: req.Resource, SourceVersion: req.SourceVersion})
	if err != nil || p.RestartAt != proposed || !p.PreviousSet || p.PreviousValue != fixtureTime {
		t.Fatalf("incorrect restart marker projection: %+v %v", p, err)
	}
}
