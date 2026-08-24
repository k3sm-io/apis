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

// TestRootfsPathContractDocumented is B144's named gate.
//
// PodBox.rootfs_path reads as a caller-supplied path, but the daemon derives it
// from its own configured root and accepts a non-empty value only when it is
// byte-equal to that derivation — anything else fails the pod as an invalid
// PodBox. A producer that trusts the field name writes a value that cannot be
// computed off-node and gets a hard rejection, so the contract has to be legible
// at the field itself.
//
// The gate is source-text over BOTH the .proto and the generated .pb.go, because
// a Go author reads the copied comment, not the .proto — a narrowing that stops
// at the .proto never reaches them. It asserts in two directions: the semantic
// keywords of the invariant are PRESENT in the field's own comment block, and the
// superseded caller-supplied framing is ABSENT from the file, so a diff that
// appends the new text beside the old one cannot go green.
//
// Keywords, not sentences: a reworded comment that keeps the invariant stays
// green, while a comment that drops server-derivation, byte-equality, the
// rejection, or the removal tracker goes red.
func TestRootfsPathContractDocumented(t *testing.T) {
	// The superseded framing's discriminating fragment. It asserted the field WAS
	// the per-pod dir "into which image payloads are clonefile'd", with no hint
	// that the daemon derives and re-checks it.
	const supersededFragment = "into which image payloads are clonefile'd"

	// Substrings tied to the invariant, matched case-insensitively except for the
	// two identifiers, which are spelled exactly.
	keywords := []struct {
		want, why string
		fold      bool
	}{
		{"server-derived", "the value is computed by the daemon; a producer cannot compute it", true},
		{"byte-equal", "a non-empty value is accepted only on byte-equality with the single derivation", true},
		{"FAILURE_REASON_INVALID_POD_BOX", "a mismatch rejects the pod; it is never coerced and never retried", false},
		{"B147", "the field is planned for consumer-first removal under that tracking id", false},
	}

	files := []struct {
		path      string
		fieldDecl string // the declaration whose preceding comment block is the contract
	}{
		{"runtime.proto", "string rootfs_path = 4;"},
		{"runtime.pb.go", "RootfsPath string `protobuf:\"bytes,4,"},
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
				t.Fatalf("%s: no declaration matching %q; the gate cannot locate the rootfs_path contract", f.path, f.fieldDecl)
			}

			for _, kw := range keywords {
				hay, needle := flattenComments(block), kw.want
				if kw.fold {
					hay, needle = strings.ToLower(hay), strings.ToLower(needle)
				}
				if !strings.Contains(hay, needle) {
					t.Errorf("%s: rootfs_path comment does not mention %q (%s)\ncomment block:\n%s", f.path, kw.want, kw.why, block)
				}
			}

			// Absence is checked over the WHOLE file, not just the block: a stray
			// surviving copy of the old framing anywhere in the contract is the same
			// mixed message the narrowing removes.
			if strings.Contains(flattenComments(src), supersededFragment) {
				t.Errorf("%s: the superseded caller-supplied framing %q is still present; it must be REPLACED, not appended beside", f.path, supersededFragment)
			}
		})
	}
}

// flattenComments flattens WHOLE-FILE text — it strips a leading //-marker
// where a line has one and passes every other line through — and collapses all
// whitespace to single spaces, so a phrase is matched however the comment
// happens to be wrapped. It is deliberately NOT comment-scoped; do not extend
// this test assuming non-comment lines were filtered out.
// Without it a line break inside a phrase hides it from a substring search — the
// hole that let an append-beside diff pass an earlier draft of this gate.
func flattenComments(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		b.WriteString(strings.TrimPrefix(strings.TrimSpace(ln), "//"))
		b.WriteString(" ")
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// commentBlockAbove returns the contiguous run of //-comment lines immediately
// preceding the first line containing decl. ok is false when no line contains
// decl.
func commentBlockAbove(src, decl string) (block string, ok bool) {
	lines := strings.Split(src, "\n")
	idx := -1
	for i, ln := range lines {
		if strings.Contains(ln, decl) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", false
	}
	start := idx
	for start > 0 && strings.HasPrefix(strings.TrimSpace(lines[start-1]), "//") {
		start--
	}
	return strings.Join(lines[start:idx], "\n"), true
}
