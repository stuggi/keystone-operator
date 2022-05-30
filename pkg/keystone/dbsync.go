package keystone

import (
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DbSyncJob func
func DbSyncJob(
	cr *keystonev1.KeystoneAPI,
) *batchv1.Job {
	runAsUser := int64(0)

	labels := map[string]string{
		"app": "keystone-api",
	}

	args := []string{"-c"}
	if cr.Spec.Debug.DBSync {
		args = append(args, DebugCommand)
	} else {
		args = append(args, DBSyncCommand)
	}

	envVars := map[string]common.EnvSetter{}
	envVars["KOLLA_CONFIG_FILE"] = common.EnvValue(KollaConfig)
	envVars["KOLLA_CONFIG_STRATEGY"] = common.EnvValue("COPY_ALWAYS")
	envVars["KOLLA_BOOTSTRAP"] = common.EnvValue("true")

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceName + "-db-sync",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      "OnFailure",
					ServiceAccountName: "keystone-operator-keystone",
					Containers: []corev1.Container{
						{
							Name: ServiceName + "-db-sync",
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

	job.Spec.Template.Spec.Volumes = getVolumes(ServiceName)

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
