# UX Design: Hook Classification Fast Path

**Date**: 2026-09-23
**Surface type**: non-interactive infrastructure with CLI diagnostics, installer output, and dashboard status

## Surface 1: Normal hook operation

Representative output:

```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"seed-safe-git: read-only command"}}
```

Acceptance criteria:
- Successful operation emits only protocol-required stdout; no success chatter.
- p95 is below 20 ms and p99 below 50 ms when warm.
- Errors never mix diagnostic prose into decision JSON.
- AskUserQuestion and final-defer paths emit empty stdout.

## Surface 2: `ssq-hooks doctor`

Representative output:

```text
Hook classifier: DEGRADED
Instance: named/manual (fingerprint a1b2c3d4)
Primary: unavailable (connect timeout)
Protocol: client 1; server unknown
Rule cache: valid, version 42, age 18s
Fallback: cached decisions, then Claude defer
Analytics: unavailable with primary
Action: start the manual instance or verify SSQ_HOOK_SOCKET
```

Acceptance criteria:
- One command reports endpoint class, shortened fingerprint, protocol compatibility, cache freshness, fallback state, and actor health.
- No command, cwd, environment value, token, session ID, full socket path, or tool-use ID appears.
- Status is conveyed in text, not color alone.
- Every degraded/error state provides one remediation and exits promptly.
- JSON output is available for automation.

## Surface 3: Installer/direct replacement

Representative output:

```text
Installed Stapler Squad hook fast path.
Removed duplicate classifier entries: 1
Preserved unrelated PreToolUse hooks: 4
Primary protocol: compatible (v1)
Settings backup: ~/.claude/settings.json.<timestamp>.bak
Rollback: ssq-hooks install claude --rollback
```

Acceptance criteria:
- Reports duplicate count and unrelated-hook preservation.
- Writes old or new complete JSON at every interruption point; never partial JSON.
- Shows compatibility before cutover and an exact rollback action.
- Does not print settings contents or secrets.

## Surface 4: Dashboard

```text
┌ Hook Decision Latency ────────┐  ┌ Decision Path ──────────────┐
│ p50  3 ms  p95 12 ms          │  │ Primary 99.7%               │
│ p99 31 ms  SLO 50 ms          │  │ Cache    0.2%               │
└───────────────────────────────┘  │ Defer    0.1%               │
                                   └──────────────────────────────┘
┌ Analytics Actor ──────────────┐  ┌ Correctness ────────────────┐
│ Queue 12/1000  Oldest 1 ms    │  │ Identity mismatch rejected 2│
│ Batch p95 128  Drops 0        │  │ Accepted mismatch 0          │
└───────────────────────────────┘  └──────────────────────────────┘
```

Acceptance criteria:
- Production and named/test instance classes are filterable and not silently merged.
- Healthy/degraded/error use textual labels and accessible table/panel names.
- p95/p99/max panels render data; current no-data percentile failure is fixed.
- Baseline annotation records 4,419 calls/day, 38,661 ms average, and ~40/min peak.
- Correctness and rollback thresholds are visible.

## Flows and exit paths

```text
Hook request
  ├─ primary healthy → decision → exit
  ├─ primary unavailable + valid cache → cached decision → exit
  └─ primary unavailable + unusable cache → empty decision/defer → exit

Install
  ├─ protocol healthy → atomic normalization → success/rollback command
  └─ protocol unhealthy → leave old config intact → remediation/exit

Doctor
  ├─ healthy → summary/exit
  ├─ degraded → remediation/exit
  └─ mismatch → no reroute; upgrade/restart guidance/exit
```

No flow waits indefinitely and no isolated flow exits through the production endpoint.
