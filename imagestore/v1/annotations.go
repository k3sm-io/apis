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

package imagestorev1

// The image store has no custom resource and so no API group of its own: its
// key sits under the k3sm.io/* prefix the rest of this module uses for labels
// and annotations, published here so no writer or reader spells a literal.
//
// The key is stable: renaming it strands every Node annotated under it.
const (
	// AnnotationClassReport is the Node annotation a node writes on its own
	// Node: a ClassReport payload (Encode, ParseClassReport) giving its image
	// store's bytes per content class.
	AnnotationClassReport = "k3sm.io/image-store-classes"
)

// The reserved class keys, as the runtime emits them in
// ImageFsInfoResponse.store_bytes_by_class. Every other key is a media type
// of the form type "/" subtype; no reserved key contains "/".
const (
	// ClassImage counts content an image names.
	ClassImage = "image"
	// ClassArtifact counts artifact bytes with no recorded media type, or
	// whose type is not a media type of at most MaxClassKeyBytes bytes.
	ClassArtifact = "artifact"
	// ClassUnattributed counts content nothing names.
	ClassUnattributed = "unattributed"
)
