#!/bin/bash
# Deploys the 12-function evaluation subset (stack-eval.yml) to the serverless VM
# on stock OpenFaaS Community Edition, via faas-cli, from public Docker Hub images.
#
# Why a subset: the load campaign exercises only the registration and
# PDU-session-establishment procedures, whose combined call graphs touch 12
# functions -- within the OpenFaaS CE 15-function limit. Deploying only these
# keeps the deployment on stock OpenFaaS CE: faas-cli deploy + the unmodified
# gateway, no workaround.
#
# Why public images: OpenFaaS CE refuses to deploy non-public images via faas-cli
# ("the Community Edition license agreement only allows public images"), so the
# function images are published to a public Docker Hub namespace (REGISTRY,
# default haidinhtuan) and faas-netes pulls them normally.
#
# Workflow:
#   1. (optional) Build + push the 12 images to the public registry  (--build)
#   2. faas-cli deploy -f stack-eval.yml                              (<= 15 functions)
#   3. Restart the gateway (it caches the function count; without this it keeps
#      returning HTTP 405 from a previous over-limit deployment)
#   4. Point at the smoke test
#
# Usage:  ./deploy-eval-subset.sh <serverless-vm-ip> [--build]
#   or:   source ../ionos/vm-ips-meridian.env && ./deploy-eval-subset.sh "$SERVERLESS_IP" --build

set -euo pipefail

VM_IP="${1:?Usage: $0 <serverless-vm-ip> [--build]}"
DO_BUILD=false
[ "${2:-}" = "--build" ] && DO_BUILD=true

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY="${REGISTRY:-haidinhtuan}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"
STACK="${SCRIPT_DIR}/stack-eval.yml"
SSH_OPTS=(-o StrictHostKeyChecking=no -i "$SSH_KEY")

# The functions to deploy, derived from stack-eval.yml.
mapfile -t FUNCS < <(grep -E '^  [a-z0-9-]+:$' "$STACK" | tr -d ' :')
if [ "${#FUNCS[@]}" -gt 15 ]; then
    echo "ERROR: ${#FUNCS[@]} functions exceeds the OpenFaaS CE 15-function limit." >&2
    exit 1
fi
echo "Functions (${#FUNCS[@]}): ${FUNCS[*]}"

if $DO_BUILD; then
    echo "=== Step 1: Build + push ${#FUNCS[@]} images to ${REGISTRY} (public) ==="
    REGISTRY="$REGISTRY" bash "${SCRIPT_DIR}/build-functions.sh" --push --registry "$REGISTRY" "${FUNCS[@]}"
fi

echo "=== Step 2: Copy stack + faas-cli deploy ==="
scp "${SSH_OPTS[@]}" "$STACK" "root@${VM_IP}:/tmp/stack-eval.yml"
ssh "${SSH_OPTS[@]}" "root@${VM_IP}" 'bash -s' <<'DEPLOY'
set -e
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
export OPENFAAS_URL=http://127.0.0.1:31112

# The CE gateway caches its function count; restart so it re-counts and clears
# any stale "over 15 functions" (HTTP 405) state from a prior deployment.
kubectl rollout restart deploy/gateway -n openfaas
kubectl rollout status deploy/gateway -n openfaas --timeout=120s

cd /tmp
faas-cli deploy -f stack-eval.yml

echo "Waiting for function pods to be ready..."
kubectl wait --for=condition=available deploy -n openfaas-fn -l faas_function --timeout=180s
faas-cli list
DEPLOY

echo ""
echo "=== Deployment complete: ${#FUNCS[@]} functions on stock OpenFaaS CE ==="
echo "Smoke-test:  bash ${SCRIPT_DIR}/smoke-eval-subset.sh ${VM_IP}"
