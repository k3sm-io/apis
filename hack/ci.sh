#!/usr/bin/env bash
# apis local CI — the docs/GO-STANDARDS.md commit gate in one command.
# The standard CI / pre-commit gate for this repo. Run from anywhere.
#
#   hack/ci.sh [--tier fast|full] [--edited <file>]
#
# --tier picks how the test stage applies hack/quarantine.txt: full (the default)
# reds on every failing test; fast excuses a listed flake outside the packages the
# change edits. --edited names a file with one edited import path per line.
set -euo pipefail
cd "$(dirname "$0")/.."   # repo root

test_tier=full
edited_file=""
while [ $# -gt 0 ]; do
	case "$1" in
		--tier) [ $# -ge 2 ] || { echo "usage: hack/ci.sh [--tier fast|full] [--edited <file>]" >&2; exit 2; }; test_tier="$2"; shift 2 ;;
		--edited) [ $# -ge 2 ] || { echo "usage: hack/ci.sh [--tier fast|full] [--edited <file>]" >&2; exit 2; }; edited_file="$2"; shift 2 ;;
		*) echo "usage: hack/ci.sh [--tier fast|full] [--edited <file>]" >&2; exit 2 ;;
	esac
done
case "$test_tier" in fast|full) ;; *) echo "hack/ci.sh: --tier must be fast or full" >&2; exit 2 ;; esac
edited_pkgs=()
if [ -n "$edited_file" ]; then
	[ -r "$edited_file" ] || { echo "hack/ci.sh: --edited $edited_file is not readable" >&2; exit 2; }
	while IFS= read -r p; do
		case "$p" in ''|'#'*) continue ;; esac
		edited_pkgs+=("$p")
	done < "$edited_file"
fi

CGO=0   # apis is pure Go
STATICCHECK_VERSION=2026.2.1   # keep equal to the install pin in docs/GO-STANDARDS.md §Commit gates
GOVULNCHECK_VERSION=v1.9.0   # the govulncheck the gate requires; keep equal across the k3sm repos and the k3sm repository's hack/etcd-wrapper.sh

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
# green — a gate that cannot even enumerate its packages must go RED.
ci_tmp="$(mktemp -d)"
trap 'rm -rf "$ci_tmp"' EXIT
golist_err="$ci_tmp/golist.err"
if ! go_pkgs="$(CGO_ENABLED=$CGO go list ./... 2>"$golist_err")"; then
	echo "FAIL: [apis] go list ./... failed — cannot enumerate packages; refusing to skip vet/build/test:" >&2
	cat "$golist_err" >&2
	exit 1
fi

if [ -n "$go_pkgs" ]; then
	echo "==> [apis] go vet";   CGO_ENABLED=$CGO go vet ./...
	echo "==> [apis] go build"; CGO_ENABLED=$CGO go build ./...
	# Every test runs under the race detector, as a go test -json stream piped
	# through hack/lib/testresults.go, which validates hack/quarantine.txt and applies
	# it for the tier. It echoes the output a plain go test prints and ends with the
	# failing tests. Its exit status says whether go test itself should have failed
	# (0: no failure, 3: only registry-excused failures), so the two statuses are
	# checked against each other: neither can turn the other green.
	echo "==> [apis] go test -race (quarantine tier: $test_tier)"
	CGO_ENABLED=$CGO go build -o "$ci_tmp/testresults" hack/lib/testresults.go
	set +e
	CGO_ENABLED=$CGO go test -race -json ./... 2>"$ci_tmp/gotest.err" \
		| "$ci_tmp/testresults" -registry hack/quarantine.txt -tier "$test_tier" ${edited_pkgs[@]+"${edited_pkgs[@]}"}
	st=("${PIPESTATUS[@]}")
	set -e
	go_rc="${st[0]}" tr_rc="${st[1]}"
	if [ -s "$ci_tmp/gotest.err" ]; then
		echo "==> [apis] go test stderr:" >&2
		cat "$ci_tmp/gotest.err" >&2
	fi
	case "$go_rc:$tr_rc" in
		0:0) ;;
		1:3) echo "==> [apis] go test failed only on quarantined tests (see QUARANTINED above)" ;;
		*:1) echo "==> [apis] test stage RED (go test exit $go_rc); the RED lines above say why" >&2; exit 1 ;;
		*:0) echo "FAIL: [apis] go test exited $go_rc but its stream holds no failure; read the stderr above" >&2; exit 1 ;;
		*) echo "FAIL: [apis] go test exit $go_rc and testresults exit $tr_rc disagree" >&2; exit 1 ;;
	esac

	# The race witness: a planted race that go test -race must catch. If the
	# toolchain's race detector is not live, this goes red instead of the gate
	# passing races silently.
	echo "==> [apis] race witness"
	if CGO_ENABLED=$CGO go test -race -count=1 ./hack/testdata/racewitness > "$ci_tmp/witness.log" 2>&1; then
		echo "FAIL: [apis] the planted race in hack/testdata/racewitness passed; go test -race does not detect races" >&2
		exit 1
	fi
	if ! grep -q 'WARNING: DATA RACE' "$ci_tmp/witness.log"; then
		echo "FAIL: [apis] the race witness failed without a race report:" >&2
		cat "$ci_tmp/witness.log" >&2
		exit 1
	fi

	# Wall-clock waits and fixed ports in tests, outside hack/test-sleeps.allow.
	echo "==> [apis] test-sleep lint"
	hack/verify-test-sleeps.sh --self-test
	hack/verify-test-sleeps.sh

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

# The dependency supply-chain check (hack/verify-deps.sh): the go environment
# cannot weaken verification, go mod verify, the hack/deps.lock module graph,
# the 14-day cooldown on new versions, the OSV malicious-package query, go.mod
# replace discipline, and exact pins on every remote go install / go run. It
# needs proxy.golang.org and api.osv.dev; there is no offline skip, because a
# network failure there is RED by design. It runs only in the real module: the
# k3sm repo's hack/acceptance/ci_guard_test.go runs this script in a throwaway
# module whose graph the lock could never describe.
if grep -qx "module k3sm.io/apis" go.mod 2>/dev/null; then
	echo "==> [apis] verify-deps (self-test)"
	hack/verify-deps.sh --self-test
	echo "==> [apis] verify-deps"
	hack/verify-deps.sh

	# govulncheck, pinned to an exact version like staticcheck: a different release
	# can report different findings. Each candidate (PATH first, then
	# $(go env GOPATH)/bin) is probed and the first reporting exactly the pin is
	# used. A missing or mismatched tool is RED, never a skip, and so is a finding
	# or a failure to reach the vulnerability database.
	gv=""; gv_seen=""
	for c in "$(command -v govulncheck 2>/dev/null || true)" "$(go env GOPATH)/bin/govulncheck"; do
		[ -n "$c" ] && [ -x "$c" ] || continue
		gv_ver="$("$c" -version 2>/dev/null | sed -n 's/^Scanner: govulncheck@//p' | head -1)" || gv_ver=""
		gv_seen="${gv_seen}    $c reports ${gv_ver:-no version}"$'\n'
		if [ "$gv_ver" = "$GOVULNCHECK_VERSION" ]; then gv="$c"; break; fi
	done
	if [ -z "$gv" ]; then
		if [ -z "$gv_seen" ]; then
			echo "==> [apis] govulncheck: not installed; the gate needs ${GOVULNCHECK_VERSION} (go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION})" >&2
		else
			echo "==> [apis] govulncheck: wrong version; the gate needs ${GOVULNCHECK_VERSION} (go install golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}). Found:" >&2
			printf '%s' "$gv_seen" >&2
		fi
		exit 1
	fi
	echo "==> [apis] govulncheck ${GOVULNCHECK_VERSION} ./..."
	GOWORK=off CGO_ENABLED=$CGO "$gv" ./...
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

# A genuine absence never passes silently — the shape k3sm/hack/acceptance uses:
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
