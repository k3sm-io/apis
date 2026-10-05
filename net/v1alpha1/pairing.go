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

// The pairing wire types: plain Go structs (NOT a CRD, NOT proto) that the
// server and a new, not-yet-joined Mac exchange over a direct cable before the
// new Mac has any credential. Each carries a version field from day one so a
// later change has a compatibility seam.

// BeaconVersion is the current Beacon wire version.
const BeaconVersion int32 = 1

// PairVersion is the current PairRequest / PairResponse wire version.
const PairVersion int32 = 1

// Beacon is the small unsigned datagram a server announces on each of its
// direct-link ports.
//
// A beacon is DATA, never an instruction. A receiver treats it only as a hint
// that it may ASK to pair; nothing in it causes an action by itself. It is
// unauthenticated, so every field is a claim by whoever sent it. In particular
// ClusterPin is information, not authentication: it lets a new Mac that was
// told which cluster to join skip a beacon from any other cluster, but a
// matching pin proves nothing about the sender, and every device on the cable
// learns the pin and the node name the beacon carries.
type Beacon struct {
	// Version is the wire version (BeaconVersion).
	Version int32 `json:"version"`
	// ClusterPin is the cluster's CA pin as the sender claims it (information,
	// not authentication; see the type comment).
	ClusterPin string `json:"clusterPin"`
	// NodeName is the sending server's node name, as it claims it.
	NodeName string `json:"nodeName"`
	// JoinPort is the TCP port the server's join listener answers on.
	JoinPort int32 `json:"joinPort"`
	// PairingOpen reports whether the server currently accepts pair requests.
	PairingOpen bool `json:"pairingOpen"`
}

// PairRequest is what a new Mac sends to ask a server for a join credential
// over a direct cable.
type PairRequest struct {
	// Version is the wire version (PairVersion).
	Version int32 `json:"version"`
	// NodeName is the name the new Mac will join as; the credential it receives
	// is bound to this name.
	NodeName string `json:"nodeName"`
}

// PairResponse is the server's answer to an accepted PairRequest.
type PairResponse struct {
	// Version is the wire version (PairVersion).
	Version int32 `json:"version"`
	// Token is the one-shot, short-lived join token, bound to the requested node
	// name. It is a credential: never log it or write it to disk.
	Token string `json:"token"`
	// ServerURL is the URL of the server's join listener on the link the request
	// arrived on.
	ServerURL string `json:"serverURL"`
	// ServerLinkIP is the server's direct-link address on the port the request
	// arrived on, so the new Mac can install its first route to the server
	// before any other link state exists.
	ServerLinkIP string `json:"serverLinkIP"`
}
