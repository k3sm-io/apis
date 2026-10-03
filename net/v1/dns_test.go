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
	"strings"
	"testing"
)

// clusterFirstGoldenConfigs is the fixture behind
// testdata/dnsconfig_clusterfirst.golden.json: a fully populated ClusterFirst
// config and the zero value. The golden was generated before DNSConfig gained
// Policy, Nameservers and Options, so byte equality proves those fields add
// nothing to a ClusterFirst config's wire shape.
func clusterFirstGoldenConfigs() []DNSConfig {
	return []DNSConfig{
		{
			ClusterDNSIP:  "10.43.0.10",
			ClusterDomain: "cluster.local",
			SearchDomains: []string{"default.svc.cluster.local", "svc.cluster.local", "cluster.local"},
			NDots:         5,
		},
		{},
	}
}

// noneGoldenConfig is the fixture behind testdata/dnsconfig_none.golden.json.
func noneGoldenConfig() DNSConfig {
	return DNSConfig{
		Policy:        DNSPolicyNone,
		Nameservers:   []string{"1.1.1.1", "9.9.9.9"},
		SearchDomains: []string{"a.example"},
		NDots:         2,
		Options:       []DNSOption{{Name: "edns0"}, {Name: "timeout", Value: "2"}},
	}
}

// marshalGolden is the one encoding both goldens use.
func marshalGolden(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return append(b, '\n')
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	return b
}

func TestDNSConfigJSONZeroValueUnchanged(t *testing.T) {
	t.Parallel()

	t.Run("ClusterFirst bytes match the pre-policy golden", func(t *testing.T) {
		t.Parallel()
		got := marshalGolden(t, clusterFirstGoldenConfigs())
		want := readGolden(t, "dnsconfig_clusterfirst.golden.json")
		if !bytes.Equal(got, want) {
			t.Fatalf("ClusterFirst JSON changed:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("None bytes match its golden", func(t *testing.T) {
		t.Parallel()
		got := marshalGolden(t, noneGoldenConfig())
		want := readGolden(t, "dnsconfig_none.golden.json")
		if !bytes.Equal(got, want) {
			t.Fatalf("None JSON changed:\ngot:\n%s\nwant:\n%s", got, want)
		}
	})

	t.Run("None round-trips", func(t *testing.T) {
		t.Parallel()
		in := noneGoldenConfig()
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out DNSConfig
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !reflect.DeepEqual(in, out) {
			t.Fatalf("round trip = %+v, want %+v", out, in)
		}
	})
}

func TestDNSConfigPolicyValidate(t *testing.T) {
	t.Parallel()

	cf := func(mut func(*DNSConfig)) DNSConfig {
		c := DNSConfig{ClusterDNSIP: "10.43.0.10", ClusterDomain: "cluster.local"}
		if mut != nil {
			mut(&c)
		}
		return c
	}
	none := func(ns ...string) DNSConfig {
		return DNSConfig{Policy: DNSPolicyNone, Nameservers: ns}
	}
	withOpts := func(c DNSConfig, opts ...DNSOption) DNSConfig {
		c.Options = opts
		return c
	}

	cases := []struct {
		name    string
		cfg     DNSConfig
		wantErr bool
		errHas  []string
	}{
		// Non-regression: every zero-policy config valid before stays valid.
		{"ClusterFirst ok", cf(nil), false, nil},
		{"ClusterFirst ok with search + ndots", cf(func(c *DNSConfig) {
			c.SearchDomains = []string{"svc.cluster.local"}
			c.NDots = 2
		}), false, nil},
		{"ClusterFirst zero ndots ok", cf(func(c *DNSConfig) { c.NDots = 0 }), false, nil},
		{"ClusterFirst missing ip", cf(func(c *DNSConfig) { c.ClusterDNSIP = "" }), true, nil},
		{"ClusterFirst missing domain", cf(func(c *DNSConfig) { c.ClusterDomain = "" }), true, nil},
		{"ClusterFirst negative ndots", cf(func(c *DNSConfig) { c.NDots = -1 }), true, nil},
		{"ClusterFirst with nameservers", cf(func(c *DNSConfig) { c.Nameservers = []string{"1.1.1.1"} }), true, nil},
		{"ClusterFirst with options", withOpts(cf(nil), DNSOption{Name: "edns0"}), true, nil},

		// None.
		{"None one nameserver", none("1.1.1.1"), false, nil},
		{"None three nameservers", none("1.1.1.1", "9.9.9.9", "2606:4700:4700::1111"), false, nil},
		{"None zero nameservers", none(), true, nil},
		{"None four nameservers", none("1.1.1.1", "1.0.0.1", "9.9.9.9", "8.8.8.8"), true, nil},
		{"None with clusterDNSIP", func() DNSConfig { c := none("1.1.1.1"); c.ClusterDNSIP = "10.43.0.10"; return c }(), true, nil},
		{"None empty domain", none("1.1.1.1"), false, nil},
		{"None with domain", func() DNSConfig { c := none("1.1.1.1"); c.ClusterDomain = "cluster.local"; return c }(), false, nil},
		{"None zoned address", none("fe80::1%en0"), true, nil},
		{"None IPv4-mapped address", none("::ffff:1.1.1.1"), false, nil},
		{"None not an address", none("dns.example"), true, nil},
		{"None duplicate nameservers", none("1.1.1.1", "1.1.1.1"), true, nil},
		{"None duplicate after parsing", none("2001:db8::1", "2001:0db8:0:0:0:0:0:1"), true, nil},
		{"None negative ndots", func() DNSConfig { c := none("1.1.1.1"); c.NDots = -1; return c }(), true, nil},
		{"None with options", withOpts(none("1.1.1.1"), DNSOption{Name: "edns0"}, DNSOption{Name: "timeout", Value: "2"}), false, nil},

		// Unknown policies.
		{"policy spelled ClusterFirst", func() DNSConfig { c := cf(nil); c.Policy = "ClusterFirst"; return c }(), true, []string{`""`, `"None"`}},
		{"policy bogus", func() DNSConfig { c := none("1.1.1.1"); c.Policy = "bogus"; return c }(), true, []string{`""`, `"None"`}},

		// Options, checked for either policy.
		{"option ndots", withOpts(none("1.1.1.1"), DNSOption{Name: "ndots", Value: "2"}), true, nil},
		{"option ndots uppercase", withOpts(none("1.1.1.1"), DNSOption{Name: "NDOTS", Value: "2"}), true, nil},
		{"option name smuggles ndots via colon", withOpts(none("1.1.1.1"), DNSOption{Name: "ndots:1"}), true, nil},
		{"option name with colon", withOpts(none("1.1.1.1"), DNSOption{Name: "timeout:2"}), true, nil},
		{"option name with equals", withOpts(none("1.1.1.1"), DNSOption{Name: "a=b"}), true, nil},
		{"option empty name", withOpts(none("1.1.1.1"), DNSOption{Value: "2"}), true, nil},
		{"option newline in name", withOpts(none("1.1.1.1"), DNSOption{Name: "edns0\nnameserver"}), true, nil},
		{"option newline in value", withOpts(none("1.1.1.1"), DNSOption{Name: "timeout", Value: "2\nnameserver 6.6.6.6"}), true, nil},
		{"option space in value", withOpts(none("1.1.1.1"), DNSOption{Name: "timeout", Value: "2 3"}), true, nil},
		{"option control char in name", withOpts(none("1.1.1.1"), DNSOption{Name: "edns0\x00"}), true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error %v does not wrap ErrInvalid", err)
			}
			for _, s := range tc.errHas {
				if !strings.Contains(err.Error(), s) {
					t.Fatalf("Validate() error %q does not mention %s", err, s)
				}
			}
		})
	}

	t.Run("Servers", func(t *testing.T) {
		t.Parallel()
		servers := []struct {
			name string
			cfg  DNSConfig
			want []string
		}{
			{"ClusterFirst", cf(nil), []string{"10.43.0.10"}},
			{"ClusterFirst without ip", DNSConfig{ClusterDomain: "cluster.local"}, nil},
			{"None", none("1.1.1.1", "9.9.9.9"), []string{"1.1.1.1", "9.9.9.9"}},
			{"None without nameservers", none(), nil},
			{"unknown policy", DNSConfig{Policy: "bogus", ClusterDNSIP: "10.43.0.10", Nameservers: []string{"1.1.1.1"}}, nil},
		}
		for _, tc := range servers {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got := tc.cfg.Servers()
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("Servers() = %#v, want %#v", got, tc.want)
				}
			})
		}
	})

	t.Run("Servers returns a copy for None", func(t *testing.T) {
		t.Parallel()
		cfg := none("1.1.1.1", "9.9.9.9")
		got := cfg.Servers()
		got[0] = "6.6.6.6"
		if cfg.Nameservers[0] != "1.1.1.1" {
			t.Fatalf("mutating Servers() result changed Nameservers to %v", cfg.Nameservers)
		}
	})
}
