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
	"errors"
	"fmt"
)

// Protocol is the L4 transport of a Service port. It is a small typed string
// mirroring corev1.Protocol so values round-trip through the Kubernetes watch
// cache unchanged. The userspace Service proxy only handles TCP and UDP; SCTP
// is intentionally not modeled (k3sm does not proxy it).
type Protocol string

const (
	// ProtocolTCP is the TCP transport — the default when a port omits one.
	ProtocolTCP Protocol = "TCP"
	// ProtocolUDP is the UDP transport.
	ProtocolUDP Protocol = "UDP"
)

// Valid reports whether p is a protocol the userspace Service proxy can serve
// (TCP or UDP). The empty value is NOT valid: callers should default it to
// ProtocolTCP explicitly before validating, matching Kubernetes semantics.
func (p Protocol) Valid() bool {
	switch p {
	case ProtocolTCP, ProtocolUDP:
		return true
	default:
		return false
	}
}

// ErrInvalid is returned by Validate methods in this package when a value is not
// usable by the proxy. Wrap it via %w; test for it with errors.Is.
var ErrInvalid = errors.New("netv1: invalid service definition")

// ServicePort is one port the Service proxy listens on for a ServiceVIP. The
// proxy binds Port on the cluster IP (an lo0 alias) and load-balances to
// Endpoint.Port (the TargetPort) on the chosen backend. NodePort, when non-zero,
// is additionally bound on all node interfaces (*:NodePort).
type ServicePort struct {
	// Name identifies the port within a multi-port Service. It is empty for a
	// single-port Service and otherwise unique within the Service (matching the
	// corev1 rule). It is the key EndpointSlice uses to associate endpoints.
	Name string `json:"name,omitempty"`
	// Port is the port the proxy exposes on the cluster IP (the Service port).
	Port int32 `json:"port"`
	// TargetPort is the backend port the proxy dials on an Endpoint. It mirrors
	// the resolved (numeric) corev1 targetPort; named targetPorts are resolved
	// upstream before reaching the proxy.
	TargetPort int32 `json:"targetPort"`
	// Protocol is the L4 transport (TCP or UDP). Empty means TCP by Kubernetes
	// convention; use WithDefaults to normalize before serving.
	Protocol Protocol `json:"protocol,omitempty"`
	// NodePort, when non-zero, is the port bound on all node interfaces
	// (*:NodePort) for type=NodePort/LoadBalancer Services. Zero means the
	// Service is ClusterIP-only and no node-wide socket is opened.
	NodePort int32 `json:"nodePort,omitempty"`
}

// Validate reports whether the port is usable by the proxy: a valid protocol
// and in-range Port / TargetPort (1–65535), with NodePort either zero or in
// range. It defaults an empty Protocol to TCP for the check (it does not mutate
// the receiver). Errors wrap ErrInvalid.
func (sp ServicePort) Validate() error {
	proto := sp.Protocol
	if proto == "" {
		proto = ProtocolTCP
	}
	if !proto.Valid() {
		return fmt.Errorf("%w: port %q protocol %q", ErrInvalid, sp.Name, sp.Protocol)
	}
	if !validPort(sp.Port) {
		return fmt.Errorf("%w: port %q port %d out of range", ErrInvalid, sp.Name, sp.Port)
	}
	if !validPort(sp.TargetPort) {
		return fmt.Errorf("%w: port %q targetPort %d out of range", ErrInvalid, sp.Name, sp.TargetPort)
	}
	if sp.NodePort != 0 && !validPort(sp.NodePort) {
		return fmt.Errorf("%w: port %q nodePort %d out of range", ErrInvalid, sp.Name, sp.NodePort)
	}
	return nil
}

// ServiceVIP is one ClusterIP Service the userspace proxy owns. The proxy aliases
// ClusterIP onto lo0, binds each Ports entry, and L4-load-balances accepted
// connections to the ready Endpoints for the matching port (local lo0 pod IPs or
// remote pods over the mesh). Namespace and Name give it identity in the watch
// cache; they are not used on the wire.
type ServiceVIP struct {
	// Namespace is the Service's namespace — half of its cluster-unique identity.
	Namespace string `json:"namespace"`
	// Name is the Service's name — the other half of its identity.
	Name string `json:"name"`
	// ClusterIP is the virtual IP the proxy owns (aliased on lo0). It is a single
	// allocated address; headless Services (no cluster IP) are not represented
	// here because the proxy has nothing to bind for them.
	ClusterIP string `json:"clusterIP"`
	// Ports are the ports the proxy serves on ClusterIP. A Service has at least
	// one; each is bound independently.
	Ports []ServicePort `json:"ports"`
}

// WithDefaults returns a copy of the Service with each port's empty Protocol set
// to ProtocolTCP (the Kubernetes default). It does not mutate the receiver, so
// it is safe to call on a shared cache object.
func (s ServiceVIP) WithDefaults() ServiceVIP {
	out := s
	if s.Ports != nil {
		out.Ports = make([]ServicePort, len(s.Ports))
		copy(out.Ports, s.Ports)
		for i := range out.Ports {
			if out.Ports[i].Protocol == "" {
				out.Ports[i].Protocol = ProtocolTCP
			}
		}
	}
	return out
}

// Validate reports whether the Service is usable by the proxy: it has an
// identity (Namespace and Name), a ClusterIP to bind, at least one port, and
// every port validates. Errors wrap ErrInvalid.
func (s ServiceVIP) Validate() error {
	if s.Namespace == "" || s.Name == "" {
		return fmt.Errorf("%w: service missing namespace/name", ErrInvalid)
	}
	if s.ClusterIP == "" {
		return fmt.Errorf("%w: service %s/%s missing clusterIP", ErrInvalid, s.Namespace, s.Name)
	}
	if len(s.Ports) == 0 {
		return fmt.Errorf("%w: service %s/%s has no ports", ErrInvalid, s.Namespace, s.Name)
	}
	for _, p := range s.Ports {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("service %s/%s: %w", s.Namespace, s.Name, err)
		}
	}
	return nil
}

// Endpoint is one backend the proxy can load-balance a Service port to: an
// address:port tuple plus readiness. It is the flattened tuple the proxy needs
// from an EndpointSlice — backend identity (Pod name, node) lives in the cache,
// not here.
type Endpoint struct {
	// IP is the backend address the proxy dials (a local lo0 pod IP or a remote
	// pod IP reachable over the mesh).
	IP string `json:"ip"`
	// Port is the backend port to dial. It equals the resolved TargetPort of the
	// ServicePort this endpoint backs.
	Port int32 `json:"port"`
	// Ready reports whether the backend has passed its readiness gate. The proxy
	// only steers new connections to ready endpoints; an unready endpoint is kept
	// (for graceful drain / accurate counts) but not selected.
	Ready bool `json:"ready"`
}

// Validate reports whether the endpoint is dialable: a non-empty IP and an
// in-range Port. Readiness is not a validity condition (an unready endpoint is
// still a well-formed tuple). Errors wrap ErrInvalid.
func (e Endpoint) Validate() error {
	if e.IP == "" {
		return fmt.Errorf("%w: endpoint missing ip", ErrInvalid)
	}
	if !validPort(e.Port) {
		return fmt.Errorf("%w: endpoint %s port %d out of range", ErrInvalid, e.IP, e.Port)
	}
	return nil
}

// validPort reports whether p is a usable TCP/UDP port number (1–65535).
func validPort(p int32) bool {
	return p >= 1 && p <= 65535
}
