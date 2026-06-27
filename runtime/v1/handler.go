package runtimev1

import (
	"errors"
	"fmt"
)

// HandlerName is the value of an upstream node.k8s.io/v1 RuntimeClass's
// `handler` field — equivalently the RuntimeClass a Pod selects via
// spec.runtimeClassName, which the kube scheduler resolves to that class's
// handler. k3sm does NOT define a RuntimeClass-like CRD of its own: the
// operator-facing surface is the standard node.k8s.io RuntimeClass object, and
// k3sm maps its handler to an isolation backend via HandlerConfig AFTER
// admission and scheduling. The empty value means "no RuntimeClass" — the
// default native host-process path.
type HandlerName string

const (
	// HandlerVM is the RuntimeClass handler that selects the
	// Virtualization.framework micro-VM backend (SandboxBackend_SANDBOX_BACKEND_VM)
	// for Linux-image / untrusted-tenancy pods — see k3sm/docs/DESIGN.md §5a. An
	// operator opts a workload in by creating a RuntimeClass with handler "vm" and
	// setting the pod's runtimeClassName to it.
	HandlerVM HandlerName = "vm"
)

// ErrUnknownHandler is returned by HandlerConfig.Backend when a non-empty
// RuntimeClass handler has no mapping in the config. k3sm fails closed on it
// rather than silently running a pod that asked for stronger isolation (e.g.
// "vm") on the weaker host-process path. Wrap it via %w; test for it with
// errors.Is.
var ErrUnknownHandler = errors.New("runtimev1: unknown RuntimeClass handler")

// ErrInvalid is returned by HandlerConfig.Validate when the mapping is
// malformed (an empty handler key, or a backend value that is not a defined
// SandboxBackend). It signals a programming/config error, distinct from the
// runtime lookup miss ErrUnknownHandler. Wrap it via %w; test for it with
// errors.Is.
var ErrInvalid = errors.New("runtimev1: invalid handler config")

// HandlerConfig is the fixed, compile-time mapping from an upstream
// node.k8s.io/v1 RuntimeClass handler name to the k3sm SandboxBackend the
// runtime uses to isolate the pod. It is plain Go config — NOT a CRD and NOT a
// protobuf wire type: k3sm neither forks nor vendors the upstream RuntimeClass
// API; this table is purely how k3sm resolves a RuntimeClass handler to a
// backend after admission/scheduling, mirroring the plain-Go-config precedent
// of net/v1's ServiceVIP / DNSConfig and storage/v1's LocalPathClass.
//
// Use DefaultHandlerConfig for the built-in table. The zero value resolves the
// empty handler to SANDBOX_BACKEND_UNSPECIFIED and treats every named handler
// as unknown; call WithDefaults to populate the built-in mappings.
type HandlerConfig struct {
	// Handlers maps a non-empty RuntimeClass handler name to its SandboxBackend.
	// The empty handler ("") is NOT a key here — Backend resolves it to
	// DefaultBackend — and an empty key is rejected by Validate.
	Handlers map[HandlerName]SandboxBackend `json:"handlers,omitempty"`

	// DefaultBackend is the backend for the empty/default handler (a pod with no
	// runtimeClassName). The zero value, SANDBOX_BACKEND_UNSPECIFIED, means "let
	// runtimed pick the best host-process Seatbelt backend for the host OS
	// version" (see SandboxBackend) — the intended host-process default.
	DefaultBackend SandboxBackend `json:"defaultBackend,omitempty"`
}

// DefaultHandlerConfig returns the built-in handler→backend table: the "vm"
// handler (HandlerVM) maps to the Virtualization.framework micro-VM backend
// (SANDBOX_BACKEND_VM), and the empty/default handler maps to
// SANDBOX_BACKEND_UNSPECIFIED, deferring to runtimed's host-OS-version-gated
// choice of host-process backend. It returns a fresh map on every call, so a
// caller may mutate the result without affecting later calls.
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		Handlers: map[HandlerName]SandboxBackend{
			HandlerVM: SandboxBackend_SANDBOX_BACKEND_VM,
		},
		DefaultBackend: SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED,
	}
}

// Backend resolves a RuntimeClass handler to the SandboxBackend the runtime
// must use. The empty handler (a pod with no runtimeClassName) resolves to
// DefaultBackend. A non-empty handler absent from the table is an error wrapping
// ErrUnknownHandler: k3sm fails closed rather than silently downgrading a pod
// that requested stronger isolation to the host-process path.
func (c HandlerConfig) Backend(handler HandlerName) (SandboxBackend, error) {
	if handler == "" {
		return c.DefaultBackend, nil
	}
	if b, ok := c.Handlers[handler]; ok {
		return b, nil
	}
	return SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED, fmt.Errorf("%w: %q", ErrUnknownHandler, handler)
}

// WithDefaults returns a copy of the config with the built-in mappings filled
// in: a nil Handlers map is replaced with DefaultHandlerConfig's table, and any
// handler the built-in table defines that the receiver omits is added (the
// receiver's own mappings win on conflict). It does not mutate the receiver, so
// it is safe to call on a shared object.
func (c HandlerConfig) WithDefaults() HandlerConfig {
	out := c
	def := DefaultHandlerConfig()
	if out.Handlers == nil {
		out.Handlers = make(map[HandlerName]SandboxBackend, len(def.Handlers))
	} else {
		cp := make(map[HandlerName]SandboxBackend, len(out.Handlers)+len(def.Handlers))
		for k, v := range out.Handlers {
			cp[k] = v
		}
		out.Handlers = cp
	}
	for k, v := range def.Handlers {
		if _, ok := out.Handlers[k]; !ok {
			out.Handlers[k] = v
		}
	}
	return out
}

// Validate reports whether the config is well-formed: no empty handler key
// (the empty handler is resolved via DefaultBackend, so an empty key would be
// dead config), and every mapped backend plus DefaultBackend is a defined
// SandboxBackend value. Errors wrap ErrInvalid. A nil/empty Handlers map is
// valid (every named handler is simply unknown).
func (c HandlerConfig) Validate() error {
	if !validBackend(c.DefaultBackend) {
		return fmt.Errorf("%w: defaultBackend %d is not a defined SandboxBackend", ErrInvalid, c.DefaultBackend)
	}
	for name, backend := range c.Handlers {
		if name == "" {
			return fmt.Errorf("%w: empty handler key is resolved via defaultBackend, not the table", ErrInvalid)
		}
		if !validBackend(backend) {
			return fmt.Errorf("%w: handler %q maps to %d, not a defined SandboxBackend", ErrInvalid, name, backend)
		}
	}
	return nil
}

// validBackend reports whether b is a defined SandboxBackend enum value (using
// the generated name table, so it stays correct as new backends are added).
func validBackend(b SandboxBackend) bool {
	_, ok := SandboxBackend_name[int32(b)]
	return ok
}
