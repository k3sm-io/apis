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
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// TestGuestMountGuestPrivateCarve pins GuestMount.guest_private: field 7, a
// singular bool with JSON name guestPrivate, absent from proto-JSON when unset
// (so an older initramfs boots a spec that does not use it), the untouched
// 100..149 band, and a field comment that names the guest-private-mounts
// capability a host must see advertised before emitting it. The test is
// descriptor-driven so it fails at runtime, not compile time, without the field.
func TestGuestMountGuestPrivateCarve(t *testing.T) {
	t.Parallel()

	md := File_guest_v1_guest_proto.Messages().ByName("GuestMount")
	if md == nil {
		t.Fatal("message k3sm.guest.v1.GuestMount not found")
	}
	fd := md.Fields().ByName("guest_private")
	if fd == nil {
		t.Fatal("GuestMount.guest_private is not declared")
	}

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		if fd.Number() != 7 {
			t.Errorf("guest_private number = %d, want 7", fd.Number())
		}
		if fd.Kind() != protoreflect.BoolKind {
			t.Errorf("guest_private kind = %v, want bool", fd.Kind())
		}
		if fd.Cardinality() != protoreflect.Optional || fd.IsList() || fd.IsMap() {
			t.Errorf("guest_private must be singular, got cardinality %v", fd.Cardinality())
		}
		if fd.JSONName() != "guestPrivate" {
			t.Errorf("guest_private JSON name = %q, want guestPrivate", fd.JSONName())
		}
	})

	t.Run("proto-JSON presence", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name    string
			value   bool
			present bool
		}{
			{"absent when unset", false, false},
			{"present when set", true, true},
		}
		for _, tc := range cases {
			m := dynamicpb.NewMessage(md)
			m.Set(md.Fields().ByName("target"), protoreflect.ValueOfString("/run/k3sm/private"))
			m.Set(fd, protoreflect.ValueOfBool(tc.value))
			got, ok := jsonKeys(t, m)["guestPrivate"]
			if ok != tc.present {
				t.Errorf("%s: key guestPrivate present = %v (value %v), want present = %v", tc.name, ok, got, tc.present)
			}
			if tc.present && got != true {
				t.Errorf("%s: guestPrivate = %v, want true", tc.name, got)
			}
		}
	})

	t.Run("reserved band is exactly 100..149", func(t *testing.T) {
		t.Parallel()
		assertOnlyGuestBand(t, md)
	})

	t.Run("comment names the capability", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile("guest.proto")
		if err != nil {
			t.Fatalf("read guest.proto: %v", err)
		}
		src := string(raw)
		start := strings.Index(src, "message GuestMount {")
		if start < 0 {
			t.Fatal("guest.proto: message GuestMount not found")
		}
		end := strings.Index(src[start:], "\n}")
		if end < 0 {
			t.Fatal("guest.proto: GuestMount body not terminated")
		}
		if body := src[start : start+end]; !strings.Contains(body, "guest-private-mounts") {
			t.Error("GuestMount.guest_private comment does not name the guest-private-mounts capability")
		}
	})
}
