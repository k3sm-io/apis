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
	"strings"
	"testing"

	"k3sm.io/apis/keystore/credential"
)

// kdItem is one spec.items entry, as the transcribed rules see it.
type kdItem struct{ field, key string }

// itemsSchema returns spec.items' schema node.
func itemsSchema(t *testing.T) map[string]any {
	t.Helper()
	return mapAt(t, specProps(t, keyDeliveryVersion(t)), "items")
}

// itemsAdmitted reports whether items pass spec.items' create-time schema:
// the item count, unique keys (listType=map keyed by key), the field enum and
// its maxLength, and the key's pattern, length and CEL rule, each read from
// the manifest.
func itemsAdmitted(t *testing.T, node map[string]any, items []kdItem) bool {
	t.Helper()
	minI, _ := node["minItems"].(int64)
	maxI, _ := node["maxItems"].(int64)
	if int64(len(items)) < minI || int64(len(items)) > maxI {
		return false
	}
	props := mapAt(t, node, "items", "properties")
	field, key := mapAt(t, props, "field"), mapAt(t, props, "key")
	keyRE := regexp.MustCompile(key["pattern"].(string))
	keyRules, _ := key["x-kubernetes-validations"].([]any)
	if len(keyRules) != 1 || keyRules[0].(map[string]any)["rule"] != "self != '.' && self != '..'" {
		t.Fatalf("spec.items[].key rules = %v; pin the new rule's meaning here", keyRules)
	}
	seen := map[string]bool{}
	for _, it := range items {
		if seen[it.key] {
			return false
		}
		seen[it.key] = true
		if !hasString(field["enum"], it.field) || int64(len(it.field)) > field["maxLength"].(int64) {
			return false
		}
		if len(it.key) < int(key["minLength"].(int64)) || len(it.key) > int(key["maxLength"].(int64)) ||
			!keyRE.MatchString(it.key) || it.key == "." || it.key == ".." {
			return false
		}
	}
	return true
}

// itemsTransitionAdmitted runs spec.items' transition rule, pinned by text
// and transcribed.
func itemsTransitionAdmitted(t *testing.T, node map[string]any, self, oldSelf []kdItem) bool {
	t.Helper()
	assertRules(t, "spec.items", node, []string{keyDeliveryItemsImmutableRule})
	if len(self) != len(oldSelf) {
		return false
	}
	for _, i := range self {
		found := false
		for _, o := range oldSelf {
			if o.key == i.key && o.field == i.field {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TestKeyDeliveryItemsSchema (U65) pins spec.items: required, no default, 1
// to 8 items, unique keys, the key pattern and both maxLengths.
func TestKeyDeliveryItemsSchema(t *testing.T) {
	t.Parallel()
	v := keyDeliveryVersion(t)
	if !hasString(mapAt(t, v, "schema", "openAPIV3Schema", "properties", "spec")["required"], "items") {
		t.Error("spec.items is not required")
	}
	n := itemsSchema(t)
	if d, ok := n["default"]; ok {
		t.Errorf("spec.items default = %v, want none: a kind-blind default is wrong", d)
	}
	if n["minItems"] != int64(1) || n["maxItems"] != int64(8) {
		t.Errorf("spec.items minItems/maxItems = %v/%v, want 1/8", n["minItems"], n["maxItems"])
	}
	if n["x-kubernetes-list-type"] != "map" || !reflect.DeepEqual(n["x-kubernetes-list-map-keys"], []any{"key"}) {
		t.Errorf("spec.items list-type/keys = %v/%v, want map/[key]", n["x-kubernetes-list-type"], n["x-kubernetes-list-map-keys"])
	}
	it := mapAt(t, n, "items")
	if !reflect.DeepEqual(it["required"], []any{"field", "key"}) {
		t.Errorf("spec.items[].required = %v, want [field key]", it["required"])
	}
	key := mapAt(t, it, "properties", "key")
	if key["pattern"] != secretDataKeyPattern || key["maxLength"] != int64(253) || key["minLength"] != int64(1) {
		t.Errorf("spec.items[].key pattern/minLength/maxLength = %v/%v/%v", key["pattern"], key["minLength"], key["maxLength"])
	}
	if got := mapAt(t, it, "properties", "field")["maxLength"]; got != int64(credential.MaxFieldLength) || credential.MaxFieldLength != 32 {
		t.Errorf("spec.items[].field maxLength = %v, want 32", got)
	}
	for _, tc := range []struct {
		name  string
		items []kdItem
		ok    bool
	}{
		{"one", []kdItem{{"value", "token"}}, true},
		{"none", nil, false},
		{"eight", eightItems(), true},
		{"nine", append(eightItems(), kdItem{"value", "k8"}), false},
		{"duplicate key", []kdItem{{"value", "a"}, {"expiresAt", "a"}}, false},
		{"bad key", []kdItem{{"value", "a/b"}}, false},
		{"dot key", []kdItem{{"value", ".."}}, false},
		{"long key", []kdItem{{"value", strings.Repeat("k", 254)}}, false},
		{"max key", []kdItem{{"value", strings.Repeat("k", 253)}}, true},
		{"unknown field", []kdItem{{"password", "a"}}, false},
	} {
		if got := itemsAdmitted(t, n, tc.items); got != tc.ok {
			t.Errorf("%s: admitted = %v, want %v", tc.name, got, tc.ok)
		}
	}
}

func eightItems() []kdItem {
	var out []kdItem
	for i := range 8 {
		out = append(out, kdItem{"value", "k" + string(rune('0'+i))})
	}
	return out
}

// TestKeyDeliveryHasNoDataKey (U65) asserts dataKey is gone from the schema.
func TestKeyDeliveryHasNoDataKey(t *testing.T) {
	t.Parallel()
	if _, ok := specProps(t, keyDeliveryVersion(t))["dataKey"]; ok {
		t.Error("spec.dataKey is declared; items replaced it")
	}
	if strings.Contains(string(KeyDeliveryCRD()), "dataKey") {
		t.Error("the KeyDelivery manifest still mentions dataKey")
	}
}

// TestItemsReorderAcceptedChangeRefused (U65) runs the items transition rule:
// a reorder is accepted; a changed field, a changed key, an added or a
// removed item is refused.
func TestItemsReorderAcceptedChangeRefused(t *testing.T) {
	t.Parallel()
	n := itemsSchema(t)
	old := []kdItem{{"accessToken", "access_token"}, {"accessExpiresAt", "access_expires_at"}}
	for _, tc := range []struct {
		name string
		new  []kdItem
		ok   bool
	}{
		{"unchanged", []kdItem{{"accessToken", "access_token"}, {"accessExpiresAt", "access_expires_at"}}, true},
		{"reordered", []kdItem{{"accessExpiresAt", "access_expires_at"}, {"accessToken", "access_token"}}, true},
		{"field changed", []kdItem{{"tokenType", "access_token"}, {"accessExpiresAt", "access_expires_at"}}, false},
		{"key changed", []kdItem{{"accessToken", "token"}, {"accessExpiresAt", "access_expires_at"}}, false},
		{"fields swapped", []kdItem{{"accessExpiresAt", "access_token"}, {"accessToken", "access_expires_at"}}, false},
		{"added", []kdItem{{"accessToken", "access_token"}, {"accessExpiresAt", "access_expires_at"}, {"tokenType", "token_type"}}, false},
		{"removed", []kdItem{{"accessToken", "access_token"}}, false},
	} {
		if got := itemsTransitionAdmitted(t, n, tc.new, old); got != tc.ok {
			t.Errorf("%s: admitted = %v, want %v", tc.name, got, tc.ok)
		}
	}
}

// TestItemsCELCostWithinBudget (U65) bounds the cost of spec.items' CEL from
// the schema's own limits. apis carries no CEL cost estimator, so this is an
// upper bound, not the apiserver's estimate: the transition rule visits at
// most maxItems x maxItems pairs, each comparing a key (maxLength) and a field
// (maxLength) at no more than one cost unit per byte, plus the per-item key
// rule. The bound must sit far under the apiserver's per-expression static
// limit (10,000,000); the bound also fails if a limit it relies on is removed.
func TestItemsCELCostWithinBudget(t *testing.T) {
	t.Parallel()
	n := itemsSchema(t)
	maxI, ok1 := n["maxItems"].(int64)
	props := mapAt(t, n, "items", "properties")
	keyLen, ok2 := mapAt(t, props, "key")["maxLength"].(int64)
	fieldLen, ok3 := mapAt(t, props, "field")["maxLength"].(int64)
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("spec.items lacks maxItems or a maxLength; the CEL cost is unbounded")
	}
	const perPairOverhead = 10
	transition := maxI*maxI*(keyLen+fieldLen+perPairOverhead) + 2*maxI
	keyRule := maxI * (2*keyLen + perPairOverhead)
	const budget = 10_000_000 / 100
	if total := transition + keyRule; total > budget {
		t.Errorf("spec.items CEL cost bound = %d, want at most %d (1%% of the per-expression limit)", total, budget)
	}
}

// TestFieldEnumEqualsCredentialTable (U66) asserts the CRD's field enum is
// exactly the credential table's union, in order. The enum is written by
// hand in the manifest; this test is what keeps it generated from the table.
func TestFieldEnumEqualsCredentialTable(t *testing.T) {
	t.Parallel()
	enum, _ := mapAt(t, itemsSchema(t), "items", "properties", "field")["enum"].([]any)
	var want []any
	for _, f := range credential.Fields() {
		want = append(want, string(f))
	}
	if !reflect.DeepEqual(enum, want) {
		t.Errorf("spec.items[].field enum = %v, want credential.Fields() %v", enum, want)
	}
}

// TestSchemaRefusesRefreshTokenField (U66) asserts an item naming the
// refresh token, or any refresh field, is refused by the schema.
func TestSchemaRefusesRefreshTokenField(t *testing.T) {
	t.Parallel()
	n := itemsSchema(t)
	enum, _ := mapAt(t, n, "items", "properties", "field")["enum"].([]any)
	for _, e := range enum {
		if s, _ := e.(string); strings.Contains(strings.ToLower(s), "refresh") {
			t.Errorf("spec.items[].field enum holds the refresh field %q", s)
		}
	}
	for _, f := range []string{credential.RefreshTokenField, "refreshExpiresAt", "refreshOwner"} {
		if itemsAdmitted(t, n, []kdItem{{f, "refresh_token"}}) {
			t.Errorf("an item naming %q is admitted", f)
		}
	}
	if !itemsAdmitted(t, n, []kdItem{{string(credential.FieldAccessToken), "access_token"}}) {
		t.Error("an item naming accessToken is refused; the control row fails")
	}
}

// TestKeyDeliveryReasonDescriptions (U68) asserts the conditions' description
// lists the CredentialValid reasons and ProjectionInvalid, says Ready means
// delivered, and says Valid, Expiring and RefreshFailing share True.
func TestKeyDeliveryReasonDescriptions(t *testing.T) {
	t.Parallel()
	c := mapAt(t, keyDeliveryVersion(t), "schema", "openAPIV3Schema", "properties", "status", "properties", "conditions")
	desc, _ := c["description"].(string)
	i := strings.Index(desc, credential.ConditionCredentialValid+" - ")
	if i < 0 {
		t.Fatal("the conditions description has no CredentialValid section")
	}
	section := desc[i:]
	if j := strings.Index(section, ". "); j > 0 {
		section = section[:j]
	}
	for _, r := range credential.CredentialValidReasons() {
		if !regexp.MustCompile(`\b` + r + `\b`).MatchString(section) {
			t.Errorf("the CredentialValid section does not list %s", r)
		}
	}
	for _, s := range []string{
		"Ready - ",
		credential.ReasonProjectionInvalid,
		"Ready means delivered. It does not change on expiry or revocation",
		"health readers read CredentialValid",
		"Valid, Expiring and RefreshFailing share True",
	} {
		if !strings.Contains(desc, s) {
			t.Errorf("the conditions description does not say %q", s)
		}
	}
}

// TestConditionsCarryObservedGeneration (U68) asserts every condition must
// carry observedGeneration.
func TestConditionsCarryObservedGeneration(t *testing.T) {
	t.Parallel()
	st := mapAt(t, keyDeliveryVersion(t), "schema", "openAPIV3Schema", "properties", "status", "properties")
	items := mapAt(t, st, "conditions", "items")
	if !hasString(items["required"], "observedGeneration") {
		t.Error("status.conditions[].required does not include observedGeneration")
	}
	og := mapAt(t, items, "properties", "observedGeneration")
	if og["type"] != "integer" || og["format"] != "int64" || og["minimum"] != int64(0) {
		t.Errorf("status.conditions[].observedGeneration = %v, want a non-negative int64", og)
	}
}
