/*
Purpose: Check complete Helm inventory grouping.
Used by: Native Helm reader unit tests.
Responsibilities: Cover exact revision ordering and ambiguous latest records.
Important: Revisions are compared as exact decimal strings.
*/
package helmreader

import (
	"context"
	"encoding/json"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestReleaseInventory(t *testing.T) {
	scope := Scope{Namespace: "demo", Driver: "secret"}
	meta := func(revision string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: "sh.helm.release.v1.demo.v" + revision, Namespace: scope.Namespace,
			UID: "uid", ResourceVersion: "8", Labels: map[string]string{"owner": "helm", "name": "demo", "version": revision}}
	}
	state := &snapshot{scope: scope}
	state.collect([]metadataRecord{{Metadata: meta("9007199254740992")}, {Metadata: meta("9007199254740993")}})
	if len(state.latest) != 1 || state.latest[0].Record.Revision != "9007199254740993" {
		t.Fatalf("incorrect latest result: %+v", state.latest)
	}
	state.collect([]metadataRecord{{Metadata: meta("1")}, {Metadata: meta("2")}, {Metadata: meta("2")}})
	if len(state.latest) != 1 || state.latest[0].Problem == nil || state.latest[0].Record.Revision != "2" {
		t.Fatalf("duplicate newest record was not surfaced: %+v", state.latest)
	}
}

func TestInventoryDoesNotReturnPartialLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/demo/secrets" || r.URL.Query().Get("labelSelector") != "owner=helm,name=sample" ||
			r.URL.Query().Get("limit") != "100" || !strings.Contains(r.Header.Get("Accept"), "PartialObjectMetadataList") {
			t.Errorf("unexpected metadata request %s", r.URL)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("continue"))
		count := 100
		next := strconv.Itoa(page + 1)
		if page == 10 {
			count, next = 1, ""
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metadataList{Metadata: metav1.ListMeta{Continue: next}, Items: make([]metadataRecord, count)})
	}))
	defer server.Close()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := scan(context.Background(), client, Scope{Namespace: "demo", Driver: "secret", ReleaseName: "sample"})
	if err == nil || len(rows) != 0 || !strings.Contains(err.Error(), "exact release name") {
		t.Fatalf("an incomplete scan exposed %d rows: %v", len(rows), err)
	}
}
