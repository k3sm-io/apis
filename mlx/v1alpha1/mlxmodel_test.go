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

package mlxv1alpha1

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fixedMLXTime is a deterministic, second-precision UTC instant so metav1.Time
// status fields round-trip losslessly through RFC3339 JSON and the golden never
// depends on the clock.
var fixedMLXTime = metav1.NewTime(time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))

func int32Ptr(v int32) *int32 { return &v }

// sampleMLXModel is a fully-populated MLXModel — every reference-typed field set
// (pointer, slice, map, resource.Quantity, conditions) so the DeepCopy and
// round-trip cases exercise each aliasing hazard rather than the easy scalars.
// spec.distributed is deliberately set even though admission rejects it: the
// field must be REPRESENTABLE, and a copy/round-trip that dropped it would hide
// the very spec a rejection needs to describe.
func sampleMLXModel() *MLXModel {
	return &MLXModel{
		TypeMeta: metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "MLXModel"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "llama-3-2-3b",
			Namespace:       "ml",
			Generation:      3,
			ResourceVersion: "4711",
			Labels:          map[string]string{"app": "chat"},
		},
		Spec: MLXModelSpec{
			Model:        "mlx-community/Llama-3.2-3B-Instruct-4bit",
			Revision:     "b1a2c3d4e5f6",
			Quantization: "4bit",
			Memory:       resource.MustParse("24Gi"),
			Replicas:     int32Ptr(2),
			Port:         8080,
			Runtime: MLXRuntime{
				Image: "ghcr.io/k3sm-io/mlx-serve:v0.1.0",
				Args:  []string{"--max-tokens", "4096"},
			},
			Cache: &MLXCache{
				Size:             resource.MustParse("40Gi"),
				StorageClassName: "k3sm-local-path",
			},
			NodeSelector: map[string]string{LabelChipFamily: "m4"},
			Distributed:  &MLXDistributed{Nodes: 2},
		},
		Status: MLXModelStatus{
			Conditions: []metav1.Condition{{
				Type:               MLXModelConditionReady,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 3,
				LastTransitionTime: fixedMLXTime,
				Reason:             "Serving",
				Message:            "2/2 replicas serving",
			}},
			ObservedGeneration: 3,
			Phase:              MLXModelPhaseReady,
			Endpoint:           "llama-3-2-3b.ml.svc.cluster.local:8080",
			ResolvedRevision:   "b1a2c3d4e5f6",
		},
	}
}

// TestMLXModelGVK asserts the alpha branding is real: the types register under
// group mlx.k3sm.io at version v1alpha1.
//
// Both halves matter and for different reasons. A wrong GROUP means the
// operator's informer watches a resource path that does not exist, and the
// object silently never reconciles. A wrong VERSION means the compatibility
// promise in the package doc is a lie — v1alpha1 is what tells a user this shape
// may change under them, and a type registered as v1 has quietly made the
// opposite promise.
func TestMLXModelGVK(t *testing.T) {
	t.Parallel()

	if GroupName != "mlx.k3sm.io" {
		t.Errorf("GroupName = %q, want mlx.k3sm.io", GroupName)
	}
	if SchemeGroupVersion.Group != "mlx.k3sm.io" {
		t.Errorf("SchemeGroupVersion.Group = %q, want mlx.k3sm.io", SchemeGroupVersion.Group)
	}
	if SchemeGroupVersion.Version != "v1alpha1" {
		t.Errorf("SchemeGroupVersion.Version = %q, want v1alpha1 (this API is alpha-branded)", SchemeGroupVersion.Version)
	}

	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	cases := []struct {
		name string
		obj  runtime.Object
		want schema.GroupVersionKind
	}{
		{"MLXModel", &MLXModel{}, schema.GroupVersionKind{Group: "mlx.k3sm.io", Version: "v1alpha1", Kind: "MLXModel"}},
		{"MLXModelList", &MLXModelList{}, schema.GroupVersionKind{Group: "mlx.k3sm.io", Version: "v1alpha1", Kind: "MLXModelList"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gvks, _, err := s.ObjectKinds(tc.obj)
			if err != nil {
				t.Fatalf("ObjectKinds: %v", err)
			}
			found := false
			for _, gvk := range gvks {
				if gvk == tc.want {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("registered GVKs %v do not include %v", gvks, tc.want)
			}
		})
	}

	if got := Resource("mlxmodels"); got.Group != "mlx.k3sm.io" || got.Resource != "mlxmodels" {
		t.Fatalf("Resource(mlxmodels) = %v, want mlx.k3sm.io/mlxmodels", got)
	}
}

// TestMLXModelJSONRoundTrip asserts an MLXModel survives a JSON
// marshal→unmarshal cycle losslessly — the kine-stored, apiserver-served wire
// form. It is byte-stable (re-marshal equality) rather than DeepEqual to avoid
// the time.Time location/monotonic pitfalls, and it re-checks the fields whose
// types can lose information across the cycle in ways equality on the whole
// object would still catch but not explain.
func TestMLXModelJSONRoundTrip(t *testing.T) {
	t.Parallel()
	orig := sampleMLXModel()
	b1, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got MLXModel
	if err := json.Unmarshal(b1, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b2, err := json.Marshal(&got)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Fatalf("round-trip not byte-stable:\n first = %s\nsecond = %s", b1, b2)
	}

	cases := []struct {
		name      string
		got, want any
	}{
		// A Quantity that changes its canonical spelling across the cycle would
		// make every scheduling comparison against it suspect.
		{"spec.memory", got.Spec.Memory.String(), "24Gi"},
		{"spec.cache.size", got.Spec.Cache.Size.String(), "40Gi"},
		// An explicit replica count must not decay into "unset".
		{"spec.replicas", *got.Spec.Replicas, int32(2)},
		{"spec.runtime.args", got.Spec.Runtime.Args, []string{"--max-tokens", "4096"}},
		{"spec.nodeSelector", got.Spec.NodeSelector, map[string]string{LabelChipFamily: "m4"}},
		// The reserved seam must survive so a rejection can name it.
		{"spec.distributed.nodes", got.Spec.Distributed.Nodes, int32(2)},
		{"status.observedGeneration", got.Status.ObservedGeneration, int64(3)},
		{"status.phase", got.Status.Phase, MLXModelPhaseReady},
		{"status.conditions[0].type", got.Status.Conditions[0].Type, MLXModelConditionReady},
		// Compared as the wire spelling: metav1.Time unmarshals into the local
		// zone, so an equality on the struct would fail for a value that is in
		// fact the same instant.
		{"status.conditions[0].lastTransitionTime", got.Status.Conditions[0].LastTransitionTime.UTC().Format(time.RFC3339), "2026-08-29T12:00:00Z"},
		{"status.resolvedRevision", got.Status.ResolvedRevision, "b1a2c3d4e5f6"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestMLXModelJSONGolden pins the serialized shape against testdata.
//
// The JSON field names ARE the CRD's schema property names and the strings a
// user types in YAML. A Go-side rename that keeps the same tag is invisible; a
// tag change is a break for every stored object and every manifest in the world,
// and nothing else in this module would notice it. Regenerate deliberately with
// -update, never reflexively.
func TestMLXModelJSONGolden(t *testing.T) {
	t.Parallel()
	got, err := json.MarshalIndent(sampleMLXModel(), "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got = append(got, '\n')

	path := filepath.Join("testdata", "mlxmodel.json")
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
		t.Errorf("MLXModel JSON differs from testdata/mlxmodel.json\n got = %s\nwant = %s", got, want)
	}
}

// TestMLXModelDeepCopy asserts the hand-written DeepCopy is a real deep copy.
//
// Hand-written deepcopy is the price of running no code generation, and its
// failure mode is a shared backing array or map that a client-go informer's
// cache and a caller both hold — the caller's "local" edit then mutates the
// cache for every other consumer in the process. Each case mutates the ORIGINAL
// after copying and asserts the copy did not move, which is the only formulation
// that catches an aliased reference type.
func TestMLXModelDeepCopy(t *testing.T) {
	t.Parallel()

	t.Run("the copy equals the original", func(t *testing.T) {
		t.Parallel()
		orig := sampleMLXModel()
		cp := orig.DeepCopy()
		if !reflect.DeepEqual(orig, cp) {
			t.Fatalf("DeepCopy differs:\norig = %+v\n  cp = %+v", orig, cp)
		}
		if _, ok := orig.DeepCopyObject().(*MLXModel); !ok {
			t.Fatal("DeepCopyObject did not return *MLXModel")
		}
		// A copy that still serializes to the golden proves the copy is complete,
		// not merely equal under a comparison that could be skipping a field.
		a, err := json.MarshalIndent(cp, "", "  ")
		if err != nil {
			t.Fatalf("marshal copy: %v", err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", "mlxmodel.json"))
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		if !bytes.Equal(append(a, '\n'), want) {
			t.Errorf("the deep copy does not serialize to the golden:\n%s", a)
		}
	})

	cases := []struct {
		name    string
		mutate  func(*MLXModel)
		observe func(*MLXModel) any
		want    any
	}{
		{
			"spec.replicas pointer",
			func(m *MLXModel) { *m.Spec.Replicas = 99 },
			func(m *MLXModel) any { return *m.Spec.Replicas },
			int32(2),
		},
		{
			"spec.runtime.args slice",
			func(m *MLXModel) { m.Spec.Runtime.Args[0] = "--clobbered" },
			func(m *MLXModel) any { return m.Spec.Runtime.Args[0] },
			"--max-tokens",
		},
		{
			"spec.nodeSelector map",
			func(m *MLXModel) { m.Spec.NodeSelector[LabelChipFamily] = "m1" },
			func(m *MLXModel) any { return m.Spec.NodeSelector[LabelChipFamily] },
			"m4",
		},
		{
			"spec.cache pointer",
			func(m *MLXModel) { m.Spec.Cache.StorageClassName = "clobbered" },
			func(m *MLXModel) any { return m.Spec.Cache.StorageClassName },
			"k3sm-local-path",
		},
		{
			"spec.distributed pointer",
			func(m *MLXModel) { m.Spec.Distributed.Nodes = 99 },
			func(m *MLXModel) any { return m.Spec.Distributed.Nodes },
			int32(2),
		},
		{
			"status.conditions slice",
			func(m *MLXModel) { m.Status.Conditions[0].Reason = "Clobbered" },
			func(m *MLXModel) any { return m.Status.Conditions[0].Reason },
			"Serving",
		},
		{
			"metadata.labels map",
			func(m *MLXModel) { m.ObjectMeta.Labels["app"] = "clobbered" },
			func(m *MLXModel) any { return m.ObjectMeta.Labels["app"] },
			"chat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orig := sampleMLXModel()
			cp := orig.DeepCopy()
			tc.mutate(orig)
			if got := tc.observe(cp); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("mutating the original changed the copy: %s = %v, want %v", tc.name, got, tc.want)
			}
		})
	}

	t.Run("nil receivers copy to nil", func(t *testing.T) {
		t.Parallel()
		var m *MLXModel
		if m.DeepCopy() != nil {
			t.Error("(*MLXModel)(nil).DeepCopy() != nil")
		}
		if m.DeepCopyObject() != nil {
			t.Error("(*MLXModel)(nil).DeepCopyObject() != nil")
		}
		var c *MLXCache
		if c.DeepCopy() != nil {
			t.Error("(*MLXCache)(nil).DeepCopy() != nil")
		}
		var d *MLXDistributed
		if d.DeepCopy() != nil {
			t.Error("(*MLXDistributed)(nil).DeepCopy() != nil")
		}
		var r *MLXRuntime
		if r.DeepCopy() != nil {
			t.Error("(*MLXRuntime)(nil).DeepCopy() != nil")
		}
	})

	t.Run("an empty spec copies without inventing values", func(t *testing.T) {
		t.Parallel()
		// The optional pointers must stay nil: a DeepCopy that helpfully
		// materialized an empty Cache or Distributed would turn "no cache" into
		// "make me a PVC" and "not sharded" into an admission rejection.
		empty := &MLXModel{Spec: MLXModelSpec{Model: "m"}}
		cp := empty.DeepCopy()
		if cp.Spec.Cache != nil || cp.Spec.Distributed != nil || cp.Spec.Replicas != nil {
			t.Errorf("DeepCopy materialized optional fields: %+v", cp.Spec)
		}
		if cp.Spec.Runtime.Args != nil || cp.Spec.NodeSelector != nil {
			t.Errorf("DeepCopy materialized nil collections: %+v", cp.Spec)
		}
	})
}

// TestMLXModelListDeepCopy asserts the list copy is deep in its Items, which is
// the copy an informer actually makes on every LIST.
func TestMLXModelListDeepCopy(t *testing.T) {
	t.Parallel()
	list := &MLXModelList{
		TypeMeta: metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "MLXModelList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "9"},
		Items:    []MLXModel{*sampleMLXModel()},
	}
	cp := list.DeepCopy()
	if !reflect.DeepEqual(list, cp) {
		t.Fatalf("DeepCopy differs:\norig = %+v\n  cp = %+v", list, cp)
	}
	list.Items[0].Spec.Runtime.Args[0] = "--clobbered"
	list.Items[0].Spec.NodeSelector[LabelChipFamily] = "m1"
	if got := cp.Items[0].Spec.Runtime.Args[0]; got != "--max-tokens" {
		t.Errorf("list item args aliased: %q", got)
	}
	if got := cp.Items[0].Spec.NodeSelector[LabelChipFamily]; got != "m4" {
		t.Errorf("list item nodeSelector aliased: %q", got)
	}
	if _, ok := list.DeepCopyObject().(*MLXModelList); !ok {
		t.Fatal("DeepCopyObject did not return *MLXModelList")
	}
	var nilList *MLXModelList
	if nilList.DeepCopy() != nil || nilList.DeepCopyObject() != nil {
		t.Error("(*MLXModelList)(nil) deep copies are not nil")
	}
}

// TestMLXModelPhaseValues pins the derived phase vocabulary. The phase is a
// printer column, so its values are seen by humans and scripted against by
// people regardless of the advice not to; renaming one is user-visible.
func TestMLXModelPhaseValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  MLXModelPhase
		want string
	}{
		{"Pending", MLXModelPhasePending, "Pending"},
		{"Downloading", MLXModelPhaseDownloading, "Downloading"},
		{"Loading", MLXModelPhaseLoading, "Loading"},
		{"Ready", MLXModelPhaseReady, "Ready"},
		{"Failed", MLXModelPhaseFailed, "Failed"},
	}
	seen := map[MLXModelPhase]string{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
		if prev, dup := seen[tc.got]; dup {
			t.Errorf("phase %q is shared by %s and %s", tc.got, prev, tc.name)
		}
		seen[tc.got] = tc.name
	}
	if MLXModelConditionReady != "Ready" {
		t.Errorf("MLXModelConditionReady = %q, want Ready", MLXModelConditionReady)
	}
}
