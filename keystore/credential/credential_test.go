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

package credential

import (
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestFieldTableVectors pins the projection field table per kind and its
// union, and the item's wire form.
func TestFieldTableVectors(t *testing.T) {
	t.Parallel()
	want := map[Kind][]Field{
		"ed25519": {"value"},
		"opaque":  {"value"},
		"token":   {"value", "expiresAt"},
		"ssh":     {"privateKey", "publicKey"},
		"oauth":   {"accessToken", "accessExpiresAt", "tokenType"},
	}
	for _, k := range Kinds() {
		if got := FieldsFor(k); !reflect.DeepEqual(got, want[k]) {
			t.Errorf("FieldsFor(%s) = %v, want %v", k, got, want[k])
		}
	}
	if got := FieldsFor("unknown"); got != nil {
		t.Errorf("FieldsFor(unknown) = %v, want nil", got)
	}
	wantUnion := []Field{"value", "expiresAt", "privateKey", "publicKey", "accessToken", "accessExpiresAt", "tokenType"}
	if got := Fields(); !reflect.DeepEqual(got, wantUnion) {
		t.Errorf("Fields() = %v, want %v", got, wantUnion)
	}
	for _, f := range Fields() {
		if len(f) > MaxFieldLength {
			t.Errorf("field %q is longer than MaxFieldLength %d", f, MaxFieldLength)
		}
	}
	b, err := json.Marshal(Item{Field: FieldAccessToken, Key: "access_token"})
	if err != nil || string(b) != `{"field":"accessToken","key":"access_token"}` {
		t.Errorf("Item wire form = %s, %v", b, err)
	}
	a := Fields()
	a[0] = "x"
	if Fields()[0] != FieldValue {
		t.Error("Fields returns an aliased slice")
	}
}

// TestNoRefreshField asserts no kind projects a refresh field, by name or by
// any name mentioning refresh.
func TestNoRefreshField(t *testing.T) {
	t.Parallel()
	for _, k := range Kinds() {
		for _, f := range FieldsFor(k) {
			if string(f) == RefreshTokenField || strings.Contains(strings.ToLower(string(f)), "refresh") {
				t.Errorf("kind %s projects the refresh field %q", k, f)
			}
		}
	}
}

// TestRefreshOwnerGrammarVectors runs the refresh-owner grammar.
func TestRefreshOwnerGrammarVectors(t *testing.T) {
	t.Parallel()
	label63 := strings.Repeat("a", 63)
	for _, tc := range []struct {
		in      string
		profile string
		ok      bool
	}{
		{"local", "", true},
		{"server/github", "github", true},
		{"server/github-e2e", "github-e2e", true},
		{"server/a", "a", true},
		{"server/" + label63, label63, true},
		{"server/" + label63 + "a", "", false},
		{"server/", "", false},
		{"server/GitHub", "", false},
		{"server/-a", "", false},
		{"server/a-", "", false},
		{"server/a.b", "", false},
		{"server/a/b", "", false},
		{"Local", "", false},
		{"local/x", "", false},
		{"pod/x", "", false},
		{"", "", false},
		{"server/a\n", "", false},
	} {
		p, err := ParseRefreshOwner(tc.in)
		if (err == nil) != tc.ok || p != tc.profile {
			t.Errorf("ParseRefreshOwner(%.20q) = %q, %v; want %q, ok %v", tc.in, p, err, tc.profile, tc.ok)
		}
		if err != nil && !errors.Is(err, ErrInvalidRefreshOwner) {
			t.Errorf("ParseRefreshOwner(%.20q) error %v is not ErrInvalidRefreshOwner", tc.in, err)
		}
	}
	if MaxRefreshOwnerLength != len(RefreshOwnerServerPrefix)+63 {
		t.Errorf("MaxRefreshOwnerLength = %d, want %d", MaxRefreshOwnerLength, len(RefreshOwnerServerPrefix)+63)
	}
}

// TestNameSetsClosed pins every closed name set.
func TestNameSetsClosed(t *testing.T) {
	t.Parallel()
	if got := Kinds(); !reflect.DeepEqual(got, []Kind{"ed25519", "opaque", "token", "ssh", "oauth"}) {
		t.Errorf("Kinds() = %v", got)
	}
	if ConditionCredentialValid != "CredentialValid" || ReasonProjectionInvalid != "ProjectionInvalid" {
		t.Error("condition or ProjectionInvalid name changed")
	}
	if got := CredentialValidReasons(); !reflect.DeepEqual(got, []string{"Valid", "Expiring", "RefreshFailing", "Expired", "Revoked", "Indeterminate"}) {
		t.Errorf("CredentialValidReasons() = %v", got)
	}
	wantEvents := map[string]EventType{
		"CredentialExpiring": "Warning", "CredentialExpired": "Warning", "CredentialRefreshFailing": "Warning",
		"CredentialRevoked": "Warning", "CredentialRecovered": "Normal", "KeystoreSecretMounted": "Normal",
	}
	if got := EventReasons(); !reflect.DeepEqual(got, wantEvents) {
		t.Errorf("EventReasons() = %v", got)
	}
	if got := RefreshStates(); !reflect.DeepEqual(got, []RefreshState{"ok", "failing", "revoked", "off"}) {
		t.Errorf("RefreshStates() = %v", got)
	}
	if LabelDelivery != "k3sm.io/keystore-delivery" || AnnotationVersion != "k3sm.io/keystore-version" {
		t.Error("metadata keys changed")
	}
}

// TestImportsStdlibOnly keeps the leaf free of every non-standard import.
func TestImportsStdlibOnly(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		af, err := parser.ParseFile(fset, f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range af.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if first, _, _ := strings.Cut(p, "/"); strings.Contains(first, ".") {
				t.Errorf("%s imports %s; the credential leaf imports the standard library only", f, p)
			}
		}
	}
}
