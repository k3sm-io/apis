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
	"errors"
	"fmt"
	"net/netip"
	"testing"

	netv1 "k3sm.io/apis/net/v1"
)

// TestLinkIPBoundaries pins the LinkIP derivation at every boundary and proves
// it injective over its whole domain.
//
// The derived address is configured on a cable and recorded in a peer's routes,
// so a change to the formula strands every configured address; and two
// (idx, port) pairs mapping to one address would put two nodes on one IP.
func TestLinkIPBoundaries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		idx, port int
		want      string
		wantErr   bool
	}{
		{idx: 0, port: 0, want: "169.254.0.1"},
		{idx: 0, port: 7, want: "169.254.0.8"},
		{idx: 0, port: 8, wantErr: true},
		{idx: 0, port: -1, wantErr: true},
		{idx: 1, port: 0, want: "169.254.0.9"},
		{idx: 30, port: 0, want: "169.254.0.241"},
		{idx: 30, port: 7, want: "169.254.0.248"},
		{idx: 30, port: 8, wantErr: true},
		{idx: 31, port: 0, want: "169.254.255.1"},
		{idx: 31, port: 7, want: "169.254.255.8"},
		{idx: 61, port: 0, want: "169.254.255.241"},
		{idx: 61, port: 7, want: "169.254.255.248"},
		{idx: 61, port: 8, wantErr: true},
		{idx: 62, port: 0, wantErr: true},
		{idx: 62, port: 7, wantErr: true},
		{idx: -1, port: 0, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("idx=%d/port=%d", tc.idx, tc.port), func(t *testing.T) {
			t.Parallel()
			got, err := LinkIP(tc.idx, tc.port)
			if tc.wantErr {
				if !errors.Is(err, ErrNoLinkAddress) {
					t.Fatalf("LinkIP(%d, %d) = %v, %v; want ErrNoLinkAddress", tc.idx, tc.port, got, err)
				}
				if got.IsValid() {
					t.Fatalf("LinkIP(%d, %d) returned an address %v alongside its error", tc.idx, tc.port, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("LinkIP(%d, %d) error: %v", tc.idx, tc.port, err)
			}
			if got.String() != tc.want {
				t.Fatalf("LinkIP(%d, %d) = %v, want %s", tc.idx, tc.port, got, tc.want)
			}
		})
	}

	t.Run("all 62x8 values are distinct reserved-half addresses", func(t *testing.T) {
		t.Parallel()
		seen := make(map[netip.Addr]string, (MaxLinkNodeIndex+1)*(MaxPortOrdinal+1))
		for idx := 0; idx <= MaxLinkNodeIndex; idx++ {
			for p := 0; p <= MaxPortOrdinal; p++ {
				a, err := LinkIP(idx, p)
				if err != nil {
					t.Fatalf("LinkIP(%d, %d): %v", idx, p, err)
				}
				if !IsLinkAddress(a) {
					t.Fatalf("LinkIP(%d, %d) = %v is outside the reserved halves", idx, p, a)
				}
				if last := a.As4()[3]; last == 0 || last == 255 {
					t.Fatalf("LinkIP(%d, %d) = %v is a network or broadcast address", idx, p, a)
				}
				key := fmt.Sprintf("(%d,%d)", idx, p)
				if prev, dup := seen[a]; dup {
					t.Fatalf("LinkIP%s and LinkIP%s both = %v", prev, key, a)
				}
				seen[a] = key
			}
		}
		if len(seen) != 62*8 {
			t.Fatalf("derived %d distinct addresses, want %d", len(seen), 62*8)
		}
	})
}

// TestIsLinkAddress pins the reserved-half address class.
func TestIsLinkAddress(t *testing.T) {
	t.Parallel()
	cases := []struct {
		addr string
		want bool
	}{
		{"169.254.0.0", true},
		{"169.254.0.1", true},
		{"169.254.0.255", true},
		{"169.254.255.0", true},
		{"169.254.255.248", true},
		{"::ffff:169.254.0.9", true},
		{"169.254.1.1", false},
		{"169.254.254.1", false},
		{"169.253.0.1", false},
		{"192.168.1.20", false},
		{"fe80::1", false},
		{"fe80::1%en2", false},
	}
	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			t.Parallel()
			if got := IsLinkAddress(netip.MustParseAddr(tc.addr)); got != tc.want {
				t.Fatalf("IsLinkAddress(%s) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
	if IsLinkAddress(netip.Addr{}) {
		t.Fatal("IsLinkAddress(zero Addr) = true")
	}
}

// TestLinkIPAddressesPassMeshPeerValidate proves the stable net/v1 MeshPeer
// validation admits every address LinkIP derives, as a direct candidate and as
// Endpoint, and refuses it as an underlay candidate.
//
// net/v1 keeps its own reserved-half check so the stable package never imports
// this alpha one; this test is what keeps the two agreeing.
func TestLinkIPAddressesPassMeshPeerValidate(t *testing.T) {
	t.Parallel()
	base := netv1.MeshPeerSpec{
		NodeName: "n", PublicKey: "k", Endpoint: "192.168.1.20:51820",
		PodCIDR: "100.64.1.0/24", AllowedIPs: []string{"100.64.1.0/24"},
	}.WithDefaults()
	for idx := 0; idx <= MaxLinkNodeIndex; idx++ {
		for p := 0; p <= MaxPortOrdinal; p++ {
			a, err := LinkIP(idx, p)
			if err != nil {
				t.Fatalf("LinkIP(%d, %d): %v", idx, p, err)
			}
			hp := netip.AddrPortFrom(a, 51820).String()

			direct := *base.DeepCopy()
			direct.Endpoints = []netv1.EndpointCandidate{{Address: hp, Link: netv1.EndpointLinkDirect}}
			if err := direct.Validate(); err != nil {
				t.Fatalf("direct candidate %s refused: %v", hp, err)
			}
			ep := *base.DeepCopy()
			ep.Endpoint = hp
			if err := ep.Validate(); err != nil {
				t.Fatalf("Endpoint %s refused: %v", hp, err)
			}
			under := *base.DeepCopy()
			under.Endpoints = []netv1.EndpointCandidate{{Address: hp, Link: netv1.EndpointLinkUnderlay}}
			if err := under.Validate(); err == nil {
				t.Fatalf("underlay candidate %s admitted", hp)
			}
		}
	}
}
