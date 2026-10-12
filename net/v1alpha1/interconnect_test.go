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

package netv1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k3sm.io/apis/config/crd"
)

// interconnectGolden is the golden fixture of a fully-populated Interconnect.
const interconnectGolden = "interconnect.golden.json"

// sampleInterconnect is a fully-populated, armed Interconnect with two up
// members in one segment.
func sampleInterconnect() *Interconnect {
	return &Interconnect{
		TypeMeta:   metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "Interconnect"},
		ObjectMeta: metav1.ObjectMeta{Name: MediumThunderbolt, ResourceVersion: "3"},
		Spec: InterconnectSpec{
			Medium:            MediumThunderbolt,
			Fallback:          InterconnectFallbackNetwork,
			Enabled:           true,
			ExcludedNodes:     []string{"studio-3"},
			AllowedNamespaces: []string{"mlx", "default"},
		},
		Status: InterconnectStatus{
			Members: []InterconnectMember{
				{
					Node: "studio-1", Index: 0, Address: "169.254.0.1", State: InterconnectMemberUp,
					SpeedGbps: 80, Segment: "seg-0", ReachablePeers: []string{"studio-2"},
					LastTransition: fixedLinkTime.DeepCopy(),
				},
				{
					Node: "studio-2", Index: 1, Address: "169.254.0.9", State: InterconnectMemberUp,
					SpeedGbps: 80, Segment: "seg-0", ReachablePeers: []string{"studio-1"},
					LastTransition: fixedLinkTime.DeepCopy(),
				},
			},
			Segments:     []InterconnectSegment{{ID: "seg-0", Nodes: []string{"studio-1", "studio-2"}}},
			AttachedPods: 2,
			ArmedBy:      "system:admin",
			ArmedAt:      fixedLinkTime.DeepCopy(),
			Conditions: []metav1.Condition{{
				Type: "Ready", Status: metav1.ConditionTrue, Reason: "SegmentsUp",
				Message: "1 segment, 2 members up", LastTransitionTime: fixedLinkTime, ObservedGeneration: 2,
			}},
		},
	}
}

// TestInterconnectValidate is the Validate table: the medium enum, the
// fallback enum (empty is the CRD default), the node-name and namespace-name
// shapes, repeats, and the one-object-per-medium name.
func TestInterconnectValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*Interconnect)
		wantErr bool
	}{
		{"ok", func(*Interconnect) {}, false},
		{"minimal", func(ic *Interconnect) { ic.Spec = InterconnectSpec{Medium: MediumThunderbolt} }, false},
		{"unnamed", func(ic *Interconnect) { ic.Name = "" }, false},
		{"fallback none", func(ic *Interconnect) { ic.Spec.Fallback = InterconnectFallbackNone }, false},
		{"fallback empty is the default", func(ic *Interconnect) { ic.Spec.Fallback = "" }, false},
		{"disabled", func(ic *Interconnect) { ic.Spec.Enabled = false }, false},
		{"dotted node name", func(ic *Interconnect) { ic.Spec.ExcludedNodes = []string{"mac.lan"} }, false},
		{"empty medium", func(ic *Interconnect) { ic.Spec.Medium = ""; ic.Name = "" }, true},
		{"unknown medium", func(ic *Interconnect) { ic.Spec.Medium = "usb4"; ic.Name = "usb4" }, true},
		{"medium wrong case", func(ic *Interconnect) { ic.Spec.Medium = "Thunderbolt"; ic.Name = "" }, true},
		{"name not the medium", func(ic *Interconnect) { ic.Name = "cable" }, true},
		{"unknown fallback", func(ic *Interconnect) { ic.Spec.Fallback = "mesh" }, true},
		{"fallback wrong case", func(ic *Interconnect) { ic.Spec.Fallback = "Network" }, true},
		{"empty excluded node", func(ic *Interconnect) { ic.Spec.ExcludedNodes = []string{""} }, true},
		{"uppercase excluded node", func(ic *Interconnect) { ic.Spec.ExcludedNodes = []string{"Studio"} }, true},
		{"repeated excluded node", func(ic *Interconnect) { ic.Spec.ExcludedNodes = []string{"a", "a"} }, true},
		{"overlong excluded node", func(ic *Interconnect) { ic.Spec.ExcludedNodes = []string{strings.Repeat("a", 254)} }, true},
		{"empty namespace", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = []string{""} }, true},
		{"uppercase namespace", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = []string{"MLX"} }, true},
		{"dotted namespace", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = []string{"a.b"} }, true},
		{"leading dash namespace", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = []string{"-mlx"} }, true},
		{"wildcard namespace", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = []string{"*"} }, true},
		{"overlong namespace", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = []string{strings.Repeat("a", 64)} }, true},
		{"repeated namespace", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = []string{"mlx", "mlx"} }, true},
		{"64 excluded nodes", func(ic *Interconnect) { ic.Spec.ExcludedNodes = peers(MaxInterconnectListItems) }, false},
		{"65 excluded nodes", func(ic *Interconnect) { ic.Spec.ExcludedNodes = peers(MaxInterconnectListItems + 1) }, true},
		{"64 namespaces", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = peers(MaxInterconnectListItems) }, false},
		{"65 namespaces", func(ic *Interconnect) { ic.Spec.AllowedNamespaces = peers(MaxInterconnectListItems + 1) }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ic := sampleInterconnect()
			tc.mutate(ic)
			err := ic.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidInterconnect) {
					t.Fatalf("Validate() = %v, want an ErrInvalidInterconnect error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestInterconnectSpecHelpers pins the fallback default and the empty-means-all
// namespace rule.
func TestInterconnectSpecHelpers(t *testing.T) {
	t.Parallel()
	if got := (InterconnectSpec{}).EffectiveFallback(); got != InterconnectFallbackNetwork {
		t.Errorf("EffectiveFallback() of empty = %q, want network", got)
	}
	if got := (InterconnectSpec{Fallback: InterconnectFallbackNone}).EffectiveFallback(); got != InterconnectFallbackNone {
		t.Errorf("EffectiveFallback() = %q, want none", got)
	}
	if !(InterconnectSpec{}).NamespaceAllowed("anything") {
		t.Error("empty allowedNamespaces must allow every namespace")
	}
	s := InterconnectSpec{AllowedNamespaces: []string{"mlx"}}
	if !s.NamespaceAllowed("mlx") || s.NamespaceAllowed("default") {
		t.Error("allowedNamespaces [mlx] must allow mlx only")
	}
}

// TestInterconnectSlotAddress pins the slot derivation at its boundaries, the
// form-B ceiling, and that every address lies in the one connected /24.
func TestInterconnectSlotAddress(t *testing.T) {
	t.Parallel()
	prefix := netip.MustParsePrefix(InterconnectCIDR)
	if prefix.String() != "169.254.0.0/24" {
		t.Fatalf("InterconnectCIDR = %s, want 169.254.0.0/24", prefix)
	}
	if MaxInterconnectNodeIndex != 30 || MaxInterconnectSlot != 7 ||
		InterconnectSlotNode != 0 || InterconnectSlotMLXRank != 1 || InterconnectSlotFirstPod != 2 {
		t.Fatal("interconnect slot constants moved; the mapping is configured on cables and must not change")
	}
	cases := []struct {
		idx, slot int
		want      string
		wantErr   bool
	}{
		{idx: 0, slot: 0, want: "169.254.0.1"},
		{idx: 0, slot: 1, want: "169.254.0.2"},
		{idx: 0, slot: 7, want: "169.254.0.8"},
		{idx: 1, slot: 0, want: "169.254.0.9"},
		{idx: 30, slot: 0, want: "169.254.0.241"},
		{idx: 30, slot: 1, want: "169.254.0.242"},
		{idx: 30, slot: 7, want: "169.254.0.248"},
		{idx: 31, slot: 0, wantErr: true},
		{idx: 61, slot: 0, wantErr: true},
		{idx: -1, slot: 0, wantErr: true},
		{idx: 0, slot: 8, wantErr: true},
		{idx: 30, slot: 8, wantErr: true},
		{idx: 0, slot: -1, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("idx=%d/slot=%d", tc.idx, tc.slot), func(t *testing.T) {
			t.Parallel()
			got, err := InterconnectSlotAddress(tc.idx, tc.slot)
			if tc.wantErr {
				if !errors.Is(err, ErrNoInterconnectAddress) {
					t.Fatalf("InterconnectSlotAddress = %v, %v; want ErrNoInterconnectAddress", got, err)
				}
				if got.IsValid() {
					t.Fatalf("a refusal returned an address %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("InterconnectSlotAddress: %v", err)
			}
			if got.String() != tc.want {
				t.Fatalf("InterconnectSlotAddress = %s, want %s", got, tc.want)
			}
		})
	}

	// Every address over the whole domain: distinct, a link address, inside the
	// low half, never the subnet's network or broadcast address.
	seen := map[netip.Addr]bool{}
	for idx := 0; idx <= MaxInterconnectNodeIndex; idx++ {
		for slot := 0; slot <= MaxInterconnectSlot; slot++ {
			a, err := InterconnectSlotAddress(idx, slot)
			if err != nil {
				t.Fatalf("(%d,%d): %v", idx, slot, err)
			}
			if !IsLinkAddress(a) || !prefix.Contains(a) {
				t.Fatalf("(%d,%d) = %s is outside %s", idx, slot, a, InterconnectCIDR)
			}
			if b := a.As4()[3]; b == 0 || b == 255 {
				t.Fatalf("(%d,%d) = %s is the network or broadcast address", idx, slot, a)
			}
			if seen[a] {
				t.Fatalf("(%d,%d) = %s is a duplicate", idx, slot, a)
			}
			seen[a] = true
		}
	}
}

// TestInterconnectSlotAddressEqualsLinkIP pins that the self-contained slot
// derivation agrees with LinkIP over the whole interconnect domain, so neither
// can drift while both exist.
func TestInterconnectSlotAddressEqualsLinkIP(t *testing.T) {
	t.Parallel()
	for idx := 0; idx <= MaxInterconnectNodeIndex; idx++ {
		for slot := 0; slot <= MaxInterconnectSlot; slot++ {
			got, err := InterconnectSlotAddress(idx, slot)
			if err != nil {
				t.Fatalf("InterconnectSlotAddress(%d,%d): %v", idx, slot, err)
			}
			want, err := LinkIP(idx, slot)
			if err != nil {
				t.Fatalf("LinkIP(%d,%d): %v", idx, slot, err)
			}
			if got != want {
				t.Fatalf("InterconnectSlotAddress(%d,%d) = %s, LinkIP = %s", idx, slot, got, want)
			}
		}
	}
}

// TestInterconnectKeys pins the annotation keys, the condition type and the
// reasons. Pods and Nodes carry them and consumers match on them, so a rename
// silently matches nothing.
func TestInterconnectKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, got, want string }{
		{"AnnotationInterconnectReport", AnnotationInterconnectReport, "k3sm.io/interconnect-report"},
		{"AnnotationInterconnectDesired", AnnotationInterconnectDesired, "k3sm.io/interconnect-desired"},
		{"AnnotationInterconnects", AnnotationInterconnects, "k3sm.io/interconnects"},
		{"ConditionInterconnectAttached", ConditionInterconnectAttached, "k3sm.io/InterconnectAttached"},
		{"ReasonInterconnectSlotsExhausted", ReasonInterconnectSlotsExhausted, "InterconnectSlotsExhausted"},
		{"ReasonInterconnectUnsupportedRuntime", ReasonInterconnectUnsupportedRuntime, "InterconnectUnsupportedRuntime"},
		{"ReasonNoInterconnectSegment", ReasonNoInterconnectSegment, "NoInterconnectSegment"},
		{"ReasonInterconnectIndexOutOfRange", ReasonInterconnectIndexOutOfRange, "InterconnectIndexOutOfRange"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
		if strings.Contains(tc.got, MediumThunderbolt) {
			t.Errorf("%s = %q names a medium; keys are medium-agnostic", tc.name, tc.got)
		}
	}
	if MaxInterconnectReportBytes != 4096 || MaxInterconnectReportPeers != 16 ||
		InterconnectReportMinInterval != 10*time.Second || InterconnectAnnotationSchema != 1 ||
		MaxInterconnectDomainUUIDBytes != 64 || MaxInterconnectListItems != 64 {
		t.Error("the I6 report bounds moved")
	}
}

func sampleReport() InterconnectReport {
	return InterconnectReport{
		Schema:           InterconnectAnnotationSchema,
		LocalDomainUUIDs: []string{"8b1c0e2a-0000-4000-8000-000000000001"},
		PeerDomainUUIDs:  []string{"8b1c0e2a-0000-4000-8000-000000000002"},
		Carrier:          true,
		SpeedGbps:        80,
		ReachablePeers:   []string{"studio-2"},
	}
}

func peers(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("node-%d", i)
	}
	return out
}

// TestInterconnectReportParse covers the I6 bounds on the untrusted report:
// round trip, oversize, too many peers, unknown and missing schema, unknown
// fields, malformed input and bad field values.
func TestInterconnectReportParse(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		want := sampleReport()
		v, err := want.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		got, err := ParseInterconnectReport(v)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip = %+v, want %+v", got, want)
		}
	})
	t.Run("sixteen peers accepted", func(t *testing.T) {
		t.Parallel()
		r := sampleReport()
		r.ReachablePeers = peers(MaxInterconnectReportPeers)
		v, err := r.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if _, err := ParseInterconnectReport(v); err != nil {
			t.Fatalf("Parse: %v", err)
		}
	})
	t.Run("64-byte uuid accepted", func(t *testing.T) {
		t.Parallel()
		u := strings.Repeat("a", MaxInterconnectDomainUUIDBytes)
		if _, err := ParseInterconnectReport(`{"schema":1,"peerDomainUUIDs":["` + u + `"]}`); err != nil {
			t.Fatalf("Parse: %v", err)
		}
	})
	t.Run("unknown fields ignored", func(t *testing.T) {
		t.Parallel()
		got, err := ParseInterconnectReport(`{"schema":1,"carrier":true,"futureField":{"x":1}}`)
		if err != nil || !got.Carrier {
			t.Fatalf("Parse = %+v, %v; want accepted", got, err)
		}
	})

	cases := []struct {
		name  string
		value string
		want  error
	}{
		{"oversize", `{"schema":1,"localDomainUUIDs":["` + strings.Repeat("a", MaxInterconnectReportBytes) + `"]}`, ErrInvalidInterconnectAnnotation},
		{"seventeen peers", mustJSON(t, map[string]any{"schema": 1, "reachablePeers": peers(MaxInterconnectReportPeers + 1)}), ErrInvalidInterconnectAnnotation},
		{"seventeen peer UUIDs", mustJSON(t, map[string]any{"schema": 1, "peerDomainUUIDs": peers(MaxInterconnectReportPeers + 1)}), ErrInvalidInterconnectAnnotation},
		{"unknown schema", `{"schema":2,"carrier":true}`, ErrUnknownInterconnectSchema},
		{"unknown schema with changed field types", `{"schema":2,"carrier":"yes"}`, ErrUnknownInterconnectSchema},
		{"schema zero", `{"schema":0}`, ErrUnknownInterconnectSchema},
		{"missing schema", `{"carrier":true}`, ErrUnknownInterconnectSchema},
		{"not json", `thunderbolt;fallback=network`, ErrInvalidInterconnectAnnotation},
		{"empty", ``, ErrInvalidInterconnectAnnotation},
		{"bad peer name", `{"schema":1,"reachablePeers":["Not A Node"]}`, ErrInvalidInterconnectAnnotation},
		{"repeated peer", `{"schema":1,"reachablePeers":["a","a"]}`, ErrInvalidInterconnectAnnotation},
		{"empty uuid", `{"schema":1,"localDomainUUIDs":[""]}`, ErrInvalidInterconnectAnnotation},
		{"65-byte uuid", `{"schema":1,"localDomainUUIDs":["` + strings.Repeat("a", MaxInterconnectDomainUUIDBytes+1) + `"]}`, ErrInvalidInterconnectAnnotation},
		{"newline in peer uuid", `{"schema":1,"peerDomainUUIDs":["abc\nFAKE log line"]}`, ErrInvalidInterconnectAnnotation},
		{"escape in local uuid", `{"schema":1,"localDomainUUIDs":["\u001b[31mred"]}`, ErrInvalidInterconnectAnnotation},
		{"non-ascii uuid", `{"schema":1,"peerDomainUUIDs":["caf\u00e9"]}`, ErrInvalidInterconnectAnnotation},
		{"negative speed", `{"schema":1,"speedGbps":-1}`, ErrInvalidInterconnectAnnotation},
		{"wrong type", `{"schema":1,"carrier":"yes"}`, ErrInvalidInterconnectAnnotation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseInterconnectReport(tc.value)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Parse = %v, want %v", err, tc.want)
			}
			if !reflect.DeepEqual(got, InterconnectReport{}) {
				t.Fatalf("a refused report returned content %+v", got)
			}
		})
	}

	t.Run("encode refuses what parse would", func(t *testing.T) {
		t.Parallel()
		r := sampleReport()
		r.ReachablePeers = peers(MaxInterconnectReportPeers + 1)
		if _, err := r.Encode(); !errors.Is(err, ErrInvalidInterconnectAnnotation) {
			t.Fatalf("Encode of 17 peers = %v", err)
		}
		r = sampleReport()
		r.LocalDomainUUIDs = []string{strings.Repeat("u", MaxInterconnectReportBytes)}
		if _, err := r.Encode(); !errors.Is(err, ErrInvalidInterconnectAnnotation) {
			t.Fatalf("Encode of an oversize report = %v", err)
		}
		r = sampleReport()
		r.Schema = 2
		if _, err := r.Encode(); !errors.Is(err, ErrUnknownInterconnectSchema) {
			t.Fatalf("Encode of schema 2 = %v", err)
		}
	})
}

// TestInterconnectDesiredParse covers the desired payload's round trip and
// refusals.
func TestInterconnectDesiredParse(t *testing.T) {
	t.Parallel()
	want := InterconnectDesired{Schema: InterconnectAnnotationSchema, Interconnect: MediumThunderbolt, Fallback: InterconnectFallbackNone}
	v, err := want.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if v != `{"schema":1,"interconnect":"thunderbolt","fallback":"none"}` {
		t.Errorf("Encode = %s", v)
	}
	got, err := ParseInterconnectDesired(v)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %+v, %v; want %+v", got, err, want)
	}

	t.Run("round trip with allowed namespaces and models", func(t *testing.T) {
		t.Parallel()
		const uid = "0f8b6c3e-1d2a-4b5c-9e7f-0123456789ab"
		want := InterconnectDesired{
			Schema:            InterconnectAnnotationSchema,
			Interconnect:      MediumThunderbolt,
			Fallback:          InterconnectFallbackNetwork,
			AllowedNamespaces: []string{"ml", "serving"},
			MLXModels:         []InterconnectModelRef{{Namespace: "ml", Name: "llm", UID: uid}},
		}
		v, err := want.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if wantV := `{"schema":1,"interconnect":"thunderbolt","fallback":"network","allowedNamespaces":["ml","serving"],"mlxModels":[{"namespace":"ml","name":"llm","uid":"` + uid + `"}]}`; v != wantV {
			t.Errorf("Encode = %s, want %s", v, wantV)
		}
		got, err := ParseInterconnectDesired(v)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Parse = %+v, %v; want %+v", got, err, want)
		}
		if !got.NamespaceAllowed("ml") || got.NamespaceAllowed("default") {
			t.Errorf("NamespaceAllowed: ml allowed=%v, default allowed=%v; want true, false", got.NamespaceAllowed("ml"), got.NamespaceAllowed("default"))
		}
		for _, tc := range []struct {
			ns, name, uid string
			want          bool
		}{
			{"ml", "llm", uid, true},
			{"serving", "llm", uid, false},
			{"ml", "other", uid, false},
			{"ml", "llm", "other-uid", false},
			{"", "", "", false},
		} {
			if got := got.HasMLXModel(tc.ns, tc.name, tc.uid); got != tc.want {
				t.Errorf("HasMLXModel(%q, %q, %q) = %v, want %v", tc.ns, tc.name, tc.uid, got, tc.want)
			}
		}
	})
	t.Run("empty allowed namespaces allows every namespace", func(t *testing.T) {
		t.Parallel()
		d := InterconnectDesired{Schema: 1, Interconnect: MediumThunderbolt, Fallback: InterconnectFallbackNetwork}
		if !d.NamespaceAllowed("anything") || d.HasMLXModel("ns", "m", "x") {
			t.Fatalf("empty lists: NamespaceAllowed=%v HasMLXModel=%v", d.NamespaceAllowed("anything"), d.HasMLXModel("ns", "m", "x"))
		}
	})
	t.Run("a reader of the old shape ignores the new fields", func(t *testing.T) {
		t.Parallel()
		var old struct {
			Schema       int    `json:"schema"`
			Interconnect string `json:"interconnect"`
			Fallback     string `json:"fallback"`
		}
		if err := json.Unmarshal([]byte(`{"schema":1,"interconnect":"thunderbolt","fallback":"none","allowedNamespaces":["a"],"mlxModels":[{"namespace":"a","name":"m","uid":"u"}]}`), &old); err != nil || old.Fallback != "none" {
			t.Fatalf("old-shape decode = %+v, %v", old, err)
		}
	})
	t.Run("64 entries accepted", func(t *testing.T) {
		t.Parallel()
		d := InterconnectDesired{Schema: 1, Interconnect: MediumThunderbolt, Fallback: InterconnectFallbackNone, AllowedNamespaces: peers(MaxInterconnectListItems)}
		if err := d.Validate(); err != nil {
			t.Fatalf("Validate of 64 namespaces: %v", err)
		}
		d = InterconnectDesired{Schema: 1, Interconnect: MediumThunderbolt, Fallback: InterconnectFallbackNone, MLXModels: models(MaxInterconnectListItems)}
		if err := d.Validate(); err != nil {
			t.Fatalf("Validate of 64 models: %v", err)
		}
	})
	t.Run("encode refuses a payload over 4 KiB", func(t *testing.T) {
		t.Parallel()
		ms := models(MaxInterconnectListItems)
		for i := range ms {
			ms[i].UID = fmt.Sprintf("%064d", i)
		}
		d := InterconnectDesired{Schema: 1, Interconnect: MediumThunderbolt, Fallback: InterconnectFallbackNone, MLXModels: ms}
		if err := d.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
		if _, err := d.Encode(); !errors.Is(err, ErrInvalidInterconnectAnnotation) {
			t.Fatalf("Encode of an oversize desired = %v", err)
		}
	})
	for _, tc := range []struct {
		name, value string
		want        error
	}{
		{"unknown schema", `{"schema":2,"interconnect":"thunderbolt","fallback":"none"}`, ErrUnknownInterconnectSchema},
		{"missing schema", `{"interconnect":"thunderbolt","fallback":"none"}`, ErrUnknownInterconnectSchema},
		{"oversize", `{"schema":1,"interconnect":"` + strings.Repeat("a", MaxInterconnectReportBytes) + `"}`, ErrInvalidInterconnectAnnotation},
		{"empty fallback", `{"schema":1,"interconnect":"thunderbolt"}`, ErrInvalidInterconnectAnnotation},
		{"unknown fallback", `{"schema":1,"interconnect":"thunderbolt","fallback":"mesh"}`, ErrInvalidInterconnectAnnotation},
		{"empty interconnect", `{"schema":1,"interconnect":"","fallback":"none"}`, ErrInvalidInterconnectAnnotation},
		{"legacy text form", `thunderbolt;fallback=network`, ErrInvalidInterconnectAnnotation},
		{"65 namespaces", mustJSON(t, map[string]any{"schema": 1, "interconnect": "thunderbolt", "fallback": "none", "allowedNamespaces": peers(MaxInterconnectListItems + 1)}), ErrInvalidInterconnectAnnotation},
		{"namespace not a label", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","allowedNamespaces":["a.b"]}`, ErrInvalidInterconnectAnnotation},
		{"namespace upper case", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","allowedNamespaces":["ML"]}`, ErrInvalidInterconnectAnnotation},
		{"namespace too long", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","allowedNamespaces":["` + strings.Repeat("a", 64) + `"]}`, ErrInvalidInterconnectAnnotation},
		{"empty namespace", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","allowedNamespaces":[""]}`, ErrInvalidInterconnectAnnotation},
		{"repeated namespace", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","allowedNamespaces":["a","a"]}`, ErrInvalidInterconnectAnnotation},
		{"65 models", mustJSON(t, map[string]any{"schema": 1, "interconnect": "thunderbolt", "fallback": "none", "mlxModels": models(MaxInterconnectListItems + 1)}), ErrInvalidInterconnectAnnotation},
		{"model namespace not a label", desiredWithModel("a.b", "m", "u"), ErrInvalidInterconnectAnnotation},
		{"model namespace empty", desiredWithModel("", "m", "u"), ErrInvalidInterconnectAnnotation},
		{"model name not an object name", desiredWithModel("ml", "Not A Name", "u"), ErrInvalidInterconnectAnnotation},
		{"model name empty", desiredWithModel("ml", "", "u"), ErrInvalidInterconnectAnnotation},
		{"empty uid", desiredWithModel("ml", "m", ""), ErrInvalidInterconnectAnnotation},
		{"65-byte uid", desiredWithModel("ml", "m", strings.Repeat("a", MaxInterconnectUIDBytes+1)), ErrInvalidInterconnectAnnotation},
		{"newline in uid", desiredWithModel("ml", "m", "abc\\nFAKE"), ErrInvalidInterconnectAnnotation},
		{"non-ascii uid", desiredWithModel("ml", "m", "caf\\u00e9"), ErrInvalidInterconnectAnnotation},
		{"repeated model", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","mlxModels":[{"namespace":"ml","name":"m","uid":"u1"},{"namespace":"ml","name":"m","uid":"u2"}]}`, ErrInvalidInterconnectAnnotation},
		{"repeated model uid", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","mlxModels":[{"namespace":"ml","name":"a","uid":"u"},{"namespace":"ml","name":"b","uid":"u"}]}`, ErrInvalidInterconnectAnnotation},
		{"models wrong type", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","mlxModels":["u"]}`, ErrInvalidInterconnectAnnotation},
		{"namespaces wrong type", `{"schema":1,"interconnect":"thunderbolt","fallback":"none","allowedNamespaces":"ml"}`, ErrInvalidInterconnectAnnotation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseInterconnectDesired(tc.value); !errors.Is(err, tc.want) {
				t.Fatalf("Parse = %v, want %v", err, tc.want)
			}
		})
	}
}

// models returns n distinct, well-formed model references.
func models(n int) []InterconnectModelRef {
	out := make([]InterconnectModelRef, n)
	for i := range out {
		out[i] = InterconnectModelRef{Namespace: "ml", Name: fmt.Sprintf("m-%d", i), UID: fmt.Sprintf("uid-%d", i)}
	}
	return out
}

// desiredWithModel is a desired payload naming one model reference, spelled
// as raw JSON so escapes reach the parser.
func desiredWithModel(ns, name, uid string) string {
	return `{"schema":1,"interconnect":"thunderbolt","fallback":"none","mlxModels":[{"namespace":"` + ns + `","name":"` + name + `","uid":"` + uid + `"}]}`
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// TestInterconnectGVK asserts the Interconnect types register under
// net.k3sm.io/v1alpha1.
func TestInterconnectGVK(t *testing.T) {
	t.Parallel()
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, tc := range []struct {
		obj  runtime.Object
		kind string
	}{{&Interconnect{}, "Interconnect"}, {&InterconnectList{}, "InterconnectList"}} {
		gvks, _, err := s.ObjectKinds(tc.obj)
		if err != nil {
			t.Fatalf("ObjectKinds(%s): %v", tc.kind, err)
		}
		want := schema.GroupVersionKind{Group: "net.k3sm.io", Version: "v1alpha1", Kind: tc.kind}
		if len(gvks) != 1 || gvks[0] != want {
			t.Fatalf("%s registered as %v, want %v", tc.kind, gvks, want)
		}
	}
}

// TestInterconnectJSONGolden pins the serialized shape against testdata.
// Regenerate deliberately with UPDATE_GOLDEN=1.
func TestInterconnectJSONGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(sampleInterconnect(), "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", interconnectGolden)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Interconnect JSON differs from testdata/%s\n got = %s\nwant = %s", interconnectGolden, got, want)
	}
	var rt Interconnect
	if err := json.Unmarshal(want, &rt); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	again, err := json.MarshalIndent(&rt, "", "  ")
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(append(again, '\n'), want) {
		t.Error("golden does not round-trip byte-stably")
	}
}

// TestInterconnectGoldenDeclaredInCRD walks every key of the golden object's
// spec and status and asserts the embedded CRD schema declares it, so no field
// the Go type writes is pruned by the apiserver.
func TestInterconnectGoldenDeclaredInCRD(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("testdata", interconnectGolden))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(crd.InterconnectCRD(), &m); err != nil {
		t.Fatalf("CRD manifest is not valid YAML: %v", err)
	}
	versions, _ := m["spec"].(map[string]any)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("CRD has %d versions, want 1", len(versions))
	}
	root := versions[0].(map[string]any)["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)
	props := root["properties"].(map[string]any)
	for _, top := range []string{"spec", "status"} {
		var missing []string
		walkDeclared(top, obj[top], props[top].(map[string]any), &missing)
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("CRD schema does not declare %s; the apiserver would prune them", strings.Join(missing, ", "))
		}
	}

	// The enums and defaults the Go constants define.
	spec := props["spec"].(map[string]any)["properties"].(map[string]any)
	assertEnum(t, "spec.medium", spec["medium"], MediumThunderbolt)
	assertEnum(t, "spec.fallback", spec["fallback"], string(InterconnectFallbackNetwork), string(InterconnectFallbackNone))
	if got := spec["fallback"].(map[string]any)["default"]; got != string(InterconnectFallbackNetwork) {
		t.Errorf("spec.fallback default = %v, want network", got)
	}
	if got := spec["enabled"].(map[string]any)["default"]; got != false {
		t.Errorf("spec.enabled default = %v, want false (armed by an admin)", got)
	}
	members := props["status"].(map[string]any)["properties"].(map[string]any)["members"].(map[string]any)
	state := members["items"].(map[string]any)["properties"].(map[string]any)["state"]
	assertEnum(t, "status.members[].state", state,
		string(InterconnectMemberUp), string(InterconnectMemberDown), string(InterconnectMemberLoop))
}

func assertEnum(t *testing.T, path string, node any, want ...string) {
	t.Helper()
	n, _ := node.(map[string]any)
	enum, _ := n["enum"].([]any)
	if len(enum) != len(want) {
		t.Errorf("%s enum = %v, want %v", path, enum, want)
		return
	}
	for i := range want {
		if enum[i] != want[i] {
			t.Errorf("%s enum = %v, want %v", path, enum, want)
			return
		}
	}
}

// TestInterconnectDeepCopy asserts DeepCopy shares no slice or pointer with the
// original.
func TestInterconnectDeepCopy(t *testing.T) {
	t.Parallel()
	orig := sampleInterconnect()
	cp := orig.DeepCopy()
	orig.Spec.ExcludedNodes[0] = "x"
	orig.Spec.AllowedNamespaces[0] = "x"
	orig.Status.Members[0].ReachablePeers[0] = "x"
	orig.Status.Members[0].LastTransition.Time = time.Unix(0, 0)
	orig.Status.Segments[0].Nodes[0] = "x"
	orig.Status.ArmedAt.Time = time.Unix(0, 0)
	orig.Status.Conditions[0].Reason = "x"
	if !reflect.DeepEqual(cp, sampleInterconnect()) {
		t.Fatal("DeepCopy shares state with the original")
	}
	if _, ok := orig.DeepCopyObject().(*Interconnect); !ok {
		t.Fatal("DeepCopyObject did not return *Interconnect")
	}
	if (*Interconnect)(nil).DeepCopy() != nil {
		t.Fatal("(*Interconnect)(nil).DeepCopy() != nil")
	}
	list := &InterconnectList{Items: []Interconnect{*sampleInterconnect()}}
	lcp := list.DeepCopy()
	list.Items[0].Status.Segments[0].Nodes[0] = "x"
	if lcp.Items[0].Status.Segments[0].Nodes[0] != "studio-1" {
		t.Fatal("list DeepCopy shared items")
	}
	if _, ok := list.DeepCopyObject().(*InterconnectList); !ok {
		t.Fatal("DeepCopyObject did not return *InterconnectList")
	}
}

// fixedLinkTime is a deterministic, second-precision UTC instant so metav1.Time
// status fields round-trip losslessly through RFC3339 JSON.
var fixedLinkTime = metav1.NewTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))

// walkDeclared records in missing every path under v that sch does not declare.
func walkDeclared(path string, v any, sch map[string]any, missing *[]string) {
	switch val := v.(type) {
	case map[string]any:
		props, _ := sch["properties"].(map[string]any)
		for k, child := range val {
			cs, ok := props[k].(map[string]any)
			if !ok {
				*missing = append(*missing, path+"."+k)
				continue
			}
			walkDeclared(path+"."+k, child, cs, missing)
		}
	case []any:
		items, _ := sch["items"].(map[string]any)
		for _, child := range val {
			walkDeclared(path+"[]", child, items, missing)
		}
	}
}
