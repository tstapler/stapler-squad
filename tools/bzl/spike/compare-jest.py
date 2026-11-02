#!/usr/bin/env python3
"""Compare the pass sets (file, full test name, status) of two `jest --json` outputs."""
import json, os, sys

def load(p):
    d = json.load(open(p))
    out = set()
    for r in d["testResults"]:
        f = os.path.basename(os.path.dirname(r["name"])) + "/" + os.path.basename(r["name"])
        f = r["name"].split("/web-app/")[-1] if "/web-app/" in r["name"] else f
        for a in r["assertionResults"]:
            out.add((f.split("/src/", 1)[-1] if "/src/" in f else f, a["fullName"], a["status"]))
    return d, out

da, a = load(sys.argv[1]); db, b = load(sys.argv[2])
print("suites", da["numTotalTestSuites"], db["numTotalTestSuites"], "tests", da["numTotalTests"], db["numTotalTests"],
      "passed", da["numPassedTests"], db["numPassedTests"])
print("only-a", len(a - b), "only-b", len(b - a), "equal", a == b)
sys.exit(0 if a == b and da["numTotalTests"] == db["numTotalTests"] else 1)
