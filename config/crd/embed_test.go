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

// TestMLXModelCRDValidatesDistributed asserts the shape validation a set
// spec.distributed meets is present in the manifest.
//
// The field is honoured by the k3sm MLX operator, so the manifest admits it
// with shape validation instead of rejecting it: ranks at least 2 (a CEL rule,
// so an unset ranks is refused too), backend and parallelism constrained by
// their enums. The rule's own behaviour is proven by a contract test in k3sm
// against a live apiserver (this module deliberately carries no apiextensions
// machinery); what is provable here is that the rule is actually shipped in
// the bytes k3sm applies, with exactly the text that test pins.
func TestMLXModelCRDValidatesDistributed(t *testing.T) {
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

	// The field must be declared: an undeclared field under a structural schema
	// is pruned, and a pruned sharding request would serve single-node.
	specProps, ok := specProp["properties"].(map[string]any)
	if !ok {
		t.Fatal("spec.properties is missing")
	}
	dist, ok := specProps["distributed"].(map[string]any)
	if !ok {
		t.Fatal("spec.distributed is not declared; structural-schema pruning would drop it")
	}
	// The shape is ranks/backend/parallelism; the earlier nodes field is gone
	// (an alpha break: it was never storable, so nothing carries it).
	distProps := mapAt(t, dist, "properties")
	if _, ok := distProps["nodes"]; ok {
		t.Error("spec.distributed still declares nodes; the shape is ranks/backend/parallelism")
	}
	for _, tc := range []struct {
		field string
		enum  []string
	}{
		{"ranks", nil},
		{"backend", []string{"auto", "ring", "jaccl"}},
		{"parallelism", []string{"tensor", "pipeline"}},
	} {
		f, ok := distProps[tc.field].(map[string]any)
		if !ok {
			t.Errorf("spec.distributed.%s is not declared", tc.field)
			continue
		}
		if tc.enum == nil {
			continue
		}
		got, _ := f["enum"].([]any)
		if len(got) != len(tc.enum) {
			t.Errorf("spec.distributed.%s enum = %v, want %v", tc.field, got, tc.enum)
			continue
		}
		for i, want := range tc.enum {
			if got[i] != want {
				t.Errorf("spec.distributed.%s enum = %v, want %v", tc.field, got, tc.enum)
				break
			}
		}
	}

	// No rule on spec may refuse a set distributed block any more.
	if specRules, ok := specProp["x-kubernetes-validations"].([]any); ok {
		for _, r := range specRules {
			if rule, ok := r.(map[string]any); ok {
				if s, _ := rule["rule"].(string); strings.Contains(s, "distributed") {
					t.Errorf("spec carries the distributed rule %q; the shape rule belongs on spec.distributed", s)
				}
			}
		}
	}
	rules, ok := dist["x-kubernetes-validations"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("spec.distributed.x-kubernetes-validations = %v, want exactly the ranks rule", dist["x-kubernetes-validations"])
	}
	rule, ok := rules[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.distributed rule is %T, want a mapping", rules[0])
	}
	if got, _ := rule["rule"].(string); got != distributedRanksRule {
		t.Errorf("spec.distributed rule = %q, want %q", got, distributedRanksRule)
	}
	// A refusal a user cannot act on is a refusal they will file a bug about;
	// the message must name the field and the bound.
	if got, _ := rule["message"].(string); got != distributedRanksMessage {
		t.Errorf("spec.distributed message = %q, want %q", got, distributedRanksMessage)
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

// distributedRanksRule and distributedRanksMessage are the exact CEL rule and
// message on spec.distributed. The k3sm contract test pins the same text.
const (
	distributedRanksRule    = "has(self.ranks) && self.ranks >= 2"
	distributedRanksMessage = "spec.distributed.ranks must be set and at least 2 (one rank per node)"
)

// collectRules returns the text of every x-kubernetes-validations rule found
// anywhere under n.
func collectRules(n any) []string {
	var out []string
	switch v := n.(type) {
	case map[string]any:
		if rs, ok := v["x-kubernetes-validations"].([]any); ok {
			for _, r := range rs {
				if rule, ok := r.(map[string]any); ok {
					if s, ok := rule["rule"].(string); ok {
						out = append(out, s)
					}
				}
			}
		}
		for _, c := range v {
			out = append(out, collectRules(c)...)
		}
	case []any:
		for _, c := range v {
			out = append(out, collectRules(c)...)
		}
	}
	return out
}

// TestMLXModelCRDAdmitsValidDistributed asserts a valid spec.distributed block
// is no longer refused by any rule in the manifest, and that an invalid one
// still is.
//
// This module evaluates no CEL, so the check is two-sided on the text: no rule
// anywhere still carries the old !has(self.distributed) refusal, and the one
// rule on spec.distributed is the pinned ranks rule, whose meaning the table
// below applies alongside the schema's own enums. A rule text that drifts from
// the pinned one fails TestMLXModelCRDValidatesDistributed, so the hand
// evaluation here cannot silently describe a different rule.
func TestMLXModelCRDAdmitsValidDistributed(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, MLXModelCRD())
	for _, r := range collectRules(m) {
		if strings.Contains(strings.ReplaceAll(r, " ", ""), "!has(self.distributed") {
			t.Errorf("the manifest still refuses a set spec.distributed: rule %q", r)
		}
	}

	versions, _ := mapAt(t, m, "spec")["versions"].([]any)
	if len(versions) == 0 {
		t.Fatal("spec.versions is empty")
	}
	v, ok := versions[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0] is %T, want a mapping", versions[0])
	}
	dist := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec", "properties", "distributed")
	rules := collectRules(dist)
	if len(rules) != 1 || rules[0] != distributedRanksRule {
		t.Fatalf("spec.distributed rules = %q, want exactly [%q]", rules, distributedRanksRule)
	}
	props := mapAt(t, dist, "properties")
	inEnum := func(field, val string) bool {
		if val == "" {
			return true // absent: the enum does not apply
		}
		f, _ := props[field].(map[string]any)
		enum, _ := f["enum"].([]any)
		for _, e := range enum {
			if e == val {
				return true
			}
		}
		return false
	}
	admits := func(ranks *int64, backend, parallelism string) bool {
		return ranks != nil && *ranks >= 2 && inEnum("backend", backend) && inEnum("parallelism", parallelism)
	}
	n := func(i int64) *int64 { return &i }

	for _, tc := range []struct {
		name        string
		ranks       *int64
		backend     string
		parallelism string
		want        bool
	}{
		{"two ranks, defaults", n(2), "", "", true},
		{"ring pipeline", n(2), "ring", "pipeline", true},
		{"jaccl tensor", n(4), "jaccl", "tensor", true},
		{"auto backend", n(3), "auto", "", true},
		{"ranks unset", nil, "ring", "tensor", false},
		{"one rank", n(1), "", "", false},
		{"zero ranks", n(0), "", "", false},
		{"unknown backend", n(2), "nccl", "", false},
		{"unknown parallelism", n(2), "", "expert", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := admits(tc.ranks, tc.backend, tc.parallelism); got != tc.want {
				t.Errorf("admits = %v, want %v", got, tc.want)
			}
		})
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

// TestMeshPeerCRDMatchesTheGoTypes asserts the embedded manifest describes the
// same object the Go types in k3sm.io/apis/net/v1 describe.
//
// The manifest and the Go types are two hand-maintained descriptions of one
// object (this module runs no controller-gen), so nothing but a test keeps them
// in step. The identifiers checked here are the ones whose divergence is silent:
// a wrong group or plural makes darwin-net watch a path that does not exist, and
// a wrong scope makes the enroll write land in a namespace nobody reads.
//
// It deliberately does NOT import net/v1 — the check has to be able to see a
// disagreement, and reading both sides from the same constant could not.
func TestMeshPeerCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, MeshPeerCRD())

	if got := m["kind"]; got != "CustomResourceDefinition" {
		t.Errorf("kind = %v, want CustomResourceDefinition", got)
	}
	if got := mapAt(t, m, "metadata")["name"]; got != MeshPeerCRDName {
		t.Errorf("metadata.name = %v, want %s (the accessor's constant)", got, MeshPeerCRDName)
	}
	if MeshPeerCRDName != "meshpeers.net.k3sm.io" {
		t.Errorf("MeshPeerCRDName = %q, want meshpeers.net.k3sm.io", MeshPeerCRDName)
	}

	spec := mapAt(t, m, "spec")
	if got := spec["group"]; got != "net.k3sm.io" {
		t.Errorf("spec.group = %v, want net.k3sm.io", got)
	}
	// Cluster-scoped, unlike the namespaced MLXModel beside it: a MeshPeer is
	// node-level mesh topology, one per node and named for the node.
	if got := spec["scope"]; got != "Cluster" {
		t.Errorf("spec.scope = %v, want Cluster", got)
	}
	names := mapAt(t, m, "spec", "names")
	for _, tc := range []struct{ key, want string }{
		{"kind", "MeshPeer"},
		{"listKind", "MeshPeerList"},
		{"plural", "meshpeers"},
		{"singular", "meshpeer"},
	} {
		t.Run("names."+tc.key, func(t *testing.T) {
			if got := names[tc.key]; got != tc.want {
				t.Errorf("spec.names.%s = %v, want %s", tc.key, got, tc.want)
			}
		})
	}
}

// TestMeshPeerCRDVersionDiscipline asserts exactly one version, v1, both served
// and stored.
//
// The manifest's own header promises a SINGLE served+stored v1 with an
// additive-only schema — intra-version payload evolution rides
// spec.schemaVersion, not a new GVK version. A second version appearing here
// would mean conversion the module ships no webhook for.
func TestMeshPeerCRDVersionDiscipline(t *testing.T) {
	t.Parallel()
	spec := mapAt(t, decodeManifest(t, MeshPeerCRD()), "spec")
	versions, ok := spec["versions"].([]any)
	if !ok {
		t.Fatalf("spec.versions is %T, want a list", spec["versions"])
	}
	if len(versions) != 1 {
		t.Fatalf("spec.versions has %d entries, want exactly 1 (evolution rides spec.schemaVersion)", len(versions))
	}
	v, ok := versions[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0] is %T, want a mapping", versions[0])
	}
	if got := v["name"]; got != "v1" {
		t.Errorf("spec.versions[0].name = %v, want v1", got)
	}
	if got := v["served"]; got != true {
		t.Errorf("spec.versions[0].served = %v, want true", got)
	}
	if got := v["storage"]; got != true {
		t.Errorf("spec.versions[0].storage = %v, want true", got)
	}
}

// TestMeshPeerCRDDeclaresEndpoints asserts spec.endpoints is declared in the
// MeshPeer structural schema with its address/link items, and that link is an
// open string.
//
// An undeclared field is pruned by the apiserver on write, so a node's endpoint
// candidates would vanish between writer and reader with no error anywhere.
// The link stays enum-free because readers ignore unknown link values; an enum
// would turn a future value into an apiserver rejection instead.
func TestMeshPeerCRDDeclaresEndpoints(t *testing.T) {
	t.Parallel()
	v := onlyVersion(t, decodeManifest(t, MeshPeerCRD()), "v1")
	props := specProps(t, v)
	ep, ok := props["endpoints"].(map[string]any)
	if !ok {
		t.Fatal("spec.endpoints is not declared; the structural schema would prune it")
	}
	if got := ep["type"]; got != "array" {
		t.Errorf("spec.endpoints.type = %v, want array", got)
	}
	items := mapAt(t, ep, "items")
	itemProps := mapAt(t, items, "properties")
	for _, f := range []string{"address", "link"} {
		if _, ok := itemProps[f]; !ok {
			t.Errorf("spec.endpoints[].%s is not declared", f)
		}
	}
	if _, ok := mapAt(t, itemProps, "link")["enum"]; ok {
		t.Error("spec.endpoints[].link carries an enum; readers ignore unknown values, so the apiserver must not refuse them")
	}
	// The pre-existing required set is unchanged: endpoints is optional.
	req, _ := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec")["required"].([]any)
	for _, r := range req {
		if r == "endpoints" {
			t.Error("spec.endpoints is required; it is an additive optional field")
		}
	}
}

// TestMeshPeerAccessorReturnsAFreshCopy asserts the MeshPeer accessor hands out
// a copy, on the same grounds as its MLXModel sibling: the embedded manifest is
// process-global, and k3sm's server re-applies it on every mesh-path bring-up,
// so a caller's in-place decode would corrupt what every later apply sends.
func TestMeshPeerAccessorReturnsAFreshCopy(t *testing.T) {
	t.Parallel()
	a := MeshPeerCRD()
	if len(a) == 0 {
		t.Fatal("MeshPeerCRD returned no bytes; the go:embed directive did not match")
	}
	a[0] = 'X'
	b := MeshPeerCRD()
	if b[0] == 'X' {
		t.Fatal("MeshPeerCRD returns aliased bytes; a caller's scribble reaches every later caller")
	}
	if got := b[0]; got != '#' {
		t.Errorf("second call starts with %q, want the manifest's leading comment", got)
	}
}

// TestNoGlobEmbed asserts the embed set is enumerated by name, one directive per
// manifest, with no glob and no embed.FS.
//
// A glob would make "which CRDs does k3sm apply" a property of what happens to
// be in this directory: adding a manifest file would silently enlist it. The
// enumerated set below is therefore the reviewable list itself — a new entry can
// only appear by editing this test, which is the review this convention exists
// to force.
func TestNoGlobEmbed(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("embed.go")
	if err != nil {
		t.Fatalf("read embed.go: %v", err)
	}

	// The manifests deliberately embedded, each by its own named directive.
	embedded := map[string]bool{
		"mlx.k3sm.io_mlxmodels.yaml":         false,
		"net.k3sm.io_meshpeers.yaml":         false,
		"net.k3sm.io_directlinks.yaml":       false,
		"helm.k3sm.io_helmcharts.yaml":       false,
		"helm.k3sm.io_helmchartconfigs.yaml": false,
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
		matched := false
		for name := range embedded {
			if strings.Contains(d, name) {
				embedded[name] = true
				matched = true
			}
		}
		if !matched {
			t.Errorf("unexpected //go:embed directive %q; enlisting a manifest is a reviewable act, so add it to this test's list first", d)
		}
	}
	for name, seen := range embedded {
		if !seen {
			t.Errorf("no //go:embed directive names %s; the accessor for it would hand out nothing", name)
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

	// Every named manifest exists beside this package — a directive naming a file
	// that is not here does not compile, but a file removed from the tree while
	// the list keeps its name is the drift worth naming explicitly.
	for name := range embedded {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("expected the %s manifest beside this package: %v", name, err)
		}
	}

	// And each accessor hands out its OWN manifest, not its neighbour's — the
	// failure a second named embed makes possible for the first time.
	if !strings.Contains(string(MLXModelCRD()), MLXModelCRDName) {
		t.Error("MLXModelCRD does not return the MLXModel CRD")
	}
	if strings.Contains(string(MLXModelCRD()), MeshPeerCRDName) {
		t.Error("MLXModelCRD returns the MeshPeer CRD")
	}
	if !strings.Contains(string(MeshPeerCRD()), MeshPeerCRDName) {
		t.Error("MeshPeerCRD does not return the MeshPeer CRD")
	}
	if strings.Contains(string(MeshPeerCRD()), MLXModelCRDName) {
		t.Error("MeshPeerCRD returns the MLXModel CRD")
	}
	if !strings.Contains(string(DirectLinkCRD()), DirectLinkCRDName) {
		t.Error("DirectLinkCRD does not return the DirectLink CRD")
	}
	if strings.Contains(string(DirectLinkCRD()), MeshPeerCRDName) {
		t.Error("DirectLinkCRD returns the MeshPeer CRD")
	}
	if strings.Contains(string(MeshPeerCRD()), DirectLinkCRDName) {
		t.Error("MeshPeerCRD returns the DirectLink CRD")
	}
	if !strings.Contains(string(HelmChartCRD()), HelmChartCRDName) {
		t.Error("HelmChartCRD does not return the HelmChart CRD")
	}
	if strings.Contains(string(HelmChartCRD()), HelmChartConfigCRDName) {
		t.Error("HelmChartCRD returns the HelmChartConfig CRD")
	}
	if !strings.Contains(string(HelmChartConfigCRD()), HelmChartConfigCRDName) {
		t.Error("HelmChartConfigCRD does not return the HelmChartConfig CRD")
	}
	if strings.Contains(string(HelmChartConfigCRD()), HelmChartCRDName) {
		t.Error("HelmChartConfigCRD returns the HelmChart CRD")
	}
}

// TestDirectLinkCRDMatchesTheGoTypes asserts the embedded DirectLink manifest
// describes the object k3sm.io/apis/net/v1alpha1 describes: its identity, the
// single served+stored v1alpha1 version, the status subresource, and every spec
// and status field declared.
//
// It deliberately does NOT import net/v1alpha1, like its siblings, so a
// disagreement stays visible; the field lists below are the reviewable copy.
// An undeclared field is pruned by the structural schema on write, so a port
// attribute the node reports would silently never reach the resolver.
func TestDirectLinkCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, DirectLinkCRD())

	if got := m["kind"]; got != "CustomResourceDefinition" {
		t.Errorf("kind = %v, want CustomResourceDefinition", got)
	}
	if got := mapAt(t, m, "metadata")["name"]; got != DirectLinkCRDName {
		t.Errorf("metadata.name = %v, want %s (the accessor's constant)", got, DirectLinkCRDName)
	}
	if DirectLinkCRDName != "directlinks.net.k3sm.io" {
		t.Errorf("DirectLinkCRDName = %q, want directlinks.net.k3sm.io", DirectLinkCRDName)
	}
	spec := mapAt(t, m, "spec")
	if got := spec["group"]; got != "net.k3sm.io" {
		t.Errorf("spec.group = %v, want net.k3sm.io", got)
	}
	// Cluster-scoped and one per node, like MeshPeer.
	if got := spec["scope"]; got != "Cluster" {
		t.Errorf("spec.scope = %v, want Cluster", got)
	}
	names := mapAt(t, m, "spec", "names")
	for _, tc := range []struct{ key, want string }{
		{"kind", "DirectLink"},
		{"listKind", "DirectLinkList"},
		{"plural", "directlinks"},
		{"singular", "directlink"},
	} {
		t.Run("names."+tc.key, func(t *testing.T) {
			if got := names[tc.key]; got != tc.want {
				t.Errorf("spec.names.%s = %v, want %s", tc.key, got, tc.want)
			}
		})
	}

	v := onlyVersion(t, m, "v1alpha1")

	// The status subresource is what lets the server write the resolved link
	// state without being able to rewrite a node's spec, and the node write its
	// spec without clobbering the resolver's status.
	sub, ok := v["subresources"].(map[string]any)
	if !ok {
		t.Fatal("spec.versions[0].subresources is missing")
	}
	if _, ok := sub["status"]; !ok {
		t.Error("the status subresource is not enabled")
	}

	sp := specProps(t, v)
	for _, f := range []string{"schemaVersion", "nodeName", "medium", "ports"} {
		if _, ok := sp[f]; !ok {
			t.Errorf("spec.%s is not declared; the structural schema would prune it", f)
		}
	}
	portProps := mapAt(t, sp, "ports", "items", "properties")
	for _, f := range []string{
		"iface", "portOrdinal", "domainUUID", "peerDomainUUID", "speedGbps",
		"rdmaDevice", "linkIP", "linkUp", "routeReady", "tunnelOnly",
	} {
		if _, ok := portProps[f]; !ok {
			t.Errorf("spec.ports[].%s is not declared; the structural schema would prune it", f)
		}
	}
	if enum, _ := mapAt(t, sp, "medium")["enum"].([]any); len(enum) != 1 || enum[0] != "thunderbolt" {
		t.Errorf("spec.medium enum = %v, want [thunderbolt]", enum)
	}
	if got := mapAt(t, portProps, "iface")["pattern"]; got != "^en[0-9]+$" {
		t.Errorf("spec.ports[].iface pattern = %v, want ^en[0-9]+$", got)
	}

	stProps := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "status", "properties")
	if _, ok := stProps["observedSchemaVersion"]; !ok {
		t.Error("status.observedSchemaVersion is not declared")
	}
	psProps := mapAt(t, stProps, "ports", "items", "properties")
	for _, f := range []string{
		"iface", "peerNodeName", "peerIface", "peerLinkIP", "peerRDMADevice",
		"state", "lastTransition",
	} {
		if _, ok := psProps[f]; !ok {
			t.Errorf("status.ports[].%s is not declared; the structural schema would prune it", f)
		}
	}
	enum, _ := mapAt(t, psProps, "state")["enum"].([]any)
	got := map[any]bool{}
	for _, e := range enum {
		got[e] = true
	}
	if len(enum) != 3 || !got["up"] || !got["peer-unknown"] || !got["down"] {
		t.Errorf("status.ports[].state enum = %v, want [up peer-unknown down]", enum)
	}
}

// TestDirectLinkAccessorReturnsAFreshCopy asserts the DirectLink accessor
// hands out a copy, on the same grounds as its siblings.
func TestDirectLinkAccessorReturnsAFreshCopy(t *testing.T) {
	t.Parallel()
	a := DirectLinkCRD()
	if len(a) == 0 {
		t.Fatal("DirectLinkCRD returned no bytes; the go:embed directive did not match")
	}
	a[0] = 'X'
	b := DirectLinkCRD()
	if b[0] == 'X' {
		t.Fatal("DirectLinkCRD returns aliased bytes; a caller's scribble reaches every later caller")
	}
	if got := b[0]; got != '#' {
		t.Errorf("second call starts with %q, want the manifest's leading comment", got)
	}
}
