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

package k3smtest_test

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"k3sm.io/apis/config/crd"
	updatesv1alpha1 "k3sm.io/apis/updates/v1alpha1"
)

// updatePolicyNameRule is the root CEL rule of the UpdatePolicy manifest, by
// its exact text. It compares against the Go constant, so the rule's literal
// and UpdatePolicyName cannot drift apart unseen.
const updatePolicyNameRule = "self.metadata.name == 'cluster'"

func init() {
	celSelfTranscriptions[updatePolicyNameRule] = func(self any) bool {
		md, _ := self.(map[string]any)["metadata"].(map[string]any)
		return md["name"] == updatesv1alpha1.UpdatePolicyName
	}
}

// TestUpdatePolicyCRDRoundTrip is the contract test between the UpdatePolicy Go
// types and the embedded manifest: the object round-trips through a scheme
// codec, its wire form validates against the schema, the empty Go value and an
// object with no spec both take the stable default, and another name or
// channel is refused. The name rule runs as the Go transcription above, not as
// CEL; the live-apiserver proof belongs to the consumer that carries
// apiextensions.
func TestUpdatePolicyCRDRoundTrip(t *testing.T) {
	t.Parallel()
	root := crdSchema(t, crd.UpdatePolicyCRD())
	policy := func(name string, ch updatesv1alpha1.UpdateChannel) *updatesv1alpha1.UpdatePolicy {
		return &updatesv1alpha1.UpdatePolicy{
			TypeMeta:   metav1.TypeMeta{APIVersion: "updates.k3sm.io/v1alpha1", Kind: "UpdatePolicy"},
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       updatesv1alpha1.UpdatePolicySpec{Channel: ch},
		}
	}
	wire := func(t *testing.T, p *updatesv1alpha1.UpdatePolicy) map[string]any {
		t.Helper()
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return m
	}

	t.Run("scheme round trip", func(t *testing.T) {
		s := runtime.NewScheme()
		if err := updatesv1alpha1.AddToScheme(s); err != nil {
			t.Fatalf("AddToScheme: %v", err)
		}
		codecs := serializer.NewCodecFactory(s)
		b, err := runtime.Encode(codecs.LegacyCodec(updatesv1alpha1.SchemeGroupVersion), policy(updatesv1alpha1.UpdatePolicyName, updatesv1alpha1.UpdateChannelRC))
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		obj, gvk, err := codecs.UniversalDeserializer().Decode(b, nil, nil)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if gvk.Group != "updates.k3sm.io" || gvk.Version != "v1alpha1" || gvk.Kind != "UpdatePolicy" {
			t.Errorf("decoded GVK = %v", gvk)
		}
		got, ok := obj.(*updatesv1alpha1.UpdatePolicy)
		if !ok {
			t.Fatalf("decoded %T, want *UpdatePolicy", obj)
		}
		if got.Name != updatesv1alpha1.UpdatePolicyName || got.Spec.Channel != updatesv1alpha1.UpdateChannelRC {
			t.Errorf("decoded %q / %q", got.Name, got.Spec.Channel)
		}
	})

	t.Run("every channel constant validates", func(t *testing.T) {
		for _, ch := range []updatesv1alpha1.UpdateChannel{updatesv1alpha1.UpdateChannelStable, updatesv1alpha1.UpdateChannelRC} {
			if errs := validate("", wire(t, policy(updatesv1alpha1.UpdatePolicyName, ch)), root); len(errs) != 0 {
				t.Errorf("channel %q refused: %v", ch, errs)
			}
		}
	})

	t.Run("the empty channel and a missing spec default to stable", func(t *testing.T) {
		noSpec := wire(t, policy(updatesv1alpha1.UpdatePolicyName, ""))
		delete(noSpec, "spec")
		for name, m := range map[string]map[string]any{
			"empty channel": wire(t, policy(updatesv1alpha1.UpdatePolicyName, "")),
			"no spec":       noSpec,
		} {
			applyDefaults(m, root)
			if errs := validate("", m, root); len(errs) != 0 {
				t.Errorf("%s refused: %v", name, errs)
			}
			sp, _ := m["spec"].(map[string]any)
			if sp["channel"] != string(updatesv1alpha1.UpdateChannelStable) {
				t.Errorf("%s: channel = %v, want stable", name, sp["channel"])
			}
		}
	})

	t.Run("refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			obj  map[string]any
			want string
		}{
			{"another name", wire(t, policy("current", updatesv1alpha1.UpdateChannelStable)), "must be named cluster"},
			{"another channel", wire(t, policy(updatesv1alpha1.UpdatePolicyName, "beta")), "spec.channel: not in enum"},
			{"a channel of the wrong case", wire(t, policy(updatesv1alpha1.UpdatePolicyName, "RC")), "spec.channel: not in enum"},
		} {
			if errs := validate("", tc.obj, root); !refused(errs, tc.want) {
				t.Errorf("%s: errors = %v, want one containing %q", tc.name, errs, tc.want)
			}
		}
		extra := wire(t, policy(updatesv1alpha1.UpdatePolicyName, updatesv1alpha1.UpdateChannelStable))
		extra["status"] = map[string]any{}
		if errs := validate("", extra, root); !refused(errs, "status: not declared") {
			t.Errorf("a status: errors = %v, want it not declared", errs)
		}
	})
}
