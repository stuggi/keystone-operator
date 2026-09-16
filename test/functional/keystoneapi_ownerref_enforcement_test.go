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

// This suite reproduces the real-cluster failure that the admin-client
// convergence specs (keystoneapi_ownerref_test.go) cannot. On a CRD version
// bump the operator must rewrite the SA controller ownerReference apiVersion
// (v1beta1 -> v1beta2). The OwnerReferencesPermissionEnforcement admission
// plugin gates any change to an object's ownerReferences on the caller having
// "delete" permission on that object. envtest's default client is
// system:masters (superuser), which bypasses the check - so those specs pass
// even when the serviceaccounts RBAC marker is missing "delete".
//
// Here the flip is driven as a non-superuser bound to the operator's *generated*
// ClusterRole (config/rbac/role.yaml), so dropping "delete" from the marker
// regenerates role.yaml without it and turns this test red.

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"
)

var _ = Describe("KeystoneAPI SA owner-ref convergence under OwnerReferencesPermissionEnforcement", func() {
	var scopedClient client.Client

	BeforeEach(func() {
		testGroup := "ownerref-gate-" + uuid.New().String()

		// Load the operator's generated ClusterRole so the test tracks the RBAC
		// markers rather than a hand-copied verb list.
		roleYAML, err := os.ReadFile(filepath.Join("..", "..", "config", "rbac", "role.yaml"))
		Expect(err).NotTo(HaveOccurred())
		managerRole := &rbacv1.ClusterRole{}
		Expect(yaml.Unmarshal(roleYAML, managerRole)).To(Succeed())

		gateRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: testGroup + "-role"},
			Rules:      managerRole.Rules,
		}
		Expect(k8sClient.Create(ctx, gateRole)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, gateRole)

		// A namespaced binding keeps the grant scoped to this test's namespace.
		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: testGroup + "-binding", Namespace: namespace},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     gateRole.Name,
			},
			Subjects: []rbacv1.Subject{{
				Kind:     rbacv1.GroupKind,
				APIGroup: rbacv1.GroupName,
				Name:     testGroup,
			}},
		}
		Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, binding)

		// Non-superuser client carrying exactly the operator's generated RBAC.
		authUser, err := testEnv.AddUser(
			envtest.User{Name: testGroup + "-user", Groups: []string{testGroup}}, cfg)
		Expect(err).NotTo(HaveOccurred())
		scopedClient, err = client.New(authUser.Config(), client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
	})

	It("permits the operator to rewrite the SA owner-ref apiVersion on a version bump", func() {
		// Owner: a real KeystoneAPI so the ownerRef carries keystone GVKs and the
		// blockOwnerDeletion finalizers check maps to keystoneapis/finalizers,
		// which the operator role grants - isolating "delete serviceaccounts" as
		// the single verb under test.
		ownerName := types.NamespacedName{Name: "ownerref-gate-" + uuid.New().String(), Namespace: namespace}
		owner := CreateKeystoneAPI(ownerName, GetDefaultKeystoneAPISpec())
		DeferCleanup(th.DeleteInstance, owner)

		// Child SA created (as admin) with a pre-bump v1beta1 controller ownerRef,
		// mirroring an SA left over from before the CRD version bump. Its name
		// differs from the controller-managed keystone-<name> SA so the running
		// controller never touches it.
		blockOwnerDeletion := true
		isController := true
		saName := types.NamespacedName{Name: ownerName.Name + "-sa", Namespace: namespace}
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName.Name,
				Namespace: saName.Namespace,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         "keystone.openstack.org/v1beta1",
					Kind:               "KeystoneAPI",
					Name:               ownerName.Name,
					UID:                owner.GetUID(),
					Controller:         &isController,
					BlockOwnerDeletion: &blockOwnerDeletion,
				}},
			},
		}
		Expect(k8sClient.Create(ctx, sa)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, sa)

		// The convergence step, performed as the operator (non-superuser): rewrite
		// the ownerRef apiVersion to the new storage version. This is exactly the
		// patch ReconcileRbac -> SetControllerReference issues after the bump; it
		// changes ownerReferences, so it is gated on "delete serviceaccounts".
		// Without that verb the admission plugin returns Forbidden and this
		// Eventually times out - which is the regression signal. The Eventually
		// also absorbs RBAC-propagation lag for the freshly created binding.
		Eventually(func(g Gomega) {
			cur := &corev1.ServiceAccount{}
			g.Expect(scopedClient.Get(ctx, saName, cur)).To(Succeed())
			patchBase := cur.DeepCopy()
			for i := range cur.OwnerReferences {
				if cur.OwnerReferences[i].Kind == "KeystoneAPI" {
					cur.OwnerReferences[i].APIVersion = "keystone.openstack.org/v1beta2"
				}
			}
			g.Expect(scopedClient.Patch(ctx, cur, client.MergeFrom(patchBase))).To(Succeed())
		}, timeout, interval).Should(Succeed())

		// Confirm convergence landed.
		Eventually(func(g Gomega) {
			cur := &corev1.ServiceAccount{}
			g.Expect(k8sClient.Get(ctx, saName, cur)).To(Succeed())
			var found string
			for _, ref := range cur.OwnerReferences {
				if ref.Kind == "KeystoneAPI" {
					found = ref.APIVersion
				}
			}
			g.Expect(found).To(Equal("keystone.openstack.org/v1beta2"))
		}, timeout, interval).Should(Succeed())
	})
})
