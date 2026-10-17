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

import (
	"errors"
	"slices"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestHandlerTable pins the built-in RuntimeClass handler table: the empty
// handler, "vm" and "wasm" resolve to their backends; every other handler,
// including near-misses of "wasm" and the not-yet-defined "container-vm", fails
// closed with ErrUnknownHandler; SandboxBackend value 5 is WASM on the wire; and
// the default table validates.
func TestHandlerTable(t *testing.T) {
	t.Parallel()
	cfg := DefaultHandlerConfig()

	cases := []struct {
		name    string
		handler HandlerName
		want    SandboxBackend
		wantErr error
	}{
		{"empty handler is the host-process default", "", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, nil},
		{"vm maps to the VM backend", HandlerVM, SandboxBackend_SANDBOX_BACKEND_VM, nil},
		{"wasm maps to the WASM backend", HandlerWasm, SandboxBackend_SANDBOX_BACKEND_WASM, nil},
		{"wasm literal maps to the WASM backend", "wasm", SandboxBackend_SANDBOX_BACKEND_WASM, nil},
		{"container-vm is unknown", "container-vm", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, ErrUnknownHandler},
		{"wasm match is case sensitive", "Wasm", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, ErrUnknownHandler},
		{"wasm match does not trim", "wasm ", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, ErrUnknownHandler},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := cfg.Backend(tc.handler)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Backend(%q) err = %v, want errors.Is %v", tc.handler, err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Backend(%q) unexpected err: %v", tc.handler, err)
			}
			if got != tc.want {
				t.Errorf("Backend(%q) = %v, want %v", tc.handler, got, tc.want)
			}
		})
	}

	t.Run("wasm handler name is the literal wasm", func(t *testing.T) {
		t.Parallel()
		if HandlerWasm != "wasm" {
			t.Errorf("HandlerWasm = %q, want %q", HandlerWasm, "wasm")
		}
	})

	t.Run("backend 5 is WASM on the wire", func(t *testing.T) {
		t.Parallel()
		if got := SandboxBackend_name[5]; got != "SANDBOX_BACKEND_WASM" {
			t.Errorf("SandboxBackend_name[5] = %q, want SANDBOX_BACKEND_WASM", got)
		}
		if got := int32(SandboxBackend_SANDBOX_BACKEND_WASM); got != 5 {
			t.Errorf("SANDBOX_BACKEND_WASM = %d, want 5", got)
		}
	})

	t.Run("default table validates", func(t *testing.T) {
		t.Parallel()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("DefaultHandlerConfig().Validate() = %v", err)
		}
	})
}

// TestPodBoxWasmSignature pins PodBox.wasm_signature: field 104, a singular
// k3sm.runtime.v1.WasmSignature, round-tripping on the wire, with the PodBox
// headroom narrowed to exactly 105..199. It is descriptor-driven so it fails at
// runtime, not compile time, on a tree without the field.
func TestPodBoxWasmSignature(t *testing.T) {
	t.Parallel()

	md := File_runtime_v1_runtime_proto.Messages().ByName("PodBox")
	if md == nil {
		t.Fatal("message k3sm.runtime.v1.PodBox not found")
	}

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("wasm_signature")
		if fd == nil {
			t.Fatal("PodBox.wasm_signature is not declared")
		}
		if fd.Number() != 104 {
			t.Errorf("wasm_signature number = %d, want 104", fd.Number())
		}
		if fd.IsList() || fd.Kind() != protoreflect.MessageKind {
			t.Errorf("wasm_signature = %v %v, want a singular message", fd.Cardinality(), fd.Kind())
		}
		if m := fd.Message(); m == nil || m.FullName() != "k3sm.runtime.v1.WasmSignature" {
			t.Errorf("wasm_signature type = %v, want k3sm.runtime.v1.WasmSignature", m)
		}
		sig := File_runtime_v1_runtime_proto.Messages().ByName("WasmSignature")
		if sig == nil {
			t.Fatal("message k3sm.runtime.v1.WasmSignature not found")
		}
		for _, want := range []struct {
			name protoreflect.Name
			num  protoreflect.FieldNumber
			kind protoreflect.Kind
		}{
			{"required", 1, protoreflect.BoolKind},
			{"public_key_spki_der", 2, protoreflect.BytesKind},
		} {
			f := sig.Fields().ByName(want.name)
			if f == nil {
				t.Errorf("WasmSignature.%s is not declared", want.name)
				continue
			}
			if f.Number() != want.num || f.Kind() != want.kind {
				t.Errorf("WasmSignature.%s = %d %v, want %d %v", want.name, f.Number(), f.Kind(), want.num, want.kind)
			}
		}
		if n := sig.Fields().Len(); n != 2 {
			t.Errorf("WasmSignature has %d fields, want 2", n)
		}
	})

	roundTrip(t, "pod with a required wasm signature", &PodBox{
		PodId: "p1",
		WasmSignature: &WasmSignature{
			Required:         true,
			PublicKeySpkiDer: []byte{0x30, 0x59, 0x30, 0x13},
		},
	}, &PodBox{})

	t.Run("reserved band is exactly 105..199", func(t *testing.T) {
		t.Parallel()
		bands := podBoxHeadroomBands(md)
		if len(bands) != 1 {
			t.Fatalf("PodBox has %d headroom reserved ranges (excluding retired fields), want exactly 1", len(bands))
		}
		if r := bands[0]; r[0] != podBoxHeadroomFirst || r[1]-1 != podBoxHeadroomLast {
			t.Errorf("PodBox reserved range = %d..%d, want %d..%d", r[0], r[1]-1, podBoxHeadroomFirst, podBoxHeadroomLast)
		}
	})
}

// TestWasmConstants pins every wasm contract string, that the canonical platform
// is built from its two parts, and that the alias function returns the alias
// tuples in order as a fresh slice on every call.
func TestWasmConstants(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, got, want string }{
		{"WasmPlatformOS", WasmPlatformOS, "wasi"},
		{"WasmPlatformArch", WasmPlatformArch, "wasm32"},
		{"WasmPlatform", WasmPlatform, "wasi/wasm32"},
		{"WasmPlatform from parts", WasmPlatformOS + "/" + WasmPlatformArch, WasmPlatform},
		{"ConditionWasmBackendAvailable", ConditionWasmBackendAvailable, "WasmBackendAvailable"},
		{"LabelWasm", LabelWasm, "k3sm.io/wasm"},
		{"AnnotationWasmSignature", AnnotationWasmSignature, "k3sm.io/wasm-signature"},
		{"AnnotationWasmSignatureKey", AnnotationWasmSignatureKey, "k3sm.io/wasm-signature-key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}

	t.Run("aliases in order", func(t *testing.T) {
		t.Parallel()
		want := []string{"wasi/wasm", "wasip1/wasm32", "wasip2/wasm32"}
		if got := WasmPlatformAliases(); !slices.Equal(got, want) {
			t.Errorf("WasmPlatformAliases() = %q, want %q", got, want)
		}
	})

	t.Run("aliases are a fresh slice each call", func(t *testing.T) {
		t.Parallel()
		first := WasmPlatformAliases()
		first[0] = "mutated"
		if got := WasmPlatformAliases(); got[0] != "wasi/wasm" {
			t.Errorf("WasmPlatformAliases()[0] = %q after mutating an earlier result, want wasi/wasm", got[0])
		}
	})
}

// TestClassifyWasmSignature pins the closed state matrix of WasmSignature. The
// required=false row is the one proto3 getters cannot tell from absent: a
// classifier that read GetRequired alone would return None for it and run the
// pod unverified.
func TestClassifyWasmSignature(t *testing.T) {
	t.Parallel()
	key := []byte{0x30, 0x59, 0x30, 0x13}
	cases := []struct {
		name string
		sig  *WasmSignature
		want WasmSignatureMode
	}{
		{"absent is none", nil, WasmSignatureNone},
		{"required with a key is enforce", &WasmSignature{Required: true, PublicKeySpkiDer: key}, WasmSignatureEnforce},
		{"not required with a key is invalid", &WasmSignature{Required: false, PublicKeySpkiDer: key}, WasmSignatureInvalid},
		{"required with an empty key is invalid", &WasmSignature{Required: true}, WasmSignatureInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyWasmSignature(tc.sig); got != tc.want {
				t.Errorf("ClassifyWasmSignature(%v) = %d, want %d", tc.sig, got, tc.want)
			}
		})
	}
}
