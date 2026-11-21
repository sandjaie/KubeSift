/*
Purpose: Own request limits and metadata snapshots for Helm inspection.
Used by: The desktop explorer.
Responsibilities: Open clients with existing credentials and clear snapshots when a connection closes.
Important: Each request uses the selected generation and shares the app-wide read cap.
*/
package helmreader

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

const sectionLimit = 2 << 20
const pageSize = 100
const snapshotLimit = 1000

type scopeKey struct {
	connection        model.ConnectionRef
	namespace, driver string
}

type Service struct {
	reader      *kube.Service
	connections *connection.Manager
	slots       chan struct{}
	mu          sync.Mutex
	snapshots   map[scopeKey]*snapshot
}

func NewService(connections *connection.Manager, reader *kube.Service) *Service {
	s := &Service{connections: connections, reader: reader, slots: make(chan struct{}, 4), snapshots: make(map[scopeKey]*snapshot)}
	connections.AddCleanup(func(ref model.ConnectionRef) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for key := range s.snapshots {
			if key.connection == ref {
				delete(s.snapshots, key)
			}
		}
	})
	return s
}

func (s *Service) CloseScope(ref model.ConnectionRef, namespace, driver string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snapshots, scopeKey{ref, namespace, driver})
}

func validateScope(scope Scope) error {
	if len(validation.IsDNS1123Label(scope.Namespace)) != 0 || (scope.Driver != "secret" && scope.Driver != "configmap") ||
		(scope.ReleaseName != "" && !validReleaseName(scope.ReleaseName)) {
		return model.Fail("invalid", "Choose a namespace and storage driver, and enter a valid Helm release name.")
	}
	return nil
}

func validReleaseName(name string) bool {
	return len(name) <= 53 && len(validation.IsDNS1123Subdomain(name)) == 0
}

func (s *Service) start(ctx context.Context, scope Scope) (context.Context, kubernetes.Interface, func(), error) {
	if err := validateScope(scope); err != nil {
		return nil, nil, nil, err
	}
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, nil, kube.ClassifyError(ctx, ctx.Err())
	default:
		return nil, nil, nil, model.Fail("busy", "Four Helm reads are already active. Try again.")
	}
	owned, err := s.connections.Lifetime(scope.Connection)
	if err != nil {
		<-s.slots
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	stop := context.AfterFunc(owned, cancel)
	done := func() { stop(); cancel(); <-s.slots }
	cfg, err := s.connections.RESTConfig(scope.Connection)
	if err != nil {
		done()
		return nil, nil, nil, err
	}
	httpClient, err := kube.BoundedHTTPClient(cfg)
	if err != nil {
		done()
		return nil, nil, nil, err
	}
	client, err := kubernetes.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		done()
		return nil, nil, nil, model.Fail("invalid", "Cannot open the Helm storage client.")
	}
	return ctx, client, done, nil
}

func checkSize(value any) error {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > sectionLimit {
		return model.Fail("capacity", "The Helm response exceeds the 2 MiB display limit.")
	}
	return nil
}
