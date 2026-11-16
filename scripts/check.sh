#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
python3 scripts/source-descriptions.py check
python3 scripts/check-docs.py
test -z "$(gofmt -l cmd internal tests)"
go test ./...
go vet ./...
node --test scripts/normalize-bindings.test.mjs
npm test
npm run lint
npm run typecheck
