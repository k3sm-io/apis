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
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestPodStatusGuestTransportAddressCarve pins the vm-pod live-transport-address
// carve: guest_transport_address at 100, taken from PodStatus's documented
// 100..149 headroom.
//
// The number and the type are the contract. The field sits beside pod_ip, is the
// same kind (string), and holds a value of the same syntactic shape — so a
// renumber or a retype is a compile error nowhere and a wire error nobody sees:
// a peer would read a node-local NAT lease as the pod's cluster-routable
// identity, which is precisely the confusion the two-address model exists to
// prevent. The descriptor is therefore asserted directly.
func TestPodStatusGuestTransportAddressCarve(t *testing.T) {
	t.Parallel()
	md := (&PodStatus{}).ProtoReflect().Descriptor()

	t.Run("the field exists at the pinned number as a string", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("guest_transport_address")
		if fd == nil {
			t.Fatal("PodStatus.guest_transport_address does not exist; the host-side dial path is specified against it")
		}
		if fd.Number() != 100 {
			t.Errorf("PodStatus.guest_transport_address = field %d, want the pinned 100", fd.Number())
		}
		if fd.Kind() != protoreflect.StringKind {
			t.Errorf("PodStatus.guest_transport_address kind = %v, want string", fd.Kind())
		}
		if fd.Cardinality() == protoreflect.Repeated {
			t.Error("PodStatus.guest_transport_address is repeated; a guest holds exactly one lease at a time")
		}
	})

	t.Run("the reserved band is re-narrowed and keeps its ceiling", func(t *testing.T) {
		t.Parallel()
		// The band was `reserved 100 to 149`. Claiming 100 must leave exactly
		// `reserved 101 to 149`: 100 can no longer be reserved, and the 149 ceiling
		// must survive so the remaining headroom is not quietly eaten.
		lo, hi := bandRange(t, md, 101)
		if lo != 101 || hi != 149 {
			t.Errorf("PodStatus reserved range = %d..%d, want 101..149", lo, hi)
		}
		if md.ReservedRanges().Has(100) {
			t.Error("PodStatus field 100 is both allocated and reserved")
		}
	})

	t.Run("the transport address is distinct from the published identity", func(t *testing.T) {
		t.Parallel()
		// Two addresses, two fields, no aliasing: setting the transport address
		// must leave pod_ip/pod_ips untouched, and vice versa. If either one ever
		// populated the other, "do not publish the lease" would become
		// unenforceable at the consumer, because the consumer would never see the
		// difference.
		in := &PodStatus{
			PodId:                 "pod-1",
			PodIp:                 "10.42.0.7",
			PodIps:                []string{"10.42.0.7"},
			GuestTransportAddress: "192.0.2.15",
		}
		b, err := proto.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out PodStatus
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !proto.Equal(in, &out) {
			t.Fatalf("round-trip differs:\n in = %v\nout = %v", in, &out)
		}
		if got := out.GetGuestTransportAddress(); got != "192.0.2.15" {
			t.Errorf("guest_transport_address = %q, want the leased address", got)
		}
		if got := out.GetPodIp(); got != "10.42.0.7" {
			t.Errorf("pod_ip = %q, want the podCIDR identity unchanged", got)
		}

		onlyTransport := &PodStatus{PodId: "pod-2", GuestTransportAddress: "192.0.2.15"}
		b, err = proto.Marshal(onlyTransport)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out.Reset()
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.GetPodIp() != "" || len(out.GetPodIps()) != 0 {
			t.Errorf("setting guest_transport_address also set pod_ip=%q pod_ips=%v", out.GetPodIp(), out.GetPodIps())
		}
	})

	t.Run("empty is the truthful reading for a pod with no lease", func(t *testing.T) {
		t.Parallel()
		// A host-process pod never has one, and a vm pod does not have one until
		// its guest's DHCP client answers. Both are the same absence, and both must
		// read as "there is nothing to dial yet" — never as a usable address.
		var zero PodStatus
		if got := zero.GetGuestTransportAddress(); got != "" {
			t.Errorf("zero PodStatus guest_transport_address = %q, want empty", got)
		}
		hostProcess := &PodStatus{PodId: "pod-3", Phase: PodPhase_POD_PHASE_RUNNING, PodIp: "10.42.0.9"}
		b, err := proto.Marshal(hostProcess)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out PodStatus
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got := out.GetGuestTransportAddress(); got != "" {
			t.Errorf("host-process PodStatus guest_transport_address = %q, want empty", got)
		}
	})
}

// TestGuestTransportAddressContractDocumented pins the three clauses that make
// the field safe to consume, at the place a consumer actually reads.
//
// The field is a bare string next to pod_ip. Nothing in its shape says it is
// node-local, nothing says it expires, and nothing stops a provider from putting
// it in an EndpointSlice — the comment is the ONLY carrier of all three, so a
// rewrite that keeps the field and drops the prose silently converts a host-side
// dial hint into a published, stale, cluster-advertised address. Asserted over
// both the .proto and the generated .pb.go, because a Go consumer reads the
// struct field's comment and never opens the .proto. Whitespace-collapsed, so a
// re-wrap is fine and only losing the statement fails.
func TestGuestTransportAddressContractDocumented(t *testing.T) {
	t.Parallel()

	clauses := []struct {
		want, why string
	}{
		{
			"Empty for every host-process pod",
			"absence is the normal state, not an error to be worked around",
		},
		{
			"has not taken a lease yet",
			"a vm pod is address-less between boot and lease",
		},
		{
			"MUST NOT publish it into EndpointSlice, DNS, status.podIP",
			"the published identity stays pod_ip(s); this is host-side only",
		},
		{
			"MAY CHANGE across a guest restart",
			"it is a DHCP lease, so it is not a stable identifier",
		},
		{
			"only as durable as the status stream that delivered it",
			"a consumer must not cache it past the stream that carried it",
		},
	}

	files := []struct {
		path      string
		fieldDecl string
	}{
		{"runtime.proto", "string guest_transport_address = 100;"},
		{"runtime.pb.go", "GuestTransportAddress string `protobuf:\"bytes,100,"},
	}

	for _, f := range files {
		t.Run(f.path, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("read %s: %v", f.path, err)
			}
			block, ok := commentBlockAbove(string(raw), f.fieldDecl)
			if !ok {
				t.Fatalf("%s: no declaration matching %q; the gate cannot locate the transport-address contract", f.path, f.fieldDecl)
			}
			hay := strings.ToLower(flattenComments(block))
			for _, c := range clauses {
				if !strings.Contains(hay, strings.ToLower(c.want)) {
					t.Errorf("%s: guest_transport_address comment does not state %q (%s)\ncomment block:\n%s", f.path, c.want, c.why, block)
				}
			}
		})
	}
}
