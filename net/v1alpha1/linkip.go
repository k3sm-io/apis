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
)

// ErrNoLinkAddress reports that a (node index, port ordinal) pair has no
// direct-link address: the index is outside 0..MaxLinkNodeIndex or the ordinal
// is outside 0..MaxPortOrdinal. A port in that position gets no address at all;
// a caller must never improvise one.
var ErrNoLinkAddress = errors.New("netv1alpha1: no direct-link address for this node index and port")

const (
	// MaxLinkNodeIndex is the largest node index LinkIP derives an address for.
	// Indexes 0..30 map into 169.254.0.0/24 and 31..61 into 169.254.255.0/24.
	MaxLinkNodeIndex = 61
	// MaxPortOrdinal is the largest port ordinal (receptacle number − 1).
	MaxPortOrdinal = 7

	// lowHalfMaxIndex is the last node index in the 169.254.0.0/24 half.
	lowHalfMaxIndex = 30
	// portsPerNode is the address stride per node index.
	portsPerNode = MaxPortOrdinal + 1
)

// LinkIP returns the direct-link IPv4 address of port portOrdinal on the node
// with pod-/24 index idx.
//
// The address is DERIVED, never allocated: p = portOrdinal (the receptacle
// number − 1, 0..7) and
//
//	idx 0..30  → 169.254.0.(8·idx + p + 1)
//	idx 31..61 → 169.254.255.(8·(idx − 31) + p + 1)
//
// Both /24s are the RFC 3927 §2.1 reserved halves of 169.254/16, which a
// conforming host never self-assigns, so a derived address cannot collide with
// a self-assigned one on another interface. The largest host octet is 248, so
// neither /24's broadcast address is ever produced, and every (idx, p) pair
// maps to a distinct address. Any other idx or portOrdinal returns
// ErrNoLinkAddress.
//
// This is the one implementation: DirectLink validation, the network helper
// that configures the address, and the node that records it all call it.
func LinkIP(idx, portOrdinal int) (netip.Addr, error) {
	if portOrdinal < 0 || portOrdinal > MaxPortOrdinal {
		return netip.Addr{}, fmt.Errorf("%w: port ordinal %d is outside 0..%d", ErrNoLinkAddress, portOrdinal, MaxPortOrdinal)
	}
	switch {
	case idx >= 0 && idx <= lowHalfMaxIndex:
		return netip.AddrFrom4([4]byte{169, 254, 0, byte(portsPerNode*idx + portOrdinal + 1)}), nil
	case idx > lowHalfMaxIndex && idx <= MaxLinkNodeIndex:
		return netip.AddrFrom4([4]byte{169, 254, 255, byte(portsPerNode*(idx-lowHalfMaxIndex-1) + portOrdinal + 1)}), nil
	default:
		return netip.Addr{}, fmt.Errorf("%w: node index %d is outside 0..%d", ErrNoLinkAddress, idx, MaxLinkNodeIndex)
	}
}

// IsLinkAddress reports whether a lies in 169.254.0.0/24 or 169.254.255.0/24,
// the two reserved halves LinkIP derives into. An IPv4-mapped IPv6 address is
// judged by its IPv4 form. It checks the address class only, not that a is the
// address LinkIP would derive for any particular node and port.
func IsLinkAddress(a netip.Addr) bool {
	a = a.Unmap()
	if !a.Is4() {
		return false
	}
	b := a.As4()
	return b[0] == 169 && b[1] == 254 && (b[2] == 0 || b[2] == 255)
}
