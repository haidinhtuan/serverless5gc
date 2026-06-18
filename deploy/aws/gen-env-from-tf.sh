#!/bin/bash
# Writes vm-ips-aws.env from terraform outputs, in the SERVERLESS_IP/LOADGEN_IP
# format consumed by setup-serverless.sh, setup-loadgen.sh and run-eval.sh.
#
# Usage: ./gen-env-from-tf.sh   (run from deploy/aws after a successful apply)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/vm-ips-aws.env"

cd "$SCRIPT_DIR"

terraform output -raw env_file > "$ENV_FILE"

echo "Wrote $ENV_FILE:"
cat "$ENV_FILE"
echo
echo "Source it before running the setup/eval scripts:"
echo "  source $ENV_FILE"
