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

package updatesv1alpha1

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const updateGolden = "update.golden.json"

// fixedUpdateTime is a deterministic, second-precision UTC instant so the
// metav1.Time status fields round-trip losslessly through RFC3339 JSON.
var fixedUpdateTime = metav1.NewTime(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))

// sampleUpdate is a fully populated Update: every field set to a non-zero
// value, so the golden and the deep-copy checks cover every field and every
// reference type. A field added to the types belongs here too.
func sampleUpdate() *Update {
	started := fixedUpdateTime
	finished := metav1.NewTime(fixedUpdateTime.Add(90 * time.Second))
	return &Update{
		TypeMeta:   metav1.TypeMeta{APIVersion: "updates.k3sm.io/v1alpha1", Kind: "Update"},
		ObjectMeta: metav1.ObjectMeta{Name: UpdateName, Generation: 2},
		Spec: UpdateSpec{
			Version: "v0.2.0",
			Strategy: UpdateStrategy{
				Order:          UpdateOrderWorkersThenServer,
				MaxUnavailable: 1,
				Drain:          true,
			},
			Verify: UpdateVerify{
				Signature: UpdateSignatureRequired,
			},
			AllowDowngrade: true,
			Cancel:         true,
		},
		Status: UpdateStatus{
			Phase: UpdatePhaseCancelled,
			Resolved: &UpdateResolved{
				Version: "v0.2.0",
				Digest:  "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			Nodes: []UpdateNodeStatus{
				{
					Name: "worker-1", From: "v0.1.7", To: "v0.2.0", Phase: UpdateNodePhaseDone,
					Message: "running v0.2.0", StartedAt: &started, FinishedAt: &finished,
				},
				{
					Name: "server-1", From: "v0.1.7", To: "v0.2.0", Phase: UpdateNodePhaseCancelled,
					Message: "cancelled before this node started", AdminTokenRotated: true,
					StartedAt: &started, FinishedAt: &finished,
				},
			},
			Conditions: []metav1.Condition{{
				Type:               "Progressing",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: 2,
				LastTransitionTime: fixedUpdateTime,
				Reason:             "Cancelled",
				Message:            "cancelled after 1 of 2 nodes",
			}},
			ObservedGeneration: 2,
		},
	}
}

// TestUpdateGVK asserts the group, the alpha version and the scheme
// registration of both kinds.
func TestUpdateGVK(t *testing.T) {
	t.Parallel()
	if GroupName != "updates.k3sm.io" {
		t.Errorf("GroupName = %q, want updates.k3sm.io", GroupName)
	}
	if SchemeGroupVersion.Version != "v1alpha1" {
		t.Errorf("SchemeGroupVersion.Version = %q, want v1alpha1", SchemeGroupVersion.Version)
	}
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, tc := range []struct {
		kind string
		obj  runtime.Object
	}{
		{"Update", &Update{}},
		{"UpdateList", &UpdateList{}},
	} {
		gvks, _, err := s.ObjectKinds(tc.obj)
		if err != nil {
			t.Fatalf("ObjectKinds(%s): %v", tc.kind, err)
		}
		want := schema.GroupVersionKind{Group: GroupName, Version: "v1alpha1", Kind: tc.kind}
		if len(gvks) != 1 || gvks[0] != want {
			t.Fatalf("%s registered as %v, want %v", tc.kind, gvks, want)
		}
	}
	if got := Resource("updates"); got.Group != GroupName || got.Resource != "updates" {
		t.Fatalf("Resource(updates) = %v, want updates.k3sm.io/updates", got)
	}
}

// TestUpdateJSONGolden pins the serialized shape against testdata: the JSON
// field names are the CRD's schema property names and what an admin types.
// Regenerate deliberately with UPDATE_GOLDEN=1.
func TestUpdateJSONGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(sampleUpdate(), "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", updateGolden)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Update JSON differs from testdata/%s\n got = %s\nwant = %s", updateGolden, got, want)
	}
}

// TestUpdateSharedKeys pins the strings the controller, the root helper and
// any reader share: the singleton name, the Reconciled condition and its
// NotTheSingleton reason, and the k3sm.io/* Node annotation keys (which are
// not under the updates.k3sm.io group).
func TestUpdateSharedKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, got, want string }{
		{"UpdateName", UpdateName, "current"},
		{"UpdateConditionReconciled", UpdateConditionReconciled, "Reconciled"},
		{"UpdateReasonNotTheSingleton", UpdateReasonNotTheSingleton, "NotTheSingleton"},
		{"AnnotationUpdateVersion", AnnotationUpdateVersion, "k3sm.io/update-version"},
		{"AnnotationUpdatePayload", AnnotationUpdatePayload, "k3sm.io/update-payload"},
		{"AnnotationUpdatePhase", AnnotationUpdatePhase, "k3sm.io/update-phase"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	for _, k := range []string{AnnotationUpdateVersion, AnnotationUpdatePayload, AnnotationUpdatePhase} {
		if strings.HasPrefix(k, GroupName) {
			t.Errorf("%s is under the CRD group %s; annotations are k3sm.io/*", k, GroupName)
		}
	}
}

// TestUpdateDeepCopy asserts DeepCopy shares no slice or pointer with the
// original: each mutation of the original after copying must leave the copy
// equal to a fresh sample.
func TestUpdateDeepCopy(t *testing.T) {
	t.Parallel()
	orig := sampleUpdate()
	cp := orig.DeepCopy()
	orig.Status.Resolved.Digest = "x"
	orig.Status.Nodes[0].Name = "x"
	orig.Status.Nodes[0].StartedAt.Time = time.Unix(0, 0)
	orig.Status.Nodes[0].FinishedAt.Time = time.Unix(0, 0)
	orig.Status.Conditions[0].Reason = "x"
	if !reflect.DeepEqual(cp, sampleUpdate()) {
		t.Fatal("DeepCopy shares state with the original")
	}
	if _, ok := orig.DeepCopyObject().(*Update); !ok {
		t.Fatal("DeepCopyObject did not return *Update")
	}
	if (*Update)(nil).DeepCopy() != nil {
		t.Fatal("(*Update)(nil).DeepCopy() != nil")
	}
	list := &UpdateList{Items: []Update{*sampleUpdate()}}
	lcp := list.DeepCopy()
	list.Items[0].Status.Nodes[0].Name = "x"
	if lcp.Items[0].Status.Nodes[0].Name != "worker-1" {
		t.Fatal("list DeepCopy shared items")
	}
	if _, ok := list.DeepCopyObject().(*UpdateList); !ok {
		t.Fatal("DeepCopyObject did not return *UpdateList")
	}
}

// TestUpdateEnumValues pins the wire strings of every enum constant. The
// signature policy is a closed enum in the CRD; the phases are open there
// (display only), so these constants are the only list of the values this
// release writes.
func TestUpdateEnumValues(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ got, want string }{
		{string(UpdateOrderWorkersThenServer), "workers-then-server"},
		{string(UpdateSignatureRequired), "required"},
		{string(UpdateSignatureIfSigned), "if-signed"},
		{string(UpdatePhasePending), "Pending"},
		{string(UpdatePhaseRolling), "Rolling"},
		{string(UpdatePhaseSucceeded), "Succeeded"},
		{string(UpdatePhaseFailed), "Failed"},
		{string(UpdatePhaseCancelled), "Cancelled"},
		{string(UpdateNodePhasePending), "Pending"},
		{string(UpdateNodePhaseSkipped), "Skipped"},
		{string(UpdateNodePhaseCordoning), "Cordoning"},
		{string(UpdateNodePhaseDraining), "Draining"},
		{string(UpdateNodePhaseDownloading), "Downloading"},
		{string(UpdateNodePhaseVerifying), "Verifying"},
		{string(UpdateNodePhaseInstalling), "Installing"},
		{string(UpdateNodePhaseRestarting), "Restarting"},
		{string(UpdateNodePhaseDone), "Done"},
		{string(UpdateNodePhaseFailed), "Failed"},
		{string(UpdateNodePhaseCancelled), "Cancelled"},
	} {
		if tc.got != tc.want {
			t.Errorf("enum constant = %q, want %q", tc.got, tc.want)
		}
	}
}
