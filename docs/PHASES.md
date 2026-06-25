---
repo: apis
schema: phases/v1
current_phase: M2
updated: 2026-06-25
updated_by: agent

phases:
  - id: M0
    title: Walking skeleton (no apis work — the M0 provider was self-contained)
    status: done
    completed: 2026-06-24
    depends_on: []
    subphases: []

  - id: M1
    title: Runtime + image + Service/DNS contracts
    status: done
    completed: 2026-06-24
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
        status: done
        completed: 2026-06-24
        deliverables:
          - id: M1.2-d1
            done: true
            desc: shared Go types that cross the repo boundary for the userspace Service proxy (ServiceVIP, endpoint tuple) and the DNS-shim config — package netv1 (k3sm.io/apis/net/v1): ServiceVIP/ServicePort/Protocol, Endpoint, DNSConfig; plain Go (not proto), additive-only, zero k3sm.io/* imports
        acceptance:
          - id: M1.2-a1
            met: true
            check: builds pure-Go and a darwin-net compile-check consumes the types
            method: build

  - id: M2
    title: Pod-spec fidelity proto additions + gRPC daemon surface + resource/metrics types
    status: todo
    depends_on: []
    subphases:
      - id: M2.1
        title: pod-spec fidelity proto additions (additive only; field numbers STABLE)
        status: todo
        depends_on: []
        deliverables:
          - id: M2.1-d1
            done: false
            desc: "PodBox volumes — add a repeated Volume field carrying the pod-level volume sources stockkitty mounts: configMap, secret, emptyDir, downwardAPI, projected (the projected source includes serviceAccountToken, plus configMap/secret/downwardAPI projections). Allocate within PodBox's OWN reserved band 100..199 (a free number in that range); never reuse a reserved number. Volume payloads (configMap/secret data, downwardAPI field selectors) are materialized by runtimed:M2 inside the pod data-volume; the proto only carries the source spec."
          - id: M2.1-d2
            done: false
            desc: "Container volume_mounts — add a repeated VolumeMount field (name, mountPath, readOnly, subPath, subPathExpr) referencing PodBox.volumes by name. Allocate within Container's OWN reserved band 100..149; secrets/SA-token mounts get a read-only sub-scope runtimed-side."
          - id: M2.1-d3
            done: false
            desc: "Container probe specs — add liveness/readiness/startup Probe fields, each a Probe message with a handler union (httpGet {path/port/scheme/httpHeaders}, tcpSocket {port}, exec {command}) plus initialDelay/period/timeout/success/failure thresholds. Allocate within Container's 100..149 band. Probes are provider-served (k3sm:M2) and drive ContainerStatus.ready + Service endpoints; behavior-bearing across the M2 provider↔runtimed daemon split, so they ride the GetRuntimeInfoResponse.api_version handshake (consumer-first ordering: tolerant readers in dependents before the producer emits the new shape)."
          - id: M2.1-d4
            done: false
            desc: "Container securityContext — add a SecurityContext message (runAsUser, runAsGroup, fsGroup) on Container/PodBox as appropriate. Allocate within the owning message's reserved band. Behavior-bearing (net-new privilege-drop runtimed-side: setgid→initgroups→setuid before sandbox_apply, fsGroup chown root-side before the drop), so it crosses the daemon split and rides the api_version handshake."
          - id: M2.1-d5
            done: false
            desc: "Container env extensions — add envFrom (repeated EnvFromSource referencing configMapRef/secretRef with optional prefix) and extend the env entry with valueFrom.fieldRef (downward-API: spec.nodeName, status.podIP, metadata.name, etc.). The existing EnvVar message has name=1/value=2; add valueFrom additively (new field number) and add the EnvFromSource message. Provider translates downward-API field refs (k3sm:M2)."
          - id: M2.1-d6
            done: false
            desc: "imagePullSecret reference — add an image_pull_secrets field (repeated reference, e.g. secret name) to PodBox within its 100..199 band. Behavior-bearing (runtimed confines the credential to the pull client, never writing it to the pod dir; signature policy enforced before ad-hoc-sign), so it crosses the daemon split and rides the api_version handshake."
          - id: M2.1-d7
            done: false
            desc: "terminationGracePeriodSeconds — add the spec field on PodBox within its 100..199 band (the SOURCE value the provider derives DeletePodRequest.grace_period_seconds from; that RPC field already exists). Behavior-bearing (net-new runtimed SIGTERM → per-PID grace timer raced against the kqueue reaper → SIGKILL), rides the api_version handshake."
          - id: M2.1-d8
            done: false
            desc: "CRITICAL lossless-mirror pairing — add the matching ContainerStatus fields (volumeMounts, resources) so PodStatus stays a LOSSLESS mirror of corev1.PodStatus: every spec addition that surfaces in status MUST pair with its status field. Allocate within ContainerStatus's OWN reserved band 100..149 (its reserved comment already earmarks resources/allocatedResources/volumeMounts for M2). Without this pairing kubectl Pod state degrades crossing the runtime boundary."
        acceptance:
          - id: M2.1-a1
            met: false
            check: proto compiles (buf generate leaves no diff) and buf breaking against the baseline is CLEAN — additive-only, no field renumber, no reserved-number reuse
            method: unit
          - id: M2.1-a2
            met: false
            check: proto.Equal round-trip (marshal then unmarshal) holds for every new field on PodBox/Container/ContainerStatus (volumes, volumeMounts, probes, securityContext, envFrom, valueFrom.fieldRef, imagePullSecrets, terminationGracePeriodSeconds, ContainerStatus volumeMounts/resources)
            method: unit
          - id: M2.1-a3
            met: false
            check: the module still imports zero k3sm.io/* packages (cycle check) and builds CGO_ENABLED=0 standalone (GOWORK=off) + under go.work
            method: unit

  - id: M3
    title: Storage volume sources (PV/PVC) + MeshPeer CRD + NodeNetwork + mesh-enroll types
    status: todo
    depends_on: []
    subphases:
      - id: M3.1
        title: storage volume sources (NodePort needs NO apis change)
        status: todo
        depends_on: []
        deliverables:
          - id: M3.1-d1
            done: false
            desc: "PV/PVC volume source on PodBox — add a persistentVolumeClaim volume source (a StorageClass-named persistent volume mount) to the M2.1 Volume set, additively within PodBox's 100..199 band. NO new RPC: the existing CreatePod/PodBox mount mechanism (M2.1 volumes + Container.volume_mounts) carries it; runtimed:M3 binds it to a stable per-PVC dir on the same APFS volume as /var/lib/k3sm (empty-create; clonefile only seeds from a template; lifecycle decoupled from pod-dir teardown)."
          - id: M3.1-d2
            done: false
            desc: "StorageClass / provisioner contract types — add any cross-repo type the APFS local-path provisioner controller (k3sm:M3) and runtimed:M3 need to agree on (e.g. a StorageClass name → provisioner parameters shape), only if the upstream storage.k8s.io objects do not already suffice. Prefer reusing upstream objects; add a k3sm-specific type only where a contract genuinely crosses the repo boundary."
          - id: M3.1-d3
            done: false
            desc: "NodePort needs NO apis change — ServicePort.NodePort ALREADY EXISTS in net/v1 (k3sm.io/apis/net/v1, validated, tested). M3 NodePort work is darwin-net proxy (bind *:port, TCP; UDP relay deferred) + k3sm wiring ONLY. Do NOT re-add, rename, or renumber the field; this deliverable is a no-op for apis, recorded so dependents' depends_on resolve and no one re-introduces the field."
        acceptance:
          - id: M3.1-a1
            met: false
            check: additive-only — buf breaking clean (proto) and net/v1 ServicePort.NodePort unchanged; the PV/PVC source compiles and round-trips
            method: unit
          - id: M3.1-a2
            met: false
            check: proto.Equal round-trip holds for the new persistentVolumeClaim volume source and any StorageClass/provisioner contract type; tests are table-driven
            method: unit

  - id: M4
    title: API-stability freeze + vanity-import resolves
    status: todo
    depends_on: []
    subphases: []

  - id: M5
    title: vm RuntimeClass handler mapping (Linux-image micro-VM backend)
    status: todo
    depends_on: []
    subphases:
      - id: M5.1
        title: vm RuntimeClass handler-config mapping (reuse SANDBOX_BACKEND_VM; do not fork upstream RuntimeClass)
        status: todo
        depends_on: []
        deliverables:
          - id: M5.1-d1
            done: false
            desc: "runtime.k3sm.io handler-config type — a config type that maps the value of the upstream node.k8s.io/v1 RuntimeClass `handler`/`runtimeClassName` (e.g. \"vm\") to a SANDBOX_BACKEND. REUSE the existing SANDBOX_BACKEND_VM enum value (runtime/v1, = 4); do NOT fork, redefine, or vendor the upstream RuntimeClass API. k3sm consumes the standard upstream RuntimeClass object AFTER admission/scheduling and looks up the backend via this handler-config; the type only carries the name→backend mapping (+ any backend params the VZ path needs)."
        acceptance:
          - id: M5.1-a1
            met: false
            check: additive-only and builds CGO_ENABLED=0 standalone + under go.work; the module still imports zero k3sm.io/* packages and does not vendor or redefine the upstream node.k8s.io RuntimeClass type
            method: unit
          - id: M5.1-a2
            met: false
            check: the handler-name → SANDBOX_BACKEND mapping is table-tested (\"vm\" → SANDBOX_BACKEND_VM; unknown handler → a defined fallback/error), -race clean
            method: unit
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

## M1 — Runtime + image + Service/DNS contracts ✅

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

### M1.2 — Service-proxy + DNS-shim shared types ✅
**Deliverables**
- ✅ `M1.2-d1` cross-boundary types for the Service proxy + DNS-shim config — pure-Go package `netv1` (`k3sm.io/apis/net/v1`): `ServiceVIP`/`ServicePort`/`Protocol` (TCP/UDP) for the userspace proxy, `Endpoint` (IP/Port/Ready) the proxy load-balances to, `DNSConfig` (CoreDNS VIP, cluster domain, search domains, ndots) the `getaddrinfo` shim consumes. Plain Go structs (NOT proto), additive-only, camelCase JSON tags, small `Validate`/`WithDefaults` helpers, zero `k3sm.io/*` imports.

**Acceptance (exit gate)**
- ✅ `M1.2-a1` builds pure-Go (`CGO_ENABLED=0`, standalone `GOWORK=off` + under `go.work`); table-driven `-race` tests (construction, validation, JSON round-trip/field-name pins); ready for the `darwin-net` proxy + shim compile-check — *method: build*

## M2 — Pod-spec fidelity proto additions + gRPC daemon surface + resource/metrics types ⬜
Decomposed now that M1 has landed. Headline: extend `runtime/v1` for the **root daemon split**
runtimed exposes as a separate process (resource-limit fields, Summary-API metric types,
`ri_phys_footprint` → `kubectl top`) **and** raise pod-spec fidelity to what the `stockkitty`
reference workload exercises (`../../docs/stockkitty-readiness.md`). All additions are **additive-only**;
field numbers are **STABLE** (`buf breaking` WIRE_JSON gate); each addition lands in its message's **own**
reserved band — `PodBox` reserves `100..199`, the sibling messages (`Container`, `ContainerStatus`,
`SandboxProfile`, …) reserve `100..149` — and **never** reuses a reserved number.

### M2.1 — pod-spec fidelity proto additions ⬜
**Deliverables**
- ⬜ `M2.1-d1` `PodBox.volumes` — a repeated `Volume` carrying the sources stockkitty mounts: `configMap`, `secret`, `emptyDir`, `downwardAPI`, `projected` (the projected source includes `serviceAccountToken` plus configMap/secret/downwardAPI projections). Allocated in `PodBox`'s own `100..199` band. The proto carries only the source spec; `runtimed:M2` materializes payloads inside the pod data-volume.
- ⬜ `M2.1-d2` `Container.volume_mounts` — a repeated `VolumeMount` (name, mountPath, readOnly, subPath, subPathExpr) referencing `PodBox.volumes` by name; allocated in `Container`'s own `100..149` band. Secrets / SA-token mounts get a read-only sub-scope runtimed-side.
- ⬜ `M2.1-d3` `Container` probes — `liveness`/`readiness`/`startup` `Probe` fields, each with a handler union (`httpGet`/`tcpSocket`/`exec`) + initialDelay/period/timeout/success/failure thresholds; in `Container`'s `100..149` band. Probes are **provider-served** (`k3sm:M2`) and drive `ContainerStatus.ready` + Service endpoints. **Behavior-bearing** across the M2 provider↔runtimed daemon split → rides the `GetRuntimeInfoResponse.api_version` handshake (**consumer-first** ordering: dependents ship tolerant readers before the producer emits the new shape).
- ⬜ `M2.1-d4` `Container.securityContext` — a `SecurityContext` (`runAsUser`/`runAsGroup`/`fsGroup`) on `Container`/`PodBox`. **Behavior-bearing** (net-new runtimed privilege-drop: `setgid→initgroups→setuid` **before** `sandbox_apply`; `fsGroup` chown root-side **before** the drop) → rides the `api_version` handshake.
- ⬜ `M2.1-d5` `Container` env extensions — `envFrom` (repeated `EnvFromSource` → `configMapRef`/`secretRef` + optional prefix) and `env[].valueFrom.fieldRef` (downward-API: `spec.nodeName`/`status.podIP`/`metadata.name`, …). `EnvVar` keeps `name=1`/`value=2`; `valueFrom` is added additively at a new field number. Provider translates the field refs (`k3sm:M2`).
- ⬜ `M2.1-d6` `PodBox.image_pull_secrets` — a private-registry credential reference (in `PodBox`'s `100..199` band). **Behavior-bearing** (runtimed confines the credential to the pull client, never writing it to the pod dir; signature policy enforced **before** ad-hoc-sign) → rides the `api_version` handshake.
- ⬜ `M2.1-d7` `PodBox.terminationGracePeriodSeconds` — the spec field (in `PodBox`'s `100..199` band) the provider derives `DeletePodRequest.grace_period_seconds` from (that RPC field already exists). **Behavior-bearing** (net-new runtimed SIGTERM → per-PID grace timer raced against the kqueue reaper → SIGKILL) → rides the `api_version` handshake.
- ⬜ `M2.1-d8` **CRITICAL lossless-mirror pairing** — the matching `ContainerStatus` fields (`volumeMounts`, `resources`) so `PodStatus` stays a **lossless mirror** of `corev1.PodStatus`: every spec addition that surfaces in status MUST pair with its status field. Allocated in `ContainerStatus`'s own `100..149` band (its `reserved` comment already earmarks `resources, allocatedResources, volumeMounts` for M2). Without the pairing, kubectl Pod state degrades crossing the runtime boundary.

**Acceptance (exit gate)**
- ⬜ `M2.1-a1` proto compiles (`buf generate` no diff) and `buf breaking` is **clean** — additive-only, no renumber, no reserved-number reuse — *method: unit*
- ⬜ `M2.1-a2` `proto.Equal` round-trip holds for every new field on `PodBox`/`Container`/`ContainerStatus` — *method: unit*
- ⬜ `M2.1-a3` zero `k3sm.io/*` imports (cycle check); builds `CGO_ENABLED=0` standalone (`GOWORK=off`) + under `go.work` — *method: unit*

## M3 — Storage volume sources (PV/PVC) + MeshPeer CRD + NodeNetwork + mesh-enroll types ⬜
Headline: persistent storage for the reference workload (Postgres / compile-artifacts PVCs) **plus** the
`net.k3sm.io` `MeshPeer` CRD (node public key + endpoint + podCIDR/AllowedIPs), `NodeNetwork`, and the
mesh-enroll payload that rides `k3sm`'s join (consumed by `darwin-net` mesh, written by `k3sm` join).

### M3.1 — storage volume sources (NodePort needs NO `apis` change) ⬜
**Deliverables**
- ⬜ `M3.1-d1` PV/PVC volume source on `PodBox` — a `persistentVolumeClaim` source added additively to the M2.1 `Volume` set (in `PodBox`'s `100..199` band). **No new RPC**: the existing `CreatePod`/`PodBox` mount mechanism (M2.1 volumes + `Container.volume_mounts`) carries it. `runtimed:M3` binds it to a stable per-PVC dir on the **same APFS volume** as `/var/lib/k3sm` (empty-create; `clonefile` only *seeds* from a template; lifecycle decoupled from pod-dir teardown).
- ⬜ `M3.1-d2` StorageClass / provisioner contract types — any cross-repo type the APFS local-path provisioner controller (`k3sm:M3`) and `runtimed:M3` must agree on, **only if** the upstream `storage.k8s.io` objects do not already suffice. Prefer reusing upstream objects; add a k3sm-specific type only where a contract genuinely crosses the repo boundary.
- ⬜ `M3.1-d3` **NodePort needs NO `apis` change** — `ServicePort.NodePort` **already exists** in `net/v1` (`k3sm.io/apis/net/v1`, validated, tested). M3 NodePort work is `darwin-net` proxy (bind `*:port`, TCP; UDP relay deferred) + `k3sm` wiring **only**. Do **not** re-add, rename, or renumber the field; this is a no-op for `apis`, recorded so dependents' `depends_on` resolve and no one re-introduces the field.

**Acceptance (exit gate)**
- ⬜ `M3.1-a1` additive-only — `buf breaking` clean and `net/v1 ServicePort.NodePort` unchanged; the PV/PVC source compiles and round-trips — *method: unit*
- ⬜ `M3.1-a2` `proto.Equal` round-trip holds for the new `persistentVolumeClaim` source and any StorageClass/provisioner contract type; tests are table-driven — *method: unit*

## M4 — API-stability freeze ⬜
Headline: version/freeze the v1 protos + CRDs for public consumption; ensure `go get k3sm.io/apis`
resolves via the vanity path (the GitHub-Pages `go-import` recipe, DESIGN §6a); doc completeness.

## M5 — vm RuntimeClass handler mapping (Linux-image micro-VM backend) ⬜
Newly-committed milestone (`../../docs/stockkitty-readiness.md`): the `vm` RuntimeClass that runs
Linux-only images (pgvector / `nats:alpine` / the C++ ELF) in a Virtualization.framework micro-VM behind
the existing swappable `sandbox.Backend` seam. `apis` is **Wave 1** — its `M5.1` is the contract the
runtimed VZ backend (`runtimed:M5`) and the guest-side networking (`darwin-net:M5`) build against.

### M5.1 — vm RuntimeClass handler-config mapping ⬜
**Deliverables**
- ⬜ `M5.1-d1` a `runtime.k3sm.io` handler-config type mapping the upstream `node.k8s.io/v1` `RuntimeClass` handler value (e.g. `vm`) → a `SANDBOX_BACKEND`. **Reuse the existing `SANDBOX_BACKEND_VM`** enum value (`runtime/v1`, `= 4`); do **not** fork, redefine, or vendor the upstream `RuntimeClass` API. `k3sm` consumes the standard upstream `RuntimeClass` object **after** admission/scheduling and looks up the backend via this handler-config; the type carries only the name→backend mapping (+ any VZ-path backend params).

**Acceptance (exit gate)**
- ⬜ `M5.1-a1` additive-only; builds `CGO_ENABLED=0` standalone + under `go.work`; zero `k3sm.io/*` imports; does **not** vendor or redefine the upstream `node.k8s.io` `RuntimeClass` type — *method: unit*
- ⬜ `M5.1-a2` the handler-name → `SANDBOX_BACKEND` mapping is table-tested (`vm` → `SANDBOX_BACKEND_VM`; unknown handler → a defined fallback/error), `-race` clean — *method: unit*

## Dependents of these `apis` sub-phases
`apis` is **Wave 1**; downstream milestones `depends_on` these ids:
- `runtimed:M2` + `k3sm:M2` + `darwin-net:M2` ← `apis:M2.1` (the pod-spec fidelity fields; behavior-bearing fields cross the provider↔runtimed split via the `api_version` handshake).
- `runtimed:M3` + `k3sm:M3` ← `apis:M3.1` (PV/PVC source). `darwin-net:M3` NodePort needs **no** `apis` edge (the field already exists).
- `runtimed:M5` + `darwin-net:M5` ← `apis:M5.1` (the `vm` handler→backend mapping).

## Next
M1 is closed. `runtime/v1` (`service Runtime`, `PodBox`, `PodStatus`, `ImageManifest`,
`SignaturePolicy`) is the contract `runtimed:M1` and `k3sm:M1` implement against, and `net/v1`
(`ServiceVIP`, `Endpoint`, `DNSConfig`) is the cross-boundary type set `darwin-net:M1` consume. **M2 is
the active milestone**: its `M2.1` raises pod-spec fidelity (volumes, volumeMounts, probes,
securityContext, envFrom + `valueFrom.fieldRef`, imagePullSecret, `terminationGracePeriodSeconds`, and
the paired `ContainerStatus` fields) to what `stockkitty` exercises, alongside the daemon-split
resource/metrics surface — all additive, field numbers stable, each in its message's own reserved band.
