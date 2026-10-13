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
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
)

// InterconnectLedger is the on-disk record of the interconnect address aliases
// the network helper added itself and therefore owns. The helper rewrites it on
// every change and reads it back at start, so it removes only the aliases it
// owns.
//
// The type carries no trust or range checks: a reader that acts on an entry
// (for example, removing an address) checks Slot and Address against the
// addresses it derived itself before acting.
type InterconnectLedger struct {
	// Aliases lists the owned aliases. Marshal writes them ordered by Slot.
	Aliases []InterconnectLedgerEntry `json:"aliases"`
}

// InterconnectLedgerEntry is one owned interconnect alias.
type InterconnectLedgerEntry struct {
	// Slot is the interconnect slot the alias occupies (see
	// InterconnectSlotAddress). It is not range-checked on parse.
	Slot int `json:"slot"`
	// Address is the alias address in netip.Addr text form. It is kept as a
	// string and not parsed, so a reader decides how to treat a malformed one.
	Address string `json:"address"`
}

// ParseInterconnectLedger decodes a ledger file. It is lenient: only malformed
// JSON is an error. Unknown fields are ignored, and an empty object or a
// missing "aliases" key is an empty ledger.
func ParseInterconnectLedger(raw []byte) (InterconnectLedger, error) {
	var l InterconnectLedger
	if err := json.Unmarshal(raw, &l); err != nil {
		return InterconnectLedger{}, fmt.Errorf("decode interconnect ledger: %w", err)
	}
	return l, nil
}

// Marshal returns the ledger file bytes: compact JSON with the aliases ordered
// by Slot and a trailing newline. A nil Aliases is written as an empty array,
// never null. The receiver is not modified.
func (l InterconnectLedger) Marshal() ([]byte, error) {
	aliases := make([]InterconnectLedgerEntry, len(l.Aliases))
	copy(aliases, l.Aliases)
	slices.SortStableFunc(aliases, func(a, b InterconnectLedgerEntry) int { return cmp.Compare(a.Slot, b.Slot) })
	raw, err := json.Marshal(InterconnectLedger{Aliases: aliases})
	if err != nil {
		return nil, fmt.Errorf("encode interconnect ledger: %w", err)
	}
	return append(raw, '\n'), nil
}
