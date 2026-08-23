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

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestImagesServiceSurface is B130a's named gate.
//
// The image verbs (list / measure / remove / prune / ingest) are a different
// caller's surface from the pod verbs, so they land as a SEPARATE SERVICE IN
// THE SAME PROTO PACKAGE — the CRI RuntimeService/ImageService split. Both
// halves of that sentence are load-bearing and both are pinned here: a service
// that drifted into its own package would fork the contract module, and an
// entry type that re-declared digest/size/media_type as scalars would duplicate
// exactly what this module exists to single-source.
//
// The gate pins four structural facts and one textual one:
//
//   - the service exists at k3sm.runtime.v1.Images with EXACTLY five methods,
//     and LoadImage alone is client-streaming (the file's first — its siblings
//     Exec/Attach/PortForward are bidirectional, and getting that wrong is a
//     wire-shape mistake no later additive change can repair);
//   - ListImages' entry type reaches the EXISTING Descriptor / ImageManifest /
//     Platform messages, never parallel ones;
//   - LoadImageRequest carries bytes and scalars ONLY (no OCI-typed framing —
//     apis depends on nothing);
//   - every new message keeps the file's `reserved 100 to 149` headroom, and
//     every new message survives a wire round-trip.
//
// And the textual one: the contract comments in images.proto — the advisory
// digest / re-hash-before-commit clause, the lease obligation with its
// transitional citation, the socket-posture precision, the not-a-signature-
// checkpoint note, and the reactive-skew decision — are asserted as SOURCE
// TEXT. They are the parts of this contract a compiler cannot check and a
// daemon implementer must read; deleting any of them reddens this test.
func TestImagesServiceSurface(t *testing.T) {
	fd := File_runtime_v1_images_proto

	t.Run("the service lives in the runtime package with exactly five methods", func(t *testing.T) {
		// Same package as runtime.proto: the CRI split is service-level, never a
		// second proto package or import path.
		siblingPkg := (&Descriptor{}).ProtoReflect().Descriptor().ParentFile().Package()
		if fd.Package() != siblingPkg {
			t.Errorf("images.proto package = %q, want the sibling runtime.proto package %q", fd.Package(), siblingPkg)
		}

		sd := fd.Services().ByName("Images")
		if sd == nil {
			t.Fatal("service Images does not exist in images.proto")
		}
		if got, want := string(sd.FullName()), "k3sm.runtime.v1.Images"; got != want {
			t.Errorf("service full name = %q, want %q", got, want)
		}

		want := map[string]bool{
			"ListImages": false, "ImageFsInfo": false, "RemoveImage": false,
			"PruneImages": false, "LoadImage": false,
		}
		ms := sd.Methods()
		if ms.Len() != len(want) {
			t.Errorf("Images has %d methods, want exactly %d", ms.Len(), len(want))
		}
		for i := range ms.Len() {
			name := string(ms.Get(i).Name())
			seen, known := want[name]
			if !known {
				t.Errorf("unexpected method %s on Images (the surface is exactly five verbs)", name)
				continue
			}
			if seen {
				t.Errorf("method %s declared twice", name)
			}
			want[name] = true
		}
		for name, seen := range want {
			if !seen {
				t.Errorf("method %s is missing from Images", name)
			}
		}
	})

	t.Run("LoadImage is client-streaming and the other four are unary", func(t *testing.T) {
		sd := fd.Services().ByName("Images")
		if sd == nil {
			t.Fatal("service Images does not exist")
		}
		cases := []struct {
			name                       string
			clientStream, serverStream bool
		}{
			{"ListImages", false, false},
			{"ImageFsInfo", false, false},
			{"RemoveImage", false, false},
			{"PruneImages", false, false},
			// The one asymmetry in the file: the client streams an archive, the
			// server answers once. Bidi (the Exec/Attach/PortForward shape) would
			// be the wrong contract — there is nothing to say until the ingest
			// commits.
			{"LoadImage", true, false},
		}
		for _, tc := range cases {
			md := sd.Methods().ByName(protoreflect.Name(tc.name))
			if md == nil {
				t.Errorf("method %s is missing", tc.name)
				continue
			}
			if md.IsStreamingClient() != tc.clientStream {
				t.Errorf("%s IsStreamingClient() = %v, want %v", tc.name, md.IsStreamingClient(), tc.clientStream)
			}
			if md.IsStreamingServer() != tc.serverStream {
				t.Errorf("%s IsStreamingServer() = %v, want %v", tc.name, md.IsStreamingServer(), tc.serverStream)
			}
		}

		// The generated grpc.ServiceDesc is what a daemon actually registers, so
		// pin it too: four unary handlers plus one client-stream.
		if got, want := Images_ServiceDesc.ServiceName, "k3sm.runtime.v1.Images"; got != want {
			t.Errorf("Images_ServiceDesc.ServiceName = %q, want %q", got, want)
		}
		if got := len(Images_ServiceDesc.Methods); got != 4 {
			t.Errorf("Images_ServiceDesc has %d unary methods, want 4", got)
		}
		if got := len(Images_ServiceDesc.Streams); got != 1 {
			t.Fatalf("Images_ServiceDesc has %d streams, want 1 (LoadImage)", got)
		}
		st := Images_ServiceDesc.Streams[0]
		if st.StreamName != "LoadImage" || !st.ClientStreams || st.ServerStreams {
			t.Errorf("stream = %+v, want LoadImage with ClientStreams=true, ServerStreams=false",
				grpc.StreamDesc{StreamName: st.StreamName, ClientStreams: st.ClientStreams, ServerStreams: st.ServerStreams})
		}
	})

	t.Run("the list entry reuses the existing content messages", func(t *testing.T) {
		// A parallel Image{digest,size,media_type} would be the duplication the
		// shared-contracts module exists to prevent, and would drift from the
		// manifest it purports to describe on the first divergent edit.
		cases := []struct {
			owner protoreflect.MessageDescriptor
			field protoreflect.Name
			want  string
		}{
			{(&ListImagesResponse{}).ProtoReflect().Descriptor(), "images", "k3sm.runtime.v1.Image"},
			{(&Image{}).ProtoReflect().Descriptor(), "manifest_descriptor", "k3sm.runtime.v1.Descriptor"},
			{(&Image{}).ProtoReflect().Descriptor(), "manifest", "k3sm.runtime.v1.ImageManifest"},
			{(&ListImagesRequest{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
			{(&RemoveImageRequest{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
			{(&LoadImageResponse{}).ProtoReflect().Descriptor(), "images", "k3sm.runtime.v1.Image"},
		}
		for _, tc := range cases {
			f := tc.owner.Fields().ByName(tc.field)
			if f == nil {
				t.Errorf("%s.%s does not exist", tc.owner.Name(), tc.field)
				continue
			}
			if f.Message() == nil {
				t.Errorf("%s.%s is not a message field (kind %v)", tc.owner.Name(), tc.field, f.Kind())
				continue
			}
			if got := string(f.Message().FullName()); got != tc.want {
				t.Errorf("%s.%s = %s, want the existing %s", tc.owner.Name(), tc.field, got, tc.want)
			}
		}

		// Image must not re-spell the descriptor's facts as its own scalars.
		imd := (&Image{}).ProtoReflect().Descriptor()
		for _, n := range []protoreflect.Name{"digest", "size", "size_bytes", "media_type"} {
			if imd.Fields().ByName(n) != nil {
				t.Errorf("Image.%s exists; that fact belongs to the embedded Descriptor, not a parallel scalar", n)
			}
		}
	})

	t.Run("LoadImageRequest is bytes and scalars only", func(t *testing.T) {
		// No OCI-typed framing on the ingest stream: apis depends on nothing, and
		// a typed archive model here would drag an OCI type system into the
		// contract module. Message-kind fields are the shape that would do it.
		md := (&LoadImageRequest{}).ProtoReflect().Descriptor()
		for i := range md.Fields().Len() {
			f := md.Fields().Get(i)
			switch f.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				t.Errorf("LoadImageRequest.%s is a %v field; the stream carries bytes and scalars only", f.Name(), f.Kind())
			}
			if f.IsMap() || f.IsList() {
				t.Errorf("LoadImageRequest.%s is a composite field; the stream carries bytes and scalars only", f.Name())
			}
		}
		chunk := md.Fields().ByName("chunk")
		if chunk == nil {
			t.Fatal("LoadImageRequest.chunk does not exist; there is nothing to stream")
		}
		if chunk.Kind() != protoreflect.BytesKind {
			t.Errorf("LoadImageRequest.chunk kind = %v, want bytes", chunk.Kind())
		}
		for _, n := range []protoreflect.Name{"reference", "digest"} {
			if f := md.Fields().ByName(n); f == nil || f.Kind() != protoreflect.StringKind {
				t.Errorf("LoadImageRequest.%s must exist as a string metadata field", n)
			}
		}
		if f := md.Fields().ByName("size"); f == nil || f.Kind() != protoreflect.Int64Kind {
			t.Errorf("LoadImageRequest.size must exist as an int64 metadata field")
		}
	})

	t.Run("every new message keeps the reserved 100..149 headroom", func(t *testing.T) {
		// The file convention. Without the band a later carve has no protected
		// numbers to allocate from, which is how a sibling message ends up
		// renumbering fields under a WIRE_JSON guard that cannot help it.
		for _, m := range newMessages() {
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
				t.Errorf("%s does not declare `reserved 100 to 149`", md.Name())
			}
			// And nothing may be allocated inside the band yet.
			for i := range md.Fields().Len() {
				if n := md.Fields().Get(i).Number(); n >= 100 && n <= 149 {
					t.Errorf("%s.%s = field %d, inside the reserved band", md.Name(), md.Fields().Get(i).Name(), n)
				}
			}
		}
	})

	t.Run("every new message survives a wire round-trip", func(t *testing.T) {
		for _, in := range populatedMessages() {
			name := string(in.ProtoReflect().Descriptor().Name())
			b, err := proto.Marshal(in)
			if err != nil {
				t.Errorf("marshal %s: %v", name, err)
				continue
			}
			out := in.ProtoReflect().New().Interface()
			if err := proto.Unmarshal(b, out); err != nil {
				t.Errorf("unmarshal %s: %v", name, err)
				continue
			}
			if !proto.Equal(in, out) {
				t.Errorf("%s did not survive the round-trip:\n in = %v\nout = %v", name, in, out)
			}
		}
	})

	t.Run("the load-bearing contract comments are present in the proto", func(t *testing.T) {
		// These clauses are the contract a compiler cannot check: the daemon-side
		// obligations (re-hash before commit, lease before first blob commit),
		// the honest socket posture, and two recorded decisions. Each is asserted
		// as source text so deleting the comment reddens the gate — a contract
		// comment nobody guards is a comment that silently disappears.
		raw, err := os.ReadFile("images.proto")
		if err != nil {
			t.Fatalf("read images.proto: %v", err)
		}
		// Normalize comment markers and wrapping so the assertions match prose
		// that spans lines.
		text := strings.Join(strings.Fields(strings.ReplaceAll(string(raw), "//", " ")), " ")

		cases := []struct {
			what, want string
		}{
			// Advisory digest + re-hash before the lease commits (the ingest
			// self-authentication obligation).
			{"advisory-digest clause", "any client-supplied digest or size is ADVISORY"},
			{"re-hash-before-commit clause", "MUST re-hash the received bytes and reject a mismatch BEFORE the lease commits"},
			// The bare "Resolution 8" citation also appears in the SOLE STORE
			// WRITER paragraph above the DIGEST CONTRACT one; pin the clause
			// that is unique to the digest paragraph so deleting IT reddens.
			{"re-hash resolution citation", "Resolution 8: every ingest path"},
			// The lease obligation, with the transitional citation and the marker
			// that says where it will move.
			{"lease obligation", "the server takes its store lease before the first blob commit and records the reference before releasing it"},
			{"lease resolution citation", "the M12 images plan, Resolution 13(c)"},
			{"lease citation TODO", "TODO(B128)"},
			// Loaded images are provenance-free; enforcement stays at exec.
			{"not-a-signature-checkpoint note", "LoadImage is NOT a SignaturePolicy CHECKPOINT"},
			{"provenance-free note", "PROVENANCE-FREE BY DESIGN"},
			{"signature-policy plan citation", "the M12 images plan, section M12.2"},
			// The socket posture, stated precisely rather than as "root-only".
			{"embedded-posture clause", "serves NO SOCKET AT ALL"},
			{"file-mode-alone clause", "guarded by POSIX FILE MODE ALONE"},
			{"no-peercred clause", "There is NO LOCAL_PEERCRED differentiation on this socket"},
			{"new-listener clause", "is a NEW LISTENER / AUTHZ DESIGN"},
			{"daemon-side control pointer", "B130b's gate"},
			// The skew decision, recorded so it is not re-derived.
			{"reactive-skew clause", "returns gRPC UNIMPLEMENTED for every method below"},
			{"deferred capability band", "Capability-band advertisement via GetRuntimeInfoResponse's reserved 100..149 band was considered and is DEFERRED"},
		}
		for _, tc := range cases {
			if !strings.Contains(text, tc.want) {
				t.Errorf("images.proto lost its %s; expected the text %q", tc.what, tc.want)
			}
		}
	})
}

// newMessages returns one zero value of every message images.proto declares.
func newMessages() []proto.Message {
	return []proto.Message{
		&ListImagesRequest{}, &ListImagesResponse{}, &Image{},
		&ImageFsInfoRequest{}, &ImageFsInfoResponse{}, &FilesystemUsage{},
		&RemoveImageRequest{}, &RemoveImageResponse{},
		&PruneImagesRequest{}, &PruneImagesResponse{}, &SkippedBlob{},
		&LoadImageRequest{}, &LoadImageResponse{},
	}
}

// populatedMessages returns one populated instance of every message
// images.proto declares, each with its embedded/reused types filled so the
// round-trip exercises the reuse and not just the wrapper.
func populatedMessages() []proto.Message {
	img := &Image{
		ManifestDescriptor: &Descriptor{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:aaaa",
			Size:      1024,
		},
		Manifest: &ImageManifest{
			Reference:   "example.com/img:tag",
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			IndexDigest: "sha256:bbbb",
			Platform:    &Platform{Os: "darwin", Architecture: "arm64"},
			Config:      &Descriptor{Digest: "sha256:cccc", Size: 7},
			Layers:      []*Descriptor{{Digest: "sha256:dddd", Size: 9}},
		},
	}
	return []proto.Message{
		&ListImagesRequest{Reference: "example.com/img:tag", Platform: &Platform{Os: "darwin", Architecture: "arm64"}},
		&ListImagesResponse{Images: []*Image{img}},
		img,
		&ImageFsInfoRequest{},
		&ImageFsInfoResponse{
			Filesystems: []*FilesystemUsage{{
				Mountpoint:     "/",
				UsedBytes:      1 << 30,
				CapacityBytes:  1 << 40,
				AvailableBytes: 1 << 39,
				InodesUsed:     42,
			}},
			StoreBytes: 1 << 20,
		},
		&FilesystemUsage{Mountpoint: "/", UsedBytes: 3, CapacityBytes: 4, AvailableBytes: 1, InodesUsed: 2},
		&RemoveImageRequest{Reference: "example.com/img:tag", Platform: &Platform{Os: "darwin", Architecture: "arm64"}, Digest: "sha256:aaaa"},
		&RemoveImageResponse{RemovedReferences: []string{"example.com/img:tag"}},
		&PruneImagesRequest{DryRun: true},
		&PruneImagesResponse{
			RemovedDigests: []string{"sha256:dddd"},
			Skipped:        []*SkippedBlob{{Digest: "sha256:cccc", Reason: PruneSkipReason_PRUNE_SKIP_REASON_REACHABLE}},
			ReclaimedBytes: 9,
		},
		&SkippedBlob{Digest: "sha256:cccc", Reason: PruneSkipReason_PRUNE_SKIP_REASON_LEASED},
		&LoadImageRequest{
			Reference: "example.com/img:tag",
			Format:    LoadImageFormat_LOAD_IMAGE_FORMAT_OCI_LAYOUT,
			Digest:    "sha256:aaaa",
			Size:      1024,
			Chunk:     []byte("archive bytes"),
		},
		&LoadImageResponse{Images: []*Image{img}, ReceivedBytes: 1024},
	}
}
