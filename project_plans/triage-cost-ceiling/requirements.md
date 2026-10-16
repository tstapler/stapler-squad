# triage-cost-ceiling — requirements

## Problem
Headless sdd triage calls (`server/services/backlog_service_trigger_triage.go` → `triageCaller.CallBlocking`)
are bounded only by `headless.idleTimeout` (10m silence, `session/headless/pool.go:98`) and
`triageCallBudget` (3h wall-clock, `server/services/backlog_service_triage.go:411`). A call that is
continuously busy but wasteful (the $106.69 incident behind #882/#883: 65 min, 93% cache-read replay)
trips neither; a full 3h run could exceed $300.

## Goal
Add a cost/token-aware backstop, independent of elapsed time, that aborts a headless triage call whose
accumulated usage far exceeds what triage should cost.

## Constraints
- Do NOT shrink `triageCallBudget`; it is coupled to `session.maxHeadlessTriageSessionStaleness` (BUG-055).
- Ceiling must be generous (abort runaway only, not normal runs) and configurable.
- Aborting must kill the subprocess, surface a distinct non-retryable error type, and preserve partial output for capture.

## Acceptance criteria
1. Mid-stream, the pool accumulates token usage (input, output, cache-creation, cache-read) from assistant stream-json lines, de-duplicated per message id.
2. `CallOptions` accepts a USD ceiling (and/or token ceiling); zero = disabled; other callers are unaffected.
3. When estimated cost exceeds the ceiling, the subprocess is killed and the call returns `ErrCostCeilingExceeded`.
4. Triage passes a ceiling from config with a documented default; value is overridable.
5. `classifyHeadlessCallError` maps it to a distinct type (`cost_ceiling`), not retried as timeout/idle, and recorded on the item with the spend at abort.
6. `triageCallBudget` and `maxHeadlessTriageSessionStaleness` are unchanged and BUG-055 invariant tests still pass.
7. Unit tests cover under-ceiling success, over-ceiling abort, duplicated-message-id dedupe, unknown-model pricing fallback, disabled ceiling.
