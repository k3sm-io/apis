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

// The page reader errors, by their language-neutral contract names. Every
// SDK maps each to its own error type and checks its mapping against the
// negative cases of the manifest (testdata/page/manifest.json).
const (
	// PageErrNotFound: no page, or no access to it.
	PageErrNotFound = "notFound"
	// PageErrABIMismatch: bad magic, an unknown ABI major, a header size
	// other than PageHeaderSize, or an unknown code in a structural table
	// (signal, unit, level, band, platform, kill mode). An unknown code in a
	// producer-vocabulary table reads as that type's "unknown" value instead.
	PageErrABIMismatch = "abiMismatch"
	// PageErrTruncated: a file shorter than headerSize + bodySize, or a
	// bodySize below the size the reader's own minor knows.
	PageErrTruncated = "truncated"
	// PageErrCorrupt: a well-formed header, but a field outside its own
	// domain (a nodeNameLen above MaxPageNodeNameBytes).
	PageErrCorrupt = "corrupt"
	// PageErrTorn: no stable read (an even seq equal on both sides) after
	// the reader's retries; the manifest's readerRetries is the bound.
	PageErrTorn = "torn"
	// PageErrStale: now - observedAt above the manifest's staleFactor
	// writer periods.
	PageErrStale = "stale"
	// PageEncoderErrNodeNameTooLong names ErrPageNodeNameTooLong in the
	// manifest, for SDK writers.
	PageEncoderErrNodeNameTooLong = "nodeNameTooLong"
)
