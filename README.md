# apis

`k3sm.io/apis` is the shared-contract module for k3sm, a macOS-native Kubernetes distribution
for Apple Silicon. It holds the gRPC protobufs, the Kubernetes custom resource types, and the
small set of plain Go types that cross a repository boundary somewhere in the system: the
runtime API a node daemon serves, the pod and image message shapes both ends of that API agree
on, the custom resources (`MeshPeer`, `Interconnect`, `MLXModel`, `HelmChart`, `HelmChartConfig`, `Update`, `NodeBudget`, `NodeBudgetPolicy`), and the service-proxy and DNS-shim
configuration structs. Every other k3sm repository imports this module, and this module imports
none of them. That keeps the dependency graph free of cycles: a shared type placed here can be
used by any repository, while a type placed in one of the other repositories and needed by a
second one cannot.

## Where this fits

k3sm is split across four repositories with a fixed build order:

```
apis  →  runtimed, darwin-net  →  k3sm
```

`apis` defines a contract before anything implements against it. `runtimed` (the native runtime
daemon) and `darwin-net` (pod networking) each build against the contracts they need and have no
dependency on each other. `k3sm` (the virtual-kubelet provider, control plane, and CLI) sits on
top of both and pulls the same `apis` types in directly. A change to a wire message or a CRD
schema therefore starts here, and the repositories that read it are expected to tolerate an
additive change before this module's own version is bumped in their `go.mod`.

## What's here

| Path | Contents |
|---|---|
| `runtime/v1` | The node runtime gRPC contract: `service Runtime` (pod lifecycle — create, delete, update, watch status, logs, exec, attach, port-forward, stats, restart) and `service Images` (list, inspect, remove, prune, load), both served by the same runtime daemon. Also carries the `PodBox` message (the daemon's unit of execution), the OCI image manifest types, the `SandboxProfile` and `SignaturePolicy` inputs, and small Go-only additions beside the generated code: label/annotation constants and the `RuntimeClass` handler-to-backend mapping. |
| `net/v1` | Plain Go types for pod networking: `ServiceVIP`/`ServicePort`/`Protocol`/`Endpoint` for the userspace Service proxy, `DNSConfig` for the in-pod DNS shim, and the `MeshPeer` custom resource (`net.k3sm.io/v1`) that carries a node's WireGuard public key, endpoint, and pod CIDR. Also carries the mesh-enrollment request/response structs exchanged during node join. Not a protobuf package — these are structs that round-trip as JSON through the API server's watch cache. |
| `storage/v1` | The local-path storage provisioner contract: the `StorageClass` parameters, the per-claim data directory derivation, and the node-topology key used to pin a local `PersistentVolume` to the node that holds its data. Plain Go, complementing the upstream `storage.k8s.io` and `core/v1` types rather than replacing them. |
| `guest/v1` | The contract for a pod that runs inside a Linux micro-VM: the `GuestAgent` gRPC service the in-guest agent serves over vsock, the `GuestSpec` boot descriptor, and the `VMHostSpec` the host-side VM process reads to build the machine. |
| `shim/v1` | The internal contract between the runtime daemon and the resident per-container shim that owns a container's stdio, log file, and exit status: the `ContainerShim` gRPC service. Both ends come from the same runtime build, so it changes in lockstep and is not covered by the additive-only promise; `api_version` makes a mismatch legible. |
| `net/v1alpha1` | The `Interconnect` custom resource (`net.k3sm.io/v1alpha1`, cluster-scoped), the direct-link description between nodes. Marked alpha: fields here may change incompatibly between releases. |
| `helm/v1` | The `HelmChart` and `HelmChartConfig` custom resources (`helm.k3sm.io/v1`, namespaced), mirroring k3s's `helm.cattle.io/v1` field for field. |
| `budget/v1alpha1` | The `NodeBudget` and `NodeBudgetPolicy` custom resources (`budget.k3sm.io/v1alpha1`, both cluster-scoped): the node budget contract, how much more work a node can take and what it has shed. Marked alpha: fields here may change incompatibly between releases. |
| `mlx/v1alpha1` | The `MLXModel` custom resource (`mlx.k3sm.io/v1alpha1`) for native Apple-Silicon model serving, plus the `mlx.k3sm.io/*` label and resource-name constants a node advertises. Marked alpha: fields here may change incompatibly between releases. |
| `updates/v1alpha1` | The `Update` custom resource (`updates.k3sm.io/v1alpha1`), cluster-scoped, that asks for every node to move to one k3sm release: the release tag, the roll strategy (workers before the server, how many nodes at once, whether to drain), the checksum and signature checks the download must pass, whether a downgrade is allowed, and a cancel switch, with per-node progress in its status. Marked alpha: fields here may change incompatibly between releases. |
| `config/crd` | The CustomResourceDefinition manifests for all eight custom resources (`MLXModel`, `MeshPeer`, `Interconnect`, `HelmChart`, `HelmChartConfig`, `Update`, `NodeBudget`, `NodeBudgetPolicy`), embedded into the Go package at build time with the standard library's `embed` directive and exposed through named accessors so a consumer applies the exact bytes checked in here. |
| `k3smtest` | A capability taxonomy and a `SkipUnless` test helper shared by the other repositories' test suites, so a test can name what it needs (root, a second machine, network egress, and so on) instead of each repository inventing its own vocabulary. |
| `buf.yaml`, `buf.gen.yaml`, `buf/baseline.binpb` | The protobuf toolchain configuration and the wire-compatibility baseline (see Building below). |

## Building and testing

The module is pure Go with no cgo:

```sh
gofmt -l .            # must print nothing
go vet ./...
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
go mod tidy
```

`hack/ci.sh` runs the full local gate in one command: formatting, vet, build, test, license-header
verification, `go mod tidy`, and the protobuf checks below.

### Regenerating the protobuf code

Generated Go (`*.pb.go`, `*_grpc.pb.go`) is checked into the repository; nobody hand-edits it. To
regenerate after changing a `.proto` file:

```sh
hack/gen.sh
```

This installs a pinned `buf` and pinned `protoc-gen-go`/`protoc-gen-go-grpc` versions (see
`hack/lib/buf-env.sh`) and runs `buf generate`. `hack/ci.sh` re-runs `buf generate` itself and
fails if it produces a diff, so the checked-in code and the `.proto` source can never drift apart
silently.

Regeneration uses the committed `buf.lock` as is. `hack/gen.sh --update-deps` runs `buf dep update`
and moves the lock deliberately; `hack/gen.sh --check-lock` (offline) fails when `buf.lock` and
`buf.yaml` disagree.

### The wire-stability check

`hack/ci.sh` also runs `buf breaking --against buf/baseline.binpb`: it fails the build if a
change would break wire or JSON compatibility with everything already committed — a renumbered
field, a retyped field, a deleted field, a reused reserved number. After a deliberate, reviewed
change to the wire surface, the baseline is regenerated with `buf build -o buf/baseline.binpb` and
committed alongside the proto change, so it always reflects the last agreed-on shape rather than
comparing a change against itself.

## Versioning and compatibility

Every proto file in this module states the same rule in its header comment: field numbers are
stable and the surface is additive-only. A field is never renumbered or repurposed, and a
reserved number or name is never reused. New fields take the next free number, or are carved out
of a range the message has explicitly set aside with a `reserved` statement — for example
`PodBox` reserves `105` to `199` for fields not yet defined (`100` to `104` are allocated), and each
carve narrows that range rather than removing it. `buf breaking` enforces this mechanically on
every change.

The Go-only packages (`net/v1`, `storage/v1`) follow the same additive-only discipline by
convention: existing exported fields, functions, and constants are stable, and new fields are
optional additions. The alpha custom resources (`mlx/v1alpha1`, `updates/v1alpha1`,
`budget/v1alpha1` and `net/v1alpha1`, which holds `Interconnect`) may change incompatibly between
releases; each package comment says so. Their group names and label and annotation keys stay
stable node- and scheduler-facing identifiers even while the field shapes are not.

Custom resources are served and stored at a single version today: `v1` for `MeshPeer`,
`HelmChart` and `HelmChartConfig`; `v1alpha1` for `MLXModel`, `Interconnect`, `Update`,
`NodeBudget` and `NodeBudgetPolicy`. Within a served version, payload evolution rides an explicit
schema version field on the object's spec rather than a new API version.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the build and test gate, commit message conventions,
and the Developer Certificate of Origin sign-off required on every commit (see
[DCO](DCO)). [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), [GOVERNANCE.md](GOVERNANCE.md), and
[SECURITY.md](SECURITY.md) cover conduct, project governance, and how to report a vulnerability.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
