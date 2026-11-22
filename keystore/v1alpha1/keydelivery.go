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

package keystorev1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DefaultDataKey is the Secret data key a KeyDelivery writes when
// Spec.DataKey is omitted (the CRD defaults it).
const DefaultDataKey = "value"

// The KeyDelivery condition types. Published so no consumer spells the
// strings.
const (
	// ConditionAuthorized reports whether every authority check passes for
	// this declaration.
	ConditionAuthorized = "Authorized"
	// ConditionReady reports whether the target Secret holds the entry's
	// current version.
	ConditionReady = "Ready"
	// ConditionSecretRetained reports whether a labelled Secret of this object
	// stays while Authorized is Unknown or False.
	ConditionSecretRetained = "SecretRetained"
)

// The closed set of KeyDelivery condition reasons. Each condition carries
// only the reasons ConditionReasons gives it. A reader treats a reason it does
// not know as a generic failure of the condition that carries it.
const (
	// ReasonAuthorized (Authorized=True): every authority check passes.
	ReasonAuthorized = "Authorized"
	// ReasonEntryUnavailable (Authorized=False): the entry does not exist or
	// is not the requester's. One reason for both, so status reveals nothing
	// about existence.
	ReasonEntryUnavailable = "EntryUnavailable"
	// ReasonNotRecorded (Authorized=False): the object is not in its owner's
	// sealed delivery record, or its target no longer matches the record.
	ReasonNotRecorded = "NotRecorded"
	// ReasonNotAuthorized (Authorized=False): an access review returned a
	// definitive denial. On Ready=False it is the one reason for any
	// authority failure, its message naming the Authorized reason.
	ReasonNotAuthorized = "NotAuthorized"
	// ReasonIndeterminate (Authorized=Unknown, SecretRetained=True): an
	// outcome that is not a definitive one, such as a transient error;
	// nothing was changed.
	ReasonIndeterminate = "Indeterminate"
	// ReasonPending (Ready=Unknown): before the first sweep decides, and while
	// Authorized is Unknown.
	ReasonPending = "Pending"
	// ReasonDelivered (Ready=True): the Secret holds the entry's current
	// version.
	ReasonDelivered = "Delivered"
	// ReasonSecretExists (Ready=False): a Secret not claimed by a KeyDelivery
	// already holds the target name.
	ReasonSecretExists = "SecretExists"
	// ReasonSecretClaimed (Ready=False): an older KeyDelivery owns the target
	// Secret; one declaration per target Secret.
	ReasonSecretClaimed = "SecretClaimed"
	// ReasonEncryptionOff (Ready=False): secrets encryption is off.
	ReasonEncryptionOff = "EncryptionOff"
	// ReasonUserScopeOff (Ready=False, SecretRetained=True): more than one
	// server is registered.
	ReasonUserScopeOff = "UserScopeOff"
	// ReasonDeletePending (SecretRetained=True): the owned Secret is due for
	// deletion but still exists.
	ReasonDeletePending = "DeletePending"
	// ReasonUnowned (SecretRetained=True): a labelled Secret names this
	// object, but the object owns none, so it stays.
	ReasonUnowned = "Unowned"
	// ReasonLabelRemoved (SecretRetained=True): a Secret stands at this
	// object's target without the label or owner annotation; it is never
	// overwritten or deleted.
	ReasonLabelRemoved = "LabelRemoved"
	// ReasonOwnerUnresolvable (SecretRetained=True): the entry owner's uid has
	// no directory record, and the Secret stays.
	ReasonOwnerUnresolvable = "OwnerUnresolvable"
	// ReasonNoneRetained (SecretRetained=False): no labelled Secret of this
	// object stays, or Authorized is True.
	ReasonNoneRetained = "NoneRetained"
)

// ConditionReasons returns the closed reason set per condition type. It
// returns a fresh map on every call.
func ConditionReasons() map[string][]string {
	return map[string][]string{
		ConditionAuthorized: {
			ReasonAuthorized, ReasonEntryUnavailable, ReasonNotRecorded,
			ReasonNotAuthorized, ReasonIndeterminate,
		},
		ConditionReady: {
			ReasonPending, ReasonDelivered, ReasonNotAuthorized, ReasonSecretExists,
			ReasonSecretClaimed, ReasonEncryptionOff, ReasonUserScopeOff,
		},
		ConditionSecretRetained: {
			ReasonDeletePending, ReasonUnowned, ReasonIndeterminate, ReasonLabelRemoved,
			ReasonUserScopeOff, ReasonOwnerUnresolvable, ReasonNoneRetained,
		},
	}
}

// KeyDelivery declares that one user-scope keystore entry is delivered into
// the object's namespace as a Secret. It is namespaced, and its status is
// written by the k3sm reconciler through the status subresource.
type KeyDelivery struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the declaration. Every field is immutable after create.
	Spec KeyDeliverySpec `json:"spec"`
	// Status is the observed state of the delivery.
	Status KeyDeliveryStatus `json:"status,omitempty"`
}

// KeyDeliverySpec is the declaration. Every field is required or defaulted,
// and immutable after create (CEL transition rules on the CRD).
type KeyDeliverySpec struct {
	// Entry is the scope-qualified entry, user/<uid>/<name>, in the grammar of
	// k3sm.io/apis/keystore/entry. A system entry is refused.
	Entry string `json:"entry"`
	// SecretName is the target Secret in the object's namespace.
	SecretName string `json:"secretName"`
	// DataKey is the key in the Secret's data that holds the value. Empty
	// means DefaultDataKey (the CRD defaults it).
	DataKey string `json:"dataKey,omitempty"`
	// RequestedBy is the local principal k3sm:local:<uid> that declared the
	// delivery; for a user entry it equals k3sm:local: and the entry's uid.
	// Informational: it authorises nothing by itself.
	RequestedBy string `json:"requestedBy"`
}

// KeyDeliveryStatus is the observed state of a KeyDelivery, written through
// the status subresource. It never carries a value or a fingerprint of one.
type KeyDeliveryStatus struct {
	// ObservedGeneration is the metadata.generation this status was computed
	// from.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions are Authorized, Ready and SecretRetained, one per type, the
	// source of truth; each carries a reason from ConditionReasons.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// DeliveredVersion is the entry version the Secret last received; 0
	// before any delivery.
	DeliveredVersion int64 `json:"deliveredVersion,omitempty"`
}

// KeyDeliveryList is a list of KeyDelivery objects (the watch/list response
// type).
type KeyDeliveryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the KeyDelivery objects.
	Items []KeyDelivery `json:"items"`
}

// DeepCopy* below are hand-written: this module runs no deepcopy-gen. Every
// slice is copied, or a client-go informer's cached object and a caller's
// mutation share memory.

// DeepCopyInto copies the receiver into out. KeyDeliverySpec holds no
// reference types, so a value copy is a deep copy.
func (in *KeyDeliverySpec) DeepCopyInto(out *KeyDeliverySpec) {
	*out = *in
}

// DeepCopy returns a deep copy of the spec.
func (in *KeyDeliverySpec) DeepCopy() *KeyDeliverySpec {
	if in == nil {
		return nil
	}
	out := new(KeyDeliverySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *KeyDeliveryStatus) DeepCopyInto(out *KeyDeliveryStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy returns a deep copy of the status.
func (in *KeyDeliveryStatus) DeepCopy() *KeyDeliveryStatus {
	if in == nil {
		return nil
	}
	out := new(KeyDeliveryStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *KeyDelivery) DeepCopyInto(out *KeyDelivery) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the KeyDelivery.
func (in *KeyDelivery) DeepCopy() *KeyDelivery {
	if in == nil {
		return nil
	}
	out := new(KeyDelivery)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object (satisfies
// runtime.Object so a client-go scheme can serve and watch the type).
func (in *KeyDelivery) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *KeyDeliveryList) DeepCopyInto(out *KeyDeliveryList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]KeyDelivery, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *KeyDeliveryList) DeepCopy() *KeyDeliveryList {
	if in == nil {
		return nil
	}
	out := new(KeyDeliveryList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *KeyDeliveryList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
