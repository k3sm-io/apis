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

package artifactv1

import (
	"errors"
	"path"
	"strings"
	"testing"
)

// TestValidatePath is the table of single-path rules.
func TestValidatePath(t *testing.T) {
	long := strings.Repeat("a", MaxComponentBytes)
	for _, tc := range []struct {
		name string
		path string
		want error
	}{
		{"plain file", "config.json", nil},
		{"weight shard", "model-00001-of-00002.safetensors", nil},
		{"nested", "tokenizer/tokenizer.json", nil},
		{"gguf split", "q4/llama-00001-of-00003.gguf", nil},
		{"dotfile", ".gitattributes", nil},
		{"max depth", "a/b/c/d/e/f/g/h.json", nil},
		{"max component", long, nil},
		{"underscore and dash", "a_b-c.txt", nil},

		{"empty", "", ErrEmptyPath},
		{"absolute", "/etc/passwd", ErrAbsolutePath},
		{"dotdot", "../x", ErrPathComponent},
		{"inner dotdot", "a/../b", ErrPathComponent},
		{"dot", "./x", ErrPathComponent},
		{"lone dot", ".", ErrPathComponent},
		{"trailing slash", "a/", ErrPathComponent},
		{"double slash", "a//b", ErrPathComponent},
		{"too deep", "a/b/c/d/e/f/g/h/i.json", ErrPathDepth},
		{"component too long", long + "a", ErrComponentTooLong},
		{"space", "a b", ErrPathCharset},
		{"backslash", `a\b`, ErrPathCharset},
		{"nul", "a\x00b", ErrPathCharset},
		{"colon", "a:b", ErrPathCharset},
		{"non-ASCII", "café.json", ErrPathCharset},
		{"NFD non-ASCII", "café.json", ErrPathCharset},
		{"atomic-writer namespace", "..data/x", ErrReservedComponent},
		{"atomic-writer file", "..2026_11_06", ErrReservedComponent},
		{"AppleDouble", "._model.safetensors", ErrReservedComponent},
		{"inner AppleDouble", "a/._b", ErrReservedComponent},
		{"python", "modeling.py", ErrCodeFile},
		{"python upper case", "MODELING.PY", ErrCodeFile},
		{"bytecode", "x.pyc", ErrCodeFile},
		{"pytorch bin", "pytorch_model.bin", ErrCodeFile},
		{"pt", "w.pt", ErrCodeFile},
		{"pth", "w.pth", ErrCodeFile},
		{"pkl", "w.pkl", ErrCodeFile},
		{"pickle", "w.pickle", ErrCodeFile},
		{"ckpt", "w.ckpt", ErrCodeFile},
		{"shared object", "lib/x.so", ErrCodeFile},
		{"dylib", "lib/x.Dylib", ErrCodeFile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePath(tc.path)
			if tc.want == nil {
				if err != nil {
					t.Errorf("ValidatePath(%q) = %v, want nil", tc.path, err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("ValidatePath(%q) = %v, want %v", tc.path, err, tc.want)
			}
		})
	}
}

// TestValidatePathSet is the table of set rules.
func TestValidatePathSet(t *testing.T) {
	tooMany := make([]string, MaxPaths+1)
	for i := range tooMany {
		tooMany[i] = "f" + strings.Repeat("x", i%7) + "-" + itoa(i)
	}
	full := tooMany[:MaxPaths]
	for _, tc := range []struct {
		name  string
		paths []string
		want  error
	}{
		{"empty set", nil, nil},
		{"a model", []string{"config.json", "model.safetensors", "tokenizer/tokenizer.json", "tokenizer/vocab.json"}, nil},
		{"siblings sharing a prefix", []string{"a/b", "a/bc", "a/b.json"}, nil},
		{"MaxPaths paths", full, nil},

		{"one invalid path", []string{"config.json", "../x"}, ErrPathComponent},
		{"duplicate", []string{"a.json", "a.json"}, ErrDuplicatePath},
		{"case collision", []string{"Model.safetensors", "model.safetensors"}, ErrCaseCollision},
		{"directory case collision", []string{"Tok/a.json", "tok/a.json"}, ErrCaseCollision},
		{"file then directory", []string{"a/b", "a/b/c"}, ErrFileDirConflict},
		{"directory then file", []string{"a/b/c", "a/b"}, ErrFileDirConflict},
		{"file and deep directory", []string{"a", "a/b/c/d"}, ErrFileDirConflict},
		{"file and directory under case folding", []string{"A/b", "a/b/c"}, ErrFileDirConflict},
		{"too many", tooMany, ErrTooManyPaths},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePathSet(tc.paths)
			if tc.want == nil {
				if err != nil {
					t.Errorf("ValidatePathSet = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("ValidatePathSet = %v, want %v", err, tc.want)
			}
		})
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// FuzzValidatePath asserts that anything ValidatePath accepts is a clean,
// local, closed-charset, bounded path that names no code file, and that a
// path and its case-folded twin form a set ValidatePathSet refuses unless
// they are identical. Seeds live in testdata/fuzz/FuzzValidatePath.
func FuzzValidatePath(f *testing.F) {
	for _, s := range []string{
		"config.json", "a/b/c.safetensors", "../x", "/abs", "a//b", "._x", "..data", "x.PY", "café",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		if ValidatePath(p) != nil {
			return
		}
		if p == "" || p[0] == '/' {
			t.Fatalf("accepted %q: empty or absolute", p)
		}
		if path.Clean(p) != p {
			t.Fatalf("accepted %q: not clean (%q)", p, path.Clean(p))
		}
		for i := 0; i < len(p); i++ {
			if !pathByte(p[i]) {
				t.Fatalf("accepted %q: byte %q outside the charset", p, p[i])
			}
		}
		comps := strings.Split(p, "/")
		if len(comps) > MaxPathDepth {
			t.Fatalf("accepted %q: %d components", p, len(comps))
		}
		for _, c := range comps {
			if c == "" || c == "." || strings.HasPrefix(c, "..") || strings.HasPrefix(c, "._") || len(c) > MaxComponentBytes {
				t.Fatalf("accepted %q: component %q", p, c)
			}
		}
		low := strings.ToLower(comps[len(comps)-1])
		for _, ext := range codeExtensions {
			if strings.HasSuffix(low, ext) {
				t.Fatalf("accepted %q: code extension %s", p, ext)
			}
		}
		if err := ValidatePathSet([]string{p}); err != nil {
			t.Fatalf("a single valid path %q is refused as a set: %v", p, err)
		}
		if up := strings.ToUpper(p); up != p {
			if err := ValidatePathSet([]string{p, up}); !errors.Is(err, ErrCaseCollision) {
				t.Fatalf("set {%q, %q} = %v, want ErrCaseCollision", p, up, err)
			}
		}
		if err := ValidatePathSet([]string{p, p + "/x"}); err == nil {
			t.Fatalf("set {%q, %q} accepted a file that is also a directory", p, p+"/x")
		}
	})
}
