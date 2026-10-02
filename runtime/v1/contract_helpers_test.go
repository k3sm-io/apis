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

import "strings"

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
