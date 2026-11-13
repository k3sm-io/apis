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
	"maps"
	"reflect"
	"testing"
	"time"
)

// FuzzParseClassReport holds ParseClassReport to its contract on arbitrary
// bytes a node could write: it never panics; an error always comes with the
// zero ClassReport and wraps one of the two top-level sentinels; and
// whatever it accepts, Encode accepts too and re-parses to an equal report,
// so a reader never accepts a value no writer could produce. The seeds live
// in testdata/fuzz/FuzzParseClassReport.
func FuzzParseClassReport(f *testing.F) {
	f.Fuzz(func(t *testing.T, value string) {
		r, err := ParseClassReport(value)
		if err != nil {
			if !reflect.DeepEqual(r, ClassReport{}) {
				t.Fatalf("error %v returned a non-zero report %+v", err, r)
			}
			if !errors.Is(err, ErrInvalidClassReport) && !errors.Is(err, ErrUnknownClassReportSchema) {
				t.Fatalf("error %v wraps neither sentinel", err)
			}
			return
		}
		if r.BytesByClass == nil {
			t.Fatal("accepted report has a nil BytesByClass")
		}
		s, err := r.Encode()
		if err != nil {
			t.Fatalf("Encode of an accepted report: %v", err)
		}
		again, err := ParseClassReport(s)
		if err != nil {
			t.Fatalf("re-parse of %q: %v", s, err)
		}
		if again.Schema != r.Schema || again.StoreBytes != r.StoreBytes ||
			!again.ObservedAt.Equal(r.ObservedAt) || again.ObservedAt.Location() != time.UTC ||
			!maps.Equal(again.BytesByClass, r.BytesByClass) {
			t.Fatalf("round trip = %+v, want %+v", again, r)
		}
		if s2, err := again.Encode(); err != nil || s2 != s {
			t.Fatalf("re-Encode = %q, %v; want %q", s2, err, s)
		}
	})
}
