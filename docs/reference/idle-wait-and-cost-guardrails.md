# Idle-wait and cost guardrails

`CapacityMonitor` (`server/services/capacity_monitor.go`) polls live Claude sessions every `poll_interval_seconds` and applies these guardrails. They exist because a triage session once spent $106 re-reading a cached history across ~1,000 "still waiting" wake turns (#882).

## Cost

Session cost comes from `tokens.PricingTable.EstimateCost`, which prices input, output, cache-creation and cache-read tokens. Cache reads dominate long sessions, so a cost estimate without them cannot trip a budget. If any model in the session is unpriced, the older input/output-only estimate is used instead of an understated total.

The budget for a session is `role_cost_budget_usd[role]` if that role is present, otherwise `cost_budget_usd` (0 = no limit). On breach:

- **Pipeline roles** (`triage`, `review`, `diagnose`): one high-priority notification, then the session's ItemSession is ended with reason `cost_budget_exceeded` and the session is killed, so the orchestrator can respawn or flag the item. Happens once per session.
- **Other sessions**: the existing capacity warning (and optional auto-transition).

## Idle-wait loops

An idle-wait turn uses only `Monitor`, `ListAgents`, `TaskStop`, `SendMessage`, or no tools. A run trips when either:

- its length reaches the ceiling (`role_idle_wait_turn_ceiling[role]`, else `idle_wait_turn_ceiling`), or
- it has at least 3 turns spanning `idle_wait_max_minutes` between its first and last turn.

First trip: `/compact` plus an info notification. If the run trips again strictly after the compact: one high-priority notification and, for a backlog-linked session, the session is stopped with end reason `idle_wait_loop_escalation`. Sessions with no backlog role are notified only. The escalation fires once per loop; a real (non-idle) turn clears the state.

## Token ceiling

A critical `SESSION_TOKEN_CEILING` finding (`session/tokens/findings.go`) on a live session now also produces one notification per session, instead of surfacing only on the Insights page.

## Config keys (`capacity` in `config.json`)

| Key | Default | Meaning |
|---|---|---|
| `cost_budget_usd` | 0 (off) | Global cost budget |
| `role_cost_budget_usd` | `{"triage": 10}` | Per-role budget; explicit 0 disables for that role |
| `idle_wait_turn_ceiling` | 15 | Consecutive idle turns before compact |
| `role_idle_wait_turn_ceiling` | `{"triage": 10}` | Per-role ceiling |
| `idle_wait_max_minutes` | 30 | Wall-clock span of an idle run that trips the guardrail |
