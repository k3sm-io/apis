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

package updatesv1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Update is one request to move every node of the cluster to the release
// Spec.Version. It is cluster-scoped (a cluster has one release) and its status
// is written by the update controller through the status subresource.
//
// Mirrors the Kubernetes object conventions (TypeMeta + ObjectMeta) so it is a
// first-class served/watchable CRD.
type Update struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the requested update.
	Spec UpdateSpec `json:"spec"`
	// Status is the observed progress of the update.
	Status UpdateStatus `json:"status,omitempty"`
}

// UpdateSpec is the requested update.
type UpdateSpec struct {
	// Version is the release tag to install on every node (for example
	// "v0.2.0"). Required.
	Version string `json:"version"`

	// Strategy is how the roll proceeds across the nodes. Every field has a
	// CRD default, so an empty value is a complete request.
	Strategy UpdateStrategy `json:"strategy,omitempty"`

	// Verify is what the downloaded release must prove before it is installed.
	Verify UpdateVerify `json:"verify,omitempty"`

	// AllowDowngrade permits a Version older than the release a node runs.
	// False refuses a downgrade before any node is touched.
	AllowDowngrade bool `json:"allowDowngrade,omitempty"`

	// Cancel stops the roll: the node in progress finishes its current step
	// and no further node is started. The update then ends Cancelled. Nodes
	// already moved stay on Version; a cancel is not a rollback.
	Cancel bool `json:"cancel,omitempty"`
}

// UpdateOrder is the order in which an Update visits the nodes.
type UpdateOrder string

const (
	// UpdateOrderWorkersThenServer updates every worker before the server, so
	// the control plane is the last thing to restart and is never ahead of the
	// workers it serves. It is the only order and the default.
	UpdateOrderWorkersThenServer UpdateOrder = "workers-then-server"
)

// UpdateStrategy is how an Update rolls across the nodes.
type UpdateStrategy struct {
	// Order is the node order. Empty means UpdateOrderWorkersThenServer (the
	// CRD defaults it).
	Order UpdateOrder `json:"order,omitempty"`

	// MaxUnavailable is the most nodes updated at once. At least 1; 0 means
	// the CRD default of 1. The server is always updated alone.
	MaxUnavailable int32 `json:"maxUnavailable,omitempty"`

	// Drain evicts each node's pods (after cordoning it) before the node is
	// restarted. False only cordons the node, so its pods ride the restart.
	Drain bool `json:"drain,omitempty"`
}

// UpdateSignaturePolicy is how an Update treats the release signature.
type UpdateSignaturePolicy string

const (
	// UpdateSignatureRequired refuses a release whose signature is missing or
	// does not verify. It is the default.
	UpdateSignatureRequired UpdateSignaturePolicy = "required"
	// UpdateSignatureIfSigned verifies the signature when the release carries
	// one and accepts a release that carries none. A signature that is present
	// and does not verify is still refused.
	UpdateSignatureIfSigned UpdateSignaturePolicy = "if-signed"
)

// UpdateVerify is what the downloaded release must prove before an Update
// installs it.
type UpdateVerify struct {
	// Checksums checks every downloaded artifact against the release's
	// published checksums. Nil means true (the CRD defaults it). It is a
	// pointer because the default is true: a plain bool marshalled with
	// omitempty would drop an explicit false, and the apiserver would then
	// default it back to true.
	Checksums *bool `json:"checksums,omitempty"`

	// Signature is the release-signature policy. Empty means
	// UpdateSignatureRequired (the CRD defaults it).
	Signature UpdateSignaturePolicy `json:"signature,omitempty"`
}

// ChecksumsEnabled reports whether the checksum check applies: true unless
// Checksums is explicitly false.
func (v UpdateVerify) ChecksumsEnabled() bool {
	return v.Checksums == nil || *v.Checksums
}

// UpdatePhase is the overall state of an Update.
type UpdatePhase string

const (
	// UpdatePhasePending means the update is accepted and no node has started.
	UpdatePhasePending UpdatePhase = "Pending"
	// UpdatePhaseRolling means at least one node has started and the roll has
	// not finished.
	UpdatePhaseRolling UpdatePhase = "Rolling"
	// UpdatePhaseSucceeded means every node runs Spec.Version.
	UpdatePhaseSucceeded UpdatePhase = "Succeeded"
	// UpdatePhaseFailed means a node failed and the roll stopped; Conditions
	// and the node's Message carry the reason.
	UpdatePhaseFailed UpdatePhase = "Failed"
	// UpdatePhaseCancelled means Spec.Cancel stopped the roll.
	UpdatePhaseCancelled UpdatePhase = "Cancelled"
)

// UpdateNodePhase is the step one node of an Update is on.
type UpdateNodePhase string

const (
	// UpdateNodePhasePending means the node has not started.
	UpdateNodePhasePending UpdateNodePhase = "Pending"
	// UpdateNodePhaseCordoning means the node is being marked unschedulable.
	UpdateNodePhaseCordoning UpdateNodePhase = "Cordoning"
	// UpdateNodePhaseDraining means the node's pods are being evicted.
	UpdateNodePhaseDraining UpdateNodePhase = "Draining"
	// UpdateNodePhaseDownloading means the release is being fetched.
	UpdateNodePhaseDownloading UpdateNodePhase = "Downloading"
	// UpdateNodePhaseVerifying means the fetched release is being checked
	// against Spec.Verify.
	UpdateNodePhaseVerifying UpdateNodePhase = "Verifying"
	// UpdateNodePhaseInstalling means the release is being installed.
	UpdateNodePhaseInstalling UpdateNodePhase = "Installing"
	// UpdateNodePhaseRestarting means the node's services are restarting on
	// the new release.
	UpdateNodePhaseRestarting UpdateNodePhase = "Restarting"
	// UpdateNodePhaseDone means the node runs Spec.Version and is schedulable
	// again.
	UpdateNodePhaseDone UpdateNodePhase = "Done"
	// UpdateNodePhaseFailed means the node's update failed; Message carries
	// the reason.
	UpdateNodePhaseFailed UpdateNodePhase = "Failed"
)

// UpdateNodeStatus is the progress of one node through an Update.
type UpdateNodeStatus struct {
	// Name is the node name.
	Name string `json:"name"`
	// From is the release the node ran when the update reached it.
	From string `json:"from,omitempty"`
	// To is the release the node is moving to (Spec.Version).
	To string `json:"to,omitempty"`
	// Phase is the step the node is on.
	Phase UpdateNodePhase `json:"phase,omitempty"`
	// Message is a human-readable detail of the current step, or the reason
	// it failed.
	Message string `json:"message,omitempty"`
	// StartedAt is when the node left Pending.
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// FinishedAt is when the node reached Done or Failed.
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
}

// UpdateStatus is the observed progress of an Update, written by the update
// controller through the status subresource.
type UpdateStatus struct {
	// Phase is the overall state of the update.
	Phase UpdatePhase `json:"phase,omitempty"`
	// Nodes is the progress of each node, one entry per node name.
	Nodes []UpdateNodeStatus `json:"nodes,omitempty"`
	// Conditions are the standard Kubernetes conditions for the update.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the metadata.generation this status was computed
	// from; a status whose ObservedGeneration trails the object's generation
	// describes an earlier spec (for example, one written before Cancel was
	// set).
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// UpdateList is a list of Update objects (the watch/list response type).
type UpdateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the Update objects.
	Items []Update `json:"items"`
}

// DeepCopy* below are hand-written: this module runs no deepcopy-gen (the
// net/v1 MeshPeer precedent). Every pointer and slice is copied, or a
// client-go informer's cached object and a caller's mutation share memory.

// DeepCopyInto copies the receiver into out.
func (in *UpdateVerify) DeepCopyInto(out *UpdateVerify) {
	*out = *in
	if in.Checksums != nil {
		v := *in.Checksums
		out.Checksums = &v
	}
}

// DeepCopy returns a deep copy of the verify policy.
func (in *UpdateVerify) DeepCopy() *UpdateVerify {
	if in == nil {
		return nil
	}
	out := new(UpdateVerify)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *UpdateSpec) DeepCopyInto(out *UpdateSpec) {
	*out = *in
	in.Verify.DeepCopyInto(&out.Verify)
}

// DeepCopy returns a deep copy of the spec.
func (in *UpdateSpec) DeepCopy() *UpdateSpec {
	if in == nil {
		return nil
	}
	out := new(UpdateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *UpdateNodeStatus) DeepCopyInto(out *UpdateNodeStatus) {
	*out = *in
	if in.StartedAt != nil {
		out.StartedAt = in.StartedAt.DeepCopy()
	}
	if in.FinishedAt != nil {
		out.FinishedAt = in.FinishedAt.DeepCopy()
	}
}

// DeepCopy returns a deep copy of the node status.
func (in *UpdateNodeStatus) DeepCopy() *UpdateNodeStatus {
	if in == nil {
		return nil
	}
	out := new(UpdateNodeStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *UpdateStatus) DeepCopyInto(out *UpdateStatus) {
	*out = *in
	if in.Nodes != nil {
		out.Nodes = make([]UpdateNodeStatus, len(in.Nodes))
		for i := range in.Nodes {
			in.Nodes[i].DeepCopyInto(&out.Nodes[i])
		}
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy returns a deep copy of the status.
func (in *UpdateStatus) DeepCopy() *UpdateStatus {
	if in == nil {
		return nil
	}
	out := new(UpdateStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *Update) DeepCopyInto(out *Update) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the Update.
func (in *Update) DeepCopy() *Update {
	if in == nil {
		return nil
	}
	out := new(Update)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object (satisfies
// runtime.Object so a client-go scheme can serve/watch the type).
func (in *Update) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *UpdateList) DeepCopyInto(out *UpdateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Update, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *UpdateList) DeepCopy() *UpdateList {
	if in == nil {
		return nil
	}
	out := new(UpdateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *UpdateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
