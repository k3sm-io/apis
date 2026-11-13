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

package imagestorev1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ClassReportSchema is the current schema of the ClassReport payload. Schema
// 1 is closed: any change to the field set, an added field included, bumps
// it; a reader refuses any other value with ErrUnknownClassReportSchema.
const ClassReportSchema = 1

// Bounds on the AnnotationClassReport value.
const (
	// MaxClassReportBytes is the largest annotation value, in bytes, a
	// reader accepts and a writer produces: an eighth of the 256 KiB the API
	// server allows for all of an object's annotations together. A report at
	// MaxClasses keys of MaxClassKeyBytes bytes each, every value the largest
	// uint64, stays under it.
	MaxClassReportBytes = 32 << 10
	// MaxClasses is the most class keys one report may carry: the three
	// reserved keys plus room for many distinct artifact media types on one
	// node. A writer with more classes does not drop any: it folds the
	// smallest media-type classes (ties broken by ascending key) into
	// ClassArtifact until at most MaxClasses keys remain, never folding a
	// reserved class, so StoreBytes is unchanged. FoldClasses applies this
	// rule.
	MaxClasses = 64
	// MaxClassKeyBytes is the longest class key, the media-type bound the
	// store_bytes_by_class field comment in runtime/v1/images.proto states.
	MaxClassKeyBytes = 255
)

// maxRestrictedNameBytes is the longest RFC 6838 restricted-name, the bound
// on each of a media type's type and subtype.
const maxRestrictedNameBytes = 127

var (
	// ErrUnknownClassReportSchema reports a payload whose schema is missing
	// or is not ClassReportSchema. A reader treats the split as not reported;
	// it is not a node error to retry.
	ErrUnknownClassReportSchema = errors.New("imagestorev1: unknown class report schema")
	// ErrInvalidClassReport is wrapped by every other payload error:
	// malformed JSON, an unknown, missing, repeated or case-folded key, a
	// null field or class value, trailing data, or a value that fails
	// validation.
	ErrInvalidClassReport = errors.New("imagestorev1: invalid class report")
	// ErrClassReportTooLarge is wrapped, beside ErrInvalidClassReport, by a
	// payload over MaxClassReportBytes, which is refused unparsed.
	ErrClassReportTooLarge = errors.New("imagestorev1: class report too large")
	// ErrTooManyClasses is wrapped, beside ErrInvalidClassReport, by a
	// report with more than MaxClasses class keys.
	ErrTooManyClasses = errors.New("imagestorev1: too many classes")
	// ErrInvalidClassKey is wrapped, beside ErrInvalidClassReport, by a
	// class key that is neither a reserved class nor a media type of at
	// most MaxClassKeyBytes bytes.
	ErrInvalidClassKey = errors.New("imagestorev1: invalid class key")
	// ErrClassSumMismatch is wrapped, beside ErrInvalidClassReport, by a
	// report whose class values do not sum to StoreBytes (or whose sum
	// overflows). A reader must treat the split as not reported.
	ErrClassSumMismatch = errors.New("imagestorev1: class bytes do not sum to store bytes")
)

// ClassReport is the payload of the AnnotationClassReport annotation, the
// JSON a node writes on its own Node: the image store's bytes split by
// content class, as ImageFsInfoResponse.store_bytes_by_class reports them.
// It is untrusted input to every reader.
type ClassReport struct {
	// Schema is the payload version (ClassReportSchema).
	Schema int `json:"schema"`
	// ObservedAt is when the runtime measured the store, in UTC; it encodes
	// as an RFC 3339 time.
	ObservedAt time.Time `json:"observedAt"`
	// StoreBytes is the image store's total bytes, the sum of BytesByClass.
	StoreBytes uint64 `json:"storeBytes"`
	// BytesByClass maps each class key to its bytes. The keys partition the
	// store: a reserved class (ClassImage, ClassArtifact, ClassUnattributed)
	// or a media type "type/subtype". An empty map with StoreBytes of zero
	// is an empty store.
	BytesByClass map[string]uint64 `json:"bytesByClass"`
}

// Validate reports whether the report is well-formed: Schema is current
// (else ErrUnknownClassReportSchema); ObservedAt is set and in UTC; there are
// at most MaxClasses keys (ErrTooManyClasses), each a valid class key
// (ErrInvalidClassKey); and the values sum to StoreBytes
// (ErrClassSumMismatch). Every error but the schema one wraps
// ErrInvalidClassReport.
func (r *ClassReport) Validate() error {
	if r.Schema != ClassReportSchema {
		return fmt.Errorf("%w: schema %d, want %d", ErrUnknownClassReportSchema, r.Schema, ClassReportSchema)
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("%w: observedAt is not set", ErrInvalidClassReport)
	}
	if r.ObservedAt.Location() != time.UTC {
		return fmt.Errorf("%w: observedAt is not in UTC", ErrInvalidClassReport)
	}
	if n := len(r.BytesByClass); n > MaxClasses {
		return fmt.Errorf("%w: %w: %d classes, at most %d", ErrInvalidClassReport, ErrTooManyClasses, n, MaxClasses)
	}
	var sum uint64
	for k, v := range r.BytesByClass {
		if !IsClassKey(k) {
			return fmt.Errorf("%w: %w: %.300q", ErrInvalidClassReport, ErrInvalidClassKey, k)
		}
		if sum+v < sum {
			return fmt.Errorf("%w: %w: the sum overflows", ErrInvalidClassReport, ErrClassSumMismatch)
		}
		sum += v
	}
	if sum != r.StoreBytes {
		return fmt.Errorf("%w: %w: classes sum to %d, storeBytes is %d", ErrInvalidClassReport, ErrClassSumMismatch, sum, r.StoreBytes)
	}
	return nil
}

// Encode validates the report and returns the annotation value: compact JSON
// with the class keys in sorted order, so equal reports encode to equal
// bytes. The value is then decoded with ParseClassReport, so Encode refuses
// anything Parse would refuse and a writer can never produce a value a
// reader of the same schema rejects. A nil BytesByClass encodes as an empty
// object. Encode refuses more than MaxClasses keys; fold them first with
// FoldClasses.
func (r *ClassReport) Encode() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	out := *r
	if out.BytesByClass == nil {
		out.BytesByClass = map[string]uint64{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// The key grammar admits '&', which HTML escaping would expand to six
	// bytes; the value is never embedded in HTML.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(&out); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidClassReport, err)
	}
	s := strings.TrimSuffix(buf.String(), "\n")
	if len(s) > MaxClassReportBytes {
		return "", fmt.Errorf("%w: %w: payload is %d bytes, at most %d", ErrInvalidClassReport, ErrClassReportTooLarge, len(s), MaxClassReportBytes)
	}
	if _, err := ParseClassReport(s); err != nil {
		return "", err
	}
	return s, nil
}

// FoldClasses returns a copy of byClass with at most MaxClasses keys, by the
// rule MaxClasses states: while there are too many keys, the smallest
// media-type class, ties broken by ascending key, is removed and its bytes
// added to ClassArtifact (created if absent). ClassImage, ClassArtifact and
// ClassUnattributed are never folded, and every key that is not one of them
// counts as a media-type class. The result is deterministic and its values
// sum to those of byClass (when that sum fits a uint64, as a valid report's
// does). A map already within MaxClasses is copied unchanged; byClass itself
// is never modified, and the result is never nil.
func FoldClasses(byClass map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, min(len(byClass), MaxClasses))
	if len(byClass) <= MaxClasses {
		for k, v := range byClass {
			out[k] = v
		}
		return out
	}
	media := make([]string, 0, len(byClass))
	for k := range byClass {
		if !isReservedClass(k) {
			media = append(media, k)
		}
	}
	sort.Slice(media, func(i, j int) bool {
		a, b := byClass[media[i]], byClass[media[j]]
		if a != b {
			return a < b
		}
		return media[i] < media[j]
	})
	// Folding n keys leaves len-n keys, plus ClassArtifact if it is new.
	n := len(byClass) - MaxClasses
	if _, ok := byClass[ClassArtifact]; !ok {
		n++
	}
	folded := make(map[string]bool, n)
	var sum uint64
	for _, k := range media[:n] {
		folded[k] = true
		sum += byClass[k]
	}
	for k, v := range byClass {
		if !folded[k] {
			out[k] = v
		}
	}
	out[ClassArtifact] += sum
	return out
}

func isReservedClass(k string) bool {
	return k == ClassImage || k == ClassArtifact || k == ClassUnattributed
}

// classReportFields is the exact set of top-level keys a payload carries,
// every one required.
var classReportFields = [...]string{"schema", "observedAt", "storeBytes", "bytesByClass"}

// ParseClassReport decodes and validates an AnnotationClassReport value. In
// order, it:
//
//  1. refuses a value over MaxClassReportBytes before any parsing
//     (ErrClassReportTooLarge);
//  2. refuses anything but one JSON object, and a key repeated within any
//     object;
//  3. reads the schema on its own as raw JSON: absent, or anything but
//     exactly 1 (a string, 1.0, 2), is ErrUnknownClassReportSchema, so a
//     future schema whose fields changed type is never reported as malformed;
//  4. refuses a top-level key that is unknown, missing, or matches a field
//     only case-insensitively, a field or class value that is JSON null
//     (which would otherwise decode as zero), and a bytesByClass that is
//     not an object;
//  5. decodes the value and validates it (ClassReport.Validate).
//
// A missing or unknown schema returns ErrUnknownClassReportSchema; every
// other failure wraps ErrInvalidClassReport. The returned report always has
// a non-nil BytesByClass.
func ParseClassReport(value string) (ClassReport, error) {
	if len(value) > MaxClassReportBytes {
		return ClassReport{}, fmt.Errorf("%w: %w: payload is %d bytes, at most %d", ErrInvalidClassReport, ErrClassReportTooLarge, len(value), MaxClassReportBytes)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &top); err != nil {
		return ClassReport{}, fmt.Errorf("%w: %w", ErrInvalidClassReport, err)
	}
	if top == nil {
		return ClassReport{}, fmt.Errorf("%w: payload is not an object", ErrInvalidClassReport)
	}
	if err := checkDuplicateKeys([]byte(value)); err != nil {
		return ClassReport{}, fmt.Errorf("%w: %w", ErrInvalidClassReport, err)
	}
	schema, ok := top["schema"]
	if !ok || string(schema) == "null" {
		return ClassReport{}, fmt.Errorf("%w: payload has no schema", ErrUnknownClassReportSchema)
	}
	if want := strconv.Itoa(ClassReportSchema); string(schema) != want {
		return ClassReport{}, fmt.Errorf("%w: schema %.32s, want %s", ErrUnknownClassReportSchema, schema, want)
	}
	for _, f := range classReportFields {
		if _, ok := top[f]; !ok {
			return ClassReport{}, fmt.Errorf("%w: field %q is missing", ErrInvalidClassReport, f)
		}
	}
	if len(top) != len(classReportFields) {
		for k := range top {
			if !isClassReportField(k) {
				return ClassReport{}, fmt.Errorf("%w: unknown field %.300q", ErrInvalidClassReport, k)
			}
		}
	}
	for _, f := range classReportFields {
		if isJSONNull(top[f]) {
			return ClassReport{}, fmt.Errorf("%w: field %q is null", ErrInvalidClassReport, f)
		}
	}
	if raw := bytes.TrimSpace(top["bytesByClass"]); len(raw) == 0 || raw[0] != '{' {
		return ClassReport{}, fmt.Errorf("%w: bytesByClass is not an object", ErrInvalidClassReport)
	}
	var classes map[string]json.RawMessage
	if err := json.Unmarshal(top["bytesByClass"], &classes); err != nil {
		return ClassReport{}, fmt.Errorf("%w: %w", ErrInvalidClassReport, err)
	}
	for k, v := range classes {
		if isJSONNull(v) {
			return ClassReport{}, fmt.Errorf("%w: class %.300q is null", ErrInvalidClassReport, k)
		}
	}
	var r ClassReport
	if err := json.Unmarshal([]byte(value), &r); err != nil {
		return ClassReport{}, fmt.Errorf("%w: %w", ErrInvalidClassReport, err)
	}
	if err := r.Validate(); err != nil {
		return ClassReport{}, err
	}
	return r, nil
}

// isJSONNull reports whether raw is the JSON literal null, which
// encoding/json would otherwise decode silently as a zero value.
func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

func isClassReportField(k string) bool {
	for _, f := range classReportFields {
		if k == f {
			return true
		}
	}
	return false
}

// IsClassKey reports whether k is a valid class key: ClassImage,
// ClassArtifact, ClassUnattributed, or a media type of at most
// MaxClassKeyBytes bytes of the form type "/" subtype, each an RFC 6838
// restricted-name, with no parameters. This is the key grammar of the
// store_bytes_by_class field comment in runtime/v1/images.proto.
func IsClassKey(k string) bool {
	switch k {
	case ClassImage, ClassArtifact, ClassUnattributed:
		return true
	}
	if len(k) > MaxClassKeyBytes {
		return false
	}
	typ, sub, ok := strings.Cut(k, "/")
	return ok && isRestrictedName(typ) && isRestrictedName(sub)
}

// isRestrictedName reports whether s is an RFC 6838 §4.2 restricted-name:
// an ALPHA or DIGIT, then up to 126 of ALPHA, DIGIT and "!#$&-^_.+".
func isRestrictedName(s string) bool {
	if len(s) == 0 || len(s) > maxRestrictedNameBytes {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case i > 0 && strings.IndexByte("!#$&-^_.+", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// checkDuplicateKeys walks the tokens of one JSON value and refuses an
// object that names a key twice (encoding/json keeps the last silently, so
// a repeated class key could otherwise hide bytes from the sum check).
func checkDuplicateKeys(b []byte) error {
	type frame struct {
		obj       bool
		expectKey bool
		keys      map[string]bool
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	var stack []*frame
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].obj {
			stack[n-1].expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if n := len(stack); n > 0 && stack[n-1].obj && stack[n-1].expectKey {
			if d, ok := tok.(json.Delim); ok && d == '}' {
				stack = stack[:n-1]
				valueDone()
				continue
			}
			k, _ := tok.(string)
			if stack[n-1].keys[k] {
				return fmt.Errorf("key %.300q is repeated", k)
			}
			stack[n-1].keys[k] = true
			stack[n-1].expectKey = false
			continue
		}
		switch d := tok.(type) {
		case json.Delim:
			switch d {
			case '{':
				stack = append(stack, &frame{obj: true, expectKey: true, keys: map[string]bool{}})
			case '[':
				stack = append(stack, &frame{})
			default: // ']'; '}' is consumed in the key position above
				stack = stack[:len(stack)-1]
				valueDone()
			}
		default:
			valueDone()
		}
		if len(stack) == 0 {
			return nil
		}
	}
}
