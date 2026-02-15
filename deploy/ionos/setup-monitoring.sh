#!/bin/bash
# Sets up the monitoring VM: Docker, Prometheus, Grafana, cAdvisor, node-exporter, cost-exporter.
# Run on the monitoring VM via: ssh vm5 'bash -s' < deploy/ionos/setup-monitoring.sh

set -euo pipefail

echo "=== Setting up Monitoring VM ==="

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

# Copy monitoring docker-compose and configs.
DEPLOY_DIR="/opt/monitoring"
mkdir -p "$DEPLOY_DIR"

if [ -d "/tmp/monitoring-deploy" ]; then
    cp -r /tmp/monitoring-deploy/* "$DEPLOY_DIR/"
else
    echo "WARNING: /tmp/monitoring-deploy not found."
    echo "Copy deploy/monitoring/ to /tmp/monitoring-deploy first."
fi

# Start monitoring stack.
if [ -f "${DEPLOY_DIR}/docker-compose.yml" ]; then
    echo "Starting monitoring stack..."
    cd "$DEPLOY_DIR"
    docker compose up -d
    echo "Waiting for services..."
    sleep 15
    docker compose ps
else
    echo "WARNING: docker-compose.yml not found in ${DEPLOY_DIR}"
fi

echo ""
echo "=== Monitoring VM setup complete ==="
echo "Prometheus: http://<VM5_IP>:9090"
echo "Grafana:    http://<VM5_IP>:3000 (admin/admin)"
