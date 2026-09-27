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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// GroupName is the API group of the helm.k3sm.io CRDs (HelmChart and
// HelmChartConfig) — the k3sm analog of k3s's helm.cattle.io.
const GroupName = "helm.k3sm.io"

// SchemeGroupVersion is the GroupVersion the helm types register under
// (helm.k3sm.io/v1 — the single served + stored version).
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

// SchemeBuilder collects the functions that register the helm.k3sm.io/v1 types
// into a runtime.Scheme; AddToScheme applies them (the standard client-go
// pattern the k3sm helm controller's informers use).
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Resource maps a resource name to its GroupResource within helm.k3sm.io, for
// building REST paths and status errors.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(SchemeGroupVersion,
		&HelmChart{}, &HelmChartList{},
		&HelmChartConfig{}, &HelmChartConfigList{},
	)
	metav1.AddToGroupVersion(s, SchemeGroupVersion)
	return nil
}

// FailurePolicyAbort and FailurePolicyReinstall are the two values
// HelmChartSpec.FailurePolicy and HelmChartConfigSpec.FailurePolicy accept (the
// CRD enforces the enum). Empty means the controller default, which is
// reinstall, as in k3s.
const (
	// FailurePolicyAbort leaves a failed release in place for the operator to
	// inspect and repair; the controller does not retry the install.
	FailurePolicyAbort = "abort"
	// FailurePolicyReinstall uninstalls a failed release and installs it again.
	FailurePolicyReinstall = "reinstall"
)

// HelmChart is one Helm release k3sm keeps installed: a chart (from a repo, an
// OCI reference, or inline as chartContent), the values it is installed with,
// and the namespace it lands in. The controller reconciles it into a Job that
// runs helm; deleting it uninstalls the release.
// Mirrors k3s's helm.cattle.io/v1 HelmChart (see the package doc).
type HelmChart struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired release.
	Spec HelmChartSpec `json:"spec,omitempty"`
	// Status is the observed state, written by the controller through the
	// status subresource.
	Status HelmChartStatus `json:"status,omitempty"`
}

// HelmChartSpec is the desired state of a HelmChart. Field names and JSON tags
// are k3s's helm.cattle.io/v1 names, verbatim.
//
// # Reserved k3s fields
//
// The following k3s HelmChartSpec fields are NOT carried in v1. Each name (and
// its k3s JSON tag) is reserved for additive later use with k3s's meaning; a
// manifest that sets one is pruned by the structural schema today.
//
//   - jobImage: the Job runs the pinned helm host binary k3sm ships, not a
//     klipper-helm container image, so there is no image to choose.
//   - helmVersion: only Helm 3 exists; k3s itself ignores the v2 value now.
//   - driver: the Helm storage driver stays helm's default (secret); no
//     consumer needs another yet.
//   - podSecurityContext, securityContext: the Job is a native Darwin pod whose
//     posture comes from the runtime sandbox, not a Linux security context.
//   - values: k3s's structured values map duplicates valuesContent; v1 keeps
//     the single string form.
//   - valuesSecrets: reads values from Secrets, an auth-adjacent surface
//     deferred with the fields below.
//   - authSecret, authPassCredentials: repository credentials are a trust
//     decision v1 does not take; public and inline charts only.
//   - dockerRegistrySecret: OCI registry credentials, deferred for the same
//     reason.
//   - repoCA, repoCAConfigMap: a custom CA for the repo is an external trust
//     root; v1 trusts the system roots only.
//   - insecureSkipTLSVerify, plainHTTP: both weaken transport security for a
//     Job running as cluster-admin, which v1 refuses outright.
//
// When both Chart and ChartContent are set, ChartContent wins (k3s semantics).
type HelmChartSpec struct {
	// Chart is the chart to install: a name in Repo, an oci:// reference, or a
	// chart URL. Ignored when ChartContent is set. An http:// value is refused.
	Chart string `json:"chart,omitempty"`
	// Repo is the chart repository URL Chart is resolved against. An http://
	// value is refused (the Job runs as cluster-admin).
	Repo string `json:"repo,omitempty"`
	// Version is the chart version; empty means the repository's latest.
	Version string `json:"version,omitempty"`
	// TargetNamespace is the namespace the release is installed into; empty
	// means the HelmChart's own namespace.
	TargetNamespace string `json:"targetNamespace,omitempty"`
	// CreateNamespace creates TargetNamespace if it does not exist.
	CreateNamespace bool `json:"createNamespace,omitempty"`
	// ValuesContent is a YAML values document passed to helm as a values file.
	ValuesContent string `json:"valuesContent,omitempty"`
	// Set are individual --set overrides, applied after ValuesContent. A value
	// is either a string or an integer, exactly as in k3s.
	Set map[string]intstr.IntOrString `json:"set,omitempty"`
	// ChartContent is a base64-encoded chart archive (.tgz) installed instead of
	// fetching Chart. It takes precedence over Chart when both are set.
	ChartContent string `json:"chartContent,omitempty"`
	// Timeout bounds each helm operation (helm's --timeout); nil means the
	// controller default.
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	// Bootstrap marks a chart needed to bring the cluster up; its Job may run
	// before the node is Ready.
	Bootstrap bool `json:"bootstrap,omitempty"`
	// BackOffLimit is the install Job's backoffLimit; nil means the controller
	// default (k3s's 1000).
	BackOffLimit *int32 `json:"backOffLimit,omitempty"`
	// FailurePolicy is what happens when an install fails: FailurePolicyAbort
	// or FailurePolicyReinstall. Empty means reinstall.
	FailurePolicy string `json:"failurePolicy,omitempty"`
	// ForceConflicts passes helm's --force-conflicts, taking over server-side
	// apply field managers that conflict with the release.
	ForceConflicts bool `json:"forceConflicts,omitempty"`
	// TakeOwnership passes helm's --take-ownership, adopting existing resources
	// that are not yet owned by any release.
	TakeOwnership bool `json:"takeOwnership,omitempty"`
}

// HelmChartConditionType names a HelmChart condition.
type HelmChartConditionType string

const (
	// HelmChartJobCreated reports that the install/upgrade Job named in
	// status.jobName exists.
	HelmChartJobCreated HelmChartConditionType = "JobCreated"
	// HelmChartFailed reports that the last install, upgrade, or uninstall
	// failed (or was refused, e.g. reason InsecureRepo).
	HelmChartFailed HelmChartConditionType = "Failed"
)

// HelmChartCondition is k3s's own HelmChart condition shape, carried verbatim
// rather than metav1.Condition so the status reads the same as on k3s.
//
// Status uses apimachinery's metav1.ConditionStatus, which has the identical
// wire values ("True", "False", "Unknown") as k3s's corev1.ConditionStatus;
// using it keeps this module free of a k8s.io/api dependency.
type HelmChartCondition struct {
	// Type is the condition type.
	Type HelmChartConditionType `json:"type"`
	// Status is True, False, or Unknown.
	Status metav1.ConditionStatus `json:"status"`
	// LastUpdateTime is the last time the condition was written.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// LastTransitionTime is the last time Status changed.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a CamelCase machine-readable cause.
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable explanation.
	Message string `json:"message,omitempty"`
}

// HelmChartStatus is the observed state of a HelmChart.
type HelmChartStatus struct {
	// JobName is the name of the Job the controller created for the current
	// install or upgrade.
	JobName string `json:"jobName,omitempty"`
	// Conditions are the HelmChart conditions (HelmChartJobCreated,
	// HelmChartFailed).
	Conditions []HelmChartCondition `json:"conditions,omitempty"`
}

// HelmChartList is a list of HelmChart objects (the watch/list response type).
type HelmChartList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the HelmChart objects.
	Items []HelmChart `json:"items"`
}

// HelmChartConfig overlays configuration onto the HelmChart of the same name
// in the same namespace — typically a packaged chart whose HelmChart the
// operator does not own. Mirrors k3s's helm.cattle.io/v1 HelmChartConfig.
type HelmChartConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the overlay.
	Spec HelmChartConfigSpec `json:"spec,omitempty"`
}

// HelmChartConfigSpec is the overlay a HelmChartConfig applies. k3s's
// valuesSecrets is reserved, as on HelmChartSpec.
type HelmChartConfigSpec struct {
	// ValuesContent is a YAML values document applied after the HelmChart's
	// own ValuesContent.
	ValuesContent string `json:"valuesContent,omitempty"`
	// FailurePolicy overrides the HelmChart's FailurePolicy when set.
	FailurePolicy string `json:"failurePolicy,omitempty"`
	// ForceConflicts overrides the HelmChart's ForceConflicts when true.
	ForceConflicts bool `json:"forceConflicts,omitempty"`
}

// HelmChartConfigList is a list of HelmChartConfig objects.
type HelmChartConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the HelmChartConfig objects.
	Items []HelmChartConfig `json:"items"`
}
