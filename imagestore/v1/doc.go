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
// that can already read Nodes, such as `k3sm image df`, needs no new RBAC to
// read it.
//
// The payload mirrors the proto field's semantics and carries no policy: the
// class keys partition the store, the values sum to the total, and no class is
// implied to be reclaimable, budgeted or eligible for anything.
//
// These are plain Go types, not a protobuf wire contract. The package is pure
// Go, imports only the standard library, and imports no k3sm.io/* package.
// It is v1, like the module's other contract packages, because its payload
// is read across releases: an incompatible Go API would move to a v2 package
// beside it rather than change this one under its importers. The annotation
// key uses the k3sm.io/ prefix because the image store has no API group of
// its own, and a key under the project's domain cannot collide with another
// owner's annotations on the same Node.
//
// # Trust
//
// A node can write its own annotations, so a reader treats the value as
// untrusted input. ParseClassReport fails closed: a byte bound checked before
// parsing, the schema read on its own, unknown, repeated and case-folded keys
// refused, then a class-count bound, the key grammar and the sum checked.
//
// ObservedAt is the node's claim of when it measured the store, not a time
// the reader observed. A reader shows a time ahead of its own clock as such
// (for example "in the future") rather than clamping it or computing a
// negative age.
//
// # Stability
//
// The annotation key is stable. Schema 1 is closed: a reader refuses unknown
// fields, so any added field, not only an incompatible change, needs a new
// ClassReportSchema. A reader of the old schema refuses the new one with
// ErrUnknownClassReportSchema rather than misreading it, and so shows the
// split as not reported. The guarantee that a writer never produces a value
// its reader rejects holds only between a writer and readers of the same
// schema.
package imagestorev1
