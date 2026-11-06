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

import "slices"

// typeRule is what one accepted artifact type admits.
type typeRule struct {
	artifactType string
	layers       []string
}

// AllowSet is a closed set of acceptable artifact types, each with the closed
// set of layer media types it admits. It is chosen by consumer, never built
// from a caller's list, so a caller cannot widen what the runtime accepts by
// naming a media type. The zero value admits nothing.
//
// Only raw layer types appear in any set: tar layers need an extractor, a
// decompressor of untrusted input, and extraction is what in-place reads
// exist to avoid.
type AllowSet struct {
	name  string
	rules []typeRule
}

// modelLayers are the ModelPack layer types a model may carry. The code type
// (MediaTypeModelCode) is deliberately absent: no code rides as a model.
var modelLayers = []string{
	MediaTypeModelWeight,
	MediaTypeModelWeightConfig,
	MediaTypeModelDoc,
}

// ImageVolumeAllowSet returns the allow-set for a pod image volume: the
// ModelPack model type and the wasm module type, nothing else.
func ImageVolumeAllowSet() AllowSet {
	return AllowSet{name: "image-volume", rules: []typeRule{
		{artifactType: ArtifactTypeModel, layers: slices.Clone(modelLayers)},
		{artifactType: MediaTypeWasmConfig, layers: []string{MediaTypeWasmLayer}},
	}}
}

// GuestAllowSet returns the allow-set for the guest kernel and initramfs. It
// is kept apart from ImageVolumeAllowSet: guest artifacts are fetched through
// an internal entry point, and no pod may mount one.
func GuestAllowSet() AllowSet {
	return AllowSet{name: "guest", rules: []typeRule{
		{artifactType: ArtifactTypeGuest, layers: []string{MediaTypeGuestKernel, MediaTypeGuestInitramfs}},
	}}
}

// Name returns the consumer name of the set ("image-volume", "guest"), for
// error messages and logs; empty for the zero value.
func (a AllowSet) Name() string { return a.name }

// AllowsType reports whether artifactType is in the set.
func (a AllowSet) AllowsType(artifactType string) bool {
	return a.rule(artifactType) != nil
}

// AllowsLayer reports whether layerMediaType is admitted for artifactType.
// An artifact type outside the set admits no layer.
func (a AllowSet) AllowsLayer(artifactType, layerMediaType string) bool {
	r := a.rule(artifactType)
	return r != nil && slices.Contains(r.layers, layerMediaType)
}

// ArtifactTypes returns the artifact types in the set, as a fresh slice.
func (a AllowSet) ArtifactTypes() []string {
	out := make([]string, 0, len(a.rules))
	for _, r := range a.rules {
		out = append(out, r.artifactType)
	}
	return out
}

// LayerMediaTypes returns the layer media types admitted for artifactType, as
// a fresh slice; nil when the type is outside the set.
func (a AllowSet) LayerMediaTypes(artifactType string) []string {
	r := a.rule(artifactType)
	if r == nil {
		return nil
	}
	return slices.Clone(r.layers)
}

func (a AllowSet) rule(artifactType string) *typeRule {
	for i := range a.rules {
		if a.rules[i].artifactType == artifactType {
			return &a.rules[i]
		}
	}
	return nil
}
