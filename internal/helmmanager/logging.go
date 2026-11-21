/*
Purpose: Prevent Helm's package-global diagnostics from exposing chart values.
Used by: Native Helm manager initialization.
Responsibilities: Drop Helm-origin slog records while retaining application diagnostics.
Important: Action-local discard loggers do not cover the SDK's schema and dependency helpers.
*/
package helmmanager

import (
	"context"
	"io"
	"log"
	"log/slog"
	"runtime"
	"strings"
	"sync"
)

var installLogFilter sync.Once

type helmLogFilter struct{ next slog.Handler }

func (h helmLogFilter) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}
func (h helmLogFilter) Handle(ctx context.Context, record slog.Record) error {
	if caller := runtime.FuncForPC(record.PC); caller != nil && strings.HasPrefix(caller.Name(), "helm.sh/helm/v4/") {
		return nil
	}
	return h.next.Handle(ctx, record)
}
func (h helmLogFilter) WithAttrs(attrs []slog.Attr) slog.Handler {
	return helmLogFilter{h.next.WithAttrs(attrs)}
}
func (h helmLogFilter) WithGroup(name string) slog.Handler {
	return helmLogFilter{h.next.WithGroup(name)}
}

func suppressHelmLogs() {
	installLogFilter.Do(func() {
		output, flags, prefix := log.Writer(), log.Flags(), log.Prefix()
		slog.SetDefault(slog.New(helmLogFilter{slog.Default().Handler()}))
		// Preserve the old sink: the default slog handler itself writes through log.
		log.SetOutput(helmLogWriter{output})
		log.SetFlags(flags)
		log.SetPrefix(prefix)
	})
}

type helmLogWriter struct{ next io.Writer }

func (w helmLogWriter) Write(data []byte) (int, error) {
	var pcs [32]uintptr
	count := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:count])
	for {
		frame, more := frames.Next()
		if strings.HasPrefix(frame.Function, "helm.sh/helm/v4/") {
			return len(data), nil
		}
		if !more {
			break
		}
	}
	return w.next.Write(data)
}
