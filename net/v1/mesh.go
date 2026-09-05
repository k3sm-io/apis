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

package netv1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ---------------------------------------------------------------------------
// MeshPeer CRD (net.k3sm.io/v1) — the wireguard mesh-topology contract.
//
// A MeshPeer is a real Kubernetes custom resource: kine-stored, apiserver-served
// and -watched, and reconciled by the darwin-net mesh (k3sm.io/darwin-net), and
// written at join time by k3sm (k3sm.io/k3sm). Because it is a served/watchable
// object it carries metav1.TypeMeta + ObjectMeta — this is apis's first and only
// k8s.io/apimachinery dependency, pinned in go.mod in lockstep with k3sm.
// Private keys NEVER appear on a MeshPeer: it carries the node's PUBLIC key only
// (DESIGN §5b). DeepCopy* are hand-written (no code-gen in apis).
// ---------------------------------------------------------------------------

// GroupName is the API group of the net.k3sm.io CRDs (MeshPeer).
const GroupName = "net.k3sm.io"

// MeshPeerSchemaVersion is the current MeshPeer spec payload version. The spec
// carries it (SchemaVersion) so the wireguard mesh encoding has an explicit
// evolution seam INSIDE the served net.k3sm.io/v1 GVK: a future protocol change
// bumps this, and readers gate on it, giving a future node-by-node roll a
// compatibility window without a CRD version bump. This is version 1.
const MeshPeerSchemaVersion int32 = 1

// DefaultPersistentKeepaliveSeconds is the wireguard PersistentKeepalive a peer
// uses to keep a NAT path warm (DESIGN §5b). WithDefaults stamps it.
const DefaultPersistentKeepaliveSeconds int32 = 25

// DefaultMeshMTU is the wireguard tunnel MTU the mesh uses (DESIGN §5b: 1380, to
// leave room for the wg overhead under the lo0 MTU). It is a link property, not
// a per-peer field — published here as the cross-repo constant darwin-net reads.
const DefaultMeshMTU int32 = 1380

// SchemeGroupVersion is the GroupVersion the MeshPeer types register under
// (net.k3sm.io/v1 — the single served + stored CRD version; the surface is
// additive-only, and intra-version payload evolution rides Spec.SchemaVersion).
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

// SchemeBuilder collects the functions that register the net.k3sm.io/v1 types
// into a runtime.Scheme; AddToScheme applies them (the standard client-go
// pattern a darwin-net informer / k3sm client uses).
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Resource maps a resource name to its GroupResource within net.k3sm.io, for
// building REST paths and status errors.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(SchemeGroupVersion, &MeshPeer{}, &MeshPeerList{})
	metav1.AddToGroupVersion(s, SchemeGroupVersion)
	return nil
}

// MeshPeer is one node's wireguard mesh membership: its PUBLIC key, reachable
// endpoint, pod /24, and the symmetric AllowedIPs the mesh programs. It is
// cluster-scoped and named for the node it represents (one MeshPeer per node);
// under the AlwaysAllow authorizer a node may write only its own MeshPeer
// (enroll is controller-/admission-mediated so a node cannot forge another's
// mesh membership — DESIGN §5b).
// Mirrors the Kubernetes object conventions (TypeMeta + ObjectMeta) so it is a
// first-class served/watchable CRD.
type MeshPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired mesh membership for this node.
	Spec MeshPeerSpec `json:"spec"`
	// Status is the observed mesh state for this node (set by the mesh
	// controller; a status subresource).
	Status MeshPeerStatus `json:"status,omitempty"`
}

// MeshPeerSpec is a node's declared mesh membership. AllowedIPs MUST equal
// PodCIDR (the node's single /24 source of truth: AllowedIPs == podnet IPAM CIDR
// == node.spec.podCIDR); symmetric-but-unequal AllowedIPs still blackholes traffic.
type MeshPeerSpec struct {
	// SchemaVersion stamps this payload (MeshPeerSchemaVersion). A reader gates on
	// it before programming the tunnel.
	SchemaVersion int32 `json:"schemaVersion"`
	// NodeName is the node this peer represents (equals ObjectMeta.Name).
	NodeName string `json:"nodeName"`
	// PublicKey is the node's wireguard PUBLIC key (base64). The matching private
	// key NEVER leaves the node and is never carried here.
	PublicKey string `json:"publicKey"`
	// Endpoint is the host:port the node's wireguard is reachable at (a
	// mutually-routable / same-L2 address — there is no relay).
	Endpoint string `json:"endpoint"`
	// PodCIDR is the node's pod /24 (from node.spec.podCIDR). It is the single
	// source of truth for this node's pod address range.
	PodCIDR string `json:"podCIDR"`
	// AllowedIPs are the symmetric wireguard routes for this peer. For correctness
	// AllowedIPs must contain exactly PodCIDR — the mesh asserts equality, not
	// just symmetry.
	AllowedIPs []string `json:"allowedIPs"`
	// MeshIP, when set, is the /32 reserved from PodCIDR this node uses as its
	// mesh-egress source address (the Service-proxy dialer's LocalAddr); a peer
	// accepts inbound only if the source is within its AllowedIPs. Optional.
	MeshIP string `json:"meshIP,omitempty"`
	// PersistentKeepaliveSeconds is the wireguard PersistentKeepalive interval;
	// zero means DefaultPersistentKeepaliveSeconds (set by WithDefaults).
	PersistentKeepaliveSeconds int32 `json:"persistentKeepaliveSeconds,omitempty"`
}

// MeshPeerStatus is the observed mesh state for a node, set by the mesh
// controller via the status subresource.
type MeshPeerStatus struct {
	// LastHandshakeTime is the most recent successful wireguard handshake the
	// local node observed with this peer (nil = never).
	LastHandshakeTime *metav1.Time `json:"lastHandshakeTime,omitempty"`
	// Reachable reports whether the local node currently has a live tunnel to this
	// peer.
	Reachable bool `json:"reachable,omitempty"`
	// ObservedSchemaVersion is the Spec.SchemaVersion the controller last
	// reconciled (so a skew during a roll is observable).
	ObservedSchemaVersion int32 `json:"observedSchemaVersion,omitempty"`
}

// MeshPeerList is a list of MeshPeer objects (the watch/list response type).
type MeshPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the MeshPeer objects.
	Items []MeshPeer `json:"items"`
}

// WithDefaults returns a copy of the spec with SchemaVersion stamped to
// MeshPeerSchemaVersion and PersistentKeepaliveSeconds to its default when
// either is zero. It does not mutate the receiver.
func (s MeshPeerSpec) WithDefaults() MeshPeerSpec {
	out := s
	if out.SchemaVersion == 0 {
		out.SchemaVersion = MeshPeerSchemaVersion
	}
	if out.PersistentKeepaliveSeconds == 0 {
		out.PersistentKeepaliveSeconds = DefaultPersistentKeepaliveSeconds
	}
	if out.AllowedIPs != nil {
		allowedIPs := make([]string, len(out.AllowedIPs))
		copy(allowedIPs, out.AllowedIPs)
		out.AllowedIPs = allowedIPs
	}
	return out
}

// Validate reports whether the spec is usable by the mesh: it is version-stamped
// (SchemaVersion non-zero) and carries an identity (NodeName), a PUBLIC key, an
// endpoint, a podCIDR, and at least one AllowedIPs entry. It does NOT cross-check
// AllowedIPs == PodCIDR (the mesh asserts that against live IPAM, not from the
// object alone). Errors wrap ErrInvalid.
func (s MeshPeerSpec) Validate() error {
	if s.SchemaVersion == 0 {
		return fmt.Errorf("%w: mesh peer %q missing schemaVersion (call WithDefaults)", ErrInvalid, s.NodeName)
	}
	if s.NodeName == "" {
		return fmt.Errorf("%w: mesh peer missing nodeName", ErrInvalid)
	}
	if s.PublicKey == "" {
		return fmt.Errorf("%w: mesh peer %q missing publicKey", ErrInvalid, s.NodeName)
	}
	if s.Endpoint == "" {
		return fmt.Errorf("%w: mesh peer %q missing endpoint", ErrInvalid, s.NodeName)
	}
	if s.PodCIDR == "" {
		return fmt.Errorf("%w: mesh peer %q missing podCIDR", ErrInvalid, s.NodeName)
	}
	if len(s.AllowedIPs) == 0 {
		return fmt.Errorf("%w: mesh peer %q has no allowedIPs", ErrInvalid, s.NodeName)
	}
	return nil
}

// DeepCopyInto copies the receiver into out (hand-written; apis runs no
// deepcopy-gen).
func (in *MeshPeerSpec) DeepCopyInto(out *MeshPeerSpec) {
	*out = *in
	if in.AllowedIPs != nil {
		out.AllowedIPs = make([]string, len(in.AllowedIPs))
		copy(out.AllowedIPs, in.AllowedIPs)
	}
}

// DeepCopy returns a deep copy of the spec.
func (in *MeshPeerSpec) DeepCopy() *MeshPeerSpec {
	if in == nil {
		return nil
	}
	out := new(MeshPeerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *MeshPeerStatus) DeepCopyInto(out *MeshPeerStatus) {
	*out = *in
	if in.LastHandshakeTime != nil {
		out.LastHandshakeTime = in.LastHandshakeTime.DeepCopy()
	}
}

// DeepCopy returns a deep copy of the status.
func (in *MeshPeerStatus) DeepCopy() *MeshPeerStatus {
	if in == nil {
		return nil
	}
	out := new(MeshPeerStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *MeshPeer) DeepCopyInto(out *MeshPeer) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the MeshPeer.
func (in *MeshPeer) DeepCopy() *MeshPeer {
	if in == nil {
		return nil
	}
	out := new(MeshPeer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object (satisfies
// runtime.Object so a client-go scheme can serve/watch the type).
func (in *MeshPeer) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *MeshPeerList) DeepCopyInto(out *MeshPeerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]MeshPeer, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *MeshPeerList) DeepCopy() *MeshPeerList {
	if in == nil {
		return nil
	}
	out := new(MeshPeerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *MeshPeerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// ---------------------------------------------------------------------------
// Mesh-enroll join payloads — the bootstrap join HTTP exchange wire
// format. These are plain Go structs (NOT a CRD, NOT proto): k3sm's join client
// and supervisor marshal them over the join HTTP exchange. They are
// version-stamped from day one (SchemaVersion) so a future node-by-node roll has a
// compatibility seam. Mesh-enroll (wg pubkey + endpoint + podCIDR) rides the
// join (DESIGN §5b/§5c); thereafter darwin-net watches the MeshPeer CRD.
// ---------------------------------------------------------------------------

// MeshEnrollSchemaVersion is the current mesh-enroll wire payload version.
const MeshEnrollSchemaVersion int32 = 1

// MeshEnrollRequest is the payload a joining node sends during bootstrap: its
// wireguard PUBLIC key + reachable endpoint + (optionally requested) podCIDR.
// The private key never leaves the node.
type MeshEnrollRequest struct {
	// SchemaVersion stamps this payload (MeshEnrollSchemaVersion).
	SchemaVersion int32 `json:"schemaVersion"`
	// NodeName is the joining node's name.
	NodeName string `json:"nodeName"`
	// PublicKey is the joining node's wireguard PUBLIC key (base64).
	PublicKey string `json:"publicKey"`
	// Endpoint is the host:port the joining node's wireguard is reachable at.
	Endpoint string `json:"endpoint"`
	// PodCIDR is the node's requested pod /24; empty asks the server to assign
	// one.
	PodCIDR string `json:"podCIDR,omitempty"`
}

// WithDefaults returns a copy with SchemaVersion stamped to
// MeshEnrollSchemaVersion when zero. It does not mutate the receiver.
func (r MeshEnrollRequest) WithDefaults() MeshEnrollRequest {
	out := r
	if out.SchemaVersion == 0 {
		out.SchemaVersion = MeshEnrollSchemaVersion
	}
	return out
}

// Validate reports whether the request is usable: version-stamped and carrying a
// node name, PUBLIC key, and endpoint (PodCIDR may be empty = server-assigned).
// Errors wrap ErrInvalid.
func (r MeshEnrollRequest) Validate() error {
	if r.SchemaVersion == 0 {
		return fmt.Errorf("%w: mesh-enroll request missing schemaVersion (call WithDefaults)", ErrInvalid)
	}
	if r.NodeName == "" {
		return fmt.Errorf("%w: mesh-enroll request missing nodeName", ErrInvalid)
	}
	if r.PublicKey == "" {
		return fmt.Errorf("%w: mesh-enroll request missing publicKey", ErrInvalid)
	}
	if r.Endpoint == "" {
		return fmt.Errorf("%w: mesh-enroll request missing endpoint", ErrInvalid)
	}
	return nil
}

// MeshEnrollResponse is the server's reply to a join: the node's assigned
// podCIDR + mesh-egress IP and the current peer snapshot, so the joining node
// can program its mesh immediately (it then keeps converging via the MeshPeer
// watch). Peers reuse the canonical MeshPeerSpec shape.
type MeshEnrollResponse struct {
	// SchemaVersion stamps this payload (MeshEnrollSchemaVersion).
	SchemaVersion int32 `json:"schemaVersion"`
	// NodeName is the joining node's name (echoed back).
	NodeName string `json:"nodeName"`
	// PodCIDR is the pod /24 the server assigned (or confirmed) for this node.
	PodCIDR string `json:"podCIDR"`
	// MeshIP is the /32 (within PodCIDR) the node should use as its mesh-egress
	// source address.
	MeshIP string `json:"meshIP,omitempty"`
	// Peers is the current mesh snapshot the joining node programs immediately
	// (the existing nodes' public keys + endpoints + podCIDRs).
	Peers []MeshPeerSpec `json:"peers,omitempty"`
}

// WithDefaults returns a copy with SchemaVersion stamped to
// MeshEnrollSchemaVersion when zero. It does not mutate the receiver: Peers is
// deep-copied element-by-element (via MeshPeerSpec.DeepCopyInto), since each
// peer carries its own AllowedIPs slice that a shallow struct copy would still
// share with the receiver.
func (r MeshEnrollResponse) WithDefaults() MeshEnrollResponse {
	out := r
	if out.SchemaVersion == 0 {
		out.SchemaVersion = MeshEnrollSchemaVersion
	}
	if out.Peers != nil {
		peers := make([]MeshPeerSpec, len(out.Peers))
		for i := range out.Peers {
			out.Peers[i].DeepCopyInto(&peers[i])
		}
		out.Peers = peers
	}
	return out
}

// Validate reports whether the response is usable: version-stamped and carrying
// the assigned node name + podCIDR. Errors wrap ErrInvalid.
func (r MeshEnrollResponse) Validate() error {
	if r.SchemaVersion == 0 {
		return fmt.Errorf("%w: mesh-enroll response missing schemaVersion (call WithDefaults)", ErrInvalid)
	}
	if r.NodeName == "" {
		return fmt.Errorf("%w: mesh-enroll response missing nodeName", ErrInvalid)
	}
	if r.PodCIDR == "" {
		return fmt.Errorf("%w: mesh-enroll response missing podCIDR", ErrInvalid)
	}
	return nil
}
