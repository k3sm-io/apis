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

package guestv1

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TestGuestContainerSidecarAndImageUserCarve pins GuestContainer.sidecar (13,
// bool) and GuestContainer.image_user (14, string), their JSON names, the
// untouched 100..149 band, and the proto-JSON property an older initramfs
// depends on: each key is present when set and ABSENT when unset, so a guest
// that predates the fields still boots a spec that does not use them. The test
// is descriptor-driven so it fails at runtime, not compile time, without them.
func TestGuestContainerSidecarAndImageUserCarve(t *testing.T) {
	t.Parallel()

	md := File_guest_v1_guest_proto.Messages().ByName("GuestContainer")
	if md == nil {
		t.Fatal("message k3sm.guest.v1.GuestContainer not found")
	}

	cases := []struct {
		name     protoreflect.Name
		number   protoreflect.FieldNumber
		kind     protoreflect.Kind
		jsonName string
		set      protoreflect.Value
		wantJSON any
	}{
		{"sidecar", 13, protoreflect.BoolKind, "sidecar", protoreflect.ValueOfBool(true), true},
		{"image_user", 14, protoreflect.StringKind, "imageUser", protoreflect.ValueOfString("app:staff"), "app:staff"},
	}

	for _, tc := range cases {
		t.Run(string(tc.name), func(t *testing.T) {
			t.Parallel()
			fd := md.Fields().ByName(tc.name)
			if fd == nil {
				t.Fatalf("GuestContainer.%s is not declared", tc.name)
			}
			if fd.Number() != tc.number {
				t.Errorf("%s number = %d, want %d", tc.name, fd.Number(), tc.number)
			}
			if fd.Kind() != tc.kind {
				t.Errorf("%s kind = %v, want %v", tc.name, fd.Kind(), tc.kind)
			}
			if fd.IsList() || fd.IsMap() {
				t.Errorf("%s must be singular", tc.name)
			}
			if fd.JSONName() != tc.jsonName {
				t.Errorf("%s JSON name = %q, want %q", tc.name, fd.JSONName(), tc.jsonName)
			}

			// Unset: the key must be absent from the proto-JSON encoding.
			unset := dynamicpb.NewMessage(md)
			unset.Set(md.Fields().ByName("name"), protoreflect.ValueOfString("app"))
			if keys := jsonKeys(t, unset); keys[tc.jsonName] != nil {
				t.Errorf("unset %s encodes key %q = %v, want the key absent", tc.name, tc.jsonName, keys[tc.jsonName])
			}

			// Set: the key must be present and round-trip.
			set := dynamicpb.NewMessage(md)
			set.Set(md.Fields().ByName("name"), protoreflect.ValueOfString("app"))
			set.Set(md.Fields().ByName("init"), protoreflect.ValueOfBool(true))
			set.Set(fd, tc.set)
			keys := jsonKeys(t, set)
			if got, ok := keys[tc.jsonName]; !ok || got != tc.wantJSON {
				t.Errorf("set %s encodes key %q = %v (present=%v), want %v", tc.name, tc.jsonName, got, ok, tc.wantJSON)
			}
			raw, err := protojson.Marshal(set)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			back := dynamicpb.NewMessage(md)
			if err := protojson.Unmarshal(raw, back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !back.Get(fd).Equal(tc.set) {
				t.Errorf("%s round trip = %v, want %v", tc.name, back.Get(fd), tc.set)
			}
		})
	}

	t.Run("reserved band is exactly 100..149", func(t *testing.T) {
		t.Parallel()
		assertOnlyGuestBand(t, md)
	})
}

// jsonKeys encodes m as proto-JSON and decodes it into a generic map, so a test
// can assert which keys are present rather than only what a decoder recovers.
func jsonKeys(t *testing.T, m protoreflect.ProtoMessage) map[string]any {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

// assertOnlyGuestBand fails unless md carries exactly one reserved range,
// 100..149, the file convention's headroom band.
func assertOnlyGuestBand(t *testing.T, md protoreflect.MessageDescriptor) {
	t.Helper()
	rr := md.ReservedRanges()
	if rr.Len() != 1 {
		t.Fatalf("%s has %d reserved ranges, want exactly 1", md.FullName(), rr.Len())
	}
	r := rr.Get(0) // [start, end) — end is exclusive
	if r[0] != 100 || r[1]-1 != 149 {
		t.Errorf("%s reserved range = %d..%d, want 100..149", md.FullName(), r[0], r[1]-1)
	}
}
