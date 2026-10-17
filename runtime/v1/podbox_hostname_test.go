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
	"bytes"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// podBoxHostnameNumber is the wire number PodBox.hostname is pinned to.
const podBoxHostnameNumber protoreflect.FieldNumber = 20

// TestPodBoxHostnameField pins PodBox.hostname: field 20, a singular string with
// JSON name hostname, carried on the wire as a known field. It reaches the
// field through the descriptor and protoreflect only, never the generated
// accessor, so on a tree without the field it fails at runtime, not compile
// time.
func TestPodBoxHostnameField(t *testing.T) {
	t.Parallel()

	md := File_runtime_v1_runtime_proto.Messages().ByName("PodBox")
	if md == nil {
		t.Fatal("message k3sm.runtime.v1.PodBox not found")
	}

	t.Run("field shape", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByName("hostname")
		if fd == nil {
			t.Fatal("PodBox.hostname is not declared")
		}
		checks := []struct {
			what string
			ok   bool
			got  any
		}{
			{"number is 20", fd.Number() == podBoxHostnameNumber, fd.Number()},
			{"kind is string", fd.Kind() == protoreflect.StringKind, fd.Kind()},
			{"is singular", !fd.IsList() && !fd.IsMap(), fd.Cardinality()},
			{"JSON name is hostname", fd.JSONName() == "hostname", fd.JSONName()},
		}
		for _, c := range checks {
			if !c.ok {
				t.Errorf("hostname: want %s, got %v", c.what, c.got)
			}
		}
	})

	t.Run("field 20 round-trips as a known field", func(t *testing.T) {
		t.Parallel()
		fd := md.Fields().ByNumber(podBoxHostnameNumber)
		if fd == nil {
			t.Fatalf("PodBox declares no field number %d", podBoxHostnameNumber)
		}
		const want = "web-0.web.default.svc"
		raw := protowire.AppendTag(nil, podBoxHostnameNumber, protowire.BytesType)
		raw = protowire.AppendString(raw, want)

		var pb PodBox
		if err := proto.Unmarshal(raw, &pb); err != nil {
			t.Fatalf("unmarshal field %d: %v", podBoxHostnameNumber, err)
		}
		if u := pb.ProtoReflect().GetUnknown(); len(u) != 0 {
			t.Errorf("field %d decoded as unknown bytes %x, want a known field", podBoxHostnameNumber, u)
		}
		if got := pb.ProtoReflect().Get(fd).String(); got != want {
			t.Errorf("decoded hostname = %q, want %q", got, want)
		}
		out, err := proto.MarshalOptions{Deterministic: true}.Marshal(&pb)
		if err != nil {
			t.Fatalf("re-marshal: %v", err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("re-marshal = %x, want %x", out, raw)
		}
	})

	t.Run("a PodBox without hostname encodes as before", func(t *testing.T) {
		t.Parallel()
		// The bytes a producer that predates the field sends, built by hand
		// from the wire format rather than by the generated code under test.
		var want []byte
		want = protowire.AppendTag(want, 1, protowire.BytesType)
		want = protowire.AppendString(want, "uid-1")
		want = protowire.AppendTag(want, 2, protowire.BytesType)
		want = protowire.AppendString(want, "default")
		want = protowire.AppendTag(want, 3, protowire.BytesType)
		want = protowire.AppendString(want, "web-0")
		want = protowire.AppendTag(want, 19, protowire.BytesType)
		want = protowire.AppendString(want, "/var/log/pods/default_web-0_uid-1")

		pb := &PodBox{
			PodId:        "uid-1",
			Namespace:    "default",
			Name:         "web-0",
			LogDirectory: "/var/log/pods/default_web-0_uid-1",
		}
		got, err := proto.MarshalOptions{Deterministic: true}.Marshal(pb)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("PodBox without hostname = %x, want %x", got, want)
		}
	})

	t.Run("retired fields and the reserved band are unchanged", func(t *testing.T) {
		t.Parallel()
		pinned := retiredField{message: "PodBox", number: 4, name: "rootfs_path", jsonName: "rootfsPath"}
		var podBoxRetired []retiredField
		for _, rf := range retiredFields {
			if rf.message == "PodBox" {
				podBoxRetired = append(podBoxRetired, rf)
			}
		}
		if len(podBoxRetired) != 1 || podBoxRetired[0] != pinned {
			t.Errorf("PodBox retired fields = %v, want exactly [%v]", podBoxRetired, pinned)
		}
		if fd := md.Fields().ByNumber(pinned.number); fd != nil {
			t.Errorf("PodBox reuses retired number %d as %q", pinned.number, fd.Name())
		}
		rr := md.ReservedRanges()
		if !rr.Has(pinned.number) {
			t.Errorf("PodBox does not reserve retired number %d", pinned.number)
		}
		if rr.Has(podBoxHostnameNumber) {
			t.Errorf("PodBox reserves number %d, which hostname uses", podBoxHostnameNumber)
		}
		assertPodBoxHeadroomBand(t, md)
	})
}
