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

package entry

import "strings"

// Vector is one row of the shared entry-grammar table.
type Vector struct {
	// Input is the candidate entry string.
	Input string
	// Valid is the verdict as an API entry: Parse, the CRD pattern with its
	// maxLength and CEL rules, and the admission policy all give it.
	Valid bool
	// KeyDelivery is the verdict as a KeyDelivery's spec.entry: Valid, and
	// not a system entry (NotSystemRule).
	KeyDelivery bool
	// Why says what the row tests.
	Why string
}

// Vectors returns the shared entry-grammar table. Every consumer of the
// grammar (Parse here, the KeyDelivery CRD schema, the admission policy's
// CEL, the store's name check) tests against it and must give each row's
// verdict. It returns a fresh slice on every call, so a caller's edit reaches
// no other caller.
func Vectors() []Vector {
	name253 := label(253)
	name254 := label(254)
	return []Vector{
		{"system/signer-key", true, false, "a system entry; never a KeyDelivery's"},
		{"user/501/claude-a", true, true, "a user entry"},
		{"user/1/a", true, true, "the smallest uid"},
		{"user/4294967295/a", true, true, "the largest uid"},
		{"user/501/registry.example-token", true, true, "a dotted name"},
		{"user/501/github.stockkitty-fund", true, true, "a <service>.<account> name"},
		{"user/4294967295/" + name253, true, true, "269 bytes, the maxLength"},
		{"user/4294967295/" + name254, false, false, "one over the name bound and the maxLength"},
		{"user/1/" + name254, false, false, "under the maxLength, refused by the name-length rule"},
		{"system/" + name254, false, false, "under the maxLength, refused by the name-length rule"},
		{"system/" + name253, true, false, "the longest system name; never a KeyDelivery's"},
		{"user/501/deliveries", true, true, "an ordinary name"},
		{"user/501/scope.key", true, true, "an ordinary name"},
		{"user/1000/a", true, true, "an all-digit segment is always a uid"},
		{"user/501/a..b", false, false, "an empty label"},
		{"user/501/.a", false, false, "an empty label"},
		{"user/501/a.", false, false, "an empty label"},
		{"user/501/-a", false, false, "a label starting with a hyphen"},
		{"user/501/a-", false, false, "a label ending with a hyphen"},
		{"system/a/b", false, false, "a / inside the name"},
		{"user/0/a", false, false, "uid 0 has no user scope"},
		{"user/4294967296/a", false, false, "past a uint32"},
		{"user/9999999999/a", false, false, "ten digits past a uint32"},
		{"user/12345678901/a", false, false, "more than ten digits"},
		{"user/0501/a", false, false, "a leading zero"},
		{"user/+501/a", false, false, "not decimal digits"},
		{"user/-1/a", false, false, "not decimal digits"},
		{"user/alice/a", false, false, "a short name is CLI input only, resolved before it reaches the API"},
		{"user/501", false, false, "no name"},
		{"user/501/", false, false, "an empty name"},
		{"user/501/a/b", false, false, "a / in the name"},
		{"system/Signer", false, false, "upper case"},
		{"system/", false, false, "an empty name"},
		{"system", false, false, "no name"},
		{"other/a", false, false, "an unknown scope"},
		{"a", false, false, "a bare name: the API takes scope-qualified names only"},
		{"", false, false, "empty"},
		{" user/501/a", false, false, "leading space"},
		{"user/501/a\n", false, false, "a trailing newline"},
		{"user/501/café", false, false, "non-ASCII"},
	}
}

// label returns a valid single-label name of n bytes.
func label(n int) string {
	return strings.Repeat("a", n)
}
