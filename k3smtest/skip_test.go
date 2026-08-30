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
	"os/exec"
	"strings"
	"testing"
)

// TestSkipUnlessHelperProcess is not a real test: it is the re-exec target
// TestSkipUnless_FatalNotSkip spawns as a separate `go test` process so it
// can observe SkipUnless's actual terminal behavior (skip vs. fatal), which
// cannot be observed in-process — t.Skip/t.Fatal both stop the calling
// goroutine, so nothing in the SAME test binary can inspect which one ran.
// This is the standard Go idiom for testing a function that calls
// os.Exit/t.Fatal/t.Skip (see os/exec's own TestHelperProcess).
//
// Guarded by K3SM_TEST_HELPER_PROCESS so a normal `go test ./...` run never
// executes it as a real test — it always skips immediately in that case.
func TestSkipUnlessHelperProcess(t *testing.T) {
	if os.Getenv("K3SM_TEST_HELPER_PROCESS") != "1" {
		t.Skip("not invoked as the SkipUnless helper process")
	}
	SkipUnless(t, Capability(os.Getenv("K3SM_TEST_HELPER_CAP")))
	// Reached only when SkipUnless decided the capability IS available.
	t.Log("k3smtest: helper process ran past SkipUnless")
}

// runHelper re-execs this test binary with only TestSkipUnlessHelperProcess
// selected, under env, and returns its combined output and exit code.
func runHelper(t *testing.T, env []string) (output string, exitCode int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.v", "-test.run=^TestSkipUnlessHelperProcess$")
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env, "K3SM_TEST_HELPER_PROCESS=1")
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("could not re-exec the test binary as a helper process: %v\noutput:\n%s", err, out)
		}
	}
	return string(out), code
}

// TestSkipUnless_FatalNotSkip is the B187 meta-test: it PROVES, by actually
// running SkipUnless to its terminal outcome in a child process, that
// K3SM_CI_REQUIRE flips an absent capability from SKIP to FATAL — the one
// behavior that keeps a lab run honest (see doc.go). A regression that made
// SkipUnless always skip (silently passing a run that promised the
// capability was there) would go GREEN on every other test in this package
// but RED here.
func TestSkipUnless_FatalNotSkip(t *testing.T) {
	// A capability that is real (so isKnown doesn't short-circuit the case
	// before K3SM_CI_REQUIRE is even consulted) but DECLARED, so it is
	// absent by construction unless the helper process's own env says
	// otherwise — Network is never true unless K3SM_CAP_NETWORK is set.
	const cap = Network

	t.Run("absent, not required -> SKIP, exit 0", func(t *testing.T) {
		out, code := runHelper(t, []string{
			"K3SM_TEST_HELPER_CAP=" + string(cap),
			"K3SM_CAP_NETWORK=",
			"K3SM_CI_REQUIRE=",
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (a skip must not fail the run)\noutput:\n%s", code, out)
		}
		if !strings.Contains(out, "--- SKIP") {
			t.Fatalf("expected a --- SKIP line, got:\n%s", out)
		}
		if strings.Contains(out, "--- FAIL") {
			t.Fatalf("did not expect a --- FAIL line, got:\n%s", out)
		}
	})

	t.Run("absent, required via K3SM_CI_REQUIRE -> FATAL, exit non-zero", func(t *testing.T) {
		out, code := runHelper(t, []string{
			"K3SM_TEST_HELPER_CAP=" + string(cap),
			"K3SM_CAP_NETWORK=",
			"K3SM_CI_REQUIRE=" + string(cap),
		})
		if code == 0 {
			t.Fatalf("exit code = 0, want non-zero (K3SM_CI_REQUIRE must make an absent capability fatal)\noutput:\n%s", out)
		}
		if !strings.Contains(out, "--- FAIL") {
			t.Fatalf("expected a --- FAIL line, got:\n%s", out)
		}
		if strings.Contains(out, "--- SKIP") {
			t.Fatalf("did not expect a --- SKIP line (this run promised the capability; it must not silently skip), got:\n%s", out)
		}
	})

	t.Run("present -> neither skip nor fatal, exit 0", func(t *testing.T) {
		out, code := runHelper(t, []string{
			"K3SM_TEST_HELPER_CAP=" + string(cap),
			"K3SM_CAP_NETWORK=1",
			"K3SM_CI_REQUIRE=" + string(cap),
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (the capability is present)\noutput:\n%s", code, out)
		}
		if !strings.Contains(out, "--- PASS") {
			t.Fatalf("expected a --- PASS line, got:\n%s", out)
		}
	})

	t.Run("unknown capability -> FATAL regardless of K3SM_CI_REQUIRE", func(t *testing.T) {
		out, code := runHelper(t, []string{
			"K3SM_TEST_HELPER_CAP=not-a-real-capability",
			"K3SM_CI_REQUIRE=",
		})
		if code == 0 {
			t.Fatalf("exit code = 0, want non-zero (an unrecognized capability is always a caller bug)\noutput:\n%s", out)
		}
		if !strings.Contains(out, "--- FAIL") {
			t.Fatalf("expected a --- FAIL line, got:\n%s", out)
		}
	})
}
