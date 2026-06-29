# apis — k3sm shared contracts

`k3sm.io/apis` holds the cross-repo contracts for [k3sm](https://github.com/k3sm-io/k3sm): gRPC
protobufs (the `runtimed` runtime API, the `netd` API), shared Go types (PodBox spec, OCI image
manifest), and CRD types (`MeshPeer`, `NodeNetwork` under `net.k3sm.io` / `runtime.k3sm.io`).

It has **no dependencies** — its purpose is to break import cycles between `runtimed`, `darwin-net`,
and `k3sm`.

Architecture: see [k3sm/docs/DESIGN.md](https://github.com/k3sm-io/k3sm/blob/main/docs/DESIGN.md).

## License

[Apache License 2.0](LICENSE). Contributions require a Developer Certificate of Origin sign-off
(`git commit -s`) — see [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md),
and [SECURITY.md](SECURITY.md).
