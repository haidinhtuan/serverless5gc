#!/bin/bash
# Sets up VM2 (open5gs): Docker, docker-compose, Open5GS baseline.
# Run via: ssh root@VM2 'bash -s' < setup-open5gs.sh
#
# Before running this script, copy the config files:
#   scp -r deploy/baselines/open5gs/ root@VM2:/opt/open5gs/

set -euo pipefail

echo "=== Setting up Open5GS Baseline VM ==="

export DEBIAN_FRONTEND=noninteractive

# ---------------------------------------------------------------------------
# System update
# ---------------------------------------------------------------------------
echo "Updating system packages..."
apt-get update -qq
apt-get upgrade -y -qq

# ---------------------------------------------------------------------------
# Install Docker (official repo)
# ---------------------------------------------------------------------------
echo "Installing Docker..."
apt-get install -y -qq ca-certificates curl gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
    tee /etc/apt/sources.list.d/docker.list > /dev/null

apt-get update -qq
apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin

systemctl enable docker
systemctl start docker
echo "Docker installed: $(docker --version)"

# ---------------------------------------------------------------------------
# Deploy Open5GS
# ---------------------------------------------------------------------------
DEPLOY_DIR="/opt/open5gs"

if [ -f "${DEPLOY_DIR}/docker-compose.yml" ]; then
    echo "Starting Open5GS..."
    cd "$DEPLOY_DIR"
    docker compose pull
    docker compose up -d
    echo "Waiting for services to stabilize (30s)..."
    sleep 30
    docker compose ps
else
    echo "WARNING: ${DEPLOY_DIR}/docker-compose.yml not found."
    echo "Copy config files first: scp -r deploy/baselines/open5gs/ root@<IP>:/opt/open5gs/"
fi

# ---------------------------------------------------------------------------
# Install cAdvisor for container metrics
# ---------------------------------------------------------------------------
echo "Installing cAdvisor..."
docker run -d \
    --name cadvisor \
    --restart unless-stopped \
    -p 8081:8080 \
    --volume=/:/rootfs:ro \
    --volume=/var/run:/var/run:ro \
    --volume=/sys:/sys:ro \
    --volume=/var/lib/docker/:/var/lib/docker:ro \
    --volume=/dev/disk/:/dev/disk:ro \
    --privileged \
    --device=/dev/kmsg \
    gcr.io/cadvisor/cadvisor:v0.49.1

# ---------------------------------------------------------------------------
# Install node-exporter for host metrics
# ---------------------------------------------------------------------------
echo "Installing node-exporter..."
docker run -d \
    --name node-exporter \
    --restart unless-stopped \
    -p 9100:9100 \
    --net="host" \
    --pid="host" \
    -v "/proc:/host/proc:ro" \
    -v "/sys:/host/sys:ro" \
    -v "/:/rootfs:ro" \
    prom/node-exporter:v1.7.0 \
    --path.procfs=/host/proc \
    --path.sysfs=/host/sys \
    --path.rootfs=/rootfs

echo ""
echo "=== Open5GS Baseline VM setup complete ==="
echo "cAdvisor:      http://localhost:8081/metrics"
echo "node-exporter: http://localhost:9100/metrics"
echo "AMF SCTP:      port 38412"
