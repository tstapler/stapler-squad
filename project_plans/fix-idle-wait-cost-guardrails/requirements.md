# Requirements: fix-idle-wait-cost-guardrails

Item ec5e3618 — a triage session (conv 63a91916…, 2026-09-22) cost $106.69: 1094 turns, 93% cache reads, caused by an unbounded idle "waiting on background subagents" wake loop.

## Asks
1. Bound idle/waiting turns (compact, then end/hand back) so history stops growing with no substantive work.
2. Determine whether `SESSION_TOKEN_CEILING` fired; consider per-role thresholds for cheap pipeline stages (triage).
3. Prevent recurrence (complements #878, which is visibility only).

## Scope
In: `server/services/capacity_monitor.go`, `config/types.go`, `session/tokens/findings.go`, cost estimation, per-role (`session.SessionRole*`) limits.
Out: #878 telemetry persistence, Stop-hook notifier (#881).

## Acceptance criteria
See JSON output; summarized: cache-aware cost in the monitor; per-role cost/turn caps for triage; terminal action after failed compact; ceiling findings actionable not just passive; regression tests from the real turn shape.
