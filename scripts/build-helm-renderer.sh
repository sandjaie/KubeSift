#!/bin/sh
# Purpose: Build the bundled Helm renderer from the same pinned SDK as the application.
# Used by: Repository checks and desktop build/binding tasks.
# Responsibilities: Produce the ignored WebAssembly asset with the Go build cache.
# Important: The asset contains code only; charts and credentials are never build inputs.
set -eu
cd "$(dirname "$0")/.."
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o internal/helmrender/worker.wasm ./cmd/helm-render-worker
