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

// TestContainerImagePlatformCarve pins the M11.1 platform-override carve.
//
// Multi-arch image selection is specified against an explicit per-container
// override, but the field did not exist — so every consumer of "the platform
// this container asked for" was specified against a carve nobody had filed.
//
// Field 100 was pinned to this name by the band comment before either of the two
// Container carves landed, which is why M12.1 started its own allocation at 101.
// The gate gives that pin teeth: the number, the TYPE (the existing Platform
// message, never a parallel string), the re-narrowed band, and the absent-means-
// no-override reading a default value would destroy.
func TestContainerImagePlatformCarve(t *testing.T) {
	t.Parallel()
	md := (&Container{}).ProtoReflect().Descriptor()

	t.Run("the field exists at the pinned number", func(t *testing.T) {
		fd := md.Fields().ByName("image_platform")
		if fd == nil {
			t.Fatal("Container.image_platform does not exist; multi-arch selection is specified against it")
		}
		if fd.Number() != 100 {
			t.Errorf("Container.image_platform = field %d, want the pinned 100", fd.Number())
		}
		if fd.Kind() != protoreflect.MessageKind {
			t.Fatalf("Container.image_platform kind = %v, want a message", fd.Kind())
		}
		// The type is the whole point: one message means one normalization point
		// for os/architecture/variant (arm64's "" and "v8" are the same platform).
		// A string field would put a second, silently divergent parser on the far
		// side of the wire.
		if got, want := string(fd.Message().FullName()), "k3sm.runtime.v1.Platform"; got != want {
			t.Errorf("Container.image_platform message = %s, want the existing %s", got, want)
		}
	})

	t.Run("no parallel string platform field exists anywhere", func(t *testing.T) {
		// The duplication this carve is most likely to grow: a convenience string
		// beside the message. Sweep every message in both files rather than trust
		// the one under test.
		for _, fd := range []protoreflect.FileDescriptor{File_runtime_v1_runtime_proto, File_runtime_v1_images_proto} {
			msgs := fd.Messages()
			for i := range msgs.Len() {
				m := msgs.Get(i)
				for j := range m.Fields().Len() {
					f := m.Fields().Get(j)
					if strings.Contains(string(f.Name()), "platform") && f.Kind() != protoreflect.MessageKind {
						t.Errorf("%s.%s is a scalar platform field (%v); platform is the Platform message everywhere", m.FullName(), f.Name(), f.Kind())
					}
				}
			}
		}
	})

	t.Run("an override survives a round-trip and absence stays absent", func(t *testing.T) {
		in := &Container{
			Name:          "app",
			Image:         "example.com/img:tag",
			ImagePlatform: &Platform{Os: "linux", Architecture: "amd64"},
		}
		b, err := proto.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out Container
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if p := out.GetImagePlatform(); p == nil {
			t.Fatal("image_platform did not survive the round-trip")
		} else if p.GetOs() != "linux" || p.GetArchitecture() != "amd64" {
			t.Errorf("image_platform = %+v, want linux/amd64", p)
		}

		// Absent must read as "no override — apply the node's default policy",
		// which only holds because the field is a message: a zero-valued Platform
		// and an unset one would otherwise be the same bytes, and the node's own
		// backend-plus-capability derivation would be overruled by a peer that
		// never made a choice.
		b, err = proto.Marshal(&Container{Name: "app", Image: "/usr/bin/true"})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out.Reset()
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.GetImagePlatform() != nil {
			t.Errorf("image_platform = %+v with no override set, want nil", out.GetImagePlatform())
		}
	})

	t.Run("the reserved band is re-narrowed and keeps its ceiling", func(t *testing.T) {
		// The band was `reserved 100, 102 to 149` while 100 was held for this
		// carve. Claiming it must leave exactly `reserved 102 to 149`: 100 must no
		// longer be reserved (a number cannot be both allocated and reserved), and
		// the 149 ceiling must survive so a later carve cannot eat the headroom.
		rr := md.ReservedRanges()
		var lo, hi protoreflect.FieldNumber
		found := false
		for i := range rr.Len() {
			r := rr.Get(i) // [start, end) — end is exclusive
			if r[1]-1 >= 102 {
				lo, hi, found = r[0], r[1]-1, true
			}
		}
		if !found {
			t.Fatal("Container has no reserved range above the allocated fields; the 149 ceiling was dropped")
		}
		if lo != 102 || hi != 149 {
			t.Errorf("reserved range = %d..%d, want 102..149", lo, hi)
		}
		for _, n := range []protoreflect.FieldNumber{100, 101} {
			if rr.Has(n) {
				t.Errorf("field %d is both allocated and reserved", n)
			}
		}
	})
}

// TestVolumeHostPathCarve pins the M11.1 hostPath carve.
//
// The field lands CONSUMER-LESS on purpose: a pod spec carrying a hostPath must
// be REPRESENTABLE so it can be rejected with a legible reason, rather than
// silently mangled into some other source on the way through. The gate therefore
// asserts the shape and, just as importantly, that the shape stayed a shape —
// the moment an enforcement field (an allowlist, a share-versus-snapshot mode)
// appears here, a consumer will trust a policy that an attacker-shaped pod spec
// asserted about itself.
func TestVolumeHostPathCarve(t *testing.T) {
	t.Parallel()

	t.Run("Volume.host_path takes the next sequential source number", func(t *testing.T) {
		fd := (&Volume{}).ProtoReflect().Descriptor().Fields().ByName("host_path")
		if fd == nil {
			t.Fatal("Volume.host_path does not exist")
		}
		if fd.Number() != 8 {
			t.Errorf("Volume.host_path = field %d, want 8 (2..7 were taken by the existing sources)", fd.Number())
		}
		if fd.Kind() != protoreflect.MessageKind || fd.Message().FullName() != "k3sm.runtime.v1.HostPathVolumeSource" {
			t.Errorf("Volume.host_path type = %v, want the HostPathVolumeSource message", fd.Kind())
		}
	})

	t.Run("HostPathVolumeSource mirrors corev1 exactly — path and type, nothing else", func(t *testing.T) {
		md := (&HostPathVolumeSource{}).ProtoReflect().Descriptor()
		if got := md.Fields().Len(); got != 2 {
			t.Errorf("HostPathVolumeSource has %d fields, want exactly 2 (corev1 carries path + type)", got)
		}
		cases := []struct {
			name   protoreflect.Name
			number protoreflect.FieldNumber
		}{{"path", 1}, {"type", 2}}
		for _, tc := range cases {
			fd := md.Fields().ByName(tc.name)
			if fd == nil {
				t.Errorf("HostPathVolumeSource.%s is missing", tc.name)
				continue
			}
			if fd.Number() != tc.number {
				t.Errorf("HostPathVolumeSource.%s = field %d, want %d", tc.name, fd.Number(), tc.number)
			}
			// Both are strings: corev1's HostPathType is a string alias whose empty
			// value is meaningful, so an enum would have to invent a zero-value
			// distinction upstream does not have.
			if fd.Kind() != protoreflect.StringKind {
				t.Errorf("HostPathVolumeSource.%s kind = %v, want string", tc.name, fd.Kind())
			}
		}
	})

	t.Run("a hostPath volume survives a round-trip", func(t *testing.T) {
		in := &Volume{Name: "hostdata", HostPath: &HostPathVolumeSource{Path: "/opt/data", Type: "DirectoryOrCreate"}}
		b, err := proto.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out Volume
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !proto.Equal(in, &out) {
			t.Errorf("round-trip differs:\n got: %v\nwant: %v", &out, in)
		}
		// A volume with no hostPath must stay distinguishable from one with an
		// empty hostPath — the source union is "exactly one is set", and only a
		// message field can express that.
		b, err = proto.Marshal(&Volume{Name: "scratch", EmptyDir: &EmptyDirVolumeSource{}})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out.Reset()
		if err := proto.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.GetHostPath() != nil {
			t.Errorf("host_path = %+v on an emptyDir volume, want nil", out.GetHostPath())
		}
	})
}

// TestContainerCommandDiscriminatorDocumented pins the resolveBinary contract at
// the field a reader actually consults.
//
// The M0 convention — empty command means "exec the image reference" — is
// DISCRIMINATED by this change, not retired: an absolute-path image keeps it,
// while an OCI reference resolves argv from the image config instead. That is a
// behavioral fork with no type to carry it, so the comment IS the contract, and
// a runtime author who reads only "if both are empty the image reference is
// treated as the binary path" would implement the pre-M11 behavior for OCI
// images and never know.
//
// Asserted over BOTH the .proto and the generated .pb.go, because a Go author
// reads the copied comment. Keywords, not sentences: a reworded comment keeps
// the gate green; one that drops the discriminator does not. The superseded
// framing is checked ABSENT file-wide, so appending the new text beside the old
// cannot go green.
func TestContainerCommandDiscriminatorDocumented(t *testing.T) {
	t.Parallel()

	// The superseded sentence's discriminating fragment: it made emptiness alone
	// decide, with no mention of the image's shape.
	const supersededFragment = "If both are empty the image reference is treated as the binary path"

	keywords := []struct {
		want, why string
	}{
		{"absolute path", "an absolute-path image keeps the M0 host-binary convention"},
		{"OCI", "an OCI reference resolves argv from the image config instead"},
		{"image config", "that is where argv comes from in the OCI case"},
	}

	files := []struct {
		path      string
		fieldDecl string
	}{
		{"runtime.proto", "repeated string command = 3;"},
		{"runtime.pb.go", "Command []string `protobuf:\"bytes,3,"},
	}

	for _, f := range files {
		t.Run(f.path, func(t *testing.T) {
			raw, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("read %s: %v", f.path, err)
			}
			src := string(raw)

			block, ok := commentBlockAbove(src, f.fieldDecl)
			if !ok {
				t.Fatalf("%s: no declaration matching %q; the gate cannot locate the command contract", f.path, f.fieldDecl)
			}
			hay := strings.ToLower(flattenComments(block))
			for _, kw := range keywords {
				if !strings.Contains(hay, strings.ToLower(kw.want)) {
					t.Errorf("%s: command comment does not mention %q (%s)\ncomment block:\n%s", f.path, kw.want, kw.why, block)
				}
			}
			if strings.Contains(flattenComments(src), supersededFragment) {
				t.Errorf("%s: the superseded emptiness-alone framing %q is still present; it must be REPLACED, not appended beside", f.path, supersededFragment)
			}
		})
	}
}
