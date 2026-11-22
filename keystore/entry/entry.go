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

// Package entry is the one home of the keystore entry grammar: the
// scope-qualified names system/<name> and user/<uid>/<name>.
//
// The grammar lives here once. The KeyDelivery CRD's spec.entry pattern, the
// admission policy's CEL and the Go resolvers in other repos all take it from
// this package, and one shared vector table (Vectors) tests all of them, so
// the three cannot drift apart.
//
// A qualified name is valid as an API entry when it matches Pattern, is at
// most MaxLength bytes, its uid (for a user entry) is at most MaxUID, and its
// name part is at most MaxNameLength bytes. A uid is decimal with no leading
// zero, from 1 to MaxUID; uid 0 has no user scope. The short-name form
// user/<shortname>/<name> is CLI input only: the CLI resolves it to the uid
// before the grammar sees the name, so this package refuses it.
//
// The package imports only the standard library, so a consumer that must stay
// free of Kubernetes packages can depend on it.
package entry

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Pattern is the entry grammar as an anchored regular expression, written in
// the subset that RE2, the OpenAPI pattern of a CRD and CEL's matches() share.
// It bounds the uid's digit count; the numeric range is UIDRangeRule, and the
// name part's length is NameLengthRule.
const Pattern = `^(system|user/[1-9][0-9]{0,9})/[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`

// MaxLength is the longest valid entry in bytes: "user/", a 10-digit uid,
// "/" and a MaxNameLength name (5 + 10 + 1 + 253). It is the CRD's maxLength
// and also bounds the cost of the CEL rules.
const MaxLength = 269

// MaxNameLength is the longest name part in bytes, the store's entry-name
// bound. MaxLength alone does not enforce it: system/ or user/1/ followed by a
// 254-byte name is still under MaxLength.
const MaxNameLength = 253

// MaxUID is the largest uid a user scope may name (a uint32).
const MaxUID = 4294967295

// UIDRangeRule is the CEL rule, on the entry string, that holds a user
// entry's uid to at most MaxUID. Pattern guarantees at most 10 digits, so the
// int() conversion cannot overflow.
const UIDRangeRule = `!self.startsWith('user/') || int(self.split('/')[1]) <= 4294967295`

// NameLengthRule is the CEL rule, on the entry string, that holds the name
// part to at most MaxNameLength bytes. Pattern allows no '/' in the name, so
// the last segment is the name, and only ASCII, so CEL's size() in code points
// equals the byte count.
const NameLengthRule = `size(self.split('/')[size(self.split('/')) - 1]) <= 253`

// NotSystemRule is the CEL rule, on a KeyDelivery's entry string, that
// refuses a system entry: a system entry reaches a namespace only through
// root's deliver, never through a KeyDelivery.
const NotSystemRule = `!self.startsWith('system/')`

// RequestedByPattern is the grammar of a KeyDelivery's spec.requestedBy: the
// local principal k3sm:local:<uid>.
const RequestedByPattern = `^k3sm:local:[1-9][0-9]{0,9}$`

// RequestedByMatchesEntryRule is the CEL rule, on a KeyDelivery's spec, that
// ties the requester to the entry's owner: for a user entry, requestedBy is
// k3sm:local: followed by the entry's uid.
const RequestedByMatchesEntryRule = `!self.entry.startsWith('user/') || self.requestedBy == 'k3sm:local:' + self.entry.split('/')[1]`

// ErrInvalid is returned, wrapped, by Parse for any input that is not a valid
// API entry. Match it with errors.Is.
var ErrInvalid = errors.New("invalid keystore entry")

// Scope is the scope part of an entry.
type Scope string

const (
	// ScopeSystem is the root-owned system scope, system/<name>.
	ScopeSystem Scope = "system"
	// ScopeUser is a per-uid user scope, user/<uid>/<name>.
	ScopeUser Scope = "user"
)

// Entry is a parsed, valid scope-qualified entry name.
type Entry struct {
	// Scope is ScopeSystem or ScopeUser.
	Scope Scope
	// UID is the user scope's uid, from 1 to MaxUID; 0 for ScopeSystem.
	UID uint32
	// Name is the name part, at most MaxNameLength bytes.
	Name string
}

// String returns the qualified form, system/<name> or user/<uid>/<name>.
func (e Entry) String() string {
	if e.Scope == ScopeUser {
		return "user/" + strconv.FormatUint(uint64(e.UID), 10) + "/" + e.Name
	}
	return "system/" + e.Name
}

var patternRE = regexp.MustCompile(Pattern)

// Parse validates s as an API entry and returns its parts. Any failure wraps
// ErrInvalid; the error never repeats the input, since an entry name can be
// private to its owner.
func Parse(s string) (Entry, error) {
	if len(s) > MaxLength {
		return Entry{}, fmt.Errorf("%w: longer than %d bytes", ErrInvalid, MaxLength)
	}
	if !patternRE.MatchString(s) {
		return Entry{}, fmt.Errorf("%w: does not match the entry grammar", ErrInvalid)
	}
	if rest, ok := strings.CutPrefix(s, "system/"); ok {
		if len(rest) > MaxNameLength {
			return Entry{}, fmt.Errorf("%w: name longer than %d bytes", ErrInvalid, MaxNameLength)
		}
		return Entry{Scope: ScopeSystem, Name: rest}, nil
	}
	rest := strings.TrimPrefix(s, "user/")
	uidPart, name, _ := strings.Cut(rest, "/")
	uid, err := strconv.ParseUint(uidPart, 10, 32)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: uid out of range", ErrInvalid)
	}
	if len(name) > MaxNameLength {
		return Entry{}, fmt.Errorf("%w: name longer than %d bytes", ErrInvalid, MaxNameLength)
	}
	return Entry{Scope: ScopeUser, UID: uint32(uid), Name: name}, nil
}
