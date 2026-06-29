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
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestProtocolValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		p    Protocol
		want bool
	}{
		{"tcp", ProtocolTCP, true},
		{"udp", ProtocolUDP, true},
		{"empty", Protocol(""), false},
		{"sctp", Protocol("SCTP"), false},
		{"lowercase", Protocol("tcp"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.Valid(); got != tc.want {
				t.Fatalf("Protocol(%q).Valid() = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestServicePortValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		port    ServicePort
		wantErr bool
	}{
		{"tcp ok", ServicePort{Port: 80, TargetPort: 8080, Protocol: ProtocolTCP}, false},
		{"udp ok", ServicePort{Port: 53, TargetPort: 53, Protocol: ProtocolUDP}, false},
		{"empty protocol defaults tcp", ServicePort{Port: 80, TargetPort: 8080}, false},
		{"with nodeport", ServicePort{Port: 80, TargetPort: 8080, NodePort: 30080}, false},
		{"zero nodeport ok", ServicePort{Port: 80, TargetPort: 8080, NodePort: 0}, false},
		{"max ports", ServicePort{Port: 65535, TargetPort: 65535, NodePort: 65535}, false},
		{"bad protocol", ServicePort{Port: 80, TargetPort: 8080, Protocol: Protocol("SCTP")}, true},
		{"port zero", ServicePort{Port: 0, TargetPort: 8080}, true},
		{"port too high", ServicePort{Port: 70000, TargetPort: 8080}, true},
		{"target zero", ServicePort{Port: 80, TargetPort: 0}, true},
		{"nodeport too high", ServicePort{Port: 80, TargetPort: 8080, NodePort: 70000}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.port.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error")
				}
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Validate() error %v does not wrap ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestServiceVIPValidate(t *testing.T) {
	t.Parallel()
	good := ServiceVIP{
		Namespace: "default",
		Name:      "web",
		ClusterIP: "10.43.0.10",
		Ports:     []ServicePort{{Port: 80, TargetPort: 8080, Protocol: ProtocolTCP}},
	}
	cases := []struct {
		name    string
		svc     ServiceVIP
		wantErr bool
	}{
		{"ok", good, false},
		{"missing namespace", ServiceVIP{Name: "web", ClusterIP: "10.43.0.10", Ports: good.Ports}, true},
		{"missing name", ServiceVIP{Namespace: "default", ClusterIP: "10.43.0.10", Ports: good.Ports}, true},
		{"missing clusterIP", ServiceVIP{Namespace: "default", Name: "web", Ports: good.Ports}, true},
		{"no ports", ServiceVIP{Namespace: "default", Name: "web", ClusterIP: "10.43.0.10"}, true},
		{"bad port propagates", ServiceVIP{Namespace: "default", Name: "web", ClusterIP: "10.43.0.10", Ports: []ServicePort{{Port: 0, TargetPort: 8080}}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.svc.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error")
				}
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Validate() error %v does not wrap ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestServiceVIPWithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("defaults empty protocol to TCP", func(t *testing.T) {
		t.Parallel()
		in := ServiceVIP{
			Namespace: "default",
			Name:      "web",
			ClusterIP: "10.43.0.10",
			Ports: []ServicePort{
				{Port: 80, TargetPort: 8080},
				{Port: 53, TargetPort: 53, Protocol: ProtocolUDP},
			},
		}
		out := in.WithDefaults()
		if out.Ports[0].Protocol != ProtocolTCP {
			t.Fatalf("port[0] protocol = %q, want TCP", out.Ports[0].Protocol)
		}
		if out.Ports[1].Protocol != ProtocolUDP {
			t.Fatalf("port[1] protocol = %q, want UDP", out.Ports[1].Protocol)
		}
	})

	t.Run("does not mutate receiver", func(t *testing.T) {
		t.Parallel()
		in := ServiceVIP{
			Namespace: "default",
			Name:      "web",
			ClusterIP: "10.43.0.10",
			Ports:     []ServicePort{{Port: 80, TargetPort: 8080}},
		}
		_ = in.WithDefaults()
		if in.Ports[0].Protocol != "" {
			t.Fatalf("receiver mutated: port[0] protocol = %q, want empty", in.Ports[0].Protocol)
		}
	})

	t.Run("nil ports stays nil", func(t *testing.T) {
		t.Parallel()
		in := ServiceVIP{Namespace: "default", Name: "web", ClusterIP: "10.43.0.10"}
		out := in.WithDefaults()
		if out.Ports != nil {
			t.Fatalf("Ports = %v, want nil", out.Ports)
		}
	})
}

func TestEndpointValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		ep      Endpoint
		wantErr bool
	}{
		{"ready ok", Endpoint{IP: "10.42.0.5", Port: 8080, Ready: true}, false},
		{"unready still valid", Endpoint{IP: "10.42.0.5", Port: 8080, Ready: false}, false},
		{"missing ip", Endpoint{Port: 8080, Ready: true}, true},
		{"port zero", Endpoint{IP: "10.42.0.5", Port: 0}, true},
		{"port too high", Endpoint{IP: "10.42.0.5", Port: 70000}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.ep.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error")
				}
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Validate() error %v does not wrap ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestDNSConfigValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     DNSConfig
		wantErr bool
	}{
		{"ok", DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local"}, false},
		{"ok with search + ndots", DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local", SearchDomains: []string{"svc.cluster.local"}, NDots: 2}, false},
		{"zero ndots ok", DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local", NDots: 0}, false},
		{"missing ip", DNSConfig{ClusterDomain: "cluster.local"}, true},
		{"missing domain", DNSConfig{ClusterDNSIP: "10.43.0.10"}, true},
		{"negative ndots", DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local", NDots: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want error")
				}
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Validate() error %v does not wrap ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestDNSConfigWithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("zero ndots becomes default", func(t *testing.T) {
		t.Parallel()
		in := DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local"}
		out := in.WithDefaults()
		if out.NDots != DefaultNDots {
			t.Fatalf("NDots = %d, want %d", out.NDots, DefaultNDots)
		}
	})

	t.Run("explicit ndots preserved", func(t *testing.T) {
		t.Parallel()
		in := DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local", NDots: 1}
		out := in.WithDefaults()
		if out.NDots != 1 {
			t.Fatalf("NDots = %d, want 1", out.NDots)
		}
	})

	t.Run("does not mutate receiver", func(t *testing.T) {
		t.Parallel()
		in := DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local"}
		_ = in.WithDefaults()
		if in.NDots != 0 {
			t.Fatalf("receiver mutated: NDots = %d, want 0", in.NDots)
		}
	})
}

// TestJSONRoundTrip asserts every exported type survives a JSON
// marshal→unmarshal cycle unchanged, so a proxy/server can persist or transport
// these through the watch cache without losing fields.
func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value any
		fresh func() any
	}{
		{
			"ServiceVIP",
			ServiceVIP{
				Namespace: "default",
				Name:      "web",
				ClusterIP: "10.43.0.10",
				Ports: []ServicePort{
					{Name: "http", Port: 80, TargetPort: 8080, Protocol: ProtocolTCP, NodePort: 30080},
					{Name: "dns", Port: 53, TargetPort: 53, Protocol: ProtocolUDP},
				},
			},
			func() any { return &ServiceVIP{} },
		},
		{
			"ServicePort",
			ServicePort{Name: "http", Port: 80, TargetPort: 8080, Protocol: ProtocolTCP, NodePort: 30080},
			func() any { return &ServicePort{} },
		},
		{
			"Endpoint",
			Endpoint{IP: "10.42.0.5", Port: 8080, Ready: true},
			func() any { return &Endpoint{} },
		},
		{
			"DNSConfig",
			DNSConfig{
				ClusterDNSIP:  "10.43.0.10",
				ClusterDomain: "cluster.local",
				SearchDomains: []string{"default.svc.cluster.local", "svc.cluster.local", "cluster.local"},
				NDots:         5,
			},
			func() any { return &DNSConfig{} },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := tc.fresh()
			if err := json.Unmarshal(b, got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			// got is a pointer; dereference to compare against the value form.
			gotVal := reflect.ValueOf(got).Elem().Interface()
			if !reflect.DeepEqual(tc.value, gotVal) {
				t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", gotVal, tc.value)
			}
		})
	}
}

// TestJSONFieldNames pins the wire field names (camelCase, matching corev1) so a
// rename — which would silently break a darwin-net proxy decoding cached JSON —
// fails the build instead.
func TestJSONFieldNames(t *testing.T) {
	t.Parallel()

	t.Run("ServiceVIP", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(ServiceVIP{
			Namespace: "default",
			Name:      "web",
			ClusterIP: "10.43.0.10",
			Ports:     []ServicePort{{Port: 80, TargetPort: 8080, Protocol: ProtocolTCP, NodePort: 30080}},
		})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"namespace", "name", "clusterIP", "ports"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("missing JSON key %q in %s", k, b)
			}
		}
	})

	t.Run("DNSConfig", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(DNSConfig{
			ClusterDNSIP:  "10.43.0.10",
			ClusterDomain: "cluster.local",
			SearchDomains: []string{"svc.cluster.local"},
			NDots:         5,
		})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"clusterDNSIP", "clusterDomain", "searchDomains", "ndots"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("missing JSON key %q in %s", k, b)
			}
		}
	})
}
