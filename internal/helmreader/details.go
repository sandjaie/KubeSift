/*
Purpose: Project one selected section from a captured Helm revision.
Used by: The Helm detail pane.
Responsibilities: Return stored evidence and compute compatible values without rendering charts.
Important: The reader does not persist section text or log chart values.
*/
package helmreader

import (
	"context"
	"time"

	"github.com/sandjaie/KubeSift/internal/kube"
	"github.com/sandjaie/KubeSift/internal/model"
	"go.yaml.in/yaml/v3"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	stored "helm.sh/helm/v4/pkg/release/v1"
)

func (s *Service) ReadSection(ctx context.Context, req DetailRequest) (ReleaseSection, error) {
	ctx, client, done, err := s.start(ctx, req.Record.Scope)
	if err != nil {
		return ReleaseSection{}, err
	}
	defer done()
	data, err := readRelease(ctx, client, req.Record, newBudget())
	if err != nil {
		return ReleaseSection{}, err
	}
	rel, ok := data.(*stored.Release)
	if !ok {
		return ReleaseSection{}, model.Fail("unsupported", "Unknown Helm release record format.")
	}
	out, err := projectSection(ctx, s.reader, req, rel)
	if err != nil {
		return ReleaseSection{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReleaseSection{}, kube.ClassifyError(ctx, err)
	}
	return out, nil
}

func projectSection(ctx context.Context, reader *kube.Service, req DetailRequest, rel *stored.Release) (ReleaseSection, error) {
	out := ReleaseSection{Record: req.Record, Section: req.Section, FetchedAt: time.Now().UTC()}
	var raw []byte
	var err error
	switch req.Section {
	case "Overview":
		value := ReleaseOverview{ReleaseSummary: summarize(req.Record, rel)}
		if rel.Info != nil {
			value.Description = rel.Info.Description
			value.FirstDeployed = optionalTime(rel.Info.FirstDeployed)
			value.DeletedAt = optionalTime(rel.Info.Deleted)
		}
		out.Overview = &value
	case "Values":
		values := rel.Config
		if req.ComputedValues {
			values, err = computedValues(rel)
			if err != nil {
				return ReleaseSection{}, err
			}
		}
		if !boundedValues(values, 0) {
			return ReleaseSection{}, model.Fail("capacity", "The stored values exceed the display depth limit.")
		}
		raw, err = yaml.Marshal(values)
	case "Manifest":
		out.Text = rel.Manifest
	case "Hooks":
		if len(rel.Hooks) > 0 {
			raw, err = yaml.Marshal(rel.Hooks)
		}
	case "Notes":
		if rel.Info != nil {
			out.Text = rel.Info.Notes
		}
	case "Resources":
		out.Resources, out.Truncated = manifestResources(ctx, reader, req.Record, rel.Manifest)
	default:
		return ReleaseSection{}, model.Fail("invalid", "Choose a supported Helm section.")
	}
	if err != nil {
		return ReleaseSection{}, model.Fail("invalid_response", "The recorded Helm section could not be displayed.")
	}
	if raw != nil {
		out.Text = string(raw)
	}
	if err := checkSize(out); err != nil {
		return ReleaseSection{}, err
	}
	return out, nil
}

func computedValues(rel *stored.Release) (map[string]any, error) {
	unavailable := model.Fail("unsupported", "Computed values are unavailable for this stored chart. Stored overrides are still available.")
	if rel.Chart == nil || rel.Chart.Metadata == nil || len(rel.Chart.Metadata.Dependencies) > 0 ||
		!boundedValues(rel.Config, 0) || !boundedValues(rel.Chart.Values, 0) ||
		!compatibleTables(rel.Chart.Values, rel.Config) {
		return nil, unavailable
	}
	// Helm 4.3.1 can log values on table conflicts. Supported stored charts
	// have no decoded dependency chart tree, and this check rules out those warnings.
	value, err := chartutil.CoalesceValues(rel.Chart, rel.Config)
	if err != nil {
		return nil, unavailable
	}
	return value, nil
}

func boundedValues(value any, depth int) bool {
	if depth > 64 {
		return false
	}
	switch value := value.(type) {
	case map[string]any:
		for _, v := range value {
			if !boundedValues(v, depth+1) {
				return false
			}
		}
	case []any:
		for _, v := range value {
			if !boundedValues(v, depth+1) {
				return false
			}
		}
	}
	return true
}

func compatibleTables(defaults, overrides map[string]any) bool {
	for key, before := range defaults {
		after, ok := overrides[key]
		if !ok || before == nil || after == nil {
			continue
		}
		base, baseTable := before.(map[string]any)
		target, targetTable := after.(map[string]any)
		if baseTable != targetTable || (baseTable && !compatibleTables(base, target)) {
			return false
		}
	}
	return true
}
