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
)

// TestDataVolumePathContractDocumented is B148's named gate.
//
// SandboxProfile.data_volume_path reads as a free-form producer-set path, but the
// daemon accepts EXACTLY TWO spellings it derives itself for the box's own pod id
// — the per-pod directory and that directory's rootfs child — and rejects
// everything else as an invalid PodBox. The old comment ("typically the
// rootfs_path") described neither the validation nor the two-spelling accept set,
// so a producer reading it would compute a value that gets a hard per-pod
// rejection. The contract therefore has to be legible at the field itself.
//
// The gate is source-text over BOTH the .proto and the generated .pb.go, because
// a Go author reads the copied comment, not the .proto — a narrowing that stops
// at the .proto never reaches them. It asserts in two directions: the semantic
// keywords are PRESENT in the field's own comment block, and the superseded
// framing is ABSENT from the file, so a diff that appends the new text beside the
// old one cannot go green.
//
// The two accepted spellings are pinned by SEPARATE, non-overlapping substrings
// on purpose: a regression that documents only one of them (the single-spelling
// story this field does NOT have) must redden, which a keyword contained in the
// other spelling's text could not do.
func TestDataVolumePathContractDocumented(t *testing.T) {
	// The superseded framing's discriminating fragment. It asserted the value was
	// merely "typically" the rootfs path, i.e. caller's choice, unvalidated.
	//
	// It is deliberately this WHOLE phrase and never a bare "rootfs_path": that
	// identifier legitimately survives in the sibling PodBox.rootfs_path field and
	// in this field's own cross-reference to it, so a bare-identifier absence check
	// would be permanently red.
	const supersededFragment = "typically the rootfs_path"

	// Substrings tied to the invariant, matched case-insensitively except for the
	// failure-reason enum, which is spelled exactly.
	keywords := []struct {
		want, why string
		fold      bool
	}{
		{"server-validated", "the value is checked against the daemon's own derivations, not taken as given", true},
		{"exactly two spellings", "the accept set has two members; a single-spelling story is the regression", true},
		{"per-pod directory (<root>/pods/<pod_id>)", "the wider accepted spelling, which the current producer sends", true},
		{"<root>/pods/<pod_id>/rootfs", "the narrower accepted spelling, strictly less privilege", true},
		{"applies the narrower rootfs spelling", "whatever is sent, the narrower spelling is what the profile gets", true},
		{"FAILURE_REASON_INVALID_POD_BOX", "a mismatch rejects the pod; it is never coerced and never retried", false},
	}

	files := []struct {
		path      string
		fieldDecl string // the declaration whose preceding comment block is the contract
	}{
		{"runtime.proto", "string data_volume_path = 2;"},
		{"runtime.pb.go", "DataVolumePath string `protobuf:\"bytes,2,"},
	}

	for _, f := range files {
		t.Run(f.path, func(t *testing.T) {
			raw, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("read %s: %v", f.path, err)
			}
			src := string(raw)

			block, ok := commentBlockAbove(src, f.fieldDecl)
			if !ok {
				t.Fatalf("%s: no declaration matching %q; the gate cannot locate the data_volume_path contract", f.path, f.fieldDecl)
			}

			for _, kw := range keywords {
				hay, needle := flattenComments(block), kw.want
				if kw.fold {
					hay, needle = strings.ToLower(hay), strings.ToLower(needle)
				}
				if !strings.Contains(hay, needle) {
					t.Errorf("%s: data_volume_path comment does not mention %q (%s)\ncomment block:\n%s", f.path, kw.want, kw.why, block)
				}
			}

			// Absence is checked over the WHOLE file, not just the block: a stray
			// surviving copy of the old framing anywhere in the contract is the same
			// mixed message the narrowing removes.
			if strings.Contains(flattenComments(src), supersededFragment) {
				t.Errorf("%s: the superseded framing %q is still present; it must be REPLACED, not appended beside", f.path, supersededFragment)
			}
		})
	}
}
