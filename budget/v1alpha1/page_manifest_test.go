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

import "strconv"

// PageManifest is the contract the SDKs vendor beside the page fixtures
// (testdata/page/manifest.json): the constants, the reader contract, the
// code tables, the fixture digests, the negative page set with the error
// each case must produce, and the notifyEpoch transition vectors. It is
// test-only Go; the JSON file is the contract.
type PageManifest struct {
	// PageABIMajor is PageABIMajor.
	PageABIMajor int `json:"pageABIMajor"`
	// PageABIMinor is PageABIMinor.
	PageABIMinor int `json:"pageABIMinor"`
	// PageFileName is PageFileName.
	PageFileName string `json:"pageFileName"`
	// DefaultPagePath is DefaultPagePath.
	DefaultPagePath string `json:"defaultPagePath"`
	// LinuxPagePath is LinuxPagePath.
	LinuxPagePath string `json:"linuxPagePath"`
	// NodeBudgetAPIVersion is NodeBudgetAPIVersion.
	NodeBudgetAPIVersion string `json:"nodeBudgetAPIVersion"`
	// PageSize is PageSize.
	PageSize int `json:"pageSize"`
	// PageBodyLimit is PageBodyLimit.
	PageBodyLimit int `json:"pageBodyLimit"`
	// HeaderSize is PageHeaderSize.
	HeaderSize int `json:"headerSize"`
	// BodySize is the bodySize this minor writes.
	BodySize int `json:"bodySize"`
	// Magic is PageMagic.
	Magic string `json:"magic"`
	// Absent is PageAbsent.
	Absent int64 `json:"absent"`
	// SeqOffset is PageSeqOffset.
	SeqOffset int `json:"seqOffset"`
	// NotifyEpochOffset is PageNotifyEpochOffset.
	NotifyEpochOffset int `json:"notifyEpochOffset"`
	// NotifyWaitOffset is the address readers wait on: the low 32-bit
	// word of notifyEpoch.
	NotifyWaitOffset int `json:"notifyWaitOffset"`
	// NotifyWaitBytes is the width of the wait word.
	NotifyWaitBytes int `json:"notifyWaitBytes"`
	// MaxNodeNameBytes is MaxPageNodeNameBytes.
	MaxNodeNameBytes int `json:"maxNodeNameBytes"`
	// StaleFactor is the reader's staleness bound: a page is stale when
	// now - observedAt > StaleFactor * writerPeriodSeconds.
	StaleFactor int `json:"staleFactor"`
	// ObservedAtResolution is observedAt's resolution: whole seconds, so a
	// staleness verdict may be up to one second early or late.
	ObservedAtResolution string `json:"observedAtResolution"`
	// ReaderRetries is how many seqlock read attempts a reader makes before
	// it returns torn.
	ReaderRetries int `json:"readerRetries"`
	// Layout is the descriptor file of this minor (PageLayout).
	Layout string `json:"layout"`
	// Codes are PageCodeTables: the value at index i has code i+1, and 0
	// is unset or unknown.
	Codes map[string][]string `json:"codes"`
	// Fixtures are the decodable and raw page fixtures.
	Fixtures []PageFixture `json:"fixtures"`
	// Negative are the pages a reader must refuse.
	Negative []PageNegativeCase `json:"negative"`
	// NotifyVectors are PageNotifyDue's transition vectors.
	NotifyVectors []PageNotifyVector `json:"notifyVectors"`
}

// PageFixture is one page fixture file.
type PageFixture struct {
	// File is the path under testdata/page.
	File string `json:"file"`
	// SHA256 is the file's hex digest.
	SHA256 string `json:"sha256"`
	// Decodes is false for a raw fixture (pattern.v1.page), whose words
	// encode their own offsets rather than valid values.
	Decodes bool `json:"decodes"`
	// Description says what the fixture holds.
	Description string `json:"description"`
}

// PageNegativeCase is one page a reader must refuse, and how.
type PageNegativeCase struct {
	// Name names the case.
	Name string `json:"name"`
	// File is the path under testdata/page.
	File string `json:"file"`
	// SHA256 is the file's hex digest.
	SHA256 string `json:"sha256"`
	// Error is the reader error, one of the PageErr names.
	Error string `json:"error"`
	// EncoderError, when set, is the error an encoder must return for the
	// snapshot the page was forged from.
	EncoderError string `json:"encoderError,omitempty"`
}

// PageNotifyVector is one PageNotifyDue case: Base with the word of Field
// set to Value is the next page, Base the previous one.
type PageNotifyVector struct {
	// Base is the fixture both pages start from.
	Base string `json:"base"`
	// Field is a layout field name.
	Field string `json:"field"`
	// Value is the field's new value (a code, a bool as 0 or 1, or an
	// i64).
	Value int64 `json:"value"`
	// Due is PageNotifyDue(base, next).
	Due bool `json:"due"`
}

// The reader contract the manifest publishes for every SDK reader.
const (
	// pageStaleFactor: a page is stale when now - observedAt exceeds this
	// many writer periods.
	pageStaleFactor = 3
	// pageReaderRetries is the seqlock read attempts an SDK reader makes
	// before it returns torn.
	pageReaderRetries = 64
)

// pageLayoutFile is the descriptor file name of a minor of this major.
func pageLayoutFile(minor int) string {
	return "layout.v" + strconv.Itoa(PageABIMajor) + "." + strconv.Itoa(minor) + ".txt"
}

// NewPageManifest returns the manifest's constants, code tables and
// negative cases; the digests and the vectors are filled from the
// committed fixtures.
func NewPageManifest() PageManifest {
	return PageManifest{
		PageABIMajor:         PageABIMajor,
		PageABIMinor:         PageABIMinor,
		PageFileName:         PageFileName,
		DefaultPagePath:      DefaultPagePath,
		LinuxPagePath:        LinuxPagePath,
		NodeBudgetAPIVersion: NodeBudgetAPIVersion,
		PageSize:             PageSize,
		PageBodyLimit:        PageBodyLimit,
		HeaderSize:           PageHeaderSize,
		BodySize:             pageBodySize(),
		Magic:                PageMagic,
		Absent:               PageAbsent,
		SeqOffset:            PageSeqOffset,
		NotifyEpochOffset:    PageNotifyEpochOffset,
		NotifyWaitOffset:     PageNotifyEpochOffset,
		NotifyWaitBytes:      4,
		MaxNodeNameBytes:     MaxPageNodeNameBytes,
		StaleFactor:          pageStaleFactor,
		ObservedAtResolution: "1s",
		ReaderRetries:        pageReaderRetries,
		Layout:               pageLayoutFile(PageABIMinor),
		Codes:                PageCodeTables(),
		Negative: []PageNegativeCase{
			{Name: "short", File: "negative/short.v1.page", Error: PageErrTruncated},
			{Name: "bad-magic", File: "negative/bad-magic.v1.page", Error: PageErrABIMismatch},
			{Name: "unknown-major", File: "negative/unknown-major.v1.page", Error: PageErrABIMismatch},
			{Name: "body-size-below-known", File: "negative/body-size-below-known.v1.page", Error: PageErrTruncated},
			{Name: "odd-seq", File: "negative/odd-seq.v1.page", Error: PageErrTorn},
			{
				Name: "node-name-too-long", File: "negative/node-name-too-long.v1.page",
				Error: PageErrCorrupt, EncoderError: PageEncoderErrNodeNameTooLong,
			},
		},
	}
}
