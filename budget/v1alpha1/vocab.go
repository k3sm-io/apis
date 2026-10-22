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

package budgetv1alpha1

import (
	"encoding/json"
	"strings"
)

// The shared vocabulary of the budget contract. Every value here is a wire
// string (or, for a signal, also a page code) that the node, the server
// relay, the SDKs and any other producer must spell identically, so no
// consumer writes a literal. Enum values are kebab-case, as elsewhere in
// this module. Each type's Valid method reports whether a value is one this
// release knows.
//
// Two kinds of enum, and how a newer producer is tolerated within one
// annotation schema:
//
//   - Structural enums (SignalID, SignalUnit, Level, Band, Platform,
//     PolicyOutcome, PolicyField, KillModeName, WriterShape, CounterWindow)
//     change what a reader computes. An unknown value is refused, so adding
//     one bumps BudgetAnnotationSchema.
//   - Producer-vocabulary enums (Trend, LimitingResource, KillModeReason,
//     WouldKillReason, CounterReason) only describe. An unknown value decodes
//     to that type's "unknown" constant instead of failing the payload, so a
//     newer producer may add one without a schema bump. The string itself is
//     never kept: no free text reaches the status.

// SignalID names one pressure signal. The upstream eviction signal names are
// kept as they are; memory.compressor, memory.swap and swapfs.available are
// k3sm's own. Each has a fixed page code (PageCode), so the page can carry it
// as a number.
type SignalID string

const (
	// SignalMemoryAvailable is the memory still available to new work, in
	// bytes. It falls under pressure.
	SignalMemoryAvailable SignalID = "memory.available"
	// SignalMemoryCompressor is how full the memory compressor is, in
	// permille: the larger of its page ratio (compressed pages against the
	// page limit) and its segment ratio (segments in use against the segment
	// limit), whichever is nearer its limit. It rises under pressure.
	SignalMemoryCompressor SignalID = "memory.compressor"
	// SignalMemorySwap is swap growth, in bytes over the record's
	// WindowSeconds. It rises under pressure and has no evict-soft rung: swap
	// alone never causes a soft eviction. Its rungs also carry a
	// producer-side gate that is not in the record: a rung counts only while
	// the swap in use is at least max(1 GiB, a quarter of physical memory).
	// A reader therefore cannot derive the level from Value and the rungs
	// alone; the published Level is the producer's verdict.
	SignalMemorySwap SignalID = "memory.swap"
	// SignalSwapfsAvailable is the free space on the volume that holds swap,
	// in bytes. It falls under pressure.
	SignalSwapfsAvailable SignalID = "swapfs.available"
	// SignalNodefsAvailable is the free space on the node's data volume, in
	// bytes. It falls under pressure.
	SignalNodefsAvailable SignalID = "nodefs.available"
	// SignalImagefsAvailable is the free space on the image volume, in bytes.
	// It falls under pressure. When the image store shares the data volume,
	// the record says so with SharedWith.
	SignalImagefsAvailable SignalID = "imagefs.available"
	// SignalPIDAvailable is the process IDs still available system-wide. It
	// falls under pressure. A per-user process limit, where the producer
	// enforces one, is a node condition only and is not in the record.
	SignalPIDAvailable SignalID = "pid.available"
)

// signalCodes is the page code table: index i holds the signal with code
// i+1. Code 0 is an empty page slot. The order is part of the page ABI and
// is append-only.
var signalCodes = [...]SignalID{
	SignalMemoryAvailable,
	SignalMemoryCompressor,
	SignalMemorySwap,
	SignalSwapfsAvailable,
	SignalNodefsAvailable,
	SignalImagefsAvailable,
	SignalPIDAvailable,
}

// Signals returns every SignalID this release knows, in page-code order.
func Signals() []SignalID {
	out := make([]SignalID, len(signalCodes))
	copy(out, signalCodes[:])
	return out
}

// Valid reports whether s is a signal this release knows.
func (s SignalID) Valid() bool { return s.PageCode() != 0 }

// PageCode returns the signal's fixed page code, 1 to 7, or 0 for a signal
// this release does not know (0 is an empty page slot).
func (s SignalID) PageCode() uint64 {
	for i, id := range signalCodes {
		if id == s {
			return uint64(i + 1)
		}
	}
	return 0
}

// SignalForPageCode returns the signal with page code c, and false for 0 or
// a code this release does not know.
func SignalForPageCode(c uint64) (SignalID, bool) {
	if c == 0 || c > uint64(len(signalCodes)) {
		return "", false
	}
	return signalCodes[c-1], true
}

// IsDescending reports whether the signal's value falls as pressure rises:
// true for every "*.available" signal. Its ladder rungs then descend
// (warnThreshold > admitStopThreshold > ... > evictHardThreshold) and each
// release sits above its rung; for a rising signal both orders are reversed.
//
// The rule is frozen and keyed on the name: every signal whose ID ends in
// ".available" falls under pressure, and every other signal rises. The CEL
// rules on both CRDs test the same suffix, so a new signal must be named to
// match its direction.
func IsDescending(s SignalID) bool { return strings.HasSuffix(string(s), ".available") }

// Unit returns the signal's one canonical unit: bytes for memory.swap and
// for every "*.available" signal except pid.available, permille for
// memory.compressor, and a count for pid.available. A record in another unit
// is refused, so a policy threshold for a signal is always in this unit. It
// returns "" for an unknown signal.
func (s SignalID) Unit() SignalUnit {
	switch s {
	case SignalMemoryCompressor:
		return UnitPermille
	case SignalPIDAvailable:
		return UnitCount
	case SignalMemoryAvailable, SignalMemorySwap, SignalSwapfsAvailable, SignalNodefsAvailable, SignalImagefsAvailable:
		return UnitBytes
	}
	return ""
}

// SignalUnit is the unit of a signal's value, rungs and releases.
type SignalUnit string

const (
	// UnitBytes is a size in bytes.
	UnitBytes SignalUnit = "bytes"
	// UnitPermille is a fraction in thousandths (0 to 1000).
	UnitPermille SignalUnit = "permille"
	// UnitCount is a plain count (for example process IDs).
	UnitCount SignalUnit = "count"
)

// Valid reports whether u is a unit this release knows.
func (u SignalUnit) Valid() bool { return oneOf(u, UnitBytes, UnitPermille, UnitCount) }

// Level is the node's derived pressure level, the most severe over its
// signals.
type Level string

const (
	// LevelOK means no signal has reached its warn rung.
	LevelOK Level = "ok"
	// LevelWarn means a signal is at or past its warn rung.
	LevelWarn Level = "warn"
	// LevelCritical means a signal is at or past its critical rung.
	LevelCritical Level = "critical"
	// LevelEvicting means a signal is at or past its evict-soft rung; the
	// observation then carries EvictDeadline.
	LevelEvicting Level = "evicting"
)

// Valid reports whether l is a level this release knows.
func (l Level) Valid() bool { return oneOf(l, LevelOK, LevelWarn, LevelCritical, LevelEvicting) }

// Band is a pod's eviction band. A pod is critical by priority, batch only by
// the explicit budget.k3sm.io/band annotation, and standard otherwise; the
// annotation can lower a band, never raise it.
type Band string

const (
	// BandCritical is never evicted by k3sm, in either tier.
	BandCritical Band = "critical"
	// BandStandard is the default band; tier 2 may take it after batch.
	BandStandard Band = "standard"
	// BandBatch is the only band tier 1 may take.
	BandBatch Band = "batch"
)

// Bands returns the three bands in their fixed order (critical, standard,
// batch), the order the page lays out its counters in.
func Bands() []Band { return []Band{BandCritical, BandStandard, BandBatch} }

// Valid reports whether b is a band this release knows.
func (b Band) Valid() bool { return oneOf(b, BandCritical, BandStandard, BandBatch) }

// Trend is the recent direction of a signal's value.
type Trend string

const (
	// TrendFalling means the value is going down.
	TrendFalling Trend = "falling"
	// TrendSteady means the value is not moving materially.
	TrendSteady Trend = "steady"
	// TrendRising means the value is going up.
	TrendRising Trend = "rising"
	// TrendUnknown stands for a trend this release does not know.
	TrendUnknown Trend = "unknown"
)

// Valid reports whether t is a trend this release knows.
func (t Trend) Valid() bool { return oneOf(t, TrendFalling, TrendSteady, TrendRising, TrendUnknown) }

// UnmarshalJSON decodes a trend, mapping a value this release does not know
// to TrendUnknown.
func (t *Trend) UnmarshalJSON(b []byte) error { return decodeOpenEnum(b, t, TrendUnknown) }

// LimitingResource names the resource that actually limits a signal, for
// example swapfs when swap growth is bounded by disk space rather than by
// memory.
type LimitingResource string

const (
	// ResourceMemory is physical memory.
	ResourceMemory LimitingResource = "memory"
	// ResourceSwapfs is the free space on the volume that holds swap.
	ResourceSwapfs LimitingResource = "swapfs"
	// ResourceNodefs is the free space on the node's data volume.
	ResourceNodefs LimitingResource = "nodefs"
	// ResourceImagefs is the free space on the image volume.
	ResourceImagefs LimitingResource = "imagefs"
	// ResourcePID is the process ID space.
	ResourcePID LimitingResource = "pid"
	// ResourceCPU is processor time.
	ResourceCPU LimitingResource = "cpu"
	// ResourceUnknown stands for a resource this release does not know.
	ResourceUnknown LimitingResource = "unknown"
)

// Valid reports whether r is a resource this release knows.
func (r LimitingResource) Valid() bool {
	return oneOf(r, ResourceMemory, ResourceSwapfs, ResourceNodefs, ResourceImagefs, ResourcePID, ResourceCPU, ResourceUnknown)
}

// UnmarshalJSON decodes a resource, mapping a value this release does not
// know to ResourceUnknown.
func (r *LimitingResource) UnmarshalJSON(b []byte) error {
	return decodeOpenEnum(b, r, ResourceUnknown)
}

// KillModeName is the tier-1 kill mode a policy sets and a node publishes.
// It governs tier 1 only; tier 2, the platform's hard eviction, is always
// armed.
type KillModeName string

const (
	// KillModeAuto observes until the node has recorded
	// ArmingEpisodesRequired matched pressure episodes, then arms. It is the
	// default.
	KillModeAuto KillModeName = "auto"
	// KillModeOn arms tier 1 without the observation period.
	KillModeOn KillModeName = "on"
	// KillModeOff disables tier 1. It does not disarm tier 2.
	KillModeOff KillModeName = "off"
)

// Valid reports whether m is a kill mode this release knows.
func (m KillModeName) Valid() bool { return oneOf(m, KillModeAuto, KillModeOn, KillModeOff) }

// KillModeReason says why the published kill mode is armed or not.
type KillModeReason string

const (
	// KillReasonColdStart means the node has no valid policy yet and
	// observes.
	KillReasonColdStart KillModeReason = "cold-start"
	// KillReasonObserving means mode auto has not yet recorded enough
	// matched episodes to arm.
	KillReasonObserving KillModeReason = "observing"
	// KillReasonArmed means tier 1 is armed.
	KillReasonArmed KillModeReason = "armed"
	// KillReasonPolicyOff means the policy sets mode off.
	KillReasonPolicyOff KillModeReason = "policy-off"
	// KillReasonDisarmFile means the node-local disarm file is present.
	KillReasonDisarmFile KillModeReason = "disarm-file"
	// KillReasonUnknown stands for a reason this release does not know.
	KillReasonUnknown KillModeReason = "unknown"
)

// Valid reports whether r is a kill-mode reason this release knows.
func (r KillModeReason) Valid() bool {
	return oneOf(r, KillReasonColdStart, KillReasonObserving, KillReasonArmed, KillReasonPolicyOff, KillReasonDisarmFile, KillReasonUnknown)
}

// UnmarshalJSON decodes a reason, mapping a value this release does not
// know to KillReasonUnknown.
func (r *KillModeReason) UnmarshalJSON(b []byte) error {
	return decodeOpenEnum(b, r, KillReasonUnknown)
}

// WouldKillReason is the trigger behind a tier-1 decision.
type WouldKillReason string

const (
	// WouldKillHeldPressure means the held-pressure condition held for its
	// duration past the eviction deadline.
	WouldKillHeldPressure WouldKillReason = "held-pressure"
	// WouldKillMemoryFloor means the memory-floor backstop tripped. It is
	// combined with the held-pressure condition by OR, never by AND.
	WouldKillMemoryFloor WouldKillReason = "memory-floor"
	// WouldKillUnknown stands for a trigger this release does not know.
	WouldKillUnknown WouldKillReason = "unknown"
)

// Valid reports whether r is a would-kill reason this release knows.
func (r WouldKillReason) Valid() bool {
	return oneOf(r, WouldKillHeldPressure, WouldKillMemoryFloor, WouldKillUnknown)
}

// UnmarshalJSON decodes a trigger, mapping a value this release does not
// know to WouldKillUnknown.
func (r *WouldKillReason) UnmarshalJSON(b []byte) error {
	return decodeOpenEnum(b, r, WouldKillUnknown)
}

// PolicyOutcome is what the node did with the policy relayed to it.
type PolicyOutcome string

const (
	// PolicyApplied means the policy was applied as written.
	PolicyApplied PolicyOutcome = "applied"
	// PolicyRejected means the policy was invalid; the node kept its last
	// valid policy.
	PolicyRejected PolicyOutcome = "rejected"
	// PolicyClamped means the policy was applied with some values raised to
	// the compiled floors.
	PolicyClamped PolicyOutcome = "clamped"
)

// Valid reports whether o is an outcome this release knows.
func (o PolicyOutcome) Valid() bool { return oneOf(o, PolicyApplied, PolicyRejected, PolicyClamped) }

// PolicyField names a NodeBudgetPolicy spec field a node may clamp, by its
// JSON path. AppliedPolicy.ClampedFields lists these and nothing else; the
// status writer turns them into the human-readable condition message.
type PolicyField string

const (
	// FieldTier1GraceSeconds is spec.tier1GraceSeconds.
	FieldTier1GraceSeconds PolicyField = "tier1GraceSeconds"
	// FieldTier1HeldSeconds is spec.tier1HeldSeconds.
	FieldTier1HeldSeconds PolicyField = "tier1HeldSeconds"
	// FieldTier1MaxDeliveriesPerHour is spec.tier1MaxDeliveriesPerHour.
	FieldTier1MaxDeliveriesPerHour PolicyField = "tier1MaxDeliveriesPerHour"
	// FieldTier1CooloffSeconds is spec.tier1CooloffSeconds.
	FieldTier1CooloffSeconds PolicyField = "tier1CooloffSeconds"
	// FieldReservesCPUSystemReserveMilli is spec.reserves.cpuSystemReserveMilli,
	// clamped to the measured capacity.
	FieldReservesCPUSystemReserveMilli PolicyField = "reserves.cpuSystemReserveMilli"
	// FieldReservesMemSystemReserveBytes is spec.reserves.memSystemReserveBytes,
	// clamped to the measured capacity.
	FieldReservesMemSystemReserveBytes PolicyField = "reserves.memSystemReserveBytes"
	// FieldReservesMemEvictionReserveBytes is
	// spec.reserves.memEvictionReserveBytes, clamped to the measured capacity.
	FieldReservesMemEvictionReserveBytes PolicyField = "reserves.memEvictionReserveBytes"
)

// PolicyFields returns every PolicyField this release knows.
func PolicyFields() []PolicyField {
	return []PolicyField{
		FieldTier1GraceSeconds, FieldTier1HeldSeconds, FieldTier1MaxDeliveriesPerHour, FieldTier1CooloffSeconds,
		FieldReservesCPUSystemReserveMilli, FieldReservesMemSystemReserveBytes, FieldReservesMemEvictionReserveBytes,
	}
}

// Valid reports whether f is a policy field this release knows.
func (f PolicyField) Valid() bool { return oneOf(f, PolicyFields()...) }

// CounterWindow is the time window of a windowed count.
type CounterWindow string

const (
	// Window1h is the last hour.
	Window1h CounterWindow = "1h"
	// Window24h is the last 24 hours.
	Window24h CounterWindow = "24h"
)

// Valid reports whether w is a window this release knows.
func (w CounterWindow) Valid() bool { return oneOf(w, Window1h, Window24h) }

// CounterReason is the event a windowed count counts, one per monotonic
// total of BandCounters.
type CounterReason string

const (
	// CounterEviction counts evictions.
	CounterEviction CounterReason = "eviction"
	// CounterOOMKill counts out-of-memory kills.
	CounterOOMKill CounterReason = "oom-kill"
	// CounterKernelKill counts kills by the kernel that the producer did not
	// initiate.
	CounterKernelKill CounterReason = "kernel-kill"
	// CounterRestart counts container restarts.
	CounterRestart CounterReason = "restart"
	// CounterMemHigh counts memory.high events.
	CounterMemHigh CounterReason = "mem-high"
	// CounterTier1Refused counts refused tier-1 deliveries.
	CounterTier1Refused CounterReason = "tier1-refused"
	// CounterUnknown stands for a counted event this release does not know.
	// Windowed counts of unknown reasons are summed into one entry per
	// window when a report is parsed.
	CounterUnknown CounterReason = "unknown"
)

// Valid reports whether r is a counter reason this release knows.
func (r CounterReason) Valid() bool {
	return oneOf(r, CounterEviction, CounterOOMKill, CounterKernelKill, CounterRestart, CounterMemHigh, CounterTier1Refused, CounterUnknown)
}

// UnmarshalJSON decodes a counted event, mapping a value this release does
// not know to CounterUnknown.
func (r *CounterReason) UnmarshalJSON(b []byte) error { return decodeOpenEnum(b, r, CounterUnknown) }

// WriterShape is how a NodeBudget's status reaches the API server.
type WriterShape string

const (
	// WriterAnnotationRelay means the node writes the budget.k3sm.io/status
	// annotation on its own Node and a server-side relay writes the status.
	// It is the only shape k3sm ships.
	WriterAnnotationRelay WriterShape = "annotation-relay"
	// WriterOwnNode means a per-node producer writes the status of its own
	// node's NodeBudget directly, held to that node by an admission policy
	// over its token's node-name claim.
	WriterOwnNode WriterShape = "own-node"
)

// Valid reports whether w is a writer shape this release knows.
func (w WriterShape) Valid() bool { return oneOf(w, WriterAnnotationRelay, WriterOwnNode) }

// Platform names the platform block a producer fills.
type Platform string

const (
	// PlatformDarwin is a macOS node; it fills the darwin block.
	PlatformDarwin Platform = "darwin"
	// PlatformLinux is a Linux node; it fills the linux block.
	PlatformLinux Platform = "linux"
)

// Valid reports whether p is a platform this release knows.
func (p Platform) Valid() bool { return oneOf(p, PlatformDarwin, PlatformLinux) }

// The NodeBudget condition types. The writer of the status (the relay, or
// an own-node producer) owns the conditions; a node's annotation payload
// never carries them.
const (
	// ConditionReporting is True while the node's budget is current.
	ConditionReporting = "Reporting"
	// ConditionPolicyRejected is True while the node refuses the policy
	// relayed to it and keeps its last valid one.
	ConditionPolicyRejected = "PolicyRejected"
	// ConditionPolicyClamped is True while the node applies its policy with
	// values raised to the compiled floors.
	ConditionPolicyClamped = "PolicyClamped"
	// ConditionPolicyConflict is True while more than one NodeBudgetPolicy
	// selects the node; the message names the policies that lost.
	ConditionPolicyConflict = "PolicyConflict"
)

// The condition reasons. Each NodeBudget condition has a closed set of
// reasons per status, so a reader (and a test) can branch on the reason
// without parsing the message. Naming: a True reason names the cause, a
// False reason names the state that holds instead, both in the upstream
// CamelCase reason style. A malformed, oversize and unsupported annotation
// are three distinct reasons, so the remedy is visible from the reason
// alone. ConditionReasons lists the sets.
const (
	// ReasonNodeReporting (Reporting True) means the node's budget is
	// current.
	ReasonNodeReporting = "NodeReporting"
	// ReasonNodeNotReporting (Reporting False) means the node has published
	// no budget, or its last one is too old.
	ReasonNodeNotReporting = "NodeNotReporting"
	// ReasonAnnotationMalformed (Reporting False) means the node's
	// annotation failed the strict decoder.
	ReasonAnnotationMalformed = "AnnotationMalformed"
	// ReasonSchemaUnsupported (Reporting False) means the node's annotation
	// carries a schema this reader does not know (for example, a newer
	// node).
	ReasonSchemaUnsupported = "SchemaUnsupported"
	// ReasonAnnotationTooLarge (Reporting False) means the node's annotation
	// is over MaxBudgetAnnotationBytes and was not parsed.
	ReasonAnnotationTooLarge = "AnnotationTooLarge"

	// ReasonPolicyInvalid (PolicyRejected True) means the relayed policy
	// failed NodeBudgetPolicySpec.Validate or the strict decoder.
	ReasonPolicyInvalid = "PolicyInvalid"
	// ReasonMergedLadderInverted (PolicyRejected True) means the policy is
	// valid on its own, but its thresholds merged over the producer's
	// defaults leave a signal's ladder out of order.
	ReasonMergedLadderInverted = "MergedLadderInverted"
	// ReasonPolicyAccepted (PolicyRejected False) means the node accepted
	// the policy it was sent, or no policy selects it.
	ReasonPolicyAccepted = "PolicyAccepted"

	// ReasonFieldsClamped (PolicyClamped True) means the node raised one or
	// more fields to a floor or to the measured capacity; the fields are in
	// status.policy.clampedFields.
	ReasonFieldsClamped = "FieldsClamped"
	// ReasonNoFieldsClamped (PolicyClamped False) means the policy in force
	// is applied as written.
	ReasonNoFieldsClamped = "NoFieldsClamped"

	// ReasonMultiplePolicies (PolicyConflict True) means more than one
	// NodeBudgetPolicy selects the node.
	ReasonMultiplePolicies = "MultiplePolicies"
	// ReasonSinglePolicy (PolicyConflict False) means at most one
	// NodeBudgetPolicy selects the node.
	ReasonSinglePolicy = "SinglePolicy"
)

// ConditionReasons returns the closed reason set of a NodeBudget condition
// type for one status ("True" or "False"), or nil for a type or status this
// release does not define.
func ConditionReasons(conditionType, status string) []string {
	switch conditionType + "/" + status {
	case ConditionReporting + "/True":
		return []string{ReasonNodeReporting}
	case ConditionReporting + "/False":
		return []string{ReasonNodeNotReporting, ReasonAnnotationMalformed, ReasonSchemaUnsupported, ReasonAnnotationTooLarge}
	case ConditionPolicyRejected + "/True":
		return []string{ReasonPolicyInvalid, ReasonMergedLadderInverted}
	case ConditionPolicyRejected + "/False":
		return []string{ReasonPolicyAccepted}
	case ConditionPolicyClamped + "/True":
		return []string{ReasonFieldsClamped}
	case ConditionPolicyClamped + "/False":
		return []string{ReasonNoFieldsClamped}
	case ConditionPolicyConflict + "/True":
		return []string{ReasonMultiplePolicies}
	case ConditionPolicyConflict + "/False":
		return []string{ReasonSinglePolicy}
	}
	return nil
}

// oneOf reports whether v equals one of want.
func oneOf[T comparable](v T, want ...T) bool {
	for _, w := range want {
		if v == w {
			return true
		}
	}
	return false
}

// decodeOpenEnum decodes a JSON string into a producer-vocabulary enum. A
// value Valid does not know becomes unknown; the empty string stays empty
// (the field is unset). Anything other than a string is an error.
func decodeOpenEnum[T interface {
	~string
	Valid() bool
}](b []byte, out *T, unknown T) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v := T(s)
	if s != "" && !v.Valid() {
		v = unknown
	}
	*out = v
	return nil
}
