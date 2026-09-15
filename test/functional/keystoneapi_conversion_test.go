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

package functional_test

import (
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	"k8s.io/apimachinery/pkg/types"

	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystonev1beta2 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta2"
)

// These tests exercise the KeystoneAPI conversion webhook end to end through
// the envtest apiserver. v1beta2 is the Hub/storage version and v1beta1 is the
// Spoke, so reading a KeystoneAPI as v1beta1 forces the apiserver to call the
// /convert webhook (v1beta2 -> v1beta1). If the webhook were not served or the
// conversion were broken, the v1beta1 Get would fail.
var _ = Describe("KeystoneAPI conversion webhook", func() {
	var keystoneAPIName types.NamespacedName

	BeforeEach(func() {
		keystoneAPIName = types.NamespacedName{
			Namespace: namespace,
			Name:      "keystone",
		}
		DeferCleanup(
			th.DeleteInstance,
			CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()),
		)
	})

	It("is a lossless no-op round trip between v1beta1 (spoke) and v1beta2 (hub)", func() {
		// v1beta2 is the storage version, so this Get is served directly.
		v2 := &keystonev1beta2.KeystoneAPI{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, keystoneAPIName, v2)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		// v1beta1 is a spoke, so this Get goes through the conversion webhook.
		v1 := &keystonev1beta1.KeystoneAPI{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, keystoneAPIName, v1)).To(Succeed())
		}, timeout, interval).Should(Succeed())

		// no-op bump: the spec must be identical across versions.
		Expect(v1.Spec.DatabaseInstance).To(Equal(v2.Spec.DatabaseInstance))
		Expect(v1.Spec.DatabaseAccount).To(Equal(v2.Spec.DatabaseAccount))
		Expect(v1.Spec.Secret).To(Equal(v2.Spec.Secret))
		Expect(v1.Spec.FernetMaxActiveKeys).To(Equal(v2.Spec.FernetMaxActiveKeys))
		Expect(v1.Spec.Replicas).To(Equal(v2.Spec.Replicas))
		// a defaulted, nested field must survive conversion too.
		Expect(v1.Spec.PasswordSelectors.Admin).To(Equal(v2.Spec.PasswordSelectors.Admin))
	})
})
