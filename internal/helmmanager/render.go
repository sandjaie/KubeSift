/*
Purpose: Route both Helm action renders through the isolated bounded SDK engine.
Used by: Install and upgrade preparation and execution.
Responsibilities: Preserve SDK-prepared values, dependencies, live lookup and original stored chart bytes.
Important: Only the generated adapter templates run in the host; original chart templates run in WebAssembly.
*/
package helmmanager

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/sandjaie/KubeSift/internal/helmrender"
	"github.com/sandjaie/KubeSift/internal/helmrender/wire"
	"github.com/sandjaie/KubeSift/internal/model"
	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/engine"
	"helm.sh/helm/v4/pkg/release"
	stored "helm.sh/helm/v4/pkg/release/v1"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
)

func boundedActionRender(ctx context.Context, cfg *action.Configuration, ch *chartv2.Chart) func() error {
	original := map[*chartv2.Chart][]*common.File{}
	markers := map[string][]*common.File{}
	var replace func(*chartv2.Chart)
	replace = func(current *chartv2.Chart) {
		original[current] = current.Templates
		marker := "templates/_kubesift_original_" + strconv.Itoa(len(original)) + ".tpl"
		markers[marker] = current.Templates
		current.Templates = []*common.File{{Name: marker}}
		for _, dependency := range current.Dependencies() {
			replace(dependency)
		}
	}
	replace(ch)
	var restoreClones func(*chartv2.Chart)
	restoreClones = func(current *chartv2.Chart) {
		if len(current.Templates) == 1 {
			if files, ok := markers[current.Templates[0].Name]; ok {
				current.Templates = files
			}
		}
		for _, dependency := range current.Dependencies() {
			restoreClones(dependency)
		}
	}
	restore := func() {
		for current, files := range original {
			current.Templates = files
		}
		// Helm may clone dependency charts while applying aliases and conditions.
		restoreClones(ch)
	}
	ch.Templates = []*common.File{
		{Name: "templates/kubesift-rendered.yaml", Data: []byte(`{{ kubesiftBoundedRender "resources" . }}`)},
		{Name: "templates/NOTES.txt", Data: []byte(`{{ kubesiftBoundedRender "notes" . }}`)},
	}
	var complete bool
	var resources, notes string
	var renderErr error
	cfg.CustomTemplateFuncs = template.FuncMap{"kubesiftBoundedRender": func(section string, data map[string]any) (string, error) {
		if !complete {
			complete = true
			// The SDK has now selected dependencies and coalesced values. Restore originals before storage.
			restore()
			request := wire.Request{Chart: wire.Capture(ch)}
			switch values := data["Values"].(type) {
			case common.Values:
				request.Values = values
			case map[string]any:
				request.Values = common.Values(values)
			}
			request.Release, _ = data["Release"].(map[string]any)
			request.Capabilities, _ = data["Capabilities"].(*common.Capabilities)
			lookup := helmrender.Lookup(func(string, string, string, string) (map[string]any, error) { return map[string]any{}, nil })
			if cfg.RESTClientGetter != nil {
				config, err := cfg.RESTClientGetter.ToRESTConfig()
				if err != nil {
					renderErr = err
					return "", err
				}
				lookup = engine.NewLookupFunction(config)
			}
			files, err := helmrender.Render(ctx, request, lookup)
			renderErr = err
			if err == nil {
				paths := make([]string, 0, len(files))
				for path := range files {
					paths = append(paths, path)
				}
				sort.Strings(paths)
				var output strings.Builder
				for _, path := range paths {
					if strings.HasSuffix(path, "/NOTES.txt") {
						if path == ch.Name()+"/templates/NOTES.txt" {
							notes = files[path]
						}
						continue
					}
					documents := releaseutil.SplitManifests(files[path])
					keys := make([]string, 0, len(documents))
					for key := range documents {
						keys = append(keys, key)
					}
					sort.Sort(releaseutil.BySplitManifestsOrder(keys))
					for _, key := range keys {
						output.WriteString("\n---\n# Source: " + path + "\n" + documents[key])
					}
				}
				resources = output.String()
				if len(resources)+len(notes) > renderedLimit {
					renderErr = model.Fail("capacity", "Rendered Helm content exceeds 8 MiB.")
				}
			}
		}
		if renderErr != nil {
			return "", renderErr
		}
		if section == "notes" {
			return notes, nil
		}
		return resources, nil
	}}
	return func() error {
		restore()
		return renderErr
	}
}

// Restore the source marker inserted per document by the bounded adapter before review or storage.
func restoreHookPaths(value release.Releaser) {
	rel, ok := value.(*stored.Release)
	if !ok || rel == nil {
		return
	}
	for _, hook := range rel.Hooks {
		if hook == nil || !strings.HasSuffix(hook.Path, "/templates/kubesift-rendered.yaml") {
			continue
		}
		line, _, _ := strings.Cut(hook.Manifest, "\n")
		if path, ok := strings.CutPrefix(line, "# Source: "); ok {
			hook.Path = path
		}
	}
}
