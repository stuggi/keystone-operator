package keystone

import (
	keystonev1beta1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
)

// ComputeSecurityHash computes a hash of security-critical spec fields
// (roles, accessRules, unrestricted). Used to detect changes that require immediate rotation.
func ComputeSecurityHash(spec keystonev1beta1.KeystoneApplicationCredentialSpec) (string, error) {
	securityFields := struct {
		Roles        []string
		AccessRules  []keystonev1beta1.ACRule
		Unrestricted bool
	}{
		Roles:        spec.Roles,
		AccessRules:  spec.AccessRules,
		Unrestricted: spec.Unrestricted,
	}
	return util.ObjectHash(securityFields)
}
