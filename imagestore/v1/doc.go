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

// Package imagestorev1 is the cross-repo contract for a node's image-store
// usage as published on its Node object: the AnnotationClassReport key, the
// ClassReport payload it carries, and the strict encoder and parser both ends
// use. The provider writes the annotation on its own Node from the runtime's
// ImageFsInfoResponse.store_bytes_by_class (k3sm.io/apis/runtime/v1); a reader
// such as `k3sm image df` reads it with the Node read access it already has,
// so no new RBAC is needed.
//
// The payload mirrors the proto field's semantics and carries no policy: the
// class keys partition the store, the values sum to the total, and no class is
// implied to be reclaimable, budgeted or eligible for anything.
//
// These are plain Go types, not a protobuf wire contract. The package is pure
// Go, imports only the standard library, and imports no k3sm.io/* package.
//
// # Trust
//
// A node can write its own annotations, so a reader treats the value as
// untrusted input. ParseClassReport fails closed: a byte bound checked before
// parsing, the schema read on its own, unknown, repeated and case-folded keys
// refused, then a class-count bound, the key grammar and the sum checked.
//
// # Stability
//
// The annotation key is stable. The payload shape changes only with a new
// ClassReportSchema, which a reader of the old schema refuses with
// ErrUnknownClassReportSchema rather than misreading.
package imagestorev1
