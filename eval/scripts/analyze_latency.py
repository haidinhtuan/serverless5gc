#!/usr/bin/env python3
"""Analyze loadgen latency results for serverless and open5gs targets."""

import json
import os
import re
from datetime import datetime

import numpy as np

BASE = os.path.join(os.path.dirname(__file__), "..", "results")
SCENARIOS = ["low", "medium", "high", "burst"]
RUNS = ["run1", "run2", "run3"]

SCENARIO_PARAMS = {
    "low":    {"ue_count": 100,  "rate": 1,  "pdu_per_ue": 1},
    "medium": {"ue_count": 500,  "rate": 5,  "pdu_per_ue": 2},
    "high":   {"ue_count": 1000, "rate": 20, "pdu_per_ue": 3},
    "burst":  {"ue_count": 500,  "rate": 50, "pdu_per_ue": 2},
}


def parse_serverless_results(filepath):
    """Parse serverless reg-results.txt or pdu-results.txt.
    Format: '<status_code> <latency>s' per line.
    Returns (ok_latencies_ms, err_latencies_ms).
    """
    ok_latencies = []
    err_latencies = []
    with open(filepath) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            parts = line.split()
            status = int(parts[0])
            latency_ms = float(parts[1].rstrip("s")) * 1000
            if status in (200, 201):
                ok_latencies.append(latency_ms)
            else:
                err_latencies.append(latency_ms)
    return ok_latencies, err_latencies


def parse_open5gs_ue_log(filepath):
    """Parse UERANSIM ue.log to extract per-UE registration and PDU session latencies.
    Returns (reg_latencies_ms, pdu_latencies_ms, reg_failures, pdu_failures).
    """
    timestamp_re = re.compile(
        r"\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})\] "
        r"\[([^|]+)\|"
    )
    reg_start = {}
    pdu_start = {}
    reg_latencies = []
    pdu_latencies = []

    with open(filepath) as f:
        for line in f:
            m = timestamp_re.search(line)
            if not m:
                continue
            ts_str, ue_id = m.group(1), m.group(2)
            ts = datetime.strptime(ts_str, "%Y-%m-%d %H:%M:%S.%f")

            if "Sending Initial Registration" in line:
                reg_start[ue_id] = ts
            elif "Initial Registration is successful" in line:
                if ue_id in reg_start:
                    delta_ms = (ts - reg_start[ue_id]).total_seconds() * 1000
                    reg_latencies.append(delta_ms)
                    del reg_start[ue_id]
            elif "Sending PDU Session Establishment Request" in line:
                pdu_start[ue_id] = ts
            elif "PDU Session establishment is successful" in line:
                if ue_id in pdu_start:
                    delta_ms = (ts - pdu_start[ue_id]).total_seconds() * 1000
                    pdu_latencies.append(delta_ms)
                    del pdu_start[ue_id]

    return reg_latencies, pdu_latencies, len(reg_start), len(pdu_start)


def compute_stats(latencies):
    if not latencies:
        return None
    arr = np.array(latencies)
    return {
        "count": int(len(arr)),
        "min": round(float(np.min(arr)), 2),
        "max": round(float(np.max(arr)), 2),
        "mean": round(float(np.mean(arr)), 2),
        "median": round(float(np.median(arr)), 2),
        "std": round(float(np.std(arr)), 2),
        "p95": round(float(np.percentile(arr, 95)), 2),
        "p99": round(float(np.percentile(arr, 99)), 2),
    }


def fmt(s, errors=0, total=None):
    if s is None:
        err_str = f" (errors={errors})" if errors else ""
        return f"  No successful data{err_str}"
    tot = total if total else s["count"] + errors
    err_rate = errors / tot * 100 if tot > 0 else 0
    return (
        f"  n={s['count']}/{tot} ({err_rate:.1f}% errors)\n"
        f"  min={s['min']:.2f} max={s['max']:.2f} mean={s['mean']:.2f} "
        f"med={s['median']:.2f} std={s['std']:.2f}\n"
        f"  p95={s['p95']:.2f} p99={s['p99']:.2f}  (all ms)"
    )


def main():
    all_results = {}

    # =========== SERVERLESS ===========
    print("=" * 72)
    print("SERVERLESS 5GC - LATENCY ANALYSIS")
    print("=" * 72)

    for scenario in SCENARIOS:
        p = SCENARIO_PARAMS[scenario]
        print(f"\n{'─'*72}")
        print(f"Scenario: {scenario.upper()} ({p['ue_count']} UEs, {p['rate']}/s, "
              f"{p['pdu_per_ue']} PDU/UE)")
        print(f"{'─'*72}")

        all_reg_ok, all_pdu_ok = [], []
        all_reg_err, all_pdu_err = [], []

        for run in RUNS:
            reg_path = os.path.join(BASE, "serverless", scenario, run, "reg-results.txt")
            pdu_path = os.path.join(BASE, "serverless", scenario, run, "pdu-results.txt")
            if not os.path.exists(reg_path):
                print(f"  {run}: MISSING")
                continue

            reg_ok, reg_err = parse_serverless_results(reg_path)
            pdu_ok, pdu_err = parse_serverless_results(pdu_path)
            all_reg_ok.extend(reg_ok); all_reg_err.extend(reg_err)
            all_pdu_ok.extend(pdu_ok); all_pdu_err.extend(pdu_err)

            print(f"  {run}: reg {len(reg_ok)}/{len(reg_ok)+len(reg_err)}, "
                  f"pdu {len(pdu_ok)}/{len(pdu_ok)+len(pdu_err)}")

        total_reg = len(all_reg_ok) + len(all_reg_err)
        total_pdu = len(all_pdu_ok) + len(all_pdu_err)
        expected_reg = p["ue_count"] * 3
        expected_pdu = p["ue_count"] * p["pdu_per_ue"] * 3

        reg_stats = compute_stats(all_reg_ok)
        pdu_stats = compute_stats(all_pdu_ok)
        reg_err_stats = compute_stats(all_reg_err)
        pdu_err_stats = compute_stats(all_pdu_err)

        print(f"\n  Registration (success):")
        print(fmt(reg_stats, len(all_reg_err), total_reg))
        if reg_err_stats:
            print(f"  Registration (errors): mean={reg_err_stats['mean']:.2f}ms, "
                  f"max={reg_err_stats['max']:.2f}ms")

        print(f"\n  PDU Session (success):")
        print(fmt(pdu_stats, len(all_pdu_err), total_pdu))
        if pdu_err_stats:
            print(f"  PDU Session (errors): mean={pdu_err_stats['mean']:.2f}ms, "
                  f"max={pdu_err_stats['max']:.2f}ms")

        all_results[f"serverless_{scenario}"] = {
            "reg": reg_stats,
            "reg_errors": len(all_reg_err),
            "reg_total": total_reg,
            "reg_error_stats": reg_err_stats,
            "pdu": pdu_stats,
            "pdu_errors": len(all_pdu_err),
            "pdu_total": total_pdu,
            "pdu_error_stats": pdu_err_stats,
        }

    # =========== OPEN5GS ===========
    print("\n" + "=" * 72)
    print("OPEN5GS - LATENCY ANALYSIS (from UERANSIM logs)")
    print("=" * 72)

    for scenario in SCENARIOS:
        p = SCENARIO_PARAMS[scenario]
        print(f"\n{'─'*72}")
        print(f"Scenario: {scenario.upper()} ({p['ue_count']} UEs, {p['rate']}/s, "
              f"{p['pdu_per_ue']} PDU/UE)")
        print(f"{'─'*72}")

        all_reg, all_pdu = [], []
        total_reg_fail, total_pdu_fail = 0, 0

        for run in RUNS:
            ue_log = os.path.join(BASE, "open5gs", scenario, run, "ue.log")
            if not os.path.exists(ue_log):
                print(f"  {run}: MISSING")
                continue
            reg_lat, pdu_lat, reg_fail, pdu_fail = parse_open5gs_ue_log(ue_log)
            all_reg.extend(reg_lat); all_pdu.extend(pdu_lat)
            total_reg_fail += reg_fail; total_pdu_fail += pdu_fail
            print(f"  {run}: reg {len(reg_lat)}/{len(reg_lat)+reg_fail}, "
                  f"pdu {len(pdu_lat)}/{len(pdu_lat)+pdu_fail}")

        reg_stats = compute_stats(all_reg)
        pdu_stats = compute_stats(all_pdu)

        print(f"\n  Registration:")
        print(fmt(reg_stats, total_reg_fail))
        print(f"\n  PDU Session:")
        print(fmt(pdu_stats, total_pdu_fail))

        all_results[f"open5gs_{scenario}"] = {
            "reg": reg_stats,
            "reg_errors": total_reg_fail,
            "reg_total": len(all_reg) + total_reg_fail,
            "pdu": pdu_stats,
            "pdu_errors": total_pdu_fail,
            "pdu_total": len(all_pdu) + total_pdu_fail,
        }

    # =========== COMPARISON TABLES ===========
    print("\n" + "=" * 72)
    print("REGISTRATION LATENCY COMPARISON (ms, success only)")
    print("=" * 72)
    hdr = (f"{'Scenario':<8} │ {'Serverless':^38} │ {'Open5GS':^38}")
    sub = (f"{'':8} │ {'mean':>8} {'p95':>8} {'p99':>8} {'err%':>7} │ "
           f"{'mean':>8} {'p95':>8} {'p99':>8} {'err%':>7}")
    print(hdr)
    print(sub)
    print("─" * len(sub))
    for scenario in SCENARIOS:
        s = all_results.get(f"serverless_{scenario}", {})
        o = all_results.get(f"open5gs_{scenario}", {})
        sr, ore = s.get("reg"), o.get("reg")
        s_err = s.get("reg_errors", 0) / max(s.get("reg_total", 1), 1) * 100
        o_err = o.get("reg_errors", 0) / max(o.get("reg_total", 1), 1) * 100
        sm = f"{sr['mean']:.1f}" if sr else "N/A"
        sp = f"{sr['p95']:.1f}" if sr else "N/A"
        s9 = f"{sr['p99']:.1f}" if sr else "N/A"
        om = f"{ore['mean']:.1f}" if ore else "N/A"
        op = f"{ore['p95']:.1f}" if ore else "N/A"
        o9 = f"{ore['p99']:.1f}" if ore else "N/A"
        print(f"{scenario:<8} │ {sm:>8} {sp:>8} {s9:>8} {s_err:>6.1f}% │ "
              f"{om:>8} {op:>8} {o9:>8} {o_err:>6.1f}%")

    print("\n" + "=" * 72)
    print("PDU SESSION LATENCY COMPARISON (ms, success only)")
    print("=" * 72)
    print(hdr)
    print(sub)
    print("─" * len(sub))
    for scenario in SCENARIOS:
        s = all_results.get(f"serverless_{scenario}", {})
        o = all_results.get(f"open5gs_{scenario}", {})
        sp_s, pp_o = s.get("pdu"), o.get("pdu")
        s_err = s.get("pdu_errors", 0) / max(s.get("pdu_total", 1), 1) * 100
        o_err = o.get("pdu_errors", 0) / max(o.get("pdu_total", 1), 1) * 100
        sm = f"{sp_s['mean']:.1f}" if sp_s else "N/A"
        sp = f"{sp_s['p95']:.1f}" if sp_s else "N/A"
        s9 = f"{sp_s['p99']:.1f}" if sp_s else "N/A"
        om = f"{pp_o['mean']:.1f}" if pp_o else "N/A"
        op = f"{pp_o['p95']:.1f}" if pp_o else "N/A"
        o9 = f"{pp_o['p99']:.1f}" if pp_o else "N/A"
        print(f"{scenario:<8} │ {sm:>8} {sp:>8} {s9:>8} {s_err:>6.1f}% │ "
              f"{om:>8} {op:>8} {o9:>8} {o_err:>6.1f}%")

    # =========== SPEEDUP TABLE ===========
    print("\n" + "=" * 72)
    print("SPEEDUP: Open5GS mean / Serverless mean (higher = serverless faster)")
    print("=" * 72)
    print(f"{'Scenario':<10} {'Reg speedup':>15} {'PDU speedup':>15}")
    print("─" * 42)
    for scenario in SCENARIOS:
        s = all_results.get(f"serverless_{scenario}", {})
        o = all_results.get(f"open5gs_{scenario}", {})
        sr, ore = s.get("reg"), o.get("reg")
        sp, opp = s.get("pdu"), o.get("pdu")
        if sr and ore and sr["mean"] > 0:
            r = f"{ore['mean']/sr['mean']:.1f}x"
        else:
            r = "N/A"
        if sp and opp and sp["mean"] > 0:
            p = f"{opp['mean']/sp['mean']:.1f}x"
        else:
            p = "N/A"
        print(f"{scenario:<10} {r:>15} {p:>15}")

    # =========== ERROR RATE SUMMARY ===========
    print("\n" + "=" * 72)
    print("ERROR RATE SUMMARY (%)")
    print("=" * 72)
    print(f"{'Scenario':<10} {'S-Reg':>8} {'S-PDU':>8} {'O-Reg':>8} {'O-PDU':>8}")
    print("─" * 44)
    for scenario in SCENARIOS:
        s = all_results.get(f"serverless_{scenario}", {})
        o = all_results.get(f"open5gs_{scenario}", {})
        sr_e = s.get("reg_errors", 0) / max(s.get("reg_total", 1), 1) * 100
        sp_e = s.get("pdu_errors", 0) / max(s.get("pdu_total", 1), 1) * 100
        or_e = o.get("reg_errors", 0) / max(o.get("reg_total", 1), 1) * 100
        op_e = o.get("pdu_errors", 0) / max(o.get("pdu_total", 1), 1) * 100
        print(f"{scenario:<10} {sr_e:>7.1f}% {sp_e:>7.1f}% {or_e:>7.1f}% {op_e:>7.1f}%")

    # Save JSON
    out_path = os.path.join(BASE, "latency_summary.json")
    with open(out_path, "w") as f:
        json.dump(all_results, f, indent=2)
    print(f"\nResults saved to: {out_path}")


if __name__ == "__main__":
    main()
