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

// The compiled-in floors and defaults of the tier-1 kill path. They are the
// numeric contract every producer is tested against: no NodeBudgetPolicy can
// lower a floor. The CRD schema refuses a value past a floor on apply, and
// ApplyFloors clamps one that reaches a node any other way (the node then
// reports ConditionPolicyClamped). Changing one is a contract change, so
// each is a literal pinned by a test.
const (
	// MinTier1GraceSeconds is the least SIGTERM grace a tier-1 eviction
	// gives; the producer passes max(this, the pod's
	// terminationGracePeriodSeconds).
	MinTier1GraceSeconds = 10
	// MinTier1HeldSeconds is the least time the held-pressure condition must
	// hold before tier 1 may act.
	MinTier1HeldSeconds = 10
	// DefaultTier1HeldSeconds is the held-pressure duration when a policy
	// sets none.
	DefaultTier1HeldSeconds = 30
	// MaxTier1DeliveriesPerHour is the most tier-1 deliveries per node per
	// hour.
	MaxTier1DeliveriesPerHour = 6
	// MinTier1DeliveriesPerHour is the least hourly cap a policy may set; a
	// policy that wants no tier-1 deliveries sets mode off instead.
	MinTier1DeliveriesPerHour = 1
	// MinTier1CooloffSeconds is the least time between two tier-1 deliveries
	// on one node.
	MinTier1CooloffSeconds = 300
	// DefaultTier1CooloffSeconds is the cool-off when a policy sets none.
	DefaultTier1CooloffSeconds = 1800
	// ArmingEpisodesRequired is the matched pressure episodes mode auto
	// records before it arms tier 1.
	ArmingEpisodesRequired = 3
)

// The defaults of the recommendation's decayed peak history, when a policy
// sets none.
const (
	// DefaultMemoryHalfLifeSeconds is the half-life of the memory peak
	// history (48 h).
	DefaultMemoryHalfLifeSeconds = 172800
	// DefaultCPUHalfLifeSeconds is the half-life of the CPU peak history
	// (12 h).
	DefaultCPUHalfLifeSeconds = 43200
	// DefaultMarginPermille is the margin taken off the recommendation
	// (15%).
	DefaultMarginPermille = 150
)
