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

package budgetv1alpha1

import (
	"fmt"
	"regexp"
)

// dns1123SubdomainRE is the shape of a Kubernetes object name.
var dns1123SubdomainRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// fieldPathRE is the shape of a policy field path in
// AppliedPolicy.ClampedFields (for example "tier1GraceSeconds" or
// "thresholds[2].graceSeconds").
var fieldPathRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*(\[[0-9]+\])?(\.[a-zA-Z][a-zA-Z0-9]*(\[[0-9]+\])?)*$`)

// isObjectName reports whether s has the shape of a Kubernetes object name.
func isObjectName(s string) bool {
	return len(s) <= 253 && dns1123SubdomainRE.MatchString(s)
}

// isPrintableASCII reports whether s is 1..max bytes of printable ASCII
// (0x20..0x7e).
func isPrintableASCII(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// nonNegative reports an error when v is set and negative.
func nonNegative[T int32 | int64](name string, v *T) error {
	if v != nil && *v < 0 {
		return fmt.Errorf("%s %d is negative", name, *v)
	}
	return nil
}

// permille reports an error when v is set and outside 0..1000.
func permille[T int32 | int64](name string, v *T) error {
	if v != nil && (*v < 0 || *v > 1000) {
		return fmt.Errorf("%s %d is not 0..1000 permille", name, *v)
	}
	return nil
}

// clonePtr returns a copy of *p in new memory, or nil.
func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// cloneSlice returns a copy of s in new memory, keeping nil as nil.
func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	out := make([]T, len(s))
	copy(out, s)
	return out
}
