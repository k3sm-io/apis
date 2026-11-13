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
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
)

var observed = time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)

// validReport returns a report with every kind of class key.
func validReport() ClassReport {
	return ClassReport{
		Schema:     ClassReportSchema,
		ObservedAt: observed,
		StoreBytes: 1<<30 + 1<<20 + 4096 + 512 + 7,
		BytesByClass: map[string]uint64{
			ClassImage: 1 << 30,
			"application/vnd.cncf.model.weight.v1.raw": 1 << 20,
			"application/wasm":                         4096,
			ClassArtifact:                              512,
			ClassUnattributed:                          7,
		},
	}
}

// maximalReport returns the largest valid report: MaxClasses keys of
// MaxClassKeyBytes bytes, each value as large as keeps the sum within a
// uint64, and a nanosecond observedAt.
func maximalReport() ClassReport {
	r := ClassReport{Schema: ClassReportSchema, ObservedAt: observed.Add(999999999), BytesByClass: map[string]uint64{}}
	per := uint64(math.MaxUint64) / MaxClasses
	for i := 0; i < MaxClasses; i++ {
		// type and subtype are each the longest restricted-name, 127 bytes.
		typ := fmt.Sprintf("a%03d", i)
		typ += strings.Repeat("&", maxRestrictedNameBytes-len(typ))
		k := typ + "/b" + strings.Repeat("&", maxRestrictedNameBytes-1)
		r.BytesByClass[k] = per
		r.StoreBytes += per
	}
	return r
}

// TestImageClassReportRoundTripAndRefusals is the gate of the node-to-reader
// payload: valid reports round-trip byte-for-byte and encode
// deterministically, and every refusal fails closed on its own sentinel, in
// Parse (the raw bytes a careless or hostile node could write) and, where a
// Go value can express it, in Encode too.
func TestImageClassReportRoundTripAndRefusals(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		r    ClassReport
		want string // the exact encoding, when pinned
	}{
		{"mixed classes", validReport(), `{"schema":1,"observedAt":"2026-11-13T09:30:00Z","storeBytes":1074795015,"bytesByClass":{"application/vnd.cncf.model.weight.v1.raw":1048576,"application/wasm":4096,"artifact":512,"image":1073741824,"unattributed":7}}`},
		{"empty store", ClassReport{Schema: ClassReportSchema, ObservedAt: observed}, `{"schema":1,"observedAt":"2026-11-13T09:30:00Z","storeBytes":0,"bytesByClass":{}}`},
		{"maximal report", maximalReport(), ""},
	} {
		t.Run("round trip "+tc.name, func(t *testing.T) {
			s, err := tc.r.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if tc.want != "" && s != tc.want {
				t.Fatalf("Encode =\n%s\nwant\n%s", s, tc.want)
			}
			if len(s) > MaxClassReportBytes {
				t.Fatalf("encoded %d bytes, over the %d cap", len(s), MaxClassReportBytes)
			}
			// Encoding is deterministic: map iteration order must not leak.
			for i := 0; i < 20; i++ {
				again, err := tc.r.Encode()
				if err != nil || again != s {
					t.Fatalf("Encode #%d = %q, %v; want %q", i, again, err, s)
				}
			}
			got, err := ParseClassReport(s)
			if err != nil {
				t.Fatalf("ParseClassReport: %v", err)
			}
			if !got.ObservedAt.Equal(tc.r.ObservedAt) || got.ObservedAt.Location() != time.UTC {
				t.Fatalf("observedAt = %v, want %v in UTC", got.ObservedAt, tc.r.ObservedAt)
			}
			want := tc.r
			if want.BytesByClass == nil {
				want.BytesByClass = map[string]uint64{}
			}
			again, err := got.Encode()
			if err != nil || again != s {
				t.Fatalf("re-Encode = %q, %v; want %q", again, err, s)
			}
			got.ObservedAt, want.ObservedAt = time.Time{}, time.Time{}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip = %+v, want %+v", got, want)
			}
		})
	}

	// Refusals a Go value can express: Encode refuses the value and Parse
	// refuses its raw bytes, with the same sentinel.
	for _, tc := range []struct {
		name   string
		mutate func(r *ClassReport)
		want   error
	}{
		{"schema 0", func(r *ClassReport) { r.Schema = 0 }, ErrUnknownClassReportSchema},
		{"schema 2", func(r *ClassReport) { r.Schema = 2 }, ErrUnknownClassReportSchema},
		{"sum below total", func(r *ClassReport) { r.StoreBytes++ }, ErrClassSumMismatch},
		{"sum above total", func(r *ClassReport) { r.StoreBytes-- }, ErrClassSumMismatch},
		{"split missing with bytes", func(r *ClassReport) { r.BytesByClass = nil }, ErrClassSumMismatch},
		{"sum overflows", func(r *ClassReport) {
			r.BytesByClass = map[string]uint64{ClassImage: math.MaxUint64, ClassArtifact: 2}
			r.StoreBytes = 1
		}, ErrClassSumMismatch},
		{"too many classes", func(r *ClassReport) {
			for i := len(r.BytesByClass); i <= MaxClasses; i++ {
				r.BytesByClass[fmt.Sprintf("application/x-%d", i)] = 0
			}
		}, ErrTooManyClasses},
		{"key without slash", func(r *ClassReport) { moveClass(r, ClassArtifact, "model") }, ErrInvalidClassKey},
		{"key with parameter", func(r *ClassReport) { moveClass(r, ClassArtifact, "text/plain;charset=utf-8") }, ErrInvalidClassKey},
		{"key with two slashes", func(r *ClassReport) { moveClass(r, ClassArtifact, "a/b/c") }, ErrInvalidClassKey},
		{"key with empty subtype", func(r *ClassReport) { moveClass(r, ClassArtifact, "application/") }, ErrInvalidClassKey},
		{"key starting with symbol", func(r *ClassReport) { moveClass(r, ClassArtifact, "application/+json") }, ErrInvalidClassKey},
		{"key with space", func(r *ClassReport) { moveClass(r, ClassArtifact, "application/x y") }, ErrInvalidClassKey},
		{"reserved name case-folded", func(r *ClassReport) { moveClass(r, ClassImage, "Image") }, ErrInvalidClassKey},
		{"empty key", func(r *ClassReport) { moveClass(r, ClassArtifact, "") }, ErrInvalidClassKey},
		{"key over 255 bytes", func(r *ClassReport) {
			moveClass(r, ClassArtifact, "application/"+strings.Repeat("x", MaxClassKeyBytes-len("application/")+1))
		}, ErrInvalidClassKey},
		{"zero observedAt", func(r *ClassReport) { r.ObservedAt = time.Time{} }, ErrInvalidClassReport},
		{"observedAt not UTC", func(r *ClassReport) { r.ObservedAt = observed.In(time.FixedZone("x", 3600)) }, ErrInvalidClassReport},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			r := validReport()
			tc.mutate(&r)
			if _, err := r.Encode(); !errors.Is(err, tc.want) {
				t.Fatalf("Encode error = %v, want %v", err, tc.want)
			}
			raw := marshalUnchecked(t, r)
			if _, err := ParseClassReport(raw); !errors.Is(err, tc.want) {
				t.Fatalf("ParseClassReport(%.200s) error = %v, want %v", raw, err, tc.want)
			}
		})
	}

	// Refusals only raw bytes can express.
	valid, err := func() (string, error) { r := validReport(); return r.Encode() }()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	body := strings.TrimPrefix(valid, "{")
	for _, tc := range []struct {
		name  string
		value string
		want  error
	}{
		{"over the byte cap", valid[:len(valid)-1] + strings.Repeat(" ", MaxClassReportBytes-len(valid)+1) + "}", ErrClassReportTooLarge},
		{"no schema", strings.Replace(valid, `"schema":1,`, "", 1), ErrUnknownClassReportSchema},
		{"null schema", strings.Replace(valid, `"schema":1`, `"schema":null`, 1), ErrUnknownClassReportSchema},
		{"string schema", strings.Replace(valid, `"schema":1`, `"schema":"1"`, 1), ErrUnknownClassReportSchema},
		{"float schema", strings.Replace(valid, `"schema":1`, `"schema":1.0`, 1), ErrUnknownClassReportSchema},
		{"not JSON", "not json", ErrInvalidClassReport},
		{"not an object", "[1]", ErrInvalidClassReport},
		{"null", "null", ErrInvalidClassReport},
		{"trailing data", valid + "{}", ErrInvalidClassReport},
		{"unknown field", `{"extra":1,` + body, ErrInvalidClassReport},
		{"case-folded field", strings.Replace(valid, `"storeBytes"`, `"StoreBytes"`, 1), ErrInvalidClassReport},
		{"missing storeBytes", strings.Replace(valid, `"storeBytes":1074795015,`, "", 1), ErrInvalidClassReport},
		{"null bytesByClass", `{"schema":1,"observedAt":"2026-11-13T09:30:00Z","storeBytes":0,"bytesByClass":null}`, ErrInvalidClassReport},
		{"repeated field", `{"storeBytes":0,` + body, ErrInvalidClassReport},
		{"repeated class key", strings.Replace(valid, `"artifact":512`, `"artifact":256,"artifact":256`, 1), ErrInvalidClassReport},
		{"negative bytes", strings.Replace(valid, `"artifact":512`, `"artifact":-512`, 1), ErrInvalidClassReport},
		{"bytes over uint64", strings.Replace(valid, `"unattributed":7`, `"unattributed":18446744073709551616`, 1), ErrInvalidClassReport},
		{"observedAt not RFC 3339", strings.Replace(valid, `2026-11-13T09:30:00Z`, `2026-11-13 09:30:00`, 1), ErrInvalidClassReport},
		{"observedAt with offset", strings.Replace(valid, `2026-11-13T09:30:00Z`, `2026-11-13T10:30:00+01:00`, 1), ErrInvalidClassReport},
	} {
		t.Run("refuses raw "+tc.name, func(t *testing.T) {
			if _, err := ParseClassReport(tc.value); !errors.Is(err, tc.want) {
				t.Fatalf("ParseClassReport(%.200s) error = %v, want %v", tc.value, err, tc.want)
			}
		})
	}

	t.Run("accepts a payload padded to exactly the cap", func(t *testing.T) {
		s := valid[:len(valid)-1] + strings.Repeat(" ", MaxClassReportBytes-len(valid)) + "}"
		if _, err := ParseClassReport(s); err != nil {
			t.Fatalf("a payload of exactly %d bytes is refused: %v", MaxClassReportBytes, err)
		}
	})
}

// moveClass renames class from to to, keeping its bytes so the sum holds.
func moveClass(r *ClassReport, from, to string) {
	v := r.BytesByClass[from]
	delete(r.BytesByClass, from)
	r.BytesByClass[to] = v
}

// marshalUnchecked encodes r with no validation, the bytes a careless or
// hostile writer would put in the annotation.
func marshalUnchecked(t *testing.T, r ClassReport) string {
	t.Helper()
	keys := make([]string, 0, len(r.BytesByClass))
	for k := range r.BytesByClass {
		keys = append(keys, k)
	}
	var b strings.Builder
	at, err := r.ObservedAt.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal observedAt: %v", err)
	}
	fmt.Fprintf(&b, `{"schema":%d,"observedAt":%s,"storeBytes":%d,"bytesByClass":{`, r.Schema, at, r.StoreBytes)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%q:%d", k, r.BytesByClass[k])
	}
	b.WriteString("}}")
	return b.String()
}
