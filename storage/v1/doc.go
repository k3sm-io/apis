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

// Package storagev1 holds the cross-repo Go contract for k3sm's APFS
// local-path persistent storage: the StorageClass / provisioner
// parameters and the PV node-affinity / topology that the local-path
// provisioner controller (k3sm.io/k3sm, the controller that creates
// PersistentVolumes) and the runtime binder (k3sm.io/runtimed, which mounts the
// per-PVC dir into the pod) must agree on — see k3sm/docs/DESIGN.md §5.
//
// The upstream storage.k8s.io StorageClass and core/v1 PersistentVolume objects
// remain the Kubernetes API surface the provisioner reads and writes; this
// package does NOT vendor or redefine them. It carries only the small k3sm
// agreement that genuinely crosses the repo boundary: the provisioner identity,
// the on-APFS storage root, the reclaim / binding policy k3sm supports, and the
// node-topology key used to pin a local PV (and thus its StatefulSet pod) to its
// owning node. The PV/PVC volume *source* a pod mounts is the proto
// PersistentVolumeClaimVolumeSource in k3sm.io/apis/runtime/v1 (carried by
// CreatePod); this package complements it with the provisioner-side contract.
//
// Like net/v1 these are plain Go structs, NOT a protobuf wire contract: shared
// in-process configuration with small construction/validation helpers, camelCase
// JSON tags, additive-only, and zero k3sm.io/* imports (it builds pure-Go,
// CGO_ENABLED=0).
//
// Stability contract: the surface is ADDITIVE-ONLY. Existing exported fields and
// the documented constants are stable; new optional fields may be appended.
package storagev1
