# Plan: fix-idle-wait-cost-guardrails

Architecture: extend existing `CapacityMonitor` (no new subsystem). Reuse `tokens.PricingTable.EstimateCost`, event-bus notifications, config defaults in `config/types.go`.

## Tasks
1. Cache-aware cost: replace `estimateCost(input,output)` for the anthropic path with `PricingTable.EstimateCost(parseRes)`; fall back to old path when model unpriced. Tests with cache-heavy fixture (`session/tokens/testdata/cache_heavy.jsonl`).
2. Per-role budget config: `RoleCostBudgetUSD map[string]float64` (e.g. triage: 10) and `RoleIdleWaitTurnCeiling` in `config/types.go`; look up role via backlog item session summary in `evaluateInstance`; `checkThresholds` takes role-effective budget.
3. Terminal action: on `cost_budget_exceeded` for a pipeline role, or idle-loop escalation, stop/pause the session and mark it for orchestrator hand-back (reuse existing transition path `handleTransitionTrigger` / backlog lifecycle end-reason) instead of notify-only; don't delete tracker so it doesn't loop.
4. Wall-clock bound: track idle-run duration (turn timestamps) and trigger when run > `IdleWaitMaxMinutes` (default 30), in addition to turn count.
5. Widen idle detection: allowlist add ToolSearch/TaskOutput/TaskGet and Bash-sleep-only turns? Decide via real transcript replay (task 6); keep allowlist narrow.
6. Replay test: fixture jsonl reproducing the incident turn shape; assert compact at 15, escalation + stop after.
7. Make `SESSION_TOKEN_CEILING` actionable: emit notification from the monitor path when ComputeFindings critical for a live session (dedupe per session), or document it as passive-only.
8. Docs: `docs/reference/` note on idle-wait + cost guardrails and config keys; `make ready`.

## Adversarial review
- Risk: stopping a legitimately long-running triage (many subagents) → mitigate with generous role budget, wall-clock only counts idle turns, notify before stop (grace turn).
- Risk: `/compact` while subagents run could drop pending context → #885 judged non-destructive; verify.
- Risk: role lookup from monitor couples session package to backlog; use snapshot field/attach role at session creation instead of DB lookup per poll.
- Risk: pricing table missing model → fallback, never fire understated cost.
- Risk: duplicate gates (dupl) in tests; use table tests.
