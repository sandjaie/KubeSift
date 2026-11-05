#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
test -z "$(gofmt -l main.go internal)"
go test ./...
go vet ./...
npm test
npm run lint
npm run typecheck
