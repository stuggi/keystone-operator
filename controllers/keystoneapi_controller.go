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
	helper "github.com/openstack-k8s-operators/lib-common/pkg/helper"
	"github.com/openstack-k8s-operators/lib-common/pkg/job"
	route "github.com/openstack-k8s-operators/lib-common/pkg/route"
	service "github.com/openstack-k8s-operators/lib-common/pkg/service"
	statefulset "github.com/openstack-k8s-operators/lib-common/pkg/statefulset"
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
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;delete;
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

	/*
		// Always patch the instance when exiting this function so we can persist any changes.
		defer func() {
			if err := patchHelper.Patch(ctx, openStackMachine); err != nil {
				if reterr == nil {
					reterr = err
				}
			}
		}()
	*/

	// Handle service delete
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, instance)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, instance, helper)

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
		Owns(&appsv1.StatefulSet{}).
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

func (r *KeystoneAPIReconciler) reconcileInit(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service init")

	//
	// create service DB instance
	//
	db := database.NewDatabase(
		instance.Spec.DatabaseHostname,
		instance.Name,
		instance.Spec.DatabaseUser,
		instance.Spec.Secret,
		map[string]string{
			"dbName": instance.Spec.DatabaseHostname,
		},
	)

	_, ctrlResult, err := db.CreateOrPatchDB(
		ctx,
		helper,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// create service DB - end

	// TODO: wait for DB to be up/created and service user can access it

	//
	// run keystone db sync
	//
	dbSyncHash := instance.Status.Hash[keystonev1.DbSyncHash]
	jobDef := keystone.DbSyncJob(instance)
	dbSyncjob := job.NewJob(
		jobDef,
		keystonev1.DbSyncHash,
		instance.Spec.PreserveJobs,
		5,
		dbSyncHash,
	)
	ctrlResult, err = dbSyncjob.DoJob(
		ctx,
		helper,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	if dbSyncjob.HasChanged() {
		instance.Status.Hash[keystonev1.DbSyncHash] = dbSyncjob.GetHash()
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[keystonev1.DbSyncHash]))
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// run keystone db sync - end

	r.Log.Info("Reconciled Service init successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileUpdate(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service update")

	r.Log.Info("Reconciled Service update successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileUpgrade(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service upgrade")

	r.Log.Info("Reconciled Service upgrade successfully")
	return ctrl.Result{}, nil
}

func (r *KeystoneAPIReconciler) reconcileNormal(ctx context.Context, instance *keystonev1.KeystoneAPI, helper *helper.Helper) (ctrl.Result, error) {
	r.Log.Info("Reconciling Service")

	// If the service object doesn't have our finalizer, add it.
	controllerutil.AddFinalizer(instance, keystonev1.KeystoneFinalizer)
	// Register the finalizer immediately to avoid orphaning resources on delete
	//if err := patchHelper.Patch(ctx, openStackCluster); err != nil {
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	// ConfigMap
	configMapVars := make(map[string]common.EnvSetter)

	//
	// check for required OpenStack secret holding passwords for service/admin user and add hash to the vars map
	//
	ospSecret, hash, err := common.GetSecret(ctx, r, instance.Spec.Secret, instance.Namespace)
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: time.Second * 10}, fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
		}
		return ctrl.Result{}, err
	}
	configMapVars[ospSecret.Name] = common.EnvValue(hash)
	// run check OpenStack secret - end

	//
	// Create ConfigMaps and Secrets required as input for the Service and calculate an overall hash of hashes
	//

	//
	// create Configmap required for keystone input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal keystone config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the OpenStack secret via the init container
	//
	err = r.generateServiceConfigMaps(ctx, instance, &configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// Create secret holding fernet keys
	//
	// TODO use keystone-manage to create / rotate the keys
	err = r.ensureFernetKeys(ctx, instance, &configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// create hash over all the different input resources to identify if any those changed
	// and a restart/recreate is required.
	//
	inputHash, err := r.createHashOfInputHashes(ctx, instance, configMapVars)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Create ConfigMaps and Secrets - end

	//
	// TODO check when/if Init, Update, or Upgrade should/could be skipped
	//

	// Handle service init
	ctrlResult, err := r.reconcileInit(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service update
	ctrlResult, err = r.reconcileUpdate(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	// Handle service upgrade
	ctrlResult, err = r.reconcileUpgrade(ctx, instance, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	//
	// normal reconcile tasks
	//
	endpointLabels := map[string]string{
		"app": "keystone-api",
	}

	// Define a new StatefulSet object
	sfs := statefulset.NewStatefulSet(
		keystone.StatefulSet(instance, inputHash),
		endpointLabels,
		5,
	)

	ctrlResult, err = sfs.CreateOrPatch(ctx, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}

	/*
		if foundDeployment.Status.ReadyReplicas == instance.Spec.Replicas {
			r.Log.Info("Deployment Replicas running:", "Replicas", foundDeployment.Status.ReadyReplicas)
		} else {
			r.Log.Info("Waiting on Keystone Deployment...")
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}
	*/
	// create statefulset - end

	//
	// Create the service if none exists
	//
	var keystonePort int32 = 5000
	svc := service.NewService(
		service.GenericService(&service.GenericServiceDetails{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    endpointLabels,
			Selector:  endpointLabels,
			Port: service.GenericServicePort{
				Name:     "api",
				Port:     keystonePort,
				Protocol: corev1.ProtocolTCP,
			}}),
		endpointLabels,
		5,
	)

	ctrlResult, err = svc.CreateOrPatch(ctx, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// create service - end

	/*
		sh, err := patch.NewHelper(service, r.Client)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = sh.Patch(ctx, service)
		if err != nil {
			return ctrl.Result{}, err
		}
	*/

	// Create the route if none exists
	// TODO TLS
	route := route.NewRoute(
		route.GenericRoute(&route.GenericRouteDetails{
			Name:           instance.Name,
			Namespace:      instance.Namespace,
			Labels:         endpointLabels,
			ServiceName:    keystone.ServiceName,
			TargetPortName: "api",
		}),
		endpointLabels,
		5,
	)

	ctrlResult, err = route.CreateOrPatch(ctx, helper)
	if err != nil {
		return ctrlResult, err
	} else if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// create route - end

	// Look at the generated route to get the value for the initial endpoint
	// FIXME: need to support https default here
	var apiEndpoint string
	if !strings.HasPrefix(route.GetHostname(), "http") {
		apiEndpoint = fmt.Sprintf("http://%s", route.GetHostname())
	} else {
		apiEndpoint = route.GetHostname()
	}
	err = r.setAPIEndpoint(ctx, instance, apiEndpoint)
	if err != nil {
		return ctrl.Result{}, err
	}

	//
	// BootStrap Job
	//
	jobDef := keystone.BootstrapJob(instance, apiEndpoint, apiEndpoint, apiEndpoint)
	bootstrapjob := job.NewJob(
		jobDef,
		keystonev1.BootstrapHash,
		instance.Spec.PreserveJobs,
		5,
		instance.Status.Hash[keystonev1.BootstrapHash],
	)
	ctrlResult, err = bootstrapjob.DoJob(
		ctx,
		helper,
	)
	if err != nil {
		return ctrl.Result{}, err
	}
	if bootstrapjob.HasChanged() {
		instance.Status.Hash[keystonev1.BootstrapHash] = bootstrapjob.GetHash()
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		r.Log.Info(fmt.Sprintf("Job %s hash added - %s", jobDef.Name, instance.Status.Hash[keystonev1.BootstrapHash]))
	}
	if (ctrlResult != ctrl.Result{}) {
		return ctrlResult, nil
	}
	// run keystone bootstrap - end

	err = r.reconcileConfigMap(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.Log.Info("Reconciled Service successfully")
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

func (r *KeystoneAPIReconciler) generateServiceConfigMaps(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	envVars *map[string]common.EnvSetter,
) error {
	//
	// create Configmap/Secret required for keystone input
	// - %-scripts configmap holding scripts to e.g. bootstrap the service
	// - %-config configmap holding minimal keystone config required to get the service up, user can add additional files to be added to the service
	// - parameters which has passwords gets added from the ospSecret via the init container
	//

	// TODO: use common.GetLabels?
	cmLabels := keystone.GetLabels(instance.Name)
	//cmLabels := common.GetLabels(instance.Name, keystone.AppLabel)

	// customData hold any customization for the service.
	// custom.conf is going to /etc/<service>/<service>.conf.d
	// all other files get placed into /etc/<service> to allow overwrite of e.g. logging.conf or policy.json
	// TODO: make sure custom.conf can not be overwritten
	customData := map[string]string{"custom.conf": instance.Spec.CustomServiceConfig}
	for key, data := range instance.Spec.DefaultConfigOverwrite {
		customData[key] = data
	}

	templateParameters := make(map[string]interface{})

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
			Name:          fmt.Sprintf("%s-config-data", instance.Name),
			Namespace:     instance.Namespace,
			Type:          common.TemplateTypeConfig,
			InstanceType:  instance.Kind,
			CustomData:    customData,
			ConfigOptions: templateParameters,
			Labels:        cmLabels,
		},
	}
	_, err := common.EnsureConfigMaps(ctx, r, instance, cms, envVars)
	if err != nil {
		return nil
	}

	/*
		// update Hashes in CR status
		var changed bool
		for k, v := range hashes {
			var hashMap map[string]string
			if hashMap, changed = common.SetHash(instance.Status.Hash, k, v); changed {
				instance.Status.Hash = hashMap
				inputHashes = common.MergeStringMaps(inputHashes, hashMap)
			}
		}
		if changed {
			if err := r.Client.Status().Update(ctx, instance); err != nil {
				return err
			}
		}
	*/

	return nil
}

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

//
// ensureFernetKeys - creates secret with fernet keys
// TODO change to use  keystone-manage fernet_setup && fernet_rotate
func (r *KeystoneAPIReconciler) ensureFernetKeys(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	envVars *map[string]common.EnvSetter,
) error {
	// TODO: use common.GetLabels?
	labels := keystone.GetLabels(instance.Name)

	//
	// check if secret already exist
	//
	secret, hash, err := common.GetSecret(ctx, r, keystone.ServiceName, instance.Namespace)
	if err != nil && !k8s_errors.IsNotFound(err) {
		return err
	} else if k8s_errors.IsNotFound(err) {
		fernetKeys := map[string]string{
			"0": keystone.GenerateFernetKey(),
			"1": keystone.GenerateFernetKey(),
		}

		tmpl := []common.Template{
			{
				Name:       keystone.ServiceName,
				Namespace:  instance.Namespace,
				Type:       common.TemplateTypeNone,
				CustomData: fernetKeys,
				Labels:     labels,
			},
		}
		err := common.EnsureSecrets(ctx, r, instance, tmpl, envVars)
		if err != nil {
			return nil
		}

		return fmt.Errorf("OpenStack secret %s not found", instance.Spec.Secret)
	}

	// TODO: fernet key rotation

	// add hash to envVars
	(*envVars)[secret.Name] = common.EnvValue(hash)

	return nil
}

//
// createHashOfInputHashes - creates a hash of hashes which gets added to the resources which requires a restart
// if any of the input resources change, like configs, passwords, ...
//
func (r *KeystoneAPIReconciler) createHashOfInputHashes(
	ctx context.Context,
	instance *keystonev1.KeystoneAPI,
	envVars map[string]common.EnvSetter,
) (string, error) {
	mergedMapVars := common.MergeEnvs([]corev1.EnvVar{}, envVars)
	hash, err := common.ObjectHash(mergedMapVars)
	if err != nil {
		return hash, err
	}
	if hashMap, changed := common.SetHash(instance.Status.Hash, keystone.InputHashName, hash); changed {
		instance.Status.Hash = hashMap
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			return hash, err
		}
		r.Log.Info(fmt.Sprintf("Input maps hash %s - %s", keystone.InputHashName, hash))
	}
	return hash, nil
}
