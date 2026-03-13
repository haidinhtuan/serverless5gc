#!/bin/bash
# Sets up Prometheus on the loadgen VM for cold-start experiment.
# Only scrapes: serverless VM (OpenFaaS gateway, node-exporter) + local node-exporter.
#
# Usage: SERVERLESS_IP=x.x.x.x bash setup-monitoring-coldstart.sh

set -euo pipefail

SERVERLESS_IP="${SERVERLESS_IP:?Set SERVERLESS_IP}"
LOADGEN_IP=$(hostname -I | awk '{print $1}')

echo "Setting up Prometheus on loadgen VM..."
echo "Scraping: serverless=${SERVERLESS_IP}, loadgen=${LOADGEN_IP}"

# Install Docker if needed
if ! command -v docker &>/dev/null; then
    curl -fsSL https://get.docker.com | sh
fi

# Install node-exporter
docker rm -f node-exporter 2>/dev/null || true
docker run -d --name node-exporter --restart unless-stopped \
    --net host --pid host \
    -v /:/host:ro,rslave \
    prom/node-exporter:v1.7.0 --path.rootfs=/host

# Generate Prometheus config
mkdir -p /opt/prometheus
cat > /opt/prometheus/prometheus.yml << PROMEOF
global:
  scrape_interval: 5s
  evaluation_interval: 5s

scrape_configs:
  - job_name: 'openfaas'
    static_configs:
      - targets: ['${SERVERLESS_IP}:31113']
    metrics_path: /metrics
    scrape_interval: 5s

  - job_name: 'node-serverless'
    static_configs:
      - targets: ['${SERVERLESS_IP}:9100']
    labels:
      instance: 'serverless'

  - job_name: 'node-loadgen'
    static_configs:
      - targets: ['${LOADGEN_IP}:9100']
    labels:
      instance: 'loadgen'
PROMEOF

# Start Prometheus
docker rm -f prometheus 2>/dev/null || true
docker run -d --name prometheus --restart unless-stopped \
    --net host \
    -v /opt/prometheus:/etc/prometheus:ro \
    -v /opt/prometheus-data:/prometheus \
    prom/prometheus:v2.51.0 \
    --config.file=/etc/prometheus/prometheus.yml \
    --storage.tsdb.retention.time=30d \
    --web.listen-address=:9090

echo "Prometheus running at http://${LOADGEN_IP}:9090"
echo "Targets: OpenFaaS gateway, node-exporter (serverless + loadgen)"
