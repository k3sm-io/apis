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

// The budget page carries every enum as a u64 code. Each table below is
// append-only: a value's code is its index plus one, a code is never reused
// or renumbered, and code 0 means unset or unknown (the page does not tell
// the two apart). Adding a value to a structural table (signal, unit, level,
// band, platform, kill mode) bumps the page ABI minor at the least, because
// an older reader cannot name it; adding a value to a producer-vocabulary
// table (trend, limiting resource, kill-mode reason) does not, because an
// older reader reads an unknown code as that type's "unknown" value. The
// signal table is signalCodes in vocab.go, shared with SignalID.PageCode.
// TestPageCodeTables asserts each table against the vocabulary.

var (
	pageUnitCodes             = [...]SignalUnit{UnitBytes, UnitPermille, UnitCount}
	pageLevelCodes            = [...]Level{LevelOK, LevelWarn, LevelCritical, LevelEvicting}
	pageBandCodes             = [...]Band{BandCritical, BandStandard, BandBatch}
	pagePlatformCodes         = [...]Platform{PlatformDarwin, PlatformLinux}
	pageKillModeCodes         = [...]KillModeName{KillModeAuto, KillModeOn, KillModeOff}
	pageTrendCodes            = [...]Trend{TrendFalling, TrendSteady, TrendRising}
	pageLimitingResourceCodes = [...]LimitingResource{
		ResourceMemory, ResourceSwapfs, ResourceNodefs, ResourceImagefs, ResourcePID, ResourceCPU,
	}
	pageKillModeReasonCodes = [...]KillModeReason{
		KillReasonColdStart, KillReasonObserving, KillReasonArmed, KillReasonPolicyOff, KillReasonDisarmFile,
	}
)

// pageCode returns v's code in table: 0 for the empty value, false for a
// value the table does not hold. unknown, when not empty, is the type's
// "unknown" value, which encodes as 0 like the empty value.
func pageCode[T ~string](table []T, v, unknown T) (uint64, bool) {
	if v == "" || (unknown != "" && v == unknown) {
		return 0, true
	}
	for i, w := range table {
		if w == v {
			return uint64(i + 1), true
		}
	}
	return 0, false
}

// PageCodeTables returns every page enum table by its manifest name, each
// as the ordered list of values: the value at index i has code i+1.
func PageCodeTables() map[string][]string {
	return map[string][]string{
		"signal":           stringsOf(signalCodes[:]),
		"unit":             stringsOf(pageUnitCodes[:]),
		"level":            stringsOf(pageLevelCodes[:]),
		"band":             stringsOf(pageBandCodes[:]),
		"platform":         stringsOf(pagePlatformCodes[:]),
		"killMode":         stringsOf(pageKillModeCodes[:]),
		"trend":            stringsOf(pageTrendCodes[:]),
		"limitingResource": stringsOf(pageLimitingResourceCodes[:]),
		"killModeReason":   stringsOf(pageKillModeReasonCodes[:]),
	}
}

func stringsOf[T ~string](vs []T) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}
