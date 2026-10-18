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

// TestPodStatsProcessCarve pins PodStats.process: field 100, a singular
// k3sm.runtime.v1.ProcessStats message carved from the PodStats headroom, so
// the band narrows to exactly 101..149. ProcessStats carries process_count = 1
// as a uint64 and keeps its own 100..149 headroom. The test is
// descriptor-driven so it fails at runtime, not compile time, on a tree
// without the field.
func TestPodStatsProcessCarve(t *testing.T) {
	t.Parallel()

	file := File_runtime_v1_runtime_proto
	md := file.Messages().ByName("PodStats")
	if md == nil {
		t.Fatal("message k3sm.runtime.v1.PodStats not found")
	}

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("process")
		if fd == nil {
			t.Fatal("PodStats.process is not declared")
		}
		checks := []struct {
			what string
			ok   bool
			got  any
		}{
			{"number is 100", fd.Number() == 100, fd.Number()},
			{"is singular", fd.Cardinality() == protoreflect.Optional && !fd.IsList() && !fd.IsMap(), fd.Cardinality()},
			{"is a message", fd.Kind() == protoreflect.MessageKind, fd.Kind()},
			{"JSON name is process", fd.JSONName() == "process", fd.JSONName()},
		}
		for _, c := range checks {
			if !c.ok {
				t.Errorf("process: want %s, got %v", c.what, c.got)
			}
		}
		if m := fd.Message(); m == nil || m.FullName() != "k3sm.runtime.v1.ProcessStats" {
			t.Errorf("process type = %v, want k3sm.runtime.v1.ProcessStats", m)
		}
	})

	t.Run("PodStats reserved band is exactly 101..149", func(t *testing.T) {
		t.Parallel()
		assertSingleBand(t, md, 101, 149)
	})

	t.Run("ProcessStats shape", func(t *testing.T) {
		t.Parallel()
		ps := file.Messages().ByName("ProcessStats")
		if ps == nil {
			t.Fatal("message k3sm.runtime.v1.ProcessStats not found")
		}
		if n := ps.Fields().Len(); n != 1 {
			t.Errorf("ProcessStats has %d fields, want exactly 1", n)
		}
		fd := ps.Fields().ByName("process_count")
		if fd == nil {
			t.Fatal("ProcessStats.process_count is not declared")
		}
		if fd.Number() != 1 {
			t.Errorf("process_count number = %d, want 1", fd.Number())
		}
		if fd.Kind() != protoreflect.Uint64Kind {
			t.Errorf("process_count kind = %v, want uint64", fd.Kind())
		}
		if fd.IsList() || fd.HasPresence() {
			t.Errorf("process_count must be a plain singular scalar (list=%v presence=%v)", fd.IsList(), fd.HasPresence())
		}
		if fd.JSONName() != "processCount" {
			t.Errorf("process_count JSON name = %q, want processCount", fd.JSONName())
		}
		assertSingleBand(t, ps, 100, 149)
	})
}

// assertSingleBand fails t unless md declares exactly one reserved range and
// it is first..last inclusive.
func assertSingleBand(t *testing.T, md protoreflect.MessageDescriptor, first, last protoreflect.FieldNumber) {
	t.Helper()
	rr := md.ReservedRanges()
	if rr.Len() != 1 {
		t.Fatalf("%s has %d reserved ranges, want exactly 1 (%d..%d)", md.Name(), rr.Len(), first, last)
	}
	if r := rr.Get(0); r[0] != first || r[1]-1 != last {
		t.Errorf("%s reserved range = %d..%d, want %d..%d", md.Name(), r[0], r[1]-1, first, last)
	}
}
