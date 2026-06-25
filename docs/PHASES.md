---
repo: apis
schema: phases/v1
current_phase: M1
updated: 2026-06-24
updated_by: human

phases:
  - id: M0
    title: Walking skeleton (no apis work — the M0 provider was self-contained)
    status: done
    completed: 2026-06-24
    depends_on: []
    subphases: []

  - id: M1
    title: Runtime + image + Service/DNS contracts
    status: todo
    depends_on: []
    subphases:
      - id: M1.1
        title: Runtime gRPC proto + image manifest + PodBox spec
        status: todo
        deliverables:
          - id: M1.1-d1
            done: false
            desc: runtime/v1 gRPC proto + generated Go (CreatePod / StartContainer / PodStatus / Logs) — the surface runtimed implements and k3sm's provider calls
          - id: M1.1-d2
            done: false
            desc: image manifest Go type (OCI artifact descriptor) plus signature-policy enum (adhoc-ok | require-signed | require-notarized)
          - id: M1.1-d3
            done: false
            desc: PodBox spec type (pod id, rootfs path, uid/gid, pod IP, profile inputs) per DESIGN §5a
        acceptance:
          - id: M1.1-a1
            met: false
            check: CGO_ENABLED=0 go build ./... passes and generated code is committed; protoc/buf regeneration is reproducible against a golden
            method: build
          - id: M1.1-a2
            met: false
            check: marshal then unmarshal of every generated type is identity (golden bytes in testdata)
            method: unit
          - id: M1.1-a3
            met: false
            check: the module imports zero k3sm.io/* packages (cycle check)
            method: build
      - id: M1.2
        title: Service-proxy + DNS-shim shared types
        status: todo
        deliverables:
          - id: M1.2-d1
            done: false
            desc: shared Go types that cross the repo boundary for the userspace Service proxy (ServiceVIP, endpoint tuple) and the DNS-shim config
        acceptance:
          - id: M1.2-a1
            met: false
            check: builds pure-Go and a darwin-net compile-check consumes the types
            method: build

  - id: M2
    title: gRPC daemon surface + resource/metrics types
    status: todo
    depends_on: []
    subphases: []

  - id: M3
    title: MeshPeer CRD + NodeNetwork + mesh-enroll types
    status: todo
    depends_on: []
    subphases: []

  - id: M4
    title: API-stability freeze + vanity-import resolves
    status: todo
    depends_on: []
    subphases: []
---

# apis — Phase roadmap

> Per-repo slice of the k3sm milestones (workspace matrix: `../../ROADMAP.md`; product design:
> `../../k3sm/docs/DESIGN.md` §6/§7). The YAML frontmatter above is **authoritative**; this prose
> mirrors it. Status: ✅ done · 🟡 in-progress · ⛔ blocked · ⬜ todo.

`apis` is always **Wave 1**: it defines the contract for a milestone before any dependent implements
against it. It **depends on nothing** in `k3sm.io/*` (that is its whole job — breaking import cycles),
so it has no `depends_on` edges; instead, every other repo's milestone `depends_on` an `apis` sub-phase.

## M0 — (no work) ✅
The walking skeleton needed no shared contracts — the `k3sm` HostProcess provider was self-contained
and `runtimed`'s M0 was a standalone Seatbelt prototype. First `apis` code lands in M1.

## M1 — Runtime + image + Service/DNS contracts ⬜

### M1.1 — runtime proto + image manifest + PodBox ⬜
**Deliverables**
- ⬜ `M1.1-d1` `runtime/v1` gRPC proto + generated Go (CreatePod / StartContainer / PodStatus / Logs).
- ⬜ `M1.1-d2` image-manifest type + signature-policy enum (`adhoc-ok | require-signed | require-notarized`).
- ⬜ `M1.1-d3` `PodBox` spec (pod id, rootfs path, uid/gid, pod IP, profile inputs) per DESIGN §5a.

**Acceptance (exit gate)**
- ⬜ `M1.1-a1` builds; generated code committed + reproducible vs golden — *method: build*
- ⬜ `M1.1-a2` generated-type marshal/unmarshal round-trip is identity — *method: unit*
- ⬜ `M1.1-a3` zero `k3sm.io/*` imports (cycle check) — *method: build*

### M1.2 — Service-proxy + DNS-shim shared types ⬜
**Deliverables**
- ⬜ `M1.2-d1` cross-boundary types for the Service proxy (ServiceVIP, endpoint tuple) + DNS-shim config.

**Acceptance (exit gate)**
- ⬜ `M1.2-a1` builds pure-Go; consumed by a darwin-net compile-check — *method: build*

## M2 — gRPC daemon surface + resource/metrics types ⬜
Decomposed when M1 closes. Headline: extend `runtime/v1` for the **root daemon split** runtimed
exposes as a separate process; add resource-limit fields (mem bytes, QoS class) and Summary-API
metric types (`ri_phys_footprint` → `kubectl top`); any `runtime.k3sm.io` CRD types.

## M3 — MeshPeer CRD + NodeNetwork + mesh-enroll types ⬜
Headline: the `net.k3sm.io` `MeshPeer` CRD (node public key + endpoint + podCIDR/AllowedIPs),
`NodeNetwork`, and the mesh-enroll payload that rides `k3sm`'s join. Consumed by `darwin-net` (mesh)
and written by `k3sm` (join).

## M4 — API-stability freeze ⬜
Headline: version/freeze the v1 protos + CRDs for public consumption; ensure `go get k3sm.io/apis`
resolves via the vanity path (the GitHub-Pages `go-import` recipe, DESIGN §6a); doc completeness.

## Next
M1.1 — define the `runtime/v1` proto + image manifest + `PodBox` first; `runtimed:M1` and `k3sm:M1`
block on it.
