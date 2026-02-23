#!/bin/bash
# Sets up VM1 (serverless5gc): K3s, Helm, OpenFaaS, Redis, etcd, cAdvisor, node-exporter.
# Run via: ssh root@VM1 'bash -s' < setup-serverless.sh
#
# Note: Function images must be pushed to a registry before deploying the stack.

set -euo pipefail

echo "=== Setting up Serverless 5GC VM ==="

export DEBIAN_FRONTEND=noninteractive

# ---------------------------------------------------------------------------
# System update
# ---------------------------------------------------------------------------
echo "Updating system packages..."
apt-get update -qq
apt-get upgrade -y -qq

# ---------------------------------------------------------------------------
# Install K3s (single-node, no traefik)
# ---------------------------------------------------------------------------
echo "Installing K3s..."
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable traefik" sh -
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
echo 'export KUBECONFIG=/etc/rancher/k3s/k3s.yaml' >> /root/.bashrc

echo "Waiting for K3s node to be Ready..."
for i in $(seq 1 60); do
    if kubectl get nodes 2>/dev/null | grep -q " Ready"; then
        echo "K3s is ready."
        break
    fi
    if [ "$i" -eq 60 ]; then
        echo "ERROR: K3s node not ready after 5 minutes."
        exit 1
    fi
    sleep 5
done

# ---------------------------------------------------------------------------
# Install Helm
# ---------------------------------------------------------------------------
echo "Installing Helm..."
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# ---------------------------------------------------------------------------
# Install OpenFaaS via Helm
# ---------------------------------------------------------------------------
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
    --wait --timeout 300s

echo "Waiting for OpenFaaS gateway..."
kubectl rollout status deployment/gateway -n openfaas --timeout=120s

# Install faas-cli.
curl -sLSf https://cli.openfaas.com | sh

# ---------------------------------------------------------------------------
# Deploy Redis pod + service in openfaas-fn namespace
# ---------------------------------------------------------------------------
echo "Deploying Redis..."
kubectl run redis \
    --image=redis:7-alpine \
    --port=6379 \
    -n openfaas-fn \
    --restart=Always
kubectl expose pod redis --port=6379 -n openfaas-fn

# ---------------------------------------------------------------------------
# Deploy etcd pod + service in openfaas-fn namespace
# ---------------------------------------------------------------------------
echo "Deploying etcd..."
kubectl run etcd \
    --image=quay.io/coreos/etcd:v3.5.17 \
    --port=2379 \
    -n openfaas-fn \
    --restart=Always \
    -- etcd \
    --advertise-client-urls=http://0.0.0.0:2379 \
    --listen-client-urls=http://0.0.0.0:2379
kubectl expose pod etcd --port=2379 -n openfaas-fn

echo "Waiting for Redis and etcd to be ready..."
kubectl wait --for=condition=Ready pod/redis -n openfaas-fn --timeout=120s
kubectl wait --for=condition=Ready pod/etcd -n openfaas-fn --timeout=120s

# ---------------------------------------------------------------------------
# Install cAdvisor (runs as a K3s static pod is not possible, use containerd-based approach)
# ---------------------------------------------------------------------------
echo "Installing cAdvisor via kubectl..."
kubectl apply -f - <<'CADVISOR_EOF'
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: cadvisor
  namespace: kube-system
  labels:
    app: cadvisor
spec:
  selector:
    matchLabels:
      app: cadvisor
  template:
    metadata:
      labels:
        app: cadvisor
    spec:
      hostNetwork: true
      containers:
      - name: cadvisor
        image: gcr.io/cadvisor/cadvisor:v0.49.1
        ports:
        - containerPort: 8081
          hostPort: 8081
        args:
        - --port=8081
        volumeMounts:
        - name: rootfs
          mountPath: /rootfs
          readOnly: true
        - name: var-run
          mountPath: /var/run
          readOnly: true
        - name: sys
          mountPath: /sys
          readOnly: true
        - name: containerd
          mountPath: /var/lib/containerd
          readOnly: true
        - name: disk
          mountPath: /dev/disk
          readOnly: true
        securityContext:
          privileged: true
        resources:
          requests:
            cpu: 100m
            memory: 200Mi
          limits:
            cpu: 300m
            memory: 500Mi
      volumes:
      - name: rootfs
        hostPath:
          path: /
      - name: var-run
        hostPath:
          path: /var/run
      - name: sys
        hostPath:
          path: /sys
      - name: containerd
        hostPath:
          path: /var/lib/rancher/k3s/agent/containerd
      - name: disk
        hostPath:
          path: /dev/disk
      tolerations:
      - effect: NoSchedule
        operator: Exists
CADVISOR_EOF

# ---------------------------------------------------------------------------
# Install node-exporter as a DaemonSet
# ---------------------------------------------------------------------------
echo "Installing node-exporter via kubectl..."
kubectl apply -f - <<'NODEEXP_EOF'
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-exporter
  namespace: kube-system
  labels:
    app: node-exporter
spec:
  selector:
    matchLabels:
      app: node-exporter
  template:
    metadata:
      labels:
        app: node-exporter
    spec:
      hostNetwork: true
      hostPID: true
      containers:
      - name: node-exporter
        image: prom/node-exporter:v1.7.0
        args:
        - --path.procfs=/host/proc
        - --path.sysfs=/host/sys
        - --path.rootfs=/host/root
        ports:
        - containerPort: 9100
          hostPort: 9100
        volumeMounts:
        - name: proc
          mountPath: /host/proc
          readOnly: true
        - name: sys
          mountPath: /host/sys
          readOnly: true
        - name: root
          mountPath: /host/root
          readOnly: true
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
      volumes:
      - name: proc
        hostPath:
          path: /proc
      - name: sys
        hostPath:
          path: /sys
      - name: root
        hostPath:
          path: /
      tolerations:
      - effect: NoSchedule
        operator: Exists
NODEEXP_EOF

# ---------------------------------------------------------------------------
# Verify
# ---------------------------------------------------------------------------
echo ""
echo "Waiting for monitoring pods..."
sleep 10
kubectl get pods -A

echo ""
echo "=== Serverless 5GC VM setup complete ==="
echo "OpenFaaS gateway: http://localhost:8080"
echo "cAdvisor:         http://localhost:8081/metrics"
echo "node-exporter:    http://localhost:9100/metrics"
echo ""
echo "Note: Push function images to a registry, then deploy with faas-cli."
