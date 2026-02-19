import os

def read_file(path):
    with open(path, 'r') as f:
        return f.read()

def main():
    main_tex_path = "main.tex"
    latency_tikz_path = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results/latency_figure.tex"
    resource_tikz_path = "/home/tdinh/WorkingSpace/02_Development/serverless5gc/eval/results/resource_figure.tex"

    main_content = read_file(main_tex_path)
    latency_tikz = read_file(latency_tikz_path)
    resource_tikz = read_file(resource_tikz_path)

    # placeholders
    # The placeholders were seen in earlier view_file of main.tex (not this one yet, but I saw them)
    # \fbox{\parbox{0.9\columnwidth}{\centering\vspace{2cm}[Pending: latency comparison figure]\vspace{2cm}}}
    # \fbox{\parbox{0.9\columnwidth}{\centering\vspace{2cm}[Pending: resource comparison figure]\vspace{2cm}}}
    
    # I need to match strictly.
    # Let's clean the main content first if it has line numbers or something?
    # No, view_file output output.txt might contain the file content directly.
    # But wait, read_file from MCP returned a JSON string?
    # "The output was large and was saved to: ..."
    # The file contains the raw output of the tool.
    # If the tool returned a JSON string, I need to parse it.
    
    import json
    try:
        # The file content might be the raw text if view_file saved it directly.
        # But usually it saves the *tool result*.
        # Let's inspect the file content first in the next step. 
        # But assuming it's the text content...
        pass
    except:
        pass

    # Actually, I'll just write the replacement logic assuming I get the text.
    
    # Placeholder 1
    p1 = r"\fbox{\parbox{0.9\columnwidth}{\centering\vspace{2cm}[Pending: latency comparison figure]\vspace{2cm}}}"
    # Placeholder 2
    p2 = r"\fbox{\parbox{0.9\columnwidth}{\centering\vspace{2cm}[Pending: resource comparison figure]\vspace{2cm}}}"
    
    # Replace
    new_content = main_content.replace(p1, latency_tikz)
    new_content = new_content.replace(p2, resource_tikz)
    
    if p1 not in main_content:
        print("Warning: Latency placeholder not found")
        # specific check for variations?
    
    if p2 not in main_content:
        print("Warning: Resource placeholder not found")

    with open("main_updated.tex", "w") as f:
        f.write(new_content)

if __name__ == "__main__":
    main()
