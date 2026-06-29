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
			SchemaVersion:              MeshPeerSchemaVersion,
			NodeName:                   "studio-1",
			PublicKey:                  "fakeBase64PublicKey0000000000000000000000000=",
			Endpoint:                   "192.168.1.20:51820",
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
	for _, k := range []string{"schemaVersion", "nodeName", "publicKey", "endpoint", "podCIDR", "allowedIPs", "meshIP"} {
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
	cp.Spec.NodeName = "mutated"
	cp.Status.Reachable = false
	if orig.Spec.AllowedIPs[0] != "100.64.1.0/24" {
		t.Fatalf("DeepCopy shared AllowedIPs backing array: %v", orig.Spec.AllowedIPs)
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

func TestMeshPeerSpecWithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("stamps version + keepalive", func(t *testing.T) {
		t.Parallel()
		out := MeshPeerSpec{NodeName: "n", PublicKey: "k", Endpoint: "e", PodCIDR: "100.64.2.0/24", AllowedIPs: []string{"100.64.2.0/24"}}.WithDefaults()
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
		ok := MeshEnrollRequest{NodeName: "n", PublicKey: "k", Endpoint: "e"}.WithDefaults()
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
