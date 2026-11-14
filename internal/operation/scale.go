/*
Purpose: Prepare an immutable, reviewed Deployment or StatefulSet scale.
Used by: The scaling bridge and API server integration tests.
Responsibilities: Validate the target, enforce bounded reads and require a matching server dry-run.
Important: A permission hint is advisory and only the original live version is used for writing.
*/
package operation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/sandjaie/KubeSift/internal/connection"
	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	auth "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/validation"
)

const reviewTTL = 5 * time.Minute
const scaleDeadline = 30 * time.Second

type ScaleService struct {
	mu          sync.Mutex
	connections *connection.Manager
	now         func() time.Time
	ctx         context.Context
	stop        context.CancelFunc
	prepares    chan struct{}
	pending     map[string]ScalePreview
	results     map[string]*scaleRecord
	byToken     map[string]string
	active      map[kube.ResourceRef]bool
	order       []string
	expiry      *time.Timer
}

func NewScaleService(connections *connection.Manager, now func() time.Time) *ScaleService {
	if now == nil {
		now = time.Now
	}
	ctx, stop := context.WithCancel(context.Background())
	s := &ScaleService{connections: connections, now: now, ctx: ctx, stop: stop, prepares: make(chan struct{}, 16),
		pending: map[string]ScalePreview{}, results: map[string]*scaleRecord{}, byToken: map[string]string{}, active: map[kube.ResourceRef]bool{}}
	connections.AddCleanup(s.CloseConnection)
	return s
}

func scaleTarget(ref kube.ResourceRef) (string, string) {
	plural := ""
	switch ref.Kind {
	case "Deployment":
		plural = "deployments"
	case "StatefulSet":
		plural = "statefulsets"
	}
	if plural == "" || ref.APIVersion != "apps/v1" || len(validation.IsDNS1123Label(ref.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(ref.Name)) != 0 || ref.UID == "" || len(ref.UID) > 128 {
		return "", ""
	}
	return "/apis/apps/v1/namespaces/" + ref.Namespace + "/" + plural + "/" + ref.Name, plural
}
func parseReplicas(input string) (int32, error) {
	invalid := model.Fail("invalid", "Enter a whole replica count from 0 through 2147483647")
	if len(input) == 0 || len(input) > 128 {
		return 0, invalid
	}
	for _, c := range input {
		if c < '0' || c > '9' {
			return 0, invalid
		}
	}
	n, err := strconv.ParseInt(input, 10, 32)
	if err != nil {
		return 0, invalid
	}
	return int32(n), nil
}

type scalePayload struct {
	metav1.TypeMeta
	Metadata struct {
		Name      string       `json:"name"`
		Namespace string       `json:"namespace"`
		UID       string       `json:"uid"`
		Version   string       `json:"resourceVersion"`
		Deletion  *metav1.Time `json:"deletionTimestamp,omitempty"`
	} `json:"metadata"`
	Spec *scaleSpec `json:"spec"`
}

type scaleSpec struct {
	Replicas *int32 `json:"replicas"`
	Policy   *struct {
		WhenScaled  string `json:"whenScaled"`
		WhenDeleted string `json:"whenDeleted"`
	} `json:"persistentVolumeClaimRetentionPolicy,omitempty"`
}

func (s *scaleSpec) desired() int32 {
	// autoscaling/v1 Scale omits its zero count in JSON.
	if s.Replicas == nil {
		return 0
	}
	return *s.Replicas
}

func (p scalePayload) matches(ref kube.ResourceRef, version string) bool {
	return p.Metadata.UID == ref.UID && p.Metadata.Name == ref.Name && p.Metadata.Namespace == ref.Namespace &&
		p.Metadata.Version != "" && len(p.Metadata.Version) <= 128 && (version == "" || p.Metadata.Version == version)
}

type scaleClient struct {
	client *http.Client
	base   *url.URL
}

func (s *ScaleService) client(ref model.ConnectionRef) (scaleClient, error) {
	cfg, err := s.connections.RESTConfig(ref)
	if err != nil {
		return scaleClient{}, err
	}
	base, err := url.Parse(cfg.Host)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return scaleClient{}, model.Fail("invalid", "Invalid Kubernetes API endpoint")
	}
	client, err := kube.BoundedHTTPClient(cfg)
	return scaleClient{client, base}, err
}

type scaleHTTPStatus int

func (e scaleHTTPStatus) Error() string { return http.StatusText(int(e)) }
func (c scaleClient) call(ctx context.Context, method, path string, dry bool, input, out any) ([]string, error) {
	addr := *c.base
	addr.Path = strings.TrimRight(addr.Path, "/") + path
	addr.RawPath, addr.RawQuery, addr.Fragment = "", "", ""
	if dry {
		addr.RawQuery = "dryRun=All"
	}
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, model.Fail("invalid", "Cannot encode scaling request")
		}
		// Keep the body non-replayable even if the underlying HTTP transport tries a retry.
		body = io.NopCloser(bytes.NewReader(raw))
	}
	req, err := http.NewRequestWithContext(ctx, method, addr.String(), body)
	if err != nil {
		return nil, model.Fail("invalid", "Cannot build scaling request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "KubeSift")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, scaleHTTPStatus(resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, out) != nil {
		return nil, model.Fail("invalid_response", "Kubernetes returned invalid scaling data")
	}
	if !dry {
		return nil, nil
	}
	return scaleWarnings(resp.Header.Values("Warning")), nil
}

func scaleWarnings(headers []string) []string {
	warnings := []string{}
	budget := 8192
	for _, header := range headers {
		if len(header) > budget {
			break
		}
		budget -= len(header)
		values, _ := utilnet.ParseWarningHeaders([]string{header})
		for _, value := range values {
			text := strings.TrimSpace(strings.Map(func(r rune) rune {
				if !unicode.IsPrint(r) {
					return ' '
				}
				return r
			}, value.Text))
			if text != "" && len(warnings) < 32 {
				warnings = append(warnings, text)
			}
		}
	}
	return warnings
}

func (c scaleClient) inspect(ctx context.Context, path string, ref kube.ResourceRef, version string) (int32, ScaleImpact, error) {
	var parent, scale scalePayload
	stale := model.Fail("stale_target", "Workload changed or is terminating. Refresh and review again.")
	if _, err := c.call(ctx, http.MethodGet, path, false, nil, &parent); err != nil {
		return 0, ScaleImpact{}, err
	}
	if parent.Kind != ref.Kind || parent.APIVersion != ref.APIVersion || !parent.matches(ref, version) || parent.Metadata.Deletion != nil {
		return 0, ScaleImpact{}, stale
	}
	if _, err := c.call(ctx, http.MethodGet, path+"/scale", false, nil, &scale); err != nil {
		return 0, ScaleImpact{}, err
	}
	if scale.Kind != "Scale" || scale.APIVersion != "autoscaling/v1" || !scale.matches(ref, version) || scale.Spec == nil ||
		parent.Spec == nil || parent.Spec.Replicas == nil || scale.Spec.desired() < 0 || scale.Spec.desired() != *parent.Spec.Replicas {
		return 0, ScaleImpact{}, stale
	}
	impact := ScaleImpact{}
	if ref.Kind == "StatefulSet" {
		impact.WhenScaled, impact.WhenDeleted = "Retain", "Retain"
		policy := parent.Spec.Policy
		impact.PolicyDefault = policy == nil || policy.WhenScaled == "" || policy.WhenDeleted == ""
		if policy != nil {
			if policy.WhenScaled != "" {
				impact.WhenScaled = policy.WhenScaled
			}
			if policy.WhenDeleted != "" {
				impact.WhenDeleted = policy.WhenDeleted
			}
		}
		if (impact.WhenScaled != "Retain" && impact.WhenScaled != "Delete") || (impact.WhenDeleted != "Retain" && impact.WhenDeleted != "Delete") {
			return 0, ScaleImpact{}, model.Fail("invalid_response", "StatefulSet returned an unrecognized claim retention policy")
		}
	}
	return scale.Spec.desired(), impact, nil
}
func permissionOf(ctx context.Context, c scaleClient, ref kube.ResourceRef, plural string) string {
	attrs := &auth.ResourceAttributes{Namespace: ref.Namespace, Name: ref.Name, Group: "apps", Version: "v1", Resource: plural, Subresource: "scale", Verb: "update"}
	input := auth.SelfSubjectAccessReview{TypeMeta: metav1.TypeMeta{Kind: "SelfSubjectAccessReview", APIVersion: "authorization.k8s.io/v1"}, Spec: auth.SelfSubjectAccessReviewSpec{ResourceAttributes: attrs}}
	var review auth.SelfSubjectAccessReview
	_, err := c.call(ctx, http.MethodPost, "/apis/authorization.k8s.io/v1/selfsubjectaccessreviews", false, input, &review)
	status := review.Status
	if err != nil || review.Kind != input.Kind || review.APIVersion != input.APIVersion || status.EvaluationError != "" || status.Allowed == status.Denied {
		return "unknown"
	}
	if status.Allowed {
		return "allowed"
	}
	return "denied"
}

func scaleInput(p ScalePreview) scalePayload {
	var payload scalePayload
	payload.Kind, payload.APIVersion = "Scale", "autoscaling/v1"
	payload.Metadata.Name, payload.Metadata.Namespace = p.Resource.Name, p.Resource.Namespace
	payload.Metadata.UID, payload.Metadata.Version = p.Resource.UID, p.SourceVersion
	payload.Spec = &scaleSpec{Replicas: &p.After}
	return payload
}
func matchingScale(result scalePayload, ref kube.ResourceRef) bool {
	return result.Kind == "Scale" && result.APIVersion == "autoscaling/v1" && result.matches(ref, "") && result.Spec != nil && result.Spec.desired() >= 0
}

func (s *ScaleService) PrepareScale(ctx context.Context, req ScaleRequest) (ScalePreview, error) {
	path, plural := scaleTarget(req.Resource)
	if path == "" || req.SourceVersion == "" || len(req.SourceVersion) > 128 {
		return ScalePreview{}, model.Fail("invalid", "Select a captured apps/v1 Deployment or StatefulSet with a UID and version")
	}
	after, err := parseReplicas(req.ReplicasText)
	if err != nil {
		return ScalePreview{}, err
	}
	select {
	case s.prepares <- struct{}{}:
		defer func() { <-s.prepares }()
	default:
		return ScalePreview{}, model.Fail("busy", "Too many scale preparations")
	}
	ctx, cancel := context.WithTimeout(ctx, scaleDeadline)
	stop := context.AfterFunc(s.ctx, cancel)
	defer func() { stop(); cancel() }()
	if s.ctx.Err() != nil || ctx.Err() != nil {
		return ScalePreview{}, model.Fail("cancelled", "Scale review cancelled")
	}
	client, err := s.client(req.Resource.Connection)
	if err != nil {
		return ScalePreview{}, scaleReadProblem(err)
	}
	before, impact, err := client.inspect(ctx, path, req.Resource, req.SourceVersion)
	if err != nil {
		return ScalePreview{}, scaleReadProblem(err)
	}
	cluster, err := s.connections.ContextName(req.Resource.Connection)
	if err != nil {
		return ScalePreview{}, err
	}
	if len(cluster) > 1024 {
		return ScalePreview{}, model.Fail("invalid", "Cluster context name exceeds the display limit")
	}
	p := ScalePreview{Cluster: cluster, Resource: req.Resource, SourceVersion: req.SourceVersion, Before: before, After: after, Impact: impact, Warnings: []string{}}
	if before == after {
		p.NoChange = true
		return p, nil
	}
	p.Impact.Direction = scaleDirection(before, after)
	p.Permission = permissionOf(ctx, client, req.Resource, plural)
	if p.Permission == "denied" {
		return ScalePreview{}, model.Fail("forbidden", "Kubernetes reports that scaling this workload is denied")
	}
	var dry scalePayload
	p.Warnings, err = client.call(ctx, http.MethodPut, path+"/scale", true, scaleInput(p), &dry)
	if err != nil {
		return ScalePreview{}, scaleDryProblem(err)
	}
	if !matchingScale(dry, req.Resource) || dry.Spec.desired() != after {
		return ScalePreview{}, model.Fail("invalid_response", "Dry-run did not preserve the selected identity and replica count")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.ctx.Err() != nil {
		return ScalePreview{}, model.Fail("cancelled", "Scale review cancelled")
	}
	if err := s.connections.Validate(req.Resource.Connection); err != nil {
		return ScalePreview{}, err
	}
	now := s.now()
	s.prune(now)
	if len(s.pending) >= 128 {
		return ScalePreview{}, model.Fail("busy", "Too many pending scale reviews")
	}
	p.Token, p.ExpiresAt = model.ID(), now.Add(reviewTTL)
	saved := p
	saved.Warnings = append([]string{}, p.Warnings...)
	s.pending[p.Token] = saved
	s.scheduleExpiry(now)
	return p, nil
}

func scaleDirection(before, after int32) string {
	if after == 0 {
		return "zero"
	}
	if after < before {
		return "down"
	}
	return "up"
}
func scaleReadProblem(err error) *model.Problem {
	var problem *model.Problem
	if errors.As(err, &problem) {
		return problem
	}
	var code scaleHTTPStatus
	if errors.As(err, &code) {
		switch code {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied access to the selected workload or its Scale"}
		case 404, 409:
			return &model.Problem{Code: "stale_target", Message: "Workload changed or disappeared. Refresh and review again."}
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &model.Problem{Code: "cancelled", Message: "Scaling inspection was interrupted"}
	}
	return &model.Problem{Code: "unavailable", Message: "Cannot read scaling data from Kubernetes"}
}
func scaleDryProblem(err error) *model.Problem {
	var code scaleHTTPStatus
	if errors.As(err, &code) {
		switch code {
		case 401, 403:
			return &model.Problem{Code: "forbidden", Message: "Kubernetes denied the mandatory scale dry-run"}
		case 404, 409:
			return &model.Problem{Code: "stale_target", Message: "Workload changed or disappeared during dry-run. Refresh and review again."}
		}
	}
	return &model.Problem{Code: "dry_run_failed", Message: "Mandatory scale dry-run failed. No review was issued."}
}
