#!/bin//bash
#
# Copyright 2020 Red Hat Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may
# not use this file except in compliance with the License. You may obtain
# a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations
# under the License.
set -ex

export USERNAME=${AdminUser:-admin}
export PASSWORD=${AdminPassword:?"Please specify a AdminPassword variable."}
export PROJECT=${Project:-"admin"}
export ROLE=${Role:-"admin"}
export ADMIN_URL=${AdminUrl:?"Please specify the AdminUrl variable."}
export INTERNAL_URL=${InternalUrl:?"Please specify the AdminUrl variable."}
export PUBLIC_URL=${PublicUrl:?"Please specify the AdminUrl variable."}
export REGION=${Region:-"regionOne"}

# Bootstrap and exit if KOLLA_BOOTSTRAP variable is set. This catches all cases
# of the KOLLA_BOOTSTRAP variable being set, including empty.
if [[ "${!KOLLA_BOOTSTRAP[@]}" ]]; then
    keystone_bootstrap=$(keystone-manage bootstrap \
        --bootstrap-username "${USERNAME}" \
        --bootstrap-password "${PASSWORD}" \
        --bootstrap-project-name "${PROJECT}" \
        --bootstrap-role-name "${ROLE}" \
        --bootstrap-admin-url "${ADMIN_URL}" \
        --bootstrap-internal-url "${INTERNAL_URL}" \
        --bootstrap-public-url "${PUBLIC_URL}" \
        --bootstrap-service-name "keystone" \
        --bootstrap-region-id "${REGION}" 2>&1 | cat -v | sed 's/\\/\\\\/g' | sed 's/"/\\"/g')
    if [[ $? != 0 ]]; then
        print "${keystone_bootstrap}"
        exit $?
    fi

    changed=$(echo "${keystone_bootstrap}" | awk '
        /Domain default already exists, skipping creation./ ||
        /Project '"${PROJECT}"' already exists, skipping creation./ ||
        /User '"${USERNAME}"' already exists, skipping creation./ ||
        /Role '"${ROLE}"' exists, skipping creation./ ||
        /User '"${USERNAME}"' already has '"${ROLE}"' on '"${PROJECT}"'./ ||
        /Region '"${REGION}"' exists, skipping creation./ ||
        /Skipping admin endpoint as already created/ ||
        /Skipping internal endpoint as already created/ ||
        /Skipping public endpoint as already created/ {count++}
        END {
            if (count == 9) changed="false"; else changed="true"
            print changed
        }'
    )
    exit 0
fi

if [[ "${!KOLLA_UPGRADE[@]}" ]]; then
    exit 0
fi

if [[ "${!KOLLA_OSM[@]}" ]]; then
    exit 0
fi


