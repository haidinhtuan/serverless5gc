
import json
import os
import sys
import glob

def calculate_percentile(buckets, total_count, percentile):
    if total_count == 0:
        return 0.0
    
    target_rank = total_count * (percentile / 100.0)
    
    # Sort buckets by le (float)
    sorted_buckets = sorted(buckets.items(), key=lambda item: item[0])
    
    current_count = 0
    for i, (le, count) in enumerate(sorted_buckets):
        # count is cumulative here? 
        # The buckets passed to this function:
        # If 'count' is the value in valid 'le' bucket, it IS cumulative in Prometheus format.
        # But let's verify if I should pass raw buckets or processed frequency.
        # Standard Prometheus buckets are cumulative.
        
        # If we use cumulative counts directly:
        if count >= target_rank:
            # Found the bucket containing the target rank
            # Interpolate
            
            prev_le = 0.0 if i == 0 else sorted_buckets[i-1][0]
            prev_count = 0 if i == 0 else sorted_buckets[i-1][1]
            
            # Count in this specific bucket (not cumulative)
            count_in_bucket = count - prev_count
            
            # Rank within this bucket
            rank_in_bucket = target_rank - prev_count
            
            # Linear interpolation
            # bound_width = le - prev_le
            # val = prev_le + (rank_in_bucket / count_in_bucket) * bound_width
            
            # For +Inf, we can't interpolate conventionally, usually max observed or just return prev_le
            if le == float('inf'):
                return prev_le # or some heuristic
                
            return prev_le + (le - prev_le) * (rank_in_bucket / count_in_bucket)
            
    return sorted_buckets[-1][0]

def process_file(filepath):
    try:
        with open(filepath, 'r') as f:
            data = json.load(f)
        
        if data['status'] != 'success':
            return None

        # Determine the "function" of interest.
        # We want Registration Latency. 
        # In serverless5gc, amf-initial-registration seems like the main entry.
        target_function = "amf-initial-registration.openfaas-fn"
        
        # Collect buckets
        # Map: le (float) -> count (int)
        buckets = {}
        
        results = data['data']['result']
        
        max_timestamp = 0
        
        for res in results:
            metric = res['metric']
            if metric.get('function_name') != target_function:
                continue
                
            le_str = metric.get('le')
            if not le_str:
                continue
                
            le = float('inf') if le_str == '+Inf' else float(le_str)
            
            # Get the LAST value in the series (assuming cumulative count grows)
            # The values are [timestamp, string_value]
            values = res['values']
            if not values:
                continue
                
            last_val = values[-1]
            # timestamp = last_val[0]
            count = int(last_val[1])
            
            buckets[le] = buckets.get(le, 0) + count
            
        if not buckets:
            return None
            
        # Get total count (from +Inf bucket)
        total_count = buckets.get(float('inf'), 0)
        
        if total_count == 0:
             # Try to find max count if +Inf is missing (unlikely for proper output)
             total_count = max(buckets.values()) if buckets else 0

        p50 = calculate_percentile(buckets, total_count, 50)
        p95 = calculate_percentile(buckets, total_count, 95)
        p99 = calculate_percentile(buckets, total_count, 99)
        
        return p50, p95, p99

    except Exception as e:
        sys.stderr.write(f"Error processing {filepath}: {e}\n")
        return None

def main():
    base_dir = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results"
    
    # Header
    print("Scenario,Target,p50,p95,p99")
    
    # Iterate scenarios
    scenarios = ['idle', 'low', 'medium', 'high', 'burst']
    targets = ['serverless-sctp', 'free5gc', 'open5gs']
    
    for scenario in scenarios:
        for target in targets:
            # Find runs
            pattern = os.path.join(base_dir, target, scenario, "run*", "gateway_functions_seconds_bucket.json")
            files = glob.glob(pattern)
            
            p50_sum = 0
            p95_sum = 0
            p99_sum = 0
            count = 0
            
            for file in files:
                res = process_file(file)
                if res:
                    p50, p95, p99 = res
                    p50_sum += p50
                    p95_sum += p95
                    p99_sum += p99
                    count += 1
            
            if count > 0:
                print(f"{scenario},{target},{p50_sum/count:.4f},{p95_sum/count:.4f},{p99_sum/count:.4f}")
            else:
                # print(f"{scenario},{target},NaN,NaN,NaN")
                pass

if __name__ == "__main__":
    main()
