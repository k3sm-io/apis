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

// TestStopContainerContract pins the StopContainer wire contract: a unary RPC
// on the Runtime service, the request and response field numbers, the 100-149
// headroom on both messages, and the FAILURE_REASON_UNSUPPORTED value the vm
// backend returns for a per-container verb it does not implement.
func TestStopContainerContract(t *testing.T) {
	t.Parallel()

	type field struct {
		name   protoreflect.Name
		number protoreflect.FieldNumber
	}
	messages := []struct {
		name   string
		md     protoreflect.MessageDescriptor
		fields []field
	}{
		{
			name: "StopContainerRequest",
			md:   (&StopContainerRequest{}).ProtoReflect().Descriptor(),
			fields: []field{
				{"pod_id", 1},
				{"container", 2},
				{"grace_period_seconds", 3},
				{"reason", 4},
			},
		},
		{
			name: "StopContainerResponse",
			md:   (&StopContainerResponse{}).ProtoReflect().Descriptor(),
			fields: []field{
				{"status", 1},
				{"error", 2},
				{"failure_reason", 3},
			},
		},
	}

	t.Run("StopContainer is a registered unary RPC", func(t *testing.T) {
		t.Parallel()
		found := false
		for _, m := range Runtime_ServiceDesc.Methods {
			if m.MethodName == "StopContainer" {
				found = true
			}
		}
		if !found {
			t.Error(`RPC "StopContainer" is not registered as a unary method on Runtime_ServiceDesc`)
		}
		for _, s := range Runtime_ServiceDesc.Streams {
			if s.StreamName == "StopContainer" {
				t.Error(`RPC "StopContainer" must be unary, but is registered as a stream`)
			}
		}
	})

	for _, msg := range messages {
		t.Run(msg.name+" fields", func(t *testing.T) {
			t.Parallel()
			if got, want := msg.md.Fields().Len(), len(msg.fields); got != want {
				t.Errorf("%s has %d fields, want %d", msg.name, got, want)
			}
			for _, f := range msg.fields {
				fd := msg.md.Fields().ByName(f.name)
				if fd == nil {
					t.Errorf("%s has no field %s", msg.name, f.name)
					continue
				}
				if got := fd.Number(); got != f.number {
					t.Errorf("%s.%s field number = %d, want %d", msg.name, f.name, got, f.number)
				}
			}
		})
		t.Run(msg.name+" reserves 100-149", func(t *testing.T) {
			t.Parallel()
			rr := msg.md.ReservedRanges()
			for _, n := range []protoreflect.FieldNumber{100, 125, 149} {
				if !rr.Has(n) {
					t.Errorf("%s does not reserve field number %d", msg.name, n)
				}
			}
			for _, n := range []protoreflect.FieldNumber{99, 150} {
				if rr.Has(n) {
					t.Errorf("%s reserves field number %d, want only 100-149", msg.name, n)
				}
			}
		})
	}

	t.Run("FAILURE_REASON_UNSUPPORTED exists", func(t *testing.T) {
		t.Parallel()
		ev := FailureReason(0).Descriptor().Values().ByName("FAILURE_REASON_UNSUPPORTED")
		if ev == nil {
			t.Fatal("FailureReason has no FAILURE_REASON_UNSUPPORTED value")
		}
		if got := ev.Number(); got != 17 {
			t.Errorf("FAILURE_REASON_UNSUPPORTED = %d, want 17", got)
		}
		if got := FailureReason_FAILURE_REASON_UNSUPPORTED; got != 17 {
			t.Errorf("FailureReason_FAILURE_REASON_UNSUPPORTED = %d, want 17", got)
		}
	})
}
