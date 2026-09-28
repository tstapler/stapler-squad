# Requirements: backlog-diagnose-and-nudge

**Date**: 2026-09-24
**Type**: feature addition (cross-cutting: backend RPC + agent-dispatch orchestration, frontend UI, session-lifecycle safety mechanisms)
**Complexity**: 4 — cross-cutting change with Large appetite, autonomous-write safety implications

## Problem Statement
Backlog items and their linked sessions periodically get stuck (rework-cap hits, non-converging review/rework cycles, silently-misrouted messages like `ce71ad1a`, or simply an idle session nobody nudged) with no automated way to investigate or resolve without Tyler manually pulling session state, logs, and backlog history and either filing a bug or nudging the session by hand — as done manually this session for backlog items `e6c2a88e`/`ce71ad1a`. The shipped `backlog-stuck-item-visibility` feature (`project_plans/backlog-stuck-item-visibility/`) solved *seeing* that items are stuck; it explicitly declined to *act* on them ("visibility, not a control panel"). That gap is now the bottleneck: visibility alone hasn't reduced how often Tyler has to intervene by hand.

## Baseline
Today: `StuckItemsSection`/`StuckItemDetail` (`web-app/src/components/backlog-stuck/`) surface that an item is stuck and why. Resolving it still requires Tyler to open a terminal/session, manually gather backlog item history, linked-session state, logs, and git diff, form a hypothesis, and either file a bug (`create_backlog_item`) or nudge the session himself (`resume_session`/`steer_session`/`write_to_session`). This is the exact manual workflow performed in this session for items `e6c2a88e` and `ce71ad1a-a6a5-485f-8245-c5a502754a8b`.

## Users / Consumers
Single user (Tyler), via the stapler-squad web UI — same user/consumer model as the sibling `backlog-stuck-item-visibility` plan. No other consumers; no multi-tenant considerations.

## Success Metrics
Primary: the number of times Tyler personally has to manually diagnose a stuck/misbehaving backlog item or session (the `e6c2a88e`/`ce71ad1a`-style investigation) trends toward zero for the stuck-reason classes this feature covers. Verified qualitatively (Tyler's own account of "did I have to do this by hand this week") since this is a single-user internal tool with no telemetry pipeline for this today — not a dashboarded KPI. Secondary/proxy, if useful at ship time: count of Diagnose dispatches that resulted in either a filed bug or a successful nudge (session transitioned out of idle) vs. ones that were inconclusive.

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- Single-developer, self-hosted instance — no multi-tenant or auth considerations beyond what already exists (mirrors `backlog-stuck-item-visibility`'s Constraints section).
- Nudge actions are genuinely autonomous writes to a live, possibly-attended session — the identity-reverification and idle-gating requirements below are hard constraints, not nice-to-haves, given the `ce71ad1a` bug this same conversation just found (a session-name collision silently delivered one session's message into an unrelated, attended session).
- Existing `docs/registry/` feature-registry rule applies: any new RPC/UI component needs a registry entry.
- No deadline beyond the stated appetite.

## Non-functional Requirements
- **Performance SLO**: not specified — low-traffic, single-user internal tool. A Diagnose dispatch is user- or reconciler-triggered, not a hot path.
- **Scalability**: not applicable (single user, backlog size in the tens of items).
- **Security classification**: internal. Diagnose dispatch reads session/backlog content and can write to a live session — treat context bundle contents as no more sensitive than what backlog work sessions already see (same trust boundary as existing review/work session dispatch).
- **Data residency**: no special requirements.

## Scope
### In Scope
- New RPC/service to assemble a diagnostic context bundle for a backlog item + its linked session(s): item description/AC/status/history, prior review verdicts, linked session(s)' `Snapshot()` state, recent relevant log lines, git diff/log for any in-flight worktree.
- Context bundle capped at ~250,000 tokens via `session/tokens` (already used by `server/services/quota_gate.go`), with compaction rather than hard truncation when the raw bundle would exceed budget. Compaction leans on the harness's native `/compact` for in-session growth and reuses `HandoffSummaryGenerator` (`session/handoff_summary_service.go`) for cross-session handoff summaries specifically — not a third, bespoke summarization scheme.
- A "Diagnose" action wired into `StuckItemDetail.tsx` (stuck items) and `BacklogItemDetail.tsx` (any item, not only ones already flagged stuck — matches Tyler's original "stops working for some reason" framing).
- Dispatch of a new diagnostic agent session carrying the bundle, instructed to choose one of: file a bug (`create_backlog_item`, same evidentiary bar as `ce71ad1a`), nudge the stuck session, or post a diagnostic note (`post_backlog_update`) when inconclusive.
- **Fully autonomous nudging** (per Tyler's explicit choice this session, overriding the more cautious human-approval-gate option): the agent calls `resume_session`/`steer_session`/`write_to_session` directly once its safety checks pass, with no human approval step in this version.
- Nudge safety gates: (a) target session confirmed `detection.StatusIdle` immediately before acting — reuse the existing safe-unattended-write gating pattern (`session/command_executor.go:52`, `session/autonomous_driver.go:590`), not a new one; (b) target session's UUID/identity re-verified immediately before the write, closing the exact cross-session-misdelivery class in `ce71ad1a-a6a5-485f-8245-c5a502754a8b`.
- Nudge-loop bound: configurable max-nudges-per-item and/or cooldown, mirroring the existing `AutonomousDriver` turn-cap precedent (`config.go`) — so a genuinely broken item degrades to "file a bug" instead of looping forever.
- **Stale retry-session cleanup**, folded in per Tyler's explicit ask this session: today, retry sessions (naming pattern `*-r2` through `*-r9`, observed live in tmux during this conversation) stick around indefinitely after a failed/abandoned attempt. When this feature's handoff/compaction logic replaces a session, the flow must be: (1) the old session generates a handoff summary of its work so far, (2) that summary is handed to the new/replacement agent session, (3) the OLD session is then cleaned up (its tmux session killed / instance transitioned to a terminal state) — but the underlying work product (git commits, branch, worktree contents) must never be deleted.
- Standard structured logging for every diagnose dispatch, nudge attempt (success/failure/skipped-not-idle/skipped-identity-mismatch), bug filed, and cap-hit event — mirroring existing `log.InfoLog`/`log.WarningLog` patterns (e.g. `quota_gate.go`'s pause/resume notification pattern). No new metrics/alerting infra beyond that, consistent with the sibling plan's Observability Requirements.
- Feature registry entries per `docs/reference/feature-registry.md` (new RPC + new React component/action).

### Out of Scope
- Redesigning stuck-item detection/visibility itself (`backlog-stuck-item-visibility` already shipped that; this plan only adds an action on top of it).
- A human-approval gate before nudging (explicitly declined this session in favor of fully autonomous nudging within the stated safety gates — revisit only if the fully-autonomous version proves unsafe in practice).
- Any remediation beyond "nudge an idle session" or "file a bug": no auto-merge, no auto-retry-rework, no auto-resolve-review verdicts — those remain declined scope carried over from the original stuck-item-visibility plan.
- Building a second, bespoke context-compaction/summarization engine — this plan reuses native `/compact` and the existing `HandoffSummaryGenerator` rather than inventing a new one.
- Multi-tenant/auth changes, new metrics/alerting infrastructure, or anything not already covered by the existing single-user internal-tool posture.

## Rabbit Holes
- **Distinguishing "safe to nudge" from "actively working, just slow."** `detection.StatusIdle` plus identity-reverification prevents *misdelivery*, but not a false-positive nudge on a session that's idle because it's waiting on something legitimate (a long-running background job, a human decision already posted elsewhere). Needs explicit heuristic design in planning, not just "idle == nudge."
- **Reusing `/compact` from outside the harness's own conversation loop.** The Diagnose dispatch is a *new* agent session, not a continuation of an existing one — "use the harness's built-in /compact" may only cleanly apply to that new session's own in-flight context growth (e.g. if it does multi-step investigation), not to summarizing the *target* stuck session's history for the bundle, which is a different data flow (that's `HandoffSummaryGenerator`'s job). Planning must not conflate the two.
- **Handoff-then-cleanup ordering.** The "generate handoff summary from old session, hand to new session, then kill the old session" sequence has an obvious failure mode if the old session dies/is killed before the handoff summary is durably captured — must be sequenced so the summary is persisted (not just streamed) before any teardown happens. This directly parallels the class of bug already documented in this repo's rules (`.claude/rules/instance-lock-free-reads.md`) — read/confirm state before acting on it, not concurrently.
- **Nudge-cap tuning.** No empirical basis yet for what "too many nudges" looks like for this specific failure mode (unlike `AutonomousDriver`'s turn cap, which has incident history behind its default). Planning should pick a conservative default and treat it as adjustable, not derive it from scratch.
- **Cost of frequent Diagnose dispatches.** Each dispatch is a real LLM session with its own cost; if it fires automatically (e.g. from a reconciler loop) rather than only on manual button-click, cost could scale with how many items are flagged stuck at once. Scope note: this plan's "Diagnose" is described as a UI-triggered *button* action, not an automatic reconciler-driven trigger — planning should confirm that boundary explicitly rather than assume.

## Alternatives Considered
- **Human-approval gate before every nudge** (safer, considered and explicitly declined by Tyler this session in favor of fully autonomous nudging within the stated safety gates).
- **Build a new bespoke compaction/summarization mechanism** instead of reusing native `/compact` + `HandoffSummaryGenerator`: rejected — duplicates existing, working infrastructure for no clear benefit.
- **Leave stale retry-sessions alone** (out of scope, handle separately): rejected by Tyler this session — explicitly folded into this plan's scope since it's directly caused by the same handoff/compaction mechanism this feature is building.

## Feasibility Risks
- The `stapler-squad` MCP server was observed disconnecting mid-conversation during this very session (`ECONNREFUSED`) — any design that assumes the backlog/session MCP tools are always reachable from a dispatched diagnostic agent needs a documented failure mode (retry/backoff, or surface as its own "diagnose dispatch failed" signal) rather than assuming success.
- `HandoffSummaryGenerator`'s existing LLM-backed summarization has a hard 60s timeout (`handoffSummaryTimeout`, `session/handoff_summary_service.go`) and resolves to an ERROR row on timeout — the new handoff-then-cleanup flow must handle that ERROR path (do not kill the old session if handoff summary generation failed) rather than assume it always succeeds within budget.
- Nudge safety hinges on `detection.StatusIdle` being reliably produced — per existing project memory, `detection.StatusReady` is dead code (never produced by `MatchLines`), which is exactly the kind of detection-signal gap that could silently make "confirmed idle" checks less reliable than assumed; planning/research should verify `StatusIdle`'s actual reliability, not just its existence.

## Observability Requirements
Standard structured logging (existing `log.InfoLog`/`log.WarningLog` patterns, mirroring `quota_gate.go`'s pause/resume notification pattern) for: every Diagnose dispatch (started/completed/failed), every nudge attempt and its outcome (succeeded / skipped-not-idle / skipped-identity-mismatch / failed), every bug filed as a result, every nudge-cap/cooldown hit, and every handoff-then-cleanup cycle (summary generated, old session torn down, or handoff failed and cleanup skipped). No new metrics/alerting infrastructure required, consistent with the sibling plan.

## Risk Control
Given fully autonomous nudging is a new class of capability (a session writing to another live session with no human in the loop), risk control is *not* "not needed" the way the read-only sibling plan was:
- **Rollout**: gate the nudge-execution path (not the diagnose/investigate path, which is read-only and low-risk) behind a live-settable feature flag per this repo's existing rollout-flag convention (global + per-scope override via the feature-flag RPC/panel, never an env var) — so it can be disabled instantly without a redeploy if fully-autonomous nudging misbehaves in practice.
- **Rollback**: disabling the flag reverts to diagnose-and-report-only (file bug / post note), which is the safe fallback behavior, not an all-or-nothing revert of the whole feature.
- **Staged rollout**: no staged/canary rollout needed beyond the flag, given single-user scope — but the flag should default OFF on first deploy so Tyler explicitly opts in to autonomous nudging after reviewing a few diagnose-only runs.

## Open Questions
- Exact numeric defaults for the nudge cap/cooldown and the "idle long enough to be safe to nudge" duration threshold — deferred to planning/pre-mortem phase to pick defensible defaults, per the Rabbit Holes section above.
- Whether Diagnose should ever be reconciler-triggered automatically (vs. purely manual button-click) is explicitly deferred — this plan scopes to manual trigger only; automatic triggering is a candidate follow-on, not baseline scope.
- Whether the "fewer manual diagnoses" success metric needs any lightweight instrumentation to actually count (vs. purely Tyler's own subjective sense of it) — left to planning to decide if it's worth building vs. just asking Tyler periodically.
