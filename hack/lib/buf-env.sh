#!/usr/bin/env bash
# Shared proto-toolchain resolution for apis — the ONE home for the pinned
# versions and for where those binaries actually live.
#
# `hack/gen.sh` installs the pinned toolchain with `go install`, which lands it in
# $(go env GOPATH)/bin — a directory that is NOT on a default PATH. Any caller that
# decides "buf is not installed" from a bare `command -v buf` therefore reaches the
# wrong verdict on a machine where buf IS installed, merely unresolved. Both
# hack/gen.sh and hack/ci.sh source this file so that verdict is made once.
#
# Sourced, never executed: it only sets variables, prepends to PATH, and defines
# tool_version_matches (the one version detection gen.sh and ci.sh both call).

# Pinned toolchain versions (keep in lockstep with go.mod + buf.gen.yaml).
BUF_VERSION=v1.73.0
PROTOC_GEN_GO_VERSION=v1.36.12
PROTOC_GEN_GO_GRPC_VERSION=v1.6.2

GOBIN="$(go env GOPATH)/bin"
case ":${PATH}:" in
*":${GOBIN}:"*) ;;
*) PATH="${GOBIN}:${PATH}" ;;
esac
export PATH

# tool_version_matches <cmd> <pinned> — succeeds iff `<cmd> --version` exits 0 and
# the LAST whitespace-separated field of its stdout equals <pinned>, with a leading
# `v` stripped from both sides. That one normalization covers the three real
# shapes: `protoc-gen-go v1.36.12`, `protoc-gen-go-grpc 1.6.2`, buf's bare `1.73.0`.
# An absent command, a non-zero exit, or empty output is "no match" — the verdict
# fails toward reinstall (gen.sh) or a WARN (ci.sh), never toward silent trust.
# It is a version-STRING check, not a binary-identity or integrity check: a binary
# that echoes the pinned string is trusted. The supply-chain control is Go's module
# checksum database, and only on the `go install` path.
tool_version_matches() {
	local cmd="$1" pinned="$2" out got
	out="$("$cmd" --version 2>/dev/null)" || return 1
	got="$(printf '%s\n' "$out" | awk 'NF { last = $NF } END { print last }')"
	[ -n "$got" ] || return 1
	[ "${got#v}" = "${pinned#v}" ]
}
