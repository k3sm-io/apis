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
	// MaxClampedFields is the most field paths AppliedPolicy.ClampedFields
	// lists.
	MaxClampedFields = 8
	// MaxClampedFieldBytes is the longest field path in ClampedFields.
	MaxClampedFieldBytes = 64
	// MaxPolicyResourceVersionBytes is the longest policy resourceVersion an
	// observation or a desired-policy payload carries.
	MaxPolicyResourceVersionBytes = 64
	// MaxPolicyMessageBytes is the longest AppliedPolicy.Message.
	MaxPolicyMessageBytes = 256
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
	// policy conditions), at most MaxBudgetConditions.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodeBudgetObservation is what a node observes about itself: the body of
// the NodeBudgetReport annotation payload and the inline part of
// NodeBudgetStatus. It carries node-level data only, never a pod, namespace,
// container or UID.
type NodeBudgetObservation struct {
	// ObservedAt is when the node took the observation.
	ObservedAt metav1.Time `json:"observedAt,omitzero"`
	// Generation bumps on a producer restart and on a policy change.
	Generation int64 `json:"generation,omitempty"`
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
	// EvictDeadlineNs is the wall-clock time, in Unix nanoseconds, at which
	// the evict-soft grace runs out: the time the soft rung was first met
	// plus its grace. Set only at level evicting.
	EvictDeadlineNs *int64 `json:"evictDeadlineNs,omitempty"`

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
	// Policy is the policy the node applied, or refused.
	Policy *AppliedPolicy `json:"policy,omitempty"`

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
// in Unit; durations are nanoseconds. A nil field is unset or could not be
// sampled.
//
// The five rungs are strictly ordered in the direction of rising pressure
// (descending for an "*.available" signal, see IsDescending), and each
// release sits on the calm side of its rung. For nodefs.available and
// pid.available the evict-hard rung is a node condition trip, not an
// eviction.
type BudgetSignal struct {
	// ID names the signal.
	ID SignalID `json:"id"`
	// Unit is the unit of Value and of every rung and release.
	Unit SignalUnit `json:"unit"`
	// Value is the current reading.
	Value *int64 `json:"value,omitempty"`

	// WarnAt is the warn rung.
	WarnAt *int64 `json:"warnAt,omitempty"`
	// AdmitStopAt is the rung at which the node stops admitting work.
	AdmitStopAt *int64 `json:"admitStopAt,omitempty"`
	// CriticalAt is the critical rung.
	CriticalAt *int64 `json:"criticalAt,omitempty"`
	// EvictSoftAt is the rung that starts the evict-soft grace (tier 1).
	EvictSoftAt *int64 `json:"evictSoftAt,omitempty"`
	// EvictHardAt is the hard trip (tier 2).
	EvictHardAt *int64 `json:"evictHardAt,omitempty"`

	// WarnReleaseAt is the value at which the warn rung releases.
	WarnReleaseAt *int64 `json:"warnReleaseAt,omitempty"`
	// AdmitStopReleaseAt is the value at which admission resumes.
	AdmitStopReleaseAt *int64 `json:"admitStopReleaseAt,omitempty"`
	// CriticalReleaseAt is the value at which the critical rung releases.
	CriticalReleaseAt *int64 `json:"criticalReleaseAt,omitempty"`
	// EvictSoftReleaseAt is the value at which the evict-soft rung releases.
	EvictSoftReleaseAt *int64 `json:"evictSoftReleaseAt,omitempty"`
	// EvictHardReleaseAt is the value at which the hard trip releases.
	EvictHardReleaseAt *int64 `json:"evictHardReleaseAt,omitempty"`

	// GraceNs is the evict-soft grace.
	GraceNs *int64 `json:"graceNs,omitempty"`
	// MinReclaimBytes is the least an eviction on this signal reclaims.
	MinReclaimBytes *int64 `json:"minReclaimBytes,omitempty"`
	// WindowNs is the window a growth signal (memory.swap) measures its
	// value over.
	WindowNs *int64 `json:"windowNs,omitempty"`
	// EtaToNextRungNs is the estimated time to the next rung, set only while
	// the value moves toward that rung (falling, for an "*.available"
	// signal) and the fit is good.
	EtaToNextRungNs *int64 `json:"etaToNextRungNs,omitempty"`

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
	return [5]*int64{s.WarnAt, s.AdmitStopAt, s.CriticalAt, s.EvictSoftAt, s.EvictHardAt}
}

// releases returns the record's releases in ladder order.
func (s *BudgetSignal) releases() [5]*int64 {
	return [5]*int64{s.WarnReleaseAt, s.AdmitStopReleaseAt, s.CriticalReleaseAt, s.EvictSoftReleaseAt, s.EvictHardReleaseAt}
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
// window and reason.
const MaxWindowCounts = 12

// AppliedPolicy is the NodeBudgetPolicy the node applied, clamped or
// refused.
type AppliedPolicy struct {
	// Name is the policy's name.
	Name string `json:"name"`
	// ResourceVersion is the policy's resourceVersion as relayed.
	ResourceVersion string `json:"resourceVersion,omitempty"`
	// Outcome is what the node did with it.
	Outcome PolicyOutcome `json:"outcome"`
	// ClampedFields are the policy field paths raised to a floor.
	ClampedFields []string `json:"clampedFields,omitempty"`
	// Message is a short human-readable detail, for example why a policy was
	// refused. Printable ASCII, at most MaxPolicyMessageBytes.
	Message string `json:"message,omitempty"`
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
	if reflect.ValueOf(s.NodeBudgetObservation).IsZero() {
		return nil
	}
	return s.NodeBudgetObservation.Validate()
}

func (o *NodeBudgetObservation) validate() error {
	if o.ObservedAt.IsZero() {
		return errors.New("observedAt is not set")
	}
	if o.Generation < 0 {
		return fmt.Errorf("generation %d is negative", o.Generation)
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
	if err := nonNegative("evictDeadlineNs", o.EvictDeadlineNs); err != nil {
		return err
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
	if !s.Unit.Valid() {
		return fmt.Errorf("%s: unit %q is not a known unit", s.ID, s.Unit)
	}
	for _, f := range []struct {
		name string
		v    *int64
	}{
		{"value", s.Value},
		{"graceNs", s.GraceNs},
		{"minReclaimBytes", s.MinReclaimBytes},
		{"windowNs", s.WindowNs},
		{"etaToNextRungNs", s.EtaToNextRungNs},
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
	for i, f := range p.ClampedFields {
		if !fieldPathRE.MatchString(f) || len(f) > MaxClampedFieldBytes {
			return fmt.Errorf("clampedFields[%d] %q is not a field path", i, f)
		}
	}
	if p.Message != "" && !isPrintableASCII(p.Message, MaxPolicyMessageBytes) {
		return fmt.Errorf("message is not 1..%d printable ASCII bytes", MaxPolicyMessageBytes)
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
	out.WarnAt, out.AdmitStopAt, out.CriticalAt = clonePtr(in.WarnAt), clonePtr(in.AdmitStopAt), clonePtr(in.CriticalAt)
	out.EvictSoftAt, out.EvictHardAt = clonePtr(in.EvictSoftAt), clonePtr(in.EvictHardAt)
	out.WarnReleaseAt, out.AdmitStopReleaseAt = clonePtr(in.WarnReleaseAt), clonePtr(in.AdmitStopReleaseAt)
	out.CriticalReleaseAt, out.EvictSoftReleaseAt = clonePtr(in.CriticalReleaseAt), clonePtr(in.EvictSoftReleaseAt)
	out.EvictHardReleaseAt = clonePtr(in.EvictHardReleaseAt)
	out.GraceNs, out.MinReclaimBytes = clonePtr(in.GraceNs), clonePtr(in.MinReclaimBytes)
	out.WindowNs, out.EtaToNextRungNs = clonePtr(in.WindowNs), clonePtr(in.EtaToNextRungNs)
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
	out.EvictDeadlineNs = clonePtr(in.EvictDeadlineNs)
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
