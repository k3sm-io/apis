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

// Package keystorev1alpha1 holds the keystore.k3sm.io/v1alpha1 API: the
// KeyDelivery custom resource, a namespaced declaration that delivers one
// user-scope keystore entry into its namespace as a Secret.
//
// # Shape
//
// Spec.Entry names the entry (user/<uid>/<name>, the grammar of
// k3sm.io/apis/keystore/entry; a system entry is refused, since a system entry
// reaches a namespace only through root's deliver). Spec.SecretName is the
// target Secret. Spec.Items are the projections: each writes one credential
// field (k3sm.io/apis/keystore/credential, which no refresh field is in)
// under one Secret data key; there is no default, because the apiserver
// cannot see the entry's kind. Spec.RequestedBy is the local principal
// k3sm:local:<uid> that declared it, which for a user entry must name the
// entry's own uid. All four are required, and all four are immutable after
// create through CRD CEL transition rules (self == oldSelf; for Items a
// map comparison, so a reorder is not a change), so the CRD holds its shape
// without an admission policy. A transition rule does not run when the old
// value is absent; that is why none of them is optional.
//
// # Status: conditions first
//
// The status subresource is on. Status.Conditions (Authorized, Ready,
// SecretRetained, and CredentialValid for kinds with an expiry; one per type,
// each with its observedGeneration) are the source of truth, with
// Status.ObservedGeneration and Status.DeliveredVersion beside them. Status
// never carries a value or a fingerprint of one. The condition reasons are a
// closed set, listed in the CRD schema description and as the Reason*
// constants here; a reader treats a reason it does not know as a generic
// failure of the condition carrying it, so a reason added later breaks no
// reader. "No such entry" and "not yours" share one reason,
// ReasonEntryUnavailable, so status reveals nothing about existence. Ready
// means delivered: it does not change on expiry or revocation, and health
// readers read CredentialValid.
//
// # Neither the schema nor admission is the authority
//
// The schema checks shape and immutability only. The authority is the
// owner's sealed delivery record and the reconciler-time access reviews in
// k3sm; RequestedBy is informational.
//
// # Stability: alpha
//
// A single version, v1alpha1, is served and stored. Fields may change
// incompatibly, and a conversion is owed if they do. The CRD manifest is
// k3sm.io/apis/config/crd (keystore.k3sm.io_keydeliveries.yaml),
// hand-authored to match these types; the types carry hand-written
// DeepCopy* methods, as every CRD type in this module does.
//
// The module still imports zero k3sm.io/* packages outside itself, and this
// package builds CGO_ENABLED=0.
package keystorev1alpha1
