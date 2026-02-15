#!/bin/bash
# Tears down the IONOS Cloud datacenter and all VMs.
# WARNING: This permanently deletes all resources.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../../.env"

export IONOS_TOKEN

DATACENTER_NAME="serverless5gc-eval"

DC_ID=$(ionosctl datacenter list --no-headers -o json | \
    jq -r ".items[] | select(.properties.name==\"${DATACENTER_NAME}\") | .id")

if [ -n "$DC_ID" ]; then
    echo "Found datacenter: ${DATACENTER_NAME} (${DC_ID})"
    echo "This will permanently delete the datacenter and ALL resources within it."
    read -p "Are you sure? (yes/no): " CONFIRM
    if [ "$CONFIRM" = "yes" ]; then
        echo "Deleting datacenter ${DC_ID}..."
        ionosctl datacenter delete --datacenter-id "$DC_ID" --wait-for-request --force
        echo "Datacenter deleted."
    else
        echo "Aborted."
    fi
else
    echo "Datacenter '${DATACENTER_NAME}' not found."
fi
