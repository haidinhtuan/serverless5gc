#!/bin/bash
# Provisions 5 VMs on IONOS Cloud for the serverless 5GC evaluation campaign.
#
# VMs:
#   VM1: serverless5gc (8 vCPU, 16GB) - K3s + OpenFaaS + Redis + etcd + UPF
#   VM2: open5gs       (8 vCPU, 16GB) - Docker + Open5GS
#   VM3: free5gc       (8 vCPU, 16GB) - Docker + free5GC
#   VM4: loadgen       (4 vCPU,  8GB) - UERANSIM load generator
#   VM5: monitoring    (4 vCPU,  8GB) - Prometheus + Grafana + cAdvisor
#
# Creates datacenter, public LAN, 5 servers with SSD boot volumes, NICs.
# Outputs vm-ips.env with all assigned IPs.
#
# Usage: ./provision.sh
# Set SKIP_CLEANUP=1 to prevent teardown on failure.

set -euo pipefail

# Use config file auth, not env var token.
unset IONOS_TOKEN 2>/dev/null || true

DC_NAME="s5gc-eval-$(date +%Y%m%d-%H%M%S)"
LOCATION="de/fra"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa.pub}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_FILE="${PROJECT_DIR}/eval/results/provision-$(date +%Y%m%d-%H%M%S).log"
ENV_FILE="${SCRIPT_DIR}/vm-ips.env"

mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG_FILE"
}

cleanup() {
    if [ "${SKIP_CLEANUP:-0}" = "1" ]; then
        log "SKIP_CLEANUP=1, skipping teardown."
        return
    fi
    log "=== CLEANUP: Tearing down datacenter ==="
    if [ -n "${DC_ID:-}" ]; then
        log "Deleting datacenter $DC_ID ($DC_NAME)..."
        ionosctl datacenter delete \
            --datacenter-id "$DC_ID" \
            --force \
            --wait-for-request \
            --timeout 600 2>&1 | tee -a "$LOG_FILE" || true
        log "Datacenter deleted."
    fi
    log "=== CLEANUP COMPLETE ==="
}

trap cleanup EXIT

if [ ! -f "$SSH_KEY" ]; then
    log "ERROR: SSH public key not found at $SSH_KEY"
    exit 1
fi

log "=== PROVISIONING START ==="
log "Datacenter: $DC_NAME"
log "Location:   $LOCATION"
log "SSH key:    $SSH_KEY"
log "Log file:   $LOG_FILE"

# ---------------------------------------------------------------------------
# VM definitions: name:cores:ram_mb:vol_size_gb
# ---------------------------------------------------------------------------
declare -a VM_DEFS=(
    "serverless5gc:4:8192:50"
    "open5gs:4:8192:50"
    "free5gc:4:8192:50"
    "loadgen:4:8192:40"
    "monitoring:4:8192:40"
)

# Arrays to track resource IDs.
declare -a VM_NAMES=()
declare -a SRV_IDS=()
declare -a VOL_IDS=()
declare -a NIC_IDS=()
declare -a VM_IPS=()

# ---------------------------------------------------------------------------
# Step 1: Create datacenter
# ---------------------------------------------------------------------------
log "Step 1: Creating datacenter..."
DC_ID=$(ionosctl datacenter create \
    --name "$DC_NAME" \
    --location "$LOCATION" \
    --wait-for-request \
    --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "Datacenter created: $DC_ID"

# ---------------------------------------------------------------------------
# Step 2: Create public LAN with DHCP
# ---------------------------------------------------------------------------
log "Step 2: Creating public LAN..."
LAN_ID=$(ionosctl lan create \
    --datacenter-id "$DC_ID" \
    --name "s5gc-public" \
    --public=true \
    --wait-for-request \
    --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "LAN created: $LAN_ID"

# ---------------------------------------------------------------------------
# Step 3: Create VMs (server + volume + attach + NIC)
# ---------------------------------------------------------------------------
for i in "${!VM_DEFS[@]}"; do
    IFS=':' read -r VM_NAME CORES RAM VOL_SIZE <<< "${VM_DEFS[$i]}"
    VM_NAMES+=("$VM_NAME")

    log ""
    log "Step 3.${i}: Creating VM '${VM_NAME}' (${CORES} vCPU, $((RAM / 1024))GB RAM, ${VOL_SIZE}GB SSD)..."

    # Create server (NO --volume-id for ENTERPRISE type).
    log "  Creating server..."
    SRV_ID=$(ionosctl server create \
        --datacenter-id "$DC_ID" \
        --name "$VM_NAME" \
        --cores "$CORES" \
        --ram "${RAM}MB" \
        --type ENTERPRISE \
        --wait-for-request \
        --wait-for-state \
        --timeout 600 \
        --output json 2>>"$LOG_FILE" | jq -r '.id')
    SRV_IDS+=("$SRV_ID")
    log "  Server created: $SRV_ID"

    # Create volume.
    log "  Creating boot volume..."
    VOL_ID=$(ionosctl volume create \
        --datacenter-id "$DC_ID" \
        --name "${VM_NAME}-vol" \
        --size "$VOL_SIZE" \
        --type SSD \
        --image-alias "ubuntu:latest" \
        --ssh-key-paths "$SSH_KEY" \
        --wait-for-request \
        --timeout 300 \
        --output json 2>>"$LOG_FILE" | jq -r '.id')
    VOL_IDS+=("$VOL_ID")
    log "  Volume created: $VOL_ID"

    # Attach volume to server.
    log "  Attaching volume..."
    ionosctl server volume attach \
        --datacenter-id "$DC_ID" \
        --server-id "$SRV_ID" \
        --volume-id "$VOL_ID" \
        --wait-for-request \
        --timeout 120 2>>"$LOG_FILE"

    # Set as boot volume.
    log "  Setting boot volume..."
    ionosctl server update \
        --datacenter-id "$DC_ID" \
        --server-id "$SRV_ID" \
        --volume-id "$VOL_ID" \
        --wait-for-request \
        --wait-for-state \
        --timeout 300 2>>"$LOG_FILE"
    log "  Boot volume attached."

    # Create NIC on public LAN with DHCP.
    log "  Creating NIC on public LAN..."
    NIC_ID=$(ionosctl nic create \
        --datacenter-id "$DC_ID" \
        --server-id "$SRV_ID" \
        --name "${VM_NAME}-nic" \
        --lan-id "$LAN_ID" \
        --dhcp=true \
        --wait-for-request \
        --timeout 120 \
        --output json 2>>"$LOG_FILE" | jq -r '.id')
    NIC_IDS+=("$NIC_ID")
    log "  NIC created: $NIC_ID"

    log "  VM '${VM_NAME}' provisioned."
done

# ---------------------------------------------------------------------------
# Step 4: Wait for DHCP IPs
# ---------------------------------------------------------------------------
log ""
log "Step 4: Waiting for DHCP IP assignment (45s)..."
sleep 45

get_ip() {
    local srv_idx=$1
    ionosctl nic get \
        --datacenter-id "$DC_ID" \
        --server-id "${SRV_IDS[$srv_idx]}" \
        --nic-id "${NIC_IDS[$srv_idx]}" \
        --output json 2>>"$LOG_FILE" | jq -r '.properties.ips[0] // empty'
}

# Retry IP retrieval up to 3 times with 30s intervals.
for attempt in 1 2 3; do
    ALL_IPS_OK=true
    VM_IPS=()
    for i in "${!VM_NAMES[@]}"; do
        IP=$(get_ip "$i")
        VM_IPS+=("$IP")
        if [ -z "$IP" ]; then
            ALL_IPS_OK=false
        fi
    done

    if $ALL_IPS_OK; then
        break
    fi

    if [ "$attempt" -lt 3 ]; then
        log "  Some IPs not assigned yet, retrying in 30s (attempt $attempt/3)..."
        sleep 30
    fi
done

# Print IP summary and check for failures.
log ""
log "=== VM IP Addresses ==="
for i in "${!VM_NAMES[@]}"; do
    log "  ${VM_NAMES[$i]}: ${VM_IPS[$i]:-MISSING}"
done

for i in "${!VM_NAMES[@]}"; do
    if [ -z "${VM_IPS[$i]:-}" ]; then
        log "ERROR: Could not obtain IP for ${VM_NAMES[$i]}. Aborting."
        exit 1
    fi
done

# ---------------------------------------------------------------------------
# Step 5: Write vm-ips.env
# ---------------------------------------------------------------------------
log ""
log "Step 5: Writing $ENV_FILE..."
cat > "$ENV_FILE" << ENVEOF
# IONOS Cloud VM IPs - generated by provision.sh
# Datacenter: ${DC_NAME} (${DC_ID})
# Generated: $(date -Iseconds)

DC_ID="${DC_ID}"
DC_NAME="${DC_NAME}"

SERVERLESS_IP="${VM_IPS[0]}"
OPEN5GS_IP="${VM_IPS[1]}"
FREE5GC_IP="${VM_IPS[2]}"
LOADGEN_IP="${VM_IPS[3]}"
MONITORING_IP="${VM_IPS[4]}"
ENVEOF
log "vm-ips.env written."

# ---------------------------------------------------------------------------
# Step 6: Wait for SSH readiness
# ---------------------------------------------------------------------------
log ""
log "Step 6: Waiting for VMs to boot (60s)..."
sleep 60

wait_for_ssh() {
    local name=$1
    local ip=$2
    local max_attempts=15

    for attempt in $(seq 1 $max_attempts); do
        if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 -o BatchMode=yes \
            -i "${SSH_KEY%.pub}" root@"$ip" "echo 'SSH OK'" >/dev/null 2>>"$LOG_FILE"; then
            log "  $name ($ip): SSH ready"
            return 0
        fi
        if [ "$attempt" -lt "$max_attempts" ]; then
            sleep 15
        fi
    done
    log "  WARNING: $name ($ip): SSH not ready after $max_attempts attempts"
    return 1
}

SSH_FAILURES=0
for i in "${!VM_NAMES[@]}"; do
    log "  Testing SSH to ${VM_NAMES[$i]} (${VM_IPS[$i]})..."
    if ! wait_for_ssh "${VM_NAMES[$i]}" "${VM_IPS[$i]}"; then
        SSH_FAILURES=$((SSH_FAILURES + 1))
    fi
done

if [ "$SSH_FAILURES" -gt 0 ]; then
    log "WARNING: $SSH_FAILURES VM(s) failed SSH check. Check connectivity manually."
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
log ""
log "========================================="
log "  PROVISIONING COMPLETE"
log "========================================="
log "Datacenter: $DC_NAME ($DC_ID)"
log ""
for i in "${!VM_NAMES[@]}"; do
    log "  ${VM_NAMES[$i]}: ${VM_IPS[$i]}"
done
log ""
log "VM IPs file: $ENV_FILE"
log "Log file:    $LOG_FILE"
log "========================================="
log ""
log "Next steps:"
log "  1. scp config files to VMs"
log "  2. Run setup-*.sh scripts on each VM"
log "  3. Run run-eval.sh to start evaluation"

# Disable cleanup trap on success.
trap - EXIT
log "Provisioning succeeded. Cleanup trap disabled."
