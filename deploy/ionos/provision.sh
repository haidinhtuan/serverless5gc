#!/bin/bash
# Provisions 5 VMs on IONOS Cloud for the serverless 5GC evaluation.
# Requires ionosctl CLI and IONOS_TOKEN set in .env.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/../../.env"

export IONOS_TOKEN

DATACENTER_NAME="serverless5gc-eval"
LOCATION="de/fra"
IMAGE_ALIAS="ubuntu:latest"
SSH_KEY_PATH="${SSH_KEY_PATH:-$HOME/.ssh/id_rsa.pub}"

echo "=== Provisioning IONOS Cloud datacenter: ${DATACENTER_NAME} ==="

# Create datacenter.
echo "Creating datacenter in ${LOCATION}..."
ionosctl datacenter create \
    --name "$DATACENTER_NAME" \
    --location "$LOCATION" \
    --wait-for-request

DC_ID=$(ionosctl datacenter list --no-headers -o json | \
    jq -r ".items[] | select(.properties.name==\"${DATACENTER_NAME}\") | .id")
echo "Datacenter ID: ${DC_ID}"

# Create LAN.
echo "Creating LAN..."
ionosctl lan create \
    --datacenter-id "$DC_ID" \
    --name "internal" \
    --public=false \
    --wait-for-request

LAN_ID=$(ionosctl lan list --datacenter-id "$DC_ID" --no-headers -o json | \
    jq -r '.items[0].id')
echo "LAN ID: ${LAN_ID}"

# VM definitions: name:cores:ram_mb
VMS=(
    "serverless5gc:8:16384"
    "open5gs:8:16384"
    "free5gc:8:16384"
    "loadgen:4:8192"
    "monitoring:4:8192"
)

for VM_DEF in "${VMS[@]}"; do
    IFS=':' read -r VM_NAME CORES RAM <<< "$VM_DEF"
    echo ""
    echo "Creating VM: ${VM_NAME} (${CORES} vCPU, $((RAM / 1024)) GB RAM)..."

    ionosctl server create \
        --datacenter-id "$DC_ID" \
        --name "$VM_NAME" \
        --cores "$CORES" \
        --ram "$RAM" \
        --image-alias "$IMAGE_ALIAS" \
        --ssh-key-path "$SSH_KEY_PATH" \
        --wait-for-request

    SERVER_ID=$(ionosctl server list --datacenter-id "$DC_ID" --no-headers -o json | \
        jq -r ".items[] | select(.properties.name==\"${VM_NAME}\") | .id")

    echo "  Server ID: ${SERVER_ID}"

    # Attach NIC to LAN.
    ionosctl nic create \
        --datacenter-id "$DC_ID" \
        --server-id "$SERVER_ID" \
        --lan-id "$LAN_ID" \
        --name "${VM_NAME}-nic" \
        --wait-for-request

    echo "  VM ${VM_NAME} created and attached to LAN."
done

echo ""
echo "=== All VMs provisioned ==="
echo "List VMs and IPs:"
ionosctl server list --datacenter-id "$DC_ID" --cols ServerId,Name,State
echo ""
echo "Datacenter ID: ${DC_ID}"
echo "Save this ID for teardown. Run setup scripts on each VM next."
