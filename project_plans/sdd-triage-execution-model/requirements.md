# Requirements: sdd-triage-execution-model

Item 6f262fa7. Decide whether "sdd" pipeline-mode triage should leave `claude -p` headless execution. Research + decision, not an implementation.

## Problem
Triage runs via headless `CallBlocking` (`server/services/backlog_service_trigger_triage.go`, gated by `triageSem`, cap 8, `backlog_service.go:567`). It cannot be observed or intervened on mid-flight. The #882 incident ($106.69, 1094 turns, 262 subagent completions) happened in sdd mode. Existing backstops: `headless.idleTimeout` (hung calls only) and `triageCallBudget` = 3h (`backlog_service_triage.go:411`; wall-clock only).
`CapacityMonitor` (`capacity_monitor.go`, `IdleWaitTurnCeiling`) can observe/compact only tmux-backed sessions.

## Options
1. tmux-backed session for sdd-mode triage only.
2. Bound sdd-mode fan-out (phase/subagent caps, prompt + enforcement).
3. Cost-aware call budget (#884) as a backstop for either model.

## Acceptance criteria
- AC1: Call-duration, turn-count and cost distributions for sdd vs default triage are measured from real data (or the absence of data is documented with an instrumentation plan).
- AC2: Per-tmux-session memory/PTY overhead in this deployment is measured and compared with 8 concurrent triage calls.
- AC3: A decision record (ADR) selects option 1, 2, 3 or a hybrid with explicit rationale and rejected alternatives.
- AC4: Default (non-sdd) triage remains on `-p` unless data shows material waste.
- AC5: Whatever is chosen has a testable backstop that would have stopped the #882 pattern before $X / N turns (thresholds configurable).
- AC6: The decision names how it interacts with `triageSem` concurrency, `triageCallBudget`, and liveness/orphan reconciliation.

## Non-goals
Migrating all triage to tmux; implementing code in this item.
