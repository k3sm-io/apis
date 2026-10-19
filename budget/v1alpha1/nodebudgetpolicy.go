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
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// The bounds of a NodeBudgetPolicy, shared by the CRD schema and Validate.
const (
	// MaxPolicyThresholds is the most threshold entries a policy carries.
	MaxPolicyThresholds = 8
	// MaxPolicyNodeSelectorLabels is the most labels a policy's nodeSelector
	// carries.
	MaxPolicyNodeSelectorLabels = 32
)

// ErrInvalidNodeBudgetPolicy is wrapped by every NodeBudgetPolicy validation
// error. A node that receives such a policy keeps its last valid one and
// reports ConditionPolicyRejected.
var ErrInvalidNodeBudgetPolicy = errors.New("budgetv1alpha1: invalid node budget policy")

// NodeBudgetPolicy is the one configuration object of the budget contract:
// cluster-scoped, spec only, written by a cluster admin. It selects node
// classes by Spec.NodeSelector and overrides the producer's defaults for
// them.
type NodeBudgetPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the policy.
	Spec NodeBudgetPolicySpec `json:"spec"`
}

// NodeBudgetPolicySpec is a policy. Every field is optional; an unset field
// keeps the producer's shipped default. The ladder fields follow the rules of
// BudgetSignal; the tier-1 timers are bounded by the floors in floors.go.
type NodeBudgetPolicySpec struct {
	// NodeSelector selects the nodes the policy applies to by exact label
	// match; empty selects every node.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Priority orders policies that select the same node: the highest wins,
	// and a tie goes to the lexically smallest name (SelectPolicy).
	Priority int32 `json:"priority,omitempty"`
	// Thresholds override the ladder of individual signals, one entry per
	// signal.
	Thresholds []SignalThresholds `json:"thresholds,omitempty"`
	// Reserves override the reserves held back from pods. A reserve above
	// the measured capacity is clamped by the node.
	Reserves *BudgetReserves `json:"reserves,omitempty"`
	// History overrides the recommendation's peak history.
	History *BudgetHistory `json:"history,omitempty"`
	// Mode is the tier-1 kill mode. Empty means KillModeAuto (the CRD
	// defaults it).
	Mode KillModeName `json:"mode,omitempty"`
	// Tier1GraceSeconds is the SIGTERM grace of a tier-1 eviction, at least
	// MinTier1GraceSeconds.
	Tier1GraceSeconds *int32 `json:"tier1GraceSeconds,omitempty"`
	// Tier1HeldSeconds is how long the held-pressure condition must hold, at
	// least MinTier1HeldSeconds (the CRD defaults it to
	// DefaultTier1HeldSeconds).
	Tier1HeldSeconds *int32 `json:"tier1HeldSeconds,omitempty"`
	// Tier1MaxDeliveriesPerHour caps tier-1 deliveries per node per hour,
	// from MinTier1DeliveriesPerHour to MaxTier1DeliveriesPerHour.
	Tier1MaxDeliveriesPerHour *int32 `json:"tier1MaxDeliveriesPerHour,omitempty"`
	// Tier1CooloffSeconds is the least time between two tier-1 deliveries on
	// one node, at least MinTier1CooloffSeconds (the CRD defaults it to
	// DefaultTier1CooloffSeconds).
	Tier1CooloffSeconds *int32 `json:"tier1CooloffSeconds,omitempty"`
	// PSIHeldPermille is the PSI memory level, in permille, that counts as
	// held pressure where PSI exists.
	PSIHeldPermille *int32 `json:"psiHeldPermille,omitempty"`
}

// SignalThresholds overrides one signal's ladder. Rungs and releases are in
// the unit the node publishes for the signal (BudgetSignal.Unit).
type SignalThresholds struct {
	// Signal names the signal.
	Signal SignalID `json:"signal"`

	// WarnAt is the warn rung.
	WarnAt *int64 `json:"warnAt,omitempty"`
	// AdmitStopAt is the admit-stop rung.
	AdmitStopAt *int64 `json:"admitStopAt,omitempty"`
	// CriticalAt is the critical rung.
	CriticalAt *int64 `json:"criticalAt,omitempty"`
	// EvictSoftAt is the evict-soft rung (not for memory.swap).
	EvictSoftAt *int64 `json:"evictSoftAt,omitempty"`
	// EvictHardAt is the hard trip.
	EvictHardAt *int64 `json:"evictHardAt,omitempty"`

	// WarnReleaseAt is the warn release.
	WarnReleaseAt *int64 `json:"warnReleaseAt,omitempty"`
	// AdmitStopReleaseAt is the admit-stop release.
	AdmitStopReleaseAt *int64 `json:"admitStopReleaseAt,omitempty"`
	// CriticalReleaseAt is the critical release.
	CriticalReleaseAt *int64 `json:"criticalReleaseAt,omitempty"`
	// EvictSoftReleaseAt is the evict-soft release.
	EvictSoftReleaseAt *int64 `json:"evictSoftReleaseAt,omitempty"`
	// EvictHardReleaseAt is the hard-trip release.
	EvictHardReleaseAt *int64 `json:"evictHardReleaseAt,omitempty"`

	// GraceSeconds is the evict-soft grace.
	GraceSeconds *int32 `json:"graceSeconds,omitempty"`
	// MinReclaimBytes is the least an eviction on this signal reclaims.
	MinReclaimBytes *int64 `json:"minReclaimBytes,omitempty"`
	// DwellSeconds is how long a level must hold before it is published.
	// Optional, with no default: unset keeps the producer's behaviour.
	DwellSeconds *int32 `json:"dwellSeconds,omitempty"`
	// WindowSeconds is the window of a growth signal (memory.swap).
	WindowSeconds *int32 `json:"windowSeconds,omitempty"`
}

// BudgetReserves override the reserves held back from pods.
type BudgetReserves struct {
	// CPUSystemReserveMilli is the CPU held back for the system.
	CPUSystemReserveMilli *int64 `json:"cpuSystemReserveMilli,omitempty"`
	// MemSystemReserveBytes is the memory held back for the system.
	MemSystemReserveBytes *int64 `json:"memSystemReserveBytes,omitempty"`
	// MemEvictionReserveBytes is the memory held back as the eviction
	// margin.
	MemEvictionReserveBytes *int64 `json:"memEvictionReserveBytes,omitempty"`
}

// BudgetHistory overrides the recommendation's decayed peak history.
type BudgetHistory struct {
	// MemoryHalfLifeSeconds is the memory history half-life (the CRD
	// defaults it to DefaultMemoryHalfLifeSeconds).
	MemoryHalfLifeSeconds *int32 `json:"memoryHalfLifeSeconds,omitempty"`
	// CPUHalfLifeSeconds is the CPU history half-life (the CRD defaults it
	// to DefaultCPUHalfLifeSeconds).
	CPUHalfLifeSeconds *int32 `json:"cpuHalfLifeSeconds,omitempty"`
	// MarginPermille is the margin taken off the recommendation (the CRD
	// defaults it to DefaultMarginPermille).
	MarginPermille *int32 `json:"marginPermille,omitempty"`
}

// NodeBudgetPolicyList is a list of NodeBudgetPolicy objects.
type NodeBudgetPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the NodeBudgetPolicy objects.
	Items []NodeBudgetPolicy `json:"items"`
}

// rungs returns the entry's rungs in ladder order.
func (t *SignalThresholds) rungs() [5]*int64 {
	return [5]*int64{t.WarnAt, t.AdmitStopAt, t.CriticalAt, t.EvictSoftAt, t.EvictHardAt}
}

// releases returns the entry's releases in ladder order.
func (t *SignalThresholds) releases() [5]*int64 {
	return [5]*int64{t.WarnReleaseAt, t.AdmitStopReleaseAt, t.CriticalReleaseAt, t.EvictSoftReleaseAt, t.EvictHardReleaseAt}
}

// rungNames and releaseNames are the JSON names of the ladder fields, in
// ladder order.
var (
	rungNames    = [5]string{"warnAt", "admitStopAt", "criticalAt", "evictSoftAt", "evictHardAt"}
	releaseNames = [5]string{"warnReleaseAt", "admitStopReleaseAt", "criticalReleaseAt", "evictSoftReleaseAt", "evictHardReleaseAt"}
)

// checkLadder applies the ladder rules to one signal: every pair of set
// rungs strictly ordered in the direction of rising pressure, every set
// release strictly on the calm side of its set rung, and no evict-soft rung
// on memory.swap. It is the Go copy of the CEL rules on the CRD's thresholds
// items, and it checks every rung and release for a negative value, the
// schema's minimum.
func checkLadder(sig SignalID, rungs, releases [5]*int64) error {
	desc := IsDescending(sig)
	for i := range rungs {
		if err := nonNegative(rungNames[i], rungs[i]); err != nil {
			return err
		}
		if err := nonNegative(releaseNames[i], releases[i]); err != nil {
			return err
		}
	}
	for i := 0; i < len(rungs); i++ {
		for j := i + 1; j < len(rungs); j++ {
			a, b := rungs[i], rungs[j]
			if a == nil || b == nil {
				continue
			}
			if desc && *a <= *b {
				return fmt.Errorf("%s %d must be above %s %d on a falling signal", rungNames[i], *a, rungNames[j], *b)
			}
			if !desc && *a >= *b {
				return fmt.Errorf("%s %d must be below %s %d on a rising signal", rungNames[i], *a, rungNames[j], *b)
			}
		}
	}
	for i := range rungs {
		r, rel := rungs[i], releases[i]
		if r == nil || rel == nil {
			continue
		}
		if desc && *rel <= *r {
			return fmt.Errorf("%s %d must be above %s %d on a falling signal", releaseNames[i], *rel, rungNames[i], *r)
		}
		if !desc && *rel >= *r {
			return fmt.Errorf("%s %d must be below %s %d on a rising signal", releaseNames[i], *rel, rungNames[i], *r)
		}
	}
	if sig == SignalMemorySwap && rungs[3] != nil {
		return errors.New("memory.swap has no evictSoftAt rung")
	}
	return nil
}

// Validate reports whether the spec is usable: every enum known, every
// threshold entry a known signal listed once with its ladder in order (the
// CEL rules of the CRD), no negative value where the schema has a minimum of
// 0, a delivery cap of at least MinTier1DeliveriesPerHour, permille values in
// 0..1000, half-lives of at least 1 s, and the bounds above. A value past a
// tier-1 floor is not an error here: ApplyFloors clamps it. Errors wrap
// ErrInvalidNodeBudgetPolicy.
func (s *NodeBudgetPolicySpec) Validate() error {
	if err := s.validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNodeBudgetPolicy, err)
	}
	return nil
}

func (s *NodeBudgetPolicySpec) validate() error {
	if n := len(s.NodeSelector); n > MaxPolicyNodeSelectorLabels {
		return fmt.Errorf("nodeSelector has %d labels, at most %d", n, MaxPolicyNodeSelectorLabels)
	}
	if n := len(s.Thresholds); n > MaxPolicyThresholds {
		return fmt.Errorf("%d thresholds, at most %d", n, MaxPolicyThresholds)
	}
	seen := make(map[SignalID]bool, len(s.Thresholds))
	for i := range s.Thresholds {
		t := &s.Thresholds[i]
		if !t.Signal.Valid() {
			return fmt.Errorf("thresholds[%d]: signal %q is not a known signal", i, t.Signal)
		}
		if seen[t.Signal] {
			return fmt.Errorf("thresholds[%d]: %s is repeated", i, t.Signal)
		}
		seen[t.Signal] = true
		if err := checkLadder(t.Signal, t.rungs(), t.releases()); err != nil {
			return fmt.Errorf("thresholds[%d] %s: %w", i, t.Signal, err)
		}
		for _, f := range []struct {
			name string
			v    *int32
		}{{"graceSeconds", t.GraceSeconds}, {"dwellSeconds", t.DwellSeconds}, {"windowSeconds", t.WindowSeconds}} {
			if err := nonNegative(f.name, f.v); err != nil {
				return fmt.Errorf("thresholds[%d] %s: %w", i, t.Signal, err)
			}
		}
		if err := nonNegative("minReclaimBytes", t.MinReclaimBytes); err != nil {
			return fmt.Errorf("thresholds[%d] %s: %w", i, t.Signal, err)
		}
	}
	if r := s.Reserves; r != nil {
		for _, f := range []struct {
			name string
			v    *int64
		}{
			{"reserves.cpuSystemReserveMilli", r.CPUSystemReserveMilli},
			{"reserves.memSystemReserveBytes", r.MemSystemReserveBytes},
			{"reserves.memEvictionReserveBytes", r.MemEvictionReserveBytes},
		} {
			if err := nonNegative(f.name, f.v); err != nil {
				return err
			}
		}
	}
	if h := s.History; h != nil {
		for _, f := range []struct {
			name string
			v    *int32
		}{{"history.memoryHalfLifeSeconds", h.MemoryHalfLifeSeconds}, {"history.cpuHalfLifeSeconds", h.CPUHalfLifeSeconds}} {
			if f.v != nil && *f.v < 1 {
				return fmt.Errorf("%s %d is below 1", f.name, *f.v)
			}
		}
		if err := permille("history.marginPermille", h.MarginPermille); err != nil {
			return err
		}
	}
	if s.Mode != "" && !s.Mode.Valid() {
		return fmt.Errorf("mode %q is not a known mode", s.Mode)
	}
	if v := s.Tier1MaxDeliveriesPerHour; v != nil && *v < MinTier1DeliveriesPerHour {
		return fmt.Errorf("tier1MaxDeliveriesPerHour %d is below %d; set mode off to stop tier 1", *v, MinTier1DeliveriesPerHour)
	}
	return permille("psiHeldPermille", s.PSIHeldPermille)
}

// ApplyFloors returns a copy of s with every tier-1 timer raised (or, for
// the hourly cap, lowered) to its compiled floor, and the JSON paths of the
// fields it changed, in a fixed order. Unset fields stay unset: the
// producer's own defaults already respect the floors. s is not modified. A
// node that clamps anything reports PolicyClamped with these paths.
func ApplyFloors(s NodeBudgetPolicySpec) (NodeBudgetPolicySpec, []string) {
	out := *s.DeepCopy()
	var clamped []string
	raise := func(name string, p *int32, floor int32) {
		if p != nil && *p < floor {
			*p = floor
			clamped = append(clamped, name)
		}
	}
	raise("tier1GraceSeconds", out.Tier1GraceSeconds, MinTier1GraceSeconds)
	raise("tier1HeldSeconds", out.Tier1HeldSeconds, MinTier1HeldSeconds)
	if p := out.Tier1MaxDeliveriesPerHour; p != nil && *p > MaxTier1DeliveriesPerHour {
		*p = MaxTier1DeliveriesPerHour
		clamped = append(clamped, "tier1MaxDeliveriesPerHour")
	}
	raise("tier1CooloffSeconds", out.Tier1CooloffSeconds, MinTier1CooloffSeconds)
	return out, clamped
}

// Selects reports whether the spec's NodeSelector matches a node with these
// labels: every selector label present with the same value. An empty
// selector matches every node.
func (s *NodeBudgetPolicySpec) Selects(nodeLabels map[string]string) bool {
	for k, v := range s.NodeSelector {
		if got, ok := nodeLabels[k]; !ok || got != v {
			return false
		}
	}
	return true
}

// SelectPolicy picks the policy that applies to a node with these labels:
// among the policies that select it, the highest Spec.Priority, and on a tie
// the lexically smallest name. It returns a deep copy of the winner (nil
// when no policy selects the node) and the names of the other selecting
// policies in precedence order. A non-empty second result is the
// ConditionPolicyConflict the status writer reports.
func SelectPolicy(policies []NodeBudgetPolicy, nodeLabels map[string]string) (*NodeBudgetPolicy, []string) {
	var sel []*NodeBudgetPolicy
	for i := range policies {
		if policies[i].Spec.Selects(nodeLabels) {
			sel = append(sel, &policies[i])
		}
	}
	if len(sel) == 0 {
		return nil, nil
	}
	sort.SliceStable(sel, func(i, j int) bool {
		if sel[i].Spec.Priority != sel[j].Spec.Priority {
			return sel[i].Spec.Priority > sel[j].Spec.Priority
		}
		return sel[i].Name < sel[j].Name
	})
	var losers []string
	for _, p := range sel[1:] {
		losers = append(losers, p.Name)
	}
	return sel[0].DeepCopy(), losers
}

// DeepCopyInto copies the receiver into out.
func (in *SignalThresholds) DeepCopyInto(out *SignalThresholds) {
	*out = *in
	out.WarnAt, out.AdmitStopAt, out.CriticalAt = clonePtr(in.WarnAt), clonePtr(in.AdmitStopAt), clonePtr(in.CriticalAt)
	out.EvictSoftAt, out.EvictHardAt = clonePtr(in.EvictSoftAt), clonePtr(in.EvictHardAt)
	out.WarnReleaseAt, out.AdmitStopReleaseAt = clonePtr(in.WarnReleaseAt), clonePtr(in.AdmitStopReleaseAt)
	out.CriticalReleaseAt, out.EvictSoftReleaseAt = clonePtr(in.CriticalReleaseAt), clonePtr(in.EvictSoftReleaseAt)
	out.EvictHardReleaseAt = clonePtr(in.EvictHardReleaseAt)
	out.GraceSeconds, out.MinReclaimBytes = clonePtr(in.GraceSeconds), clonePtr(in.MinReclaimBytes)
	out.DwellSeconds, out.WindowSeconds = clonePtr(in.DwellSeconds), clonePtr(in.WindowSeconds)
}

// DeepCopyInto copies the receiver into out.
func (in *NodeBudgetPolicySpec) DeepCopyInto(out *NodeBudgetPolicySpec) {
	*out = *in
	if in.NodeSelector != nil {
		out.NodeSelector = make(map[string]string, len(in.NodeSelector))
		for k, v := range in.NodeSelector {
			out.NodeSelector[k] = v
		}
	}
	if in.Thresholds != nil {
		out.Thresholds = make([]SignalThresholds, len(in.Thresholds))
		for i := range in.Thresholds {
			in.Thresholds[i].DeepCopyInto(&out.Thresholds[i])
		}
	}
	if r := in.Reserves; r != nil {
		out.Reserves = &BudgetReserves{
			CPUSystemReserveMilli:   clonePtr(r.CPUSystemReserveMilli),
			MemSystemReserveBytes:   clonePtr(r.MemSystemReserveBytes),
			MemEvictionReserveBytes: clonePtr(r.MemEvictionReserveBytes),
		}
	}
	if h := in.History; h != nil {
		out.History = &BudgetHistory{
			MemoryHalfLifeSeconds: clonePtr(h.MemoryHalfLifeSeconds),
			CPUHalfLifeSeconds:    clonePtr(h.CPUHalfLifeSeconds),
			MarginPermille:        clonePtr(h.MarginPermille),
		}
	}
	out.Tier1GraceSeconds = clonePtr(in.Tier1GraceSeconds)
	out.Tier1HeldSeconds = clonePtr(in.Tier1HeldSeconds)
	out.Tier1MaxDeliveriesPerHour = clonePtr(in.Tier1MaxDeliveriesPerHour)
	out.Tier1CooloffSeconds = clonePtr(in.Tier1CooloffSeconds)
	out.PSIHeldPermille = clonePtr(in.PSIHeldPermille)
}

// DeepCopy returns a deep copy of the spec.
func (in *NodeBudgetPolicySpec) DeepCopy() *NodeBudgetPolicySpec {
	if in == nil {
		return nil
	}
	out := new(NodeBudgetPolicySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *NodeBudgetPolicy) DeepCopyInto(out *NodeBudgetPolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy returns a deep copy of the policy.
func (in *NodeBudgetPolicy) DeepCopy() *NodeBudgetPolicy {
	if in == nil {
		return nil
	}
	out := new(NodeBudgetPolicy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *NodeBudgetPolicy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *NodeBudgetPolicyList) DeepCopyInto(out *NodeBudgetPolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NodeBudgetPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *NodeBudgetPolicyList) DeepCopy() *NodeBudgetPolicyList {
	if in == nil {
		return nil
	}
	out := new(NodeBudgetPolicyList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *NodeBudgetPolicyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
