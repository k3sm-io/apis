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
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// TestPairingWireTypes pins the pairing wire versions, round-trips each type,
// and pins its JSON field names. These are exchanged between two binaries of
// possibly different releases, so a renamed tag is a silent break.
func TestPairingWireTypes(t *testing.T) {
	t.Parallel()
	if BeaconVersion != 1 || PairVersion != 1 {
		t.Fatalf("BeaconVersion = %d, PairVersion = %d; want 1, 1", BeaconVersion, PairVersion)
	}
	cases := []struct {
		name  string
		value any
		fresh func() any
		keys  []string
	}{
		{
			"Beacon",
			Beacon{Version: BeaconVersion, ClusterPin: "K10abc", NodeName: "server-1", JoinPort: 9345, PairingOpen: true},
			func() any { return &Beacon{} },
			[]string{"clusterPin", "joinPort", "nodeName", "pairingOpen", "version"},
		},
		{
			"PairRequest",
			PairRequest{Version: PairVersion, NodeName: "new-mac"},
			func() any { return &PairRequest{} },
			[]string{"nodeName", "version"},
		},
		{
			"PairResponse",
			PairResponse{Version: PairVersion, Token: "K10abc::node:secret", ServerURL: "https://[fe80::1%en2]:9345", ServerLinkIP: "169.254.0.1"},
			func() any { return &PairResponse{} },
			[]string{"serverLinkIP", "serverURL", "token", "version"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := tc.fresh()
			if err := json.Unmarshal(b, got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if v := reflect.ValueOf(got).Elem().Interface(); !reflect.DeepEqual(v, tc.value) {
				t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", v, tc.value)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if !reflect.DeepEqual(keys, tc.keys) {
				t.Fatalf("JSON keys = %v, want %v", keys, tc.keys)
			}
		})
	}
}
