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
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fixedMeshTime is a deterministic, second-precision UTC instant so metav1.Time
// status fields round-trip losslessly through RFC3339 JSON.
var fixedMeshTime = metav1.NewTime(time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC))

// sampleMeshPeer is a fully-populated MeshPeer (incl. a status time) used across
// the round-trip / deep-copy cases.
func sampleMeshPeer() *MeshPeer {
	return &MeshPeer{
		TypeMeta:   metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "MeshPeer"},
		ObjectMeta: metav1.ObjectMeta{Name: "studio-1", ResourceVersion: "42", Labels: map[string]string{"k3sm.io/role": "worker"}},
		Spec: MeshPeerSpec{
			SchemaVersion: MeshPeerSchemaVersion,
			NodeName:      "studio-1",
			PublicKey:     "fakeBase64PublicKey0000000000000000000000000=",
			Endpoint:      "192.168.1.20:51820",
			Endpoints: []EndpointCandidate{
				{Address: "192.168.1.20:51820", Link: EndpointLinkUnderlay},
				{Address: "169.254.0.9:51820", Link: EndpointLinkDirect},
			},
			PodCIDR:                    "100.64.1.0/24",
			AllowedIPs:                 []string{"100.64.1.0/24"},
			MeshIP:                     "100.64.1.1",
			PersistentKeepaliveSeconds: DefaultPersistentKeepaliveSeconds,
		},
		Status: MeshPeerStatus{
			LastHandshakeTime:     &fixedMeshTime,
			Reachable:             true,
			ObservedSchemaVersion: MeshPeerSchemaVersion,
		},
	}
}

// TestMeshPeerJSONRoundTrip asserts a MeshPeer (incl. its status metav1.Time)
// survives a JSON marshal→unmarshal cycle losslessly — the kine-stored,
// apiserver-served wire form. It is byte-stable (re-marshal equality) to avoid
// time.Time location/monotonic DeepEqual pitfalls, and explicitly re-checks the
// version stamp survives. M3.2 acceptance evidence (the type did not exist
// before).
func TestMeshPeerJSONRoundTrip(t *testing.T) {
	t.Parallel()
	orig := sampleMeshPeer()
	b1, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got MeshPeer
	if err := json.Unmarshal(b1, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b2, err := json.Marshal(&got)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("round-trip not byte-stable:\n got: %s\nwant: %s", b2, b1)
	}
	if got.Spec.SchemaVersion != MeshPeerSchemaVersion {
		t.Fatalf("schemaVersion lost in round-trip: got %d, want %d", got.Spec.SchemaVersion, MeshPeerSchemaVersion)
	}
	if got.Status.LastHandshakeTime == nil || !got.Status.LastHandshakeTime.Equal(&fixedMeshTime) {
		t.Fatalf("status time lost in round-trip: %v", got.Status.LastHandshakeTime)
	}
}

// TestMeshPeerSpecJSONRoundTrip asserts the spec round-trips losslessly via
// DeepEqual (no time fields), pinning the camelCase field names too.
func TestMeshPeerSpecJSONRoundTrip(t *testing.T) {
	t.Parallel()
	spec := sampleMeshPeer().Spec
	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got MeshPeerSpec
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(spec, got) {
		t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", got, spec)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schemaVersion", "nodeName", "publicKey", "endpoint", "endpoints", "podCIDR", "allowedIPs", "meshIP"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing JSON key %q in %s", k, b)
		}
	}
}

// TestMeshPeerDeepCopy asserts DeepCopy produces an independent object: mutating
// the copy's slice / status pointer must not touch the original (the property a
// client-go informer cache depends on).
func TestMeshPeerDeepCopy(t *testing.T) {
	t.Parallel()
	orig := sampleMeshPeer()
	cp := orig.DeepCopy()
	if cp == orig {
		t.Fatal("DeepCopy returned the same pointer")
	}
	cp.Spec.AllowedIPs[0] = "10.0.0.0/8"
	cp.Spec.Endpoints[0].Address = "10.0.0.1:51820"
	cp.Spec.NodeName = "mutated"
	cp.Status.Reachable = false
	if orig.Spec.AllowedIPs[0] != "100.64.1.0/24" {
		t.Fatalf("DeepCopy shared AllowedIPs backing array: %v", orig.Spec.AllowedIPs)
	}
	if orig.Spec.Endpoints[0].Address != "192.168.1.20:51820" {
		t.Fatalf("DeepCopy shared Endpoints backing array: %v", orig.Spec.Endpoints)
	}
	if orig.Spec.NodeName != "studio-1" || !orig.Status.Reachable {
		t.Fatalf("DeepCopy shared scalar state: %#v", orig.Spec)
	}
	// The status time pointer must be a distinct allocation.
	if cp.Status.LastHandshakeTime == orig.Status.LastHandshakeTime {
		t.Fatal("DeepCopy shared the status time pointer")
	}
	// DeepCopyObject returns a runtime.Object that is also independent.
	if _, ok := orig.DeepCopyObject().(*MeshPeer); !ok {
		t.Fatal("DeepCopyObject did not return *MeshPeer")
	}
}

// TestMeshPeerListDeepCopy asserts the list deep-copies its items independently.
func TestMeshPeerListDeepCopy(t *testing.T) {
	t.Parallel()
	list := &MeshPeerList{
		TypeMeta: metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "MeshPeerList"},
		Items:    []MeshPeer{*sampleMeshPeer()},
	}
	cp := list.DeepCopy()
	cp.Items[0].Spec.NodeName = "mutated"
	if list.Items[0].Spec.NodeName != "studio-1" {
		t.Fatalf("list DeepCopy shared item state: %q", list.Items[0].Spec.NodeName)
	}
	if _, ok := list.DeepCopyObject().(*MeshPeerList); !ok {
		t.Fatal("DeepCopyObject did not return *MeshPeerList")
	}
}

// TestMeshPeerGVK is the table test that the MeshPeer types register under the
// net.k3sm.io group (the CRD's GVK). A wrong group means darwin-net's informer
// watches the wrong resource path.
func TestMeshPeerGVK(t *testing.T) {
	t.Parallel()

	if SchemeGroupVersion.Group != "net.k3sm.io" {
		t.Fatalf("SchemeGroupVersion.Group = %q, want net.k3sm.io", SchemeGroupVersion.Group)
	}
	if GroupName != "net.k3sm.io" {
		t.Fatalf("GroupName = %q, want net.k3sm.io", GroupName)
	}

	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	cases := []struct {
		name string
		obj  runtime.Object
		want schema.GroupVersionKind
	}{
		{"MeshPeer", &MeshPeer{}, schema.GroupVersionKind{Group: "net.k3sm.io", Version: "v1", Kind: "MeshPeer"}},
		{"MeshPeerList", &MeshPeerList{}, schema.GroupVersionKind{Group: "net.k3sm.io", Version: "v1", Kind: "MeshPeerList"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gvks, _, err := s.ObjectKinds(tc.obj)
			if err != nil {
				t.Fatalf("ObjectKinds: %v", err)
			}
			found := false
			for _, gvk := range gvks {
				if gvk == tc.want {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("registered GVKs %v do not include %v", gvks, tc.want)
			}
		})
	}

	// Resource maps into the same group (the REST-path helper).
	if got := Resource("meshpeers"); got.Group != "net.k3sm.io" || got.Resource != "meshpeers" {
		t.Fatalf("Resource(meshpeers) = %v, want net.k3sm.io/meshpeers", got)
	}
}

func TestMeshPeerSpecValidate(t *testing.T) {
	t.Parallel()
	good := sampleMeshPeer().Spec
	cases := []struct {
		name    string
		mutate  func(*MeshPeerSpec)
		wantErr bool
	}{
		{"ok", func(*MeshPeerSpec) {}, false},
		{"unstamped version", func(s *MeshPeerSpec) { s.SchemaVersion = 0 }, true},
		{"missing nodeName", func(s *MeshPeerSpec) { s.NodeName = "" }, true},
		{"missing publicKey", func(s *MeshPeerSpec) { s.PublicKey = "" }, true},
		{"missing endpoint", func(s *MeshPeerSpec) { s.Endpoint = "" }, true},
		{"missing podCIDR", func(s *MeshPeerSpec) { s.PodCIDR = "" }, true},
		{"no allowedIPs", func(s *MeshPeerSpec) { s.AllowedIPs = nil }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := good.DeepCopy()
			tc.mutate(spec)
			err := spec.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatal("Validate() = nil, want error")
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

// TestMeshPeerEndpointMustBeHostPort pins the endpoint syntax both sides of the
// mesh contract accept. A node republishes its endpoint periodically, so a
// malformed value would otherwise be written once and fanned out to every peer,
// failing only at wireguard-configuration time on each of them. MeshPeerSpec
// and MeshEnrollRequest must agree exactly — they share validateHostPort.
func TestMeshPeerEndpointMustBeHostPort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"ipv4 host:port", "192.0.2.111:51820", false},
		{"dns host:port", "host.local:51820", false},
		{"bracketed ipv6", "[fd00::1]:51820", false},
		{"zoned link-local ipv6", "[fe80::1%en2]:51820", false},
		{"zoned ipv4", "192.0.2.111%en0:51820", true},
		{"empty", "", true},
		{"no port", "192.0.2.111", true},
		{"port zero", "192.0.2.111:0", true},
		{"port out of range", "192.0.2.111:70000", true},
		{"unbracketed ipv6", "fd00::1:51820", true},
		{"non-numeric port", "192.0.2.111:abc", true},
		{"leading space", " 192.0.2.111:51820", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := sampleMeshPeer().Spec
			spec.Endpoint = tc.endpoint
			specErr := spec.Validate()

			req := MeshEnrollRequest{
				NodeName:  "studio-1",
				PublicKey: "fakeBase64PublicKey0000000000000000000000000=",
				Endpoint:  tc.endpoint,
			}.WithDefaults()
			reqErr := req.Validate()

			for _, got := range []struct {
				who string
				err error
			}{{"MeshPeerSpec.Validate", specErr}, {"MeshEnrollRequest.Validate", reqErr}} {
				if tc.wantErr {
					if got.err == nil {
						t.Fatalf("%s(%q) = nil, want error", got.who, tc.endpoint)
					}
					if !errors.Is(got.err, ErrInvalid) {
						t.Fatalf("%s(%q) error %v does not wrap ErrInvalid", got.who, tc.endpoint, got.err)
					}
					continue
				}
				if got.err != nil {
					t.Fatalf("%s(%q) = %v, want nil", got.who, tc.endpoint, got.err)
				}
			}
		})
	}
}

func TestMeshPeerSpecWithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("stamps version + keepalive", func(t *testing.T) {
		t.Parallel()
		out := MeshPeerSpec{NodeName: "n", PublicKey: "k", Endpoint: "192.168.1.21:51820", PodCIDR: "100.64.2.0/24", AllowedIPs: []string{"100.64.2.0/24"}}.WithDefaults()
		if out.SchemaVersion != MeshPeerSchemaVersion {
			t.Fatalf("SchemaVersion = %d, want %d", out.SchemaVersion, MeshPeerSchemaVersion)
		}
		if out.PersistentKeepaliveSeconds != DefaultPersistentKeepaliveSeconds {
			t.Fatalf("keepalive = %d, want %d", out.PersistentKeepaliveSeconds, DefaultPersistentKeepaliveSeconds)
		}
		if err := out.Validate(); err != nil {
			t.Fatalf("defaulted spec must validate: %v", err)
		}
	})

	t.Run("does not mutate receiver", func(t *testing.T) {
		t.Parallel()
		in := MeshPeerSpec{AllowedIPs: []string{"100.64.2.0/24"}}
		_ = in.WithDefaults()
		if in.SchemaVersion != 0 {
			t.Fatalf("receiver mutated: SchemaVersion = %d", in.SchemaVersion)
		}
	})

	t.Run("does not alias Endpoints", func(t *testing.T) {
		t.Parallel()
		in := MeshPeerSpec{NodeName: "n", Endpoints: []EndpointCandidate{{Address: "192.168.1.21:51820", Link: EndpointLinkUnderlay}}}
		out := in.WithDefaults()

		out.Endpoints[0].Address = "mutated"

		if in.Endpoints[0].Address != "192.168.1.21:51820" {
			t.Fatalf("receiver Endpoints aliased: got %v", in.Endpoints)
		}
	})

	t.Run("does not alias AllowedIPs", func(t *testing.T) {
		t.Parallel()
		in := MeshPeerSpec{NodeName: "n", AllowedIPs: []string{"100.64.2.0/24"}}
		out := in.WithDefaults()

		out.AllowedIPs[0] = "mutated"
		out.AllowedIPs = append(out.AllowedIPs, "100.64.3.0/24")

		if in.AllowedIPs[0] != "100.64.2.0/24" || len(in.AllowedIPs) != 1 {
			t.Fatalf("receiver AllowedIPs aliased: got %v", in.AllowedIPs)
		}
	})
}

// TestMeshEnrollResponseWithDefaults asserts WithDefaults does not mutate the
// receiver's Peers: each MeshPeerSpec carries its own AllowedIPs slice, so a
// mutation through the returned copy's Peers must not corrupt the original
// snapshot the caller may still be holding.
func TestMeshEnrollResponseWithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("does not alias peer AllowedIPs", func(t *testing.T) {
		t.Parallel()
		in := MeshEnrollResponse{
			NodeName: "studio-1",
			PodCIDR:  "100.64.1.0/24",
			Peers: []MeshPeerSpec{
				{NodeName: "studio-2", AllowedIPs: []string{"100.64.2.0/24"}},
			},
		}
		out := in.WithDefaults()

		out.Peers[0].AllowedIPs[0] = "mutated"
		out.Peers[0].AllowedIPs = append(out.Peers[0].AllowedIPs, "100.64.3.0/24")

		if in.Peers[0].AllowedIPs[0] != "100.64.2.0/24" || len(in.Peers[0].AllowedIPs) != 1 {
			t.Fatalf("receiver peer AllowedIPs aliased: got %v", in.Peers[0].AllowedIPs)
		}
	})
}

// TestMeshEnrollRoundTrip asserts the version-stamped enroll payloads round-trip
// losslessly and carry a version. M3.2 acceptance evidence.
func TestMeshEnrollRoundTrip(t *testing.T) {
	t.Parallel()

	req := MeshEnrollRequest{
		NodeName:  "studio-1",
		PublicKey: "fakeBase64PublicKey0000000000000000000000000=",
		Endpoint:  "192.168.1.20:51820",
		PodCIDR:   "100.64.1.0/24",
	}.WithDefaults()
	resp := MeshEnrollResponse{
		NodeName: "studio-1",
		PodCIDR:  "100.64.1.0/24",
		MeshIP:   "100.64.1.1",
		Peers:    []MeshPeerSpec{sampleMeshPeer().Spec},
	}.WithDefaults()

	cases := []struct {
		name  string
		value any
		fresh func() any
		ver   int32
	}{
		{"MeshEnrollRequest", req, func() any { return &MeshEnrollRequest{} }, req.SchemaVersion},
		{"MeshEnrollResponse", resp, func() any { return &MeshEnrollResponse{} }, resp.SchemaVersion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.ver != MeshEnrollSchemaVersion {
				t.Fatalf("payload not version-stamped: got %d, want %d", tc.ver, MeshEnrollSchemaVersion)
			}
			b, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := tc.fresh()
			if err := json.Unmarshal(b, got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			gotVal := reflect.ValueOf(got).Elem().Interface()
			if !reflect.DeepEqual(tc.value, gotVal) {
				t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", gotVal, tc.value)
			}
		})
	}
}

func TestMeshEnrollValidate(t *testing.T) {
	t.Parallel()

	t.Run("request", func(t *testing.T) {
		t.Parallel()
		if err := (MeshEnrollRequest{}).Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty request Validate() = %v, want ErrInvalid (unstamped)", err)
		}
		ok := MeshEnrollRequest{NodeName: "n", PublicKey: "k", Endpoint: "192.168.1.21:51820"}.WithDefaults()
		if err := ok.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("response", func(t *testing.T) {
		t.Parallel()
		if err := (MeshEnrollResponse{}).Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("empty response Validate() = %v, want ErrInvalid (unstamped)", err)
		}
		ok := MeshEnrollResponse{NodeName: "n", PodCIDR: "100.64.1.0/24"}.WithDefaults()
		if err := ok.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})
}

// TestNodePortUnchangedM3 is the M3.1 no-op confirmation: ServicePort.NodePort
// already exists (M1.2), so M3 NodePort work is darwin-net + k3sm only. This
// pins the field's presence + behavior so no one re-adds, renames, or renumbers
// it. The field is exercised end-to-end in TestServicePortValidate /
// TestJSONRoundTrip; here we assert its JSON name and round-trip explicitly.
func TestNodePortUnchangedM3(t *testing.T) {
	t.Parallel()
	sp := ServicePort{Name: "http", Port: 80, TargetPort: 8080, Protocol: ProtocolTCP, NodePort: 30080}
	b, err := json.Marshal(sp)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["nodePort"]; !ok {
		t.Fatalf("ServicePort lost its nodePort JSON field: %s", b)
	}
	var got ServicePort
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.NodePort != 30080 {
		t.Fatalf("NodePort round-trip = %d, want 30080", got.NodePort)
	}
}

// TestMeshPeerDefaultsStampSchemaOne pins that WithDefaults stamps schema
// version 1, with and without Endpoints set.
//
// A reader skips every peer whose stamp differs from the one it knows, so a
// bump for an additive field would make every new node invisible to every
// not-yet-upgraded node in a mixed-version cluster. Endpoints is additive and
// must ride the existing stamp; this test is the tripwire for a reflexive bump.
func TestMeshPeerDefaultsStampSchemaOne(t *testing.T) {
	t.Parallel()
	if MeshPeerSchemaVersion != 1 {
		t.Fatalf("MeshPeerSchemaVersion = %d, want 1 (an additive field never bumps it)", MeshPeerSchemaVersion)
	}
	cases := []struct {
		name string
		spec MeshPeerSpec
	}{
		{"without endpoints", MeshPeerSpec{NodeName: "n", PublicKey: "k", Endpoint: "192.168.1.21:51820", PodCIDR: "100.64.2.0/24", AllowedIPs: []string{"100.64.2.0/24"}}},
		{"with endpoints", MeshPeerSpec{NodeName: "n", PublicKey: "k", Endpoint: "192.168.1.21:51820", PodCIDR: "100.64.2.0/24", AllowedIPs: []string{"100.64.2.0/24"},
			Endpoints: []EndpointCandidate{{Address: "192.168.1.21:51820", Link: EndpointLinkUnderlay}, {Address: "169.254.0.17:51820", Link: EndpointLinkDirect}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := tc.spec.WithDefaults()
			if out.SchemaVersion != 1 {
				t.Fatalf("WithDefaults stamped %d, want 1", out.SchemaVersion)
			}
			if err := out.Validate(); err != nil {
				t.Fatalf("defaulted spec must validate: %v", err)
			}
		})
	}
}

// meshPeerGolden is the golden fixture of a MeshPeer carrying Endpoints.
const meshPeerGolden = "meshpeer_endpoints.golden.json"

// TestMeshPeerJSONGolden pins the serialized MeshPeer shape, Endpoints
// included, against testdata. The JSON names are the CRD's schema property
// names; a tag change is a break for every stored object. Regenerate
// deliberately with UPDATE_GOLDEN=1, never reflexively.
func TestMeshPeerJSONGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(sampleMeshPeer(), "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", meshPeerGolden)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("MeshPeer JSON differs from testdata/%s\n got = %s\nwant = %s", meshPeerGolden, got, want)
	}
}

// preM17MeshPeerSpec is MeshPeerSpec as it was before Endpoints existed: the
// shape every not-yet-upgraded reader in a mixed-version cluster decodes into.
type preM17MeshPeerSpec struct {
	SchemaVersion              int32    `json:"schemaVersion"`
	NodeName                   string   `json:"nodeName"`
	PublicKey                  string   `json:"publicKey"`
	Endpoint                   string   `json:"endpoint"`
	PodCIDR                    string   `json:"podCIDR"`
	AllowedIPs                 []string `json:"allowedIPs"`
	MeshIP                     string   `json:"meshIP,omitempty"`
	PersistentKeepaliveSeconds int32    `json:"persistentKeepaliveSeconds,omitempty"`
}

// TestMeshPeerEndpointsDecodeAndValidate pins the mixed-version fact behind
// the additive Endpoints field: a MeshPeer carrying endpoints decodes and
// validates on the current reader, a pre-Endpoints reader decodes the same
// bytes without error and loses nothing it knew about (stamp 1, Endpoint), and
// a candidate with a link value this reader does not know still DECODES (the
// reader ignores it) even though a writer may not emit it.
func TestMeshPeerEndpointsDecodeAndValidate(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("testdata", meshPeerGolden))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	t.Run("current reader", func(t *testing.T) {
		t.Parallel()
		var mp MeshPeer
		if err := json.Unmarshal(raw, &mp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(mp.Spec.Endpoints) != 2 {
			t.Fatalf("decoded %d endpoints, want 2", len(mp.Spec.Endpoints))
		}
		if err := mp.Spec.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("pre-endpoints reader", func(t *testing.T) {
		t.Parallel()
		var obj struct {
			Spec preM17MeshPeerSpec `json:"spec"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			t.Fatalf("an old reader fails to decode a MeshPeer with endpoints: %v", err)
		}
		if obj.Spec.SchemaVersion != 1 {
			t.Fatalf("old reader sees schemaVersion %d, want 1 (it skips any other stamp)", obj.Spec.SchemaVersion)
		}
		if obj.Spec.Endpoint != "192.168.1.20:51820" || obj.Spec.NodeName != "studio-1" || len(obj.Spec.AllowedIPs) != 1 {
			t.Fatalf("old reader lost a field it knows: %+v", obj.Spec)
		}
	})

	t.Run("reserved-half endpoint for an old reader", func(t *testing.T) {
		t.Parallel()
		// A node with no underlay writes its direct-link address into Endpoint;
		// an old reader's only syntax check is validateHostPort, which accepts it.
		if err := validateHostPort("169.254.0.9:51820"); err != nil {
			t.Fatalf("validateHostPort rejected a reserved-half endpoint: %v", err)
		}
	})

	t.Run("unknown link decodes", func(t *testing.T) {
		t.Parallel()
		b := []byte(`{"schemaVersion":1,"nodeName":"n","publicKey":"k","endpoint":"192.168.1.21:51820","podCIDR":"100.64.2.0/24","allowedIPs":["100.64.2.0/24"],"endpoints":[{"address":"192.168.1.21:51820","link":"future-medium"}]}`)
		var spec MeshPeerSpec
		if err := json.Unmarshal(b, &spec); err != nil {
			t.Fatalf("an unknown link value must decode: %v", err)
		}
		if spec.Endpoints[0].Link != "future-medium" {
			t.Fatalf("link = %q, want the unknown value preserved", spec.Endpoints[0].Link)
		}
		if err := spec.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Validate() = %v, want ErrInvalid (a writer never emits an unknown link)", err)
		}
	})
}

// TestMeshPeerValidateReservedHalves is the link-local table: a link-local IPv4
// address is admitted only inside 169.254.0.0/24 and 169.254.255.0/24, and in
// an Endpoints candidate only when its Link is direct; Endpoint itself may
// carry a reserved-half address (a node with no underlay); a zoned IPv6 literal
// passes the syntax check; an unknown or empty Link is refused on write.
func TestMeshPeerValidateReservedHalves(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		cand     *EndpointCandidate
		wantErr  bool
	}{
		// Endpoint (no candidates).
		{name: "endpoint underlay", endpoint: "192.168.1.20:51820"},
		{name: "endpoint low reserved half first", endpoint: "169.254.0.1:51820"},
		{name: "endpoint low reserved half last", endpoint: "169.254.0.248:51820"},
		{name: "endpoint high reserved half", endpoint: "169.254.255.248:51820"},
		{name: "endpoint self-assigned link-local", endpoint: "169.254.1.1:51820", wantErr: true},
		{name: "endpoint self-assigned link-local high", endpoint: "169.254.254.10:51820", wantErr: true},
		{name: "endpoint mapped self-assigned link-local", endpoint: "[::ffff:169.254.7.7]:51820", wantErr: true},
		{name: "endpoint zoned ipv6", endpoint: "[fe80::1%en2]:51820"},
		// Candidates.
		{name: "direct low half", cand: &EndpointCandidate{"169.254.0.9:51820", EndpointLinkDirect}},
		{name: "direct high half", cand: &EndpointCandidate{"169.254.255.1:51820", EndpointLinkDirect}},
		{name: "direct mapped low half", cand: &EndpointCandidate{"[::ffff:169.254.0.9]:51820", EndpointLinkDirect}},
		{name: "direct self-assigned", cand: &EndpointCandidate{"169.254.1.1:51820", EndpointLinkDirect}, wantErr: true},
		{name: "direct 169.254.128.x", cand: &EndpointCandidate{"169.254.128.1:51820", EndpointLinkDirect}, wantErr: true},
		{name: "underlay in reserved half", cand: &EndpointCandidate{"169.254.0.9:51820", EndpointLinkUnderlay}, wantErr: true},
		{name: "underlay self-assigned", cand: &EndpointCandidate{"169.254.1.1:51820", EndpointLinkUnderlay}, wantErr: true},
		{name: "underlay lan", cand: &EndpointCandidate{"192.168.1.20:51820", EndpointLinkUnderlay}},
		{name: "underlay dns", cand: &EndpointCandidate{"node-a.lan:51820", EndpointLinkUnderlay}},
		{name: "underlay zoned ipv6", cand: &EndpointCandidate{"[fe80::1%en2]:51820", EndpointLinkUnderlay}},
		{name: "direct zoned ipv6", cand: &EndpointCandidate{"[fe80::1%en2]:51820", EndpointLinkDirect}},
		{name: "unknown link", cand: &EndpointCandidate{"192.168.1.20:51820", "future-medium"}, wantErr: true},
		{name: "empty link", cand: &EndpointCandidate{"192.168.1.20:51820", ""}, wantErr: true},
		{name: "empty address", cand: &EndpointCandidate{"", EndpointLinkDirect}, wantErr: true},
		{name: "address not host:port", cand: &EndpointCandidate{"169.254.0.9", EndpointLinkDirect}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := sampleMeshPeer().Spec
			spec.Endpoints = nil
			if tc.endpoint != "" {
				spec.Endpoint = tc.endpoint
			}
			if tc.cand != nil {
				spec.Endpoints = []EndpointCandidate{*tc.cand}
			}
			err := spec.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Validate() = %v, want an ErrInvalid error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}
