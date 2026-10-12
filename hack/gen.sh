#!/usr/bin/env bash
# apis proto codegen — installs the PINNED buf + protoc plugins onto PATH and runs
# `buf generate`. The generated *.pb.go / *_grpc.pb.go are checked in; CI re-runs
# this and fails on any diff (reproducible codegen). Run from anywhere.
#
# Usage: hack/gen.sh [--update-deps | --check-lock]
#   (none)         regenerate using the committed buf.lock as is (never moves it)
#   --update-deps  also run `buf dep update`, deliberately moving buf.lock
#   --check-lock   offline: fail when buf.lock and buf.yaml deps disagree
#
# Sourceable: sourcing defines needs_install / ensure_tool / ensure_toolchain (and,
# via hack/lib/buf-env.sh, the pins) without installing or generating anything, so
# a gate can drive the install decision in isolation. Executing it runs main.

# Strict mode only when executed — never imposed on a caller that sources this.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	set -euo pipefail
fi

# Pinned versions, $(go env GOPATH)/bin on PATH, and tool_version_matches — one
# home, shared with hack/ci.sh.
. "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/buf-env.sh"

# needs_install <cmd> <pinned> — true when <cmd> is absent OR reports a version
# other than <pinned> (or an unparseable one). Presence alone is not enough: a
# stale pre-bump binary left in GOPATH/bin would otherwise silently win over a
# bumped pin and break reproducible codegen.
needs_install() { ! tool_version_matches "$1" "$2"; }

# ensure_tool <cmd> <pinned> <module-path> — `go install <module-path>@<pinned>`
# unless <cmd> already reports exactly <pinned>.
ensure_tool() {
	if needs_install "$1" "$2"; then
		echo "    ${1}: not at pinned ${2} — installing"
		GOFLAGS=-mod=mod go install "${3}@${2}"
	else
		echo "    ${1}: ${2} (pinned, present)"
	fi
}

ensure_toolchain() {
	ensure_tool buf                "$BUF_VERSION"                github.com/bufbuild/buf/cmd/buf
	ensure_tool protoc-gen-go      "$PROTOC_GEN_GO_VERSION"      google.golang.org/protobuf/cmd/protoc-gen-go
	ensure_tool protoc-gen-go-grpc "$PROTOC_GEN_GO_GRPC_VERSION" google.golang.org/grpc/cmd/protoc-gen-go-grpc
}

# yaml_dep_names <file> <key-regex> — module names listed under the top-level
# `deps:` of buf.yaml (`  - buf.build/x/y`) or buf.lock (`  - name: buf.build/x/y`).
yaml_dep_names() {
	awk -v pat="$2" '
		/^deps:/ { in_deps = 1; next }
		/^[^ #]/ { in_deps = 0 }
		in_deps && $0 ~ pat { sub(pat, ""); sub(/[ \t]*(#.*)?$/, ""); print }
	' "$1" | sort
}

# check_lock — offline check that buf.lock still matches buf.yaml: the same set
# of deps, each pinned with a commit and digest. It cannot see upstream moves
# (that needs the network and `buf dep update`); it catches the drift a regen
# would otherwise paper over. Never modifies any file.
check_lock() {
	local want have bad=0
	want=$(yaml_dep_names buf.yaml '^[ \t]*-[ \t]+')
	have=$(yaml_dep_names buf.lock '^[ \t]*-[ \t]+name:[ \t]+')
	if [ "$want" != "$have" ]; then
		echo "FAIL  [apis] buf.lock deps differ from buf.yaml deps" >&2
		echo "      buf.yaml: $(echo $want)" >&2
		echo "      buf.lock: $(echo $have)" >&2
		bad=1
	fi
	local n
	n=$(echo "$have" | grep -c . || true)
	if [ "$(grep -cE '^[ ]+commit: [0-9a-f]+$' buf.lock)" != "$n" ] ||
		[ "$(grep -cE '^[ ]+digest: b5:[0-9a-f]+$' buf.lock)" != "$n" ]; then
		echo "FAIL  [apis] buf.lock has a dep without a pinned commit and digest" >&2
		bad=1
	fi
	[ "$bad" = 0 ] || { echo "      run hack/gen.sh --update-deps and commit buf.lock deliberately" >&2; return 1; }
	echo "OK: buf.lock matches buf.yaml"
}

main() {
	cd "$(dirname "${BASH_SOURCE[0]}")/.."   # repo root

	local update_deps=0
	case "${1:-}" in
	--check-lock) check_lock; return ;;
	--update-deps) update_deps=1 ;;
	"") ;;
	*) echo "usage: hack/gen.sh [--update-deps | --check-lock]" >&2; return 2 ;;
	esac

	echo "==> [apis] ensuring pinned codegen toolchain"
	ensure_toolchain

	if [ "$update_deps" = 1 ]; then
		echo "==> [apis] buf dep update (deliberately moving buf.lock)"
		buf dep update
	fi

	echo "==> [apis] buf generate"
	buf generate

	echo "OK: apis codegen done"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main "$@"
fi
