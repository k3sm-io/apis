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
	"slices"
	"sync"
	"testing"
	"time"

	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "k3sm.io/apis/runtime/v1"
)

// refRuntime is the smallest in-memory runtime that keeps the contract. It is
// the baseline the broken variants below each break in exactly one place, so a
// variant that still passes would mean the suite does not check that field.
type refRuntime struct {
	runtimev1.UnimplementedRuntimeServer

	mu   sync.Mutex
	pods map[string]*runtimev1.PodBox
}

func newRefRuntime() *refRuntime {
	return &refRuntime{pods: map[string]*runtimev1.PodBox{}}
}

func (r *refRuntime) statusLocked(id string) *runtimev1.PodStatus {
	return &runtimev1.PodStatus{PodId: id, Phase: runtimev1.PodPhase_POD_PHASE_RUNNING}
}

func (r *refRuntime) GetRuntimeInfo(context.Context, *runtimev1.GetRuntimeInfoRequest) (*runtimev1.GetRuntimeInfoResponse, error) {
	return &runtimev1.GetRuntimeInfoResponse{RuntimeName: "reference", Healthy: true}, nil
}

func (r *refRuntime) CreatePod(_ context.Context, req *runtimev1.CreatePodRequest) (*runtimev1.CreatePodResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := req.GetPod().GetPodId()
	r.pods[id] = req.GetPod()
	return &runtimev1.CreatePodResponse{Status: r.statusLocked(id)}, nil
}

func (r *refRuntime) GetPodStatus(_ context.Context, req *runtimev1.GetPodStatusRequest) (*runtimev1.GetPodStatusResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pods[req.GetPodId()]; !ok {
		return &runtimev1.GetPodStatusResponse{
			Error: &rpcstatus.Status{Code: int32(codes.NotFound), Message: "not found"},
		}, nil
	}
	return &runtimev1.GetPodStatusResponse{Status: r.statusLocked(req.GetPodId())}, nil
}

func (r *refRuntime) DeletePod(_ context.Context, req *runtimev1.DeletePodRequest) (*runtimev1.DeletePodResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pods, req.GetPodId())
	return &runtimev1.DeletePodResponse{}, nil
}

func (r *refRuntime) ListPodStats(_ context.Context, req *runtimev1.ListPodStatsRequest) (*runtimev1.ListPodStatsResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resp := &runtimev1.ListPodStatsResponse{}
	for id := range r.pods {
		if req.GetPodId() == "" || req.GetPodId() == id {
			resp.PodStats = append(resp.PodStats, &runtimev1.PodStats{PodId: id})
		}
	}
	return resp, nil
}

func (r *refRuntime) WatchPodStatus(req *runtimev1.WatchPodStatusRequest, stream grpc.ServerStreamingServer[runtimev1.PodStatusEvent]) error {
	r.mu.Lock()
	var evs []*runtimev1.PodStatusEvent
	for id := range r.pods {
		if req.GetPodId() == "" || req.GetPodId() == id {
			evs = append(evs, &runtimev1.PodStatusEvent{
				Type:   runtimev1.PodStatusEventType_POD_STATUS_EVENT_TYPE_ADDED,
				Status: r.statusLocked(id),
			})
		}
	}
	r.mu.Unlock()
	for _, ev := range evs {
		if err := stream.Send(ev); err != nil {
			return err
		}
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

// Each broken variant drops or bends one thing the node relies on.

type dropInfoName struct{ *refRuntime }

func (d dropInfoName) GetRuntimeInfo(context.Context, *runtimev1.GetRuntimeInfoRequest) (*runtimev1.GetRuntimeInfoResponse, error) {
	return &runtimev1.GetRuntimeInfoResponse{Healthy: true}, nil
}

type dropCreatePodID struct{ *refRuntime }

func (d dropCreatePodID) CreatePod(ctx context.Context, req *runtimev1.CreatePodRequest) (*runtimev1.CreatePodResponse, error) {
	resp, err := d.refRuntime.CreatePod(ctx, req)
	if resp != nil {
		resp.Status.PodId = ""
	}
	return resp, err
}

type dropStatusPodID struct{ *refRuntime }

func (d dropStatusPodID) GetPodStatus(ctx context.Context, req *runtimev1.GetPodStatusRequest) (*runtimev1.GetPodStatusResponse, error) {
	resp, err := d.refRuntime.GetPodStatus(ctx, req)
	if resp.GetStatus() != nil {
		resp.Status.PodId = ""
	}
	return resp, err
}

type dropStatusPhase struct{ *refRuntime }

func (d dropStatusPhase) GetPodStatus(ctx context.Context, req *runtimev1.GetPodStatusRequest) (*runtimev1.GetPodStatusResponse, error) {
	resp, err := d.refRuntime.GetPodStatus(ctx, req)
	if resp.GetStatus() != nil {
		resp.Status.Phase = runtimev1.PodPhase_POD_PHASE_UNSPECIFIED
	}
	return resp, err
}

type unknownAsTransportError struct{ *refRuntime }

func (d unknownAsTransportError) GetPodStatus(ctx context.Context, req *runtimev1.GetPodStatusRequest) (*runtimev1.GetPodStatusResponse, error) {
	resp, err := d.refRuntime.GetPodStatus(ctx, req)
	if resp.GetError() != nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	return resp, err
}

type dropWatchPodID struct{ *refRuntime }

func (d dropWatchPodID) WatchPodStatus(req *runtimev1.WatchPodStatusRequest, stream grpc.ServerStreamingServer[runtimev1.PodStatusEvent]) error {
	if err := stream.Send(&runtimev1.PodStatusEvent{
		Type:   runtimev1.PodStatusEventType_POD_STATUS_EVENT_TYPE_ADDED,
		Status: &runtimev1.PodStatus{Phase: runtimev1.PodPhase_POD_PHASE_RUNNING},
	}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

type dropStatsPodID struct{ *refRuntime }

func (d dropStatsPodID) ListPodStats(ctx context.Context, req *runtimev1.ListPodStatsRequest) (*runtimev1.ListPodStatsResponse, error) {
	resp, err := d.refRuntime.ListPodStats(ctx, req)
	for _, ps := range resp.GetPodStats() {
		ps.PodId = ""
	}
	return resp, err
}

type deleteNotIdempotent struct{ *refRuntime }

func (d deleteNotIdempotent) DeletePod(ctx context.Context, req *runtimev1.DeletePodRequest) (*runtimev1.DeletePodResponse, error) {
	d.mu.Lock()
	_, ok := d.pods[req.GetPodId()]
	d.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "no such pod")
	}
	return d.refRuntime.DeletePod(ctx, req)
}

type deleteKeepsPod struct{ *refRuntime }

func (d deleteKeepsPod) DeletePod(context.Context, *runtimev1.DeletePodRequest) (*runtimev1.DeletePodResponse, error) {
	return &runtimev1.DeletePodResponse{}, nil
}

func refPod(id string) *runtimev1.PodBox {
	return &runtimev1.PodBox{
		PodId:      id,
		Namespace:  "default",
		Name:       "p",
		Containers: []*runtimev1.Container{{Name: "main", Image: "/bin/sleep"}},
	}
}

// TestRuntimeContract proves the suite in both directions: the reference
// runtime passes every case, and each broken variant fails the one case that
// owns the field it breaks while the suite as a whole goes red.
func TestRuntimeContract(t *testing.T) {
	t.Run("reference passes", func(t *testing.T) {
		RunRuntimeContract(t, RuntimeContract{
			Client: ServeRuntime(t, newRefRuntime()),
			Pod:    refPod,
		})
	})

	broken := []struct {
		name   string
		srv    func(*refRuntime) runtimev1.RuntimeServer
		wantRd string
	}{
		{"runtime name dropped", func(r *refRuntime) runtimev1.RuntimeServer { return dropInfoName{r} }, "runtime-info"},
		{"create pod id dropped", func(r *refRuntime) runtimev1.RuntimeServer { return dropCreatePodID{r} }, "create"},
		{"status pod id dropped", func(r *refRuntime) runtimev1.RuntimeServer { return dropStatusPodID{r} }, "status"},
		{"status phase dropped", func(r *refRuntime) runtimev1.RuntimeServer { return dropStatusPhase{r} }, "status"},
		{"unknown pod as transport error", func(r *refRuntime) runtimev1.RuntimeServer { return unknownAsTransportError{r} }, "status-unknown"},
		{"watch pod id dropped", func(r *refRuntime) runtimev1.RuntimeServer { return dropWatchPodID{r} }, "watch"},
		{"stats pod id dropped", func(r *refRuntime) runtimev1.RuntimeServer { return dropStatsPodID{r} }, "stats"},
		{"delete not idempotent", func(r *refRuntime) runtimev1.RuntimeServer { return deleteNotIdempotent{r} }, "delete"},
		{"delete keeps the pod", func(r *refRuntime) runtimev1.RuntimeServer { return deleteKeepsPod{r} }, "delete"},
	}
	for _, tc := range broken {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			results := CheckRuntimeContract(ctx, RuntimeContract{
				Client: ServeRuntime(t, tc.srv(newRefRuntime())),
				Pod:    refPod,
			})
			var red []string
			for _, res := range results {
				if res.Err != nil {
					red = append(red, res.Case)
				}
			}
			if !slices.Contains(red, tc.wantRd) {
				t.Errorf("case %q stayed green against a runtime with %s; red cases: %v", tc.wantRd, tc.name, red)
			}
		})
	}

	t.Run("every case is named once", func(t *testing.T) {
		seen := map[string]bool{}
		for _, c := range runtimeContractCases {
			if c.name == "" || seen[c.name] {
				t.Errorf("case name %q is empty or repeated", c.name)
			}
			seen[c.name] = true
		}
		for _, want := range []string{"runtime-info", "create", "status", "status-unknown", "watch", "stats", "delete"} {
			if !seen[want] {
				t.Errorf("contract is missing case %q", want)
			}
		}
	})
}
