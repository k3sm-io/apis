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

// Package crd embeds the k3sm CustomResourceDefinition manifests so a consumer
// applies the SAME bytes this module ships, with no shadow copy.
//
// A CRD manifest that is maintained here and separately pasted into the applying
// repo is two schemas: they agree until one is edited, and the disagreement
// surfaces as an object that validates in one place and is rejected in another.
// Embedding removes the second copy entirely — k3sm's server-side-apply ensure
// reads these bytes.
//
// # Named accessors, deliberately not a glob
//
// Each manifest is embedded by NAME and exposed by its own accessor. There is no
// embed.FS and no *.yaml pattern, because a glob makes "which CRDs does k3sm
// apply" a property of what happens to be in this directory: dropping a file in
// would silently enlist it, and adopting a CRD into the applied set must be a
// deliberate, reviewable act.
//
// The MeshPeer manifest beside this file (net.k3sm.io_meshpeers.yaml) has such
// an accessor: k3sm's server applies it fail-closed on the mesh join path, so
// every worker join depends on this CRD actually being installed.
//
// The Interconnect manifest (net.k3sm.io_interconnects.yaml) is the alpha
// net.k3sm.io/v1alpha1 record of the cable as an opt-in network; k3sm's server
// ensures it through its accessor.
//
// The two helm.k3sm.io manifests (HelmChart and HelmChartConfig) are the k3s
// helm.cattle.io/v1 contract mirrored; k3sm's helm controller applies both.
//
// The Update manifest (updates.k3sm.io_updates.yaml) is the alpha
// updates.k3sm.io/v1alpha1 request to move the whole cluster to one release;
// k3sm's update controller ensures it through its accessor.
//
// The two budget.k3sm.io manifests (NodeBudget and NodeBudgetPolicy) are the
// alpha budget.k3sm.io/v1alpha1 node budget contract: the per-node published
// budget and the one policy object. Neither carries labels.
package crd

import (
	_ "embed"
)

// MLXModelCRDName is the metadata.name of the MLXModel CustomResourceDefinition
// (the <plural>.<group> form the API server addresses it by). Published so a
// consumer waiting on or reading back the CRD does not spell the string.
const MLXModelCRDName = "mlxmodels.mlx.k3sm.io"

//go:embed mlx.k3sm.io_mlxmodels.yaml
var mlxModelCRDYAML string

// MLXModelCRD returns the mlx.k3sm.io MLXModel CustomResourceDefinition manifest
// as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call. The embedded manifest is process-global
// state, and a caller that decodes in place, appends, or otherwise scribbles on
// the returned slice would corrupt what every later caller applies — including
// the one that re-applies during a reconcile loop.
func MLXModelCRD() []byte {
	return []byte(mlxModelCRDYAML)
}

// MeshPeerCRDName is the metadata.name of the MeshPeer CustomResourceDefinition
// (the <plural>.<group> form the API server addresses it by). Published so a
// consumer waiting on or reading back the CRD does not spell the string.
const MeshPeerCRDName = "meshpeers.net.k3sm.io"

//go:embed net.k3sm.io_meshpeers.yaml
var meshPeerCRDYAML string

// MeshPeerCRD returns the net.k3sm.io MeshPeer CustomResourceDefinition manifest
// as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call. The embedded manifest is process-global
// state, and a caller that decodes in place, appends, or otherwise scribbles on
// the returned slice would corrupt what every later caller applies — including
// the one that re-applies during a reconcile loop.
func MeshPeerCRD() []byte {
	return []byte(meshPeerCRDYAML)
}

// InterconnectCRDName is the metadata.name of the Interconnect
// CustomResourceDefinition (the <plural>.<group> form the API server addresses
// it by). Published so a consumer waiting on or reading back the CRD does not
// spell the string.
const InterconnectCRDName = "interconnects.net.k3sm.io"

//go:embed net.k3sm.io_interconnects.yaml
var interconnectCRDYAML string

// InterconnectCRD returns the net.k3sm.io/v1alpha1 Interconnect
// CustomResourceDefinition manifest as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call, for the same reason as MLXModelCRD:
// the embedded manifest is process-global, and a caller's in-place edit would
// corrupt what every later apply sends.
func InterconnectCRD() []byte {
	return []byte(interconnectCRDYAML)
}

// HelmChartCRDName is the metadata.name of the HelmChart
// CustomResourceDefinition (the <plural>.<group> form the API server addresses
// it by). Published so a consumer waiting on or reading back the CRD does not
// spell the string.
const HelmChartCRDName = "helmcharts.helm.k3sm.io"

//go:embed helm.k3sm.io_helmcharts.yaml
var helmChartCRDYAML string

// HelmChartCRD returns the helm.k3sm.io HelmChart CustomResourceDefinition
// manifest as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call, for the same reason as MLXModelCRD:
// the embedded manifest is process-global, and a caller's in-place edit would
// corrupt what every later apply sends.
func HelmChartCRD() []byte {
	return []byte(helmChartCRDYAML)
}

// HelmChartConfigCRDName is the metadata.name of the HelmChartConfig
// CustomResourceDefinition (the <plural>.<group> form the API server addresses
// it by). Published so a consumer waiting on or reading back the CRD does not
// spell the string.
const HelmChartConfigCRDName = "helmchartconfigs.helm.k3sm.io"

//go:embed helm.k3sm.io_helmchartconfigs.yaml
var helmChartConfigCRDYAML string

// HelmChartConfigCRD returns the helm.k3sm.io HelmChartConfig
// CustomResourceDefinition manifest as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call, for the same reason as MLXModelCRD.
func HelmChartConfigCRD() []byte {
	return []byte(helmChartConfigCRDYAML)
}

// UpdateCRDName is the metadata.name of the Update CustomResourceDefinition
// (the <plural>.<group> form the API server addresses it by). Published so a
// consumer waiting on or reading back the CRD does not spell the string.
const UpdateCRDName = "updates.updates.k3sm.io"

//go:embed updates.k3sm.io_updates.yaml
var updateCRDYAML string

// UpdateCRD returns the updates.k3sm.io/v1alpha1 Update
// CustomResourceDefinition manifest as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call, for the same reason as MLXModelCRD.
func UpdateCRD() []byte {
	return []byte(updateCRDYAML)
}

// NodeBudgetCRDName is the metadata.name of the NodeBudget
// CustomResourceDefinition (the <plural>.<group> form the API server
// addresses it by). Published so a consumer waiting on or reading back the
// CRD does not spell the string.
const NodeBudgetCRDName = "nodebudgets.budget.k3sm.io"

//go:embed budget.k3sm.io_nodebudgets.yaml
var nodeBudgetCRDYAML string

// NodeBudgetCRD returns the budget.k3sm.io/v1alpha1 NodeBudget
// CustomResourceDefinition manifest as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call, for the same reason as MLXModelCRD.
func NodeBudgetCRD() []byte {
	return []byte(nodeBudgetCRDYAML)
}

// NodeBudgetPolicyCRDName is the metadata.name of the NodeBudgetPolicy
// CustomResourceDefinition (the <plural>.<group> form the API server
// addresses it by). Published so a consumer waiting on or reading back the
// CRD does not spell the string.
const NodeBudgetPolicyCRDName = "nodebudgetpolicies.budget.k3sm.io"

//go:embed budget.k3sm.io_nodebudgetpolicies.yaml
var nodeBudgetPolicyCRDYAML string

// NodeBudgetPolicyCRD returns the budget.k3sm.io/v1alpha1 NodeBudgetPolicy
// CustomResourceDefinition manifest as YAML bytes, ready to apply.
//
// It returns a fresh copy on every call, for the same reason as MLXModelCRD.
func NodeBudgetPolicyCRD() []byte {
	return []byte(nodeBudgetPolicyCRDYAML)
}
