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

package k3smtest

import (
	"runtime"
	"testing"
)

// TestCapabilityTaxonomy pins the taxonomy against the phases.json
// "requires" vocabulary this package was carved to cover (see doc.go): every
// constant is present in All(), has a registered prober, round-trips through
// envVarName for the DECLARED ones, and the set has no duplicate or unknown
// entry. A renamed or dropped constant, or an entry missing its prober, goes
// red here rather than surfacing as a silent "always unavailable" at a
// caller.
func TestCapabilityTaxonomy(t *testing.T) {
	cases := []struct {
		cap      Capability
		declared bool
		envVar   string // only checked when declared
	}{
		{DevMac, false, ""},
		{Root, false, ""},
		{Network, true, "K3SM_CAP_NETWORK"},
		{Signing, true, "K3SM_CAP_SIGNING"},
		{AppleGPU, true, "K3SM_CAP_APPLE_GPU"},
		{TwoMacs, true, "K3SM_CAP_TWO_MACS"},
		{VZ, true, "K3SM_CAP_VZ"},
		{Postgres, true, "K3SM_CAP_POSTGRES"},
		{MacOSRunner, true, "K3SM_CAP_MACOS_RUNNER"},
		{Reboot, true, "K3SM_CAP_REBOOT"},
	}

	if got, want := len(All()), len(cases); got != want {
		t.Fatalf("All() has %d capabilities, want %d (taxonomy table out of sync)", got, want)
	}

	seen := make(map[Capability]bool, len(cases))
	for _, tc := range cases {
		t.Run(string(tc.cap), func(t *testing.T) {
			if seen[tc.cap] {
				t.Fatalf("capability %q listed twice in the test table", tc.cap)
			}
			seen[tc.cap] = true

			found := false
			for _, c := range All() {
				if c == tc.cap {
					found = true
				}
			}
			if !found {
				t.Fatalf("capability %q missing from All()", tc.cap)
			}
			if _, ok := probers[tc.cap]; !ok {
				t.Fatalf("capability %q has no registered prober", tc.cap)
			}
			if got := isDeclared(tc.cap); got != tc.declared {
				t.Fatalf("isDeclared(%q) = %v, want %v", tc.cap, got, tc.declared)
			}
			if tc.declared {
				if got := envVarName(tc.cap); got != tc.envVar {
					t.Fatalf("envVarName(%q) = %q, want %q", tc.cap, got, tc.envVar)
				}
			}
		})
	}

	// Every constant this package exports must appear in the table above —
	// catches a new const added to capability.go without a matching test
	// case (the inverse of the "missing from All()" check).
	for _, cap := range All() {
		if !seen[cap] {
			t.Fatalf("capability %q is in All() but missing from the test table", cap)
		}
	}
}

// TestAvailable_UnknownCapability pins the documented behavior: a
// Capability outside the taxonomy is always unavailable, never a panic or a
// false positive.
func TestAvailable_UnknownCapability(t *testing.T) {
	if Available(Capability("not-a-real-capability")) {
		t.Fatal("Available reported an unknown capability as present")
	}
}

// TestAvailable_DevMac pins the one deterministic, environment-independent
// assertion this suite can make about the PROBED capabilities: DevMac's
// verdict is exactly runtime.GOOS == "darwin", on every platform this
// package builds for (it is pure Go / CGO_ENABLED=0, so it also builds and
// runs on a non-darwin CI worker).
func TestAvailable_DevMac(t *testing.T) {
	want := runtime.GOOS == "darwin"
	if got := Available(DevMac); got != want {
		t.Fatalf("Available(DevMac) = %v, want %v (GOOS=%s)", got, want, runtime.GOOS)
	}
}

// TestAvailable_DeclaredRoundTrips pins the DECLARED strategy end to end for
// every non-probed capability: absent by default, present once its
// K3SM_CAP_<NAME> variable is set to a truthy value, and absent again once
// cleared — using t.Setenv so state never leaks to another test.
func TestAvailable_DeclaredRoundTrips(t *testing.T) {
	for _, cap := range All() {
		cap := cap
		if !isDeclared(cap) {
			continue
		}
		t.Run(string(cap), func(t *testing.T) {
			name := envVarName(cap)
			t.Setenv(name, "")
			if Available(cap) {
				t.Fatalf("Available(%q) = true with %s unset", cap, name)
			}
			for _, v := range []string{"1", "true", "TRUE", "yes", "Yes"} {
				t.Setenv(name, v)
				if !Available(cap) {
					t.Fatalf("Available(%q) = false with %s=%q", cap, name, v)
				}
			}
			for _, v := range []string{"0", "false", "no", "nope"} {
				t.Setenv(name, v)
				if Available(cap) {
					t.Fatalf("Available(%q) = true with %s=%q", cap, name, v)
				}
			}
		})
	}
}

// TestRequired pins the K3SM_CI_REQUIRE list-membership parsing: comma and
// whitespace separators, surrounding whitespace on an entry, and a name that
// is a real capability but not the one being asked about.
func TestRequired(t *testing.T) {
	cases := []struct {
		name string
		env  string
		cap  Capability
		want bool
	}{
		{"unset", "", Network, false},
		{"exact match", "network", Network, true},
		{"comma list", "root,network,signing", Network, true},
		{"whitespace list", "root network signing", Network, true},
		{"mixed separators + padding", " root , network\tsigning ", Network, true},
		{"present but not this one", "root,signing", Network, false},
		{"substring is not a match", "networking", Network, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("K3SM_CI_REQUIRE", tc.env)
			if got := required(tc.cap); got != tc.want {
				t.Fatalf("required(%q) with K3SM_CI_REQUIRE=%q = %v, want %v", tc.cap, tc.env, got, tc.want)
			}
		})
	}
}
