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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// samplePolicy is a policy that sets every field.
func samplePolicy() *NodeBudgetPolicy {
	return &NodeBudgetPolicy{
		TypeMeta:   metav1.TypeMeta{APIVersion: "budget.k3sm.io/v1alpha1", Kind: "NodeBudgetPolicy"},
		ObjectMeta: metav1.ObjectMeta{Name: "linux-workers"},
		Spec: NodeBudgetPolicySpec{
			NodeSelector: map[string]string{"kubernetes.io/os": "linux"},
			Priority:     10,
			Thresholds: []SignalThresholds{
				{
					Signal: SignalMemoryCompressor,
					WarnAt: i64(500), AdmitStopAt: i64(600), CriticalAt: i64(700), EvictSoftAt: i64(750), EvictHardAt: i64(800),
					WarnReleaseAt: i64(450), AdmitStopReleaseAt: i64(550), CriticalReleaseAt: i64(650),
					EvictSoftReleaseAt: i64(720), EvictHardReleaseAt: i64(780),
					GraceSeconds: i32(60), MinReclaimBytes: i64(256 * mib), DwellSeconds: i32(5),
				},
				{
					Signal: SignalMemorySwap, WarnAt: i64(256 * mib), AdmitStopAt: i64(512 * mib), CriticalAt: i64(1 * gib),
					WindowSeconds: i32(60),
				},
			},
			Reserves: &BudgetReserves{
				CPUSystemReserveMilli: i64(500), MemSystemReserveBytes: i64(2 * gib), MemEvictionReserveBytes: i64(512 * mib),
			},
			History: &BudgetHistory{
				MemoryHalfLifeSeconds: i32(DefaultMemoryHalfLifeSeconds), CPUHalfLifeSeconds: i32(DefaultCPUHalfLifeSeconds),
				MarginPermille: i32(DefaultMarginPermille),
			},
			Mode:                      KillModeOn,
			Tier1GraceSeconds:         i32(20),
			Tier1HeldSeconds:          i32(30),
			Tier1MaxDeliveriesPerHour: i32(4),
			Tier1CooloffSeconds:       i32(1800),
			PSIHeldPermille:           i32(600),
		},
	}
}

// TestFloorsAreLiteral pins the compiled floors and defaults by value: they
// are the numeric contract other producers test against, so a change here is
// a contract change a reviewer must see.
func TestFloorsAreLiteral(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		got, want int
	}{
		{"MinTier1GraceSeconds", MinTier1GraceSeconds, 10},
		{"MinTier1HeldSeconds", MinTier1HeldSeconds, 10},
		{"DefaultTier1HeldSeconds", DefaultTier1HeldSeconds, 30},
		{"MaxTier1DeliveriesPerHour", MaxTier1DeliveriesPerHour, 6},
		{"MinTier1DeliveriesPerHour", MinTier1DeliveriesPerHour, 1},
		{"MinTier1CooloffSeconds", MinTier1CooloffSeconds, 300},
		{"DefaultTier1CooloffSeconds", DefaultTier1CooloffSeconds, 1800},
		{"ArmingEpisodesRequired", ArmingEpisodesRequired, 3},
		{"DefaultMemoryHalfLifeSeconds", DefaultMemoryHalfLifeSeconds, 172800},
		{"DefaultCPUHalfLifeSeconds", DefaultCPUHalfLifeSeconds, 43200},
		{"DefaultMarginPermille", DefaultMarginPermille, 150},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

// TestApplyFloorsClamps asserts each tier-1 timer below its floor (or the
// hourly cap above its ceiling) is clamped and named, values within the
// floors are kept, unset fields stay unset, and the input is not modified.
func TestApplyFloorsClamps(t *testing.T) {
	t.Parallel()
	in := NodeBudgetPolicySpec{
		Tier1GraceSeconds: i32(1), Tier1HeldSeconds: i32(0),
		Tier1MaxDeliveriesPerHour: i32(60), Tier1CooloffSeconds: i32(-5),
	}
	out, clamped := ApplyFloors(in)
	want := []string{"tier1GraceSeconds", "tier1HeldSeconds", "tier1MaxDeliveriesPerHour", "tier1CooloffSeconds"}
	if !reflect.DeepEqual(clamped, want) {
		t.Fatalf("clamped = %v, want %v", clamped, want)
	}
	if *out.Tier1GraceSeconds != MinTier1GraceSeconds || *out.Tier1HeldSeconds != MinTier1HeldSeconds ||
		*out.Tier1MaxDeliveriesPerHour != MaxTier1DeliveriesPerHour || *out.Tier1CooloffSeconds != MinTier1CooloffSeconds {
		t.Fatalf("clamped spec = %+v", out)
	}
	if *in.Tier1GraceSeconds != 1 || *in.Tier1CooloffSeconds != -5 {
		t.Fatal("ApplyFloors modified its input")
	}
	ok := samplePolicy().Spec
	got, clamped := ApplyFloors(ok)
	if len(clamped) != 0 || !reflect.DeepEqual(got, ok) {
		t.Fatalf("a spec within the floors changed: clamped %v", clamped)
	}
	empty, clamped := ApplyFloors(NodeBudgetPolicySpec{})
	if len(clamped) != 0 || empty.Tier1GraceSeconds != nil || empty.Tier1CooloffSeconds != nil {
		t.Fatal("ApplyFloors set an unset field")
	}
}

// TestNodeBudgetPolicyValidate covers the policy rules a node refuses a
// relayed policy for. The CEL mirror over every ladder ordering is
// TestNodeBudgetPolicyValidateMirrorsCEL in k3sm.io/apis/k3smtest.
func TestNodeBudgetPolicyValidate(t *testing.T) {
	t.Parallel()
	if err := samplePolicy().Spec.Validate(); err != nil {
		t.Fatalf("the sample policy does not validate: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(s *NodeBudgetPolicySpec)
	}{
		{"unknown signal", func(s *NodeBudgetPolicySpec) { s.Thresholds[0].Signal = "cpu.pressure" }},
		{"repeated signal", func(s *NodeBudgetPolicySpec) { s.Thresholds[1].Signal = SignalMemoryCompressor }},
		{"rising ladder misordered", func(s *NodeBudgetPolicySpec) { s.Thresholds[0].CriticalAt = i64(550) }},
		{"release on the pressure side", func(s *NodeBudgetPolicySpec) { s.Thresholds[0].WarnReleaseAt = i64(500) }},
		{"memory.swap evict-soft", func(s *NodeBudgetPolicySpec) { s.Thresholds[1].EvictSoftAt = i64(2 * gib) }},
		{"negative rung", func(s *NodeBudgetPolicySpec) { s.Thresholds[1].WarnAt = i64(-1) }},
		{"negative grace", func(s *NodeBudgetPolicySpec) { s.Thresholds[0].GraceSeconds = i32(-1) }},
		{"negative dwell", func(s *NodeBudgetPolicySpec) { s.Thresholds[0].DwellSeconds = i32(-1) }},
		{"negative reserve", func(s *NodeBudgetPolicySpec) { s.Reserves.MemSystemReserveBytes = i64(-1) }},
		{"zero half-life", func(s *NodeBudgetPolicySpec) { s.History.CPUHalfLifeSeconds = i32(0) }},
		{"margin above 1000", func(s *NodeBudgetPolicySpec) { s.History.MarginPermille = i32(1001) }},
		{"unknown mode", func(s *NodeBudgetPolicySpec) { s.Mode = "observe" }},
		{"zero deliveries per hour", func(s *NodeBudgetPolicySpec) { s.Tier1MaxDeliveriesPerHour = i32(0) }},
		{"PSI above 1000", func(s *NodeBudgetPolicySpec) { s.PSIHeldPermille = i32(1001) }},
		{"too many thresholds", func(s *NodeBudgetPolicySpec) {
			for len(s.Thresholds) <= MaxPolicyThresholds {
				s.Thresholds = append(s.Thresholds, SignalThresholds{Signal: SignalPIDAvailable})
			}
		}},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			s := samplePolicy().Spec
			tc.mutate(&s)
			if err := s.Validate(); !errors.Is(err, ErrInvalidNodeBudgetPolicy) {
				t.Fatalf("error = %v, want ErrInvalidNodeBudgetPolicy", err)
			}
		})
	}
}

// TestSelectPolicy pins the precedence when several policies select a node:
// the highest priority, then the smallest name, the others reported as the
// conflict.
func TestSelectPolicy(t *testing.T) {
	t.Parallel()
	mk := func(name string, prio int32, sel map[string]string) NodeBudgetPolicy {
		return NodeBudgetPolicy{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: NodeBudgetPolicySpec{Priority: prio, NodeSelector: sel}}
	}
	linux := map[string]string{"kubernetes.io/os": "linux"}
	ps := []NodeBudgetPolicy{
		mk("zeta", 5, nil),
		mk("alpha", 5, linux),
		mk("beta", 1, nil),
		mk("darwin-only", 99, map[string]string{"kubernetes.io/os": "darwin"}),
	}
	w, losers := SelectPolicy(ps, map[string]string{"kubernetes.io/os": "linux", "x": "y"})
	if w == nil || w.Name != "alpha" || !reflect.DeepEqual(losers, []string{"zeta", "beta"}) {
		t.Fatalf("winner %v losers %v, want alpha over [zeta beta]", w, losers)
	}
	w.Spec.NodeSelector["kubernetes.io/os"] = "changed"
	if ps[1].Spec.NodeSelector["kubernetes.io/os"] != "linux" {
		t.Fatal("SelectPolicy returned the caller's policy, not a copy")
	}
	if w, losers := SelectPolicy(ps[3:], map[string]string{}); w != nil || losers != nil {
		t.Fatalf("no selecting policy: got %v %v", w, losers)
	}
	if w, losers := SelectPolicy(ps[2:3], nil); w == nil || w.Name != "beta" || len(losers) != 0 {
		t.Fatalf("one selecting policy: got %v %v", w, losers)
	}
}

// TestNodeBudgetPolicyDeepCopyIsIndependent asserts the policy copy shares
// no memory with its source.
func TestNodeBudgetPolicyDeepCopyIsIndependent(t *testing.T) {
	t.Parallel()
	p := samplePolicy()
	c := p.DeepCopy()
	if !reflect.DeepEqual(c, p) {
		t.Fatal("the copy differs from the source")
	}
	assertNoSharedMemory(t, "NodeBudgetPolicy", reflect.ValueOf(p), reflect.ValueOf(c))
	l := &NodeBudgetPolicyList{Items: []NodeBudgetPolicy{*samplePolicy()}}
	lc := l.DeepCopy()
	assertNoSharedMemory(t, "NodeBudgetPolicyList", reflect.ValueOf(l), reflect.ValueOf(lc))
	if _, ok := p.DeepCopyObject().(*NodeBudgetPolicy); !ok {
		t.Fatal("DeepCopyObject did not return a *NodeBudgetPolicy")
	}
}
