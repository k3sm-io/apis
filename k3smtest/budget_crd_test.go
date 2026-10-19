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

package k3smtest_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"

	budgetv1alpha1 "k3sm.io/apis/budget/v1alpha1"
	"k3sm.io/apis/config/crd"
)

// celSelfTranscriptions maps each create-time x-kubernetes-validations rule
// text in the manifests this package validates to a Go transcription of it.
// The validator in update_crd_test.go looks every rule up here by its exact
// text and treats a missing entry as an error.
var celSelfTranscriptions = budgetSelfRules()

// budgetLadder is the ladder field order of a NodeBudgetPolicy thresholds
// item.
var (
	budgetRungs    = []string{"warnAt", "admitStopAt", "criticalAt", "evictSoftAt", "evictHardAt"}
	budgetReleases = []string{"warnReleaseAt", "admitStopReleaseAt", "criticalReleaseAt", "evictSoftReleaseAt", "evictHardReleaseAt"}
)

// budgetSelfRules transcribes the budget manifests' rules: the NodeBudget
// status platform rule, and on each NodeBudgetPolicy thresholds item the ten
// pairwise rung orderings, the five release sides (both direction-aware: an
// .available signal falls under pressure) and the memory.swap rule. has(x)
// is key presence; numbers arrive as float64.
func budgetSelfRules() map[string]func(self any) bool {
	num := func(o map[string]any, k string) (float64, bool) { f, ok := o[k].(float64); return f, ok }
	falling := func(o map[string]any) bool { s, _ := o["signal"].(string); return strings.HasSuffix(s, ".available") }
	// calmSide reports whether a sits on the calm side of b: above it on a
	// falling signal, below it on a rising one; true when either is absent.
	calmSide := func(a, b string) func(any) bool {
		return func(self any) bool {
			o, _ := self.(map[string]any)
			av, aok := num(o, a)
			bv, bok := num(o, b)
			if !aok || !bok {
				return true
			}
			if falling(o) {
				return av > bv
			}
			return av < bv
		}
	}
	m := map[string]func(any) bool{
		"!has(self.level) || (has(self.darwin) != has(self.linux))": func(self any) bool {
			o, _ := self.(map[string]any)
			_, level := o["level"]
			_, d := o["darwin"]
			_, l := o["linux"]
			return !level || d != l
		},
		"self.signal != 'memory.swap' || !has(self.evictSoftAt)": func(self any) bool {
			o, _ := self.(map[string]any)
			_, soft := o["evictSoftAt"]
			return o["signal"] != "memory.swap" || !soft
		},
	}
	for i := range budgetRungs {
		for j := i + 1; j < len(budgetRungs); j++ {
			a, b := budgetRungs[i], budgetRungs[j]
			m[fmt.Sprintf("!has(self.%[1]s) || !has(self.%[2]s) || (self.signal.endsWith('.available') ? self.%[1]s > self.%[2]s : self.%[1]s < self.%[2]s)", a, b)] = calmSide(a, b)
		}
	}
	for i := range budgetRungs {
		r, rel := budgetRungs[i], budgetReleases[i]
		m[fmt.Sprintf("!has(self.%[1]s) || !has(self.%[2]s) || (self.signal.endsWith('.available') ? self.%[2]s > self.%[1]s : self.%[2]s < self.%[1]s)", r, rel)] = calmSide(rel, r)
	}
	return m
}

// crdSchema returns the openAPIV3Schema of a single-version manifest, with
// numbers as float64 like a decoded object's.
func crdSchema(t *testing.T, manifest []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(manifest, &m); err != nil {
		t.Fatalf("the manifest is not valid YAML: %v", err)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	versions, _ := m["spec"].(map[string]any)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("the manifest has %d versions, want 1", len(versions))
	}
	return versions[0].(map[string]any)["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)
}

// goldenStatus reads one of the conformance documents as a generic object.
func goldenStatus(t *testing.T, platform string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "budget", "v1alpha1", "testdata", "status", platform+".golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	return m
}

// refused reports whether errs holds one containing want.
func refused(errs []string, want string) bool {
	for _, e := range errs {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}

// TestNodeBudgetCRDRoundTrip is the contract test between the budget Go types
// and the two embedded manifests: both golden status documents round-trip
// through a scheme codec and validate against the NodeBudget schema with its
// status rule, a sample policy validates against the NodeBudgetPolicy schema,
// a minimal policy picks up the defaults the Go doc comments promise, and
// objects that break the schema, the ladder rules or the floors are refused.
// This is the module-local stand-in for a dry-run apply; the live apiserver
// proof belongs to the consumer that carries apiextensions.
func TestNodeBudgetCRDRoundTrip(t *testing.T) {
	t.Parallel()
	nbRoot := crdSchema(t, crd.NodeBudgetCRD())
	polRoot := crdSchema(t, crd.NodeBudgetPolicyCRD())

	for _, p := range []string{"darwin", "linux"} {
		t.Run("the "+p+" golden validates and round-trips", func(t *testing.T) {
			m := goldenStatus(t, p)
			if errs := validate("", m, nbRoot); len(errs) > 0 {
				t.Fatalf("the golden does not validate against the CRD:\n%s", strings.Join(errs, "\n"))
			}
			s := runtime.NewScheme()
			if err := budgetv1alpha1.AddToScheme(s); err != nil {
				t.Fatalf("AddToScheme: %v", err)
			}
			codecs := serializer.NewCodecFactory(s)
			b, _ := json.Marshal(m)
			obj, gvk, err := codecs.UniversalDeserializer().Decode(b, nil, nil)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if gvk.Group != "budget.k3sm.io" || gvk.Kind != "NodeBudget" {
				t.Fatalf("decoded GVK = %v", gvk)
			}
			wire, err := runtime.Encode(codecs.LegacyCodec(budgetv1alpha1.SchemeGroupVersion), obj)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			var back map[string]any
			if err := json.Unmarshal(wire, &back); err != nil {
				t.Fatalf("unmarshal wire: %v", err)
			}
			if !reflect.DeepEqual(back["status"], m["status"]) {
				t.Fatalf("the codec round trip changed the status:\n got = %s\nwant = %s", wire, b)
			}
		})
	}

	for _, tc := range []struct {
		name   string
		mutate func(st map[string]any)
		want   string
	}{
		{"both platform blocks", func(st map[string]any) { st["linux"] = map[string]any{} }, "status: a reporting status carries exactly one of darwin and linux"},
		{"no platform block", func(st map[string]any) { delete(st, "darwin") }, "status: a reporting status carries exactly one of darwin and linux"},
		{"unknown level", func(st map[string]any) { st["level"] = "red" }, "status.level: not in enum"},
		{"unknown signal", func(st map[string]any) { sigAt(st, 0)["id"] = "memory.free" }, "status.signals[0].id: not in enum"},
		{"duplicate signal", func(st map[string]any) { sigAt(st, 1)["id"] = sigAt(st, 0)["id"] }, "status.signals: duplicate key"},
		{"signal without unit", func(st map[string]any) { delete(sigAt(st, 0), "unit") }, "status.signals[0].unit: required"},
		{"negative counter", func(st map[string]any) { st["counters"].([]any)[0].(map[string]any)["evictionsTotal"] = -1.0 }, "status.counters[0].evictionsTotal: below minimum"},
		{"memorystatus over 1000", func(st map[string]any) { st["darwin"].(map[string]any)["memorystatusLevelPermille"] = 1001.0 }, "status.darwin.memorystatusLevelPermille: above maximum"},
		{"a pod field", func(st map[string]any) { st["pod"] = "default/web-0" }, "status.pod: not declared"},
		{"a victim in the would-kill record", func(st map[string]any) {
			st["wouldKill"] = map[string]any{"band": "batch", "reason": "held-pressure", "at": "2026-10-19T12:00:00Z", "victim": "web-0"}
		}, "status.wouldKill.victim: not declared"},
		{"unknown writer shape", func(st map[string]any) { st["writer"].(map[string]any)["shape"] = "sidecar" }, "status.writer.shape: not in enum"},
	} {
		t.Run("refuses a status with "+tc.name, func(t *testing.T) {
			m := goldenStatus(t, "darwin")
			tc.mutate(m["status"].(map[string]any))
			if errs := validate("", m, nbRoot); !refused(errs, tc.want) {
				t.Errorf("want an error containing %q, got %q", tc.want, errs)
			}
		})
	}
	t.Run("refuses a spec", func(t *testing.T) {
		m := goldenStatus(t, "linux")
		m["spec"] = map[string]any{}
		if errs := validate("", m, nbRoot); !refused(errs, "spec: not declared") {
			t.Errorf("want spec refused, got %q", errs)
		}
	})
	t.Run("admits a status of a node that has not reported", func(t *testing.T) {
		m := map[string]any{"apiVersion": "budget.k3sm.io/v1alpha1", "kind": "NodeBudget", "metadata": map[string]any{"name": "n"},
			"status": map[string]any{"conditions": []any{map[string]any{"type": "Reporting", "status": "False", "reason": "NodeNotReporting", "lastTransitionTime": "2026-10-19T12:00:00Z"}}}}
		if errs := validate("", m, nbRoot); len(errs) > 0 {
			t.Fatalf("a not-reporting status is refused: %q", errs)
		}
	})

	t.Run("a full policy validates", func(t *testing.T) {
		if errs := validate("", policyWire(t), polRoot); len(errs) > 0 {
			t.Fatalf("the sample policy does not validate:\n%s", strings.Join(errs, "\n"))
		}
	})
	t.Run("defaults", func(t *testing.T) {
		m := map[string]any{
			"apiVersion": "budget.k3sm.io/v1alpha1", "kind": "NodeBudgetPolicy",
			"metadata": map[string]any{"name": "minimal"},
			"spec":     map[string]any{"history": map[string]any{}},
		}
		applyDefaults(m, polRoot)
		if errs := validate("", m, polRoot); len(errs) > 0 {
			t.Fatalf("the defaulted minimal policy does not validate: %q", errs)
		}
		b, _ := json.Marshal(m)
		var p budgetv1alpha1.NodeBudgetPolicy
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		s := p.Spec
		if s.Mode != budgetv1alpha1.KillModeAuto || s.Priority != 0 ||
			*s.Tier1HeldSeconds != budgetv1alpha1.DefaultTier1HeldSeconds ||
			*s.Tier1CooloffSeconds != budgetv1alpha1.DefaultTier1CooloffSeconds ||
			*s.History.MemoryHalfLifeSeconds != budgetv1alpha1.DefaultMemoryHalfLifeSeconds ||
			*s.History.CPUHalfLifeSeconds != budgetv1alpha1.DefaultCPUHalfLifeSeconds ||
			*s.History.MarginPermille != budgetv1alpha1.DefaultMarginPermille {
			t.Fatalf("defaulted spec = %s, want the Go defaults", b)
		}
		if s.Tier1GraceSeconds != nil || s.Tier1MaxDeliveriesPerHour != nil {
			t.Fatalf("a field with no default was defaulted: %s", b)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(sp map[string]any)
		want   string
	}{
		{"a grace below the floor", func(sp map[string]any) { sp["tier1GraceSeconds"] = 9.0 }, "spec.tier1GraceSeconds: below minimum 10"},
		{"a hold below the floor", func(sp map[string]any) { sp["tier1HeldSeconds"] = 9.0 }, "spec.tier1HeldSeconds: below minimum 10"},
		{"a delivery cap above the floor", func(sp map[string]any) { sp["tier1MaxDeliveriesPerHour"] = 7.0 }, "spec.tier1MaxDeliveriesPerHour: above maximum 6"},
		{"a zero delivery cap", func(sp map[string]any) { sp["tier1MaxDeliveriesPerHour"] = 0.0 }, "spec.tier1MaxDeliveriesPerHour: below minimum 1"},
		{"a cool-off below the floor", func(sp map[string]any) { sp["tier1CooloffSeconds"] = 299.0 }, "spec.tier1CooloffSeconds: below minimum 300"},
		{"an unknown mode", func(sp map[string]any) { sp["mode"] = "observe" }, "spec.mode: not in enum"},
		{"a band kill-order field", func(sp map[string]any) { sp["killOrder"] = []any{"batch"} }, "spec.killOrder: not declared"},
		{"a non-string selector value", func(sp map[string]any) { sp["nodeSelector"] = map[string]any{"a": 1.0} }, "spec.nodeSelector.a: want string"},
		{"too many selector labels", func(sp map[string]any) {
			sel := map[string]any{}
			for i := 0; i < 33; i++ {
				sel[fmt.Sprintf("l%d", i)] = "v"
			}
			sp["nodeSelector"] = sel
		}, "spec.nodeSelector: more than maxProperties 32"},
		{"an unknown signal", func(sp map[string]any) { thAt(sp, 0)["signal"] = "cpu.pressure" }, "spec.thresholds[0].signal: not in enum"},
		{"a repeated signal", func(sp map[string]any) { thAt(sp, 1)["signal"] = thAt(sp, 0)["signal"] }, "spec.thresholds: duplicate key"},
		{"a misordered rising ladder", func(sp map[string]any) { thAt(sp, 0)["criticalAt"] = 550.0 }, "admitStopAt must be on the calm side of criticalAt"},
		{"a misordered falling ladder", func(sp map[string]any) {
			sp["thresholds"] = append(sp["thresholds"].([]any), map[string]any{"signal": "memory.available", "warnAt": 1.0, "admitStopAt": 2.0})
		}, "warnAt must be on the calm side of admitStopAt"},
		{"a release on the pressure side", func(sp map[string]any) { thAt(sp, 0)["warnReleaseAt"] = 501.0 }, "warnReleaseAt must be on the calm side of warnAt"},
		{"a memory.swap evict-soft rung", func(sp map[string]any) { thAt(sp, 1)["evictSoftAt"] = 2e9 }, "memory.swap has no evictSoftAt rung"},
		{"a negative rung", func(sp map[string]any) { thAt(sp, 1)["warnAt"] = -1.0 }, "spec.thresholds[1].warnAt: below minimum 0"},
		{"a status", func(sp map[string]any) {}, ""},
	} {
		t.Run("refuses a policy with "+tc.name, func(t *testing.T) {
			m := policyWire(t)
			sp := m["spec"].(map[string]any)
			tc.mutate(sp)
			want := tc.want
			if want == "" {
				m["status"] = map[string]any{}
				want = "status: not declared"
			}
			if errs := validate("", m, polRoot); !refused(errs, want) {
				t.Errorf("want an error containing %q, got %q", want, errs)
			}
		})
	}

	t.Run("the schema floors are the Go floors", func(t *testing.T) {
		sp := polRoot["properties"].(map[string]any)["spec"].(map[string]any)["properties"].(map[string]any)
		bound := func(f, k string) any { return sp[f].(map[string]any)[k] }
		for _, tc := range []struct {
			field, key string
			want       int
		}{
			{"tier1GraceSeconds", "minimum", budgetv1alpha1.MinTier1GraceSeconds},
			{"tier1HeldSeconds", "minimum", budgetv1alpha1.MinTier1HeldSeconds},
			{"tier1HeldSeconds", "default", budgetv1alpha1.DefaultTier1HeldSeconds},
			{"tier1MaxDeliveriesPerHour", "minimum", budgetv1alpha1.MinTier1DeliveriesPerHour},
			{"tier1MaxDeliveriesPerHour", "maximum", budgetv1alpha1.MaxTier1DeliveriesPerHour},
			{"tier1CooloffSeconds", "minimum", budgetv1alpha1.MinTier1CooloffSeconds},
			{"tier1CooloffSeconds", "default", budgetv1alpha1.DefaultTier1CooloffSeconds},
		} {
			if got := bound(tc.field, tc.key); got != float64(tc.want) {
				t.Errorf("spec.%s %s = %v, want the Go constant %d", tc.field, tc.key, got, tc.want)
			}
		}
		th := sp["thresholds"].(map[string]any)
		if th["maxItems"] != float64(budgetv1alpha1.MaxPolicyThresholds) {
			t.Errorf("spec.thresholds maxItems = %v, want %d", th["maxItems"], budgetv1alpha1.MaxPolicyThresholds)
		}
		if sp["nodeSelector"].(map[string]any)["maxProperties"] != float64(budgetv1alpha1.MaxPolicyNodeSelectorLabels) {
			t.Errorf("spec.nodeSelector maxProperties differs from MaxPolicyNodeSelectorLabels")
		}
	})
}

// TestNodeBudgetPolicyValidateMirrorsCEL asserts NodeBudgetPolicySpec.Validate
// and the manifest's ladder rules agree on every ladder: each of the three
// ladder shapes (a falling signal, a rising one, memory.swap), every
// assignment of the five rungs from {unset, 10, 20, 30, 40, 50}, and every
// release position (unset, the pressure side, equal, the calm side) on an
// ordered ladder. The rules are read from the embedded manifest, so a rule
// text that drifts loses its transcription and fails here.
func TestNodeBudgetPolicyValidateMirrorsCEL(t *testing.T) {
	t.Parallel()
	polRoot := crdSchema(t, crd.NodeBudgetPolicyCRD())
	item := polRoot["properties"].(map[string]any)["spec"].(map[string]any)["properties"].(map[string]any)["thresholds"].(map[string]any)["items"].(map[string]any)
	rules := rulesAt(item)
	if len(rules) != 16 {
		t.Fatalf("the thresholds item carries %d rules, want 16", len(rules))
	}
	celOK := func(th budgetv1alpha1.SignalThresholds) bool {
		b, _ := json.Marshal(th)
		var self map[string]any
		_ = json.Unmarshal(b, &self) // round trip of a value json.Marshal just produced
		for _, r := range rules {
			eval, ok := celSelfTranscriptions[r[0]]
			if !ok {
				t.Fatalf("rule %q has no transcription", r[0])
			}
			if !eval(self) {
				return false
			}
		}
		return true
	}
	set := func(th *budgetv1alpha1.SignalThresholds, names []string, vals []int64) {
		ptrs := map[string]**int64{
			"warnAt": &th.WarnAt, "admitStopAt": &th.AdmitStopAt, "criticalAt": &th.CriticalAt,
			"evictSoftAt": &th.EvictSoftAt, "evictHardAt": &th.EvictHardAt,
			"warnReleaseAt": &th.WarnReleaseAt, "admitStopReleaseAt": &th.AdmitStopReleaseAt,
			"criticalReleaseAt": &th.CriticalReleaseAt, "evictSoftReleaseAt": &th.EvictSoftReleaseAt,
			"evictHardReleaseAt": &th.EvictHardReleaseAt,
		}
		for i, n := range names {
			if vals[i] != 0 {
				v := vals[i]
				*ptrs[n] = &v
			}
		}
	}
	check := func(th budgetv1alpha1.SignalThresholds) (bool, bool) {
		spec := budgetv1alpha1.NodeBudgetPolicySpec{Thresholds: []budgetv1alpha1.SignalThresholds{th}}
		return celOK(th), spec.Validate() == nil
	}
	var cases, valid, invalid int
	for _, sig := range []budgetv1alpha1.SignalID{budgetv1alpha1.SignalMemoryAvailable, budgetv1alpha1.SignalMemoryCompressor, budgetv1alpha1.SignalMemorySwap} {
		choices := []int64{0, 10, 20, 30, 40, 50}
		idx := make([]int, 5)
		for {
			vals := make([]int64, 5)
			for i := range idx {
				vals[i] = choices[idx[i]]
			}
			th := budgetv1alpha1.SignalThresholds{Signal: sig}
			set(&th, budgetRungs, vals)
			cel, goV := check(th)
			if cel != goV {
				t.Fatalf("%s rungs %v: CEL admits %v, Validate admits %v", sig, vals, cel, goV)
			}
			cases++
			if cel {
				valid++
			} else {
				invalid++
			}
			k := 0
			for k < 5 {
				idx[k]++
				if idx[k] < len(choices) {
					break
				}
				idx[k] = 0
				k++
			}
			if k == 5 {
				break
			}
		}
		// Releases around an ordered ladder: 0 unset, then the pressure
		// side, equal, the calm side of each rung.
		ordered := []int64{100, 200, 300, 400, 500}
		calm := int64(-5)
		if budgetv1alpha1.IsDescending(sig) {
			ordered = []int64{500, 400, 300, 200, 100}
			calm = 5
		}
		pos := make([]int, 5)
		for {
			th := budgetv1alpha1.SignalThresholds{Signal: sig}
			rungs := append([]int64(nil), ordered...)
			if sig == budgetv1alpha1.SignalMemorySwap {
				rungs[3] = 0
			}
			set(&th, budgetRungs, rungs)
			rel := make([]int64, 5)
			for i, p := range pos {
				switch p {
				case 1:
					rel[i] = ordered[i] - calm
				case 2:
					rel[i] = ordered[i]
				case 3:
					rel[i] = ordered[i] + calm
				}
			}
			set(&th, budgetReleases, rel)
			cel, goV := check(th)
			if cel != goV {
				t.Fatalf("%s releases %v around %v: CEL admits %v, Validate admits %v", sig, rel, rungs, cel, goV)
			}
			cases++
			if cel {
				valid++
			} else {
				invalid++
			}
			k := 0
			for k < 5 {
				pos[k]++
				if pos[k] < 4 {
					break
				}
				pos[k] = 0
				k++
			}
			if k == 5 {
				break
			}
		}
	}
	t.Logf("%d ladders: %d admitted, %d refused by both", cases, valid, invalid)
	if valid == 0 || invalid == 0 {
		t.Fatal("the enumeration is vacuous: it must reach both verdicts")
	}
}

// policyWire is a policy setting every field, as decoded JSON.
func policyWire(t *testing.T) map[string]any {
	t.Helper()
	p := `{"apiVersion":"budget.k3sm.io/v1alpha1","kind":"NodeBudgetPolicy","metadata":{"name":"linux-workers"},
"spec":{"nodeSelector":{"kubernetes.io/os":"linux"},"priority":10,
"thresholds":[
 {"signal":"memory.compressor","warnAt":500,"admitStopAt":600,"criticalAt":700,"evictSoftAt":750,"evictHardAt":800,
  "warnReleaseAt":450,"admitStopReleaseAt":550,"criticalReleaseAt":650,"evictSoftReleaseAt":720,"evictHardReleaseAt":780,
  "graceSeconds":60,"minReclaimBytes":268435456,"dwellSeconds":5},
 {"signal":"memory.swap","warnAt":268435456,"admitStopAt":536870912,"criticalAt":1073741824,"windowSeconds":60}],
"reserves":{"cpuSystemReserveMilli":500,"memSystemReserveBytes":2147483648,"memEvictionReserveBytes":536870912},
"history":{"memoryHalfLifeSeconds":172800,"cpuHalfLifeSeconds":43200,"marginPermille":150},
"mode":"on","tier1GraceSeconds":20,"tier1HeldSeconds":30,"tier1MaxDeliveriesPerHour":4,"tier1CooloffSeconds":1800,"psiHeldPermille":600}}`
	var m map[string]any
	if err := json.Unmarshal([]byte(p), &m); err != nil {
		t.Fatalf("parse policy: %v", err)
	}
	var typed budgetv1alpha1.NodeBudgetPolicy
	dec := json.NewDecoder(strings.NewReader(p))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&typed); err != nil {
		t.Fatalf("the policy fixture does not decode into the Go type: %v", err)
	}
	if err := typed.Spec.Validate(); err != nil {
		t.Fatalf("the policy fixture does not validate in Go: %v", err)
	}
	return m
}

func sigAt(st map[string]any, i int) map[string]any { return st["signals"].([]any)[i].(map[string]any) }
func thAt(sp map[string]any, i int) map[string]any {
	return sp["thresholds"].([]any)[i].(map[string]any)
}
