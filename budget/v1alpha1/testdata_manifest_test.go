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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

// The status testdata directory, its digest manifest, its schema path list,
// and the NodeBudget CRD the path list is derived from.
var (
	statusTestdataDir   = filepath.Join("testdata", "status")
	statusSumsFile      = filepath.Join(statusTestdataDir, "SHA256SUMS")
	statusPathsFile     = filepath.Join(statusTestdataDir, "nodebudget.paths.json")
	nodeBudgetCRDSource = filepath.Join("..", "..", "config", "crd", "budget.k3sm.io_nodebudgets.yaml")
)

// Kinds of a schema path in the path list.
const (
	pathKindScalar = "scalar"
	pathKindArray  = "array"
	pathKindObject = "object"
)

// schemaPath is one entry of nodebudget.paths.json: a dotted property path
// of the status schema ("[]" for an array element), its kind and, for a
// scalar, its OpenAPI type and format.
type schemaPath struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Type   string `json:"type,omitempty"`
	Format string `json:"format,omitempty"`
}

// openAPISchema is the slice of an OpenAPI v3 schema the path walk reads.
type openAPISchema struct {
	Type       string                   `json:"type"`
	Format     string                   `json:"format"`
	Properties map[string]openAPISchema `json:"properties"`
	Items      *openAPISchema           `json:"items"`
	Required   []string                 `json:"required"`
}

// nodeBudgetStatusSchema reads the status schema of the v1alpha1 version
// from the NodeBudget CRD.
func nodeBudgetStatusSchema(t *testing.T) openAPISchema {
	t.Helper()
	raw, err := os.ReadFile(nodeBudgetCRDSource)
	if err != nil {
		t.Fatalf("read CRD: %v", err)
	}
	var crd struct {
		Spec struct {
			Versions []struct {
				Name   string `json:"name"`
				Schema struct {
					OpenAPIV3Schema openAPISchema `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	if err := utilyaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("parse CRD: %v", err)
	}
	for _, v := range crd.Spec.Versions {
		if v.Name != "v1alpha1" {
			continue
		}
		st, ok := v.Schema.OpenAPIV3Schema.Properties["status"]
		if !ok {
			t.Fatal("the v1alpha1 schema has no status property")
		}
		return st
	}
	t.Fatal("the CRD serves no v1alpha1 version")
	return openAPISchema{}
}

// walkSchema appends every path of s under prefix to paths and every
// required property path to required. A schema without a type, or an array
// without items, is an error: the path list must say what each path holds.
func walkSchema(s openAPISchema, prefix string, paths *[]schemaPath, required *[]string) error {
	switch s.Type {
	case "object":
		*paths = append(*paths, schemaPath{Path: prefix, Kind: pathKindObject})
		for _, r := range s.Required {
			if _, ok := s.Properties[r]; !ok {
				return fmt.Errorf("%s requires %q, which it does not declare", prefix, r)
			}
			*required = append(*required, prefix+"."+r)
		}
		for name, child := range s.Properties {
			if err := walkSchema(child, prefix+"."+name, paths, required); err != nil {
				return err
			}
		}
	case "array":
		if s.Items == nil {
			return fmt.Errorf("%s is an array without items", prefix)
		}
		*paths = append(*paths, schemaPath{Path: prefix, Kind: pathKindArray})
		return walkSchema(*s.Items, prefix+"[]", paths, required)
	case "string", "integer", "number", "boolean":
		*paths = append(*paths, schemaPath{Path: prefix, Kind: pathKindScalar, Type: s.Type, Format: s.Format})
	default:
		return fmt.Errorf("%s has unsupported schema type %q", prefix, s.Type)
	}
	return nil
}

// derivedStatusPaths returns the sorted path list and the sorted required
// paths of the CRD's status schema.
func derivedStatusPaths(t *testing.T) ([]schemaPath, []string) {
	t.Helper()
	var paths []schemaPath
	var required []string
	if err := walkSchema(nodeBudgetStatusSchema(t), "status", &paths, &required); err != nil {
		t.Fatalf("walk the status schema: %v", err)
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i].Path < paths[j].Path })
	sort.Strings(required)
	return paths, required
}

// renderStatusPaths is the byte form of nodebudget.paths.json.
func renderStatusPaths(t *testing.T, paths []schemaPath) []byte {
	t.Helper()
	b, err := json.MarshalIndent(paths, "", "  ")
	if err != nil {
		t.Fatalf("marshal paths: %v", err)
	}
	return append(b, '\n')
}

// renderStatusSums is the byte form of SHA256SUMS over every regular file in
// the status testdata directory except the manifest itself, sorted by name,
// in the "<hex>  <name>" form `sha256sum -c` and `shasum -a 256 -c` read.
func renderStatusSums(t *testing.T) []byte {
	t.Helper()
	entries, err := os.ReadDir(statusTestdataDir)
	if err != nil {
		t.Fatalf("read %s: %v", statusTestdataDir, err)
	}
	var buf bytes.Buffer
	for _, e := range entries {
		if !e.Type().IsRegular() || e.Name() == filepath.Base(statusSumsFile) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(statusTestdataDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		sum := sha256.Sum256(b)
		fmt.Fprintf(&buf, "%s  %s\n", hex.EncodeToString(sum[:]), e.Name())
	}
	return buf.Bytes()
}

// goldenNodes returns every path present in a golden status document (each
// object and array node as well as each leaf) and, separately, its leaves.
func goldenNodes(t *testing.T, file string) (all, leaves map[string]bool) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	all, leaves = map[string]bool{}, map[string]bool{}
	var walk func(v any, prefix string)
	walk = func(v any, prefix string) {
		all[prefix] = true
		switch x := v.(type) {
		case map[string]any:
			if len(x) == 0 {
				leaves[prefix] = true
			}
			for k, c := range x {
				walk(c, prefix+"."+k)
			}
		case []any:
			if len(x) == 0 {
				leaves[prefix] = true
			}
			for _, c := range x {
				walk(c, prefix+"[]")
			}
		default:
			leaves[prefix] = true
		}
	}
	walk(doc["status"], "status")
	return all, leaves
}

// writeGoldenStatusDocuments rewrites the two golden documents in the form
// TestNodeBudgetGoldenStatusDocuments pins, so one UPDATE_GOLDEN run leaves
// the manifest digesting the regenerated bytes whatever order tests run in.
func writeGoldenStatusDocuments(t *testing.T) {
	t.Helper()
	for p, nb := range samples() {
		b, err := json.MarshalIndent(nb, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s: %v", p, err)
		}
		if err := os.WriteFile(goldenStatusFiles[p], append(b, '\n'), 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
}

// TestBudgetStatusTestdataCurrent pins the SDK vendoring contract under
// testdata/status: the path list equals the paths derived from the CRD's
// status schema, every golden leaf is a listed path, every required schema
// path appears in a golden, and SHA256SUMS digests the files as they are.
// Regenerate deliberately with UPDATE_GOLDEN=1, which also rewrites the
// goldens. The subtests run in order: the manifest digests the path list.
func TestBudgetStatusTestdataCurrent(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") != ""
	if update {
		writeGoldenStatusDocuments(t)
	}
	paths, required := derivedStatusPaths(t)

	t.Run("paths file matches the CRD", func(t *testing.T) {
		got := renderStatusPaths(t, paths)
		if update {
			if err := os.WriteFile(statusPathsFile, got, 0o644); err != nil {
				t.Fatalf("update paths: %v", err)
			}
		}
		want, err := os.ReadFile(statusPathsFile)
		if err != nil {
			t.Fatalf("read paths: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s is stale against %s; regenerate with UPDATE_GOLDEN=1", statusPathsFile, nodeBudgetCRDSource)
		}
		// Not vacuous: the walk reaches scalars, arrays, objects and the
		// deepest nested list elements.
		kinds := map[string]string{}
		for _, p := range paths {
			kinds[p.Path] = p.Kind
		}
		for path, kind := range map[string]string{
			"status":                            pathKindObject,
			"status.signals":                    pathKindArray,
			"status.signals[]":                  pathKindObject,
			"status.signals[].id":               pathKindScalar,
			"status.counters[].windows[].count": pathKindScalar,
		} {
			if kinds[path] != kind {
				t.Errorf("path %s: kind %q, want %q", path, kinds[path], kind)
			}
		}
	})

	listed := map[string]bool{}
	for _, p := range paths {
		listed[p.Path] = true
	}
	present := map[string]bool{}
	for p, file := range goldenStatusFiles {
		t.Run("golden leaves are listed/"+string(p), func(t *testing.T) {
			all, leaves := goldenNodes(t, file)
			for n := range all {
				present[n] = true
			}
			var unlisted []string
			for l := range leaves {
				if !listed[l] {
					unlisted = append(unlisted, l)
				}
			}
			sort.Strings(unlisted)
			if len(unlisted) > 0 {
				t.Fatalf("%s sets %d paths the CRD status schema does not declare:\n%s", file, len(unlisted), strings.Join(unlisted, "\n"))
			}
			if len(leaves) == 0 {
				t.Fatalf("%s has no status leaves", file)
			}
		})
	}

	t.Run("required paths appear in a golden", func(t *testing.T) {
		if len(required) == 0 {
			t.Fatal("the status schema declares no required property; the check would be vacuous")
		}
		var missing []string
		for _, r := range required {
			if !present[r] {
				missing = append(missing, r)
			}
		}
		if len(missing) > 0 {
			t.Fatalf("no golden document sets %d required paths:\n%s", len(missing), strings.Join(missing, "\n"))
		}
	})

	t.Run("SHA256SUMS matches the files", func(t *testing.T) {
		got := renderStatusSums(t)
		if update {
			if err := os.WriteFile(statusSumsFile, got, 0o644); err != nil {
				t.Fatalf("update sums: %v", err)
			}
		}
		want, err := os.ReadFile(statusSumsFile)
		if err != nil {
			t.Fatalf("read sums: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s is stale; regenerate with UPDATE_GOLDEN=1\n got = %s\nwant = %s", statusSumsFile, got, want)
		}
		for _, name := range []string{"darwin.golden.json", "linux.golden.json", filepath.Base(statusPathsFile)} {
			if !strings.Contains(string(got), "  "+name+"\n") {
				t.Errorf("SHA256SUMS does not cover %s", name)
			}
		}
	})
}
