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

package mlxv1alpha1

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/util/yaml"

	"k3sm.io/apis/config/crd"
)

// TestDistributedInterconnectEnum pins the MLXDistributed.Interconnect values,
// its JSON name and omitempty, and that the CRD declares exactly the same enum
// with the default auto.
//
// The operator branches on these strings and the CRD admits them; a value the
// Go side knows and the CRD does not is refused at admission, and a field the
// CRD does not declare is pruned, which would silently serve over the network.
func TestDistributedInterconnectEnum(t *testing.T) {
	t.Parallel()
	want := []MLXInterconnect{MLXInterconnectAuto, MLXInterconnectThunderbolt, MLXInterconnectNetwork}
	for i, s := range []string{"auto", "thunderbolt", "network"} {
		if string(want[i]) != s {
			t.Errorf("MLXInterconnect value %d = %q, want %q", i, want[i], s)
		}
	}

	b, err := json.Marshal(MLXDistributed{Ranks: 2, Interconnect: MLXInterconnectThunderbolt})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(b); got != `{"ranks":2,"interconnect":"thunderbolt"}` {
		t.Errorf("MLXDistributed JSON = %s", got)
	}
	b, err = json.Marshal(MLXDistributed{Ranks: 2})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(b); got != `{"ranks":2}` {
		t.Errorf("an unset interconnect must be omitted (additive field), got %s", got)
	}
	cp := (&MLXDistributed{Interconnect: MLXInterconnectNetwork}).DeepCopy()
	if cp.Interconnect != MLXInterconnectNetwork {
		t.Errorf("DeepCopy lost interconnect: %q", cp.Interconnect)
	}

	var m map[string]any
	if err := yaml.Unmarshal(crd.MLXModelCRD(), &m); err != nil {
		t.Fatalf("CRD manifest is not valid YAML: %v", err)
	}
	versions, _ := m["spec"].(map[string]any)["versions"].([]any)
	if len(versions) != 1 {
		t.Fatalf("CRD has %d versions, want 1", len(versions))
	}
	node := versions[0].(map[string]any)
	for _, k := range []string{"schema", "openAPIV3Schema", "properties", "spec", "properties", "distributed", "properties", "interconnect"} {
		next, ok := node[k].(map[string]any)
		if !ok {
			t.Fatalf("CRD has no %s on the path to spec.distributed.interconnect", k)
		}
		node = next
	}
	if got := node["default"]; got != string(MLXInterconnectAuto) {
		t.Errorf("spec.distributed.interconnect default = %v, want %q", got, MLXInterconnectAuto)
	}
	enum, _ := node["enum"].([]any)
	if len(enum) != len(want) {
		t.Fatalf("spec.distributed.interconnect enum = %v, want %v", enum, want)
	}
	for i := range want {
		if enum[i] != string(want[i]) {
			t.Errorf("spec.distributed.interconnect enum = %v, want %v", enum, want)
			break
		}
	}
}
