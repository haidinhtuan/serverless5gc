#!/bin/bash
# Provisions 2 VMs on IONOS Cloud for cold-start storm experiment.
#
# VMs:
#   VM1: serverless5gc (4 vCPU, 8GB) - K3s + OpenFaaS + Redis + etcd + SCTP proxy
#   VM2: loadgen       (4 vCPU, 8GB) - UERANSIM + Prometheus + node-exporter
#
# Usage: ./provision-coldstart.sh

set -euo pipefail
unset IONOS_TOKEN 2>/dev/null || true

DC_NAME="s5gc-coldstart-$(date +%Y%m%d-%H%M%S)"
LOCATION="de/fra"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa.pub}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_FILE="${PROJECT_DIR}/eval/results/provision-coldstart-$(date +%Y%m%d-%H%M%S).log"
ENV_FILE="${SCRIPT_DIR}/vm-ips-coldstart.env"

mkdir -p "$(dirname "$LOG_FILE")"

log() { echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG_FILE"; }

cleanup() {
    if [ "${SKIP_CLEANUP:-0}" = "1" ]; then
        log "SKIP_CLEANUP=1, skipping teardown."
        return
    fi
    log "=== CLEANUP: Tearing down datacenter ==="
    if [ -n "${DC_ID:-}" ]; then
        ionosctl datacenter delete --datacenter-id "$DC_ID" --force --wait-for-request --timeout 600 2>&1 | tee -a "$LOG_FILE" || true
    fi
}
trap cleanup EXIT

# Only 2 VMs needed
declare -a VM_DEFS=(
    "serverless5gc:4:8192:50"
    "loadgen:4:8192:40"
)

declare -a VM_NAMES=() SRV_IDS=() VOL_IDS=() NIC_IDS=() VM_IPS=()

# Step 1: Create datacenter
log "Step 1: Creating datacenter $DC_NAME..."
DC_ID=$(ionosctl datacenter create \
    --name "$DC_NAME" --location "$LOCATION" \
    --wait-for-request --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "Datacenter created: $DC_ID"

# Step 2: Create public LAN
log "Step 2: Creating public LAN..."
LAN_ID=$(ionosctl lan create \
    --datacenter-id "$DC_ID" --name "s5gc-public" --public=true \
    --wait-for-request --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')

# Step 3: Create VMs
for i in "${!VM_DEFS[@]}"; do
    IFS=':' read -r VM_NAME CORES RAM VOL_SIZE <<< "${VM_DEFS[$i]}"
    VM_NAMES+=("$VM_NAME")
    log "Creating VM '$VM_NAME' (${CORES}vCPU, $((RAM/1024))GB, ${VOL_SIZE}GB SSD)..."

    SRV_ID=$(ionosctl server create \
        --datacenter-id "$DC_ID" --name "$VM_NAME" \
        --cores "$CORES" --ram "${RAM}MB" --type ENTERPRISE \
        --wait-for-request --wait-for-state --timeout 600 \
        --output json 2>>"$LOG_FILE" | jq -r '.id')
    SRV_IDS+=("$SRV_ID")

    VOL_ID=$(ionosctl volume create \
        --datacenter-id "$DC_ID" --name "${VM_NAME}-vol" \
        --size "$VOL_SIZE" --type SSD --image-alias "ubuntu:latest" \
        --ssh-key-paths "$SSH_KEY" \
        --wait-for-request --timeout 300 \
        --output json 2>>"$LOG_FILE" | jq -r '.id')
    VOL_IDS+=("$VOL_ID")

    ionosctl server volume attach \
        --datacenter-id "$DC_ID" --server-id "$SRV_ID" --volume-id "$VOL_ID" \
        --wait-for-request --timeout 120 2>>"$LOG_FILE"

    ionosctl server update \
        --datacenter-id "$DC_ID" --server-id "$SRV_ID" --volume-id "$VOL_ID" \
        --wait-for-request --wait-for-state --timeout 300 2>>"$LOG_FILE"

    NIC_ID=$(ionosctl nic create \
        --datacenter-id "$DC_ID" --server-id "$SRV_ID" \
        --name "${VM_NAME}-nic" --lan-id "$LAN_ID" --dhcp=true \
        --wait-for-request --timeout 120 \
        --output json 2>>"$LOG_FILE" | jq -r '.id')
    NIC_IDS+=("$NIC_ID")
    log "VM '$VM_NAME' created."
done

# Step 4: Wait for DHCP + retrieve IPs
log "Waiting for DHCP (45s)..."
sleep 45

get_ip() {
    ionosctl nic get \
        --datacenter-id "$DC_ID" --server-id "${SRV_IDS[$1]}" --nic-id "${NIC_IDS[$1]}" \
        --output json 2>>"$LOG_FILE" | jq -r '.properties.ips[0] // empty'
}

for attempt in 1 2 3; do
    ALL_OK=true; VM_IPS=()
    for i in "${!VM_NAMES[@]}"; do
        IP=$(get_ip "$i"); VM_IPS+=("$IP")
        [ -z "$IP" ] && ALL_OK=false
    done
    $ALL_OK && break
    [ "$attempt" -lt 3 ] && sleep 30
done

# Step 5: Write env file
cat > "$ENV_FILE" << ENVEOF
# Cold-start experiment VMs - $(date -Iseconds)
# Datacenter: ${DC_NAME} (${DC_ID})
DC_ID="${DC_ID}"
DC_NAME="${DC_NAME}"
SERVERLESS_IP="${VM_IPS[0]}"
LOADGEN_IP="${VM_IPS[1]}"
MONITORING_IP="${VM_IPS[1]}"
TARGET_AMF_IP="${VM_IPS[0]}"
ENVEOF

# Step 6: Wait for SSH
log "Waiting for VMs to boot (60s)..."
sleep 60
for i in "${!VM_NAMES[@]}"; do
    for attempt in $(seq 1 15); do
        ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 -o BatchMode=yes \
            -i "${SSH_KEY%.pub}" root@"${VM_IPS[$i]}" "echo OK" >/dev/null 2>&1 && break
        [ "$attempt" -lt 15 ] && sleep 15
    done
    log "${VM_NAMES[$i]}: ${VM_IPS[$i]}"
done

trap - EXIT
log "=== PROVISIONING COMPLETE ==="
log "Source env: source $ENV_FILE"
