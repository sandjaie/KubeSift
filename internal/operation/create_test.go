/*
Purpose: Verify creation drafts and one server dry-run on the discovered collection.
Used by: Native operation regression tests.
Responsibilities: Cover exact parsing, scope, server projection and draft lifecycle.
Important: Synthetic HTTP requests never reach a real cluster.
*/
package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/session"
)

const createSecret = "apiVersion: v1\nkind: Secret\nmetadata:\n  name: demo\nstringData:\n  key: synthetic\ndata:\n  key: b2xk\n"
const createMap = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\ndata:\n  key: synthetic\n"
const createDiscovery = `{"kind":"APIResourceList","groupVersion":"v1","resources":[{"name":"secrets","kind":"Secret","namespaced":true,"verbs":["create"]},{"name":"configmaps","kind":"ConfigMap","namespaced":true,"verbs":["create"]},{"name":"namespaces","kind":"Namespace","namespaced":false,"verbs":["create"]}]}`

func TestPrepareCreateDiscoveryAndServerPreview(t *testing.T) {
	var posts atomic.Int32
	base, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1":
			fmt.Fprint(w, createDiscovery)
		case "POST /api/v1/namespaces/default/secrets":
			posts.Add(1)
			if q := r.URL.Query(); q.Get("dryRun") != "All" || q.Get("fieldValidation") != "Strict" || q.Get("fieldManager") != "kubesift-create" {
				t.Error("wrong creation query")
			}
			body, _ := io.ReadAll(r.Body)
			if text := string(body); strings.Contains(text, "stringData") || strings.Contains(text, "synthetic") || !strings.Contains(text, `"key":"c3ludGhldGlj"`) {
				t.Error("wrong Secret normalization")
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"kind":"Secret","apiVersion":"v1","metadata":{"name":"demo","namespace":"default","uid":"dry-id","resourceVersion":"0","creationTimestamp":"2026-11-16T00:00:00Z"},"type":"Opaque","data":{"key":"c3ludGhldGlj"}}`)
		default:
			t.Error("unexpected API request")
			w.WriteHeader(404)
		}
	})
	service := fixtureCreateService(t, base)
	ctx := context.Background()
	draft, err := service.OpenCreate(ctx, req.Resource.Connection, "default")
	if err != nil || posts.Load() != 0 {
		t.Fatal("open changed cluster", err)
	}
	p, err := service.PrepareCreate(ctx, CreateRequest{DraftID: draft.ID, YAML: createSecret, Namespace: "default"})
	if err != nil || p.Token == "" || posts.Load() != 1 || p.Target.Resource != "secrets" {
		t.Fatalf("bad preview: %v", err)
	}
	for _, text := range []string{p.ProposedYAML, p.ServerYAML} {
		obj, err := parseEditYAML(text)
		if err != nil {
			t.Fatal(err)
		}
		meta := obj["metadata"].(map[string]any)
		if meta["uid"] != nil || meta["resourceVersion"] != nil || meta["creationTimestamp"] != nil || meta["namespace"] != "default" {
			t.Fatal("unsafe preview projection")
		}
	}
	projected, _ := parseEditYAML(p.ServerYAML)
	proposed, _ := parseEditYAML(p.ProposedYAML)
	if projected["type"] != "Opaque" || proposed["type"] != nil {
		t.Fatal("preview did not show server default")
	}
	service.CloseCreate(draft.ID)
	if _, _, err := base.coordinator.begin("create", p.Token, ""); err == nil {
		t.Fatal("closed create review survived")
	}
}

func TestCreateDocumentValidation(t *testing.T) {
	for _, input := range []string{
		"[]", "kind: List\nitems: []", "apiVersion: v1\nkind: Secret\nmetadata: {generateName: a-}", createMap + "---\n{}",
		createMap + "status: {}", strings.Replace(createMap, "  name: demo", "  name: demo\n  uid: old", 1),
		createMap + "data: {}", "x: &a hello\ny: *a", "x: !unsafe hello", strings.Repeat("x", editLimit+1),
	} {
		if _, _, _, _, err := parseCreateDocument(input, "default"); err == nil {
			t.Fatalf("accepted invalid manifest of %d bytes", len(input))
		}
	}
	if _, _, _, _, err := parseCreateDocument(strings.Replace(createMap, "  name: demo", "  name: demo\n  namespace: other", 1), "default"); err == nil {
		t.Fatal("manifest namespace mismatch was accepted")
	}
	obj, _, _, _, err := parseCreateDocument(createMap+"unknown: 9007199254740993\n", "default")
	if err != nil || obj["unknown"] != json.Number("9007199254740993") {
		t.Fatal("lost unknown number", err)
	}
}

func fixtureCreateService(t *testing.T, base *ScaleService) *CreateService {
	t.Helper()
	sessions := session.NewManager(nil)
	t.Cleanup(sessions.Close)
	return NewCreateService(base.connections, kube.NewService(base.connections, sessions), base.coordinator)
}

func TestProjectRequestYAMLReviewNeverPosts(t *testing.T) {
	var posts atomic.Int32
	base, req := scaleServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			posts.Add(1)
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, `{"kind":"APIResourceList","groupVersion":"project.openshift.io/v1","resources":[{"name":"projectrequests","kind":"ProjectRequest","namespaced":false,"verbs":["create"]}]}`)
	})
	s := fixtureCreateService(t, base)
	d, e := s.OpenCreate(context.Background(), req.Resource.Connection, "")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.PrepareCreate(context.Background(), CreateRequest{DraftID: d.ID, YAML: "apiVersion: project.openshift.io/v1\nkind: ProjectRequest\nmetadata: {name: example}"})
	if e == nil || posts.Load() != 0 {
		t.Fatalf("review sent %d POSTs: %v", posts.Load(), e)
	}
}
