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
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/util/yaml"

	"k3sm.io/apis/keystore/entry"
)

// keyDeliveryImmutableRule is the transition rule every spec field carries.
const keyDeliveryImmutableRule = "self == oldSelf"

// keyDeliverySpecFields is the reviewable copy of KeyDeliverySpec's wire
// names, in schema order.
var keyDeliverySpecFields = []string{"entry", "secretName", "dataKey", "requestedBy"}

// keyDeliveryReasons is the reviewable copy of the closed reason set per
// condition (keystore/v1alpha1 ConditionReasons).
var keyDeliveryReasons = map[string][]string{
	"Authorized":     {"Authorized", "EntryUnavailable", "NotRecorded", "NotAuthorized", "Indeterminate"},
	"Ready":          {"Pending", "Delivered", "NotAuthorized", "SecretExists", "SecretClaimed", "EncryptionOff", "UserScopeOff"},
	"SecretRetained": {"DeletePending", "Unowned", "Indeterminate", "LabelRemoved", "UserScopeOff", "OwnerUnresolvable", "NoneRetained"},
}

// keyDeliveryVersion decodes the embedded manifest and returns its single
// version.
func keyDeliveryVersion(t *testing.T) map[string]any {
	t.Helper()
	return onlyVersion(t, decodeManifest(t, KeyDeliveryCRD()), "v1alpha1")
}

// TestKeyDeliveryCRDMatchesTheGoTypes asserts the embedded KeyDelivery
// manifest describes the object k3sm.io/apis/keystore/v1alpha1 describes:
// identity, the single served+stored v1alpha1 version, the status
// subresource, the printer columns, every spec and status field, the entry
// grammar taken from keystore/entry, and the reason set in the conditions'
// description.
//
// Like its siblings it deliberately does NOT import keystore/v1alpha1, so a
// disagreement stays visible; the lists in this file are the reviewable copy.
// It does import keystore/entry, the one home of the grammar the schema must
// carry verbatim.
func TestKeyDeliveryCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, KeyDeliveryCRD())
	if got := m["kind"]; got != "CustomResourceDefinition" {
		t.Errorf("kind = %v, want CustomResourceDefinition", got)
	}
	if got := mapAt(t, m, "metadata")["name"]; got != KeyDeliveryCRDName || KeyDeliveryCRDName != "keydeliveries.keystore.k3sm.io" {
		t.Errorf("metadata.name = %v, constant %q, want keydeliveries.keystore.k3sm.io", got, KeyDeliveryCRDName)
	}
	spec := mapAt(t, m, "spec")
	if got := spec["group"]; got != "keystore.k3sm.io" {
		t.Errorf("spec.group = %v, want keystore.k3sm.io", got)
	}
	if got := spec["scope"]; got != "Namespaced" {
		t.Errorf("spec.scope = %v, want Namespaced (a declaration delivers into its own namespace)", got)
	}
	names := mapAt(t, m, "spec", "names")
	for _, tc := range []struct{ key, want string }{
		{"kind", "KeyDelivery"}, {"listKind", "KeyDeliveryList"}, {"plural", "keydeliveries"}, {"singular", "keydelivery"},
	} {
		if got := names[tc.key]; got != tc.want {
			t.Errorf("spec.names.%s = %v, want %s", tc.key, got, tc.want)
		}
	}

	v := onlyVersion(t, m, "v1alpha1")
	if _, ok := mapAt(t, v, "subresources")["status"]; !ok {
		t.Error("the status subresource is not enabled")
	}
	assertPrinterColumns(t, v, [][2]string{
		{"Entry", ".spec.entry"},
		{"Secret", ".spec.secretName"},
		{"Ready", `.status.conditions[?(@.type=="Ready")].status`},
		{"Age", ".metadata.creationTimestamp"},
	})

	root := mapAt(t, v, "schema", "openAPIV3Schema")
	if !hasString(root["required"], "spec") {
		t.Error("openAPIV3Schema.required does not include spec")
	}
	assertDeclared(t, "", mapAt(t, root, "properties"), "apiVersion", "kind", "metadata", "spec", "status")
	sp := specProps(t, v)
	assertDeclared(t, "spec", sp, keyDeliverySpecFields...)

	// The entry grammar, defined once in keystore/entry.
	e := mapAt(t, sp, "entry")
	if got := e["pattern"]; got != entry.Pattern {
		t.Errorf("spec.entry pattern = %v, want entry.Pattern %q", got, entry.Pattern)
	}
	if got := e["maxLength"]; got != int64(entry.MaxLength) {
		t.Errorf("spec.entry maxLength = %v, want entry.MaxLength %d", got, entry.MaxLength)
	}
	assertRules(t, "spec.entry", e, []string{keyDeliveryImmutableRule, entry.UIDRangeRule, entry.NameLengthRule, entry.NotSystemRule})
	assertRules(t, "spec", mapAt(t, root, "properties", "spec"), []string{entry.RequestedByMatchesEntryRule})
	rb := mapAt(t, sp, "requestedBy")
	if got := rb["pattern"]; got != entry.RequestedByPattern {
		t.Errorf("spec.requestedBy pattern = %v, want entry.RequestedByPattern %q", got, entry.RequestedByPattern)
	}
	if got := rb["maxLength"]; got != int64(len("k3sm:local:4294967295")) {
		t.Errorf("spec.requestedBy maxLength = %v, want %d", got, len("k3sm:local:4294967295"))
	}
	// Kubernetes object-name and Secret-key bounds.
	for _, tc := range []struct {
		field, pattern string
	}{
		{"secretName", `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`},
		{"dataKey", `^[-._a-zA-Z0-9]+$`},
	} {
		f := mapAt(t, sp, tc.field)
		if f["pattern"] != tc.pattern || f["maxLength"] != int64(253) || f["minLength"] != int64(1) {
			t.Errorf("spec.%s pattern/minLength/maxLength = %v/%v/%v, want %q/1/253", tc.field, f["pattern"], f["minLength"], f["maxLength"], tc.pattern)
		}
	}
	for _, f := range keyDeliverySpecFields {
		if got := mapAt(t, sp, f)["type"]; got != "string" {
			t.Errorf("spec.%s type = %v, want string", f, got)
		}
	}

	st := mapAt(t, root, "properties", "status", "properties")
	assertDeclared(t, "status", st, "observedGeneration", "deliveredVersion", "conditions")
	for _, f := range []string{"observedGeneration", "deliveredVersion"} {
		if n := mapAt(t, st, f); n["type"] != "integer" || n["format"] != "int64" || n["minimum"] != int64(0) {
			t.Errorf("status.%s = %v, want a non-negative int64", f, n)
		}
	}
	cp := mapAt(t, st, "conditions", "items", "properties")
	assertDeclared(t, "status.conditions[]", cp, "type", "status", "observedGeneration", "lastTransitionTime", "reason", "message")
	assertStringEnum(t, "status.conditions[].status", mapAt(t, cp, "status"), "True", "False", "Unknown")

	// The closed reason set is listed in the schema description, each
	// condition with its own reasons.
	desc, _ := mapAt(t, st, "conditions")["description"].(string)
	for cond, reasons := range keyDeliveryReasons {
		i := strings.Index(desc, cond+" - ")
		if i < 0 {
			t.Errorf("status.conditions description does not introduce the %s reasons", cond)
			continue
		}
		section := desc[i:]
		for other := range keyDeliveryReasons {
			if j := strings.Index(section, other+" - "); other != cond && j > 0 {
				section = section[:j]
			}
		}
		for _, r := range reasons {
			if !regexp.MustCompile(`\b` + r + `\b`).MatchString(section) {
				t.Errorf("status.conditions description lists no %s reason under %s", r, cond)
			}
		}
	}
	if !strings.Contains(desc, "EntryUnavailable means the entry does not exist or is not the requester's") {
		t.Error("status.conditions description does not say EntryUnavailable covers both absent and not yours")
	}
}

// TestKeyDeliveryImmutableFields (U18) asserts every spec field carries the
// transition rule self == oldSelf, and that every field is required or
// defaulted, without which the rule would not run for a field absent at
// create. apis has no CEL evaluator, so the rule is pinned by text and its
// meaning transcribed: an unchanged value passes, a changed one is refused.
func TestKeyDeliveryImmutableFields(t *testing.T) {
	t.Parallel()
	v := keyDeliveryVersion(t)
	spec := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec")
	sp := specProps(t, v)
	for _, f := range keyDeliverySpecFields {
		node := mapAt(t, sp, f)
		rules, _ := node["x-kubernetes-validations"].([]any)
		found := 0
		for _, r := range rules {
			rule, _ := r.(map[string]any)
			if rule["rule"] == keyDeliveryImmutableRule {
				found++
				if rule["message"] != "spec."+f+" is immutable after create" {
					t.Errorf("spec.%s immutability message = %v", f, rule["message"])
				}
			}
		}
		if found != 1 {
			t.Errorf("spec.%s carries %d %q rules, want exactly 1", f, found, keyDeliveryImmutableRule)
		}
		_, defaulted := node["default"]
		if !hasString(spec["required"], f) && !defaulted {
			t.Errorf("spec.%s is neither required nor defaulted, so its transition rule would not run for a value set after create", f)
		}
	}
	immutable := func(self, oldSelf any) bool { return reflect.DeepEqual(self, oldSelf) }
	for _, tc := range []struct {
		old, new string
		ok       bool
	}{
		{"user/501/a", "user/501/a", true},
		{"user/501/a", "user/501/b", false},
		{"value", "token", false},
		{"k3sm:local:501", "k3sm:local:502", false},
	} {
		if got := immutable(tc.new, tc.old); got != tc.ok {
			t.Errorf("transition %q -> %q admitted = %v, want %v", tc.old, tc.new, got, tc.ok)
		}
	}
}

// TestKeyDeliveryRequiredFields (U18) pins the required set and the dataKey
// default.
func TestKeyDeliveryRequiredFields(t *testing.T) {
	t.Parallel()
	v := keyDeliveryVersion(t)
	spec := mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec")
	req, _ := spec["required"].([]any)
	want := []any{"entry", "secretName", "requestedBy"}
	if !reflect.DeepEqual(req, want) {
		t.Errorf("spec.required = %v, want %v", req, want)
	}
	if got := mapAt(t, specProps(t, v), "dataKey")["default"]; got != "value" {
		t.Errorf("spec.dataKey default = %v, want value", got)
	}
}

// TestKeyDeliveryRequestedByMatchesEntryUID (U18) runs the cross-field rule,
// pinned by text and transcribed, and the requestedBy pattern.
func TestKeyDeliveryRequestedByMatchesEntryUID(t *testing.T) {
	t.Parallel()
	v := keyDeliveryVersion(t)
	assertRules(t, "spec", mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec"), []string{entry.RequestedByMatchesEntryRule})
	rule := func(e, rb string) bool {
		return !strings.HasPrefix(e, "user/") || rb == "k3sm:local:"+strings.Split(e, "/")[1]
	}
	pat := regexp.MustCompile(mapAt(t, specProps(t, v), "requestedBy")["pattern"].(string))
	for _, tc := range []struct {
		entry, requestedBy string
		ok                 bool
	}{
		{"user/501/a", "k3sm:local:501", true},
		{"user/501/a", "k3sm:local:502", false},
		{"user/501/a", "k3sm:local:5010", false},
		{"user/501/a", "alice", false},
		{"user/1/a", "k3sm:local:1", true},
	} {
		got := rule(tc.entry, tc.requestedBy) && pat.MatchString(tc.requestedBy)
		if got != tc.ok {
			t.Errorf("entry %q requestedBy %q admitted = %v, want %v", tc.entry, tc.requestedBy, got, tc.ok)
		}
	}
	for _, s := range []string{"k3sm:local:0", "k3sm:local:0501", "k3sm:local:", "system:serviceaccount:a:b", "k3sm:local:501 "} {
		if pat.MatchString(s) {
			t.Errorf("requestedBy pattern admits %q", s)
		}
	}
}

// TestKeyDeliveryConditionsListMap (U18) asserts each condition type appears
// once.
func TestKeyDeliveryConditionsListMap(t *testing.T) {
	t.Parallel()
	c := mapAt(t, keyDeliveryVersion(t), "schema", "openAPIV3Schema", "properties", "status", "properties", "conditions")
	if got := c["x-kubernetes-list-type"]; got != "map" {
		t.Errorf("status.conditions list-type = %v, want map", got)
	}
	if got, _ := c["x-kubernetes-list-map-keys"].([]any); !reflect.DeepEqual(got, []any{"type"}) {
		t.Errorf("status.conditions list-map-keys = %v, want [type]", got)
	}
	items := mapAt(t, c, "items")
	for _, f := range []string{"type", "status", "lastTransitionTime", "reason"} {
		if !hasString(items["required"], f) {
			t.Errorf("status.conditions[].required does not include %s", f)
		}
	}
}

// TestKeyDeliveryRefusesSystemEntry (U18) asserts a system entry, valid as an
// API entry, is refused on a KeyDelivery.
func TestKeyDeliveryRefusesSystemEntry(t *testing.T) {
	t.Parallel()
	e := mapAt(t, specProps(t, keyDeliveryVersion(t)), "entry")
	for _, in := range []string{"system/signer-key", "system/a"} {
		if !entryGrammarAdmits(t, e, in, true) {
			t.Fatalf("%q is refused by the grammar alone; the test needs a grammar-valid system entry", in)
		}
		if entryGrammarAdmits(t, e, in, false) {
			t.Errorf("spec.entry admits the system entry %q", in)
		}
	}
}

// TestEntryGrammarVectors (U19) runs the shared vector table from
// keystore/entry through spec.entry's schema: the pattern, the maxLength and
// each CEL rule. Each row gives its Valid verdict under the grammar rules and
// its KeyDelivery verdict once the system refusal is added.
func TestEntryGrammarVectors(t *testing.T) {
	t.Parallel()
	e := mapAt(t, specProps(t, keyDeliveryVersion(t)), "entry")
	for _, vec := range entry.Vectors() {
		if got := entryGrammarAdmits(t, e, vec.Input, true); got != vec.Valid {
			t.Errorf("grammar admits %.40q = %v, want %v (%s)", vec.Input, got, vec.Valid, vec.Why)
		}
		if got := entryGrammarAdmits(t, e, vec.Input, false); got != vec.KeyDelivery {
			t.Errorf("spec.entry admits %.40q = %v, want %v (%s)", vec.Input, got, vec.KeyDelivery, vec.Why)
		}
	}
}

// entryRuleTranscriptions maps each non-transition CEL rule on spec.entry, by
// exact text, to its Go meaning. A CEL evaluation error (int() of a
// non-number) refuses, as the apiserver does. A rule edited in the manifest
// misses this map and fails the test rather than going unchecked.
var entryRuleTranscriptions = map[string]func(self string) bool{
	entry.UIDRangeRule: func(self string) bool {
		if !strings.HasPrefix(self, "user/") {
			return true
		}
		n, err := strconv.ParseInt(strings.Split(self, "/")[1], 10, 64)
		return err == nil && n <= 4294967295
	},
	entry.NameLengthRule: func(self string) bool {
		parts := strings.Split(self, "/")
		return utf8.RuneCountInString(parts[len(parts)-1]) <= 253
	},
	entry.NotSystemRule: func(self string) bool { return !strings.HasPrefix(self, "system/") },
}

// entryGrammarAdmits reports whether spec.entry's schema node admits s:
// maxLength (in code points, as OpenAPI counts), the pattern, and every
// non-transition rule. grammarOnly skips NotSystemRule, the one rule that is
// a KeyDelivery's and not the grammar's.
func entryGrammarAdmits(t *testing.T, node map[string]any, s string, grammarOnly bool) bool {
	t.Helper()
	maxLen, _ := node["maxLength"].(int64)
	if int64(utf8.RuneCountInString(s)) > maxLen {
		return false
	}
	pat, _ := node["pattern"].(string)
	if !regexp.MustCompile(pat).MatchString(s) {
		return false
	}
	rules, _ := node["x-kubernetes-validations"].([]any)
	for _, r := range rules {
		text, _ := r.(map[string]any)["rule"].(string)
		if strings.Contains(text, "oldSelf") || (grammarOnly && text == entry.NotSystemRule) {
			continue
		}
		fn, ok := entryRuleTranscriptions[text]
		if !ok {
			t.Fatalf("spec.entry rule %q has no transcription; pin its meaning here", text)
		}
		if !fn(s) {
			return false
		}
	}
	return true
}

// TestKeyDeliveryAccessorReturnsAFreshCopy asserts the accessor hands out a
// copy, on the same grounds as its siblings.
func TestKeyDeliveryAccessorReturnsAFreshCopy(t *testing.T) {
	t.Parallel()
	a := KeyDeliveryCRD()
	if len(a) == 0 {
		t.Fatal("KeyDeliveryCRD returned no bytes; the go:embed directive did not match")
	}
	a[0] = 'X'
	if b := KeyDeliveryCRD(); b[0] != '#' {
		t.Fatalf("KeyDeliveryCRD returns aliased bytes (first byte %q)", b[0])
	}
	var probe map[string]any
	if err := yaml.Unmarshal(KeyDeliveryCRD(), &probe); err != nil {
		t.Fatalf("manifest does not decode: %v", err)
	}
}
