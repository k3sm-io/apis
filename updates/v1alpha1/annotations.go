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

package updatesv1alpha1

// The k3sm.io/* Node annotation keys the update controller and the root
// helper that installs a release on each node share. Both sides must agree
// on the exact byte string, so they are published here and no consumer
// spells a literal.
//
// Note the project convention: CRD groups are <area>.k3sm.io (GroupName is
// updates.k3sm.io), while labels and annotations are k3sm.io/*. The two
// namespaces are not interchangeable; these keys are deliberately not under
// updates.k3sm.io.
//
// Unlike the object shape in this package, these keys are stable. The alpha
// licence in the package doc does not extend to them: renaming one strands
// every node annotated mid-roll, which no version bump can migrate.
const (
	// AnnotationUpdateVersion is the Node annotation carrying the release tag
	// the controller asks that node to install.
	AnnotationUpdateVersion = "k3sm.io/update-version"

	// AnnotationUpdatePayload is the Node annotation identifying the release
	// payload the node is to install.
	AnnotationUpdatePayload = "k3sm.io/update-payload"

	// AnnotationUpdatePhase is the Node annotation carrying the node's current
	// step of the update, an UpdateNodePhase value.
	AnnotationUpdatePhase = "k3sm.io/update-phase"
)
