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

// Package budgetv1alpha1 holds the budget.k3sm.io/v1alpha1 API, the node
// budget contract: how much more work a node can take, how close it is to
// shedding work, and what it has evicted, published in a form any
// application on the cluster can read.
//
// # The objects
//
// NodeBudget is cluster-scoped, status only, and named for the node it
// describes; no other key exists. Its status is a NodeBudgetStatus: the
// node-produced NodeBudgetObservation (generic signal records, the derived
// level, the admit verdict, the eviction deadline, the recommendation, the
// published kill mode, the per-band counters, and exactly one platform
// block, darwin or linux) plus the writer shape and the conditions, which
// belong to whoever writes the object, never to the node's payload.
//
// NodeBudgetPolicy is cluster-scoped, spec only, and the one configuration
// object of the group: it selects node classes by nodeSelector and overrides
// the threshold ladder, the reserves, the history half-lives and the tier-1
// kill mode and its timers. When several policies select one node, the
// highest spec.priority wins and a tie goes to the lexically smallest name;
// the losers are reported on the NodeBudget through the PolicyConflict
// condition (SelectPolicy).
//
// # Signals and the ladder
//
// A signal is a generic record keyed by a SignalID (vocab.go). Its five rungs
// (warn, admit-stop, critical, evict-soft, evict-hard) are strictly ordered
// in the direction of rising pressure: ascending for a signal whose value
// grows under pressure, descending for an "*.available" signal
// (IsDescending). Each release value sits on the calm side of its rung.
// memory.swap has no evict-soft rung. The same rules are CEL on the
// NodeBudgetPolicy CRD and Go in NodeBudgetPolicySpec.Validate and
// NodeBudgetObservation.Validate.
//
// # The node annotation payloads
//
// A node publishes its observation as the NodeBudgetReport payload of the
// budget.k3sm.io/status Node annotation, and the server relays the resolved
// policy back as the NodeBudgetDesiredPolicy payload of budget.k3sm.io/policy.
// Both carry a schema version and are bounded by MaxBudgetAnnotationBytes.
// The decoder is strict: the size bound is checked before parsing, the
// schema is read on its own, unknown, duplicate or case-folded keys and
// trailing data are refused, and the result is validated. A report can
// never carry the writer or the conditions. Encode refuses anything the
// matching Parse function would refuse.
//
// # No pod identity
//
// Nothing in a NodeBudget names a pod, a namespace, a container or a UID.
// Victim identity stays in namespaced Events, which RBAC already governs; the
// object carries only node-level data, enum values and the name of the
// policy applied.
//
// # Floors
//
// The tier-1 grace, the held-pressure duration, the hourly delivery cap and
// the cool-off have compiled-in floors (floors.go) that no policy can lower.
// The CRD schema enforces them on apply, and ApplyFloors clamps any relayed
// value that reaches a node below them (the node then reports
// PolicyClamped).
//
// # Conformance
//
// The contract is platform-neutral so another producer, on any platform, can
// implement it. The golden NodeBudget documents under testdata/status, one
// Darwin and one Linux, are what a conforming producer's status is checked
// against, beside a dry-run apply against the CRD.
//
// Being served objects, NodeBudget and NodeBudgetPolicy embed
// metav1.TypeMeta and ObjectMeta and carry hand-written DeepCopy methods
// (this module runs no code generation). The CRD manifests are in
// k3sm.io/apis/config/crd (budget.k3sm.io_nodebudgets.yaml and
// budget.k3sm.io_nodebudgetpolicies.yaml), hand-authored to match these
// types.
//
// # Stability: alpha
//
// A single version, v1alpha1, is served and stored, and the object shapes
// may change incompatibly until they graduate. The group name, the
// annotation and taint keys and the annotation schema numbers are stable
// from the start: renaming a key strands every node annotated under it.
//
// The package imports no k3sm.io/* module and builds CGO_ENABLED=0.
package budgetv1alpha1
