/*
Copyright The k3sm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package shimv1 is the generated Go binding for the k3sm.shim.v1 protobuf
// package (shim/v1/shim.proto): the ContainerShim gRPC service a resident
// per-container shim serves to the node runtime daemon. The shim owns one
// container's stdio, log file, and exit status, so that output, exit code,
// and exec survive a restart of the daemon.
//
// # Contract placement: internal, lockstep
//
// This is an INTERNAL daemon-to-shim contract. Both ends are built from the
// same runtimed build and versioned by that build; a daemon and a shim from
// different builds are never paired, because a daemon upgrade recreates every
// pod (and every shim with it). The package lives in k3sm.io/apis only so it
// shares the module's buf toolchain and codegen.
//
// It is therefore EXPLICITLY NOT covered by runtime/v1's additive-only-forever
// promise. A change here is a lockstep change to both ends. APIVersion,
// reported in StatusResponse.api_version, makes skew legible: the daemon
// refuses a shim speaking a different version with a stated reason. There is
// no capability negotiation.
//
// # Side channels beyond the messages
//
// The contract relies on three lockstep details owned by the runtime build.
// They are listed here so a reader knows they exist; they are not part of the
// messages. A second unix socket in the same shim directory carries file
// descriptors by SCM_RIGHTS: a tty Exec's pty slave and a rotated log file for
// ReopenLog. Each descriptor is matched to its call by a token in gRPC
// metadata (k3sm-pty-token on Exec, k3sm-log-token on ReopenLog). An Exec
// stream reports its session's pid and start time in response-header metadata
// (k3sm-session-pid, k3sm-session-start), so the caller can end that session
// when the stream ends.
//
// # Message reuse
//
// Exec and Follow reuse the k3sm.io/apis/runtime/v1 stream messages by
// descriptor (ExecRequest/ExecResponse and AttachResponse) rather than
// declaring parallel copies, the guest/v1 precedent. On Follow,
// AttachResponse.exit is a liveness hint only; StatusResponse.exit is the
// exit authority.
//
// Like the rest of this module the package imports nothing from another
// k3sm.io repo (only its sibling k3sm.io/apis/runtime/v1) and builds pure Go,
// CGO_ENABLED=0.
package shimv1
