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

package storagev1

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Defaults for the k3sm APFS local-path provisioner. They are the values the
// provisioner controller stamps onto the StorageClass it owns and that runtimed
// resolves a per-PVC dir against, so both repos derive identical paths.
const (
	// DefaultStorageClassName is the name of the k3sm local-path StorageClass.
	DefaultStorageClassName = "local-path"
	// ProvisionerName is the StorageClass.provisioner value identifying the k3sm
	// local-path provisioner controller (k3sm:M3). It is the reverse-DNS-free
	// k3sm.io/* form Kubernetes uses for in-tree-style provisioner names.
	ProvisionerName = "k3sm.io/local-path"
	// DefaultBasePath is the APFS storage root under which per-PVC dirs live. It
	// is on the SAME APFS volume as /var/lib/k3sm (kine's SQLite shares it), so a
	// runaway PVC can fill the datastore volume — capacity is not enforced vs
	// free space (over-commit → write-time ENOSPC); see docs/m3-plan.md.
	DefaultBasePath = "/var/lib/k3sm/storage"
	// TopologyKeyHostname is the node-label key a local PV's node-affinity and the
	// scheduler pin against — the standard well-known hostname label. The
	// provisioner stamps it onto PersistentVolume.spec.nodeAffinity so a
	// StatefulSet pod reschedules to the SAME Mac and mounts its existing data.
	TopologyKeyHostname = "kubernetes.io/hostname"
)

// ReclaimPolicy is the PersistentVolume reclaim policy. It mirrors the
// corev1.PersistentVolumeReclaimPolicy string so it round-trips through the
// Kubernetes objects the provisioner writes unchanged.
type ReclaimPolicy string

const (
	// ReclaimRetain keeps the backing dir after the PVC is deleted (manual
	// reclamation). It is the ONLY policy k3sm M3 supports: there is no
	// volume-delete RPC, and root-rmdir'ing the dir would bypass the pod SBPL
	// deny-set — see docs/m3-plan.md (runtimed:M3.1).
	ReclaimRetain ReclaimPolicy = "Retain"
	// ReclaimDelete is the upstream delete-on-release policy. It is a well-formed
	// value but is NOT supported by k3sm M3 (LocalPathClass.Validate rejects it).
	ReclaimDelete ReclaimPolicy = "Delete"
)

// Valid reports whether p is a well-formed reclaim policy value (Retain or
// Delete). It does not assert M3 support — LocalPathClass.Validate enforces the
// Retain-only invariant.
func (p ReclaimPolicy) Valid() bool {
	switch p {
	case ReclaimRetain, ReclaimDelete:
		return true
	default:
		return false
	}
}

// VolumeBindingMode is when a PersistentVolume bound to a PVC of this class is
// provisioned. It mirrors the storagev1.VolumeBindingMode string.
type VolumeBindingMode string

const (
	// BindingWaitForFirstConsumer delays provisioning until a pod that uses the
	// PVC is scheduled, so the PV is created on (and node-affinity-pinned to) that
	// pod's node. It is the ONLY mode k3sm M3 supports — a local PV pinned to the
	// wrong Mac mounts empty storage (docs/m3-plan.md, k3sm:M3.2).
	BindingWaitForFirstConsumer VolumeBindingMode = "WaitForFirstConsumer"
	// BindingImmediate provisions as soon as the PVC is created. It is a
	// well-formed value but is NOT supported by k3sm M3 (it cannot pin topology).
	BindingImmediate VolumeBindingMode = "Immediate"
)

// Valid reports whether m is a well-formed binding mode value.
func (m VolumeBindingMode) Valid() bool {
	switch m {
	case BindingWaitForFirstConsumer, BindingImmediate:
		return true
	default:
		return false
	}
}

// ErrInvalid is returned by the Validate methods in this package when a value is
// not usable by the provisioner. Wrap it via %w; test for it with errors.Is.
var ErrInvalid = errors.New("storagev1: invalid storage contract")

// LocalPathClass is the k3sm local-path StorageClass / provisioner contract: the
// parameters the provisioner controller (k3sm:M3) stamps onto the StorageClass
// it owns and that the runtime binder (runtimed:M3) resolves a per-PVC dir
// against. It is the cross-repo agreement; the served Kubernetes object remains
// an upstream storage.k8s.io StorageClass.
type LocalPathClass struct {
	// Name is the StorageClass name (DefaultStorageClassName).
	Name string `json:"name"`
	// Provisioner is the StorageClass.provisioner identity (ProvisionerName).
	Provisioner string `json:"provisioner"`
	// BasePath is the APFS storage root per-PVC dirs are created under
	// (DefaultBasePath). It must be absolute.
	BasePath string `json:"basePath"`
	// ReclaimPolicy is the PV reclaim policy (M3: ReclaimRetain only).
	ReclaimPolicy ReclaimPolicy `json:"reclaimPolicy"`
	// VolumeBindingMode is when the PV is provisioned (M3:
	// BindingWaitForFirstConsumer only).
	VolumeBindingMode VolumeBindingMode `json:"volumeBindingMode"`
	// Parameters are extra StorageClass.parameters passed through verbatim
	// (additive headroom; empty for the default class).
	Parameters map[string]string `json:"parameters,omitempty"`
}

// DefaultLocalPathClass returns the canonical k3sm local-path class: the default
// name/provisioner/base path, Retain reclaim, and WaitForFirstConsumer binding.
func DefaultLocalPathClass() LocalPathClass {
	return LocalPathClass{
		Name:              DefaultStorageClassName,
		Provisioner:       ProvisionerName,
		BasePath:          DefaultBasePath,
		ReclaimPolicy:     ReclaimRetain,
		VolumeBindingMode: BindingWaitForFirstConsumer,
	}
}

// WithDefaults returns a copy of the class with any empty field set to its
// canonical default. It does not mutate the receiver, so it is safe to call on a
// shared cache object.
func (c LocalPathClass) WithDefaults() LocalPathClass {
	out := c
	if out.Name == "" {
		out.Name = DefaultStorageClassName
	}
	if out.Provisioner == "" {
		out.Provisioner = ProvisionerName
	}
	if out.BasePath == "" {
		out.BasePath = DefaultBasePath
	}
	if out.ReclaimPolicy == "" {
		out.ReclaimPolicy = ReclaimRetain
	}
	if out.VolumeBindingMode == "" {
		out.VolumeBindingMode = BindingWaitForFirstConsumer
	}
	return out
}

// Validate reports whether the class is usable by the k3sm M3 provisioner: a
// name and provisioner, an absolute BasePath, and the M3-supported policies
// (ReclaimRetain + BindingWaitForFirstConsumer). It rejects the well-formed but
// unsupported Delete / Immediate values fail-fast rather than provisioning a
// volume k3sm cannot honor. Errors wrap ErrInvalid.
func (c LocalPathClass) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("%w: storage class missing name", ErrInvalid)
	}
	if c.Provisioner == "" {
		return fmt.Errorf("%w: storage class %q missing provisioner", ErrInvalid, c.Name)
	}
	if c.BasePath == "" || !path.IsAbs(c.BasePath) {
		return fmt.Errorf("%w: storage class %q basePath %q must be an absolute path", ErrInvalid, c.Name, c.BasePath)
	}
	if c.ReclaimPolicy != ReclaimRetain {
		return fmt.Errorf("%w: storage class %q reclaimPolicy %q unsupported (M3 supports %q only)", ErrInvalid, c.Name, c.ReclaimPolicy, ReclaimRetain)
	}
	if c.VolumeBindingMode != BindingWaitForFirstConsumer {
		return fmt.Errorf("%w: storage class %q volumeBindingMode %q unsupported (M3 supports %q only)", ErrInvalid, c.Name, c.VolumeBindingMode, BindingWaitForFirstConsumer)
	}
	return nil
}

// DataDir returns the stable on-APFS directory for a PVC, keyed by its
// (namespace, claimName). It is the SINGLE source of truth both repos compute:
// the provisioner writes it as the bound PersistentVolume's local path, and
// runtimed binds the same dir into the pod — runtimed can resolve it from the
// PodBox alone (namespace + the PVC source's claim_name), never needing the PV
// UID. The (namespace, claimName) key is globally unique and stable across the
// PVC's life (PVCs are never renamed), so the same claim always maps to the same
// dir. Returns an error if either component is empty. Errors wrap ErrInvalid.
func (c LocalPathClass) DataDir(namespace, claimName string) (string, error) {
	base := c.BasePath
	if base == "" {
		base = DefaultBasePath
	}
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(claimName) == "" {
		return "", fmt.Errorf("%w: DataDir needs a namespace and claimName, got %q/%q", ErrInvalid, namespace, claimName)
	}
	return path.Join(base, namespace, claimName), nil
}

// PVName returns the canonical PersistentVolume object name the provisioner
// derives from a PVC UID ("pvc-<uid>"). Deriving the PV name from the immutable
// PVC UID makes provisioning idempotent: a stale watch-cache replay re-derives
// the same name, so a check-before-create sees AlreadyExists and is a no-op
// (docs/m3-plan.md, k3sm:M3.2). This is the kube object name, distinct from
// DataDir (the on-disk path keyed by namespace/claimName). Returns an error if
// uid is empty. Errors wrap ErrInvalid.
func PVName(pvcUID string) (string, error) {
	if strings.TrimSpace(pvcUID) == "" {
		return "", fmt.Errorf("%w: PVName needs a non-empty PVC UID", ErrInvalid)
	}
	return "pvc-" + pvcUID, nil
}

// NodeTopology pins a local PersistentVolume to its owning node: the well-known
// topology key (TopologyKeyHostname) and the node's name. The provisioner turns
// it into the PV's required node-affinity (a single-key node selector) so the
// scheduler places a consuming StatefulSet pod on the SAME Mac that holds the
// data. It is the small, repo-crossing shape of the upstream
// corev1.VolumeNodeAffinity the provisioner builds.
type NodeTopology struct {
	// Key is the node-label key to match (default TopologyKeyHostname).
	Key string `json:"key"`
	// NodeName is the node-label value: the owning node's name.
	NodeName string `json:"nodeName"`
}

// WithDefaults returns a copy with Key defaulted to TopologyKeyHostname when
// empty. It does not mutate the receiver.
func (t NodeTopology) WithDefaults() NodeTopology {
	out := t
	if out.Key == "" {
		out.Key = TopologyKeyHostname
	}
	return out
}

// Validate reports whether the topology can pin a PV: a key (defaulting to the
// hostname label) and a node name. Errors wrap ErrInvalid.
func (t NodeTopology) Validate() error {
	key := t.Key
	if key == "" {
		key = TopologyKeyHostname
	}
	if key == "" {
		return fmt.Errorf("%w: node topology missing key", ErrInvalid)
	}
	if t.NodeName == "" {
		return fmt.Errorf("%w: node topology missing nodeName", ErrInvalid)
	}
	return nil
}
