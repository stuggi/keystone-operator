package keystone

import (
	common "github.com/openstack-k8s-operators/lib-common/pkg/common"

	corev1 "k8s.io/api/core/v1"
)

// APIDetails information
type APIDetails struct {
	ContainerImage string
	DatabaseHost   string
	DatabaseUser   string
	DatabaseName   string
	OSPSecret      string
	VolumeMounts   []corev1.VolumeMount
}

// initContainer - init container for keystone api pods
func initContainer(init APIDetails) []corev1.Container {
	runAsUser := int64(0)

	args := []string{
		"-c",
		InitContainerCommand,
	}

	envVars := map[string]common.EnvSetter{}
	envVars["DatabaseHost"] = common.EnvValue(init.DatabaseHost)
	envVars["DatabaseUser"] = common.EnvValue(init.DatabaseUser)
	envVars["DatabaseName"] = common.EnvValue(init.DatabaseName)

	envs := []corev1.EnvVar{
		{
			Name: "DatabasePassword",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: init.OSPSecret,
					},
					Key: KeystoneDatabasePassword,
				},
			},
		},
		{
			Name: "AdminPassword",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: init.OSPSecret,
					},
					Key: AdminPassword,
				},
			},
		},
	}
	envs = common.MergeEnvs(envs, envVars)

	return []corev1.Container{
		{
			Name:  "init",
			Image: init.ContainerImage,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser: &runAsUser,
			},
			Command: []string{
				"/bin/bash",
			},
			Args:         args,
			Env:          envs,
			VolumeMounts: getInitVolumeMounts(),
		},
	}
}
