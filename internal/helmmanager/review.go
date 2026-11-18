/*
Purpose: Prepare immutable native Helm inputs and an honest bounded review.
Used by: The desktop Prepare, ReadReview and Discard endpoints.
Responsibilities: Capture current storage identity, render without writes and issue expiring tokens.
Important: A preview pins inputs, not admission results or later lookup/time-dependent output.
*/
package helmmanager

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sandjaie/KubeSift/internal/helmreader"
	appkube "github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"go.yaml.in/yaml/v3"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	stored "helm.sh/helm/v4/pkg/release/v1"
)

const renderedLimit = 8 << 20

const previewWarning = "Helm preview only: hooks, live lookup, time/random templates, admission and concurrent changes can make execution differ. Partial changes may remain after failure or cancellation; no automatic rollback or cleanup."

type reviewData struct {
	Review
	request  PrepareRequest
	raw      []byte
	source   string
	sections map[string]string
	offsets  map[string]map[int]bool
	cancel   context.CancelFunc
}

func reviewValues(text string) (map[string]any, error) {
	if len(text) > 1<<20 {
		return nil, model.Fail("capacity", "Override values exceed 1 MiB.")
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}, nil
	}
	decoder := yaml.NewDecoder(strings.NewReader(text))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, model.Fail("invalid", "Values must be one YAML mapping.")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, model.Fail("invalid", "Values must contain exactly one YAML document.")
	}
	if err := boundedYAML(&document, 0); err != nil {
		return nil, err
	}
	var values map[string]any
	if err := document.Decode(&values); err != nil {
		return nil, model.Fail("invalid", "Values contain duplicate keys or invalid mapping values.")
	}
	return values, nil
}

func boundedYAML(node *yaml.Node, depth int) error {
	if depth > 32 || node.Kind == yaml.AliasNode {
		return model.Fail("capacity", "YAML aliases and nesting beyond 32 levels are not supported.")
	}
	for _, child := range node.Content {
		if err := boundedYAML(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validatePrepare(req *PrepareRequest) error {
	switch req.Action {
	case "install", "upgrade":
		if req.ChartID == "" || req.RollbackRecord != nil || req.Options.KeepHistory {
			return model.Fail("invalid", "Install or upgrade requires a chart and replacement override values.")
		}
	case "rollback":
		if req.RollbackRecord == nil || req.ChartID != "" || req.ValuesYAML != "" || req.Options.KeepHistory {
			return model.Fail("invalid", "Rollback requires one retained revision and its stored values.")
		}
	case "uninstall":
		if req.RollbackRecord != nil || req.ChartID != "" || req.ValuesYAML != "" {
			return model.Fail("invalid", "Uninstall uses the current release without chart overrides.")
		}
	default:
		return model.Fail("invalid", "Choose install, upgrade, rollback or uninstall.")
	}
	if req.Action != "install" && req.Options.CreateNamespace {
		return model.Fail("invalid", "Create namespace is available only during install.")
	}
	if req.Options.TimeoutSeconds == 0 {
		req.Options.TimeoutSeconds = 300
	}
	if req.Options.TimeoutSeconds < 60 || req.Options.TimeoutSeconds > 1800 {
		return model.Fail("invalid", "Choose a timeout from 1 to 30 minutes.")
	}
	return nil
}

func (s *Service) Prepare(ctx context.Context, req PrepareRequest) (Review, error) {
	if req.RollbackRecord != nil {
		record := *req.RollbackRecord
		req.RollbackRecord = &record
	}
	if err := validatePrepare(&req); err != nil {
		return Review{}, err
	}
	values, err := reviewValues(req.ValuesYAML)
	if err != nil {
		return Review{}, err
	}
	ctx, done := s.owned(ctx, 60*time.Second)
	defer done()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r := &reviewData{Review: Review{Token: model.ID(), Target: req.Target, Action: req.Action, Options: req.Options, ExpiresAt: time.Now().Add(reviewTTL), Warnings: []string{previewWarning, "Readiness and Jobs are awaited. Force, ownership adoption and automatic recovery are disabled; history is not pruned.", "CRDs from crds/ are installed only during install and retained afterwards. Keep-policy and chart-retained hook resources may remain after uninstall.", "An omitted resource namespace uses the target namespace only for namespaced API kinds; cluster-scoped resources remain cluster-scoped."}}, request: req, cancel: cancel, sections: map[string]string{}, offsets: map[string]map[int]bool{}}
	s.mu.Lock()
	s.expireLocked()
	if s.closed {
		s.mu.Unlock()
		return Review{}, model.Fail("cancelled", "Helm manager is closed.")
	}
	if req.ChartID != "" {
		c := s.charts[req.ChartID]
		if c == nil || len(c.raw) == 0 {
			s.mu.Unlock()
			return Review{}, model.Fail("stale_target", "Chart expired. Resolve it again.")
		}
		r.raw, r.source = c.raw, c.source
		r.ChartName, r.ChartVersion, r.Digest = c.Name, c.Version, c.Digest
		delete(s.charts, req.ChartID)
	} else if len(s.charts)+len(s.reviews) >= 4 {
		s.mu.Unlock()
		return Review{}, model.Fail("capacity", "Four Helm drafts are already retained.")
	}
	s.reviews[r.Token] = r
	s.mu.Unlock()
	complete := false
	defer func() {
		if !complete {
			s.Discard(r.Token)
		}
	}()
	cfg, err := s.configuration(ctx, req.Target, true)
	if err != nil {
		return Review{}, err
	}
	current, previous, err := helmreader.CaptureHead(ctx, cfg.client, req.Target)
	if err != nil {
		return Review{}, err
	}
	if req.Action == "install" && current != nil {
		return Review{}, model.Fail("conflict", "A release with this name already exists, including retained history.")
	}
	if req.Action != "install" && (previous == nil || previous.Info == nil) {
		return Review{}, model.Fail("not_found", "The current release is unavailable.")
	}
	if previous != nil && previous.Info.Status.IsPending() {
		return Review{}, model.Fail("conflict", "This release has a pending Helm operation. Resolve it externally before continuing.")
	}
	if s.connections != nil {
		r.ContextName, err = s.connections.ContextName(req.Target.Connection)
		if err != nil {
			return Review{}, err
		}
	}
	r.Current = current
	if previous != nil {
		r.sections["currentManifest"] = previous.Manifest
		old, err := yaml.Marshal(previous.Config)
		if err != nil || len(old) > 1<<20 {
			return Review{}, model.Fail("capacity", "Stored override values exceed the review limit.")
		}
		r.sections["currentValues"] = string(old)
	}
	var proposed *stored.Release
	switch req.Action {
	case "install", "upgrade":
		ch, err := loadChart(ctx, r.raw)
		if err != nil {
			return Review{}, err
		}
		proposed, err = previewRelease(ctx, cfg.sdk, req, ch, values)
		if err != nil {
			return Review{}, safeHelmError(ctx, err)
		}
		v2, ok := ch.(*chartv2.Chart)
		if !ok {
			return Review{}, model.Fail("unsupported", "This chart format is not supported by Helm actions.")
		}
		for _, crd := range v2.CRDObjects() {
			r.sections["crds"] += "---\n" + string(crd.File.Data) + "\n"
			if len(r.sections["crds"]) > renderedLimit {
				return Review{}, model.Fail("capacity", "Chart CRDs exceed the review limit.")
			}
		}
	case "rollback":
		if req.RollbackRecord.Scope != req.Target || req.RollbackRecord.Revision == current.Revision {
			return Review{}, model.Fail("invalid", "Choose a different retained revision of this exact release.")
		}
		proposed, err = helmreader.ReadCaptured(ctx, cfg.client, *req.RollbackRecord)
		if err != nil {
			return Review{}, err
		}
		if proposed.Version >= previous.Version {
			return Review{}, model.Fail("invalid", "Choose an earlier retained revision.")
		}
		record := *req.RollbackRecord
		r.Rollback = &record
		rollback := action.NewRollback(cfg.sdk)
		rollback.Version, err = strconv.Atoi(record.Revision)
		if err != nil {
			return Review{}, model.Fail("invalid", "Revision is outside the SDK range.")
		}
		rollback.DryRunStrategy = action.DryRunServer
		rollback.DisableHooks = req.Options.DisableHooks
		if err := rollback.Run(req.Target.ReleaseName); err != nil {
			return Review{}, safeHelmError(ctx, err)
		}
	case "uninstall":
		uninstall := action.NewUninstall(cfg.sdk)
		uninstall.DryRun = true
		if _, err := uninstall.Run(req.Target.ReleaseName); err != nil {
			return Review{}, safeHelmError(ctx, err)
		}
		proposed = previous
	}
	if proposed == nil {
		return Review{}, model.Fail("failed", "Helm returned no preview.")
	}
	if req.Action != "uninstall" {
		r.sections["manifest"] = proposed.Manifest
	}
	if req.Action == "rollback" || req.Action == "uninstall" {
		if proposed.Chart != nil && proposed.Chart.Metadata != nil {
			r.ChartName, r.ChartVersion = proposed.Chart.Metadata.Name, proposed.Chart.Metadata.Version
		}
		if proposed.Chart != nil {
			for _, crd := range proposed.Chart.CRDObjects() {
				r.sections["crds"] += "---\n" + string(crd.File.Data) + "\n"
			}
		}
		storedValues, err := yaml.Marshal(proposed.Config)
		if err != nil || len(storedValues) > 1<<20 {
			return Review{}, model.Fail("capacity", "Stored override values exceed the review limit.")
		}
		r.sections["values"] = string(storedValues)
	} else {
		r.sections["values"] = req.ValuesYAML
	}
	for _, hook := range proposed.Hooks {
		r.sections["hooks"] += "---\n# Hook: " + hook.Name + "\n" + hook.Manifest + "\n"
		if len(r.sections["hooks"]) > renderedLimit {
			return Review{}, model.Fail("capacity", "Hooks exceed the review limit.")
		}
	}
	if err := reviewContent(r); err != nil {
		return Review{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reviews[r.Token] != r || ctx.Err() != nil {
		return Review{}, model.Fail("cancelled", "Review was cancelled or invalidated.")
	}
	if len(r.ContextName) > 1024 || len(r.ChartName) > 256 || len(r.ChartVersion) > 256 {
		return Review{}, model.Fail("capacity", "Release metadata exceeds the review display limit.")
	}
	r.cancel = nil
	complete = true
	return r.Review, nil
}

func previewRelease(ctx context.Context, cfg *action.Configuration, req PrepareRequest, ch chart.Charter, values map[string]any) (*stored.Release, error) {
	if req.Action == "install" {
		install := action.NewInstall(cfg)
		install.ReleaseName, install.Namespace = req.Target.ReleaseName, req.Target.Namespace
		install.DryRunStrategy = action.DryRunServer
		install.DisableHooks = req.Options.DisableHooks
		rel, err := install.RunWithContext(ctx, ch, values)
		v, _ := rel.(*stored.Release)
		return v, err
	}
	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = req.Target.Namespace
	upgrade.DryRunStrategy = action.DryRunServer
	upgrade.ResetValues = true
	upgrade.DisableHooks = req.Options.DisableHooks
	upgrade.MaxHistory = 0
	rel, err := upgrade.RunWithContext(ctx, req.Target.ReleaseName, ch, values)
	v, _ := rel.(*stored.Release)
	return v, err
}

func reviewContent(r *reviewData) error {
	if len(r.sections["manifest"])+len(r.sections["hooks"])+len(r.sections["crds"]) > renderedLimit {
		return model.Fail("capacity", "Rendered manifests, hooks and CRDs exceed 8 MiB.")
	}
	resources := []map[string]string{}
	for _, section := range []string{"manifest", "hooks", "crds"} {
		text := r.sections[section]
		if r.Action == "uninstall" && section == "manifest" {
			text = r.sections["currentManifest"]
		}
		decoder := yaml.NewDecoder(strings.NewReader(text))
		for {
			var document yaml.Node
			err := decoder.Decode(&document)
			if err == io.EOF {
				break
			}
			if err != nil {
				return model.Fail("invalid", "A rendered resource could not be reviewed.")
			}
			if err := boundedYAML(&document, 0); err != nil {
				return err
			}
			if err := collectIdentities(&document, section, &resources); err != nil {
				return err
			}
		}
	}
	identities, _ := yaml.Marshal(resources)
	r.sections["resources"] = string(identities)
	for name := range r.sections {
		r.Sections = append(r.Sections, name)
		r.offsets[name] = map[int]bool{0: true}
	}
	sort.Strings(r.Sections)
	return nil
}

func collectIdentities(node *yaml.Node, section string, resources *[]map[string]string) error {
	var object struct {
		APIVersion string      `yaml:"apiVersion"`
		Kind       string      `yaml:"kind"`
		Items      []yaml.Node `yaml:"items"`
		Metadata   struct {
			Name        string            `yaml:"name"`
			Namespace   string            `yaml:"namespace"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"metadata"`
	}
	if err := node.Decode(&object); err != nil {
		return model.Fail("invalid", "A rendered resource has invalid metadata.")
	}
	if object.Kind == "List" {
		for _, item := range object.Items {
			if err := collectIdentities(&item, section, resources); err != nil {
				return err
			}
		}
		return nil
	}
	if object.Kind == "" {
		return nil
	}
	*resources = append(*resources, map[string]string{"apiVersion": object.APIVersion, "kind": object.Kind, "name": object.Metadata.Name, "namespace": object.Metadata.Namespace, "section": section, "keep": object.Metadata.Annotations["helm.sh/resource-policy"]})
	if len(*resources) > 1000 {
		return model.Fail("capacity", "Review exceeds 1,000 resource identities.")
	}
	return nil
}

func (s *Service) ReadReview(ctx context.Context, token, section string, offset int) (ReviewSection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked()
	r := s.reviews[token]
	if r == nil || r.cancel != nil || ctx.Err() != nil || !r.offsets[section][offset] {
		return ReviewSection{}, model.Fail("stale_target", "Review or section cursor is unavailable. Prepare again.")
	}
	return sectionChunk(r.sections[section], offset, r.offsets[section])
}

func sectionChunk(text string, offset int, issued map[int]bool) (ReviewSection, error) {
	end := min(offset+(256<<10), len(text))
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	out := ReviewSection{Text: text[offset:end], Offset: offset, NextOffset: end, Done: end == len(text)}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 2<<20 {
		return ReviewSection{}, model.Fail("capacity", "Review chunk exceeds the bridge limit.")
	}
	issued[end] = true
	return out, nil
}

func safeHelmError(ctx context.Context, err error) error {
	var problem *model.Problem
	if errors.As(err, &problem) {
		return problem
	}
	classified := appkube.ClassifyError(ctx, err)
	if classified.Code == "forbidden" || classified.Code == "auth_required" || classified.Code == "unreachable" || classified.Code == "timeout" || ctx.Err() != nil {
		return classified
	}
	return model.Fail("failed", "Helm could not complete this step. Check chart schema, resource compatibility and permissions; inspect the release for partial effects.")
}
