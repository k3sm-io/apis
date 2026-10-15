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
	"bytes"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// guestSpecPodFQDNNumber is the wire number GuestSpec.pod_fqdn is pinned to.
const guestSpecPodFQDNNumber protoreflect.FieldNumber = 8

// TestGuestSpecPodFQDNRoundTrip pins GuestSpec.pod_fqdn: field 8, a singular
// string with JSON name podFqdn, that round-trips through binary and proto-JSON
// (the guest-spec.json encoding), leaves a spec without it byte-identical, and
// leaves the 100..149 reserved band intact. It reaches the field through the
// descriptor and protoreflect only, never the generated accessor, so on a tree
// without the field it fails at runtime, not compile time.
func TestGuestSpecPodFQDNRoundTrip(t *testing.T) {
	t.Parallel()

	md := File_guest_v1_guest_proto.Messages().ByName("GuestSpec")
	if md == nil {
		t.Fatal("message k3sm.guest.v1.GuestSpec not found")
	}
	const fqdn = "web-0.web.default.svc.cluster.local"

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("pod_fqdn")
		if fd == nil {
			t.Fatal("GuestSpec.pod_fqdn is not declared")
		}
		checks := []struct {
			what string
			ok   bool
			got  any
		}{
			{"number is 8", fd.Number() == guestSpecPodFQDNNumber, fd.Number()},
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

	t.Run("field 8 round-trips through binary as a known field", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByNumber(guestSpecPodFQDNNumber)
		if fd == nil {
			t.Fatalf("GuestSpec declares no field number %d", guestSpecPodFQDNNumber)
		}
		raw := protowire.AppendTag(nil, guestSpecPodFQDNNumber, protowire.BytesType)
		raw = protowire.AppendString(raw, fqdn)

		var spec GuestSpec
		if err := proto.Unmarshal(raw, &spec); err != nil {
			t.Fatalf("unmarshal field %d: %v", guestSpecPodFQDNNumber, err)
		}
		if u := spec.ProtoReflect().GetUnknown(); len(u) != 0 {
			t.Errorf("field %d decoded as unknown bytes %x, want a known field", guestSpecPodFQDNNumber, u)
		}
		if got := spec.ProtoReflect().Get(fd).String(); got != fqdn {
			t.Errorf("decoded pod_fqdn = %q, want %q", got, fqdn)
		}
		out, err := proto.MarshalOptions{Deterministic: true}.Marshal(&spec)
		if err != nil {
			t.Fatalf("re-marshal: %v", err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("re-marshal = %x, want %x", out, raw)
		}
	})

	t.Run("podFqdn round-trips through proto-JSON under a strict decoder", func(t *testing.T) {
		t.Parallel()
		// The guest init's decoder posture: an unknown field is an error.
		strict := protojson.UnmarshalOptions{DiscardUnknown: false}
		in := []byte(`{"hostname":"web-0","agentPort":1024,"podFqdn":"` + fqdn + `"}`)

		var spec GuestSpec
		if err := strict.Unmarshal(in, &spec); err != nil {
			t.Fatalf("strict decode of podFqdn: %v", err)
		}
		fd := md.Fields().ByName("pod_fqdn")
		if fd == nil {
			t.Fatal("GuestSpec.pod_fqdn is not declared")
		}
		if got := spec.ProtoReflect().Get(fd).String(); got != fqdn {
			t.Fatalf("decoded pod_fqdn = %q, want %q", got, fqdn)
		}
		out, err := protojson.Marshal(&spec)
		if err != nil {
			t.Fatalf("protojson marshal: %v", err)
		}
		back := &GuestSpec{}
		if err := strict.Unmarshal(out, back); err != nil {
			t.Fatalf("strict re-decode of %s: %v", out, err)
		}
		if !proto.Equal(&spec, back) {
			t.Errorf("proto-JSON round trip changed the spec: %s", out)
		}
	})

	t.Run("a GuestSpec without pod_fqdn encodes as before", func(t *testing.T) {
		t.Parallel()
		// The bytes a host that predates the field sends, built by hand from
		// the wire format rather than by the generated code under test.
		var want []byte
		want = protowire.AppendTag(want, 1, protowire.BytesType)
		want = protowire.AppendString(want, "web-0")
		want = protowire.AppendTag(want, 7, protowire.VarintType)
		want = protowire.AppendVarint(want, 1024)

		spec := &GuestSpec{Hostname: "web-0", AgentPort: 1024}
		got, err := proto.MarshalOptions{Deterministic: true}.Marshal(spec)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("GuestSpec without pod_fqdn = %x, want %x", got, want)
		}

		// proto-JSON omits the empty value, so an older strict guest init
		// still decodes what the host sends while it sends no pod_fqdn.
		js, err := protojson.Marshal(spec)
		if err != nil {
			t.Fatalf("protojson marshal: %v", err)
		}
		if bytes.Contains(js, []byte("podFqdn")) || bytes.Contains(js, []byte("pod_fqdn")) {
			t.Errorf("proto-JSON of a spec without pod_fqdn names the field: %s", js)
		}
	})

	t.Run("the reserved band is unchanged", func(t *testing.T) {
		t.Parallel()
		rr := md.ReservedRanges()
		if rr.Has(guestSpecPodFQDNNumber) {
			t.Errorf("GuestSpec reserves number %d, which pod_fqdn uses", guestSpecPodFQDNNumber)
		}
		for n := protoreflect.FieldNumber(100); n <= 149; n++ {
			if !rr.Has(n) {
				t.Fatalf("GuestSpec no longer reserves %d; the 100..149 band must stay intact", n)
			}
		}
		if rr.Has(99) || rr.Has(150) {
			t.Errorf("GuestSpec reserved band grew past 100..149")
		}
		if rr.Len() != 1 {
			t.Errorf("GuestSpec has %d reserved ranges, want exactly the 100..149 band", rr.Len())
		}
	})
}
