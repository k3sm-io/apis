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

// Package guestv1 is the generated Go binding for the k3sm.guest.v1 protobuf
// package (guest/v1/guest.proto): the whole host↔guest contract for a
// `vm`-backend pod, in one home. There is deliberately no k3sm.io/apis/vm/v1 —
// all three parts below describe one boundary.
//
//   - GuestAgent — the vsock gRPC service the in-guest agent serves
//     (Health, ContainerEvents, Exec, Logs, Stats, Stop). Exec and Logs reuse
//     the k3sm.io/apis/runtime/v1 stream messages verbatim rather than
//     declaring parallel copies; in a single-pod guest their pod_id must equal
//     the pod the guest booted and container selects within it.
//   - GuestSpec — the boot contract. The on-disk `guest-spec.json` is the
//     proto-JSON encoding of this message, so its schema lives here and
//     nowhere else.
//   - VMHostSpec — the machine contract. The on-disk `vmhost.spec.json` is the
//     proto-JSON encoding of this message.
//
// # Why this is a versioned wire contract
//
// Both ends are built from k3sm.io/runtimed, but the guest end ships inside
// the initramfs: an independently released, sha256-pinned artifact acquired
// at install time, not a symbol linked into the daemon. A node can therefore
// run a daemon and an initramfs that were not built together, so this
// boundary gets the same additive-only discipline and buf baseline coverage
// as runtime/v1.
//
// # Compat posture
//
// Compat is lockstep via the in-code initramfs sha256 pin: the supported
// configuration is exactly the daemon paired with the initramfs its own pin
// names. The dev-lab --guest-artifacts-dir override is unsupported skew.
// HealthResponse carries api_version (plus additive-only capabilities
// tokens) so that skew is a legible rejection rather than silent stream
// corruption: a host that dials an agent speaking a different api_version
// fails the pod with that stated reason.
//
// # Stability contract
//
// Field numbers are stable and the surface is additive-only forever. Never
// renumber or repurpose a field; never reuse a reserved number or name. Every
// message keeps the file convention's reserved 100 to 149 headroom band.
// `buf breaking` (WIRE_JSON) enforces this against buf/baseline.binpb.
//
// Like the rest of this module the package imports nothing from another
// k3sm.io repo (only its sibling k3sm.io/apis/runtime/v1) and builds pure Go,
// CGO_ENABLED=0.
package guestv1
