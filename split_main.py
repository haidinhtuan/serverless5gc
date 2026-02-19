import os
import re

def main():
    try:
        with open('main_updated.tex', 'r') as f:
            content = f.read()
    except FileNotFoundError:
        print("Error: main_updated.tex not found.")
        return

    # Define sections to split
    # Note: references typically start with \bibliographystyle
    sections = [
        ("introduction", r"\\section\{Introduction\}"),
        ("background", r"\\section\{Background and Related Work\}"),
        ("architecture", r"\\section\{System Architecture\}"),
        ("evaluation", r"\\section\{Evaluation\}"),
        ("conclusion", r"\\section\{Conclusion\}"),
        ("references", r"\\bibliographystyle\{IEEEtran\}") 
    ]
    
    keys = []
    indices = []
    
    # Find start indices
    for key, pattern in sections:
        m = re.search(pattern, content)
        if m:
            keys.append(key)
            indices.append(m.start())
        else:
            print(f"Warning: Section {key} not found.")

    if not keys:
        print("No sections found to split.")
        return

    # Sort checks to ensure order
    # (Assuming sections appear in order, but good to be safe)
    # Actually, let's rely on the order in the list matching the file for simplicity in reconstruction
    # But indices must be sorted to slice correctly.
    
    combined = sorted(zip(indices, keys))
    indices = [x[0] for x in combined]
    keys = [x[1] for x in combined]
    
    # Append end of file index
    indices.append(len(content))
    
    # Create directory
    if not os.path.exists("sections"):
        os.makedirs("sections")
        
    # Preamble + Abstract (everything before first section)
    # We'll check if introduction is the first
    first_idx = indices[0]
    preamble = content[:first_idx]
    
    # Write sections
    for i in range(len(keys)):
        key = keys[i]
        start = indices[i]
        end = indices[i+1]
        # Remove trailing newlines for cleanliness? Not strictly necessary.
        section_content = content[start:end]
        
        with open(f"sections/{key}.tex", "w") as f:
            f.write(section_content)
        print(f"Created sections/{key}.tex ({len(section_content)} bytes)")

    # Create new main.tex
    new_main = preamble
    for key in keys:
        new_main += f"\\input{{sections/{key}}}\n"
    
    # If the last section didn't include \end{document} (unlikely if we slice to EOF), check.
    # The 'references' section starts at \bibliographystyle and goes to EOF, so it includes \end{document}.
    
    with open("main_split.tex", "w") as f:
        f.write(new_main)
    print(f"Created main_split.tex ({len(new_main)} bytes)")

if __name__ == "__main__":
    main()
