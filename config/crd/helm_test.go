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

// onlyVersion asserts the manifest declares exactly one version, served AND
// stored, named want, and returns it. One served+stored v1 is the additive-only
// contract the helm.k3sm.io package doc makes: a second version would mean a
// conversion this module ships no webhook for.
func onlyVersion(t *testing.T, m map[string]any, want string) map[string]any {
	t.Helper()
	versions, ok := mapAt(t, m, "spec")["versions"].([]any)
	if !ok {
		t.Fatalf("spec.versions is %T, want a list", mapAt(t, m, "spec")["versions"])
	}
	if len(versions) != 1 {
		t.Fatalf("spec.versions has %d entries, want exactly 1 (served+stored v1, additive-only)", len(versions))
	}
	v, ok := versions[0].(map[string]any)
	if !ok {
		t.Fatalf("spec.versions[0] is %T, want a mapping", versions[0])
	}
	if got := v["name"]; got != want {
		t.Errorf("spec.versions[0].name = %v, want %s", got, want)
	}
	if got := v["served"]; got != true {
		t.Errorf("spec.versions[0].served = %v, want true", got)
	}
	if got := v["storage"]; got != true {
		t.Errorf("spec.versions[0].storage = %v, want true", got)
	}
	return v
}

// assertIdentity checks the identifiers whose divergence from the Go types in
// k3sm.io/apis/helm/v1 is silent: a wrong group or plural makes the controller
// watch a path that does not exist. Like the MLXModel test, it deliberately
// does NOT import helm/v1, so a disagreement stays visible.
func assertIdentity(t *testing.T, m map[string]any, crdName, constName, kind, plural string) {
	t.Helper()
	if got := m["kind"]; got != "CustomResourceDefinition" {
		t.Errorf("kind = %v, want CustomResourceDefinition", got)
	}
	if got := mapAt(t, m, "metadata")["name"]; got != constName {
		t.Errorf("metadata.name = %v, want %s (the accessor's constant)", got, constName)
	}
	if constName != crdName {
		t.Errorf("CRD name constant = %q, want %s", constName, crdName)
	}
	spec := mapAt(t, m, "spec")
	if got := spec["group"]; got != "helm.k3sm.io" {
		t.Errorf("spec.group = %v, want helm.k3sm.io", got)
	}
	// Namespaced, as in k3s: a HelmChart lives beside the release it installs.
	if got := spec["scope"]; got != "Namespaced" {
		t.Errorf("spec.scope = %v, want Namespaced", got)
	}
	names := mapAt(t, m, "spec", "names")
	for _, tc := range []struct{ key, want string }{
		{"kind", kind},
		{"listKind", kind + "List"},
		{"plural", plural},
		{"singular", strings.ToLower(kind)},
	} {
		t.Run("names."+tc.key, func(t *testing.T) {
			if got := names[tc.key]; got != tc.want {
				t.Errorf("spec.names.%s = %v, want %s", tc.key, got, tc.want)
			}
		})
	}
}

// specProps returns spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.
func specProps(t *testing.T, v map[string]any) map[string]any {
	t.Helper()
	return mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec", "properties")
}

// assertFailurePolicyEnum asserts spec.failurePolicy is the closed enum
// [abort, reinstall]. An open string would let a typo ("Abort") through
// admission and silently fall back to the reinstall default.
func assertFailurePolicyEnum(t *testing.T, props map[string]any) {
	t.Helper()
	fp, ok := props["failurePolicy"].(map[string]any)
	if !ok {
		t.Fatal("spec.failurePolicy is not declared")
	}
	enum, ok := fp["enum"].([]any)
	if !ok {
		t.Fatal("spec.failurePolicy has no enum; any string would be admitted")
	}
	got := map[any]bool{}
	for _, e := range enum {
		got[e] = true
	}
	if len(enum) != 2 || !got["abort"] || !got["reinstall"] {
		t.Errorf("spec.failurePolicy enum = %v, want [abort reinstall]", enum)
	}
}

// TestHelmChartCRDMatchesTheGoTypes asserts the embedded HelmChart manifest
// describes the object k3sm.io/apis/helm/v1 describes, and that it ships the
// admission rules and printer column the helm controller's contract relies on.
func TestHelmChartCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, HelmChartCRD())
	assertIdentity(t, m, "helmcharts.helm.k3sm.io", HelmChartCRDName, "HelmChart", "helmcharts")
	v := onlyVersion(t, m, "v1")

	// The status subresource lets the controller write jobName and conditions
	// without being able to rewrite the user's spec.
	sub, ok := v["subresources"].(map[string]any)
	if !ok {
		t.Fatal("spec.versions[0].subresources is missing")
	}
	if _, ok := sub["status"]; !ok {
		t.Error("the status subresource is not enabled")
	}

	props := specProps(t, v)
	// Every Go spec field is declared: an undeclared field is pruned by the
	// structural schema, and the object would silently lose it on write.
	for _, f := range []string{
		"chart", "repo", "version", "targetNamespace", "createNamespace",
		"valuesContent", "set", "chartContent", "timeout", "bootstrap",
		"backOffLimit", "failurePolicy", "forceConflicts", "takeOwnership",
	} {
		if _, ok := props[f]; !ok {
			t.Errorf("spec.%s is not declared; the structural schema would prune it", f)
		}
	}
	assertFailurePolicyEnum(t, props)

	// set values are int-or-string, as in k3s: a numeric --set must not be
	// forced into a quoted string.
	if ap := mapAt(t, props, "set", "additionalProperties"); ap["x-kubernetes-int-or-string"] != true {
		t.Error("spec.set values are not x-kubernetes-int-or-string")
	}

	// The one deliberate divergence from k3s: an http:// repo or chart is
	// refused at admission, because the install Job runs as cluster-admin.
	for _, field := range []string{"repo", "chart"} {
		t.Run("cleartext refusal on "+field, func(t *testing.T) {
			f := mapAt(t, props, field)
			rules, ok := f["x-kubernetes-validations"].([]any)
			if !ok || len(rules) == 0 {
				t.Fatalf("spec.%s has no x-kubernetes-validations; an http:// value would be admitted", field)
			}
			found := false
			for _, r := range rules {
				rule, _ := r.(map[string]any)
				s, _ := rule["rule"].(string)
				if !strings.Contains(s, "startsWith('http://')") || !strings.HasPrefix(s, "!") {
					continue
				}
				found = true
				// A refusal the user cannot act on is a bug report; the message
				// names the field and the reason.
				msg, _ := rule["message"].(string)
				if !strings.Contains(msg, "spec."+field) || !strings.Contains(msg, "cleartext") {
					t.Errorf("the spec.%s rule's message is %q; it must name the field and the cleartext reason", field, msg)
				}
			}
			if !found {
				t.Errorf("no CEL rule on spec.%s refuses an http:// prefix", field)
			}
		})
	}

	// k3s's printer columns; Job is the one that tells a user which Job to read
	// logs from when an install hangs.
	cols, ok := v["additionalPrinterColumns"].([]any)
	if !ok {
		t.Fatal("spec.versions[0].additionalPrinterColumns is missing")
	}
	want := map[string]string{
		"Job":             ".status.jobName",
		"Chart":           ".spec.chart",
		"TargetNamespace": ".spec.targetNamespace",
		"Version":         ".spec.version",
		"Repo":            ".spec.repo",
		"Bootstrap":       ".spec.bootstrap",
		"Failed":          ".status.conditions[?(@.type=='Failed')].status",
	}
	got := map[string]string{}
	for _, c := range cols {
		col, _ := c.(map[string]any)
		name, _ := col["name"].(string)
		path, _ := col["jsonPath"].(string)
		got[name] = path
	}
	for name, path := range want {
		if got[name] != path {
			t.Errorf("printer column %s jsonPath = %q, want %q", name, got[name], path)
		}
	}
}

// TestHelmChartConfigCRDMatchesTheGoTypes asserts the embedded HelmChartConfig
// manifest describes the object k3sm.io/apis/helm/v1 describes.
func TestHelmChartConfigCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, HelmChartConfigCRD())
	assertIdentity(t, m, "helmchartconfigs.helm.k3sm.io", HelmChartConfigCRDName, "HelmChartConfig", "helmchartconfigs")
	v := onlyVersion(t, m, "v1")
	props := specProps(t, v)
	for _, f := range []string{"valuesContent", "failurePolicy", "forceConflicts"} {
		if _, ok := props[f]; !ok {
			t.Errorf("spec.%s is not declared; the structural schema would prune it", f)
		}
	}
	assertFailurePolicyEnum(t, props)
}

// TestHelmAccessorsReturnAFreshCopy asserts both helm accessors hand out a
// copy, on the same grounds as the MLXModel sibling.
func TestHelmAccessorsReturnAFreshCopy(t *testing.T) {
	t.Parallel()
	for name, fn := range map[string]func() []byte{
		"HelmChartCRD":       HelmChartCRD,
		"HelmChartConfigCRD": HelmChartConfigCRD,
	} {
		t.Run(name, func(t *testing.T) {
			a := fn()
			if len(a) == 0 {
				t.Fatalf("%s returned no bytes; the go:embed directive did not match", name)
			}
			a[0] = 'X'
			if b := fn(); b[0] != '#' {
				t.Fatalf("%s returns aliased bytes or lost its leading comment: starts with %q", name, b[0])
			}
		})
	}
}
