#!/usr/bin/env bash
# apis proto codegen — installs the PINNED buf + protoc plugins onto PATH and runs
# `buf generate`. The generated *.pb.go / *_grpc.pb.go are checked in; CI re-runs
# this and fails on any diff (reproducible codegen). Run from anywhere.
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

main() {
	cd "$(dirname "${BASH_SOURCE[0]}")/.."   # repo root

	echo "==> [apis] ensuring pinned codegen toolchain"
	ensure_toolchain

	echo "==> [apis] buf dep update (refresh buf.lock for the google/rpc/status import)"
	buf dep update

	echo "==> [apis] buf generate"
	buf generate

	echo "OK: apis codegen done"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main "$@"
fi
