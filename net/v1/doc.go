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

// Package netv1 holds the cross-repo Go types for k3sm's pod networking. It is
// consumed at compile time by k3sm.io/darwin-net (the proxy, the DYLD
// getaddrinfo shim, and the wireguard mesh) and produced by k3sm.io/k3sm (the
// server that watches Services/EndpointSlices, hosts CoreDNS, and drives the
// node join) — see k3sm/docs/DESIGN.md §5b/§6. It holds three kinds of contract:
//
//   - Service-proxy + DNS-shim config (service.go, dns.go): plain Go structs,
//     NOT a protobuf wire contract — shared in-process configuration the proxy
//     and shim round-trip through the watch cache as JSON.
//   - The MeshPeer CRD (mesh.go): a real, served/watchable net.k3sm.io/v1
//     Kubernetes custom resource (kine-stored, apiserver-served) carrying a
//     node's wireguard PUBLIC key + endpoint + podCIDR + symmetric AllowedIPs +
//     a SchemaVersion. Being a CRD it embeds metav1.TypeMeta + ObjectMeta — the
//     one reason this module depends on k8s.io/apimachinery (pinned in go.mod in
//     lockstep with k3sm). Private keys are NEVER carried (DESIGN §5b).
//   - The mesh-enroll join payloads (mesh.go): plain Go structs the bootstrap
//     join HTTP exchange marshals, version-stamped from day one.
//
// The module still imports zero k3sm.io/* packages; k8s.io/apimachinery is pure
// Go, so the package builds CGO_ENABLED=0.
//
// Stability contract: the surface is ADDITIVE-ONLY. Existing exported fields and
// the documented constants are stable; new optional fields may be appended.
// JSON struct tags use camelCase to match the corev1 objects these mirror, so a
// proxy / informer can round-trip them through the watch cache unchanged; the
// MeshPeer payload's own evolution rides Spec.SchemaVersion within the v1 GVK.
package netv1
