/*


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

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	gophercloud "github.com/gophercloud/gophercloud"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	external "github.com/openstack-k8s-operators/keystone-operator/pkg/external"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	gopher "github.com/openstack-k8s-operators/lib-common/pkg/gopher"
	helper "github.com/openstack-k8s-operators/lib-common/pkg/helper"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GetClient -
func (r *KeystoneServiceReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *KeystoneServiceReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *KeystoneServiceReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *KeystoneServiceReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// KeystoneServiceReconciler reconciles a KeystoneService object
type KeystoneServiceReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch

// Reconcile keystone service requests
func (r *KeystoneServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("keystoneservice", req.NamespacedName)

	// Fetch the KeystoneService instance
	instance := &keystonev1.KeystoneService{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	helper, err := helper.NewHelper(
		instance,
		r.Client,
		r.Kclient,
		r.Scheme,
		r.Log,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch the instance status when exiting this function so we can persist any changes.
	defer func() {
		if err := helper.SetAfter(instance); err != nil {
			common.LogErrorForObject(r, err, "Set after and calc patch/diff", instance)
		}

		if changed := helper.GetChanges()["status"]; changed {
			patch := client.MergeFrom(helper.GetBeforeObject())

			if err := r.Status().Patch(ctx, instance, patch); err != nil && !k8s_errors.IsNotFound(err) {
				common.LogErrorForObject(r, err, "Update status", instance)
			}
		}
	}()

	//
	// Validate that keystoneAPI is up
	//
	keystoneAPI, err := external.GetKeystoneAPI(ctx, helper, instance.Namespace, map[string]string{})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			r.Log.Info("KeystoneAPI not found!")
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	if !keystoneAPI.IsReady() {
		r.Log.Info("KeystoneAPI not yet ready")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	//
	// get admin authentication details from keystoneAPI
	//
	identityClient, cond, ctrlResult, err := external.GetAdminServiceClient(ctx, helper, keystoneAPI)
	instance.Status.Conditions.UpdateCurrentCondition(cond)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance, helper, keystoneAPI, identityClient)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper, keystoneAPI, identityClient)

}

// SetupWithManager x
func (r *KeystoneServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneService{}).
		Complete(r)
}

func (r *KeystoneServiceReconciler) reconcileDelete(
	ctx context.Context,
	instance *keystonev1.KeystoneService,
	helper *helper.Helper,
	keystoneAPI *keystonev1.KeystoneAPI,
	identityClient *gophercloud.ServiceClient,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service delete")

	// Delete Endpoints
	err := gopher.DeleteEndpoints(
		identityClient,
		r.Log,
		instance.Status.ServiceID,
		instance.Spec.ServiceName,
		keystoneAPI.Spec.Region)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete User
	err = gopher.DeleteUser(
		identityClient,
		r.Log,
		instance.Spec.ServiceUser)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete Service
	err = gopher.DeleteService(identityClient, r.Log, instance.Status.ServiceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, helper.GetFinalizer())
	r.Log.Info("Reconciled Service delete successfully")
	if err := r.Update(ctx, instance); err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeystoneServiceReconciler) reconcileNormal(
	ctx context.Context,
	instance *keystonev1.KeystoneService,
	helper *helper.Helper,
	keystoneAPI *keystonev1.KeystoneAPI,
	identityClient *gophercloud.ServiceClient,
) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, helper.GetFinalizer())
	// Register the finalizer immediately to avoid orphaning resources on delete
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	//
	// Create new service if ServiceID is not already set
	//
	err := r.reconcileService(identityClient, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create/update/delete endpoint
	//
	err = r.reconcileEndpoints(
		identityClient,
		instance.Status.ServiceID,
		instance.Spec.ServiceName,
		keystoneAPI.Spec.Region,
		instance.Spec.APIEndpoints)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create/update service user
	//
	ctrlResult, err := r.reconcileUser(
		ctx,
		helper,
		identityClient,
		instance)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	r.Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneServiceReconciler) reconcileService(
	client *gophercloud.ServiceClient,
	instance *keystonev1.KeystoneService,
) error {
	r.Log.Info(fmt.Sprintf("Reconciling Service %s", instance.Spec.ServiceName))

	// Create new service if ServiceID is not already set
	if instance.Status.ServiceID == "" {

		// verify if there is still an existing service in keystone for
		// type and name, if so register the ID
		service, err := gopher.GetService(
			client,
			instance.Spec.ServiceType,
			instance.Spec.ServiceName,
		)
		if err != nil && !k8s_errors.IsNotFound(err) {
			return err
		}

		// if there is already a service registered use it
		if service != nil && instance.Status.ServiceID != service.ID {
			instance.Status.ServiceID = service.ID
		} else {
			instance.Status.ServiceID, err = gopher.CreateService(
				client,
				instance.Spec.ServiceName,
				instance.Spec.ServiceType,
				instance.Spec.ServiceDescription,
				instance.Spec.Enabled,
			)
			if err != nil {
				return err
			}
		}
	} else {
		// ServiceID is already set, update the service
		err := gopher.UpdateService(
			client,
			instance.Spec.ServiceName,
			instance.Status.ServiceID,
			instance.Spec.ServiceType,
			instance.Spec.ServiceDescription,
			instance.Spec.Enabled,
		)
		if err != nil {
			return err
		}
	}

	r.Log.Info("Reconciled Service successfully")
	return nil
}

func (r *KeystoneServiceReconciler) reconcileEndpoints(
	client *gophercloud.ServiceClient,
	serviceID string,
	serviceName string,
	region string,
	serviceEndpoints map[string]string,
) error {
	r.Log.Info("Reconciling Endpoints")

	// get all registered endpoints for the service
	allEndpoints, err := gopher.GetEndpoints(client, serviceID, region, "")
	if err != nil {
		return err
	}

	// create a map[string]string of the existing endpoints to verify if
	// something changed
	existingServiceEndpoints := make(map[string]string)
	existingServiceEndpointsIDX := make(map[string]int)
	for idx, endpoint := range allEndpoints {
		existingServiceEndpoints[string(endpoint.Availability)] = endpoint.URL
		existingServiceEndpointsIDX[string(endpoint.Availability)] = idx
	}

	if !reflect.DeepEqual(existingServiceEndpoints, serviceEndpoints) {
		// service endpoint got removed
		if len(serviceEndpoints) < len(existingServiceEndpoints) {
			for endpointInterface := range existingServiceEndpoints {
				if url, ok := serviceEndpoints[endpointInterface]; !ok {
					// TODO delete endpoint
					r.Log.Info(fmt.Sprintf("Delete endpoint %s %s - %s", serviceName, endpointInterface, url))
				}
			}
		}

		// create / update endpoints
		for endpointInterface, url := range serviceEndpoints {
			// get the gopher availability mapping for the endpointInterface
			availability, err := gopher.GetAvailability(endpointInterface)
			if err != nil {
				return err
			}

			if existingURL, ok := existingServiceEndpoints[endpointInterface]; ok {
				if url != existingURL {
					endpointIDX := existingServiceEndpointsIDX[endpointInterface]

					// Update the endpoint
					_, err := gopher.UpdateEndpoint(
						client,
						r.Log,
						allEndpoints[endpointIDX].ID,
						serviceID,
						serviceName,
						availability,
						region,
						url)
					if err != nil {
						return err
					}
				}
			} else {
				// Create the endpoint
				_, err := gopher.CreateEndpoint(
					client,
					r.Log,
					serviceID,
					serviceName,
					availability,
					region,
					url)
				if err != nil {
					return err
				}
			}
		}
	}

	r.Log.Info("Reconciled Endpoints successfully")
	return nil
}

func (r *KeystoneServiceReconciler) reconcileUser(
	ctx context.Context,
	h *helper.Helper,
	client *gophercloud.ServiceClient,
	instance *keystonev1.KeystoneService,
) (reconcile.Result, error) {
	r.Log.Info(fmt.Sprintf("Reconciling User %s", instance.Spec.ServiceUser))
	roleName := "admin"

	// get the password of the service user from the secret
	password, _, ctrlResult, err := common.GetDataFromSecret(
		ctx,
		h,
		instance.Spec.Secret,
		10,
		instance.Spec.PasswordSelector)
	if err != nil {
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	//  create service project if it does not exist
	//
	serviceProjectID, err := gopher.CreateProject(client, r.Log, "service", "service")
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	//  create role if it does not exist
	//
	_, err = gopher.CreateRole(client, r.Log, roleName)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create user if it does not exist
	//
	userID, err := gopher.CreateUser(client, r.Log, instance.Spec.ServiceUser, password, serviceProjectID)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// add user to admin role
	//
	err = gopher.AssignUserRole(client, r.Log, roleName, userID, serviceProjectID)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.Log.Info("Reconciled User successfully")
	return ctrl.Result{}, nil
}
