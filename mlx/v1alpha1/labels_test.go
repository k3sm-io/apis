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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMLXKeys pins the exact byte strings of the mlx.k3sm.io identifiers.
//
// These are the one part of this alpha package that is NOT allowed to change: a
// node is labelled with them and a pod requests the resource by name, so a
// rename breaks every already-labelled node and every already-scheduled pod, and
// it breaks them silently — a selector that matches nothing is not an error.
func TestMLXKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, got, want string
	}{
		{"GroupName", GroupName, "mlx.k3sm.io"},
		{"ResourceGPU", ResourceGPU, "mlx.k3sm.io/gpu"},
		{"LabelGPUPresent", LabelGPUPresent, "mlx.k3sm.io/gpu.present"},
		{"LabelChip", LabelChip, "mlx.k3sm.io/chip"},
		{"LabelChipFamily", LabelChipFamily, "mlx.k3sm.io/chip-family"},
		{"LabelMemoryGB", LabelMemoryGB, "mlx.k3sm.io/memory-gb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}

	// Every prefixed key in this package is in the CRD-group domain
	// mlx.k3sm.io/*, which is what makes the placement rule checkable at all: a
	// bare k3sm.io/* key here would be one that belongs in the runtime-area
	// package instead.
	for _, tc := range cases[1:] {
		t.Run(tc.name+" is a valid prefixed key in the mlx domain", func(t *testing.T) {
			if !strings.HasPrefix(tc.got, GroupName+"/") {
				t.Errorf("%s = %q, want the %s/ domain", tc.name, tc.got, GroupName)
			}
			if strings.Count(tc.got, "/") != 1 {
				t.Errorf("%s = %q is not a valid prefixed key (exactly one '/')", tc.name, tc.got)
			}
			name := tc.got[strings.Index(tc.got, "/")+1:]
			if name == "" || len(name) > 63 {
				t.Errorf("%s name segment %q must be 1..63 characters", tc.name, name)
			}
		})
	}

	// The resource and the presence label answer different questions — "how many
	// are left" versus "does this node have one" — so collapsing them into one
	// string would make a nodeSelector match on capacity semantics.
	if ResourceGPU == LabelGPUPresent {
		t.Error("ResourceGPU and LabelGPUPresent must be distinct keys")
	}
}

// TestChipSlugRuleDocumented asserts the chip-slug normalization rule is stated
// in this package.
//
// The rule is documented here and implemented by the node advertiser, so the
// documentation IS the contract: GPUFacts.chip_brand is carried verbatim
// ("Apple M4 Max") and is not a legal label value, while LabelChip's value is
// the slug ("apple-m4-max"). A consumer that compares a label value against the
// raw brand silently never matches. If this text goes missing, two independent
// consumers will each invent a slugging and disagree.
func TestChipSlugRuleDocumented(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("labels.go")
	if err != nil {
		t.Fatalf("read labels.go: %v", err)
	}
	flat := strings.Join(strings.Fields(strings.ReplaceAll(string(src), "//", " ")), " ")
	for _, want := range []string{
		"Chip-slug normalization rule",
		`"Apple M4 Max" becomes "apple-m4-max"`,
		"truncate to 63 characters",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("labels.go does not document the chip-slug clause %q", want)
		}
	}
}

// TestAlphaBrandingDocumented asserts the package doc says, in words, that
// incompatible changes are allowed.
//
// The v1alpha1 path component is a convention; a user's expectation is set by
// what the documentation actually promises. This package deliberately diverges
// from net/v1's additive-only posture, and that divergence is only safe if it is
// stated — an alpha API whose doc reads like a stable one has made the stable
// promise regardless of its directory name.
func TestAlphaBrandingDocumented(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("read doc.go: %v", err)
	}
	flat := strings.Join(strings.Fields(strings.ReplaceAll(string(src), "//", " ")), " ")
	for _, want := range []string{
		"THIS IS AN ALPHA API — INCOMPATIBLE CHANGES ARE ALLOWED",
		"may be renamed, retyped, given different defaults, or REMOVED",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("doc.go does not carry the alpha-branding clause %q", want)
		}
	}
}

// TestOnlyMLXDomainKeysLiveHere is the placement gate: the internet-egress
// annotation constant — and any other k3sm.io/*-domain key — must NOT be in this
// package.
//
// The rule is not bookkeeping. The egress opt-in parameterizes the runtime
// contract's SandboxProfile and applies to any pod, so a copy of it here would
// make MLX look like the owner of a general runtime capability, and the two
// spellings would drift the moment one of them changed. The check reads this
// package's own sources rather than trusting the constant list above, so a key
// added to a new file in this package is caught too.
func TestOnlyMLXDomainKeysLiveHere(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		t.Run(name, func(t *testing.T) {
			// The egress annotation by name and by value, since either spelling
			// arriving here is the same mistake.
			for _, banned := range []string{"AnnotationInternetEgress", "k3sm.io/internet-egress"} {
				if strings.Contains(string(src), banned) {
					t.Errorf("%s mentions %q; the internet-egress opt-in belongs in the runtime-area package, not mlx/v1alpha1", name, banned)
				}
			}
			// And the general rule behind it: every key literal here is
			// mlx.k3sm.io-domain. A bare "k3sm.io/..." literal is a key that
			// belongs in the runtime area. Comment prose is stripped first so the
			// doc may still explain the rule it enforces.
			for _, line := range strings.Split(string(src), "\n") {
				code := line
				if i := strings.Index(code, "//"); i >= 0 {
					code = code[:i]
				}
				if idx := strings.Index(code, `"k3sm.io/`); idx >= 0 {
					t.Errorf("%s declares a bare k3sm.io/* key (%s); this package holds only %s/* keys", name, strings.TrimSpace(line), GroupName)
				}
			}
		})
	}
}
