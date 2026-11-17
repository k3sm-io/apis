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
- `net/v1alpha1/`, `mlx/v1alpha1/`, `helm/v1/`, `budget/v1alpha1/`, `updates/v1alpha1/` — CRD types; the CRD manifests live in `config/crd/`
- `k3smtest/` — shared test helpers (capability skips)
- `buf/` — the breaking-change baseline for `buf breaking`

## Containers only, and no CI on GitHub

- **Containers only.** Nothing is installed on the host Mac, for supply-chain safety: no `brew`, no global
  `go install`/`cargo install`/`pip install`/`npm -g` of a tool, no `curl | sh`, no `sudo` installs. Toolchains
  and services run in k3sm pods; a tool the gate needs runs in a pod image pinned to an exact version and
  checksum. When a gate names a missing tool, it gives the pinned command to run in the pod, not on the host.
- **No CI on GitHub: no GitHub Actions workflows, ever, in any project, and Actions stays disabled; all gates
  run locally in k3sm pods**, through Yureisha CI (the `yureisha/ci` commit status) or this repo's own gate
  script, `hack/ci.sh`. No file may exist under `.github/workflows/`, commented out or not. `hack/ci.sh`
  checks the DCO sign-off on every commit a branch adds.

## Standards
@docs/GO-STANDARDS.md
