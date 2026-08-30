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
// # Stability: THIS IS AN ALPHA API — INCOMPATIBLE CHANGES ARE ALLOWED
//
// This package is v1alpha1 and it means it. Fields may be renamed, retyped,
// given different defaults, or REMOVED in any release, and a stored object may
// require hand-editing across an upgrade. That is a deliberate divergence from
// net/v1's MeshPeer, which is served-v1 and additive-only: MeshPeer is an
// internal topology record written by k3sm and read by darwin-net, whereas
// MLXModel is the headline USER-FACING surface of MLX serving — the object a
// human writes by hand — and a user-facing API earns a graduation runway rather
// than being frozen at its first guess. Do not build anything on this package
// that cannot tolerate a breaking change at a minor release; when the shape has
// been proven against real workloads it graduates to a served v1 under the
// normal additive-only contract, and only then do the usual stability promises
// apply.
//
// Two exceptions to the alpha licence, because they are not this package's to
// break: the group name mlx.k3sm.io and the mlx.k3sm.io/* resource and label
// keys (see labels.go) are node- and scheduler-facing identifiers whose rename
// would break every already-labelled node and every already-scheduled pod. Treat
// those constants as stable even while the object shape is not.
//
// # What is NOT here
//
// The CEL validation that rejects a set spec.distributed lives in the CRD
// manifest (k3sm.io/apis/config/crd) and its contract test lives in
// k3sm.io/k3sm beside the CRD-ensure code, which already carries the
// apiextensions structural-schema dependencies. It is deliberately not tested
// here: a faithful CEL test would pull the apiextensions machinery into this
// module's intentionally minimal dependency graph.
//
// The internet-egress opt-in annotation is likewise NOT here. It is a
// k3sm.io/*-domain key that parameterizes the runtime contract's SandboxProfile
// and is not MLX-specific, so it lives with the other runtime-area keys in
// k3sm.io/apis/runtime/v1. This package holds only mlx.k3sm.io/* keys.
//
// The module still imports zero k3sm.io/* packages; k8s.io/apimachinery is pure
// Go, so this package builds CGO_ENABLED=0.
package mlxv1alpha1
