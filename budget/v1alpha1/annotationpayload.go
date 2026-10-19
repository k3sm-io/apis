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

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BudgetAnnotationSchema is the current schema of the NodeBudgetReport and
// NodeBudgetDesiredPolicy annotation payloads. An incompatible change bumps
// it; a reader refuses any other value with ErrUnknownBudgetSchema.
const BudgetAnnotationSchema = 1

// MaxBudgetAnnotationBytes is the largest budget.k3sm.io/status or
// budget.k3sm.io/policy annotation value, in bytes, a reader accepts and a
// writer produces. A maximal valid report stays at or below three quarters
// of it.
const MaxBudgetAnnotationBytes = 16384

var (
	// ErrUnknownBudgetSchema reports an annotation payload whose schema is
	// missing or is not BudgetAnnotationSchema. The relay reports it as
	// Reporting False with ReasonSchemaUnsupported; it is not a node error
	// to retry.
	ErrUnknownBudgetSchema = errors.New("budgetv1alpha1: unknown budget annotation schema")
	// ErrInvalidBudgetAnnotation is wrapped by every other annotation
	// payload error: oversize, malformed JSON, an unknown, repeated or
	// case-folded key, trailing data, or a value that fails validation. The
	// relay reports it as Reporting False with ReasonAnnotationMalformed.
	ErrInvalidBudgetAnnotation = errors.New("budgetv1alpha1: invalid budget annotation")
)

// NodeBudgetReport is the payload of the AnnotationStatus annotation, the
// JSON a node writes on its own Node. It is untrusted input to the relay.
// It carries the observation only: the writer and the conditions belong to
// the status writer, and a payload that names them is refused.
type NodeBudgetReport struct {
	// Schema is the payload version (BudgetAnnotationSchema).
	Schema int `json:"schema"`

	NodeBudgetObservation `json:",inline"`
}

// Validate reports whether the report is well-formed: Schema is current
// (else ErrUnknownBudgetSchema) and the observation passes
// NodeBudgetObservation.Validate (else an error wrapping
// ErrInvalidBudgetAnnotation and ErrInvalidNodeBudget).
func (r *NodeBudgetReport) Validate() error {
	if r.Schema != BudgetAnnotationSchema {
		return fmt.Errorf("%w: report schema %d, want %d", ErrUnknownBudgetSchema, r.Schema, BudgetAnnotationSchema)
	}
	if err := r.NodeBudgetObservation.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	return nil
}

// Encode validates the report and returns the annotation value. The value
// is then decoded with ParseNodeBudgetReport, so Encode refuses anything
// Parse would refuse and a writer can never produce a value its reader
// rejects.
func (r *NodeBudgetReport) Encode() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	s, err := encodeBudgetAnnotation(r)
	if err != nil {
		return "", err
	}
	if _, err := ParseNodeBudgetReport(s); err != nil {
		return "", err
	}
	return s, nil
}

// ParseNodeBudgetReport decodes and validates an AnnotationStatus value with
// the strict decoder (see decodeBudgetAnnotation). A missing or unknown
// schema returns ErrUnknownBudgetSchema; every other failure wraps
// ErrInvalidBudgetAnnotation.
func ParseNodeBudgetReport(value string) (NodeBudgetReport, error) {
	var r NodeBudgetReport
	if err := decodeBudgetAnnotation(value, &r); err != nil {
		return NodeBudgetReport{}, err
	}
	if err := r.Validate(); err != nil {
		return NodeBudgetReport{}, err
	}
	return r, nil
}

// NodeBudgetDesiredPolicy is the payload of the AnnotationPolicy annotation,
// which the relay writes on a Node: the policy SelectPolicy chose for it.
// The node can rewrite its own annotation, so it validates the payload and
// applies the floors itself (ApplyFloors) whatever the relay checked.
type NodeBudgetDesiredPolicy struct {
	// Schema is the payload version (BudgetAnnotationSchema).
	Schema int `json:"schema"`
	// Name is the policy's name.
	Name string `json:"name"`
	// ResourceVersion is the policy's resourceVersion when relayed.
	ResourceVersion string `json:"resourceVersion"`
	// Spec is the policy's spec.
	Spec NodeBudgetPolicySpec `json:"spec"`
}

// Validate reports whether the payload is well-formed: Schema is current
// (else ErrUnknownBudgetSchema), Name is an object name, ResourceVersion is
// 1..MaxPolicyResourceVersionBytes printable ASCII bytes, and Spec passes
// NodeBudgetPolicySpec.Validate. Other errors wrap
// ErrInvalidBudgetAnnotation.
func (d *NodeBudgetDesiredPolicy) Validate() error {
	if d.Schema != BudgetAnnotationSchema {
		return fmt.Errorf("%w: desired schema %d, want %d", ErrUnknownBudgetSchema, d.Schema, BudgetAnnotationSchema)
	}
	if !isObjectName(d.Name) {
		return fmt.Errorf("%w: desired name %q is not an object name", ErrInvalidBudgetAnnotation, d.Name)
	}
	if !isPrintableASCII(d.ResourceVersion, MaxPolicyResourceVersionBytes) {
		return fmt.Errorf("%w: desired resourceVersion is not 1..%d printable ASCII bytes", ErrInvalidBudgetAnnotation, MaxPolicyResourceVersionBytes)
	}
	if err := d.Spec.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	return nil
}

// Encode validates the payload and returns the annotation value; like
// NodeBudgetReport.Encode it refuses anything ParseNodeBudgetDesiredPolicy
// would refuse.
func (d *NodeBudgetDesiredPolicy) Encode() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	s, err := encodeBudgetAnnotation(d)
	if err != nil {
		return "", err
	}
	if _, err := ParseNodeBudgetDesiredPolicy(s); err != nil {
		return "", err
	}
	return s, nil
}

// ParseNodeBudgetDesiredPolicy decodes and validates an AnnotationPolicy
// value, with the same decoder and errors as ParseNodeBudgetReport.
func ParseNodeBudgetDesiredPolicy(value string) (NodeBudgetDesiredPolicy, error) {
	var d NodeBudgetDesiredPolicy
	if err := decodeBudgetAnnotation(value, &d); err != nil {
		return NodeBudgetDesiredPolicy{}, err
	}
	if err := d.Validate(); err != nil {
		return NodeBudgetDesiredPolicy{}, err
	}
	return d, nil
}

// encodeBudgetAnnotation marshals v and enforces the size bound.
func encodeBudgetAnnotation(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	if len(b) > MaxBudgetAnnotationBytes {
		return "", fmt.Errorf("%w: payload is %d bytes, at most %d", ErrInvalidBudgetAnnotation, len(b), MaxBudgetAnnotationBytes)
	}
	return string(b), nil
}

// decodeBudgetAnnotation is the strict decoder. In order, it:
//
//  1. refuses a value over MaxBudgetAnnotationBytes before any parsing;
//  2. reads the schema on its own, so a future schema whose fields changed
//     type is reported as ErrUnknownBudgetSchema, not as malformed;
//  3. decodes with unknown fields disallowed;
//  4. refuses any data after the one JSON value;
//  5. refuses a key repeated within one object; and
//  6. refuses a key that matches a field only case-insensitively
//     (encoding/json would otherwise accept "Level" for "level").
//
// Validation of the decoded value is the caller's next step.
func decodeBudgetAnnotation(value string, v any) error {
	if len(value) > MaxBudgetAnnotationBytes {
		return fmt.Errorf("%w: payload is %d bytes, at most %d", ErrInvalidBudgetAnnotation, len(value), MaxBudgetAnnotationBytes)
	}
	var probe struct {
		Schema *int `json:"schema"`
	}
	if err := json.Unmarshal([]byte(value), &probe); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	if probe.Schema == nil {
		return fmt.Errorf("%w: payload has no schema", ErrUnknownBudgetSchema)
	}
	if *probe.Schema != BudgetAnnotationSchema {
		return fmt.Errorf("%w: schema %d, want %d", ErrUnknownBudgetSchema, *probe.Schema, BudgetAnnotationSchema)
	}
	dec := json.NewDecoder(strings.NewReader(value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing data after the payload", ErrInvalidBudgetAnnotation)
	}
	if err := checkDuplicateKeys([]byte(value)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	var generic any
	if err := json.Unmarshal([]byte(value), &generic); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	if err := checkExactKeys(generic, reflect.TypeOf(v).Elem(), ""); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBudgetAnnotation, err)
	}
	return nil
}

// checkDuplicateKeys walks the tokens of one JSON value and refuses an
// object that names a key twice (encoding/json keeps the last silently).
func checkDuplicateKeys(b []byte) error {
	type frame struct {
		obj       bool
		expectKey bool
		keys      map[string]bool
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	var stack []*frame
	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].obj {
			stack[n-1].expectKey = true
		}
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if n := len(stack); n > 0 && stack[n-1].obj && stack[n-1].expectKey {
			if d, ok := tok.(json.Delim); ok && d == '}' {
				stack = stack[:n-1]
				valueDone()
				continue
			}
			k, _ := tok.(string)
			if stack[n-1].keys[k] {
				return fmt.Errorf("key %q is repeated", k)
			}
			stack[n-1].keys[k] = true
			stack[n-1].expectKey = false
			continue
		}
		switch d := tok.(type) {
		case json.Delim:
			switch d {
			case '{':
				stack = append(stack, &frame{obj: true, expectKey: true, keys: map[string]bool{}})
			case '[':
				stack = append(stack, &frame{})
			default: // ']'; '}' is consumed in the key position above
				stack = stack[:len(stack)-1]
				valueDone()
			}
		default:
			valueDone()
		}
		if len(stack) == 0 {
			return nil
		}
	}
}

var metav1TimeType = reflect.TypeOf(metav1.Time{})

// checkExactKeys refuses any object key in v that is not, byte for byte, the
// JSON name of a field of t. encoding/json matches keys case-insensitively,
// so DisallowUnknownFields alone admits "Level" for "level".
func checkExactKeys(v any, t reflect.Type, path string) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v == nil || t == metav1TimeType {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		fields := map[string]reflect.Type{}
		collectJSONFields(t, fields)
		for k, child := range obj {
			ft, ok := fields[k]
			if !ok {
				return fmt.Errorf("%s: unknown field %q", strings.TrimPrefix(path, "."), k)
			}
			if err := checkExactKeys(child, ft, path+"."+k); err != nil {
				return err
			}
		}
	case reflect.Slice:
		list, ok := v.([]any)
		if !ok {
			return nil
		}
		for i, e := range list {
			if err := checkExactKeys(e, t.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// collectJSONFields maps every JSON field name of struct t to its type,
// descending into embedded structs that encoding/json inlines.
func collectJSONFields(t reflect.Type, out map[string]reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			collectJSONFields(f.Type, out)
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
}
