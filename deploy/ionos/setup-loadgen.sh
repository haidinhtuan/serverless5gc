#!/bin/bash
# Sets up the load generator VM: UERANSIM from source.
# Run on the loadgen VM via: ssh vm4 'bash -s' < deploy/ionos/setup-loadgen.sh

set -euo pipefail

echo "=== Setting up Load Generator VM (UERANSIM) ==="

# Update system.
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq && apt-get upgrade -y -qq

# Install build dependencies for UERANSIM.
echo "Installing UERANSIM build dependencies..."
apt-get install -y -qq \
    build-essential \
    cmake \
    git \
    libsctp-dev \
    lksctp-tools \
    iproute2 \
    wget

# Install a recent CMake if the distro version is too old.
CMAKE_VERSION=$(cmake --version 2>/dev/null | head -1 | awk '{print $3}' || echo "0.0.0")
REQUIRED_CMAKE="3.17.0"
if [ "$(printf '%s\n' "$REQUIRED_CMAKE" "$CMAKE_VERSION" | sort -V | head -n1)" != "$REQUIRED_CMAKE" ]; then
    echo "Upgrading CMake..."
    wget -q https://github.com/Kitware/CMake/releases/download/v3.28.3/cmake-3.28.3-linux-x86_64.sh -O /tmp/cmake.sh
    chmod +x /tmp/cmake.sh
    /tmp/cmake.sh --prefix=/usr/local --skip-license
fi

# Clone and build UERANSIM.
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

# Create symlinks for convenience.
ln -sf "${UERANSIM_DIR}/build/nr-gnb" /usr/local/bin/nr-gnb
ln -sf "${UERANSIM_DIR}/build/nr-ue" /usr/local/bin/nr-ue
ln -sf "${UERANSIM_DIR}/build/nr-cli" /usr/local/bin/nr-cli

# Create config directory for scenarios.
mkdir -p /opt/ueransim-configs

# Install node-exporter for monitoring.
echo "Installing node-exporter..."
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

echo "=== Load Generator VM setup complete ==="
echo "UERANSIM binaries: nr-gnb, nr-ue, nr-cli"
