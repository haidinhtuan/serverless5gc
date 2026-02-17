#!/bin/bash
# Builds Docker images for all 31 OpenFaaS functions.
# Each function is package function with a Handle() method; this script
# generates a main.go wrapper for each and builds via Dockerfile.template.
#
# Usage: ./build-functions.sh [--push] [--registry REGISTRY]
#   --push      Push images to registry after build
#   --registry  Registry prefix (default: serverless5gc)
#
# Run from project root:  bash deploy/openfaas/build-functions.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

REGISTRY="${REGISTRY:-serverless5gc}"
PUSH=false

for arg in "$@"; do
    case "$arg" in
        --push) PUSH=true ;;
        --registry) shift; REGISTRY="$1" ;;
    esac
done

MODULE="github.com/tdinh/serverless5gc"
DOCKERFILE="${SCRIPT_DIR}/Dockerfile.template"
ENTRY_DIR="${PROJECT_DIR}/cmd/entry"

# Function definitions: image_name:function_package_path
declare -a FUNCTIONS=(
    "nrf-register:functions/nrf/register"
    "nrf-discover:functions/nrf/discover"
    "nrf-status-notify:functions/nrf/status-notify"
    "amf-initial-registration:functions/amf/registration"
    "amf-deregistration:functions/amf/deregistration"
    "amf-service-request:functions/amf/service-request"
    "amf-pdu-session-relay:functions/amf/pdu-session-relay"
    "amf-auth-initiate:functions/amf/auth-initiate"
    "amf-handover:functions/amf/handover"
    "smf-pdu-session-create:functions/smf/pdu-session-create"
    "smf-pdu-session-update:functions/smf/pdu-session-update"
    "smf-pdu-session-release:functions/smf/pdu-session-release"
    "smf-n4-session-setup:functions/smf/n4-session-setup"
    "udm-generate-auth-data:functions/udm/generate-auth-data"
    "udm-get-subscriber-data:functions/udm/get-subscriber-data"
    "udr-data-read:functions/udr/data-read"
    "udr-data-write:functions/udr/data-write"
    "ausf-authenticate:functions/ausf/authenticate"
    "pcf-policy-create:functions/pcf/policy-create"
    "pcf-policy-get:functions/pcf/policy-get"
    "nssf-slice-select:functions/nssf/slice-select"
    "nwdaf-analytics-subscribe:functions/nwdaf/analytics-subscribe"
    "nwdaf-data-collect:functions/nwdaf/data-collect"
    "chf-charging-create:functions/chf/charging-create"
    "chf-charging-update:functions/chf/charging-update"
    "chf-charging-release:functions/chf/charging-release"
    "nsacf-slice-availability-check:functions/nsacf/slice-availability-check"
    "nsacf-update-counters:functions/nsacf/update-counters"
    "bsf-binding-register:functions/bsf/binding-register"
    "bsf-binding-discover:functions/bsf/binding-discover"
    "bsf-binding-deregister:functions/bsf/binding-deregister"
)

generate_main() {
    local func_pkg="$1"
    mkdir -p "$ENTRY_DIR"
    cat > "${ENTRY_DIR}/main.go" << GOEOF
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	handler "github.com/openfaas/templates-sdk/go-http"
	function "${MODULE}/${func_pkg}"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("read body: %s", err), http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		req := handler.Request{
			Body:        body,
			Header:      r.Header,
			QueryString: r.URL.RawQuery,
			Method:      r.Method,
			Host:        r.Host,
		}
		req.WithContext(r.Context())

		resp, err := function.Handle(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("handler error: %s", err), http.StatusInternalServerError)
			return
		}

		for k, vals := range resp.Header {
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		if resp.StatusCode == 0 {
			resp.StatusCode = http.StatusOK
		}
		w.WriteHeader(resp.StatusCode)
		w.Write(resp.Body)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	log.Printf("Function listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
GOEOF
}

echo "=== Building ${#FUNCTIONS[@]} function images ==="
echo "Registry: ${REGISTRY}"
echo ""

BUILT=0
FAILED=0

for entry in "${FUNCTIONS[@]}"; do
    IFS=':' read -r IMAGE_NAME FUNC_PKG <<< "$entry"
    IMAGE="${REGISTRY}/${IMAGE_NAME}:latest"

    echo "[$((BUILT + FAILED + 1))/${#FUNCTIONS[@]}] Building ${IMAGE}..."

    # Generate main.go for this function.
    generate_main "$FUNC_PKG"

    # Build Docker image from project root.
    if docker build \
        -f "$DOCKERFILE" \
        -t "$IMAGE" \
        "$PROJECT_DIR" > /dev/null 2>&1; then
        echo "  OK: ${IMAGE}"
        BUILT=$((BUILT + 1))

        if $PUSH; then
            echo "  Pushing ${IMAGE}..."
            docker push "$IMAGE" > /dev/null 2>&1
        fi
    else
        echo "  FAILED: ${IMAGE}"
        # Retry with output for debugging.
        docker build -f "$DOCKERFILE" -t "$IMAGE" "$PROJECT_DIR" 2>&1 | tail -5
        FAILED=$((FAILED + 1))
    fi
done

# Clean up generated entry point.
rm -rf "$ENTRY_DIR"

echo ""
echo "=== Build complete: ${BUILT} succeeded, ${FAILED} failed ==="

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
