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

package mlxv1alpha1

// The mlx.k3sm.io identifiers a node advertises and a pod selects on. Every
// consumer must agree on the exact byte string — the k3sm node advertiser that
// writes them, the scheduler predicates that match them, the operator that
// requests the resource — so they are published here and no consumer spells a
// literal. A literal in a consumer is a second source of truth that compiles
// perfectly while being wrong.
//
// UNLIKE the object shape in this package, these keys are STABLE. The alpha
// licence in the package doc does not extend to them: renaming one breaks every
// already-labelled node and every already-scheduled pod, and neither is a thing
// a version bump can migrate.
const (
	// GroupName is the API group of the mlx.k3sm.io CRDs (MLXModel). Note the
	// project convention: CRD groups are <area>.k3sm.io, while labels and
	// annotations are k3sm.io/*. The two namespaces are not interchangeable.
	GroupName = "mlx.k3sm.io"

	// ResourceGPU is the extended resource name a node advertises GPU capacity
	// under and a pod requests. It is a RESOURCE, not a label: it is counted and
	// consumed by the scheduler, so requesting it both selects a GPU node and
	// reserves the GPU against other pods.
	ResourceGPU = "mlx.k3sm.io/gpu"

	// LabelGPUPresent is the node LABEL reporting that this node has a usable
	// GPU. It is deliberately a DISTINCT key from ResourceGPU, not the same
	// string reused: a label answers "does this node have one" for selectors and
	// for humans reading `kubectl get nodes -L`, while the resource answers "how
	// many are left" for the scheduler. Reusing one string for both would make a
	// nodeSelector silently match on capacity semantics, and would collide the
	// two in any tooling that enumerates one namespace.
	//
	// Its value is the string "true" when present; the label is absent otherwise
	// (never present-and-"false", which a selector cannot express).
	LabelGPUPresent = "mlx.k3sm.io/gpu.present"

	// LabelChip is the node label carrying the exact chip, as a SLUG (e.g.
	// "apple-m4-max"). See the chip-slug rule below — the raw GPUFacts.chip_brand
	// ("Apple M4 Max") is NOT a valid label value.
	LabelChip = "mlx.k3sm.io/chip"

	// LabelChipFamily is the node label carrying the coarser chip family as a
	// slug (e.g. "m4"), for selecting a generation rather than an exact model.
	// Same slug rule as LabelChip.
	LabelChipFamily = "mlx.k3sm.io/chip-family"

	// LabelMemoryGB is the node label carrying the node's unified memory in whole
	// gibibytes as a decimal string (e.g. "128"). Whole GiB, and no unit suffix:
	// a label value is matched as an opaque string, so any suffix or fractional
	// spelling would make two nodes with identical memory carry non-matching
	// values. The authoritative byte-exact number stays in GPUFacts.mem_bytes;
	// this label exists for coarse selection only.
	LabelMemoryGB = "mlx.k3sm.io/memory-gb"
)

// Chip-slug normalization rule (the LabelChip / LabelChipFamily value form).
//
// GPUFacts.chip_brand and chip_family are carried VERBATIM across the runtime
// contract because they are raw host facts — "Apple M4 Max", spaces and all. A
// Kubernetes label value may not contain spaces, so the label form is a slug
// derived from the raw fact by the node advertiser, deterministically:
//
//  1. lowercase (ASCII);
//  2. replace every run of characters that are not [a-z0-9] with a single "-";
//  3. trim any leading or trailing "-";
//  4. truncate to 63 characters (the label-value limit), trimming any trailing
//     "-" left behind.
//
// So "Apple M4 Max" becomes "apple-m4-max" and the family "M4" becomes "m4".
//
// The rule is documented rather than implemented here on purpose: the derivation
// belongs to the single consumer that advertises the node (k3sm), which is also
// the only place that holds the raw facts. What this module owns is the KEY, and
// the guarantee that every consumer reading the label agrees on what shape the
// value has. A consumer must never compare a label value against a raw
// chip_brand — the raw fact is not the slug, and the comparison silently never
// matches.
