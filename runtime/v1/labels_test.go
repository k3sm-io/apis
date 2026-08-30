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
)

// TestRosettaAndPlatformKeys pins the exact byte strings of the M11 label and
// annotation keys.
//
// These are not decoration: a node is SELECTED by these keys and a pod's images
// are RESOLVED by that annotation, so a typo or a rename is a break for every
// already-labelled node and every already-scheduled pod — and one that compiles
// perfectly. The constants exist so no consumer spells a literal; this test
// exists so the constants themselves cannot drift silently.
func TestRosettaAndPlatformKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, got, want string
	}{
		{"LabelRosetta", LabelRosetta, "k3sm.io/rosetta"},
		{"LabelRosettaLinux", LabelRosettaLinux, "k3sm.io/rosetta-linux"},
		{"AnnotationImagePlatform", AnnotationImagePlatform, "k3sm.io/image-platform"},
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
