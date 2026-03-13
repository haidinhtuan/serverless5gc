#!/usr/bin/env python3
"""Analyze cold-start storm experiment results.

Compares cold-start latency against warm-start baseline data.
Outputs: summary table, per-scenario stats, time-to-first-success.

Usage: python3 analyze-coldstart.py
"""
import re
import os
import json
from datetime import datetime
import numpy as np

BASE = os.path.join(os.path.dirname(__file__), "..", "results")
COLD_TARGET = "serverless-sctp-coldstart"
WARM_TARGET = "serverless-sctp"
SCENARIOS = ["low", "medium", "high", "burst"]
RUNS = ["run1", "run2", "run3"]

ts_re = re.compile(r"\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})\] \[([^|]+)\|")


def parse_registrations(filepath):
    """Parse ue.log for registration latencies. Returns list of (timestamp, latency_ms)."""
    starts = {}
    results = []
    with open(filepath) as f:
        for line in f:
            m = ts_re.search(line)
            if not m:
                continue
            ts = datetime.strptime(m.group(1), "%Y-%m-%d %H:%M:%S.%f")
            ue = m.group(2)
            if "Sending Initial Registration" in line:
                starts[ue] = ts
            elif "Initial Registration is successful" in line:
                if ue in starts:
                    latency = (ts - starts[ue]).total_seconds() * 1000
                    results.append((ts, latency))
                    del starts[ue]
    return results


def parse_pdu(filepath):
    """Parse ue.log for PDU session latencies."""
    starts = {}
    results = []
    with open(filepath) as f:
        for line in f:
            m = ts_re.search(line)
            if not m:
                continue
            ts = datetime.strptime(m.group(1), "%Y-%m-%d %H:%M:%S.%f")
            ue = m.group(2)
            if "Sending PDU Session Establishment Request" in line:
                starts[ue] = ts
            elif "PDU Session establishment is successful" in line:
                if ue in starts:
                    latency = (ts - starts[ue]).total_seconds() * 1000
                    results.append((ts, latency))
                    del starts[ue]
    return results


def count_successes(filepath):
    """Count registration and PDU session successes."""
    reg_ok = 0
    pdu_ok = 0
    with open(filepath) as f:
        for line in f:
            if "Initial Registration is successful" in line:
                reg_ok += 1
            elif "PDU Session establishment is successful" in line:
                pdu_ok += 1
    return reg_ok, pdu_ok


def get_stats(latencies):
    """Compute p50, p95, p99 from list of latencies."""
    if not latencies:
        return None, None, None
    arr = np.array(latencies)
    return (
        int(round(np.percentile(arr, 50))),
        int(round(np.percentile(arr, 95))),
        int(round(np.percentile(arr, 99))),
    )


def main():
    print("=" * 100)
    print("COLD-START STORM EXPERIMENT ANALYSIS")
    print("=" * 100)

    # Header
    fmt = "{:<10} {:<6} {:>5} | {:>7} {:>7} {:>7} | {:>7} {:>7} {:>7} | {:>6} {:>6} | {:>8}"
    print()
    print(fmt.format(
        "Scenario", "Type", "n",
        "p50", "p95", "p99",
        "Δp50", "Δp95", "Δp99",
        "Reg%", "PDU%", "1st(ms)"
    ))
    print("-" * 100)

    for scenario in SCENARIOS:
        # Collect warm baseline
        warm_lats = []
        for run in RUNS:
            path = os.path.join(BASE, WARM_TARGET, scenario, run, "ue.log")
            if os.path.exists(path):
                warm_lats.extend([lat for _, lat in parse_registrations(path)])
        warm_p50, warm_p95, warm_p99 = get_stats(warm_lats)

        # Collect cold-start data
        cold_lats = []
        cold_results_all = []
        total_reg_expected = 0
        total_reg_ok = 0
        total_pdu_ok = 0
        first_success_times = []

        for run in RUNS:
            path = os.path.join(BASE, COLD_TARGET, scenario, run, "ue.log")
            if not os.path.exists(path):
                continue

            results = parse_registrations(path)
            cold_results_all.extend(results)
            cold_lats.extend([lat for _, lat in results])

            reg_ok, pdu_ok = count_successes(path)
            total_reg_ok += reg_ok
            total_pdu_ok += pdu_ok

            # Time to first success
            if results:
                # Get scenario start time from metadata
                meta_path = os.path.join(BASE, COLD_TARGET, scenario, run, "start_time")
                if os.path.exists(meta_path):
                    with open(meta_path) as f:
                        start_str = f.read().strip()
                    # First successful registration timestamp
                    first_ts = min(ts for ts, _ in results)
                    first_lat = results[0][1] if results else 0
                    first_success_times.append(first_lat)

            # Count expected registrations from scenario config
            scenario_file = os.path.join(BASE, "..", "scenarios", f"{scenario}.yaml")
            if os.path.exists(scenario_file):
                with open(scenario_file) as f:
                    for line in f:
                        if "count:" in line:
                            total_reg_expected = int(line.split(":")[1].strip()) * len(RUNS)
                            break

        cold_p50, cold_p95, cold_p99 = get_stats(cold_lats)

        # Print warm baseline
        if warm_p50 is not None:
            print(fmt.format(
                scenario, "warm", len(warm_lats),
                warm_p50, warm_p95, warm_p99,
                "-", "-", "-",
                "100.0", "100.0", "-"
            ))

        # Print cold-start
        if cold_p50 is not None:
            d50 = cold_p50 - warm_p50 if warm_p50 else 0
            d95 = cold_p95 - warm_p95 if warm_p95 else 0
            d99 = cold_p99 - warm_p99 if warm_p99 else 0
            reg_pct = f"{100 * total_reg_ok / total_reg_expected:.1f}" if total_reg_expected > 0 else "?"
            pdu_pct = f"{100 * total_pdu_ok / total_reg_expected:.1f}" if total_reg_expected > 0 else "?"
            first_ms = f"{int(np.mean(first_success_times))}" if first_success_times else "?"

            print(fmt.format(
                scenario, "cold", len(cold_lats),
                cold_p50, cold_p95, cold_p99,
                f"+{d50}" if d50 >= 0 else str(d50),
                f"+{d95}" if d95 >= 0 else str(d95),
                f"+{d99}" if d99 >= 0 else str(d99),
                reg_pct, pdu_pct, first_ms
            ))
        else:
            print(f"{scenario:<10} {'cold':<6}   NO DATA")

        print()

    # Also compute first-N-UEs latency breakdown for cold starts
    print()
    print("=" * 60)
    print("COLD-START CONVERGENCE (first N registrations)")
    print("=" * 60)
    for scenario in SCENARIOS:
        all_results = []
        for run in RUNS:
            path = os.path.join(BASE, COLD_TARGET, scenario, run, "ue.log")
            if os.path.exists(path):
                results = parse_registrations(path)
                # Sort by timestamp within each run
                results.sort(key=lambda x: x[0])
                all_results.append(results)

        if not all_results:
            continue

        print(f"\n{scenario}:")
        buckets = [1, 5, 10, 20, 50, 100]
        for n in buckets:
            lats = []
            for run_results in all_results:
                lats.extend([lat for _, lat in run_results[:n]])
            if lats:
                p50 = int(round(np.percentile(lats, 50)))
                print(f"  First {n:>3} UEs: p50={p50}ms (n={len(lats)})")


if __name__ == "__main__":
    main()
