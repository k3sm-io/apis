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

package entry

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestParseVectors runs Parse over the shared table: each row's verdict, a
// valid row's parts re-encode to the input, and an error never repeats the
// input.
func TestParseVectors(t *testing.T) {
	t.Parallel()
	for _, v := range Vectors() {
		e, err := Parse(v.Input)
		if got := err == nil; got != v.Valid {
			t.Errorf("Parse(%.40q) valid = %v, want %v (%s): %v", v.Input, got, v.Valid, v.Why, err)
			continue
		}
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("Parse(%.40q) error %v does not wrap ErrInvalid", v.Input, err)
			}
			if len(v.Input) > 8 && strings.Contains(err.Error(), v.Input) {
				t.Errorf("Parse(%.40q) error repeats the input: %v", v.Input, err)
			}
			continue
		}
		if e.String() != v.Input {
			t.Errorf("Parse(%.40q).String() = %.40q, want the input back", v.Input, e.String())
		}
		if (e.Scope == ScopeSystem) == v.KeyDelivery {
			t.Errorf("%.40q: KeyDelivery verdict %v disagrees with scope %s", v.Input, v.KeyDelivery, e.Scope)
		}
	}
}

// TestParseParts pins the parts Parse returns.
func TestParseParts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want Entry
	}{
		{"system/signer-key", Entry{Scope: ScopeSystem, Name: "signer-key"}},
		{"user/501/claude-a", Entry{Scope: ScopeUser, UID: 501, Name: "claude-a"}},
		{"user/4294967295/a", Entry{Scope: ScopeUser, UID: MaxUID, Name: "a"}},
	} {
		got, err := Parse(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
}

// TestBoundsAgreeWithEachOther pins the length arithmetic the CRD relies on.
func TestBoundsAgreeWithEachOther(t *testing.T) {
	t.Parallel()
	if want := len("user/") + len(strconv.FormatUint(MaxUID, 10)) + len("/") + MaxNameLength; MaxLength != want {
		t.Errorf("MaxLength = %d, want %d", MaxLength, want)
	}
	if !strings.Contains(UIDRangeRule, strconv.FormatUint(MaxUID, 10)) {
		t.Errorf("UIDRangeRule %q does not name MaxUID", UIDRangeRule)
	}
	if !strings.Contains(NameLengthRule, strconv.Itoa(MaxNameLength)) {
		t.Errorf("NameLengthRule %q does not name MaxNameLength", NameLengthRule)
	}
}

// TestVectorsReturnsAFreshCopy asserts a caller's edit reaches no other
// caller.
func TestVectorsReturnsAFreshCopy(t *testing.T) {
	t.Parallel()
	a := Vectors()
	a[0].Valid = !a[0].Valid
	if b := Vectors(); b[0].Valid == a[0].Valid {
		t.Fatal("Vectors returns aliased rows")
	}
}

// TestImportsStdlibOnly keeps the leaf free of every non-standard import, so
// a Kubernetes-free consumer can depend on it.
func TestImportsStdlibOnly(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if first, _, _ := strings.Cut(p, "/"); strings.Contains(first, ".") {
				t.Errorf("%s imports %s; the entry leaf imports the standard library only", f, p)
			}
		}
	}
}
