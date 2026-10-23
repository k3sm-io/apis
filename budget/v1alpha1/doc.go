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
// block, darwin or linux) plus the writer shape and the conditions. The
// conditions are owned by the status writer (k3sm's relay, or an own-node
// producer), never carried in the node's payload.
//
// NodeBudgetPolicy is cluster-scoped, spec only, and the one configuration
// object of the group: it selects node classes by nodeSelector and overrides
// the threshold ladder, the reserves, the history half-lives and the tier-1
// kill mode and its timers. When several policies select one node, the
// highest spec.priority wins and a tie goes to the lexically smallest name;
// the losers are reported on the NodeBudget through the PolicyConflict
// condition (SelectPolicy). status.policy says what the node did with the
// policy it was last sent; status.inForce names the policy it actually runs,
// so a rejected policy and the last valid one kept in its place are both
// visible. The conditions use the closed reason sets of ConditionReasons.
//
// The policy CRD defaults no field. An unset field means the producer's
// shipped default, the pinned Default* constant in floors.go, so each
// default has one owner; an unset mode means auto. Mode values must be
// quoted in YAML ("on", "off"): YAML 1.1, which the Kubernetes YAML decoders
// follow, reads bare on and off as booleans, and the object is then refused.
//
// # Signals and the ladder
//
// A signal is a generic record keyed by a SignalID (vocab.go). Its five rungs
// (warn, admit-stop, critical, evict-soft, evict-hard) are strictly ordered
// in the direction of rising pressure: ascending for a signal whose value
// grows under pressure, descending for an "*.available" signal
// (IsDescending). Each release value sits on the calm side of its rung.
// memory.swap has no evict-soft rung. Every signal has one canonical unit
// (SignalID.Unit), so a threshold means the same on every node. Durations
// on both objects are whole seconds, held in an int64, and times are
// metav1.Time; the budget page carries the same seconds, and its times as
// whole Unix seconds. The same rules are CEL on both CRDs (the
// NodeBudget status has no relay in front of it when an own-node producer
// writes it) and Go in NodeBudgetPolicySpec.Validate and
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
// Skew within one schema: a new field, or a new value of a structural enum
// (a signal, a unit, a level, a band, a platform, a policy outcome or field),
// bumps BudgetAnnotationSchema, because an older reader would misread it. A
// new value of a producer-vocabulary enum (a trend, a limiting resource, a
// kill-mode reason, a would-kill reason, a counter reason) does not: an
// older reader decodes it to that type's "unknown" constant and keeps the
// rest of the report.
//
// A producer that writes status directly (the own-node shape) has no decoder
// of ours in front of the CRD, and the CRD enum of a producer-vocabulary
// field is checked first, so it MUST map any trend, limiting resource,
// kill-mode reason, would-kill reason or counter reason it does not know to
// "unknown" before it writes. "unknown" is the read side's word for a value
// the reader cannot name: a producer never emits it for a value it does
// know.
//
// The unknown windowed counter (CounterUnknown) sums the counts of every
// reason a reader cannot name, so it is not monotonic across a producer
// upgrade: when the producer starts naming a reason an older reader folded
// into unknown, that count moves to the named reason and unknown falls.
// Consumers must not compute a rate from it.
//
// Adding a SignalID (a structural enum value) moves two versions together,
// in one change: BudgetAnnotationSchema is bumped, and the budget page's ABI
// version is bumped. The new signal takes the next unused page code (codes
// are never reused or renumbered, and 0 stays the empty slot). A reader of
// either that sees a version it does not know refuses the whole payload or
// page rather than reading a part of it.
//
// # No free text, so no pod identity
//
// The exact guarantee: apart from metadata and the writer-owned conditions,
// every string in a NodeBudget is a value of one of this package's enums,
// except the name and resourceVersion in status.policy and status.inForce
// (the object name an admin chose for a cluster-scoped NodeBudgetPolicy, and
// the API server's version of it). There is no map and no free-text field, so the
// node payload has nowhere to put a pod, namespace, container or UID. Victim
// identity stays in namespaced Events, which RBAC already governs.
//
// The one exception is a condition's message: free text written by the
// status writer (the relay or an own-node producer), never by the node's
// payload, which cannot carry conditions. Its reason is not free: the CRD
// and Validate hold it to the closed set of ConditionReasons for the
// condition's type and status. Only True and False are used as a condition
// status here; Unknown is refused rather than given a reason set, because
// every condition of the group is a fact the writer either has or has not
// established, and an absent condition already means not yet known.
//
// # Floors
//
// The tier-1 grace, the held-pressure duration, the hourly delivery cap and
// the cool-off have compiled-in floors (floors.go) that no policy can lower.
// The CRD schema enforces them on apply, and ApplyFloors clamps any relayed
// value that reaches a node below them (the node then reports outcome
// clamped).
//
// The asymmetry is deliberate. A value past a floor is clamped, because the
// operator's intent (more aggressive tier 1) is clear and the floor is the
// nearest safe reading of it. A value that makes the policy meaningless (an
// unknown enum, a misordered ladder, a negative size, a zero delivery cap) is
// refused: the node keeps its last valid policy and reports outcome
// rejected, because no reading of it is safe.
//
// # The budget page
//
// The page is the SDK's fast path over the same node-level data: one
// fixed 4096-byte little-endian file per node, PageFileName, at
// DefaultPagePath for native Darwin pods and LinuxPagePath on Linux. Its
// ABI (PageABIMajor, PageABIMinor) is versioned independently of the CRD
// version. page.go holds one ordered field table; the offsets, the layout
// descriptor (PageLayout), the byte budget and EncodePage all derive from
// it, and the golden fixtures, the frozen descriptors and the manifest
// under testdata/page are what the SDKs vendor and test against. Three
// decisions fix its shape:
//
// The page mirrors the status. A page field has the name, the meaning and
// the unit of the NodeBudget field it carries (seconds, not nanoseconds;
// windowSeconds, sharedWith and unit on every signal; all six per-band
// counters; the darwin and linux blocks as the status has them), so a
// value reads the same through either channel. Only the header (magic,
// abiMajor, abiMinor, headerSize, bodySize), seq, notifyEpoch and
// writerPeriodSeconds are page-only; generation is
// status.producerGeneration. The conditions, the writer, the policy
// references, wouldKill, the windowed counts and every string but the node
// name stay on the NodeBudget. Enums are u64 codes from append-only tables
// (page_codes.go, PageCodeTables) in which 0 is unset or unknown, and an
// optional i64 the producer could not sample holds PageAbsent. A vm guest
// page is the host's page with the linux block filled by guest-init as the
// guest section.
//
// The node name is inline: a length word, then a 256-byte field. A name
// over MaxPageNodeNameBytes is an encoder error (ErrPageNodeNameTooLong),
// and a reader refuses a page whose length word exceeds it.
//
// notifyEpoch is a u64 alone on its own 64-byte line at
// PageNotifyEpochOffset, outside the seqlock region. Readers that wait use
// its low 32-bit word, the four bytes at that same offset, as the futex or
// os_sync_wait_on_address target. PageWriter stores it after the final
// even seq store and only when PageNotifyDue (a change of level, admit, a
// threshold or the generation). It wraps freely, and readers compare it
// for inequality only.
//
// # Conformance
//
// The contract is platform-neutral so another producer, on any platform, can
// implement it. The golden NodeBudget documents under testdata/status, one
// Darwin and one Linux, are what a conforming producer's status is checked
// against, beside a dry-run apply against the CRD.
//
// Two files beside the goldens are the vendoring contract for the client
// SDKs. SHA256SUMS digests every other file in testdata/status (the form
// sha256sum -c and shasum -a 256 -c read), so an SDK vendors the golden
// bytes and checks them against the digest. nodebudget.paths.json lists
// every property path of the CRD's status schema (dotted, "[]" for an array
// element) with its kind (scalar, array or object) and, for a scalar, its
// OpenAPI type and format; an SDK's schema-walk tests are driven from it.
// Both are derived, never hand-edited: they are regenerated with the goldens
// (UPDATE_GOLDEN=1), and a test fails when either is stale.
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
