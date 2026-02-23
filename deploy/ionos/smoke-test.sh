#!/bin/bash
# Reproducible smoke test: provision IONOS VMs, deploy, test, teardown
# Uses ionosctl config file auth (personal profile)
#
# This creates a minimal 2-VM setup:
#   VM1: Serverless 5GC (K3s + OpenFaaS + Redis + etcd)
#   VM2: Open5GS baseline (Docker Compose)
# Then runs a quick connectivity test and tears everything down.
#
# Uses DHCP on public LAN (no reserved IP blocks needed).

set -euo pipefail

# Ensure we don't use env var token (use config file instead)
unset IONOS_TOKEN 2>/dev/null || true

DC_NAME="s5gc-smoke-$(date +%Y%m%d-%H%M%S)"
LOCATION="de/fra"
SSH_KEY="$HOME/.ssh/id_rsa.pub"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_FILE="$PROJECT_DIR/eval/results/smoke-test-$(date +%Y%m%d-%H%M%S).log"

mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG_FILE"
}

cleanup() {
    log "=== CLEANUP: Tearing down all resources ==="
    if [ -n "${DC_ID:-}" ]; then
        log "Deleting datacenter $DC_ID ($DC_NAME)..."
        ionosctl datacenter delete --datacenter-id "$DC_ID" --force --wait-for-request --timeout 300 2>&1 | tee -a "$LOG_FILE" || true
        log "Datacenter deleted."
    fi
    log "=== CLEANUP COMPLETE ==="
}

# Always cleanup on exit
trap cleanup EXIT

log "=== SMOKE TEST START ==="
log "Datacenter: $DC_NAME"
log "Location: $LOCATION"

# -----------------------------------------------
# Step 1: Create Datacenter
# -----------------------------------------------
log "Step 1: Creating datacenter..."
DC_ID=$(ionosctl datacenter create \
    --name "$DC_NAME" \
    --location "$LOCATION" \
    --wait-for-request \
    --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "Datacenter created: $DC_ID"

# -----------------------------------------------
# Step 2: Create public LAN (DHCP will assign IPs)
# -----------------------------------------------
log "Step 2: Creating public LAN..."
LAN_ID=$(ionosctl lan create \
    --datacenter-id "$DC_ID" \
    --name "s5gc-lan" \
    --public=true \
    --wait-for-request \
    --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "LAN created: $LAN_ID"

# -----------------------------------------------
# Step 3: Create VM1 (Serverless 5GC) - 4 vCPU, 8GB
# -----------------------------------------------
log "Step 3: Creating VM1 boot volume..."
VOL1_ID=$(ionosctl volume create \
    --datacenter-id "$DC_ID" \
    --name "s5gc-vol" \
    --size 50 \
    --type SSD \
    --image-alias "ubuntu:latest" \
    --ssh-key-paths "$SSH_KEY" \
    --wait-for-request \
    --timeout 300 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "Volume 1 created: $VOL1_ID"

log "Creating VM1 server..."
SRV1_ID=$(ionosctl server create \
    --datacenter-id "$DC_ID" \
    --name "s5gc-serverless" \
    --cores 4 \
    --ram 8GB \
    --type ENTERPRISE \
    --wait-for-request \
    --wait-for-state \
    --timeout 600 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "Server 1 created: $SRV1_ID"

log "Attaching boot volume to VM1..."
ionosctl server volume attach \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV1_ID" \
    --volume-id "$VOL1_ID" \
    --wait-for-request \
    --timeout 120 2>>"$LOG_FILE"
ionosctl server update \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV1_ID" \
    --volume-id "$VOL1_ID" \
    --wait-for-request \
    --wait-for-state \
    --timeout 300 2>>"$LOG_FILE"
log "Boot volume attached to VM1"

log "Creating VM1 NIC on public LAN (DHCP)..."
NIC1_ID=$(ionosctl nic create \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV1_ID" \
    --name "s5gc-nic" \
    --lan-id "$LAN_ID" \
    --dhcp=true \
    --wait-for-request \
    --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "NIC 1 created: $NIC1_ID"

# -----------------------------------------------
# Step 4: Create VM2 (Open5GS baseline) - 4 vCPU, 8GB
# -----------------------------------------------
log "Step 4: Creating VM2 boot volume..."
VOL2_ID=$(ionosctl volume create \
    --datacenter-id "$DC_ID" \
    --name "open5gs-vol" \
    --size 50 \
    --type SSD \
    --image-alias "ubuntu:latest" \
    --ssh-key-paths "$SSH_KEY" \
    --wait-for-request \
    --timeout 300 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "Volume 2 created: $VOL2_ID"

log "Creating VM2 server..."
SRV2_ID=$(ionosctl server create \
    --datacenter-id "$DC_ID" \
    --name "s5gc-open5gs" \
    --cores 4 \
    --ram 8GB \
    --type ENTERPRISE \
    --wait-for-request \
    --wait-for-state \
    --timeout 600 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "Server 2 created: $SRV2_ID"

log "Attaching boot volume to VM2..."
ionosctl server volume attach \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV2_ID" \
    --volume-id "$VOL2_ID" \
    --wait-for-request \
    --timeout 120 2>>"$LOG_FILE"
ionosctl server update \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV2_ID" \
    --volume-id "$VOL2_ID" \
    --wait-for-request \
    --wait-for-state \
    --timeout 300 2>>"$LOG_FILE"
log "Boot volume attached to VM2"

log "Creating VM2 NIC on public LAN (DHCP)..."
NIC2_ID=$(ionosctl nic create \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV2_ID" \
    --name "open5gs-nic" \
    --lan-id "$LAN_ID" \
    --dhcp=true \
    --wait-for-request \
    --timeout 120 \
    --output json 2>>"$LOG_FILE" | jq -r '.id')
log "NIC 2 created: $NIC2_ID"

# -----------------------------------------------
# Step 5: Get DHCP-assigned IPs
# -----------------------------------------------
log "Step 5: Waiting for DHCP IP assignment (30s)..."
sleep 30

IP1=$(ionosctl nic get \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV1_ID" \
    --nic-id "$NIC1_ID" \
    --output json 2>>"$LOG_FILE" | jq -r '.properties.ips[0] // empty')

IP2=$(ionosctl nic get \
    --datacenter-id "$DC_ID" \
    --server-id "$SRV2_ID" \
    --nic-id "$NIC2_ID" \
    --output json 2>>"$LOG_FILE" | jq -r '.properties.ips[0] // empty')

if [ -z "$IP1" ] || [ -z "$IP2" ]; then
    log "Waiting longer for IPs (30s more)..."
    sleep 30
    IP1=$(ionosctl nic get --datacenter-id "$DC_ID" --server-id "$SRV1_ID" --nic-id "$NIC1_ID" --output json 2>>"$LOG_FILE" | jq -r '.properties.ips[0] // empty')
    IP2=$(ionosctl nic get --datacenter-id "$DC_ID" --server-id "$SRV2_ID" --nic-id "$NIC2_ID" --output json 2>>"$LOG_FILE" | jq -r '.properties.ips[0] // empty')
fi

log "VM1 IP: $IP1"
log "VM2 IP: $IP2"

if [ -z "$IP1" ] || [ -z "$IP2" ]; then
    log "ERROR: Could not obtain IPs. Aborting."
    exit 1
fi

# -----------------------------------------------
# Step 6: Wait for VMs to boot and test SSH
# -----------------------------------------------
log "Step 6: Waiting for VMs to boot (60s)..."
sleep 60

log "Testing SSH to VM1 ($IP1)..."
VM1_SSH_OK=false
for i in $(seq 1 10); do
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 -i ~/.ssh/id_rsa root@"$IP1" "echo 'VM1 OK'" 2>>"$LOG_FILE"; then
        log "VM1 SSH OK"
        VM1_SSH_OK=true
        break
    fi
    log "SSH attempt $i/10 failed, retrying in 15s..."
    sleep 15
done

log "Testing SSH to VM2 ($IP2)..."
VM2_SSH_OK=false
for i in $(seq 1 10); do
    if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=10 -i ~/.ssh/id_rsa root@"$IP2" "echo 'VM2 OK'" 2>>"$LOG_FILE"; then
        log "VM2 SSH OK"
        VM2_SSH_OK=true
        break
    fi
    log "SSH attempt $i/10 failed, retrying in 15s..."
    sleep 15
done

if ! $VM1_SSH_OK || ! $VM2_SSH_OK; then
    log "ERROR: SSH failed. Aborting."
    exit 1
fi

# -----------------------------------------------
# Step 7: Install Docker on VM2 and deploy Open5GS
# -----------------------------------------------
log "Step 7: Setting up Open5GS on VM2..."
ssh -o StrictHostKeyChecking=no root@"$IP2" 'bash -s' <<'SETUP_OPEN5GS'
set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq docker.io docker-compose-v2 > /dev/null 2>&1
systemctl enable --now docker
echo "Docker installed: $(docker --version)"
SETUP_OPEN5GS
log "Docker installed on VM2"

# Copy Open5GS configs and deploy
ssh -o StrictHostKeyChecking=no root@"$IP2" 'mkdir -p /opt/open5gs' 2>>"$LOG_FILE"
scp -o StrictHostKeyChecking=no -r "$PROJECT_DIR/deploy/baselines/open5gs/"* root@"$IP2":/opt/open5gs/ 2>>"$LOG_FILE"
ssh -o StrictHostKeyChecking=no root@"$IP2" 'cd /opt/open5gs && docker compose up -d 2>&1' | tee -a "$LOG_FILE"
log "Open5GS deployed on VM2"

# -----------------------------------------------
# Step 8: Install K3s + OpenFaaS on VM1
# -----------------------------------------------
log "Step 8: Setting up K3s on VM1..."
ssh -o StrictHostKeyChecking=no root@"$IP1" 'bash -s' <<'SETUP_K3S'
set -e
export DEBIAN_FRONTEND=noninteractive

# Install K3s
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Wait for K3s to be ready
for i in $(seq 1 30); do
    if kubectl get nodes 2>/dev/null | grep -q " Ready"; then
        echo "K3s ready"
        break
    fi
    echo "Waiting for K3s... ($i/30)"
    sleep 10
done

# Install Helm
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Install OpenFaaS
kubectl create namespace openfaas || true
kubectl create namespace openfaas-fn || true
helm repo add openfaas https://openfaas.github.io/faas-netes/
helm repo update
helm install openfaas openfaas/openfaas \
    --namespace openfaas \
    --set functionNamespace=openfaas-fn \
    --set generateBasicAuth=true \
    --wait --timeout 300s

echo "OpenFaaS installed"
kubectl -n openfaas get pods
SETUP_K3S
log "K3s + OpenFaaS installed on VM1"

# Deploy Redis and etcd on VM1
log "Deploying Redis and etcd on VM1..."
ssh -o StrictHostKeyChecking=no root@"$IP1" 'bash -s' <<'SETUP_INFRA'
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml

# Redis
kubectl run redis --image=redis:7-alpine --port=6379 -n openfaas-fn
kubectl expose pod redis --port=6379 -n openfaas-fn

# etcd
kubectl run etcd --image=quay.io/coreos/etcd:v3.5.17 --port=2379 -n openfaas-fn \
    -- etcd --advertise-client-urls=http://0.0.0.0:2379 --listen-client-urls=http://0.0.0.0:2379
kubectl expose pod etcd --port=2379 -n openfaas-fn

# Wait for pods
sleep 15
kubectl get pods -n openfaas-fn
echo "Redis and etcd deployed"
SETUP_INFRA
log "Redis and etcd deployed on VM1"

# -----------------------------------------------
# Step 9: Connectivity verification
# -----------------------------------------------
log "Step 9: Running connectivity tests..."

# Check Open5GS containers
log "--- Open5GS containers ---"
ssh -o StrictHostKeyChecking=no root@"$IP2" 'docker compose -f /opt/open5gs/docker-compose.yml ps 2>/dev/null' 2>>"$LOG_FILE" | tee -a "$LOG_FILE" || log "Open5GS check failed"

# Check K3s pods
log "--- K3s pods ---"
ssh -o StrictHostKeyChecking=no root@"$IP1" 'KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl get pods -A 2>/dev/null' 2>>"$LOG_FILE" | tee -a "$LOG_FILE" || log "K3s check failed"

# Check OpenFaaS gateway
OPENFAAS_HEALTH=$(ssh -o StrictHostKeyChecking=no root@"$IP1" 'curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/healthz 2>/dev/null' || echo "FAILED")
log "OpenFaaS health check: HTTP $OPENFAAS_HEALTH"

# Check Redis connectivity
REDIS_CHECK=$(ssh -o StrictHostKeyChecking=no root@"$IP1" 'KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl exec -n openfaas-fn redis -- redis-cli ping 2>/dev/null' || echo "FAILED")
log "Redis ping: $REDIS_CHECK"

# -----------------------------------------------
# Step 10: Collect resource metrics
# -----------------------------------------------
log "Step 10: Collecting resource metrics..."

ssh -o StrictHostKeyChecking=no root@"$IP1" 'echo "=== VM1 (Serverless) ==="; echo "CPU: $(nproc) cores"; free -h | head -2; df -h / | tail -1' 2>>"$LOG_FILE" | tee -a "$LOG_FILE"
ssh -o StrictHostKeyChecking=no root@"$IP2" 'echo "=== VM2 (Open5GS) ==="; echo "CPU: $(nproc) cores"; free -h | head -2; df -h / | tail -1' 2>>"$LOG_FILE" | tee -a "$LOG_FILE"

log ""
log "========================================="
log "  SMOKE TEST RESULTS"
log "========================================="
log "VM1 (Serverless): $IP1"
log "  - K3s: INSTALLED"
log "  - OpenFaaS: HTTP $OPENFAAS_HEALTH"
log "  - Redis: $REDIS_CHECK"
log "VM2 (Open5GS):    $IP2"
log "  - Docker: INSTALLED"
log "  - Open5GS: DEPLOYED"
log "========================================="
log "Infrastructure provisioning: SUCCESS"
log "Log file: $LOG_FILE"
log "========================================="
log ""
log "=== AUTO-TEARDOWN IN 10 SECONDS ==="
sleep 10
