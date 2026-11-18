/*
Purpose: Run the pinned Helm template engine inside a restricted WebAssembly instance.
Used by: The native Helm rendering host; never launched as a CLI.
Responsibilities: Render SDK-prepared values and request read-only lookup through the host.
Important: The host supplies hard linear-memory and execution-context limits; no filesystem or network is mounted.
*/
package main

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"text/template"

	"github.com/sandjaie/KubeSift/internal/helmrender/wire"
	"helm.sh/helm/v4/pkg/chart/common"
	"helm.sh/helm/v4/pkg/engine"
)

func main() {
	log.SetOutput(io.Discard)
	slog.SetDefault(slog.New(slog.DiscardHandler))
	input := os.Stdin
	output := os.Stdout
	var request wire.Request
	if wire.Read(input, &request) != nil {
		_ = wire.Write(output, wire.Message{Error: "invalid"})
		return
	}
	lookup := func(apiVersion, kind, namespace, name string) (map[string]any, error) {
		if err := wire.Write(output, wire.Message{Lookup: []string{apiVersion, kind, namespace, name}}); err != nil {
			return nil, err
		}
		var response wire.LookupResponse
		if err := wire.Read(input, &response); err != nil || response.Failed {
			return nil, errors.New("lookup failed")
		}
		return wire.Unpack(response.Object).(map[string]any), nil
	}
	request.Values = wire.Unpack(request.Values).(common.Values)
	request.Release = wire.Unpack(request.Release).(map[string]any)
	renderer := engine.Engine{CustomTemplateFuncs: template.FuncMap{"lookup": lookup}}
	values := common.Values{"Values": request.Values, "Release": request.Release, "Capabilities": request.Capabilities}
	files, err := renderer.RenderWithContext(context.Background(), request.Chart.Restore(), values)
	if err != nil {
		_ = wire.Write(output, wire.Message{Error: "invalid"})
		return
	}
	size := 0
	for name, text := range files {
		size += len(name) + len(text)
		if size > wire.OutputLimit {
			_ = wire.Write(output, wire.Message{Error: "capacity"})
			return
		}
	}
	_ = wire.Write(output, wire.Message{Files: files})
}
