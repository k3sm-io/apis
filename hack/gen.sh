#!/usr/bin/env bash
# apis proto codegen — installs the PINNED buf + protoc plugins onto PATH and runs
# `buf generate`. The generated *.pb.go / *_grpc.pb.go are checked in; CI re-runs
# this and fails on any diff (reproducible codegen). Run from anywhere.
set -euo pipefail
cd "$(dirname "$0")/.."   # repo root

# Pinned toolchain versions (keep in lockstep with go.mod + buf.gen.yaml).
BUF_VERSION=v1.71.0
PROTOC_GEN_GO_VERSION=v1.36.11
PROTOC_GEN_GO_GRPC_VERSION=v1.5.1

GOBIN="$(go env GOPATH)/bin"
export PATH="$GOBIN:$PATH"

# `go install` is hermetic on version; skip if the pinned binary is already present.
need() { ! command -v "$1" >/dev/null 2>&1; }
echo "==> [apis] ensuring pinned codegen toolchain"
need buf                && GOFLAGS=-mod=mod go install "github.com/bufbuild/buf/cmd/buf@${BUF_VERSION}"
need protoc-gen-go      && GOFLAGS=-mod=mod go install "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
need protoc-gen-go-grpc && GOFLAGS=-mod=mod go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"

echo "==> [apis] buf dep update (refresh buf.lock for the google/rpc/status import)"
buf dep update

echo "==> [apis] buf generate"
buf generate

echo "OK: apis codegen done"
