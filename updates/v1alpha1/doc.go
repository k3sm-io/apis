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
// prove before they are installed (Verify: the code-signature policy; the
// checksum is always verified), whether moving to an older release is allowed
// (AllowDowngrade), and a Cancel switch that stops the roll before the next
// node. Its status is written by the update controller in k3sm.io/k3sm through
// the status subresource: the standard conditions, the release that was
// verified, one entry per node with the release it moved from and to and the
// step it is on, a derived overall Phase, and the generation the status
// describes. The Node annotation keys the controller and the per-node root
// helper share are in annotations.go.
//
// # One at a time: the singleton
//
// A cluster runs at most one roll, so the update controller reconciles one
// name only, UpdateName ("current"). An Update under any other name is
// ignored: its UpdateConditionReconciled condition is set False with the
// reason UpdateReasonNotTheSingleton and no node is touched. The schema
// cannot see other objects, so this is the controller's rule, not the CRD's.
// Spec.Version is immutable after create and Spec.Cancel is one-way (false to
// true); both are CEL transition rules on the CRD.
//
// # Conditions are the source of truth
//
// Status.Conditions are the contract. Status.Phase and each node's Phase are
// derived, display-only summaries; the CRD schema carries no closed enum for
// them, and readers tolerate values they do not know and never branch on
// them. Status.Resolved records the release tag and payload digest that were
// actually verified.
//
// # Verification
//
// The payload's checksum is always verified against the release's published
// checksums; there is no switch for it. Spec.Verify.Signature then sets the
// code-signature policy. Under if-signed, the default, the payload is matched
// to the installed build: an installed build that carries a Developer ID team
// signature requires every Mach-O in the payload to carry the same team's
// signature, and an ad-hoc signed installed build accepts an ad-hoc signed
// payload. Under required, any payload whose Mach-Os do not carry a team
// signature is refused, whatever the installed build carries.
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
