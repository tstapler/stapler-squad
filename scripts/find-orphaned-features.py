#!/usr/bin/env python3
import os
import re
import sys

def find_orphaned():
    src_dir = "web-app/src"
    if not os.path.exists(src_dir):
        print(f"Error: {src_dir} not found. Run this from the project root.")
        sys.exit(1)

    candidates = []
    all_search_files = []
    
    # 1. Walk and collect files
    for root, dirs, files in os.walk(src_dir):
        # Exclude gen, __mocks__, __tests__, node_modules
        if any(p in root for p in ["/gen", "/__mocks__", "/__tests__", "node_modules"]):
            continue
        for file in files:
            if not (file.endswith(".ts") or file.endswith(".tsx")):
                continue
            if file.endswith(".css.ts") or file.endswith(".test.ts") or file.endswith(".test.tsx") or file.endswith(".spec.ts") or file.endswith(".spec.tsx"):
                continue
            
            full_path = os.path.join(root, file)
            all_search_files.append(full_path)
            
            # Candidates to check (must not be routing entry points)
            if file not in ["page.tsx", "layout.tsx", "route.ts", "loading.tsx", "error.tsx", "not-found.tsx", "global-error.tsx", "middleware.ts"]:
                candidates.append(full_path)

    print(f"Scanning {len(candidates)} candidate files, searching in {len(all_search_files)} total files for references...")
    
    # 2. Build map of content
    contents = {}
    for path in all_search_files:
        try:
            with open(path, "r", encoding="utf-8") as f:
                contents[path] = f.read()
        except Exception as e:
            print(f"Warning: failed to read {path}: {e}")

    orphaned = []
    for path in candidates:
        base_name = os.path.basename(path)
        name_without_ext = os.path.splitext(base_name)[0]
        
        # Check if the component name is referenced in any other file's content
        referenced = False
        for other_path, content in contents.items():
            if other_path == path:
                continue
            
            # Use regex to find references: module name or component name as a word boundary
            pattern = r'\b' + re.escape(name_without_ext) + r'\b'
            if re.search(pattern, content):
                referenced = True
                break
        
        if not referenced:
            orphaned.append(path)

    print("\n--- Orphaned / Unused Source Files ---")
    if not orphaned:
        print("None found!")
    else:
        for p in sorted(orphaned):
            print(p)

if __name__ == "__main__":
    find_orphaned()
