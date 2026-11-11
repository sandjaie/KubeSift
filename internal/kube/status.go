/*
Purpose: Establish resource-read evidence and classify connection failures safely.
Used by: The explorer coordinator and existing Kubernetes error adapter.
Responsibilities: Bound namespace/Pod reads and keep Pod permission failures separate from access evidence.
Important: No discovery endpoint or mutation proves access; raw server and helper text stays native.
*/
package kube

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/sandjaie/KubeSift/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"net"
	"sort"
)

type AccessResult struct {
	Namespaces         []string
	NamespacesComplete bool
	Namespace          string
	Pods               []PodSummary
	AuthorizedRead     bool
	PodProblem         *model.Problem
	Problem            *model.Problem
	Probe              string
}

func ClassifyError(ctx context.Context, err error) *model.Problem {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &model.Problem{Code: "timeout", Message: "Kubernetes request timed out"}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return &model.Problem{Code: "cancelled", Message: "Request cancelled"}
	}
	var safe *model.Error
	if errors.As(err, &safe) {
		return safe
	}
	switch {
	case apierrors.IsUnauthorized(err):
		return &model.Problem{Code: "auth_required", Message: "Authentication was rejected. Refresh credentials in your terminal, then Retry"}
	case apierrors.IsForbidden(err):
		return &model.Problem{Code: "forbidden", Message: "Access denied for this resource and namespace"}
	case apierrors.IsNotFound(err):
		return &model.Problem{Code: "invalid", Message: "Selected resource is unavailable"}
	case apierrors.IsConflict(err):
		return &model.Problem{Code: "stale_target", Message: "Resource changed; refresh the target"}
	}
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	var verify *tls.CertificateVerificationError
	if errors.As(err, &unknown) || errors.As(err, &invalid) || errors.As(err, &host) || errors.As(err, &verify) {
		return &model.Problem{Code: "certificate", Message: "Certificate verification failed; check the configured CA and endpoint"}
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return &model.Problem{Code: "timeout", Message: "Kubernetes request timed out"}
		}
		return &model.Problem{Code: "unreachable", Message: "Cannot reach the configured Kubernetes endpoint"}
	}
	return &model.Problem{Code: "failed", Message: "Kubernetes request failed"}
}
func (s *Service) CheckAccess(ctx context.Context, ref model.ConnectionRef, ns string) (AccessResult, error) {
	return s.ProbeAccess(ctx, ref, ns, "")
}

// ProbeAccess repeats a previously successful resource path during periodic checks.
func (s *Service) ProbeAccess(ctx context.Context, ref model.ConnectionRef, ns, probe string) (AccessResult, error) {
	if e := namespace(ns); e != nil {
		return AccessResult{}, e
	}
	client, e := s.connections.Client(ref)
	if e != nil {
		return AccessResult{}, e
	}
	r := checkClient(ctx, client, ns, probe)
	if r.Problem != nil {
		return r, r.Problem
	}
	return r, nil
}
func checkClient(ctx context.Context, client kubernetes.Interface, ns, probe string) AccessResult {
	r := AccessResult{Namespace: ns, Namespaces: []string{}, Pods: []PodSummary{}}
	var nsProblem *model.Problem
	if probe != "pods" {
		list, e := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 100})
		nsProblem = ClassifyError(ctx, e)
		if e == nil {
			r.AuthorizedRead = true
			r.Probe = "namespaces"
			r.NamespacesComplete = list.Continue == ""
			for _, n := range list.Items {
				r.Namespaces = append(r.Namespaces, n.Name)
			}
			sort.Strings(r.Namespaces)
		} else if nsProblem.Code != "forbidden" {
			r.Problem = nsProblem
			return r
		}
	}
	if probe == "namespaces" {
		return r
	}
	list, e := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{Limit: 100})
	r.PodProblem = ClassifyError(ctx, e)
	if e == nil {
		r.AuthorizedRead = true
		if r.Probe == "" {
			r.Probe = "pods"
		}
		if list.Continue != "" {
			r.PodProblem = &model.Problem{Code: "capacity", Message: "More than 100 Pods; pagination is not available yet. Choose a smaller namespace"}
		} else {
			for _, p := range list.Items {
				r.Pods = append(r.Pods, Summarize(p))
			}
		}
	}
	if r.PodProblem != nil && r.PodProblem.Code != "forbidden" && r.PodProblem.Code != "capacity" {
		r.AuthorizedRead = false
		r.Problem = r.PodProblem
	} else if !r.AuthorizedRead {
		r.Problem = r.PodProblem
		if r.Problem == nil {
			r.Problem = nsProblem
		}
	}
	return r
}
