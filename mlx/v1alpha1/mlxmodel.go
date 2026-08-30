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

package mlxv1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemeGroupVersion is the GroupVersion the MLXModel types register under
// (mlx.k3sm.io/v1alpha1 — the single served + stored version). The v1alpha1
// suffix is a promise about compatibility, not decoration: see the package doc.
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// SchemeBuilder collects the functions that register the mlx.k3sm.io/v1alpha1
// types into a runtime.Scheme; AddToScheme applies them (the standard client-go
// pattern the k3sm MLX operator's informer uses).
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Resource maps a resource name to its GroupResource within mlx.k3sm.io, for
// building REST paths and status errors.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(SchemeGroupVersion, &MLXModel{}, &MLXModelList{})
	metav1.AddToGroupVersion(s, SchemeGroupVersion)
	return nil
}

// MLXModel is one MLX-served model: the model to fetch, the memory it needs, and
// how many replicas serve it. It is namespaced (it owns namespaced workload
// objects — a serving deployment, a Service, and optionally a cache PVC) and its
// pods are scheduled onto nodes advertising the mlx.k3sm.io/gpu extended
// resource.
//
// Mirrors the Kubernetes object conventions (TypeMeta + ObjectMeta) so it is a
// first-class served/watchable CRD.
type MLXModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired serving configuration.
	Spec MLXModelSpec `json:"spec"`
	// Status is the observed serving state, written by the MLX operator through
	// the status subresource.
	Status MLXModelStatus `json:"status,omitempty"`
}

// MLXModelSpec is the desired state of an MLX-served model.
type MLXModelSpec struct {
	// Model is the model repository reference to serve (e.g.
	// "mlx-community/Llama-3.2-3B-Instruct-4bit"). Required.
	Model string `json:"model"`

	// Revision pins the model to an exact immutable revision. Empty means the
	// repository's default branch, which is MUTABLE — two replicas admitted at
	// different times can then serve different weights under one object. Status
	// reports what was actually resolved (ResolvedRevision) so that drift is at
	// least observable.
	Revision string `json:"revision,omitempty"`

	// Quantization is the quantization variant to serve (e.g. "4bit"), when the
	// repository offers a choice and the reference does not already name one.
	// Empty means whatever the referenced repository provides.
	Quantization string `json:"quantization,omitempty"`

	// Memory is the unified memory the served model needs. Required, and NOT a
	// hint: on Apple Silicon the GPU shares system memory, so this is the real
	// scheduling constraint — the number a node's advertised capacity is checked
	// against and the reason a model lands on one Mac rather than another. It is
	// a resource.Quantity so it is written the way every other Kubernetes memory
	// value is written ("24Gi"), with no second unit convention to get wrong.
	//
	// There is deliberately no default. A guessed default would schedule
	// successfully and then fail at load time on the node, which is the worst
	// place to discover the number was wrong.
	Memory resource.Quantity `json:"memory"`

	// Replicas is the number of serving replicas. Nil means one. It is a pointer
	// so that an explicit 0 (scale to zero — stop serving without deleting the
	// object and its cache) is distinguishable from "unset".
	Replicas *int32 `json:"replicas,omitempty"`

	// Port is the port the serving process listens on, and the port the generated
	// Service exposes. 0 means the operator's default.
	Port int32 `json:"port,omitempty"`

	// Runtime configures the serving container itself. It is a value rather than
	// a pointer because a served model always has a runtime; an empty value means
	// "use the operator's defaults for every part of it".
	Runtime MLXRuntime `json:"runtime,omitempty"`

	// Cache requests a persistent volume for downloaded weights. Nil means no
	// cache volume — weights are re-fetched when a replica starts on a node that
	// has not served this model before. It is a POINTER, not a value with a zero
	// size, because "no cache at all" and "a cache whose size I have not stated"
	// call for different actions from the operator (create nothing versus create
	// a PVC), and a zero-valued struct cannot tell them apart.
	Cache *MLXCache `json:"cache,omitempty"`

	// NodeSelector narrows which nodes may serve this model, on top of the
	// mlx.k3sm.io/gpu resource request the operator always adds. It is the seam
	// for pinning a model to a chip generation or a memory class using the
	// LabelChip / LabelChipFamily / LabelMemoryGB node labels.
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Distributed is RESERVED for multi-node sharded serving and MUST NOT be set.
	//
	// The field exists so the seam has a stable shape and so a spec that sets it
	// is REPRESENTABLE and can therefore be rejected with a legible reason rather
	// than silently ignored — a silently-ignored sharding request would serve the
	// model single-node and look like success. Setting it is rejected at
	// admission by a CEL rule on the CRD (see k3sm.io/apis/config/crd); nothing
	// in this module enforces that, and no controller honours the field.
	Distributed *MLXDistributed `json:"distributed,omitempty"`
}

// MLXRuntime configures the serving container for an MLXModel.
type MLXRuntime struct {
	// Image is the serving image to run. Empty means the operator's pinned
	// default mlx-serve image.
	Image string `json:"image,omitempty"`
	// Args are extra arguments appended to the serving command.
	Args []string `json:"args,omitempty"`
}

// MLXCache is the persistent weight cache for an MLXModel.
type MLXCache struct {
	// Size is the requested capacity of the cache volume. It must comfortably
	// exceed the on-disk size of the model being cached; a volume too small to
	// hold the weights fails the download rather than degrading to no cache.
	Size resource.Quantity `json:"size"`
	// StorageClassName selects the storage class for the cache volume. Empty
	// means the cluster default.
	StorageClassName string `json:"storageClassName,omitempty"`
}

// MLXDistributed is the RESERVED multi-node sharding seam (see
// MLXModelSpec.Distributed). It is declared, not implemented: no controller
// reads it, and admission rejects a spec that sets it. Its shape is explicitly
// NOT part of even this package's alpha contract — it may change entirely when
// multi-node serving lands.
type MLXDistributed struct {
	// Nodes is the number of nodes the model would shard across. Reserved; it is
	// not honoured.
	Nodes int32 `json:"nodes,omitempty"`
}

// MLXModelPhase is the DERIVED, human-facing summary of an MLXModel's state — a
// single word for `kubectl get` and nothing more.
//
// It is derived from Status.Conditions by the operator and is never the source
// of truth: this API is conditions-first, so a controller, a test, or a script
// making a decision reads the condition it actually cares about. A phase
// necessarily loses information (a model can be serving on two nodes and failing
// on a third), which is exactly why it may be shown to a human and must not be
// branched on.
type MLXModelPhase string

const (
	// MLXModelPhasePending means the model has been accepted but is not yet
	// serving and is not yet fetching weights.
	MLXModelPhasePending MLXModelPhase = "Pending"
	// MLXModelPhaseDownloading means the weights are being fetched.
	MLXModelPhaseDownloading MLXModelPhase = "Downloading"
	// MLXModelPhaseReady means at least the desired replicas are serving.
	MLXModelPhaseReady MLXModelPhase = "Ready"
	// MLXModelPhaseFailed means the model cannot be served and will not retry
	// without a change; Conditions carry the reason.
	MLXModelPhaseFailed MLXModelPhase = "Failed"
)

// MLXModelConditionReady is the condition type carrying whether the model is
// serving. It is the condition MLXModelPhase summarizes, and the one a consumer
// should read instead of the phase. Published as a constant so no consumer
// spells the string.
const MLXModelConditionReady = "Ready"

// MLXModelStatus is the observed state of an MLXModel, written by the operator
// through the status subresource.
//
// It is CONDITIONS-FIRST: Conditions is the contract, and Phase/Endpoint/
// ResolvedRevision are derived conveniences on top of it.
type MLXModelStatus struct {
	// Conditions are the standard Kubernetes conditions for this model, of which
	// MLXModelConditionReady is the primary one.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the metadata.generation this status was computed
	// from. A consumer that skips this check can read a Ready condition that
	// describes the PREVIOUS spec — the standard stale-status trap, and the
	// reason the status subresource is enabled at all.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the derived single-word summary shown as a printer column. See
	// MLXModelPhase: display only, never a branch condition.
	Phase MLXModelPhase `json:"phase,omitempty"`

	// Endpoint is the in-cluster address clients use to reach the served model
	// (the generated Service), empty until it exists.
	Endpoint string `json:"endpoint,omitempty"`

	// ResolvedRevision is the exact model revision actually being served. When
	// Spec.Revision was empty this is what the mutable default branch resolved
	// to, which is the only record of what is really running.
	ResolvedRevision string `json:"resolvedRevision,omitempty"`
}

// MLXModelList is a list of MLXModel objects (the watch/list response type).
type MLXModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the MLXModel objects.
	Items []MLXModel `json:"items"`
}

// ---------------------------------------------------------------------------
// DeepCopy* are hand-written — this module runs no deepcopy-gen (the net/v1
// MeshPeer precedent). Every reference type (slices, maps, pointers, and
// resource.Quantity's own internal state) must be copied, or a client-go
// informer's cached object and a caller's mutation share memory.
// ---------------------------------------------------------------------------

// DeepCopyInto copies the receiver into out.
func (in *MLXRuntime) DeepCopyInto(out *MLXRuntime) {
	*out = *in
	if in.Args != nil {
		out.Args = make([]string, len(in.Args))
		copy(out.Args, in.Args)
	}
}

// DeepCopy returns a deep copy of the runtime config.
func (in *MLXRuntime) DeepCopy() *MLXRuntime {
	if in == nil {
		return nil
	}
	out := new(MLXRuntime)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *MLXCache) DeepCopyInto(out *MLXCache) {
	*out = *in
	out.Size = in.Size.DeepCopy()
}

// DeepCopy returns a deep copy of the cache request.
func (in *MLXCache) DeepCopy() *MLXCache {
	if in == nil {
		return nil
	}
	out := new(MLXCache)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *MLXDistributed) DeepCopyInto(out *MLXDistributed) {
	*out = *in
}

// DeepCopy returns a deep copy of the reserved sharding seam.
func (in *MLXDistributed) DeepCopy() *MLXDistributed {
	if in == nil {
		return nil
	}
	out := new(MLXDistributed)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *MLXModelSpec) DeepCopyInto(out *MLXModelSpec) {
	*out = *in
	out.Memory = in.Memory.DeepCopy()
	if in.Replicas != nil {
		r := *in.Replicas
		out.Replicas = &r
	}
	in.Runtime.DeepCopyInto(&out.Runtime)
	if in.Cache != nil {
		out.Cache = in.Cache.DeepCopy()
	}
	if in.NodeSelector != nil {
		out.NodeSelector = make(map[string]string, len(in.NodeSelector))
		for k, v := range in.NodeSelector {
			out.NodeSelector[k] = v
		}
	}
	if in.Distributed != nil {
		out.Distributed = in.Distributed.DeepCopy()
	}
}

// DeepCopy returns a deep copy of the spec.
func (in *MLXModelSpec) DeepCopy() *MLXModelSpec {
	if in == nil {
		return nil
	}
	out := new(MLXModelSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *MLXModelStatus) DeepCopyInto(out *MLXModelStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy returns a deep copy of the status.
func (in *MLXModelStatus) DeepCopy() *MLXModelStatus {
	if in == nil {
		return nil
	}
	out := new(MLXModelStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *MLXModel) DeepCopyInto(out *MLXModel) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the MLXModel.
func (in *MLXModel) DeepCopy() *MLXModel {
	if in == nil {
		return nil
	}
	out := new(MLXModel)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object (satisfies
// runtime.Object so a client-go scheme can serve/watch the type).
func (in *MLXModel) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *MLXModelList) DeepCopyInto(out *MLXModelList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]MLXModel, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *MLXModelList) DeepCopy() *MLXModelList {
	if in == nil {
		return nil
	}
	out := new(MLXModelList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *MLXModelList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
