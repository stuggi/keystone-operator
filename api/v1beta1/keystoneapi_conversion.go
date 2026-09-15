/*
Copyright 2025.

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

package v1beta1

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	keystonev1beta2 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta2"
)

// This file makes KeystoneAPI v1beta1 a conversion "spoke": it converts to and
// from the v1beta2 "hub" (storage) version.
//
// v1beta2 is currently a no-op bump - its schema is identical to v1beta1 - so
// these conversions are lossless field-by-field copies. They are written out
// explicitly (rather than using unsafe pointer casts) on purpose: this file is
// the reference template other operators copy, and once v1beta2 diverges from
// v1beta1 the per-field mapping is exactly where the version-specific handling
// belongs.
//
// Fields whose Go type comes from another module (e.g. tls.API,
// topologyv1.TopoRef, rabbitmqv1.RabbitMqConfig, corev1.ResourceRequirements,
// condition.Conditions and the service.* override maps) are shared between the
// two versions, so they are assigned directly.
//
// Reference-type fields (ObjectMeta and its maps, pointers, slices and maps in
// the spec/status) are shallow-copied: src and dst share the same backing
// memory after conversion. That is safe here because controller-runtime
// discards the source object once conversion returns - nothing mutates it
// afterwards. Operators copying this template that mutate a converted object in
// place (e.g. a defaulter that runs post-conversion) must deep-copy the
// affected field first, otherwise they would corrupt the other version's object.

// ConvertTo converts this KeystoneAPI (v1beta1, spoke) to the Hub version (v1beta2).
func (src *KeystoneAPI) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*keystonev1beta2.KeystoneAPI)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = keystonev1beta2.KeystoneAPISpec{
		ContainerImage:      src.Spec.ContainerImage,
		KeystoneAPISpecCore: SpecCoreToV1beta2(src.Spec.KeystoneAPISpecCore),
	}
	dst.Status = statusToV1beta2(src.Status)

	return nil
}

// ConvertFrom converts from the Hub version (v1beta2) to this KeystoneAPI (v1beta1, spoke).
func (dst *KeystoneAPI) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*keystonev1beta2.KeystoneAPI)

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = KeystoneAPISpec{
		ContainerImage:      src.Spec.ContainerImage,
		KeystoneAPISpecCore: SpecCoreFromV1beta2(src.Spec.KeystoneAPISpecCore),
	}
	dst.Status = statusFromV1beta2(src.Status)

	return nil
}

// SpecCoreToV1beta2 converts the shared KeystoneAPISpecCore from v1beta1 to
// v1beta2. It is exported because KeystoneAPISpecCore is the unit bubbled up
// into OpenStackControlPlane (spec.keystone.template); openstack-operator calls
// this from its OpenStackControlPlane conversion so the field-by-field mapping
// lives once here in the service operator, the same way defaulting/validation
// are shared. The object-level ConvertTo above is a thin wrapper around it.
func SpecCoreToV1beta2(in KeystoneAPISpecCore) keystonev1beta2.KeystoneAPISpecCore {
	return keystonev1beta2.KeystoneAPISpecCore{
		DatabaseInstance:       in.DatabaseInstance,
		DatabaseAccount:        in.DatabaseAccount,
		MemcachedInstance:      in.MemcachedInstance,
		Region:                 in.Region,
		AdminProject:           in.AdminProject,
		AdminUser:              in.AdminUser,
		Replicas:               in.Replicas,
		Secret:                 in.Secret,
		EnableSecureRBAC:       in.EnableSecureRBAC,
		TrustFlushArgs:         in.TrustFlushArgs,
		TrustFlushSchedule:     in.TrustFlushSchedule,
		TrustFlushSuspend:      in.TrustFlushSuspend,
		FernetRotationDays:     in.FernetRotationDays,
		FernetMaxActiveKeys:    in.FernetMaxActiveKeys,
		PasswordSelectors:      keystonev1beta2.PasswordSelector{Admin: in.PasswordSelectors.Admin},
		NodeSelector:           in.NodeSelector,
		PreserveJobs:           in.PreserveJobs,
		CustomServiceConfig:    in.CustomServiceConfig,
		DefaultConfigOverwrite: in.DefaultConfigOverwrite,
		HttpdCustomization: keystonev1beta2.HttpdCustomization{
			ProcessNumber:      in.HttpdCustomization.ProcessNumber,
			CustomConfigSecret: in.HttpdCustomization.CustomConfigSecret,
		},
		Resources:           in.Resources,
		NetworkAttachments:  in.NetworkAttachments,
		Override:            keystonev1beta2.APIOverrideSpec{Service: in.Override.Service},
		RabbitMqClusterName: in.RabbitMqClusterName,
		TLS:                 in.TLS,
		APITimeout:          in.APITimeout,
		TopologyRef:         in.TopologyRef,
		ExtraMounts:         extraMountsToV1beta2(in.ExtraMounts),
		FederatedRealmConfig: in.FederatedRealmConfig,
		ExternalKeystoneAPI:  in.ExternalKeystoneAPI,
		NotificationsBus:     in.NotificationsBus,
	}
}

// SpecCoreFromV1beta2 converts the shared KeystoneAPISpecCore from v1beta2 back
// to v1beta1. See SpecCoreToV1beta2 for why it is exported.
func SpecCoreFromV1beta2(in keystonev1beta2.KeystoneAPISpecCore) KeystoneAPISpecCore {
	return KeystoneAPISpecCore{
		DatabaseInstance:       in.DatabaseInstance,
		DatabaseAccount:        in.DatabaseAccount,
		MemcachedInstance:      in.MemcachedInstance,
		Region:                 in.Region,
		AdminProject:           in.AdminProject,
		AdminUser:              in.AdminUser,
		Replicas:               in.Replicas,
		Secret:                 in.Secret,
		EnableSecureRBAC:       in.EnableSecureRBAC,
		TrustFlushArgs:         in.TrustFlushArgs,
		TrustFlushSchedule:     in.TrustFlushSchedule,
		TrustFlushSuspend:      in.TrustFlushSuspend,
		FernetRotationDays:     in.FernetRotationDays,
		FernetMaxActiveKeys:    in.FernetMaxActiveKeys,
		PasswordSelectors:      PasswordSelector{Admin: in.PasswordSelectors.Admin},
		NodeSelector:           in.NodeSelector,
		PreserveJobs:           in.PreserveJobs,
		CustomServiceConfig:    in.CustomServiceConfig,
		DefaultConfigOverwrite: in.DefaultConfigOverwrite,
		HttpdCustomization: HttpdCustomization{
			ProcessNumber:      in.HttpdCustomization.ProcessNumber,
			CustomConfigSecret: in.HttpdCustomization.CustomConfigSecret,
		},
		Resources:           in.Resources,
		NetworkAttachments:  in.NetworkAttachments,
		Override:            APIOverrideSpec{Service: in.Override.Service},
		RabbitMqClusterName: in.RabbitMqClusterName,
		TLS:                 in.TLS,
		APITimeout:          in.APITimeout,
		TopologyRef:         in.TopologyRef,
		ExtraMounts:         extraMountsFromV1beta2(in.ExtraMounts),
		FederatedRealmConfig: in.FederatedRealmConfig,
		ExternalKeystoneAPI:  in.ExternalKeystoneAPI,
		NotificationsBus:     in.NotificationsBus,
	}
}

func extraMountsToV1beta2(in []KeystoneExtraMounts) []keystonev1beta2.KeystoneExtraMounts {
	if in == nil {
		return nil
	}
	out := make([]keystonev1beta2.KeystoneExtraMounts, len(in))
	for i, m := range in {
		out[i] = keystonev1beta2.KeystoneExtraMounts{
			Name:      m.Name,
			Region:    m.Region,
			VolMounts: m.VolMounts,
		}
	}
	return out
}

func extraMountsFromV1beta2(in []keystonev1beta2.KeystoneExtraMounts) []KeystoneExtraMounts {
	if in == nil {
		return nil
	}
	out := make([]KeystoneExtraMounts, len(in))
	for i, m := range in {
		out[i] = KeystoneExtraMounts{
			Name:      m.Name,
			Region:    m.Region,
			VolMounts: m.VolMounts,
		}
	}
	return out
}

func statusToV1beta2(in KeystoneAPIStatus) keystonev1beta2.KeystoneAPIStatus {
	return keystonev1beta2.KeystoneAPIStatus{
		ReadyCount:          in.ReadyCount,
		Hash:                in.Hash,
		APIEndpoints:        in.APIEndpoints,
		Conditions:          in.Conditions,
		DatabaseHostname:    in.DatabaseHostname,
		NetworkAttachments:  in.NetworkAttachments,
		TransportURLSecret:  in.TransportURLSecret,
		ObservedGeneration:  in.ObservedGeneration,
		LastAppliedTopology: in.LastAppliedTopology,
		Region:              in.Region,
	}
}

func statusFromV1beta2(in keystonev1beta2.KeystoneAPIStatus) KeystoneAPIStatus {
	return KeystoneAPIStatus{
		ReadyCount:          in.ReadyCount,
		Hash:                in.Hash,
		APIEndpoints:        in.APIEndpoints,
		Conditions:          in.Conditions,
		DatabaseHostname:    in.DatabaseHostname,
		NetworkAttachments:  in.NetworkAttachments,
		TransportURLSecret:  in.TransportURLSecret,
		ObservedGeneration:  in.ObservedGeneration,
		LastAppliedTopology: in.LastAppliedTopology,
		Region:              in.Region,
	}
}
