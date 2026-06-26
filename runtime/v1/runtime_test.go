package runtimev1

import (
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// fixedTime is a deterministic instant used across the round-trip cases so
// timestamp fields are populated (and stable) on the wire.
var fixedTime = time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

// roundTrip marshals m, unmarshals into a fresh message of the same type, and
// asserts proto.Equal. We compare by proto.Equal (semantic), NOT byte identity,
// because proto3 wire output is not canonical (map ordering, etc.).
func roundTrip[M proto.Message](t *testing.T, name string, m M, fresh M) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		b, err := proto.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := proto.Unmarshal(b, fresh); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !proto.Equal(m, fresh) {
			t.Fatalf("round-trip mismatch:\n got: %v\nwant: %v", fresh, m)
		}
	})
}

// TestRoundTrip exercises a populated value of every generated message and
// asserts a lossless marshal→unmarshal cycle. Each case fills every field
// (incl. nested oneof-style unions and maps) so the round-trip actually covers
// the wire surface.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	ts := timestamppb.New(fixedTime)
	errStatus := &status.Status{Code: 9, Message: "failed precondition"}

	roundTrip(t, "EnvVar", &EnvVar{Name: "PATH", Value: "/usr/bin"}, &EnvVar{})

	roundTrip(t, "Container", &Container{
		Name:       "app",
		Image:      "registry.example/app@sha256:abc",
		Command:    []string{"/app"},
		Args:       []string{"--flag", "v"},
		WorkingDir: "/app",
		Env:        []*EnvVar{{Name: "K", Value: "V"}},
		Tty:        true,
		Stdin:      true,
	}, &Container{})

	roundTrip(t, "SandboxProfile", &SandboxProfile{
		Backend:         SandboxBackend_SANDBOX_BACKEND_SEATBELT_INPROC,
		DataVolumePath:  "/var/lib/k3sm/pods/p1/rootfs",
		ExtraReadPaths:  []string{"/opt/data"},
		ExtraWritePaths: []string{"/var/log/app"},
		AllowNetwork:    true,
	}, &SandboxProfile{})

	roundTrip(t, "PodBox", &PodBox{
		PodId:           "11111111-2222-3333-4444-555555555555",
		Namespace:       "default",
		Name:            "hello",
		RootfsPath:      "/var/lib/k3sm/pods/p1/rootfs",
		Uid:             501,
		Gid:             20,
		PodIp:           "100.64.0.7",
		PodIps:          []string{"100.64.0.7"},
		InitContainers:  []*Container{{Name: "init", Image: "/bin/true"}},
		Containers:      []*Container{{Name: "app", Image: "/app", Command: []string{"/app"}}},
		SandboxProfile:  &SandboxProfile{Backend: SandboxBackend_SANDBOX_BACKEND_SEATBELT_EXEC, DataVolumePath: "/var/lib/k3sm/pods/p1/rootfs"},
		SignaturePolicy: SignaturePolicy_SIGNATURE_POLICY_ADHOC_OK,
		Labels:          map[string]string{"app": "hello"},
		Annotations:     map[string]string{"k3sm.io/profile": "default"},
	}, &PodBox{})

	roundTrip(t, "Descriptor", &Descriptor{
		MediaType:   "application/vnd.oci.image.layer.v1.tar+gzip",
		Digest:      "sha256:deadbeef",
		Size:        4096,
		Urls:        []string{"https://mirror.example/blob"},
		Annotations: map[string]string{"org.opencontainers.image.title": "layer"},
		Platform:    &Platform{Os: "darwin", Architecture: "arm64", Variant: "v8", OsVersion: "26.0"},
	}, &Descriptor{})

	roundTrip(t, "Platform", &Platform{Os: "darwin", Architecture: "arm64", Variant: "v8", OsVersion: "26.0"}, &Platform{})

	roundTrip(t, "ImageManifest", &ImageManifest{
		Reference:   "registry.example/app:1.0",
		MediaType:   "application/vnd.oci.image.manifest.v1+json",
		Config:      &Descriptor{MediaType: "application/vnd.oci.image.config.v1+json", Digest: "sha256:cfg", Size: 512},
		Layers:      []*Descriptor{{MediaType: "application/vnd.oci.image.layer.v1.tar+gzip", Digest: "sha256:l1", Size: 1024}},
		Annotations: map[string]string{"k": "v"},
	}, &ImageManifest{})

	roundTrip(t, "ContainerStateWaiting", &ContainerStateWaiting{Reason: "StartError", Message: "boom"}, &ContainerStateWaiting{})
	roundTrip(t, "ContainerStateRunning", &ContainerStateRunning{StartedAt: ts}, &ContainerStateRunning{})
	roundTrip(t, "ContainerStateTerminated", &ContainerStateTerminated{
		ExitCode: 137, Signal: 9, Reason: "OOMKilled", Message: "mem", StartedAt: ts, FinishedAt: ts, ContainerId: "c1",
	}, &ContainerStateTerminated{})

	roundTrip(t, "ContainerState_running", &ContainerState{Running: &ContainerStateRunning{StartedAt: ts}}, &ContainerState{})
	roundTrip(t, "ContainerState_terminated", &ContainerState{Terminated: &ContainerStateTerminated{ExitCode: 1, Reason: "Error", StartedAt: ts, FinishedAt: ts}}, &ContainerState{})
	roundTrip(t, "ContainerState_waiting", &ContainerState{Waiting: &ContainerStateWaiting{Reason: "Pending"}}, &ContainerState{})

	roundTrip(t, "ContainerStatus", &ContainerStatus{
		Name:                 "app",
		State:                &ContainerState{Running: &ContainerStateRunning{StartedAt: ts}},
		LastTerminationState: &ContainerState{Terminated: &ContainerStateTerminated{ExitCode: 1}},
		Ready:                true,
		RestartCount:         2,
		Image:                "/app",
		ImageId:              "/app",
		ContainerId:          "c1",
		Started:              true,
		StartedSet:           true,
	}, &ContainerStatus{})

	roundTrip(t, "PodCondition", &PodCondition{
		Type:               "Ready",
		Status:             ConditionStatus_CONDITION_STATUS_TRUE,
		LastProbeTime:      ts,
		LastTransitionTime: ts,
		Reason:             "PodReady",
		Message:            "ok",
	}, &PodCondition{})

	roundTrip(t, "PodStatus", &PodStatus{
		PodId: "11111111-2222-3333-4444-555555555555",
		Phase: PodPhase_POD_PHASE_RUNNING,
		Conditions: []*PodCondition{
			{Type: "Initialized", Status: ConditionStatus_CONDITION_STATUS_TRUE, LastTransitionTime: ts},
			{Type: "Ready", Status: ConditionStatus_CONDITION_STATUS_TRUE, LastTransitionTime: ts},
			{Type: "ContainersReady", Status: ConditionStatus_CONDITION_STATUS_TRUE},
			{Type: "PodScheduled", Status: ConditionStatus_CONDITION_STATUS_TRUE},
		},
		Message:           "Running",
		Reason:            "",
		HostIp:            "192.168.1.10",
		HostIps:           []string{"192.168.1.10"},
		PodIp:             "100.64.0.7",
		PodIps:            []string{"100.64.0.7"},
		StartTime:         ts,
		QosClass:          "BestEffort",
		NominatedNodeName: "",
		InitContainerStatuses: []*ContainerStatus{
			{Name: "init", Ready: false, State: &ContainerState{Terminated: &ContainerStateTerminated{ExitCode: 0, Reason: "Completed", StartedAt: ts, FinishedAt: ts}}},
		},
		ContainerStatuses: []*ContainerStatus{
			{Name: "app", Ready: true, Started: true, StartedSet: true, State: &ContainerState{Running: &ContainerStateRunning{StartedAt: ts}}},
		},
		EphemeralContainerStatuses: []*ContainerStatus{
			{Name: "debugger", State: &ContainerState{Waiting: &ContainerStateWaiting{Reason: "ContainerCreating"}}},
		},
	}, &PodStatus{})

	roundTrip(t, "CreatePodRequest", &CreatePodRequest{Pod: &PodBox{PodId: "p1", Name: "n"}}, &CreatePodRequest{})
	roundTrip(t, "CreatePodResponse", &CreatePodResponse{
		Status:        &PodStatus{PodId: "p1", Phase: PodPhase_POD_PHASE_PENDING},
		Error:         errStatus,
		FailureReason: FailureReason_FAILURE_REASON_SIGNATURE_REJECTED,
	}, &CreatePodResponse{})

	roundTrip(t, "DeletePodRequest", &DeletePodRequest{PodId: "p1", GracePeriodSeconds: 30}, &DeletePodRequest{})
	roundTrip(t, "DeletePodResponse", &DeletePodResponse{Error: errStatus, FailureReason: FailureReason_FAILURE_REASON_NOT_FOUND}, &DeletePodResponse{})

	roundTrip(t, "UpdatePodRequest", &UpdatePodRequest{Pod: &PodBox{PodId: "p1"}}, &UpdatePodRequest{})
	roundTrip(t, "UpdatePodResponse", &UpdatePodResponse{Status: &PodStatus{PodId: "p1"}, FailureReason: FailureReason_FAILURE_REASON_NOT_UPDATABLE}, &UpdatePodResponse{})

	roundTrip(t, "WatchPodStatusRequest", &WatchPodStatusRequest{PodId: "p1"}, &WatchPodStatusRequest{})
	roundTrip(t, "PodStatusEvent", &PodStatusEvent{Type: PodStatusEventType_POD_STATUS_EVENT_TYPE_MODIFIED, Status: &PodStatus{PodId: "p1", Phase: PodPhase_POD_PHASE_RUNNING}}, &PodStatusEvent{})

	roundTrip(t, "GetPodStatusRequest", &GetPodStatusRequest{PodId: "p1"}, &GetPodStatusRequest{})
	roundTrip(t, "GetPodStatusResponse", &GetPodStatusResponse{Status: &PodStatus{PodId: "p1"}, Error: errStatus}, &GetPodStatusResponse{})

	roundTrip(t, "GetLogsRequest", &GetLogsRequest{
		PodId: "p1", Container: "app", Follow: true, TailLines: 100, SinceTime: ts, Timestamps: true, Previous: true, LimitBytes: 1 << 20,
	}, &GetLogsRequest{})
	roundTrip(t, "LogEntry", &LogEntry{Line: []byte("hello\n"), Timestamp: ts, Stream: LogStream_LOG_STREAM_STDERR}, &LogEntry{})

	roundTrip(t, "ExecRequest", &ExecRequest{
		PodId: "p1", Container: "app", Command: []string{"sh", "-c", "echo hi"}, Tty: true, Stdin: true, Stdout: true, Stderr: true,
		StdinData: []byte("input"), Resize: &TerminalSize{Width: 80, Height: 24},
	}, &ExecRequest{})
	roundTrip(t, "ExecResponse", &ExecResponse{Stdout: []byte("out"), Stderr: []byte("err"), Exit: &ExecResult{ExitCode: 0}}, &ExecResponse{})
	roundTrip(t, "ExecResult", &ExecResult{ExitCode: 2, Error: errStatus}, &ExecResult{})

	roundTrip(t, "AttachRequest", &AttachRequest{PodId: "p1", Container: "app", Stdin: true, Stdout: true, Stderr: true, Tty: true, StdinData: []byte("x"), Resize: &TerminalSize{Width: 100, Height: 40}}, &AttachRequest{})
	roundTrip(t, "AttachResponse", &AttachResponse{Stdout: []byte("o"), Stderr: []byte("e"), Exit: &ExecResult{ExitCode: 0}}, &AttachResponse{})

	roundTrip(t, "PortForwardRequest", &PortForwardRequest{PodId: "p1", Port: 8080, ConnectionId: 7, Data: []byte("GET /"), Close: true}, &PortForwardRequest{})
	roundTrip(t, "PortForwardResponse", &PortForwardResponse{ConnectionId: 7, Data: []byte("200 OK"), Close: true, Error: errStatus}, &PortForwardResponse{})

	roundTrip(t, "TerminalSize", &TerminalSize{Width: 132, Height: 50}, &TerminalSize{})

	roundTrip(t, "GetRuntimeInfoRequest", &GetRuntimeInfoRequest{}, &GetRuntimeInfoRequest{})
	roundTrip(t, "GetRuntimeInfoResponse", &GetRuntimeInfoResponse{
		RuntimeName: "k3sm-runtimed", RuntimeVersion: "1.0.0", ApiVersion: "runtime.v1", Healthy: true,
		Conditions: []*RuntimeCondition{{Type: "SandboxReady", Status: ConditionStatus_CONDITION_STATUS_TRUE, Reason: "OK"}},
	}, &GetRuntimeInfoResponse{})
	roundTrip(t, "RuntimeCondition", &RuntimeCondition{Type: "ImageStoreReady", Status: ConditionStatus_CONDITION_STATUS_FALSE, Reason: "Init", Message: "warming"}, &RuntimeCondition{})

	// --- M2.1 pod-spec fidelity additions ---------------------------------
	// One fully-populated case per new message. These are the fails-before
	// (the messages did not exist) / passes-after acceptance evidence for
	// apis:M2.1 — proto.Equal must survive marshal→unmarshal for every field.

	// Container scope: volume mounts, named ports, probes (all three handlers),
	// security context, envFrom, and env[].valueFrom.fieldRef.
	roundTrip(t, "VolumeMount", &VolumeMount{
		Name: "config", MountPath: "/etc/nats", ReadOnly: true, SubPath: "nats.conf",
	}, &VolumeMount{})

	roundTrip(t, "ContainerPort", &ContainerPort{
		Name: "http", ContainerPort: 8080, Protocol: "TCP",
	}, &ContainerPort{})

	roundTrip(t, "IntOrString_int", &IntOrString{IntVal: 8080}, &IntOrString{})
	roundTrip(t, "IntOrString_str", &IntOrString{StrVal: "http"}, &IntOrString{})

	roundTrip(t, "HTTPHeader", &HTTPHeader{Name: "X-Probe", Value: "1"}, &HTTPHeader{})
	roundTrip(t, "HTTPGetAction", &HTTPGetAction{
		Path: "/healthz", Port: &IntOrString{StrVal: "http"}, Scheme: "HTTPS", Host: "127.0.0.1",
		HttpHeaders: []*HTTPHeader{{Name: "Accept", Value: "application/json"}},
	}, &HTTPGetAction{})
	roundTrip(t, "TCPSocketAction", &TCPSocketAction{Port: &IntOrString{IntVal: 6222}, Host: "127.0.0.1"}, &TCPSocketAction{})
	roundTrip(t, "ExecAction", &ExecAction{Command: []string{"sh", "-c", "test -f /ready"}}, &ExecAction{})

	roundTrip(t, "Probe_httpGet", &Probe{
		InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 1, SuccessThreshold: 1, FailureThreshold: 3,
		HttpGet: &HTTPGetAction{Path: "/healthz", Port: &IntOrString{IntVal: 8080}, Scheme: "HTTP"},
	}, &Probe{})
	roundTrip(t, "Probe_tcpSocket", &Probe{
		InitialDelaySeconds: 0, PeriodSeconds: 5, TimeoutSeconds: 2, SuccessThreshold: 1, FailureThreshold: 6,
		TcpSocket: &TCPSocketAction{Port: &IntOrString{StrVal: "nats"}},
	}, &Probe{})
	roundTrip(t, "Probe_exec", &Probe{
		PeriodSeconds: 10, FailureThreshold: 3,
		Exec: &ExecAction{Command: []string{"/bin/true"}},
	}, &Probe{})

	roundTrip(t, "SecurityContext", &SecurityContext{
		RunAsUser: 1000, RunAsGroup: 2000, RunAsNonRoot: true,
	}, &SecurityContext{})

	roundTrip(t, "ConfigMapEnvSource", &ConfigMapEnvSource{Name: "app-config", Optional: true}, &ConfigMapEnvSource{})
	roundTrip(t, "SecretEnvSource", &SecretEnvSource{Name: "app-secret", Optional: true}, &SecretEnvSource{})
	roundTrip(t, "EnvFromSource_configMap", &EnvFromSource{Prefix: "CFG_", ConfigMapRef: &ConfigMapEnvSource{Name: "app-config"}}, &EnvFromSource{})
	roundTrip(t, "EnvFromSource_secret", &EnvFromSource{Prefix: "SEC_", SecretRef: &SecretEnvSource{Name: "app-secret"}}, &EnvFromSource{})

	roundTrip(t, "ObjectFieldSelector", &ObjectFieldSelector{ApiVersion: "v1", FieldPath: "status.podIP"}, &ObjectFieldSelector{})
	roundTrip(t, "ConfigMapKeySelector", &ConfigMapKeySelector{Name: "app-config", Key: "log.level", Optional: true}, &ConfigMapKeySelector{})
	roundTrip(t, "SecretKeySelector", &SecretKeySelector{Name: "app-secret", Key: "token", Optional: true}, &SecretKeySelector{})
	roundTrip(t, "EnvVarSource_fieldRef", &EnvVarSource{FieldRef: &ObjectFieldSelector{ApiVersion: "v1", FieldPath: "spec.nodeName"}}, &EnvVarSource{})
	roundTrip(t, "EnvVarSource_configMapKeyRef", &EnvVarSource{ConfigMapKeyRef: &ConfigMapKeySelector{Name: "app-config", Key: "k"}}, &EnvVarSource{})
	roundTrip(t, "EnvVarSource_secretKeyRef", &EnvVarSource{SecretKeyRef: &SecretKeySelector{Name: "app-secret", Key: "k"}}, &EnvVarSource{})
	roundTrip(t, "EnvVar_valueFrom", &EnvVar{
		Name: "POD_IP", ValueFrom: &EnvVarSource{FieldRef: &ObjectFieldSelector{ApiVersion: "v1", FieldPath: "status.podIP"}},
	}, &EnvVar{})

	// Pod scope: every Volume source (incl. projected.serviceAccountToken),
	// the pod-level security context (fsGroup), grace period, and pull secrets.
	roundTrip(t, "KeyToPath", &KeyToPath{Key: "nats.conf", Path: "conf/nats.conf", Mode: 0o644}, &KeyToPath{})
	roundTrip(t, "ConfigMapVolumeSource", &ConfigMapVolumeSource{
		Name: "nats-config", Items: []*KeyToPath{{Key: "nats.conf", Path: "nats.conf", Mode: 0o600}}, DefaultMode: 0o644, Optional: true,
	}, &ConfigMapVolumeSource{})
	roundTrip(t, "SecretVolumeSource", &SecretVolumeSource{
		SecretName: "git-ssh-key", Items: []*KeyToPath{{Key: "id_ed25519", Path: "ssh/id_ed25519", Mode: 0o400}}, DefaultMode: 0o400, Optional: false,
	}, &SecretVolumeSource{})
	roundTrip(t, "EmptyDirVolumeSource", &EmptyDirVolumeSource{Medium: "Memory", SizeLimit: "1Gi"}, &EmptyDirVolumeSource{})
	roundTrip(t, "DownwardAPIVolumeFile", &DownwardAPIVolumeFile{
		Path: "labels", FieldRef: &ObjectFieldSelector{ApiVersion: "v1", FieldPath: "metadata.labels"}, Mode: 0o644,
	}, &DownwardAPIVolumeFile{})
	roundTrip(t, "DownwardAPIVolumeSource", &DownwardAPIVolumeSource{
		Items:       []*DownwardAPIVolumeFile{{Path: "podName", FieldRef: &ObjectFieldSelector{FieldPath: "metadata.name"}}},
		DefaultMode: 0o644,
	}, &DownwardAPIVolumeSource{})
	roundTrip(t, "ServiceAccountTokenProjection", &ServiceAccountTokenProjection{
		Audience: "https://kubernetes.default.svc", ExpirationSeconds: 3600, Path: "token",
	}, &ServiceAccountTokenProjection{})
	roundTrip(t, "ConfigMapProjection", &ConfigMapProjection{Name: "ca", Items: []*KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}, Optional: true}, &ConfigMapProjection{})
	roundTrip(t, "SecretProjection", &SecretProjection{Name: "tls", Items: []*KeyToPath{{Key: "tls.crt", Path: "tls.crt"}}, Optional: false}, &SecretProjection{})
	roundTrip(t, "DownwardAPIProjection", &DownwardAPIProjection{
		Items: []*DownwardAPIVolumeFile{{Path: "ns", FieldRef: &ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
	}, &DownwardAPIProjection{})
	roundTrip(t, "VolumeProjection_configMap", &VolumeProjection{ConfigMap: &ConfigMapProjection{Name: "ca"}}, &VolumeProjection{})
	roundTrip(t, "VolumeProjection_secret", &VolumeProjection{Secret: &SecretProjection{Name: "tls"}}, &VolumeProjection{})
	roundTrip(t, "VolumeProjection_downwardAPI", &VolumeProjection{DownwardApi: &DownwardAPIProjection{Items: []*DownwardAPIVolumeFile{{Path: "name", FieldRef: &ObjectFieldSelector{FieldPath: "metadata.name"}}}}}, &VolumeProjection{})
	roundTrip(t, "VolumeProjection_serviceAccountToken", &VolumeProjection{
		ServiceAccountToken: &ServiceAccountTokenProjection{Audience: "api", ExpirationSeconds: 3600, Path: "token"},
	}, &VolumeProjection{})
	roundTrip(t, "ProjectedVolumeSource", &ProjectedVolumeSource{
		Sources: []*VolumeProjection{
			{ServiceAccountToken: &ServiceAccountTokenProjection{Audience: "https://kubernetes.default.svc", ExpirationSeconds: 3600, Path: "token"}},
			{ConfigMap: &ConfigMapProjection{Name: "kube-root-ca.crt", Items: []*KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
			{DownwardApi: &DownwardAPIProjection{Items: []*DownwardAPIVolumeFile{{Path: "namespace", FieldRef: &ObjectFieldSelector{FieldPath: "metadata.namespace"}}}}},
		},
		DefaultMode: 0o644,
	}, &ProjectedVolumeSource{})

	roundTrip(t, "Volume_configMap", &Volume{Name: "config", ConfigMap: &ConfigMapVolumeSource{Name: "nats-config"}}, &Volume{})
	roundTrip(t, "Volume_secret", &Volume{Name: "ssh", Secret: &SecretVolumeSource{SecretName: "git-ssh-key"}}, &Volume{})
	roundTrip(t, "Volume_emptyDir", &Volume{Name: "shm", EmptyDir: &EmptyDirVolumeSource{Medium: "Memory", SizeLimit: "256Mi"}}, &Volume{})
	roundTrip(t, "Volume_downwardAPI", &Volume{Name: "podinfo", DownwardApi: &DownwardAPIVolumeSource{Items: []*DownwardAPIVolumeFile{{Path: "name", FieldRef: &ObjectFieldSelector{FieldPath: "metadata.name"}}}}}, &Volume{})
	roundTrip(t, "Volume_projected", &Volume{Name: "kube-api-access", Projected: &ProjectedVolumeSource{
		Sources: []*VolumeProjection{{ServiceAccountToken: &ServiceAccountTokenProjection{Audience: "https://kubernetes.default.svc", ExpirationSeconds: 3600, Path: "token"}}},
	}}, &Volume{})

	roundTrip(t, "PodSecurityContext", &PodSecurityContext{FsGroup: 999, RunAsUser: 1000, RunAsGroup: 2000}, &PodSecurityContext{})
	roundTrip(t, "LocalObjectReference", &LocalObjectReference{Name: "regcred"}, &LocalObjectReference{})

	// ContainerStatus lossless-mirror additions: volume mount status + the
	// effective ContainerUser (resolved uid/gid/supplemental groups).
	roundTrip(t, "VolumeMountStatus", &VolumeMountStatus{Name: "config", MountPath: "/etc/nats", ReadOnly: true}, &VolumeMountStatus{})
	roundTrip(t, "LinuxContainerUser", &LinuxContainerUser{Uid: 1000, Gid: 2000, SupplementalGroups: []int64{999, 1000}}, &LinuxContainerUser{})
	roundTrip(t, "ContainerUser", &ContainerUser{Linux: &LinuxContainerUser{Uid: 1000, Gid: 2000, SupplementalGroups: []int64{999}}}, &ContainerUser{})

	// A Container populated with every M2.1 field at once (named-port table +
	// probes referencing it by name + mounts + envFrom + valueFrom + secctx).
	roundTrip(t, "Container_full_M2_1", &Container{
		Name:    "app",
		Image:   "registry.example/app@sha256:abc",
		Command: []string{"/app"},
		Env: []*EnvVar{
			{Name: "STATIC", Value: "v"},
			{Name: "NODE", ValueFrom: &EnvVarSource{FieldRef: &ObjectFieldSelector{ApiVersion: "v1", FieldPath: "spec.nodeName"}}},
			{Name: "LOG", ValueFrom: &EnvVarSource{ConfigMapKeyRef: &ConfigMapKeySelector{Name: "app-config", Key: "log.level"}}},
		},
		VolumeMounts: []*VolumeMount{
			{Name: "config", MountPath: "/etc/nats", ReadOnly: true},
			{Name: "kube-api-access", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true},
		},
		Ports: []*ContainerPort{
			{Name: "http", ContainerPort: 8080, Protocol: "TCP"},
			{Name: "nats", ContainerPort: 4222, Protocol: "TCP"},
		},
		LivenessProbe:   &Probe{PeriodSeconds: 10, FailureThreshold: 3, HttpGet: &HTTPGetAction{Path: "/healthz", Port: &IntOrString{StrVal: "http"}}},
		ReadinessProbe:  &Probe{PeriodSeconds: 5, FailureThreshold: 3, TcpSocket: &TCPSocketAction{Port: &IntOrString{StrVal: "nats"}}},
		StartupProbe:    &Probe{PeriodSeconds: 2, FailureThreshold: 30, Exec: &ExecAction{Command: []string{"/bin/true"}}},
		SecurityContext: &SecurityContext{RunAsUser: 1000, RunAsGroup: 2000, RunAsNonRoot: true},
		EnvFrom: []*EnvFromSource{
			{Prefix: "CFG_", ConfigMapRef: &ConfigMapEnvSource{Name: "app-config"}},
			{SecretRef: &SecretEnvSource{Name: "app-secret", Optional: true}},
		},
	}, &Container{})

	// A PodBox populated with every M2.1 pod-scope field at once.
	roundTrip(t, "PodBox_full_M2_1", &PodBox{
		PodId:      "11111111-2222-3333-4444-555555555555",
		Namespace:  "default",
		Name:       "stockkitty",
		RootfsPath: "/var/lib/k3sm/pods/p1/rootfs",
		Uid:        501,
		Gid:        20,
		Containers: []*Container{{Name: "app", Image: "/app"}},
		Volumes: []*Volume{
			{Name: "config", ConfigMap: &ConfigMapVolumeSource{Name: "nats-config"}},
			{Name: "ssh", Secret: &SecretVolumeSource{SecretName: "git-ssh-key", DefaultMode: 0o400}},
			{Name: "shm", EmptyDir: &EmptyDirVolumeSource{Medium: "Memory", SizeLimit: "256Mi"}},
			{Name: "podinfo", DownwardApi: &DownwardAPIVolumeSource{Items: []*DownwardAPIVolumeFile{{Path: "name", FieldRef: &ObjectFieldSelector{FieldPath: "metadata.name"}}}}},
			{Name: "kube-api-access", Projected: &ProjectedVolumeSource{Sources: []*VolumeProjection{
				{ServiceAccountToken: &ServiceAccountTokenProjection{Audience: "https://kubernetes.default.svc", ExpirationSeconds: 3600, Path: "token"}},
				{ConfigMap: &ConfigMapProjection{Name: "kube-root-ca.crt", Items: []*KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
			}}},
		},
		PodSecurityContext:            &PodSecurityContext{FsGroup: 999, RunAsUser: 1000, RunAsGroup: 2000},
		TerminationGracePeriodSeconds: 30,
		ImagePullSecrets:              []*LocalObjectReference{{Name: "regcred"}},
	}, &PodBox{})

	// A ContainerStatus populated with the new lossless-mirror fields.
	roundTrip(t, "ContainerStatus_full_M2_1", &ContainerStatus{
		Name:         "app",
		State:        &ContainerState{Running: &ContainerStateRunning{StartedAt: ts}},
		Ready:        true,
		RestartCount: 0,
		Image:        "/app",
		ImageId:      "/app",
		ContainerId:  "c1",
		Started:      true,
		StartedSet:   true,
		VolumeMounts: []*VolumeMountStatus{
			{Name: "config", MountPath: "/etc/nats", ReadOnly: true},
			{Name: "kube-api-access", MountPath: "/var/run/secrets/kubernetes.io/serviceaccount", ReadOnly: true},
		},
		User: &ContainerUser{Linux: &LinuxContainerUser{Uid: 1000, Gid: 2000, SupplementalGroups: []int64{999, 1000}}},
	}, &ContainerStatus{})

	// --- M2.2 resource limits + metrics additions ----------------------------
	// One fully-populated case per new message/field. These are the fails-before
	// (the messages/fields did not exist) / passes-after acceptance evidence for
	// apis:M2.2 — proto.Equal must survive marshal→unmarshal for every new field.

	roundTrip(t, "ResourceLimit", &ResourceLimit{Type: "RLIMIT_NOFILE", Soft: 1024, Hard: 4096}, &ResourceLimit{})
	roundTrip(t, "ResourceList", &ResourceList{Quantities: map[string]string{"cpu": "500m", "memory": "1Gi"}}, &ResourceList{})
	roundTrip(t, "ResourceRequirements", &ResourceRequirements{
		Limits:   &ResourceList{Quantities: map[string]string{"cpu": "1", "memory": "1Gi"}},
		Requests: &ResourceList{Quantities: map[string]string{"cpu": "500m", "memory": "512Mi"}},
	}, &ResourceRequirements{})

	roundTrip(t, "CPUStats", &CPUStats{Timestamp: ts, UsageNanoCores: 250_000_000, UsageCoreNanoSeconds: 1_234_567_890}, &CPUStats{})
	roundTrip(t, "MemoryStats", &MemoryStats{Timestamp: ts, WorkingSetBytes: 64 << 20, UsageBytes: 80 << 20, RssBytes: 48 << 20}, &MemoryStats{})
	roundTrip(t, "ContainerStats", &ContainerStats{
		Name: "app", Timestamp: ts,
		Cpu:    &CPUStats{Timestamp: ts, UsageNanoCores: 100_000_000, UsageCoreNanoSeconds: 5_000_000_000},
		Memory: &MemoryStats{Timestamp: ts, WorkingSetBytes: 32 << 20},
	}, &ContainerStats{})
	roundTrip(t, "PodStats", &PodStats{
		PodId: "11111111-2222-3333-4444-555555555555", Namespace: "default", Name: "stockkitty", Timestamp: ts,
		Cpu:    &CPUStats{Timestamp: ts, UsageNanoCores: 250_000_000, UsageCoreNanoSeconds: 9_000_000_000},
		Memory: &MemoryStats{Timestamp: ts, WorkingSetBytes: 96 << 20, UsageBytes: 128 << 20, RssBytes: 72 << 20},
		Containers: []*ContainerStats{
			{Name: "app", Timestamp: ts, Cpu: &CPUStats{UsageNanoCores: 150_000_000}, Memory: &MemoryStats{WorkingSetBytes: 64 << 20}},
			{Name: "sidecar", Timestamp: ts, Cpu: &CPUStats{UsageNanoCores: 100_000_000}, Memory: &MemoryStats{WorkingSetBytes: 32 << 20}},
		},
	}, &PodStats{})

	// PodBox carrying every M2.2 resource-limit field (the typed memory limit +
	// QoS class + rlimits that replace the M2.1 k3sm.io/memory-limit-bytes
	// annotation seam runtimed bridged the limit on).
	roundTrip(t, "PodBox_resources_M2_2", &PodBox{
		PodId:            "11111111-2222-3333-4444-555555555555",
		Name:             "stockkitty",
		RootfsPath:       "/var/lib/k3sm/pods/p1/rootfs",
		MemoryLimitBytes: 512 << 20,
		QosClass:         QOSClass_QOS_CLASS_BURSTABLE,
		Rlimits: []*ResourceLimit{
			{Type: "RLIMIT_NOFILE", Soft: 1024, Hard: 4096},
			{Type: "RLIMIT_NPROC", Soft: 256, Hard: 512},
		},
	}, &PodBox{})

	// ContainerStatus carrying the M2.2 resource mirror (resources +
	// allocatedResources) — completes the lossless corev1 mirror.
	roundTrip(t, "ContainerStatus_resources_M2_2", &ContainerStatus{
		Name:  "app",
		State: &ContainerState{Running: &ContainerStateRunning{StartedAt: ts}},
		Ready: true,
		Resources: &ResourceRequirements{
			Limits:   &ResourceList{Quantities: map[string]string{"cpu": "1", "memory": "512Mi"}},
			Requests: &ResourceList{Quantities: map[string]string{"cpu": "250m", "memory": "256Mi"}},
		},
		AllocatedResources: &ResourceList{Quantities: map[string]string{"cpu": "250m", "memory": "256Mi"}},
	}, &ContainerStatus{})

	// M2.2 RPC request/response messages (the metrics + restart wire surface).
	roundTrip(t, "ListPodStatsRequest", &ListPodStatsRequest{PodId: "p1"}, &ListPodStatsRequest{})
	roundTrip(t, "ListPodStatsResponse", &ListPodStatsResponse{PodStats: []*PodStats{
		{PodId: "p1", Namespace: "default", Name: "n", Timestamp: ts, Memory: &MemoryStats{WorkingSetBytes: 16 << 20}},
	}}, &ListPodStatsResponse{})
	roundTrip(t, "RestartContainerRequest", &RestartContainerRequest{
		PodId: "p1", Container: "app", Reason: "liveness probe failed", GracePeriodSeconds: 30,
	}, &RestartContainerRequest{})
	roundTrip(t, "RestartContainerResponse", &RestartContainerResponse{
		Status:        &ContainerStatus{Name: "app", RestartCount: 1, State: &ContainerState{Running: &ContainerStateRunning{StartedAt: ts}}},
		Error:         errStatus,
		FailureReason: FailureReason_FAILURE_REASON_NOT_FOUND,
	}, &RestartContainerResponse{})

	// --- M3.1 persistent-storage volume source --------------------------------
	// The PV/PVC volume source added to the Volume union (field 7) — the
	// fails-before (the message did not exist) / passes-after acceptance evidence
	// for apis:M3.1. proto.Equal must survive marshal→unmarshal.
	roundTrip(t, "PersistentVolumeClaimVolumeSource", &PersistentVolumeClaimVolumeSource{
		ClaimName: "postgres-data", ReadOnly: false,
	}, &PersistentVolumeClaimVolumeSource{})
	roundTrip(t, "PersistentVolumeClaimVolumeSource_readOnly", &PersistentVolumeClaimVolumeSource{
		ClaimName: "shared-models", ReadOnly: true,
	}, &PersistentVolumeClaimVolumeSource{})
	roundTrip(t, "Volume_persistentVolumeClaim", &Volume{
		Name: "data", PersistentVolumeClaim: &PersistentVolumeClaimVolumeSource{ClaimName: "postgres-data"},
	}, &Volume{})

	// A PodBox carrying a PVC-backed volume alongside an ephemeral M2.1 source
	// (the StatefulSet-storage shape stockkitty's Postgres needs), mounted by a
	// container — the cross-message M3.1 wire surface.
	roundTrip(t, "PodBox_persistentVolume_M3_1", &PodBox{
		PodId:      "11111111-2222-3333-4444-555555555555",
		Namespace:  "stockkitty",
		Name:       "postgres-0",
		RootfsPath: "/var/lib/k3sm/pods/p1/rootfs",
		Uid:        501,
		Gid:        20,
		Containers: []*Container{{
			Name:  "postgres",
			Image: "/postgres",
			VolumeMounts: []*VolumeMount{
				{Name: "data", MountPath: "/var/lib/postgresql/data"},
			},
		}},
		Volumes: []*Volume{
			{Name: "data", PersistentVolumeClaim: &PersistentVolumeClaimVolumeSource{ClaimName: "postgres-data"}},
			{Name: "shm", EmptyDir: &EmptyDirVolumeSource{Medium: "Memory", SizeLimit: "256Mi"}},
		},
	}, &PodBox{})
}

// TestSignaturePolicyFailClosed asserts the zero value of SignaturePolicy is
// UNSPECIFIED (fail-closed): a decoder seeing an unset policy must NOT treat it
// as any permissive value, and the numeric zero must be UNSPECIFIED.
func TestSignaturePolicyFailClosed(t *testing.T) {
	t.Parallel()
	if SignaturePolicy_SIGNATURE_POLICY_UNSPECIFIED != 0 {
		t.Fatalf("SignaturePolicy zero value must be UNSPECIFIED, got %d", SignaturePolicy_SIGNATURE_POLICY_UNSPECIFIED)
	}
	// A freshly-decoded PodBox (signature_policy unset on the wire) decodes to
	// the zero value, which is UNSPECIFIED — so the runtime must reject it
	// rather than run unverified code.
	got := &PodBox{}
	b, err := proto.Marshal(&PodBox{PodId: "p1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := proto.Unmarshal(b, got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SignaturePolicy != SignaturePolicy_SIGNATURE_POLICY_UNSPECIFIED {
		t.Fatalf("unset SignaturePolicy must decode to UNSPECIFIED, got %v", got.SignaturePolicy)
	}
	// The permissive policies must all be non-zero so they can never be the
	// accidental default.
	for _, p := range []SignaturePolicy{
		SignaturePolicy_SIGNATURE_POLICY_ADHOC_OK,
		SignaturePolicy_SIGNATURE_POLICY_REQUIRE_SIGNED,
		SignaturePolicy_SIGNATURE_POLICY_REQUIRE_NOTARIZED,
	} {
		if p == 0 {
			t.Fatalf("policy %v must be non-zero", p)
		}
	}
}

// TestSandboxBackendZeroValue asserts the SandboxBackend zero value is the
// "let the runtime choose" UNSPECIFIED, consistent with the fail-closed enum
// convention.
func TestSandboxBackendZeroValue(t *testing.T) {
	t.Parallel()
	if SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED != 0 {
		t.Fatalf("SandboxBackend zero value must be UNSPECIFIED, got %d", SandboxBackend_SANDBOX_BACKEND_UNSPECIFIED)
	}
}

// TestQOSClassZeroValue asserts the M2.2 QOSClass zero value is UNSPECIFIED:
// the provider must classify a pod explicitly; an unset class is "not
// classified" (runtimed treats it as BestEffort), never an accidental
// Guaranteed. The three real classes must all be non-zero.
func TestQOSClassZeroValue(t *testing.T) {
	t.Parallel()
	if QOSClass_QOS_CLASS_UNSPECIFIED != 0 {
		t.Fatalf("QOSClass zero value must be UNSPECIFIED, got %d", QOSClass_QOS_CLASS_UNSPECIFIED)
	}
	for _, q := range []QOSClass{
		QOSClass_QOS_CLASS_GUARANTEED,
		QOSClass_QOS_CLASS_BURSTABLE,
		QOSClass_QOS_CLASS_BEST_EFFORT,
	} {
		if q == 0 {
			t.Fatalf("QOSClass %v must be non-zero", q)
		}
	}
}

// TestM2_2RPCsRegistered asserts the M2.2 additive RPCs (ListPodStats,
// RestartContainer) are registered on the generated Runtime gRPC service
// descriptor as UNARY methods — the wire surface a runtimed server implements
// and the provider calls. This is the passes-after evidence the new RPCs
// reached the generated service, not just the .proto, and that they are
// request/response (not streaming).
func TestM2_2RPCsRegistered(t *testing.T) {
	t.Parallel()
	want := map[string]bool{"ListPodStats": false, "RestartContainer": false}
	for _, m := range Runtime_ServiceDesc.Methods {
		if _, ok := want[m.MethodName]; ok {
			want[m.MethodName] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("RPC %q is not registered as a unary method on Runtime_ServiceDesc", name)
		}
	}
	// Guard against a regression that turns either into a stream — the M2.2
	// contract is unary request/response.
	for _, s := range Runtime_ServiceDesc.Streams {
		if _, ok := want[s.StreamName]; ok {
			t.Errorf("RPC %q must be a unary method, but is registered as a stream", s.StreamName)
		}
	}
}
