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

package netv1alpha1

import (
	"errors"
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ErrInvalidInterconnect is wrapped by every Interconnect validation error.
var ErrInvalidInterconnect = errors.New("netv1alpha1: invalid interconnect")

// dns1123LabelRE is the shape of a namespace name (an RFC 1123 label, at most
// 63 characters, checked separately).
var dns1123LabelRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// dns1123SubdomainRE is the shape of a node name (an RFC 1123 subdomain, at
// most 253 characters, checked separately).
var dns1123SubdomainRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// MaxInterconnectListItems is the most entries Spec.ExcludedNodes and
// Spec.AllowedNamespaces may each hold (the CRD's maxItems).
const MaxInterconnectListItems = 64

// isNodeName reports whether s has the shape of a Kubernetes node name.
func isNodeName(s string) bool {
	return len(s) <= 253 && dns1123SubdomainRE.MatchString(s)
}

// isNamespaceName reports whether s has the shape of a Kubernetes namespace
// name.
func isNamespaceName(s string) bool {
	return len(s) <= 63 && dns1123LabelRE.MatchString(s)
}

// Interconnect is a cluster-wide opt-in network over one medium of cables
// between nodes (today the Thunderbolt cable, Medium MediumThunderbolt). It is
// cluster-scoped, ONE PER MEDIUM and named for it ("thunderbolt"); islands of
// cabled nodes are Segments in status, not separate objects.
//
// The server auto-creates it with Enabled false the first time a node reports a
// cabled peer. Nothing gets an interconnect address until a cluster admin sets
// Enabled true: that act is the trust statement for every segment, recorded as
// Status.ArmedBy and Status.ArmedAt. Plugging a cable in never joins anything.
//
// Addresses use one connected /24, InterconnectCIDR (169.254.0.0/24), on the
// node's bridge0 interface, with no extra route: InterconnectSlotAddress derives
// slot s of the node with cluster index idx as 169.254.0.(8·idx + s + 1). A
// cabled node must therefore have cluster index 0..MaxInterconnectNodeIndex
// (30), which caps the cable at 31 nodes; a node beyond it is reported with
// ReasonInterconnectIndexOutOfRange and gets no interconnect address.
//
// A node never reads this object. It reports on its own Node through the
// AnnotationInterconnectReport annotation (InterconnectReport) and reads the
// server's decision from AnnotationInterconnectDesired (InterconnectDesired).
type Interconnect struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the admin's configuration of the interconnect.
	Spec InterconnectSpec `json:"spec"`
	// Status is the resolved membership and segments (set by the server; a
	// status subresource).
	Status InterconnectStatus `json:"status,omitempty"`
}

// InterconnectFallback is what happens to a pod whose interconnect attach
// cannot be satisfied.
type InterconnectFallback string

const (
	// InterconnectFallbackNetwork starts the pod without the interconnect
	// address and records the attach condition False with the reason. It is the
	// default.
	InterconnectFallbackNetwork InterconnectFallback = "network"
	// InterconnectFallbackNone holds the pod in creation and reports the reason
	// as an Event.
	InterconnectFallbackNone InterconnectFallback = "none"
)

// InterconnectSpec is the admin-owned configuration of an Interconnect.
type InterconnectSpec struct {
	// Medium is the kind of cable (MediumThunderbolt, the only value).
	Medium string `json:"medium"`
	// Fallback is the behaviour when a pod's attach cannot be satisfied. Empty
	// means InterconnectFallbackNetwork (the CRD defaults it); see
	// EffectiveFallback.
	Fallback InterconnectFallback `json:"fallback,omitempty"`
	// Enabled arms the interconnect. It defaults to false: nothing gets an
	// address until an admin sets it.
	Enabled bool `json:"enabled,omitempty"`
	// ExcludedNodes are node names that never become members, even when cabled.
	ExcludedNodes []string `json:"excludedNodes,omitempty"`
	// AllowedNamespaces limits which namespaces' pods may attach. Empty means
	// every namespace.
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}

// EffectiveFallback returns Fallback, or InterconnectFallbackNetwork when it is
// empty (an object read before the CRD default applied, or built in Go).
func (s InterconnectSpec) EffectiveFallback() InterconnectFallback {
	if s.Fallback == "" {
		return InterconnectFallbackNetwork
	}
	return s.Fallback
}

// NamespaceAllowed reports whether pods in namespace ns may attach under the
// namespace rule alone. An EMPTY AllowedNamespaces allows every namespace (plan
// I9); a non-empty one allows only the namespaces it lists.
//
// It does not check that the interconnect is armed: a caller must also require
// Spec.Enabled before letting any pod attach.
func (s InterconnectSpec) NamespaceAllowed(ns string) bool {
	if len(s.AllowedNamespaces) == 0 {
		return true
	}
	for _, n := range s.AllowedNamespaces {
		if n == ns {
			return true
		}
	}
	return false
}

// Validate reports whether the spec is well-formed on its own: Medium is
// MediumThunderbolt; Fallback is empty, "network" or "none"; every
// ExcludedNodes entry is a node-name shape (an RFC 1123 subdomain); and every
// AllowedNamespaces entry is a namespace-name shape (an RFC 1123 label). Neither
// list may repeat an entry or hold more than MaxInterconnectListItems entries.
//
// It checks shape only; whether a named node or namespace exists is the
// server's concern. Errors wrap ErrInvalidInterconnect.
func (s InterconnectSpec) Validate() error {
	if s.Medium != MediumThunderbolt {
		return fmt.Errorf("%w: medium %q, want %q", ErrInvalidInterconnect, s.Medium, MediumThunderbolt)
	}
	switch s.Fallback {
	case "", InterconnectFallbackNetwork, InterconnectFallbackNone:
	default:
		return fmt.Errorf("%w: fallback %q, want %q or %q", ErrInvalidInterconnect, s.Fallback, InterconnectFallbackNetwork, InterconnectFallbackNone)
	}
	if n := len(s.ExcludedNodes); n > MaxInterconnectListItems {
		return fmt.Errorf("%w: excludedNodes has %d entries, at most %d", ErrInvalidInterconnect, n, MaxInterconnectListItems)
	}
	if n := len(s.AllowedNamespaces); n > MaxInterconnectListItems {
		return fmt.Errorf("%w: allowedNamespaces has %d entries, at most %d", ErrInvalidInterconnect, n, MaxInterconnectListItems)
	}
	seen := make(map[string]bool, len(s.ExcludedNodes))
	for i, n := range s.ExcludedNodes {
		if !isNodeName(n) {
			return fmt.Errorf("%w: excludedNodes[%d] %q is not a node name", ErrInvalidInterconnect, i, n)
		}
		if seen[n] {
			return fmt.Errorf("%w: excludedNodes[%d] %q is repeated", ErrInvalidInterconnect, i, n)
		}
		seen[n] = true
	}
	seen = make(map[string]bool, len(s.AllowedNamespaces))
	for i, n := range s.AllowedNamespaces {
		if !isNamespaceName(n) {
			return fmt.Errorf("%w: allowedNamespaces[%d] %q is not a namespace name", ErrInvalidInterconnect, i, n)
		}
		if seen[n] {
			return fmt.Errorf("%w: allowedNamespaces[%d] %q is repeated", ErrInvalidInterconnect, i, n)
		}
		seen[n] = true
	}
	return nil
}

// Validate reports whether the object is well-formed: the name, when set,
// equals Spec.Medium (one object per medium, named for it), and the spec
// passes InterconnectSpec.Validate. Errors wrap ErrInvalidInterconnect.
func (in *Interconnect) Validate() error {
	if in.Name != "" && in.Name != in.Spec.Medium {
		return fmt.Errorf("%w: name %q, want the medium %q", ErrInvalidInterconnect, in.Name, in.Spec.Medium)
	}
	return in.Spec.Validate()
}

// InterconnectMemberState is the resolved state of one member node.
type InterconnectMemberState string

const (
	// InterconnectMemberUp means the node's report is fresh, its address equals
	// the server's recomputation, and at least one peer it reaches also reaches
	// it.
	InterconnectMemberUp InterconnectMemberState = "up"
	// InterconnectMemberDown means the node is cabled but not usable.
	InterconnectMemberDown InterconnectMemberState = "down"
	// InterconnectMemberLoop means the node's cables form a loop back to itself
	// (a port reaches another port of the same node).
	InterconnectMemberLoop InterconnectMemberState = "loop"
)

// InterconnectStatus is the resolved interconnect, written by the server
// through the status subresource.
type InterconnectStatus struct {
	// Members are the nodes that reported a cabled peer.
	Members []InterconnectMember `json:"members,omitempty"`
	// Segments are the connected components of mutual reachability among up
	// members.
	Segments []InterconnectSegment `json:"segments,omitempty"`
	// AttachedPods is the number of pods currently holding an interconnect
	// address.
	AttachedPods int32 `json:"attachedPods,omitempty"`
	// ArmedBy is the user that set Spec.Enabled true.
	ArmedBy string `json:"armedBy,omitempty"`
	// ArmedAt is when Spec.Enabled was set true.
	ArmedAt *metav1.Time `json:"armedAt,omitempty"`
	// Conditions are the standard Kubernetes conditions for the interconnect.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// InterconnectMember is one node's resolved place on the interconnect.
type InterconnectMember struct {
	// Node is the member's node name.
	Node string `json:"node"`
	// Index is the node's cluster index (its pod /24 index).
	Index int32 `json:"index"`
	// Address is the node's slot-0 interconnect address,
	// InterconnectSlotAddress(Index, InterconnectSlotNode), recomputed by the
	// server; empty when the node has none.
	Address string `json:"address,omitempty"`
	// State is the resolved state of the member.
	State InterconnectMemberState `json:"state"`
	// SpeedGbps is the reported link speed in Gb/s (0 = unknown).
	SpeedGbps int32 `json:"speedGbps,omitempty"`
	// Segment is the ID of the segment the member belongs to; empty when it is
	// in none.
	Segment string `json:"segment,omitempty"`
	// ReachablePeers are the node names the member reports reaching.
	ReachablePeers []string `json:"reachablePeers,omitempty"`
	// LastTransition is when State last changed.
	LastTransition *metav1.Time `json:"lastTransition,omitempty"`
}

// InterconnectSegment is one island of mutually reachable members (one L2
// domain on the cable).
type InterconnectSegment struct {
	// ID identifies the segment within this object's status.
	ID string `json:"id"`
	// Nodes are the member node names in the segment.
	Nodes []string `json:"nodes,omitempty"`
}

// InterconnectList is a list of Interconnect objects (the watch/list response
// type).
type InterconnectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the Interconnect objects.
	Items []Interconnect `json:"items"`
}

// DeepCopyInto copies the receiver into out (hand-written; apis runs no
// deepcopy-gen).
func (in *InterconnectSpec) DeepCopyInto(out *InterconnectSpec) {
	*out = *in
	if in.ExcludedNodes != nil {
		out.ExcludedNodes = append([]string(nil), in.ExcludedNodes...)
	}
	if in.AllowedNamespaces != nil {
		out.AllowedNamespaces = append([]string(nil), in.AllowedNamespaces...)
	}
}

// DeepCopy returns a deep copy of the spec.
func (in *InterconnectSpec) DeepCopy() *InterconnectSpec {
	if in == nil {
		return nil
	}
	out := new(InterconnectSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *InterconnectMember) DeepCopyInto(out *InterconnectMember) {
	*out = *in
	if in.ReachablePeers != nil {
		out.ReachablePeers = append([]string(nil), in.ReachablePeers...)
	}
	if in.LastTransition != nil {
		out.LastTransition = in.LastTransition.DeepCopy()
	}
}

// DeepCopy returns a deep copy of the member.
func (in *InterconnectMember) DeepCopy() *InterconnectMember {
	if in == nil {
		return nil
	}
	out := new(InterconnectMember)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *InterconnectSegment) DeepCopyInto(out *InterconnectSegment) {
	*out = *in
	if in.Nodes != nil {
		out.Nodes = append([]string(nil), in.Nodes...)
	}
}

// DeepCopy returns a deep copy of the segment.
func (in *InterconnectSegment) DeepCopy() *InterconnectSegment {
	if in == nil {
		return nil
	}
	out := new(InterconnectSegment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *InterconnectStatus) DeepCopyInto(out *InterconnectStatus) {
	*out = *in
	if in.Members != nil {
		out.Members = make([]InterconnectMember, len(in.Members))
		for i := range in.Members {
			in.Members[i].DeepCopyInto(&out.Members[i])
		}
	}
	if in.Segments != nil {
		out.Segments = make([]InterconnectSegment, len(in.Segments))
		for i := range in.Segments {
			in.Segments[i].DeepCopyInto(&out.Segments[i])
		}
	}
	if in.ArmedAt != nil {
		out.ArmedAt = in.ArmedAt.DeepCopy()
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy returns a deep copy of the status.
func (in *InterconnectStatus) DeepCopy() *InterconnectStatus {
	if in == nil {
		return nil
	}
	out := new(InterconnectStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *Interconnect) DeepCopyInto(out *Interconnect) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the Interconnect.
func (in *Interconnect) DeepCopy() *Interconnect {
	if in == nil {
		return nil
	}
	out := new(Interconnect)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *Interconnect) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *InterconnectList) DeepCopyInto(out *InterconnectList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]Interconnect, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *InterconnectList) DeepCopy() *InterconnectList {
	if in == nil {
		return nil
	}
	out := new(InterconnectList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *InterconnectList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
