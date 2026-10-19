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
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	budgetRungs    = []string{"warnThreshold", "admitStopThreshold", "criticalThreshold", "evictSoftThreshold", "evictHardThreshold"}
	budgetReleases = []string{"warnRelease", "admitStopRelease", "criticalRelease", "evictSoftRelease", "evictHardRelease"}
)

// budgetSelfRules transcribes the budget manifests' rules: on NodeBudget
// status the three platform rules and the policy-in-force rule; on each NodeBudget signal record the unit
// rule; and on both that record and a NodeBudgetPolicy thresholds item (whose
// signal key is "id" and "signal" respectively) the ten pairwise rung
// orderings, the five release sides (both direction-aware: an .available
// signal falls under pressure) and the memory.swap rule. has(x) is key
// presence, so a value of 0 is present; numbers arrive as float64.
func budgetSelfRules() map[string]func(self any) bool {
	obj := func(self any) map[string]any { o, _ := self.(map[string]any); return o }
	has := func(o map[string]any, k string) bool { _, ok := o[k]; return ok }
	m := map[string]func(any) bool{
		"!has(self.level) || (has(self.darwin) != has(self.linux))": func(self any) bool {
			o := obj(self)
			return !has(o, "level") || has(o, "darwin") != has(o, "linux")
		},
		"!has(self.level) || has(self.platform)": func(self any) bool {
			o := obj(self)
			return !has(o, "level") || has(o, "platform")
		},
		"!has(self.platform) || (self.platform == 'darwin' ? !has(self.linux) : !has(self.darwin))": func(self any) bool {
			o := obj(self)
			if !has(o, "platform") {
				return true
			}
			if o["platform"] == "darwin" {
				return !has(o, "linux")
			}
			return !has(o, "darwin")
		},
		"!has(self.policy) || self.policy.outcome == 'rejected' || (has(self.inForce) && self.inForce.name == self.policy.name && (has(self.inForce.resourceVersion) ? has(self.policy.resourceVersion) && self.inForce.resourceVersion == self.policy.resourceVersion : !has(self.policy.resourceVersion)))": func(self any) bool {
			o := obj(self)
			if !has(o, "policy") {
				return true
			}
			p := obj(o["policy"])
			if p["outcome"] == "rejected" {
				return true
			}
			if !has(o, "inForce") {
				return false
			}
			f := obj(o["inForce"])
			if f["name"] != p["name"] {
				return false
			}
			if has(f, "resourceVersion") {
				return has(p, "resourceVersion") && f["resourceVersion"] == p["resourceVersion"]
			}
			return !has(p, "resourceVersion")
		},
		"self.unit == (self.id == 'memory.compressor' ? 'permille' : self.id == 'pid.available' ? 'count' : 'bytes')": func(self any) bool {
			o := obj(self)
			want := "bytes"
			switch o["id"] {
			case "memory.compressor":
				want = "permille"
			case "pid.available":
				want = "count"
			}
			return o["unit"] == want
		},
	}
	for _, key := range []string{"signal", "id"} {
		key := key
		// calmSide reports whether a sits on the calm side of b: above it on
		// a falling signal, below it on a rising one; true when either is
		// absent.
		calmSide := func(a, b string) func(any) bool {
			return func(self any) bool {
				o := obj(self)
				av, aok := o[a].(float64)
				bv, bok := o[b].(float64)
				if !aok || !bok {
					return true
				}
				if sig, _ := o[key].(string); strings.HasSuffix(sig, ".available") {
					return av > bv
				}
				return av < bv
			}
		}
		m["self."+key+" != 'memory.swap' || !has(self.evictSoftThreshold)"] = func(self any) bool {
			o := obj(self)
			return o[key] != "memory.swap" || !has(o, "evictSoftThreshold")
		}
		for i := range budgetRungs {
			for j := i + 1; j < len(budgetRungs); j++ {
				a, b := budgetRungs[i], budgetRungs[j]
				m[fmt.Sprintf("!has(self.%[1]s) || !has(self.%[2]s) || (self.%[3]s.endsWith('.available') ? self.%[1]s > self.%[2]s : self.%[1]s < self.%[2]s)", a, b, key)] = calmSide(a, b)
			}
		}
		for i := range budgetRungs {
			r, rel := budgetRungs[i], budgetReleases[i]
			m[fmt.Sprintf("!has(self.%[1]s) || !has(self.%[2]s) || (self.%[3]s.endsWith('.available') ? self.%[2]s > self.%[1]s : self.%[2]s < self.%[1]s)", r, rel, key)] = calmSide(rel, r)
		}
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
		{"a bytes signal in permille", func(st map[string]any) { sigAt(st, 0)["unit"] = "permille" }, "unit must be the signal's canonical unit"},
		{"a misordered falling ladder", func(st map[string]any) { sigAt(st, 0)["warnThreshold"] = 1.0 }, "warnThreshold must be on the calm side of admitStopThreshold"},
		{"a release on the pressure side", func(st map[string]any) { sigAt(st, 1)["warnRelease"] = 501.0 }, "warnRelease must be on the calm side of warnThreshold"},
		{"a memory.swap evict-soft rung", func(st map[string]any) { sigAt(st, 2)["evictSoftThreshold"] = 2e9 }, "memory.swap has no evictSoftThreshold rung"},
		{"no platform", func(st map[string]any) { delete(st, "platform") }, "status: a reporting status names its platform"},
		{"the wrong platform block", func(st map[string]any) { st["platform"] = "linux" }, "status: the platform block set must be the one platform names"},
		{"an applied policy with no policy in force", func(st map[string]any) { delete(st, "inForce") }, "status: an applied or clamped policy is the policy in force"},
		{"an applied policy with another version in force", func(st map[string]any) {
			st["inForce"].(map[string]any)["resourceVersion"] = "1"
		}, "status: an applied or clamped policy is the policy in force"},
		{"an in-force field outside the schema", func(st map[string]any) { st["inForce"].(map[string]any)["uid"] = "x" }, "status.inForce.uid: not declared"},
		{"a policy message", func(st map[string]any) { st["policy"].(map[string]any)["message"] = "free text" }, "status.policy.message: not declared"},
		{"an unknown clamped field", func(st map[string]any) {
			st["policy"].(map[string]any)["clampedFields"] = []any{"spec.thresholds"}
		}, "status.policy.clampedFields[0]: not in enum"},
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
	t.Run("admits a rejected policy with the last valid one in force", func(t *testing.T) {
		m := goldenStatus(t, "darwin")
		st := m["status"].(map[string]any)
		st["policy"].(map[string]any)["outcome"] = "rejected"
		st["inForce"] = map[string]any{"name": "default", "resourceVersion": "4700"}
		if errs := validate("", m, nbRoot); len(errs) > 0 {
			t.Fatalf("a rejected policy with an older one in force is refused: %q", errs)
		}
		delete(st, "inForce")
		if errs := validate("", m, nbRoot); len(errs) > 0 {
			t.Fatalf("a rejected policy with the defaults in force is refused: %q", errs)
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
	t.Run("no defaults", func(t *testing.T) {
		// The CRD defaults nothing: an unset field means the producer's
		// shipped default (the Default* constants), so a minimal policy is
		// stored exactly as written.
		m := map[string]any{
			"apiVersion": "budget.k3sm.io/v1alpha1", "kind": "NodeBudgetPolicy",
			"metadata": map[string]any{"name": "minimal"},
			"spec":     map[string]any{"history": map[string]any{}},
		}
		before, _ := json.Marshal(m)
		applyDefaults(m, polRoot)
		after, _ := json.Marshal(m)
		if string(before) != string(after) {
			t.Fatalf("the schema defaulted a field:\nbefore = %s\nafter  = %s", before, after)
		}
		if errs := validate("", m, polRoot); len(errs) > 0 {
			t.Fatalf("the minimal policy does not validate: %q", errs)
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
		{"a misordered rising ladder", func(sp map[string]any) { thAt(sp, 0)["criticalThreshold"] = 550.0 }, "admitStopThreshold must be on the calm side of criticalThreshold"},
		{"a misordered falling ladder", func(sp map[string]any) {
			sp["thresholds"] = append(sp["thresholds"].([]any), map[string]any{"signal": "memory.available", "warnThreshold": 1.0, "admitStopThreshold": 2.0})
		}, "warnThreshold must be on the calm side of admitStopThreshold"},
		{"a release on the pressure side", func(sp map[string]any) { thAt(sp, 0)["warnRelease"] = 501.0 }, "warnRelease must be on the calm side of warnThreshold"},
		{"a memory.swap evict-soft rung", func(sp map[string]any) { thAt(sp, 1)["evictSoftThreshold"] = 2e9 }, "memory.swap has no evictSoftThreshold rung"},
		{"a negative rung", func(sp map[string]any) { thAt(sp, 1)["warnThreshold"] = -1.0 }, "spec.thresholds[1].warnThreshold: below minimum 0"},
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
			{"tier1MaxDeliveriesPerHour", "minimum", budgetv1alpha1.MinTier1DeliveriesPerHour},
			{"tier1MaxDeliveriesPerHour", "maximum", budgetv1alpha1.MaxTier1DeliveriesPerHour},
			{"tier1CooloffSeconds", "minimum", budgetv1alpha1.MinTier1CooloffSeconds},
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

// TestNodeBudgetPolicyValidateMirrorsCEL asserts the Go Validate and the
// manifests' ladder rules (with the schema's minimum of 0) agree on every
// ladder, on both sides of the contract: a NodeBudgetPolicy thresholds item
// against NodeBudgetPolicySpec.Validate, and a NodeBudget signal record
// against NodeBudgetStatus.Validate. It walks the three ladder shapes (a
// falling signal, a rising one, memory.swap), every assignment of the five
// rungs from {unset, 0, 10, 20, 30, 40}, every release position (unset, the
// pressure side, equal, the calm side) around an ordered ladder that starts
// at 0, and every signal against every unit. The rules are read from the
// embedded manifests, so a rule text that drifts loses its transcription and
// fails here.
func TestNodeBudgetPolicyValidateMirrorsCEL(t *testing.T) {
	t.Parallel()
	polRoot := crdSchema(t, crd.NodeBudgetPolicyCRD())
	nbRoot := crdSchema(t, crd.NodeBudgetCRD())
	field := func(sch map[string]any, path ...string) map[string]any {
		for _, k := range path {
			sch = sch[k].(map[string]any)
		}
		return sch
	}
	polItem := field(polRoot, "properties", "spec", "properties", "thresholds", "items")
	sigItem := field(nbRoot, "properties", "status", "properties", "signals", "items")
	polRules, sigRules := rulesAt(polItem), rulesAt(sigItem)
	if len(polRules) != 16 || len(sigRules) != 17 {
		t.Fatalf("rules: %d on the policy item, %d on the signal record; want 16 and 17", len(polRules), len(sigRules))
	}
	// celOK evaluates the rules and the schema minimum of 0 on every number.
	celOK := func(v any, rules [][2]string) bool {
		b, _ := json.Marshal(v)
		var self map[string]any
		_ = json.Unmarshal(b, &self) // round trip of a value json.Marshal just produced
		for _, x := range self {
			if f, ok := x.(float64); ok && f < 0 {
				return false
			}
		}
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
	const unset = math.MinInt64
	ptr := func(v int64) *int64 {
		if v == unset {
			return nil
		}
		return &v
	}
	statusOK := func(s budgetv1alpha1.BudgetSignal) bool {
		st := budgetv1alpha1.NodeBudgetStatus{NodeBudgetObservation: budgetv1alpha1.NodeBudgetObservation{
			ObservedAt: metav1.NewTime(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)),
			Platform:   budgetv1alpha1.PlatformDarwin, Level: budgetv1alpha1.LevelOK,
			Darwin: &budgetv1alpha1.DarwinSignals{}, Signals: []budgetv1alpha1.BudgetSignal{s},
		}}
		return st.Validate() == nil
	}
	var cases, valid, invalid int
	check := func(what string, sig budgetv1alpha1.SignalID, unit budgetv1alpha1.SignalUnit, r, rel [5]int64) {
		th := budgetv1alpha1.SignalThresholds{
			Signal: sig, WarnThreshold: ptr(r[0]), AdmitStopThreshold: ptr(r[1]), CriticalThreshold: ptr(r[2]),
			EvictSoftThreshold: ptr(r[3]), EvictHardThreshold: ptr(r[4]),
			WarnRelease: ptr(rel[0]), AdmitStopRelease: ptr(rel[1]), CriticalRelease: ptr(rel[2]),
			EvictSoftRelease: ptr(rel[3]), EvictHardRelease: ptr(rel[4]),
		}
		spec := budgetv1alpha1.NodeBudgetPolicySpec{Thresholds: []budgetv1alpha1.SignalThresholds{th}}
		if cel, goV := celOK(th, polRules), spec.Validate() == nil; cel != goV {
			t.Fatalf("policy %s %s %v %v: CEL admits %v, Validate admits %v", what, sig, r, rel, cel, goV)
		}
		bs := budgetv1alpha1.BudgetSignal{
			ID: sig, Unit: unit, WarnThreshold: th.WarnThreshold, AdmitStopThreshold: th.AdmitStopThreshold,
			CriticalThreshold: th.CriticalThreshold, EvictSoftThreshold: th.EvictSoftThreshold, EvictHardThreshold: th.EvictHardThreshold,
			WarnRelease: th.WarnRelease, AdmitStopRelease: th.AdmitStopRelease, CriticalRelease: th.CriticalRelease,
			EvictSoftRelease: th.EvictSoftRelease, EvictHardRelease: th.EvictHardRelease,
		}
		cel, goV := celOK(bs, sigRules), statusOK(bs)
		if cel != goV {
			t.Fatalf("status %s %s %s %v %v: CEL admits %v, Validate admits %v", what, sig, unit, r, rel, cel, goV)
		}
		cases++
		if cel {
			valid++
		} else {
			invalid++
		}
	}
	none := [5]int64{unset, unset, unset, unset, unset}
	for _, sig := range []budgetv1alpha1.SignalID{budgetv1alpha1.SignalMemoryAvailable, budgetv1alpha1.SignalMemoryCompressor, budgetv1alpha1.SignalMemorySwap} {
		choices := []int64{unset, 0, 10, 20, 30, 40}
		var idx [5]int
		for {
			var r [5]int64
			for i := range idx {
				r[i] = choices[idx[i]]
			}
			check("rungs", sig, sig.Unit(), r, none)
			if !advance(idx[:], len(choices)) {
				break
			}
		}
		ordered := [5]int64{0, 10, 20, 30, 40}
		calm := int64(-5)
		if budgetv1alpha1.IsDescending(sig) {
			ordered = [5]int64{40, 30, 20, 10, 0}
			calm = 5
		}
		rungs := ordered
		if sig == budgetv1alpha1.SignalMemorySwap {
			rungs[3] = unset
		}
		var pos [5]int
		for {
			rel := none
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
			check("releases", sig, sig.Unit(), rungs, rel)
			if !advance(pos[:], 4) {
				break
			}
		}
	}
	for _, sig := range budgetv1alpha1.Signals() {
		for _, unit := range []budgetv1alpha1.SignalUnit{budgetv1alpha1.UnitBytes, budgetv1alpha1.UnitPermille, budgetv1alpha1.UnitCount} {
			check("unit", sig, unit, none, none)
		}
	}
	t.Logf("%d ladders on each side: %d admitted, %d refused by both", cases, valid, invalid)
	if valid == 0 || invalid == 0 {
		t.Fatal("the enumeration is vacuous: it must reach both verdicts")
	}
}

// advance steps idx as a base-n counter and reports false after the last
// combination.
func advance(idx []int, n int) bool {
	for k := range idx {
		idx[k]++
		if idx[k] < n {
			return true
		}
		idx[k] = 0
	}
	return false
}

// policyWire is a policy setting every field, as decoded JSON.
func policyWire(t *testing.T) map[string]any {
	t.Helper()
	p := `{"apiVersion":"budget.k3sm.io/v1alpha1","kind":"NodeBudgetPolicy","metadata":{"name":"linux-workers"},
"spec":{"nodeSelector":{"kubernetes.io/os":"linux"},"priority":10,
"thresholds":[
 {"signal":"memory.compressor","warnThreshold":500,"admitStopThreshold":600,"criticalThreshold":700,"evictSoftThreshold":750,"evictHardThreshold":800,
  "warnRelease":450,"admitStopRelease":550,"criticalRelease":650,"evictSoftRelease":720,"evictHardRelease":780,
  "graceSeconds":60,"minReclaimBytes":268435456,"dwellSeconds":5},
 {"signal":"memory.swap","warnThreshold":268435456,"admitStopThreshold":536870912,"criticalThreshold":1073741824,"windowSeconds":60}],
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
