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

package runtimev1

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

// TestRosettaAndPlatformKeys pins the exact byte strings of the runtime-area
// label and annotation keys (M11's Rosetta/platform keys and M8's
// internet-egress opt-in).
//
// These are not decoration: a node is SELECTED by these keys, a pod's images are
// RESOLVED by the platform annotation, and a pod's sandbox is WIDENED by the
// egress annotation — so a typo or a rename is a break for every already-labelled
// node and every already-admitted pod, and one that compiles perfectly. The
// constants exist so no consumer spells a literal; this test exists so the
// constants themselves cannot drift silently.
func TestRosettaAndPlatformKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, got, want string
	}{
		{"LabelRosetta", LabelRosetta, "k3sm.io/rosetta"},
		{"LabelRosettaLinux", LabelRosettaLinux, "k3sm.io/rosetta-linux"},
		{"AnnotationImagePlatform", AnnotationImagePlatform, "k3sm.io/image-platform"},
		{"AnnotationInternetEgress", AnnotationInternetEgress, "k3sm.io/internet-egress"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
			// Project convention: labels and annotations are k3sm.io/*-domain
			// (CRD groups are <area>.k3sm.io; the two must not be confused).
			if !strings.HasPrefix(tc.got, "k3sm.io/") {
				t.Errorf("%s = %q, want the k3sm.io/ label domain", tc.name, tc.got)
			}
			// A Kubernetes label/annotation key is one optional prefix and one
			// name; a second slash makes it unsettable, which no test of the
			// constant's value alone would notice.
			if strings.Count(tc.got, "/") != 1 {
				t.Errorf("%s = %q is not a valid prefixed key (exactly one '/')", tc.name, tc.got)
			}
			if name := tc.got[strings.Index(tc.got, "/")+1:]; name == "" || len(name) > 63 {
				t.Errorf("%s name segment %q must be 1..63 characters", tc.name, name)
			}
		})
	}

	// The two Rosetta labels answer different questions — host capability versus
	// guest capability — so they must stay distinct keys. Collapsing them would
	// make one answer a silent lie on every node where the conjuncts disagree.
	if LabelRosetta == LabelRosettaLinux {
		t.Error("LabelRosetta and LabelRosettaLinux must be distinct keys")
	}
	// And the guest label must not merely be a prefix relationship a selector
	// could match by accident.
	if LabelRosettaLinux == LabelRosetta+"/linux" {
		t.Error("LabelRosettaLinux must be a sibling key, not a sub-path of LabelRosetta")
	}
}

// TestHardwareKeysArePinned pins the exact byte strings of the hardware keys and
// proves each is syntax a live apiserver accepts, with the same validation helpers
// the apiserver uses. A hand-rolled shape check would only repeat this test's own
// reading of the rule.
//
// The name check has a ceiling: it shows that requests.k3sm.io/gpu is well formed,
// not that a live ResourceQuota counts it. Only a real quota can show that.
func TestHardwareKeysArePinned(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, got, want string
		// values are the forms the key carries, each checked as a label value.
		values []string
	}{
		{"ResourceGPU", ResourceGPU, "k3sm.io/gpu", []string{"1", "2"}},
		{"LabelGPUPresent", LabelGPUPresent, "k3sm.io/gpu.present", []string{"true"}},
		{"LabelGPUMemoryGB", LabelGPUMemoryGB, "k3sm.io/gpu-memory-gb", []string{"1", "48", "96"}},
		{"LabelChip", LabelChip, "k3sm.io/chip", []string{"apple-m4-max"}},
		{"LabelChipFamily", LabelChipFamily, "k3sm.io/chip-family", []string{"m4"}},
		{"LabelMemoryGB", LabelMemoryGB, "k3sm.io/memory-gb", []string{"16", "128"}},
	}
	seen := make(map[string]string, len(cases))
	for _, tc := range cases {
		if other, dup := seen[tc.got]; dup {
			t.Errorf("%s and %s are the same key %q", tc.name, other, tc.got)
		}
		seen[tc.got] = tc.name
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
			if errs := validation.IsQualifiedName(tc.got); len(errs) != 0 {
				t.Errorf("%s = %q is not a qualified name: %v", tc.name, tc.got, errs)
			}
			for _, v := range tc.values {
				if errs := validation.IsValidLabelValue(v); len(errs) != 0 {
					t.Errorf("%s value %q is not a valid label value: %v", tc.name, v, errs)
				}
			}
		})
	}

	// A ResourceQuota caps an extended resource under the requests. form of its
	// name, so that form is published syntax too.
	const wantQuotaKey = "requests.k3sm.io/gpu"
	quotaKey := "requests." + ResourceGPU
	if quotaKey != wantQuotaKey {
		t.Errorf("quota key = %q, want %q", quotaKey, wantQuotaKey)
	}
	if errs := validation.IsQualifiedName(quotaKey); len(errs) != 0 {
		t.Errorf("quota key %q is not a qualified name: %v", quotaKey, errs)
	}

	// Named on their own because a selector can easily confuse them: the first is
	// the whole of unified memory, the second is what the GPU can use.
	if LabelMemoryGB == LabelGPUMemoryGB {
		t.Error("LabelMemoryGB and LabelGPUMemoryGB must be different keys")
	}
}

// TestChipSlug pins the slug rule. The node advertiser and every selector agree on
// a chip label's value only through this function, so each step of the rule has a
// case, and each result has to be a label value the apiserver accepts.
func TestChipSlug(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"brand_with_spaces", "Apple M4 Max", "apple-m4-max"},
		{"family", "M4", "m4"},
		{"already_a_slug", "apple-m4-max", "apple-m4-max"},
		{"leading_and_trailing_junk", "  Apple M4 Max!! ", "apple-m4-max"},
		{"run_collapses_to_one_dash", "Apple   M4///Max", "apple-m4-max"},
		{"non_ascii_becomes_a_separator", "Apple™ M4", "apple-m4"},
		// Case folding is Unicode, not ASCII: these two fold into [a-z].
		{"kelvin_sign_folds_to_k", "K9", "k9"},
		{"dotted_capital_i_folds_to_i", "İx", "ix"},
		{"empty", "", ""},
		{"all_junk", " -- ", ""},
		{"exactly_the_limit_kept", strings.Repeat("a", 63), strings.Repeat("a", 63)},
		{"over_the_limit_truncated", strings.Repeat("a", 64), strings.Repeat("a", 63)},
		{"truncation_trims_a_boundary_dash", strings.Repeat("a", 62) + " b", strings.Repeat("a", 62)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ChipSlug(tc.raw)
			if got != tc.want {
				t.Errorf("ChipSlug(%q) = %q, want %q", tc.raw, got, tc.want)
			}
			if errs := validation.IsValidLabelValue(got); len(errs) != 0 {
				t.Errorf("ChipSlug(%q) = %q is not a valid label value: %v", tc.raw, got, errs)
			}
		})
	}
}
