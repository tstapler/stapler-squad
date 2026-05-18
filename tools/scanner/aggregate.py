#!/usr/bin/env python3
"""Aggregate per-feature JSON files into the legacy monolithic registry format.

Usage:
  python3 tools/scanner/aggregate.py <features-dir> <output-file>

Example:
  python3 tools/scanner/aggregate.py docs/registry/features/backend docs/registry/backend-features.json
  python3 tools/scanner/aggregate.py docs/registry/features/frontend docs/registry/frontend-features.json
"""
import json
import os
import sys
import glob


def aggregate_backend(features_dir: str) -> list[dict]:
    entries = []
    for path in sorted(glob.glob(os.path.join(features_dir, "**/*.json"), recursive=True)):
        with open(path) as f:
            doc = json.load(f)
        entry = {
            "id": doc["id"],
            "type": doc["type"],
            "backend": {
                "service": doc["service"],
                "method": doc["method"],
                "protoFile": doc.get("protoFile", ""),
                "markerFound": doc.get("markerFound", False),
            },
            "tested": doc.get("tested", False),
            "testIds": doc.get("testIds", []),
            "lastModified": doc.get("lastModified", ""),
        }
        if doc.get("handlerFile"):
            entry["backend"]["handlerFile"] = doc["handlerFile"]
        entries.append(entry)
    return entries


def aggregate_frontend(features_dir: str) -> list[dict]:
    entries = []
    for path in sorted(glob.glob(os.path.join(features_dir, "**/*.json"), recursive=True)):
        with open(path) as f:
            doc = json.load(f)
        entry = {
            "id": doc["id"],
            "type": doc["type"],
            "frontend": {
                "component": doc["component"],
                "path": doc["path"],
                "markerLine": doc.get("markerLine", 0),
            },
            "tested": doc.get("tested", False),
            "testIds": doc.get("testIds", []),
            "lastModified": doc.get("lastModified", ""),
        }
        entries.append(entry)
    return entries


def main():
    if len(sys.argv) != 3:
        print(f"Usage: {sys.argv[0]} <features-dir> <output-file>", file=sys.stderr)
        sys.exit(2)

    features_dir = sys.argv[1]
    output_file = sys.argv[2]

    # Detect type from directory name
    if "frontend" in features_dir:
        features = aggregate_frontend(features_dir)
    else:
        features = aggregate_backend(features_dir)

    registry = {
        "version": "1",
        "features": features,
    }

    os.makedirs(os.path.dirname(output_file) or ".", exist_ok=True)
    with open(output_file, "w") as f:
        json.dump(registry, f, indent=2)
        f.write("\n")

    print(f"Wrote {len(features)} features to {output_file}")


if __name__ == "__main__":
    main()
