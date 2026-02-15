#!/bin/bash
# Sets up the serverless 5GC VM: K3s, Helm, OpenFaaS, and all K3s manifests.
# Run on the serverless5gc VM via: ssh vm1 'bash -s' < deploy/ionos/setup-serverless.sh

set -euo pipefail

echo "=== Setting up Serverless 5GC VM ==="

# Update system.
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq && apt-get upgrade -y -qq

# Install K3s.
echo "Installing K3s..."
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo 'export KUBECONFIG=/etc/rancher/k3s/k3s.yaml' >> /root/.bashrc

# Wait for K3s to be ready.
echo "Waiting for K3s..."
until kubectl get nodes | grep -q " Ready"; do
    sleep 2
done
echo "K3s is ready."

# Install Helm.
echo "Installing Helm..."
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Install OpenFaaS via Helm.
echo "Installing OpenFaaS..."
kubectl apply -f https://raw.githubusercontent.com/openfaas/faas-netes/master/namespaces.yml

helm repo add openfaas https://openfaas.github.io/faas-netes/
helm repo update

helm upgrade openfaas openfaas/openfaas \
    --install \
    --namespace openfaas \
    --set functionNamespace=openfaas-fn \
    --set basic_auth=false \
    --set gateway.upstreamTimeout=120s \
    --set gateway.writeTimeout=120s \
    --set gateway.readTimeout=120s \
    --set faasnetes.writeTimeout=120s \
    --set faasnetes.readTimeout=120s \
    --set queueWorker.maxInflight=50 \
    --wait

# Wait for OpenFaaS gateway.
echo "Waiting for OpenFaaS gateway..."
kubectl rollout status deployment/gateway -n openfaas --timeout=120s

# Install faas-cli.
curl -sLSf https://cli.openfaas.com | sh

# Deploy K3s manifests (Redis, etcd, UPF, SCTP proxy).
echo "Deploying infrastructure manifests..."
MANIFESTS_DIR="/tmp/serverless5gc-manifests"
mkdir -p "$MANIFESTS_DIR"

# These manifests should be copied to the VM before running this script,
# or pulled from the git repo.
for MANIFEST in redis-deployment.yaml etcd-deployment.yaml upf-deployment.yaml sctp-proxy-deployment.yaml; do
    if [ -f "${MANIFESTS_DIR}/${MANIFEST}" ]; then
        echo "  Applying ${MANIFEST}..."
        kubectl apply -f "${MANIFESTS_DIR}/${MANIFEST}"
    else
        echo "  WARNING: ${MANIFEST} not found in ${MANIFESTS_DIR}, skipping."
    fi
done

# Deploy OpenFaaS functions.
echo "Deploying OpenFaaS functions..."
if [ -f "${MANIFESTS_DIR}/stack.yml" ]; then
    cd "$MANIFESTS_DIR"
    faas-cli deploy -f stack.yml
else
    echo "  WARNING: stack.yml not found, skipping function deployment."
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

echo "=== Serverless 5GC VM setup complete ==="
