/*
Copyright 2025.

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

package v1beta1

import (
	"reflect"
	"testing"

	fuzz "github.com/google/gofuzz"

	keystonev1beta2 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta2"
)

// TestKeystoneAPIConversionRoundTrip verifies that converting a v1beta1
// KeystoneAPI up to the v1beta2 hub and back is lossless. v1beta2 is a no-op
// bump (identical schema), so the round trip must be the identity.
//
// Fuzzing populates every field, so if a field is later added to the spec or
// status but forgotten in the explicit conversion copies in
// keystoneapi_conversion.go, ConvertTo drops it, ConvertFrom leaves it zero,
// and this test fails - which is the regression guard we want.
func TestKeystoneAPIConversionRoundTrip(t *testing.T) {
	f := fuzz.New()

	for i := 0; i < 100; i++ {
		// ConvertTo/ConvertFrom only copy ObjectMeta+Spec+Status (not TypeMeta),
		// so only fuzz those to keep the comparison meaningful.
		src := &KeystoneAPI{}
		f.Fuzz(&src.ObjectMeta)
		f.Fuzz(&src.Spec)
		f.Fuzz(&src.Status)

		hub := &keystonev1beta2.KeystoneAPI{}
		if err := src.ConvertTo(hub); err != nil {
			t.Fatalf("ConvertTo failed: %v", err)
		}

		got := &KeystoneAPI{}
		if err := got.ConvertFrom(hub); err != nil {
			t.Fatalf("ConvertFrom failed: %v", err)
		}

		if !reflect.DeepEqual(src, got) {
			t.Fatalf("conversion round trip is not lossless:\n src = %#v\n got = %#v", src, got)
		}
	}
}
