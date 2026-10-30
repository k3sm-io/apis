/*
Copyright The k3sm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k3smtest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	runtimev1 "k3sm.io/apis/runtime/v1"
)

// RuntimeContract names the runtime under test for the runtime/v1 contract
// suite: the node to runtime seam, checked once, here, instead of again in
// every end-to-end test that happens to cross it.
//
// The suite checks the fields and outcomes the node acts on: the pod id on
// every status it receives, a known phase, an unknown pod reported in-band
// rather than as a transport error, an idempotent delete, and a non-empty
// runtime name. It does not check how a pod runs; that is each runtime's own
// test surface.
type RuntimeContract struct {
	// Client reaches the runtime under test, normally through ServeRuntime.
	Client runtimev1.RuntimeClient
	// Pod returns a PodBox with the given id that this runtime can create and
	// run without privilege. Ids the suite passes are lowercase letters and
	// digits only.
	Pod func(id string) *runtimev1.PodBox
}

// RuntimeContractResult is the verdict of one contract case: Err is nil when
// the runtime kept the case.
type RuntimeContractResult struct {
	Case string
	Err  error
}

// contractCase is one named check. Each case creates the pods it needs and
// deletes them before it returns, so cases are independent of each other.
type contractCase struct {
	name  string
	check func(ctx context.Context, c RuntimeContract) error
}

// contractAbsentPodID is a pod id no case ever creates.
const contractAbsentPodID = "contractabsentpod"

// contractCaseTimeout bounds one case, watch included, so a runtime that never
// answers fails the case instead of hanging the test binary.
const contractCaseTimeout = 20 * time.Second

var runtimeContractCases = []contractCase{
	{"runtime-info", checkRuntimeInfo},
	{"create", checkCreate},
	{"status", checkStatus},
	{"status-unknown", checkStatusUnknown},
	{"watch", checkWatch},
	{"stats", checkStats},
	{"delete", checkDelete},
}

// RunRuntimeContract runs every contract case as a subtest of t.
func RunRuntimeContract(t *testing.T, c RuntimeContract) {
	t.Helper()
	for _, cc := range runtimeContractCases {
		t.Run(cc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), contractCaseTimeout)
			defer cancel()
			if err := cc.check(ctx, c); err != nil {
				t.Error(err)
			}
		})
	}
}

// CheckRuntimeContract runs every contract case and returns one result per
// case, in a fixed order. It is the form a test uses to assert that a runtime
// breaks the contract.
func CheckRuntimeContract(ctx context.Context, c RuntimeContract) []RuntimeContractResult {
	out := make([]RuntimeContractResult, 0, len(runtimeContractCases))
	for _, cc := range runtimeContractCases {
		cctx, cancel := context.WithTimeout(ctx, contractCaseTimeout)
		out = append(out, RuntimeContractResult{Case: cc.name, Err: cc.check(cctx, c)})
		cancel()
	}
	return out
}

// ServeRuntime serves srv over an in-memory gRPC connection and returns a
// client for it. The server and the connection stop when t ends.
func ServeRuntime(t testing.TB, srv runtimev1.RuntimeServer) runtimev1.RuntimeClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	runtimev1.RegisterRuntimeServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	cc, err := grpc.NewClient("passthrough:///runtime",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		gs.Stop()
		t.Fatalf("dial in-memory runtime: %v", err)
	}
	t.Cleanup(func() {
		_ = cc.Close()
		gs.Stop()
	})
	return runtimev1.NewRuntimeClient(cc)
}

// createPod creates the pod with id and returns a cleanup that deletes it.
func createPod(ctx context.Context, c RuntimeContract, id string) (*runtimev1.CreatePodResponse, func(), error) {
	resp, err := c.Client.CreatePod(ctx, &runtimev1.CreatePodRequest{Pod: c.Pod(id)})
	if err != nil {
		return nil, func() {}, fmt.Errorf("CreatePod %s: %w", id, err)
	}
	cleanup := func() {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), contractCaseTimeout)
		defer cancel()
		_, _ = c.Client.DeletePod(dctx, &runtimev1.DeletePodRequest{PodId: id})
	}
	if e := resp.GetError(); e != nil && e.GetCode() != 0 {
		return nil, cleanup, fmt.Errorf("CreatePod %s refused in-band: code %d: %s (%s)",
			id, e.GetCode(), e.GetMessage(), resp.GetFailureReason())
	}
	return resp, cleanup, nil
}

// checkStatusBody holds the two fields every status the node consumes must
// carry: the pod it describes, and a phase.
func checkStatusBody(where, id string, st *runtimev1.PodStatus) error {
	if st == nil {
		return fmt.Errorf("%s: no status for pod %s", where, id)
	}
	if st.GetPodId() != id {
		return fmt.Errorf("%s: status pod_id = %q, want %q", where, st.GetPodId(), id)
	}
	if st.GetPhase() == runtimev1.PodPhase_POD_PHASE_UNSPECIFIED {
		return fmt.Errorf("%s: status for pod %s carries no phase", where, id)
	}
	return nil
}

// wantInBandNotFound asserts the shape the node reads as "the runtime does not
// hold this pod": a successful RPC carrying an in-band NotFound. A transport
// error means "no verdict", which the node handles differently.
func wantInBandNotFound(ctx context.Context, c RuntimeContract, id string) error {
	resp, err := c.Client.GetPodStatus(ctx, &runtimev1.GetPodStatusRequest{PodId: id})
	if err != nil {
		return fmt.Errorf("GetPodStatus %s: transport error %w, want an in-band NotFound", id, err)
	}
	if got := resp.GetError().GetCode(); got != int32(codes.NotFound) {
		return fmt.Errorf("GetPodStatus %s: in-band code = %d, want NotFound (%d)", id, got, codes.NotFound)
	}
	return nil
}

func checkRuntimeInfo(ctx context.Context, c RuntimeContract) error {
	info, err := c.Client.GetRuntimeInfo(ctx, &runtimev1.GetRuntimeInfoRequest{})
	if err != nil {
		return fmt.Errorf("GetRuntimeInfo: %w", err)
	}
	if info.GetRuntimeName() == "" {
		return errors.New("GetRuntimeInfo: runtime_name is empty")
	}
	return nil
}

func checkCreate(ctx context.Context, c RuntimeContract) error {
	const id = "contractcreate"
	resp, cleanup, err := createPod(ctx, c, id)
	defer cleanup()
	if err != nil {
		return err
	}
	return checkStatusBody("CreatePod", id, resp.GetStatus())
}

func checkStatus(ctx context.Context, c RuntimeContract) error {
	const id = "contractstatus"
	_, cleanup, err := createPod(ctx, c, id)
	defer cleanup()
	if err != nil {
		return err
	}
	resp, err := c.Client.GetPodStatus(ctx, &runtimev1.GetPodStatusRequest{PodId: id})
	if err != nil {
		return fmt.Errorf("GetPodStatus %s: %w", id, err)
	}
	if e := resp.GetError(); e != nil && e.GetCode() != 0 {
		return fmt.Errorf("GetPodStatus %s: in-band error code %d for a created pod: %s", id, e.GetCode(), e.GetMessage())
	}
	return checkStatusBody("GetPodStatus", id, resp.GetStatus())
}

func checkStatusUnknown(ctx context.Context, c RuntimeContract) error {
	return wantInBandNotFound(ctx, c, contractAbsentPodID)
}

func checkWatch(ctx context.Context, c RuntimeContract) error {
	const id = "contractwatch"
	_, cleanup, err := createPod(ctx, c, id)
	defer cleanup()
	if err != nil {
		return err
	}
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.Client.WatchPodStatus(wctx, &runtimev1.WatchPodStatusRequest{PodId: id})
	if err != nil {
		return fmt.Errorf("WatchPodStatus %s: %w", id, err)
	}
	// The first event is the snapshot of the already-created pod.
	ev, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("WatchPodStatus %s: no snapshot event: %w", id, err)
	}
	return checkStatusBody("WatchPodStatus", id, ev.GetStatus())
}

func checkStats(ctx context.Context, c RuntimeContract) error {
	const id = "contractstats"
	_, cleanup, err := createPod(ctx, c, id)
	defer cleanup()
	if err != nil {
		return err
	}
	resp, err := c.Client.ListPodStats(ctx, &runtimev1.ListPodStatsRequest{})
	if err != nil {
		return fmt.Errorf("ListPodStats: %w", err)
	}
	// A runtime may omit a pod it has not sampled yet; every entry it does
	// return must name its pod, or the node cannot attribute the numbers.
	for i, ps := range resp.GetPodStats() {
		if ps.GetPodId() == "" {
			return fmt.Errorf("ListPodStats: entry %d has an empty pod_id", i)
		}
	}
	return nil
}

func checkDelete(ctx context.Context, c RuntimeContract) error {
	const id = "contractdelete"
	_, cleanup, err := createPod(ctx, c, id)
	defer cleanup()
	if err != nil {
		return err
	}
	if _, err := c.Client.DeletePod(ctx, &runtimev1.DeletePodRequest{PodId: id}); err != nil {
		return fmt.Errorf("DeletePod %s: %w", id, err)
	}
	if err := wantInBandNotFound(ctx, c, id); err != nil {
		return fmt.Errorf("after DeletePod: %w", err)
	}
	// The node repeats a delete it is not sure landed; a second delete of the
	// same pod, and a delete of a pod never created, both succeed.
	if _, err := c.Client.DeletePod(ctx, &runtimev1.DeletePodRequest{PodId: id}); err != nil {
		return fmt.Errorf("second DeletePod %s: %w", id, err)
	}
	if _, err := c.Client.DeletePod(ctx, &runtimev1.DeletePodRequest{PodId: contractAbsentPodID}); err != nil {
		return fmt.Errorf("DeletePod of an absent pod: %w", err)
	}
	return nil
}
