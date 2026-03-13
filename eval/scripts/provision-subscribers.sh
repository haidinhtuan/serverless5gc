#!/bin/bash
# Provisions 1000 subscribers in Redis via the udr-data-write OpenFaaS function.
#
# Usage: ./provision-subscribers.sh <serverless_ip>
#
# Subscriber parameters (matching UERANSIM config):
#   IMSI: imsi-001010000000001 to imsi-001010000001000
#   K:    465B5CE8B199B49FAA5F0A2EE238A6BC
#   OPc:  E8ED289DEBA952E4283B54E88E6183CA
#   AMF:  8000
#   SQN:  000000000020 (hex, = 32 decimal)

set -euo pipefail

SERVERLESS_IP="${1:?Usage: $0 <serverless_ip>}"
GATEWAY="http://${SERVERLESS_IP}:31113/function/udr-data-write"
TOTAL=1000
BATCH=50

# Base64-encode the auth parameters (these are hex strings encoded as bytes)
K_B64=$(echo -n "465B5CE8B199B49FAA5F0A2EE238A6BC" | xxd -r -p | base64 -w0)
OPC_B64=$(echo -n "E8ED289DEBA952E4283B54E88E6183CA" | xxd -r -p | base64 -w0)
AMF_B64=$(echo -n "8000" | xxd -r -p | base64 -w0)
SQN_B64=$(echo -n "000000000020" | xxd -r -p | base64 -w0)

echo "Provisioning ${TOTAL} subscribers via ${GATEWAY}..."
echo "K (b64): ${K_B64}"
echo "OPc (b64): ${OPC_B64}"

SUCCESS=0
FAIL=0

for i in $(seq 1 $TOTAL); do
    SUPI=$(printf "imsi-001010%09d" "$i")
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$GATEWAY" \
        -H "Content-Type: application/json" \
        -d "{
            \"supi\": \"${SUPI}\",
            \"authenticationSubscription\": {
                \"authenticationMethod\": \"5G_AKA\",
                \"permanentKey\": {\"permanentKeyValue\": \"${K_B64}\"},
                \"milenage\": {\"op\": {\"opValue\": \"${OPC_B64}\", \"opType\": \"OPC\"}},
                \"authenticationManagementField\": \"${AMF_B64}\",
                \"sequenceNumber\": \"${SQN_B64}\"
            },
            \"accessAndMobilitySubscriptionData\": {
                \"subscribedNssai\": [{\"sst\": 1, \"sd\": \"010203\"}],
                \"subscribedDnn\": [\"internet\"]
            }
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
