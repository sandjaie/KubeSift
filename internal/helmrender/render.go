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
	"encoding/binary"
	"io"

	"github.com/sandjaie/KubeSift/internal/helmrender/wire"
	"github.com/sandjaie/KubeSift/internal/model"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"helm.sh/helm/v4/pkg/chart/common"
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
	request.Values = wire.Pack(request.Values).(common.Values)
	request.Release = wire.Pack(request.Release).(map[string]any)
	stdin := &bytes.Buffer{}
	if err := wire.Write(stdin, request); err != nil {
		return nil, model.Fail("capacity", "Chart rendering input exceeds its memory budget.")
	}

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
		if o.pending.Len() < 4 {
			break
		}
		frameSize := int(binary.BigEndian.Uint32(o.pending.Bytes()[:4]))
		if frameSize > wire.WireLimit {
			o.err = model.Fail("capacity", "Helm renderer message exceeds its limit.")
			return 0, o.err
		}
		if o.pending.Len() < 4+frameSize {
			break
		}
		var message wire.Message
		if err := wire.Read(bytes.NewReader(o.pending.Next(4+frameSize)), &message); err != nil || o.complete {
			o.err = model.Fail("invalid_response", "Invalid bounded-renderer response.")
			return 0, o.err
		}

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
			var response bytes.Buffer
			encodeErr := wire.Write(&response, wire.LookupResponse{Object: wire.Pack(object).(map[string]any), Failed: err != nil})
			if encodeErr != nil || response.Len() > wire.OutputLimit {
				o.err = model.Fail("capacity", "Helm lookup exceeds its rendering budget.")
				return 0, o.err
			}
			o.input.Write(response.Bytes())
			continue
		}
		o.complete = true
		if message.Error != "" {
			if o.err != nil {
				continue
			}
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
