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
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	budgetv1alpha1 "k3sm.io/apis/budget/v1alpha1"
)

// TestGuestContainerMemoryLimitFields pins the additive guest/v1 surface for
// vm-pod memory limits and the node budget page:
//
//   - GuestContainer.memory_limit_bytes (15) and memory_high_bytes (16), int64,
//     present in proto-JSON when set and ABSENT when unset, so an older guest
//     init that decodes strictly still boots a spec that sets neither;
//   - ContainerExited.oom_killed (3, bool), the field the guest reports a
//     memory.max kill through, unchanged;
//   - the unary PublishBudget(Budget) returns (PublishBudgetResponse) verb, and
//     Budget.page (1, bytes) documented as exactly one page of the budget page
//     ABI's fixed size;
//   - the publish-budget and memory-limits capability tokens named in the
//     HealthResponse.capabilities comment;
//   - every touched message keeps exactly the 100..149 headroom band.
//
// It is descriptor-driven, so it fails at runtime rather than compile time
// against a guest.proto without the new surface.
func TestGuestContainerMemoryLimitFields(t *testing.T) {
	t.Parallel()
	fd := File_guest_v1_guest_proto

	src, err := os.ReadFile("guest.proto")
	if err != nil {
		t.Fatalf("read guest.proto: %v", err)
	}
	body := func(t *testing.T, decl string) string {
		t.Helper()
		s := string(src)
		start := strings.Index(s, decl)
		if start < 0 {
			t.Fatalf("guest.proto: %q not found", decl)
		}
		end := strings.Index(s[start:], "\n}")
		if end < 0 {
			t.Fatalf("guest.proto: %q body not terminated", decl)
		}
		return s[start : start+end]
	}

	gc := fd.Messages().ByName("GuestContainer")
	if gc == nil {
		t.Fatal("message k3sm.guest.v1.GuestContainer not found")
	}

	fields := []struct {
		name     protoreflect.Name
		number   protoreflect.FieldNumber
		jsonName string
		value    int64
	}{
		{"memory_limit_bytes", 15, "memoryLimitBytes", 1 << 30},
		{"memory_high_bytes", 16, "memoryHighBytes", 966367641},
	}
	for _, tc := range fields {
		t.Run("GuestContainer."+string(tc.name), func(t *testing.T) {
			t.Parallel()
			f := gc.Fields().ByName(tc.name)
			if f == nil {
				t.Fatalf("GuestContainer.%s is not declared", tc.name)
			}
			if f.Number() != tc.number {
				t.Errorf("number = %d, want %d", f.Number(), tc.number)
			}
			if f.Kind() != protoreflect.Int64Kind {
				t.Errorf("kind = %v, want int64", f.Kind())
			}
			if f.IsList() || f.IsMap() || f.HasPresence() {
				t.Errorf("%s must be a singular proto3 scalar without presence", tc.name)
			}
			if f.JSONName() != tc.jsonName {
				t.Errorf("JSON name = %q, want %q", f.JSONName(), tc.jsonName)
			}

			unset := dynamicpb.NewMessage(gc)
			unset.Set(gc.Fields().ByName("name"), protoreflect.ValueOfString("app"))
			if keys := jsonKeys(t, unset); keys[tc.jsonName] != nil {
				t.Errorf("unset %s encodes key %q, want it absent", tc.name, tc.jsonName)
			}

			set := dynamicpb.NewMessage(gc)
			set.Set(gc.Fields().ByName("name"), protoreflect.ValueOfString("app"))
			set.Set(f, protoreflect.ValueOfInt64(tc.value))
			// proto-JSON carries an int64 as a decimal string.
			want := strconv.FormatInt(tc.value, 10)
			if got, ok := jsonKeys(t, set)[tc.jsonName]; !ok || got != want {
				t.Errorf("set %s encodes %q = %v (present=%v), want %q", tc.name, tc.jsonName, got, ok, want)
			}
			raw, err := protojson.Marshal(set)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			back := dynamicpb.NewMessage(gc)
			if err := protojson.Unmarshal(raw, back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := back.Get(f).Int(); got != tc.value {
				t.Errorf("round trip = %d, want %d", got, tc.value)
			}
		})
	}

	t.Run("GuestContainer comment names the memory-limits capability", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(body(t, "message GuestContainer {"), "memory-limits") {
			t.Error("GuestContainer memory fields do not name the memory-limits capability")
		}
	})

	t.Run("ContainerExited.oom_killed is unchanged", func(t *testing.T) {
		t.Parallel()
		md := fd.Messages().ByName("ContainerExited")
		if md == nil {
			t.Fatal("message ContainerExited not found")
		}
		f := md.Fields().ByName("oom_killed")
		if f == nil || f.Number() != 3 || f.Kind() != protoreflect.BoolKind {
			t.Fatalf("ContainerExited.oom_killed must be field 3, bool; got %v", f)
		}
	})

	t.Run("PublishBudget is a unary Budget to PublishBudgetResponse verb", func(t *testing.T) {
		t.Parallel()
		sd := fd.Services().ByName("GuestAgent")
		if sd == nil {
			t.Fatal("service GuestAgent not found")
		}
		m := sd.Methods().ByName("PublishBudget")
		if m == nil {
			t.Fatal("GuestAgent.PublishBudget is not declared")
		}
		if m.IsStreamingClient() || m.IsStreamingServer() {
			t.Error("PublishBudget must be unary")
		}
		if got := m.Input().FullName(); got != "k3sm.guest.v1.Budget" {
			t.Errorf("PublishBudget input = %s, want k3sm.guest.v1.Budget", got)
		}
		if got := m.Output().FullName(); got != "k3sm.guest.v1.PublishBudgetResponse" {
			t.Errorf("PublishBudget output = %s, want k3sm.guest.v1.PublishBudgetResponse", got)
		}
	})

	t.Run("Budget.page is field 1, bytes, one page of the ABI's fixed size", func(t *testing.T) {
		t.Parallel()
		md := fd.Messages().ByName("Budget")
		if md == nil {
			t.Fatal("message Budget not found")
		}
		if md.Fields().Len() != 1 {
			t.Errorf("Budget has %d fields, want exactly 1", md.Fields().Len())
		}
		f := md.Fields().ByName("page")
		if f == nil || f.Number() != 1 || f.Kind() != protoreflect.BytesKind || f.IsList() {
			t.Fatalf("Budget.page must be field 1, singular bytes; got %v", f)
		}
		// The comment states the exact size a receiver enforces; it must agree
		// with the page ABI the producer encodes.
		b := body(t, "message Budget {")
		if want := strconv.Itoa(budgetv1alpha1.PageSize); !strings.Contains(b, want) {
			t.Errorf("Budget comment does not state the page size %s", want)
		}
		if !strings.Contains(b, "PageSize") {
			t.Error("Budget comment does not name budget/v1alpha1 PageSize")
		}
	})

	t.Run("HealthResponse.capabilities names both new tokens", func(t *testing.T) {
		t.Parallel()
		b := body(t, "message HealthResponse {")
		for _, tok := range []string{"publish-budget", "memory-limits"} {
			if !strings.Contains(b, tok) {
				t.Errorf("HealthResponse.capabilities comment does not name %s", tok)
			}
		}
	})

	for _, name := range []protoreflect.Name{"GuestContainer", "Budget", "PublishBudgetResponse"} {
		t.Run(string(name)+" reserved band is exactly 100..149", func(t *testing.T) {
			t.Parallel()
			md := fd.Messages().ByName(name)
			if md == nil {
				t.Fatalf("message %s not found", name)
			}
			assertOnlyGuestBand(t, md)
		})
	}
}
