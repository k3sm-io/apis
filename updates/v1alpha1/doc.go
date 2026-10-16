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

// Package updatesv1alpha1 holds the updates.k3sm.io/v1alpha1 API: the Update
// custom resource, the request to move a whole k3sm cluster to one release.
//
// An Update is a cluster-scoped, kine-stored, apiserver-served object. Its spec
// names the release tag to install (Version), how the roll proceeds across the
// nodes (Strategy: workers first, the server last, at most MaxUnavailable nodes
// at a time, optionally draining each one), what the downloaded artifacts must
// prove before they are installed (Verify: the published checksums and the
// release signature), whether moving to an older release is allowed
// (AllowDowngrade), and a Cancel switch that stops the roll before the next
// node. Its status is written by the update controller in k3sm.io/k3sm through
// the status subresource: an overall Phase, one entry per node with the
// release it moved from and to and the step it is on, the standard
// conditions, and the generation the status describes.
//
// # One at a time
//
// A cluster runs at most one roll. Nothing in this package or in the CRD
// schema enforces that, because a schema cannot see other objects; the
// controller refuses a second Update while one is not yet in a terminal phase
// (Succeeded, Failed or Cancelled) and reports the refusal on the second
// object's conditions.
//
// Being a served object it embeds metav1.TypeMeta + ObjectMeta and carries
// hand-written DeepCopy*/DeepCopyObject methods (this module runs no code
// generation), the same shape mlx/v1alpha1's MLXModel and net/v1alpha1's
// Interconnect use. The CRD manifest is k3sm.io/apis/config/crd
// (updates.k3sm.io_updates.yaml), hand-authored to match these types.
//
// # Stability: alpha
//
// A single version, v1alpha1, is served and stored. Fields may change
// incompatibly until the shape is proven by real cluster updates; it then
// graduates to a served v1 under the additive-only contract. The group name
// updates.k3sm.io is stable from the start.
//
// The module still imports zero k3sm.io/* packages; k8s.io/apimachinery is pure
// Go, so this package builds CGO_ENABLED=0.
package updatesv1alpha1
