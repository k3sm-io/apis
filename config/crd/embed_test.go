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
	"os"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/yaml"
)

// decodeManifest parses the embedded manifest into a generic map. Parsing rather
// than string-matching is the point: a manifest that no longer parses is one the
// API server would reject at apply time, which is exactly the failure this
// module is supposed to make impossible for its consumer to ship.
func decodeManifest(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("embedded manifest is not valid YAML: %v", err)
	}
	return m
}

func mapAt(t *testing.T, m map[string]any, path ...string) map[string]any {
	t.Helper()
	cur := m
	for i, k := range path {
		v, ok := cur[k]
		if !ok {
			t.Fatalf("manifest has no %s", strings.Join(path[:i+1], "."))
		}
		cur, ok = v.(map[string]any)
		if !ok {
			t.Fatalf("manifest %s is %T, want a mapping", strings.Join(path[:i+1], "."), v)
		}
	}
	return cur
}

// TestMLXModelCRDMatchesTheGoTypes asserts the embedded manifest describes the
// same object the Go types in k3sm.io/apis/mlx/v1alpha1 describe.
//
// The manifest and the Go types are two hand-maintained descriptions of one
// object (this module runs no controller-gen), so nothing but a test keeps them
// in step. The identifiers checked here are the ones whose divergence is silent:
// a wrong group or plural makes the operator watch a path that does not exist,
// and a wrong version makes it watch a version the server does not serve.
//
// It deliberately does NOT import mlx/v1alpha1 — the check has to be able to see
// a disagreement, and reading both sides from the same constant could not.
func TestMLXModelCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, MLXModelCRD())

	if got := m["kind"]; got != "CustomResourceDefinition" {
		t.Errorf("kind = %v, want CustomResourceDefinition", got)
	}
	if got := mapAt(t, m, "metadata")["name"]; got != MLXModelCRDName {
		t.Errorf("metadata.name = %v, want %s (the accessor's constant)", got, MLXModelCRDName)
	}
	if MLXModelCRDName != "mlxmodels.mlx.k3sm.io" {
		t.Errorf("MLXModelCRDName = %q, want mlxmodels.mlx.k3sm.io", MLXModelCRDName)
	}

	spec := mapAt(t, m, "spec")
	if got := spec["group"]; got != "mlx.k3sm.io" {
		t.Errorf("spec.group = %v, want mlx.k3sm.io", got)
	}
	// Namespaced, unlike the cluster-scoped MeshPeer beside it: an MLXModel owns
	// namespaced workload objects.
	if got := spec["scope"]; got != "Namespaced" {
		t.Errorf("spec.scope = %v, want Namespaced", got)
	}
	names := mapAt(t, m, "spec", "names")
	for _, tc := range []struct{ key, want string }{
		{"kind", "MLXModel"},
		{"listKind", "MLXModelList"},
		{"plural", "mlxmodels"},
		{"singular", "mlxmodel"},
	} {
		t.Run("names."+tc.key, func(t *testing.T) {
			if got := names[tc.key]; got != tc.want {
				t.Errorf("spec.names.%s = %v, want %s", tc.key, got, tc.want)
			}
		})
	}
}

// TestMLXModelCRDVersionDiscipline asserts exactly one version, v1alpha1, both
// served and stored — the alpha branding as the API server sees it.
//
// One served+stored version is what makes the alpha licence in the Go package
// doc coherent: with a single version there is no conversion to get wrong when
// an incompatible change lands, and the version string itself is the warning a
// user reads before depending on the shape.
func TestMLXModelCRDVersionDiscipline(t *testing.T) {
	t.Parallel()
	spec := mapAt(t, decodeManifest(t, MLXModelCRD()), "spec")
	versions, ok := spec["versions"].([]any)
	if !ok {
		t.Fatalf("spec.versions is %T, want a list", spec["versions"])
	}
	if len(versions) != 1 {
		t.Fatalf("spec.versions has %d entries, want exactly 1 (an alpha CRD carries no conversion)", len(versions))
	}
	v, ok := versions[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0] is %T, want a mapping", versions[0])
	}
	if got := v["name"]; got != "v1alpha1" {
		t.Errorf("spec.versions[0].name = %v, want v1alpha1", got)
	}
	if got := v["served"]; got != true {
		t.Errorf("spec.versions[0].served = %v, want true", got)
	}
	if got := v["storage"]; got != true {
		t.Errorf("spec.versions[0].storage = %v, want true", got)
	}

	// The status subresource is what makes the conditions-first status writable
	// by the operator without it being able to rewrite the user's spec, and what
	// makes observedGeneration meaningful at all.
	sub, ok := v["subresources"].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0].subresources is %T, want a mapping with status", v["subresources"])
	}
	if _, ok := sub["status"]; !ok {
		t.Error("the status subresource is not enabled; the operator would have to write the whole object")
	}

	// The derived Phase printer column — the one-word summary `kubectl get`
	// shows. Its absence is not a crash, just a permanently unhelpful table.
	cols, ok := v["additionalPrinterColumns"].([]any)
	if !ok {
		t.Fatalf("spec.versions[0].additionalPrinterColumns is %T, want a list", v["additionalPrinterColumns"])
	}
	foundPhase := false
	for _, c := range cols {
		col, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if col["name"] == "Phase" {
			foundPhase = true
			if got := col["jsonPath"]; got != ".status.phase" {
				t.Errorf("Phase printer column jsonPath = %v, want .status.phase", got)
			}
		}
	}
	if !foundPhase {
		t.Error("no Phase printer column; the derived summary is never shown")
	}
}

// TestMLXModelCRDReservesDistributed asserts the CEL rule that rejects a set
// spec.distributed is present in the manifest.
//
// The field is representable ON PURPOSE so a sharding request can be refused
// with a legible reason rather than ignored — and "ignored" is exactly what the
// object degrades to if this rule goes missing: the model would serve
// single-node and report success. The rule's own behaviour is proven by a
// contract test in k3sm against a live apiserver (this module deliberately
// carries no apiextensions machinery); what is provable here, and what this
// asserts, is that the rule is actually shipped in the bytes k3sm applies.
func TestMLXModelCRDReservesDistributed(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, MLXModelCRD())
	versions, ok := mapAt(t, m, "spec")["versions"].([]any)
	if !ok || len(versions) == 0 {
		t.Fatalf("spec.versions is %T, want a non-empty list", mapAt(t, m, "spec")["versions"])
	}
	v, ok := versions[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0] is %T, want a mapping", versions[0])
	}
	schema, ok := v["schema"].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0].schema is %T, want a mapping", v["schema"])
	}
	root, ok := schema["openAPIV3Schema"].(map[string]any)
	if !ok {
		t.Fatal("schema.openAPIV3Schema is missing")
	}
	props, ok := root["properties"].(map[string]any)
	if !ok {
		t.Fatal("openAPIV3Schema.properties is missing")
	}
	specProp, ok := props["spec"].(map[string]any)
	if !ok {
		t.Fatal("openAPIV3Schema.properties.spec is missing")
	}

	// The reserved field must still be declared — an undeclared field under a
	// structural schema is pruned, and a pruned field cannot be rejected.
	specProps, ok := specProp["properties"].(map[string]any)
	if !ok {
		t.Fatal("spec.properties is missing")
	}
	if _, ok := specProps["distributed"]; !ok {
		t.Error("spec.distributed is not declared; structural-schema pruning would drop it and the rejection could never fire")
	}

	rules, ok := specProp["x-kubernetes-validations"].([]any)
	if !ok {
		t.Fatalf("spec has no x-kubernetes-validations; nothing rejects a set spec.distributed")
	}
	found := false
	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if s, _ := rule["rule"].(string); strings.Contains(s, "distributed") {
			found = true
			if !strings.Contains(s, "!has(self.distributed)") {
				t.Errorf("the distributed rule is %q, want the !has(self.distributed) rejection", s)
			}
			// A rejection a user cannot act on is a rejection they will file a bug
			// about; the message must name the field and say it is reserved.
			msg, _ := rule["message"].(string)
			if !strings.Contains(msg, "distributed") || !strings.Contains(msg, "reserved") {
				t.Errorf("the distributed rule's message is %q; it must name the field and say it is reserved", msg)
			}
		}
	}
	if !found {
		t.Error("no CEL rule mentions distributed")
	}

	// Required spec fields: model and memory. Memory carries no default on
	// purpose — a guessed one schedules successfully and then fails at load time
	// on the node, the worst place to learn the number was wrong.
	req, ok := specProp["required"].([]any)
	if !ok {
		t.Fatal("spec.required is missing")
	}
	got := map[string]bool{}
	for _, r := range req {
		if s, ok := r.(string); ok {
			got[s] = true
		}
	}
	for _, want := range []string{"model", "memory"} {
		if !got[want] {
			t.Errorf("spec.required does not include %q", want)
		}
	}
}

// TestAccessorReturnsAFreshCopy asserts the accessor hands out a copy.
//
// The embedded manifest is process-global. A consumer that decodes in place or
// appends to the returned slice would corrupt what every LATER caller applies —
// notably the next pass of a reconcile loop, which would then apply a manifest
// no one wrote.
func TestAccessorReturnsAFreshCopy(t *testing.T) {
	t.Parallel()
	a := MLXModelCRD()
	if len(a) == 0 {
		t.Fatal("MLXModelCRD returned no bytes; the go:embed directive did not match")
	}
	a[0] = 'X'
	b := MLXModelCRD()
	if b[0] == 'X' {
		t.Fatal("MLXModelCRD returns aliased bytes; a caller's scribble reaches every later caller")
	}
	if got := b[0]; got != '#' {
		t.Errorf("second call starts with %q, want the manifest's leading comment", got)
	}
}

// TestNoGlobEmbedAndNoMeshPeerAccessor asserts the embed set is enumerated by
// name and that MeshPeer is not in it.
//
// A glob would make "which CRDs does k3sm apply" a property of what happens to
// be in this directory: adding a manifest file would silently enlist it. That is
// specifically why MeshPeer — which sits right beside the embedded file and is
// applied out-of-band today — has no accessor. Adopting it into this ensure owes
// a mesh-regression check and must be a deliberate act, not an embed-pattern
// side effect.
func TestNoGlobEmbedAndNoMeshPeerAccessor(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("embed.go")
	if err != nil {
		t.Fatalf("read embed.go: %v", err)
	}

	var directives []string
	for _, line := range strings.Split(string(src), "\n") {
		if s := strings.TrimSpace(line); strings.HasPrefix(s, "//go:embed") {
			directives = append(directives, s)
		}
	}
	if len(directives) == 0 {
		t.Fatal("embed.go declares no //go:embed directive")
	}
	for _, d := range directives {
		if strings.ContainsAny(d, "*?[") {
			t.Errorf("//go:embed directive %q uses a glob; each manifest must be embedded by name", d)
		}
		if !strings.Contains(d, "mlx.k3sm.io_mlxmodels.yaml") {
			t.Errorf("unexpected //go:embed directive %q; only the mlxmodels manifest is embedded", d)
		}
	}
	// Comment prose is stripped first — the doc comment explains WHY there is no
	// embed.FS, and that explanation must not read as a violation.
	var code strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		l := line
		if i := strings.Index(l, "//"); i >= 0 {
			l = l[:i]
		}
		code.WriteString(l)
		code.WriteByte('\n')
	}
	if strings.Contains(code.String(), "embed.FS") {
		t.Error("embed.go uses an embed.FS; a filesystem re-introduces the glob problem by another route")
	}

	// The MeshPeer manifest exists beside this package and must stay unembedded.
	if _, err := os.Stat("net.k3sm.io_meshpeers.yaml"); err != nil {
		t.Fatalf("expected the MeshPeer manifest beside this package: %v", err)
	}
	// Same treatment for MeshPeer: the doc comment explains why it is excluded.
	for _, line := range strings.Split(code.String(), "\n") {
		if strings.Contains(line, "meshpeers") || strings.Contains(line, "MeshPeer") {
			t.Errorf("embed.go embeds or exposes MeshPeer (%q); it stays out-of-band", strings.TrimSpace(line))
		}
	}
	// And the bytes handed out are the MLXModel CRD, not something else.
	if !strings.Contains(string(MLXModelCRD()), "mlxmodels.mlx.k3sm.io") {
		t.Error("the embedded manifest is not the MLXModel CRD")
	}
	if strings.Contains(string(MLXModelCRD()), "meshpeers.net.k3sm.io") {
		t.Error("the embedded manifest carries the MeshPeer CRD")
	}
}
