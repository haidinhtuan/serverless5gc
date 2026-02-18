#!/bin/bash
# Sets up a fresh Ubuntu 24.04 VM for the free5GC v4.2.0 baseline.
#
# Prerequisites:
#   - Ubuntu 24.04 LTS
#   - Root access
#   - Internet connectivity
#
# What this script installs:
#   - Docker CE
#   - gtp5g kernel module v0.9.5 (required by free5GC UPF v4.x)
#   - node_exporter (Prometheus metrics)
#   - cAdvisor (container metrics)
#   - Self-signed TLS certificates (required by CHF)
#
# After running this script:
#   1. Copy config files:  scp -r config/ cert/ docker-compose.yml root@<VM_IP>:/opt/free5gc/
#   2. Start free5GC:      cd /opt/free5gc && docker compose up -d
#   3. Provision subs:     ./provision-subscribers.sh
#
# Usage: ssh root@<VM_IP> 'bash -s' < setup-vm.sh

set -euo pipefail

echo "=== Installing Docker ==="
apt-get update -qq
apt-get install -y -qq ca-certificates curl gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  > /etc/apt/sources.list.d/docker.list
apt-get update -qq
apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-compose-plugin
docker --version

echo "=== Installing gtp5g v0.9.5 ==="
apt-get install -y -qq build-essential linux-headers-$(uname -r) git
cd /tmp
rm -rf gtp5g
git clone -b v0.9.5 https://github.com/free5gc/gtp5g.git
cd gtp5g
make clean && make
make install
modprobe gtp5g
echo "gtp5g" >> /etc/modules-load.d/gtp5g.conf
lsmod | grep gtp5g

echo "=== Installing node_exporter ==="
cd /tmp
curl -sLO https://github.com/prometheus/node_exporter/releases/download/v1.8.2/node_exporter-1.8.2.linux-amd64.tar.gz
tar xzf node_exporter-1.8.2.linux-amd64.tar.gz
cp node_exporter-1.8.2.linux-amd64/node_exporter /usr/local/bin/
cat > /etc/systemd/system/node_exporter.service << 'EOF'
[Unit]
Description=Node Exporter
After=network.target
[Service]
ExecStart=/usr/local/bin/node_exporter
[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now node_exporter

echo "=== Starting cAdvisor ==="
docker run -d --name cadvisor --restart always \
  -p 8081:8080 \
  --volume=/:/rootfs:ro \
  --volume=/var/run:/var/run:ro \
  --volume=/sys:/sys:ro \
  --volume=/var/lib/docker/:/var/lib/docker:ro \
  gcr.io/cadvisor/cadvisor:latest

echo "=== Creating free5GC directory ==="
mkdir -p /opt/free5gc/config /opt/free5gc/cert

echo "=== Generating self-signed TLS certificates ==="
# Required by CHF (Diameter server) and WebUI (billing server).
# Without these, CHF crashes with nil pointer dereference in rf.OpenServer.
cd /opt/free5gc/cert
openssl req -x509 -newkey rsa:2048 -keyout chf.key -out chf.pem \
  -days 365 -nodes -subj "/CN=chf" 2>/dev/null
cp chf.pem nrf.pem
cp chf.key nrf.key

echo "=== Setup complete ==="
echo "Next steps:"
echo "  1. Copy docker-compose.yml and config/ to /opt/free5gc/"
echo "  2. Copy cert/ is already at /opt/free5gc/cert/"
echo "  3. cd /opt/free5gc && docker compose up -d"
echo "  4. Run provision-subscribers.sh"
