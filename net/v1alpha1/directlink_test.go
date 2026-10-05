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
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k3sm.io/apis/config/crd"
)

// fixedLinkTime is a deterministic, second-precision UTC instant so metav1.Time
// status fields round-trip losslessly through RFC3339 JSON.
var fixedLinkTime = metav1.NewTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))

// directLinkGolden is the golden fixture of a fully-populated DirectLink.
const directLinkGolden = "directlink.golden.json"

// sampleDirectLink is a fully-populated DirectLink for node index 1: one
// plugged port with RDMA and a configured address, and one unplugged port.
//
// It allocates its own status time so a test that mutates the returned object
// never reaches the shared fixedLinkTime.
func sampleDirectLink() *DirectLink {
	lastTransition := fixedLinkTime.DeepCopy()
	return &DirectLink{
		TypeMeta:   metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "DirectLink"},
		ObjectMeta: metav1.ObjectMeta{Name: "studio-1", ResourceVersion: "7"},
		Spec: DirectLinkSpec{
			SchemaVersion: DirectLinkSchemaVersion,
			NodeName:      "studio-1",
			Medium:        MediumThunderbolt,
			Ports: []DirectLinkPort{
				{
					Iface:          "en2",
					PortOrdinal:    0,
					DomainUUID:     "8b1c0e2a-0000-4000-8000-000000000001",
					PeerDomainUUID: "8b1c0e2a-0000-4000-8000-000000000002",
					SpeedGbps:      80,
					RDMADevice:     "rdma_en2",
					LinkIP:         "169.254.0.9",
					LinkUp:         true,
					RouteReady:     true,
				},
				{
					Iface:       "en3",
					PortOrdinal: 1,
					DomainUUID:  "8b1c0e2a-0000-4000-8000-000000000003",
				},
			},
		},
		Status: DirectLinkStatus{
			Ports: []DirectLinkPortStatus{
				{
					Iface:          "en2",
					PeerNodeName:   "studio-2",
					PeerIface:      "en5",
					PeerLinkIP:     "169.254.0.18",
					PeerRDMADevice: "rdma_en5",
					State:          DirectLinkStateUp,
					LastTransition: lastTransition,
				},
				{Iface: "en3", State: DirectLinkStateDown},
			},
			ObservedSchemaVersion: DirectLinkSchemaVersion,
		},
	}
}

// TestDirectLinkGVK asserts the DirectLink types register under
// net.k3sm.io/v1alpha1. A wrong group or version means an informer watches a
// resource path the server does not serve.
func TestDirectLinkGVK(t *testing.T) {
	t.Parallel()
	if GroupName != "net.k3sm.io" || SchemeGroupVersion.Version != "v1alpha1" {
		t.Fatalf("SchemeGroupVersion = %v, want net.k3sm.io/v1alpha1", SchemeGroupVersion)
	}
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, tc := range []struct {
		obj  runtime.Object
		kind string
	}{{&DirectLink{}, "DirectLink"}, {&DirectLinkList{}, "DirectLinkList"}} {
		gvks, _, err := s.ObjectKinds(tc.obj)
		if err != nil {
			t.Fatalf("ObjectKinds(%s): %v", tc.kind, err)
		}
		want := schema.GroupVersionKind{Group: "net.k3sm.io", Version: "v1alpha1", Kind: tc.kind}
		if len(gvks) != 1 || gvks[0] != want {
			t.Fatalf("%s registered as %v, want %v", tc.kind, gvks, want)
		}
	}
	if got := Resource("directlinks"); got.Group != "net.k3sm.io" || got.Resource != "directlinks" {
		t.Fatalf("Resource(directlinks) = %v", got)
	}
}

// TestDirectLinkJSONRoundTrip asserts a DirectLink survives marshal→unmarshal
// byte-stably, including its status time.
func TestDirectLinkJSONRoundTrip(t *testing.T) {
	t.Parallel()
	b1, err := json.Marshal(sampleDirectLink())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got DirectLink
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
	if lt := got.Status.Ports[0].LastTransition; lt == nil || !lt.Equal(&fixedLinkTime) {
		t.Fatalf("status lastTransition lost in round-trip: %v", lt)
	}
}

// TestDirectLinkJSONGolden pins the serialized shape against testdata. The JSON
// names are the CRD's schema property names. Regenerate deliberately with
// UPDATE_GOLDEN=1, never reflexively.
func TestDirectLinkJSONGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(sampleDirectLink(), "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", directLinkGolden)
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
		t.Errorf("DirectLink JSON differs from testdata/%s\n got = %s\nwant = %s", directLinkGolden, got, want)
	}
}

// TestDirectLinkGoldenDeclaredInCRD walks every key of the golden object's
// spec and status and asserts the embedded CRD schema declares it.
//
// The CRD and the Go type are two hand-maintained descriptions of one object;
// a JSON field the schema does not declare is pruned by the apiserver on write,
// silently. Walking the golden (which TestDirectLinkJSONGolden ties to the Go
// type) against the manifest closes that gap mechanically.
func TestDirectLinkGoldenDeclaredInCRD(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("testdata", directLinkGolden))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(crd.DirectLinkCRD(), &m); err != nil {
		t.Fatalf("CRD manifest is not valid YAML: %v", err)
	}
	versions, _ := m["spec"].(map[string]any)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("CRD has %d versions, want 1", len(versions))
	}
	root := versions[0].(map[string]any)["schema"].(map[string]any)["openAPIV3Schema"].(map[string]any)
	for _, top := range []string{"spec", "status"} {
		sch := root["properties"].(map[string]any)[top].(map[string]any)
		var missing []string
		walkDeclared(top, obj[top], sch, &missing)
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("CRD schema does not declare %s; the apiserver would prune them", strings.Join(missing, ", "))
		}
	}
}

// walkDeclared records in missing every path under v that sch does not declare.
func walkDeclared(path string, v any, sch map[string]any, missing *[]string) {
	switch val := v.(type) {
	case map[string]any:
		props, _ := sch["properties"].(map[string]any)
		for k, child := range val {
			cs, ok := props[k].(map[string]any)
			if !ok {
				*missing = append(*missing, path+"."+k)
				continue
			}
			walkDeclared(path+"."+k, child, cs, missing)
		}
	case []any:
		items, _ := sch["items"].(map[string]any)
		for _, child := range val {
			walkDeclared(path+"[]", child, items, missing)
		}
	}
}

// TestDirectLinkDeepCopy asserts DeepCopy is a real deep copy: mutating the
// original after copying must not move the copy (the informer-cache property).
func TestDirectLinkDeepCopy(t *testing.T) {
	t.Parallel()
	orig := sampleDirectLink()
	cp := orig.DeepCopy()
	orig.Spec.Ports[0].Iface = "en9"
	orig.Status.Ports[0].PeerNodeName = "mutated"
	orig.Status.Ports[0].LastTransition.Time = time.Unix(0, 0)
	if cp.Spec.Ports[0].Iface != "en2" {
		t.Fatal("DeepCopy shared spec.ports")
	}
	if cp.Status.Ports[0].PeerNodeName != "studio-2" {
		t.Fatal("DeepCopy shared status.ports")
	}
	if !cp.Status.Ports[0].LastTransition.Equal(&fixedLinkTime) {
		t.Fatal("DeepCopy shared the lastTransition pointer")
	}
	if _, ok := orig.DeepCopyObject().(*DirectLink); !ok {
		t.Fatal("DeepCopyObject did not return *DirectLink")
	}
	if (*DirectLink)(nil).DeepCopy() != nil {
		t.Fatal("(*DirectLink)(nil).DeepCopy() != nil")
	}

	list := &DirectLinkList{Items: []DirectLink{*sampleDirectLink()}}
	lcp := list.DeepCopy()
	list.Items[0].Spec.Ports[0].Iface = "en9"
	if lcp.Items[0].Spec.Ports[0].Iface != "en2" {
		t.Fatal("list DeepCopy shared item ports")
	}
	if _, ok := list.DeepCopyObject().(*DirectLinkList); !ok {
		t.Fatal("DeepCopyObject did not return *DirectLinkList")
	}
}

// TestDirectLinkSpecValidate is the Validate table: medium, schema stamp,
// node name, the iface shape, the port-ordinal range, the domain-UUID pair, the
// RDMA device shape, and the linkIP address class.
func TestDirectLinkSpecValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*DirectLinkSpec)
		wantErr bool
	}{
		{"ok", func(*DirectLinkSpec) {}, false},
		{"no ports", func(s *DirectLinkSpec) { s.Ports = nil }, false},
		{"unstamped", func(s *DirectLinkSpec) { s.SchemaVersion = 0 }, true},
		{"missing nodeName", func(s *DirectLinkSpec) { s.NodeName = "" }, true},
		{"empty medium", func(s *DirectLinkSpec) { s.Medium = "" }, true},
		{"unknown medium", func(s *DirectLinkSpec) { s.Medium = "usb4" }, true},
		{"medium wrong case", func(s *DirectLinkSpec) { s.Medium = "Thunderbolt" }, true},
		{"iface en0", func(s *DirectLinkSpec) { s.Ports[0].Iface = "en0"; s.Ports[0].RDMADevice = "rdma_en0" }, false},
		{"iface en12", func(s *DirectLinkSpec) { s.Ports[0].Iface = "en12"; s.Ports[0].RDMADevice = "rdma_en12" }, false},
		{"iface bridge0", func(s *DirectLinkSpec) { s.Ports[0].Iface = "bridge0" }, true},
		{"iface utun3", func(s *DirectLinkSpec) { s.Ports[0].Iface = "utun3" }, true},
		{"iface en", func(s *DirectLinkSpec) { s.Ports[0].Iface = "en" }, true},
		{"iface empty", func(s *DirectLinkSpec) { s.Ports[0].Iface = "" }, true},
		{"iface en2 trailing", func(s *DirectLinkSpec) { s.Ports[0].Iface = "en2x" }, true},
		{"ordinal 7", func(s *DirectLinkSpec) { s.Ports[0].PortOrdinal = 7; s.Ports[0].LinkIP = "" }, false},
		{"ordinal 8", func(s *DirectLinkSpec) { s.Ports[0].PortOrdinal = 8 }, true},
		{"ordinal -1", func(s *DirectLinkSpec) { s.Ports[0].PortOrdinal = -1 }, true},
		{"missing domainUUID", func(s *DirectLinkSpec) { s.Ports[1].DomainUUID = "" }, true},
		{"domainUUID equals peer", func(s *DirectLinkSpec) { s.Ports[0].PeerDomainUUID = s.Ports[0].DomainUUID }, true},
		{"unplugged peer empty", func(s *DirectLinkSpec) { s.Ports[0].PeerDomainUUID = "" }, false},
		{"negative speed", func(s *DirectLinkSpec) { s.Ports[0].SpeedGbps = -40 }, true},
		{"rdma other iface", func(s *DirectLinkSpec) { s.Ports[0].RDMADevice = "rdma_en3" }, true},
		{"rdma bare", func(s *DirectLinkSpec) { s.Ports[0].RDMADevice = "en2" }, true},
		{"rdma empty", func(s *DirectLinkSpec) { s.Ports[0].RDMADevice = "" }, false},
		{"linkIP empty", func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "" }, false},
		{"linkIP high half", func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "169.254.255.1" }, false},
		{"linkIP self-assigned", func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "169.254.1.1" }, true},
		{"linkIP lan", func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "192.168.1.20" }, true},
		{"linkIP ipv6", func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "fe80::1" }, true},
		{"linkIP garbage", func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "not-an-ip" }, true},
		{"linkIP with port", func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "169.254.0.9:51820" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := sampleDirectLink().Spec.DeepCopy()
			tc.mutate(spec)
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

// TestDirectLinkSpecValidateWithIndex pins the derivation check: a set linkIP
// must equal LinkIP(idx, portOrdinal) for the caller's node index, and a node
// index with no direct-link address cannot carry one.
func TestDirectLinkSpecValidateWithIndex(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		idx     int
		mutate  func(*DirectLinkSpec)
		wantErr error
	}{
		{name: "matches idx 1 port 0", idx: 1, mutate: func(*DirectLinkSpec) {}},
		{name: "wrong idx", idx: 2, mutate: func(*DirectLinkSpec) {}, wantErr: ErrInvalid},
		{name: "wrong port", idx: 1, mutate: func(s *DirectLinkSpec) { s.Ports[0].PortOrdinal = 1 }, wantErr: ErrInvalid},
		{name: "high half idx 31", idx: 31, mutate: func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "169.254.255.1" }},
		{name: "high half idx 61 port 7", idx: 61, mutate: func(s *DirectLinkSpec) { s.Ports[0].PortOrdinal = 7; s.Ports[0].LinkIP = "169.254.255.248" }},
		{name: "idx 62 with linkIP", idx: 62, mutate: func(*DirectLinkSpec) {}, wantErr: ErrNoLinkAddress},
		{name: "idx 62 without linkIP", idx: 62, mutate: func(s *DirectLinkSpec) { s.Ports[0].LinkIP = "" }},
		{name: "class check still runs", idx: 1, mutate: func(s *DirectLinkSpec) { s.Medium = "usb4" }, wantErr: ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			spec := sampleDirectLink().Spec.DeepCopy()
			tc.mutate(spec)
			err := spec.ValidateWithIndex(tc.idx)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateWithIndex(%d) = %v, want nil", tc.idx, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) || !errors.Is(err, ErrInvalid) {
				t.Fatalf("ValidateWithIndex(%d) = %v, want an error wrapping %v and ErrInvalid", tc.idx, err, tc.wantErr)
			}
		})
	}
}

// TestDirectLinkStateValues pins the state strings the resolver writes and the
// CRD enum admits.
func TestDirectLinkStateValues(t *testing.T) {
	t.Parallel()
	for got, want := range map[DirectLinkState]string{
		DirectLinkStateUp:          "up",
		DirectLinkStatePeerUnknown: "peer-unknown",
		DirectLinkStateDown:        "down",
	} {
		if string(got) != want {
			t.Errorf("state %q, want %q", got, want)
		}
	}
	if MediumThunderbolt != "thunderbolt" || DirectLinkSchemaVersion != 1 {
		t.Errorf("MediumThunderbolt = %q, DirectLinkSchemaVersion = %d", MediumThunderbolt, DirectLinkSchemaVersion)
	}
}
