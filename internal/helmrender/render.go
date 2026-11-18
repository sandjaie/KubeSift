/*
Purpose: Contain Helm template allocation and execution inside a bounded native WebAssembly runtime.
Used by: Helm preparation and execution through the action template adapter.
Responsibilities: Limit memory, cancel CPU-bound templates and mediate live lookup without ambient access.
Important: Only the trusted bundled worker runs; no chart code gets filesystem, network or process access.
*/
package helmrender

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"io"

	"github.com/sandjaie/KubeSift/internal/helmrender/wire"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed worker.wasm
var worker []byte

// Only trusted compiled code is shared; each invocation owns a separate linear memory.
var compileLock = make(chan struct{}, 1)
var renderer wazero.Runtime
var compiled wazero.CompiledModule

func compiledRenderer(ctx context.Context) (wazero.Runtime, wazero.CompiledModule, error) {
	select {
	case compileLock <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	defer func() { <-compileLock }()
	if renderer == nil {
		config := wazero.NewRuntimeConfig().WithMemoryLimitPages(4096).WithCloseOnContextDone(true)
		renderer = wazero.NewRuntimeWithConfig(context.Background(), config)
		if _, err := wasi_snapshot_preview1.Instantiate(context.Background(), renderer); err != nil {
			return nil, nil, err
		}
	}
	if compiled == nil {
		var err error
		compiled, err = renderer.CompileModule(ctx, worker)
		if err != nil {
			return nil, nil, err
		}
	}
	return renderer, compiled, nil
}

type Lookup func(apiVersion, kind, namespace, name string) (map[string]any, error)

func Render(ctx context.Context, request wire.Request, lookup Lookup) (map[string]string, error) {
	input, err := json.Marshal(request)
	if err != nil || len(input) > wire.WireLimit {
		return nil, model.Fail("capacity", "Chart rendering input exceeds its memory budget.")
	}
	stdin := bytes.NewBuffer(append(input, '\n'))
	output := &renderOutput{input: stdin, lookup: lookup}
	runtime, code, err := compiledRenderer(ctx)
	if err == nil {
		module := wazero.NewModuleConfig().WithName("").WithArgs("kubesift-renderer").WithStdin(stdin).WithStdout(output).WithStderr(io.Discard).WithSysWalltime().WithSysNanotime().WithRandSource(rand.Reader)
		instance, runErr := runtime.InstantiateModule(ctx, code, module)
		err = runErr
		if instance != nil {
			defer instance.Close(context.Background())
		}
	}
	if ctx.Err() != nil {
		return nil, model.Fail("cancelled", "Helm rendering was cancelled or timed out.")
	}
	if output.err != nil {
		return nil, output.err
	}
	if err != nil || !output.complete {
		return nil, model.Fail("capacity", "Helm rendering exceeded its isolated memory budget or could not finish safely.")
	}
	return output.files, nil
}

type renderOutput struct {
	input    *bytes.Buffer
	pending  bytes.Buffer
	lookup   Lookup
	files    map[string]string
	err      error
	complete bool
}

func (o *renderOutput) Write(data []byte) (int, error) {
	if o.pending.Len()+len(data) > wire.WireLimit {
		o.err = model.Fail("capacity", "Helm rendering output exceeds its transport budget.")
		return 0, o.err
	}
	o.pending.Write(data)
	for {
		line, _, found := bytes.Cut(o.pending.Bytes(), []byte{'\n'})
		if !found {
			break
		}
		var message wire.Message
		if err := json.Unmarshal(line, &message); err != nil || o.complete {
			o.err = model.Fail("invalid_response", "Invalid bounded-renderer response.")
			return 0, o.err
		}
		o.pending.Next(len(line) + 1)
		if len(message.Lookup) != 0 {
			if len(message.Lookup) != 4 || o.lookup == nil {
				o.err = model.Fail("invalid", "Invalid Helm lookup request.")
				return 0, o.err
			}
			for _, value := range message.Lookup {
				if len(value) > 1024 {
					o.err = model.Fail("capacity", "Helm lookup identity exceeds its size limit.")
					return 0, o.err
				}
			}
			object, err := o.lookup(message.Lookup[0], message.Lookup[1], message.Lookup[2], message.Lookup[3])
			if err != nil {
				o.err = err
			}
			response, encodeErr := json.Marshal(wire.LookupResponse{Object: object, Failed: err != nil})
			if encodeErr != nil || len(response) > wire.OutputLimit {
				o.err = model.Fail("capacity", "Helm lookup exceeds its rendering budget.")
				return 0, o.err
			}
			o.input.Write(response)
			o.input.WriteByte('\n')
			continue
		}
		o.complete = true
		if message.Error != "" {
			o.err = model.Fail("invalid", "Helm template rendering failed. Check the chart and values.")
			if message.Error == "capacity" {
				o.err = model.Fail("capacity", "Rendered Helm content exceeds 8 MiB.")
			}
			continue
		}
		size := 0
		for name, text := range message.Files {
			size += len(name) + len(text)
		}
		if size > wire.OutputLimit {
			o.err = model.Fail("capacity", "Rendered Helm content exceeds 8 MiB.")
		}
		o.files = message.Files
	}
	return len(data), nil
}
