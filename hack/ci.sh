#!/usr/bin/env bash
# apis local CI — the docs/GO-STANDARDS.md commit gate in one command.
# The standard CI / pre-commit gate for this repo. Run from anywhere.
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

# buf checks — the WIRE-STABILITY contract of this repo, and the whole reason the
# module exists. `buf generate` (no-diff) proves the checked-in *.pb.go match the
# .proto + pinned plugins; `buf breaking` proves the wire contract every other
# k3sm.io repo compiles against did not regress. A run that skips them has proved
# neither, so it MUST NOT report "green".
#
# Resolution first: hack/lib/buf-env.sh puts the pinned $(go env GOPATH)/bin (where
# hack/gen.sh's `go install` lands buf) ahead on PATH, so "not installed" below means
# genuinely absent rather than merely unresolved.
. hack/lib/buf-env.sh

# A genuine absence never passes silently — the k3sm/hack/acceptance/B58.sh shape:
# hard FAIL under $CI (CI must carry the pinned toolchain), PENDING + non-zero
# locally so no caller can mistake a degraded run for a green one by exit code.
if ! command -v buf >/dev/null 2>&1; then
	echo "----------------------------------------"
	if [ -n "${CI:-}" ]; then
		echo "FAIL  [apis] buf not installed — CI MUST provide the pinned proto toolchain (hack/gen.sh)" >&2
		echo "apis ci: buf absent under CI — hard FAIL" >&2
		exit 1
	fi
	echo "PENDING: buf not installed — buf lint / generate-diff / breaking did NOT run." >&2
	echo "         The Go gate above passed; the WIRE-STABILITY gate is UNPROVEN." >&2
	echo "         Install the pinned toolchain with hack/gen.sh, then re-run." >&2
	echo "DEGRADED: apis ci NOT green — buf stages skipped" >&2
	# Non-zero so an un-run wire-stability gate can never be read as a pass.
	exit 3
fi

buf_bin="$(command -v buf)"
buf_ver="v$(buf --version 2>/dev/null || echo unknown)"
echo "==> [apis] buf toolchain: ${buf_ver} (${buf_bin})"
if [ "$buf_ver" != "$BUF_VERSION" ]; then
	echo "WARN: buf ${buf_ver} is not the pinned ${BUF_VERSION} — run hack/gen.sh if the generate-diff below is noisy" >&2
fi

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

echo "OK: apis ci green (incl. buf lint + generate-diff + breaking)"
