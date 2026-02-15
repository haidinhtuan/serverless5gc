#!/bin/bash
# Sets up the Open5GS baseline VM: Docker + Open5GS docker-compose.
# Run on the open5gs VM via: ssh vm2 'bash -s' < deploy/ionos/setup-open5gs.sh

set -euo pipefail

echo "=== Setting up Open5GS Baseline VM ==="

# Update system.
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq && apt-get upgrade -y -qq

# Install Docker.
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

# Enable and start Docker.
systemctl enable docker
systemctl start docker

# Copy docker-compose files (should be transferred before running).
DEPLOY_DIR="/opt/open5gs"
mkdir -p "$DEPLOY_DIR"

if [ -d "/tmp/open5gs-deploy" ]; then
    cp -r /tmp/open5gs-deploy/* "$DEPLOY_DIR/"
else
    echo "WARNING: /tmp/open5gs-deploy not found. Copy baselines/open5gs/ to /tmp/open5gs-deploy first."
fi

# Start Open5GS.
if [ -f "${DEPLOY_DIR}/docker-compose.yml" ]; then
    echo "Starting Open5GS..."
    cd "$DEPLOY_DIR"
    docker compose up -d
    echo "Waiting for services to be healthy..."
    sleep 30
    docker compose ps
else
    echo "WARNING: docker-compose.yml not found in ${DEPLOY_DIR}"
fi

# Install cAdvisor and node-exporter for monitoring.
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

echo "=== Open5GS Baseline VM setup complete ==="
