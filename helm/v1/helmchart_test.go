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

package helmv1

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// fixedHelmTime is a deterministic, second-precision UTC instant so the
// condition timestamps round-trip losslessly through RFC3339 JSON.
var fixedHelmTime = metav1.NewTime(time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC))

func int32Ptr(v int32) *int32 { return &v }

// sampleHelmChart is a fully-populated HelmChart: every reference-typed field
// (the set map, both pointers, the conditions slice and its times) is set so
// the DeepCopy and round-trip cases exercise each aliasing hazard. The set map
// carries BOTH intstr forms, because the numeric one is the easy one to lose:
// an IntOrString that decays to a string turns `--set replicas=3` into a
// quoted "3" in the chart.
func sampleHelmChart() *HelmChart {
	return &HelmChart{
		TypeMeta: metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "HelmChart"},
		ObjectMeta: metav1.ObjectMeta{
			Name:            "traefik",
			Namespace:       "kube-system",
			Generation:      2,
			ResourceVersion: "812",
			Labels:          map[string]string{"app": "ingress"},
		},
		Spec: HelmChartSpec{
			Chart:           "traefik",
			Repo:            "https://traefik.github.io/charts",
			Version:         "34.1.0",
			TargetNamespace: "ingress",
			CreateNamespace: true,
			ValuesContent:   "service:\n  type: LoadBalancer\n",
			Set: map[string]intstr.IntOrString{
				"deployment.replicas": intstr.FromInt32(3),
				"logs.general.level":  intstr.FromString("INFO"),
			},
			ChartContent:   "H4sIAAAAAAAA",
			Timeout:        &metav1.Duration{Duration: 5 * time.Minute},
			Bootstrap:      true,
			BackOffLimit:   int32Ptr(1000),
			FailurePolicy:  FailurePolicyAbort,
			ForceConflicts: true,
			TakeOwnership:  true,
		},
		Status: HelmChartStatus{
			JobName: "helm-install-traefik",
			Conditions: []HelmChartCondition{{
				Type:               HelmChartJobCreated,
				Status:             metav1.ConditionTrue,
				LastUpdateTime:     fixedHelmTime,
				LastTransitionTime: fixedHelmTime,
				Reason:             "Created",
				Message:            "Job kube-system/helm-install-traefik created",
			}},
		},
	}
}

func sampleHelmChartConfig() *HelmChartConfig {
	return &HelmChartConfig{
		TypeMeta:   metav1.TypeMeta{APIVersion: SchemeGroupVersion.String(), Kind: "HelmChartConfig"},
		ObjectMeta: metav1.ObjectMeta{Name: "traefik", Namespace: "kube-system"},
		Spec: HelmChartConfigSpec{
			ValuesContent:  "ports:\n  web:\n    port: 8080\n",
			FailurePolicy:  FailurePolicyReinstall,
			ForceConflicts: true,
		},
	}
}

// TestHelmChartGVK asserts the four types register under helm.k3sm.io/v1. A
// wrong group makes the controller's informer watch a path that does not
// exist; a wrong version makes it watch one the server does not serve.
func TestHelmChartGVK(t *testing.T) {
	t.Parallel()
	if GroupName != "helm.k3sm.io" {
		t.Errorf("GroupName = %q, want helm.k3sm.io", GroupName)
	}
	if SchemeGroupVersion.Version != "v1" {
		t.Errorf("SchemeGroupVersion.Version = %q, want v1", SchemeGroupVersion.Version)
	}
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	cases := []struct {
		obj  runtime.Object
		kind string
	}{
		{&HelmChart{}, "HelmChart"},
		{&HelmChartList{}, "HelmChartList"},
		{&HelmChartConfig{}, "HelmChartConfig"},
		{&HelmChartConfigList{}, "HelmChartConfigList"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			want := schema.GroupVersionKind{Group: "helm.k3sm.io", Version: "v1", Kind: tc.kind}
			gvks, _, err := s.ObjectKinds(tc.obj)
			if err != nil {
				t.Fatalf("ObjectKinds: %v", err)
			}
			for _, gvk := range gvks {
				if gvk == want {
					return
				}
			}
			t.Fatalf("registered GVKs %v do not include %v", gvks, want)
		})
	}
	if got := Resource("helmcharts"); got.Group != "helm.k3sm.io" || got.Resource != "helmcharts" {
		t.Fatalf("Resource(helmcharts) = %v, want helm.k3sm.io/helmcharts", got)
	}
}

// TestHelmChartJSONGolden pins the serialized shape against testdata. The JSON
// names ARE k3s's helm.cattle.io/v1 names — the whole point of the mirror — and
// a tag change breaks every stored object and every manifest ported from k3s.
// Regenerate deliberately with UPDATE_GOLDEN=1, never reflexively.
func TestHelmChartJSONGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		obj  any
		file string
	}{
		{"HelmChart", sampleHelmChart(), "helmchart.json"},
		{"HelmChartConfig", sampleHelmChartConfig(), "helmchartconfig.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.MarshalIndent(tc.obj, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", tc.file)
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
				t.Errorf("%s JSON differs from testdata/%s\n got = %s\nwant = %s", tc.name, tc.file, got, want)
			}
		})
	}
}

// TestHelmChartJSONRoundTrip asserts the golden decodes back into the same
// object — the direction the controller actually exercises when it reads a
// user's manifest — and re-checks the fields whose types can lose information.
func TestHelmChartJSONRoundTrip(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("testdata", "helmchart.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var got HelmChart
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b2, err := json.MarshalIndent(&got, "", "  ")
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !bytes.Equal(append(b2, '\n'), b) {
		t.Fatalf("round-trip not byte-stable:\n got = %s\nwant = %s", b2, b)
	}
	cases := []struct {
		name      string
		got, want any
	}{
		// The numeric set value must stay an Int, not decay into a String.
		{"spec.set numeric type", got.Spec.Set["deployment.replicas"].Type, intstr.Int},
		{"spec.set numeric value", got.Spec.Set["deployment.replicas"].IntVal, int32(3)},
		{"spec.set string type", got.Spec.Set["logs.general.level"].Type, intstr.String},
		{"spec.timeout", got.Spec.Timeout.Duration, 5 * time.Minute},
		{"spec.backOffLimit", *got.Spec.BackOffLimit, int32(1000)},
		{"status.jobName", got.Status.JobName, "helm-install-traefik"},
		{"status.conditions[0].type", got.Status.Conditions[0].Type, HelmChartJobCreated},
		{"status.conditions[0].lastUpdateTime", got.Status.Conditions[0].LastUpdateTime.UTC().Format(time.RFC3339), "2026-09-26T12:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.want) {
				t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestHelmChartDeepCopy asserts the hand-written DeepCopy is a real deep copy:
// each case mutates the ORIGINAL after copying and asserts the copy did not
// move, the only formulation that catches an aliased reference type (an
// informer cache and a caller sharing a map is the failure this prevents).
func TestHelmChartDeepCopy(t *testing.T) {
	t.Parallel()

	t.Run("the copy equals the original", func(t *testing.T) {
		t.Parallel()
		orig := sampleHelmChart()
		if cp := orig.DeepCopy(); !reflect.DeepEqual(orig, cp) {
			t.Fatalf("DeepCopy differs:\norig = %+v\n  cp = %+v", orig, cp)
		}
		if _, ok := orig.DeepCopyObject().(*HelmChart); !ok {
			t.Fatal("DeepCopyObject did not return *HelmChart")
		}
	})

	cases := []struct {
		name    string
		mutate  func(*HelmChart)
		observe func(*HelmChart) any
		want    any
	}{
		{
			"spec.set map",
			func(h *HelmChart) { h.Spec.Set["deployment.replicas"] = intstr.FromInt32(99) },
			func(h *HelmChart) any { return h.Spec.Set["deployment.replicas"].IntVal },
			int32(3),
		},
		{
			"spec.timeout pointer",
			func(h *HelmChart) { h.Spec.Timeout.Duration = time.Second },
			func(h *HelmChart) any { return h.Spec.Timeout.Duration },
			5 * time.Minute,
		},
		{
			"spec.backOffLimit pointer",
			func(h *HelmChart) { *h.Spec.BackOffLimit = 1 },
			func(h *HelmChart) any { return *h.Spec.BackOffLimit },
			int32(1000),
		},
		{
			"status.conditions slice",
			func(h *HelmChart) { h.Status.Conditions[0].Reason = "Clobbered" },
			func(h *HelmChart) any { return h.Status.Conditions[0].Reason },
			"Created",
		},
		{
			"metadata.labels map",
			func(h *HelmChart) { h.Labels["app"] = "clobbered" },
			func(h *HelmChart) any { return h.Labels["app"] },
			"ingress",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orig := sampleHelmChart()
			cp := orig.DeepCopy()
			tc.mutate(orig)
			if got := tc.observe(cp); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("mutating the original changed the copy: %s = %v, want %v", tc.name, got, tc.want)
			}
		})
	}

	t.Run("list items are deep", func(t *testing.T) {
		t.Parallel()
		list := &HelmChartList{Items: []HelmChart{*sampleHelmChart()}}
		cp := list.DeepCopy()
		list.Items[0].Spec.Set["deployment.replicas"] = intstr.FromInt32(99)
		if got := cp.Items[0].Spec.Set["deployment.replicas"].IntVal; got != 3 {
			t.Errorf("list item set aliased: %d", got)
		}
		if _, ok := list.DeepCopyObject().(*HelmChartList); !ok {
			t.Fatal("DeepCopyObject did not return *HelmChartList")
		}
	})

	t.Run("HelmChartConfig copies", func(t *testing.T) {
		t.Parallel()
		orig := sampleHelmChartConfig()
		orig.Labels = map[string]string{"a": "b"}
		cp := orig.DeepCopy()
		orig.Labels["a"] = "clobbered"
		orig.Spec.ValuesContent = "clobbered"
		if cp.Labels["a"] != "b" || cp.Spec.ValuesContent == "clobbered" {
			t.Errorf("HelmChartConfig copy aliased: %+v", cp)
		}
		if _, ok := orig.DeepCopyObject().(*HelmChartConfig); !ok {
			t.Fatal("DeepCopyObject did not return *HelmChartConfig")
		}
		l := &HelmChartConfigList{Items: []HelmChartConfig{*cp}}
		if _, ok := l.DeepCopyObject().(*HelmChartConfigList); !ok {
			t.Fatal("DeepCopyObject did not return *HelmChartConfigList")
		}
	})

	t.Run("an empty spec copies without inventing values", func(t *testing.T) {
		t.Parallel()
		// nil Timeout/BackOffLimit mean "controller default"; a copy that
		// materialized zero values would turn that into a 0s timeout and a Job
		// that never retries.
		cp := (&HelmChart{}).DeepCopy()
		if cp.Spec.Set != nil || cp.Spec.Timeout != nil || cp.Spec.BackOffLimit != nil || cp.Status.Conditions != nil {
			t.Errorf("DeepCopy materialized optional fields: %+v", cp)
		}
	})

	t.Run("nil receivers copy to nil", func(t *testing.T) {
		t.Parallel()
		var h *HelmChart
		var hl *HelmChartList
		var c *HelmChartConfig
		var cl *HelmChartConfigList
		if h.DeepCopy() != nil || h.DeepCopyObject() != nil ||
			hl.DeepCopy() != nil || hl.DeepCopyObject() != nil ||
			c.DeepCopy() != nil || c.DeepCopyObject() != nil ||
			cl.DeepCopy() != nil || cl.DeepCopyObject() != nil {
			t.Error("a nil receiver deep-copied to a non-nil value")
		}
	})
}
