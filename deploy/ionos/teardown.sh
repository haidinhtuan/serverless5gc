#!/bin/bash
# Tears down the IONOS Cloud evaluation datacenter and all resources within it.
# Finds the datacenter by name pattern or from vm-ips.env, then deletes it.
#
# Usage: ./teardown.sh
#   or:  ./teardown.sh <datacenter-id>
#
# Set FORCE=1 to skip confirmation prompt.

set -euo pipefail

# Use config file auth, not env var token.
unset IONOS_TOKEN 2>/dev/null || true

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/vm-ips.env"
FORCE="${FORCE:-0}"

# ---------------------------------------------------------------------------
# Determine datacenter ID
# ---------------------------------------------------------------------------
if [ -n "${1:-}" ]; then
    # Datacenter ID provided as argument.
    DC_ID="$1"
    DC_NAME="(provided via argument)"
elif [ -f "$ENV_FILE" ]; then
    # Read from vm-ips.env.
    # shellcheck source=/dev/null
    source "$ENV_FILE"
    DC_ID="${DC_ID:?DC_ID not found in vm-ips.env}"
    DC_NAME="${DC_NAME:-unknown}"
else
    # Try to find datacenter by name pattern.
    echo "Searching for s5gc-eval-* datacenters..."
    DC_INFO=$(ionosctl datacenter list --output json 2>/dev/null | \
        jq -r '.items[] | select(.properties.name | startswith("s5gc-eval-")) | "\(.id) \(.properties.name)"' | \
        head -1)

    if [ -z "$DC_INFO" ]; then
        echo "No evaluation datacenter found (name pattern: s5gc-eval-*)."
        echo "Usage: $0 [datacenter-id]"
        exit 0
    fi

    DC_ID=$(echo "$DC_INFO" | awk '{print $1}')
    DC_NAME=$(echo "$DC_INFO" | awk '{print $2}')
fi

echo "Found datacenter: ${DC_NAME} (${DC_ID})"
echo ""
echo "WARNING: This will permanently delete the datacenter and ALL resources within it."
echo "  - All VMs, volumes, NICs, and LANs will be destroyed."
echo "  - This action cannot be undone."

# ---------------------------------------------------------------------------
# Confirmation
# ---------------------------------------------------------------------------
if [ "$FORCE" != "1" ]; then
    echo ""
    read -p "Type 'yes' to confirm deletion: " CONFIRM
    if [ "$CONFIRM" != "yes" ]; then
        echo "Aborted."
        exit 0
    fi
fi

# ---------------------------------------------------------------------------
# Delete datacenter
# ---------------------------------------------------------------------------
echo ""
echo "Deleting datacenter ${DC_ID}..."
ionosctl datacenter delete \
    --datacenter-id "$DC_ID" \
    --force \
    --wait-for-request \
    --timeout 600

echo "Datacenter deleted successfully."

# ---------------------------------------------------------------------------
# Clean up local env file
# ---------------------------------------------------------------------------
if [ -f "$ENV_FILE" ]; then
    echo "Removing $ENV_FILE..."
    rm -f "$ENV_FILE"
fi

echo ""
echo "Teardown complete."
