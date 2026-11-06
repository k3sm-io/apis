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

package mlxv1alpha1

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k3sm.io/apis/config/crd"
)

// artifactMLXModel is an artifact-only MLXModel: no Model, Spec.Artifact set,
// Status.ResolvedArtifact recorded.
func artifactMLXModel() *MLXModel {
	return &MLXModel{
		TypeMeta:   metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "MLXModel"},
		ObjectMeta: metav1.ObjectMeta{Name: "qwen", Namespace: "ml", Generation: 2},
		Spec: MLXModelSpec{
			Artifact: &ArtifactRef{Reference: "registry.k3sm.local/models/qwen:4bit", Variant: "4bit"},
			Memory:   resource.MustParse("8Gi"),
		},
		Status: MLXModelStatus{
			ObservedGeneration: 2,
			ResolvedArtifact:   "sha256:" + strings.Repeat("a", 64),
		},
	}
}

// TestMLXModelArtifactJSON pins the wire form of the artifact fields: an
// artifact-only object carries no model key (an empty string would count as
// set to the CRD's model-xor-artifact rule), the field names are artifact,
// reference, variant and resolvedArtifact, and the object round-trips.
func TestMLXModelArtifactJSON(t *testing.T) {
	t.Parallel()
	in := artifactMLXModel()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	spec, _ := raw["spec"].(map[string]any)
	if _, ok := spec["model"]; ok {
		t.Errorf("artifact-only spec serializes a model key: %s", b)
	}
	art, _ := spec["artifact"].(map[string]any)
	if art["reference"] != "registry.k3sm.local/models/qwen:4bit" || art["variant"] != "4bit" {
		t.Errorf("spec.artifact = %v, want reference and variant", spec["artifact"])
	}
	status, _ := raw["status"].(map[string]any)
	if status["resolvedArtifact"] != in.Status.ResolvedArtifact {
		t.Errorf("status.resolvedArtifact = %v, want %s", status["resolvedArtifact"], in.Status.ResolvedArtifact)
	}

	var out MLXModel
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(out.Spec.Artifact, in.Spec.Artifact) || out.Status.ResolvedArtifact != in.Status.ResolvedArtifact || out.Spec.Model != "" {
		t.Errorf("round-trip lost the artifact fields: got spec.artifact=%+v status.resolvedArtifact=%q model=%q",
			out.Spec.Artifact, out.Status.ResolvedArtifact, out.Spec.Model)
	}

	t.Run("a variantless reference omits variant", func(t *testing.T) {
		b, err := json.Marshal(ArtifactRef{Reference: "r"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if got := string(b); got != `{"reference":"r"}` {
			t.Errorf("ArtifactRef JSON = %s, want {\"reference\":\"r\"}", got)
		}
	})

	t.Run("DeepCopy does not alias the artifact", func(t *testing.T) {
		cp := in.DeepCopy()
		in.Spec.Artifact.Reference = "mutated"
		if cp.Spec.Artifact.Reference == "mutated" {
			t.Error("DeepCopy shares Spec.Artifact with the original")
		}
	})
}

// TestMLXModelTypesMatchCRDSchema round-trips the Go types against the shipped
// CRD schema, both ways: every JSON field of MLXModelSpec and MLXModelStatus
// (recursively) is declared in the schema with a matching type, and every
// property the schema declares under spec and status has a Go field. The two
// are hand-maintained descriptions of one object (this module runs no
// controller-gen); a field on one side only is either pruned by the apiserver
// or unreadable by the operator, and nothing else would notice.
func TestMLXModelTypesMatchCRDSchema(t *testing.T) {
	t.Parallel()
	var m map[string]any
	if err := yaml.Unmarshal(crd.MLXModelCRD(), &m); err != nil {
		t.Fatalf("decode the shipped MLXModel CRD: %v", err)
	}
	versions, _ := m["spec"].(map[string]any)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("CRD declares %d versions, want 1", len(versions))
	}
	root := schemaAt(t, versions[0].(map[string]any), "schema", "openAPIV3Schema", "properties")
	for _, tc := range []struct {
		name string
		typ  reflect.Type
	}{
		{"spec", reflect.TypeOf(MLXModelSpec{})},
		{"status", reflect.TypeOf(MLXModelStatus{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, ok := root[tc.name].(map[string]any)
			if !ok {
				t.Fatalf("schema has no %s", tc.name)
			}
			matchSchema(t, tc.name, s, tc.typ)
		})
	}
}

func schemaAt(t *testing.T, m map[string]any, path ...string) map[string]any {
	t.Helper()
	cur := m
	for _, k := range path {
		next, ok := cur[k].(map[string]any)
		if !ok {
			t.Fatalf("schema has no mapping at %s", strings.Join(path, "."))
		}
		cur = next
	}
	return cur
}

var (
	quantityType = reflect.TypeOf(resource.Quantity{})
	timeType     = reflect.TypeOf(metav1.Time{})
)

// matchSchema checks that the schema node s at path describes typ.
func matchSchema(t *testing.T, path string, s map[string]any, typ reflect.Type) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch {
	case typ == quantityType:
		if s["x-kubernetes-int-or-string"] != true {
			t.Errorf("%s is a resource.Quantity but the schema is not int-or-string", path)
		}
		return
	case typ == timeType:
		if s["type"] != "string" {
			t.Errorf("%s is a metav1.Time but the schema type is %v", path, s["type"])
		}
		return
	}
	want := map[reflect.Kind]string{
		reflect.String: "string", reflect.Bool: "boolean",
		reflect.Int32: "integer", reflect.Int64: "integer",
		reflect.Struct: "object", reflect.Map: "object", reflect.Slice: "array",
	}[typ.Kind()]
	if want == "" {
		t.Errorf("%s has Go kind %v, which this check does not map", path, typ.Kind())
		return
	}
	if s["type"] != want {
		t.Errorf("%s schema type = %v, want %s (Go %v)", path, s["type"], want, typ)
		return
	}
	switch typ.Kind() {
	case reflect.Slice:
		items, ok := s["items"].(map[string]any)
		if !ok {
			t.Errorf("%s is a list without items", path)
			return
		}
		matchSchema(t, path+"[]", items, typ.Elem())
	case reflect.Map:
		ap, ok := s["additionalProperties"].(map[string]any)
		if !ok {
			t.Errorf("%s is a map without additionalProperties", path)
			return
		}
		matchSchema(t, path+"{}", ap, typ.Elem())
	case reflect.Struct:
		props, _ := s["properties"].(map[string]any)
		goFields := map[string]bool{}
		for i := range typ.NumField() {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			goFields[name] = true
			ps, ok := props[name].(map[string]any)
			if !ok {
				t.Errorf("%s.%s (Go %s.%s) is not declared in the CRD schema; the apiserver would prune it", path, name, typ.Name(), f.Name)
				continue
			}
			matchSchema(t, path+"."+name, ps, f.Type)
		}
		var extra []string
		for name := range props {
			if !goFields[name] {
				extra = append(extra, name)
			}
		}
		sort.Strings(extra)
		for _, name := range extra {
			t.Errorf("%s.%s is declared in the CRD schema but %s has no such field", path, name, typ.Name())
		}
	}
}

// TestArtifactRefBoundsMatchCRD pins that the CRD's maxLength on
// spec.artifact.reference and spec.artifact.variant equals the Go bounds.
func TestArtifactRefBoundsMatchCRD(t *testing.T) {
	t.Parallel()
	var m map[string]any
	if err := yaml.Unmarshal(crd.MLXModelCRD(), &m); err != nil {
		t.Fatalf("decode the shipped MLXModel CRD: %v", err)
	}
	versions, _ := m["spec"].(map[string]any)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("CRD declares %d versions, want 1", len(versions))
	}
	art := schemaAt(t, versions[0].(map[string]any),
		"schema", "openAPIV3Schema", "properties", "spec", "properties", "artifact", "properties")
	for _, tc := range []struct {
		field string
		want  int64
	}{
		{"reference", MaxArtifactReferenceLength},
		{"variant", MaxArtifactVariantLength},
	} {
		f, _ := art[tc.field].(map[string]any)
		var got int64
		switch v := f["maxLength"].(type) {
		case int64:
			got = v
		case float64:
			got = int64(v)
		}
		if got != tc.want {
			t.Errorf("spec.artifact.%s maxLength = %v, want %d", tc.field, f["maxLength"], tc.want)
		}
	}
}
