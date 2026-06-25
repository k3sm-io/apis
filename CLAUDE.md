# apis — k3sm shared contracts

Module **`k3sm.io/apis`**. The shared-contracts module for k3sm: gRPC protos + generated Go,
cross-repo Go types (PodBox spec, OCI image manifest), and CRD types (`net.k3sm.io`, `runtime.k3sm.io`).
**Depends on nothing in `k3sm.io/*`** — it exists to break import cycles between `runtimed`,
`darwin-net`, and `k3sm`.

> Roadmap & current phase: `docs/PHASES.md` (workspace matrix: `../ROADMAP.md`).

## Build / test (pure Go)
```sh
gofmt -l .            # must be empty
go vet ./...
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
go mod tidy
```

## Layout (planned)
- `runtime/v1/`, `net/v1/` — gRPC protos + generated Go
- shared Go types + CRD types

## Standards
@docs/GO-STANDARDS.md
