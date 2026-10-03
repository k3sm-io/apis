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

package shimv1

import (
	"context"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	runtimev1 "k3sm.io/apis/runtime/v1"
)

// fakeShim is a minimal ContainerShimServer that reports APIVersion, standing in
// for what a real shim built from the same tree reports.
type fakeShim struct {
	UnimplementedContainerShimServer
}

func (fakeShim) Status(context.Context, *StatusRequest) (*StatusResponse, error) {
	return &StatusResponse{ApiVersion: APIVersion}, nil
}

// TestShimContract pins the daemon-to-shim contract's shape: the five verbs and
// their streaming shapes, the runtime/v1 message reuse by descriptor identity,
// the ExitStatus and StatusResponse field numbers and types, and APIVersion.
func TestShimContract(t *testing.T) {
	t.Parallel()
	fd := File_shim_v1_shim_proto

	if got, want := string(fd.Package()), "k3sm.shim.v1"; got != want {
		t.Fatalf("proto package = %q, want %q", got, want)
	}
	sd := fd.Services().ByName("ContainerShim")
	if sd == nil {
		t.Fatal("service ContainerShim does not exist in shim.proto")
	}

	rt := runtimev1.File_runtime_v1_runtime_proto.Messages()
	execReq := rt.ByName("ExecRequest")
	execResp := rt.ByName("ExecResponse")
	attachResp := rt.ByName("AttachResponse")
	if execReq == nil || execResp == nil || attachResp == nil {
		t.Fatal("runtime/v1 ExecRequest/ExecResponse/AttachResponse not found")
	}
	local := fd.Messages()

	methods := []struct {
		name         string
		clientStream bool
		serverStream bool
		in, out      protoreflect.MessageDescriptor
	}{
		{"Status", false, false, local.ByName("StatusRequest"), local.ByName("StatusResponse")},
		{"Exec", true, true, execReq, execResp},
		{"Follow", false, true, local.ByName("FollowRequest"), attachResp},
		{"Signal", false, false, local.ByName("SignalRequest"), local.ByName("SignalResponse")},
		{"ReopenLog", false, false, local.ByName("ReopenLogRequest"), local.ByName("ReopenLogResponse")},
	}
	if got := sd.Methods().Len(); got != len(methods) {
		t.Errorf("ContainerShim has %d methods, want exactly %d", got, len(methods))
	}
	for _, tc := range methods {
		t.Run("method "+tc.name, func(t *testing.T) {
			md := sd.Methods().ByName(protoreflect.Name(tc.name))
			if md == nil {
				t.Fatalf("method %s missing", tc.name)
			}
			if md.IsStreamingClient() != tc.clientStream || md.IsStreamingServer() != tc.serverStream {
				t.Errorf("%s streaming = client:%v server:%v, want client:%v server:%v",
					tc.name, md.IsStreamingClient(), md.IsStreamingServer(), tc.clientStream, tc.serverStream)
			}
			if tc.in == nil || tc.out == nil {
				t.Fatalf("%s: expected message descriptor not found", tc.name)
			}
			// Identity, not a copy: the reused runtime/v1 messages must be the
			// very descriptors runtime/v1 registers.
			if md.Input() != tc.in {
				t.Errorf("%s input = %s, want %s (by descriptor identity)", tc.name, md.Input().FullName(), tc.in.FullName())
			}
			if md.Output() != tc.out {
				t.Errorf("%s output = %s, want %s (by descriptor identity)", tc.name, md.Output().FullName(), tc.out.FullName())
			}
		})
	}

	type field struct {
		name    string
		number  protoreflect.FieldNumber
		kind    protoreflect.Kind
		message protoreflect.FullName // for MessageKind
	}
	messages := []struct {
		name   protoreflect.Name
		exact  bool
		fields []field
	}{
		{"ExitStatus", true, []field{
			{"exit_code", 1, protoreflect.Int32Kind, ""},
			{"signal", 2, protoreflect.Int32Kind, ""},
			{"finished_at", 3, protoreflect.MessageKind, "google.protobuf.Timestamp"},
		}},
		{"StatusResponse", true, []field{
			{"api_version", 1, protoreflect.StringKind, ""},
			{"shim_pid", 2, protoreflect.Int32Kind, ""},
			{"shim_start_unix_nano", 3, protoreflect.Int64Kind, ""},
			{"child_pid", 4, protoreflect.Int32Kind, ""},
			{"child_start_unix_nano", 5, protoreflect.Int64Kind, ""},
			{"running", 6, protoreflect.BoolKind, ""},
			{"exit", 7, protoreflect.MessageKind, "k3sm.shim.v1.ExitStatus"},
			{"dropped_bytes", 8, protoreflect.Uint64Kind, ""},
		}},
		{"StatusRequest", true, []field{{"container", 1, protoreflect.StringKind, ""}}},
		{"FollowRequest", true, []field{{"container", 1, protoreflect.StringKind, ""}}},
		{"SignalRequest", true, []field{
			{"container", 1, protoreflect.StringKind, ""},
			{"signal", 2, protoreflect.Int32Kind, ""},
			{"group", 3, protoreflect.BoolKind, ""},
		}},
		{"ReopenLogRequest", true, []field{{"container", 1, protoreflect.StringKind, ""}}},
		{"SignalResponse", true, nil},
		{"ReopenLogResponse", true, nil},
	}
	for _, tc := range messages {
		t.Run("message "+string(tc.name), func(t *testing.T) {
			md := local.ByName(tc.name)
			if md == nil {
				t.Fatalf("message %s missing", tc.name)
			}
			if tc.exact && md.Fields().Len() != len(tc.fields) {
				t.Errorf("%s has %d fields, want exactly %d", tc.name, md.Fields().Len(), len(tc.fields))
			}
			for _, f := range tc.fields {
				fld := md.Fields().ByName(protoreflect.Name(f.name))
				if fld == nil {
					t.Errorf("%s.%s missing", tc.name, f.name)
					continue
				}
				if fld.Number() != f.number {
					t.Errorf("%s.%s number = %d, want %d", tc.name, f.name, fld.Number(), f.number)
				}
				if fld.Kind() != f.kind {
					t.Errorf("%s.%s kind = %v, want %v", tc.name, f.name, fld.Kind(), f.kind)
				}
				if fld.Cardinality() == protoreflect.Repeated {
					t.Errorf("%s.%s is repeated, want singular", tc.name, f.name)
				}
				if f.kind == protoreflect.MessageKind && fld.Message().FullName() != f.message {
					t.Errorf("%s.%s type = %s, want %s", tc.name, f.name, fld.Message().FullName(), f.message)
				}
			}
		})
	}

	t.Run("APIVersion is non-empty and is what a shim reports", func(t *testing.T) {
		if APIVersion == "" {
			t.Fatal("APIVersion is empty; skew would be illegible")
		}
		resp, err := fakeShim{}.Status(t.Context(), &StatusRequest{})
		if err != nil {
			t.Fatalf("fake Status: %v", err)
		}
		if resp.GetApiVersion() != APIVersion {
			t.Errorf("reported api_version = %q, want %q", resp.GetApiVersion(), APIVersion)
		}
	})
}
