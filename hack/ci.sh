#!/usr/bin/env bash
# apis local CI — the docs/GO-STANDARDS.md commit gate in one command.
# Exactly what a /orchestrate builder agent runs before checkpointing, and what the
# workspace hack/ci.sh invokes for this repo. Run from anywhere.
set -euo pipefail
cd "$(dirname "$0")/.."   # repo root

CGO=0   # apis is pure Go

echo "==> [apis] gofmt"
fmt=$(gofmt -l .) || true
[ -z "$fmt" ] || { echo "gofmt -w needed:"; echo "$fmt"; exit 1; }

echo "==> [apis] license headers"
hack/verify-boilerplate.sh

if [ -n "$(CGO_ENABLED=$CGO go list ./... 2>/dev/null)" ]; then
	echo "==> [apis] go vet";   CGO_ENABLED=$CGO go vet ./...
	echo "==> [apis] go build"; CGO_ENABLED=$CGO go build ./...
	echo "==> [apis] go test";  CGO_ENABLED=$CGO go test ./...
else
	echo "==> [apis] (no Go packages yet — skipping vet/build/test)"
fi

echo "==> [apis] go mod tidy (no-diff)"
go mod tidy
if [ -n "$(git status --porcelain -- go.mod go.sum 2>/dev/null)" ]; then
	echo "go.mod/go.sum not tidy after 'go mod tidy':"; git --no-pager diff -- go.mod go.sum; exit 1
fi

# buf checks — guarded like the Go steps above: a no-op when buf isn't installed
# (CI provisions it via hack/gen.sh; local runs without buf still pass the Go gate).
if command -v buf >/dev/null 2>&1; then
	echo "==> [apis] buf lint"
	buf lint

	echo "==> [apis] buf generate (reproducible — no diff)"
	buf generate
	# buf generate only writes *.pb.go; any diff means the checked-in generated
	# code is stale vs the .proto + pinned plugins.
	if ! git diff --quiet -- ':(glob)**/*.pb.go'; then
		echo "generated code is stale — run hack/gen.sh and commit:"
		git --no-pager diff -- ':(glob)**/*.pb.go'; exit 1
	fi

	echo "==> [apis] buf breaking (wire stability vs checked-in baseline)"
	buf breaking --against buf/baseline.binpb
else
	echo "==> [apis] (buf not installed — skipping buf generate-diff + breaking; run hack/gen.sh)"
fi

echo "OK: apis ci green"
