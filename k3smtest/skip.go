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

import "testing"

// SkipUnless gates t on cap being available in the current process
// (Available(cap)):
//
//   - cap available: SkipUnless returns immediately; the test runs normally.
//   - cap absent, and NOT named in K3SM_CI_REQUIRE: t.Skip — the ordinary
//     case for a laptop `go test ./...` run that legitimately lacks e.g. a
//     second Mac or an Apple GPU.
//   - cap absent, and named in K3SM_CI_REQUIRE: t.Fatal, NOT t.Skip. A lab
//     or CI run that was launched specifically to prove a capability-gated
//     path executes must not silently report green by skipping the very
//     thing it exists to test; naming the capability in K3SM_CI_REQUIRE is
//     the harness's promise that it is present, and an absent-but-promised
//     capability is the harness's own bug, not a reason to skip.
//   - cap is not a Capability this package knows about: t.Fatal
//     unconditionally — an unrecognized name is always a caller bug, never
//     a legitimate skip.
//
// SkipUnless calls t.Helper(); like t.Skip and t.Fatal, it does not return
// when it decides to skip or fail.
func SkipUnless(t *testing.T, cap Capability) {
	t.Helper()
	if _, ok := probers[cap]; !ok {
		t.Fatalf("k3smtest: %q is not a recognized capability", cap)
	}
	if Available(cap) {
		return
	}
	if required(cap) {
		t.Fatalf("k3smtest: capability %q is absent but named in K3SM_CI_REQUIRE — this run promised it and must not silently skip", cap)
	}
	if isDeclared(cap) {
		t.Skipf("k3smtest: capability %q not available (set %s=1 to declare it present, or list it in K3SM_CI_REQUIRE to make its absence fatal instead of a skip)", cap, envVarName(cap))
	}
	t.Skipf("k3smtest: capability %q not available in this environment (auto-detected; list it in K3SM_CI_REQUIRE to make its absence fatal instead of a skip)", cap)
}
