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
	"fmt"
	"testing"
)

// budgetSignals is the closed signal vocabulary, in page-code order.
var budgetSignals = []string{
	"memory.available", "memory.compressor", "memory.swap", "swapfs.available",
	"nodefs.available", "imagefs.available", "pid.available",
}

// budgetLadderRules returns the pinned CEL rule texts on a NodeBudgetPolicy
// thresholds item, in manifest order: the ten pairwise rung orderings, the
// five release sides, and the memory.swap rule. Each ordering is
// direction-aware: an .available signal falls under pressure, so its rungs
// descend and its releases sit above them. k3smtest's
// TestNodeBudgetCRDRoundTrip evaluates the same texts.
func budgetLadderRules() []string {
	rungs := []string{"warnAt", "admitStopAt", "criticalAt", "evictSoftAt", "evictHardAt"}
	rels := []string{"warnReleaseAt", "admitStopReleaseAt", "criticalReleaseAt", "evictSoftReleaseAt", "evictHardReleaseAt"}
	var out []string
	for i := range rungs {
		for j := i + 1; j < len(rungs); j++ {
			a, b := rungs[i], rungs[j]
			out = append(out, fmt.Sprintf("!has(self.%[1]s) || !has(self.%[2]s) || (self.signal.endsWith('.available') ? self.%[1]s > self.%[2]s : self.%[1]s < self.%[2]s)", a, b))
		}
	}
	for i := range rungs {
		r, rel := rungs[i], rels[i]
		out = append(out, fmt.Sprintf("!has(self.%[1]s) || !has(self.%[2]s) || (self.signal.endsWith('.available') ? self.%[2]s > self.%[1]s : self.%[2]s < self.%[1]s)", r, rel))
	}
	return append(out, "self.signal != 'memory.swap' || !has(self.evictSoftAt)")
}

// nodeBudgetStatusRule is the pinned CEL rule on NodeBudget.status: a
// reporting status carries exactly one platform block.
const nodeBudgetStatusRule = "!has(self.level) || (has(self.darwin) != has(self.linux))"

// assertBudgetIdentity checks a budget.k3sm.io manifest's identity: group,
// Cluster scope, names, the single served+stored v1alpha1 version, and no
// labels or annotations on the CRD object itself (aggregation labels belong
// on ClusterRoles, never on a CRD).
func assertBudgetIdentity(t *testing.T, m map[string]any, constName, wantName, kind, plural string) map[string]any {
	t.Helper()
	if got := m["kind"]; got != "CustomResourceDefinition" {
		t.Errorf("kind = %v, want CustomResourceDefinition", got)
	}
	md := mapAt(t, m, "metadata")
	if got := md["name"]; got != constName || constName != wantName {
		t.Errorf("metadata.name = %v, constant %q, want %s", got, constName, wantName)
	}
	for k := range md {
		if k != "name" {
			t.Errorf("metadata.%s is set; a budget CRD carries no labels or annotations", k)
		}
	}
	spec := mapAt(t, m, "spec")
	if got := spec["group"]; got != "budget.k3sm.io" {
		t.Errorf("spec.group = %v, want budget.k3sm.io", got)
	}
	if got := spec["scope"]; got != "Cluster" {
		t.Errorf("spec.scope = %v, want Cluster", got)
	}
	names := mapAt(t, m, "spec", "names")
	for _, tc := range []struct{ key, want string }{
		{"kind", kind}, {"listKind", kind + "List"}, {"plural", plural}, {"singular", plural[:len(plural)-len("s")]},
	} {
		want := tc.want
		if tc.key == "singular" && plural == "nodebudgetpolicies" {
			want = "nodebudgetpolicy"
		}
		if got := names[tc.key]; got != want {
			t.Errorf("spec.names.%s = %v, want %s", tc.key, got, want)
		}
	}
	return onlyVersion(t, m, "v1alpha1")
}

// assertPrinterColumns asserts the version's printer columns are exactly
// want, by name and jsonPath, in order.
func assertPrinterColumns(t *testing.T, v map[string]any, want [][2]string) {
	t.Helper()
	cols, _ := v["additionalPrinterColumns"].([]any)
	if len(cols) != len(want) {
		t.Fatalf("printer columns = %v, want %v", cols, want)
	}
	for i, c := range cols {
		col, _ := c.(map[string]any)
		if col["name"] != want[i][0] || col["jsonPath"] != want[i][1] {
			t.Errorf("printer column %d = %v %v, want %s %s", i, col["name"], col["jsonPath"], want[i][0], want[i][1])
		}
	}
}

// assertDeclared asserts every name is a property of props.
func assertDeclared(t *testing.T, path string, props map[string]any, names ...string) {
	t.Helper()
	for _, f := range names {
		if _, ok := props[f]; !ok {
			t.Errorf("%s.%s is not declared; the structural schema would prune it", path, f)
		}
	}
	if len(props) != len(names) {
		t.Errorf("%s declares %d properties, want exactly %v", path, len(props), names)
	}
}

// TestNodeBudgetCRDMatchesTheGoTypes asserts the embedded NodeBudget manifest
// describes the object k3sm.io/apis/budget/v1alpha1 describes: identity,
// version, the status subresource, the printer columns, every status field,
// the enums and list keys, and the pinned status rule. It deliberately does
// NOT import budget/v1alpha1, so a disagreement stays visible; the lists
// below are the reviewable copy. The Go-side agreement (both golden documents
// validated against this schema) is TestNodeBudgetCRDRoundTrip in
// k3sm.io/apis/k3smtest.
func TestNodeBudgetCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, NodeBudgetCRD())
	v := assertBudgetIdentity(t, m, NodeBudgetCRDName, "nodebudgets.budget.k3sm.io", "NodeBudget", "nodebudgets")
	sub, ok := v["subresources"].(map[string]any)
	if !ok {
		t.Fatal("the status subresource is missing")
	}
	if _, ok := sub["status"]; !ok || len(sub) != 1 {
		t.Errorf("subresources = %v, want only status", sub)
	}
	assertPrinterColumns(t, v, [][2]string{
		{"Level", ".status.level"}, {"Admit", ".status.admit"}, {"Mode", ".status.killMode.mode"},
		{"Armed", ".status.killMode.armed"}, {"Age", ".metadata.creationTimestamp"},
	})

	root := mapAt(t, v, "schema", "openAPIV3Schema")
	rootProps := mapAt(t, root, "properties")
	if _, ok := rootProps["spec"]; ok {
		t.Error("NodeBudget declares a spec; it is status only")
	}
	status := mapAt(t, rootProps, "status")
	rules, _ := status["x-kubernetes-validations"].([]any)
	if len(rules) != 1 || rules[0].(map[string]any)["rule"] != nodeBudgetStatusRule {
		t.Errorf("status rules = %v, want exactly %q", rules, nodeBudgetStatusRule)
	}
	sp := mapAt(t, status, "properties")
	assertDeclared(t, "status", sp,
		"observedAt", "generation", "platform", "level", "admit", "admitReasonSignal", "evictDeadlineNs",
		"capacity", "signals", "recommendation", "killMode", "wouldKill", "counters", "policy",
		"writer", "conditions", "darwin", "linux")
	assertDeclared(t, "status.capacity", mapAt(t, sp, "capacity", "properties"),
		"cpuCapacityMilli", "cpuSystemReserveMilli", "memCapacityBytes", "memSystemReserveBytes",
		"memEvictionReserveBytes", "memAllocatableBytes", "swapTotalBytes", "swapUsedBytes")
	sig := mapAt(t, sp, "signals", "items", "properties")
	assertDeclared(t, "status.signals[]", sig,
		"id", "unit", "value", "warnAt", "admitStopAt", "criticalAt", "evictSoftAt", "evictHardAt",
		"warnReleaseAt", "admitStopReleaseAt", "criticalReleaseAt", "evictSoftReleaseAt", "evictHardReleaseAt",
		"graceNs", "minReclaimBytes", "windowNs", "etaToNextRungNs", "trend", "limitingResource", "sharedWith")
	assertDeclared(t, "status.recommendation", mapAt(t, sp, "recommendation", "properties"), "safeAdditionalBytes", "basis")
	assertDeclared(t, "status.killMode", mapAt(t, sp, "killMode", "properties"), "mode", "armed", "armingEpisodes", "reason")
	assertDeclared(t, "status.wouldKill", mapAt(t, sp, "wouldKill", "properties"), "band", "reason", "at")
	cnt := mapAt(t, sp, "counters", "items", "properties")
	assertDeclared(t, "status.counters[]", cnt,
		"band", "evictionsTotal", "oomKillsTotal", "kernelKillsTotal", "restartsTotal", "memHighEventsTotal",
		"tier1RefusedTotal", "windows")
	assertDeclared(t, "status.counters[].windows[]", mapAt(t, cnt, "windows", "items", "properties"), "window", "reason", "count")
	assertDeclared(t, "status.policy", mapAt(t, sp, "policy", "properties"), "name", "resourceVersion", "outcome", "clampedFields", "message")
	assertDeclared(t, "status.writer", mapAt(t, sp, "writer", "properties"), "shape", "nodeNameExtra")
	assertDeclared(t, "status.darwin", mapAt(t, sp, "darwin", "properties"),
		"vmPressureLevel", "memorystatusLevelPermille", "compressor", "swapinsTotal", "swapoutsTotal")
	assertDeclared(t, "status.linux", mapAt(t, sp, "linux", "properties"), "psi", "memoryEvents")
	for _, r := range []string{"cpu", "memory", "io"} {
		for _, l := range []string{"some", "full"} {
			assertDeclared(t, "status.linux.psi."+r+"."+l,
				mapAt(t, sp, "linux", "properties", "psi", "properties", r, "properties", l, "properties"),
				"avg10Permille", "avg60Permille", "totalUs")
		}
	}

	assertStringEnum(t, "status.platform", mapAt(t, sp, "platform"), "darwin", "linux")
	assertStringEnum(t, "status.level", mapAt(t, sp, "level"), "ok", "warn", "critical", "evicting")
	for path, node := range map[string]map[string]any{
		"status.admitReasonSignal":    mapAt(t, sp, "admitReasonSignal"),
		"status.signals[].id":         mapAt(t, sig, "id"),
		"status.signals[].sharedWith": mapAt(t, sig, "sharedWith"),
		"status.recommendation.basis": mapAt(t, sp, "recommendation", "properties", "basis"),
	} {
		assertStringEnum(t, path, node, budgetSignals...)
	}
	assertStringEnum(t, "status.signals[].unit", mapAt(t, sig, "unit"), "bytes", "permille", "count")
	assertStringEnum(t, "status.killMode.mode", mapAt(t, sp, "killMode", "properties", "mode"), "auto", "on", "off")
	assertStringEnum(t, "status.writer.shape", mapAt(t, sp, "writer", "properties", "shape"), "annotation-relay", "own-node")
	assertStringEnum(t, "status.counters[].band", mapAt(t, cnt, "band"), "critical", "standard", "batch")

	for _, tc := range []struct {
		path string
		node map[string]any
		key  string
		max  int64
	}{
		{"status.signals", mapAt(t, sp, "signals"), "id", 8},
		{"status.counters", mapAt(t, sp, "counters"), "band", 3},
		{"status.conditions", mapAt(t, sp, "conditions"), "type", 8},
	} {
		if tc.node["x-kubernetes-list-type"] != "map" || !hasString(tc.node["x-kubernetes-list-map-keys"], tc.key) {
			t.Errorf("%s is not a list-type map keyed by %s", tc.path, tc.key)
		}
		if got := tc.node["maxItems"]; got != tc.max {
			t.Errorf("%s maxItems = %v, want %d", tc.path, got, tc.max)
		}
	}
}

// TestNodeBudgetPolicyCRDMatchesTheGoTypes asserts the embedded
// NodeBudgetPolicy manifest: identity, version, no status, every spec field,
// the tier-1 floors as schema bounds, the defaults, no band kill-order
// field, no dwell default, and the sixteen pinned ladder rules.
func TestNodeBudgetPolicyCRDMatchesTheGoTypes(t *testing.T) {
	t.Parallel()
	m := decodeManifest(t, NodeBudgetPolicyCRD())
	v := assertBudgetIdentity(t, m, NodeBudgetPolicyCRDName, "nodebudgetpolicies.budget.k3sm.io", "NodeBudgetPolicy", "nodebudgetpolicies")
	if _, ok := v["subresources"]; ok {
		t.Error("NodeBudgetPolicy declares subresources; it is spec only")
	}
	assertPrinterColumns(t, v, [][2]string{
		{"Priority", ".spec.priority"}, {"Mode", ".spec.mode"}, {"Age", ".metadata.creationTimestamp"},
	})
	root := mapAt(t, v, "schema", "openAPIV3Schema")
	if _, ok := mapAt(t, root, "properties")["status"]; ok {
		t.Error("NodeBudgetPolicy declares a status")
	}
	if !hasString(root["required"], "spec") {
		t.Error("openAPIV3Schema.required does not include spec")
	}
	sp := specProps(t, v)
	assertDeclared(t, "spec", sp,
		"nodeSelector", "priority", "thresholds", "reserves", "history", "mode",
		"tier1GraceSeconds", "tier1HeldSeconds", "tier1MaxDeliveriesPerHour", "tier1CooloffSeconds", "psiHeldPermille")
	th := mapAt(t, sp, "thresholds")
	if th["x-kubernetes-list-type"] != "map" || !hasString(th["x-kubernetes-list-map-keys"], "signal") || th["maxItems"] != int64(8) {
		t.Errorf("spec.thresholds = %v, want a list-type map keyed by signal, at most 8", th)
	}
	item := mapAt(t, th, "items")
	tp := mapAt(t, item, "properties")
	assertDeclared(t, "spec.thresholds[]", tp,
		"signal", "warnAt", "admitStopAt", "criticalAt", "evictSoftAt", "evictHardAt",
		"warnReleaseAt", "admitStopReleaseAt", "criticalReleaseAt", "evictSoftReleaseAt", "evictHardReleaseAt",
		"graceSeconds", "minReclaimBytes", "dwellSeconds", "windowSeconds")
	assertStringEnum(t, "spec.thresholds[].signal", mapAt(t, tp, "signal"), budgetSignals...)
	if _, ok := mapAt(t, tp, "dwellSeconds")["default"]; ok {
		t.Error("spec.thresholds[].dwellSeconds has a default; the dwell is optional with no default")
	}
	assertDeclared(t, "spec.reserves", mapAt(t, sp, "reserves", "properties"),
		"cpuSystemReserveMilli", "memSystemReserveBytes", "memEvictionReserveBytes")
	assertDeclared(t, "spec.history", mapAt(t, sp, "history", "properties"),
		"memoryHalfLifeSeconds", "cpuHalfLifeSeconds", "marginPermille")
	assertStringEnum(t, "spec.mode", mapAt(t, sp, "mode"), "auto", "on", "off")

	// The floors, as literal schema bounds; k3smtest cross-checks them with
	// the Go constants.
	for _, tc := range []struct {
		field         string
		node          map[string]any
		min, max, def any
	}{
		{"tier1GraceSeconds", mapAt(t, sp, "tier1GraceSeconds"), int64(10), nil, nil},
		{"tier1HeldSeconds", mapAt(t, sp, "tier1HeldSeconds"), int64(10), nil, int64(30)},
		{"tier1MaxDeliveriesPerHour", mapAt(t, sp, "tier1MaxDeliveriesPerHour"), int64(1), int64(6), nil},
		{"tier1CooloffSeconds", mapAt(t, sp, "tier1CooloffSeconds"), int64(300), nil, int64(1800)},
		{"psiHeldPermille", mapAt(t, sp, "psiHeldPermille"), int64(0), int64(1000), nil},
		{"mode", mapAt(t, sp, "mode"), nil, nil, "auto"},
		{"priority", mapAt(t, sp, "priority"), nil, nil, int64(0)},
		{"history.memoryHalfLifeSeconds", mapAt(t, sp, "history", "properties", "memoryHalfLifeSeconds"), int64(1), nil, int64(172800)},
		{"history.cpuHalfLifeSeconds", mapAt(t, sp, "history", "properties", "cpuHalfLifeSeconds"), int64(1), nil, int64(43200)},
		{"history.marginPermille", mapAt(t, sp, "history", "properties", "marginPermille"), int64(0), int64(1000), int64(150)},
	} {
		if got := tc.node["minimum"]; got != tc.min {
			t.Errorf("spec.%s minimum = %v, want %v", tc.field, got, tc.min)
		}
		if got := tc.node["maximum"]; got != tc.max {
			t.Errorf("spec.%s maximum = %v, want %v", tc.field, got, tc.max)
		}
		if got := tc.node["default"]; got != tc.def {
			t.Errorf("spec.%s default = %v, want %v", tc.field, got, tc.def)
		}
	}

	raw, _ := item["x-kubernetes-validations"].([]any)
	want := budgetLadderRules()
	if len(raw) != len(want) {
		t.Fatalf("spec.thresholds[] carries %d rules, want %d", len(raw), len(want))
	}
	for i, r := range raw {
		rule, _ := r.(map[string]any)
		if rule["rule"] != want[i] {
			t.Errorf("rule %d = %q, want %q", i, rule["rule"], want[i])
		}
		if msg, _ := rule["message"].(string); msg == "" {
			t.Errorf("rule %d has no message", i)
		}
	}
	if n := len(collectRules(m)); n != len(want) {
		t.Errorf("the manifest carries %d rules in all, want only the %d ladder rules", n, len(want))
	}
}

// TestBudgetAccessorsReturnAFreshCopy asserts both budget accessors hand out
// a copy, on the same grounds as their siblings.
func TestBudgetAccessorsReturnAFreshCopy(t *testing.T) {
	t.Parallel()
	for name, acc := range map[string]func() []byte{"NodeBudgetCRD": NodeBudgetCRD, "NodeBudgetPolicyCRD": NodeBudgetPolicyCRD} {
		a := acc()
		if len(a) == 0 {
			t.Fatalf("%s returned no bytes; the go:embed directive did not match", name)
		}
		a[0] = 'X'
		if b := acc(); b[0] != '#' {
			t.Fatalf("%s returns aliased bytes (first byte %q)", name, b[0])
		}
	}
}
