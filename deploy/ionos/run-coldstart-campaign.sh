#!/bin/bash
# Runs the full cold-start storm experiment campaign:
# 4 scenarios × 3 runs = 12 total runs.
#
# Usage: ./run-coldstart-campaign.sh
#
# Requires: source deploy/ionos/vm-ips-coldstart.env first

set -euo pipefail

SERVERLESS_IP="${SERVERLESS_IP:?Source vm-ips-coldstart.env first}"
LOADGEN_IP="${LOADGEN_IP:?Source vm-ips-coldstart.env first}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
COLDSTART_SCRIPT="${PROJECT_DIR}/eval/scripts/run-coldstart.sh"

SCENARIOS=("low" "medium" "high" "burst")
RUNS=3
COOLDOWN=60  # seconds between runs

export SERVERLESS_IP LOADGEN_IP

TOTAL=$((${#SCENARIOS[@]} * RUNS))
CURRENT=0

echo "=== Cold-Start Storm Campaign ==="
echo "Scenarios: ${SCENARIOS[*]}"
echo "Runs per scenario: ${RUNS}"
echo "Total runs: ${TOTAL}"
echo "Cooldown: ${COOLDOWN}s"
echo ""

for scenario in "${SCENARIOS[@]}"; do
    for run in $(seq 1 $RUNS); do
        CURRENT=$((CURRENT + 1))
        echo ""
        echo "================================================================"
        echo "  Run ${CURRENT}/${TOTAL}: ${scenario} run${run}"
        echo "  $(date)"
        echo "================================================================"

        if ! "$COLDSTART_SCRIPT" "$scenario" "$run"; then
            echo "WARNING: ${scenario}/run${run} FAILED. Continuing..."
        fi

        if [ $CURRENT -lt $TOTAL ]; then
            echo "Cooldown ${COOLDOWN}s..."
            sleep $COOLDOWN
        fi
    done
done

echo ""
echo "=== Campaign complete: ${CURRENT}/${TOTAL} runs ==="
echo "Results: ${PROJECT_DIR}/eval/results/serverless-sctp-coldstart/"
