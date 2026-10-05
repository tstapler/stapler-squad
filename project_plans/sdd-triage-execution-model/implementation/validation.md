# Validation

| AC | Covered by | Gate |
|---|---|---|
| AC1 | findings.md tables (DB-derived, queries inline) + documented gap/instrumentation (`turns`,`subagents` log fields) | PASS: real data for cost/duration; turn/subagent counts for completed calls documented as a gap; instrumentation plan = stats hook on `CallOptions` (follow-up), aborts already carry counters |
| AC2 | findings.md AC2 table (`ps` RSS, 17 live sessions) | PASS: measured; point-in-time caveat stated |
| AC3 | ADR-029 | PASS |
| AC4 | ADR-029 decision 4 + data (default max $6.34, p90 11.2 min); mode-gating test | PASS |
| AC5 | `fanout_test.go` counter tests; pool replay test of #882 shape cancelling before the 3h budget; config default test | must pass before done |
| AC6 | ADR-029 "Interactions" | PASS |

Readiness gate: PASS — every AC maps to an artifact or a named test; no AC depends on unmeasured data.

## Pre-mortem
- Ceiling fires on a legit large item → configurable, `0` disables, distinct `end_reason`, generous defaults.
- Counter mis-parses a stream-format change → unknown lines are ignored (fail-open); tests pin both `Agent` and `Task`.
- Seed is create-if-absent: the prompt-budget text only reaches fresh installs; the stream ceiling is the real enforcement.
- Decision made on thin data → data gaps are listed explicitly; ADR schedules re-tuning from the new log fields.
- Retry loop via shared remediation backoff (found by sdd:6-verify) → reconciler gate skips `fanout_ceiling`; tested.
- Conflict with #884 → independent accumulators, documented.
