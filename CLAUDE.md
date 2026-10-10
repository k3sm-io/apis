# apis — k3sm shared contracts

Module **`k3sm.io/apis`**. The shared-contracts module for k3sm: gRPC protos + generated Go,
cross-repo Go types (PodBox spec, OCI image manifest), and CRD types.
**Depends on nothing in `k3sm.io/*`** — it exists to break import cycles between `runtimed`,
`darwin-net`, and `k3sm`.

> Roadmap & current phase: `docs/PHASES.md`.

## Build / test (pure Go)
```sh
gofmt -l .            # must be empty
go vet ./...
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
go mod tidy
```

## Layout
- `runtime/v1/`, `guest/v1/`, `shim/v1/` — gRPC protos + generated Go
- `net/v1/`, `storage/v1/` — shared Go types (DNS, mesh/`MeshPeer`, Service, storage)
- `net/v1alpha1/`, `mlx/v1alpha1/`, `helm/v1/` — CRD types; the CRD manifests live in `config/crd/`
- `k3smtest/` — shared test helpers (capability skips)
- `buf/` — the breaking-change baseline for `buf breaking`

## Standards
@docs/GO-STANDARDS.md
