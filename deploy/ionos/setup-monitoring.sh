#!/bin/bash
# Sets up VM5 (monitoring): Docker, Prometheus, Grafana, cost-exporter.
# Run via: ssh root@VM5 'bash -s' < setup-monitoring.sh
#
# Environment variables (set before piping):
#   SERVERLESS_IP - IP of the serverless5gc VM
#   OPEN5GS_IP    - IP of the open5gs VM
#   FREE5GC_IP    - IP of the free5gc VM
#   LOADGEN_IP    - IP of the loadgen VM
#   MONITORING_IP - IP of this monitoring VM
#
# Example:
#   export SERVERLESS_IP=... OPEN5GS_IP=... FREE5GC_IP=... LOADGEN_IP=... MONITORING_IP=...
#   ssh root@VM5 "SERVERLESS_IP=$SERVERLESS_IP OPEN5GS_IP=$OPEN5GS_IP FREE5GC_IP=$FREE5GC_IP \
#     LOADGEN_IP=$LOADGEN_IP MONITORING_IP=$MONITORING_IP bash -s" < setup-monitoring.sh

set -euo pipefail

echo "=== Setting up Monitoring VM ==="

# Validate required IPs.
SERVERLESS_IP="${SERVERLESS_IP:?Set SERVERLESS_IP}"
OPEN5GS_IP="${OPEN5GS_IP:?Set OPEN5GS_IP}"
FREE5GC_IP="${FREE5GC_IP:?Set FREE5GC_IP}"
LOADGEN_IP="${LOADGEN_IP:?Set LOADGEN_IP}"
MONITORING_IP="${MONITORING_IP:?Set MONITORING_IP}"

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
# Create monitoring directory structure
# ---------------------------------------------------------------------------
DEPLOY_DIR="/opt/monitoring"
mkdir -p "${DEPLOY_DIR}/prometheus"
mkdir -p "${DEPLOY_DIR}/grafana/provisioning/datasources"

# ---------------------------------------------------------------------------
# Generate Prometheus configuration with VM IPs
# ---------------------------------------------------------------------------
echo "Generating Prometheus configuration..."
cat > "${DEPLOY_DIR}/prometheus/prometheus.yml" << PROMEOF
global:
  scrape_interval: 5s
  evaluation_interval: 5s

scrape_configs:
  # OpenFaaS gateway metrics (serverless VM).
  - job_name: 'openfaas-gateway'
    static_configs:
      - targets: ['${SERVERLESS_IP}:8080']
    metrics_path: /metrics

  # cAdvisor (container metrics) from each VM.
  - job_name: 'cadvisor'
    static_configs:
      - targets:
          - '${SERVERLESS_IP}:8081'
          - '${OPEN5GS_IP}:8081'
          - '${FREE5GC_IP}:8081'
        labels:
          __meta_role: 'target-vm'

  # node-exporter (host metrics) from all VMs.
  - job_name: 'node-exporter'
    static_configs:
      - targets:
          - '${SERVERLESS_IP}:9100'
          - '${OPEN5GS_IP}:9100'
          - '${FREE5GC_IP}:9100'
          - '${LOADGEN_IP}:9100'
          - '${MONITORING_IP}:9100'
    relabel_configs:
      - source_labels: [__address__]
        regex: '${SERVERLESS_IP}:.*'
        target_label: vm
        replacement: 'serverless5gc'
      - source_labels: [__address__]
        regex: '${OPEN5GS_IP}:.*'
        target_label: vm
        replacement: 'open5gs'
      - source_labels: [__address__]
        regex: '${FREE5GC_IP}:.*'
        target_label: vm
        replacement: 'free5gc'
      - source_labels: [__address__]
        regex: '${LOADGEN_IP}:.*'
        target_label: vm
        replacement: 'loadgen'
      - source_labels: [__address__]
        regex: '${MONITORING_IP}:.*'
        target_label: vm
        replacement: 'monitoring'

  # Cost exporter (runs on this VM).
  - job_name: 'cost-exporter'
    static_configs:
      - targets: ['localhost:9200']
PROMEOF

# ---------------------------------------------------------------------------
# Generate Grafana datasource provisioning
# ---------------------------------------------------------------------------
cat > "${DEPLOY_DIR}/grafana/provisioning/datasources/prometheus.yml" << 'GRAFANAEOF'
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: true
GRAFANAEOF

# ---------------------------------------------------------------------------
# Generate docker-compose.yml for the monitoring stack
# ---------------------------------------------------------------------------
cat > "${DEPLOY_DIR}/docker-compose.yml" << 'COMPOSEEOF'
version: "3.8"

services:
  prometheus:
    image: prom/prometheus:v2.51.0
    container_name: prometheus
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --storage.tsdb.retention.time=30d
      - --web.enable-lifecycle

  grafana:
    image: grafana/grafana:10.4.1
    container_name: grafana
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      GF_SECURITY_ADMIN_USER: admin
      GF_SECURITY_ADMIN_PASSWORD: admin
      GF_AUTH_ANONYMOUS_ENABLED: "true"
      GF_AUTH_ANONYMOUS_ORG_ROLE: Viewer
    volumes:
      - grafana_data:/var/lib/grafana
      - ./grafana/provisioning:/etc/grafana/provisioning:ro
    depends_on:
      - prometheus

  cost-exporter:
    image: golang:1.22-alpine
    container_name: cost-exporter
    restart: unless-stopped
    ports:
      - "9200:9200"
    environment:
      PROMETHEUS_URL: http://prometheus:9090
      LISTEN_ADDR: ":9200"
    volumes:
      - ./cost-exporter:/app:ro
    working_dir: /app
    command: ["go", "run", "main.go"]
    depends_on:
      - prometheus

  node-exporter:
    image: prom/node-exporter:v1.7.0
    container_name: node-exporter
    restart: unless-stopped
    ports:
      - "9100:9100"
    pid: host
    network_mode: host
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /:/rootfs:ro
    command:
      - --path.procfs=/host/proc
      - --path.sysfs=/host/sys
      - --path.rootfs=/rootfs

volumes:
  prometheus_data:
  grafana_data:
COMPOSEEOF

# ---------------------------------------------------------------------------
# Copy cost-exporter source
# ---------------------------------------------------------------------------
echo "Setting up cost-exporter..."
mkdir -p "${DEPLOY_DIR}/cost-exporter"

# The cost-exporter Go source should be scp'd separately. Create a go.mod if needed.
if [ ! -f "${DEPLOY_DIR}/cost-exporter/main.go" ]; then
    echo "NOTE: cost-exporter source not found at ${DEPLOY_DIR}/cost-exporter/main.go"
    echo "Copy it: scp eval/scripts/cost-exporter/* root@<IP>:${DEPLOY_DIR}/cost-exporter/"
fi

if [ ! -f "${DEPLOY_DIR}/cost-exporter/go.mod" ]; then
    cat > "${DEPLOY_DIR}/cost-exporter/go.mod" << 'GOMODEOF'
module cost-exporter

go 1.22

require github.com/prometheus/client_golang v1.19.0

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	golang.org/x/sys v0.16.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)
GOMODEOF
fi

# ---------------------------------------------------------------------------
# Start monitoring stack
# ---------------------------------------------------------------------------
echo "Starting monitoring stack..."
cd "$DEPLOY_DIR"
docker compose pull
docker compose up -d

echo "Waiting for services to start (15s)..."
sleep 15
docker compose ps

echo ""
echo "=== Monitoring VM setup complete ==="
echo "Prometheus: http://${MONITORING_IP}:9090"
echo "Grafana:    http://${MONITORING_IP}:3000 (admin/admin)"
echo "Cost exp:   http://${MONITORING_IP}:9200/metrics"
echo "Node exp:   http://${MONITORING_IP}:9100/metrics"
