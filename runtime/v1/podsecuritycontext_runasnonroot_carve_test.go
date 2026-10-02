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
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestPodSecurityContextRunAsNonRootCarve pins PodSecurityContext.run_as_non_root:
// field 4, a singular bool with JSON name runAsNonRoot, the new 100..149
// headroom band, and the field comment that carries its contract — the logical
// OR composition with the container value and the producer stamping rule that
// keeps a consumer predating the field enforcing. The test is descriptor-driven
// so it fails at runtime, not at compile time, on a tree without the field.
func TestPodSecurityContextRunAsNonRootCarve(t *testing.T) {
	t.Parallel()

	md := File_runtime_v1_runtime_proto.Messages().ByName("PodSecurityContext")
	if md == nil {
		t.Fatal("message k3sm.runtime.v1.PodSecurityContext not found")
	}

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("run_as_non_root")
		if fd == nil {
			t.Fatal("PodSecurityContext.run_as_non_root is not declared")
		}
		if fd.Number() != 4 {
			t.Errorf("run_as_non_root number = %d, want 4", fd.Number())
		}
		if fd.Kind() != protoreflect.BoolKind {
			t.Errorf("run_as_non_root kind = %v, want bool", fd.Kind())
		}
		if fd.Cardinality() != protoreflect.Optional || fd.IsList() || fd.IsMap() {
			t.Errorf("run_as_non_root must be singular, got cardinality %v", fd.Cardinality())
		}
		if fd.JSONName() != "runAsNonRoot" {
			t.Errorf("run_as_non_root JSON name = %q, want runAsNonRoot", fd.JSONName())
		}
	})

	t.Run("reserved band is exactly 100..149", func(t *testing.T) {
		t.Parallel()
		rr := md.ReservedRanges()
		if rr.Len() != 1 {
			t.Fatalf("PodSecurityContext has %d reserved ranges, want exactly 1", rr.Len())
		}
		r := rr.Get(0) // [start, end) — end is exclusive
		if r[0] != 100 || r[1]-1 != 149 {
			t.Errorf("PodSecurityContext reserved range = %d..%d, want 100..149", r[0], r[1]-1)
		}
	})

	t.Run("comment states the contract", func(t *testing.T) {
		t.Parallel()
		raw, err := os.ReadFile("runtime.proto")
		if err != nil {
			t.Fatalf("read runtime.proto: %v", err)
		}
		src := string(raw)
		start := strings.Index(src, "message PodSecurityContext {")
		if start < 0 {
			t.Fatal("runtime.proto: message PodSecurityContext not found")
		}
		end := strings.Index(src[start:], "\n}")
		if end < 0 {
			t.Fatal("runtime.proto: PodSecurityContext body not terminated")
		}
		body := strings.Join(strings.Fields(strings.ReplaceAll(src[start:start+end], "//", " ")), " ")
		for _, phrase := range []string{
			"by logical OR",
			"NOT a default",
			"stamps every container's effective value",
			"sends this field as false when any container explicitly opts out",
			"A consumer that predates this field therefore still enforces",
		} {
			if !strings.Contains(body, phrase) {
				t.Errorf("PodSecurityContext.run_as_non_root comment does not state %q", phrase)
			}
		}
	})
}
