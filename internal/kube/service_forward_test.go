/*
Purpose: Verify Service-to-Pod mappings and local forward cleanup.
Used by: Kubernetes interactive session tests.
Responsibilities: Exercise version, TCP target resolution, pagination and occupied ports.
Important: No remote traffic or real cluster is used.
*/
package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/portforward"
)

func TestServiceForward(t *testing.T) {
	for _, tc := range []struct {
		name     string
		target   intstr.IntOrString
		protocol core.Protocol
		conflict bool
		want     uint16
	}{
		{"default", intstr.IntOrString{}, core.ProtocolTCP, false, 80},
		{"numeric", intstr.FromInt32(8081), core.ProtocolTCP, false, 8081},
		{"named", intstr.FromString("web"), core.ProtocolTCP, false, 8080},
		{"conflicting names", intstr.FromString("web"), core.ProtocolTCP, true, 0},
		{"udp", intstr.FromString("web"), core.ProtocolUDP, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pod := &core.Pod{Spec: core.PodSpec{Containers: []core.Container{{Ports: []core.ContainerPort{{Name: "web", ContainerPort: 8080, Protocol: core.ProtocolTCP}}}}}}
			if tc.conflict {
				pod.Spec.Containers = append(pod.Spec.Containers, core.Container{Ports: []core.ContainerPort{{Name: "web", ContainerPort: 8081}}})
			}
			got, err := resolveForwardPort(core.ServicePort{Port: 80, Protocol: tc.protocol, TargetPort: tc.target}, pod)
			if got != tc.want || (err != nil) != (tc.want == 0) {
				t.Fatalf("resolution %d, %v", got, err)
			}
		})
	}
	const pod = `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"web-1","namespace":"demo","uid":"pod-uid","resourceVersion":"5","labels":{"app":"web"}},"status":{"phase":"Running"},"spec":{"containers":[{"name":"server","ports":[{"name":"web","containerPort":8080}]}]}}`
	var serviceMu sync.Mutex
	serviceType, selector := "ClusterIP", map[string]string{"app": "web"}
	s, ref := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/demo/services/web":
			serviceMu.Lock()
			defer serviceMu.Unlock()
			fmt.Fprintf(w, `{"kind":"Service","apiVersion":"v1","metadata":{"name":"web","namespace":"demo","uid":"svc-uid","resourceVersion":"10"},"spec":{"type":%q,"selector":`, serviceType)
			raw, _ := json.Marshal(selector)
			fmt.Fprintf(w, `%s,"ports":[{"name":"http","port":80,"targetPort":"web"}]}}`, raw)
		case "/api/v1/namespaces/demo/pods":
			if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("labelSelector") != "app=web" {
				t.Error("missing list bounds or selector")
			}
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"PodList","metadata":{"resourceVersion":"5","continue":"partial"},"items":[%s]}`, pod)
		case "/api/v1/namespaces/demo/pods/web-1":
			fmt.Fprint(w, pod)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	refService := ResourceRef{Connection: ref, Kind: "Service", APIVersion: "v1", Name: "web", Namespace: "demo", UID: "svc-uid"}
	req := ForwardCandidateRequest{Service: refService, ServiceVersion: "10", PortName: "http", Port: 80}
	ctx := context.Background()
	page, err := s.ReadForwardCandidates(ctx, req)
	if err != nil || page.Complete || page.Page.NextCursor == "" || len(page.Items) != 1 || page.Items[0].RemotePort != 8080 {
		t.Fatalf("partial candidate page: %+v %v", page, err)
	}
	start := ForwardRequest{Target: PodSessionTarget{Resource: page.Items[0].Pod}, RemotePort: 8080,
		Service: &ServiceForwardSelection{Resource: refService, ResourceVersion: "9", PortName: "http", Port: 80}}
	if _, err = s.OpenForward(ctx, start); err == nil {
		t.Fatal("stale Service accepted")
	}
	start.Service.ResourceVersion = "10"
	start.Target.Resource.UID = "old-pod"
	if _, err = s.OpenForward(ctx, start); err == nil {
		t.Fatal("replacement Pod accepted")
	}
	serviceMu.Lock()
	selector = nil
	serviceMu.Unlock()
	if _, err = s.ReadForwardCandidates(ctx, req); err == nil {
		t.Fatal("selectorless Service returned candidates")
	}
	serviceMu.Lock()
	serviceType = "ExternalName"
	serviceMu.Unlock()
	if _, err = s.ReadForwardCandidates(ctx, req); err == nil {
		t.Fatal("ExternalName returned candidates")
	}
}

type fakeForwardConnection struct{ httpstream.Connection }

func (fakeForwardConnection) Close() error           { return nil }
func (fakeForwardConnection) CloseChan() <-chan bool { return nil }

func TestForwardListener(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	local := uint16(listener.Addr().(*net.TCPAddr).Port)
	dial := forwardDialFunc(func(...string) (httpstream.Connection, string, error) {
		return fakeForwardConnection{}, portforward.PortForwardProtocolV1Name, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := func(uint16) error { t.Error("occupied port reported ready"); return nil }
	err = forwardListener(ctx, dial, local, 8080, ready)
	problem := ClassifyError(ctx, err)
	if problem == nil || problem.Code != "port_conflict" {
		t.Fatalf("occupied port error: %v", err)
	}
	cancel()
	err = forwardListener(ctx, dial, 0, 8080, func(uint16) error { t.Error("cancelled forward became ready"); return nil })
	if problem = ClassifyError(ctx, err); problem == nil || problem.Code != "timeout" && problem.Code != "cancelled" {
		t.Fatalf("cancelled forward: %v", err)
	}
}

func TestCancelInteractiveUpgrade(t *testing.T) {
	requested, release := make(chan struct{}), make(chan struct{})
	s, connection := workloadServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"work","namespace":"demo","uid":"present"},"status":{"phase":"Running"}}`)
			return
		}
		close(requested)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	resource := ResourceRef{Connection: connection, APIVersion: "v1", Kind: "Pod", Namespace: "demo", Name: "work", UID: "present"}
	entry, err := s.OpenForward(context.Background(), ForwardRequest{Target: PodSessionTarget{Resource: resource}, RemotePort: 8080})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.sessions.Activate(entry.Session); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requested:
	case <-time.After(2 * time.Second):
		t.Fatal("upgrade did not start")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- s.CloseInteractive(entry.Session) }()
	select {
	case err := <-stopped:
		if err != nil || s.sessions.Stats().Sessions != 0 {
			t.Fatalf("failed to release cancelled upgrade: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not unblock the stalled upgrade")
	}
}
