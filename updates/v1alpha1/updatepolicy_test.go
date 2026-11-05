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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const updatePolicyGolden = "updatepolicy.golden.json"

// sampleUpdatePolicy is a fully populated UpdatePolicy. A field added to the
// types belongs here too.
func sampleUpdatePolicy() *UpdatePolicy {
	return &UpdatePolicy{
		TypeMeta:   metav1.TypeMeta{APIVersion: "updates.k3sm.io/v1alpha1", Kind: "UpdatePolicy"},
		ObjectMeta: metav1.ObjectMeta{Name: UpdatePolicyName, Generation: 2, Labels: map[string]string{"a": "b"}},
		Spec:       UpdatePolicySpec{Channel: UpdateChannelRC},
	}
}

// TestUpdatePolicyGVK asserts the scheme registers both kinds in the group's
// alpha version, beside Update.
func TestUpdatePolicyGVK(t *testing.T) {
	t.Parallel()
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	for _, tc := range []struct {
		kind string
		obj  runtime.Object
	}{
		{"UpdatePolicy", &UpdatePolicy{}},
		{"UpdatePolicyList", &UpdatePolicyList{}},
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
}

// TestUpdatePolicyJSONGolden pins the serialized shape against testdata: the
// JSON field names are the CRD's schema property names and what an admin
// types. Regenerate deliberately with UPDATE_GOLDEN=1.
func TestUpdatePolicyJSONGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(sampleUpdatePolicy(), "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", updatePolicyGolden)
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
		t.Errorf("UpdatePolicy JSON differs from testdata/%s\n got = %s\nwant = %s", updatePolicyGolden, got, want)
	}
}

// TestUpdatePolicyZeroChannelIsOmitted asserts the zero value of the Go field
// is not sent: the closed enum would refuse an empty string, and the CRD's
// default fills in the omitted field.
func TestUpdatePolicyZeroChannelIsOmitted(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(UpdatePolicySpec{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != "{}" {
		t.Fatalf("zero spec = %s, want {}", b)
	}
}

// TestUpdatePolicySharedKeys pins the strings consumers share: the singleton
// name, the channel values, and the Node annotation carrying the channel,
// which is under k3sm.io/* and not the CRD group.
func TestUpdatePolicySharedKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, got, want string }{
		{"UpdatePolicyName", UpdatePolicyName, "cluster"},
		{"UpdateChannelStable", string(UpdateChannelStable), "stable"},
		{"UpdateChannelRC", string(UpdateChannelRC), "rc"},
		{"AnnotationReleaseChannel", AnnotationReleaseChannel, "k3sm.io/release-channel"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	if strings.HasPrefix(AnnotationReleaseChannel, GroupName) {
		t.Errorf("AnnotationReleaseChannel is under the CRD group %s; annotations are k3sm.io/*", GroupName)
	}
}

// TestUpdatePolicyDeepCopy asserts DeepCopy shares no map or slice with the
// original.
func TestUpdatePolicyDeepCopy(t *testing.T) {
	t.Parallel()
	orig := sampleUpdatePolicy()
	cp := orig.DeepCopy()
	orig.Labels["a"] = "x"
	orig.Spec.Channel = UpdateChannelStable
	if !reflect.DeepEqual(cp, sampleUpdatePolicy()) {
		t.Fatal("DeepCopy shares state with the original")
	}
	if _, ok := orig.DeepCopyObject().(*UpdatePolicy); !ok {
		t.Fatal("DeepCopyObject did not return *UpdatePolicy")
	}
	if (*UpdatePolicy)(nil).DeepCopy() != nil {
		t.Fatal("(*UpdatePolicy)(nil).DeepCopy() != nil")
	}
	if (*UpdatePolicySpec)(nil).DeepCopy() != nil {
		t.Fatal("(*UpdatePolicySpec)(nil).DeepCopy() != nil")
	}
	list := &UpdatePolicyList{Items: []UpdatePolicy{*sampleUpdatePolicy()}}
	lcp := list.DeepCopy()
	list.Items[0].Labels["a"] = "x"
	if lcp.Items[0].Labels["a"] != "b" {
		t.Fatal("list DeepCopy shared items")
	}
	if _, ok := list.DeepCopyObject().(*UpdatePolicyList); !ok {
		t.Fatal("DeepCopyObject did not return *UpdatePolicyList")
	}
}
