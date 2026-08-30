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

package runtimev1

// The k3sm.io/* node labels and pod annotations that parameterize this runtime
// contract. They live in the runtime-area apis package — not in an area package
// of their own and not in a consumer — because they are k3sm.io-domain keys that
// describe or steer what this contract's messages mean, and because every
// consumer (the k3sm provider that writes them, the runtime that reads them,
// the scheduler predicates that select on them) must agree on the exact byte
// string. A string literal in a consumer is a second source of truth that
// compiles perfectly while being wrong; these constants exist so no consumer has
// to spell one.
//
// The values are part of the stable surface. A key rename is a break for every
// already-labelled node and every already-scheduled pod, so treat them the way
// field numbers are treated: additive only.
const (
	// LabelRosetta is the node label reporting that the HOST can execute
	// linux/amd64 binaries under Rosetta translation — the native-path
	// capability, advertised by the node itself.
	//
	// It says nothing about VM pods: a host with Rosetta installed may have no
	// usable virtualization backend at all. Selecting a node for an amd64 Linux
	// workload therefore requires LabelRosettaLinux, never this one.
	LabelRosetta = "k3sm.io/rosetta"

	// LabelRosettaLinux is the node label reporting that a LINUX GUEST on this
	// node can execute linux/amd64 payloads under Rosetta.
	//
	// It is a COMPOSITION, deliberately not a synonym of LabelRosetta: it is
	// present only when guest Rosetta is usable AND the VM backend is available
	// (the same single availability probe the backend selection reads — not a
	// second, separately-drifting derivation). Either conjunct alone is
	// insufficient and must not set it: Rosetta with no VM backend has no guest
	// to translate for, and a VM backend without Rosetta boots arm64 guests only.
	//
	// Two labels rather than one because they answer different questions — "can
	// this host run an amd64 Mach-O" and "can a Linux pod on this node run an
	// amd64 ELF" — and collapsing them would make one of the two answers a
	// silent lie on every node where the conjuncts disagree.
	LabelRosettaLinux = "k3sm.io/rosetta-linux"

	// AnnotationImagePlatform is the POD-LEVEL annotation overriding the platform
	// a multi-platform image reference resolves to (its value is an OCI platform
	// string, e.g. "linux/amd64" or "linux/arm64/v8").
	//
	// It is pod-level and applies to EVERY container in the pod. The k3sm
	// provider parses it ONCE and stamps the parsed result onto each container's
	// Container.image_platform, so the runtime consumes a normalized message and
	// never re-parses a user-supplied string — one parse point, one place a
	// malformed value is rejected.
	//
	// There is deliberately NO per-container key form in v1. A per-container
	// annotation would need a container name inside the key, which makes the key
	// space unbounded and un-validatable, and mixed-platform containers within
	// one pod is not a workload k3sm supports (they share one guest, hence one
	// binfmt registration set). If that need appears, it arrives as a new
	// explicit surface, not as a key-naming convention.
	AnnotationImagePlatform = "k3sm.io/image-platform"
)
