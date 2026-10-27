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
	"errors"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// The bounds of a NodeBudget, shared by the CRD schema and Validate.
const (
	// MaxBudgetSignals is the most signal records an observation carries.
	MaxBudgetSignals = 8
	// MaxBudgetConditions is the most conditions a NodeBudget status carries.
	MaxBudgetConditions = 8
	// MaxClampedFields is the most entries AppliedPolicy.ClampedFields
	// lists. Each PolicyField appears at most once, and the bound leaves room
	// above the seven this release knows.
	MaxClampedFields = 8
	// MaxPolicyResourceVersionBytes is the longest policy resourceVersion an
	// observation or a desired-policy payload carries.
	MaxPolicyResourceVersionBytes = 64
)

// ErrInvalidNodeBudget is wrapped by every NodeBudget validation error.
var ErrInvalidNodeBudget = errors.New("budgetv1alpha1: invalid node budget")

// NodeBudget is the budget of one node: cluster-scoped, named for the node,
// and status only. Its status is written by the relay (from the node's
// budget.k3sm.io/status annotation) or by an own-node producer, never by an
// admin.
type NodeBudget struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Status is the node's published budget.
	Status NodeBudgetStatus `json:"status,omitempty"`
}

// NodeBudgetStatus is a NodeBudget's status: the node-produced observation,
// inline, plus the writer shape and the conditions, which belong to the
// status writer.
type NodeBudgetStatus struct {
	NodeBudgetObservation `json:",inline"`

	// Writer is how this status reaches the API server.
	Writer *BudgetWriter `json:"writer,omitempty"`
	// Conditions are the standard conditions (ConditionReporting and the
	// policy conditions), at most MaxBudgetConditions. They are owned by the
	// status writer (the relay, or an own-node producer); a node's
	// annotation payload never carries them.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodeBudgetObservation is what a node observes about itself: the body of
// the NodeBudgetReport annotation payload and the inline part of
// NodeBudgetStatus. It carries node-level data only, never a pod, namespace,
// container or UID.
type NodeBudgetObservation struct {
	// ObservedAt is when the node took the observation.
	ObservedAt metav1.Time `json:"observedAt,omitzero"`
	// ProducerGeneration is the producer's own generation, not the
	// object's metadata.generation: it bumps on a producer restart and on a
	// policy change.
	ProducerGeneration int64 `json:"producerGeneration,omitempty"`
	// Platform names the platform block the producer fills.
	Platform Platform `json:"platform,omitempty"`

	// Level is the derived pressure level, the most severe over the
	// signals.
	Level Level `json:"level,omitempty"`
	// Admit is true while every signal is below its admit-stop rung. Absent
	// reads as false.
	Admit bool `json:"admit,omitempty"`
	// AdmitReasonSignal names the signal that blocks admission. Empty while
	// Admit is true.
	AdmitReasonSignal SignalID `json:"admitReasonSignal,omitempty"`
	// EvictDeadline is when the evict-soft grace runs out: the time the soft
	// rung was first met plus its grace. Set only at level evicting.
	EvictDeadline *metav1.Time `json:"evictDeadline,omitempty"`

	// Capacity is the node's capacity and reserves.
	Capacity BudgetCapacity `json:"capacity,omitzero"`
	// Signals are the signal records, one per SignalID, at most
	// MaxBudgetSignals.
	Signals []BudgetSignal `json:"signals,omitempty"`
	// Recommendation is how much more the node can safely take.
	Recommendation BudgetRecommendation `json:"recommendation,omitzero"`
	// KillMode is the published tier-1 kill mode.
	KillMode KillMode `json:"killMode,omitzero"`
	// WouldKill is the last tier-1 decision: the band and the trigger, never
	// the victim.
	WouldKill *WouldKill `json:"wouldKill,omitempty"`
	// Counters are the per-band monotonic totals, at most one entry per
	// band.
	Counters []BandCounters `json:"counters,omitempty"`
	// Policy is the policy the node was last sent and what it did with it.
	Policy *AppliedPolicy `json:"policy,omitempty"`
	// InForce names the policy the node actually runs. It equals Policy's
	// name and resourceVersion when Policy was applied or clamped; when
	// Policy was rejected it names the last valid policy the node kept.
	// Absent while the producer's shipped defaults are in force (no policy
	// selects the node, or none was ever valid).
	InForce *PolicyRef `json:"inForce,omitempty"`

	// Darwin is the macOS platform block.
	Darwin *DarwinSignals `json:"darwin,omitempty"`
	// Linux is the Linux platform block.
	Linux *LinuxSignals `json:"linux,omitempty"`
}

// BudgetCapacity is the node's capacity and the reserves held back from
// pods. A nil field is one the producer could not sample.
type BudgetCapacity struct {
	// CPUCapacityMilli is the node's CPU capacity in millicores.
	CPUCapacityMilli *int64 `json:"cpuCapacityMilli,omitempty"`
	// CPUSystemReserveMilli is the CPU held back for the system.
	CPUSystemReserveMilli *int64 `json:"cpuSystemReserveMilli,omitempty"`
	// MemCapacityBytes is the node's physical memory.
	MemCapacityBytes *int64 `json:"memCapacityBytes,omitempty"`
	// MemSystemReserveBytes is the memory held back for the system.
	MemSystemReserveBytes *int64 `json:"memSystemReserveBytes,omitempty"`
	// MemEvictionReserveBytes is the memory held back as the eviction
	// margin.
	MemEvictionReserveBytes *int64 `json:"memEvictionReserveBytes,omitempty"`
	// MemAllocatableBytes is the memory left for pods after the reserves.
	MemAllocatableBytes *int64 `json:"memAllocatableBytes,omitempty"`
	// SwapTotalBytes is the swap in use plus the swap still free.
	SwapTotalBytes *int64 `json:"swapTotalBytes,omitempty"`
	// SwapUsedBytes is the swap in use.
	SwapUsedBytes *int64 `json:"swapUsedBytes,omitempty"`
}

// BudgetSignal is one generic signal record. Values, rungs and releases are
// in Unit, which must be the signal's canonical unit (SignalID.Unit);
// durations are whole seconds. A nil field is unset or could not be sampled.
// (The budget page carries the same durations, in the same whole seconds.)
//
// The five rungs are strictly ordered in the direction of rising pressure
// (descending for an "*.available" signal, see IsDescending), and each
// release sits on the calm side of its rung. For nodefs.available and
// pid.available the evict-hard rung is a node condition trip, not an
// eviction.
type BudgetSignal struct {
	// ID names the signal.
	ID SignalID `json:"id"`
	// Unit is the unit of Value and of every rung and release; always
	// ID.Unit().
	Unit SignalUnit `json:"unit"`
	// Value is the current reading.
	Value *int64 `json:"value,omitempty"`

	// WarnThreshold is the warn rung.
	WarnThreshold *int64 `json:"warnThreshold,omitempty"`
	// AdmitStopThreshold is the rung at which the node stops admitting work.
	AdmitStopThreshold *int64 `json:"admitStopThreshold,omitempty"`
	// CriticalThreshold is the critical rung.
	CriticalThreshold *int64 `json:"criticalThreshold,omitempty"`
	// EvictSoftThreshold is the rung that starts the evict-soft grace (tier 1).
	EvictSoftThreshold *int64 `json:"evictSoftThreshold,omitempty"`
	// EvictHardThreshold is the hard trip (tier 2).
	EvictHardThreshold *int64 `json:"evictHardThreshold,omitempty"`

	// WarnRelease is the value at which the warn rung releases.
	WarnRelease *int64 `json:"warnRelease,omitempty"`
	// AdmitStopRelease is the value at which admission resumes.
	AdmitStopRelease *int64 `json:"admitStopRelease,omitempty"`
	// CriticalRelease is the value at which the critical rung releases.
	CriticalRelease *int64 `json:"criticalRelease,omitempty"`
	// EvictSoftRelease is the value at which the evict-soft rung releases.
	EvictSoftRelease *int64 `json:"evictSoftRelease,omitempty"`
	// EvictHardRelease is the value at which the hard trip releases.
	EvictHardRelease *int64 `json:"evictHardRelease,omitempty"`

	// GraceSeconds is the evict-soft grace, in seconds.
	GraceSeconds *int64 `json:"graceSeconds,omitempty"`
	// MinReclaimBytes is the least an eviction on this signal reclaims.
	MinReclaimBytes *int64 `json:"minReclaimBytes,omitempty"`
	// WindowSeconds is the window, in seconds, a growth signal (memory.swap)
	// measures its value over.
	WindowSeconds *int64 `json:"windowSeconds,omitempty"`
	// EtaToNextRungSeconds is the estimated time to the next rung, in
	// seconds, set only while
	// the value moves toward that rung (falling, for an "*.available"
	// signal) and the fit is good.
	EtaToNextRungSeconds *int64 `json:"etaToNextRungSeconds,omitempty"`

	// Trend is the recent direction of Value itself, not of pressure: an
	// "*.available" signal under rising pressure is falling.
	Trend Trend `json:"trend,omitempty"`
	// LimitingResource is the resource that actually limits this signal.
	LimitingResource LimitingResource `json:"limitingResource,omitempty"`
	// SharedWith names a signal measuring the same volume (for example
	// imagefs.available sharing the nodefs volume).
	SharedWith SignalID `json:"sharedWith,omitempty"`
}

// rungs returns the record's rungs in ladder order.
func (s *BudgetSignal) rungs() [5]*int64 {
	return [5]*int64{s.WarnThreshold, s.AdmitStopThreshold, s.CriticalThreshold, s.EvictSoftThreshold, s.EvictHardThreshold}
}

// releases returns the record's releases in ladder order.
func (s *BudgetSignal) releases() [5]*int64 {
	return [5]*int64{s.WarnRelease, s.AdmitStopRelease, s.CriticalRelease, s.EvictSoftRelease, s.EvictHardRelease}
}

// BudgetRecommendation is how much more the node can safely take. It is a
// node-level figure; slot sizing is the consumer's.
type BudgetRecommendation struct {
	// SafeAdditionalBytes is the memory a new workload can take before the
	// first admit-stop rung, less the history margin; 0 while Admit is
	// false.
	SafeAdditionalBytes *int64 `json:"safeAdditionalBytes,omitempty"`
	// Basis names the signal SafeAdditionalBytes was computed from.
	Basis SignalID `json:"basis,omitempty"`
}

// KillMode is the published tier-1 kill mode.
type KillMode struct {
	// Mode is the mode the node runs.
	Mode KillModeName `json:"mode,omitempty"`
	// Armed is true while tier 1 may deliver.
	Armed bool `json:"armed,omitempty"`
	// ArmingEpisodes is the matched pressure episodes recorded since the
	// last reset; mode auto arms at ArmingEpisodesRequired.
	ArmingEpisodes int32 `json:"armingEpisodes,omitempty"`
	// Reason says why tier 1 is armed or not.
	Reason KillModeReason `json:"reason,omitempty"`
}

// WouldKill is the last tier-1 decision, armed or observed. It carries the
// band and the trigger only; the victim appears only in a namespaced Event.
type WouldKill struct {
	// Band is the band the decision took from (always batch).
	Band Band `json:"band"`
	// Reason is the trigger.
	Reason WouldKillReason `json:"reason"`
	// At is when the decision was made.
	At metav1.Time `json:"at"`
}

// BandCounters are one band's persisted monotonic totals, which survive pod
// death and producer restart, and its windowed counts.
type BandCounters struct {
	// Band is the band counted.
	Band Band `json:"band"`
	// EvictionsTotal counts evictions.
	EvictionsTotal int64 `json:"evictionsTotal"`
	// OOMKillsTotal counts out-of-memory kills.
	OOMKillsTotal int64 `json:"oomKillsTotal"`
	// KernelKillsTotal counts kernel kills the producer did not initiate.
	KernelKillsTotal int64 `json:"kernelKillsTotal"`
	// RestartsTotal counts container restarts.
	RestartsTotal int64 `json:"restartsTotal"`
	// MemHighEventsTotal counts memory.high events.
	MemHighEventsTotal int64 `json:"memHighEventsTotal"`
	// Tier1RefusedTotal counts refused tier-1 deliveries.
	Tier1RefusedTotal int64 `json:"tier1RefusedTotal"`
	// Windows are the windowed counts, one per window and reason.
	Windows []WindowCount `json:"windows,omitempty"`
}

// WindowCount is the count of one reason within one window.
type WindowCount struct {
	// Window is the window.
	Window CounterWindow `json:"window"`
	// Reason is what is counted.
	Reason CounterReason `json:"reason"`
	// Count is the count.
	Count int64 `json:"count"`
}

// MaxWindowCounts is the most windowed counts one band carries: one per
// window (two) and named reason (six). Unknown entries count against the same
// bound.
const MaxWindowCounts = 12

// AppliedPolicy is the NodeBudgetPolicy the node applied, clamped or
// refused. It carries no free text: the status writer composes the
// human-readable condition message from Outcome and ClampedFields.
type AppliedPolicy struct {
	// PolicyRef names the policy and the version of it the node was sent.
	PolicyRef `json:",inline"`
	// Outcome is what the node did with it.
	Outcome PolicyOutcome `json:"outcome"`
	// ClampedFields are the policy fields the node clamped, each listed
	// once.
	ClampedFields []PolicyField `json:"clampedFields,omitempty"`
}

// PolicyRef names one version of a NodeBudgetPolicy.
type PolicyRef struct {
	// Name is the policy's name.
	Name string `json:"name"`
	// ResourceVersion is the policy's resourceVersion as relayed.
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

// BudgetWriter is how a NodeBudget's status reaches the API server.
type BudgetWriter struct {
	// Shape is the writer shape.
	Shape WriterShape `json:"shape"`
	// NodeNameExtra is true when an own-node producer verified its token's
	// node-name claim at start.
	NodeNameExtra bool `json:"nodeNameExtra,omitempty"`
}

// DarwinSignals is the macOS platform block: information the generic
// records do not carry. None of it is a ladder rung or a kill trigger.
type DarwinSignals struct {
	// VMPressureLevel is the kernel's memory pressure level (1, 2 or 4).
	VMPressureLevel *int64 `json:"vmPressureLevel,omitempty"`
	// MemorystatusLevelPermille is the kernel's memorystatus level, in
	// permille.
	MemorystatusLevelPermille *int64 `json:"memorystatusLevelPermille,omitempty"`
	// Compressor is the memory compressor detail.
	Compressor *DarwinCompressor `json:"compressor,omitempty"`
	// SwapinsTotal counts pages swapped in since boot.
	SwapinsTotal *int64 `json:"swapinsTotal,omitempty"`
	// SwapoutsTotal counts pages swapped out since boot.
	SwapoutsTotal *int64 `json:"swapoutsTotal,omitempty"`
}

// DarwinCompressor is the memory compressor detail.
type DarwinCompressor struct {
	// BytesUsed is the memory the compressor occupies.
	BytesUsed *int64 `json:"bytesUsed,omitempty"`
	// BytesStored is the uncompressed size of what it holds.
	BytesStored *int64 `json:"bytesStored,omitempty"`
	// SegmentLimitBytes is the compressor's segment limit.
	SegmentLimitBytes *int64 `json:"segmentLimitBytes,omitempty"`
}

// LinuxSignals is the Linux platform block.
type LinuxSignals struct {
	// PSI is the pressure stall information.
	PSI *LinuxPSI `json:"psi,omitempty"`
	// MemoryEvents are the root cgroup's memory.events totals.
	MemoryEvents *LinuxMemoryEvents `json:"memoryEvents,omitempty"`
}

// LinuxPSI is the pressure stall information per resource.
type LinuxPSI struct {
	// CPU is /proc/pressure/cpu.
	CPU *PSIResource `json:"cpu,omitempty"`
	// Memory is /proc/pressure/memory.
	Memory *PSIResource `json:"memory,omitempty"`
	// IO is /proc/pressure/io.
	IO *PSIResource `json:"io,omitempty"`
}

// PSIResource is one resource's some and full lines.
type PSIResource struct {
	// Some is the share of time at least one task stalled.
	Some *PSILine `json:"some,omitempty"`
	// Full is the share of time every task stalled.
	Full *PSILine `json:"full,omitempty"`
}

// PSILine is one PSI line.
type PSILine struct {
	// Avg10Permille is the 10 s average, in permille.
	Avg10Permille *int64 `json:"avg10Permille,omitempty"`
	// Avg60Permille is the 60 s average, in permille.
	Avg60Permille *int64 `json:"avg60Permille,omitempty"`
	// TotalUs is the total stall time, in microseconds.
	TotalUs *int64 `json:"totalUs,omitempty"`
}

// LinuxMemoryEvents are the memory.events counters.
type LinuxMemoryEvents struct {
	// High counts memory.high breaches.
	High *int64 `json:"high,omitempty"`
	// Max counts memory.max hits.
	Max *int64 `json:"max,omitempty"`
	// OOM counts out-of-memory events.
	OOM *int64 `json:"oom,omitempty"`
	// OOMKill counts processes killed by the OOM killer.
	OOMKill *int64 `json:"oomKill,omitempty"`
}

// NodeBudgetList is a list of NodeBudget objects.
type NodeBudgetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the NodeBudget objects.
	Items []NodeBudget `json:"items"`
}

// Validate reports whether a reporting observation is well-formed. It
// requires ObservedAt, a known Platform and Level, and exactly one platform
// block, the one Platform names; every enum value known; every signal known
// and listed once, with its ladder in order (see BudgetSignal); no negative
// size, duration or counter; and the bounds above. Errors wrap
// ErrInvalidNodeBudget.
func (o *NodeBudgetObservation) Validate() error {
	if err := o.validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNodeBudget, err)
	}
	return nil
}

// Validate reports whether the status is well-formed: a status with no
// observation (a node that has not reported) needs only a valid writer and
// conditions; any other status must also pass NodeBudgetObservation.Validate.
// Errors wrap ErrInvalidNodeBudget.
func (s *NodeBudgetStatus) Validate() error {
	if s.Writer != nil && !s.Writer.Shape.Valid() {
		return fmt.Errorf("%w: writer.shape %q is not a known writer shape", ErrInvalidNodeBudget, s.Writer.Shape)
	}
	if n := len(s.Conditions); n > MaxBudgetConditions {
		return fmt.Errorf("%w: %d conditions, at most %d", ErrInvalidNodeBudget, n, MaxBudgetConditions)
	}
	for i, c := range s.Conditions {
		if !oneOf(c.Type, ConditionReporting, ConditionPolicyRejected, ConditionPolicyClamped, ConditionPolicyConflict) {
			continue
		}
		reasons := ConditionReasons(c.Type, string(c.Status))
		if reasons == nil {
			return fmt.Errorf("%w: conditions[%d] %s has status %q, want True or False (Unknown is not used)", ErrInvalidNodeBudget, i, c.Type, c.Status)
		}
		if !oneOf(c.Reason, reasons...) {
			return fmt.Errorf("%w: conditions[%d] %s=%s reason %q, want one of %v", ErrInvalidNodeBudget, i, c.Type, c.Status, c.Reason, reasons)
		}
	}
	if reflect.ValueOf(s.NodeBudgetObservation).IsZero() {
		return nil
	}
	return s.NodeBudgetObservation.Validate()
}

func (o *NodeBudgetObservation) validate() error {
	if o.ObservedAt.IsZero() {
		return errors.New("observedAt is not set")
	}
	if o.ProducerGeneration < 0 {
		return fmt.Errorf("producerGeneration %d is negative", o.ProducerGeneration)
	}
	if !o.Platform.Valid() {
		return fmt.Errorf("platform %q is not a known platform", o.Platform)
	}
	if (o.Darwin != nil) == (o.Linux != nil) {
		return errors.New("exactly one of darwin and linux must be set")
	}
	if (o.Platform == PlatformDarwin) != (o.Darwin != nil) {
		return fmt.Errorf("platform %q does not match the platform block set", o.Platform)
	}
	if !o.Level.Valid() {
		return fmt.Errorf("level %q is not a known level", o.Level)
	}
	if o.AdmitReasonSignal != "" {
		if !o.AdmitReasonSignal.Valid() {
			return fmt.Errorf("admitReasonSignal %q is not a known signal", o.AdmitReasonSignal)
		}
		if o.Admit {
			return errors.New("admitReasonSignal is set while admit is true")
		}
	}
	if o.EvictDeadline != nil && o.EvictDeadline.IsZero() {
		return errors.New("evictDeadline is set but zero")
	}
	if err := o.Capacity.validate(); err != nil {
		return err
	}
	if n := len(o.Signals); n > MaxBudgetSignals {
		return fmt.Errorf("%d signals, at most %d", n, MaxBudgetSignals)
	}
	seen := make(map[SignalID]bool, len(o.Signals))
	for i := range o.Signals {
		s := &o.Signals[i]
		if err := s.validate(); err != nil {
			return fmt.Errorf("signals[%d]: %w", i, err)
		}
		if seen[s.ID] {
			return fmt.Errorf("signals[%d]: %s is repeated", i, s.ID)
		}
		seen[s.ID] = true
	}
	if err := nonNegative("recommendation.safeAdditionalBytes", o.Recommendation.SafeAdditionalBytes); err != nil {
		return err
	}
	if b := o.Recommendation.Basis; b != "" && !b.Valid() {
		return fmt.Errorf("recommendation.basis %q is not a known signal", b)
	}
	if err := o.KillMode.validate(); err != nil {
		return err
	}
	if w := o.WouldKill; w != nil {
		if !w.Band.Valid() {
			return fmt.Errorf("wouldKill.band %q is not a known band", w.Band)
		}
		if !w.Reason.Valid() {
			return fmt.Errorf("wouldKill.reason %q is not a known reason", w.Reason)
		}
		if w.At.IsZero() {
			return errors.New("wouldKill.at is not set")
		}
	}
	if n := len(o.Counters); n > len(Bands()) {
		return fmt.Errorf("%d counter entries, at most %d", n, len(Bands()))
	}
	bands := make(map[Band]bool, len(o.Counters))
	for i := range o.Counters {
		c := &o.Counters[i]
		if err := c.validate(); err != nil {
			return fmt.Errorf("counters[%d]: %w", i, err)
		}
		if bands[c.Band] {
			return fmt.Errorf("counters[%d]: band %s is repeated", i, c.Band)
		}
		bands[c.Band] = true
	}
	if p := o.Policy; p != nil {
		if err := p.validate(); err != nil {
			return fmt.Errorf("policy: %w", err)
		}
	}
	if r := o.InForce; r != nil {
		if !isObjectName(r.Name) {
			return fmt.Errorf("inForce.name %q is not an object name", r.Name)
		}
		if r.ResourceVersion != "" && !isPrintableASCII(r.ResourceVersion, MaxPolicyResourceVersionBytes) {
			return fmt.Errorf("inForce.resourceVersion is not 1..%d printable ASCII bytes", MaxPolicyResourceVersionBytes)
		}
	}
	if p := o.Policy; p != nil && p.Outcome != PolicyRejected {
		if r := o.InForce; r == nil || r.Name != p.Name || r.ResourceVersion != p.ResourceVersion {
			return fmt.Errorf("policy %s was %s, so inForce must name the same name and resourceVersion", p.Name, p.Outcome)
		}
	}
	if o.Darwin != nil {
		if err := o.Darwin.validate(); err != nil {
			return fmt.Errorf("darwin: %w", err)
		}
	}
	if o.Linux != nil {
		if err := o.Linux.validate(); err != nil {
			return fmt.Errorf("linux: %w", err)
		}
	}
	return nil
}

func (c *BudgetCapacity) validate() error {
	for _, f := range []struct {
		name string
		v    *int64
	}{
		{"cpuCapacityMilli", c.CPUCapacityMilli},
		{"cpuSystemReserveMilli", c.CPUSystemReserveMilli},
		{"memCapacityBytes", c.MemCapacityBytes},
		{"memSystemReserveBytes", c.MemSystemReserveBytes},
		{"memEvictionReserveBytes", c.MemEvictionReserveBytes},
		{"memAllocatableBytes", c.MemAllocatableBytes},
		{"swapTotalBytes", c.SwapTotalBytes},
		{"swapUsedBytes", c.SwapUsedBytes},
	} {
		if err := nonNegative("capacity."+f.name, f.v); err != nil {
			return err
		}
	}
	return nil
}

func (s *BudgetSignal) validate() error {
	if !s.ID.Valid() {
		return fmt.Errorf("id %q is not a known signal", s.ID)
	}
	if s.Unit != s.ID.Unit() {
		return fmt.Errorf("%s: unit %q, want %q", s.ID, s.Unit, s.ID.Unit())
	}
	for _, f := range []struct {
		name string
		v    *int64
	}{
		{"value", s.Value},
		{"graceSeconds", s.GraceSeconds},
		{"minReclaimBytes", s.MinReclaimBytes},
		{"windowSeconds", s.WindowSeconds},
		{"etaToNextRungSeconds", s.EtaToNextRungSeconds},
	} {
		if err := nonNegative(f.name, f.v); err != nil {
			return fmt.Errorf("%s: %w", s.ID, err)
		}
	}
	if err := checkLadder(s.ID, s.rungs(), s.releases()); err != nil {
		return fmt.Errorf("%s: %w", s.ID, err)
	}
	if s.Trend != "" && !s.Trend.Valid() {
		return fmt.Errorf("%s: trend %q is not a known trend", s.ID, s.Trend)
	}
	if s.LimitingResource != "" && !s.LimitingResource.Valid() {
		return fmt.Errorf("%s: limitingResource %q is not a known resource", s.ID, s.LimitingResource)
	}
	if s.SharedWith != "" && (!s.SharedWith.Valid() || s.SharedWith == s.ID) {
		return fmt.Errorf("%s: sharedWith %q is not another known signal", s.ID, s.SharedWith)
	}
	return nil
}

func (k *KillMode) validate() error {
	if *k == (KillMode{}) {
		return nil
	}
	if !k.Mode.Valid() {
		return fmt.Errorf("killMode.mode %q is not a known mode", k.Mode)
	}
	if k.ArmingEpisodes < 0 {
		return fmt.Errorf("killMode.armingEpisodes %d is negative", k.ArmingEpisodes)
	}
	if k.Reason != "" && !k.Reason.Valid() {
		return fmt.Errorf("killMode.reason %q is not a known reason", k.Reason)
	}
	return nil
}

func (c *BandCounters) validate() error {
	if !c.Band.Valid() {
		return fmt.Errorf("band %q is not a known band", c.Band)
	}
	for _, f := range []struct {
		name string
		v    int64
	}{
		{"evictionsTotal", c.EvictionsTotal},
		{"oomKillsTotal", c.OOMKillsTotal},
		{"kernelKillsTotal", c.KernelKillsTotal},
		{"restartsTotal", c.RestartsTotal},
		{"memHighEventsTotal", c.MemHighEventsTotal},
		{"tier1RefusedTotal", c.Tier1RefusedTotal},
	} {
		if f.v < 0 {
			return fmt.Errorf("%s %s %d is negative", c.Band, f.name, f.v)
		}
	}
	if n := len(c.Windows); n > MaxWindowCounts {
		return fmt.Errorf("%s: %d windowed counts, at most %d", c.Band, n, MaxWindowCounts)
	}
	seen := make(map[WindowCount]bool, len(c.Windows))
	for i, w := range c.Windows {
		if !w.Window.Valid() || !w.Reason.Valid() {
			return fmt.Errorf("%s windows[%d]: unknown window %q or reason %q", c.Band, i, w.Window, w.Reason)
		}
		if w.Count < 0 {
			return fmt.Errorf("%s windows[%d]: count %d is negative", c.Band, i, w.Count)
		}
		key := WindowCount{Window: w.Window, Reason: w.Reason}
		if seen[key] {
			return fmt.Errorf("%s windows[%d]: %s %s is repeated", c.Band, i, w.Window, w.Reason)
		}
		seen[key] = true
	}
	return nil
}

func (p *AppliedPolicy) validate() error {
	if !isObjectName(p.Name) {
		return fmt.Errorf("name %q is not an object name", p.Name)
	}
	if p.ResourceVersion != "" && !isPrintableASCII(p.ResourceVersion, MaxPolicyResourceVersionBytes) {
		return fmt.Errorf("resourceVersion is not 1..%d printable ASCII bytes", MaxPolicyResourceVersionBytes)
	}
	if !p.Outcome.Valid() {
		return fmt.Errorf("outcome %q is not a known outcome", p.Outcome)
	}
	if n := len(p.ClampedFields); n > MaxClampedFields {
		return fmt.Errorf("%d clampedFields, at most %d", n, MaxClampedFields)
	}
	seen := make(map[PolicyField]bool, len(p.ClampedFields))
	for i, f := range p.ClampedFields {
		if !f.Valid() {
			return fmt.Errorf("clampedFields[%d] %q is not a known policy field", i, f)
		}
		if seen[f] {
			return fmt.Errorf("clampedFields[%d] %q is repeated", i, f)
		}
		seen[f] = true
	}
	return nil
}

func (d *DarwinSignals) validate() error {
	vals := []struct {
		name string
		v    *int64
	}{
		{"vmPressureLevel", d.VMPressureLevel},
		{"memorystatusLevelPermille", d.MemorystatusLevelPermille},
		{"swapinsTotal", d.SwapinsTotal},
		{"swapoutsTotal", d.SwapoutsTotal},
	}
	if c := d.Compressor; c != nil {
		vals = append(vals, []struct {
			name string
			v    *int64
		}{
			{"compressor.bytesUsed", c.BytesUsed},
			{"compressor.bytesStored", c.BytesStored},
			{"compressor.segmentLimitBytes", c.SegmentLimitBytes},
		}...)
	}
	for _, f := range vals {
		if err := nonNegative(f.name, f.v); err != nil {
			return err
		}
	}
	return permille("memorystatusLevelPermille", d.MemorystatusLevelPermille)
}

func (l *LinuxSignals) validate() error {
	if p := l.PSI; p != nil {
		for _, r := range []struct {
			name string
			v    *PSIResource
		}{{"cpu", p.CPU}, {"memory", p.Memory}, {"io", p.IO}} {
			if r.v == nil {
				continue
			}
			for _, ln := range []struct {
				name string
				v    *PSILine
			}{{"some", r.v.Some}, {"full", r.v.Full}} {
				if ln.v == nil {
					continue
				}
				path := "psi." + r.name + "." + ln.name
				for _, f := range []struct {
					name string
					v    *int64
				}{
					{"avg10Permille", ln.v.Avg10Permille},
					{"avg60Permille", ln.v.Avg60Permille},
				} {
					if err := permille(path+"."+f.name, f.v); err != nil {
						return err
					}
				}
				if err := nonNegative(path+".totalUs", ln.v.TotalUs); err != nil {
					return err
				}
			}
		}
	}
	if e := l.MemoryEvents; e != nil {
		for _, f := range []struct {
			name string
			v    *int64
		}{{"high", e.High}, {"max", e.Max}, {"oom", e.OOM}, {"oomKill", e.OOMKill}} {
			if err := nonNegative("memoryEvents."+f.name, f.v); err != nil {
				return err
			}
		}
	}
	return nil
}

// DeepCopy* below are hand-written: this module runs no deepcopy-gen. Every
// pointer and slice is copied, or an informer's cached object and a caller's
// mutation would share memory.

// DeepCopyInto copies the receiver into out.
func (in *BudgetCapacity) DeepCopyInto(out *BudgetCapacity) {
	*out = BudgetCapacity{
		CPUCapacityMilli:        clonePtr(in.CPUCapacityMilli),
		CPUSystemReserveMilli:   clonePtr(in.CPUSystemReserveMilli),
		MemCapacityBytes:        clonePtr(in.MemCapacityBytes),
		MemSystemReserveBytes:   clonePtr(in.MemSystemReserveBytes),
		MemEvictionReserveBytes: clonePtr(in.MemEvictionReserveBytes),
		MemAllocatableBytes:     clonePtr(in.MemAllocatableBytes),
		SwapTotalBytes:          clonePtr(in.SwapTotalBytes),
		SwapUsedBytes:           clonePtr(in.SwapUsedBytes),
	}
}

// DeepCopyInto copies the receiver into out.
func (in *BudgetSignal) DeepCopyInto(out *BudgetSignal) {
	*out = *in
	out.Value = clonePtr(in.Value)
	out.WarnThreshold, out.AdmitStopThreshold, out.CriticalThreshold = clonePtr(in.WarnThreshold), clonePtr(in.AdmitStopThreshold), clonePtr(in.CriticalThreshold)
	out.EvictSoftThreshold, out.EvictHardThreshold = clonePtr(in.EvictSoftThreshold), clonePtr(in.EvictHardThreshold)
	out.WarnRelease, out.AdmitStopRelease = clonePtr(in.WarnRelease), clonePtr(in.AdmitStopRelease)
	out.CriticalRelease, out.EvictSoftRelease = clonePtr(in.CriticalRelease), clonePtr(in.EvictSoftRelease)
	out.EvictHardRelease = clonePtr(in.EvictHardRelease)
	out.GraceSeconds, out.MinReclaimBytes = clonePtr(in.GraceSeconds), clonePtr(in.MinReclaimBytes)
	out.WindowSeconds, out.EtaToNextRungSeconds = clonePtr(in.WindowSeconds), clonePtr(in.EtaToNextRungSeconds)
}

// DeepCopyInto copies the receiver into out.
func (in *BandCounters) DeepCopyInto(out *BandCounters) {
	*out = *in
	out.Windows = cloneSlice(in.Windows)
}

// DeepCopyInto copies the receiver into out.
func (in *DarwinSignals) DeepCopyInto(out *DarwinSignals) {
	*out = DarwinSignals{
		VMPressureLevel:           clonePtr(in.VMPressureLevel),
		MemorystatusLevelPermille: clonePtr(in.MemorystatusLevelPermille),
		SwapinsTotal:              clonePtr(in.SwapinsTotal),
		SwapoutsTotal:             clonePtr(in.SwapoutsTotal),
	}
	if c := in.Compressor; c != nil {
		out.Compressor = &DarwinCompressor{
			BytesUsed:         clonePtr(c.BytesUsed),
			BytesStored:       clonePtr(c.BytesStored),
			SegmentLimitBytes: clonePtr(c.SegmentLimitBytes),
		}
	}
}

// DeepCopyInto copies the receiver into out.
func (in *LinuxSignals) DeepCopyInto(out *LinuxSignals) {
	*out = LinuxSignals{}
	if p := in.PSI; p != nil {
		out.PSI = &LinuxPSI{CPU: p.CPU.deepCopy(), Memory: p.Memory.deepCopy(), IO: p.IO.deepCopy()}
	}
	if e := in.MemoryEvents; e != nil {
		out.MemoryEvents = &LinuxMemoryEvents{
			High: clonePtr(e.High), Max: clonePtr(e.Max), OOM: clonePtr(e.OOM), OOMKill: clonePtr(e.OOMKill),
		}
	}
}

func (in *PSIResource) deepCopy() *PSIResource {
	if in == nil {
		return nil
	}
	return &PSIResource{Some: in.Some.deepCopy(), Full: in.Full.deepCopy()}
}

func (in *PSILine) deepCopy() *PSILine {
	if in == nil {
		return nil
	}
	return &PSILine{
		Avg10Permille: clonePtr(in.Avg10Permille),
		Avg60Permille: clonePtr(in.Avg60Permille),
		TotalUs:       clonePtr(in.TotalUs),
	}
}

// DeepCopyInto copies the receiver into out.
func (in *NodeBudgetObservation) DeepCopyInto(out *NodeBudgetObservation) {
	*out = *in
	in.ObservedAt.DeepCopyInto(&out.ObservedAt)
	if in.EvictDeadline != nil {
		out.EvictDeadline = in.EvictDeadline.DeepCopy()
	}
	in.Capacity.DeepCopyInto(&out.Capacity)
	if in.Signals != nil {
		out.Signals = make([]BudgetSignal, len(in.Signals))
		for i := range in.Signals {
			in.Signals[i].DeepCopyInto(&out.Signals[i])
		}
	}
	out.Recommendation.SafeAdditionalBytes = clonePtr(in.Recommendation.SafeAdditionalBytes)
	if in.WouldKill != nil {
		w := *in.WouldKill
		in.WouldKill.At.DeepCopyInto(&w.At)
		out.WouldKill = &w
	}
	if in.Counters != nil {
		out.Counters = make([]BandCounters, len(in.Counters))
		for i := range in.Counters {
			in.Counters[i].DeepCopyInto(&out.Counters[i])
		}
	}
	if in.Policy != nil {
		p := *in.Policy
		p.ClampedFields = cloneSlice(in.Policy.ClampedFields)
		out.Policy = &p
	}
	out.InForce = clonePtr(in.InForce)
	if in.Darwin != nil {
		out.Darwin = new(DarwinSignals)
		in.Darwin.DeepCopyInto(out.Darwin)
	}
	if in.Linux != nil {
		out.Linux = new(LinuxSignals)
		in.Linux.DeepCopyInto(out.Linux)
	}
}

// DeepCopy returns a deep copy of the observation.
func (in *NodeBudgetObservation) DeepCopy() *NodeBudgetObservation {
	if in == nil {
		return nil
	}
	out := new(NodeBudgetObservation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *NodeBudgetStatus) DeepCopyInto(out *NodeBudgetStatus) {
	*out = *in
	in.NodeBudgetObservation.DeepCopyInto(&out.NodeBudgetObservation)
	out.Writer = clonePtr(in.Writer)
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy returns a deep copy of the status.
func (in *NodeBudgetStatus) DeepCopy() *NodeBudgetStatus {
	if in == nil {
		return nil
	}
	out := new(NodeBudgetStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *NodeBudget) DeepCopyInto(out *NodeBudget) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the NodeBudget.
func (in *NodeBudget) DeepCopy() *NodeBudget {
	if in == nil {
		return nil
	}
	out := new(NodeBudget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *NodeBudget) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *NodeBudgetList) DeepCopyInto(out *NodeBudgetList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NodeBudget, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *NodeBudgetList) DeepCopy() *NodeBudgetList {
	if in == nil {
		return nil
	}
	out := new(NodeBudgetList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *NodeBudgetList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
