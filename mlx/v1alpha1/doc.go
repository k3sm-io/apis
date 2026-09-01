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

// Package mlxv1alpha1 holds the mlx.k3sm.io/v1alpha1 API: the MLXModel custom
// resource and the mlx.k3sm.io node-advertisement constants for k3sm's native
// Apple-Silicon ML serving.
//
// An MLXModel is a real Kubernetes custom resource — kine-stored,
// apiserver-served and -watched — reconciled by the MLX operator in
// k3sm.io/k3sm. Being a served object it embeds metav1.TypeMeta + ObjectMeta and
// carries hand-written DeepCopy*/DeepCopyObject methods (this module runs no
// code generation), the same shape net/v1's MeshPeer uses.
//
// # Stability: alpha — incompatible changes are allowed
//
// Fields may be renamed, retyped, given different defaults, or removed in any
// release, and a stored object may require hand-editing across an upgrade.
// Unlike net/v1's MeshPeer (served-v1, additive-only, an internal topology
// record), MLXModel is the user-facing surface of MLX serving and earns a
// graduation runway before it freezes: it graduates to served v1 under the
// normal additive-only contract once its shape is proven against real
// workloads. Do not build anything on this package that cannot tolerate a
// breaking change at a minor release.
//
// Two exceptions stay stable even while the object shape is not, because a
// rename would break every already-labelled node and every already-scheduled
// pod: the group name mlx.k3sm.io and the mlx.k3sm.io/* resource and label
// keys (see labels.go).
//
// # What is NOT here
//
// The CEL validation that rejects a set spec.distributed lives in the CRD
// manifest (k3sm.io/apis/config/crd); its contract test lives in
// k3sm.io/k3sm beside the CRD-ensure code, which already carries the
// apiextensions structural-schema dependencies this module avoids.
//
// The internet-egress opt-in annotation is also not here: it parameterizes
// the runtime contract's SandboxProfile and is not MLX-specific, so it lives
// with the other runtime-area keys in k3sm.io/apis/runtime/v1. This package
// holds only mlx.k3sm.io/* keys.
//
// The module still imports zero k3sm.io/* packages; k8s.io/apimachinery is pure
// Go, so this package builds CGO_ENABLED=0.
package mlxv1alpha1
