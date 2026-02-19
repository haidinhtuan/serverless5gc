import json
import sys

def dump_buckets(filepath):
    try:
        with open(filepath, 'r') as f:
            data = json.load(f)
        
        target_function = "amf-initial-registration.openfaas-fn"
        
        results = data['data']['result']
        buckets = {}
        
        for res in results:
            metric = res['metric']
            if metric.get('function_name') != target_function:
                continue
            
            le = metric.get('le')
            if not le: continue
            
            # Get last value
            count = int(res['values'][-1][1])
            buckets[le] = buckets.get(le, 0) + count
            
        return buckets
    except Exception as e:
        return str(e)

file1 = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results/serverless-sctp/medium/run1/gateway_functions_seconds_bucket.json"
file2 = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results/free5gc/medium/run1/gateway_functions_seconds_bucket.json"
file3 = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results/open5gs/medium/run1/gateway_functions_seconds_bucket.json"

b1 = dump_buckets(file1)
b2 = dump_buckets(file2)
b3 = dump_buckets(file3)

print("Serverless Keys:", len(b1) if isinstance(b1, dict) else b1)
print("Free5GC Keys:", len(b2) if isinstance(b2, dict) else b2)
print("Open5GS Keys:", len(b3) if isinstance(b3, dict) else b3)

if isinstance(b1, dict) and isinstance(b2, dict):
    print("\nComparing Serverless vs Free5GC:")
    shared_keys = set(b1.keys()) & set(b2.keys())
    shared_keys = set(b1.keys()) & set(b2.keys())
    print("\nDetailed Comparison (All Buckets):")
    for k in sorted(shared_keys, key=lambda x: float(x) if x != '+Inf' else float('inf')):
        try:
            count1 = int(b1[k])
            count2 = int(b2[k])
            print(f"le={k:<6}: S={count1:<6}, F={count2:<6} {'(MATCH)' if count1 == count2 else '(DIFF)'}")
        except ValueError:
             print(f"le={k:<6}: S={b1[k]}, F={b2[k]} (Type Error)")

if isinstance(b2, dict) and isinstance(b3, dict):
     print("\nComparing Free5GC vs Open5GS:")
     shared_keys = set(b2.keys()) & set(b3.keys())
     for k in sorted(shared_keys, key=lambda x: float(x) if x != '+Inf' else float('inf'))[:5]:
        print(f"le={k}: F={b2[k]}, O={b3[k]}")
