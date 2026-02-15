#!/bin/bash
# Runs a load generation scenario against a target system and collects metrics.
#
# Usage: ./run-scenario.sh <scenario> <target> [run_number]
#   scenario: idle | low | medium | high | burst
#   target:   serverless | open5gs | free5gc
#   run_number: 1 (default)
#
# Environment variables:
#   MONITORING_IP  - IP of the Prometheus/monitoring VM
#   LOADGEN_IP     - IP of the load generator VM
#   TARGET_AMF_IP  - IP of the target system's AMF endpoint
#   UERANSIM_DIR   - Path to UERANSIM installation (default: /opt/UERANSIM)

set -euo pipefail

SCENARIO=${1:?Usage: $0 <scenario> <target> [run_number]}
TARGET=${2:?Usage: $0 <scenario> <target> [run_number]}
RUN=${3:-1}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

SCENARIO_FILE="${PROJECT_DIR}/eval/scenarios/${SCENARIO}.yaml"
RESULTS_DIR="${PROJECT_DIR}/eval/results/${TARGET}/${SCENARIO}/run${RUN}"

MONITORING_IP="${MONITORING_IP:?Set MONITORING_IP}"
LOADGEN_IP="${LOADGEN_IP:?Set LOADGEN_IP}"
TARGET_AMF_IP="${TARGET_AMF_IP:?Set TARGET_AMF_IP}"
UERANSIM_DIR="${UERANSIM_DIR:-/opt/UERANSIM}"

if [ ! -f "$SCENARIO_FILE" ]; then
    echo "ERROR: Scenario file not found: ${SCENARIO_FILE}"
    exit 1
fi

mkdir -p "$RESULTS_DIR"

echo "=== Starting scenario: ${SCENARIO} on ${TARGET} (run ${RUN}) ==="
echo "Scenario file: ${SCENARIO_FILE}"
echo "Results dir:   ${RESULTS_DIR}"

# Parse scenario config.
UE_COUNT=$(grep 'count:' "$SCENARIO_FILE" | head -1 | awk '{print $2}')
REG_RATE=$(grep 'registration_rate_per_sec:' "$SCENARIO_FILE" | head -1 | awk '{print $2}')
DURATION=$(grep 'duration_minutes:' "$SCENARIO_FILE" | head -1 | awk '{print $2}')
PDU_SESSIONS=$(grep 'pdu_sessions_per_ue:' "$SCENARIO_FILE" | head -1 | awk '{print $2}')

echo "UEs: ${UE_COUNT}, Rate: ${REG_RATE}/s, Duration: ${DURATION}min, PDU sessions/UE: ${PDU_SESSIONS}"

# Record start time.
START_TIME=$(date -Iseconds)
echo "$START_TIME" > "${RESULTS_DIR}/start_time"

# Generate UERANSIM gNB config.
GNB_CONFIG="${RESULTS_DIR}/gnb.yaml"
cat > "$GNB_CONFIG" << GNBEOF
mcc: '001'
mnc: '01'
nci: '0x000000010'
idLength: 32
tac: 1
linkIp: ${LOADGEN_IP}
ngapIp: ${LOADGEN_IP}
gtpIp: ${LOADGEN_IP}
amfConfigs:
  - address: ${TARGET_AMF_IP}
    port: 38412
slices:
  - sst: 1
    sd: 0x010203
ignoreStreamIds: true
GNBEOF

# Generate UERANSIM UE config.
UE_CONFIG="${RESULTS_DIR}/ue.yaml"
cat > "$UE_CONFIG" << UEEOF
supi: 'imsi-001010000000001'
mcc: '001'
mnc: '01'
key: '465B5CE8B199B49FAA5F0A2EE238A6BC'
op: 'E8ED289DEBA952E4283B54E88E6183CA'
opType: 'OPC'
amf: '8000'
gnbSearchList:
  - ${LOADGEN_IP}
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

if [ "$UE_COUNT" -eq 0 ]; then
    echo "Idle scenario: no UEs to start, just collecting baseline metrics."
    sleep $((DURATION * 60))
else
    # Start gNB.
    echo "Starting gNB..."
    ssh "root@${LOADGEN_IP}" \
        "${UERANSIM_DIR}/build/nr-gnb -c /dev/stdin" < "$GNB_CONFIG" \
        > "${RESULTS_DIR}/gnb.log" 2>&1 &
    GNB_PID=$!
    sleep 5

    # Start UEs with rate limiting.
    echo "Starting ${UE_COUNT} UEs at ${REG_RATE}/s..."
    STARTED=0
    while [ "$STARTED" -lt "$UE_COUNT" ]; do
        BATCH=$REG_RATE
        REMAINING=$((UE_COUNT - STARTED))
        if [ "$BATCH" -gt "$REMAINING" ]; then
            BATCH=$REMAINING
        fi

        for i in $(seq 1 "$BATCH"); do
            IMSI_NUM=$((STARTED + i))
            IMSI=$(printf "imsi-001010%09d" "$IMSI_NUM")
            ssh "root@${LOADGEN_IP}" \
                "sed 's/imsi-001010000000001/${IMSI}/' /dev/stdin | ${UERANSIM_DIR}/build/nr-ue -c /dev/stdin" \
                < "$UE_CONFIG" \
                >> "${RESULTS_DIR}/ue.log" 2>&1 &
        done

        STARTED=$((STARTED + BATCH))
        echo "  Started ${STARTED}/${UE_COUNT} UEs"
        sleep 1
    done

    echo "All UEs started. Waiting for ${DURATION} minutes..."
    sleep $((DURATION * 60))

    # Stop UEs and gNB.
    echo "Stopping UERANSIM..."
    ssh "root@${LOADGEN_IP}" "killall nr-ue nr-gnb 2>/dev/null || true"
    wait "$GNB_PID" 2>/dev/null || true
fi

# Record end time.
END_TIME=$(date -Iseconds)
echo "$END_TIME" > "${RESULTS_DIR}/end_time"

# Collect Prometheus metrics for the test window.
echo "Collecting metrics from Prometheus..."
PROM_URL="http://${MONITORING_IP}:9090"

METRICS=(
    "gateway_function_invocation_total"
    "gateway_functions_seconds_sum"
    "gateway_functions_seconds_count"
    "gateway_functions_seconds_bucket"
    "container_cpu_usage_seconds_total"
    "container_memory_usage_bytes"
    "serverless5gc_function_cost_usd"
    "serverless5gc_total_cost_serverless_usd"
    "serverless5gc_total_cost_traditional_usd"
    "node_cpu_seconds_total"
    "node_memory_MemAvailable_bytes"
)

for METRIC in "${METRICS[@]}"; do
    echo "  Querying ${METRIC}..."
    curl -s "${PROM_URL}/api/v1/query_range?query=${METRIC}&start=${START_TIME}&end=${END_TIME}&step=5s" \
        > "${RESULTS_DIR}/${METRIC}.json" || echo "  WARNING: Failed to query ${METRIC}"
done

# Save scenario metadata.
cat > "${RESULTS_DIR}/metadata.json" << METAEOF
{
    "scenario": "${SCENARIO}",
    "target": "${TARGET}",
    "run": ${RUN},
    "start_time": "${START_TIME}",
    "end_time": "${END_TIME}",
    "ue_count": ${UE_COUNT},
    "registration_rate": ${REG_RATE},
    "pdu_sessions_per_ue": ${PDU_SESSIONS},
    "duration_minutes": ${DURATION}
}
METAEOF

echo "=== Scenario complete. Results in ${RESULTS_DIR} ==="
