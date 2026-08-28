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
# Sourced, never executed: it only sets variables and prepends to PATH.

# Pinned toolchain versions (keep in lockstep with go.mod + buf.gen.yaml).
BUF_VERSION=v1.71.0
PROTOC_GEN_GO_VERSION=v1.36.11
PROTOC_GEN_GO_GRPC_VERSION=v1.5.1

GOBIN="$(go env GOPATH)/bin"
case ":${PATH}:" in
*":${GOBIN}:"*) ;;
*) PATH="${GOBIN}:${PATH}" ;;
esac
export PATH
