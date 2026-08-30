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

package guestv1

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// mustTime is the fixed instant every fixture in this package uses, so goldens
// never depend on the clock.
func mustTime(t *testing.T) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, "2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatalf("parse fixture time: %v", err)
	}
	return ts
}

// goldenGuestSpec is the fixture behind testdata/guest-spec.json: a pod with one
// init container and one main container, a virtiofs rootfs each, a projected
// read-only share, a bounded tmpfs, and an idmapped writable claim.
func goldenGuestSpec() *GuestSpec {
	return &GuestSpec{
		Hostname: "web-0",
		ResolvConf: &ResolvConf{
			Nameservers: []string{"10.43.0.10"},
			Searches:    []string{"default.svc.cluster.local", "svc.cluster.local", "cluster.local"},
			Options:     []string{"ndots:5"},
		},
		Containers: []*GuestContainer{
			{
				Name:       "init-db",
				RootfsTag:  "k3sm.rootfs.init-db",
				Command:    []string{"/bin/sh", "-c"},
				Args:       []string{"initdb /pgdata"},
				Env:        []string{"PGDATA=/pgdata"},
				WorkingDir: "/",
				Uid:        999,
				Gid:        999,
				Init:       true,
			},
			{
				Name:             "postgres",
				RootfsTag:        "k3sm.rootfs.postgres",
				Command:          []string{"/usr/local/bin/postgres"},
				Args:             []string{"-D", "/pgdata"},
				Env:              []string{"PGDATA=/pgdata", "POSTGRES_DB=stockkitty"},
				WorkingDir:       "/var/lib/postgresql",
				Tty:              false,
				Stdin:            false,
				Uid:              999,
				Gid:              999,
				SupplementalGids: []int64{999, 2000},
			},
		},
		Mounts: []*GuestMount{
			{
				TagOrSource: "k3sm.proj",
				Target:      "/var/run/secrets/kubernetes.io/serviceaccount",
				Kind:        GuestMountKind_GUEST_MOUNT_KIND_VIRTIOFS,
				ReadOnly:    true,
			},
			{
				TagOrSource:    "",
				Target:         "/dev/shm",
				Kind:           GuestMountKind_GUEST_MOUNT_KIND_TMPFS,
				SizeLimitBytes: 67108864,
			},
			{
				TagOrSource: "k3sm.pvc.default.pgdata",
				Target:      "/pgdata",
				Kind:        GuestMountKind_GUEST_MOUNT_KIND_VIRTIOFS,
				Idmap:       true,
			},
		},
		Rosetta:   true,
		FsGroup:   2000,
		AgentPort: 1024,
	}
}

// goldenVMHostSpec is the fixture behind testdata/vmhost.spec.json: the machine
// the above pod boots in.
func goldenVMHostSpec() *VMHostSpec {
	return &VMHostSpec{
		PodId:          "0f6d0b3a-1c2e-4f5a-9b7c-8d9e0f1a2b3c",
		Vcpus:          2,
		MemoryBytes:    2147483648,
		KernelPath:     "/var/lib/k3sm/guest/vmlinuz-6.12.0-k3sm1",
		InitramfsPath:  "/var/lib/k3sm/guest/initramfs-6.12.0-k3sm1.cpio",
		Cmdline:        "console=hvc0 reboot=k panic=1 quiet",
		Rosetta:        true,
		AgentVsockPort: 1024,
		MacAddress:     "5a:9b:7c:8d:9e:0f",
		Shares: []*VMShare{
			{Tag: "k3sm.rootfs.postgres", HostPath: "/var/lib/k3sm/pods/0f6d0b3a/rootfs/postgres", ReadOnly: true},
			{Tag: "k3sm.proj", HostPath: "/var/lib/k3sm/pods/0f6d0b3a/projected", ReadOnly: true},
			{Tag: "k3sm.pvc.default.pgdata", HostPath: "/var/lib/k3sm/storage/default/pgdata", ReadOnly: false},
		},
	}
}

// TestSpecProtoJSONGoldens is the a2 golden gate for the two on-disk spec files.
//
// `guest-spec.json` and `vmhost.spec.json` ARE the proto-JSON encodings of
// GuestSpec and VMHostSpec — there is no second schema for them anywhere, so
// these goldens are the schema's only executable statement. Both directions are
// asserted, because both are real:
//
//   - PARSE (guest init / vmhost read the file): the golden must decode to the
//     fixture message exactly, with unknown fields REJECTED — a typo'd key in a
//     spec must fail loudly at boot rather than be silently dropped.
//   - EMIT (the host writes the file): the fixture must encode to the golden's
//     content. protojson deliberately varies its whitespace, so the comparison
//     is over the decoded JSON values, never the raw bytes.
func TestSpecProtoJSONGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		golden string
		want   proto.Message
	}{
		{"guest-spec.json", goldenGuestSpec()},
		{"vmhost.spec.json", goldenVMHostSpec()},
	}

	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", tc.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}

			t.Run("the golden parses to the fixture, rejecting unknown fields", func(t *testing.T) {
				got := tc.want.ProtoReflect().New().Interface()
				if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(raw, got); err != nil {
					t.Fatalf("protojson.Unmarshal: %v", err)
				}
				if !proto.Equal(tc.want, got) {
					t.Errorf("golden decodes to a different message:\n got: %v\nwant: %v", got, tc.want)
				}
			})

			t.Run("the fixture encodes to the golden", func(t *testing.T) {
				out, err := protojson.Marshal(tc.want)
				if err != nil {
					t.Fatalf("protojson.Marshal: %v", err)
				}
				var gotV, wantV any
				if err := json.Unmarshal(out, &gotV); err != nil {
					t.Fatalf("re-decode emitted json: %v", err)
				}
				if err := json.Unmarshal(raw, &wantV); err != nil {
					t.Fatalf("re-decode golden json: %v", err)
				}
				if !reflect.DeepEqual(gotV, wantV) {
					t.Errorf("emitted proto-JSON differs from the golden:\n got: %s\nwant: %s", out, raw)
				}
			})
		})
	}
}
