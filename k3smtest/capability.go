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
	"os"
	"runtime"
	"strings"
)

// Capability names one discrete thing a test's environment either has or
// does not: real macOS hardware, a privileged process, live network egress,
// a code-signing/notarization identity, an Apple GPU, or a specific lab
// topology (a second Mac, a VZ-capable host, a live Postgres instance, a
// GitHub-hosted macOS runner, a host the test may safely reboot). The set
// below covers every distinct string k3sm/hack/acceptance/phases.json's
// "requires" arrays use across every milestone gate (m0 through the
// M10-lab tail) as of the apis M7.2-d2 carve — see doc.go.
type Capability string

const (
	// DevMac is a real macOS development machine, as opposed to a headless
	// or foreign-OS build box. Auto-PROBED (see probers below): true
	// whenever GOOS is darwin.
	DevMac Capability = "dev-mac"

	// Root is an effective-UID-0 process. Auto-PROBED via os.Geteuid.
	Root Capability = "root"

	// Network is live, unfirewalled network egress (pulling an image,
	// resolving a registry, dialing a real endpoint). DECLARED — see the
	// package-level note on probed vs. declared capabilities below.
	Network Capability = "network"

	// Signing is a code-signing / notarization identity (a Developer ID or
	// ad-hoc signing credential) usable by the process. DECLARED.
	Signing Capability = "signing"

	// AppleGPU is a Metal-capable Apple GPU available to the process — the
	// capability runtimed's M8.2-a4 gates its MLX path on. DECLARED.
	AppleGPU Capability = "apple-gpu"

	// TwoMacs is a two-machine lab topology, as used by the mesh and
	// multi-node acceptance labs. DECLARED.
	TwoMacs Capability = "two-macs"

	// VZ is a Virtualization.framework-capable host, as used by the `vm`
	// backend's lab. DECLARED.
	VZ Capability = "vz"

	// Postgres is a live, reachable PostgreSQL instance, as used by the kine
	// alternate-datastore lab. DECLARED.
	Postgres Capability = "postgres"

	// MacOSRunner is a GitHub-hosted macOS Actions runner identity, as used
	// by the reboot-survival lab tail. DECLARED.
	MacOSRunner Capability = "macos-runner"

	// Reboot is a host the test is authorized to reboot. DECLARED.
	Reboot Capability = "reboot"
)

// all is the taxonomy's canonical enumeration order. All returns a copy of
// this slice.
var all = []Capability{
	DevMac,
	Root,
	Network,
	Signing,
	AppleGPU,
	TwoMacs,
	VZ,
	Postgres,
	MacOSRunner,
	Reboot,
}

// All returns every Capability this package knows about, in a stable order.
// Callers must not mutate the returned slice.
func All() []Capability {
	out := make([]Capability, len(all))
	copy(out, all)
	return out
}

// probers holds, for every Capability in all, the function that decides
// whether it is present in THIS process's environment.
//
// Two probing strategies, deliberately kept apart:
//
//   - PROBED (DevMac, Root): a cheap, deterministic, portable stdlib check
//     that is either objectively true or objectively false for the running
//     process — there is nothing a caller could tell us that the process
//     doesn't already know for itself.
//   - DECLARED (everything else): presence that a test process cannot
//     safely or portably determine on its own — Apple-GPU/Metal hardware
//     presence, a live network policy, a signing identity's usability, a
//     specific multi-machine lab topology, or a CI runner's identity all
//     require either a privileged/hardware probe this package has no
//     business performing implicitly, or knowledge only the harness that
//     launched the test process actually has. Declaration is a
//     K3SM_CAP_<NAME> environment variable (dashes become underscores,
//     e.g. K3SM_CAP_APPLE_GPU), set by the lab/CI harness that verified the
//     capability is real; a test binary run bare has none of these set, so
//     it defaults to "absent" — the safe default for an unattended `go test
//     ./...` on a laptop.
var probers = map[Capability]func() bool{
	DevMac:      func() bool { return runtime.GOOS == "darwin" },
	Root:        func() bool { return os.Geteuid() == 0 },
	Network:     declared(Network),
	Signing:     declared(Signing),
	AppleGPU:    declared(AppleGPU),
	TwoMacs:     declared(TwoMacs),
	VZ:          declared(VZ),
	Postgres:    declared(Postgres),
	MacOSRunner: declared(MacOSRunner),
	Reboot:      declared(Reboot),
}

// isDeclared reports whether cap uses the DECLARED strategy (a
// K3SM_CAP_<NAME> environment variable) rather than PROBED (a direct
// runtime check). SkipUnless uses this to keep its skip message honest: a
// PROBED capability's env var, if it named one, would do nothing.
func isDeclared(cap Capability) bool {
	switch cap {
	case DevMac, Root:
		return false
	default:
		_, ok := probers[cap]
		return ok
	}
}

// declared builds a prober for a DECLARED capability: present iff its
// K3SM_CAP_<NAME> environment variable is set to a truthy value.
func declared(cap Capability) func() bool {
	name := envVarName(cap)
	return func() bool { return truthy(os.Getenv(name)) }
}

// envVarName returns the K3SM_CAP_<NAME> environment variable a DECLARED
// capability is read from, e.g. AppleGPU -> "K3SM_CAP_APPLE_GPU".
func envVarName(cap Capability) string {
	return "K3SM_CAP_" + strings.ToUpper(strings.ReplaceAll(string(cap), "-", "_"))
}

// truthy reports whether an environment variable's raw string value should
// be read as "set" — "1", "true", "yes" (case-insensitively); everything
// else, including the empty string, is not set.
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// Available reports whether cap is present in the current process's
// environment, using the PROBED/DECLARED strategy documented on probers. An
// unknown Capability (one outside All()) is always reported unavailable.
func Available(cap Capability) bool {
	p, ok := probers[cap]
	if !ok {
		return false
	}
	return p()
}

// required reports whether cap is named in the K3SM_CI_REQUIRE environment
// variable — a comma/whitespace-separated list of capability names a lab or
// CI run promises are present. SkipUnless treats a required-but-absent
// capability as fatal rather than skipped.
func required(cap Capability) bool {
	raw := os.Getenv("K3SM_CI_REQUIRE")
	if raw == "" {
		return false
	}
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		if Capability(strings.TrimSpace(f)) == cap {
			return true
		}
	}
	return false
}
