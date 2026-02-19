import pandas as pd

def calculate_averages():
    df = pd.read_csv('eval/results/summary.csv')
    
    # Constants
    VM_VCPUS = {
        'free5gc': 4,
        'open5gs': 4,
        'serverless-sctp': 4
    }
    
    # Calculate VM CPU Utilization %
    # Util = Total CPU Seconds / (Duration Mins * 60 * vCPUs)
    df['vm_cpu_util'] = df.apply(lambda row: 
        (row['total_cpu_seconds'] / (row['duration_minutes'] * 60 * VM_VCPUS.get(row['target'], 4))) * 100, axis=1)

    # Group by target and scenario
    grouped = df.groupby(['target', 'scenario']).agg(
        avg_cpu_seconds=('total_cpu_seconds', 'mean'),
        avg_vm_cpu_util=('vm_cpu_util', 'mean'),
        avg_peak_memory=('peak_memory_mb', 'mean'),
        avg_avg_memory=('avg_memory_mb', 'mean'),
        avg_cost=('projected_cost_usd', 'mean'),
        count=('run', 'count')
    ).round(2)
    
    pd.set_option('display.max_rows', None)
    pd.set_option('display.max_columns', None)
    pd.set_option('display.width', 1000)
    print("Resource Consumption Averages (3-run):")
    print(grouped)
    
    # Also print latency percentiles for easy copying
    lat_df = pd.read_csv('eval/results/latency_percentiles.csv')
    print("\nLatency Percentiles:")
    print(lat_df)

if __name__ == "__main__":
    calculate_averages()
