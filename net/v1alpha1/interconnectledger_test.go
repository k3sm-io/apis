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
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// interconnectLedgerGolden holds the exact bytes the network helper writes
// for a three-alias ledger today.
const interconnectLedgerGolden = "interconnect-ledger.golden.json"

func TestInterconnectLedgerRoundTrip(t *testing.T) {
	t.Parallel()
	golden, err := os.ReadFile(filepath.Join("testdata", interconnectLedgerGolden))
	if err != nil {
		t.Fatal(err)
	}
	wantGolden := InterconnectLedger{Aliases: []InterconnectLedgerEntry{
		{Slot: 0, Address: "169.254.0.9"},
		{Slot: 1, Address: "169.254.0.10"},
		{Slot: 2, Address: "169.254.0.11"},
	}}

	t.Run("golden parse then marshal is byte-identical", func(t *testing.T) {
		t.Parallel()
		got, err := ParseInterconnectLedger(golden)
		if err != nil {
			t.Fatalf("ParseInterconnectLedger(golden): %v", err)
		}
		if !reflect.DeepEqual(got, wantGolden) {
			t.Fatalf("parsed %+v, want %+v", got, wantGolden)
		}
		out, err := got.Marshal()
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if !bytes.Equal(out, golden) {
			t.Fatalf("Marshal = %q, want golden %q", out, golden)
		}
	})

	marshalCases := []struct {
		name string
		in   InterconnectLedger
		want string
	}{
		{
			name: "tags pinned, compact, trailing newline",
			in:   InterconnectLedger{Aliases: []InterconnectLedgerEntry{{Slot: 3, Address: "169.254.0.4"}}},
			want: `{"aliases":[{"slot":3,"address":"169.254.0.4"}]}` + "\n",
		},
		{
			name: "nil aliases is an empty array",
			in:   InterconnectLedger{},
			want: `{"aliases":[]}` + "\n",
		},
		{
			name: "aliases ordered by slot",
			in: InterconnectLedger{Aliases: []InterconnectLedgerEntry{
				{Slot: 2, Address: "169.254.0.11"},
				{Slot: 0, Address: "169.254.0.9"},
				{Slot: 1, Address: "169.254.0.10"},
			}},
			want: string(golden),
		},
	}
	for _, tc := range marshalCases {
		t.Run("marshal/"+tc.name, func(t *testing.T) {
			t.Parallel()
			before := append([]InterconnectLedgerEntry(nil), tc.in.Aliases...)
			out, err := tc.in.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(out) != tc.want {
				t.Fatalf("Marshal = %q, want %q", out, tc.want)
			}
			if !reflect.DeepEqual(tc.in.Aliases, before) && len(before) > 0 {
				t.Fatalf("Marshal modified the receiver: %+v, was %+v", tc.in.Aliases, before)
			}
		})
	}

	parseCases := []struct {
		name    string
		in      string
		want    InterconnectLedger
		wantErr bool
	}{
		{
			name: "literal tags",
			in:   `{"aliases":[{"slot":5,"address":"169.254.0.6"}]}`,
			want: InterconnectLedger{Aliases: []InterconnectLedgerEntry{{Slot: 5, Address: "169.254.0.6"}}},
		},
		{
			name: "unknown fields tolerated",
			in:   `{"schema":9,"aliases":[{"slot":1,"address":"169.254.0.2","owner":"x"}],"extra":{"a":1}}`,
			want: InterconnectLedger{Aliases: []InterconnectLedgerEntry{{Slot: 1, Address: "169.254.0.2"}}},
		},
		{
			name: "address kept as an unparsed string",
			in:   `{"aliases":[{"slot":99,"address":"not-an-ip"}]}`,
			want: InterconnectLedger{Aliases: []InterconnectLedgerEntry{{Slot: 99, Address: "not-an-ip"}}},
		},
		{name: "empty object", in: `{}`, want: InterconnectLedger{}},
		{name: "missing aliases key", in: `{"other":true}`, want: InterconnectLedger{}},
		{name: "empty aliases", in: `{"aliases":[]}` + "\n", want: InterconnectLedger{Aliases: []InterconnectLedgerEntry{}}},
		{name: "malformed JSON", in: `{"aliases":[`, wantErr: true},
		{name: "empty input", in: ``, wantErr: true},
		{name: "wrong type", in: `{"aliases":{"slot":1}}`, wantErr: true},
	}
	for _, tc := range parseCases {
		t.Run("parse/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseInterconnectLedger([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseInterconnectLedger(%q) = %+v, want an error", tc.in, got)
				}
				if !reflect.DeepEqual(got, InterconnectLedger{}) {
					t.Fatalf("an error returned a non-zero ledger %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseInterconnectLedger(%q): %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseInterconnectLedger(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}
