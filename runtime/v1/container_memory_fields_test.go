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

package runtimev1

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

// TestContainerMemoryFieldsRoundTrip pins Container.memory_limit_bytes and
// MemoryStats.memory_high_events: both survive a marshal/unmarshal and the
// zero value reads as unset (no limit, no events).
func TestContainerMemoryFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		limit     int64
		highEvent uint64
	}{
		{name: "zero is unset", limit: 0, highEvent: 0},
		{name: "limit only", limit: 512 << 20, highEvent: 0},
		{name: "events only", limit: 0, highEvent: 3},
		{name: "both", limit: 1 << 30, highEvent: 42},
		{name: "large", limit: 1 << 40, highEvent: 1 << 40},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := &Container{Name: "app", MemoryLimitBytes: tc.limit}
			cb, err := proto.Marshal(c)
			if err != nil {
				t.Fatalf("marshal Container: %v", err)
			}
			var gotC Container
			if err := proto.Unmarshal(cb, &gotC); err != nil {
				t.Fatalf("unmarshal Container: %v", err)
			}
			if gotC.GetMemoryLimitBytes() != tc.limit {
				t.Errorf("Container.MemoryLimitBytes = %d, want %d", gotC.GetMemoryLimitBytes(), tc.limit)
			}

			m := &MemoryStats{UsageBytes: 7, MemoryHighEvents: tc.highEvent}
			mb, err := proto.Marshal(m)
			if err != nil {
				t.Fatalf("marshal MemoryStats: %v", err)
			}
			var gotM MemoryStats
			if err := proto.Unmarshal(mb, &gotM); err != nil {
				t.Fatalf("unmarshal MemoryStats: %v", err)
			}
			if gotM.GetMemoryHighEvents() != tc.highEvent {
				t.Errorf("MemoryStats.MemoryHighEvents = %d, want %d", gotM.GetMemoryHighEvents(), tc.highEvent)
			}
			if gotM.GetUsageBytes() != 7 {
				t.Errorf("MemoryStats.UsageBytes = %d, want 7", gotM.GetUsageBytes())
			}
		})
	}
}
