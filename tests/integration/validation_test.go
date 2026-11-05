//go:build integration

/*
Purpose: Validate the Phase 1A adapters against only explicitly prepared local fixtures.
Used by: The opt-in integration command in the approved plan.
Responsibilities: Check distinct A/B routing, sessions, helper trust and reviewed marker writes with journal-safe recreation.
Important: Require the isolated kubeconfig and exact mapping; never fall back to ambient contexts or use provider credentials.
*/
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/sandjaie/KubeSift/internal/config"
	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/sandjaie/KubeSift/internal/operation"
	"github.com/sandjaie/KubeSift/internal/session"
	"github.com/sandjaie/KubeSift/internal/trust"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLocalValidation(t *testing.T) {
	path := os.Getenv("KUBESIFT_TEST_KUBECONFIG")
	if path == "" || !filepath.IsAbs(path) {
		t.Fatal("explicit absolute KUBESIFT_TEST_KUBECONFIG required")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	type identity struct {
		Context   string `json:"context"`
		Namespace string `json:"namespace"`
		Server    string `json:"server"`
	}
	var mapping struct {
		Version int      `json:"version"`
		RunID   string   `json:"runId"`
		A       identity `json:"a"`
		B       identity `json:"b"`
	}
	data, err := os.ReadFile(path + ".targets.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&mapping); err != nil {
		t.Fatal(err)
	}
	if mapping.Version != 1 || mapping.RunID == "" || mapping.A.Context != "kind-dev-local" || mapping.B.Context != "kind-kubesift-b" || mapping.A.Namespace != "payments" || mapping.B.Namespace != "payments" || mapping.A.Server == mapping.B.Server {
		t.Fatal("unrecognized or aliased fixture mapping")
	}
	for _, target := range []identity{mapping.A, mapping.B} {
		endpoint, e := url.Parse(target.Server)
		if e != nil {
			t.Fatal(e)
		}
		ip := net.ParseIP(endpoint.Hostname())
		if ip == nil || !ip.IsLoopback() || endpoint.Scheme != "https" {
			t.Fatal("only explicit TLS loopback fixture endpoints allowed")
		}
	}
	snapshot, err := config.Load([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"kind-dev-local": true, "kind-kubesift-b": true, "phase1a-denied": true, "phase1a-helper": true}
	for _, summary := range snapshot.Contexts() {
		if !allowed[summary.Name] {
			t.Fatal("unrecognized context in isolated fixture source")
		}
	}
	store, err := trust.OpenStore(filepath.Join(t.TempDir(), "trust.json"))
	if err != nil {
		t.Fatal(err)
	}
	guard := trust.NewGuard(store)
	connections := connection.NewManager(guard)
	events := make(chan model.Event, 64)
	sessions := session.NewManager(func(event model.Event) {
		select {
		case events <- event:
		default:
		}
	})
	sessions.SetValidate(func(target model.Target) error { return connections.Validate(target.Connection) })
	connections.SetCleanup(sessions.StopConnection)
	defer func() { connections.CloseAll(); sessions.Close() }()
	service := kube.NewService(connections, sessions)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	refs := map[string]model.ConnectionRef{}
	for _, target := range []identity{mapping.A, mapping.B} {
		resolved, e := snapshot.Resolve(target.Context)
		if e != nil {
			t.Fatal(e)
		}
		if resolved.RESTConfig().Host != target.Server {
			t.Fatal("configuration disagrees with explicit mapping")
		}
		ref, e := connections.Open(ctx, resolved)
		if e != nil {
			t.Fatal(e)
		}
		refs[target.Context] = ref
	}
	t.Run("distinct-markers-and-logs", func(t *testing.T) {
		for _, target := range []struct{ context, marker string }{{mapping.A.Context, "cluster-a"}, {mapping.B.Context, "cluster-b"}} {
			ref := refs[target.context]
			pods, e := service.Pods(ctx, ref, "payments")
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, pod := range pods {
				if pod.Name == "ks-validation" {
					found = true
					if pod.Annotations["kubesift.dev/marker"] != target.marker || pod.Labels["kubesift.dev/run"] != mapping.RunID {
						t.Fatal("wrong cluster marker or fixture ownership")
					}
				}
			}
			if !found {
				t.Fatal("owned Pod absent")
			}
			logs, e := service.Logs(ctx, model.Target{Connection: ref, Namespace: "payments", Name: "ks-validation", Container: "fixture"}, false, 2)
			if e != nil {
				t.Fatal(e)
			}
			if e = sessions.Activate(logs); e != nil {
				t.Fatal(e)
			}
			matched := false
			deadline := time.NewTimer(10 * time.Second)
			for !matched {
				select {
				case event := <-events:
					if event.Session == logs && event.Kind == "data" && bytes.Contains(event.Data, []byte(target.marker)) {
						matched = true
					}
				case <-deadline.C:
					t.Fatal("expected marker missing from logs")
				}
			}
			deadline.Stop()
			if e = sessions.Stop(logs); e != nil {
				t.Fatal(e)
			}
		}
	})
	t.Run("denied-identity", func(t *testing.T) {
		resolved, e := snapshot.Resolve("phase1a-denied")
		if e != nil {
			t.Fatal(e)
		}
		ref, e := connections.Open(ctx, resolved)
		if e != nil {
			t.Fatal(e)
		}
		_, e = service.Pods(ctx, ref, "payments")
		var safe *model.Error
		if !errors.As(e, &safe) || safe.Code != "forbidden" {
			t.Fatalf("expected forbidden, got %v", e)
		}
	})
	t.Run("loopback-forward", func(t *testing.T) {
		forward, e := service.Forward(ctx, model.Target{Connection: refs[mapping.A.Context], Namespace: "payments", Name: "ks-validation", Container: "fixture"}, 0, 8080)
		if e != nil {
			t.Fatal(e)
		}
		address := net.JoinHostPort("127.0.0.1", fmtPort(forward.LocalPort))
		client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
		response, e := client.Get("http://" + address + "/marker")
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(io.LimitReader(response.Body, 1024))
		response.Body.Close()
		if e != nil || strings.TrimSpace(string(body)) != "cluster-a" {
			t.Fatal("forward returned wrong marker")
		}
		if e = sessions.Stop(forward.Session); e != nil {
			t.Fatal(e)
		}
		listener, e := net.DialTimeout("tcp", address, time.Second)
		if e == nil {
			listener.Close()
			t.Fatal("forward listener survived stop")
		}
	})

	t.Run("malformed-terminal-input", func(t *testing.T) {
		if e := service.Input(model.SessionRef{ID: "unknown", Connection: refs[mapping.A.Context]}, nil); e == nil {
			t.Fatal("unknown terminal accepted an empty input request")
		}
	})
	t.Run("terminal-roundtrip", func(t *testing.T) {
		terminal, e := service.Exec(ctx, model.Target{Connection: refs[mapping.A.Context], Namespace: "payments", Name: "ks-validation", Container: "fixture"})
		if e != nil {
			t.Fatal(e)
		}
		defer sessions.Stop(terminal)
		if e = sessions.Activate(terminal); e != nil {
			t.Fatal(e)
		}
		if e = service.Resize(terminal, 80, 24); e != nil {
			t.Fatal(e)
		}
		if e = service.Input(terminal, []byte("printf '__KS_%s__\\n' 'ready'\n")); e != nil {
			t.Fatal(e)
		}
		output := []byte{}
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		for !bytes.Contains(output, []byte("__KS_ready__")) {
			select {
			case event := <-events:
				if event.Session == terminal && event.Kind == "data" {
					output = append(output, event.Data...)
					if len(output) > 16<<10 {
						t.Fatal("terminal roundtrip output exceeded validation bound")
					}
					if e = sessions.Acknowledge(terminal, event.Sequence); e != nil {
						t.Fatal(e)
					}
				}
				if event.Session == terminal && event.Kind == "error" {
					t.Fatalf("terminal failed: %s", event.Data)
				}
			case <-deadline.C:
				t.Fatal("terminal did not return the command marker")
			}
		}
	})
	t.Run("owned-helper", func(t *testing.T) {
		resolved, e := snapshot.Resolve("phase1a-helper")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = connections.Open(ctx, resolved); e == nil {
			t.Fatal("untrusted helper opened")
		}
		preview, e := guard.Review(resolved)
		if e != nil {
			t.Fatal(e)
		}
		if e = guard.ApprovePreview(preview.ID, resolved); e != nil {
			t.Fatal(e)
		}
		ref, e := connections.Open(ctx, resolved)
		if e != nil {
			t.Fatal(e)
		}
		pods, e := service.Pods(ctx, ref, "payments")
		if e != nil || len(pods) == 0 {
			t.Fatalf("owned helper authentication failed: %v", e)
		}
		if e = guard.Revoke(preview.Fingerprint); e != nil {
			t.Fatal(e)
		}
		if _, e = service.Pods(ctx, ref, "payments"); e == nil {
			t.Fatal("revoked helper executed")
		}
	})
	t.Run("prepared-mutation", func(t *testing.T) {
		target := model.Target{Connection: refs[mapping.A.Context], Namespace: "payments", Name: "ks-marker"}
		clientA, e := connections.Client(target.Connection)
		if e != nil {
			t.Fatal(e)
		}
		clientB, e := connections.Client(refs[mapping.B.Context])
		if e != nil {
			t.Fatal(e)
		}
		getA := func() *corev1.ConfigMap {
			cm, e := clientA.CoreV1().ConfigMaps("payments").Get(ctx, "ks-marker", metav1.GetOptions{})
			if e != nil {
				t.Fatal("cannot read A marker")
			}
			return cm
		}
		beforeA := getA()
		beforeB, e := clientB.CoreV1().ConfigMaps("payments").Get(ctx, "ks-marker", metav1.GetOptions{})
		if e != nil {
			t.Fatal("cannot read B marker")
		}
		ownerPath := filepath.Join(filepath.Dir(path), "ownership.json")
		verifyMarkerOwnership(t, ownerPath, mapping.RunID, mapping.A.Context, string(beforeA.UID))
		verifyMarkerOwnership(t, ownerPath, mapping.RunID, mapping.B.Context, string(beforeB.UID))
		if beforeA.Labels["kubesift.dev/run"] != mapping.RunID || beforeB.Labels["kubesift.dev/run"] != mapping.RunID {
			t.Fatal("marker ownership label mismatch")
		}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			client, e := connections.Client(refs[mapping.A.Context])
			if e != nil {
				t.Error("cannot restore fixture marker connection")
				return
			}
			cm, e := client.CoreV1().ConfigMaps("payments").Get(cleanupCtx, "ks-marker", metav1.GetOptions{})
			if e != nil {
				t.Error("cannot inspect fixture marker for restoration")
				return
			}
			verifyMarkerOwnership(t, ownerPath, mapping.RunID, mapping.A.Context, string(cm.UID))
			if cm.Labels["kubesift.dev/run"] != mapping.RunID {
				t.Error("restoration refused for unowned marker")
				return
			}
			if cm.Data["marker"] == beforeA.Data["marker"] {
				return
			}
			cm.Data["marker"] = beforeA.Data["marker"]
			if e = client.CoreV1().RESTClient().Put().Namespace("payments").Resource("configmaps").Name("ks-marker").Body(cm).MaxRetries(0).Do(cleanupCtx).Error(); e != nil {
				t.Error("fixture marker restoration failed; inspect before retrying")
			}
		})
		write := func(cm *corev1.ConfigMap) {
			if e := clientA.CoreV1().RESTClient().Put().Namespace("payments").Resource("configmaps").Name("ks-marker").Body(cm).MaxRetries(0).Do(ctx).Error(); e != nil {
				t.Fatal("fixture update failed; inspect marker before retrying")
			}
		}
		operations := operation.NewService(connections, time.Now)
		preview, e := operations.Prepare(ctx, target, "task6-reviewed")
		if e != nil {
			t.Fatal(e)
		}
		// Switch presentation to B before confirming the original A token.
		presented := refs[mapping.B.Context]
		if _, e = service.Pods(ctx, presented, "payments"); e != nil {
			t.Fatal(e)
		}
		if e = operations.Execute(ctx, preview.Token); e != nil {
			t.Fatal(e)
		}
		if getA().Data["marker"] != "task6-reviewed" {
			t.Fatal("A did not receive its reviewed value")
		}
		afterB, e := clientB.CoreV1().ConfigMaps("payments").Get(ctx, "ks-marker", metav1.GetOptions{})
		if e != nil || !reflect.DeepEqual(beforeB, afterB) {
			t.Fatal("B marker changed after confirming A")
		}
		requireOperationCode(t, operations.Execute(ctx, preview.Token), "invalid_token")
		preview, e = operations.Prepare(ctx, target, "must-not-write")
		if e != nil {
			t.Fatal(e)
		}
		edited := getA()
		edited.Data["marker"] = "task6-interference"
		write(edited)
		requireOperationCode(t, operations.Execute(ctx, preview.Token), "stale_target")
		current := getA()
		if current.Data["marker"] != "task6-interference" {
			t.Fatal("stale review changed A")
		}
		current.Data["marker"] = beforeA.Data["marker"]
		write(current)
		preview, e = operations.Prepare(ctx, target, "must-not-write")
		if e != nil {
			t.Fatal(e)
		}
		current = getA()
		verifyMarkerOwnership(t, ownerPath, mapping.RunID, mapping.A.Context, string(current.UID))
		options := metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &current.UID, ResourceVersion: &current.ResourceVersion}}
		if e = clientA.CoreV1().RESTClient().Delete().Namespace("payments").Resource("configmaps").Name("ks-marker").Body(&options).MaxRetries(0).Do(ctx).Error(); e != nil {
			t.Fatal("owned marker delete failed; inspect before retrying")
		}
		replacement := &corev1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: metav1.ObjectMeta{Name: "ks-marker", Namespace: "payments", Labels: beforeA.Labels, Annotations: beforeA.Annotations}, Data: beforeA.Data}
		var recreated corev1.ConfigMap
		if e = clientA.CoreV1().RESTClient().Post().Namespace("payments").Resource("configmaps").Body(replacement).MaxRetries(0).Do(ctx).Into(&recreated); e != nil {
			t.Fatal("marker recreation outcome needs inspection; ownership journal still records previous UID")
		}
		replaceMarkerOwnership(t, ownerPath, mapping.RunID, mapping.A.Context, string(current.UID), string(recreated.UID))
		if recreated.UID == current.UID {
			t.Fatal("recreation did not change UID")
		}
		requireOperationCode(t, operations.Execute(ctx, preview.Token), "stale_target")
		if getA().Data["marker"] != beforeA.Data["marker"] {
			t.Fatal("recreated marker changed by stale review")
		}
		preview, e = operations.Prepare(ctx, target, "must-not-write")
		if e != nil {
			t.Fatal(e)
		}
		resolved, e := snapshot.Resolve(mapping.A.Context)
		if e != nil {
			t.Fatal(e)
		}
		refs[mapping.A.Context], e = connections.Open(ctx, resolved)
		if e != nil {
			t.Fatal(e)
		}
		requireOperationCode(t, operations.Execute(ctx, preview.Token), "stale_target")
	})

	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("isolated kubeconfig mutated")
	}
}
func fmtPort(port uint16) string { return strconv.Itoa(int(port)) }

func requireOperationCode(t *testing.T, err error, code string) {
	t.Helper()
	var safe *model.Error
	if !errors.As(err, &safe) || safe.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

type journalResource struct {
	Context string `json:"context"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	UID     string `json:"uid"`
}

func markerOwnership(t *testing.T, path, run, contextName, uid string) ([]byte, map[string]json.RawMessage, []journalResource, int) {
	t.Helper()
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("cannot read ownership journal")
	}
	var record map[string]json.RawMessage
	if json.Unmarshal(data, &record) != nil {
		t.Fatal("invalid ownership journal")
	}
	var runID, phase string
	_ = json.Unmarshal(record["runId"], &runID)
	_ = json.Unmarshal(record["phase"], &phase)
	if runID != run || phase != "ready" {
		t.Fatal("ownership run is not ready")
	}
	var resources []journalResource
	if json.Unmarshal(record["resources"], &resources) != nil {
		t.Fatal("invalid ownership resources")
	}
	for i, r := range resources {
		if r.Context == contextName && r.Kind == "ConfigMap" && r.Name == "ks-marker" {
			if r.UID != uid || uid == "" {
				t.Fatal("marker UID is not owned by this run")
			}
			return data, record, resources, i
		}
	}
	t.Fatal("marker is absent from ownership journal")
	return nil, nil, nil, 0
}
func verifyMarkerOwnership(t *testing.T, path, run, contextName, uid string) {
	t.Helper()
	markerOwnership(t, path, run, contextName, uid)
}
func replaceMarkerOwnership(t *testing.T, path, run, contextName, previous, next string) {
	t.Helper()
	if next == "" || next == previous {
		t.Fatal("invalid replacement UID")
	}
	before, record, resources, index := markerOwnership(t, path, run, contextName, previous)
	resources[index].UID = next
	encoded, e := json.Marshal(resources)
	if e != nil {
		t.Fatal(e)
	}
	record["resources"] = encoded
	data, e := json.MarshalIndent(record, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	file, e := os.CreateTemp(filepath.Dir(path), "ownership-*.tmp")
	if e != nil {
		t.Fatal("cannot stage replacement ownership UID")
	}
	defer os.Remove(file.Name())
	if e = file.Chmod(0600); e == nil {
		_, e = file.Write(append(data, '\n'))
	}
	if e == nil {
		e = file.Sync()
	}
	closeErr := file.Close()
	if e != nil || closeErr != nil {
		t.Fatal("cannot persist replacement ownership UID")
	}
	current, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(current, before) {
		t.Fatal("ownership journal changed concurrently; inspect recreated marker")
	}
	if e = os.Rename(file.Name(), path); e != nil {
		t.Fatal("cannot install replacement ownership UID; inspect recreated marker")
	}
	directory, e := os.Open(filepath.Dir(path))
	if e != nil {
		t.Fatal(e)
	}
	defer directory.Close()
	if e = directory.Sync(); e != nil {
		t.Fatal(e)
	}
	t.Log("A ks-marker recreated; ownership journal atomically updated to its new UID")
}
