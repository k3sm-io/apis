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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DeepCopy* are hand-written — this module runs no deepcopy-gen (the net/v1 and
// mlx/v1alpha1 precedent). Every reference type (maps, slices, pointers, and
// metav1.Time) must be copied, or a client-go informer's cached object and a
// caller's mutation share memory.

// DeepCopyInto copies the receiver into out.
func (in *HelmChartSpec) DeepCopyInto(out *HelmChartSpec) {
	*out = *in
	if in.Set != nil {
		out.Set = make(map[string]intstr.IntOrString, len(in.Set))
		for k, v := range in.Set {
			out.Set[k] = v
		}
	}
	if in.Timeout != nil {
		t := *in.Timeout
		out.Timeout = &t
	}
	if in.BackOffLimit != nil {
		b := *in.BackOffLimit
		out.BackOffLimit = &b
	}
}

// DeepCopy returns a deep copy of the spec.
func (in *HelmChartSpec) DeepCopy() *HelmChartSpec {
	if in == nil {
		return nil
	}
	out := new(HelmChartSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *HelmChartCondition) DeepCopyInto(out *HelmChartCondition) {
	*out = *in
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy returns a deep copy of the condition.
func (in *HelmChartCondition) DeepCopy() *HelmChartCondition {
	if in == nil {
		return nil
	}
	out := new(HelmChartCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *HelmChartStatus) DeepCopyInto(out *HelmChartStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]HelmChartCondition, len(in.Conditions))
		for i := range in.Conditions {
			in.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// DeepCopy returns a deep copy of the status.
func (in *HelmChartStatus) DeepCopy() *HelmChartStatus {
	if in == nil {
		return nil
	}
	out := new(HelmChartStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *HelmChart) DeepCopyInto(out *HelmChart) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the HelmChart.
func (in *HelmChart) DeepCopy() *HelmChart {
	if in == nil {
		return nil
	}
	out := new(HelmChart)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object (satisfies
// runtime.Object so a client-go scheme can serve/watch the type).
func (in *HelmChart) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *HelmChartList) DeepCopyInto(out *HelmChartList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]HelmChart, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *HelmChartList) DeepCopy() *HelmChartList {
	if in == nil {
		return nil
	}
	out := new(HelmChartList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *HelmChartList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out. HelmChartConfigSpec holds only
// value fields, so a shallow copy is already deep.
func (in *HelmChartConfigSpec) DeepCopyInto(out *HelmChartConfigSpec) {
	*out = *in
}

// DeepCopy returns a deep copy of the overlay spec.
func (in *HelmChartConfigSpec) DeepCopy() *HelmChartConfigSpec {
	if in == nil {
		return nil
	}
	out := new(HelmChartConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies the receiver into out.
func (in *HelmChartConfig) DeepCopyInto(out *HelmChartConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy returns a deep copy of the HelmChartConfig.
func (in *HelmChartConfig) DeepCopy() *HelmChartConfig {
	if in == nil {
		return nil
	}
	out := new(HelmChartConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *HelmChartConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto copies the receiver into out.
func (in *HelmChartConfigList) DeepCopyInto(out *HelmChartConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]HelmChartConfig, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the list.
func (in *HelmChartConfigList) DeepCopy() *HelmChartConfigList {
	if in == nil {
		return nil
	}
	out := new(HelmChartConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject returns a deep copy as a runtime.Object.
func (in *HelmChartConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
