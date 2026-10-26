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

// The node-label key. Every consumer must agree on the exact byte string, so it
// is published here and no consumer spells a literal.
//
// The label is ADVISORY. A node can set any k3sm.io/* label on itself, so it is
// for display and coarse selection only. Routing reads the Interconnect status.
//
// Unlike the object shapes in this package, this key is stable: renaming it
// breaks every already-labelled node and every selector written against it.
const (
	// LabelRDMA reports that RDMA is enabled on the node and at least one
	// direct-link port has an RDMA device. Its value is "true" when present; the
	// label is absent otherwise.
	LabelRDMA = "k3sm.io/rdma"
)

// The interconnect annotation keys, pod condition type and condition reasons.
// Like the label keys above they are stable byte strings every consumer must
// agree on: the node reporter, the server resolver, the provider and the MLX
// operator. "thunderbolt" appears only as a value, never in a key.
const (
	// AnnotationInterconnectReport is the Node annotation a node writes on
	// itself with its cable side (an InterconnectReport, JSON).
	AnnotationInterconnectReport = "k3sm.io/interconnect-report"
	// AnnotationInterconnectDesired is the Node annotation the server writes
	// with the node's armed membership (an InterconnectDesired, JSON). Absent
	// means unconfigured.
	AnnotationInterconnectDesired = "k3sm.io/interconnect-desired"
	// AnnotationInterconnects is the Pod annotation that opts a pod into one or
	// more Interconnects, a comma list of Interconnect names (e.g.
	// "thunderbolt").
	AnnotationInterconnects = "k3sm.io/interconnects"

	// ConditionInterconnectAttached is the Pod condition type the provider
	// records for an opted-in pod: True with the medium as reason and the
	// address as message, or False with a reason saying why the attach failed (the
	// shared reasons below, or a provider-local one). The reasons below are also
	// used on the Interconnect's and an MLXModel's own status.
	ConditionInterconnectAttached = "k3sm.io/InterconnectAttached"

	// ReasonInterconnectSlotsExhausted means every slot on the node is taken.
	ReasonInterconnectSlotsExhausted = "InterconnectSlotsExhausted"
	// ReasonInterconnectUnsupportedRuntime means the pod's RuntimeClass cannot
	// attach (vm and container-vm guests at v1).
	ReasonInterconnectUnsupportedRuntime = "InterconnectUnsupportedRuntime"
	// ReasonNoInterconnectSegment means no segment satisfies the request (for
	// a sharded MLXModel, no segment holds every placed rank's node).
	ReasonNoInterconnectSegment = "NoInterconnectSegment"
	// ReasonInterconnectIndexOutOfRange means the node's cluster index is
	// beyond MaxInterconnectNodeIndex, so it has no interconnect address.
	ReasonInterconnectIndexOutOfRange = "InterconnectIndexOutOfRange"
)
