# Research findings (all VERIFIED by reading source in this worktree)

## Ask #1 is partly shipped (#885, merged 2026-09-29, after the 09-22 incident)
- `server/services/capacity_monitor.go` `checkIdleWaitLoop`: after `IdleWaitTurnCeiling` (default 15, `config/types.go:781`) consecutive idle turns it sends `/compact`; if a second ceiling's worth of idle turns follows, `escalateIdleWaitLoop` publishes a notification only and deletes the tracker (so it re-arms forever; nothing ends/hands back the session).
- Idle turn = tool names ⊆ {Monitor, ListAgents, TaskStop, SendMessage} or no tools (`idleWaitSignalTools`). A wake turn that also uses ToolSearch, Bash(sleep), Read of task output, etc. breaks the run → pattern evades detection. Unverified against the raw 63a91916 transcript (not in this worktree).
- Evaluation only runs for `Status == Active`, only when a provider client exists and `GetClaudeConversationUUID() != ""`, polled every 60s (`Start`). Turn-count based, not wall-clock (item asked "N turns or M minutes").

## Ask #2
- `sessionTokenCeiling = 2_000_000` (`session/tokens/findings.go:33`). 494M tokens ≫ 2M and cost > $20 ⇒ `detectSessionTokenCeiling` returns SeverityCritical. So it WOULD fire — but `ComputeFindings` is only called from `server/services/insights_service.go:520`, i.e. a passive Insights page render. No enforcement, no notification.
- Ceiling is global (calibrated to flag 30.8% of ~600 sessions); no per-role variant.
- **Bug found:** `CapacityMonitor.estimateCost` (capacity_monitor.go ~559) prices only input+output tokens and ignores cache read/creation, so `CostBudgetUSD` (default 0 = off, `checkThresholds` ~281) could never trip on this incident (cache reads were $98.88 of $106.69; fresh input $0.0044). `tokens.PricingTable.EstimateCost(r)` already prices all four categories.
- `CostBudgetUSD` is global; roles exist (`session/backlog.go:50` `SessionRoleTriage`, etc.) but the monitor does not consult them.

## Ask #3
Compaction cannot be the only guard; idle loops waiting on subagents that never finish need a terminal action (stop + report to orchestrator).
