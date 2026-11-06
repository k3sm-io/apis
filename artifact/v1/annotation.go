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

// AnnotationArtifactPaths is the pod annotation that selects, per image
// volume, the subset of an artifact's file paths a pod mounts (a pipeline
// rank's weight files). Its value is a JSON object from volume name to a list
// of paths: {"<volume>":["<path>",...]}. The provider alone parses it, once
// at pod creation, into ImageVolumeSource.paths; each list obeys
// ValidatePathSet, and a path the manifest does not carry refuses the pod.
const AnnotationArtifactPaths = "k3sm.io/artifact-paths"

// Bounds on the AnnotationArtifactPaths value, checked before it is decoded
// (bytes) and while it is decoded (paths).
const (
	// MaxArtifactPathsAnnotationBytes is the largest annotation value accepted.
	MaxArtifactPathsAnnotationBytes = 64 << 10
	// MaxArtifactPathsAnnotationPaths is the most paths the annotation may
	// list, across all volumes.
	MaxArtifactPathsAnnotationPaths = 4096
)
