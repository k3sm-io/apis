---
repo: apis
schema: phases/v1
current_phase: M7
updated: 2026-08-31
updated_by: roadmap-validation-sweep

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
    status: done
    completed: 2026-06-25
    depends_on: []
    subphases:
      - id: M2.1
        title: pod-spec fidelity proto additions (additive only; field numbers STABLE)
        status: done
        completed: 2026-06-25
        depends_on: []
        notes: >-
          The M2 provider↔runtimed split is a SAME-BINARY, same-node hard cut:
          the provider and its node's runtimed are the same k3sm build, restarted
          together via `launchctl kickstart`. Behavior-bearing fields therefore
          need NO version negotiation — there is no independent-upgrade skew
          window, and GetRuntimeInfoResponse.api_version is an inert constant
          (no negotiation surface is built on it). Allocation rule: M2.1 fields
          take the next FREE sequential numbers below 100 on each message
          (PodBox 15..18; Container 9..15; EnvVar 3; ContainerStatus 11..12);
          the reserved 100..199 (PodBox) / 100..149 (siblings) bands are
          UNTOUCHED, earmarked for M2.2 (resource/metrics) so the two never
          collide.
        deliverables:
          - id: M2.1-d1
            done: true
            desc: "PodBox.volumes (field 15) — a repeated Volume carrying the pod-level volume sources stockkitty mounts: configMap, secret, emptyDir, downwardAPI, projected. ProjectedVolumeSource includes ServiceAccountTokenProjection{audience, expirationSeconds, path} (bound token — the in-pod-kubectl path) plus configMap/secret/downwardAPI projections. The source union is modeled as optional message fields (the ContainerState pointer-union precedent), not a oneof. Volume payloads are materialized by runtimed:M2 inside the pod data-volume; the proto carries only the source spec."
          - id: M2.1-d2
            done: true
            desc: "Container.volume_mounts (field 9) — a repeated VolumeMount{name, mountPath, readOnly, subPath} referencing PodBox.volumes by name. Secrets/SA-token mounts get a read-only sub-scope runtimed-side."
          - id: M2.1-d3
            done: true
            desc: "Container probe specs (fields 11/12/13 liveness/readiness/startup) — each a Probe{initialDelaySeconds, periodSeconds, timeoutSeconds, successThreshold, failureThreshold} with the handler modeled as THREE optional message fields (http_get/tcp_socket/exec, NOT a oneof — matching ContainerState; 'exactly one set' documented). HTTPGetAction{path, port IntOrString, scheme, host, httpHeaders}; TCPSocketAction{port IntOrString, host}; ExecAction{command}. Adds Container.ports (field 10) — a repeated ContainerPort{name, containerPort, protocol} named-port table so named probe ports + named Service targetPorts resolve, and IntOrString{intVal, strVal} mirroring intstr.IntOrString. Probes are provider-served (k3sm:M2) and drive ContainerStatus.ready + Service endpoints."
          - id: M2.1-d4
            done: true
            desc: "securityContext — Container.security_context (field 14) is a SecurityContext{runAsUser int64, runAsGroup int64, runAsNonRoot bool} (container scope); fsGroup is NOT here. PodBox.pod_security_context (field 16) is a PodSecurityContext{fsGroup int64, runAsUser int64, runAsGroup int64} — fsGroup lives at pod scope only. Net-new privilege-drop runtimed-side (setgid→initgroups→setuid before sandbox_apply; fsGroup chown root-side before the drop)."
          - id: M2.1-d5
            done: true
            desc: "Container env extensions — Container.env_from (field 15) is a repeated EnvFromSource{prefix, configMapRef ConfigMapEnvSource, secretRef SecretEnvSource}; EnvVar gains value_from (field 3) = EnvVarSource{fieldRef ObjectFieldSelector, configMapKeyRef ConfigMapKeySelector, secretKeyRef SecretKeySelector} (the corev1 union; fieldRef = ObjectFieldSelector{apiVersion, fieldPath} covers spec.nodeName/status.podIP/metadata.name). EnvVar keeps name=1/value=2; value-vs-valueFrom mutual exclusion documented. Provider translates downward-API field refs (k3sm:M2)."
          - id: M2.1-d6
            done: true
            desc: "PodBox.image_pull_secrets (field 18) — a repeated LocalObjectReference{name} of Secrets carrying private-registry credentials. runtimed confines the credential to the pull client (never written to the pod dir); signature policy enforced before ad-hoc-sign."
          - id: M2.1-d7
            done: true
            desc: "PodBox.termination_grace_period_seconds (field 17, int64) — the SOURCE value the provider derives DeletePodRequest.grace_period_seconds from (that RPC field already exists). Net-new runtimed SIGTERM → per-PID grace timer raced against the kqueue reaper → SIGKILL."
          - id: M2.1-d8
            done: true
            desc: "CRITICAL lossless-mirror pairing — ContainerStatus.volume_mounts (field 11) = repeated VolumeMountStatus{name, mountPath, readOnly} and ContainerStatus.user (field 12) = ContainerUser{linux LinuxContainerUser}, LinuxContainerUser{uid int64, gid int64, supplementalGroups repeated int64} (the EFFECTIVE uid/gid/groups the privilege-drop produces). Only the corev1 status surface for M2.1 spec fields is added; resources/allocatedResources are M2.2 (reserved band). Without this pairing kubectl Pod state degrades crossing the runtime boundary."
        acceptance:
          - id: M2.1-a1
            met: true
            check: proto compiles (buf generate is reproducible — no diff) and buf breaking (WIRE_JSON) against buf/baseline.binpb is CLEAN — additive-only, no field renumber, no reserved-number reuse (reserved bands untouched)
            method: unit
          - id: M2.1-a2
            met: true
            check: proto.Equal round-trip (marshal then unmarshal) holds for every new message (one fully-populated case each — every Volume source incl. serviceAccountToken; each Probe handler; SecurityContext/PodSecurityContext; EnvFromSource; EnvVarSource/fieldRef; ContainerPort; ContainerStatus volumeMounts+user) plus full-Container/full-PodBox/full-ContainerStatus cases; -race clean
            method: unit
          - id: M2.1-a3
            met: true
            check: the module still imports zero k3sm.io/* packages (cycle check) and builds CGO_ENABLED=0 standalone (GOWORK=off) + under go.work
            method: unit
      - id: M2.2
        title: resource-limit + metrics types (reserved 100..199 / 100..149 bands)
        status: done
        completed: 2026-06-25
        depends_on: []
        deliverables:
          - id: M2.2-d1
            done: true
            desc: "PodBox resource-limit fields — memory bytes, QoS class, and RLIMITs, allocated within PodBox's OWN reserved band 100..199 (never the low M2.1 numbers). These are the inputs runtimed:M2.2 enforces for OOMKilled (proc_pid_rusage; ri_phys_footprint ≠ RSS) and best-effort CPU QoS (not CFS millicores)."
          - id: M2.2-d2
            done: true
            desc: "ContainerStatus.{resources, allocatedResources} — the status mirror of the M2.2 resource-limit fields, allocated within ContainerStatus's OWN reserved band 100..149 (its reserved comment earmarks resources/allocatedResources). Keeps PodStatus a lossless mirror of corev1 once resources land."
          - id: M2.2-d3
            done: true
            desc: "Summary-API / pod-stats message(s) for kubectl top — the metric snapshot type(s) (per-pod/per-container CPU + memory working-set) the provider serves the metrics path from. New message(s); does NOT consume the reserved bands of the lifecycle messages."
        acceptance:
          - id: M2.2-a1
            met: true
            check: additive-only — buf breaking (WIRE_JSON) clean; resource/metrics fields land in the reserved 100..199 (PodBox) / 100..149 (sibling) bands so they never collide with the M2.1 low-number fields
            method: unit
          - id: M2.2-a2
            met: true
            check: proto.Equal round-trip holds for the new resource-limit fields and the Summary-API/pod-stats message(s); table-driven, -race clean
            method: unit

  - id: M3
    title: Storage volume sources (PV/PVC) + MeshPeer CRD + NodeNetwork + mesh-enroll types
    status: done
    completed: 2026-06-25
    depends_on: []
    notes: >-
      Wave 1 of M3 is M3.1 (storage) + M3.2 (mesh contracts); both landed
      additive-only (stable field numbers, buf breaking WIRE_JSON clean). The
      M3 re-plan split mesh OUT of M3.1 into a new M3.2
      because darwin-net:M3 (wireguard mesh) and k3sm:M3 (join/mesh-enroll) must
      depend on the mesh contracts, NOT on the storage source. M3.2 introduces
      apis's FIRST k8s.io/apimachinery dependency (pinned v0.35.0, lockstep with
      k3sm/go.mod) for the served MeshPeer CRD; the module still imports zero
      k3sm.io/* packages and builds CGO_ENABLED=0.
    subphases:
      - id: M3.1
        title: storage volume sources (NodePort needs NO apis change)
        status: done
        completed: 2026-06-25
        depends_on: []
        deliverables:
          - id: M3.1-d1
            done: true
            desc: "PV/PVC volume source on PodBox — persistentVolumeClaim added as the durable source to the M2.1 Volume union as PersistentVolumeClaimVolumeSource{claim_name, read_only} on Volume.persistent_volume_claim=7 (the next FREE sequential source number; the M2.1 sources took 2..6 — Volume has no reserved band, so this is a plain additive append, not a reserved-band allocation). NO new RPC: the existing CreatePod/PodBox mount mechanism (M2.1 volumes + Container.volume_mounts) carries it; runtimed:M3 binds it to a stable per-PVC dir on the same APFS volume as /var/lib/k3sm (empty-create; clonefile only seeds from a template; lifecycle decoupled from pod-dir teardown). Mirrors corev1.Volume.PersistentVolumeClaim."
          - id: M3.1-d2
            done: true
            desc: "StorageClass / provisioner contract — added the plain-Go package k3sm.io/apis/storage/v1 (storagev1) because the upstream storage.k8s.io StorageClass / core/v1 PersistentVolume objects remain the served API surface (NOT vendored or redefined), but a small agreement DOES cross the k3sm-provisioner ↔ runtimed-binder boundary: LocalPathClass{name, provisioner=k3sm.io/local-path, basePath=/var/lib/k3sm/storage, reclaimPolicy=Retain-only, volumeBindingMode=WaitForFirstConsumer-only} + the stable per-PVC DataDir(namespace, claimName) derivation BOTH repos compute (runtimed resolves it from the PodBox alone, never needing the PV UID) + PVName(pvcUID) (the idempotency key) + NodeTopology{key=kubernetes.io/hostname, nodeName} (the PV node-affinity that pins a local PV — and its StatefulSet pod — to its owning Mac). Plain Go (net/v1 precedent), additive-only, camelCase JSON, Validate/WithDefaults, zero k3sm.io/* imports."
          - id: M3.1-d3
            done: true
            desc: "NodePort needs NO apis change — ServicePort.NodePort ALREADY EXISTS in net/v1 (k3sm.io/apis/net/v1, validated, tested). M3 NodePort work is darwin-net proxy (bind *:port, TCP; UDP relay deferred) + k3sm wiring ONLY. Confirmed no-op: the field was NOT re-added, renamed, or renumbered; TestNodePortUnchangedM3 pins its presence + JSON name + round-trip so no one re-introduces it. Recorded so dependents' depends_on resolve."
        acceptance:
          - id: M3.1-a1
            met: true
            check: additive-only — buf breaking (WIRE_JSON) clean vs buf/baseline.binpb (proto) and net/v1 ServicePort.NodePort unchanged (git diff main confirms no existing field number/name changed); the PV/PVC source compiles and round-trips, builds CGO_ENABLED=0 standalone + under go.work
            method: unit
          - id: M3.1-a2
            met: true
            check: proto.Equal round-trip holds for PersistentVolumeClaimVolumeSource + Volume.persistent_volume_claim + a PVC-backed PodBox (runtime/v1); Go JSON round-trip + Validate/WithDefaults hold for the storagev1 LocalPathClass + NodeTopology contract incl. DataDir/PVName derivation; all table-driven, -race clean
            method: unit
      - id: M3.2
        title: mesh contracts — MeshPeer CRD + mesh-enroll payloads (the re-plan's #1 architectural fix)
        status: done
        completed: 2026-06-25
        depends_on: []
        notes: >-
          NEW sub-phase from the M3 re-plan: M3.1 produced no mesh type, but
          darwin-net:M3 (wireguard mesh) and k3sm:M3 (join) depend on one. The
          MeshPeer CRD carries node PUBLIC keys only (private keys never leave
          the node — DESIGN §5b). The Go type lives in the existing net/v1
          package (GVK net.k3sm.io/v1) and pulls apis's FIRST
          k8s.io/apimachinery dep (pinned v0.35.0, lockstep with k3sm).
        deliverables:
          - id: M3.2-d1
            done: true
            desc: "MeshPeer CRD (net.k3sm.io/v1) — a real served/watchable, kine-stored Kubernetes custom resource in net/v1 (mesh.go): MeshPeer/MeshPeerList embedding metav1.TypeMeta+ObjectMeta (apis's FIRST k8s.io/apimachinery dependency — pinned v0.35.0 in go.mod, lockstep with k3sm), hand-written DeepCopy*/DeepCopyObject (no code-gen in apis), and a SchemeBuilder/AddToScheme/Resource registration. Cluster-scoped (one per node, named for the node). MeshPeerSpec carries schemaVersion (the wireguard-encoding evolution seam INSIDE the v1 GVK) + nodeName + publicKey + endpoint + podCIDR + symmetric allowedIPs (must equal podCIDR) + meshIP + persistentKeepaliveSeconds; MeshPeerStatus carries lastHandshakeTime/reachable/observedSchemaVersion (status subresource). Authored CRD manifest config/crd/net.k3sm.io_meshpeers.yaml with served+stored discipline (single v1 served+stored, additive-only)."
          - id: M3.2-d2
            done: true
            desc: "mesh-enroll / join payloads — plain Go structs (NOT a CRD, NOT proto) the bootstrap join HTTP exchange marshals (k3sm's join client + supervisor): MeshEnrollRequest{schemaVersion, nodeName, publicKey, endpoint, podCIDR} + MeshEnrollResponse{schemaVersion, nodeName, podCIDR, meshIP, peers []MeshPeerSpec}. Version-stamped from day one (MeshEnrollSchemaVersion=1) so an M4+ node-by-node roll has a compatibility seam; the peer snapshot reuses the canonical MeshPeerSpec. Validate/WithDefaults; zero k3sm.io/* imports."
          - id: M3.2-d3
            done: true
            desc: "NodeNetwork — explicit NO-OP for apis (recorded, like NodePort). No concrete cross-repo NodeNetwork type emerged: the M3 networking work (per-node CoreDNS + infra-VIP exemption in darwin-net:M3.3, and the node-local kubernetes endpoint rewrite in k3sm:M3.3) is darwin-net-internal + a k3sm-owned controller, neither of which needs a new apis type. Do NOT invent one; recorded so dependents' depends_on resolve."
        acceptance:
          - id: M3.2-a1
            met: true
            check: the MeshPeer CRD type round-trips losslessly (JSON, byte-stable incl. the status metav1.Time + the schemaVersion stamp) and DeepCopy/DeepCopyObject produce independent objects; a table test asserts the registered GVK is net.k3sm.io/v1 (MeshPeer + MeshPeerList) via a runtime.Scheme; builds CGO_ENABLED=0 standalone (GOWORK=off) + under go.work; the module still imports zero k3sm.io/* packages (k8s.io/apimachinery is k8s.io + pure Go)
            method: unit
          - id: M3.2-a2
            met: true
            check: the mesh-enroll payloads (MeshEnrollRequest/Response) round-trip (JSON) and carry a non-zero SchemaVersion (== MeshEnrollSchemaVersion); MeshPeerSpec + enroll Validate/WithDefaults are table-tested (version-stamp + required fields); -race clean
            method: unit

  - id: M4
    title: API-stability freeze + vanity-import resolves
    status: todo
    depends_on: []
    subphases: []

  - id: M5
    title: vm RuntimeClass handler mapping (Linux-image micro-VM backend)
    status: done
    completed: 2026-06-27
    depends_on: []
    subphases:
      - id: M5.1
        title: vm RuntimeClass handler-config mapping (reuse SANDBOX_BACKEND_VM; do not fork upstream RuntimeClass)
        status: done
        depends_on: []
        deliverables:
          - id: M5.1-d1
            done: true
            desc: "runtime.k3sm.io handler-config type — a config type that maps the value of the upstream node.k8s.io/v1 RuntimeClass `handler`/`runtimeClassName` (e.g. \"vm\") to a SANDBOX_BACKEND. REUSE the existing SANDBOX_BACKEND_VM enum value (runtime/v1, = 4); do NOT fork, redefine, or vendor the upstream RuntimeClass API. k3sm consumes the standard upstream RuntimeClass object AFTER admission/scheduling and looks up the backend via this handler-config; the type only carries the name→backend mapping (+ any backend params the VZ path needs)."
        acceptance:
          - id: M5.1-a1
            met: true
            check: additive-only and builds CGO_ENABLED=0 standalone + under go.work; the module still imports zero k3sm.io/* packages and does not vendor or redefine the upstream node.k8s.io RuntimeClass type
            method: unit
          - id: M5.1-a2
            met: true
            check: the handler-name → SANDBOX_BACKEND mapping is table-tested (\"vm\" → SANDBOX_BACKEND_VM; unknown handler → a defined fallback/error), -race clean
            method: unit

  - id: M7
    title: Public open-source launch — release engineering (apis slice — CI + README)
    status: todo
    depends_on: []
    notes: >-
      apis is a small M7 participant: a public GitHub Actions CI workflow calling
      the existing hack/ci.sh, plus HOSTING the shared k3smtest.SkipUnless helper +
      its owned capability taxonomy (apis is the only DAG-legal home a helper
      runtimed, darwin-net, AND k3sm all import; a leaf copy would drift or force a
      sideways import). The release-engineering pipeline,
      signing, user docs, website, and hygiene scrub are k3sm / site-repo work with
      no apis edge.
    subphases:
      - id: M7.2
        title: public CI workflow + README polish
        status: todo
        depends_on: []
        deliverables:
          - id: M7.2-d1
            done: false
            desc: "Public CI workflow — apis/.github/workflows/ci.yml, a THIN wrapper over the existing hack/ci.sh (no logic duplication): CGO_ENABLED=0, macos-15 arm64 runner, gofmt -l / go vet / go build / go test with -race, plus the buf generate-diff + buf breaking gates apis already runs. dco.yml already exists in all repos, so the red-at-main reason is 'no ci.yml exists', not 'no workflows exist'. README refresh: apis's front-door README becomes the public go-gettable contract page (badges, pitch, the k3sm.io/apis vanity path) — apis stays clean / go-gettable (no committed replaces, unlike k3sm)."
          - id: M7.2-d2
            done: true  # landed standalone 2026-08-29 via the B187 carve (apis#34) — m11-plan R25(b); the prose capability list below is superseded by the phases.json-derived taxonomy in k3smtest
            desc: "k3smtest.SkipUnless shared test helper + capability taxonomy HOSTED IN apis. A new package (k3sm.io/apis/k3smtest) exporting SkipUnless(t, cap) over the ONE owned capability taxonomy (root, lo0, utun, pf, clang, apple-gpu, macos-26, network); when K3SM_CI_REQUIRE names a cap that is absent, the helper is FATAL (not skip) so self-skips turn red, not silent — an env-var helper without the meta-test fails open by construction. apis is the DAG-legal home runtimed, darwin-net, and k3sm all import (a leaf copy would drift or force a sideways import). Pure Go, zero k3sm.io/* imports; the no-raw-t.Skip lint + the meta-test get named homes in the consuming repos."
        acceptance:
          - id: M7.2-a1
            met: false
            check: "the GitHub Actions run on this repo's PR is green — ci.yml runs gofmt/vet/build/test/-race + the buf gates on a macos-15 arm64 runner, CGO_ENABLED=0"
            method: build
          - id: M7.2-a2
            met: false
            check: "the SkipUnless meta-test passes — with K3SM_CI_REQUIRE naming a cap in the REQUIRE set that is absent, SkipUnless is FATAL (t.Fatal), not t.Skip; the taxonomy is table-tested; -race clean"
            method: unit

  - id: M8
    title: MLX — native Apple-Silicon ML serving (apis slice — MLXModel CRD + proto GPU/egress facts)
    status: done
    completed: 2026-08-29
    depends_on: []
    notes: >-
      apis is Wave 1 of M8 (CRD introduction and the reserved-band carve +
      buf-baseline regenerate are human-reviewed). Hard cut: every change is
      additive — a NEW alpha CRD group (mlx.k3sm.io/v1alpha1, no existing consumers)
      and proto fields carved from the EXPLICITLY RESERVED bands of
      runtime/v1/runtime.proto (SandboxProfile 102..149, GetRuntimeInfoResponse
      100..149). The new SandboxProfile booleans default false, so an old runtimed
      ignores them and an old provider never sets them — no provider↔runtimed phased
      exception.
    subphases:
      - id: M8.1
        title: MLXModel CRD + proto GPU/egress facts
        status: done
        completed: 2026-08-29
        depends_on: []
        deliverables:
          - id: M8.1-d1
            done: true
            desc: "MLXModel CRD types in a NEW k3sm.io/apis/mlx/v1alpha1 package — hand-written types mirroring the net/v1 MeshPeer precedent (metav1 TypeMeta/ObjectMeta, hand-written DeepCopy*/DeepCopyObject, no code-gen), but EXPLICITLY ALPHA-BRANDED: v1alpha1, incompatible changes allowed + documented in the package doc (a deliberate divergence from MeshPeer's served-v1 posture — the headline user-facing API earns a graduation runway). MLXModelSpec{Model, Revision, Quantization, Memory resource.Quantity (required — the real scheduling constraint), Replicas *int32, Port int32, Runtime{Image,Args}, Cache{Size,StorageClassName}, NodeSelector, Distributed *MLXDistributed}; Status is conditions-first ([]metav1.Condition + observedGeneration + status subresource) with a derived Phase printer column, Endpoint, ResolvedRevision. spec.Distributed is RESERVED (the JACCL seam) — its CEL-rejection test lives in k3sm, NOT here (see M8.1-d5)."
          - id: M8.1-d2
            done: true
            desc: "Additive proto fields carved from the reserved bands of runtime/v1/runtime.proto, following the in-file allocation-comment convention (the M2.2/M5.1 band-carve precedent): SandboxProfile bool allow_gpu = 102 + bool allow_internet_egress = 103 (re-carve to `reserved 104 to 149`, and REWRITE the band comment HONESTLY — the current 'fine-grained mach-service / sysctl allow rules' text fits allow_gpu but not egress, so the comment stays truthful when re-carving); GetRuntimeInfoResponse GPUFacts gpu = 100 (message: metal_available, chip_brand, chip_family, mem_bytes, iogpu_wired_limit_bytes (0 = the kernel-default sentinel, modeled explicitly — not 'no limit known'), recommended_max_working_set_bytes, sandbox_gpu_supported (scoped 'the currently-selected backend supports Metal')). RESERVE 101 — not merely earmark it: the carve is `reserved 101 to 149;` with the backend-capabilities earmark comment on 101."
          - id: M8.1-d3
            done: true
            desc: "NEW k3sm.io/apis/config/crd //go:embed package — the CRD manifest apis/config/crd/mlx.k3sm.io_mlxmodels.yaml (beside the existing net.k3sm.io_meshpeers.yaml) exported via a new go:embed package with a NAMED PER-CRD ACCESSOR, so k3sm's SSA ensure consumes the embedded bytes with NO shadow copy. M8's k3sm crdensure applies mlxmodels ONLY; MeshPeer stays out-of-band as today (adopting it is a named future follow-up with an M3-regression check, not an embed-glob side effect)."
          - id: M8.1-d4
            done: true
            desc: "Exported string constants — in k3sm.io/apis/mlx/v1alpha1: GroupName = \\\"mlx.k3sm.io\\\", ResourceGPU = \\\"mlx.k3sm.io/gpu\\\", LabelGPUPresent = \\\"mlx.k3sm.io/gpu.present\\\" (DISTINCT from the resource name), LabelChip/LabelChipFamily/LabelMemoryGB, plus the chip-slug normalization rule (apple-m4-max, not the raw space-bearing chip_brand). BUT the internet-egress annotation constant is k3sm.io/*-domain and parameterizes runtime/v1 SandboxProfile, so it lives in a RUNTIME-AREA apis package, NOT mlx/v1alpha1 (the MLX package holds only mlx.k3sm.io/* constants). No string literals in k3sm/runtimed."
          - id: M8.1-d5
            done: true
            desc: "RECORD-ONLY placement note (no apis code — satisfied by this ledger entry plus the a2/a3 gates asserting apis carries ONLY DeepCopy/round-trip tests): the spec.distributed-set→rejected CEL contract test lives in k3sm beside pkg/crdensure (which already carries the k8s apiextensions structural-schema dependency set), recorded here like the NodePort/NodeNetwork no-ops so nobody adds k8s deps to apis — a faithful CEL test would drag apiextensions machinery into apis's deliberately-minimal graph, violating the zero-heavy-dep posture. The M8.1 wave marks this done together with d4 (no separate build step). RE-SCOPED 2026-08-29 (operator-directed reconciliation): was worded as a deliverable; it is a record with a no-op done criterion."
          - id: M8.1-d6
            done: true  # 2026-08-29 — k3sm#171 (DESIGN 5a egress ceiling + GPU opt-in) + the workspace privilege-model section
            desc: "DESIGN + privilege-model doc edits (named M8.1 deliverables — these ARE apis-milestone work, not k3sm's): k3sm/docs/DESIGN.md §5a (the per-pod internet-egress opt-in widening + the content-addressed-tree materialize note) / §5b, plus the privilege-model doc, edited for the egress opt-in and the new mlx.k3sm.io CRD group."
        acceptance:
          - id: M8.1-a1
            met: true
            check: "buf breaking runs against the PRE-CARVE committed baseline FIRST (proves the reserved-band carve is additive — no field renumber, no reserved-number reuse, 101 reserved), THEN buf/baseline.binpb is regenerated as the new floor (checking against a regenerated baseline is self-comparison and proves nothing); buf generate leaves no diff; apis/hack/ci.sh green"
            method: unit
          - id: M8.1-a2
            met: true
            check: "builds CGO_ENABLED=0 standalone (GOWORK=off) + under go.work; the MLXModel DeepCopy golden + JSON round-trip and proto.Equal round-trip hold for GPUFacts + the new SandboxProfile fields (table-driven, -race clean)"
            method: unit
          - id: M8.1-a3
            met: true
            check: "the module still imports zero k3sm.io/* packages (cycle check); alpha branding is asserted (registered GVK group mlx.k3sm.io, version v1alpha1); the egress annotation constant is NOT in the mlx package"
            method: build

  - id: M10
    title: Kubernetes conformance hardening (apis slice — native-sidecar Container.restart_policy proto field)
    status: done
    completed: 2026-07-06
    updated_by: orchestrator
    depends_on: []
    notes: >-
      apis is Wave 1 of M10: a wire-affecting runtime/v1 proto change is a
      named-exception (apis proto change, consumer-first). apis's ONLY M10 work is a
      single additive field: Container.restart_policy (the native-sidecar signal).
      Verified against
      main: the Container message (runtime/v1/runtime.proto ~L367) has NO
      restart_policy field, so the initContainer restartPolicy:Always signal for a
      native sidecar cannot cross the provider↔runtimed gRPC contract (translate.go:507
      drops it). NEVER a k3sm.io/* annotation — the signal is a first-class proto field.
      Every OTHER M10 type (Ingress/IngressClass/NetworkPolicy/ResourceQuota/LimitRange/
      PSA) is an upstream type the embedded real apiserver already serves, so apis needs
      no other M10 change. Additive-only, stable field number, next FREE sequential
      number below 100 on Container (reserved bands untouched); consumer-first per the
      apis-proto named exception — dependents (k3sm translate + runtimed sidecar
      lifecycle) ship tolerant readers first, then the field is consumed in M10.2.
    subphases:
      - id: M10.2
        title: Container.restart_policy proto field (native sidecar signal)
        status: done
        depends_on: []
        deliverables:
          - id: M10.2-d1
            done: true
            desc: "Container.restart_policy — a single additive field on the runtime/v1 Container message (runtime.proto ~L367) carrying the per-container restartPolicy so the native-sidecar signal (initContainer restartPolicy:Always, stable since k8s 1.33) crosses the provider↔runtimed gRPC contract, which today it cannot (the message has no such field; translate.go:507 drops it). Modeled on corev1.Container.RestartPolicy (a ContainerRestartPolicy string/enum; only \"Always\" is meaningful for the sidecar case). Allocated the next FREE sequential field number below 100 on Container (the M2.1/M3.1 stable-number precedent; reserved bands untouched, no renumber). NOT a k3sm.io/* annotation — a first-class proto field (Res.8). Consumer-first per the named exception (apis proto change): dependents ship tolerant readers first; the field is consumed by k3sm translate + runtimed sidecar lifecycle in M10.2. buf breaking runs against the PRE-carve committed baseline FIRST (proves additive — no renumber, no reserved-number reuse), THEN buf/baseline.binpb is regenerated as the new floor (checking against a regenerated baseline is self-comparison and proves nothing — the M8.1 baseline discipline)."
        acceptance:
          - id: M10.2-a1
            met: true
            check: "CGO_ENABLED=0 build/test standalone (GOWORK=off) + under go.work; buf breaking (WIRE_JSON) runs against the committed pre-carve baseline/binpb FIRST and is clean (additive-only, no field renumber, no reserved-number reuse), THEN buf/baseline.binpb is regenerated as the new floor; buf generate leaves no diff; proto.Equal round-trip golden holds for a Container carrying restart_policy; the field is consumed by k3sm translate + runtimed sidecar lifecycle in M10.2; the module still imports zero k3sm.io/* packages"
            method: unit

  - id: M11
    title: Linux containers & multi-arch (apis slice — guest/v1 vsock contract + platform fields + Rosetta label constants)
    status: done
    completed: 2026-08-29
    depends_on: []
    notes: >-
      apis is Wave 1 of M11 (docs/m11-plan.md — authoritative; Phase C encoded from it).
      RE-SEQUENCED PRE-LAUNCH (2026-07-11, its R16): the M11 build waves ship the vm path
      functional-EXPERIMENTAL at v0.1; run out of ledger order with recorded rationale —
      the M10 remainder is hardware-gated and not a dependency.
      Hard cut: a NEW proto package (guest/v1, zero existing consumers) + additive
      carves from the EXPLICITLY RESERVED runtime/v1 bands (Container 100..149,
      ImageManifest 100..149) + the next free Volume source number. The guest/v1
      contract crosses the runtimed↔initramfs boundary (an independently-shipped pinned
      artifact), so it carries the runtime/v1-style additive-only stability header, buf
      baseline discipline, and an api_version handshake on Health — compat = lockstep
      via the in-code initramfs sha256 pin; --guest-artifacts-dir is unsupported skew.
      Recorded NON-changes (so nobody adds deps): NO SandboxProfile.guest_network field
      (the B6 carrier is the in-process runtimed.Deps podnet adapter, provider-side
      SetupGuest BEFORE toPodBox — the M10.1 one-authority ordering; the 102..149 band
      keeps its earmark); NO default-platform field (the default policy derives
      runtimed-side from SandboxProfile.Backend + probed capabilities — only the
      explicit override rides the wire); Rosetta capability advertisement rides the
      existing RuntimeCondition repeated field (the B1 precedent, zero proto change).
    subphases:
      - id: M11.1
        title: guest/v1 (GuestAgent + GuestSpec + VMHostSpec) + runtime/v1 platform/hostPath carves + label constants
        status: done
        completed: 2026-08-29
        depends_on: []
        deliverables:
          - id: M11.1-d1
            done: true
            desc: "NEW k3sm.io/apis/guest/v1 proto package — the WHOLE VM contract family in ONE home (deliberately no apis/vm/v1): the GuestAgent vsock gRPC service (Health{ready, leased guest IP, rosetta_registered, api_version/capabilities}, ContainerEvents stream{started/exited{code,signal,oom}}, Exec/Logs REUSING runtime/v1 stream messages (the reuse doc states pod_id must match the booted pod and container selects — single-pod-guest semantics), Stats{per-container cgroup2 cpu usage_usec + working set = memory.current − inactive_file}, Stop{grace_seconds → TERM→KILL→poweroff}), the GuestSpec message (guest-spec.json is its proto-JSON: hostname, resolv_conf, containers[]{name, rootfs_tag, RunSpec fields, uid/gid/supplemental_gids, init}, mounts[]{tag_or_source, target, kind VIRTIOFS|TMPFS|BIND, read_only, size_limit, idmap}, rosetta, fs_group, agent_port), and the VMHostSpec message (vmhost.spec.json: pod_id, vcpus, memory_bytes, kernel_path, initramfs_path, cmdline, shares[]{tag, host_path, read_only}, rosetta, agent_vsock_port, mac_address). Package doc records the placement rationale (the initramfs embedding the agent is an independently-shipped pinned artifact ⇒ genuine versioned wire contract), the additive-only stability header, and the lockstep-pin compat posture. doc.go required."
          - id: M11.1-d2
            done: true
            desc: "runtime/v1 band carves (additive; re-narrow each reserved range + REWRITE the band comments honestly — the M8.1-d4 convention). RE-SCOPED 2026-08-29 (operator-directed reconciliation) — REMAINING (B127 landed ImageManifest.platform=100 + index_digest=101 on 2026-08-10; M12.1's image_pull_policy already occupies Container 101): Container.image_platform = 100 (the pinned number — the band comment reserves it by name; typed as the existing Platform message; rewrite the band comment, re-carve reserved 102 to 149 minus 101), Volume.host_path = 8 + HostPathVolumeSource{path,type} verbatim-corev1/consumer-less, and the Container.command discriminator doc comment. The ImageManifest half of the old text is DONE-by-B127 — do not re-carve it. Surviving contract detail on the three remaining carves: image_platform is typed as the EXISTING Platform message, never a string (one normalization point for os/arch/variant incl. arm64 \"\"≡v8); HostPathVolumeSource{path, type} is a VERBATIM corev1 mirror — shape only, ALL enforcement semantics (allowlist, share-vs-snapshot) stay OUT of the proto, and the field lands CONSUMER-LESS (guest hostPath is fail-closed rejected until human-gated B98 lands — m11-plan Resolution 1); the resolveBinary contract comment on Container.command DISCRIMINATES rather than retires the M0 empty-command convention — absolute-path image string ⇒ host-binary convention; OCI reference ⇒ image-config merge (consumed in runtimed M11.2-d1)."
          - id: M11.1-d3
            done: true
            desc: "Exported label/annotation constants (runtime-area apis package, the M8.1-d4 placement rule): k3sm.io/rosetta (host Rosetta), k3sm.io/rosetta-linux (guest Rosetta ∧ VMBackendAvailable — the composition is documented on the constant), k3sm.io/image-platform (the per-POD override annotation; the k3sm provider parses once and stamps Container.image_platform per-container — the pod-level annotation applies to every container, no per-container key form in v1). No string literals in runtimed/k3sm."
          - id: M11.1-d4
            done: true  # verified pre-encoded by the W0 roadmap PR (DESIGN.md:84/:102/:107), 2026-08-29
            desc: "DESIGN doc edits (named M11.1 deliverables — the m8-Res.19 pattern): k3sm/docs/DESIGN.md §5a multi-arch-aware pull sentence (platform-keyed cache, fail-closed ErrNoPlatformMatch, vm-path whiteout unpack, digest-pin stays the vm integrity anchor) + §5c/§6 third-entitled-artifact prose (k3sm-vmhost carries com.apple.security.virtualization ONLY; the one-signed-binary statement gains the helper the same way k3sm-netd did) — pre-encoded by the roadmap PR, kept truthful here if the wave moves them."
        acceptance:
          - id: M11.1-a1
            met: true
            check: "buf breaking runs against the PRE-carve committed baseline FIRST (additive only — no renumber, no reserved-number reuse), THEN buf/baseline.binpb regenerated as the new floor (the M8.1 baseline discipline); buf generate no-diff; apis/hack/ci.sh green"
            method: unit
          - id: M11.1-a2
            met: true
            check: "guest/v1 proto round-trips (proto.Equal golden); GuestSpec + VMHostSpec proto-JSON goldens hold (the spec files ARE this encoding); Health carries api_version; CGO_ENABLED=0 standalone (GOWORK=off) + go.work builds; -race clean"
            method: unit
          - id: M11.1-a3
            met: true
            check: "the module still imports zero k3sm.io/* packages (cycle check); Container.image_platform/ImageManifest.platform are the Platform message type (no string platform field anywhere); Volume.host_path mirrors corev1 exactly (path+type only); NO SandboxProfile.guest_network field exists"
            method: build

  - id: M12
    title: Images & build engine (apis slice — the imagePullPolicy enum carve)
    status: todo
    depends_on: []
    notes: >-
      docs/m12-plan.md is authoritative (Phase C encoded from it). One additive
      reserved-band carve, hard cut (the restart_policy precedent: the enum zero value is
      legacy behavior in both skew directions). Consumers ride queue item B120, which
      depends on this wave.
    subphases:
      - id: M12.1
        title: Container.image_pull_policy enum carve
        status: done
        depends_on: []
        deliverables:
          - id: M12.1-d1
            done: true
            desc: "ImagePullPolicy enum (IMAGE_PULL_POLICY_UNSPECIFIED = 0 = legacy pull-through in BOTH skew directions / ALWAYS / IF_NOT_PRESENT / NEVER) + Container.image_pull_policy. Field number: allocate the next free number in the 100–149 band AT ENCODE TIME from the proto file — NEVER 100 (pinned to M11.1's image_platform regardless of landing order; allocate ≥101), re-narrow the reserved range + rewrite the band comment honestly (the PodBox allocation precedent). Semantics comment pins: the embedded apiserver OWNS defaulting (:latest/untagged → Always) — the provider translates the stamped value verbatim, runtimed never re-derives from the tag."
        acceptance:
          - id: M12.1-a1
            met: true
            check: "buf breaking vs the PRE-carve committed baseline first, then baseline regenerated; buf generate no-diff; the enum zero value is UNSPECIFIED with the legacy-in-both-skew-directions comment (the restart_policy pattern); the allocated field number is ≥101 and the reserved range re-narrowed; apis/hack/ci.sh green"
            method: unit
---

# apis — Phase roadmap

> Per-repo slice of the k3sm milestones (product design: `k3sm/docs/DESIGN.md` §6/§7). The YAML
> frontmatter above is **authoritative**; this prose mirrors it.
> Status: ✅ done · 🟡 in-progress · ⛔ blocked · ⬜ todo.

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

## M2 — Pod-spec fidelity proto additions + gRPC daemon surface + resource/metrics types ✅
Decomposed now that M1 has landed. Headline: raise pod-spec fidelity to what the `stockkitty`
reference workload exercises in **M2.1**, then extend `runtime/v1`
for the **resource/metrics** surface (`ri_phys_footprint` → `kubectl top`) in **M2.2**. All additions are
**additive-only**; field numbers are **STABLE** (`buf breaking` WIRE_JSON gate). **Allocation split** so the
two sub-phases never collide: **M2.1** pod-spec fields take the next **FREE sequential numbers below 100**
on each message; the **reserved bands** (`PodBox` `100..199`; sibling messages `Container`,
`ContainerStatus`, `SandboxProfile`, … `100..149`) are **left UNTOUCHED for M2.2** (resource limits +
metrics) and never reuse a reserved number.

> **No `api_version` handshake.** The M2 provider↔runtimed split is a **same-binary, same-node hard cut**:
> the provider and its node's `runtimed` are the **same `k3sm` build**, restarted together via
> `launchctl kickstart`. Behavior-bearing fields therefore need **no version negotiation** — there is no
> independent-upgrade skew window, and `GetRuntimeInfoResponse.api_version` is an inert constant (no
> negotiation surface is built on it). (Earlier drafts had these fields "ride the `api_version` handshake";
> that was struck — the field is a constant, not a negotiation point.)

### M2.1 — pod-spec fidelity proto additions ✅
Field numbers allocated (all below 100; reserved bands untouched): **PodBox** `volumes=15`,
`pod_security_context=16`, `termination_grace_period_seconds=17`, `image_pull_secrets=18`; **Container**
`volume_mounts=9`, `ports=10`, `liveness_probe=11`, `readiness_probe=12`, `startup_probe=13`,
`security_context=14`, `env_from=15`; **EnvVar** `value_from=3`; **ContainerStatus** `volume_mounts=11`,
`user=12`.

**Deliverables**
- ✅ `M2.1-d1` `PodBox.volumes` (15) — a repeated `Volume` carrying the sources stockkitty mounts: `configMap`, `secret`, `emptyDir`, `downwardAPI`, `projected`. `ProjectedVolumeSource` includes `ServiceAccountTokenProjection{audience, expirationSeconds, path}` (bound token — the in-pod-kubectl path) plus configMap/secret/downwardAPI projections. The source union is **optional message fields** (the `ContainerState` pointer-union precedent), not a `oneof`. `runtimed:M2` materializes payloads inside the pod data-volume.
- ✅ `M2.1-d2` `Container.volume_mounts` (9) — a repeated `VolumeMount{name, mountPath, readOnly, subPath}` referencing `PodBox.volumes` by name. Secrets / SA-token mounts get a read-only sub-scope runtimed-side.
- ✅ `M2.1-d3` `Container` probes (`liveness_probe=11`/`readiness_probe=12`/`startup_probe=13`) — each a `Probe{initialDelaySeconds, periodSeconds, timeoutSeconds, successThreshold, failureThreshold}` with the handler modeled as **three optional message fields** (`http_get`/`tcp_socket`/`exec`, **not** a `oneof` — matching `ContainerState`; "exactly one set" documented). `HTTPGetAction{path, port IntOrString, scheme, host, httpHeaders}`; `TCPSocketAction{port IntOrString, host}`; `ExecAction{command}`. Adds `Container.ports` (10) = repeated `ContainerPort{name, containerPort, protocol}` (named-port table so named probe ports + named Service targetPorts resolve) and `IntOrString{intVal, strVal}`. Probes are **provider-served** (`k3sm:M2`), driving `ContainerStatus.ready` + Service endpoints.
- ✅ `M2.1-d4` `securityContext` — `Container.security_context` (14) = `SecurityContext{runAsUser, runAsGroup, runAsNonRoot}` (**container scope; no `fsGroup`**); `PodBox.pod_security_context` (16) = `PodSecurityContext{fsGroup, runAsUser, runAsGroup}` (**`fsGroup` is pod-scope only**). Net-new runtimed privilege-drop: `setgid→initgroups→setuid` **before** `sandbox_apply`; `fsGroup` chown root-side **before** the drop.
- ✅ `M2.1-d5` `Container` env extensions — `Container.env_from` (15) = repeated `EnvFromSource{prefix, configMapRef, secretRef}`; `EnvVar.value_from` (3) = `EnvVarSource{fieldRef ObjectFieldSelector, configMapKeyRef, secretKeyRef}` (the corev1 union; `fieldRef = ObjectFieldSelector{apiVersion, fieldPath}` covers `spec.nodeName`/`status.podIP`/`metadata.name`). `EnvVar` keeps `name=1`/`value=2`; value-vs-valueFrom mutual exclusion documented.
- ✅ `M2.1-d6` `PodBox.image_pull_secrets` (18) — repeated `LocalObjectReference{name}`. runtimed confines the credential to the pull client (never written to the pod dir); signature policy enforced **before** ad-hoc-sign.
- ✅ `M2.1-d7` `PodBox.termination_grace_period_seconds` (17, int64) — the source the provider derives `DeletePodRequest.grace_period_seconds` from (that RPC field already exists). Net-new runtimed SIGTERM → per-PID grace timer raced against the kqueue reaper → SIGKILL.
- ✅ `M2.1-d8` **CRITICAL lossless-mirror pairing** — `ContainerStatus.volume_mounts` (11) = repeated `VolumeMountStatus{name, mountPath, readOnly}` and `ContainerStatus.user` (12) = `ContainerUser{linux LinuxContainerUser}`, `LinuxContainerUser{uid, gid, supplementalGroups}` (the **effective** identity the privilege-drop produces). Only the corev1 status surface for M2.1 spec fields is added — `resources`/`allocatedResources` are **M2.2** (reserved band). Without the pairing, kubectl Pod state degrades crossing the runtime boundary.

**Acceptance (exit gate)** — all met
- ✅ `M2.1-a1` `buf generate` is reproducible (no diff) and `buf breaking` (WIRE_JSON) vs `buf/baseline.binpb` is **clean** — additive-only, no renumber, reserved bands untouched — *method: unit*
- ✅ `M2.1-a2` `proto.Equal` round-trip holds for every new message (one fully-populated case each — every `Volume` source incl. `serviceAccountToken`; each `Probe` handler; `SecurityContext`/`PodSecurityContext`; `EnvFromSource`; `EnvVarSource`/`fieldRef`; `ContainerPort`; `ContainerStatus` `volumeMounts`+`user`) plus full-`Container`/full-`PodBox`/full-`ContainerStatus` cases; `-race` clean — *method: unit*
- ✅ `M2.1-a3` zero `k3sm.io/*` imports (cycle check); builds `CGO_ENABLED=0` standalone (`GOWORK=off`) + under `go.work` — *method: unit*

### M2.2 — resource-limit + metrics types ✅
Owns the **reserved bands** (`PodBox` `100..199`; sibling messages `100..149`) the M2.1 low-number fields
deliberately avoid, so M2.1 (free numbers) and M2.2 (reserved bands) **never collide**. `depends_on: []`
(apis-internal). Field numbers allocated **within** each message's own reserved band, **ceilings
preserved**: **PodBox** `memory_limit_bytes=100`, `qos_class=101` (typed `QOSClass` enum), `rlimits=102`
(repeated `ResourceLimit{type,soft,hard}`, OCI-style) → reserved narrowed `100..199`→`103..199`;
**ContainerStatus** `resources=100` (`ResourceRequirements`), `allocated_resources=101` (`ResourceList`)
→ reserved narrowed `100..149`→`102..149`. New **standalone** types `PodStats`/`ContainerStats`/`CPUStats`/
`MemoryStats` (working-set from `ri_phys_footprint`) and shared `ResourceList`/`ResourceRequirements`/
`ResourceLimit`/`QOSClass` do **not** touch the lifecycle messages' reserved bands. Two additive RPCs
append to `service Runtime`: **`ListPodStats`** (the `kubectl top` / kubelet-Summary-API metrics path) and
**`RestartContainer`** (the liveness-restart action the provider's probe runner invokes — the seam that,
pre-M2.2, only bumped `restart_count` via a nil `restartFunc`).

**Deliverables**
- ✅ `M2.2-d1` `PodBox` resource-limit fields — `memory_limit_bytes` (100, the typed field that **replaces the M2.1 `k3sm.io/memory-limit-bytes` annotation seam** runtimed bridged the limit on), `qos_class` (101, `QOSClass` Guaranteed/Burstable/BestEffort), `rlimits` (102, repeated `ResourceLimit`). Inputs `runtimed:M2.2` enforces for `OOMKilled` (`proc_pid_rusage`; `ri_phys_footprint` ≠ RSS) and best-effort CPU QoS (not CFS millicores).
- ✅ `M2.2-d2` `ContainerStatus.{resources, allocated_resources}` (100/101) — the status mirror (`ResourceRequirements` limits/requests + a bare `ResourceList`), completing the lossless corev1 mirror now that resources land.
- ✅ `M2.2-d3` Summary-API / pod-stats messages for `kubectl top` — `PodStats`/`ContainerStats` (per-pod/per-container) with `CPUStats` + `MemoryStats` (`working_set_bytes` from `ri_phys_footprint`), served via the additive `ListPodStats` RPC. Standalone messages; do not consume the lifecycle messages' reserved bands.

**Acceptance (exit gate)** — all met
- ✅ `M2.2-a1` additive-only — `buf breaking` (WIRE_JSON) clean vs `buf/baseline.binpb`; resource/metrics fields land in the reserved `100..199` / `100..149` bands (ceilings preserved). Consuming a reserved **message** band narrows its declaration, which buf's `RESERVED_MESSAGE_NO_DELETE` flags; that **one** rule is `except`ed in `buf.yaml` (documented) because converting never-used reserved headroom into a field is wire-safe — every field-level guard (no renumber/retype/delete, stable JSON names) and `RESERVED_ENUM_NO_DELETE` stay ON (verified: a sanity renumber still trips breaking) — *method: unit*
- ✅ `M2.2-a2` `proto.Equal` round-trip holds for every new field/message (fully-populated cases) **plus** the new RPCs are asserted registered as unary methods on `Runtime_ServiceDesc`; table-driven, `-race` clean — *method: unit*

## M3 — Storage volume sources (PV/PVC) + MeshPeer CRD + NodeNetwork + mesh-enroll types ✅
Headline: persistent storage for the reference workload (Postgres / compile-artifacts PVCs) **plus** the
`net.k3sm.io` `MeshPeer` CRD (node public key + endpoint + podCIDR/AllowedIPs) and the mesh-enroll payload
that rides `k3sm`'s join (consumed by `darwin-net` mesh, written by `k3sm` join). The M3 re-plan
**split mesh out of M3.1 into a new M3.2**, because `darwin-net:M3` (wireguard
mesh) and `k3sm:M3` (join) must depend on the mesh contracts, **not** on the storage source. Both landed
additive-only (stable field numbers; `buf breaking` WIRE_JSON clean). M3.2 introduces **`apis`'s first
`k8s.io/apimachinery` dependency** (pinned `v0.35.0`, lockstep with `k3sm/go.mod`) for the served MeshPeer
CRD; the module still imports zero `k3sm.io/*` packages and builds `CGO_ENABLED=0`.

### M3.1 — storage volume sources (NodePort needs NO `apis` change) ✅
**Deliverables**
- ✅ `M3.1-d1` PV/PVC volume source on `PodBox` — `persistentVolumeClaim` added as the durable source to the M2.1 `Volume` union: `PersistentVolumeClaimVolumeSource{claim_name, read_only}` on `Volume.persistent_volume_claim=7` (the next **free sequential** source number; the M2.1 sources took `2..6`, and `Volume` has **no** reserved band, so this is a plain additive append). **No new RPC** — the existing `CreatePod`/`PodBox` mount mechanism (M2.1 volumes + `Container.volume_mounts`) carries it. `runtimed:M3` binds it to a stable per-PVC dir on the **same APFS volume** as `/var/lib/k3sm` (empty-create; `clonefile` only *seeds*; lifecycle decoupled from pod-dir teardown). Mirrors `corev1.Volume.PersistentVolumeClaim`.
- ✅ `M3.1-d2` StorageClass / provisioner contract — added the plain-Go package **`k3sm.io/apis/storage/v1`** (`storagev1`). The upstream `storage.k8s.io` `StorageClass` / `core/v1` `PersistentVolume` objects remain the served API surface (**not** vendored or redefined), but a small agreement **does** cross the `k3sm`-provisioner ↔ `runtimed`-binder boundary: `LocalPathClass` (`provisioner=k3sm.io/local-path`, `basePath=/var/lib/k3sm/storage`, `reclaimPolicy=Retain`-only, `volumeBindingMode=WaitForFirstConsumer`-only) + the stable `DataDir(namespace, claimName)` derivation **both** repos compute (runtimed resolves it from the `PodBox` alone — it never needs the PV UID) + `PVName(pvcUID)` (the idempotency key) + `NodeTopology{key=kubernetes.io/hostname, nodeName}` (the PV **node-affinity** that pins a local PV — and its StatefulSet pod — to its owning Mac). Plain Go (the `net/v1` precedent), additive-only, camelCase JSON, `Validate`/`WithDefaults`, zero `k3sm.io/*` imports.
- ✅ `M3.1-d3` **NodePort needs NO `apis` change** — `ServicePort.NodePort` **already exists** in `net/v1` (validated, tested). M3 NodePort work is `darwin-net` proxy (bind `*:port`, TCP; UDP relay deferred) + `k3sm` wiring **only**. **Confirmed no-op**: the field was **not** re-added, renamed, or renumbered; `TestNodePortUnchangedM3` pins its presence + JSON name + round-trip. Recorded so dependents' `depends_on` resolve.

**Acceptance (exit gate)** — all met
- ✅ `M3.1-a1` additive-only — `buf breaking` (WIRE_JSON) clean vs `buf/baseline.binpb` and `net/v1 ServicePort.NodePort` unchanged (`git diff main` confirms no existing field number/name changed); builds `CGO_ENABLED=0` standalone + under `go.work` — *method: unit*
- ✅ `M3.1-a2` `proto.Equal` round-trip holds for `PersistentVolumeClaimVolumeSource` + `Volume.persistent_volume_claim` + a PVC-backed `PodBox`; Go JSON round-trip + `Validate`/`WithDefaults` hold for the `storagev1` `LocalPathClass` + `NodeTopology` (incl. `DataDir`/`PVName`); table-driven, `-race` clean — *method: unit*

### M3.2 — mesh contracts: MeshPeer CRD + mesh-enroll payloads ✅
The re-plan's **#1 architectural fix**: M3.1 produced no mesh type, but `darwin-net:M3` and `k3sm:M3`
depend on one. Public keys only — **private keys never leave the node** (DESIGN §5b).

**Deliverables**
- ✅ `M3.2-d1` **`MeshPeer` CRD (`net.k3sm.io/v1`)** — a real served/watchable, kine-stored Kubernetes custom resource in `net/v1` (`mesh.go`): `MeshPeer`/`MeshPeerList` embedding `metav1.TypeMeta`+`ObjectMeta` (**`apis`'s first `k8s.io/apimachinery` dependency** — pinned `v0.35.0`, lockstep with `k3sm`), hand-written `DeepCopy*`/`DeepCopyObject` (no code-gen in `apis`), and a `SchemeBuilder`/`AddToScheme`/`Resource` registration. **Cluster-scoped** (one per node, named for the node). `MeshPeerSpec`: `schemaVersion` (the wireguard-encoding evolution seam **inside** the `v1` GVK) + `nodeName` + `publicKey` + `endpoint` + `podCIDR` + symmetric `allowedIPs` (must equal `podCIDR`) + `meshIP` + `persistentKeepaliveSeconds`; `MeshPeerStatus`: `lastHandshakeTime`/`reachable`/`observedSchemaVersion` (status subresource). Authored manifest `config/crd/net.k3sm.io_meshpeers.yaml` (single `v1` **served+stored**, additive-only).
- ✅ `M3.2-d2` **mesh-enroll / join payloads** — plain Go structs (NOT a CRD, NOT proto) the bootstrap join HTTP exchange marshals (`k3sm`'s join client + supervisor): `MeshEnrollRequest{schemaVersion, nodeName, publicKey, endpoint, podCIDR}` + `MeshEnrollResponse{schemaVersion, nodeName, podCIDR, meshIP, peers []MeshPeerSpec}`. **Version-stamped from day one** (`MeshEnrollSchemaVersion=1`) so an M4+ node-by-node roll has a compatibility seam; the peer snapshot reuses the canonical `MeshPeerSpec`. `Validate`/`WithDefaults`.
- ✅ `M3.2-d3` **`NodeNetwork` — explicit NO-OP** for `apis` (recorded, like NodePort). No concrete cross-repo `NodeNetwork` type emerged: the M3 networking work (per-node CoreDNS + infra-VIP exemption in `darwin-net:M3.3`, the node-local `kubernetes` endpoint rewrite in `k3sm:M3.3`) is `darwin-net`-internal + a `k3sm`-owned controller — neither needs a new `apis` type. Do **not** invent one.

**Acceptance (exit gate)** — all met
- ✅ `M3.2-a1` the `MeshPeer` CRD type round-trips losslessly (JSON, byte-stable incl. the status `metav1.Time` + the `schemaVersion` stamp) and `DeepCopy`/`DeepCopyObject` produce independent objects; a table test asserts the registered **GVK is `net.k3sm.io/v1`** (`MeshPeer` + `MeshPeerList`) via a `runtime.Scheme`; builds `CGO_ENABLED=0` standalone + under `go.work`; the module still imports zero `k3sm.io/*` packages — *method: unit*
- ✅ `M3.2-a2` the mesh-enroll payloads round-trip (JSON) and carry a non-zero `SchemaVersion`; `MeshPeerSpec` + enroll `Validate`/`WithDefaults` are table-tested (version-stamp + required fields); `-race` clean — *method: unit*

## M4 — API-stability freeze ⬜
Headline: version/freeze the v1 protos + CRDs for public consumption; ensure `go get k3sm.io/apis`
resolves via the vanity path (the GitHub-Pages `go-import` recipe, DESIGN §6a); doc completeness.

## M5 — vm RuntimeClass handler mapping (Linux-image micro-VM backend) ⬜
Newly-committed milestone: the `vm` RuntimeClass that runs
Linux-only images (pgvector / `nats:alpine` / the C++ ELF) in a Virtualization.framework micro-VM behind
the existing swappable `sandbox.Backend` seam. `apis` is **Wave 1** — its `M5.1` is the contract the
runtimed VZ backend (`runtimed:M5`) and the guest-side networking (`darwin-net:M5`) build against.

### M5.1 — vm RuntimeClass handler-config mapping ⬜
**Deliverables**
- ⬜ `M5.1-d1` a `runtime.k3sm.io` handler-config type mapping the upstream `node.k8s.io/v1` `RuntimeClass` handler value (e.g. `vm`) → a `SANDBOX_BACKEND`. **Reuse the existing `SANDBOX_BACKEND_VM`** enum value (`runtime/v1`, `= 4`); do **not** fork, redefine, or vendor the upstream `RuntimeClass` API. `k3sm` consumes the standard upstream `RuntimeClass` object **after** admission/scheduling and looks up the backend via this handler-config; the type carries only the name→backend mapping (+ any VZ-path backend params).

**Acceptance (exit gate)**
- ⬜ `M5.1-a1` additive-only; builds `CGO_ENABLED=0` standalone + under `go.work`; zero `k3sm.io/*` imports; does **not** vendor or redefine the upstream `node.k8s.io` `RuntimeClass` type — *method: unit*
- ⬜ `M5.1-a2` the handler-name → `SANDBOX_BACKEND` mapping is table-tested (`vm` → `SANDBOX_BACKEND_VM`; unknown handler → a defined fallback/error), `-race` clean — *method: unit*

## M7 — Public open-source launch: release engineering (apis slice) ⬜
`apis` is a **small M7 participant**: a public GitHub Actions CI workflow and the shared test-skip
helper. The release-engineering pipeline, signing,
user docs, website, and hygiene scrub are `k3sm` / site-repo work with **no `apis` edge**.

### M7.2 — public CI workflow + README polish ⬜
**Deliverables**
- ⬜ `M7.2-d1` Public CI workflow — `apis/.github/workflows/ci.yml`, a **thin** wrapper over the existing `hack/ci.sh` (no logic duplication): `CGO_ENABLED=0`, macOS-15 arm64 runner, `gofmt -l` / `go vet` / `go build` / `go test -race`, plus the `buf` generate-diff + breaking gates `apis` already runs. `dco.yml` already exists in all repos, so the red-at-`main` reason is "no `ci.yml` exists". **README refresh** → the public go-gettable contract page; `k3sm.io/apis` stays clean / go-gettable (no committed replaces, unlike `k3sm`).
- ⬜ `M7.2-d2` **`k3smtest.SkipUnless` shared test helper + capability taxonomy HOSTED IN `apis`**. A new package (`k3sm.io/apis/k3smtest`) exports `SkipUnless(t, cap)` over the **one owned taxonomy** (`root, lo0, utun, pf, clang, apple-gpu, macos-26, network`); when `K3SM_CI_REQUIRE` names an absent cap the helper is **fatal, not skip**, so self-skips turn red, not silent. `apis` is the only **DAG-legal home** `runtimed`, `darwin-net`, and `k3sm` all import (a leaf copy would drift or force a sideways import). Pure Go, zero `k3sm.io/*` imports.

**Acceptance (exit gate)**
- ⬜ `M7.2-a1` the GitHub Actions run on this repo's PR is **green** — `ci.yml` runs gofmt/vet/build/test/`-race` + the `buf` gates on a macOS-15 arm64 runner, `CGO_ENABLED=0` — *method: build*
- ⬜ `M7.2-a2` the **`SkipUnless` meta-test passes** — with `K3SM_CI_REQUIRE` naming a REQUIRE-set cap that is absent, `SkipUnless` is `t.Fatal`, not `t.Skip`; the taxonomy is table-tested; `-race` clean — *method: unit*

## M8 — MLX: native Apple-Silicon ML serving (apis slice) ⬜

**M8.1 d1–d5 ✅ 2026-08-29** (apis#33 — MLXModel v1alpha1 + GPU/egress carves + embedded CRD + constants; a1 proven vs the pre-carve baseline with a live negative control, a2/a3 mutation-spot-checked; d6 — the DESIGN/privilege-model prose in k3sm — is the one open row).
`apis` is **Wave 1** of M8 (CRD introduction is human-reviewed). **Hard cut** — additive only: a **new
alpha CRD group** (`mlx.k3sm.io/v1alpha1`, no existing consumers) and proto fields carved from the
**explicitly reserved bands** of `runtime/v1/runtime.proto` (`SandboxProfile 102..149`,
`GetRuntimeInfoResponse 100..149`). The new `SandboxProfile` booleans default false, so an old `runtimed`
ignores them and an old provider never sets them — no provider↔runtimed phased exception.

### M8.1 — MLXModel CRD + proto GPU/egress facts ⬜
**Deliverables**
- ⬜ `M8.1-d1` **MLXModel CRD types in a new `k3sm.io/apis/mlx/v1alpha1` package** — hand-written types mirroring the `net/v1` `MeshPeer` precedent (metav1 `TypeMeta`/`ObjectMeta`, hand-written `DeepCopy*`/`DeepCopyObject`, no code-gen), but **explicitly alpha-branded**: `v1alpha1`, incompatible changes allowed + documented in the package doc (a deliberate divergence from `MeshPeer`'s served-`v1` posture — the headline user-facing API earns a graduation runway). `MLXModelSpec{Model, Revision, Quantization, Memory resource.Quantity (required), Replicas, Port, Runtime{Image,Args}, Cache{Size,StorageClassName}, NodeSelector, Distributed *MLXDistributed}`; status conditions-first (`[]metav1.Condition` + `observedGeneration` + status subresource) with a derived `Phase` printer column, `Endpoint`, `ResolvedRevision`. `spec.Distributed` is **reserved** (the JACCL seam) — its CEL-rejection test lives in **`k3sm`**, not here (see `M8.1-d5`).
- ⬜ `M8.1-d2` **Additive proto fields from the reserved bands** of `runtime/v1/runtime.proto` (the M2.2/M5.1 band-carve precedent): `SandboxProfile bool allow_gpu = 102` + `bool allow_internet_egress = 103` (re-carve `reserved 104 to 149`, and **rewrite the band comment honestly** — the current "fine-grained mach-service / sysctl allow rules" text fits `allow_gpu` but not egress); `GetRuntimeInfoResponse GPUFacts gpu = 100` (`metal_available`, `chip_brand`, `chip_family`, `mem_bytes`, `iogpu_wired_limit_bytes` (0 = kernel-default sentinel, modeled explicitly), `recommended_max_working_set_bytes`, `sandbox_gpu_supported`). **Reserve 101** — not merely earmark it: `reserved 101 to 149;` with the backend-capabilities earmark comment on 101.
- ⬜ `M8.1-d3` **New `k3sm.io/apis/config/crd` `//go:embed` package** — the manifest `apis/config/crd/mlx.k3sm.io_mlxmodels.yaml` (beside `net.k3sm.io_meshpeers.yaml`) exported with a **named per-CRD accessor**, so `k3sm`'s SSA ensure consumes the embedded bytes with **no shadow copy**. `crdensure` applies `mlxmodels` only; `MeshPeer` stays out-of-band (adoption is a named future follow-up, not an embed-glob side effect).
- ⬜ `M8.1-d4` **Exported string constants** — in `mlx/v1alpha1`: `GroupName="mlx.k3sm.io"`, `ResourceGPU="mlx.k3sm.io/gpu"`, `LabelGPUPresent="mlx.k3sm.io/gpu.present"` (**distinct** from the resource name), `LabelChip`/`LabelChipFamily`/`LabelMemoryGB`, + the chip-slug rule (`apple-m4-max`, not the space-bearing `chip_brand`). **But** the internet-egress annotation constant is `k3sm.io/*`-domain and parameterizes `runtime/v1` `SandboxProfile`, so it lives in a **runtime-area `apis` package, NOT `mlx/v1alpha1`**.
- ⬜ `M8.1-d5` **Record-only placement note** — *no `apis` code*: satisfied by this ledger entry plus the `a2`/`a3` gates asserting `apis` carries **only** DeepCopy/round-trip tests. The `spec.distributed`→rejected CEL contract test lives in **`k3sm`** beside `pkg/crdensure` (which already carries the apiextensions structural-schema dep set) — recorded here like the NodePort/NodeNetwork no-ops so nobody adds k8s deps to `apis`. The M8.1 wave marks this **done together with `d4`** (no separate build step). *(Re-scoped 2026-08-29 — was worded as a deliverable; it is a record with a no-op done criterion.)*
- ⬜ `M8.1-d6` **DESIGN + privilege-model doc edits** (named M8.1 deliverables — these **are** `apis`-milestone work): `k3sm/docs/DESIGN.md` §5a (per-pod internet-egress opt-in widening + content-addressed-tree materialize note) / §5b, plus the privilege-model doc, for the egress opt-in and the new `mlx.k3sm.io` CRD group.

**Acceptance (exit gate)**
- ⬜ `M8.1-a1` `buf breaking` runs against the **pre-carve committed baseline first** (proves the carve is additive — no renumber, no reserved-number reuse, 101 reserved), **then** `buf/baseline.binpb` is regenerated as the new floor (checking against a regenerated baseline is self-comparison); `buf generate` no-diff; `apis/hack/ci.sh` green — *method: unit*
- ⬜ `M8.1-a2` builds `CGO_ENABLED=0` standalone (`GOWORK=off`) + under `go.work`; the MLXModel DeepCopy golden + JSON round-trip and `proto.Equal` round-trip hold for `GPUFacts` + the new `SandboxProfile` fields; table-driven, `-race` clean — *method: unit*
- ⬜ `M8.1-a3` the module still imports **zero `k3sm.io/*` packages** (cycle check); alpha branding asserted (GVK group `mlx.k3sm.io`, version `v1alpha1`); the egress annotation constant is **not** in the `mlx` package — *method: build*

## M10 — Kubernetes conformance hardening (apis slice) ⬜
`apis` is **Wave 1** of M10: a wire-affecting `runtime/v1` proto change is a **named exception** (apis
proto change, **consumer-first**). `apis`'s **only** M10 work is a single additive field — every other M10 type
(Ingress/IngressClass/NetworkPolicy/ResourceQuota/LimitRange/PSA) is an upstream type the embedded real
apiserver already serves, so `apis` needs no other M10 change.

### M10.2 — `Container.restart_policy` proto field (native sidecar signal) ⬜
**Deliverables**
- ⬜ `M10.2-d1` **`Container.restart_policy`** — a single additive field on the `runtime/v1` `Container` message (`runtime.proto` ~L367) carrying the per-container `restartPolicy`, so the **native-sidecar signal** (`initContainer` `restartPolicy:Always`, stable since k8s 1.33) crosses the provider↔runtimed gRPC contract — which today it **cannot** (the message has no such field; `translate.go:507` drops it). Modeled on `corev1.Container.RestartPolicy` (only `Always` is meaningful for the sidecar case). Allocated the **next free sequential** field number below 100 on `Container` (the M2.1/M3.1 stable-number precedent; reserved bands untouched, no renumber). **NOT a `k3sm.io/*` annotation** — a first-class proto field (Res. 8). **Consumer-first** per the named exception: dependents (`k3sm` translate + `runtimed` sidecar lifecycle) ship tolerant readers first, then the field is **consumed in M10.2**.

**Acceptance (exit gate)**
- ⬜ `M10.2-a1` `CGO_ENABLED=0` build/test standalone (`GOWORK=off`) + under `go.work`; **`buf breaking` (WIRE_JSON) runs against the committed pre-carve baseline first** and is clean (additive-only, no field renumber, no reserved-number reuse), **then** `buf/baseline.binpb` is regenerated as the new floor (checking against a regenerated baseline is self-comparison — the M8.1 discipline); `buf generate` leaves no diff; `proto.Equal` round-trip **golden** holds for a `Container` carrying `restart_policy`; the field is **consumed by `k3sm` translate + `runtimed` sidecar lifecycle in M10.2**; the module still imports **zero `k3sm.io/*` packages** — *method: unit*

## Dependents of these `apis` sub-phases
`apis` is **Wave 1**; downstream milestones `depends_on` these ids:
- `runtimed:M2` + `k3sm:M2` + `darwin-net:M2` ← `apis:M2.1` (the pod-spec fidelity fields). The provider↔runtimed split is a **same-binary, same-node hard cut** (restarted together via `launchctl kickstart`), so behavior-bearing fields need **no** version handshake — there is no independent-upgrade skew window.
- **Storage** — `runtimed:M3` (APFS PV binder) + `k3sm:M3` (local-path provisioner / StatefulSet) ← `apis:M3.1` (the PV/PVC volume source + the `storage/v1` provisioner contract). `darwin-net:M3` NodePort needs **no** `apis` edge (`net/v1 ServicePort.NodePort` already exists).
- **Mesh** — `darwin-net:M3` (wireguard mesh) + `k3sm:M3` (join / mesh-enroll write) ← **`apis:M3.2`** (the `MeshPeer` CRD + the mesh-enroll join payloads), **NOT** `apis:M3.1`. `NodeNetwork` is a recorded no-op (no edge).
- `runtimed:M5` + `darwin-net:M5` ← `apis:M5.1` (the `vm` handler→backend mapping).
- **CI helper** — `runtimed`, `darwin-net`, and `k3sm` all import `apis:M7.2`'s `k3smtest.SkipUnless` (the only DAG-legal home for a helper all three consume); the per-repo `ci.yml` files are independent (no cross-repo edge).
- **MLX** — `runtimed:M8.2` (Metal SBPL + egress + `GPUFacts`) + `k3sm:M8.3` (node advertisement + translate + VAP) + `k3sm:M8.5` (the `pkg/mlx` operator) ← **`apis:M8.1`** (the `MLXModel` CRD, the `allow_gpu`/`allow_internet_egress`/`GPUFacts` proto fields, the `config/crd` embed, and the constants). `darwin-net` has **no** M8 work (machine-checked via the S1 exit criterion). The egress annotation constant lives in a runtime-area `apis` package, not `mlx/v1alpha1`.
- **Native sidecars** — `k3sm:M10` (translate wires the sidecar `restartPolicy:Always` signal) + `runtimed:M10` (the sidecar container lifecycle — stays-running + reverse-order teardown) ← **`apis:M10.2`** (the additive `Container.restart_policy` field). The provider↔runtimed split is a **same-binary, same-node hard cut**, but the field ships **consumer-first** per the apis-proto named exception (tolerant readers first, then producer). Every other M10 conformance type is an upstream object the embedded real apiserver already serves — **no `apis` edge**.

## Next
**M3 is closed (Wave 1)** — `M3.1` (PV/PVC volume source + the `storage/v1` provisioner contract) and
`M3.2` (the `MeshPeer` CRD + mesh-enroll join payloads) both landed, additive-only with stable field
numbers. M3.1 + M3.2 are the **only** `apis` M3 work (the re-plan's `apis` Wave 1 — there is no `apis`
M3.3). This unblocks the downstream M3 round (a **separate** follow-up, not `apis` work):
- `runtimed:M3` binds `PodBox.volumes[].persistent_volume_claim` to a stable per-PVC APFS dir (resolving
  the path via `storage/v1` `DataDir`, `ReclaimPolicy: Retain`); `k3sm:M3` runs the local-path provisioner
  (PV name via `PVName`, `nodeAffinity` via `NodeTopology`, `WaitForFirstConsumer`) + StatefulSet.
- `darwin-net:M3` watches the `MeshPeer` CRD and reconciles the wireguard mesh; `k3sm:M3` mints the join,
  writes each node's own `MeshPeer`, and serves the mesh-enroll exchange (`MeshEnrollRequest`/`Response`).

**M4** is the next `apis` milestone (API-stability freeze): freeze the `v1` protos + the `net.k3sm.io`
CRDs (now incl. `MeshPeer`) for public consumption and resolve the vanity import path.

**M7–M8 are now encoded as ledger stubs** (all `todo`, `done:false`/`met:false`) — planned work, no code
in this change:
- **M7.2** (`apis` slice of the public-launch release engineering) — the public `ci.yml` (thin `hack/ci.sh`
  wrapper) and the shared `k3smtest.SkipUnless` helper + capability taxonomy that `runtimed`/`darwin-net`/
  `k3sm` all import (`apis` is the DAG-legal home).
- **M8.1** (`apis` Wave 1 of MLX) — the alpha `MLXModel` CRD (`mlx/v1alpha1`), the
  `allow_gpu`/`allow_internet_egress`/`GPUFacts` proto fields carved from the reserved bands (reserving 101),
  the `config/crd` `go:embed` package, the exported constants (egress annotation
  in a runtime-area package), and the DESIGN §5a/§5b + privilege-model doc edits. The
  `spec.distributed` CEL test lives in `k3sm`, not `apis` (keeps `apis`'s graph minimal).

## M11 — Linux containers & multi-arch (apis slice) ✅

**M11.1 ✅ DONE 2026-08-29** (apis#32 — guest/v1 GuestAgent contract + the three runtime/v1 carves + label constants; a1 proven vs the pre-carve baseline, a2 goldens mutation-checked, a3 module-graph clean; d4 verified pre-encoded by the W0 roadmap PR).
`apis` is **Wave 1** of M11 (`docs/m11-plan.md` is authoritative; new API surface is human-reviewed).
**Hard cut** — additive only: a **new proto package** (`guest/v1`, zero existing consumers) + carves
from the **explicitly reserved** `runtime/v1` bands (`Container 100..149`, `ImageManifest 100..149`)
+ the next free `Volume` source number. `guest/v1` crosses the runtimed↔initramfs boundary (an
independently-shipped pinned artifact) so it is a versioned wire contract: additive-only stability
header, buf-baseline discipline, `api_version` handshake on `Health`; compat = lockstep via the
in-code initramfs sha256 pin (`--guest-artifacts-dir` is unsupported skew).

### M11.1 — guest/v1 contract + platform/hostPath carves + Rosetta constants ✅
**Deliverables**
- ✅ `M11.1-d1` **`k3sm.io/apis/guest/v1`** — the whole VM contract family in one home (deliberately no `vm/v1`): `GuestAgent` (Health/ContainerEvents/Exec/Logs (reusing `runtime/v1` stream messages, single-pod-guest semantics documented)/Stats/Stop), `GuestSpec` (`guest-spec.json` = its proto-JSON), `VMHostSpec` (`vmhost.spec.json` = its proto-JSON). Placement rationale + compat posture in the package doc; `doc.go` required.
- ✅ `M11.1-d2` **`runtime/v1` carves** (additive; re-narrow each reserved range + rewrite the band comments honestly). *Re-scoped 2026-08-29* — **remaining** (B127 landed `ImageManifest.platform = 100` + `index_digest = 101` on 2026-08-10; M12.1's `image_pull_policy` already occupies `Container` 101): `Container.image_platform = 100` (the pinned number — the band comment reserves it by name; the existing `Platform` message, never a string; rewrite the band comment, re-carve `reserved 102 to 149` minus 101); `Volume.host_path = 8` + `HostPathVolumeSource{path,type}` as a **verbatim corev1 mirror, consumer-less** (guest hostPath fail-closed until human-gated B98 — m11-plan Res. 1); and the `Container.command` doc comment gains the **discriminator** (absolute path ⇒ M0 host-binary convention; OCI ref ⇒ image-config merge). The `ImageManifest` half of the old text is **done-by-B127** — do not re-carve it.
- ✅ `M11.1-d3` **Constants**: `k3sm.io/rosetta`, `k3sm.io/rosetta-linux` (composition documented), `k3sm.io/image-platform` (pod-level annotation → per-container stamp). No string literals in consumers.
- ✅ `M11.1-d4` **DESIGN edits** (m8-Res.19 pattern): §5a multi-arch pull sentence + §5c/§6 third-entitled-artifact (`k3sm-vmhost`) prose — pre-encoded by the roadmap PR, kept truthful by this wave.

**Acceptance (exit gate)**
- ✅ `M11.1-a1` `buf breaking` vs the **pre-carve committed baseline first**, then baseline regenerated; `buf generate` no-diff; `apis/hack/ci.sh` green — *method: unit*
- ✅ `M11.1-a2` `guest/v1` round-trip + GuestSpec/VMHostSpec proto-JSON goldens; `Health.api_version` present; `CGO_ENABLED=0` standalone + go.work builds; `-race` clean — *method: unit*
- ✅ `M11.1-a3` zero `k3sm.io/*` imports; platform fields are the `Platform` message; `host_path` mirrors corev1 exactly; **no** `SandboxProfile.guest_network` — *method: build*

**Recorded non-changes**: no `SandboxProfile.guest_network` (B6 rides the in-process `runtimed.Deps`
podnet adapter, provider-side `SetupGuest` **before** `toPodBox` — M10.1 one-authority ordering; the
102..149 band keeps its earmark); no default-platform field (policy derives runtimed-side); Rosetta
advertisement rides the existing `RuntimeCondition` repeated field (the B1 precedent).
