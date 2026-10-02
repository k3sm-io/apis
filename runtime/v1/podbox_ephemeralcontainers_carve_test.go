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

package runtimev1

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestPodBoxEphemeralContainersCarve pins PodBox.ephemeral_containers: field
// 103, a repeated k3sm.runtime.v1.Container with JSON name ephemeralContainers,
// carved from the PodBox headroom so the band is exactly 104..199 and no other
// headroom range exists (a retired field's single-number reservation, listed in
// retiredFields, is not headroom). The test is descriptor-driven so it fails at runtime,
// not compile time, on a tree without the field.
func TestPodBoxEphemeralContainersCarve(t *testing.T) {
	t.Parallel()

	md := File_runtime_v1_runtime_proto.Messages().ByName("PodBox")
	if md == nil {
		t.Fatal("message k3sm.runtime.v1.PodBox not found")
	}

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("ephemeral_containers")
		if fd == nil {
			t.Fatal("PodBox.ephemeral_containers is not declared")
		}
		checks := []struct {
			what string
			ok   bool
			got  any
		}{
			{"number is 103", fd.Number() == 103, fd.Number()},
			{"is repeated", fd.IsList(), fd.Cardinality()},
			{"is a message", fd.Kind() == protoreflect.MessageKind, fd.Kind()},
			{"JSON name is ephemeralContainers", fd.JSONName() == "ephemeralContainers", fd.JSONName()},
		}
		for _, c := range checks {
			if !c.ok {
				t.Errorf("ephemeral_containers: want %s, got %v", c.what, c.got)
			}
		}
		if m := fd.Message(); m == nil || m.FullName() != "k3sm.runtime.v1.Container" {
			t.Errorf("ephemeral_containers element type = %v, want k3sm.runtime.v1.Container", m)
		}
	})

	t.Run("reserved band is exactly 104..199", func(t *testing.T) {
		t.Parallel()
		// Single-number reservations of retired fields (retiredFields) are not
		// headroom; every other reserved range must be the one 104..199 band.
		retired := map[protoreflect.FieldNumber]bool{}
		for _, rf := range retiredFields {
			if rf.message == "PodBox" {
				retired[rf.number] = true
			}
		}
		var bands [][2]protoreflect.FieldNumber
		rr := md.ReservedRanges()
		for i := 0; i < rr.Len(); i++ {
			r := rr.Get(i) // [start, end) — end is exclusive
			if r[1]-r[0] == 1 && retired[r[0]] {
				continue
			}
			bands = append(bands, r)
		}
		if len(bands) != 1 {
			t.Fatalf("PodBox has %d headroom reserved ranges (excluding retired fields), want exactly 1", len(bands))
		}
		if r := bands[0]; r[0] != 104 || r[1]-1 != 199 {
			t.Errorf("PodBox reserved range = %d..%d, want 104..199", r[0], r[1]-1)
		}
	})
}
