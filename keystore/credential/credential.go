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

// Package credential holds the public names of typed keystore credentials:
// every name a consumer matches on, in one place.
//
// It owns the credential kinds, the projection field table (which fields of
// each kind may be delivered into a Secret), the {field, key} item a
// KeyDelivery and the root-file target both carry, the refresh-owner grammar,
// the CredentialValid condition reasons, ProjectionInvalid, the Event reasons
// and types, the refresh_state values, and the label and annotation keys on a
// delivered Secret. Rendering a credential into Secret bytes is not here; it
// stays in k3sm, and a conformance test there checks the renderer supports
// exactly this table.
//
// # Access only
//
// No kind has a projectable refresh field. A refresh token is write-only
// after it is stored, so no item can name one: the KeyDelivery CRD's field
// enum is the union of this table (Fields), and RefreshTokenField is not in
// it.
//
// The package imports only the standard library.
package credential

import (
	"errors"
	"regexp"
	"strings"
)

// Kind is a credential kind, as the CLI's --kind names it.
type Kind string

// The credential kinds.
const (
	// KindEd25519 is an ed25519 signing key (Phase A).
	KindEd25519 Kind = "ed25519"
	// KindOpaque is raw bytes that never expire (Phase A).
	KindOpaque Kind = "opaque"
	// KindToken is a bearer token that always carries an expiry.
	KindToken Kind = "token"
	// KindSSH is an ed25519 OpenSSH key pair.
	KindSSH Kind = "ssh"
	// KindOAuth is an OAuth access and refresh token pair with one refresh
	// owner.
	KindOAuth Kind = "oauth"
)

// Kinds returns every kind, in table order. It returns a fresh slice.
func Kinds() []Kind {
	return []Kind{KindEd25519, KindOpaque, KindToken, KindSSH, KindOAuth}
}

// Field is a projectable credential field: the field half of an Item.
type Field string

// The projectable fields.
const (
	// FieldValue is the stored bytes (opaque, ed25519) or the token (token).
	FieldValue Field = "value"
	// FieldExpiresAt is a token's expiry, rendered as RFC 3339 UTC text.
	FieldExpiresAt Field = "expiresAt"
	// FieldPrivateKey is an ssh key's private half, unencrypted OpenSSH PEM.
	// It is projected only when an item names it.
	FieldPrivateKey Field = "privateKey"
	// FieldPublicKey is an ssh key's authorized_keys line.
	FieldPublicKey Field = "publicKey"
	// FieldAccessToken is an oauth pair's access token.
	FieldAccessToken Field = "accessToken"
	// FieldAccessExpiresAt is an oauth access token's expiry, rendered as RFC
	// 3339 UTC text.
	FieldAccessExpiresAt Field = "accessExpiresAt"
	// FieldTokenType is an oauth pair's token type (Bearer by default).
	FieldTokenType Field = "tokenType"
)

// RefreshTokenField names the oauth refresh token. It is NOT a Field and no
// kind projects it: the KeyDelivery schema refuses it, and it exists here so
// tests can assert exactly that.
const RefreshTokenField = "refreshToken"

// MaxFieldLength is the longest Field name the KeyDelivery schema admits.
const MaxFieldLength = 32

// FieldsFor returns the projectable fields of kind, in table order, or nil
// for an unknown kind. It returns a fresh slice.
func FieldsFor(kind Kind) []Field {
	switch kind {
	case KindEd25519, KindOpaque:
		return []Field{FieldValue}
	case KindToken:
		return []Field{FieldValue, FieldExpiresAt}
	case KindSSH:
		return []Field{FieldPrivateKey, FieldPublicKey}
	case KindOAuth:
		return []Field{FieldAccessToken, FieldAccessExpiresAt, FieldTokenType}
	}
	return nil
}

// Fields returns the union of every kind's projectable fields, each once, in
// first-appearance order over Kinds. It is the KeyDelivery CRD's field enum.
// It returns a fresh slice.
func Fields() []Field {
	var out []Field
	seen := map[Field]bool{}
	for _, k := range Kinds() {
		for _, f := range FieldsFor(k) {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	return out
}

// Item is one projection: the credential Field written under Key in the
// target Secret's data. A KeyDelivery's spec.items and the root-file target
// both carry it. Order among items is not semantic.
type Item struct {
	// Field is the credential field to project.
	Field Field `json:"field"`
	// Key is the Secret data key, in the Secret data-key grammar.
	Key string `json:"key"`
}

// RefreshOwnerLocal names a process outside k3sm as an oauth pair's refresh
// owner; the server never refreshes such a pair.
const RefreshOwnerLocal = "local"

// RefreshOwnerServerPrefix prefixes server/<profile>: the k3sm server
// refreshes the pair through the root-declared refresh profile <profile>.
const RefreshOwnerServerPrefix = "server/"

// MaxRefreshOwnerLength is the longest refresh owner in bytes: "server/" and
// a 63-byte DNS-1123 label.
const MaxRefreshOwnerLength = 70

// RefreshOwnerPattern is the refresh-owner grammar: local, or server/ and a
// DNS-1123 label.
const RefreshOwnerPattern = `^(local|server/[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?)$`

// ErrInvalidRefreshOwner is returned, wrapped or bare, by ParseRefreshOwner.
var ErrInvalidRefreshOwner = errors.New("invalid refresh owner")

var refreshOwnerRE = regexp.MustCompile(RefreshOwnerPattern)

// ParseRefreshOwner validates s and returns the profile name for
// server/<profile>, or "" for local.
func ParseRefreshOwner(s string) (profile string, err error) {
	if len(s) > MaxRefreshOwnerLength || !refreshOwnerRE.MatchString(s) {
		return "", ErrInvalidRefreshOwner
	}
	if s == RefreshOwnerLocal {
		return "", nil
	}
	return strings.TrimPrefix(s, RefreshOwnerServerPrefix), nil
}

// ConditionCredentialValid is the KeyDelivery condition type reporting the
// delivered credential's health. It is set only for kinds with an expiry
// (token, oauth) and is absent for the others. Ready means delivered and
// does not change on expiry or revocation; health readers read this.
const ConditionCredentialValid = "CredentialValid"

// The CredentialValid reasons, a closed set. Valid, Expiring and
// RefreshFailing share status True on purpose: the access token still works
// in all three, and readers key on the reason.
const (
	// ReasonValid (True): not expiring, refresh healthy.
	ReasonValid = "Valid"
	// ReasonExpiring (True): within the expiring threshold.
	ReasonExpiring = "Expiring"
	// ReasonRefreshFailing (True): the last server refresh failed transiently
	// and the access token is still valid.
	ReasonRefreshFailing = "RefreshFailing"
	// ReasonExpired (False): past expiry with no newer version.
	ReasonExpired = "Expired"
	// ReasonRevoked (False): the token endpoint answered a revoked code.
	ReasonRevoked = "Revoked"
	// ReasonIndeterminate (Unknown): the entry could not be opened.
	ReasonIndeterminate = "Indeterminate"
)

// CredentialValidReasons returns the closed CredentialValid reason set. It
// returns a fresh slice.
func CredentialValidReasons() []string {
	return []string{ReasonValid, ReasonExpiring, ReasonRefreshFailing, ReasonExpired, ReasonRevoked, ReasonIndeterminate}
}

// ReasonProjectionInvalid is the Ready=False reason when an item names a
// field the entry's kind does not have; any existing Secret is left
// untouched.
const ReasonProjectionInvalid = "ProjectionInvalid"

// EventType is an Event's type.
type EventType string

// The Event types k3sm emits for keystore deliveries.
const (
	EventTypeNormal  EventType = "Normal"
	EventTypeWarning EventType = "Warning"
)

// The Event reasons k3sm emits for keystore deliveries. Each fires on a real
// transition only.
const (
	// EventCredentialExpiring (Warning): into Expiring.
	EventCredentialExpiring = "CredentialExpiring"
	// EventCredentialExpired (Warning): into Expired.
	EventCredentialExpired = "CredentialExpired"
	// EventCredentialRefreshFailing (Warning): into RefreshFailing.
	EventCredentialRefreshFailing = "CredentialRefreshFailing"
	// EventCredentialRevoked (Warning): into Revoked.
	EventCredentialRevoked = "CredentialRevoked"
	// EventCredentialRecovered (Normal): back to Valid after any other
	// reason.
	EventCredentialRecovered = "CredentialRecovered"
	// EventKeystoreSecretMounted (Normal): on a Pod, once per pod per
	// delivered version of a labelled Secret.
	EventKeystoreSecretMounted = "KeystoreSecretMounted"
)

// EventReasons returns every Event reason with its type. It returns a fresh
// map.
func EventReasons() map[string]EventType {
	return map[string]EventType{
		EventCredentialExpiring:       EventTypeWarning,
		EventCredentialExpired:        EventTypeWarning,
		EventCredentialRefreshFailing: EventTypeWarning,
		EventCredentialRevoked:        EventTypeWarning,
		EventCredentialRecovered:      EventTypeNormal,
		EventKeystoreSecretMounted:    EventTypeNormal,
	}
}

// RefreshState is an oauth pair's refresh state, as list -o json's
// refresh_state reports it.
type RefreshState string

// The refresh states.
const (
	RefreshStateOK      RefreshState = "ok"
	RefreshStateFailing RefreshState = "failing"
	RefreshStateRevoked RefreshState = "revoked"
	RefreshStateOff     RefreshState = "off"
)

// RefreshStates returns every refresh state. It returns a fresh slice.
func RefreshStates() []RefreshState {
	return []RefreshState{RefreshStateOK, RefreshStateFailing, RefreshStateRevoked, RefreshStateOff}
}

// The metadata keys on a delivered Secret, so a reader (the provider) never
// imports the keystore itself.
const (
	// LabelDelivery is the label every keystore-delivered Secret carries.
	LabelDelivery = "k3sm.io/keystore-delivery"
	// AnnotationVersion is the annotation holding the delivered entry
	// version.
	AnnotationVersion = "k3sm.io/keystore-version"
)
