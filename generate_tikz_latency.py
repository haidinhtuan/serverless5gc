import csv

def generate_tikz_latency(csv_file):
    data = []
    with open(csv_file, 'r') as f:
        reader = csv.DictReader(f)
        for row in reader:
            if row['Scenario'] == 'medium':
                data.append(row)

    # Order: serverless-sctp, free5gc, open5gs
    order = ['serverless-sctp', 'free5gc', 'open5gs']
    ordered_data = []
    for target in order:
        for row in data:
            if row['Target'] == target:
                ordered_data.append(row)
                break
    
    # Map target names to display names if needed
    display_names = {
        'serverless-sctp': 'Serverless',
        'free5gc': 'Free5GC',
        'open5gs': 'Open5GS'
    }

    tikz_code = r"""
\begin{tikzpicture}
    \begin{axis}[
        ybar,
        bar width=15pt,
        width=\columnwidth,
        height=6cm,
        ylabel={Latency (s)},
        symbolic x coords={p50, p95, p99},
        xtick=data,
        nodes near coords,
        nodes near coords align={vertical},
        every node near coord/.append style={font=\tiny, /pgf/number format/.cd, fixed, precision=3},
        ymin=0, ymax=0.04, % Adjusted based on data range
        legend style={at={(0.5,-0.15)}, anchor=north, legend columns=-1},
        enlarge x limits=0.2,
        grid=y,
    ]
"""

    for row in ordered_data:
        target = row['Target']
        name = display_names.get(target, target)
        p50 = float(row['p50'])
        p95 = float(row['p95'])
        p99 = float(row['p99'])
        
        tikz_code += f"    \\addplot coordinates {{ (p50,{p50}) (p95,{p95}) (p99,{p99}) }};\n"
        tikz_code += f"    \\addlegendentry{{{name}}}\n"

    tikz_code += r"""
    \end{axis}
\end{tikzpicture}
"""
    return tikz_code

if __name__ == "__main__":
    csv_path = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results/latency_percentiles.csv"
    print(generate_tikz_latency(csv_path))
