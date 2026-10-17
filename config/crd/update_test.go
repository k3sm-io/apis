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

package crd

import (
	"strings"
	"testing"
)

// The pinned transition-rule texts and the display-only wording on the
// phase fields.
const (
	updateVersionImmutableRule = "self == oldSelf"
	updateCancelOneWayRule     = "!oldSelf || self"
	displayOnly                = "Display only, derived from conditions; readers tolerate values they do not know."
)

// TestUpdateCRDMatchesTheGoTypes asserts the embedded Update manifest
// describes the object k3sm.io/apis/updates/v1alpha1 describes: its identity,
// the single served+stored v1alpha1 version, the status subresource, the
// printer columns, every spec and status field, and the enums and defaults.
//
// It deliberately does NOT import updates/v1alpha1, like its siblings, so a
// disagreement stays visible; the lists below are the reviewable copy. The
// Go-side agreement (a sample Update validated against this schema) is
// TestUpdateCRDRoundTrip in k3sm.io/apis/k3smtest.
func TestUpdateCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, UpdateCRD())

	if got := m["kind"]; got != "CustomResourceDefinition" {
		t.Errorf("kind = %v, want CustomResourceDefinition", got)
	}
	if got := mapAt(t, m, "metadata")["name"]; got != UpdateCRDName {
		t.Errorf("metadata.name = %v, want %s (the accessor's constant)", got, UpdateCRDName)
	}
	if UpdateCRDName != "updates.updates.k3sm.io" {
		t.Errorf("UpdateCRDName = %q, want updates.updates.k3sm.io", UpdateCRDName)
	}
	spec := mapAt(t, m, "spec")
	if got := spec["group"]; got != "updates.k3sm.io" {
		t.Errorf("spec.group = %v, want updates.k3sm.io", got)
	}
	// Cluster-scoped: a cluster runs one release.
	if got := spec["scope"]; got != "Cluster" {
		t.Errorf("spec.scope = %v, want Cluster", got)
	}
	names := mapAt(t, m, "spec", "names")
	for _, tc := range []struct{ key, want string }{
		{"kind", "Update"},
		{"listKind", "UpdateList"},
		{"plural", "updates"},
		{"singular", "update"},
	} {
		if got := names[tc.key]; got != tc.want {
			t.Errorf("spec.names.%s = %v, want %s", tc.key, got, tc.want)
		}
	}

	v := onlyVersion(t, m, "v1alpha1")
	// The status subresource lets the controller write progress without being
	// able to rewrite the admin's request, and an admin set cancel without
	// clobbering progress.
	sub, ok := v["subresources"].(map[string]any)
	if !ok {
		t.Fatal("spec.versions[0].subresources is missing")
	}
	if _, ok := sub["status"]; !ok {
		t.Error("the status subresource is not enabled")
	}

	cols, _ := v["additionalPrinterColumns"].([]any)
	want := map[string]string{"Version": ".spec.version", "Phase": ".status.phase"}
	for _, c := range cols {
		col, _ := c.(map[string]any)
		name, _ := col["name"].(string)
		if path, ok := want[name]; ok {
			if col["jsonPath"] != path {
				t.Errorf("%s printer column jsonPath = %v, want %s", name, col["jsonPath"], path)
			}
			if d, _ := col["description"].(string); name == "Phase" && !strings.Contains(d, displayOnly) {
				t.Errorf("Phase printer column description = %q, want it to say %q", d, displayOnly)
			}
			delete(want, name)
		}
	}
	for name := range want {
		t.Errorf("no %s printer column", name)
	}

	root := mapAt(t, v, "schema", "openAPIV3Schema")
	if !hasString(root["required"], "spec") {
		t.Error("openAPIV3Schema.required does not include spec")
	}
	sp := specProps(t, v)
	for _, f := range []string{"version", "strategy", "verify", "allowDowngrade", "cancel"} {
		if _, ok := sp[f]; !ok {
			t.Errorf("spec.%s is not declared; the structural schema would prune it", f)
		}
	}
	if !hasString(mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec")["required"], "version") {
		t.Error("spec.required does not include version")
	}
	if got := mapAt(t, sp, "version")["minLength"]; got != int64(1) {
		t.Errorf("spec.version minLength = %v, want 1", got)
	}

	// Defaults: a nested default applies only when its parent object exists,
	// so strategy and verify default to {} and the leaves then default too.
	for _, f := range []string{"strategy", "verify"} {
		d, ok := mapAt(t, sp, f)["default"].(map[string]any)
		if !ok || len(d) != 0 {
			t.Errorf("spec.%s default = %v, want {} so its leaf defaults apply when it is omitted", f, mapAt(t, sp, f)["default"])
		}
	}
	st := mapAt(t, sp, "strategy", "properties")
	assertStringEnum(t, "spec.strategy.order", mapAt(t, st, "order"), "workers-then-server")
	for _, tc := range []struct {
		path string
		node map[string]any
		want any
	}{
		{"spec.strategy.order", mapAt(t, st, "order"), "workers-then-server"},
		{"spec.strategy.maxUnavailable", mapAt(t, st, "maxUnavailable"), int64(1)},
		{"spec.strategy.drain", mapAt(t, st, "drain"), false},
		{"spec.verify.signature", mapAt(t, sp, "verify", "properties", "signature"), "if-signed"},
		{"spec.allowDowngrade", mapAt(t, sp, "allowDowngrade"), false},
		{"spec.cancel", mapAt(t, sp, "cancel"), false},
	} {
		if got := tc.node["default"]; got != tc.want {
			t.Errorf("%s default = %v, want %v", tc.path, got, tc.want)
		}
	}
	if got := mapAt(t, st, "maxUnavailable")["minimum"]; got != int64(1) {
		t.Errorf("spec.strategy.maxUnavailable minimum = %v, want 1", got)
	}
	assertStringEnum(t, "spec.verify.signature", mapAt(t, sp, "verify", "properties", "signature"), "if-signed", "required")
	// The checksum is always verified; there is no field to turn it off.
	if _, ok := mapAt(t, sp, "verify", "properties")["checksums"]; ok {
		t.Error("spec.verify.checksums is declared; the checksum is always verified and has no switch")
	}

	// The two transition rules, pinned by text: version is immutable and
	// cancel is one-way. k3smtest's TestUpdateCRDRoundTrip evaluates the same
	// texts against old/new pairs.
	for _, tc := range []struct{ field, rule, message string }{
		{"version", updateVersionImmutableRule, "spec.version is immutable after create"},
		{"cancel", updateCancelOneWayRule, "spec.cancel cannot be set back to false"},
	} {
		rules, _ := mapAt(t, sp, tc.field)["x-kubernetes-validations"].([]any)
		if len(rules) != 1 {
			t.Errorf("spec.%s x-kubernetes-validations = %v, want exactly one rule", tc.field, rules)
			continue
		}
		r, _ := rules[0].(map[string]any)
		if r["rule"] != tc.rule || r["message"] != tc.message {
			t.Errorf("spec.%s rule = %v / %v, want %q / %q", tc.field, r["rule"], r["message"], tc.rule, tc.message)
		}
	}

	stp := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "status", "properties")
	for _, f := range []string{"phase", "resolved", "nodes", "conditions", "observedGeneration"} {
		if _, ok := stp[f]; !ok {
			t.Errorf("status.%s is not declared; the structural schema would prune it", f)
		}
	}
	for _, f := range []string{"version", "digest"} {
		if _, ok := mapAt(t, stp, "resolved", "properties")[f]; !ok {
			t.Errorf("status.resolved.%s is not declared", f)
		}
	}
	for _, tc := range []struct {
		field string
		want  int64
	}{{"nodes", 64}, {"conditions", 16}} {
		if got := mapAt(t, stp, tc.field)["maxItems"]; got != tc.want {
			t.Errorf("status.%s maxItems = %v, want %d", tc.field, got, tc.want)
		}
	}
	np := mapAt(t, stp, "nodes", "items", "properties")
	for _, f := range []string{"name", "from", "to", "phase", "message", "adminTokenRotated", "startedAt", "finishedAt"} {
		if _, ok := np[f]; !ok {
			t.Errorf("status.nodes[].%s is not declared", f)
		}
	}
	if got := mapAt(t, np, "adminTokenRotated")["type"]; got != "boolean" {
		t.Errorf("status.nodes[].adminTokenRotated type = %v, want boolean", got)
	}
	// The phases are display only and open: a closed enum would make an older
	// apiserver schema refuse a phase a newer controller writes.
	for path, node := range map[string]map[string]any{
		"status.phase":         mapAt(t, stp, "phase"),
		"status.nodes[].phase": mapAt(t, np, "phase"),
	} {
		if _, ok := node["enum"]; ok {
			t.Errorf("%s carries a closed enum; the phases are display only and readers tolerate unknown values", path)
		}
		if d, _ := node["description"].(string); !strings.Contains(d, displayOnly) {
			t.Errorf("%s description = %q, want it to say %q", path, d, displayOnly)
		}
	}
	if got := mapAt(t, stp, "nodes")["x-kubernetes-list-type"]; got != "map" {
		t.Errorf("status.nodes list-type = %v, want map (one entry per node name)", got)
	}
}

// TestUpdateAccessorReturnsAFreshCopy asserts the Update accessor hands out a
// copy, on the same grounds as its siblings.
func TestUpdateAccessorReturnsAFreshCopy(t *testing.T) {
	t.Parallel()
	a := UpdateCRD()
	if len(a) == 0 {
		t.Fatal("UpdateCRD returned no bytes; the go:embed directive did not match")
	}
	a[0] = 'X'
	if b := UpdateCRD(); b[0] != '#' {
		t.Fatalf("UpdateCRD returns aliased bytes (first byte %q); a caller's scribble reaches every later caller", b[0])
	}
}

// hasString reports whether list is a []any holding s.
func hasString(list any, s string) bool {
	l, _ := list.([]any)
	for _, e := range l {
		if e == s {
			return true
		}
	}
	return false
}
