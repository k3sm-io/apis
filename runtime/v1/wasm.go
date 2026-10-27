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

package runtimev1

// The wasm RuntimeClass contract strings, published for the wasm backend's
// producer (runtimed) and consumer (the k3sm provider), which must import them
// rather than re-declare a literal, so the two cannot drift apart. The backend
// is not implemented yet; the rules below are the contract it must meet.
const (
	// WasmPlatformOS is the OCI platform os of a WebAssembly (WASI) module
	// image.
	WasmPlatformOS = "wasi"

	// WasmPlatformArch is the OCI platform architecture of a WebAssembly
	// (WASI) module image.
	WasmPlatformArch = "wasm32"

	// WasmPlatform is the canonical OCI platform string of a WebAssembly module
	// image, WasmPlatformOS + "/" + WasmPlatformArch. An image whose platform is
	// this tuple or one of WasmPlatformAliases is a wasm image.
	WasmPlatform = WasmPlatformOS + "/" + WasmPlatformArch

	// ConditionWasmBackendAvailable is the RuntimeCondition type runtimed sets
	// in its GetRuntimeInfo response when the wasm backend, including its module
	// verifier, is ready on the node (its status is true then, false with a
	// reason otherwise). A producer must never stamp PodBox.wasm_signature for a
	// node that does not report this condition as true: a runtime that predates
	// the verifier does not report it, and would ignore the requirement. When a
	// pod requests the signature check and its node does not report the
	// condition, the provider refuses the pod; it never drops the stamp and runs
	// the pod unverified. A runtime that receives a SANDBOX_BACKEND_WASM pod with
	// wasm_signature present but has no verifier refuses the pod, never skipping
	// the check.
	ConditionWasmBackendAvailable = "WasmBackendAvailable"

	// LabelWasm is the label that will mark a node whose runtime reports
	// ConditionWasmBackendAvailable, so the "wasm" RuntimeClass schedules only
	// onto nodes that can run it.
	LabelWasm = "k3sm.io/wasm"

	// AnnotationWasmSignature is the pod-level annotation opting a wasm pod in
	// to the module signature check. "required" is its one accepted value.
	//
	// The k3sm provider must read it and AnnotationWasmSignatureKey once, at pod
	// create, under a closed truth table:
	//
	//   - "required" with a well-formed key annotation: stamp
	//     PodBox.wasm_signature in the WasmSignatureEnforce state.
	//   - neither annotation: no stamp; no signature check.
	//   - the key annotation without this one: refuse the pod.
	//   - "required" without a key annotation, or with a malformed key: refuse
	//     the pod.
	//   - any other value of this annotation: refuse the pod.
	//
	// The PodBox stamped at create is authoritative for the life of the pod: a
	// later PATCH of either annotation is ignored, and a restart or resync never
	// re-derives the stamp from the live annotations.
	//
	// Scope: the check gives integrity and provenance for modules signed with
	// the pod author's own key. It is not an admission or tenancy control, since
	// whoever writes the pod chooses both the key and whether to ask for it.
	AnnotationWasmSignature = "k3sm.io/wasm-signature"

	// AnnotationWasmSignatureKey is the pod-level annotation carrying the PEM
	// public key a wasm pod's modules must verify against. It is valid only
	// together with AnnotationWasmSignature set to "required"; on its own the
	// provider refuses the pod (see AnnotationWasmSignature for the full truth
	// table). The provider reads it once, at pod create, decodes the PEM to
	// DER, failing closed on a malformed key, and stamps
	// WasmSignature.public_key_spki_der. The stamp is authoritative for the
	// life of the pod: a later PATCH is ignored and a restart or resync never
	// re-derives it. The runtime never sees PEM. Like the signature annotation,
	// it is an integrity check for the pod author's own key, not an admission
	// or tenancy control.
	AnnotationWasmSignatureKey = "k3sm.io/wasm-signature-key"
)

// WasmPlatformAliases returns the OCI platform strings, other than WasmPlatform,
// that also identify a wasm module image: "wasi/wasm", "wasip1/wasm32" and
// "wasip2/wasm32", in that order. It returns a fresh slice on every call, so a
// caller may mutate the result without affecting later calls or other callers.
func WasmPlatformAliases() []string {
	return []string{"wasi/wasm", "wasip1/wasm32", "wasip2/wasm32"}
}

// WasmSignatureMode is the classification of a PodBox.wasm_signature value.
// ClassifyWasmSignature is the one place that classification is made.
type WasmSignatureMode int

const (
	// WasmSignatureNone means the message is absent: no signature check.
	WasmSignatureNone WasmSignatureMode = iota

	// WasmSignatureEnforce means required is true and the key is non-empty:
	// every module of the pod must verify against the key before it runs.
	WasmSignatureEnforce

	// WasmSignatureInvalid means the message is present in any other state
	// (required false with or without a key, or required true with an empty
	// key). The consumer refuses the pod; it never reads this as "no check".
	WasmSignatureInvalid
)

// ClassifyWasmSignature returns the mode of a PodBox.wasm_signature value: nil
// is WasmSignatureNone, required with a non-empty key is WasmSignatureEnforce,
// and every other present state is WasmSignatureInvalid, which means refuse
// the pod.
//
// Consumers MUST classify through this function rather than
// GetWasmSignature().GetRequired(): proto3 getters collapse "absent" and
// "present with required false" to the same false, and only the first means
// "no check".
func ClassifyWasmSignature(s *WasmSignature) WasmSignatureMode {
	switch {
	case s == nil:
		return WasmSignatureNone
	case s.GetRequired() && len(s.GetPublicKeySpkiDer()) > 0:
		return WasmSignatureEnforce
	default:
		return WasmSignatureInvalid
	}
}
