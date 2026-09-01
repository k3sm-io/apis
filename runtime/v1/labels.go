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
	// LabelRosetta is the node label reporting that the host can execute
	// linux/amd64 binaries under Rosetta translation — the native-path
	// capability, advertised by the node itself.
	//
	// It says nothing about VM pods: a host with Rosetta installed may have no
	// usable virtualization backend at all. Selecting a node for an amd64 Linux
	// workload therefore requires LabelRosettaLinux, never this one.
	LabelRosetta = "k3sm.io/rosetta"

	// LabelRosettaLinux is the node label reporting that a Linux guest on this
	// node can execute linux/amd64 payloads under Rosetta.
	//
	// It is a composition, not a synonym of LabelRosetta: it is present only
	// when guest Rosetta is usable and the VM backend is available (the same
	// single availability probe the backend selection reads — not a second,
	// separately-drifting derivation). Either conjunct alone is insufficient
	// and must not set it: Rosetta with no VM backend has no guest to
	// translate for, and a VM backend without Rosetta boots arm64 guests only.
	//
	// Two labels rather than one because they answer different questions — "can
	// this host run an amd64 Mach-O" and "can a Linux pod on this node run an
	// amd64 ELF" — and collapsing them would make one of the two answers a
	// silent lie on every node where the conjuncts disagree.
	LabelRosettaLinux = "k3sm.io/rosetta-linux"

	// AnnotationImagePlatform is the pod-level annotation overriding the platform
	// a multi-platform image reference resolves to (its value is an OCI platform
	// string, e.g. "linux/amd64" or "linux/arm64/v8").
	//
	// It applies to every container in the pod. The k3sm provider parses it
	// once and stamps the parsed result onto each container's
	// Container.image_platform, so the runtime consumes a normalized message and
	// never re-parses a user-supplied string — one parse point, one place a
	// malformed value is rejected.
	//
	// There is deliberately no per-container key form in v1: a per-container
	// annotation would need a container name inside the key, which makes the key
	// space unbounded and un-validatable, and mixed-platform containers within
	// one pod is not a workload k3sm supports (they share one guest, hence one
	// binfmt registration set). If that need appears, it arrives as a new
	// explicit surface, not as a key-naming convention.
	AnnotationImagePlatform = "k3sm.io/image-platform"

	// AnnotationInternetEgress is the pod-level annotation opting a workload in
	// to reaching networks beyond the cluster. It is the operator-facing spelling
	// of SandboxProfile.allow_internet_egress: the k3sm provider reads this
	// annotation at admission and sets that proto field; nothing else sets it.
	//
	// It lives in the runtime-area package, not mlx/v1alpha1: it is a
	// k3sm.io/*-domain key that parameterizes this runtime contract's
	// SandboxProfile, and it is not MLX-specific (any pod that must fetch from
	// the internet wants it). The mlx package holds only mlx.k3sm.io/* keys.
	//
	// Enforcement ceiling, stated plainly (the same words the proto field
	// carries, because a consumer that reads only one of the two must still get
	// the truth): at the Seatbelt layer this currently emits the same
	// unfiltered-but-compilable network stanza as allow_network (macOS 26 cannot
	// express per-IP scoping); it is the API/admission contract — network-layer
	// (packet-filter) enforcement is tracked as future work. Treat its absence as
	// an admission-visible declaration, not as a proven network boundary.
	AnnotationInternetEgress = "k3sm.io/internet-egress"
)
