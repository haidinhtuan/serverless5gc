#!/bin/bash
# Smoke-tests the evaluation subset: provisions a test subscriber, then runs
# UE registration and PDU session establishment through the real OpenFaaS
# gateway. Exits non-zero if either procedure fails.
#
# Usage:  ./smoke-eval-subset.sh <serverless-vm-ip>

set -euo pipefail

VM_IP="${1:?Usage: $0 <serverless-vm-ip>}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"

# Test subscriber (matches test/integration/helpers_test.go).
ssh -o StrictHostKeyChecking=no -i "$SSH_KEY" "root@${VM_IP}" 'python3 - "$@"' _ <<'PY'
import base64, json, sys, urllib.request, urllib.error

GW = "http://127.0.0.1:31112/function"   # OpenFaaS gateway NodePort
SUPI = "imsi-001010000000001"

def b64hex(h): return base64.b64encode(bytes.fromhex(h)).decode()

def call(fn, payload):
    req = urllib.request.Request(f"{GW}/{fn}", data=json.dumps(payload).encode(),
                                 headers={"Content-Type": "application/json"}, method="POST")
    try:
        r = urllib.request.urlopen(req, timeout=20)
        return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()

sub = {
    "supi": SUPI,
    "auth_data": {
        "auth_method": "5G_AKA",
        "k":   b64hex("465B5CE8B199B49FAA5F0A2EE238A6BC"),
        "opc": b64hex("E8ED289DEBA952E4283B54E88E6183CA"),
        "amf": base64.b64encode(bytes([0x80, 0x00])).decode(),
        "sqn": base64.b64encode(bytes([0, 0, 0, 0, 0, 1])).decode(),
    },
    "access_mobility_data": {"nssai": [{"sst": 1, "sd": "010203"}], "default_dnn": "internet"},
    "session_management": [{"snssai": {"sst": 1, "sd": "010203"}, "dnn": "internet", "qos_ref": 9}],
}

failed = False

st, _ = call("udr-data-write", sub)
print(f"provision subscriber : HTTP {st}")
failed |= st != 201

st, body = call("amf-initial-registration",
                {"supi": SUPI, "ran_ue_ngap_id": 1001, "gnb_id": "gnb-001",
                 "registration_type": 1, "requested_nssai": [{"sst": 1, "sd": "010203"}]})
print(f"registration         : HTTP {st}  {body}")
failed |= st != 200

st, body = call("smf-pdu-session-create",
                {"supi": SUPI, "pdu_session_id": 1, "dnn": "internet",
                 "snssai": {"sst": 1, "sd": "010203"}})
print(f"pdu session create   : HTTP {st}  {body}")
failed |= st != 201

sys.exit(1 if failed else 0)
PY
