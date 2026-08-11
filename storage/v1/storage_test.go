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

package storagev1

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestReclaimPolicyValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		p    ReclaimPolicy
		want bool
	}{
		{"retain", ReclaimRetain, true},
		{"delete", ReclaimDelete, true},
		{"empty", ReclaimPolicy(""), false},
		{"bogus", ReclaimPolicy("Recycle"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.Valid(); got != tc.want {
				t.Fatalf("ReclaimPolicy(%q).Valid() = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

func TestVolumeBindingModeValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		m    VolumeBindingMode
		want bool
	}{
		{"wffc", BindingWaitForFirstConsumer, true},
		{"immediate", BindingImmediate, true},
		{"empty", VolumeBindingMode(""), false},
		{"bogus", VolumeBindingMode("Lazy"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.m.Valid(); got != tc.want {
				t.Fatalf("VolumeBindingMode(%q).Valid() = %v, want %v", tc.m, got, tc.want)
			}
		})
	}
}

func TestLocalPathClassValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		class   LocalPathClass
		wantErr bool
	}{
		{"default ok", DefaultLocalPathClass(), false},
		{"missing name", LocalPathClass{Provisioner: ProvisionerName, BasePath: DefaultBasePath, ReclaimPolicy: ReclaimRetain, VolumeBindingMode: BindingWaitForFirstConsumer}, true},
		{"missing provisioner", LocalPathClass{Name: "local-path", BasePath: DefaultBasePath, ReclaimPolicy: ReclaimRetain, VolumeBindingMode: BindingWaitForFirstConsumer}, true},
		{"relative basePath", LocalPathClass{Name: "local-path", Provisioner: ProvisionerName, BasePath: "var/lib/k3sm/storage", ReclaimPolicy: ReclaimRetain, VolumeBindingMode: BindingWaitForFirstConsumer}, true},
		{"empty basePath", LocalPathClass{Name: "local-path", Provisioner: ProvisionerName, ReclaimPolicy: ReclaimRetain, VolumeBindingMode: BindingWaitForFirstConsumer}, true},
		{"delete reclaim rejected", LocalPathClass{Name: "local-path", Provisioner: ProvisionerName, BasePath: DefaultBasePath, ReclaimPolicy: ReclaimDelete, VolumeBindingMode: BindingWaitForFirstConsumer}, true},
		{"immediate binding rejected", LocalPathClass{Name: "local-path", Provisioner: ProvisionerName, BasePath: DefaultBasePath, ReclaimPolicy: ReclaimRetain, VolumeBindingMode: BindingImmediate}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.class.Validate()
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

func TestLocalPathClassWithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("fills every empty field", func(t *testing.T) {
		t.Parallel()
		out := LocalPathClass{}.WithDefaults()
		want := DefaultLocalPathClass()
		if !reflect.DeepEqual(out, want) {
			t.Fatalf("WithDefaults() = %#v, want %#v", out, want)
		}
		if err := out.Validate(); err != nil {
			t.Fatalf("defaulted class must validate: %v", err)
		}
	})

	t.Run("preserves explicit fields", func(t *testing.T) {
		t.Parallel()
		in := LocalPathClass{Name: "fast", Provisioner: "custom", BasePath: "/data", ReclaimPolicy: ReclaimRetain, VolumeBindingMode: BindingWaitForFirstConsumer}
		out := in.WithDefaults()
		if !reflect.DeepEqual(out, in) {
			t.Fatalf("WithDefaults() mutated explicit fields: %#v", out)
		}
	})

	t.Run("does not mutate receiver", func(t *testing.T) {
		t.Parallel()
		in := LocalPathClass{}
		_ = in.WithDefaults()
		if in.Name != "" || in.BasePath != "" {
			t.Fatalf("receiver mutated: %#v", in)
		}
	})
}

func TestLocalPathClassDataDir(t *testing.T) {
	t.Parallel()

	t.Run("stable derivation from namespace/claim", func(t *testing.T) {
		t.Parallel()
		c := DefaultLocalPathClass()
		got, err := c.DataDir("stockkitty", "postgres-data")
		if err != nil {
			t.Fatal(err)
		}
		want := "/var/lib/k3sm/storage/stockkitty/postgres-data"
		if got != want {
			t.Fatalf("DataDir = %q, want %q", got, want)
		}
		// Determinism: the same claim always maps to the same dir (the property
		// runtimed and the provisioner both rely on).
		again, _ := c.DataDir("stockkitty", "postgres-data")
		if again != got {
			t.Fatalf("DataDir not deterministic: %q != %q", again, got)
		}
	})

	t.Run("distinct namespaces do not collide", func(t *testing.T) {
		t.Parallel()
		c := DefaultLocalPathClass()
		a, _ := c.DataDir("ns-a", "data")
		b, _ := c.DataDir("ns-b", "data")
		if a == b {
			t.Fatalf("DataDir collided across namespaces: %q", a)
		}
	})

	t.Run("empty base falls back to default", func(t *testing.T) {
		t.Parallel()
		got, err := LocalPathClass{}.DataDir("ns", "claim")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/var/lib/k3sm/storage/ns/claim" {
			t.Fatalf("DataDir = %q", got)
		}
	})

	t.Run("missing component errors", func(t *testing.T) {
		t.Parallel()
		c := DefaultLocalPathClass()
		for _, tc := range []struct{ ns, claim string }{{"", "claim"}, {"ns", ""}, {"", ""}} {
			if _, err := c.DataDir(tc.ns, tc.claim); !errors.Is(err, ErrInvalid) {
				t.Fatalf("DataDir(%q,%q) error = %v, want ErrInvalid", tc.ns, tc.claim, err)
			}
		}
	})
}

// TestDataDirRejectsEscape pins the DataDir name grammar: namespace is an RFC
// 1123 DNS label, claimName an RFC 1123 DNS subdomain, both checked against the
// RAW argument. The rejection cases cover traversal ("..", "a/b", "/abs"), the
// case-insensitive-APFS aliasing case (an upper-case namespace and an
// upper-case claim resolve to the SAME on-disk dir as their lowercase twins on
// the default macOS volume), untrimmed whitespace, emptiness, and the length
// ceilings. The POSITIVE controls are load-bearing: without them the table
// cannot distinguish "the guard works" from "DataDir rejects everything".
func TestDataDirRejectsEscape(t *testing.T) {
	t.Parallel()

	const maxLabel = 63
	c := DefaultLocalPathClass()

	cases := []struct {
		name  string
		ns    string
		claim string
		want  string // "" means the call must be rejected with ErrInvalid
	}{
		// Positive controls — ordinary values still produce the expected path.
		{"ordinary", "stockkitty", "postgres-data", "/var/lib/k3sm/storage/stockkitty/postgres-data"},
		{"digits and hyphens", "ns-1", "claim-2", "/var/lib/k3sm/storage/ns-1/claim-2"},
		{"single char components", "a", "b", "/var/lib/k3sm/storage/a/b"},
		{"max-length label namespace", strings.Repeat("a", maxLabel), "data", "/var/lib/k3sm/storage/" + strings.Repeat("a", maxLabel) + "/data"},
		// A dotted name is a valid SUBDOMAIN, so it is legal for a claim...
		{"dotted claim is a valid subdomain", "prod", "my.claim", "/var/lib/k3sm/storage/prod/my.claim"},
		// ...but NOT for a namespace, which must be a bare label.
		{"dotted namespace is not a label", "my.ns", "data", ""},

		// Traversal.
		{"parent namespace", "..", "server", ""},
		{"parent claim", "prod", "..", ""},
		{"dot namespace", ".", "data", ""},
		{"dot claim", "prod", ".", ""},
		{"separator in namespace", "a/b", "data", ""},
		{"separator in claim", "prod", "a/b", ""},
		{"absolute namespace", "/abs", "data", ""},
		{"absolute claim", "prod", "/abs", ""},
		{"traversal suffix in claim", "prod", "data/../../server", ""},

		// Whitespace — the raw value is what would be joined.
		{"leading space namespace", " prod", "data", ""},
		{"trailing space namespace", "prod ", "data", ""},
		{"trailing space claim", "prod", "data ", ""},
		{"blank namespace", "   ", "data", ""},

		// APFS case-insensitivity: an upper-case name aliases its lowercase twin.
		{"upper-case namespace", "Default", "data", ""},
		{"upper-case claim", "default", "Data", ""},

		// Emptiness.
		{"empty namespace", "", "data", ""},
		{"empty claim", "prod", "", ""},
		{"both empty", "", "", ""},

		// Length ceilings.
		{"over-long label namespace", strings.Repeat("a", maxLabel+1), "data", ""},
		{"over-long subdomain claim", "prod", strings.Repeat("a", 254), ""},

		// Other out-of-class bytes.
		{"underscore claim", "prod", "my_claim", ""},
		{"leading hyphen namespace", "-prod", "data", ""},
		{"nul byte in claim", "prod", "data\x00", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := c.DataDir(tc.ns, tc.claim)
			if tc.want == "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("DataDir(%q,%q) error = %v, want ErrInvalid", tc.ns, tc.claim, err)
				}
				if got != "" {
					t.Fatalf("DataDir(%q,%q) returned path %q on rejection, want no path", tc.ns, tc.claim, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DataDir(%q,%q) = %v, want %q", tc.ns, tc.claim, err, tc.want)
			}
			if got != tc.want {
				t.Fatalf("DataDir(%q,%q) = %q, want %q", tc.ns, tc.claim, got, tc.want)
			}
		})
	}

	// The escape the grammar exists to stop, stated as the property rather than
	// the error: no accepted (namespace, claim) pair may resolve outside the
	// storage root, and no two distinct pairs may resolve to one dir on a
	// case-insensitive volume.
	t.Run("no accepted pair escapes the base path", func(t *testing.T) {
		t.Parallel()
		base := DefaultBasePath + "/"
		for _, tc := range cases {
			got, err := c.DataDir(tc.ns, tc.claim)
			if err != nil {
				continue
			}
			if !strings.HasPrefix(got, base) || strings.Contains(got, "/../") {
				t.Fatalf("DataDir(%q,%q) = %q escapes %q", tc.ns, tc.claim, got, base)
			}
		}
	})
}

func TestPVName(t *testing.T) {
	t.Parallel()

	t.Run("derives from uid", func(t *testing.T) {
		t.Parallel()
		got, err := PVName("11111111-2222-3333-4444-555555555555")
		if err != nil {
			t.Fatal(err)
		}
		if got != "pvc-11111111-2222-3333-4444-555555555555" {
			t.Fatalf("PVName = %q", got)
		}
	})

	t.Run("empty uid errors", func(t *testing.T) {
		t.Parallel()
		if _, err := PVName("  "); !errors.Is(err, ErrInvalid) {
			t.Fatalf("PVName(blank) error = %v, want ErrInvalid", err)
		}
	})
}

func TestNodeTopology(t *testing.T) {
	t.Parallel()

	t.Run("defaults key to hostname label", func(t *testing.T) {
		t.Parallel()
		out := NodeTopology{NodeName: "studio-1"}.WithDefaults()
		if out.Key != TopologyKeyHostname {
			t.Fatalf("Key = %q, want %q", out.Key, TopologyKeyHostname)
		}
		if err := out.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("validate requires nodeName", func(t *testing.T) {
		t.Parallel()
		if err := (NodeTopology{Key: TopologyKeyHostname}).Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Validate() error = %v, want ErrInvalid", err)
		}
	})

	t.Run("does not mutate receiver", func(t *testing.T) {
		t.Parallel()
		in := NodeTopology{NodeName: "studio-1"}
		_ = in.WithDefaults()
		if in.Key != "" {
			t.Fatalf("receiver mutated: Key = %q", in.Key)
		}
	})
}

// TestJSONRoundTrip asserts the storage contract types survive a JSON
// marshal→unmarshal cycle unchanged — the M3.1 acceptance evidence for the
// StorageClass/provisioner contract + the PV node-affinity/topology fields.
func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value any
		fresh func() any
	}{
		{
			"LocalPathClass",
			LocalPathClass{
				Name:              "local-path",
				Provisioner:       ProvisionerName,
				BasePath:          DefaultBasePath,
				ReclaimPolicy:     ReclaimRetain,
				VolumeBindingMode: BindingWaitForFirstConsumer,
				Parameters:        map[string]string{"fsType": "apfs"},
			},
			func() any { return &LocalPathClass{} },
		},
		{
			"NodeTopology",
			NodeTopology{Key: TopologyKeyHostname, NodeName: "studio-1"},
			func() any { return &NodeTopology{} },
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
			gotVal := reflect.ValueOf(got).Elem().Interface()
			if !reflect.DeepEqual(tc.value, gotVal) {
				t.Fatalf("round-trip mismatch:\n got: %#v\nwant: %#v", gotVal, tc.value)
			}
		})
	}
}

// TestJSONFieldNames pins the wire field names (camelCase) so a rename — which
// would silently break a k3sm provisioner or runtimed binder decoding the
// contract — fails the build instead.
func TestJSONFieldNames(t *testing.T) {
	t.Parallel()

	t.Run("LocalPathClass", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(DefaultLocalPathClass())
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"name", "provisioner", "basePath", "reclaimPolicy", "volumeBindingMode"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("missing JSON key %q in %s", k, b)
			}
		}
	})

	t.Run("NodeTopology", func(t *testing.T) {
		t.Parallel()
		b, err := json.Marshal(NodeTopology{Key: TopologyKeyHostname, NodeName: "studio-1"})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"key", "nodeName"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("missing JSON key %q in %s", k, b)
			}
		}
	})
}
