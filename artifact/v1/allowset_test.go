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

package artifactv1

import (
	"slices"
	"strings"
	"testing"
)

// TestMediaTypeConstants pins the wire spellings. They are the strings in
// every pushed manifest; a typo here makes the pusher and the runtime agree
// with each other and with nothing else.
func TestMediaTypeConstants(t *testing.T) {
	for _, tc := range []struct{ name, got, want string }{
		{"ArtifactTypeModel", ArtifactTypeModel, "application/vnd.cncf.model.manifest.v1+json"},
		{"MediaTypeModelConfig", MediaTypeModelConfig, "application/vnd.cncf.model.config.v1+json"},
		{"MediaTypeModelWeight", MediaTypeModelWeight, "application/vnd.cncf.model.weight.v1.raw"},
		{"MediaTypeModelWeightConfig", MediaTypeModelWeightConfig, "application/vnd.cncf.model.weight.config.v1.raw"},
		{"MediaTypeModelDoc", MediaTypeModelDoc, "application/vnd.cncf.model.doc.v1.raw"},
		{"MediaTypeModelCode", MediaTypeModelCode, "application/vnd.cncf.model.code.v1.raw"},
		{"AnnotationFilepath", AnnotationFilepath, "org.cncf.model.filepath"},
		{"MediaTypeWasmConfig", MediaTypeWasmConfig, "application/vnd.wasm.config.v0+json"},
		{"MediaTypeWasmLayer", MediaTypeWasmLayer, "application/wasm"},
		{"ArtifactTypeGuest", ArtifactTypeGuest, "application/vnd.k3sm.guest.v1"},
		{"MediaTypeGuestKernel", MediaTypeGuestKernel, "application/vnd.k3sm.guest.kernel.v1.raw"},
		{"MediaTypeGuestInitramfs", MediaTypeGuestInitramfs, "application/vnd.k3sm.guest.initramfs.v1.raw"},
		{"MediaTypeOCIImageConfig", MediaTypeOCIImageConfig, "application/vnd.oci.image.config.v1+json"},
		{"AnnotationArtifactPaths", AnnotationArtifactPaths, "k3sm.io/artifact-paths"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// TestArtifactType pins the OCI 1.1 rule: artifactType, else the config media
// type.
func TestArtifactType(t *testing.T) {
	if got := ArtifactType(ArtifactTypeModel, MediaTypeModelConfig); got != ArtifactTypeModel {
		t.Errorf("with artifactType set: %q, want %q", got, ArtifactTypeModel)
	}
	if got := ArtifactType("", MediaTypeWasmConfig); got != MediaTypeWasmConfig {
		t.Errorf("with artifactType empty: %q, want the config media type %q", got, MediaTypeWasmConfig)
	}
}

// TestImageVolumeAllowSet pins what a pod image volume accepts: the model and
// wasm types, the model's weight, weight-config and doc layers, and never the
// code layer.
func TestImageVolumeAllowSet(t *testing.T) {
	a := ImageVolumeAllowSet()
	if got, want := a.ArtifactTypes(), []string{ArtifactTypeModel, MediaTypeWasmConfig}; !slices.Equal(got, want) {
		t.Errorf("artifact types = %v, want %v", got, want)
	}
	for _, tc := range []struct {
		name, typ, layer string
		want             bool
	}{
		{"model weight", ArtifactTypeModel, MediaTypeModelWeight, true},
		{"model weight config", ArtifactTypeModel, MediaTypeModelWeightConfig, true},
		{"model doc", ArtifactTypeModel, MediaTypeModelDoc, true},
		{"model code is refused", ArtifactTypeModel, MediaTypeModelCode, false},
		{"model config is not a layer", ArtifactTypeModel, MediaTypeModelConfig, false},
		{"tar weight is refused", ArtifactTypeModel, "application/vnd.cncf.model.weight.v1.tar", false},
		{"wasm module", MediaTypeWasmConfig, MediaTypeWasmLayer, true},
		{"wasm with a model layer", MediaTypeWasmConfig, MediaTypeModelWeight, false},
		{"model with a wasm layer", ArtifactTypeModel, MediaTypeWasmLayer, false},
		{"guest type is not a volume", ArtifactTypeGuest, MediaTypeGuestKernel, false},
		{"image config is not an artifact", MediaTypeOCIImageConfig, "application/vnd.oci.image.layer.v1.tar", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := a.AllowsLayer(tc.typ, tc.layer); got != tc.want {
				t.Errorf("AllowsLayer(%q, %q) = %v, want %v", tc.typ, tc.layer, got, tc.want)
			}
		})
	}
	if a.AllowsType(ArtifactTypeGuest) {
		t.Error("the image-volume set admits the guest type; guest artifacts use an internal entry point")
	}
}

// TestGuestAllowSetIsSeparate pins that the guest set holds only the guest
// type, and that neither set admits the other's types.
func TestGuestAllowSetIsSeparate(t *testing.T) {
	g := GuestAllowSet()
	if got, want := g.ArtifactTypes(), []string{ArtifactTypeGuest}; !slices.Equal(got, want) {
		t.Errorf("guest artifact types = %v, want %v", got, want)
	}
	if got, want := g.LayerMediaTypes(ArtifactTypeGuest), []string{MediaTypeGuestKernel, MediaTypeGuestInitramfs}; !slices.Equal(got, want) {
		t.Errorf("guest layers = %v, want %v", got, want)
	}
	for _, typ := range ImageVolumeAllowSet().ArtifactTypes() {
		if g.AllowsType(typ) {
			t.Errorf("the guest set admits the image-volume type %q", typ)
		}
	}
	if g.Name() == ImageVolumeAllowSet().Name() {
		t.Error("the guest and image-volume sets share a name")
	}
}

// TestNoAllowSetAdmitsTarCodeOrImageConfig is the fail-closed sweep: no set
// admits an OCI image config as a type, and no set admits a tar, compressed
// or code layer.
func TestNoAllowSetAdmitsTarCodeOrImageConfig(t *testing.T) {
	var zero AllowSet
	if zero.AllowsType(ArtifactTypeModel) || len(zero.ArtifactTypes()) != 0 {
		t.Error("the zero AllowSet admits something; it must admit nothing")
	}
	for _, a := range []AllowSet{ImageVolumeAllowSet(), GuestAllowSet()} {
		if a.AllowsType(MediaTypeOCIImageConfig) {
			t.Errorf("%s admits the OCI image config", a.Name())
		}
		for _, typ := range a.ArtifactTypes() {
			for _, l := range a.LayerMediaTypes(typ) {
				if strings.Contains(l, "tar") || strings.Contains(l, "gzip") || strings.Contains(l, "zstd") {
					t.Errorf("%s admits the archive layer %q for %q", a.Name(), l, typ)
				}
				if l == MediaTypeModelCode {
					t.Errorf("%s admits the code layer for %q", a.Name(), typ)
				}
			}
		}
	}
}

// TestAllowSetIsNotMutableThroughItsAccessors pins that the slices handed out
// are copies: a caller that appends to one cannot widen the set.
func TestAllowSetIsNotMutableThroughItsAccessors(t *testing.T) {
	a := ImageVolumeAllowSet()
	layers := a.LayerMediaTypes(ArtifactTypeModel)
	layers[0] = MediaTypeModelCode
	if a.AllowsLayer(ArtifactTypeModel, MediaTypeModelCode) {
		t.Error("editing LayerMediaTypes' result widened the set")
	}
	b := ImageVolumeAllowSet()
	b.LayerMediaTypes(ArtifactTypeModel)[0] = MediaTypeModelCode
	if ImageVolumeAllowSet().AllowsLayer(ArtifactTypeModel, MediaTypeModelCode) {
		t.Error("editing one ImageVolumeAllowSet widened the next")
	}
}

// TestArtifactPathsAnnotationBounds pins the annotation's bounds.
func TestArtifactPathsAnnotationBounds(t *testing.T) {
	if MaxArtifactPathsAnnotationBytes != 64*1024 {
		t.Errorf("MaxArtifactPathsAnnotationBytes = %d, want 65536", MaxArtifactPathsAnnotationBytes)
	}
	if MaxArtifactPathsAnnotationPaths != 4096 {
		t.Errorf("MaxArtifactPathsAnnotationPaths = %d, want 4096", MaxArtifactPathsAnnotationPaths)
	}
	if MaxPaths != MaxArtifactPathsAnnotationPaths {
		t.Errorf("MaxPaths = %d, want the annotation bound %d", MaxPaths, MaxArtifactPathsAnnotationPaths)
	}
}
