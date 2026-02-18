#!/bin/bash
# Runs a load generation scenario against a target system and collects metrics.
#
# For serverless target: Uses HTTP load testing against OpenFaaS functions.
# For open5gs/free5gc:   Uses UERANSIM (Docker) with SCTP/NGAP.
#
# Usage: ./run-scenario.sh <scenario> <target> [run_number]
#   scenario: idle | low | medium | high | burst
#   target:   serverless | open5gs | free5gc
#   run_number: 1 (default)
#
# Environment variables:
#   MONITORING_IP  - IP of the Prometheus VM (or loadgen if co-located)
#   LOADGEN_IP     - IP of the load generator VM
#   TARGET_AMF_IP  - IP of the target system's AMF endpoint

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

UERANSIM_IMAGE="${UERANSIM_IMAGE:-openverso/ueransim:3.2.6}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"
# OpenFaaS gateway NodePort on the serverless VM.
OPENFAAS_PORT="${OPENFAAS_PORT:-31113}"

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

# ---------------------------------------------------------------------------
# Load generation: serverless (HTTP) vs traditional (UERANSIM)
# ---------------------------------------------------------------------------
if [ "$UE_COUNT" -eq 0 ]; then
    echo "Idle scenario: no UEs to start, just collecting baseline metrics."
    sleep $((DURATION * 60))

elif [ "$TARGET" = "serverless" ]; then
    # HTTP load testing against OpenFaaS functions.
    GATEWAY_URL="http://${TARGET_AMF_IP}:${OPENFAAS_PORT}/function"

    echo "Mode: HTTP load testing against serverless functions"
    echo "Gateway: ${GATEWAY_URL}"

    # Install hey on loadgen if not present.
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" '
        if ! command -v hey &>/dev/null; then
            apt-get install -y -qq golang-go 2>/dev/null
            go install github.com/rakyll/hey@latest 2>/dev/null
            cp ~/go/bin/hey /usr/local/bin/ 2>/dev/null || true
        fi
        which hey || echo "hey not found, using curl"
    ' 2>/dev/null

    # Generate load: simulate UE registration + PDU session procedures.
    # Each "UE" triggers: 1x registration + N PDU sessions.
    # Rate-limit to REG_RATE registrations/sec.
    TOTAL_CALLS=$((UE_COUNT + UE_COUNT * PDU_SESSIONS))
    DURATION_SECS=$((DURATION * 60))

    echo "Generating HTTP load: ${UE_COUNT} registrations + ${PDU_SESSIONS} PDU sessions each"
    echo "Total function calls: ~${TOTAL_CALLS}, Duration: ${DURATION_SECS}s"

    # Run load generation on the loadgen VM via SSH.
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" "bash -s" << LOADEOF > "${RESULTS_DIR}/loadgen.log" 2>&1 &
LOADGEN_PID=\$\$
GATEWAY="${GATEWAY_URL}"
UE_COUNT=${UE_COUNT}
REG_RATE=${REG_RATE}
PDU_SESSIONS=${PDU_SESSIONS}
DURATION=${DURATION_SECS}

echo "[\$(date +%H:%M:%S)] Starting HTTP load generation..."
echo "  Gateway: \${GATEWAY}"
echo "  UEs: \${UE_COUNT}, Rate: \${REG_RATE}/s, PDU: \${PDU_SESSIONS}/UE"

STARTED=0
START_TS=\$(date +%s)

while [ \$STARTED -lt \$UE_COUNT ]; do
    BATCH=\$REG_RATE
    REMAINING=\$((UE_COUNT - STARTED))
    [ \$BATCH -gt \$REMAINING ] && BATCH=\$REMAINING

    for i in \$(seq 1 \$BATCH); do
        IMSI_NUM=\$((STARTED + i))
        SUPI=\$(printf "imsi-001010%09d" \$IMSI_NUM)

        # Registration request
        curl -s -X POST "\${GATEWAY}/amf-initial-registration" \
            -H "Content-Type: application/json" \
            -d "{\"supi\":\"\${SUPI}\",\"ran_ue_ngap_id\":\${IMSI_NUM},\"registration_type\":1}" \
            -o /dev/null -w "%{http_code} %{time_total}s\n" >> /tmp/s5gc-eval/reg-results.txt &

        # PDU session requests
        for p in \$(seq 1 \$PDU_SESSIONS); do
            curl -s -X POST "\${GATEWAY}/smf-pdu-session-create" \
                -H "Content-Type: application/json" \
                -d "{\"supi\":\"\${SUPI}\",\"pdu_session_id\":\${p},\"dnn\":\"internet\",\"snssai\":{\"sst\":1,\"sd\":\"010203\"}}" \
                -o /dev/null -w "%{http_code} %{time_total}s\n" >> /tmp/s5gc-eval/pdu-results.txt &
        done
    done

    STARTED=\$((STARTED + BATCH))
    echo "  [\$(date +%H:%M:%S)] Sent \${STARTED}/\${UE_COUNT} UE registrations"
    sleep 1
done

echo "[\$(date +%H:%M:%S)] All registrations sent. Waiting for remaining duration..."

# Wait for the full scenario duration.
NOW=\$(date +%s)
ELAPSED=\$((NOW - START_TS))
REMAIN=\$((DURATION - ELAPSED))
[ \$REMAIN -gt 0 ] && sleep \$REMAIN

# Wait for background curls to finish.
wait

echo "[\$(date +%H:%M:%S)] Load generation complete."
echo "Registration results:"
wc -l /tmp/s5gc-eval/reg-results.txt 2>/dev/null || echo "  No reg results"
echo "PDU session results:"
wc -l /tmp/s5gc-eval/pdu-results.txt 2>/dev/null || echo "  No PDU results"
LOADEOF

    LOAD_PID=$!
    echo "Load generation running (PID: ${LOAD_PID}). Waiting ${DURATION} minutes..."
    sleep $((DURATION * 60))

    # Retrieve results from loadgen.
    wait "$LOAD_PID" 2>/dev/null || true
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no \
        "root@${LOADGEN_IP}:/tmp/s5gc-eval/reg-results.txt" "${RESULTS_DIR}/reg-results.txt" 2>/dev/null || true
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no \
        "root@${LOADGEN_IP}:/tmp/s5gc-eval/pdu-results.txt" "${RESULTS_DIR}/pdu-results.txt" 2>/dev/null || true

    # Clean up on loadgen.
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" \
        "rm -f /tmp/s5gc-eval/reg-results.txt /tmp/s5gc-eval/pdu-results.txt" 2>/dev/null || true

else
    # UERANSIM mode for open5gs / free5gc targets.
    echo "Mode: UERANSIM (SCTP/NGAP) against ${TARGET}"

    # Generate UERANSIM configs.
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

    UE_CONFIG="${RESULTS_DIR}/ue.yaml"
    cat > "$UE_CONFIG" << UEEOF
supi: 'imsi-001010000000001'
mcc: '001'
mnc: '01'
key: '465B5CE8B199B49FAA5F0A2EE238A6BC'
op: 'E8ED289DEBA952E4283B54E88E6183CA'
opType: 'OPC'
amf: '8000'
imei: '356938035643803'
imeiSv: '4370816125816151'
gnbSearchList:
  - ${LOADGEN_IP}
uacAic:
  mps: false
  mcs: false
uacAcc:
  normalClass: 0
  class11: false
  class12: false
  class13: false
  class14: false
  class15: false
sessions:
  - type: 'IPv4'
    apn: 'internet'
    slice:
      sst: 1
      sd: 0x010203
    emergency: false
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
integrityMaxRate:
  uplink: 'full'
  downlink: 'full'
UEEOF

    # Copy config files to loadgen VM.
    echo "Copying configs to loadgen..."
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" "mkdir -p /tmp/s5gc-eval"
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no "$GNB_CONFIG" "root@${LOADGEN_IP}:/tmp/s5gc-eval/gnb.yaml"
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no "$UE_CONFIG" "root@${LOADGEN_IP}:/tmp/s5gc-eval/ue.yaml"

    # Start gNB via Docker.
    echo "Starting gNB..."
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" \
        "docker rm -f s5gc-gnb 2>/dev/null; \
         docker run -d --name s5gc-gnb --network host \
           --entrypoint nr-gnb \
           -v /tmp/s5gc-eval:/config:ro \
           ${UERANSIM_IMAGE} -c /config/gnb.yaml" \
        > "${RESULTS_DIR}/gnb.log" 2>&1
    sleep 5

    # Verify gNB is running.
    if ! ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" \
        "docker ps --filter name=s5gc-gnb --format '{{.Status}}'" 2>/dev/null | grep -q "Up"; then
        echo "ERROR: gNB failed to start"
        ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" \
            "docker logs s5gc-gnb 2>&1" >> "${RESULTS_DIR}/gnb.log" 2>&1
        exit 1
    fi

    # Start UEs via Docker. Split into batches of BATCH_SIZE to avoid
    # UERANSIM single-instance limits (crashes above ~200 UEs per gNB
    # when started simultaneously). Each batch gets a separate container
    # with a distinct IMSI range and a stagger delay between batches.
    BATCH_SIZE=100
    BATCH_STAGGER=20  # seconds between batches
    NUM_BATCHES=$(( (UE_COUNT + BATCH_SIZE - 1) / BATCH_SIZE ))

    echo "Starting ${UE_COUNT} UEs in ${NUM_BATCHES} batches of ${BATCH_SIZE}..."

    # Clean up any leftover UE containers.
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" \
        "for i in \$(seq 1 ${NUM_BATCHES}); do docker rm -f s5gc-ue\${i} 2>/dev/null; done" 2>/dev/null || true

    for BATCH in $(seq 1 "$NUM_BATCHES"); do
        BATCH_START=$(( (BATCH - 1) * BATCH_SIZE + 1 ))
        BATCH_COUNT=$BATCH_SIZE
        REMAINING=$((UE_COUNT - (BATCH - 1) * BATCH_SIZE))
        [ "$BATCH_COUNT" -gt "$REMAINING" ] && BATCH_COUNT=$REMAINING
        SUPI=$(printf "imsi-001010%09d" "$BATCH_START")

        # Generate per-batch UE config with unique starting IMSI.
        BATCH_UE_CONFIG="${RESULTS_DIR}/ue-batch${BATCH}.yaml"
        sed "s/^supi: .*/supi: '${SUPI}'/" "$UE_CONFIG" > "$BATCH_UE_CONFIG"
        scp -i "$SSH_KEY" -o StrictHostKeyChecking=no \
            "$BATCH_UE_CONFIG" "root@${LOADGEN_IP}:/tmp/s5gc-eval/ue-batch${BATCH}.yaml"

        echo "  Batch ${BATCH}/${NUM_BATCHES}: ${BATCH_COUNT} UEs from ${SUPI}"
        ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" \
            "docker run -d --name s5gc-ue${BATCH} --network host \
               --entrypoint nr-ue \
               -v /tmp/s5gc-eval:/config:ro \
               ${UERANSIM_IMAGE} -c /config/ue-batch${BATCH}.yaml -n ${BATCH_COUNT}" \
            > /dev/null 2>&1

        # Stagger between batches to avoid gNB overload.
        [ "$BATCH" -lt "$NUM_BATCHES" ] && sleep "$BATCH_STAGGER"
    done

    echo "All UEs started. Waiting for ${DURATION} minutes..."
    sleep $((DURATION * 60))

    # Stop UEs and gNB, collect logs.
    echo "Stopping UERANSIM..."
    ssh -i "$SSH_KEY" -o StrictHostKeyChecking=no "root@${LOADGEN_IP}" \
        "docker logs s5gc-gnb > /tmp/s5gc-eval/gnb-full.log 2>&1; \
         > /tmp/s5gc-eval/ue-full.log; \
         for i in \$(seq 1 ${NUM_BATCHES}); do \
           echo '=== UE Batch '\$i' ===' >> /tmp/s5gc-eval/ue-full.log; \
           docker logs s5gc-ue\${i} >> /tmp/s5gc-eval/ue-full.log 2>&1; \
         done; \
         for i in \$(seq 1 ${NUM_BATCHES}); do docker rm -f s5gc-ue\${i} 2>/dev/null; done; \
         docker rm -f s5gc-gnb 2>/dev/null || true"
    # Retrieve logs.
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no \
        "root@${LOADGEN_IP}:/tmp/s5gc-eval/gnb-full.log" "${RESULTS_DIR}/gnb.log" 2>/dev/null || true
    scp -i "$SSH_KEY" -o StrictHostKeyChecking=no \
        "root@${LOADGEN_IP}:/tmp/s5gc-eval/ue-full.log" "${RESULTS_DIR}/ue.log" 2>/dev/null || true
fi

# Record end time.
END_TIME=$(date -Iseconds)
echo "$END_TIME" > "${RESULTS_DIR}/end_time"

# ---------------------------------------------------------------------------
# Collect Prometheus metrics for the test window.
# ---------------------------------------------------------------------------
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
    curl -s -G "${PROM_URL}/api/v1/query_range" --data-urlencode "query=${METRIC}" --data-urlencode "start=${START_TIME}" --data-urlencode "end=${END_TIME}" --data-urlencode "step=5s" \
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
