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
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k3sm.io/apis/config/crd"
	updatesv1alpha1 "k3sm.io/apis/updates/v1alpha1"
)

// TestUpdateCRDRoundTrip is the contract test between the Update Go types and
// the embedded CRD manifest: an Update encoded through a runtime.Scheme codec
// decodes back to the same object, its wire form validates against the
// manifest's openAPIV3Schema (every field declared, every enum, bound, pattern
// and required field honoured), a minimal object picks up the schema defaults
// the Go doc comments promise, and objects that break the schema are refused.
//
// This module carries no apiextensions machinery, so the schema check is the
// small structural validator below. It covers exactly the keywords the Update
// manifest uses, and the refusal table proves it is not vacuous: a validator
// that admitted everything would fail every row of it.
func TestUpdateCRDRoundTrip(t *testing.T) {
	t.Parallel()
	root := updateSchema(t)

	t.Run("scheme round trip", func(t *testing.T) {
		s := runtime.NewScheme()
		if err := updatesv1alpha1.AddToScheme(s); err != nil {
			t.Fatalf("AddToScheme: %v", err)
		}
		codecs := serializer.NewCodecFactory(s)
		orig := sampleUpdate()
		wire, err := runtime.Encode(codecs.LegacyCodec(updatesv1alpha1.SchemeGroupVersion), orig)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		obj, gvk, err := codecs.UniversalDeserializer().Decode(wire, nil, nil)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if gvk.Group != "updates.k3sm.io" || gvk.Version != "v1alpha1" || gvk.Kind != "Update" {
			t.Fatalf("decoded GVK = %v, want updates.k3sm.io/v1alpha1, Kind=Update", gvk)
		}
		got, ok := obj.(*updatesv1alpha1.Update)
		if !ok {
			t.Fatalf("decoded %T, want *Update", obj)
		}
		// metav1.Time decodes into the local zone; compare the instants.
		normalizeTimes(got)
		if !reflect.DeepEqual(got, orig) {
			t.Fatalf("round trip changed the object:\n got = %+v\nwant = %+v", got, orig)
		}

		var m map[string]any
		if err := json.Unmarshal(wire, &m); err != nil {
			t.Fatalf("unmarshal wire: %v", err)
		}
		if errs := validate("", m, root); len(errs) > 0 {
			t.Fatalf("the encoded Update does not validate against the CRD schema:\n%s", strings.Join(errs, "\n"))
		}
	})

	t.Run("defaults", func(t *testing.T) {
		m := map[string]any{
			"apiVersion": "updates.k3sm.io/v1alpha1",
			"kind":       "Update",
			"metadata":   map[string]any{"name": "minimal"},
			"spec":       map[string]any{"version": "v0.2.0"},
		}
		applyDefaults(m, root)
		if errs := validate("", m, root); len(errs) > 0 {
			t.Fatalf("the defaulted minimal Update does not validate:\n%s", strings.Join(errs, "\n"))
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var u updatesv1alpha1.Update
		if err := json.Unmarshal(b, &u); err != nil {
			t.Fatalf("unmarshal into Update: %v", err)
		}
		for _, tc := range []struct {
			field     string
			got, want any
		}{
			{"strategy.order", u.Spec.Strategy.Order, updatesv1alpha1.UpdateOrderWorkersThenServer},
			{"strategy.maxUnavailable", u.Spec.Strategy.MaxUnavailable, int32(1)},
			{"strategy.drain", u.Spec.Strategy.Drain, false},
			{"verify.checksums", u.Spec.Verify.ChecksumsEnabled(), true},
			{"verify.checksums set", u.Spec.Verify.Checksums != nil, true},
			{"verify.signature", u.Spec.Verify.Signature, updatesv1alpha1.UpdateSignatureRequired},
			{"allowDowngrade", u.Spec.AllowDowngrade, false},
			{"cancel", u.Spec.Cancel, false},
		} {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("defaulted spec.%s = %v, want %v", tc.field, tc.got, tc.want)
			}
		}
	})

	t.Run("an explicit false survives defaulting", func(t *testing.T) {
		f := false
		u := sampleUpdate()
		u.Spec.Verify.Checksums = &f
		b, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		applyDefaults(m, root)
		var back updatesv1alpha1.Update
		b, _ = json.Marshal(m)
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal into Update: %v", err)
		}
		if back.Spec.Verify.ChecksumsEnabled() {
			t.Error("checksums: false was defaulted back to true")
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(m map[string]any)
		want   string
	}{
		{"missing version", func(m map[string]any) { delete(spec(m), "version") }, "spec.version: required"},
		{"empty version", func(m map[string]any) { spec(m)["version"] = "" }, "spec.version"},
		{"version not a tag", func(m map[string]any) { spec(m)["version"] = "latest" }, "spec.version: does not match"},
		{"unknown order", func(m map[string]any) { sub(m, "strategy")["order"] = "server-then-workers" }, "spec.strategy.order: not in enum"},
		{"maxUnavailable zero", func(m map[string]any) { sub(m, "strategy")["maxUnavailable"] = 0 }, "spec.strategy.maxUnavailable: below minimum"},
		{"drain not a bool", func(m map[string]any) { sub(m, "strategy")["drain"] = "yes" }, "spec.strategy.drain: want boolean"},
		{"unknown signature", func(m map[string]any) { sub(m, "verify")["signature"] = "optional" }, "spec.verify.signature: not in enum"},
		{"undeclared spec field", func(m map[string]any) { spec(m)["channel"] = "stable" }, "spec.channel: not declared"},
		{"unknown phase", func(m map[string]any) { status(m)["phase"] = "Paused" }, "status.phase: not in enum"},
		{"unknown node phase", func(m map[string]any) { node(m)["phase"] = "Rebooting" }, "status.nodes[0].phase: not in enum"},
		{"node without name", func(m map[string]any) { delete(node(m), "name") }, "status.nodes[0].name: required"},
		{"duplicate node name", func(m map[string]any) {
			ns := status(m)["nodes"].([]any)
			ns[1].(map[string]any)["name"] = ns[0].(map[string]any)["name"]
		}, "status.nodes: duplicate key"},
		{"startedAt not a time", func(m map[string]any) { node(m)["startedAt"] = "yesterday" }, "status.nodes[0].startedAt: not a date-time"},
		{"negative observedGeneration", func(m map[string]any) { status(m)["observedGeneration"] = -1 }, "status.observedGeneration: below minimum"},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			m := sampleWire(t)
			tc.mutate(m)
			errs := validate("", normalizeNumbers(m), root)
			for _, e := range errs {
				if strings.Contains(e, tc.want) {
					return
				}
			}
			t.Errorf("want an error containing %q, got %q", tc.want, errs)
		})
	}
}

// sampleUpdate is a fully populated Update in UTC.
func sampleUpdate() *updatesv1alpha1.Update {
	at := metav1.NewTime(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
	done := metav1.NewTime(at.Add(90 * time.Second))
	yes := true
	return &updatesv1alpha1.Update{
		TypeMeta:   metav1.TypeMeta{APIVersion: "updates.k3sm.io/v1alpha1", Kind: "Update"},
		ObjectMeta: metav1.ObjectMeta{Name: "to-v0.2.0", Generation: 2},
		Spec: updatesv1alpha1.UpdateSpec{
			Version: "v0.2.0-rc.1",
			Strategy: updatesv1alpha1.UpdateStrategy{
				Order:          updatesv1alpha1.UpdateOrderWorkersThenServer,
				MaxUnavailable: 2,
				Drain:          true,
			},
			Verify: updatesv1alpha1.UpdateVerify{
				Checksums: &yes,
				Signature: updatesv1alpha1.UpdateSignatureIfSigned,
			},
			AllowDowngrade: true,
			Cancel:         true,
		},
		Status: updatesv1alpha1.UpdateStatus{
			Phase: updatesv1alpha1.UpdatePhaseCancelled,
			Nodes: []updatesv1alpha1.UpdateNodeStatus{
				{Name: "worker-1", From: "v0.1.7", To: "v0.2.0-rc.1", Phase: updatesv1alpha1.UpdateNodePhaseDone,
					Message: "running v0.2.0-rc.1", StartedAt: &at, FinishedAt: &done},
				{Name: "server-1", From: "v0.1.7", To: "v0.2.0-rc.1", Phase: updatesv1alpha1.UpdateNodePhasePending},
			},
			Conditions: []metav1.Condition{{
				Type: "Progressing", Status: metav1.ConditionFalse, ObservedGeneration: 2,
				LastTransitionTime: at, Reason: "Cancelled", Message: "cancelled after 1 of 2 nodes",
			}},
			ObservedGeneration: 2,
		},
	}
}

func sampleWire(t *testing.T) map[string]any {
	t.Helper()
	b, err := json.Marshal(sampleUpdate())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func spec(m map[string]any) map[string]any   { return m["spec"].(map[string]any) }
func status(m map[string]any) map[string]any { return m["status"].(map[string]any) }
func sub(m map[string]any, k string) map[string]any {
	return spec(m)[k].(map[string]any)
}
func node(m map[string]any) map[string]any {
	return status(m)["nodes"].([]any)[0].(map[string]any)
}

// normalizeTimes puts every metav1.Time of u in UTC so a decoded object
// compares equal to a UTC sample.
func normalizeTimes(u *updatesv1alpha1.Update) {
	for i := range u.Status.Nodes {
		for _, p := range []*metav1.Time{u.Status.Nodes[i].StartedAt, u.Status.Nodes[i].FinishedAt} {
			if p != nil {
				p.Time = p.UTC()
			}
		}
	}
	for i := range u.Status.Conditions {
		c := &u.Status.Conditions[i]
		c.LastTransitionTime.Time = c.LastTransitionTime.UTC()
	}
}

// normalizeNumbers turns the Go ints a mutation inserts into the float64
// encoding/json produces, so the validator sees one numeric representation.
func normalizeNumbers(v map[string]any) map[string]any {
	b, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(b, &out) // the input was produced by json.Unmarshal; it re-encodes
	return out
}

// updateSchema returns the openAPIV3Schema of the embedded Update manifest's
// single version.
func updateSchema(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(crd.UpdateCRD(), &m); err != nil {
		t.Fatalf("the Update CRD manifest is not valid YAML: %v", err)
	}
	// Re-encode through JSON so schema numbers are float64 like the object's.
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	versions, _ := m["spec"].(map[string]any)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("the Update CRD has %d versions, want 1", len(versions))
	}
	v := versions[0].(map[string]any)
	if v["name"] != "v1alpha1" || v["served"] != true || v["storage"] != true {
		t.Fatalf("the Update CRD version is %v served=%v storage=%v, want v1alpha1 served and stored", v["name"], v["served"], v["storage"])
	}
	return v["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)
}

// validate checks v against the structural-schema subset the Update manifest
// uses (type, properties, required, items, enum, minimum, minLength,
// maxLength, pattern, format date-time, and x-kubernetes-list-type map keys)
// and returns one message per violation. An object property the schema does
// not declare is a violation: the apiserver would prune it, so a Go field
// that writes it would be silently lost. metadata is the apiserver's own
// (type: object, no properties) and is not walked.
func validate(path string, v any, sch map[string]any) []string {
	var errs []string
	at := func(format string, a ...any) {
		p := strings.TrimPrefix(path, ".")
		if p == "" {
			p = "<root>"
		}
		errs = append(errs, p+": "+fmt.Sprintf(format, a...))
	}
	switch sch["type"] {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			at("want object, got %T", v)
			return errs
		}
		for _, r := range asList(sch["required"]) {
			if _, ok := obj[r.(string)]; !ok {
				errs = append(errs, strings.TrimPrefix(path+"."+r.(string), ".")+": required")
			}
		}
		props, declared := sch["properties"].(map[string]any)
		if !declared {
			return errs
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			cs, ok := props[k].(map[string]any)
			if !ok {
				errs = append(errs, strings.TrimPrefix(path+"."+k, ".")+": not declared")
				continue
			}
			errs = append(errs, validate(path+"."+k, obj[k], cs)...)
		}
	case "array":
		list, ok := v.([]any)
		if !ok {
			at("want array, got %T", v)
			return errs
		}
		items, _ := sch["items"].(map[string]any)
		for i, e := range list {
			errs = append(errs, validate(fmt.Sprintf("%s[%d]", path, i), e, items)...)
		}
		if sch["x-kubernetes-list-type"] == "map" {
			seen := map[string]bool{}
			for _, e := range list {
				em, _ := e.(map[string]any)
				var key []string
				for _, k := range asList(sch["x-kubernetes-list-map-keys"]) {
					key = append(key, fmt.Sprint(em[k.(string)]))
				}
				id := strings.Join(key, "\x00")
				if seen[id] {
					at("duplicate key %q", id)
				}
				seen[id] = true
			}
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			at("want string, got %T", v)
			return errs
		}
		if n, ok := sch["minLength"].(float64); ok && float64(len(s)) < n {
			at("shorter than minLength %v", n)
		}
		if n, ok := sch["maxLength"].(float64); ok && float64(len(s)) > n {
			at("longer than maxLength %v", n)
		}
		if p, ok := sch["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(s) {
			at("does not match %s", p)
		}
		if sch["format"] == "date-time" {
			if _, err := time.Parse(time.RFC3339, s); err != nil {
				at("not a date-time")
			}
		}
	case "integer":
		n, ok := v.(float64)
		if !ok || n != float64(int64(n)) {
			at("want integer, got %v", v)
			return errs
		}
		if min, ok := sch["minimum"].(float64); ok && n < min {
			at("below minimum %v", min)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			at("want boolean, got %T", v)
		}
	default:
		at("schema type %v is outside the validator's subset", sch["type"])
	}
	if enum, ok := sch["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if e == v {
				found = true
			}
		}
		if !found {
			at("not in enum %v", enum)
		}
	}
	return errs
}

// applyDefaults fills in the schema defaults the apiserver would, top-down: a
// missing property with a default takes it, and an object that exists (or was
// just defaulted) is then walked for its own properties' defaults.
func applyDefaults(v any, sch map[string]any) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	props, _ := sch["properties"].(map[string]any)
	for k, p := range props {
		cs := p.(map[string]any)
		if _, set := obj[k]; !set {
			if d, ok := cs["default"]; ok {
				b, _ := json.Marshal(d)
				var cp any
				_ = json.Unmarshal(b, &cp) // a deep copy of a value that was decoded from JSON
				obj[k] = cp
			}
		}
		if child, ok := obj[k]; ok {
			applyDefaults(child, cs)
		}
	}
}

func asList(v any) []any { l, _ := v.([]any); return l }
