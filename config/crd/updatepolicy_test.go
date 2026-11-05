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

// The singleton rule on the UpdatePolicy manifest and its message, pinned by
// text. k3sm.io/apis/k3smtest transcribes the same rule text.
const (
	updatePolicyNameRule    = "self.metadata.name == 'cluster'"
	updatePolicyNameMessage = "the UpdatePolicy must be named cluster"
)

// TestUpdatePolicyCRDRefusesAnotherNameOrChannel asserts the embedded
// UpdatePolicy manifest declares the two refusals the setting rests on: a CEL
// rule on the root schema refuses any name but cluster, and a closed enum
// refuses any channel but stable and rc, with stable the default. It also
// pins the rest of the shape: the object is cluster-scoped and spec only, one
// served and stored v1alpha1 version with a Channel printer column, and no
// labels, so no aggregated role can pick it up.
//
// The check is structural, like its siblings: this module carries no CEL
// evaluator. It deliberately does NOT import updates/v1alpha1, so a
// disagreement stays visible; TestUpdatePolicyCRDRoundTrip in
// k3sm.io/apis/k3smtest holds the Go side against this schema.
func TestUpdatePolicyCRDRefusesAnotherNameOrChannel(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, UpdatePolicyCRD())
	v := onlyVersion(t, m, "v1alpha1")
	root := mapAt(t, v, "schema", "openAPIV3Schema")
	sp := specProps(t, v)

	t.Run("identity", func(t *testing.T) {
		if got := m["kind"]; got != "CustomResourceDefinition" {
			t.Errorf("kind = %v, want CustomResourceDefinition", got)
		}
		if UpdatePolicyCRDName != "updatepolicies.updates.k3sm.io" {
			t.Errorf("UpdatePolicyCRDName = %q, want updatepolicies.updates.k3sm.io", UpdatePolicyCRDName)
		}
		if got := mapAt(t, m, "metadata")["name"]; got != UpdatePolicyCRDName {
			t.Errorf("metadata.name = %v, want %s (the accessor's constant)", got, UpdatePolicyCRDName)
		}
		spec := mapAt(t, m, "spec")
		if got := spec["group"]; got != "updates.k3sm.io" {
			t.Errorf("spec.group = %v, want updates.k3sm.io", got)
		}
		if got := spec["scope"]; got != "Cluster" {
			t.Errorf("spec.scope = %v, want Cluster", got)
		}
		names := mapAt(t, m, "spec", "names")
		for _, tc := range []struct{ key, want string }{
			{"kind", "UpdatePolicy"},
			{"listKind", "UpdatePolicyList"},
			{"plural", "updatepolicies"},
			{"singular", "updatepolicy"},
		} {
			if got := names[tc.key]; got != tc.want {
				t.Errorf("spec.names.%s = %v, want %s", tc.key, got, tc.want)
			}
		}
	})

	t.Run("refuses another name", func(t *testing.T) {
		rules, _ := root["x-kubernetes-validations"].([]any)
		if len(rules) != 1 {
			t.Fatalf("openAPIV3Schema x-kubernetes-validations = %v, want exactly one rule", rules)
		}
		r, _ := rules[0].(map[string]any)
		if r["rule"] != updatePolicyNameRule || r["message"] != updatePolicyNameMessage {
			t.Errorf("name rule = %v / %v, want %q / %q", r["rule"], r["message"], updatePolicyNameRule, updatePolicyNameMessage)
		}
		// The apiserver owns metadata; a schema that restricted it here would
		// be a second, weaker statement of the rule.
		md := mapAt(t, root, "properties", "metadata")
		if md["type"] != "object" || len(md) != 1 {
			t.Errorf("openAPIV3Schema.properties.metadata = %v, want only type: object", md)
		}
	})

	t.Run("refuses another channel", func(t *testing.T) {
		assertDeclared(t, "spec", sp, "channel")
		ch := mapAt(t, sp, "channel")
		if got := ch["type"]; got != "string" {
			t.Errorf("spec.channel type = %v, want string", got)
		}
		assertStringEnum(t, "spec.channel", ch, "stable", "rc")
	})

	t.Run("defaults to stable", func(t *testing.T) {
		if got := mapAt(t, sp, "channel")["default"]; got != "stable" {
			t.Errorf("spec.channel default = %v, want stable", got)
		}
		// A nested default applies only when its parent exists, so spec
		// defaults to {} and an object without one still reads stable. Neither
		// level is required, which would refuse what the defaults fill in.
		spec := mapAt(t, root, "properties", "spec")
		if d, ok := spec["default"].(map[string]any); !ok || len(d) != 0 {
			t.Errorf("spec default = %v, want {} so the channel default applies when spec is omitted", spec["default"])
		}
		if got, ok := spec["required"]; ok {
			t.Errorf("spec.required = %v, want none; the channel has a default", got)
		}
		if got, ok := root["required"]; ok {
			t.Errorf("openAPIV3Schema.required = %v, want none; spec has a default", got)
		}
	})

	t.Run("spec only", func(t *testing.T) {
		if _, ok := v["subresources"]; ok {
			t.Error("UpdatePolicy declares subresources; it is spec only")
		}
		assertDeclared(t, "openAPIV3Schema", mapAt(t, root, "properties"), "apiVersion", "kind", "metadata", "spec")
	})

	t.Run("printer column", func(t *testing.T) {
		assertPrinterColumns(t, v, [][2]string{
			{"Channel", ".spec.channel"}, {"Age", ".metadata.creationTimestamp"},
		})
	})

	t.Run("no labels", func(t *testing.T) {
		for k := range mapAt(t, m, "metadata") {
			if k != "name" {
				t.Errorf("metadata.%s is set; the UpdatePolicy CRD carries no labels or annotations", k)
			}
		}
		if s := string(UpdatePolicyCRD()); strings.Contains(s, "aggregate-to-") {
			t.Error("the manifest names an aggregation label; this kind adds no role")
		}
	})
}

// TestUpdatePolicyAccessorReturnsAFreshCopy asserts the UpdatePolicy accessor
// hands out a copy, on the same grounds as its siblings.
func TestUpdatePolicyAccessorReturnsAFreshCopy(t *testing.T) {
	t.Parallel()
	a := UpdatePolicyCRD()
	if len(a) == 0 {
		t.Fatal("UpdatePolicyCRD returned no bytes; the go:embed directive did not match")
	}
	a[0] = 'X'
	if b := UpdatePolicyCRD(); b[0] != '#' {
		t.Fatalf("UpdatePolicyCRD returns aliased bytes (first byte %q); a caller's scribble reaches every later caller", b[0])
	}
}
