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
	"testing"
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
		{"spec.verify.checksums", mapAt(t, sp, "verify", "properties", "checksums"), true},
		{"spec.verify.signature", mapAt(t, sp, "verify", "properties", "signature"), "required"},
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
	assertStringEnum(t, "spec.verify.signature", mapAt(t, sp, "verify", "properties", "signature"), "required", "if-signed")

	stp := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "status", "properties")
	for _, f := range []string{"phase", "nodes", "conditions", "observedGeneration"} {
		if _, ok := stp[f]; !ok {
			t.Errorf("status.%s is not declared; the structural schema would prune it", f)
		}
	}
	assertStringEnum(t, "status.phase", mapAt(t, stp, "phase"),
		"Pending", "Rolling", "Succeeded", "Failed", "Cancelled")
	np := mapAt(t, stp, "nodes", "items", "properties")
	for _, f := range []string{"name", "from", "to", "phase", "message", "startedAt", "finishedAt"} {
		if _, ok := np[f]; !ok {
			t.Errorf("status.nodes[].%s is not declared", f)
		}
	}
	assertStringEnum(t, "status.nodes[].phase", mapAt(t, np, "phase"),
		"Pending", "Cordoning", "Draining", "Downloading", "Verifying", "Installing", "Restarting", "Done", "Failed")
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
