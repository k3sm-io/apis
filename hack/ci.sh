#!/usr/bin/env bash
# apis local CI — the docs/GO-STANDARDS.md commit gate in one command.
# The standard CI / pre-commit gate for this repo. Run from anywhere.
set -euo pipefail
cd "$(dirname "$0")/.."   # repo root

CGO=0   # apis is pure Go
STATICCHECK_VERSION=2026.2.1   # keep equal to the install pin in docs/GO-STANDARDS.md §Commit gates

echo "==> [apis] gofmt"
fmt=$(gofmt -l .) || true
[ -z "$fmt" ] || { echo "gofmt -w needed:"; echo "$fmt"; exit 1; }

echo "==> [apis] license headers"
hack/verify-boilerplate.sh

# Enumerate the Go packages BEFORE deciding to skip anything. Exit 0 with empty
# output means "no Go packages yet" — the legitimate skip this guard was written
# for. A NON-ZERO exit (broken go.mod, unresolvable dependency, bad GOWORK, absent
# toolchain) is a HARD ERROR: the old `[ -n "$(go list ./... 2>/dev/null)" ]` could
# not tell the two apart, so it silently skipped vet/build/test and still reported
# green — a gate that cannot even enumerate its packages must go RED (B168).
golist_err="$(mktemp)"
trap 'rm -f "$golist_err"' EXIT
if ! go_pkgs="$(CGO_ENABLED=$CGO go list ./... 2>"$golist_err")"; then
	echo "FAIL: [apis] go list ./... failed — cannot enumerate packages; refusing to skip vet/build/test:" >&2
	cat "$golist_err" >&2
	exit 1
fi

if [ -n "$go_pkgs" ]; then
	echo "==> [apis] go vet";   CGO_ENABLED=$CGO go vet ./...
	echo "==> [apis] go build"; CGO_ENABLED=$CGO go build ./...
	echo "==> [apis] go test";  CGO_ENABLED=$CGO go test ./...

	# staticcheck, pinned to an exact version: a different staticcheck reports
	# different findings, so an unpinned tool would make the verdict depend on the
	# host. Bump STATICCHECK_VERSION together with the Go toolchain and with
	# docs/GO-STANDARDS.md (keep in step with the other k3sm repos).
	#
	# Scope is -tests=false: test code is out of scope until a separate sweep. The
	# generated *.pb.go files are non-test code and ARE checked; the tree has zero
	# findings under this scope today.
	#
	# Suppress a finding with `//lint:ignore <Check> <reason>` on the preceding
	# line. (`//nolint` is golangci-lint's spelling and is inert here.)
	#
	# Every candidate is probed and the first whose version matches exactly wins, so
	# a stale copy earlier on PATH does not shadow a correct one in GOPATH/bin. No
	# match is RED, never a skip.
	sc=""; sc_seen=""
	sc_cands="$(command -v staticcheck 2>/dev/null || true)"
	sc_gopath_bin="$(go env GOPATH)/bin/staticcheck"
	if [ -x "$sc_gopath_bin" ] && [ "$sc_gopath_bin" != "$sc_cands" ]; then sc_cands="${sc_cands}"$'\n'"${sc_gopath_bin}"; fi
	while IFS= read -r c; do
		[ -n "$c" ] || continue
		sc_ver="$("$c" -version 2>/dev/null | awk '{print $2}' || true)"
		sc_seen="${sc_seen}  ${c}: ${sc_ver:-unknown}"$'\n'
		if [ -z "$sc" ] && [ "$sc_ver" = "$STATICCHECK_VERSION" ]; then sc="$c"; fi
	done <<< "$sc_cands"
	if [ -z "$sc" ]; then
		if [ -z "$sc_seen" ]; then
			echo "==> [apis] staticcheck: not installed; the gate needs ${STATICCHECK_VERSION} (go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION})" >&2
		else
			echo "==> [apis] staticcheck: no candidate matches; the gate needs ${STATICCHECK_VERSION}:" >&2
			printf '%s' "$sc_seen" >&2
			echo "    (go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION})" >&2
		fi
		exit 1
	fi
	echo "==> [apis] staticcheck ${STATICCHECK_VERSION} (non-test)"; CGO_ENABLED=$CGO "$sc" -tests=false ./...
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

# The version verdict is buf-env.sh's tool_version_matches — the same detection
# hack/gen.sh uses to decide a reinstall; here a mismatch only WARNs.
buf_bin="$(command -v buf)"
buf_ver="$(buf --version 2>/dev/null || echo unknown)"
echo "==> [apis] buf toolchain: ${buf_ver} (${buf_bin})"
if ! tool_version_matches buf "$BUF_VERSION"; then
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
