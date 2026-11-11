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

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestImagesEvictImageSurface pins the EvictImage wire: one additive, unary,
// non-deprecated method on Images, its request, response and kept-blob
// messages with their exact field names, kinds and numbers, the file's
// reserved 100..149 band on each, a dedicated EvictSkipReason enum whose
// values all carry the EVICT_SKIP_REASON_ prefix, and PruneSkipReason left
// exactly as it was. Every one of these is a one-way door once released.
func TestImagesEvictImageSurface(t *testing.T) {
	fd := File_runtime_v1_images_proto

	t.Run("EvictImage is a unary, non-deprecated Images method with its own types", func(t *testing.T) {
		sd := fd.Services().ByName("Images")
		if sd == nil {
			t.Fatal("service Images does not exist")
		}
		md := sd.Methods().ByName("EvictImage")
		if md == nil {
			t.Fatal("method EvictImage is missing from Images")
		}
		if md.IsStreamingClient() || md.IsStreamingServer() {
			t.Errorf("EvictImage streams (client=%v server=%v), want unary", md.IsStreamingClient(), md.IsStreamingServer())
		}
		if opts, _ := md.Options().(*descriptorpb.MethodOptions); opts.GetDeprecated() {
			t.Error("EvictImage is deprecated, want not deprecated")
		}
		if got, want := string(md.Input().FullName()), "k3sm.runtime.v1.EvictImageRequest"; got != want {
			t.Errorf("EvictImage request = %s, want %s", got, want)
		}
		if got, want := string(md.Output().FullName()), "k3sm.runtime.v1.EvictImageResponse"; got != want {
			t.Errorf("EvictImage response = %s, want %s", got, want)
		}
		// Declared right after PruneImages.
		prune := sd.Methods().ByName("PruneImages")
		if prune == nil || md.Index() != prune.Index()+1 {
			t.Errorf("EvictImage is not declared immediately after PruneImages")
		}
	})

	type field struct {
		name     protoreflect.Name
		number   protoreflect.FieldNumber
		kind     protoreflect.Kind
		repeated bool
		typeName string // message or enum full name, when kind needs one
	}
	msgs := []struct {
		name   protoreflect.Name
		fields []field
	}{
		{"EvictImageRequest", []field{
			{"reference", 1, protoreflect.StringKind, false, ""},
			{"platform", 2, protoreflect.MessageKind, false, "k3sm.runtime.v1.Platform"},
			{"digest", 3, protoreflect.StringKind, false, ""},
			{"force", 4, protoreflect.BoolKind, false, ""},
		}},
		{"EvictImageResponse", []field{
			{"manifest_digest", 1, protoreflect.StringKind, false, ""},
			{"removed_digests", 2, protoreflect.StringKind, true, ""},
			{"kept", 3, protoreflect.MessageKind, true, "k3sm.runtime.v1.EvictKeptBlob"},
			{"reclaimed_bytes", 4, protoreflect.Uint64Kind, false, ""},
		}},
		{"EvictKeptBlob", []field{
			{"digest", 1, protoreflect.StringKind, false, ""},
			{"reason", 2, protoreflect.EnumKind, false, "k3sm.runtime.v1.EvictSkipReason"},
		}},
	}
	for _, m := range msgs {
		t.Run(string(m.name)+" fields and reserved band", func(t *testing.T) {
			md := fd.Messages().ByName(m.name)
			if md == nil {
				t.Fatalf("message %s is missing from images.proto", m.name)
			}
			if got := md.Fields().Len(); got != len(m.fields) {
				t.Errorf("%s has %d fields, want exactly %d", m.name, got, len(m.fields))
			}
			for _, w := range m.fields {
				f := md.Fields().ByName(w.name)
				if f == nil {
					t.Errorf("%s.%s is missing", m.name, w.name)
					continue
				}
				if f.Number() != w.number {
					t.Errorf("%s.%s = %d, want %d", m.name, w.name, f.Number(), w.number)
				}
				if f.Kind() != w.kind {
					t.Errorf("%s.%s kind = %v, want %v", m.name, w.name, f.Kind(), w.kind)
				}
				if f.IsList() != w.repeated {
					t.Errorf("%s.%s repeated = %v, want %v", m.name, w.name, f.IsList(), w.repeated)
				}
				var tn string
				switch w.kind {
				case protoreflect.MessageKind:
					tn = string(f.Message().FullName())
				case protoreflect.EnumKind:
					tn = string(f.Enum().FullName())
				}
				if tn != w.typeName {
					t.Errorf("%s.%s type = %q, want %q", m.name, w.name, tn, w.typeName)
				}
			}
			rr := md.ReservedRanges()
			found := false
			for i := range rr.Len() {
				if r := rr.Get(i); r[0] == 100 && r[1]-1 == 149 {
					found = true
				}
			}
			if !found {
				t.Errorf("%s does not declare `reserved 100 to 149`", m.name)
			}
		})
	}

	t.Run("EvictSkipReason is dedicated, prefixed and exact", func(t *testing.T) {
		ed := fd.Enums().ByName("EvictSkipReason")
		if ed == nil {
			t.Fatal("enum EvictSkipReason is missing from images.proto")
		}
		want := []struct {
			name   protoreflect.Name
			number protoreflect.EnumNumber
		}{
			{"EVICT_SKIP_REASON_UNSPECIFIED", 0},
			{"EVICT_SKIP_REASON_ROOTED", 1},
			{"EVICT_SKIP_REASON_IN_USE", 2},
			{"EVICT_SKIP_REASON_LEASED", 3},
			{"EVICT_SKIP_REASON_SHARED", 4},
			{"EVICT_SKIP_REASON_YOUNG", 5},
		}
		vs := ed.Values()
		if vs.Len() != len(want) {
			t.Errorf("EvictSkipReason has %d values, want exactly %d", vs.Len(), len(want))
		}
		for i := range vs.Len() {
			if n := string(vs.Get(i).Name()); !strings.HasPrefix(n, "EVICT_SKIP_REASON_") {
				t.Errorf("EvictSkipReason value %s lacks the EVICT_SKIP_REASON_ prefix", n)
			}
		}
		for _, w := range want {
			v := vs.ByName(w.name)
			if v == nil {
				t.Errorf("EvictSkipReason value %s is missing", w.name)
				continue
			}
			if v.Number() != w.number {
				t.Errorf("%s = %d, want %d", w.name, v.Number(), w.number)
			}
		}
	})

	t.Run("PruneSkipReason is unchanged", func(t *testing.T) {
		ed := fd.Enums().ByName("PruneSkipReason")
		if ed == nil {
			t.Fatal("enum PruneSkipReason is missing")
		}
		want := map[protoreflect.Name]protoreflect.EnumNumber{
			"PRUNE_SKIP_REASON_UNSPECIFIED":   0,
			"PRUNE_SKIP_REASON_REACHABLE":     1,
			"PRUNE_SKIP_REASON_LEASED":        2,
			"PRUNE_SKIP_REASON_IN_USE":        3,
			"PRUNE_SKIP_REASON_UNLINK_FAILED": 4,
		}
		vs := ed.Values()
		if vs.Len() != len(want) {
			t.Errorf("PruneSkipReason has %d values, want exactly %d", vs.Len(), len(want))
		}
		for name, num := range want {
			v := vs.ByName(name)
			if v == nil {
				t.Errorf("PruneSkipReason value %s is missing", name)
				continue
			}
			if v.Number() != num {
				t.Errorf("%s = %d, want %d", name, v.Number(), num)
			}
		}
	})

	t.Run("the ErrorInfo domain and reason constants are exact", func(t *testing.T) {
		for _, c := range []struct{ name, got, want string }{
			{"EvictErrorInfoDomain", EvictErrorInfoDomain, "runtime.k3sm.io"},
			{"EvictReasonImageInUse", EvictReasonImageInUse, "IMAGE_IN_USE"},
			{"EvictReasonReferenceAmbiguous", EvictReasonReferenceAmbiguous, "REFERENCE_AMBIGUOUS"},
			{"EvictReasonRootsIncomplete", EvictReasonRootsIncomplete, "ROOTS_INCOMPLETE"},
		} {
			if c.got != c.want {
				t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
			}
		}
	})

	t.Run("the EvictImage contract comment is present in the proto", func(t *testing.T) {
		raw, err := os.ReadFile("images.proto")
		if err != nil {
			t.Fatalf("read images.proto: %v", err)
		}
		block, ok := commentBlockAbove(string(raw), "rpc EvictImage(")
		if !ok || block == "" {
			t.Fatal("images.proto has no comment block above rpc EvictImage")
		}
		method := flattenComments(block)
		for _, want := range []string{
			"It removes no root, no index entry and no artifact record",
			"An unset force deletes nothing",
			"a platform with a digest is INVALID_ARGUMENT, unlike InspectImage, where it is ignored",
			"NOT_FOUND: no index entry, artifact record or released copy names the target",
			"FAILED_PRECONDITION, nothing unlinked, in three cases",
			"(a) the target is in use by a pod",
			"(b) the reference is ambiguous across several platform entries",
			"(c) the root set could not be read completely",
			"google.rpc.ErrorInfo status detail",
			"Only these three cases carry the detail",
			"UNAVAILABLE:",
			"INTERNAL:",
			"a re-run converges",
			// The ErrorInfo strings a caller matches on, from the Go constants.
			"domain " + EvictErrorInfoDomain,
			"reason " + EvictReasonImageInUse,
			"reason " + EvictReasonReferenceAmbiguous,
			"reason " + EvictReasonRootsIncomplete,
		} {
			if !strings.Contains(method, want) {
				t.Errorf("the EvictImage method comment lost the contract text %q", want)
			}
		}
		if strings.Contains(method, "UNLINK_FAILED") {
			t.Error("the EvictImage method comment still names the removed UNLINK_FAILED reason")
		}

		resp, ok := commentBlockAbove(string(raw), "message EvictImageResponse {")
		if !ok {
			t.Fatal("images.proto has no comment block above message EvictImageResponse")
		}
		if want := "does not say whether anything was unlinked"; !strings.Contains(flattenComments(resp), want) {
			t.Errorf("the EvictImageResponse comment lost the contract text %q", want)
		}

		if strings.Contains(flattenComments(string(raw)), "blob deletion happens only in PruneImages") {
			t.Error("the RemoveImage comment still says PruneImages is the only blob deletion")
		}
	})
}
