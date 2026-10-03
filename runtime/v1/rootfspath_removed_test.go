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

// retiredField names a field deleted from a runtime/v1 message whose number and
// name must stay reserved forever, so neither is ever reused with a different
// meaning on the wire or in JSON.
type retiredField struct {
	message  protoreflect.Name
	number   protoreflect.FieldNumber
	name     protoreflect.Name
	jsonName string
}

// retiredFields is APPEND-ONLY. Deleting a row removes the only lasting guard
// against reusing that number or name: once buf/baseline.binpb is refreshed after
// the removal, `buf breaking` no longer sees the deleted field, and with
// RESERVED_MESSAGE_NO_DELETE excepted in buf.yaml it does not flag dropping the
// reservation either. The next removal is one more row.
var retiredFields = []retiredField{
	{message: "PodBox", number: 4, name: "rootfs_path", jsonName: "rootfsPath"},
}

// TestRootfsPathRemoved asserts, over the compiled descriptor, that every retired
// field is gone under its number, its name and its JSON name, and that both its
// number and its name are reserved. The generated descriptor is proven to match
// the .proto by the ci `buf generate` no-diff stage, so no source-text matching
// is needed here.
func TestRootfsPathRemoved(t *testing.T) {
	// Pin the first row outside the table, so deleting it from retiredFields
	// alone does not leave a loop over nothing that passes vacuously.
	pinned := retiredField{message: "PodBox", number: 4, name: "rootfs_path", jsonName: "rootfsPath"}
	found := false
	for _, rf := range retiredFields {
		found = found || rf == pinned
	}
	if !found {
		t.Fatalf("retiredFields lost its %s.%s row; the table is append-only", pinned.message, pinned.name)
	}

	msgs := File_runtime_v1_runtime_proto.Messages()
	for _, rf := range retiredFields {
		t.Run(string(rf.message)+"."+string(rf.name), func(t *testing.T) {
			md := msgs.ByName(rf.message)
			if md == nil {
				t.Fatalf("message %s not found in runtime/v1/runtime.proto", rf.message)
			}
			fields := md.Fields()
			if fd := fields.ByName(rf.name); fd != nil {
				t.Errorf("%s still declares a field named %q (number %d)", rf.message, rf.name, fd.Number())
			}
			if fd := fields.ByNumber(rf.number); fd != nil {
				t.Errorf("%s reuses retired number %d as field %q", rf.message, rf.number, fd.Name())
			}
			if fd := fields.ByJSONName(rf.jsonName); fd != nil {
				t.Errorf("%s has a field with retired JSON name %q (%q)", rf.message, rf.jsonName, fd.Name())
			}
			if !md.ReservedRanges().Has(rf.number) {
				t.Errorf("%s does not reserve retired number %d", rf.message, rf.number)
			}
			if !md.ReservedNames().Has(rf.name) {
				t.Errorf("%s does not reserve retired name %q", rf.message, rf.name)
			}
		})
	}
}
