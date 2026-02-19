"""
Generate paper-ready charts from evaluation results.

Reads:
- eval/results/summary.csv (resource + cost data from Prometheus)
- eval/results/latency_summary.json (registration/PDU latencies from loadgen)

Produces charts in eval/results/charts/.

Usage: python charts_v2.py [results_dir]
"""

import json
import sys
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import numpy as np
import pandas as pd

COLORS = {
    "serverless": "#2196F3",
    "open5gs": "#FF9800",
}

LABELS = {
    "serverless": "Serverless 5GC",
    "open5gs": "Open5GS (Fargate)",
}

SCENARIO_ORDER = ["idle", "low", "medium", "high", "burst"]
LOAD_SCENARIOS = ["low", "medium", "high", "burst"]


def setup_style():
    plt.rcParams.update({
        "figure.figsize": (8, 5),
        "figure.dpi": 150,
        "font.size": 11,
        "font.family": "serif",
        "axes.grid": True,
        "grid.alpha": 0.3,
        "axes.spines.top": False,
        "axes.spines.right": False,
        "savefig.bbox": "tight",
        "savefig.pad_inches": 0.1,
    })


def plot_cost_comparison(df, output_dir):
    """Bar chart: cost per scenario, serverless vs open5gs."""
    fig, ax = plt.subplots()

    avg = df.groupby(["scenario", "target"])["projected_cost_usd"].mean().unstack()
    avg = avg.reindex(SCENARIO_ORDER)

    x = np.arange(len(SCENARIO_ORDER))
    width = 0.35

    for i, target in enumerate(["serverless", "open5gs"]):
        if target in avg.columns:
            values = avg[target].values
            bars = ax.bar(x + i * width - width / 2, values, width,
                          label=LABELS[target], color=COLORS[target], edgecolor="white")
            for bar, val in zip(bars, values):
                if val > 0:
                    ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height(),
                            f"${val:.4f}", ha="center", va="bottom", fontsize=8)

    ax.set_xlabel("Scenario")
    ax.set_ylabel("Projected Cost (USD)")
    ax.set_title("Cost Comparison: Serverless (Lambda) vs Traditional (Fargate)")
    ax.set_xticks(x)
    ax.set_xticklabels([s.capitalize() for s in SCENARIO_ORDER])
    ax.legend()

    plt.savefig(output_dir / "cost_comparison.png")
    plt.savefig(output_dir / "cost_comparison.pdf")
    plt.close()
    print("  cost_comparison")


def plot_cost_crossover(df, output_dir):
    """Line chart: cost vs UE count."""
    fig, ax = plt.subplots()

    for target in ["serverless", "open5gs"]:
        subset = df[(df["target"] == target) & (df["scenario"] != "idle")]
        avg = subset.groupby("ue_count")["projected_cost_usd"].mean().sort_index()
        ax.plot(avg.index, avg.values, "o-",
                label=LABELS[target], color=COLORS[target], linewidth=2, markersize=6)

    ax.set_xlabel("Number of UEs")
    ax.set_ylabel("Projected Cost (USD)")
    ax.set_title("Cost Scaling: Serverless vs Traditional")
    ax.legend()

    plt.savefig(output_dir / "cost_crossover.png")
    plt.savefig(output_dir / "cost_crossover.pdf")
    plt.close()
    print("  cost_crossover")


def plot_registration_latency(latency_data, output_dir):
    """Bar chart: registration latency comparison across scenarios."""
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 5), gridspec_kw={"width_ratios": [2, 1]})

    # Mean + p95 + p99 grouped bar chart
    scenarios = LOAD_SCENARIOS
    x = np.arange(len(scenarios))
    width = 0.35

    for target_i, target in enumerate(["serverless", "open5gs"]):
        means, p95s, p99s, errs = [], [], [], []
        for sc in scenarios:
            key = f"{target}_{sc}"
            d = latency_data.get(key, {})
            reg = d.get("reg")
            if reg:
                means.append(reg["mean"])
                p95s.append(reg["p95"])
                p99s.append(reg["p99"])
            else:
                means.append(0)
                p95s.append(0)
                p99s.append(0)
            total = d.get("reg_total", 1)
            errs.append(d.get("reg_errors", 0) / max(total, 1) * 100)

        offset = (target_i - 0.5) * width
        bars = ax1.bar(x + offset, means, width * 0.8,
                       label=f"{LABELS[target]} (mean)", color=COLORS[target], alpha=0.8)
        # p95 error bars
        ax1.errorbar(x + offset, means,
                     yerr=[np.zeros(len(means)), np.array(p95s) - np.array(means)],
                     fmt="none", ecolor=COLORS[target], capsize=4, alpha=0.6)

    ax1.set_xlabel("Scenario")
    ax1.set_ylabel("Registration Latency (ms)")
    ax1.set_title("Registration Latency: Mean + P95 Whisker")
    ax1.set_xticks(x)
    ax1.set_xticklabels([s.capitalize() for s in scenarios])
    ax1.legend(fontsize=9)
    ax1.set_yscale("log")
    ax1.yaxis.set_major_formatter(ticker.ScalarFormatter())

    # Speedup chart
    speedups = []
    for sc in scenarios:
        s_reg = latency_data.get(f"serverless_{sc}", {}).get("reg")
        o_reg = latency_data.get(f"open5gs_{sc}", {}).get("reg")
        if s_reg and o_reg and s_reg["mean"] > 0:
            speedups.append(o_reg["mean"] / s_reg["mean"])
        else:
            speedups.append(0)

    bars = ax2.bar(x, speedups, 0.6, color="#4CAF50", edgecolor="white")
    for bar, val in zip(bars, speedups):
        if val > 0:
            ax2.text(bar.get_x() + bar.get_width() / 2, bar.get_height(),
                     f"{val:.0f}x", ha="center", va="bottom", fontsize=10, fontweight="bold")
    ax2.set_xlabel("Scenario")
    ax2.set_ylabel("Speedup (Open5GS / Serverless)")
    ax2.set_title("Registration Latency Speedup")
    ax2.set_xticks(x)
    ax2.set_xticklabels([s.capitalize() for s in scenarios])

    plt.suptitle("Registration Latency Comparison", fontsize=13, y=1.02)
    plt.tight_layout()
    plt.savefig(output_dir / "registration_latency.png")
    plt.savefig(output_dir / "registration_latency.pdf")
    plt.close()
    print("  registration_latency")


def plot_pdu_session_latency(latency_data, output_dir):
    """Bar chart: PDU session latency (low scenario only for serverless due to errors)."""
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 5))

    # Left: low scenario comparison (only scenario with valid data for both)
    metrics = ["mean", "median", "p95", "p99"]
    x = np.arange(len(metrics))
    width = 0.35

    s_pdu = latency_data.get("serverless_low", {}).get("pdu")
    o_pdu = latency_data.get("open5gs_low", {}).get("pdu")

    if s_pdu:
        values = [s_pdu[m] for m in metrics]
        ax1.bar(x - width / 2, values, width,
                label=LABELS["serverless"], color=COLORS["serverless"])
    if o_pdu:
        values = [o_pdu[m] for m in metrics]
        ax1.bar(x + width / 2, values, width,
                label=LABELS["open5gs"], color=COLORS["open5gs"])

    ax1.set_xlabel("Statistic")
    ax1.set_ylabel("PDU Session Latency (ms)")
    ax1.set_title("PDU Session Latency (Low Scenario)")
    ax1.set_xticks(x)
    ax1.set_xticklabels([m.upper() for m in metrics])
    ax1.legend()

    # Right: error rates across scenarios
    scenarios = LOAD_SCENARIOS
    x2 = np.arange(len(scenarios))
    for target_i, target in enumerate(["serverless", "open5gs"]):
        err_rates = []
        for sc in scenarios:
            d = latency_data.get(f"{target}_{sc}", {})
            total = d.get("pdu_total", 1)
            errors = d.get("pdu_errors", 0)
            err_rates.append(errors / max(total, 1) * 100)
        offset = (target_i - 0.5) * width
        ax2.bar(x2 + offset, err_rates, width * 0.8,
                label=LABELS[target], color=COLORS[target], alpha=0.8)

    ax2.set_xlabel("Scenario")
    ax2.set_ylabel("PDU Session Error Rate (%)")
    ax2.set_title("PDU Session Reliability")
    ax2.set_xticks(x2)
    ax2.set_xticklabels([s.capitalize() for s in scenarios])
    ax2.legend()
    ax2.set_ylim(0, 110)
    ax2.axhline(y=100, color="red", linestyle="--", alpha=0.3)

    plt.suptitle("PDU Session Establishment Analysis", fontsize=13, y=1.02)
    plt.tight_layout()
    plt.savefig(output_dir / "pdu_session_latency.png")
    plt.savefig(output_dir / "pdu_session_latency.pdf")
    plt.close()
    print("  pdu_session_latency")


def plot_resource_utilization(df, output_dir):
    """CPU and memory comparison."""
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 5))

    scenarios = LOAD_SCENARIOS
    x = np.arange(len(scenarios))
    width = 0.35

    for i, target in enumerate(["serverless", "open5gs"]):
        cpu_vals, mem_vals = [], []
        for scenario in scenarios:
            subset = df[(df["scenario"] == scenario) & (df["target"] == target)]
            cpu_vals.append(subset["total_cpu_seconds"].mean() if len(subset) > 0 else 0)
            mem_vals.append(subset["peak_memory_mb"].mean() if len(subset) > 0 else 0)

        offset = (i - 0.5) * width
        ax1.bar(x + offset, cpu_vals, width * 0.8,
                label=LABELS[target], color=COLORS[target], edgecolor="white")
        ax2.bar(x + offset, mem_vals, width * 0.8,
                label=LABELS[target], color=COLORS[target], edgecolor="white")

    ax1.set_xlabel("Scenario")
    ax1.set_ylabel("Total CPU Seconds")
    ax1.set_title("CPU Usage")
    ax1.set_xticks(x)
    ax1.set_xticklabels([s.capitalize() for s in scenarios])
    ax1.legend()

    ax2.set_xlabel("Scenario")
    ax2.set_ylabel("Peak Memory (MB)")
    ax2.set_title("Memory Usage")
    ax2.set_xticks(x)
    ax2.set_xticklabels([s.capitalize() for s in scenarios])
    ax2.legend()

    plt.suptitle("Resource Utilization Comparison", fontsize=13, y=1.02)
    plt.tight_layout()
    plt.savefig(output_dir / "resource_utilization.png")
    plt.savefig(output_dir / "resource_utilization.pdf")
    plt.close()
    print("  resource_utilization")


def plot_latency_vs_load(latency_data, output_dir):
    """Line chart: how latency scales with UE count."""
    fig, ax = plt.subplots()

    # UE counts for each scenario
    ue_counts = {"low": 100, "medium": 500, "high": 1000, "burst": 500}
    scenarios_by_ue = ["low", "medium", "high"]  # burst has same UEs as medium

    for target in ["serverless", "open5gs"]:
        ues, means, p95s = [], [], []
        for sc in scenarios_by_ue:
            reg = latency_data.get(f"{target}_{sc}", {}).get("reg")
            if reg:
                ues.append(ue_counts[sc])
                means.append(reg["mean"])
                p95s.append(reg["p95"])

        if ues:
            ax.plot(ues, means, "o-", label=f"{LABELS[target]} (mean)",
                    color=COLORS[target], linewidth=2, markersize=8)
            ax.plot(ues, p95s, "s--", label=f"{LABELS[target]} (p95)",
                    color=COLORS[target], linewidth=1.5, markersize=6, alpha=0.6)

    ax.set_xlabel("Number of UEs")
    ax.set_ylabel("Registration Latency (ms)")
    ax.set_title("Registration Latency vs Load")
    ax.legend()
    ax.set_yscale("log")
    ax.yaxis.set_major_formatter(ticker.ScalarFormatter())

    plt.savefig(output_dir / "latency_vs_load.png")
    plt.savefig(output_dir / "latency_vs_load.pdf")
    plt.close()
    print("  latency_vs_load")


def plot_error_summary(latency_data, output_dir):
    """Comprehensive error rate comparison."""
    fig, ax = plt.subplots()

    scenarios = LOAD_SCENARIOS
    x = np.arange(len(scenarios))
    width = 0.2
    offsets = [-1.5, -0.5, 0.5, 1.5]
    labels_list = ["S-Reg", "S-PDU", "O-Reg", "O-PDU"]
    colors_list = ["#2196F3", "#90CAF9", "#FF9800", "#FFE0B2"]

    for idx, (target, metric) in enumerate([
        ("serverless", "reg"), ("serverless", "pdu"),
        ("open5gs", "reg"), ("open5gs", "pdu"),
    ]):
        rates = []
        for sc in scenarios:
            d = latency_data.get(f"{target}_{sc}", {})
            total = d.get(f"{metric}_total", 1)
            errors = d.get(f"{metric}_errors", 0)
            rates.append(errors / max(total, 1) * 100)
        ax.bar(x + offsets[idx] * width, rates, width * 0.9,
               label=labels_list[idx], color=colors_list[idx], edgecolor="white")

    ax.set_xlabel("Scenario")
    ax.set_ylabel("Error Rate (%)")
    ax.set_title("Procedure Error Rates: Serverless vs Open5GS")
    ax.set_xticks(x)
    ax.set_xticklabels([s.capitalize() for s in scenarios])
    ax.legend(ncol=2)
    ax.set_ylim(0, 110)

    plt.savefig(output_dir / "error_rates.png")
    plt.savefig(output_dir / "error_rates.pdf")
    plt.close()
    print("  error_rates")


def main():
    setup_style()

    results_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("eval/results")
    output_dir = results_dir / "charts"
    output_dir.mkdir(parents=True, exist_ok=True)

    summary_csv = results_dir / "summary.csv"
    latency_json = results_dir / "latency_summary.json"

    if not summary_csv.exists():
        print(f"Missing: {summary_csv}")
        sys.exit(1)

    df = pd.read_csv(summary_csv)
    print(f"Loaded {len(df)} rows from {summary_csv}")

    latency_data = {}
    if latency_json.exists():
        with open(latency_json) as f:
            latency_data = json.load(f)
        print(f"Loaded latency data from {latency_json}")

    # Flag duration mismatch
    low_durations = df[(df["scenario"] == "low") & (df["target"] == "open5gs")]["duration_minutes"]
    if low_durations.nunique() > 1:
        print(f"\n  WARNING: open5gs/low has mixed durations: "
              f"{sorted(low_durations.unique())} min")
        print("  run1=30min vs runs 2-3=10min. Cost comparison may be affected.\n")

    print(f"Generating charts in {output_dir}...\n")

    plot_cost_comparison(df, output_dir)
    plot_cost_crossover(df, output_dir)
    plot_resource_utilization(df, output_dir)

    if latency_data:
        plot_registration_latency(latency_data, output_dir)
        plot_pdu_session_latency(latency_data, output_dir)
        plot_latency_vs_load(latency_data, output_dir)
        plot_error_summary(latency_data, output_dir)

    print(f"\nAll charts generated in {output_dir}")


if __name__ == "__main__":
    main()
