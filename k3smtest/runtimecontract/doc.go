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

// Package runtimecontract checks the node to runtime gRPC seam
// (k3sm.io/apis/runtime/v1) once, in one place.
//
// RunRuntimeContract checks the fields and outcomes the node acts on: the pod
// id and phase on every status, an unknown pod reported in-band as NotFound,
// an idempotent delete, and a named runtime. The real runtime daemon and each
// in-memory fake the node's tests use run the same suite, so a fake that
// drifts from the daemon goes red in its own repository instead of passing
// tests that the daemon would fail. TestRuntimeContract proves the suite
// itself: a reference fake passes it, and fakes that each drop one field fail
// the case that owns that field. ServeRuntime serves a RuntimeServer over an
// in-memory listener for the suite to call.
//
// # Why a separate package
//
// The suite imports gRPC. It lives apart from k3sm.io/apis/k3smtest so that
// the repositories importing k3smtest only for SkipUnless do not take gRPC
// and its transitive modules into their module graphs.
//
// # Stability contract
//
// The RuntimeContract, RuntimeContractResult, RunRuntimeContract,
// CheckRuntimeContract and ServeRuntime signatures are part of the shared
// contract every importing repository compiles against, and change only
// additively.
package runtimecontract
