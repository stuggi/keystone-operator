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
	"strings"
	"time"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	keystone "github.com/openstack-k8s-operators/keystone-operator/pkg/keystone"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	database "github.com/openstack-k8s-operators/lib-common/pkg/database"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// GetClient -
func (r *KeystoneAPIReconciler) GetClient() client.Client {
	return r.Client
}

// GetKClient -
func (r *KeystoneAPIReconciler) GetKClient() kubernetes.Interface {
	return r.Kclient
}

// GetLogger -
func (r *KeystoneAPIReconciler) GetLogger() logr.Logger {
	return r.Log
}

// GetScheme -
func (r *KeystoneAPIReconciler) GetScheme() *runtime.Scheme {
	return r.Scheme
}

// KeystoneAPIReconciler reconciles a KeystoneAPI object
type KeystoneAPIReconciler struct {
	client.Client
	Kclient kubernetes.Interface
	Log     logr.Logger
	Scheme  *runtime.Scheme
}

// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=keystone.openstack.org,resources=keystoneapis/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;delete;
// +kubebuilder:rbac:groups=mariadb.openstack.org,resources=mariadbdatabases,verbs=get;list;watch;create;update;delete;

// Reconcile reconcile keystone API requests
func (r *KeystoneAPIReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("keystoneapi", req.NamespacedName)

	// Fetch the KeystoneAPI instance
	instance := &keystonev1.KeystoneAPI{}
	err := r.Client.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected.
			// For additional cleanup logic use finalizers. Return and don't requeue.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance)

}

// SetupWithManager x
func (r *KeystoneAPIReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&keystonev1.KeystoneAPI{}).
		Owns(&mariadbv1.MariaDBDatabase{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&routev1.Route{}).
		Complete(r)
}

func (r *KeystoneAPIReconciler) reconcileDelete(ctx context.Context, instance *keystonev1.KeystoneAPI) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service delete")

	// Service is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(instance, keystonev1.KeystoneFinalizer)
	r.Log.Info("Reconciled Service delete successfully")
	//if err := patchHelper.Patch(ctx, openStackCluster); err != nil {
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileInit(ctx context.Context, instance *keystonev1.KeystoneAPI) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service init")

	dbOptions := database.Options{
		DatabaseHostname: instance.Spec.DatabaseHostname,
		DatabaseName:     instance.Name,
		Secret:           instance.Spec.Secret,
		Labels: map[string]string{
			"dbName": instance.Spec.DatabaseHostname,
		},
	}

	//
	// create service DB instance
	//
	_, _, ctrlResult, err := database.CreateOrPatchDB(
		ctx,
		r.Client,
		instance,
		r.GetScheme(),
		dbOptions,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// TODO: wait for DB to be up

	//
	// run keystone bootstrap
	//
	job, err := keystone.DbSyncJob(ctx, r.GetClient(), instance)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting dbSyncJob: %v", err)
	}
	dbSyncHash, err := common.ObjectHash(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating DB sync hash: %v", err)
	}

	//
	// run dbsync job if there is no stored DbSyncHash in the object status, or the hash
	// does not match and there is the need to rerun the job
	//
	if hash, ok := instance.Status.Hash[keystonev1.DbSyncHash]; !ok || (ok && dbSyncHash != hash) {
		r.Log.Info(fmt.Sprintf("BOOOO dbSyncHash %s %+v", dbSyncHash, job))
		r.Log.Info(fmt.Sprintf("BOOOO hash %s", hash))

		op, err := controllerutil.CreateOrPatch(ctx, r.Client, job, func() error {
			err := controllerutil.SetControllerReference(instance, job, r.Scheme)
			if err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			if k8s_errors.IsNotFound(err) {
				r.Log.Info(fmt.Sprintf("DBSync job %s not found, reconcile in 5s", job.Name))
				return ctrl.Result{RequeueAfter: time.Second * 5}, nil
			}
			return ctrl.Result{}, err
		}
		if op != controllerutil.OperationResultNone {
			// TODO: condition ?
			r.Log.Info(fmt.Sprintf("DBSync job %s - %s", job.Name, op))
		}

		requeue, err := common.WaitOnJob(ctx, job, r.Client, r.Log)
		r.Log.Info("Running DB Sync")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on DB sync")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// db sync completed... okay to store the hash to disable it
	r.Log.Info(fmt.Sprintf("Writing DB Sync hash %s to status", dbSyncHash))
	if ctrlResult, err := r.setHash(ctx, instance, keystonev1.DbSyncHash, dbSyncHash); err != nil {
		return ctrl.Result{}, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// delete the job if debug is not enabled
	if !instance.Spec.Debug.DBSync {
		_, err = common.DeleteJob(ctx, job, r.Kclient, r.Log)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	r.Log.Info("Reconciled Service init successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileUpdate(ctx context.Context, instance *keystonev1.KeystoneAPI) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service update")

	r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileUpgrade(ctx context.Context, instance *keystonev1.KeystoneAPI) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service upgrade")

	r.Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileNormal(ctx context.Context, instance *keystonev1.KeystoneAPI) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, keystonev1.KeystoneFinalizer)
	// Register the finalizer immediately to avoid orphaning resources on delete
	//if err := patchHelper.Patch(ctx, openStackCluster); err != nil {
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	envVars := make(map[string]common.EnvSetter)
	inputHashes := make(map[string]string)

	//
	// check for required OpenStack secret holding passwords for service/admin user
	//
	_, hash, err := common.GetSecret(ctx, r, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
		}
		return ctrl.Result{}, err
	}
	inputHashes[instance.Spec.Secret] = hash

	//
	// create Configmap/Secret required for keystone input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal keystone config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//
	cmLabels := map[string]string{}
	//cmLabels := common.GetLabels(instance.Name, nova.AppLabel)
	//cmLabels["upper-cr"] = instance.Name

	cms := []common.Template{
		// ScriptsConfigMap
		{
			Name:               fmt.Sprintf("%s-scripts", instance.Name),
			Namespace:          instance.Namespace,
			Type:               common.TemplateTypeScripts,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{"common.sh": "/common/common.sh"},
			Labels:             cmLabels,
		},
		// ConfigMap
		{
			Name:               fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:          instance.Namespace,
			Type:               common.TemplateTypeConfig,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{},
			Labels:             cmLabels,
		},
	}
	hashes, err := common.EnsureConfigMaps(ctx, r, instance, cms, &envVars)
	if err != nil {
		return ctrl.Result{}, nil
	}

	// update Hashes in CR status
	for k, v := range hashes {
		if ctrlResult, err := r.setHash(ctx, instance, k, v); err != nil {
			return ctrl.Result{}, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}
	}

	// Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      keystone.ServiceName,
			Namespace: instance.Namespace,
		},
		Type: "Opaque",
	}
	fernetToken := map[string]string{}

	// Check if this Secret already exists
	r.Log.Info(fmt.Sprintf("Create keystone secret %s", secret.Name))
	foundSecret := &corev1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, foundSecret)
	if err != nil && k8s_errors.IsNotFound(err) {
		fernetToken = map[string]string{
			"0": keystone.GenerateFernetKey(),
			"1": keystone.GenerateFernetKey(),
		}

	} else if err != nil {
		// TODO error conditions
		return ctrl.Result{}, err
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, secret, func() error {
		// TODO Labels
		secret.StringData = fernetToken

		err := controllerutil.SetControllerReference(instance, secret, r.Scheme)
		if err != nil {
			// TODO error conditions
			return err
		}

		return nil
	})
	if err != nil && !k8s_errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		// TODO: conditions
		r.Log.Info(fmt.Sprintf("Create keyston secret %s - %s", secret.Name, op))
	}

	// TODO add fernet hash to status

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//

	// ConfigMap
	configMapVars := make(map[string]common.EnvSetter)
	//err = r.generateServiceConfigMaps(ctx, instance, &configMapVars)
	//if err != nil {
	//	return ctrl.Result{}, err
	//}
	mergedMapVars := common.MergeEnvs([]corev1.EnvVar{}, configMapVars)
	configHash := ""
	for _, hashEnv := range mergedMapVars {
		configHash = configHash + hashEnv.Value
	}

	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating configmap hash: %v", err)
	}

	// Define a new Deployment object
	r.Log.Info("ConfigMapHash: ", "Data Hash:", configHash)
	deployment, err := keystone.Deployment(instance, configHash)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating deployment: %v", err)
	}
	deploymentHash, err := common.ObjectHash(deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error deployment hash: %v", err)
	}
	r.Log.Info("DeploymentHash: ", "Deployment Hash:", deploymentHash)

	// Check if this Deployment already exists
	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, foundDeployment)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Client.Create(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Set KeystoneAPI instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, err

	} else if err != nil {
		return ctrl.Result{}, err
	} else {

		if hash, ok := instance.Status.Hash[keystonev1.DeploymentHash]; !ok || hash != deploymentHash {
			r.Log.Info("Deployment Updated")
			foundDeployment.Spec = deployment.Spec
			err = r.Client.Update(ctx, foundDeployment)
			if err != nil {
				return ctrl.Result{}, err
			}
			// db sync completed... okay to store the hash to disable it
			if ctrlResult, err := r.setHash(ctx, instance, keystonev1.DeploymentHash, deploymentHash); err != nil {
				return ctrl.Result{}, err
			} else if (ctrlResult != ctrl.Result{}) {
				return ctrlResult, nil
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, err
		}
		if foundDeployment.Status.ReadyReplicas == instance.Spec.Replicas {
			r.Log.Info("Deployment Replicas running:", "Replicas", foundDeployment.Status.ReadyReplicas)
		} else {
			r.Log.Info("Waiting on Keystone Deployment...")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	}

	// Create the service if none exists
	var keystonePort int32 = 5000
	service := keystone.Service(instance, instance.Name, keystonePort)

	// Check if this Service already exists
	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, foundService)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Client.Create(ctx, service)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Set KeystoneAPI instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Create the route if none exists
	route := keystone.Route(instance, instance.Name)

	// Check if this Route already exists
	foundRoute := &routev1.Route{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: route.Name, Namespace: route.Namespace}, foundRoute)
	if err != nil && k8s_errors.IsNotFound(err) {
		r.Log.Info("Creating a new Route", "Route.Namespace", route.Namespace, "Route.Name", route.Name)
		err = r.Client.Create(ctx, route)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Set Keystone instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, route, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Look at the generated route to get the value for the initial endpoint
	// FIXME: need to support https default here
	var apiEndpoint string
	if !strings.HasPrefix(foundRoute.Spec.Host, "http") {
		apiEndpoint = fmt.Sprintf("http://%s", foundRoute.Spec.Host)
	} else {
		apiEndpoint = foundRoute.Spec.Host
	}
	err = r.setAPIEndpoint(ctx, instance, apiEndpoint)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Define a new BootStrap Job object
	bootstrapJob, err := keystone.BootstrapJob(instance, apiEndpoint, apiEndpoint, apiEndpoint)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error creating bootstrap job: %v", err)
	}
	bootstrapHash, err := common.ObjectHash(bootstrapJob)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error calculating bootstrap hash: %v", err)
	}

	// Set KeystoneAPI instance as the owner and controller
	if hash, ok := instance.Status.Hash[keystonev1.BootstrapHash]; !ok || hash != deploymentHash {
		op, err := controllerutil.CreateOrPatch(ctx, r.Client, bootstrapJob, func() error {
			err := controllerutil.SetControllerReference(instance, bootstrapJob, r.Scheme)
			if err != nil {
				// FIXME error conditions
				return err
			}

			return nil
		})
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if op != controllerutil.OperationResultNone {
			// FIXME: error conditions
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}

		requeue, err := common.WaitOnJob(ctx, bootstrapJob, r.Client, r.Log)
		r.Log.Info("Running keystone bootstrap")
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			r.Log.Info("Waiting on keystone bootstrap")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// db sync completed... okay to store the hash to disable it
		if ctrlResult, err := r.setHash(ctx, instance, keystonev1.BootstrapHash, bootstrapHash); err != nil {
			return ctrl.Result{}, err
		} else if (ctrlResult != ctrl.Result{}) {
			return ctrlResult, nil
		}

	}

	// delete the job if debug is not enabled
	if !instance.Spec.Debug.Bootstrap {
		_, err = common.DeleteJob(ctx, bootstrapJob, r.Kclient, r.Log)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.reconcileConfigMap(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.Log.Info("Reconciled Service successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) setHash(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	hashType string,
	hashStr string,
) (ctrl.Result, error) {

	// patch hash map if it is different from what is stored in status
	if hash, ok := instance.Status.Hash[hashType]; !ok || hash != hashStr {
		op, err := controllerutil.CreateOrPatch(ctx, r.Client, instance, func() error {
			if instance.Status.Hash == nil {
				instance.Status.Hash = make(map[string]string)
			}
			instance.Status.Hash[hashType] = hashStr

			return nil
		})
		if err != nil && !k8s_errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if op != controllerutil.OperationResultNone {
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
	}

	return ctrl.Result{}, nil
}

// setAPIEndpoint func
func (r *KeystoneAPIReconciler) setAPIEndpoint(ctx context.Context, instance *keystonev1.KeystoneAPI, apiEndpoint string) error {

	if apiEndpoint != instance.Status.APIEndpoint {
		instance.Status.APIEndpoint = apiEndpoint
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil

}

/*
func (r *KeystoneAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	envVars *map[string]common.EnvSetter,
) error {
	// FIXME: use common.GetLabels?
	cmLabels := keystone.GetLabels(instance.Name)
	templateParameters := make(map[string]interface{})

	// ConfigMaps for keystoneapi
	cms := []common.Template{
		// ScriptsConfigMap
		{
			Name:               "keystone-" + instance.Name,
			Namespace:          instance.Namespace,
			Type:               common.TemplateTypeConfig,
			InstanceType:       instance.Kind,
			AdditionalTemplate: map[string]string{},
			ConfigOptions:      templateParameters,
			Labels:             cmLabels,
		},
	}

	err := common.EnsureConfigMaps(ctx, r, instance, cms, envVars)

	if err != nil {
		// FIXME error conditions here
		return err
	}

	return nil
}
*/
func (r *KeystoneAPIReconciler) reconcileConfigMap(ctx context.Context, instance *keystonev1.KeystoneAPI) error {

	configMapName := "openstack-config"
	var openStackConfig keystone.OpenStackConfig
	openStackConfig.Clouds.Default.Auth.AuthURL = instance.Status.APIEndpoint
	openStackConfig.Clouds.Default.Auth.ProjectName = "admin"
	openStackConfig.Clouds.Default.Auth.UserName = "admin"
	openStackConfig.Clouds.Default.Auth.UserDomainName = "Default"
	openStackConfig.Clouds.Default.Auth.ProjectDomainName = "Default"
	openStackConfig.Clouds.Default.RegionName = "regionOne"

	cloudsYamlVal, err := yaml.Marshal(&openStackConfig)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
		},
	}

	r.Log.Info("Reconciling ConfigMap", "ConfigMap.Namespace", instance.Namespace, "configMap.Name", configMapName)
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.TypeMeta = metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		}
		cm.ObjectMeta = metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
		}
		cm.Data = map[string]string{
			"clouds.yaml": string(cloudsYamlVal),
			"OS_CLOUD":    "default",
		}
		return nil
	})
	if err != nil {
		return err
	}

	keystoneSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Spec.Secret,
			Namespace: instance.Namespace,
		},
		Type: "Opaque",
	}

	err = r.Client.Get(ctx, types.NamespacedName{Name: keystoneSecret.Name, Namespace: instance.Namespace}, keystoneSecret)
	if err != nil {
		return err
	}

	secretName := "openstack-config-secret"
	var openStackConfigSecret keystone.OpenStackConfigSecret
	openStackConfigSecret.Clouds.Default.Auth.Password = string(keystoneSecret.Data["AdminPassword"])

	secretVal, err := yaml.Marshal(&openStackConfigSecret)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		},
	}
	if err != nil {
		return err
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.TypeMeta = metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		}
		secret.ObjectMeta = metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.Namespace,
		}
		secret.StringData = map[string]string{
			"secure.yaml": string(secretVal),
		}
		return nil
	})

	return err
}
