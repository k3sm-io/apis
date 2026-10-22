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

package budgetv1alpha1

// The budget.k3sm.io/* keys the node, the server relay, the runtime and the
// SDKs share. Every side must agree on the exact byte string, so they are
// published here and no consumer spells a literal.
//
// These keys sit under the budget.k3sm.io domain, which is inside k3sm.io:
// the budget contract is meant to be implemented by producers other than
// k3sm, so its keys carry the group's own prefix rather than the k3sm.io/*
// one the rest of this module uses.
//
// Unlike the object shapes in this package, these keys are stable. The alpha
// licence in the package doc does not extend to them: renaming one strands
// every node or pod annotated under it.
const (
	// AnnotationStatus is the Node annotation a node writes on its own Node:
	// a NodeBudgetReport payload (Encode, ParseNodeBudgetReport).
	AnnotationStatus = "budget.k3sm.io/status"
	// AnnotationPolicy is the Node annotation the server relay writes: a
	// NodeBudgetDesiredPolicy payload (Encode, ParseNodeBudgetDesiredPolicy).
	AnnotationPolicy = "budget.k3sm.io/policy"
	// AnnotationBand is the Pod annotation that lowers a pod's band. "batch"
	// is the only meaningful value: it makes the pod tier-1 eligible (unless
	// its priority makes it critical, which the annotation never overrides).
	// Any other value leaves the pod in the band its priority gives.
	AnnotationBand = "budget.k3sm.io/band"
	// AnnotationPage is the Pod annotation that opts a pod into reading the
	// budget page; its value is "true".
	AnnotationPage = "budget.k3sm.io/page"
	// TaintAdmitStop is the node taint key the relay applies while a node's
	// admit is false, always with effect TaintAdmitStopEffect. It is a
	// scheduler hint added beside, never instead of, the platform's own
	// pressure conditions and taints.
	TaintAdmitStop = "budget.k3sm.io/admit-stop"
	// TaintAdmitStopEffect is the only effect TaintAdmitStop carries:
	// PreferNoSchedule, a hint to place new pods elsewhere. It is never
	// NoSchedule and never NoExecute; evicting running pods is the eviction
	// tiers' job, not a taint's.
	TaintAdmitStopEffect = "PreferNoSchedule"
)
