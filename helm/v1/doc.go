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

// Package helmv1 holds the helm.k3sm.io/v1 API: the HelmChart and
// HelmChartConfig custom resources that k3sm's helm controller reconciles into
// a `helm upgrade --install` (or `helm uninstall`) Job.
//
// # The contract is k3s's, mirrored field for field
//
// These types mirror k3s's helm-controller API, helm.cattle.io/v1, field for
// field and JSON tag for JSON tag, INCLUDING the status shape: status.jobName
// plus k3s's own HelmChartCondition (type, status, lastUpdateTime,
// lastTransitionTime, reason, message) with the condition types JobCreated and
// Failed — deliberately not metav1.Condition, and with no observedGeneration.
// A HelmChart written for k3s therefore reads the same here once its apiVersion
// is changed, and nothing about the shape is a k3sm invention waiting to be
// proven. That is why the group ships served+stored v1 on day one instead of an
// alpha runway (compare mlx/v1alpha1, whose shape IS novel).
//
// Only a subset of k3s's spec is carried. Every omitted k3s field is named in
// the HelmChartSpec doc comment as reserved for additive later use, under its
// k3s name and tag, so adding one later never collides with a k3sm-chosen name.
//
// # Stability: served+stored v1, ADDITIVE-ONLY
//
// Existing exported fields, their JSON tags, and the documented constants are
// stable. New optional fields may be appended (the reserved k3s fields first);
// nothing is renamed, retyped, or removed within v1. This is the same contract
// net/v1's MeshPeer carries.
//
// # One deliberate divergence from k3s
//
// The CRD manifest (k3sm.io/apis/config/crd) refuses an http:// spec.repo or
// spec.chart at admission with a CEL rule, and the reconciler refuses it again
// with condition Failed, reason InsecureRepo: the install Job runs as
// cluster-admin and must not fetch a chart over cleartext. k3s accepts it.
//
// Being served objects the types embed metav1.TypeMeta + ObjectMeta and carry
// hand-written DeepCopy*/DeepCopyObject methods (this module runs no code
// generation). The package imports only k8s.io/apimachinery, which is pure Go,
// so it builds CGO_ENABLED=0 and adds nothing to the module's dependency graph.
package helmv1
