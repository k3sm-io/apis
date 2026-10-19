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
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	gib = int64(1) << 30
	mib = int64(1) << 20
)

func i64(v int64) *int64 { return &v }
func i32(v int32) *int32 { return &v }

// goldenStatusFiles are the conformance documents, one per platform.
var goldenStatusFiles = map[Platform]string{
	PlatformDarwin: filepath.Join("testdata", "status", "darwin.golden.json"),
	PlatformLinux:  filepath.Join("testdata", "status", "linux.golden.json"),
}

// sampleAt is the fixed instant every golden is stamped with.
var sampleAt = metav1.NewTime(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))

// sampleDarwin is a calm macOS node written through the annotation relay.
// Together with sampleLinux it sets every field of the status at least
// once (TestGoldenStatusSetCoversEveryOptionalField).
func sampleDarwin() *NodeBudget {
	return &NodeBudget{
		TypeMeta:   metav1.TypeMeta{APIVersion: "budget.k3sm.io/v1alpha1", Kind: "NodeBudget"},
		ObjectMeta: metav1.ObjectMeta{Name: "mac-worker-1"},
		Status: NodeBudgetStatus{
			NodeBudgetObservation: NodeBudgetObservation{
				ObservedAt:         sampleAt,
				ProducerGeneration: 4,
				Platform:           PlatformDarwin,
				Level:              LevelOK,
				Admit:              true,
				Capacity: BudgetCapacity{
					CPUCapacityMilli: i64(8000), CPUSystemReserveMilli: i64(500),
					MemCapacityBytes: i64(16 * gib), MemSystemReserveBytes: i64(2 * gib),
					MemEvictionReserveBytes: i64(512 * mib), MemAllocatableBytes: i64(13*gib + 512*mib),
					SwapTotalBytes: i64(4 * gib), SwapUsedBytes: i64(1 * gib),
				},
				Signals: []BudgetSignal{
					{
						ID: SignalMemoryAvailable, Unit: UnitBytes, Value: i64(6 * gib),
						WarnThreshold: i64(2 * gib), AdmitStopThreshold: i64(1536 * mib), CriticalThreshold: i64(1 * gib),
						EvictSoftThreshold: i64(512 * mib), EvictHardThreshold: i64(100 * mib),
						WarnRelease: i64(2560 * mib), AdmitStopRelease: i64(2 * gib), CriticalRelease: i64(1536 * mib),
						EvictSoftRelease: i64(768 * mib), EvictHardRelease: i64(256 * mib),
						GraceSeconds: i64(90), MinReclaimBytes: i64(256 * mib),
						Trend: TrendSteady, LimitingResource: ResourceMemory,
					},
					{
						ID: SignalMemoryCompressor, Unit: UnitPermille, Value: i64(210),
						WarnThreshold: i64(500), AdmitStopThreshold: i64(600), CriticalThreshold: i64(700), EvictSoftThreshold: i64(750), EvictHardThreshold: i64(800),
						WarnRelease: i64(450), AdmitStopRelease: i64(550), CriticalRelease: i64(650),
						EvictSoftRelease: i64(720), EvictHardRelease: i64(780),
						GraceSeconds: i64(60), Trend: TrendRising, LimitingResource: ResourceMemory,
					},
					{
						ID: SignalMemorySwap, Unit: UnitBytes, Value: i64(0),
						WarnThreshold: i64(256 * mib), AdmitStopThreshold: i64(512 * mib), CriticalThreshold: i64(1 * gib),
						WindowSeconds: i64(60), Trend: TrendSteady, LimitingResource: ResourceMemory,
					},
					{
						ID: SignalSwapfsAvailable, Unit: UnitBytes, Value: i64(120 * gib),
						WarnThreshold: i64(24 * gib), AdmitStopThreshold: i64(16 * gib), CriticalThreshold: i64(12 * gib), EvictHardThreshold: i64(8 * gib),
						LimitingResource: ResourceSwapfs,
					},
					{
						ID: SignalNodefsAvailable, Unit: UnitBytes, Value: i64(120 * gib),
						WarnThreshold: i64(20 * gib), AdmitStopThreshold: i64(15 * gib), CriticalThreshold: i64(10 * gib), EvictHardThreshold: i64(2 * gib),
						EvictHardRelease: i64(10*gib + 1), LimitingResource: ResourceNodefs,
					},
					{
						ID: SignalImagefsAvailable, Unit: UnitBytes, Value: i64(120 * gib),
						SharedWith: SignalNodefsAvailable, LimitingResource: ResourceNodefs,
					},
					{
						ID: SignalPIDAvailable, Unit: UnitCount, Value: i64(9000),
						WarnThreshold: i64(3000), AdmitStopThreshold: i64(2500), CriticalThreshold: i64(1500), EvictHardThreshold: i64(1000),
						LimitingResource: ResourcePID,
					},
				},
				Recommendation: BudgetRecommendation{SafeAdditionalBytes: i64(3 * gib), Basis: SignalMemoryAvailable},
				KillMode:       KillMode{Mode: KillModeAuto, ArmingEpisodes: 2, Reason: KillReasonObserving},
				Counters: []BandCounters{
					{Band: BandCritical},
					{Band: BandStandard, RestartsTotal: 3},
					{
						Band: BandBatch, EvictionsTotal: 5, OOMKillsTotal: 1, KernelKillsTotal: 1, RestartsTotal: 7,
						Windows: []WindowCount{
							{Window: Window1h, Reason: CounterEviction, Count: 1},
							{Window: Window24h, Reason: CounterEviction, Count: 5},
						},
					},
				},
				Policy:  &AppliedPolicy{Name: "default", ResourceVersion: "4711", Outcome: PolicyApplied},
				InForce: &PolicyRef{Name: "default", ResourceVersion: "4711"},
				Darwin: &DarwinSignals{
					VMPressureLevel: i64(1), MemorystatusLevelPermille: i64(640),
					Compressor: &DarwinCompressor{
						BytesUsed: i64(1 * gib), BytesStored: i64(3 * gib), SegmentLimitBytes: i64(8 * gib),
					},
					SwapinsTotal: i64(1200), SwapoutsTotal: i64(3400),
				},
			},
			Writer: &BudgetWriter{Shape: WriterAnnotationRelay},
			Conditions: []metav1.Condition{
				{
					Type: ConditionReporting, Status: metav1.ConditionTrue, ObservedGeneration: 1,
					LastTransitionTime: sampleAt, Reason: ReasonNodeReporting, Message: "the node's budget is current",
				},
				{Type: ConditionPolicyConflict, Status: metav1.ConditionFalse, LastTransitionTime: sampleAt, Reason: ReasonSinglePolicy},
			},
		},
	}
}

// sampleLinux is a Linux node under pressure, written by an own-node
// producer.
func sampleLinux() *NodeBudget {
	deadline := metav1.NewTime(sampleAt.Add(45 * time.Second))
	return &NodeBudget{
		TypeMeta:   metav1.TypeMeta{APIVersion: "budget.k3sm.io/v1alpha1", Kind: "NodeBudget"},
		ObjectMeta: metav1.ObjectMeta{Name: "linux-worker-1"},
		Status: NodeBudgetStatus{
			NodeBudgetObservation: NodeBudgetObservation{
				ObservedAt:         sampleAt,
				ProducerGeneration: 9,
				Platform:           PlatformLinux,
				Level:              LevelEvicting,
				AdmitReasonSignal:  SignalMemoryAvailable,
				EvictDeadline:      &deadline,
				Capacity: BudgetCapacity{
					CPUCapacityMilli: i64(16000), MemCapacityBytes: i64(32 * gib), MemAllocatableBytes: i64(30 * gib),
					SwapTotalBytes: i64(0), SwapUsedBytes: i64(0),
				},
				Signals: []BudgetSignal{
					{
						ID: SignalMemoryAvailable, Unit: UnitBytes, Value: i64(400 * mib),
						WarnThreshold: i64(4 * gib), AdmitStopThreshold: i64(3 * gib), CriticalThreshold: i64(1600 * mib),
						EvictSoftThreshold: i64(800 * mib), EvictHardThreshold: i64(100 * mib),
						GraceSeconds: i64(90), MinReclaimBytes: i64(512 * mib),
						EtaToNextRungSeconds: i64(30), Trend: TrendFalling, LimitingResource: ResourceMemory,
					},
				},
				Recommendation: BudgetRecommendation{SafeAdditionalBytes: i64(0), Basis: SignalMemoryAvailable},
				KillMode:       KillMode{Mode: KillModeOn, Armed: true, ArmingEpisodes: 3, Reason: KillReasonArmed},
				WouldKill:      &WouldKill{Band: BandBatch, Reason: WouldKillHeldPressure, At: sampleAt},
				Counters: []BandCounters{
					{
						Band: BandBatch, EvictionsTotal: 2, MemHighEventsTotal: 11, Tier1RefusedTotal: 1,
						Windows: []WindowCount{{Window: Window1h, Reason: CounterTier1Refused, Count: 1}},
					},
				},
				Policy: &AppliedPolicy{
					Name: "linux-workers", ResourceVersion: "88", Outcome: PolicyClamped,
					ClampedFields: []PolicyField{FieldTier1GraceSeconds},
				},
				InForce: &PolicyRef{Name: "linux-workers", ResourceVersion: "88"},
				Linux: &LinuxSignals{
					PSI: &LinuxPSI{
						CPU:    &PSIResource{Some: &PSILine{Avg10Permille: i64(120), Avg60Permille: i64(80), TotalUs: i64(5_000_000)}, Full: &PSILine{Avg10Permille: i64(0), Avg60Permille: i64(0), TotalUs: i64(0)}},
						Memory: &PSIResource{Some: &PSILine{Avg10Permille: i64(610), Avg60Permille: i64(420), TotalUs: i64(9_000_000)}, Full: &PSILine{Avg10Permille: i64(300), Avg60Permille: i64(200), TotalUs: i64(4_000_000)}},
						IO:     &PSIResource{Some: &PSILine{Avg10Permille: i64(10), Avg60Permille: i64(5), TotalUs: i64(700_000)}, Full: &PSILine{Avg10Permille: i64(4), Avg60Permille: i64(2), TotalUs: i64(300_000)}},
					},
					MemoryEvents: &LinuxMemoryEvents{High: i64(11), Max: i64(2), OOM: i64(1), OOMKill: i64(1)},
				},
			},
			Writer: &BudgetWriter{Shape: WriterOwnNode, NodeNameExtra: true},
			Conditions: []metav1.Condition{
				{Type: ConditionReporting, Status: metav1.ConditionTrue, LastTransitionTime: sampleAt, Reason: ReasonNodeReporting},
				{Type: ConditionPolicyClamped, Status: metav1.ConditionTrue, LastTransitionTime: sampleAt, Reason: ReasonFieldsClamped, Message: "tier1GraceSeconds raised to the 10 s floor"},
			},
		},
	}
}

// samples returns both goldens by platform.
func samples() map[Platform]*NodeBudget {
	return map[Platform]*NodeBudget{PlatformDarwin: sampleDarwin(), PlatformLinux: sampleLinux()}
}

// TestNodeBudgetGoldenStatusDocuments pins the two conformance documents
// against testdata and checks that each decodes and validates. A producer's
// status is compared with these. Regenerate deliberately with
// UPDATE_GOLDEN=1.
func TestNodeBudgetGoldenStatusDocuments(t *testing.T) {
	t.Parallel()
	for p, nb := range samples() {
		t.Run(string(p), func(t *testing.T) {
			got, err := json.MarshalIndent(nb, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got = append(got, '\n')
			path := goldenStatusFiles[p]
			if os.Getenv("UPDATE_GOLDEN") != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("update golden: %v", err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("NodeBudget JSON differs from %s\n got = %s\nwant = %s", path, got, want)
			}
			var back NodeBudget
			dec := json.NewDecoder(bytes.NewReader(want))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&back); err != nil {
				t.Fatalf("the golden does not decode strictly: %v", err)
			}
			if err := back.Status.Validate(); err != nil {
				t.Fatalf("the golden does not validate: %v", err)
			}
			if back.Status.Platform != p {
				t.Fatalf("golden platform = %q, want %q", back.Status.Platform, p)
			}
		})
	}
}

// jsonPaths lists the JSON paths of every field reachable from t, with "[]"
// for a list element, treating metav1.Time as a leaf and inlining embedded
// structs as encoding/json does.
func jsonPaths(t reflect.Type, prefix string, out map[string]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		if t.Kind() == reflect.Slice {
			prefix += "[]"
		}
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == metav1TimeType {
		out[prefix] = true
		return
	}
	fields := map[string]reflect.Type{}
	collectJSONFields(t, fields)
	for name, ft := range fields {
		jsonPaths(ft, prefix+"."+name, out)
	}
}

// presentPaths lists the JSON paths present in a decoded document.
func presentPaths(v any, prefix string, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			presentPaths(c, prefix+"."+k, out)
		}
	case []any:
		for _, c := range x {
			presentPaths(c, prefix+"[]", out)
		}
	default:
		out[prefix] = true
	}
}

// TestGoldenStatusSetCoversEveryOptionalField asserts every field of
// NodeBudgetStatus appears in at least one golden document, so a reader
// tested against the set has seen every field, and a field added to the
// status without a golden update fails here.
func TestGoldenStatusSetCoversEveryOptionalField(t *testing.T) {
	t.Parallel()
	want := map[string]bool{}
	jsonPaths(reflect.TypeOf(NodeBudgetStatus{}), "status", want)
	got := map[string]bool{}
	for _, path := range goldenStatusFiles {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		presentPaths(doc["status"], "status", got)
	}
	var missing []string
	for p := range want {
		if !got[p] {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("no golden document sets %d status fields:\n%s", len(missing), strings.Join(missing, "\n"))
	}
	// The walk is not vacuous: it reaches the inline observation, list
	// elements, the deepest platform leaves and the writer's conditions.
	for _, p := range []string{
		"status.level", "status.signals[].warnThreshold", "status.counters[].windows[].count",
		"status.linux.psi.memory.full.totalUs", "status.darwin.compressor.bytesUsed",
		"status.writer.nodeNameExtra", "status.conditions[].reason",
	} {
		if !want[p] {
			t.Errorf("the type walk did not reach %s", p)
		}
	}
}

// podIdentityRE matches a field name that could carry pod identity.
var podIdentityRE = regexp.MustCompile(`(?i)pod|namespace|uid|container|victim`)

// enumTypes are the closed string types a status may carry.
var enumTypes = map[reflect.Type]bool{
	reflect.TypeOf(SignalID("")): true, reflect.TypeOf(SignalUnit("")): true,
	reflect.TypeOf(Level("")): true, reflect.TypeOf(Band("")): true,
	reflect.TypeOf(Trend("")): true, reflect.TypeOf(LimitingResource("")): true,
	reflect.TypeOf(KillModeName("")): true, reflect.TypeOf(KillModeReason("")): true,
	reflect.TypeOf(WouldKillReason("")): true, reflect.TypeOf(PolicyOutcome("")): true,
	reflect.TypeOf(CounterWindow("")): true, reflect.TypeOf(CounterReason("")): true,
	reflect.TypeOf(WriterShape("")): true, reflect.TypeOf(Platform("")): true,
	reflect.TypeOf(PolicyField("")): true,
}

// freeStringPaths are the status strings that are not enum values, each
// with the reason it cannot carry pod identity.
var freeStringPaths = map[string]string{
	"status.policy.name":             "the name of a cluster-scoped NodeBudgetPolicy, chosen by an admin",
	"status.policy.resourceVersion":  "an apiserver resourceVersion",
	"status.inForce.name":            "the name of a cluster-scoped NodeBudgetPolicy, chosen by an admin",
	"status.inForce.resourceVersion": "an apiserver resourceVersion",
}

// identityProblems walks a type and reports every field whose name could
// carry pod identity, every map, and every string that is neither an enum
// type nor a listed free string. metav1.Condition (the status writer's own
// conditions) and metav1.Time are leaves.
func identityProblems(t reflect.Type, prefix string) []string {
	var out []string
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		if t.Kind() == reflect.Slice {
			prefix += "[]"
		}
		t = t.Elem()
	}
	switch {
	case t == metav1TimeType || t == reflect.TypeOf(metav1.Condition{}):
		return nil
	case t.Kind() == reflect.Map:
		return []string{prefix + ": a map can carry any key"}
	case t.Kind() == reflect.String:
		if !enumTypes[t] {
			if _, ok := freeStringPaths[prefix]; !ok {
				out = append(out, prefix+": a free string ("+t.String()+")")
			}
		}
		return out
	case t.Kind() != reflect.Struct:
		return nil
	}
	fields := map[string]reflect.Type{}
	collectJSONFields(t, fields)
	for name, ft := range fields {
		if podIdentityRE.MatchString(name) {
			out = append(out, prefix+"."+name+": the field name suggests pod identity")
		}
		out = append(out, identityProblems(ft, prefix+"."+name)...)
	}
	return out
}

// TestNodeBudgetStatusHasNoPodIdentity asserts the status can name no pod,
// namespace, container or UID: its only strings are enum values and the
// listed policy fields, it has no map, and no field name suggests identity.
// Victim identity stays in namespaced Events.
func TestNodeBudgetStatusHasNoPodIdentity(t *testing.T) {
	t.Parallel()
	if p := identityProblems(reflect.TypeOf(NodeBudgetStatus{}), "status"); len(p) > 0 {
		sort.Strings(p)
		t.Fatalf("the status can carry pod identity:\n%s", strings.Join(p, "\n"))
	}
	t.Run("the walker is not vacuous", func(t *testing.T) {
		type leaky struct {
			VictimPod string            `json:"victimPod"`
			Labels    map[string]string `json:"labels"`
		}
		p := identityProblems(reflect.TypeOf(leaky{}), "status")
		if len(p) != 3 {
			t.Fatalf("want 3 problems (name, free string, map), got %q", p)
		}
	})
}

// TestNodeBudgetStatusValidate covers the observation rules the strict
// decoder relies on, each row breaking one.
func TestNodeBudgetStatusValidate(t *testing.T) {
	t.Parallel()
	if err := (&NodeBudgetStatus{Writer: &BudgetWriter{Shape: WriterAnnotationRelay}}).Validate(); err != nil {
		t.Fatalf("a status with no observation (a node not reporting) must validate: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(o *NodeBudgetObservation)
	}{
		{"no observedAt", func(o *NodeBudgetObservation) { o.ObservedAt = metav1.Time{} }},
		{"negative generation", func(o *NodeBudgetObservation) { o.ProducerGeneration = -1 }},
		{"unknown platform", func(o *NodeBudgetObservation) { o.Platform = "windows" }},
		{"no platform block", func(o *NodeBudgetObservation) { o.Darwin = nil }},
		{"platform names the other block", func(o *NodeBudgetObservation) { o.Platform = PlatformLinux }},
		{"unknown level", func(o *NodeBudgetObservation) { o.Level = "panic" }},
		{"admit reason while admitting", func(o *NodeBudgetObservation) { o.AdmitReasonSignal = SignalMemoryAvailable }},
		{"unknown admit reason", func(o *NodeBudgetObservation) { o.Admit = false; o.AdmitReasonSignal = "cpu.available" }},
		{"zero deadline", func(o *NodeBudgetObservation) { o.EvictDeadline = &metav1.Time{} }},
		{"negative capacity", func(o *NodeBudgetObservation) { o.Capacity.SwapUsedBytes = i64(-1) }},
		{"unknown unit", func(o *NodeBudgetObservation) { o.Signals[0].Unit = "pages" }},
		{"bytes signal in permille", func(o *NodeBudgetObservation) { o.Signals[0].Unit = UnitPermille }},
		{"compressor in bytes", func(o *NodeBudgetObservation) { o.Signals[1].Unit = UnitBytes }},
		{"pid in bytes", func(o *NodeBudgetObservation) { o.Signals[6].Unit = UnitBytes }},
		{"negative value", func(o *NodeBudgetObservation) { o.Signals[0].Value = i64(-1) }},
		{"release on the pressure side", func(o *NodeBudgetObservation) { o.Signals[0].WarnRelease = i64(1 * gib) }},
		{"memory.swap evict-soft rung", func(o *NodeBudgetObservation) { o.Signals[2].EvictSoftThreshold = i64(2 * gib) }},
		{"unknown trend", func(o *NodeBudgetObservation) { o.Signals[0].Trend = "flat" }},
		{"unknown limiting resource", func(o *NodeBudgetObservation) { o.Signals[0].LimitingResource = "gpu" }},
		{"shared with itself", func(o *NodeBudgetObservation) { o.Signals[5].SharedWith = SignalImagefsAvailable }},
		{"too many signals", func(o *NodeBudgetObservation) {
			for len(o.Signals) <= MaxBudgetSignals {
				o.Signals = append(o.Signals, BudgetSignal{ID: "x.available", Unit: UnitBytes})
			}
		}},
		{"negative recommendation", func(o *NodeBudgetObservation) { o.Recommendation.SafeAdditionalBytes = i64(-1) }},
		{"unknown basis", func(o *NodeBudgetObservation) { o.Recommendation.Basis = "cpu" }},
		{"unknown kill mode", func(o *NodeBudgetObservation) { o.KillMode.Mode = "maybe" }},
		{"unknown kill reason", func(o *NodeBudgetObservation) { o.KillMode.Reason = "because" }},
		{"negative arming episodes", func(o *NodeBudgetObservation) { o.KillMode.ArmingEpisodes = -1 }},
		{"would-kill without time", func(o *NodeBudgetObservation) {
			o.WouldKill = &WouldKill{Band: BandBatch, Reason: WouldKillMemoryFloor}
		}},
		{"would-kill unknown band", func(o *NodeBudgetObservation) {
			o.WouldKill = &WouldKill{Band: "best-effort", Reason: WouldKillMemoryFloor, At: sampleAt}
		}},
		{"repeated band", func(o *NodeBudgetObservation) { o.Counters[1].Band = BandCritical }},
		{"repeated window", func(o *NodeBudgetObservation) { o.Counters[2].Windows[1].Window = Window1h }},
		{"negative window count", func(o *NodeBudgetObservation) { o.Counters[2].Windows[0].Count = -1 }},
		{"policy name not an object name", func(o *NodeBudgetObservation) { o.Policy.Name = "Default Policy" }},
		{"unknown policy outcome", func(o *NodeBudgetObservation) { o.Policy.Outcome = "ignored" }},
		{"an applied policy with no policy in force", func(o *NodeBudgetObservation) { o.InForce = nil }},
		{"an applied policy with another version in force", func(o *NodeBudgetObservation) { o.InForce.ResourceVersion = "4700" }},
		{"an applied policy with another policy in force", func(o *NodeBudgetObservation) { o.InForce.Name = "other" }},
		{"an in-force name that is not an object name", func(o *NodeBudgetObservation) {
			o.Policy.Outcome = PolicyRejected
			o.InForce.Name = "Not A Name"
		}},
		{"unknown clamped field", func(o *NodeBudgetObservation) { o.Policy.ClampedFields = []PolicyField{"tier1 grace"} }},
		{"repeated clamped field", func(o *NodeBudgetObservation) {
			o.Policy.ClampedFields = []PolicyField{FieldTier1HeldSeconds, FieldTier1HeldSeconds}
		}},
		{"memorystatus above 1000 permille", func(o *NodeBudgetObservation) { o.Darwin.MemorystatusLevelPermille = i64(1001) }},
		{"negative compressor", func(o *NodeBudgetObservation) { o.Darwin.Compressor.BytesUsed = i64(-1) }},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			s := sampleDarwin().Status
			tc.mutate(&s.NodeBudgetObservation)
			if err := s.Validate(); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
	t.Run("a rejected policy keeps the last valid one in force", func(t *testing.T) {
		for _, inForce := range []*PolicyRef{{Name: "default", ResourceVersion: "4700"}, {Name: "older"}, nil} {
			s := sampleDarwin().Status
			s.Policy = &AppliedPolicy{Name: "default", ResourceVersion: "4711", Outcome: PolicyRejected}
			s.InForce = inForce
			if err := s.Validate(); err != nil {
				t.Errorf("rejected policy with inForce %+v: %v", inForce, err)
			}
		}
	})
	t.Run("refuses a condition reason outside its closed set", func(t *testing.T) {
		for _, c := range []metav1.Condition{
			{Type: ConditionReporting, Status: metav1.ConditionFalse, Reason: ReasonNodeReporting},
			{Type: ConditionPolicyRejected, Status: metav1.ConditionTrue, Reason: "BadPolicy"},
			{Type: ConditionPolicyClamped, Status: metav1.ConditionTrue, Reason: ReasonNoFieldsClamped},
			{Type: ConditionPolicyConflict, Status: metav1.ConditionFalse, Reason: ReasonMultiplePolicies},
		} {
			s := sampleDarwin().Status
			c.LastTransitionTime = sampleAt
			s.Conditions = []metav1.Condition{c}
			if err := s.Validate(); err == nil {
				t.Errorf("%s=%s reason %s was admitted", c.Type, c.Status, c.Reason)
			}
		}
	})
	t.Run("refuses an unknown writer shape", func(t *testing.T) {
		s := sampleDarwin().Status
		s.Writer.Shape = "sidecar"
		if err := s.Validate(); err == nil {
			t.Fatal("want an error, got nil")
		}
	})
	t.Run("refuses linux PSI above 1000 permille", func(t *testing.T) {
		s := sampleLinux().Status
		s.Linux.PSI.Memory.Full.Avg10Permille = i64(1001)
		if err := s.Validate(); err == nil {
			t.Fatal("want an error, got nil")
		}
	})
}

// assertNoSharedMemory fails when any pointer, slice or map reachable from a
// is the same memory in b: a DeepCopy that missed a field.
func assertNoSharedMemory(t *testing.T, path string, a, b reflect.Value) {
	t.Helper()
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() {
			return
		}
		if a.Pointer() == b.Pointer() {
			t.Errorf("%s: the copy shares the pointer", path)
			return
		}
		assertNoSharedMemory(t, path, a.Elem(), b.Elem())
	case reflect.Slice:
		if a.Len() > 0 && a.Pointer() == b.Pointer() {
			t.Errorf("%s: the copy shares the slice", path)
			return
		}
		for i := 0; i < a.Len(); i++ {
			assertNoSharedMemory(t, path+"[]", a.Index(i), b.Index(i))
		}
	case reflect.Map:
		if !a.IsNil() && a.Pointer() == b.Pointer() {
			t.Errorf("%s: the copy shares the map", path)
		}
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if a.Type().Field(i).IsExported() {
				assertNoSharedMemory(t, path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i))
			}
		}
	}
}

// TestNodeBudgetDeepCopyIsIndependent asserts a DeepCopy equals its source
// and shares no memory with it, over both samples (which between them set
// every field).
func TestNodeBudgetDeepCopyIsIndependent(t *testing.T) {
	t.Parallel()
	for p, nb := range samples() {
		t.Run(string(p), func(t *testing.T) {
			c := nb.DeepCopy()
			if !reflect.DeepEqual(c, nb) {
				t.Fatal("the copy differs from the source")
			}
			assertNoSharedMemory(t, "NodeBudget", reflect.ValueOf(nb), reflect.ValueOf(c))
			obj := nb.DeepCopyObject()
			if _, ok := obj.(*NodeBudget); !ok {
				t.Fatalf("DeepCopyObject returned %T", obj)
			}
		})
	}
	list := &NodeBudgetList{Items: []NodeBudget{*sampleDarwin(), *sampleLinux()}}
	lc := list.DeepCopy()
	if !reflect.DeepEqual(lc, list) {
		t.Fatal("the list copy differs from the source")
	}
	assertNoSharedMemory(t, "NodeBudgetList", reflect.ValueOf(list), reflect.ValueOf(lc))
	var nilNB *NodeBudget
	if nilNB.DeepCopy() != nil || nilNB.DeepCopyObject() != nil {
		t.Fatal("a nil NodeBudget must copy to nil")
	}
}

// TestBudgetRegistersTypes asserts the four kinds register under
// budget.k3sm.io/v1alpha1.
func TestBudgetRegistersTypes(t *testing.T) {
	t.Parallel()
	if GroupName != "budget.k3sm.io" || SchemeGroupVersion.Version != "v1alpha1" {
		t.Fatalf("SchemeGroupVersion = %v, want budget.k3sm.io/v1alpha1", SchemeGroupVersion)
	}
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, tc := range []struct {
		kind string
		obj  runtime.Object
	}{
		{"NodeBudget", &NodeBudget{}},
		{"NodeBudgetList", &NodeBudgetList{}},
		{"NodeBudgetPolicy", &NodeBudgetPolicy{}},
		{"NodeBudgetPolicyList", &NodeBudgetPolicyList{}},
	} {
		gvks, _, err := s.ObjectKinds(tc.obj)
		if err != nil {
			t.Fatalf("ObjectKinds(%s): %v", tc.kind, err)
		}
		want := schema.GroupVersionKind{Group: GroupName, Version: "v1alpha1", Kind: tc.kind}
		if len(gvks) != 1 || gvks[0] != want {
			t.Fatalf("%s registered as %v, want %v", tc.kind, gvks, want)
		}
	}
	if got := Resource("nodebudgets"); got.Group != GroupName || got.Resource != "nodebudgets" {
		t.Fatalf("Resource(nodebudgets) = %v", got)
	}
}

// TestBudgetVocabulary pins the wire strings and the signal page codes,
// which are part of the page ABI and append-only.
func TestBudgetVocabulary(t *testing.T) {
	t.Parallel()
	codes := []struct {
		id   SignalID
		code uint64
		desc bool
		unit SignalUnit
	}{
		{"memory.available", 1, true, "bytes"},
		{"memory.compressor", 2, false, "permille"},
		{"memory.swap", 3, false, "bytes"},
		{"swapfs.available", 4, true, "bytes"},
		{"nodefs.available", 5, true, "bytes"},
		{"imagefs.available", 6, true, "bytes"},
		{"pid.available", 7, true, "count"},
	}
	if got := Signals(); len(got) != len(codes) {
		t.Fatalf("Signals() = %v, want %d signals", got, len(codes))
	}
	for i, tc := range codes {
		if Signals()[i] != tc.id || tc.id.PageCode() != tc.code || !tc.id.Valid() || IsDescending(tc.id) != tc.desc {
			t.Errorf("%s: code %d valid %v descending %v, want code %d descending %v",
				tc.id, tc.id.PageCode(), tc.id.Valid(), IsDescending(tc.id), tc.code, tc.desc)
		}
		if tc.id.Unit() != tc.unit {
			t.Errorf("%s unit = %q, want %q", tc.id, tc.id.Unit(), tc.unit)
		}
		if id, ok := SignalForPageCode(tc.code); !ok || id != tc.id {
			t.Errorf("SignalForPageCode(%d) = %q, %v, want %q", tc.code, id, ok, tc.id)
		}
	}
	for _, c := range []uint64{0, 8, 1 << 40} {
		if _, ok := SignalForPageCode(c); ok {
			t.Errorf("SignalForPageCode(%d) is known; 0 is an empty slot and 8+ are unassigned", c)
		}
	}
	if SignalID("cpu.available").Valid() || SignalID("").PageCode() != 0 {
		t.Error("an unknown signal is valid")
	}
	for _, tc := range []struct{ got, want string }{
		{AnnotationStatus, "budget.k3sm.io/status"},
		{AnnotationPolicy, "budget.k3sm.io/policy"},
		{AnnotationBand, "budget.k3sm.io/band"},
		{AnnotationPage, "budget.k3sm.io/page"},
		{TaintAdmitStop, "budget.k3sm.io/admit-stop"},
		{TaintAdmitStopEffect, "PreferNoSchedule"},
		{ReasonAnnotationTooLarge, "AnnotationTooLarge"},
		{ReasonNodeReporting, "NodeReporting"},
		{ReasonPolicyInvalid, "PolicyInvalid"},
		{ReasonMergedLadderInverted, "MergedLadderInverted"},
		{ReasonPolicyAccepted, "PolicyAccepted"},
		{ReasonFieldsClamped, "FieldsClamped"},
		{ReasonNoFieldsClamped, "NoFieldsClamped"},
		{ReasonSinglePolicy, "SinglePolicy"},
		{string(TrendUnknown) + string(ResourceUnknown) + string(KillReasonUnknown) + string(WouldKillUnknown) + string(CounterUnknown),
			"unknownunknownunknownunknownunknown"},
		{ConditionReporting, "Reporting"},
		{ConditionPolicyRejected, "PolicyRejected"},
		{ConditionPolicyClamped, "PolicyClamped"},
		{ConditionPolicyConflict, "PolicyConflict"},
		{ReasonNodeNotReporting, "NodeNotReporting"},
		{ReasonAnnotationMalformed, "AnnotationMalformed"},
		{ReasonSchemaUnsupported, "SchemaUnsupported"},
		{ReasonMultiplePolicies, "MultiplePolicies"},
		{string(WriterAnnotationRelay), "annotation-relay"},
		{string(WriterOwnNode), "own-node"},
		{string(KillModeAuto) + string(KillModeOn) + string(KillModeOff), "autoonoff"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
	wantFields := []PolicyField{"tier1GraceSeconds", "tier1HeldSeconds", "tier1MaxDeliveriesPerHour", "tier1CooloffSeconds",
		"reserves.cpuSystemReserveMilli", "reserves.memSystemReserveBytes", "reserves.memEvictionReserveBytes"}
	if got := PolicyFields(); !reflect.DeepEqual(got, wantFields) || len(got) > MaxClampedFields {
		t.Errorf("PolicyFields() = %v, want %v (and at most MaxClampedFields)", got, wantFields)
	}
	if SignalID("cpu.available").Unit() != "" {
		t.Error("an unknown signal has a unit")
	}
	for _, tc := range []struct {
		typ, status string
		want        []string
	}{
		{"Reporting", "True", []string{"NodeReporting"}},
		{"Reporting", "False", []string{"NodeNotReporting", "AnnotationMalformed", "SchemaUnsupported", "AnnotationTooLarge"}},
		{"PolicyRejected", "True", []string{"PolicyInvalid", "MergedLadderInverted"}},
		{"PolicyRejected", "False", []string{"PolicyAccepted"}},
		{"PolicyClamped", "True", []string{"FieldsClamped"}},
		{"PolicyClamped", "False", []string{"NoFieldsClamped"}},
		{"PolicyConflict", "True", []string{"MultiplePolicies"}},
		{"PolicyConflict", "False", []string{"SinglePolicy"}},
		{"PolicyConflict", "Unknown", nil},
		{"Ready", "True", nil},
	} {
		if got := ConditionReasons(tc.typ, tc.status); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ConditionReasons(%s, %s) = %v, want %v", tc.typ, tc.status, got, tc.want)
		}
	}
	if b := Bands(); len(b) != 3 || b[0] != BandCritical || b[1] != BandStandard || b[2] != BandBatch {
		t.Errorf("Bands() = %v, want critical, standard, batch", b)
	}
}
