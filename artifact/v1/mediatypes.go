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

// ModelPack media types (CNCF modelpack/model-spec). The k3sm profile is a
// subset, not a fork: one raw layer per file, each carrying its file path in
// AnnotationFilepath, so a ModelPack tool can read what k3sm writes.
const (
	// ArtifactTypeModel is the ModelPack manifest artifactType.
	ArtifactTypeModel = "application/vnd.cncf.model.manifest.v1+json"
	// MediaTypeModelConfig is the ModelPack config blob media type. Config
	// blobs are digest-verified and stored, never parsed on the node.
	MediaTypeModelConfig = "application/vnd.cncf.model.config.v1+json"
	// MediaTypeModelWeight is a raw (unarchived, uncompressed) weight file.
	MediaTypeModelWeight = "application/vnd.cncf.model.weight.v1.raw"
	// MediaTypeModelWeightConfig is a raw weight-adjacent config file: the
	// tokenizer, config.json, the safetensors index.
	MediaTypeModelWeightConfig = "application/vnd.cncf.model.weight.config.v1.raw"
	// MediaTypeModelDoc is a raw documentation file (README, licence).
	MediaTypeModelDoc = "application/vnd.cncf.model.doc.v1.raw"
	// MediaTypeModelCode is a raw code file. It is named so it can be refused:
	// no code rides as a model, so it is in no allow-set.
	MediaTypeModelCode = "application/vnd.cncf.model.code.v1.raw"

	// AnnotationFilepath is the layer annotation that carries the layer's
	// file path inside the artifact. Required on every model layer; its value
	// obeys ValidatePath.
	AnnotationFilepath = "org.cncf.model.filepath"
)

// Wasm media types (the CNCF wasm OCI artifact). Its manifest carries no
// artifactType, so the type is the config media type (see ArtifactType).
const (
	// MediaTypeWasmConfig is the wasm artifact config media type, and so its
	// artifact type.
	MediaTypeWasmConfig = "application/vnd.wasm.config.v0+json"
	// MediaTypeWasmLayer is a wasm module layer.
	MediaTypeWasmLayer = "application/wasm"
)

// Guest media types: the pinned guest kernel and initramfs the server
// publishes to its own registry. They are fetched through an internal entry
// point, never through a pod image volume.
const (
	// ArtifactTypeGuest is the guest artifact's artifactType.
	ArtifactTypeGuest = "application/vnd.k3sm.guest.v1"
	// MediaTypeGuestKernel is the raw guest kernel image layer.
	MediaTypeGuestKernel = "application/vnd.k3sm.guest.kernel.v1.raw"
	// MediaTypeGuestInitramfs is the raw guest initramfs layer.
	MediaTypeGuestInitramfs = "application/vnd.k3sm.guest.initramfs.v1.raw"
)

// MediaTypeOCIImageConfig is the OCI image config media type. It is in no
// allow-set, so an image can never be mounted as an artifact.
const MediaTypeOCIImageConfig = "application/vnd.oci.image.config.v1+json"

// ArtifactType returns a manifest's artifact type under the OCI 1.1 rule: the
// manifest's artifactType when set, else its config media type.
func ArtifactType(manifestArtifactType, configMediaType string) string {
	if manifestArtifactType != "" {
		return manifestArtifactType
	}
	return configMediaType
}
