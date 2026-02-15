"""
Generate paper-ready charts from evaluation analysis results.

Reads eval/results/summary.csv and produces charts in eval/results/charts/.

Usage: python charts.py [summary_csv] [output_dir]
"""

import sys
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

# Color scheme for the three systems.
COLORS = {
    "serverless": "#2196F3",
    "open5gs": "#FF9800",
    "free5gc": "#4CAF50",
}

LABELS = {
    "serverless": "Serverless 5GC",
    "open5gs": "Open5GS",
    "free5gc": "free5GC",
}

SCENARIO_ORDER = ["idle", "low", "medium", "high", "burst"]


def setup_style():
    """Set up matplotlib style for paper-quality figures."""
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
    """Bar chart: cost per scenario across all three systems."""
    fig, ax = plt.subplots()

    avg = df.groupby(["scenario", "target"])["projected_cost_usd"].mean().unstack()
    avg = avg.reindex(SCENARIO_ORDER)

    x = np.arange(len(SCENARIO_ORDER))
    width = 0.25

    for i, target in enumerate(["serverless", "open5gs", "free5gc"]):
        if target in avg.columns:
            values = avg[target].values
            ax.bar(x + i * width, values, width,
                   label=LABELS[target], color=COLORS[target], edgecolor="white")

    ax.set_xlabel("Scenario")
    ax.set_ylabel("Projected Cost (USD)")
    ax.set_title("Cost Comparison: Serverless vs Traditional 5G Core")
    ax.set_xticks(x + width)
    ax.set_xticklabels([s.capitalize() for s in SCENARIO_ORDER])
    ax.legend()

    plt.savefig(output_dir / "cost_comparison.png")
    plt.savefig(output_dir / "cost_comparison.pdf")
    plt.close()
    print("  Generated: cost_comparison.png/pdf")


def plot_cost_crossover(df, output_dir):
    """Line chart: cost vs UE count, showing crossover point."""
    fig, ax = plt.subplots()

    for target in ["serverless", "open5gs", "free5gc"]:
        subset = df[df["target"] == target]
        avg = subset.groupby("ue_count")["projected_cost_usd"].mean().sort_index()
        ax.plot(avg.index, avg.values, "o-",
                label=LABELS[target], color=COLORS[target], linewidth=2, markersize=6)

    ax.set_xlabel("Number of UEs")
    ax.set_ylabel("Projected Cost (USD)")
    ax.set_title("Cost Crossover: Serverless vs Traditional")
    ax.legend()

    # Add annotation for crossover region.
    ax.axhline(y=0, color="gray", linewidth=0.5)

    plt.savefig(output_dir / "cost_crossover.png")
    plt.savefig(output_dir / "cost_crossover.pdf")
    plt.close()
    print("  Generated: cost_crossover.png/pdf")


def plot_latency_comparison(df, output_dir):
    """Box plot: registration latency distribution per system per scenario."""
    fig, ax = plt.subplots(figsize=(10, 5))

    scenarios = [s for s in SCENARIO_ORDER if s != "idle"]
    targets = ["serverless", "open5gs", "free5gc"]
    positions = []
    box_data = []
    box_colors = []
    tick_positions = []
    tick_labels = []

    pos = 0
    for scenario in scenarios:
        group_start = pos
        for target in targets:
            subset = df[(df["scenario"] == scenario) & (df["target"] == target)]
            latencies = subset["avg_duration_seconds"].values * 1000  # to ms
            if len(latencies) > 0:
                box_data.append(latencies)
                positions.append(pos)
                box_colors.append(COLORS[target])
            pos += 1
        tick_positions.append(group_start + 1)
        tick_labels.append(scenario.capitalize())
        pos += 1  # gap between groups

    if box_data:
        bp = ax.boxplot(box_data, positions=positions, widths=0.7, patch_artist=True)
        for patch, color in zip(bp["boxes"], box_colors):
            patch.set_facecolor(color)
            patch.set_alpha(0.7)

    ax.set_ylabel("Avg Latency (ms)")
    ax.set_title("Registration Latency by Scenario and System")
    ax.set_xticks(tick_positions)
    ax.set_xticklabels(tick_labels)

    # Custom legend.
    from matplotlib.patches import Patch
    legend_elements = [Patch(facecolor=COLORS[t], label=LABELS[t]) for t in targets]
    ax.legend(handles=legend_elements)

    plt.savefig(output_dir / "latency_comparison.png")
    plt.savefig(output_dir / "latency_comparison.pdf")
    plt.close()
    print("  Generated: latency_comparison.png/pdf")


def plot_resource_utilization(df, output_dir):
    """Grouped bar chart: CPU and memory usage per scenario and system."""
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 5))

    scenarios = [s for s in SCENARIO_ORDER if s != "idle"]
    targets = ["serverless", "open5gs", "free5gc"]

    x = np.arange(len(scenarios))
    width = 0.25

    # CPU usage.
    for i, target in enumerate(targets):
        values = []
        for scenario in scenarios:
            subset = df[(df["scenario"] == scenario) & (df["target"] == target)]
            values.append(subset["total_cpu_seconds"].mean() if len(subset) > 0 else 0)
        ax1.bar(x + i * width, values, width,
                label=LABELS[target], color=COLORS[target], edgecolor="white")

    ax1.set_xlabel("Scenario")
    ax1.set_ylabel("Total CPU Seconds")
    ax1.set_title("CPU Usage")
    ax1.set_xticks(x + width)
    ax1.set_xticklabels([s.capitalize() for s in scenarios])
    ax1.legend()

    # Memory usage.
    for i, target in enumerate(targets):
        values = []
        for scenario in scenarios:
            subset = df[(df["scenario"] == scenario) & (df["target"] == target)]
            values.append(subset["peak_memory_mb"].mean() if len(subset) > 0 else 0)
        ax2.bar(x + i * width, values, width,
                label=LABELS[target], color=COLORS[target], edgecolor="white")

    ax2.set_xlabel("Scenario")
    ax2.set_ylabel("Peak Memory (MB)")
    ax2.set_title("Memory Usage")
    ax2.set_xticks(x + width)
    ax2.set_xticklabels([s.capitalize() for s in scenarios])
    ax2.legend()

    plt.suptitle("Resource Utilization Comparison", fontsize=13, y=1.02)
    plt.tight_layout()
    plt.savefig(output_dir / "resource_utilization.png")
    plt.savefig(output_dir / "resource_utilization.pdf")
    plt.close()
    print("  Generated: resource_utilization.png/pdf")


def plot_cold_start_histogram(df, output_dir):
    """Histogram: function cold start latency distribution for serverless."""
    fig, ax = plt.subplots()

    serverless = df[df["target"] == "serverless"]
    latencies = serverless["avg_duration_seconds"].values * 1000  # to ms

    if len(latencies) > 0:
        ax.hist(latencies, bins=20, color=COLORS["serverless"],
                edgecolor="white", alpha=0.8)
        ax.axvline(np.median(latencies), color="red", linestyle="--",
                   linewidth=1.5, label=f"Median: {np.median(latencies):.1f} ms")
        ax.axvline(np.percentile(latencies, 99), color="orange", linestyle="--",
                   linewidth=1.5, label=f"P99: {np.percentile(latencies, 99):.1f} ms")

    ax.set_xlabel("Latency (ms)")
    ax.set_ylabel("Count")
    ax.set_title("Serverless Function Latency Distribution (incl. Cold Starts)")
    ax.legend()

    plt.savefig(output_dir / "cold_start_histogram.png")
    plt.savefig(output_dir / "cold_start_histogram.pdf")
    plt.close()
    print("  Generated: cold_start_histogram.png/pdf")


def main():
    setup_style()

    summary_csv = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("eval/results/summary.csv")
    output_dir = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("eval/results/charts")

    if not summary_csv.exists():
        print(f"Summary CSV not found: {summary_csv}")
        print("Run analyze.py first to generate the summary.")
        sys.exit(1)

    output_dir.mkdir(parents=True, exist_ok=True)

    df = pd.read_csv(summary_csv)
    print(f"Loaded {len(df)} results from {summary_csv}")
    print(f"Generating charts in {output_dir}...\n")

    plot_cost_comparison(df, output_dir)
    plot_cost_crossover(df, output_dir)
    plot_latency_comparison(df, output_dir)
    plot_resource_utilization(df, output_dir)
    plot_cold_start_histogram(df, output_dir)

    print(f"\nAll charts generated in {output_dir}")


if __name__ == "__main__":
    main()
