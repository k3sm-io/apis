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

// Package k3smtest holds the cross-repo test-support contract k3sm's
// milestone acceptance harness needs: a shared capability taxonomy and the
// SkipUnless helper that gates a test on it.
//
// # Why this lives in apis
//
// runtimed, darwin-net, and k3sm all carry tests that only make sense on a
// real Apple-Silicon dev box, with root, with network egress, with a
// notarization/signing identity, or in a two-machine lab — the vocabulary
// already enumerated once, per milestone, in the read-only-here
// k3sm/hack/acceptance/phases.json "requires" arrays. Three repos needing one
// small, stable helper is exactly the case apis exists for (see
// k3sm.io/apis's own doc comment): apis is the only DAG-legal home all three
// import without creating a cycle, so the taxonomy is defined once here
// rather than copied into each repo (a copy drifts) or reached for sideways
// (which apis's "depends on nothing in k3sm.io/*" rule forbids).
//
// This carve is deliberately narrow: it is the one piece of apis's public
// CI and test-support work that runtimed's MLX acceptance gate
// (SkipUnless(t, "apple-gpu")) needs before the wider public-CI milestone
// (which also owns the still-deferred CI workflow itself) is ready to open.
// Nothing here touches CI configuration. The capability helpers are pure Go
// and stdlib-only; the runtime contract suite below also imports gRPC and
// this module's runtime/v1, and nothing outside this module.
//
// # The runtime contract suite
//
// RunRuntimeContract checks the node to runtime gRPC seam once: the fields
// and outcomes the node acts on (the pod id and phase on every status, an
// unknown pod reported in-band as NotFound, an idempotent delete, a named
// runtime). The real runtime daemon and each in-memory fake the node's tests
// use run the same suite, so a fake that drifts from the daemon goes red in
// its own repository instead of passing tests that the daemon would fail.
// TestRuntimeContract proves the suite itself: a reference fake passes it,
// and fakes that each drop one field fail the case that owns that field.
//
// # Skip vs. fatal
//
// A test that calls SkipUnless(t, cap) skips when cap is not available in the
// current process — correct for a laptop `go test ./...` run, where most
// capabilities (a second Mac, an Apple GPU, a code-signing identity) are
// legitimately absent. It is wrong for a lab run that exists specifically to
// prove a capability-gated code path executes: if every gated test quietly
// skips, the run reports green while proving nothing.
//
// SkipUnless resolves this with one environment variable, K3SM_CI_REQUIRE: a
// comma/whitespace-separated list of capability names the current run
// promises are present. When cap is absent and named in K3SM_CI_REQUIRE,
// SkipUnless calls t.Fatal instead of t.Skip, so the run cannot silently pass
// by skipping the very thing it was launched to prove. See capability.go for
// how each capability's presence is determined.
//
// # Stability contract
//
// The Capability values, the taxonomy (All), the SkipUnless/Available
// signatures, and the RuntimeContract/RunRuntimeContract/CheckRuntimeContract/
// ServeRuntime signatures are part of the shared contract every importing repo compiles
// against — additive-only: a new Capability constant may be added; an
// existing one is never renamed or repurposed once a consumer depends on it.
package k3smtest
