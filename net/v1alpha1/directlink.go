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
	"net/netip"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DirectLinkSchemaVersion is the current DirectLink spec payload version. The
// spec carries it (SchemaVersion) so a reader can tell which writer shape it is
// looking at; status echoes the version the resolver last processed.
const DirectLinkSchemaVersion int32 = 1

// MediumThunderbolt is the Medium value for a Thunderbolt cable between two
// Macs. It is the only medium today; a future point-to-point medium is a new
// value of DirectLinkSpec.Medium, not a new type.
const MediumThunderbolt = "thunderbolt"

// ErrInvalid is wrapped by every DirectLink validation error.
var ErrInvalid = errors.New("netv1alpha1: invalid direct link")

// ifaceRE is the interface-name shape a direct-link port may carry: a Darwin
// Ethernet-class interface (en0, en12, ...). It pins the shape only; which
// hardware port an interface belongs to is decided by the node from the
// system's own port mapping, never from the name.
var ifaceRE = regexp.MustCompile(`^en[0-9]+$`)

// DirectLink is one node's view of its point-to-point cable ports: which ports
// it has, which remote domain each is plugged into, and the address each
// carries. It is cluster-scoped, ONE PER NODE and named for the node, because a
// node can observe only its own side of a cable — a per-cable object would need
// two writers agreeing on a name. The node writes Spec; the server's resolver
// joins every node's ports by domain UUID and writes Status.
type DirectLink struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is this node's own side of its cables, as the node observes them.
	Spec DirectLinkSpec `json:"spec"`
	// Status is the resolved link state per port (set by the server; a status
	// subresource).
	Status DirectLinkStatus `json:"status,omitempty"`
}

// DirectLinkSpec is a node's declared direct-link ports.
type DirectLinkSpec struct {
	// SchemaVersion stamps this payload (DirectLinkSchemaVersion).
	SchemaVersion int32 `json:"schemaVersion"`
	// NodeName is the node these ports belong to (equals ObjectMeta.Name).
	NodeName string `json:"nodeName"`
	// Medium is the kind of cable (MediumThunderbolt).
	Medium string `json:"medium"`
	// Ports are the node's direct-link ports, plugged or not.
	Ports []DirectLinkPort `json:"ports,omitempty"`
}

// DirectLinkPort is one cable port on the node.
type DirectLinkPort struct {
	// Iface is the network interface the port presents as (e.g. "en2").
	// Interface names can change across an unplug, so state is keyed by
	// DomainUUID, never by Iface.
	Iface string `json:"iface"`
	// PortOrdinal is the receptacle number − 1 (0..7). With the node's index it
	// determines LinkIP.
	PortOrdinal int32 `json:"portOrdinal"`
	// DomainUUID identifies this port's end of the cable.
	DomainUUID string `json:"domainUUID"`
	// PeerDomainUUID identifies the far end of the cable; empty means nothing is
	// plugged in.
	PeerDomainUUID string `json:"peerDomainUUID,omitempty"`
	// SpeedGbps is the negotiated link speed in Gb/s (0 = unknown).
	SpeedGbps int32 `json:"speedGbps,omitempty"`
	// RDMADevice is the RDMA device on this port ("rdma_" + Iface), or empty
	// when the port has none.
	RDMADevice string `json:"rdmaDevice,omitempty"`
	// LinkIP is the port's direct-link address, LinkIP(node index,
	// PortOrdinal); empty until the address is configured.
	LinkIP string `json:"linkIP,omitempty"`
	// LinkUp reports whether the interface has link.
	LinkUp bool `json:"linkUp"`
	// RouteReady reports that the address and its routes were configured and
	// read back from the kernel on this node.
	RouteReady bool `json:"routeReady"`
	// TunnelOnly opts this port out of carrying plaintext pod traffic: the
	// cable is not used as a direct route even when it is up.
	TunnelOnly bool `json:"tunnelOnly,omitempty"`
}

// DirectLinkState is the resolved state of one port's cable.
type DirectLinkState string

const (
	// DirectLinkStateUp means both ends list each other, both report link and
	// ready routes, neither opts out, and the peer node is live.
	DirectLinkStateUp DirectLinkState = "up"
	// DirectLinkStatePeerUnknown means the far end is plugged in but belongs to
	// no node in the cluster (a cabled Mac that has not joined).
	DirectLinkStatePeerUnknown DirectLinkState = "peer-unknown"
	// DirectLinkStateDown means the port is not usable as a direct route.
	DirectLinkStateDown DirectLinkState = "down"
)

// DirectLinkStatus is the resolved link state for a node's ports, written by
// the server's resolver via the status subresource.
type DirectLinkStatus struct {
	// Ports is the resolved state per port.
	Ports []DirectLinkPortStatus `json:"ports,omitempty"`
	// ObservedSchemaVersion is the Spec.SchemaVersion the resolver last
	// processed.
	ObservedSchemaVersion int32 `json:"observedSchemaVersion,omitempty"`
}

// DirectLinkPortStatus is the resolved state of one port.
type DirectLinkPortStatus struct {
	// Iface is the local interface this entry describes.
	Iface string `json:"iface"`
	// PeerNodeName is the node at the far end, when known.
	PeerNodeName string `json:"peerNodeName,omitempty"`
	// PeerIface is the far end's interface, when known.
	PeerIface string `json:"peerIface,omitempty"`
	// PeerLinkIP is the far end's direct-link address, when known.
	PeerLinkIP string `json:"peerLinkIP,omitempty"`
	// PeerRDMADevice is the far end's RDMA device, when it has one.
	PeerRDMADevice string `json:"peerRDMADevice,omitempty"`
	// State is the resolved state of the cable.
	State DirectLinkState `json:"state"`
	// LastTransition is when State last changed.
	LastTransition *metav1.Time `json:"lastTransition,omitempty"`
}

// DirectLinkList is a list of DirectLink objects (the watch/list response
// type).
type DirectLinkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the DirectLink objects.
	Items []DirectLink `json:"items"`
}

// Validate reports whether the spec is well-formed on its own: it is
// version-stamped and names its node; Medium is MediumThunderbolt; and for every
// port, Iface matches ^en[0-9]+$, PortOrdinal is 0..7, DomainUUID is set and
// differs from PeerDomainUUID, SpeedGbps is not negative, RDMADevice is empty or
// "rdma_" + Iface, and LinkIP, when set, is an address in one of the two
// reserved halves (IsLinkAddress).
//
// It checks the address CLASS only. That LinkIP is exactly the derived address
// for this node needs the node's index (ValidateWithIndex), and that no other
// node claims the same DomainUUID needs every DirectLink; both are server-side
// checks. Errors wrap ErrInvalid.
func (s DirectLinkSpec) Validate() error {
	if s.SchemaVersion == 0 {
		return fmt.Errorf("%w: direct link %q missing schemaVersion", ErrInvalid, s.NodeName)
	}
	if s.NodeName == "" {
		return fmt.Errorf("%w: direct link missing nodeName", ErrInvalid)
	}
	if s.Medium != MediumThunderbolt {
		return fmt.Errorf("%w: direct link %q has medium %q, want %q", ErrInvalid, s.NodeName, s.Medium, MediumThunderbolt)
	}
	for i, p := range s.Ports {
		if err := p.validate(); err != nil {
			return fmt.Errorf("%w: direct link %q ports[%d]: %w", ErrInvalid, s.NodeName, i, err)
		}
	}
	return nil
}

// ValidateWithIndex is Validate plus the derivation check: every port whose
// LinkIP is set must carry exactly LinkIP(idx, PortOrdinal), where idx is the
// node's index (the caller resolves it from the node's pod /24). A port with a
// LinkIP on a node whose index has no direct-link address is an error wrapping
// both ErrInvalid and ErrNoLinkAddress.
func (s DirectLinkSpec) ValidateWithIndex(idx int) error {
	if err := s.Validate(); err != nil {
		return err
	}
	for i, p := range s.Ports {
		if p.LinkIP == "" {
			continue
		}
		want, err := LinkIP(idx, int(p.PortOrdinal))
		if err != nil {
			return fmt.Errorf("%w: direct link %q ports[%d]: %w", ErrInvalid, s.NodeName, i, err)
		}
		got, _ := netip.ParseAddr(p.LinkIP) // parsed successfully by Validate
		if got.Unmap() != want {
			return fmt.Errorf("%w: direct link %q ports[%d] linkIP %s, want %s for node index %d port %d", ErrInvalid, s.NodeName, i, p.LinkIP, want, idx, p.PortOrdinal)
		}
	}
	return nil
}

// validate checks one port's own fields. The returned error carries no
// ErrInvalid; the spec attaches it once.
func (p DirectLinkPort) validate() error {
	if !ifaceRE.MatchString(p.Iface) {
		return fmt.Errorf("iface %q does not match ^en[0-9]+$", p.Iface)
	}
	if p.PortOrdinal < 0 || p.PortOrdinal > MaxPortOrdinal {
		return fmt.Errorf("port ordinal %d is outside 0..%d", p.PortOrdinal, MaxPortOrdinal)
	}
	if p.DomainUUID == "" {
		return fmt.Errorf("missing domainUUID")
	}
	if p.DomainUUID == p.PeerDomainUUID {
		return fmt.Errorf("domainUUID equals peerDomainUUID %q (a port cannot be cabled to itself)", p.DomainUUID)
	}
	if p.SpeedGbps < 0 {
		return fmt.Errorf("speedGbps %d is negative", p.SpeedGbps)
	}
	if p.RDMADevice != "" && p.RDMADevice != "rdma_"+p.Iface {
		return fmt.Errorf("rdmaDevice %q, want empty or %q", p.RDMADevice, "rdma_"+p.Iface)
	}
	if p.LinkIP != "" {
		a, err := netip.ParseAddr(p.LinkIP)
		if err != nil {
			return fmt.Errorf("linkIP %q is not an IP address", p.LinkIP)
		}
		if !IsLinkAddress(a) {
			return fmt.Errorf("linkIP %q is outside 169.254.0.0/24 and 169.254.255.0/24", p.LinkIP)
		}
	}
	return nil
}

// DeepCopyInto copies the receiver into out (hand-written; apis runs no
// deepcopy-gen).
func (in *DirectLinkSpec) DeepCopyInto(out *DirectLinkSpec) {
	*out = *in
	if in.Ports != nil {
		out.Ports = make([]DirectLinkPort, len(in.Ports))
		copy(out.Ports, in.Ports)
	}
}

// DeepCopy returns a deep copy of the spec.
func (in *DirectLinkSpec) DeepCopy() *DirectLinkSpec {
	if in == nil {
		return nil
	}
	out := new(DirectLinkSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *DirectLinkPortStatus) DeepCopyInto(out *DirectLinkPortStatus) {
	*out = *in
	if in.LastTransition != nil {
		out.LastTransition = in.LastTransition.DeepCopy()
	}
}

// DeepCopy returns a deep copy of the port status.
func (in *DirectLinkPortStatus) DeepCopy() *DirectLinkPortStatus {
	if in == nil {
		return nil
	}
	out := new(DirectLinkPortStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *DirectLinkStatus) DeepCopyInto(out *DirectLinkStatus) {
	*out = *in
	if in.Ports != nil {
		out.Ports = make([]DirectLinkPortStatus, len(in.Ports))
		for i := range in.Ports {
			in.Ports[i].DeepCopyInto(&out.Ports[i])
		}
	}
}

// DeepCopy returns a deep copy of the status.
func (in *DirectLinkStatus) DeepCopy() *DirectLinkStatus {
	if in == nil {
		return nil
	}
	out := new(DirectLinkStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *DirectLink) DeepCopyInto(out *DirectLink) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the DirectLink.
func (in *DirectLink) DeepCopy() *DirectLink {
	if in == nil {
		return nil
	}
	out := new(DirectLink)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object (satisfies
// runtime.Object so a client-go scheme can serve/watch the type).
func (in *DirectLink) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *DirectLinkList) DeepCopyInto(out *DirectLinkList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]DirectLink, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *DirectLinkList) DeepCopy() *DirectLinkList {
	if in == nil {
		return nil
	}
	out := new(DirectLinkList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *DirectLinkList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
