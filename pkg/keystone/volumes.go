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

package keystone

import (
	corev1 "k8s.io/api/core/v1"
)

// getVolumes - service volumes
func getVolumes(
	name string,
	caList []string,
) []corev1.Volume {
	var scriptsVolumeDefaultMode int32 = 0755
	var config0640AccessMode int32 = 0640

	volumes := []corev1.Volume{
		{
			Name: "scripts",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-scripts",
					},
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					DefaultMode: &config0640AccessMode,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name + "-config-data",
					},
				},
			},
		},
		{
			Name: "config-data-merged",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
		{
			Name: "fernet-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ServiceName,
					Items: []corev1.KeyToPath{
						{
							Key:  "FernetKeys0",
							Path: "0",
						},
						{
							Key:  "FernetKeys1",
							Path: "1",
						},
					},
				},
			},
		},
		{
			Name: "credential-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: ServiceName,
					Items: []corev1.KeyToPath{
						{
							Key:  "CredentialKeys0",
							Path: "0",
						},
						{
							Key:  "CredentialKeys1",
							Path: "1",
						},
					},
				},
			},
		},
	}

	for _, secret := range caList {
		volumes = append(volumes, corev1.Volume{
			Name: secret + "-ca",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secret,
					Items: []corev1.KeyToPath{
						{
							Key:  "ca.crt",
							Path: secret + "-ca.crt",
						},
					},
				},
			},
		})
	}

	return volumes
}

// getInitVolumeMounts - general init task VolumeMounts
func getInitVolumeMounts(
	caList []string,
) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/var/lib/config-data/default",
			ReadOnly:  true,
		},
		{
			Name:      "config-data-merged",
			MountPath: "/var/lib/config-data/merged",
			ReadOnly:  false,
		},
	}

	for _, secret := range caList {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			MountPath: "/etc/pki/ca-trust/source/anchors/" + secret + "-ca.crt",
			SubPath:   secret + "-ca.crt",
			ReadOnly:  true,
			Name:      secret + "-ca",
		})
	}

	return volumeMounts
}

// getVolumeMounts - general VolumeMounts
func getVolumeMounts(
	caList []string,
) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "config-data-merged",
			MountPath: "/var/lib/config-data/merged",
			ReadOnly:  false,
		},
		{
			MountPath: "/var/lib/fernet-keys",
			ReadOnly:  true,
			Name:      "fernet-keys",
		},
		{
			MountPath: "/var/lib/credential-keys",
			ReadOnly:  true,
			Name:      "credential-keys",
		},
	}

	for _, secret := range caList {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			MountPath: "/etc/pki/ca-trust/source/anchors/" + secret + "-ca.crt",
			SubPath:   secret + "-ca.crt",
			ReadOnly:  true,
			Name:      secret + "-ca",
		})
	}

	return volumeMounts
}
