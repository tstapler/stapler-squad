# Research (codebase-verified via grep/read)
- Stuck taxonomy: `domain.StuckReason*` constants, reconcilers in `session/backlog_lifecycle.go` (e.g. reconcileBouncingItems ~L1900-1965 using MarkStuck / FindOpenStuckStates / MarkStuckNotified / l.notify). Pure decision helpers live in `session/stuck_decisions.go` (`isBouncing` L94: `count>=threshold && !hasPass`) — bouncing explicitly excludes PASS, so the PASS-in-review no-op loop is uncovered.
- `selfHealStuck` (~L2000-2080) has a per-reason switch resolving rows; any new reason MUST get a case (comment at L2000 says so).
- `report_duplicate`: `server/mcp/tools_backlog.go` ~L2112-2260. Duplicate claim recorded as `duplicate_ref=<url> ...` line in ItemSession.VerificationNotes; item moved to review. Idempotent retry returns "already recorded ... (status already review) — no changes made." — no pending-confirmation wording.
- Dispatch gate location for /backlog/ship sessions: not yet located; plan task 1 is to find it (grep dispatch/ship in session/backlog_*.go and server/services/backlog_service_*.go).
- Existing docs: docs/tasks/backlog-stuck-item-auto-remediation.md, docs/reference/backlog-completion-gate-and-cleanup.md.
- Gap: no dispatch-count/no-diff signal exists; must derive from activity log entries or a counter on the item.
## Design options
A. Derive from activity log (no schema change) — fragile, log parsing.
B. Per-item counter `noop_dispatch_count` reset on new commit/status change — needs ent schema field (do not commit generated ent). Recommended.
C. Duplicate-pending as derived state from VerificationNotes marker (no schema) — recommended; expose in proto as `duplicate_pending` + `duplicate_ref`.
