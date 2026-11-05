#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
python3 scripts/source-descriptions.py check
test -z "$(gofmt -l cmd internal tests)"
go test ./...
go vet ./...
npm test
npm run lint
npm run typecheck
