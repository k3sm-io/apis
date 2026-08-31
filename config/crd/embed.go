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

// Package crd embeds the k3sm CustomResourceDefinition manifests so a consumer
// applies the SAME bytes this module ships, with no shadow copy.
//
// A CRD manifest that is maintained here and separately pasted into the applying
// repo is two schemas: they agree until one is edited, and the disagreement
// surfaces as an object that validates in one place and is rejected in another.
// Embedding removes the second copy entirely — k3sm's server-side-apply ensure
// reads these bytes.
//
// # Named accessors, deliberately not a glob
//
// Each manifest is embedded by NAME and exposed by its own accessor. There is no
// embed.FS and no *.yaml pattern, because a glob makes "which CRDs does k3sm
// apply" a property of what happens to be in this directory: dropping a file in
// would silently enlist it, and adopting a CRD into the applied set must be a
// deliberate, reviewable act.
//
// The MeshPeer manifest beside this file (net.k3sm.io_meshpeers.yaml) now has
// such an accessor, added as exactly the deliberate act that rule demands. It
// previously had none, on the belief that MeshPeer was applied out-of-band by
// the bootstrap path — and that belief was wrong: nothing applied it, so every
// worker join failed at the enroll write until someone installed the CRD by
// hand. k3sm's server provisions it fail-closed on the mesh path, and the
// mesh-regression check that adopting it owes rides that consumer change's gate.
package crd

import (
	_ "embed"
)

// MLXModelCRDName is the metadata.name of the MLXModel CustomResourceDefinition
// (the <plural>.<group> form the API server addresses it by). Published so a
// consumer waiting on or reading back the CRD does not spell the string.
const MLXModelCRDName = "mlxmodels.mlx.k3sm.io"

//go:embed mlx.k3sm.io_mlxmodels.yaml
var mlxModelCRDYAML string

// MLXModelCRD returns the mlx.k3sm.io MLXModel CustomResourceDefinition manifest
// as YAML bytes, ready to apply.
//
// It returns a FRESH COPY on every call. The embedded manifest is process-global
// state, and a caller that decodes in place, appends, or otherwise scribbles on
// the returned slice would corrupt what every later caller applies — including
// the one that re-applies during a reconcile loop.
func MLXModelCRD() []byte {
	return []byte(mlxModelCRDYAML)
}

// MeshPeerCRDName is the metadata.name of the MeshPeer CustomResourceDefinition
// (the <plural>.<group> form the API server addresses it by). Published so a
// consumer waiting on or reading back the CRD does not spell the string.
const MeshPeerCRDName = "meshpeers.net.k3sm.io"

//go:embed net.k3sm.io_meshpeers.yaml
var meshPeerCRDYAML string

// MeshPeerCRD returns the net.k3sm.io MeshPeer CustomResourceDefinition manifest
// as YAML bytes, ready to apply.
//
// It returns a FRESH COPY on every call. The embedded manifest is process-global
// state, and a caller that decodes in place, appends, or otherwise scribbles on
// the returned slice would corrupt what every later caller applies — including
// the one that re-applies during a reconcile loop.
func MeshPeerCRD() []byte {
	return []byte(meshPeerCRDYAML)
}
