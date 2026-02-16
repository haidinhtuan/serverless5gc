#!/bin/bash
# Deploys all function images to the serverless VM.
#
# Workflow:
#   1. Saves Docker images to a tar archive
#   2. SCPs the archive to the VM
#   3. Imports into K3s containerd
#   4. Deploys via faas-cli using stack.yml
#
# Prerequisites:
#   - Run build-functions.sh first to build images
#   - VM must have K3s + OpenFaaS running (setup-serverless.sh)
#
# Usage: ./deploy-functions.sh <vm-ip>
#   or source vm-ips.env && ./deploy-functions.sh $SERVERLESS_IP

set -euo pipefail

VM_IP=${1:?Usage: $0 <serverless-vm-ip>}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
REGISTRY="${REGISTRY:-serverless5gc}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"

IMAGES=(
    "nrf-register"
    "nrf-discover"
    "nrf-status-notify"
    "amf-initial-registration"
    "amf-deregistration"
    "amf-service-request"
    "amf-pdu-session-relay"
    "amf-handover"
    "amf-auth-initiate"
    "smf-pdu-session-create"
    "smf-pdu-session-update"
    "smf-pdu-session-release"
    "smf-n4-session-setup"
    "udm-generate-auth-data"
    "udm-get-subscriber-data"
    "udr-data-read"
    "udr-data-write"
    "ausf-authenticate"
    "pcf-policy-create"
    "pcf-policy-get"
    "nssf-slice-select"
)

echo "=== Deploying ${#IMAGES[@]} functions to ${VM_IP} ==="

# Step 1: Save all images to a single tar.
ARCHIVE="/tmp/s5gc-functions.tar"
echo "Step 1: Saving Docker images..."
IMAGE_REFS=()
for img in "${IMAGES[@]}"; do
    IMAGE_REFS+=("${REGISTRY}/${img}:latest")
done
docker save "${IMAGE_REFS[@]}" | gzip > "${ARCHIVE}.gz"
SIZE=$(du -h "${ARCHIVE}.gz" | cut -f1)
echo "  Archive: ${ARCHIVE}.gz (${SIZE})"

# Step 2: SCP to VM.
echo "Step 2: Copying to VM..."
scp -o StrictHostKeyChecking=no -i "$SSH_KEY" \
    "${ARCHIVE}.gz" "root@${VM_IP}:/tmp/s5gc-functions.tar.gz"

# Step 3: Import into K3s containerd.
echo "Step 3: Importing images into K3s..."
ssh -o StrictHostKeyChecking=no -i "$SSH_KEY" "root@${VM_IP}" \
    'gunzip -f /tmp/s5gc-functions.tar.gz && k3s ctr images import /tmp/s5gc-functions.tar && rm -f /tmp/s5gc-functions.tar'

# Step 4: Copy stack.yml and deploy with faas-cli.
echo "Step 4: Deploying functions via faas-cli..."
scp -o StrictHostKeyChecking=no -i "$SSH_KEY" \
    "${SCRIPT_DIR}/stack.yml" "root@${VM_IP}:/tmp/stack.yml"

ssh -o StrictHostKeyChecking=no -i "$SSH_KEY" "root@${VM_IP}" 'bash -s' << 'DEPLOY'
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
export OPENFAAS_URL=http://127.0.0.1:31112

cd /tmp
faas-cli deploy -f stack.yml --update=false --replace

echo ""
echo "Waiting for functions to be ready (30s)..."
sleep 30
faas-cli list
DEPLOY

# Cleanup local archive.
rm -f "${ARCHIVE}.gz"

echo ""
echo "=== Deployment complete ==="
echo "Functions deployed to ${VM_IP}"
echo "Test: curl -s http://${VM_IP}:31112/function/nrf-discover -d '{\"nf_type\":\"AMF\"}'"
