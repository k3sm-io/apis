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

// wireField is one expected field of a wire message: name, number, kind, and
// (for message and enum kinds) the full name of the referenced type.
type wireField struct {
	name     protoreflect.Name
	number   protoreflect.FieldNumber
	kind     protoreflect.Kind
	repeated bool
	ref      protoreflect.FullName
}

// assertExactFields fails unless md declares exactly want, field for field.
func assertExactFields(t *testing.T, md protoreflect.MessageDescriptor, want []wireField) {
	t.Helper()
	if got := md.Fields().Len(); got != len(want) {
		t.Errorf("%s has %d fields, want exactly %d", md.Name(), got, len(want))
	}
	for _, w := range want {
		fd := md.Fields().ByName(w.name)
		if fd == nil {
			t.Errorf("%s.%s is missing", md.Name(), w.name)
			continue
		}
		if fd.Number() != w.number {
			t.Errorf("%s.%s = field %d, want %d", md.Name(), w.name, fd.Number(), w.number)
		}
		if fd.Kind() != w.kind {
			t.Errorf("%s.%s kind = %v, want %v", md.Name(), w.name, fd.Kind(), w.kind)
		}
		if fd.IsList() != w.repeated {
			t.Errorf("%s.%s repeated = %v, want %v", md.Name(), w.name, fd.IsList(), w.repeated)
		}
		var ref protoreflect.FullName
		switch {
		case fd.Message() != nil:
			ref = fd.Message().FullName()
		case fd.Enum() != nil:
			ref = fd.Enum().FullName()
		}
		if ref != w.ref {
			t.Errorf("%s.%s references %q, want %q", md.Name(), w.name, ref, w.ref)
		}
	}
}

// TestImageVolumeSourceWire pins the image volume source: Volume field 9, and
// the exact shape of ImageVolumeSource (reference, pull_policy, paths).
func TestImageVolumeSourceWire(t *testing.T) {
	t.Run("Volume.image takes the next sequential source number", func(t *testing.T) {
		fd := (&Volume{}).ProtoReflect().Descriptor().Fields().ByName("image")
		if fd == nil {
			t.Fatal("Volume.image does not exist")
		}
		if fd.Number() != 9 {
			t.Errorf("Volume.image = field %d, want 9 (2..8 were taken by the existing sources)", fd.Number())
		}
		if fd.Message() == nil || fd.Message().FullName() != "k3sm.runtime.v1.ImageVolumeSource" {
			t.Errorf("Volume.image is not the ImageVolumeSource message")
		}
	})

	t.Run("ImageVolumeSource is reference, pull_policy and paths, nothing else", func(t *testing.T) {
		assertExactFields(t, (&ImageVolumeSource{}).ProtoReflect().Descriptor(), []wireField{
			{name: "reference", number: 1, kind: protoreflect.StringKind},
			{name: "pull_policy", number: 2, kind: protoreflect.EnumKind, ref: "k3sm.runtime.v1.ImagePullPolicy"},
			{name: "paths", number: 3, kind: protoreflect.StringKind, repeated: true},
		})
	})

	t.Run("an image volume survives a wire round-trip", func(t *testing.T) {
		in := &Volume{Name: "model", Image: &ImageVolumeSource{
			Reference:  "registry.k3sm.local/models/m@sha256:aaaa",
			PullPolicy: ImagePullPolicy_IMAGE_PULL_POLICY_IF_NOT_PRESENT,
			Paths:      []string{"config.json", "model-00001-of-00002.safetensors"},
		}}
		b, err := proto.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out := &Volume{}
		if err := proto.Unmarshal(b, out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !proto.Equal(in, out) {
			t.Errorf("round-trip mismatch:\n in = %v\nout = %v", in, out)
		}
	})

	t.Run("the hand-kept union comments name the image source", func(t *testing.T) {
		raw, err := os.ReadFile("runtime.proto")
		if err != nil {
			t.Fatalf("read runtime.proto: %v", err)
		}
		src := string(raw)
		for _, want := range []string{
			"persistentVolumeClaim / hostPath / image",
			"persistent_volume_claim, host_path, and image are the source union",
			"ImageVolumeSource image = 9;",
			"must refuse a non-empty paths",
		} {
			if !strings.Contains(src, want) {
				t.Errorf("runtime.proto lost the contract text %q", want)
			}
		}
	})
}

// TestPullArtifactWire pins the Images.PullArtifact contract: its request and
// response fields, the closed consumer enum, and the idempotent, resumable,
// fail-closed comment text a compiler cannot check.
func TestPullArtifactWire(t *testing.T) {
	t.Run("the request carries reference, pull policy, paths, consumer policy and credentials", func(t *testing.T) {
		assertExactFields(t, (&PullArtifactRequest{}).ProtoReflect().Descriptor(), []wireField{
			{name: "reference", number: 1, kind: protoreflect.StringKind},
			{name: "pull_policy", number: 2, kind: protoreflect.EnumKind, ref: "k3sm.runtime.v1.ImagePullPolicy"},
			{name: "paths", number: 3, kind: protoreflect.StringKind, repeated: true},
			{name: "artifact_policy", number: 4, kind: protoreflect.EnumKind, ref: "k3sm.runtime.v1.ArtifactPolicy"},
			{name: "namespace", number: 5, kind: protoreflect.StringKind},
			{name: "image_pull_secrets", number: 6, kind: protoreflect.MessageKind, repeated: true, ref: "k3sm.runtime.v1.LocalObjectReference"},
		})
	})

	t.Run("the response carries the manifest digest and the committed layer digests", func(t *testing.T) {
		assertExactFields(t, (&PullArtifactResponse{}).ProtoReflect().Descriptor(), []wireField{
			{name: "manifest_digest", number: 1, kind: protoreflect.StringKind},
			{name: "layer_digests", number: 2, kind: protoreflect.StringKind, repeated: true},
			{name: "error", number: 3, kind: protoreflect.MessageKind, ref: "google.rpc.Status"},
		})
	})

	t.Run("the consumer policy is a closed enum with no default", func(t *testing.T) {
		ed := ArtifactPolicy(0).Descriptor()
		want := map[protoreflect.Name]protoreflect.EnumNumber{
			"ARTIFACT_POLICY_UNSPECIFIED":  0,
			"ARTIFACT_POLICY_IMAGE_VOLUME": 1,
		}
		if got := ed.Values().Len(); got != len(want) {
			t.Errorf("ArtifactPolicy has %d values, want exactly %d", got, len(want))
		}
		for name, num := range want {
			v := ed.Values().ByName(name)
			if v == nil {
				t.Errorf("ArtifactPolicy.%s is missing", name)
				continue
			}
			if v.Number() != num {
				t.Errorf("ArtifactPolicy.%s = %d, want %d", name, v.Number(), num)
			}
		}
	})

	t.Run("the contract comments are present in the proto", func(t *testing.T) {
		raw, err := os.ReadFile("images.proto")
		if err != nil {
			t.Fatalf("read images.proto: %v", err)
		}
		src := string(raw)
		for _, want := range []string{
			"rpc PullArtifact(PullArtifactRequest) returns (PullArtifactResponse);",
			"never a relaxation of it",
			"Idempotent: a second call for an artifact already committed fetches no",
			"Resumable per layer",
			"survives an error and a daemon restart",
			"is refused before any byte is written",
			"UNSPECIFIED is refused",
			"never secret bytes on this wire",
		} {
			if !strings.Contains(src, want) {
				t.Errorf("images.proto lost the contract text %q", want)
			}
		}
	})
}
