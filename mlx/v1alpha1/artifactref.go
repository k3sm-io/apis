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

package mlxv1alpha1

// Bounds on ArtifactRef, mirrored as maxLength in the MLXModel CRD.
const (
	// MaxArtifactReferenceLength is the longest ArtifactRef.Reference.
	MaxArtifactReferenceLength = 512
	// MaxArtifactVariantLength is the longest ArtifactRef.Variant.
	MaxArtifactVariantLength = 128
)

// ArtifactRef names a model stored as an OCI artifact in the cluster registry.
// It is the one shape every model kind embeds (MLXModel today, LlamaModel
// when it lands), so the operator, the pusher and the runtime agree on what a
// stored model reference is.
//
// It holds only plain strings, so a value copy is a deep copy.
type ArtifactRef struct {
	// Reference is the OCI reference of the artifact in the cluster registry,
	// by tag or by digest (registry/repo:tag or registry/repo@sha256:<hex>).
	// Required. The registry host must be the cluster registry; the provider
	// and the runtime refuse any other host. At most
	// MaxArtifactReferenceLength bytes.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Reference string `json:"reference"`

	// Variant selects one entry of an artifact index by its variant annotation
	// (for example "4bit"). Empty means the reference names a single manifest,
	// and an index without a variant is refused. At most
	// MaxArtifactVariantLength bytes.
	//
	// +optional
	// +kubebuilder:validation:MaxLength=128
	Variant string `json:"variant,omitempty"`
}
