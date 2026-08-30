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
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	runtimev1 "k3sm.io/apis/runtime/v1"
)

// TestGuestAgentServiceSurface pins the shape of the host↔guest contract that no
// later additive change can repair if it lands wrong:
//
//   - the service is k3sm.guest.v1.GuestAgent with EXACTLY the six verbs;
//   - each verb's streaming shape (Exec bidirectional, ContainerEvents/Logs
//     server-streaming, the rest unary) — a wrong stream shape is a wire break,
//     not a detail;
//   - Exec and Logs REUSE the k3sm.runtime.v1 stream messages rather than
//     parallel copies of them, which is the whole reason this package imports
//     runtime/v1 at all;
//   - Health carries api_version, the field that makes unsupported
//     initramfs/daemon skew a legible rejection instead of stream garbage.
func TestGuestAgentServiceSurface(t *testing.T) {
	t.Parallel()
	fd := File_guest_v1_guest_proto

	sd := fd.Services().ByName("GuestAgent")
	if sd == nil {
		t.Fatal("service GuestAgent does not exist in guest.proto")
	}

	t.Run("the service is k3sm.guest.v1.GuestAgent with exactly six verbs", func(t *testing.T) {
		if got, want := string(sd.FullName()), "k3sm.guest.v1.GuestAgent"; got != want {
			t.Errorf("service full name = %q, want %q", got, want)
		}
		want := map[string]bool{
			"Health": false, "ContainerEvents": false, "Exec": false,
			"Logs": false, "Stats": false, "Stop": false,
		}
		ms := sd.Methods()
		if ms.Len() != len(want) {
			t.Errorf("GuestAgent has %d methods, want exactly %d", ms.Len(), len(want))
		}
		for i := range ms.Len() {
			name := string(ms.Get(i).Name())
			seen, known := want[name]
			if !known {
				t.Errorf("unexpected method %s on GuestAgent (the surface is exactly six verbs)", name)
				continue
			}
			if seen {
				t.Errorf("method %s declared twice", name)
			}
			want[name] = true
		}
		for name, seen := range want {
			if !seen {
				t.Errorf("method %s is missing from GuestAgent", name)
			}
		}
	})

	t.Run("each verb has its intended streaming shape", func(t *testing.T) {
		cases := []struct {
			name                       protoreflect.Name
			clientStream, serverStream bool
		}{
			// Health is the boot-deadline probe and the live-address read: one
			// question, one answer.
			{"Health", false, false},
			// Lifecycle transitions arrive when the guest observes them.
			{"ContainerEvents", false, true},
			// Bidirectional, exactly as runtime/v1's Exec — stdin and resize flow
			// up while output flows down.
			{"Exec", true, true},
			// The client asks once; log chunks flow down (runtime/v1's GetLogs).
			{"Logs", false, true},
			// On demand, never a sampling ticker against a guest.
			{"Stats", false, false},
			{"Stop", false, false},
		}
		for _, tc := range cases {
			md := sd.Methods().ByName(tc.name)
			if md == nil {
				t.Errorf("method %s is missing", tc.name)
				continue
			}
			if md.IsStreamingClient() != tc.clientStream {
				t.Errorf("%s client-streaming = %v, want %v", tc.name, md.IsStreamingClient(), tc.clientStream)
			}
			if md.IsStreamingServer() != tc.serverStream {
				t.Errorf("%s server-streaming = %v, want %v", tc.name, md.IsStreamingServer(), tc.serverStream)
			}
		}
	})

	t.Run("Exec and Logs reuse the runtime/v1 stream messages", func(t *testing.T) {
		// A parallel guest-local copy of these messages is exactly the duplication
		// the shared-contracts module exists to prevent: two shapes for one stream
		// drift, and the runtimed side would have to translate between them on the
		// hot path of every kubectl exec.
		cases := []struct {
			method         protoreflect.Name
			input, output  protoreflect.FullName
			reflectedInput proto.Message
		}{
			{"Exec", "k3sm.runtime.v1.ExecRequest", "k3sm.runtime.v1.ExecResponse", &runtimev1.ExecRequest{}},
			{"Logs", "k3sm.runtime.v1.GetLogsRequest", "k3sm.runtime.v1.LogEntry", &runtimev1.GetLogsRequest{}},
		}
		for _, tc := range cases {
			md := sd.Methods().ByName(tc.method)
			if md == nil {
				t.Errorf("method %s is missing", tc.method)
				continue
			}
			if got := md.Input().FullName(); got != tc.input {
				t.Errorf("%s input = %s, want the reused %s", tc.method, got, tc.input)
			}
			if got := md.Output().FullName(); got != tc.output {
				t.Errorf("%s output = %s, want the reused %s", tc.method, got, tc.output)
			}
			// And the reused descriptor must be the one the runtime/v1 Go package
			// serves, not a same-named type registered from a copied file.
			if got, want := tc.reflectedInput.ProtoReflect().Descriptor().FullName(), md.Input().FullName(); got != want {
				t.Errorf("%s input descriptor identity = %s, want %s", tc.method, got, want)
			}
		}
	})

	t.Run("Health carries api_version", func(t *testing.T) {
		md := (&HealthResponse{}).ProtoReflect().Descriptor()
		fld := md.Fields().ByName("api_version")
		if fld == nil {
			t.Fatal("HealthResponse.api_version does not exist; unsupported initramfs/daemon skew would be undetectable")
		}
		if fld.Kind() != protoreflect.StringKind {
			t.Errorf("HealthResponse.api_version kind = %v, want string", fld.Kind())
		}
		// The capabilities list is what lets a genuinely additive guest feature be
		// negotiated WITHOUT an api_version bump; without it every addition would
		// have to break the version handshake.
		caps := md.Fields().ByName("capabilities")
		if caps == nil || !caps.IsList() || caps.Kind() != protoreflect.StringKind {
			t.Error("HealthResponse.capabilities must be a repeated string")
		}
	})
}

// TestGuestMessagesRoundTrip is the a2 round-trip gate: every message in the
// contract survives marshal → unmarshal with proto.Equal, so a value written by
// the host is read identically by the guest (and back).
func TestGuestMessagesRoundTrip(t *testing.T) {
	t.Parallel()
	ts := timestamppb.New(mustTime(t))

	cases := []struct {
		name string
		msg  proto.Message
	}{
		{"HealthResponse", &HealthResponse{
			Ready:             true,
			GuestIp:           "192.168.64.7",
			RosettaRegistered: true,
			ApiVersion:        "guest.v1",
			Capabilities:      []string{"idmap", "cgroup2"},
		}},
		{"ContainerEvent started", &ContainerEvent{
			Container: "app",
			Timestamp: ts,
			Started:   &ContainerStarted{Pid: 42},
		}},
		{"ContainerEvent exited oom", &ContainerEvent{
			Container: "app",
			Timestamp: ts,
			Exited:    &ContainerExited{ExitCode: 137, Signal: 9, OomKilled: true},
		}},
		{"StatsResponse", &StatsResponse{
			Timestamp: ts,
			Containers: []*GuestContainerStats{{
				Container:             "app",
				CpuUsageUsec:          123456,
				MemoryWorkingSetBytes: 78 << 20,
			}},
		}},
		{"StopRequest", &StopRequest{GraceSeconds: 30}},
		{"GuestSpec", goldenGuestSpec()},
		{"VMHostSpec", goldenVMHostSpec()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := proto.Marshal(tc.msg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			out := tc.msg.ProtoReflect().New().Interface()
			if err := proto.Unmarshal(b, out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !proto.Equal(tc.msg, out) {
				t.Errorf("round-trip differs:\n got: %v\nwant: %v", out, tc.msg)
			}
		})
	}
}

// TestGuestEventUnionDistinguishesAbsence pins the reason the started/exited
// union is optional MESSAGE fields rather than scalars: a container that exited
// 0 must be distinguishable from a container that has not exited at all, and a
// scalar exit_code on ContainerEvent could not express that difference.
func TestGuestEventUnionDistinguishesAbsence(t *testing.T) {
	t.Parallel()
	b, err := proto.Marshal(&ContainerEvent{Container: "app", Started: &ContainerStarted{Pid: 1}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ContainerEvent
	if err := proto.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.GetExited() != nil {
		t.Errorf("exited = %v on a started event, want nil", out.GetExited())
	}
	if out.GetStarted() == nil {
		t.Fatal("started did not survive the round-trip")
	}

	b, err = proto.Marshal(&ContainerEvent{Container: "app", Exited: &ContainerExited{ExitCode: 0}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out.Reset()
	if err := proto.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.GetExited() == nil {
		t.Fatal("a clean exit must still be an explicit exited event")
	}
	if out.GetExited().GetExitCode() != 0 || out.GetExited().GetOomKilled() {
		t.Errorf("exited = %+v, want a clean exit", out.GetExited())
	}
}

// TestGuestMessagesKeepTheReservedBand pins the file convention: every message
// carries the reserved 100..149 headroom, so a later additive field has a band
// to be allocated from and cannot silently take a low sequential number that a
// future ordering change would want.
func TestGuestMessagesKeepTheReservedBand(t *testing.T) {
	t.Parallel()
	// HealthRequest and the empty-by-design messages are excluded only where they
	// declare no band; every message that carries state must have one.
	banded := []proto.Message{
		&HealthResponse{}, &ContainerEventsRequest{}, &ContainerEvent{},
		&ContainerStarted{}, &ContainerExited{}, &StatsRequest{}, &StatsResponse{},
		&GuestContainerStats{}, &StopRequest{}, &StopResponse{},
		&GuestSpec{}, &ResolvConf{}, &GuestContainer{}, &GuestMount{},
		&VMHostSpec{}, &VMShare{},
	}
	for _, m := range banded {
		md := m.ProtoReflect().Descriptor()
		rr := md.ReservedRanges()
		found := false
		for i := range rr.Len() {
			r := rr.Get(i) // [start, end) — end is exclusive
			if r[0] == 100 && r[1]-1 == 149 {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not carry the file convention's `reserved 100 to 149` band", md.FullName())
		}
	}
}

// TestNoParallelPlatformOrStatsShape guards the two duplications this package is
// most likely to grow by accident: a second platform type (runtime/v1 owns the
// only one) and a stats shape that pretends guest cgroup numbers are the Darwin
// rusage numbers runtime/v1's ContainerStats carries.
func TestNoParallelPlatformOrStatsShape(t *testing.T) {
	t.Parallel()
	fd := File_guest_v1_guest_proto
	for i := range fd.Messages().Len() {
		md := fd.Messages().Get(i)
		switch md.Name() {
		case "Platform", "ContainerStats", "CPUStats", "MemoryStats":
			t.Errorf("guest/v1 declares %s, duplicating a k3sm.runtime.v1 message", md.FullName())
		}
	}
	// The guest sample is deliberately its own shape, and its field names must
	// carry the cgroup2 provenance so nobody reads them as rusage values.
	md := (&GuestContainerStats{}).ProtoReflect().Descriptor()
	for _, name := range []protoreflect.Name{"cpu_usage_usec", "memory_working_set_bytes"} {
		if md.Fields().ByName(name) == nil {
			t.Errorf("GuestContainerStats.%s is missing", name)
		}
	}
}
