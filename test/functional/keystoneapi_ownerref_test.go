/*
Copyright 2026.

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

// Tests for SA owner-ref convergence across CRD version bumps.
//
// The existing ReconcileRbac → serviceaccount.CreateOrPatch → SetControllerReference
// path self-heals a stale ownerReferences[].apiVersion on the next reconcile once
// the controller operates on v1beta2 objects. These tests prove that convergence,
// its idempotency, and the recreate-path edge case. No net-new patch code is
// required.

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KeystoneAPI SA owner-ref convergence across CRD version bumps", func() {
	var keystoneAPIName types.NamespacedName
	var saName types.NamespacedName
	var keystoneAccountName types.NamespacedName
	var keystoneDatabaseName types.NamespacedName

	// waitForSA blocks until the SA exists and returns it.
	waitForSA := func() *corev1.ServiceAccount {
		sa := &corev1.ServiceAccount{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
		}, timeout, interval).Should(Succeed())
		return sa
	}

	// ownerRefAPIVersion returns the apiVersion on the KeystoneAPI controller
	// owner ref, or "" if none is found.
	ownerRefAPIVersion := func(sa *corev1.ServiceAccount) string {
		for _, ref := range sa.OwnerReferences {
			if ref.Kind == "KeystoneAPI" && ref.Controller != nil && *ref.Controller {
				return ref.APIVersion
			}
		}
		return ""
	}

	BeforeEach(func() {
		// Skip the whole suite when the CRD serves only a single version: the
		// controller writes v1beta1 refs in that case and there is nothing to
		// self-heal. Once v1beta2 is added as a served version these tests
		// become active automatically.
		crd := &apiextv1.CustomResourceDefinition{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "keystoneapis.keystone.openstack.org"}, crd)).To(Succeed())
		served := 0
		for _, v := range crd.Spec.Versions {
			if v.Served {
				served++
			}
		}
		if served < 2 {
			Skip("SA owner-ref convergence tests require v1beta2 as a served CRD version")
		}

		keystoneAPIName = types.NamespacedName{Name: "keystone", Namespace: namespace}
		// RbacResourceName() = "keystone-" + instance.Name
		saName = types.NamespacedName{Name: "keystone-keystone", Namespace: namespace}
		keystoneAccountName = types.NamespacedName{Name: AccountName, Namespace: namespace}
		keystoneDatabaseName = types.NamespacedName{Name: DatabaseCRName, Namespace: namespace}

		// Full setup required to advance past all reconcileNormal gates and
		// reach reconcileInit (where ReconcileRbac creates the SA).
		DeferCleanup(
			k8sClient.Delete, ctx,
			CreateKeystoneMessageBusSecret(namespace, "rabbitmq-secret"),
		)
		DeferCleanup(th.DeleteInstance, CreateKeystoneAPI(keystoneAPIName, GetDefaultKeystoneAPISpec()))
		DeferCleanup(
			k8sClient.Delete, ctx,
			CreateKeystoneAPISecret(namespace, SecretName),
		)
		DeferCleanup(
			infra.DeleteMemcached,
			infra.CreateMemcached(namespace, "memcached", infra.GetDefaultMemcachedSpec()),
		)
		DeferCleanup(
			mariadb.DeleteDBService,
			mariadb.CreateDBService(
				namespace,
				"openstack", // spec.databaseInstance from GetDefaultKeystoneAPISpec
				corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 3306}},
				},
			),
		)
		mariadb.SimulateMariaDBAccountCompleted(keystoneAccountName)
		mariadb.SimulateMariaDBDatabaseCompleted(keystoneDatabaseName)
		infra.SimulateTransportURLReady(types.NamespacedName{
			Name:      fmt.Sprintf("%s-keystone-transport", keystoneAPIName.Name),
			Namespace: namespace,
		})
		infra.SimulateMemcachedReady(types.NamespacedName{
			Name:      "memcached",
			Namespace: namespace,
		})
	})

	It("self-heals a stale v1beta1 SA owner ref to v1beta2 on the next reconcile", func() {
		// Step 1: wait for SA to be created with the current (v1beta2) owner ref.
		waitForSA()
		Eventually(func(g Gomega) {
			sa := &corev1.ServiceAccount{}
			g.Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
			g.Expect(ownerRefAPIVersion(sa)).To(Equal("keystone.openstack.org/v1beta2"))
		}, timeout, interval).Should(Succeed())

		// Step 2: simulate a pre-bump SA by forcing the owner ref back to v1beta1.
		sa := &corev1.ServiceAccount{}
		Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
		base := sa.DeepCopy()
		for i, ref := range sa.OwnerReferences {
			if ref.Kind == "KeystoneAPI" {
				sa.OwnerReferences[i].APIVersion = "keystone.openstack.org/v1beta1"
			}
		}
		Expect(k8sClient.Patch(ctx, sa, client.MergeFrom(base))).To(Succeed())

		// Confirm the patch landed.
		Eventually(func(g Gomega) {
			sa := &corev1.ServiceAccount{}
			g.Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
			g.Expect(ownerRefAPIVersion(sa)).To(Equal("keystone.openstack.org/v1beta1"))
		}, timeout, interval).Should(Succeed())

		// Step 3: the SA change triggers a reconcile (controller Owns() the SA);
		// assert that ReconcileRbac → SetControllerReference → CreateOrPatch
		// self-heals the owner ref back to v1beta2.
		Eventually(func(g Gomega) {
			sa := &corev1.ServiceAccount{}
			g.Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
			g.Expect(ownerRefAPIVersion(sa)).To(Equal("keystone.openstack.org/v1beta2"))
		}, timeout, interval).Should(Succeed())
	})

	It("is idempotent: a second reconcile leaves the v1beta2 owner ref unchanged", func() {
		// Wait for SA with v1beta2 owner ref.
		Eventually(func(g Gomega) {
			sa := &corev1.ServiceAccount{}
			g.Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
			g.Expect(ownerRefAPIVersion(sa)).To(Equal("keystone.openstack.org/v1beta2"))
		}, timeout, interval).Should(Succeed())

		// Capture the resourceVersion after convergence.
		sa := &corev1.ServiceAccount{}
		Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
		stableRV := sa.ResourceVersion

		// In envtest no other actor modifies the SA, so a stable ResourceVersion
		// proves CreateOrPatch returned OperationResultNone (owner ref was already
		// correct — SetControllerReference found no diff to patch).
		Consistently(func(g Gomega) {
			sa := &corev1.ServiceAccount{}
			g.Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
			g.Expect(ownerRefAPIVersion(sa)).To(Equal("keystone.openstack.org/v1beta2"))
			g.Expect(sa.ResourceVersion).To(Equal(stableRV))
		}, time.Second*5, interval).Should(Succeed())
	})

	It("recreates a deleted SA with a v1beta2 owner ref", func() {
		// Wait for initial SA creation.
		waitForSA()

		// Delete the SA.
		sa := &corev1.ServiceAccount{}
		Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
		Expect(k8sClient.Delete(ctx, sa)).To(Succeed())

		// ReconcileRbac recreates the SA; the new one must carry a v1beta2 ref.
		Eventually(func(g Gomega) {
			sa := &corev1.ServiceAccount{}
			g.Expect(k8sClient.Get(ctx, saName, sa)).To(Succeed())
			g.Expect(ownerRefAPIVersion(sa)).To(Equal("keystone.openstack.org/v1beta2"))
		}, timeout, interval).Should(Succeed())
	})
})
