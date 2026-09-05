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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestImagesServiceSurface is B130a's named gate.
//
// The image verbs (list / measure / remove / prune / ingest) are a different
// caller's surface from the pod verbs, so they land as a separate service in
// the same proto package — the CRI RuntimeService/ImageService split. Both
// halves of that sentence are load-bearing and both are pinned here: a service
// that drifted into its own package would fork the contract module, and an
// entry type that re-declared digest/size/media_type as scalars would duplicate
// exactly what this module exists to single-source.
//
// The gate pins five structural facts and one textual one:
//
//   - the service exists at k3sm.runtime.v1.Images with EXACTLY ten methods —
//     the five store verbs it opened with plus the five image primitives
//     (pull / tag / untag / inspect / save) — each bound to its OWN request and
//     response type, so a later carve cannot quietly re-home a verb onto a
//     sibling's message;
//   - the streaming shapes: LoadImage is client-streaming (ingest) and
//     SaveImage is server-streaming (export), and nothing else streams. Its
//     siblings Exec/Attach/PortForward are bidirectional, and getting a stream
//     direction wrong is a wire-shape mistake no later additive change repairs;
//   - the entry types reach the EXISTING Descriptor / ImageManifest / Platform
//     messages and the existing ImagePullPolicy / LoadImageFormat enums, never
//     parallel ones;
//   - LoadImageRequest and SaveImageResponse carry bytes and scalars ONLY (no
//     OCI-typed framing — apis depends on nothing);
//   - every new message keeps the file's `reserved 100 to 149` headroom, and
//     every new message survives a wire round-trip.
//
// And the textual one: the contract comments in images.proto — the advisory
// digest / re-hash-before-commit clause, the lease obligation with its
// transitional citation, the socket-posture precision, the not-a-signature-
// checkpoint note, the reactive-skew decision, and the provenance contracts the
// image primitives are bound by (pull records an operator root; a tag is
// additive and never re-points; untag is the sanctioned explicit root removal
// and RemoveImage's refusal stands; export's terminal frame) — are asserted as
// SOURCE TEXT. They are the parts of this contract a compiler cannot check and a
// daemon implementer must read; deleting any of them reddens this test.
func TestImagesServiceSurface(t *testing.T) {
	fd := File_runtime_v1_images_proto

	t.Run("the service lives in the runtime package with exactly ten methods", func(t *testing.T) {
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
			"PullImage": false, "TagImage": false, "UntagImage": false,
			"InspectImage": false, "SaveImage": false,
		}
		ms := sd.Methods()
		if ms.Len() != len(want) {
			t.Errorf("Images has %d methods, want exactly %d", ms.Len(), len(want))
		}
		for i := range ms.Len() {
			name := string(ms.Get(i).Name())
			seen, known := want[name]
			if !known {
				t.Errorf("unexpected method %s on Images (the surface is exactly ten verbs)", name)
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

	t.Run("LoadImage is client-streaming, SaveImage server-streaming, the rest unary", func(t *testing.T) {
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
			{"PullImage", false, false},
			{"TagImage", false, false},
			{"UntagImage", false, false},
			{"InspectImage", false, false},
			// The two asymmetries in the file, and they point opposite ways.
			// Ingest: the client streams an archive and the server answers once —
			// there is nothing to say until the ingest commits. Export: the
			// server streams the archive and the client says nothing after the
			// request. Bidi (the Exec/Attach/PortForward shape) would be the
			// wrong contract for either.
			{"LoadImage", true, false},
			{"SaveImage", false, true},
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
		// pin it too: eight unary handlers plus the two streams, one in each
		// direction.
		if got, want := Images_ServiceDesc.ServiceName, "k3sm.runtime.v1.Images"; got != want {
			t.Errorf("Images_ServiceDesc.ServiceName = %q, want %q", got, want)
		}
		if got := len(Images_ServiceDesc.Methods); got != 8 {
			t.Errorf("Images_ServiceDesc has %d unary methods, want 8", got)
		}
		if got := len(Images_ServiceDesc.Streams); got != 2 {
			t.Fatalf("Images_ServiceDesc has %d streams, want 2 (LoadImage, SaveImage)", got)
		}
		wantStreams := map[string]grpc.StreamDesc{
			"LoadImage": {StreamName: "LoadImage", ClientStreams: true, ServerStreams: false},
			"SaveImage": {StreamName: "SaveImage", ClientStreams: false, ServerStreams: true},
		}
		for _, st := range Images_ServiceDesc.Streams {
			w, ok := wantStreams[st.StreamName]
			if !ok {
				t.Errorf("unexpected stream %q on Images_ServiceDesc", st.StreamName)
				continue
			}
			if st.ClientStreams != w.ClientStreams || st.ServerStreams != w.ServerStreams {
				t.Errorf("stream %s = ClientStreams:%v ServerStreams:%v, want ClientStreams:%v ServerStreams:%v",
					st.StreamName, st.ClientStreams, st.ServerStreams, w.ClientStreams, w.ServerStreams)
			}
		}
	})

	t.Run("every verb is bound to its own request and response type", func(t *testing.T) {
		// Per-RPC identity, pinned pairwise. Reusing a sibling's message would
		// couple two verbs' evolution forever: the reserved band that protects
		// one would have to be spent for the other, and buf's
		// RPC_REQUEST_RESPONSE_UNIQUE exemption (deliberately scoped to
		// guest.proto and runtime.proto, never this file) would have to widen.
		sd := fd.Services().ByName("Images")
		if sd == nil {
			t.Fatal("service Images does not exist")
		}
		cases := []struct{ method, in, out string }{
			{"ListImages", "ListImagesRequest", "ListImagesResponse"},
			{"ImageFsInfo", "ImageFsInfoRequest", "ImageFsInfoResponse"},
			{"RemoveImage", "RemoveImageRequest", "RemoveImageResponse"},
			{"PruneImages", "PruneImagesRequest", "PruneImagesResponse"},
			{"LoadImage", "LoadImageRequest", "LoadImageResponse"},
			{"PullImage", "PullImageRequest", "PullImageResponse"},
			{"TagImage", "TagImageRequest", "TagImageResponse"},
			{"UntagImage", "UntagImageRequest", "UntagImageResponse"},
			{"InspectImage", "InspectImageRequest", "InspectImageResponse"},
			{"SaveImage", "SaveImageRequest", "SaveImageResponse"},
		}
		if len(cases) != sd.Methods().Len() {
			t.Errorf("the identity table covers %d verbs, the service has %d", len(cases), sd.Methods().Len())
		}
		for _, tc := range cases {
			md := sd.Methods().ByName(protoreflect.Name(tc.method))
			if md == nil {
				t.Errorf("method %s is missing", tc.method)
				continue
			}
			if got, want := string(md.Input().FullName()), "k3sm.runtime.v1."+tc.in; got != want {
				t.Errorf("%s request = %s, want %s", tc.method, got, want)
			}
			if got, want := string(md.Output().FullName()), "k3sm.runtime.v1."+tc.out; got != want {
				t.Errorf("%s response = %s, want %s", tc.method, got, want)
			}
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
			// The image primitives reuse the same wrapper and the same platform
			// message: a pull, a tag and a list must agree on what an entry IS.
			{(&PullImageRequest{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
			{(&PullImageResponse{}).ProtoReflect().Descriptor(), "image", "k3sm.runtime.v1.Image"},
			{(&TagImageRequest{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
			{(&TagImageResponse{}).ProtoReflect().Descriptor(), "image", "k3sm.runtime.v1.Image"},
			{(&UntagImageRequest{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
			{(&UntagImageResponse{}).ProtoReflect().Descriptor(), "removed", "k3sm.runtime.v1.Image"},
			{(&InspectImageRequest{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
			{(&InspectImageResponse{}).ProtoReflect().Descriptor(), "image", "k3sm.runtime.v1.Image"},
			{(&InspectImageResponse{}).ProtoReflect().Descriptor(), "config", "k3sm.runtime.v1.ImageConfig"},
			// The DECLARED platform of the config blob — the existing Platform
			// message again, not a third os/arch spelling.
			{(&ImageConfig{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
			{(&SaveImageRequest{}).ProtoReflect().Descriptor(), "platform", "k3sm.runtime.v1.Platform"},
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

		// Nor may the wrapper-carrying responses re-spell them alongside the
		// wrapper: two spellings of one digest drift on the first divergent edit.
		for _, tc := range []struct {
			owner protoreflect.MessageDescriptor
			names []protoreflect.Name
		}{
			{(&PullImageResponse{}).ProtoReflect().Descriptor(), []protoreflect.Name{"digest", "platform", "media_type"}},
			{(&TagImageResponse{}).ProtoReflect().Descriptor(), []protoreflect.Name{"digest", "platform"}},
			{(&InspectImageResponse{}).ProtoReflect().Descriptor(), []protoreflect.Name{"digest", "media_type", "layers"}},
		} {
			for _, n := range tc.names {
				if tc.owner.Fields().ByName(n) != nil {
					t.Errorf("%s.%s exists; that fact belongs to the embedded Image, not a parallel field", tc.owner.Name(), n)
				}
			}
		}

		// The primitives reuse the file's EXISTING enums rather than minting
		// CLI-only twins of the same vocabulary.
		for _, tc := range []struct {
			owner protoreflect.MessageDescriptor
			field protoreflect.Name
			want  string
		}{
			{(&PullImageRequest{}).ProtoReflect().Descriptor(), "policy", "k3sm.runtime.v1.ImagePullPolicy"},
			{(&SaveImageRequest{}).ProtoReflect().Descriptor(), "format", "k3sm.runtime.v1.LoadImageFormat"},
		} {
			f := tc.owner.Fields().ByName(tc.field)
			if f == nil {
				t.Errorf("%s.%s does not exist", tc.owner.Name(), tc.field)
				continue
			}
			if f.Enum() == nil {
				t.Errorf("%s.%s is not an enum field (kind %v)", tc.owner.Name(), tc.field, f.Kind())
				continue
			}
			if got := string(f.Enum().FullName()); got != tc.want {
				t.Errorf("%s.%s = %s, want the existing %s", tc.owner.Name(), tc.field, got, tc.want)
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

	t.Run("SaveImageResponse mirrors that framing in reverse", func(t *testing.T) {
		// Export is ingest with the arrow turned around, so it gets the same
		// discipline: bytes and scalars only (apart from the shared error
		// status), a chunk field, and a terminal frame whose digest + sent_bytes
		// are what tells a client the archive is COMPLETE. Without a terminal
		// frame, truncation is indistinguishable from a short final chunk.
		md := (&SaveImageResponse{}).ProtoReflect().Descriptor()
		for i := range md.Fields().Len() {
			f := md.Fields().Get(i)
			if f.Name() == "error" { // the file-wide google.rpc.Status outcome field
				continue
			}
			switch f.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				t.Errorf("SaveImageResponse.%s is a %v field; the stream carries bytes and scalars only", f.Name(), f.Kind())
			}
			if f.IsMap() || f.IsList() {
				t.Errorf("SaveImageResponse.%s is a composite field; the stream carries bytes and scalars only", f.Name())
			}
		}
		if f := md.Fields().ByName("chunk"); f == nil || f.Kind() != protoreflect.BytesKind {
			t.Error("SaveImageResponse.chunk must exist as a bytes field; there is nothing to stream")
		}
		if f := md.Fields().ByName("digest"); f == nil || f.Kind() != protoreflect.StringKind {
			t.Error("SaveImageResponse.digest must exist as a string terminal-frame field")
		}
		if f := md.Fields().ByName("sent_bytes"); f == nil || f.Kind() != protoreflect.Int64Kind {
			t.Error("SaveImageResponse.sent_bytes must exist as an int64 terminal-frame field")
		}
		if f := md.Fields().ByName("error"); f == nil || f.Message() == nil ||
			string(f.Message().FullName()) != "google.rpc.Status" {
			t.Error("SaveImageResponse.error must exist as a google.rpc.Status")
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
			{"advisory-digest clause", "any client-supplied digest or size is advisory"},
			{"re-hash-before-commit clause", "must re-hash the received bytes and reject a mismatch before the lease commits"},
			// The bare "Resolution 8" citation also appears in the sole-store-
			// writer paragraph above the digest-contract one; pin the clause
			// that is unique to the digest paragraph so deleting it reddens.
			{"re-hash obligation clause", "every ingest path re-hashes content against its claimed digest before commit"},
			// The lease obligation, with the transitional citation and the marker
			// that says where it will move.
			{"lease obligation", "the server takes its store lease before the first blob commit and records the reference before releasing it"},
			{"lease citation placement note", "stated here only until the store/metastore package can carry it in its own doc.go"},
			{"lease citation TODO", "TODO: once that package is promoted, re-point this citation there"},
			// Loaded images are provenance-free; enforcement stays at exec.
			{"not-a-signature-checkpoint note", "LoadImage is not a SignaturePolicy checkpoint"},
			{"provenance-free note", "provenance-free by design"},
			{"operator-CLI-only surface clause", "an operator-CLI-only surface"},
			// The socket posture, stated precisely rather than as "root-only".
			{"embedded-posture clause", "serves no socket at all"},
			{"file-mode-alone clause", "guarded by POSIX file mode alone"},
			{"no-peercred clause", "There is no LOCAL_PEERCRED differentiation on this socket"},
			{"new-listener clause", "needs a new listener and authorizer design"},
			{"daemon-side control pointer", "asserted by a gate in the daemon repo"},
			// The skew decision, recorded so it is not re-derived.
			{"reactive-skew clause", "returns gRPC UNIMPLEMENTED for every method below"},
			{"deferred capability band", "Capability-band advertisement via GetRuntimeInfoResponse's reserved 100..149 band was considered and"},
			// The image primitives' provenance contracts. These are what bind a
			// daemon implementer: the proto alone must say that a pull is the
			// pod path plus an operator root, that a tag is monotone, that
			// untag — not RemoveImage — is the sanctioned root removal, and
			// that an export is complete only at its terminal frame.
			{"pull-uses-the-pod-path clause", "the same code path a pod-driven pull takes"},
			{"pull-is-the-CLI-primitive clause", "explicitly the CLI-pull primitive"},
			{"pull-records-an-operator-root clause", "an edge, plus an OPERATOR root over that reference"},
			{"provenance-model citation", "provenance model: edges are monotone, roots are digest-pinned, and root removal is authorized and local"},
			{"tag-is-monotone clause", "a tag can make content reachable, never unreachable"},
			{"tag-never-repoints clause", "never RE-POINTS an existing entry at a different digest"},
			{"tag-needs-present-content clause", "NOT_FOUND when the digest is absent from the store"},
			{"explicit-untag clause", "provenance model's sanctioned EXPLICIT UNTAG"},
			{"untag-removes-a-name clause", "Untag removes a NAME, not bytes"},
			{"untag-leaves-pinned-content clause", "leaves the content reachable and the pod unharmed"},
			{"RemoveImage-refusal-stands clause", "Deliberately distinct from RemoveImage, whose refusal stands"},
			{"inspect-is-read-only clause", "it resolves nothing against a registry, takes no lease and records no root"},
			{"inspect-fields-are-optional clause", "an absent field means the daemon reported no value for that fact, never that the image asserts an empty one"},
			{"save-inverts-load clause", "the exact inverse of LoadImage's direction"},
			{"save-terminal-frame clause", "exactly ONE terminal frame"},
			{"save-truncation clause", "has a truncated archive and must discard it"},
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
		&PullImageRequest{}, &PullImageResponse{},
		&TagImageRequest{}, &TagImageResponse{},
		&UntagImageRequest{}, &UntagImageResponse{},
		&InspectImageRequest{}, &InspectImageResponse{}, &ImageConfig{},
		&SaveImageRequest{}, &SaveImageResponse{},
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
		&PullImageRequest{
			Reference: "example.com/img:tag",
			Platform:  &Platform{Os: "darwin", Architecture: "arm64"},
			Policy:    ImagePullPolicy_IMAGE_PULL_POLICY_IF_NOT_PRESENT,
		},
		&PullImageResponse{Image: img, AlreadyPresent: true},
		&TagImageRequest{
			Digest:    "sha256:aaaa",
			Reference: "example.com/img:pinned",
			Platform:  &Platform{Os: "darwin", Architecture: "arm64"},
		},
		&TagImageResponse{Image: img},
		&UntagImageRequest{
			Reference: "example.com/img:pinned",
			Platform:  &Platform{Os: "darwin", Architecture: "arm64"},
			Digest:    "sha256:aaaa",
		},
		&UntagImageResponse{Removed: img},
		&InspectImageRequest{Reference: "example.com/img:tag", Platform: &Platform{Os: "darwin", Architecture: "arm64"}},
		&InspectImageResponse{
			Image: img,
			Config: &ImageConfig{
				Platform:   &Platform{Os: "linux", Architecture: "arm64", Variant: "v8"},
				Created:    timestamppb.New(time.Unix(1, 0).UTC()),
				Entrypoint: []string{"/usr/bin/blightmud"},
				Cmd:        []string{"--help"},
				Env:        []string{"PATH=/usr/bin", "TERM=xterm"},
				User:       "1000:1000",
				WorkingDir: "/srv",
				Labels:     map[string]string{"org.opencontainers.image.title": "blightmud"},
			},
			TotalSizeBytes: 1040,
		},
		&ImageConfig{Entrypoint: []string{"/bin/sh"}, Env: []string{"A=1"}},
		&SaveImageRequest{
			Reference: "example.com/img:tag",
			Platform:  &Platform{Os: "darwin", Architecture: "arm64"},
			Format:    LoadImageFormat_LOAD_IMAGE_FORMAT_OCI_LAYOUT,
		},
		&SaveImageResponse{Chunk: []byte("layout bytes")},
		// The terminal frame: no chunk, digest + the server's own byte count.
		&SaveImageResponse{Digest: "sha256:aaaa", SentBytes: 1024},
	}
}
