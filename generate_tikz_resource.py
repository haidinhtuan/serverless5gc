import csv
import statistics

def generate_tikz_resource(csv_file):
    # Data aggregation
    grouped_data = {} # (target, scenario) -> {cpu: [], mem: []}
    
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            target = row['target']
            scenario = row['scenario']
            key = (target, scenario)
            
            if key not in grouped_data:
                grouped_data[key] = {'cpu': [], 'mem': []}
                
            grouped_data[key]['cpu'].append(float(row['total_cpu_seconds']))
            grouped_data[key]['mem'].append(float(row['avg_memory_mb']))

    # Filter for 'medium' scenario as per likely requirement (same as latency)
    scenario_filter = 'medium'
    targets = ['serverless-sctp', 'free5gc', 'open5gs']
    display_names = {'serverless-sctp': 'Serverless', 'free5gc': 'Free5GC', 'open5gs': 'Open5GS'}

    # Prepare data for TikZ
    # We will likely want two subplots or a double y-axis chart, or just CPU for now if resource figure implies Cost/Resource.
    # Let's assume comparisons of CPU usage and Memory usage.
    
    # Calculate averages
    avgs = {}
    for target in targets:
        key = (target, scenario_filter)
        if key in grouped_data:
            cpu_vals = grouped_data[key]['cpu']
            mem_vals = grouped_data[key]['mem']
            avgs[target] = {
                'cpu': statistics.mean(cpu_vals),
                'mem': statistics.mean(mem_vals)
            }
        else:
             avgs[target] = {'cpu': 0, 'mem': 0}

    # Generate TikZ
    # Grouped bar chart: CPU vs RAM? Or just one?
    # Let's do a chart with two y-axes or two sets of bars.
    # Actually, usually resource comparison separates CPU and RAM.
    # Let's create a grouped bar chart with "CPU (s)" and "Memory (MB)" groups on x-axis?
    # Or maybe just CPU.
    # Let's try to fit both.
    
    tikz_code = r"""
\begin{tikzpicture}
    \begin{axis}[
        ybar,
        bar width=15pt,
        width=\columnwidth,
        height=6cm,
        ylabel={Normalized Resource Usage},
        symbolic x coords={CPU, Memory},
        xtick=data,
        nodes near coords,
        nodes near coords align={vertical},
        every node near coord/.append style={font=\tiny, /pgf/number format/.cd, fixed, precision=0},
        ymode=log,
        log origin=infty,
        ymin=100,
        legend style={at={(0.5,-0.15)}, anchor=north, legend columns=-1},
        enlarge x limits=0.5,
        grid=y,
        % Scaling to make them comparable? Or just two separate plots?
        % Let's use specific units in legend or axes
        ylabel={Usage (Seconds / MB)},
    ]
"""
    
    for target in targets:
        cpu = avgs[target]['cpu']
        mem = avgs[target]['mem']
        name = display_names.get(target, target)
        
        # We need to handle the scale difference. CPU is ~60000, Mem is ~400-6000.
        # Maybe logarithmic? Or just plot CPU.
        # Let's just plot them as is, but it might be ugly.
        # Actually, let's normalize to Free5GC?
        # Or just plot values.
        
        tikz_code += f"    \\addplot coordinates {{ (CPU,{cpu:.0f}) (Memory,{mem:.0f}) }};\n"
        tikz_code += f"    \\addlegendentry{{{name}}}\n"

    tikz_code += r"""
    \end{axis}
\end{tikzpicture}
"""
    return tikz_code

if __name__ == "__main__":
    csv_path = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results/summary.csv"
    print(generate_tikz_resource(csv_path))
