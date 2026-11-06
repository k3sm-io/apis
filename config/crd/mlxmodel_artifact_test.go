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

// modelXorArtifactRule and modelXorArtifactMessage are the exact CEL rule and
// message on spec that make spec.model and spec.artifact mutually exclusive
// and one of them required.
const (
	modelXorArtifactRule    = "has(self.model) != has(self.artifact)"
	modelXorArtifactMessage = "exactly one of spec.model and spec.artifact must be set"
)

// TestMLXModelArtifactFieldCEL asserts the manifest declares spec.artifact and
// status.resolvedArtifact, drops model from spec.required, and carries the
// rule that exactly one of model and artifact is set.
//
// Rollback safety rests on this shape: the released CRD required model and
// the released operator refuses an empty one, so after a rollback an
// artifact-only object is refused rather than rendered as a Hub model with
// egress. This module evaluates no CEL (a k3sm contract test compiles the
// shipped rule against a real apiserver validator), so the rule's meaning is
// applied by hand below against the pinned text.
func TestMLXModelArtifactFieldCEL(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, MLXModelCRD())
	versions, _ := mapAt(t, m, "spec")["versions"].([]any)
	if len(versions) == 0 {
		t.Fatal("spec.versions is empty")
	}
	v, ok := versions[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0] is %T, want a mapping", versions[0])
	}
	spec := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec")
	specProps := mapAt(t, spec, "properties")

	t.Run("spec.artifact is declared with a required reference and an optional variant", func(t *testing.T) {
		art, ok := specProps["artifact"].(map[string]any)
		if !ok {
			t.Fatal("spec.artifact is not declared; structural-schema pruning would drop it")
		}
		if art["type"] != "object" {
			t.Errorf("spec.artifact type = %v, want object", art["type"])
		}
		req, _ := art["required"].([]any)
		if len(req) != 1 || req[0] != "reference" {
			t.Errorf("spec.artifact.required = %v, want [reference]", req)
		}
		props := mapAt(t, art, "properties")
		if len(props) != 2 {
			t.Errorf("spec.artifact declares %d properties, want exactly reference and variant", len(props))
		}
		ref, ok := props["reference"].(map[string]any)
		if !ok {
			t.Fatal("spec.artifact.reference is not declared")
		}
		if ref["type"] != "string" {
			t.Errorf("spec.artifact.reference type = %v, want string", ref["type"])
		}
		if n, _ := ref["minLength"].(int64); n != 1 {
			if f, _ := ref["minLength"].(float64); f != 1 {
				t.Errorf("spec.artifact.reference minLength = %v, want 1", ref["minLength"])
			}
		}
		if vr, ok := props["variant"].(map[string]any); !ok || vr["type"] != "string" {
			t.Errorf("spec.artifact.variant = %v, want a declared string", props["variant"])
		}
	})

	t.Run("status.resolvedArtifact is declared", func(t *testing.T) {
		status := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "status", "properties")
		ra, ok := status["resolvedArtifact"].(map[string]any)
		if !ok || ra["type"] != "string" {
			t.Errorf("status.resolvedArtifact = %v, want a declared string", status["resolvedArtifact"])
		}
	})

	t.Run("spec carries exactly the model-xor-artifact rule", func(t *testing.T) {
		rules, ok := spec["x-kubernetes-validations"].([]any)
		if !ok || len(rules) != 1 {
			t.Fatalf("spec.x-kubernetes-validations = %v, want exactly the model-xor-artifact rule", spec["x-kubernetes-validations"])
		}
		rule, ok := rules[0].(map[string]any)
		if !ok {
			t.Fatalf("spec rule is %T, want a mapping", rules[0])
		}
		if got, _ := rule["rule"].(string); got != modelXorArtifactRule {
			t.Errorf("spec rule = %q, want %q", got, modelXorArtifactRule)
		}
		if got, _ := rule["message"].(string); got != modelXorArtifactMessage {
			t.Errorf("spec message = %q, want %q", got, modelXorArtifactMessage)
		}
		// The rule must sit on spec, not the root: at the root has(self.model)
		// is always false and the rule would refuse every Hub model.
		root := mapAt(t, v, "schema", "openAPIV3Schema")
		if _, ok := root["x-kubernetes-validations"]; ok {
			t.Error("the schema root carries validation rules; the model-xor-artifact rule belongs on spec")
		}
	})

	t.Run("model is optional in the schema and non-empty when set", func(t *testing.T) {
		for _, r := range spec["required"].([]any) {
			if r == "model" {
				t.Fatal("spec.required still lists model")
			}
		}
		model := mapAt(t, specProps, "model")
		if n, _ := model["minLength"].(int64); n != 1 {
			if f, _ := model["minLength"].(float64); f != 1 {
				t.Errorf("spec.model minLength = %v, want 1 (an empty model must not count as set)", model["minLength"])
			}
		}
	})

	t.Run("the rule admits exactly one of model and artifact", func(t *testing.T) {
		// has(self.model) != has(self.artifact), applied by hand; the subtest
		// above pins that this is the shipped text.
		admits := func(model, artifact bool) bool { return model != artifact }
		for _, tc := range []struct {
			name            string
			model, artifact bool
			want            bool
		}{
			{"model only (the Hub path)", true, false, true},
			{"artifact only (the store path)", false, true, true},
			{"both set", true, true, false},
			{"neither set", false, false, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if got := admits(tc.model, tc.artifact); got != tc.want {
					t.Errorf("admits(model=%v, artifact=%v) = %v, want %v", tc.model, tc.artifact, got, tc.want)
				}
			})
		}
	})
}
