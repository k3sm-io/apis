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
		{name: "path_wrong_version_falls_back_to_plain", onPath: other, plain: pin, wantPick: "plain"},
		{name: "no_candidate_is_red", wantRed: true, wantErr: []string{"not installed", "staticcheck@" + pin, "staticcheck-" + pin}},
		{name: "no_candidate_at_pin_is_red", versioned: other, onPath: other, wantRed: true, wantErr: []string{"no candidate matches", other, pin}},
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
				"HOME=" + t.TempDir(),
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
