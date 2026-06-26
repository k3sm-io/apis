package storagev1

import (
	"encoding/json"
	"errors"
	"reflect"
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
