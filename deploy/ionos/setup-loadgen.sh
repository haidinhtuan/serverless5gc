#!/bin/bash
# Sets up VM4 (loadgen): UERANSIM from source for 5G UE/gNB simulation.
# Run via: ssh root@VM4 'bash -s' < setup-loadgen.sh
#
# After setup, gNB config templates will be in /opt/ueransim-configs/.
# The run-scenario.sh script fills in the TARGET_AMF_IP at runtime.

set -euo pipefail

echo "=== Setting up Load Generator VM (UERANSIM) ==="

export DEBIAN_FRONTEND=noninteractive

# ---------------------------------------------------------------------------
# System update
# ---------------------------------------------------------------------------
echo "Updating system packages..."
apt-get update -qq
apt-get upgrade -y -qq

# ---------------------------------------------------------------------------
# Install build dependencies
# ---------------------------------------------------------------------------
echo "Installing UERANSIM build dependencies..."
apt-get install -y -qq \
    build-essential \
    cmake \
    git \
    g++ \
    make \
    libsctp-dev \
    lksctp-tools \
    iproute2 \
    wget

# Check CMake version (UERANSIM needs >= 3.17).
CMAKE_VERSION=$(cmake --version 2>/dev/null | head -1 | awk '{print $3}' || echo "0.0.0")
REQUIRED_CMAKE="3.17.0"
if [ "$(printf '%s\n' "$REQUIRED_CMAKE" "$CMAKE_VERSION" | sort -V | head -n1)" != "$REQUIRED_CMAKE" ]; then
    echo "Upgrading CMake (current: $CMAKE_VERSION, required: >= $REQUIRED_CMAKE)..."
    wget -q https://github.com/Kitware/CMake/releases/download/v3.28.3/cmake-3.28.3-linux-x86_64.sh -O /tmp/cmake.sh
    chmod +x /tmp/cmake.sh
    /tmp/cmake.sh --prefix=/usr/local --skip-license
    hash -r
fi

# ---------------------------------------------------------------------------
# Clone and build UERANSIM
# ---------------------------------------------------------------------------
UERANSIM_DIR="/opt/UERANSIM"
UERANSIM_VERSION="v3.2.6"

echo "Cloning UERANSIM ${UERANSIM_VERSION}..."
if [ ! -d "$UERANSIM_DIR" ]; then
    git clone --branch "$UERANSIM_VERSION" --depth 1 \
        https://github.com/aligungr/UERANSIM.git "$UERANSIM_DIR"
fi

echo "Building UERANSIM..."
cd "$UERANSIM_DIR"
make

# Verify binaries.
echo "Verifying UERANSIM binaries..."
ls -la build/nr-gnb build/nr-ue build/nr-cli

# Symlinks for convenience.
ln -sf "${UERANSIM_DIR}/build/nr-gnb" /usr/local/bin/nr-gnb
ln -sf "${UERANSIM_DIR}/build/nr-ue" /usr/local/bin/nr-ue
ln -sf "${UERANSIM_DIR}/build/nr-cli" /usr/local/bin/nr-cli

# ---------------------------------------------------------------------------
# Create gNB config template (TARGET_AMF_IP is a placeholder)
# ---------------------------------------------------------------------------
CONFIGS_DIR="/opt/ueransim-configs"
mkdir -p "$CONFIGS_DIR"

cat > "${CONFIGS_DIR}/gnb-template.yaml" << 'GNBEOF'
mcc: '001'
mnc: '01'
nci: '0x000000010'
idLength: 32
tac: 1
linkIp: __LOADGEN_IP__
ngapIp: __LOADGEN_IP__
gtpIp: __LOADGEN_IP__
amfConfigs:
  - address: __TARGET_AMF_IP__
    port: 38412
slices:
  - sst: 1
    sd: 0x010203
ignoreStreamIds: true
GNBEOF

cat > "${CONFIGS_DIR}/ue-template.yaml" << 'UEEOF'
supi: 'imsi-001010000000001'
mcc: '001'
mnc: '01'
key: '465B5CE8B199B49FAA5F0A2EE238A6BC'
op: 'E8ED289DEBA952E4283B54E88E6183CA'
opType: 'OPC'
amf: '8000'
gnbSearchList:
  - __LOADGEN_IP__
sessions:
  - type: 'IPv4'
    apn: 'internet'
    slice:
      sst: 1
      sd: 0x010203
configured-nssai:
  - sst: 1
    sd: 0x010203
default-nssai:
  - sst: 1
    sd: 0x010203
integrity:
  IA1: true
  IA2: true
  IA3: true
ciphering:
  EA1: true
  EA2: true
  EA3: true
UEEOF

echo "Config templates written to ${CONFIGS_DIR}/"

# ---------------------------------------------------------------------------
# Install Docker + node-exporter for host metrics
# ---------------------------------------------------------------------------
echo "Installing Docker for node-exporter..."
apt-get install -y -qq docker.io
systemctl enable docker
systemctl start docker

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
echo "=== Load Generator VM setup complete ==="
echo "UERANSIM binaries: nr-gnb, nr-ue, nr-cli"
echo "Config templates:  ${CONFIGS_DIR}/"
echo "node-exporter:     http://localhost:9100/metrics"
