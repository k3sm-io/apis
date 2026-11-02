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

package testresults_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestStaticcheckCandidateOrder pins the staticcheck lookup that hack/ci.sh runs
// through pick_staticcheck (hack/lib/staticcheck-pick.sh): the versioned
// $GOPATH/bin/staticcheck-<pin> wins over a PATH binary, a wrong-version
// versioned binary falls back to the next candidate at the pin, and no candidate
// at the pin is a named red. Fake staticcheck stubs sit on a temp GOPATH and
// PATH, so no real tool and no network are involved.
func TestStaticcheckCandidateOrder(t *testing.T) {
	t.Run("ci_sh_wires_picker", checkStaticcheckCIWiring)
	const pin = "2026.2.2"
	const other = "2025.1"

	cases := []struct {
		name      string
		versioned string // version the GOPATH/bin/staticcheck-<pin> stub reports; "" = absent
		onPath    string // version the PATH staticcheck stub reports; "" = absent
		plain     string // version the GOPATH/bin/staticcheck stub reports; "" = absent
		wantRed   bool
		wantPick  string // "versioned", "path" or "plain"
		wantErr   []string
	}{
		{name: "versioned_binary_wins", versioned: pin, onPath: pin, wantPick: "versioned"},
		{name: "versioned_wrong_version_falls_back", versioned: other, onPath: pin, wantPick: "path"},
		{name: "path_before_plain_gopath", onPath: pin, plain: pin, wantPick: "path"},
		{name: "plain_gopath_last_resort", plain: pin, wantPick: "plain"},
		{name: "path_wrong_version_falls_back_to_plain", onPath: other, plain: pin, wantPick: "plain"},
		{name: "no_candidate_is_red", wantRed: true, wantErr: []string{"not installed", "staticcheck@" + pin, "staticcheck-" + pin}},
		{name: "no_candidate_at_pin_is_red", versioned: other, onPath: other, wantRed: true, wantErr: []string{other, pin}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gopath := t.TempDir()
			pathDir := t.TempDir()
			bin := filepath.Join(gopath, "bin")
			if err := os.MkdirAll(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			fake := func(path, who, ver string) {
				if ver == "" {
					return
				}
				body := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"-version\" ]; then echo 'staticcheck %s (0.0.0)'; exit 0; fi\necho %s\n", ver, who)
				if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			fake(filepath.Join(bin, "staticcheck-"+pin), "versioned", tc.versioned)
			fake(filepath.Join(pathDir, "staticcheck"), "path", tc.onPath)
			fake(filepath.Join(bin, "staticcheck"), "plain", tc.plain)

			// The stubs are named for which candidate they are, so the chosen
			// binary is identified by running it: it echoes its own name.
			script := `set -eu
source "$PICK"
STATICCHECK_VERSION=` + pin + `
pick_staticcheck || exit 1
"$sc" run
`
			cmd := exec.Command("bash", "-c", script)
			cmd.Env = []string{
				"PATH=" + pathDir + ":" + goBinDir(t),
				"HOME=" + telemetryOffHome(t),
				"GOPATH=" + gopath,
				"GOFLAGS=",
				"GOWORK=off",
				"GOTOOLCHAIN=local",
				"GOPROXY=off",
				"PICK=" + pickScript(t),
			}
			var stdout, stderr strings.Builder
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()

			if tc.wantRed {
				if err == nil {
					t.Fatalf("pick_staticcheck succeeded, want a named red; stdout %q", stdout.String())
				}
				for _, w := range tc.wantErr {
					if !strings.Contains(stderr.String(), w) {
						t.Errorf("stderr lacks %q:\n%s", w, stderr.String())
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("pick_staticcheck failed: %v\n%s", err, stderr.String())
			}
			if got := strings.TrimSpace(stdout.String()); got != tc.wantPick {
				t.Errorf("picked the %q stub, want %q", got, tc.wantPick)
			}
		})
	}
}

// checkStaticcheckCIWiring pins the call site, which the order test never sees
// because it sources the picker directly: hack/ci.sh must source the picker and
// call it followed by `|| exit 1`, so a red pick stops the gate.
func checkStaticcheckCIWiring(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "ci.sh"))
	if err != nil {
		t.Fatalf("read ci.sh: %v", err)
	}
	ci := string(raw)
	if !regexp.MustCompile(`(?m)^\s*(source|\.)\s+hack/lib/staticcheck-pick\.sh\s*$`).MatchString(ci) {
		t.Errorf("ci.sh does not source hack/lib/staticcheck-pick.sh")
	}
	if !regexp.MustCompile(`(?m)^\s*pick_staticcheck\s*\|\|\s*exit 1\s*$`).MatchString(ci) {
		t.Errorf("ci.sh does not call pick_staticcheck followed by || exit 1")
	}
}

// goBinDir returns a directory holding only links to the tools
// pick_staticcheck needs (go and awk), so no host staticcheck beside
// the go binary can leak into a case.
func goBinDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range []string{"go", "awk"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("%s not on PATH: %v", tool, err)
		}
		if p, err = filepath.Abs(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(p, filepath.Join(dir, tool)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// pickScript returns the absolute path of hack/lib/staticcheck-pick.sh.
func pickScript(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("staticcheck-pick.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("staticcheck-pick.sh: %v", err)
	}
	return p
}

// telemetryOffHome returns a temp HOME whose Go telemetry mode is off, for a
// child that runs go. In the default local mode every go invocation under a
// fresh HOME starts a telemetry sidecar in its own session (setsid, so the test
// cannot wait for it) that keeps writing under HOME/Library/Application
// Support/go after go exits, and t.TempDir cleanup then races it ("directory
// not empty"). GOTELEMETRY is read-only in the environment; the mode file is the
// switch, and with it off go opens no counter file and starts no sidecar.
func telemetryOffHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "Library", "Application Support", "go", "telemetry")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mode"), []byte("off\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Registered after t.TempDir, so it runs before the removal: anything
		// beside the mode file means go ignored it and a late writer may exist.
		ents, err := os.ReadDir(dir)
		if err != nil {
			t.Errorf("read telemetry dir: %v", err)
			return
		}
		for _, e := range ents {
			if e.Name() != "mode" {
				t.Errorf("go wrote telemetry %s despite mode off", e.Name())
			}
		}
	})
	return home
}
