# Requirements: backlog-stuck-item-visibility

**Date**: 2026-07-14
**Type**: feature addition (cross-cutting: DB schema, backend reconciliation, frontend UI)
**Complexity**: 4 — cross-cutting change with Large appetite

## Problem Statement
Backlog items that stop making forward progress toward merge are invisible to the user except as ephemeral, easy-to-miss toast notifications. Live investigation of the running instance found 6 items parked in `review` status and one fully green, mergeable PR (#148) sitting unmerged for 3 days with zero signal to the user. There is no durable, queryable answer to "which backlog items have unmerged work right now, and why are they stuck?"

Four confirmed root causes, all currently invisible in the same way:
1. **PR-ready-to-merge has no notification path.** `pushAndCreatePR` (`session/backlog_lifecycle.go`) calls `EnablePRAutoMerge` assuming GitHub auto-merge will finish the job, but the repo has `allow_auto_merge: false` — the call fails silently (logged as a warning only) and no other code path tells the user a PR is ready. `ReconcilePRPending`'s 60s poll only acts on CI-failing/blocked/conflicted PRs; a healthy green PR just polls forever with no signal.
2. **Rework-cap parking is silent.** `maxAutoReworkIterations = 3` (`server/services/backlog_service_triage.go`) stops auto-respawning work after 3 failed review cycles and fires one ephemeral WARNING notification. Confirmed live on item `96cc9eaa` (3 work sessions, sitting in `review`).
3. **Stuck-item bookkeeping doesn't survive restarts.** `reconcileStuckReviewItems` / `reconcileStaleWorkSessions` track "already notified" in in-memory maps (`stuckReviewNotified`, `staleWorkNotified`) that reset on every service restart. This dev instance restarts multiple times per day (15+ times observed on a single day in the journal), so any accumulated "this has been stuck since X" context is lost constantly.
4. **Some items cycle without converging**, independent of the cap — e.g. item `df0d5872` has bounced `in_progress ↔ review` across 6 triage + 3 review + 2 work sessions over 4 days with no persistent record of the pattern.

## Baseline
Today, the only signals for any of the above are: (a) a toast-style notification event that is easy to miss and isn't persisted anywhere queryable, and (b) manually reading server logs or querying the sqlite backlog tables directly (as done for this investigation). The existing `/unfinished` page shows dirty/unmerged git worktree state but has no concept of backlog item status, stuck reason, or PR mergeability — it cannot answer "why is this stuck."

## Users / Consumers
The single user of this self-hosted instance (Tyler), via the stapler-squad web UI. No other consumers.

## Success Metrics
Zero silently-stuck backlog items: every item that has been stuck for more than a defined threshold, for any of the 4 confirmed reasons, is visible in a persistent, queryable UI view — not dependent on catching a toast notification or reading logs. Verification: re-run the live-data queries used in this investigation (status counts, session-role counts per item, PR mergeability check) against the shipped feature's data source and confirm the same 6+ stuck items surface with their correct reason. This metric is verified **both** once at ship time (the live-data re-run described above, gated in `validation.md`) **and** is intended as an ongoing invariant — any *future* backlog item that becomes stuck for one of these reasons, not just the original 6+1 observed cases, should surface the same way; the ship-time check is the initial proof, not a one-time box to tick.

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

**Fallback increment**: the implementation plan's phase structure already supports a smaller shippable slice if the appetite tightens — Phases 1–3 (durable stuck-state storage + detection + the read RPC/notification path) are independently valuable and shippable on their own, delivering durable, restart-surviving detection and notification even before Phase 4's browsable UI or Phase 5's snooze control land. No restructuring is needed to cut down to that slice.

## Constraints
- Single-developer, self-hosted instance — no multi-tenant or auth considerations beyond what already exists.
- Must work correctly across the frequent service restarts this dev environment already exhibits (many per day) — durable state is a hard requirement, not a nice-to-have.
- Existing `docs/registry/` feature-registry rule applies: any new RPC/UI component needs a registry entry (see `.claude/rules/feature-registry.md`).
- Existing session-creation-registry and omnibar-registry rules do not apply (this is not a new session-creation mode or omnibar action).

## Non-functional Requirements
- **Performance SLO**: not specified — this is a low-traffic, single-user internal tool; existing 60s reconcile ticker cadence is an acceptable baseline for freshness.
- **Scalability**: not applicable (single user, backlog size in the tens of items).
- **Security classification**: internal.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Durable (DB-backed, not in-memory) tracking of stuck state per backlog item, replacing `stuckReviewNotified` / `staleWorkNotified` and equivalent cap-hit tracking, so state survives service restarts.
- Detection and surfacing of all 4 confirmed stuck-reason classes: rework-cap hit, non-converging review/rework cycling, PR-pending-but-healthy-with-no-merge-signal, and push/PR-creation failure.
- A new notification/surfacing path for "PR is green and mergeable but not merged" (fixes root cause #1's missing signal) — this is a new signal, not a change to auto-merge/GitHub settings.
- A persistent, browsable UI view of stuck items and their reason (exact location — extend `/unfinished` vs. a new view — is a planning-phase decision).
- Snoozing a stuck item — a **visibility control only** (temporarily suppress a known/intentionally-parked item from the active view and re-notification until a chosen time), NOT a remediation action: it does not retry, fix, re-review, push, or merge anything. Surfaced via a `SnoozeStuckItem` RPC and per-item snooze control.
- Feature registry entries per `.claude/rules/feature-registry.md`.

### Out of Scope
- Changing the GitHub repo's `allow_auto_merge` setting or any other repo/branch-protection configuration. That's a manual, external decision explicitly deferred by the user — this feature surfaces the fact that a PR is ready, it does not change how merging happens.
- Changing the value or policy of `maxAutoReworkIterations` (the rework cap).
- One-click remediation actions from the new view (retry rework, re-trigger review, retry push, merge now). Explicitly declined in favor of "surface + fix confirmed bugs" only — this is visibility, not a control panel.
- Any change to review-verdict logic, the diff auto-repair mechanism from `c99f6595`, or the underlying reasons individual reviews FAIL.

## Rabbit Holes
- Distinguishing "genuinely stuck" from "actively cycling but making progress" for root cause #4 (non-converging review loops) is fuzzy — a naive time-since-last-transition threshold risks false positives on legitimately slow-but-working items. Needs explicit heuristic design in planning (e.g. N consecutive FAIL verdicts with no diff change, not just elapsed time).
- Migrating existing in-memory notify-once state to a DB-backed model touches the same reconciliation code path the `c99f6595` fix just changed — needs care to not regress the auto-repair/notify-once behavior that was just shipped.
- Deciding the UI location (extend `/unfinished` vs. new view) has real information-architecture implications (that page's data model is git/worktree-centric, not backlog-item-centric) — don't let this stall implementation; planning should timebox the decision.

## Alternatives Considered
- Rely on existing ephemeral notifications and just make them louder/higher-priority: rejected — doesn't solve the restart-durability problem (root cause #3), and doesn't help review items that were never in flight when the observer wasn't watching.
- Fix root causes without adding new UI (e.g., just enable GitHub auto-merge, raise the cap): rejected by explicit user choice — auto-merge/cap changes are out of scope, and the user wants durable visibility regardless of whether those policies also change later.

## Feasibility Risks
- `EnablePRAutoMerge`'s current failure mode is silent (logged warning only) — the new "PR ready to merge" signal needs to positively confirm PR state (mergeable, CI green, not already merged) via polling rather than relying on the auto-merge attempt's success/failure, since auto-merge will keep failing regardless (out of scope to fix that setting).
- The existing reconcile ticker (60s, `server/dependencies.go`) is the only scheduled entry point for backlog reconciliation; new stuck-detection logic should hook into it rather than adding a second competing ticker.

## Observability Requirements
Standard structured logging (existing `log.InfoLog`/`log.WarningLog` patterns) for all new state transitions (item marked stuck, reason recorded, item un-stuck). No new metrics/alerting infrastructure required beyond the in-app UI view itself, since this is a single-user internal tool without an oncall rotation.

## Risk Control
Not needed — low risk. This is additive observability/UI work with no changes to existing merge, review, or auto-repair behavior. Rollback is a simple revert if the new reconciliation path misbehaves; the existing `c99f6595` safety nets remain in place as a fallback.

## Open Questions
- Should the "PR ready to merge" detection also account for PRs where `allow_auto_merge` might later be turned on (i.e., should the feature poll for and report on the *repo's* auto-merge setting so it's visible in the UI, even though changing it is out of scope)? Recommend surfacing it as read-only context.
- Exact time thresholds for "stuck" (e.g., review cycling with no forward progress for how long before flagging?) — leave to planning/pre-mortem phase to pick defensible defaults.
- UI location (extend `/unfinished` vs. new dedicated view) — explicitly deferred to planning phase per user answer.
