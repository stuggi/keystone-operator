package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deployment func
func Deployment(
	cr *keystonev1beta1.KeystoneAPI,
	configHash string,
) (*appsv1.Deployment, error) {
	runAsUser := int64(0)

	labels := map[string]string{
		"app": "keystone-api",
	}

	// validate if debug is enabled for this step
	args := []string{"-c"}
	if cr.Spec.Debug.Service {
		args = append(args, DebugCommand)
	} else {
		args = append(args, ServiceCommand)
	}

	envVars := map[string]common.EnvSetter{}
	envVars["KOLLA_CONFIG_FILE"] = common.EnvValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = common.EnvValue("COPY_ALWAYS")
	envVars["CONFIG_HASH"] = common.EnvValue(configHash)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName,
			Namespace: cr.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Replicas: &cr.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "keystone-operator-keystone",
					Containers: []corev1.Container{
						{
							Name: ServiceName + "-api",
							Command: []string{
								"/bin/bash",
							},
							Args:  args,
							Image: cr.Spec.ContainerImage,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env:          common.MergeEnvs([]corev1.EnvVar{}, envVars),
							VolumeMounts: getVolumeMounts(),
						},
					},
				},
			},
		},
	}
	deployment.Spec.Template.Spec.Volumes = getVolumes(cr.Name)

	initContainerDetails := APIDetails{
		ContainerImage: cr.Spec.ContainerImage,
		DatabaseHost:   cr.Spec.DatabaseHostname,
		DatabaseUser:   cr.Spec.DatabaseUser,
		DatabaseName:   DatabaseName,
		OSPSecret:      cr.Spec.Secret,
		VolumeMounts:   getInitVolumeMounts(),
	}
	deployment.Spec.Template.Spec.InitContainers = initContainer(initContainerDetails)

	return deployment, nil
}
