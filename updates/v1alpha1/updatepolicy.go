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

// UpdatePolicyName is the one metadata.name an UpdatePolicy may carry. The CRD
// refuses any other name with a CEL rule, so unlike UpdateName this is the
// schema's rule and not a controller's.
const UpdatePolicyName = "cluster"

// UpdateChannel is a release channel: which published releases a cluster is
// offered. The CRD holds it to a closed enum. A reader that meets a value it
// does not know says so and never takes it for a known channel.
type UpdateChannel string

const (
	// UpdateChannelStable offers the newest final release. It is the CRD's
	// default, and it is what no UpdatePolicy object and an empty value mean.
	UpdateChannelStable UpdateChannel = "stable"
	// UpdateChannelRC offers the highest of the final releases and release
	// candidates by semver precedence, so a final outranks its own candidates.
	UpdateChannelRC UpdateChannel = "rc"
)

// UpdatePolicy is the cluster's standing choice of release channel. It is a
// cluster-scoped singleton named UpdatePolicyName and spec only: it has no
// status subresource, because a standing setting has no request in progress to
// report on.
//
// Setting the channel changes what is offered and never starts a roll; only an
// Update does that.
type UpdatePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the chosen channel.
	Spec UpdatePolicySpec `json:"spec"`
}

// UpdatePolicySpec is the chosen channel.
type UpdatePolicySpec struct {
	// Channel is the release channel the cluster follows. The CRD defaults it
	// to UpdateChannelStable, and the empty Go value means the same, so code
	// that builds the object by hand never sends an empty channel.
	Channel UpdateChannel `json:"channel,omitempty"`
}

// UpdatePolicyList is a list of UpdatePolicy objects (the watch/list response
// type).
type UpdatePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the UpdatePolicy objects.
	Items []UpdatePolicy `json:"items"`
}

// DeepCopyInto copies the receiver into out. UpdatePolicySpec holds no
// reference types, so a value copy is a deep copy.
func (in *UpdatePolicySpec) DeepCopyInto(out *UpdatePolicySpec) {
	*out = *in
}

// DeepCopy returns a deep copy of the spec.
func (in *UpdatePolicySpec) DeepCopy() *UpdatePolicySpec {
	if in == nil {
		return nil
	}
	out := new(UpdatePolicySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *UpdatePolicy) DeepCopyInto(out *UpdatePolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy returns a deep copy of the policy.
func (in *UpdatePolicy) DeepCopy() *UpdatePolicy {
	if in == nil {
		return nil
	}
	out := new(UpdatePolicy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *UpdatePolicy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *UpdatePolicyList) DeepCopyInto(out *UpdatePolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]UpdatePolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *UpdatePolicyList) DeepCopy() *UpdatePolicyList {
	if in == nil {
		return nil
	}
	out := new(UpdatePolicyList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *UpdatePolicyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
