package runtimev1

import (
	"errors"
	"testing"
)

// TestHandlerConfigMapsVMBackend is the M5.1-a2 mapping table test: the built-in
// DefaultHandlerConfig resolves "vm" to SANDBOX_BACKEND_VM, the empty/default
// handler to the host-process default (SANDBOX_BACKEND_UNSPECIFIED), and any
// unknown non-empty handler to ErrUnknownHandler (fail closed).
func TestHandlerConfigMapsVMBackend(t *testing.T) {
	t.Parallel()
	cfg := DefaultHandlerConfig()
	cases := []struct {
		name    string
		handler HandlerName
		want    SandboxBackend
		wantErr error
	}{
		{"vm maps to the VM backend", HandlerVM, SandboxBackend_SANDBOX_BACKEND_VM, nil},
		{"empty handler is the host-process default", "", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, nil},
		{"unknown handler fails closed", "kata", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, ErrUnknownHandler},
		{"handler match is case sensitive", "VM", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, ErrUnknownHandler},
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
}

// TestHandlerConfigBackendDefault verifies DefaultBackend governs the empty
// handler and that the zero-value config treats every named handler as unknown.
func TestHandlerConfigBackendDefault(t *testing.T) {
	t.Parallel()

	t.Run("custom default backend honored for empty handler", func(t *testing.T) {
		t.Parallel()
		cfg := HandlerConfig{DefaultBackend: SandboxBackend_SANDBOX_BACKEND_SEATBELT_INPROC}
		got, err := cfg.Backend("")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != SandboxBackend_SANDBOX_BACKEND_SEATBELT_INPROC {
			t.Errorf("Backend(\"\") = %v, want SEATBELT_INPROC", got)
		}
	})

	t.Run("zero value: empty handler is UNSPECIFIED, named handler is unknown", func(t *testing.T) {
		t.Parallel()
		var cfg HandlerConfig
		if got, err := cfg.Backend(""); err != nil || got != SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED {
			t.Fatalf("Backend(\"\") = %v, %v; want UNSPECIFIED, nil", got, err)
		}
		if _, err := cfg.Backend(HandlerVM); !errors.Is(err, ErrUnknownHandler) {
			t.Errorf("Backend(vm) on zero config err = %v, want ErrUnknownHandler", err)
		}
	})
}

// TestHandlerConfigWithDefaults verifies WithDefaults fills the built-in table,
// preserves caller mappings (caller wins on conflict), and does not mutate the
// receiver.
func TestHandlerConfigWithDefaults(t *testing.T) {
	t.Parallel()

	t.Run("nil map gets the built-in table", func(t *testing.T) {
		t.Parallel()
		got, err := HandlerConfig{}.WithDefaults().Backend(HandlerVM)
		if err != nil || got != SandboxBackend_SANDBOX_BACKEND_VM {
			t.Fatalf("Backend(vm) = %v, %v; want VM, nil", got, err)
		}
	})

	t.Run("caller mapping wins and extras are preserved", func(t *testing.T) {
		t.Parallel()
		base := HandlerConfig{Handlers: map[HandlerName]SandboxBackend{
			HandlerVM: SandboxBackend_SANDBOX_BACKEND_UIDJAIL, // override the built-in
			"custom":  SandboxBackend_SANDBOX_BACKEND_SEATBELT_EXEC,
		}}
		out := base.WithDefaults()
		if got, _ := out.Backend(HandlerVM); got != SandboxBackend_SANDBOX_BACKEND_UIDJAIL {
			t.Errorf("Backend(vm) = %v, want caller override UIDJAIL", got)
		}
		if got, _ := out.Backend("custom"); got != SandboxBackend_SANDBOX_BACKEND_SEATBELT_EXEC {
			t.Errorf("Backend(custom) = %v, want SEATBELT_EXEC", got)
		}
		// Receiver is not mutated (still its single original entry).
		if len(base.Handlers) != 2 {
			t.Errorf("WithDefaults mutated receiver: len = %d, want 2", len(base.Handlers))
		}
	})
}

// TestHandlerConfigValidate verifies config well-formedness checks wrap
// ErrInvalid and that the built-in table validates.
func TestHandlerConfigValidate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     HandlerConfig
		wantErr bool
	}{
		{"built-in table is valid", DefaultHandlerConfig(), false},
		{"zero value is valid", HandlerConfig{}, false},
		{"empty handler key rejected", HandlerConfig{Handlers: map[HandlerName]SandboxBackend{"": SandboxBackend_SANDBOX_BACKEND_VM}}, true},
		{"undefined backend value rejected", HandlerConfig{Handlers: map[HandlerName]SandboxBackend{HandlerVM: SandboxBackend(99)}}, true},
		{"undefined default backend rejected", HandlerConfig{DefaultBackend: SandboxBackend(99)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Validate() err = %v, want errors.Is ErrInvalid", err)
				}
			} else if err != nil {
				t.Fatalf("Validate() unexpected err: %v", err)
			}
		})
	}
}

// TestSandboxProfileVMParams proves the additive M5.1 VM-sizing fields carry the
// micro-VM's vCPU count + RAM, round-trip on the wire, and are DISTINCT from the
// PodBox OOM ceiling (PodBox.memory_limit_bytes): a VM-backed pod can be
// BestEffort (memory_limit_bytes == 0) yet still boot with a concrete VM memory
// size.
func TestSandboxProfileVMParams(t *testing.T) {
	t.Parallel()
	const vmMemoryBytes = 512 << 20 // 512 MiB

	sp := &SandboxProfile{
		Backend:       SandboxBackend_SANDBOX_BACKEND_VM,
		VmVcpus:       2,
		VmMemoryBytes: vmMemoryBytes,
	}
	roundTrip(t, "vm-sized sandbox profile", sp, &SandboxProfile{})

	if got := sp.GetVmVcpus(); got != 2 {
		t.Errorf("GetVmVcpus() = %d, want 2", got)
	}
	if got := sp.GetVmMemoryBytes(); got != vmMemoryBytes {
		t.Errorf("GetVmMemoryBytes() = %d, want %d", got, vmMemoryBytes)
	}

	t.Run("vm memory is distinct from the OOM ceiling", func(t *testing.T) {
		t.Parallel()
		pod := &PodBox{
			PodId:            "p1",
			MemoryLimitBytes: 0, // BestEffort: no OOM ceiling
			SandboxProfile:   sp,
		}
		roundTrip(t, "besteffort-pod-with-vm-memory", pod, &PodBox{})
		if pod.GetMemoryLimitBytes() != 0 {
			t.Errorf("MemoryLimitBytes = %d, want 0 (BestEffort)", pod.GetMemoryLimitBytes())
		}
		if pod.GetSandboxProfile().GetVmMemoryBytes() != vmMemoryBytes {
			t.Errorf("VM memory = %d, want %d (independent of the OOM ceiling)", pod.GetSandboxProfile().GetVmMemoryBytes(), vmMemoryBytes)
		}
	})
}
