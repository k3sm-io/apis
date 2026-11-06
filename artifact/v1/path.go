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
	"fmt"
	"strings"
)

// Path bounds.
const (
	// MaxPathDepth is the most components a file path may have.
	MaxPathDepth = 8
	// MaxComponentBytes is the longest a single path component may be.
	MaxComponentBytes = 255
	// MaxPathBytes is the longest a whole path may be: MaxPathDepth components
	// of MaxComponentBytes, each with its separator. It is checked first, so
	// no other rule ever walks a longer input.
	MaxPathBytes = MaxPathDepth * (MaxComponentBytes + 1)
	// MaxPaths is the most paths one set may hold: an artifact carries at
	// most this many layers, and the artifact-paths annotation at most this
	// many paths.
	MaxPaths = 4096
)

// The sentinel errors ValidatePath and ValidatePathSet wrap. Compare with
// errors.Is; the wrapping message names the offending path.
var (
	// ErrEmptyPath is an empty path.
	ErrEmptyPath = errors.New("empty artifact path")
	// ErrAbsolutePath is a path with a leading "/".
	ErrAbsolutePath = errors.New("artifact path is absolute")
	// ErrPathCharset is a byte outside [A-Za-z0-9._-] and "/".
	ErrPathCharset = errors.New("artifact path has a byte outside [A-Za-z0-9._-/]")
	// ErrPathComponent is an empty, "." or ".." component (a trailing or
	// doubled "/" makes an empty one).
	ErrPathComponent = errors.New("artifact path has an empty, \".\" or \"..\" component")
	// ErrReservedComponent is a component starting ".." (the atomic-writer
	// namespace) or "._" (AppleDouble metadata).
	ErrReservedComponent = errors.New("artifact path has a component starting \"..\" or \"._\"")
	// ErrPathTooLong is a path longer than MaxPathBytes.
	ErrPathTooLong = errors.New("artifact path is too long")
	// ErrPathDepth is a path with more than MaxPathDepth components.
	ErrPathDepth = errors.New("artifact path is too deep")
	// ErrComponentTooLong is a component longer than MaxComponentBytes.
	ErrComponentTooLong = errors.New("artifact path component is too long")
	// ErrCodeFile is a file name with a code or pickle extension.
	ErrCodeFile = errors.New("artifact path names a code file")
	// ErrDuplicatePath is a path listed twice in one set.
	ErrDuplicatePath = errors.New("duplicate artifact path")
	// ErrCaseCollision is two paths equal under ASCII case folding.
	ErrCaseCollision = errors.New("artifact paths collide under case folding")
	// ErrFileDirConflict is a path that is both a file and a directory
	// prefix of another path in the set.
	ErrFileDirConflict = errors.New("artifact path is both a file and a directory")
	// ErrTooManyPaths is a set larger than MaxPaths.
	ErrTooManyPaths = errors.New("too many artifact paths")
)

// codeExtensions are the file-name extensions refused by name, before any
// byte is read: source, bytecode, pickles and native libraries. The match is
// case-insensitive, so a model mounts or is refused the same way on a
// case-sensitive and a case-insensitive volume.
var codeExtensions = []string{
	".py", ".pyc", ".bin", ".pt", ".pth", ".pkl", ".pickle", ".ckpt", ".so", ".dylib",
}

// ValidatePath checks one artifact file path. A valid path is at most
// MaxPathBytes long (checked before anything else), relative, made
// only of [A-Za-z0-9._-] components joined by single "/", at most
// MaxPathDepth components of at most MaxComponentBytes bytes each, with no
// empty, ".", or ".." component, no component starting ".." or "._", and a
// final component that does not name a code file (codeExtensions).
//
// The charset is closed ASCII, so Unicode normalization cannot make two
// distinct accepted paths name one file on APFS; ASCII case folding is the
// only equivalence left, and ValidatePathSet refuses it.
func ValidatePath(p string) error {
	if len(p) > MaxPathBytes {
		return fmt.Errorf("path of %d bytes, at most %d: %w", len(p), MaxPathBytes, ErrPathTooLong)
	}
	if p == "" {
		return ErrEmptyPath
	}
	if p[0] == '/' {
		return fmt.Errorf("%q: %w", p, ErrAbsolutePath)
	}
	for i := 0; i < len(p); i++ {
		if !pathByte(p[i]) {
			return fmt.Errorf("%q: byte %d: %w", p, i, ErrPathCharset)
		}
	}
	depth := strings.Count(p, "/") + 1
	if depth > MaxPathDepth {
		return fmt.Errorf("%q: %d components, at most %d: %w", p, depth, MaxPathDepth, ErrPathDepth)
	}
	var last string
	for c := range strings.SplitSeq(p, "/") {
		switch {
		case c == "" || c == "." || c == "..":
			return fmt.Errorf("%q: %w", p, ErrPathComponent)
		case strings.HasPrefix(c, "..") || strings.HasPrefix(c, "._"):
			return fmt.Errorf("%q: component %q: %w", p, c, ErrReservedComponent)
		case len(c) > MaxComponentBytes:
			return fmt.Errorf("%q: component of %d bytes, at most %d: %w", p, len(c), MaxComponentBytes, ErrComponentTooLong)
		}
		last = c
	}
	name := strings.ToLower(last)
	for _, ext := range codeExtensions {
		if strings.HasSuffix(name, ext) {
			return fmt.Errorf("%q: %w (%s)", p, ErrCodeFile, ext)
		}
	}
	return nil
}

// ValidatePathSet checks every path with ValidatePath, then the set as a
// whole: at most MaxPaths paths, no duplicate, no two paths equal under ASCII
// case folding, and no path that is both a file and a directory prefix of
// another (a/b beside a/b/c). The order of paths does not matter.
func ValidatePathSet(paths []string) error {
	if len(paths) > MaxPaths {
		return fmt.Errorf("%d paths, at most %d: %w", len(paths), MaxPaths, ErrTooManyPaths)
	}
	exact := make(map[string]struct{}, len(paths))
	folded := make(map[string]string, len(paths))
	for _, p := range paths {
		if err := ValidatePath(p); err != nil {
			return err
		}
		if _, dup := exact[p]; dup {
			return fmt.Errorf("%q: %w", p, ErrDuplicatePath)
		}
		exact[p] = struct{}{}
		f := strings.ToLower(p)
		if prev, ok := folded[f]; ok {
			return fmt.Errorf("%q and %q: %w", prev, p, ErrCaseCollision)
		}
		folded[f] = p
	}
	// Every path is valid, so it has no empty component and no trailing "/":
	// each proper "/"-prefix is a directory. Folded, so a/B and a/b/c conflict
	// as they would on a case-insensitive volume.
	for _, p := range paths {
		f := strings.ToLower(p)
		for i := 0; i < len(f); i++ {
			if f[i] != '/' {
				continue
			}
			if file, ok := folded[f[:i]]; ok {
				return fmt.Errorf("%q is a file and a directory of %q: %w", file, p, ErrFileDirConflict)
			}
		}
	}
	return nil
}

func pathByte(b byte) bool {
	switch {
	case 'a' <= b && b <= 'z', 'A' <= b && b <= 'Z', '0' <= b && b <= '9':
		return true
	}
	return b == '.' || b == '_' || b == '-' || b == '/'
}
