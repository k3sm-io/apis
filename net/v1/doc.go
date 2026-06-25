// Package netv1 holds the cross-repo Go types for k3sm's userspace pod
// networking: the data the userspace Service proxy and the getaddrinfo DNS-shim
// exchange at the repo boundary. It is consumed at compile time by
// k3sm.io/darwin-net (which implements the proxy and the DYLD getaddrinfo shim)
// and produced by k3sm.io/k3sm (the server that watches Services/EndpointSlices
// and hosts CoreDNS) — see k3sm/docs/DESIGN.md §6.
//
// Unlike runtime/v1 these are plain Go structs, NOT a protobuf wire contract:
// they are shared in-process configuration, not RPC messages. They are pure
// data with no behavior beyond small construction/validation helpers, carry no
// k3sm.io/* imports, and build pure-Go (CGO_ENABLED=0).
//
// Stability contract: the surface is ADDITIVE-ONLY. Existing exported fields and
// the documented enum constants are stable; new optional fields may be appended.
// JSON struct tags use camelCase to match the corev1 objects these mirror, so a
// proxy can round-trip them through the watch cache unchanged.
package netv1
