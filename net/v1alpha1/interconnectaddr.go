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

// ErrNoInterconnectAddress reports that a (node index, slot) pair has no
// interconnect address: the index is outside 0..MaxInterconnectNodeIndex (the
// cable's node ceiling) or the slot is outside 0..MaxInterconnectSlot. A node or
// pod in that position gets no interconnect address at all; a caller must never
// improvise one.
var ErrNoInterconnectAddress = errors.New("netv1alpha1: no interconnect address for this node index and slot")

const (
	// InterconnectCIDR is the one connected subnet every interconnect address
	// lies in: the low RFC 3927 §2.1 reserved half of 169.254/16, configured on
	// bridge0 with no extra route.
	InterconnectCIDR = "169.254.0.0/24"

	// MaxInterconnectNodeIndex is the largest node cluster index that gets an
	// interconnect address. Indexes 0..30 fill InterconnectCIDR, so the cable
	// carries at most 31 nodes.
	MaxInterconnectNodeIndex = 30

	// InterconnectSlotNode is the slot of the node's own interconnect address.
	InterconnectSlotNode = 0
	// InterconnectSlotMLXRank is the slot reserved for the node's sharded-MLX
	// rank pod.
	InterconnectSlotMLXRank = 1
	// InterconnectSlotFirstPod is the first slot for other opted-in pods.
	InterconnectSlotFirstPod = 2
	// MaxInterconnectSlot is the largest slot; slots 2..7 give six general pods
	// per node.
	MaxInterconnectSlot = 7

	// interconnectSlotsPerNode is the address stride per node index.
	interconnectSlotsPerNode = MaxInterconnectSlot + 1
)

// InterconnectSlotAddress returns the interconnect IPv4 address of slot slot on
// the node with cluster index idx:
//
//	169.254.0.(8·idx + slot + 1)
//
// Slot InterconnectSlotNode (0) is the node's address, InterconnectSlotMLXRank
// (1) its sharded-MLX rank, and 2..MaxInterconnectSlot general opted-in pods.
// The address is DERIVED, never allocated. The largest host octet is 248, so
// the subnet's broadcast address is never produced, and every (idx, slot) pair
// maps to a distinct address in InterconnectCIDR.
//
// It is deliberately self-contained rather than a call to LinkIP: for idx
// 0..30 it yields the same address as LinkIP(idx, slot) (a test pins the
// equality), but it refuses idx 31 and above, which LinkIP maps into the other
// reserved half that the interconnect never configures. Any idx outside
// 0..MaxInterconnectNodeIndex or slot outside 0..MaxInterconnectSlot returns
// ErrNoInterconnectAddress.
func InterconnectSlotAddress(idx, slot int) (netip.Addr, error) {
	if idx < 0 || idx > MaxInterconnectNodeIndex {
		return netip.Addr{}, fmt.Errorf("%w: node index %d is outside 0..%d", ErrNoInterconnectAddress, idx, MaxInterconnectNodeIndex)
	}
	if slot < 0 || slot > MaxInterconnectSlot {
		return netip.Addr{}, fmt.Errorf("%w: slot %d is outside 0..%d", ErrNoInterconnectAddress, slot, MaxInterconnectSlot)
	}
	return netip.AddrFrom4([4]byte{169, 254, 0, byte(interconnectSlotsPerNode*idx + slot + 1)}), nil
}
