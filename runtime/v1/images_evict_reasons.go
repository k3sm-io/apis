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

package runtimev1

// The google.rpc.ErrorInfo strings an Images.EvictImage FAILED_PRECONDITION
// status carries. Only those three cases carry the detail; every other status
// of the call has none. They are plain strings so this package needs no
// errdetails import: the daemon sets them as ErrorInfo.Domain and
// ErrorInfo.Reason, and a client compares them instead of matching on message
// text.
const (
	// EvictErrorInfoDomain is the ErrorInfo.Domain of every EvictImage
	// FAILED_PRECONDITION detail.
	EvictErrorInfoDomain = "runtime.k3sm.io"

	// EvictReasonImageInUse is the ErrorInfo.Reason when the target is in use
	// by a pod. Retryable once the pod is gone.
	EvictReasonImageInUse = "IMAGE_IN_USE"

	// EvictReasonReferenceAmbiguous is the ErrorInfo.Reason when the reference
	// names several platform entries and no platform was set. Retryable with a
	// platform.
	EvictReasonReferenceAmbiguous = "REFERENCE_AMBIGUOUS"

	// EvictReasonRootsIncomplete is the ErrorInfo.Reason when the root set
	// could not be read completely. Retryable after the root set is repaired.
	EvictReasonRootsIncomplete = "ROOTS_INCOMPLETE"
)
