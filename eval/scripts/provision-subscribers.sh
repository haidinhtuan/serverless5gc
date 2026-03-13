#!/bin/bash
# Provisions subscribers in Redis via the udr-data-write OpenFaaS function.
#
# Usage: ./provision-subscribers.sh <serverless_ip> [count]
#
# Subscriber parameters (matching UERANSIM config):
#   IMSI range: imsi-001010000000001 to imsi-001010000001000
#   K:    465B5CE8B199B49FAA5F0A2EE238A6BC
#   OPc:  E8ED289DEBA952E4283B54E88E6183CA
#   AMF:  8000
#   SQN:  000000000020 (hex, = 32 decimal)

set -euo pipefail

SERVERLESS_IP="${1:?Usage: $0 <serverless_ip> [count]}"
TOTAL="${2:-1000}"
GATEWAY="http://${SERVERLESS_IP}:31112/function/udr-data-write"
BATCH=200

# Auth parameters as base64 (byte arrays in Go JSON)
K_B64=$(echo -n "465B5CE8B199B49FAA5F0A2EE238A6BC" | xxd -r -p | base64 -w0)
OPC_B64=$(echo -n "E8ED289DEBA952E4283B54E88E6183CA" | xxd -r -p | base64 -w0)
AMF_B64=$(echo -n "8000" | xxd -r -p | base64 -w0)
SQN_B64=$(echo -n "000000000020" | xxd -r -p | base64 -w0)

echo "Provisioning ${TOTAL} subscribers via ${GATEWAY}..."

SUCCESS=0
FAIL=0

for i in $(seq 1 $TOTAL); do
    SUPI=$(printf "imsi-001010%09d" "$i")
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$GATEWAY" \
        -H "Content-Type: application/json" \
        -d "{
            \"supi\": \"${SUPI}\",
            \"auth_data\": {
                \"auth_method\": \"5G_AKA\",
                \"k\": \"${K_B64}\",
                \"opc\": \"${OPC_B64}\",
                \"amf\": \"${AMF_B64}\",
                \"sqn\": \"${SQN_B64}\"
            },
            \"access_mobility_data\": {
                \"nssai\": [{\"sst\": 1, \"sd\": \"010203\"}],
                \"default_dnn\": \"internet\"
            },
            \"session_management\": [{
                \"snssai\": {\"sst\": 1, \"sd\": \"010203\"},
                \"dnn\": \"internet\",
                \"qos_ref\": 9
            }]
        }")

    if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
        SUCCESS=$((SUCCESS + 1))
    else
        FAIL=$((FAIL + 1))
        echo "  FAIL: ${SUPI} -> HTTP ${HTTP_CODE}"
    fi

    if [ $((i % BATCH)) -eq 0 ]; then
        echo "  Progress: ${i}/${TOTAL} (${SUCCESS} ok, ${FAIL} fail)"
    fi
done

echo "Done: ${SUCCESS} success, ${FAIL} fail out of ${TOTAL}"
