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
	"bytes"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// podBoxPodFQDNNumber is the wire number PodBox.pod_fqdn is pinned to.
const podBoxPodFQDNNumber protoreflect.FieldNumber = 21

// TestPodBoxPodFQDNField pins PodBox.pod_fqdn: field 21, a singular string with
// JSON name podFqdn, carried on the wire as a known field and through
// proto-JSON. It reaches the field through the descriptor and protoreflect
// only, never the generated accessor, so on a tree without the field it fails
// at runtime, not compile time.
func TestPodBoxPodFQDNField(t *testing.T) {
	t.Parallel()

	md := File_runtime_v1_runtime_proto.Messages().ByName("PodBox")
	if md == nil {
		t.Fatal("message k3sm.runtime.v1.PodBox not found")
	}
	const fqdn = "web-0.web.default.svc.cluster.local"

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("pod_fqdn")
		if fd == nil {
			t.Fatal("PodBox.pod_fqdn is not declared")
		}
		checks := []struct {
			what string
			ok   bool
			got  any
		}{
			{"number is 21", fd.Number() == podBoxPodFQDNNumber, fd.Number()},
			{"kind is string", fd.Kind() == protoreflect.StringKind, fd.Kind()},
			{"is singular", !fd.IsList() && !fd.IsMap(), fd.Cardinality()},
			{"JSON name is podFqdn", fd.JSONName() == "podFqdn", fd.JSONName()},
		}
		for _, c := range checks {
			if !c.ok {
				t.Errorf("pod_fqdn: want %s, got %v", c.what, c.got)
			}
		}
	})

	t.Run("field 21 round-trips as a known field", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByNumber(podBoxPodFQDNNumber)
		if fd == nil {
			t.Fatalf("PodBox declares no field number %d", podBoxPodFQDNNumber)
		}
		raw := protowire.AppendTag(nil, podBoxPodFQDNNumber, protowire.BytesType)
		raw = protowire.AppendString(raw, fqdn)

		var pb PodBox
		if err := proto.Unmarshal(raw, &pb); err != nil {
			t.Fatalf("unmarshal field %d: %v", podBoxPodFQDNNumber, err)
		}
		if u := pb.ProtoReflect().GetUnknown(); len(u) != 0 {
			t.Errorf("field %d decoded as unknown bytes %x, want a known field", podBoxPodFQDNNumber, u)
		}
		if got := pb.ProtoReflect().Get(fd).String(); got != fqdn {
			t.Errorf("decoded pod_fqdn = %q, want %q", got, fqdn)
		}
		out, err := proto.MarshalOptions{Deterministic: true}.Marshal(&pb)
		if err != nil {
			t.Fatalf("re-marshal: %v", err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("re-marshal = %x, want %x", out, raw)
		}
	})

	t.Run("podFqdn round-trips through proto-JSON", func(t *testing.T) {
		t.Parallel()
		in := []byte(`{"podId":"uid-1","hostname":"web-0","podFqdn":"` + fqdn + `"}`)
		var pb PodBox
		if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(in, &pb); err != nil {
			t.Fatalf("strict decode of podFqdn: %v", err)
		}
		fd := md.Fields().ByName("pod_fqdn")
		if fd == nil {
			t.Fatal("PodBox.pod_fqdn is not declared")
		}
		if got := pb.ProtoReflect().Get(fd).String(); got != fqdn {
			t.Errorf("decoded pod_fqdn = %q, want %q", got, fqdn)
		}
	})

	t.Run("a PodBox without pod_fqdn encodes as before", func(t *testing.T) {
		t.Parallel()
		// The bytes a producer that predates the field sends, built by hand
		// from the wire format rather than by the generated code under test.
		var want []byte
		want = protowire.AppendTag(want, 1, protowire.BytesType)
		want = protowire.AppendString(want, "uid-1")
		want = protowire.AppendTag(want, 3, protowire.BytesType)
		want = protowire.AppendString(want, "web-0")
		want = protowire.AppendTag(want, 20, protowire.BytesType)
		want = protowire.AppendString(want, "web-0")

		pb := &PodBox{PodId: "uid-1", Name: "web-0", Hostname: "web-0"}
		got, err := proto.MarshalOptions{Deterministic: true}.Marshal(pb)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("PodBox without pod_fqdn = %x, want %x", got, want)
		}
	})

	t.Run("the reserved band is unchanged", func(t *testing.T) {
		t.Parallel()
		rr := md.ReservedRanges()
		if rr.Has(podBoxPodFQDNNumber) {
			t.Errorf("PodBox reserves number %d, which pod_fqdn uses", podBoxPodFQDNNumber)
		}
		assertPodBoxHeadroomBand(t, md)
	})
}
