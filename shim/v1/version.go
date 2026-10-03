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

package shimv1

// APIVersion is the shim/v1 contract version, reported by a shim in
// StatusResponse.api_version. It is legible, never negotiated: the daemon and
// the shim come from the same build, so the daemon refuses a shim reporting any
// other value with a stated reason rather than proceeding. There is no
// capability negotiation; a contract change is a lockstep change to both ends.
const APIVersion = "k3sm.shim.v1"
