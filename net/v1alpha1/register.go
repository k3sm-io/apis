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

package netv1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the API group of the net.k3sm.io CRDs. It is the same group as
// net/v1 (MeshPeer); this package serves its own version within it.
const GroupName = "net.k3sm.io"

// SchemeGroupVersion is the GroupVersion the DirectLink types register under
// (net.k3sm.io/v1alpha1 — the single served + stored version of the DirectLink
// CRD).
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// SchemeBuilder collects the functions that register the net.k3sm.io/v1alpha1
// types into a runtime.Scheme; AddToScheme applies them (the standard client-go
// pattern an informer uses).
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Resource maps a resource name to its GroupResource within net.k3sm.io, for
// building REST paths and status errors.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(SchemeGroupVersion, &DirectLink{}, &DirectLinkList{})
	metav1.AddToGroupVersion(s, SchemeGroupVersion)
	return nil
}
