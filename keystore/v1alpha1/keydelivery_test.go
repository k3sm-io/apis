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

package keystorev1alpha1

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// sampleKeyDelivery sets every field, so the deep-copy and wire checks cover
// every field and every reference type. A field added belongs here too.
func sampleKeyDelivery() *KeyDelivery {
	return &KeyDelivery{
		TypeMeta:   metav1.TypeMeta{APIVersion: "keystore.k3sm.io/v1alpha1", Kind: "KeyDelivery"},
		ObjectMeta: metav1.ObjectMeta{Name: "claude", Namespace: "agents", Generation: 1},
		Spec: KeyDeliverySpec{
			Entry:       "user/501/claude-a",
			SecretName:  "claude-token",
			DataKey:     "token",
			RequestedBy: "k3sm:local:501",
		},
		Status: KeyDeliveryStatus{
			ObservedGeneration: 1,
			Conditions: []metav1.Condition{{
				Type: ConditionReady, Status: metav1.ConditionTrue, Reason: ReasonDelivered,
				ObservedGeneration: 1, LastTransitionTime: metav1.NewTime(time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)),
			}},
			DeliveredVersion: 3,
		},
	}
}

// TestKeyDeliveryGVK asserts the scheme registers both kinds under
// keystore.k3sm.io/v1alpha1.
func TestKeyDeliveryGVK(t *testing.T) {
	t.Parallel()
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"KeyDelivery", "KeyDeliveryList"} {
		gvk := schema.GroupVersionKind{Group: "keystore.k3sm.io", Version: "v1alpha1", Kind: kind}
		if !s.Recognizes(gvk) {
			t.Errorf("scheme does not recognise %v", gvk)
		}
	}
	if got := Resource("keydeliveries").String(); got != "keydeliveries.keystore.k3sm.io" {
		t.Errorf("Resource = %q", got)
	}
}

// TestKeyDeliveryWireNames pins the JSON field names the CRD schema declares.
func TestKeyDeliveryWireNames(t *testing.T) {
	t.Parallel()
	b, err := json.Marshal(sampleKeyDelivery())
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Spec, Status map[string]any
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"entry", "secretName", "dataKey", "requestedBy"} {
		if _, ok := m.Spec[f]; !ok {
			t.Errorf("spec.%s missing from the wire form", f)
		}
	}
	for _, f := range []string{"observedGeneration", "conditions", "deliveredVersion"} {
		if _, ok := m.Status[f]; !ok {
			t.Errorf("status.%s missing from the wire form", f)
		}
	}
}

// TestKeyDeliveryDeepCopy asserts a deep copy is equal and shares no memory.
func TestKeyDeliveryDeepCopy(t *testing.T) {
	t.Parallel()
	in := sampleKeyDelivery()
	out := in.DeepCopy()
	if !reflect.DeepEqual(in, out) {
		t.Fatal("deep copy differs")
	}
	out.Status.Conditions[0].Reason = "Changed"
	if in.Status.Conditions[0].Reason != ReasonDelivered {
		t.Error("deep copy shares Conditions")
	}
	list := &KeyDeliveryList{Items: []KeyDelivery{*sampleKeyDelivery()}}
	lc, ok := list.DeepCopyObject().(*KeyDeliveryList)
	if !ok || !reflect.DeepEqual(list, lc) {
		t.Fatal("list deep copy differs")
	}
	lc.Items[0].Status.Conditions[0].Reason = "Changed"
	if list.Items[0].Status.Conditions[0].Reason != ReasonDelivered {
		t.Error("list deep copy shares Items")
	}
	var nilKD *KeyDelivery
	if nilKD.DeepCopy() != nil || nilKD.DeepCopyObject() != nil {
		t.Error("nil DeepCopy is not nil")
	}
}

// TestConditionReasonsClosedSet pins the per-condition reason set.
func TestConditionReasonsClosedSet(t *testing.T) {
	t.Parallel()
	want := map[string][]string{
		"Authorized":     {"Authorized", "EntryUnavailable", "NotRecorded", "NotAuthorized", "Indeterminate"},
		"Ready":          {"Pending", "Delivered", "NotAuthorized", "SecretExists", "SecretClaimed", "EncryptionOff", "UserScopeOff"},
		"SecretRetained": {"DeletePending", "Unowned", "Indeterminate", "LabelRemoved", "UserScopeOff", "OwnerUnresolvable", "NoneRetained"},
	}
	got := ConditionReasons()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ConditionReasons = %v, want %v", got, want)
	}
	got[ConditionReady][0] = "X"
	if ConditionReasons()[ConditionReady][0] != ReasonPending {
		t.Error("ConditionReasons returns aliased slices")
	}
}
