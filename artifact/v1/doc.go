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

// Package artifactv1 is the one home of the rules every k3sm component applies
// to an OCI artifact (a platform-less manifest such as a ModelPack model or a
// wasm module) stored in the cluster registry: the media-type constants, the
// per-consumer allow-sets, the file-path validator, and the pod annotation
// that selects a per-rank subset of an artifact's files.
//
// The pusher, the operator, the provider, the runtime and the registry GC all
// import these; none mirrors them. A rule that lived in two places would drift,
// and the drift would surface as an artifact one side accepts and the other
// refuses, or worse, the reverse.
//
// The package is pure Go. Outside the standard library it imports only
// k3sm.io/apis/runtime/v1, for the ArtifactPolicy enum AllowSetFor maps (the
// runtime package does not import this one). The media types are string constants: the modelpack/model-spec Go module is
// deliberately not imported for a handful of strings.
//
// # Trust
//
// File paths come from a stranger's manifest (the org.cncf.model.filepath
// layer annotation) or from a pod author (the k3sm.io/artifact-paths
// annotation). ValidatePath and ValidatePathSet are therefore fail-closed
// parsers of untrusted input: a closed ASCII charset, bounded depth and
// component length, and refusal of anything they do not recognise. The pusher
// applies them before it reads a byte, and the runtime applies them again at
// fetch, never trusting the pusher.
package artifactv1
