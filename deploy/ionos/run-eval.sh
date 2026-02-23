#!/bin/bash
# Master evaluation runner for the serverless 5GC evaluation campaign.
# Runs all 45 test combinations: 5 scenarios x 3 targets x 3 runs.
# Sources vm-ips.env for VM IPs, calls eval/scripts/run-scenario.sh for each run.
#
# Usage: ./run-eval.sh
#
# Options:
#   SCENARIOS  - Space-separated list of scenarios (default: "idle low medium high burst")
#   TARGETS    - Space-separated list of targets (default: "serverless open5gs free5gc")
#   RUNS       - Number of runs per combination (default: 3)
#   COOLDOWN   - Seconds between runs (default: 60)
#   START_FROM - Skip runs until this number (e.g., START_FROM=10 skips first 9)
#   DRY_RUN    - Set to 1 to print plan without executing

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
ENV_FILE="${SCRIPT_DIR}/vm-ips.env"

# ---------------------------------------------------------------------------
# Load VM IPs
# ---------------------------------------------------------------------------
if [ ! -f "$ENV_FILE" ]; then
    echo "ERROR: $ENV_FILE not found. Run provision.sh first."
    exit 1
fi

# shellcheck source=/dev/null
source "$ENV_FILE"

# Verify required variables are set.
SERVERLESS_IP="${SERVERLESS_IP:?SERVERLESS_IP not set in vm-ips.env}"
OPEN5GS_IP="${OPEN5GS_IP:?OPEN5GS_IP not set in vm-ips.env}"
FREE5GC_IP="${FREE5GC_IP:?FREE5GC_IP not set in vm-ips.env}"
LOADGEN_IP="${LOADGEN_IP:?LOADGEN_IP not set in vm-ips.env}"
MONITORING_IP="${MONITORING_IP:?MONITORING_IP not set in vm-ips.env}"

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
SCENARIOS="${SCENARIOS:-idle low medium high burst}"
TARGETS="${TARGETS:-serverless open5gs free5gc}"
RUNS="${RUNS:-3}"
COOLDOWN="${COOLDOWN:-60}"
START_FROM="${START_FROM:-1}"
DRY_RUN="${DRY_RUN:-0}"

RUN_SCENARIO="${PROJECT_DIR}/eval/scripts/run-scenario.sh"
LOG_FILE="${PROJECT_DIR}/eval/results/eval-campaign-$(date +%Y%m%d-%H%M%S).log"

mkdir -p "$(dirname "$LOG_FILE")"

log() {
    echo "[$(date +%H:%M:%S)] $*" | tee -a "$LOG_FILE"
}

# Map target name to its AMF IP.
get_amf_ip() {
    local target=$1
    case "$target" in
        serverless) echo "$SERVERLESS_IP" ;;
        open5gs)    echo "$OPEN5GS_IP" ;;
        free5gc)    echo "$FREE5GC_IP" ;;
        *)
            log "ERROR: Unknown target: $target"
            return 1
            ;;
    esac
}

# ---------------------------------------------------------------------------
# Build run plan
# ---------------------------------------------------------------------------
declare -a PLAN_SCENARIO=()
declare -a PLAN_TARGET=()
declare -a PLAN_RUN=()

for scenario in $SCENARIOS; do
    for target in $TARGETS; do
        for run in $(seq 1 "$RUNS"); do
            PLAN_SCENARIO+=("$scenario")
            PLAN_TARGET+=("$target")
            PLAN_RUN+=("$run")
        done
    done
done

TOTAL_RUNS=${#PLAN_SCENARIO[@]}

# ---------------------------------------------------------------------------
# Print plan
# ---------------------------------------------------------------------------
log "========================================="
log "  EVALUATION CAMPAIGN"
log "========================================="
log "Scenarios: $SCENARIOS"
log "Targets:   $TARGETS"
log "Runs/combo: $RUNS"
log "Total runs: $TOTAL_RUNS"
log "Cooldown:  ${COOLDOWN}s"
log "Start from: $START_FROM"
log ""
log "VM IPs:"
log "  serverless: $SERVERLESS_IP"
log "  open5gs:    $OPEN5GS_IP"
log "  free5gc:    $FREE5GC_IP"
log "  loadgen:    $LOADGEN_IP"
log "  monitoring: $MONITORING_IP"
log "========================================="

if [ "$DRY_RUN" = "1" ]; then
    log ""
    log "DRY RUN - printing plan only:"
    for i in "${!PLAN_SCENARIO[@]}"; do
        RUN_NUM=$((i + 1))
        log "  [$RUN_NUM/$TOTAL_RUNS] ${PLAN_SCENARIO[$i]} / ${PLAN_TARGET[$i]} / run${PLAN_RUN[$i]}"
    done
    log ""
    log "Set DRY_RUN=0 to execute."
    exit 0
fi

# ---------------------------------------------------------------------------
# Execute runs
# ---------------------------------------------------------------------------
PASSED=0
FAILED=0
SKIPPED=0
CAMPAIGN_START=$(date +%s)

for i in "${!PLAN_SCENARIO[@]}"; do
    RUN_NUM=$((i + 1))
    SCENARIO="${PLAN_SCENARIO[$i]}"
    TARGET="${PLAN_TARGET[$i]}"
    RUN="${PLAN_RUN[$i]}"

    # Skip runs before START_FROM.
    if [ "$RUN_NUM" -lt "$START_FROM" ]; then
        SKIPPED=$((SKIPPED + 1))
        continue
    fi

    AMF_IP=$(get_amf_ip "$TARGET")

    log ""
    log "==========================================================="
    log "  RUN $RUN_NUM/$TOTAL_RUNS: $SCENARIO / $TARGET / run$RUN"
    log "  AMF IP: $AMF_IP"
    log "==========================================================="

    RUN_START=$(date +%s)

    if TARGET_AMF_IP="$AMF_IP" \
       MONITORING_IP="$MONITORING_IP" \
       LOADGEN_IP="$LOADGEN_IP" \
       bash "$RUN_SCENARIO" "$SCENARIO" "$TARGET" "$RUN" 2>&1 | tee -a "$LOG_FILE"; then
        PASSED=$((PASSED + 1))
        log "  RESULT: PASSED"
    else
        FAILED=$((FAILED + 1))
        log "  RESULT: FAILED (exit code $?)"
    fi

    RUN_END=$(date +%s)
    RUN_DURATION=$((RUN_END - RUN_START))
    log "  Duration: ${RUN_DURATION}s"

    # Cooldown between runs (skip after the last run).
    if [ "$RUN_NUM" -lt "$TOTAL_RUNS" ] && [ "$RUN_NUM" -ge "$START_FROM" ]; then
        log "  Cooldown: ${COOLDOWN}s..."
        sleep "$COOLDOWN"
    fi
done

CAMPAIGN_END=$(date +%s)
CAMPAIGN_DURATION=$((CAMPAIGN_END - CAMPAIGN_START))
CAMPAIGN_HOURS=$((CAMPAIGN_DURATION / 3600))
CAMPAIGN_MINS=$(( (CAMPAIGN_DURATION % 3600) / 60 ))

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
log ""
log "========================================="
log "  CAMPAIGN COMPLETE"
log "========================================="
log "Total runs: $TOTAL_RUNS"
log "Passed:     $PASSED"
log "Failed:     $FAILED"
log "Skipped:    $SKIPPED"
log "Duration:   ${CAMPAIGN_HOURS}h ${CAMPAIGN_MINS}m"
log "Log file:   $LOG_FILE"
log "========================================="

# ---------------------------------------------------------------------------
# Run analysis
# ---------------------------------------------------------------------------
log ""
log "Running analysis..."
ANALYSIS_SCRIPT="${PROJECT_DIR}/eval/analysis/analyze.py"
PYTHON="${HOME}/.venvs/eval/bin/python3"
[ -x "$PYTHON" ] || PYTHON="python3"

if [ -f "$ANALYSIS_SCRIPT" ]; then
    if "$PYTHON" "$ANALYSIS_SCRIPT" "${PROJECT_DIR}/eval/results" 2>&1 | tee -a "$LOG_FILE"; then
        log "Analysis complete. Results in eval/results/summary.csv"
    else
        log "WARNING: Analysis script failed. Run manually: $PYTHON $ANALYSIS_SCRIPT"
    fi

    CHARTS_SCRIPT="${PROJECT_DIR}/eval/analysis/charts.py"
    SUMMARY_CSV="${PROJECT_DIR}/eval/results/summary.csv"
    if [ -f "$CHARTS_SCRIPT" ] && [ -f "$SUMMARY_CSV" ]; then
        log "Generating charts..."
        if "$PYTHON" "$CHARTS_SCRIPT" "$SUMMARY_CSV" "${PROJECT_DIR}/eval/results/charts" 2>&1 | tee -a "$LOG_FILE"; then
            log "Charts generated."
        else
            log "WARNING: Chart generation failed."
        fi
    fi
else
    log "WARNING: Analysis script not found at $ANALYSIS_SCRIPT"
fi

log ""
log "All done. Results are in ${PROJECT_DIR}/eval/results/"

# Exit with failure if any runs failed.
if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
