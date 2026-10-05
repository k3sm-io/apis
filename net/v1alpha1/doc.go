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

// Package netv1alpha1 holds the net.k3sm.io/v1alpha1 API: the DirectLink custom
// resource that records a node's point-to-point cable ports, the LinkIP
// derivation that gives each cabled port its address, the pairing wire types a
// new Mac uses to join over such a cable, and the direct-link node-label keys.
//
// DirectLink is a real Kubernetes custom resource (kine-stored, apiserver-served
// and -watched), written by each node for itself and resolved into a link graph
// by the k3sm server. Being a served object it embeds metav1.TypeMeta +
// ObjectMeta and carries hand-written DeepCopy*/DeepCopyObject methods (this
// module runs no code generation), the same shape net/v1's MeshPeer uses.
//
// # Stability: alpha — incompatible changes are allowed
//
// The object shapes in this package may be renamed, retyped, given different
// defaults, or removed in any release. net.k3sm.io/v1 keeps only the stable
// MeshPeer; DirectLink lives here until its behaviour is proven on real cables.
// Three things are stable regardless, because a change would strand state that
// no version bump can migrate: the LinkIP derivation (an address already
// configured on a cable and recorded in a peer's routes), the k3sm.io/* label
// keys in labels.go, and the version fields of the pairing wire types.
//
// The names here are medium-agnostic. "thunderbolt" is a Medium value and a
// label value, never part of a Go identifier or a key, so a future
// point-to-point medium is a new value, not a new type.
//
// The module still imports zero k3sm.io/* packages; k8s.io/apimachinery is pure
// Go, so this package builds CGO_ENABLED=0.
package netv1alpha1
