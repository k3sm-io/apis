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
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// goldenReport returns the observation of a golden status document as a
// report payload, read from testdata so the gate decodes the committed
// conformance bytes.
func goldenReport(t *testing.T, p Platform) NodeBudgetReport {
	t.Helper()
	b, err := os.ReadFile(goldenStatusFiles[p])
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var nb NodeBudget
	if err := json.Unmarshal(b, &nb); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	return NodeBudgetReport{Schema: BudgetAnnotationSchema, NodeBudgetObservation: nb.Status.NodeBudgetObservation}
}

// marshalRaw marshals v without Encode's checks, the bytes a careless or
// hostile writer would put in the annotation.
func marshalRaw(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// inject puts a top-level member into a JSON object.
func inject(obj, member string) string { return "{" + member + "," + obj[1:] }

// TestNodeBudgetAnnotationStrictDecode is the gate of the node-to-relay
// payload. The budget.k3sm.io/status annotation is written by the node on its
// own Node, so the relay treats it as untrusted: a size bound checked before
// parsing, the schema read on its own, unknown, repeated and case-folded keys
// and trailing data refused, then the observation validated. Every row that
// Parse refuses, Encode refuses too, so a writer can never produce a value its
// reader rejects.
func TestNodeBudgetAnnotationStrictDecode(t *testing.T) {
	t.Parallel()

	for _, p := range []Platform{PlatformDarwin, PlatformLinux} {
		t.Run("decodes the "+string(p)+" golden", func(t *testing.T) {
			r := goldenReport(t, p)
			s, err := r.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			got, err := ParseNodeBudgetReport(s)
			if err != nil {
				t.Fatalf("ParseNodeBudgetReport: %v", err)
			}
			if a, b := marshalRaw(t, got), marshalRaw(t, r); a != b {
				t.Fatalf("round trip changed the report:\n got = %s\nwant = %s", a, b)
			}
		})
	}

	t.Run("decodes a payload padded to exactly the bound", func(t *testing.T) {
		r := goldenReport(t, PlatformDarwin)
		s := marshalRaw(t, &r)
		s = s[:len(s)-1] + strings.Repeat(" ", MaxBudgetAnnotationBytes-len(s)) + "}"
		if len(s) != MaxBudgetAnnotationBytes {
			t.Fatalf("padded to %d bytes, want %d", len(s), MaxBudgetAnnotationBytes)
		}
		if _, err := ParseNodeBudgetReport(s); err != nil {
			t.Fatalf("a payload of exactly %d bytes is refused: %v", MaxBudgetAnnotationBytes, err)
		}
	})

	// Rows that a Go value can express: Parse refuses the raw bytes and
	// Encode refuses the value, with the same sentinel.
	for _, tc := range []struct {
		name   string
		mutate func(r *NodeBudgetReport)
		want   error
	}{
		{"schema 0", func(r *NodeBudgetReport) { r.Schema = 0 }, ErrUnknownBudgetSchema},
		{"schema 2", func(r *NodeBudgetReport) { r.Schema = 2 }, ErrUnknownBudgetSchema},
		{"duplicate signal", func(r *NodeBudgetReport) { r.Signals = append(r.Signals, r.Signals[0]) }, ErrInvalidBudgetAnnotation},
		{"unknown signal", func(r *NodeBudgetReport) { r.Signals[0].ID = "memory.free" }, ErrInvalidBudgetAnnotation},
		{"both platform blocks", func(r *NodeBudgetReport) {
			r.Linux = &LinuxSignals{MemoryEvents: &LinuxMemoryEvents{OOM: i64(0)}}
		}, ErrInvalidBudgetAnnotation},
		{"no platform block", func(r *NodeBudgetReport) { r.Darwin = nil }, ErrInvalidBudgetAnnotation},
		{"falling ladder misordered", func(r *NodeBudgetReport) {
			// memory.available falls under pressure: warn must sit above
			// admit-stop.
			r.Signals[0].WarnThreshold, r.Signals[0].AdmitStopThreshold = i64(1*gib), i64(2*gib)
		}, ErrInvalidBudgetAnnotation},
		{"rising ladder misordered", func(r *NodeBudgetReport) {
			// memory.compressor rises under pressure: warn must sit below
			// admit-stop.
			r.Signals[1].WarnThreshold, r.Signals[1].AdmitStopThreshold = i64(650), i64(600)
		}, ErrInvalidBudgetAnnotation},
		{"falling ladder written as rising", func(r *NodeBudgetReport) {
			s := &r.Signals[0]
			s.WarnThreshold, s.AdmitStopThreshold, s.CriticalThreshold, s.EvictSoftThreshold, s.EvictHardThreshold = i64(1), i64(2), i64(3), i64(4), i64(5)
			s.WarnRelease, s.AdmitStopRelease, s.CriticalRelease, s.EvictSoftRelease, s.EvictHardRelease = nil, nil, nil, nil, nil
		}, ErrInvalidBudgetAnnotation},
		{"negative counter", func(r *NodeBudgetReport) { r.Counters[2].EvictionsTotal = -1 }, ErrInvalidBudgetAnnotation},
		{"unknown level", func(r *NodeBudgetReport) { r.Level = "red" }, ErrInvalidBudgetAnnotation},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			r := goldenReport(t, PlatformDarwin)
			tc.mutate(&r)
			raw := marshalRaw(t, &r)
			if _, err := ParseNodeBudgetReport(raw); !errors.Is(err, tc.want) {
				t.Errorf("ParseNodeBudgetReport error = %v, want %v", err, tc.want)
			}
			if _, err := r.Encode(); !errors.Is(err, tc.want) {
				t.Errorf("Encode error = %v, want %v", err, tc.want)
			}
		})
	}

	// Rows only bytes can express. A Go value cannot carry them, so the Encode
	// side is checked by decoding the bytes leniently and showing that
	// whatever Encode then emits is not the refused payload and does parse.
	for _, tc := range []struct {
		name string
		raw  func(valid string) string
		want error
		msg  string
	}{
		{"bound+1 bytes, before parsing", func(string) string {
			return `{"schema":1,` + strings.Repeat(" ", MaxBudgetAnnotationBytes+1-len(`{"schema":1,`))
		}, ErrBudgetAnnotationTooLarge, "16385 bytes"},
		{"missing schema", func(v string) string { return strings.Replace(v, `"schema":1,`, "", 1) }, ErrUnknownBudgetSchema, "no schema"},
		{"schema as a string", func(v string) string { return strings.Replace(v, `"schema":1`, `"schema":"1"`, 1) }, ErrUnknownBudgetSchema, ""},
		{"schema 1.0", func(v string) string { return strings.Replace(v, `"schema":1`, `"schema":1.0`, 1) }, ErrUnknownBudgetSchema, ""},
		{"schema null", func(v string) string { return strings.Replace(v, `"schema":1`, `"schema":null`, 1) }, ErrUnknownBudgetSchema, "no schema"},
		{"an unknown structural enum value", func(v string) string {
			return strings.Replace(v, `"unit":"bytes"`, `"unit":"pages"`, 1)
		}, ErrInvalidBudgetAnnotation, ""},
		{"not JSON", func(string) string { return "level=ok" }, ErrInvalidBudgetAnnotation, ""},
		{"unknown field", func(v string) string { return inject(v, `"extra":1`) }, ErrInvalidBudgetAnnotation, "unknown field"},
		{"unknown nested field", func(v string) string {
			return strings.Replace(v, `"id":"memory.available"`, `"id":"memory.available","note":"x"`, 1)
		}, ErrInvalidBudgetAnnotation, "unknown field"},
		{"trailing bytes", func(v string) string { return v + ` {}` }, ErrInvalidBudgetAnnotation, ""},
		{"trailing garbage", func(v string) string { return v + `x` }, ErrInvalidBudgetAnnotation, ""},
		{"writer in the payload", func(v string) string {
			return inject(v, `"writer":{"shape":"annotation-relay"}`)
		}, ErrInvalidBudgetAnnotation, "unknown field"},
		{"conditions in the payload", func(v string) string {
			return inject(v, `"conditions":[{"type":"Reporting","status":"True","lastTransitionTime":"2026-10-19T12:00:00Z","reason":"Forged","message":""}]`)
		}, ErrInvalidBudgetAnnotation, "unknown field"},
		{"a pod-identity key", func(v string) string { return inject(v, `"pod":"default/web-0"`) }, ErrInvalidBudgetAnnotation, "unknown field"},
		{"a victim key in the would-kill record", func(v string) string {
			return inject(v, `"wouldKill":{"band":"batch","reason":"held-pressure","at":"2026-10-19T12:00:00Z","victimPod":"default/web-0"}`)
		}, ErrInvalidBudgetAnnotation, "unknown field"},
		{"a case-folded key", func(v string) string { return strings.Replace(v, `"level":"ok"`, `"Level":"ok"`, 1) }, ErrInvalidBudgetAnnotation, "unknown field"},
		{"a repeated key", func(v string) string { return inject(v, `"level":"evicting"`) }, ErrInvalidBudgetAnnotation, "repeated"},
		{"a repeated nested key", func(v string) string {
			return strings.Replace(v, `"band":"critical"`, `"band":"critical","band":"batch"`, 1)
		}, ErrInvalidBudgetAnnotation, "repeated"},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			r := goldenReport(t, PlatformDarwin)
			valid, err := r.Encode()
			if err != nil {
				t.Fatalf("Encode the base: %v", err)
			}
			raw := tc.raw(valid)
			if raw == valid {
				t.Fatal("the row did not change the payload")
			}
			_, err = ParseNodeBudgetReport(raw)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ParseNodeBudgetReport error = %v, want %v", err, tc.want)
			}
			if tc.msg != "" && !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("error %q does not say %q", err, tc.msg)
			}
			var lenient NodeBudgetReport
			if json.Unmarshal([]byte(raw), &lenient) != nil {
				return
			}
			out, err := lenient.Encode()
			if err != nil {
				return
			}
			if out == raw {
				t.Fatal("Encode emitted the payload Parse refuses")
			}
			if _, err := ParseNodeBudgetReport(out); err != nil {
				t.Fatalf("Encode emitted a payload Parse refuses: %v", err)
			}
		})
	}
}

// maximalReport is the largest report the contract admits: every signal with
// every field at a 19-digit value, every band with every window, the longest
// strings, and the larger platform block.
func maximalReport() NodeBudgetReport {
	const m = int64(math.MaxInt64)
	var sigs []BudgetSignal
	for _, id := range Signals() {
		s := BudgetSignal{
			ID: id, Unit: id.Unit(), Value: i64(m),
			GraceSeconds: i64(m), MinReclaimBytes: i64(m), WindowSeconds: i64(m), EtaToNextRungSeconds: i64(m),
			Trend: TrendFalling, LimitingResource: ResourceImagefs, SharedWith: SignalImagefsAvailable,
		}
		if id == SignalImagefsAvailable {
			s.SharedWith = SignalNodefsAvailable
		}
		r := []*int64{i64(m - 9), i64(m - 8), i64(m - 7), i64(m - 6), i64(m - 5)}
		rel := []*int64{i64(m - 14), i64(m - 13), i64(m - 12), i64(m - 11), i64(m - 10)}
		if IsDescending(id) {
			r = []*int64{i64(m - 5), i64(m - 6), i64(m - 7), i64(m - 8), i64(m - 9)}
			rel = []*int64{i64(m), i64(m - 1), i64(m - 2), i64(m - 3), i64(m - 4)}
		}
		s.WarnThreshold, s.AdmitStopThreshold, s.CriticalThreshold, s.EvictSoftThreshold, s.EvictHardThreshold = r[0], r[1], r[2], r[3], r[4]
		s.WarnRelease, s.AdmitStopRelease, s.CriticalRelease, s.EvictSoftRelease, s.EvictHardRelease = rel[0], rel[1], rel[2], rel[3], rel[4]
		if id == SignalMemorySwap {
			s.EvictSoftThreshold, s.EvictSoftRelease = nil, nil
		}
		sigs = append(sigs, s)
	}
	var counters []BandCounters
	for _, b := range Bands() {
		c := BandCounters{Band: b, EvictionsTotal: m, OOMKillsTotal: m, KernelKillsTotal: m, RestartsTotal: m, MemHighEventsTotal: m, Tier1RefusedTotal: m}
		for _, w := range []CounterWindow{Window1h, Window24h} {
			for _, reason := range []CounterReason{CounterEviction, CounterOOMKill, CounterKernelKill, CounterRestart, CounterMemHigh, CounterTier1Refused} {
				c.Windows = append(c.Windows, WindowCount{Window: w, Reason: reason, Count: m})
			}
		}
		counters = append(counters, c)
	}
	clamped := PolicyFields()
	line := func() *PSILine { return &PSILine{Avg10Permille: i64(1000), Avg60Permille: i64(1000), TotalUs: i64(m)} }
	res := func() *PSIResource { return &PSIResource{Some: line(), Full: line()} }
	at := metav1.NewTime(time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC))
	return NodeBudgetReport{
		Schema: BudgetAnnotationSchema,
		NodeBudgetObservation: NodeBudgetObservation{
			ObservedAt: at, ProducerGeneration: m, Platform: PlatformLinux, Level: LevelEvicting,
			AdmitReasonSignal: SignalImagefsAvailable, EvictDeadline: &at,
			Capacity: BudgetCapacity{
				CPUCapacityMilli: i64(m), CPUSystemReserveMilli: i64(m), MemCapacityBytes: i64(m), MemSystemReserveBytes: i64(m),
				MemEvictionReserveBytes: i64(m), MemAllocatableBytes: i64(m), SwapTotalBytes: i64(m), SwapUsedBytes: i64(m),
			},
			Signals:        sigs,
			Recommendation: BudgetRecommendation{SafeAdditionalBytes: i64(m), Basis: SignalImagefsAvailable},
			KillMode:       KillMode{Mode: KillModeAuto, Armed: true, ArmingEpisodes: math.MaxInt32, Reason: KillReasonDisarmFile},
			WouldKill:      &WouldKill{Band: BandStandard, Reason: WouldKillHeldPressure, At: at},
			Counters:       counters,
			Policy: &AppliedPolicy{
				PolicyRef: PolicyRef{Name: strings.Repeat("a", 253), ResourceVersion: strings.Repeat("9", MaxPolicyResourceVersionBytes)},
				Outcome:   PolicyRejected, ClampedFields: clamped,
			},
			InForce: &PolicyRef{Name: strings.Repeat("b", 253), ResourceVersion: strings.Repeat("8", MaxPolicyResourceVersionBytes)},
			Linux: &LinuxSignals{
				PSI:          &LinuxPSI{CPU: res(), Memory: res(), IO: res()},
				MemoryEvents: &LinuxMemoryEvents{High: i64(m), Max: i64(m), OOM: i64(m), OOMKill: i64(m)},
			},
		},
	}
}

// TestBudgetAnnotationMaximalReportHeadroom asserts the largest report the
// contract admits encodes within three quarters of MaxBudgetAnnotationBytes,
// so the bound keeps 25% headroom for fields a later minor appends.
func TestBudgetAnnotationMaximalReportHeadroom(t *testing.T) {
	t.Parallel()
	r := maximalReport()
	s, err := r.Encode()
	if err != nil {
		t.Fatalf("the maximal report does not encode: %v", err)
	}
	const limit = MaxBudgetAnnotationBytes * 3 / 4
	t.Logf("maximal report: %d bytes (limit %d, bound %d)", len(s), limit, MaxBudgetAnnotationBytes)
	if len(s) > limit {
		t.Fatalf("the maximal report is %d bytes, over %d (75%% of the %d bound)", len(s), limit, MaxBudgetAnnotationBytes)
	}
	if MaxBudgetAnnotationBytes != 16384 || BudgetAnnotationSchema != 1 {
		t.Fatalf("bound %d schema %d, want 16384 and 1", MaxBudgetAnnotationBytes, BudgetAnnotationSchema)
	}
}

// sampleDesired is a relayed policy that sets every field.
func sampleDesired() NodeBudgetDesiredPolicy {
	return NodeBudgetDesiredPolicy{
		Schema: BudgetAnnotationSchema, Name: "linux-workers", ResourceVersion: "88",
		Spec: samplePolicy().Spec,
	}
}

// TestNodeBudgetDesiredPolicyStrictDecode is the relay-to-node payload: the
// same strict decoder, the policy validated, and a value below a floor
// admitted for ApplyFloors to clamp rather than refused.
func TestNodeBudgetDesiredPolicyStrictDecode(t *testing.T) {
	t.Parallel()
	d := sampleDesired()
	s, err := d.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := ParseNodeBudgetDesiredPolicy(s)
	if err != nil {
		t.Fatalf("ParseNodeBudgetDesiredPolicy: %v", err)
	}
	if a, b := marshalRaw(t, got), marshalRaw(t, d); a != b {
		t.Fatalf("round trip changed the payload:\n got = %s\nwant = %s", a, b)
	}

	below := sampleDesired()
	below.Spec.Tier1GraceSeconds = i64(1)
	if _, err := below.Encode(); err != nil {
		t.Fatalf("a grace below the floor is clamped by the node, not refused: %v", err)
	}

	for _, tc := range []struct {
		name string
		raw  func(valid string) string
		want error
	}{
		{"missing schema", func(v string) string { return strings.Replace(v, `"schema":1,`, "", 1) }, ErrUnknownBudgetSchema},
		{"unknown spec field", func(v string) string {
			return strings.Replace(v, `"spec":{`, `"spec":{"killOrder":["batch"],`, 1)
		}, ErrInvalidBudgetAnnotation},
		{"a status block", func(v string) string { return inject(v, `"status":{}`) }, ErrInvalidBudgetAnnotation},
		{"case-folded key", func(v string) string { return strings.Replace(v, `"name":`, `"Name":`, 1) }, ErrInvalidBudgetAnnotation},
		{"unknown mode", func(v string) string { return strings.Replace(v, `"mode":"on"`, `"mode":"always"`, 1) }, ErrInvalidBudgetAnnotation},
		{"misordered ladder", func(v string) string {
			return strings.Replace(v, `"warnThreshold":500`, `"warnThreshold":900`, 1)
		}, ErrInvalidBudgetAnnotation},
		{"name not an object name", func(v string) string {
			return strings.Replace(v, `"name":"linux-workers"`, `"name":"Linux Workers"`, 1)
		}, ErrInvalidBudgetAnnotation},
		{"empty resourceVersion", func(v string) string {
			return strings.Replace(v, `"resourceVersion":"88"`, `"resourceVersion":""`, 1)
		}, ErrInvalidBudgetAnnotation},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			raw := tc.raw(s)
			if raw == s {
				t.Fatal("the row did not change the payload")
			}
			if _, err := ParseNodeBudgetDesiredPolicy(raw); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestBudgetAnnotationSkewTolerance pins how a report from a newer producer
// on the same schema is read. Producer-vocabulary values this release does
// not know decode to their "unknown" constants (the original string is
// dropped, so no free text reaches the status), and windowed counts of
// unknown reasons are summed per window. Structural enum values this release
// does not know still refuse the payload: adding one bumps the schema.
func TestBudgetAnnotationSkewTolerance(t *testing.T) {
	t.Parallel()
	base := func(t *testing.T) map[string]any {
		r := goldenReport(t, PlatformLinux)
		s, err := r.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	sig := func(m map[string]any) map[string]any { return m["signals"].([]any)[0].(map[string]any) }
	batch := func(m map[string]any) map[string]any { return m["counters"].([]any)[0].(map[string]any) }

	t.Run("tolerates new producer vocabulary", func(t *testing.T) {
		m := base(t)
		sig(m)["trend"] = "accelerating"
		sig(m)["limitingResource"] = "gpu-memory"
		m["killMode"].(map[string]any)["reason"] = "maintenance-window"
		m["wouldKill"].(map[string]any)["reason"] = "psi-full"
		batch(m)["windows"] = []any{
			map[string]any{"window": "1h", "reason": "eviction", "count": 2},
			map[string]any{"window": "1h", "reason": "swap-storm", "count": 3},
			map[string]any{"window": "1h", "reason": "page-fault-storm", "count": 4},
			map[string]any{"window": "24h", "reason": "swap-storm", "count": 5},
		}
		raw := marshalRaw(t, m)
		r, err := ParseNodeBudgetReport(raw)
		if err != nil {
			t.Fatalf("a report with new producer vocabulary is refused: %v", err)
		}
		s := r.Signals[0]
		if s.Trend != TrendUnknown || s.LimitingResource != ResourceUnknown ||
			r.KillMode.Reason != KillReasonUnknown || r.WouldKill.Reason != WouldKillUnknown {
			t.Fatalf("unknown values decoded to %q %q %q %q", s.Trend, s.LimitingResource, r.KillMode.Reason, r.WouldKill.Reason)
		}
		want := []WindowCount{
			{Window: Window1h, Reason: CounterEviction, Count: 2},
			{Window: Window1h, Reason: CounterUnknown, Count: 7},
			{Window: Window24h, Reason: CounterUnknown, Count: 5},
		}
		if got := r.Counters[0].Windows; !reflect.DeepEqual(got, want) {
			t.Fatalf("windows = %+v, want %+v", got, want)
		}
		out, err := r.Encode()
		if err != nil {
			t.Fatalf("the tolerated report does not re-encode: %v", err)
		}
		for _, leak := range []string{"accelerating", "gpu-memory", "maintenance-window", "psi-full", "swap-storm", "page-fault-storm"} {
			if strings.Contains(out, leak) {
				t.Errorf("the producer's string %q survived into the re-encoded report", leak)
			}
		}
	})

	t.Run("saturates a merged count", func(t *testing.T) {
		got := mergeUnknownWindows([]WindowCount{
			{Window: Window1h, Reason: CounterUnknown, Count: math.MaxInt64 - 1},
			{Window: Window1h, Reason: CounterUnknown, Count: 5},
		})
		if len(got) != 1 || got[0].Count != math.MaxInt64 {
			t.Fatalf("merged = %+v, want one entry saturated at MaxInt64", got)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(m map[string]any)
	}{
		{"signal", func(m map[string]any) { sig(m)["id"] = "gpu.available" }},
		{"unit", func(m map[string]any) { sig(m)["unit"] = "pages" }},
		{"level", func(m map[string]any) { m["level"] = "red" }},
		{"band", func(m map[string]any) { batch(m)["band"] = "best-effort" }},
		{"platform", func(m map[string]any) { m["platform"] = "windows" }},
		{"policy outcome", func(m map[string]any) { m["policy"].(map[string]any)["outcome"] = "deferred" }},
		{"clamped field", func(m map[string]any) { m["policy"].(map[string]any)["clampedFields"] = []any{"spec.thresholds"} }},
		{"kill mode", func(m map[string]any) { m["killMode"].(map[string]any)["mode"] = "observe" }},
		{"counter window", func(m map[string]any) { batch(m)["windows"].([]any)[0].(map[string]any)["window"] = "7d" }},
	} {
		t.Run("refuses an unknown structural "+tc.name, func(t *testing.T) {
			m := base(t)
			tc.mutate(m)
			if _, err := ParseNodeBudgetReport(marshalRaw(t, m)); !errors.Is(err, ErrInvalidBudgetAnnotation) {
				t.Fatalf("error = %v, want ErrInvalidBudgetAnnotation", err)
			}
		})
	}

	t.Run("an open enum still refuses a non-string", func(t *testing.T) {
		m := base(t)
		sig(m)["trend"] = 3
		if _, err := ParseNodeBudgetReport(marshalRaw(t, m)); !errors.Is(err, ErrInvalidBudgetAnnotation) {
			t.Fatalf("error = %v, want ErrInvalidBudgetAnnotation", err)
		}
	})
}
