"""
Analyze evaluation results and compute cost comparisons.

Reads Prometheus metric exports from eval/results/ and produces:
- Cost comparison summary (CSV)
- Latency percentile summaries
- Resource utilization summaries

Usage: python analyze.py [results_dir]
"""

import csv
import json
import sys
from dataclasses import dataclass, asdict
from pathlib import Path

# AWS Lambda pricing (x86, us-east-1).
LAMBDA_PRICE_PER_GB_SEC = 0.0000166667
LAMBDA_PRICE_PER_REQUEST = 0.0000002  # $0.20 / 1M

# AWS Fargate pricing for baseline comparison.
FARGATE_PRICE_PER_VCPU_HR = 0.04048
FARGATE_PRICE_PER_GB_HR = 0.004445

# Baseline VM specs.
BASELINE_VCPUS = 8
BASELINE_MEMORY_GB = 16

DEFAULT_FUNCTION_MEMORY_MB = 128


@dataclass
class ScenarioResult:
    scenario: str
    target: str
    run: int
    duration_minutes: float
    ue_count: int
    total_invocations: float
    avg_duration_seconds: float
    total_cpu_seconds: float
    peak_memory_mb: float
    avg_memory_mb: float
    projected_cost_usd: float
    cost_model: str


def load_prometheus_json(filepath):
    """Load and parse a Prometheus query_range JSON export."""
    try:
        with open(filepath) as f:
            data = json.load(f)
        if data.get("status") != "success":
            return []
        return data.get("data", {}).get("result", [])
    except (FileNotFoundError, json.JSONDecodeError):
        return []


def extract_last_value(results):
    """Extract the last numeric value from Prometheus results."""
    total = 0.0
    for series in results:
        values = series.get("values", [])
        if values:
            try:
                total += float(values[-1][1])
            except (ValueError, IndexError):
                pass
    return total


def extract_max_value(results):
    """Extract the maximum value across all series and time points."""
    max_val = 0.0
    for series in results:
        for _, val in series.get("values", []):
            try:
                v = float(val)
                if v > max_val:
                    max_val = v
            except ValueError:
                pass
    return max_val


def extract_avg_value(results):
    """Extract the average value across all series and time points."""
    total = 0.0
    count = 0
    for series in results:
        for _, val in series.get("values", []):
            try:
                total += float(val)
                count += 1
            except ValueError:
                pass
    return total / count if count > 0 else 0.0


def compute_serverless_cost(invocations, avg_duration_s, memory_mb=DEFAULT_FUNCTION_MEMORY_MB):
    """Compute projected AWS Lambda cost."""
    gb_seconds = (memory_mb / 1024.0) * avg_duration_s * invocations
    compute_cost = gb_seconds * LAMBDA_PRICE_PER_GB_SEC
    request_cost = invocations * LAMBDA_PRICE_PER_REQUEST
    return compute_cost + request_cost


def compute_traditional_cost(duration_hours, vcpus=BASELINE_VCPUS, memory_gb=BASELINE_MEMORY_GB):
    """Compute projected AWS Fargate cost for a fixed-size deployment."""
    vcpu_cost = vcpus * FARGATE_PRICE_PER_VCPU_HR * duration_hours
    memory_cost = memory_gb * FARGATE_PRICE_PER_GB_HR * duration_hours
    return vcpu_cost + memory_cost


def analyze_run(run_dir):
    """Analyze a single evaluation run."""
    run_dir = Path(run_dir)

    # Load metadata.
    metadata_file = run_dir / "metadata.json"
    if not metadata_file.exists():
        return None

    with open(metadata_file) as f:
        meta = json.load(f)

    scenario = meta["scenario"]
    target = meta["target"]
    run = meta["run"]
    duration_minutes = meta["duration_minutes"]
    ue_count = meta["ue_count"]
    duration_hours = duration_minutes / 60.0

    # Load invocation metrics.
    invoc_results = load_prometheus_json(
        run_dir / "gateway_function_invocation_total.json"
    )
    total_invocations = extract_last_value(invoc_results)

    # Load duration metrics.
    duration_sum = load_prometheus_json(
        run_dir / "gateway_functions_seconds_sum.json"
    )
    duration_count = load_prometheus_json(
        run_dir / "gateway_functions_seconds_count.json"
    )
    total_duration = extract_last_value(duration_sum)
    total_count = extract_last_value(duration_count)
    avg_duration = total_duration / total_count if total_count > 0 else 0.1

    # Load CPU metrics.
    cpu_results = load_prometheus_json(
        run_dir / "container_cpu_usage_seconds_total.json"
    )
    total_cpu_seconds = extract_last_value(cpu_results)

    # Load memory metrics.
    mem_results = load_prometheus_json(
        run_dir / "container_memory_usage_bytes.json"
    )
    peak_memory_bytes = extract_max_value(mem_results)
    avg_memory_bytes = extract_avg_value(mem_results)

    # Compute costs.
    if target.startswith("serverless"):
        cost = compute_serverless_cost(total_invocations, avg_duration)
        cost_model = "lambda"
    else:
        cost = compute_traditional_cost(duration_hours)
        cost_model = "fargate"

    return ScenarioResult(
        scenario=scenario,
        target=target,
        run=run,
        duration_minutes=duration_minutes,
        ue_count=ue_count,
        total_invocations=total_invocations,
        avg_duration_seconds=avg_duration,
        total_cpu_seconds=total_cpu_seconds,
        peak_memory_mb=peak_memory_bytes / (1024 * 1024),
        avg_memory_mb=avg_memory_bytes / (1024 * 1024),
        projected_cost_usd=cost,
        cost_model=cost_model,
    )


def main():
    results_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("eval/results")

    if not results_dir.exists():
        print(f"Results directory not found: {results_dir}")
        sys.exit(1)

    results = []

    # Walk through results directory: target/scenario/runN/
    for target_dir in sorted(results_dir.iterdir()):
        if not target_dir.is_dir() or target_dir.name in ("charts",):
            continue
        for scenario_dir in sorted(target_dir.iterdir()):
            if not scenario_dir.is_dir():
                continue
            for run_dir in sorted(scenario_dir.iterdir()):
                if not run_dir.is_dir():
                    continue
                result = analyze_run(run_dir)
                if result:
                    results.append(result)

    if not results:
        print("No results found to analyze.")
        sys.exit(1)

    # Write summary CSV.
    output_csv = results_dir / "summary.csv"
    fieldnames = list(asdict(results[0]).keys())

    with open(output_csv, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        for r in results:
            writer.writerow(asdict(r))

    print(f"Summary written to {output_csv} ({len(results)} runs)")

    # Print summary table.
    print("\n=== Cost Comparison Summary ===\n")
    print(f"{'Scenario':<10} {'Target':<12} {'UEs':<6} {'Invocations':<12} "
          f"{'Avg Latency':<14} {'Cost (USD)':<12} {'Model':<8}")
    print("-" * 80)

    for r in sorted(results, key=lambda x: (x.scenario, x.target, x.run)):
        print(f"{r.scenario:<10} {r.target:<12} {r.ue_count:<6} "
              f"{r.total_invocations:<12.0f} {r.avg_duration_seconds*1000:<14.2f}ms "
              f"${r.projected_cost_usd:<11.6f} {r.cost_model:<8}")

    # Compute averages per scenario/target.
    print("\n=== Averaged Results (across runs) ===\n")
    from collections import defaultdict
    grouped = defaultdict(list)
    for r in results:
        grouped[(r.scenario, r.target)].append(r)

    print(f"{'Scenario':<10} {'Target':<12} {'Avg Cost (USD)':<16} {'Avg Latency (ms)':<18}")
    print("-" * 60)
    for (scenario, target), runs in sorted(grouped.items()):
        avg_cost = sum(r.projected_cost_usd for r in runs) / len(runs)
        avg_latency = sum(r.avg_duration_seconds for r in runs) / len(runs) * 1000
        print(f"{scenario:<10} {target:<12} ${avg_cost:<15.6f} {avg_latency:<18.2f}")


if __name__ == "__main__":
    main()
