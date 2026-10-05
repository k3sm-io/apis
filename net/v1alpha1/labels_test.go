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

package netv1alpha1

import (
	"strings"
	"testing"
)

// TestDirectLinkLabelKeys pins the exact byte strings of the direct-link label
// keys and that each is a valid, medium-agnostic k3sm.io/* key.
//
// A node is labelled with them and selectors are written against them, so a
// rename breaks every already-labelled node silently: a selector that matches
// nothing is not an error.
func TestDirectLinkLabelKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, got, want string
	}{
		{"LabelDirectLinkPorts", LabelDirectLinkPorts, "k3sm.io/direct-link-ports"},
		{"LabelDirectLinkMedium", LabelDirectLinkMedium, "k3sm.io/direct-link-medium"},
		{"LabelDirectLinkSpeedGbps", LabelDirectLinkSpeedGbps, "k3sm.io/direct-link-speed-gbps"},
		{"LabelDirectLinks", LabelDirectLinks, "k3sm.io/direct-links"},
		{"LabelRDMA", LabelRDMA, "k3sm.io/rdma"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
			if !strings.HasPrefix(tc.got, "k3sm.io/") || strings.Count(tc.got, "/") != 1 {
				t.Errorf("%s = %q is not a k3sm.io/<name> key", tc.name, tc.got)
			}
			if name := tc.got[strings.Index(tc.got, "/")+1:]; name == "" || len(name) > 63 {
				t.Errorf("%s name segment %q must be 1..63 characters", tc.name, name)
			}
			// The medium is a value, never part of a key.
			if strings.Contains(tc.got, MediumThunderbolt) {
				t.Errorf("%s = %q names a medium; keys are medium-agnostic", tc.name, tc.got)
			}
		})
	}
}
