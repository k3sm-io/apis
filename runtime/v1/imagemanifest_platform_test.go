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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestImageManifestPlatformCarve is B127's named gate.
//
// The local image index is specified as keyed by (reference x platform), and the
// GC's provenance edges and the container-status image_id are specified against
// the index the reference resolved through — but neither field existed, so every
// one of those consumers was specified against a carve nobody had filed.
//
// This allocates both out of ImageManifest's already-reserved 100..149 band,
// which is why the change is additive and wire-compatible for every existing
// client rather than the phased apis exception: no existing field number moves,
// and a peer that predates the carve simply skips two unknown fields.
//
// The gate pins three things: the fields exist with the intended numbers and
// types, resolved values survive a wire round-trip, and the reserved band's
// ceiling is preserved so a later carve cannot silently eat the headroom.
func TestImageManifestPlatformCarve(t *testing.T) {
	md := (&ImageManifest{}).ProtoReflect().Descriptor()

	t.Run("fields exist with the allocated numbers", func(t *testing.T) {
		cases := []struct {
			name   protoreflect.Name
			number protoreflect.FieldNumber
			kind   protoreflect.Kind
		}{
			{"platform", 100, protoreflect.MessageKind},
			{"index_digest", 101, protoreflect.StringKind},
		}
		for _, tc := range cases {
			fd := md.Fields().ByName(tc.name)
			if fd == nil {
				t.Errorf("ImageManifest.%s does not exist; the index key and the GC provenance edges are specified against it", tc.name)
				continue
			}
			if fd.Number() != tc.number {
				t.Errorf("ImageManifest.%s = field %d, want %d (allocated from the reserved band)", tc.name, fd.Number(), tc.number)
			}
			if fd.Kind() != tc.kind {
				t.Errorf("ImageManifest.%s kind = %v, want %v", tc.name, fd.Kind(), tc.kind)
			}
		}
		// platform must be the existing Platform message, not a new parallel type —
		// a second platform shape would be exactly the duplication the shared
		// contracts module exists to prevent.
		if fd := md.Fields().ByName("platform"); fd != nil && fd.Message() != nil {
			if got := string(fd.Message().FullName()); got != "k3sm.runtime.v1.Platform" {
				t.Errorf("ImageManifest.platform message = %s, want the existing k3sm.runtime.v1.Platform", got)
			}
		}
	})

	t.Run("resolved values survive a round-trip", func(t *testing.T) {
		in := &ImageManifest{
			Reference:   "example.com/img:tag",
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			IndexDigest: "sha256:bbbb",
			Platform: &Platform{
				Os:           "linux",
				Architecture: "arm64",
				Variant:      "v8",
				OsVersion:    "",
			},
		}
		b, err := proto.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out ImageManifest
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.GetIndexDigest() != "sha256:bbbb" {
			t.Errorf("index_digest = %q, want sha256:bbbb", out.GetIndexDigest())
		}
		if p := out.GetPlatform(); p == nil {
			t.Fatal("platform did not survive the round-trip")
		} else if p.GetOs() != "linux" || p.GetArchitecture() != "arm64" || p.GetVariant() != "v8" {
			t.Errorf("platform = %+v, want linux/arm64/v8", p)
		}
	})

	t.Run("absent values stay absent", func(t *testing.T) {
		// A reference that resolved directly to a manifest has no index and no
		// resolved platform; both must read as empty rather than as a zero-valued
		// Platform, or a consumer cannot distinguish "resolved to linux/amd64"
		// from "there was nothing to resolve".
		b, err := proto.Marshal(&ImageManifest{Reference: "example.com/img@sha256:cccc"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out ImageManifest
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.GetPlatform() != nil {
			t.Errorf("platform = %+v on a direct-manifest resolve, want nil", out.GetPlatform())
		}
		if out.GetIndexDigest() != "" {
			t.Errorf("index_digest = %q on a direct-manifest resolve, want empty", out.GetIndexDigest())
		}
	})

	t.Run("the reserved band keeps its ceiling", func(t *testing.T) {
		// The band was 100..149. Allocating 100 and 101 must NARROW it to 102..149,
		// never drop it: without the remaining range a later field could reuse a
		// number the band was set aside to protect.
		var lo, hi protoreflect.FieldNumber
		found := false
		rr := md.ReservedRanges()
		for i := range rr.Len() {
			r := rr.Get(i) // [start, end) — end is exclusive
			if r[1]-1 >= 102 {
				lo, hi, found = r[0], r[1]-1, true
			}
		}
		if !found {
			t.Fatal("ImageManifest has no reserved range above the allocated fields; the 149 ceiling was dropped")
		}
		if lo != 102 {
			t.Errorf("reserved range starts at %d, want 102 (100..101 are now allocated)", lo)
		}
		if hi != 149 {
			t.Errorf("reserved range ends at %d, want the preserved 149 ceiling", hi)
		}
		// And the allocated numbers must no longer be reserved, or the descriptor
		// would be self-contradictory.
		for _, n := range []protoreflect.FieldNumber{100, 101} {
			if rr.Has(n) {
				t.Errorf("field %d is both allocated and reserved", n)
			}
		}
	})
}
