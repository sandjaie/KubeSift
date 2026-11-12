/*
Purpose: Protect truthful access evidence and safe failure categories.
Used by: Go tests.
Responsibilities: Exercise namespace/Pod permission combinations and typed transport failures.
Important: Fake Kubernetes clients never contact a cluster.
*/
package kube

import (
	"context"
	"crypto/x509"
	"errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	kt "k8s.io/client-go/testing"
	"testing"
)

func TestAccessEvidenceAndErrorCategories(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("private body"))
	for _, tc := range []struct {
		name     string
		ns, pods error
		evidence bool
		problem  string
	}{{"both allowed", nil, nil, true, ""}, {"namespace denied", forbidden, nil, true, ""}, {"both denied", forbidden, forbidden, false, "forbidden"}, {"Pods denied", nil, forbidden, true, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			client.PrependReactor("list", "namespaces", func(kt.Action) (bool, runtime.Object, error) { return tc.ns != nil, nil, tc.ns })
			client.PrependReactor("list", "pods", func(kt.Action) (bool, runtime.Object, error) { return tc.pods != nil, nil, tc.pods })
			r := checkClient(context.Background(), client, "default", "")
			if r.AuthorizedRead != tc.evidence {
				t.Fatalf("evidence: %+v", r)
			}
			if tc.problem != "" && (r.Problem == nil || r.Problem.Code != tc.problem) {
				t.Fatal(r.Problem)
			}
			if tc.pods != nil && r.PodProblem == nil {
				t.Fatal("Pod failure lost")
			}
		})
	}
	for _, tc := range []struct {
		err  error
		code string
	}{{apierrors.NewUnauthorized("private"), "auth_required"}, {forbidden, "forbidden"}, {x509.UnknownAuthorityError{}, "certificate"}, {context.DeadlineExceeded, "timeout"}, {context.Canceled, "cancelled"}, {errors.New("private error"), "failed"}} {
		if p := ClassifyError(context.Background(), tc.err); p.Code != tc.code {
			t.Fatalf("got %s want %s", p.Code, tc.code)
		}
	}
}
