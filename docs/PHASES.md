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
        title: runtime/v1 daemon-ready gRPC contract + image manifest + PodBox (buf-generated)
        status: done
        completed: 2026-06-24
        deliverables:
          - id: M1.1-d1
            done: true
            desc: buf toolchain — buf.yaml + buf.gen.yaml with PINNED protoc-gen-go / protoc-gen-go-grpc versions; a hack/gen.sh (buf generate) + a checked-in buf-breaking baseline; wire `buf generate && git diff --exit-code` and `buf breaking` into apis/hack/ci.sh
          - id: M1.1-d2
            done: true
            desc: runtime/v1 proto — DAEMON-READY full Pod lifecycle (pod-granular CreatePod / DeletePod / UpdatePod), server-streaming PodStatus (watch) and Logs, Exec / Attach / PortForward, and GetRuntimeInfo (version+health for the M2 IPC compat window); reserved field/RPC ranges for the M2 resources/metrics extension
          - id: M1.1-d3
            done: true
            desc: PodStatus message is a LOSSLESS mirror of corev1.PodStatus (phase; ContainerStatuses[].State {Running/Terminated/Waiting} with ExitCode/Reason/StartedAt/FinishedAt; Ready/Started; all Conditions; PodIP(s)/HostIP/StartTime) so kubectl Pod state never degrades
          - id: M1.1-d4
            done: true
            desc: PodBox as a proto message (pod id, rootfs path, uid/gid, pod IP, profile inputs per DESIGN §5a) carried by CreatePod — single definition, no parallel hand struct; plus the OCI image-manifest type and the signature-policy enum reserving 0 = SIGNATURE_POLICY_UNSPECIFIED (fail-closed; adhoc-ok/require-signed/require-notarized are non-zero)
          - id: M1.1-d5
            done: true
            desc: apis carries its OWN go.mod resolution for the google.golang.org/genproto monolith-vs-split ambiguity (a documented replace, or proven-clean pinned grpc/protobuf versions) so the module builds standalone outside go.work
        acceptance:
          - id: M1.1-a1
            met: true
            check: CGO_ENABLED=0 go build ./... and go test ./... pass both standalone (GOWORK=off) and under go.work; generated code is committed
            method: build
          - id: M1.1-a2
            met: true
            check: buf generate leaves no diff (git diff --exit-code) and buf breaking against the baseline passes (the no-renumber / wire-stability gate), both wired into hack/ci.sh
            method: build
          - id: M1.1-a3
            met: true
            check: proto.Equal round-trip (marshal then unmarshal) holds for every generated message incl. PodStatus and PodBox; any hand-written Go type uses a testdata golden
            method: unit
          - id: M1.1-a4
            met: true
            check: the module imports zero k3sm.io/* packages (cycle check); the signature-policy zero value is UNSPECIFIED and decodes fail-closed
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

## M1 — Runtime + image + Service/DNS contracts 🟡

### M1.1 — runtime proto + image manifest + PodBox ✅
**Deliverables**
- ✅ `M1.1-d1` buf toolchain (`buf.yaml`/`buf.gen.yaml`, pinned `protoc-gen-go` v1.36.11 + `protoc-gen-go-grpc` v1.5.1), `hack/gen.sh`, checked-in breaking baseline (`buf/baseline.binpb`); generate-diff + breaking wired into `hack/ci.sh`.
- ✅ `M1.1-d2` `runtime/v1` daemon-ready `service Runtime`: CreatePod / DeletePod / UpdatePod, server-streaming WatchPodStatus + GetLogs, Exec / Attach / PortForward, GetRuntimeInfo; reserved field ranges per message for the M2 resources/metrics extension.
- ✅ `M1.1-d3` `PodStatus` is a lossless mirror of `corev1.PodStatus` (phase, all conditions, container states Running/Terminated/Waiting, ready/started, podIP(s)/hostIP/startTime); structured failure via `google.rpc.Status` + `FailureReason` enum.
- ✅ `M1.1-d4` `PodBox` proto message (one definition) carried by CreatePod; OCI `ImageManifest`/`Descriptor`; `SignaturePolicy` enum with `SIGNATURE_POLICY_UNSPECIFIED = 0` (fail-closed).
- ✅ `M1.1-d5` `apis/go.mod` resolves standalone — pinned grpc v1.81.1 / protobuf v1.36.11 (proven `GOWORK=off` build) plus the documented `genproto` replace forward-guard.

**Acceptance (exit gate)**
- ✅ `M1.1-a1` `CGO_ENABLED=0 go build`/`test` pass standalone (`GOWORK=off`) and under `go.work`; generated code committed — *method: build*
- ✅ `M1.1-a2` `buf generate` leaves no diff; `buf breaking` passes vs baseline; both in `hack/ci.sh` — *method: build*
- ✅ `M1.1-a3` `proto.Equal` round-trip holds for every generated message (incl. `PodStatus`, `PodBox`) — *method: unit*
- ✅ `M1.1-a4` zero `k3sm.io/*` imports; `SignaturePolicy` zero value is UNSPECIFIED (fail-closed) — *method: build*

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
M1.1 is closed: `runtime/v1` (`service Runtime`, `PodBox`, `PodStatus`, `ImageManifest`,
`SignaturePolicy`) is the contract `runtimed:M1` and `k3sm:M1` now implement against. Next is
**M1.2** — the Service-proxy + DNS-shim shared Go types for `darwin-net`.
