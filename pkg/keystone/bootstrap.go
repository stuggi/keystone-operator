package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BootstrapJob func
func BootstrapJob(
	cr *keystonev1beta1.KeystoneAPI,
	adminURL string,
	internalURL string,
	publicURL string,
) *batchv1.Job {
	runAsUser := int64(0)

	args := []string{"-c"}
	if cr.Spec.Debug.Bootstrap {
		args = append(args, DebugCommand)
	} else {
		args = append(args, BootStrapCommand)
	}

	envVars := map[string]common.EnvSetter{}
	envVars["KOLLA_CONFIG_FILE"] = common.EnvValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = common.EnvValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = common.EnvValue("true")
	envVars["OS_BOOTSTRAP_USERNAME"] = common.EnvValue(cr.Spec.AdminUser)
	envVars["OS_BOOTSTRAP_PROJECT_NAME"] = common.EnvValue(cr.Spec.AdminProject)
	envVars["OS_BOOTSTRAP_ROLE_NAME"] = common.EnvValue(cr.Spec.AdminRole)
	envVars["OS_BOOTSTRAP_SERVICE_NAME"] = common.EnvValue(ServiceName)
	envVars["OS_BOOTSTRAP_ADMIN_URL"] = common.EnvValue(adminURL)
	envVars["OS_BOOTSTRAP_PUBLIC_URL"] = common.EnvValue(internalURL)
	envVars["OS_BOOTSTRAP_INTERNAL_URL"] = common.EnvValue(publicURL)
	envVars["OS_BOOTSTRAP_REGION_ID"] = common.EnvValue(cr.Spec.Region)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-bootstrap",
			Namespace: cr.Namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "keystone-operator-keystone",
					Containers: []corev1.Container{
						{
							Name:  ServiceName + "-bootstrap",
							Image: cr.Spec.ContainerImage,
							Command: []string{
								"/bin/bash",
							},
							Args: args,
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: &runAsUser,
							},
							Env: []corev1.EnvVar{
								{
									Name: "OS_BOOTSTRAP_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: cr.Spec.Secret,
											},
											Key: "AdminPassword",
										},
									},
								},
							},
							VolumeMounts: getVolumeMounts(),
						},
					},
				},
			},
		},
	}
	job.Spec.Template.Spec.Containers[0].Env = common.MergeEnvs(job.Spec.Template.Spec.Containers[0].Env, envVars)
	job.Spec.Template.Spec.Volumes = getVolumes(cr.Name)

	initContainerDetails := APIDetails{
		ContainerImage: cr.Spec.ContainerImage,
		DatabaseHost:   cr.Spec.DatabaseHostname,
		DatabaseUser:   cr.Spec.DatabaseUser,
		DatabaseName:   DatabaseName,
		OSPSecret:      cr.Spec.Secret,
		VolumeMounts:   getInitVolumeMounts(),
	}
	job.Spec.Template.Spec.InitContainers = initContainer(initContainerDetails)

	return job
}
