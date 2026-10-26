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
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// InterconnectAnnotationSchema is the current schema of the InterconnectReport
// and InterconnectDesired annotation payloads.
const InterconnectAnnotationSchema = 1

// The bounds a reader applies to an InterconnectReport (a node writes it on its
// own Node, so the server treats it as untrusted input).
const (
	// MaxInterconnectReportBytes is the largest report annotation value, in
	// bytes, a reader accepts. The same bound applies to the desired
	// annotation.
	MaxInterconnectReportBytes = 4096
	// MaxInterconnectReportPeers is the most reachable peers (and the most peer
	// domain UUIDs) a report may list.
	MaxInterconnectReportPeers = 16
	// MaxInterconnectDomainUUIDBytes is the longest domain UUID a report may
	// carry. Each must be printable ASCII, so a node cannot inject control
	// characters into the Events and logs that quote it.
	MaxInterconnectDomainUUIDBytes = 64
	// MaxInterconnectUIDBytes is the longest object UID a desired payload may
	// carry in MLXModels, in bytes (a Kubernetes UID is 36).
	MaxInterconnectUIDBytes = 64
	// InterconnectReportMinInterval is the shortest interval at which a node
	// rewrites its report.
	InterconnectReportMinInterval = 10 * time.Second
)

var (
	// ErrUnknownInterconnectSchema reports an annotation payload whose schema
	// is missing or is not InterconnectAnnotationSchema. A reader ignores such a
	// payload (the server records an Event); it is not a node error to retry.
	ErrUnknownInterconnectSchema = errors.New("netv1alpha1: unknown interconnect annotation schema")
	// ErrInvalidInterconnectAnnotation is wrapped by every other annotation
	// payload error: oversize, too many peers, malformed JSON, or a bad field.
	ErrInvalidInterconnectAnnotation = errors.New("netv1alpha1: invalid interconnect annotation")
)

// InterconnectReport is the payload of the AnnotationInterconnectReport
// annotation, the JSON a node writes on its own Node object to report its cable
// side. It is advisory and untrusted: the server checks that peer names are
// existing Nodes, recomputes every address itself, and counts adjacency only
// when both ends report it.
//
// Readers ignore unknown fields, and ignore a payload with an unknown schema
// (ParseInterconnectReport returns ErrUnknownInterconnectSchema so the caller
// can record an Event and move on). An incompatible change bumps Schema.
type InterconnectReport struct {
	// Schema is the payload version (InterconnectAnnotationSchema).
	Schema int `json:"schema"`
	// LocalDomainUUIDs identify the node's own cable-port ends.
	LocalDomainUUIDs []string `json:"localDomainUUIDs,omitempty"`
	// PeerDomainUUIDs identify the far ends of the node's plugged cables.
	PeerDomainUUIDs []string `json:"peerDomainUUIDs,omitempty"`
	// Carrier reports that at least one cable port has link.
	Carrier bool `json:"carrier"`
	// SpeedGbps is the negotiated link speed in Gb/s (0 = unknown).
	SpeedGbps int32 `json:"speedGbps,omitempty"`
	// ReachablePeers are the node names the node reaches over the cable (from
	// its probe of each peer's slot-0 address).
	ReachablePeers []string `json:"reachablePeers,omitempty"`
}

// Validate reports whether the report is within the reader bounds above on
// its own:
// Schema is current (else ErrUnknownInterconnectSchema), at most
// MaxInterconnectReportPeers reachable peers and peer domain UUIDs, local
// domain UUIDs no more than MaxInterconnectSlot+1, every UUID 1 to
// MaxInterconnectDomainUUIDBytes bytes of printable ASCII, SpeedGbps not
// negative, and every peer a node-name shape listed once. Other errors wrap
// ErrInvalidInterconnectAnnotation.
func (r InterconnectReport) Validate() error {
	if r.Schema != InterconnectAnnotationSchema {
		return fmt.Errorf("%w: report schema %d, want %d", ErrUnknownInterconnectSchema, r.Schema, InterconnectAnnotationSchema)
	}
	if n := len(r.ReachablePeers); n > MaxInterconnectReportPeers {
		return fmt.Errorf("%w: report lists %d reachable peers, at most %d", ErrInvalidInterconnectAnnotation, n, MaxInterconnectReportPeers)
	}
	if n := len(r.PeerDomainUUIDs); n > MaxInterconnectReportPeers {
		return fmt.Errorf("%w: report lists %d peer domain UUIDs, at most %d", ErrInvalidInterconnectAnnotation, n, MaxInterconnectReportPeers)
	}
	if n := len(r.LocalDomainUUIDs); n > MaxInterconnectSlot+1 {
		return fmt.Errorf("%w: report lists %d local domain UUIDs, at most %d", ErrInvalidInterconnectAnnotation, n, MaxInterconnectSlot+1)
	}
	for i, u := range r.LocalDomainUUIDs {
		if !isDomainUUID(u) {
			return fmt.Errorf("%w: report localDomainUUIDs[%d] is not 1..%d printable ASCII bytes", ErrInvalidInterconnectAnnotation, i, MaxInterconnectDomainUUIDBytes)
		}
	}
	for i, u := range r.PeerDomainUUIDs {
		if !isDomainUUID(u) {
			return fmt.Errorf("%w: report peerDomainUUIDs[%d] is not 1..%d printable ASCII bytes", ErrInvalidInterconnectAnnotation, i, MaxInterconnectDomainUUIDBytes)
		}
	}
	if r.SpeedGbps < 0 {
		return fmt.Errorf("%w: report speedGbps %d is negative", ErrInvalidInterconnectAnnotation, r.SpeedGbps)
	}
	seen := make(map[string]bool, len(r.ReachablePeers))
	for i, p := range r.ReachablePeers {
		if !isNodeName(p) {
			return fmt.Errorf("%w: report reachablePeers[%d] %q is not a node name", ErrInvalidInterconnectAnnotation, i, p)
		}
		if seen[p] {
			return fmt.Errorf("%w: report reachablePeers[%d] %q is repeated", ErrInvalidInterconnectAnnotation, i, p)
		}
		seen[p] = true
	}
	return nil
}

// Encode validates the report and returns the annotation value. It refuses a
// payload larger than MaxInterconnectReportBytes, so a writer can never produce
// a value its reader rejects.
func (r InterconnectReport) Encode() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return encodeInterconnectAnnotation(r)
}

// ParseInterconnectReport decodes and validates an AnnotationInterconnectReport
// value. The size bound is checked before any decoding. A missing or unknown
// schema returns ErrUnknownInterconnectSchema; every other failure wraps
// ErrInvalidInterconnectAnnotation. That peer names are existing Nodes is the
// caller's (the server's) check.
func ParseInterconnectReport(value string) (InterconnectReport, error) {
	var r InterconnectReport
	if err := decodeInterconnectAnnotation(value, &r); err != nil {
		return InterconnectReport{}, err
	}
	if err := r.Validate(); err != nil {
		return InterconnectReport{}, err
	}
	return r, nil
}

// InterconnectDesired is the payload of the AnnotationInterconnectDesired
// annotation, which the server writes on a Node to tell it which Interconnect
// it is an armed member of. Absent means the node is unconfigured.
//
// It is advisory: a node can rewrite its own annotation, so the network helper
// recomputes and bounds every address itself. Readers ignore unknown fields,
// and ignore a payload with an unknown schema (ParseInterconnectDesired returns
// ErrUnknownInterconnectSchema).
type InterconnectDesired struct {
	// Schema is the payload version (InterconnectAnnotationSchema).
	Schema int `json:"schema"`
	// Interconnect is the name of the Interconnect the node is a member of.
	Interconnect string `json:"interconnect"`
	// Fallback is the Interconnect's fallback, for the node's pod attach.
	Fallback InterconnectFallback `json:"fallback"`
	// AllowedNamespaces is a copy of the Interconnect's
	// Spec.AllowedNamespaces, so the node's pod attach can apply the namespace
	// rule without reading the Interconnect. Empty means every namespace.
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
	// MLXModels are the existing MLXModels that use an interconnect, so the
	// node's pod attach can verify a slot-1 pod's controller ownerReference
	// (its namespace, name and UID) without reading MLXModel.
	MLXModels []InterconnectModelRef `json:"mlxModels,omitempty"`
}

// InterconnectModelRef names one MLXModel by namespace, name and UID. A pod's
// controller ownerReference matches it only when all three agree (the pod's
// own namespace standing for the reference's), so a reference copied into a pod
// in another namespace, or under another name, matches nothing.
type InterconnectModelRef struct {
	// Namespace is the MLXModel's namespace.
	Namespace string `json:"namespace"`
	// Name is the MLXModel's name.
	Name string `json:"name"`
	// UID is the MLXModel's UID.
	UID string `json:"uid"`
}

// NamespaceAllowed reports whether pods in namespace ns may attach: an EMPTY
// AllowedNamespaces allows every namespace, a non-empty one only the
// namespaces it lists (the InterconnectSpec.NamespaceAllowed rule).
func (d InterconnectDesired) NamespaceAllowed(ns string) bool {
	return InterconnectSpec{AllowedNamespaces: d.AllowedNamespaces}.NamespaceAllowed(ns)
}

// HasMLXModel reports whether MLXModels lists the model with this namespace,
// name and UID, all three. An empty field is never a match.
func (d InterconnectDesired) HasMLXModel(namespace, name, uid string) bool {
	if namespace == "" || name == "" || uid == "" {
		return false
	}
	for _, m := range d.MLXModels {
		if m.Namespace == namespace && m.Name == name && m.UID == uid {
			return true
		}
	}
	return false
}

// Validate reports whether the payload is well-formed: Schema is current (else
// ErrUnknownInterconnectSchema), Interconnect is an object-name shape,
// Fallback is "network" or "none", AllowedNamespaces holds at most
// MaxInterconnectListItems distinct namespace-name shapes (RFC 1123 labels),
// and MLXModels holds at most MaxInterconnectListItems entries, each with a
// namespace-name shape, an object-name shape (an RFC 1123 subdomain) and a UID
// of 1 to MaxInterconnectUIDBytes printable ASCII bytes, no namespace/name and
// no UID repeated. Other errors wrap ErrInvalidInterconnectAnnotation.
func (d InterconnectDesired) Validate() error {
	if d.Schema != InterconnectAnnotationSchema {
		return fmt.Errorf("%w: desired schema %d, want %d", ErrUnknownInterconnectSchema, d.Schema, InterconnectAnnotationSchema)
	}
	if !isNodeName(d.Interconnect) {
		return fmt.Errorf("%w: desired interconnect %q is not an object name", ErrInvalidInterconnectAnnotation, d.Interconnect)
	}
	switch d.Fallback {
	case InterconnectFallbackNetwork, InterconnectFallbackNone:
	default:
		return fmt.Errorf("%w: desired fallback %q, want %q or %q", ErrInvalidInterconnectAnnotation, d.Fallback, InterconnectFallbackNetwork, InterconnectFallbackNone)
	}
	if n := len(d.AllowedNamespaces); n > MaxInterconnectListItems {
		return fmt.Errorf("%w: desired allowedNamespaces has %d entries, at most %d", ErrInvalidInterconnectAnnotation, n, MaxInterconnectListItems)
	}
	if n := len(d.MLXModels); n > MaxInterconnectListItems {
		return fmt.Errorf("%w: desired mlxModels has %d entries, at most %d", ErrInvalidInterconnectAnnotation, n, MaxInterconnectListItems)
	}
	seen := make(map[string]bool, len(d.AllowedNamespaces))
	for i, ns := range d.AllowedNamespaces {
		if !isNamespaceName(ns) {
			return fmt.Errorf("%w: desired allowedNamespaces[%d] %q is not a namespace name", ErrInvalidInterconnectAnnotation, i, ns)
		}
		if seen[ns] {
			return fmt.Errorf("%w: desired allowedNamespaces[%d] %q is repeated", ErrInvalidInterconnectAnnotation, i, ns)
		}
		seen[ns] = true
	}
	names := make(map[string]bool, len(d.MLXModels))
	uids := make(map[string]bool, len(d.MLXModels))
	for i, m := range d.MLXModels {
		if !isNamespaceName(m.Namespace) {
			return fmt.Errorf("%w: desired mlxModels[%d] namespace %q is not a namespace name", ErrInvalidInterconnectAnnotation, i, m.Namespace)
		}
		if !isNodeName(m.Name) {
			return fmt.Errorf("%w: desired mlxModels[%d] name %q is not an object name", ErrInvalidInterconnectAnnotation, i, m.Name)
		}
		if !isPrintableASCII(m.UID, MaxInterconnectUIDBytes) {
			return fmt.Errorf("%w: desired mlxModels[%d] uid is not 1..%d printable ASCII bytes", ErrInvalidInterconnectAnnotation, i, MaxInterconnectUIDBytes)
		}
		key := m.Namespace + "/" + m.Name
		if names[key] || uids[m.UID] {
			return fmt.Errorf("%w: desired mlxModels[%d] %s (uid %q) is repeated", ErrInvalidInterconnectAnnotation, i, key, m.UID)
		}
		names[key], uids[m.UID] = true, true
	}
	return nil
}

// Encode validates the payload and returns the annotation value.
func (d InterconnectDesired) Encode() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	return encodeInterconnectAnnotation(d)
}

// ParseInterconnectDesired decodes and validates an
// AnnotationInterconnectDesired value, with the same size bound and errors as
// ParseInterconnectReport.
func ParseInterconnectDesired(value string) (InterconnectDesired, error) {
	var d InterconnectDesired
	if err := decodeInterconnectAnnotation(value, &d); err != nil {
		return InterconnectDesired{}, err
	}
	if err := d.Validate(); err != nil {
		return InterconnectDesired{}, err
	}
	return d, nil
}

// encodeInterconnectAnnotation marshals v and enforces the size bound.
func encodeInterconnectAnnotation(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidInterconnectAnnotation, err)
	}
	if len(b) > MaxInterconnectReportBytes {
		return "", fmt.Errorf("%w: payload is %d bytes, at most %d", ErrInvalidInterconnectAnnotation, len(b), MaxInterconnectReportBytes)
	}
	return string(b), nil
}

// decodeInterconnectAnnotation enforces the size bound, reads the schema on
// its own, and only for the current schema decodes value into v. Reading the
// schema first means a future schema whose fields changed type is reported as
// ErrUnknownInterconnectSchema, not as malformed. Unknown fields are ignored.
func decodeInterconnectAnnotation(value string, v any) error {
	if len(value) > MaxInterconnectReportBytes {
		return fmt.Errorf("%w: payload is %d bytes, at most %d", ErrInvalidInterconnectAnnotation, len(value), MaxInterconnectReportBytes)
	}
	var probe struct {
		Schema *int `json:"schema"`
	}
	if err := json.Unmarshal([]byte(value), &probe); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInterconnectAnnotation, err)
	}
	if probe.Schema == nil {
		return fmt.Errorf("%w: payload has no schema", ErrUnknownInterconnectSchema)
	}
	if *probe.Schema != InterconnectAnnotationSchema {
		return fmt.Errorf("%w: schema %d, want %d", ErrUnknownInterconnectSchema, *probe.Schema, InterconnectAnnotationSchema)
	}
	if err := json.Unmarshal([]byte(value), v); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInterconnectAnnotation, err)
	}
	return nil
}

// isDomainUUID reports whether u is 1..MaxInterconnectDomainUUIDBytes bytes of
// printable ASCII (0x20..0x7e).
func isDomainUUID(u string) bool {
	return isPrintableASCII(u, MaxInterconnectDomainUUIDBytes)
}

// isPrintableASCII reports whether u is 1..max bytes of printable ASCII
// (0x20..0x7e).
func isPrintableASCII(u string, max int) bool {
	if u == "" || len(u) > max {
		return false
	}
	for i := 0; i < len(u); i++ {
		if u[i] < 0x20 || u[i] > 0x7e {
			return false
		}
	}
	return true
}
