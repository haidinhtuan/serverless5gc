#!/bin/bash
# Sets up the free5GC baseline VM: Docker + free5GC docker-compose.
# Run on the free5gc VM via: ssh vm3 'bash -s' < deploy/ionos/setup-free5gc.sh

set -euo pipefail

echo "=== Setting up free5GC Baseline VM ==="

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

# Load GTP5G kernel module (required by free5GC UPF).
echo "Loading GTP5G kernel module..."
apt-get install -y -qq build-essential linux-headers-$(uname -r) git
if [ ! -d "/tmp/gtp5g" ]; then
    git clone https://github.com/free5gc/gtp5g.git /tmp/gtp5g
fi
cd /tmp/gtp5g
make clean && make
make install
modprobe gtp5g
echo "gtp5g" >> /etc/modules-load.d/gtp5g.conf

# Copy docker-compose files (should be transferred before running).
DEPLOY_DIR="/opt/free5gc"
mkdir -p "$DEPLOY_DIR"

if [ -d "/tmp/free5gc-deploy" ]; then
    cp -r /tmp/free5gc-deploy/* "$DEPLOY_DIR/"
else
    echo "WARNING: /tmp/free5gc-deploy not found. Copy baselines/free5gc/ to /tmp/free5gc-deploy first."
fi

# Start free5GC.
if [ -f "${DEPLOY_DIR}/docker-compose.yml" ]; then
    echo "Starting free5GC..."
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

echo "=== free5GC Baseline VM setup complete ==="
