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

// bandRange returns the reserved range of md that ends at or above want, so a
// band assertion reads the range that actually carries the ceiling rather than
// whichever range happens to be first.
func bandRange(t *testing.T, md protoreflect.MessageDescriptor, want protoreflect.FieldNumber) (protoreflect.FieldNumber, protoreflect.FieldNumber) {
	t.Helper()
	rr := md.ReservedRanges()
	for i := range rr.Len() {
		r := rr.Get(i) // [start, end) — end is exclusive
		if r[1]-1 >= want {
			return r[0], r[1] - 1
		}
	}
	t.Fatalf("%s has no reserved range reaching %d; the band ceiling was dropped", md.FullName(), want)
	return 0, 0
}

// TestSandboxProfileGPUEgressCarve pins the M8.1 SandboxProfile carve: allow_gpu
// at 102 and allow_internet_egress at 103, taken from the documented 100..149
// headroom.
//
// The numbers are the contract. Both fields are bare booleans, so a renumber is
// not a compile error anywhere — an old daemon would simply read one widening as
// the other, granting GPU access to a pod that asked for internet or vice versa.
// That failure is silent on the wire and invisible to a functional test, which
// is why the descriptor is asserted directly.
func TestSandboxProfileGPUEgressCarve(t *testing.T) {
	t.Parallel()
	md := (&SandboxProfile{}).ProtoReflect().Descriptor()

	t.Run("the fields exist at the pinned numbers as booleans", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			num  protoreflect.FieldNumber
		}{
			{"allow_gpu", 102},
			{"allow_internet_egress", 103},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				fd := md.Fields().ByName(protoreflect.Name(tc.name))
				if fd == nil {
					t.Fatalf("SandboxProfile.%s does not exist", tc.name)
				}
				if fd.Number() != tc.num {
					t.Errorf("SandboxProfile.%s = field %d, want the pinned %d", tc.name, fd.Number(), tc.num)
				}
				if fd.Kind() != protoreflect.BoolKind {
					t.Errorf("SandboxProfile.%s kind = %v, want bool", tc.name, fd.Kind())
				}
			})
		}
	})

	t.Run("the reserved band is re-narrowed and keeps its ceiling", func(t *testing.T) {
		t.Parallel()
		lo, hi := bandRange(t, md, 104)
		if lo != 104 || hi != 149 {
			t.Errorf("SandboxProfile reserved range = %d..%d, want 104..149", lo, hi)
		}
		for _, n := range []protoreflect.FieldNumber{100, 101, 102, 103} {
			if md.ReservedRanges().Has(n) {
				t.Errorf("SandboxProfile field %d is both allocated and reserved", n)
			}
		}
	})

	t.Run("both widenings round-trip and default false", func(t *testing.T) {
		t.Parallel()
		// Default-false is the whole reason this carve needs no phased
		// provider<->runtimed rollout: a provider that predates the fields sends
		// nothing, and a daemon must read that as "not granted", never as
		// "unspecified, pick a default".
		var zero SandboxProfile
		if zero.GetAllowGpu() || zero.GetAllowInternetEgress() {
			t.Fatalf("zero SandboxProfile grants a widening: gpu=%v egress=%v", zero.GetAllowGpu(), zero.GetAllowInternetEgress())
		}

		cases := []struct {
			name        string
			gpu, egress bool
		}{
			{"neither", false, false},
			{"gpu only", true, false},
			{"egress only", false, true},
			{"both", true, true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				in := &SandboxProfile{
					Backend:             SandboxBackend_SANDBOX_BACKEND_SEATBELT_EXEC,
					DataVolumePath:      "/var/lib/k3sm/pods/pod-1/rootfs",
					AllowNetwork:        true,
					AllowGpu:            tc.gpu,
					AllowInternetEgress: tc.egress,
				}
				b, err := proto.Marshal(in)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var out SandboxProfile
				if err := proto.Unmarshal(b, &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !proto.Equal(in, &out) {
					t.Fatalf("round-trip differs:\n in = %v\nout = %v", in, &out)
				}
				if out.GetAllowGpu() != tc.gpu || out.GetAllowInternetEgress() != tc.egress {
					t.Errorf("gpu=%v egress=%v, want gpu=%v egress=%v", out.GetAllowGpu(), out.GetAllowInternetEgress(), tc.gpu, tc.egress)
				}
			})
		}
	})

	t.Run("the two widenings stay distinct fields", func(t *testing.T) {
		t.Parallel()
		// Granting GPU must not grant internet, and vice versa: they are separate
		// admission decisions, and a single combined "allow_extra" would make one
		// of them un-refusable.
		onlyGPU := &SandboxProfile{AllowGpu: true}
		b, err := proto.Marshal(onlyGPU)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out SandboxProfile
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.GetAllowInternetEgress() {
			t.Error("setting allow_gpu also set allow_internet_egress")
		}
	})
}

// TestInternetEgressCeilingDocumented asserts the .proto states the enforcement
// ceiling of allow_internet_egress in plain words.
//
// This is not a style check. The field's honest status is that it is an
// API/admission contract, not a packet-level boundary — macOS 26 cannot express
// per-IP scoping in a Seatbelt profile, so the emitted stanza is the same
// unfiltered-but-compilable one allow_network emits. A consumer that reads the
// field name alone will conclude the opposite, and would build isolation on a
// guarantee the runtime does not make. The doc comment is the only place that
// correction can live, so the gate pins it there.
func TestInternetEgressCeilingDocumented(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("runtime.proto")
	if err != nil {
		t.Fatalf("read runtime.proto: %v", err)
	}
	proto := string(src)

	// The load-bearing clauses, each checked separately so a partial rewrite that
	// drops the honesty while keeping the shape fails here.
	for _, want := range []string{
		"same unfiltered-but-compilable network stanza as allow_network",
		"macOS 26 cannot express per-IP scoping",
		"it is the API/admission contract",
		"network-layer (packet-filter) enforcement is tracked as future work",
	} {
		if !strings.Contains(proto, want) {
			t.Errorf("runtime.proto does not document the egress ceiling clause %q", want)
		}
	}

	// And the same correction must reach the Go-side operator spelling, since a
	// provider author reads the annotation constant and never the .proto. The
	// comparison is over whitespace-collapsed text so a re-wrap of the comment is
	// not a failure — only losing the statement is.
	labels, err := os.ReadFile("labels.go")
	if err != nil {
		t.Fatalf("read labels.go: %v", err)
	}
	flat := strings.Join(strings.Fields(strings.ReplaceAll(string(labels), "//", " ")), " ")
	for _, want := range []string{
		"same unfiltered-but-compilable network stanza as allow_network",
		"network-layer (packet-filter) enforcement is tracked as future work",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("AnnotationInternetEgress does not carry the enforcement-ceiling clause %q", want)
		}
	}
}

// TestGPUFactsCarve pins the M8.1 GetRuntimeInfoResponse carve: the GPUFacts
// message at field 100, with 101 RESERVED rather than merely earmarked.
//
// Reserving 101 is the point of the carve's shape. The band comment has
// earmarked it for backend capabilities since M2, but an earmark is a comment: a
// later carve reading only the free numbers would take it, and the eventual
// backend-capabilities message would then have to land somewhere else, silently
// breaking the one place the number was promised.
func TestGPUFactsCarve(t *testing.T) {
	t.Parallel()
	md := (&GetRuntimeInfoResponse{}).ProtoReflect().Descriptor()

	t.Run("gpu is the GPUFacts message at 100", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("gpu")
		if fd == nil {
			t.Fatal("GetRuntimeInfoResponse.gpu does not exist; node GPU advertisement is specified against it")
		}
		if fd.Number() != 100 {
			t.Errorf("gpu = field %d, want 100", fd.Number())
		}
		if fd.Kind() != protoreflect.MessageKind {
			t.Fatalf("gpu kind = %v, want a message", fd.Kind())
		}
		if got, want := string(fd.Message().FullName()), "k3sm.runtime.v1.GPUFacts"; got != want {
			t.Errorf("gpu message = %s, want %s", got, want)
		}
	})

	t.Run("101 stays reserved and the band keeps its ceiling", func(t *testing.T) {
		t.Parallel()
		lo, hi := bandRange(t, md, 101)
		if lo != 101 || hi != 149 {
			t.Errorf("GetRuntimeInfoResponse reserved range = %d..%d, want 101..149", lo, hi)
		}
		if md.ReservedRanges().Has(100) {
			t.Error("field 100 is both allocated and reserved")
		}
		if !md.ReservedRanges().Has(101) {
			t.Error("field 101 is not reserved; the backend-capabilities earmark has no teeth")
		}
	})

	t.Run("GPUFacts carries the specified facts with the specified kinds", func(t *testing.T) {
		t.Parallel()
		gmd := (&GPUFacts{}).ProtoReflect().Descriptor()
		cases := []struct {
			name string
			num  protoreflect.FieldNumber
			kind protoreflect.Kind
		}{
			{"metal_available", 1, protoreflect.BoolKind},
			{"chip_brand", 2, protoreflect.StringKind},
			{"chip_family", 3, protoreflect.StringKind},
			{"mem_bytes", 4, protoreflect.Uint64Kind},
			{"iogpu_wired_limit_bytes", 5, protoreflect.Uint64Kind},
			{"recommended_max_working_set_bytes", 6, protoreflect.Uint64Kind},
			{"sandbox_gpu_supported", 7, protoreflect.BoolKind},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				fd := gmd.Fields().ByName(protoreflect.Name(tc.name))
				if fd == nil {
					t.Fatalf("GPUFacts.%s does not exist", tc.name)
				}
				if fd.Number() != tc.num {
					t.Errorf("GPUFacts.%s = field %d, want %d", tc.name, fd.Number(), tc.num)
				}
				if fd.Kind() != tc.kind {
					t.Errorf("GPUFacts.%s kind = %v, want %v", tc.name, fd.Kind(), tc.kind)
				}
			})
		}
	})

	t.Run("facts round-trip and absent gpu stays absent", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			in   *GetRuntimeInfoResponse
		}{
			{
				"a Metal host with an explicit iogpu limit",
				&GetRuntimeInfoResponse{
					RuntimeName: "k3sm-runtimed", RuntimeVersion: "v0.1.0", ApiVersion: "runtime.v1", Healthy: true,
					Gpu: &GPUFacts{
						MetalAvailable:                true,
						ChipBrand:                     "Apple M4 Max",
						ChipFamily:                    "M4",
						MemBytes:                      137438953472,
						IogpuWiredLimitBytes:          103079215104,
						RecommendedMaxWorkingSetBytes: 115964116992,
						SandboxGpuSupported:           true,
					},
				},
			},
			{
				// The 0 sentinel is a MODELLED value, not a missing one: the daemon
				// determined the host carries no override and the kernel default
				// applies. It must survive the wire exactly as the sibling facts do,
				// because a consumer that re-reads it as "unknown" would size a model
				// against unbounded headroom.
				"a Metal host at the kernel-default iogpu limit (the 0 sentinel)",
				&GetRuntimeInfoResponse{
					RuntimeName: "k3sm-runtimed", Healthy: true,
					Gpu: &GPUFacts{
						MetalAvailable:                true,
						ChipBrand:                     "Apple M2",
						ChipFamily:                    "M2",
						MemBytes:                      17179869184,
						IogpuWiredLimitBytes:          0,
						RecommendedMaxWorkingSetBytes: 11453246668,
						SandboxGpuSupported:           false,
					},
				},
			},
			{
				// Known-absent GPU: the message is PRESENT and says so.
				"a host with no usable Metal device",
				&GetRuntimeInfoResponse{RuntimeName: "k3sm-runtimed", Gpu: &GPUFacts{MetalAvailable: false}},
			},
			{
				// Unknown: a daemon that predates the field reports nothing at all.
				"a daemon that reports no GPU facts",
				&GetRuntimeInfoResponse{RuntimeName: "k3sm-runtimed"},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				b, err := proto.Marshal(tc.in)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var out GetRuntimeInfoResponse
				if err := proto.Unmarshal(b, &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !proto.Equal(tc.in, &out) {
					t.Fatalf("round-trip differs:\n in = %v\nout = %v", tc.in, &out)
				}
				if (tc.in.GetGpu() == nil) != (out.GetGpu() == nil) {
					t.Fatalf("gpu presence changed across the wire: in=%v out=%v", tc.in.GetGpu() != nil, out.GetGpu() != nil)
				}
			})
		}
	})

	t.Run("unknown and known-absent are distinguishable", func(t *testing.T) {
		t.Parallel()
		// The distinction only survives because gpu is a MESSAGE. If it were
		// flattened into scalars on the response, "no daemon support" and "no GPU"
		// would be the same zero bytes, and the node advertiser would have to
		// guess — advertising mlx.k3sm.io/gpu on nodes that have none, or on none
		// at all.
		unknown := &GetRuntimeInfoResponse{RuntimeName: "old"}
		absent := &GetRuntimeInfoResponse{RuntimeName: "new", Gpu: &GPUFacts{MetalAvailable: false}}
		bu, err := proto.Marshal(unknown)
		if err != nil {
			t.Fatalf("marshal unknown: %v", err)
		}
		ba, err := proto.Marshal(absent)
		if err != nil {
			t.Fatalf("marshal absent: %v", err)
		}
		var gu, ga GetRuntimeInfoResponse
		if err := proto.Unmarshal(bu, &gu); err != nil {
			t.Fatalf("unmarshal unknown: %v", err)
		}
		if err := proto.Unmarshal(ba, &ga); err != nil {
			t.Fatalf("unmarshal absent: %v", err)
		}
		if gu.GetGpu() != nil {
			t.Error("a daemon reporting no GPU facts decoded as a present GPUFacts")
		}
		if ga.GetGpu() == nil {
			t.Fatal("a daemon reporting metal_available=false decoded as absent facts")
		}
		if ga.GetGpu().GetMetalAvailable() {
			t.Error("known-absent decoded as metal_available=true")
		}
	})
}
