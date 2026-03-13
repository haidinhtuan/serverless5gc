#!/bin/bash
# Runs a cold-start storm experiment: scales all OpenFaaS functions to 0 replicas,
# then triggers UERANSIM load to measure cold-start latency.
#
# Usage: ./run-coldstart.sh <scenario> [run_number]
#   scenario: low | medium | high | burst
#   run_number: 1 (default)
#
# Environment variables (required):
#   SERVERLESS_IP  - IP of the serverless VM (K3s + OpenFaaS)
#   LOADGEN_IP     - IP of the load generator VM (UERANSIM + Prometheus)

set -euo pipefail

SCENARIO="${1:?Usage: $0 <scenario> [run_number]}"
RUN="${2:-1}"

SERVERLESS_IP="${SERVERLESS_IP:?Set SERVERLESS_IP}"
LOADGEN_IP="${LOADGEN_IP:?Set LOADGEN_IP}"

SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Results go to serverless-sctp-coldstart (not serverless-sctp)
RESULTS_DIR="${PROJECT_DIR}/eval/results/serverless-sctp-coldstart/${SCENARIO}/run${RUN}"
mkdir -p "$RESULTS_DIR"

echo "=== Cold-Start Experiment: ${SCENARIO} run${RUN} ==="
echo "Serverless VM: ${SERVERLESS_IP}"
echo "Loadgen VM:    ${LOADGEN_IP}"
echo "Results:       ${RESULTS_DIR}"

# ---------------------------------------------------------------------------
# Step 1: Get list of all OpenFaaS functions
# ---------------------------------------------------------------------------
echo ""
echo "Step 1: Getting function list..."
FUNCTIONS=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${SERVERLESS_IP}" \
    "faas-cli list --gateway http://localhost:31113 2>/dev/null | tail -n +2 | awk '{print \$1}'")

FUNC_COUNT=$(echo "$FUNCTIONS" | wc -l)
echo "  Found ${FUNC_COUNT} functions"

# Verify all functions are healthy before scaling down
echo "  Verifying all functions are ready..."
READY_COUNT=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${SERVERLESS_IP}" \
    "faas-cli list --gateway http://localhost:31113 2>/dev/null | tail -n +2 | awk '{print \$3}' | grep -c '1' || echo 0")
echo "  ${READY_COUNT}/${FUNC_COUNT} functions have replicas ready"

if [ "$READY_COUNT" -lt "$FUNC_COUNT" ]; then
    echo "  WARNING: Not all functions ready. Scaling up first..."
    for fn in $FUNCTIONS; do
        ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${SERVERLESS_IP}" \
            "faas-cli scale ${fn} --replicas=1 --gateway http://localhost:31113" 2>/dev/null
    done
    echo "  Waiting 30s for functions to become ready..."
    sleep 30
fi

# ---------------------------------------------------------------------------
# Step 2: Scale all functions to 0 replicas
# ---------------------------------------------------------------------------
echo ""
echo "Step 2: Scaling all ${FUNC_COUNT} functions to 0 replicas..."
for fn in $FUNCTIONS; do
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${SERVERLESS_IP}" \
        "faas-cli scale ${fn} --replicas=0 --gateway http://localhost:31113" 2>/dev/null
done
echo "  Scale-to-zero commands sent."

# ---------------------------------------------------------------------------
# Step 3: Wait for all function pods to terminate
# ---------------------------------------------------------------------------
echo ""
echo "Step 3: Waiting for function pods to terminate..."
MAX_WAIT=120
ELAPSED=0
while [ $ELAPSED -lt $MAX_WAIT ]; do
    # Count pods that are NOT redis or etcd (infrastructure pods stay)
    FUNC_PODS=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${SERVERLESS_IP}" \
        "kubectl get pods -n openfaas-fn --no-headers 2>/dev/null | grep -v -E '^(redis|etcd)' | wc -l" 2>/dev/null || echo "0")
    FUNC_PODS=$(echo "$FUNC_PODS" | tr -d '[:space:]')

    if [ "$FUNC_PODS" = "0" ]; then
        echo "  All function pods terminated (${ELAPSED}s)"
        break
    fi
    echo "  ${FUNC_PODS} function pods still running (${ELAPSED}s)..."
    sleep 5
    ELAPSED=$((ELAPSED + 5))
done

if [ "$FUNC_PODS" != "0" ]; then
    echo "  WARNING: ${FUNC_PODS} pods still running after ${MAX_WAIT}s timeout"
fi

# ---------------------------------------------------------------------------
# Step 4: Restart SCTP proxy
# ---------------------------------------------------------------------------
echo ""
echo "Step 4: Restarting SCTP proxy..."
ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${SERVERLESS_IP}" \
    "kill \$(pgrep sctp-proxy) 2>/dev/null || true; sleep 2; \
     OPENFAAS_GATEWAY='http://localhost:31113/function/' REDIS_ADDR='localhost:32660' \
     nohup /usr/local/bin/sctp-proxy > /var/log/sctp-proxy.log 2>&1 &"
sleep 5

# Verify proxy is running
PROXY_PID=$(ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${SERVERLESS_IP}" \
    "pgrep sctp-proxy" 2>/dev/null || echo "")
if [ -z "$PROXY_PID" ]; then
    echo "  ERROR: SCTP proxy failed to start"
    exit 1
fi
echo "  SCTP proxy running (PID: ${PROXY_PID})"

# ---------------------------------------------------------------------------
# Step 5: Run the scenario via existing run-scenario.sh
# ---------------------------------------------------------------------------
echo ""
echo "Step 5: Running scenario ${SCENARIO} with cold-start functions..."

# Set env vars for run-scenario.sh
export MONITORING_IP="${LOADGEN_IP}"  # Prometheus co-located with loadgen
export LOADGEN_IP="${LOADGEN_IP}"
export TARGET_AMF_IP="${SERVERLESS_IP}"

# run-scenario.sh will try to restart sctp-proxy via systemctl (fails silently, ok)
# It handles UERANSIM launch, metric collection, log retrieval
"${SCRIPT_DIR}/run-scenario.sh" "${SCENARIO}" "serverless-sctp" "${RUN}"

# ---------------------------------------------------------------------------
# Step 6: Move results to coldstart directory
# ---------------------------------------------------------------------------
WARM_DIR="${PROJECT_DIR}/eval/results/serverless-sctp/${SCENARIO}/run${RUN}"
if [ -d "$WARM_DIR" ] && [ "$WARM_DIR" != "$RESULTS_DIR" ]; then
    # run-scenario.sh saves to serverless-sctp/; move to serverless-sctp-coldstart/
    echo ""
    echo "Step 6: Moving results to coldstart directory..."
    cp -r "${WARM_DIR}/"* "${RESULTS_DIR}/" 2>/dev/null || true
    rm -rf "${WARM_DIR}"
fi

# Save cold-start metadata
cat > "${RESULTS_DIR}/coldstart-metadata.json" << METAEOF
{
    "experiment": "cold-start-storm",
    "scenario": "${SCENARIO}",
    "run": ${RUN},
    "functions_scaled_to_zero": ${FUNC_COUNT},
    "pod_termination_wait_seconds": ${ELAPSED},
    "warm_start": false
}
METAEOF

echo ""
echo "=== Cold-start experiment complete: ${SCENARIO} run${RUN} ==="
echo "Results: ${RESULTS_DIR}"
