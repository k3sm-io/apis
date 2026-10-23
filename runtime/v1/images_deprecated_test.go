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

import (
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"
)

// TestImagesDeprecatedVerbSet pins which Images RPCs carry deprecated=true: the
// five user-image verbs, and no others.
func TestImagesDeprecatedVerbSet(t *testing.T) {
	want := map[string]bool{
		"LoadImage": true, "TagImage": true, "UntagImage": true,
		"SaveImage": true, "RemoveImage": true,
	}
	methods := File_runtime_v1_images_proto.Services().ByName("Images").Methods()
	if methods.Len() == 0 {
		t.Fatal("Images service has no methods")
	}
	for i := 0; i < methods.Len(); i++ {
		m := methods.Get(i)
		name := string(m.Name())
		opts, _ := m.Options().(*descriptorpb.MethodOptions)
		got := opts.GetDeprecated()
		if got != want[name] {
			t.Errorf("%s: deprecated=%v, want %v", name, got, want[name])
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("deprecated verb %s not found on the Images service", name)
	}
}
