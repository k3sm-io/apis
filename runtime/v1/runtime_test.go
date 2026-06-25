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
