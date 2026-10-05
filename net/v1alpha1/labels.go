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

// The direct-link node-label keys. Every consumer must agree on the exact byte
// string, so they are published here and no consumer spells a literal.
//
// The keys are medium-agnostic: "thunderbolt" appears only as a value
// (LabelDirectLinkMedium=thunderbolt), never in a key.
//
// These labels are ADVISORY. A node can set any k3sm.io/* label on itself, so
// they are for display and coarse selection only. Placement and routing read
// DirectLink.status, never these labels.
//
// Unlike the object shapes in this package, these keys are stable: renaming one
// breaks every already-labelled node and every selector written against it.
const (
	// LabelDirectLinkPorts is the number of direct-link ports on the node, as a
	// decimal string.
	LabelDirectLinkPorts = "k3sm.io/direct-link-ports"
	// LabelDirectLinkMedium is the medium of the node's direct-link ports (e.g.
	// MediumThunderbolt).
	LabelDirectLinkMedium = "k3sm.io/direct-link-medium"
	// LabelDirectLinkSpeedGbps is the node's direct-link port speed in Gb/s, as
	// a decimal string (e.g. "40" or "80").
	LabelDirectLinkSpeedGbps = "k3sm.io/direct-link-speed-gbps"
	// LabelDirectLinks is the number of the node's direct links currently up, as
	// a decimal string. Set by the server's resolver, for display.
	LabelDirectLinks = "k3sm.io/direct-links"
	// LabelRDMA reports that RDMA is enabled on the node and at least one
	// direct-link port has an RDMA device. Its value is "true" when present; the
	// label is absent otherwise.
	LabelRDMA = "k3sm.io/rdma"
)
