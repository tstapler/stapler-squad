# Requirements: backlog-pr-mergeability-policy

**Date**: 2026-07-17
**Type**: feature addition (opt-in policy on an existing backlog automation subsystem)

## Problem Statement

Work that finishes in the backlog/review pipeline does not reach a merged PR without
repeated manual human clicks, and the operator is asked to intervene at points where the
system already has enough information to proceed on its own. Three distinct manual gates
compound into "nothing gets merged":

1. **PR creation is manual.** When a session reports `TASK_COMPLETE`, a human must click
   "Create PR" in the cross-repo Review Queue
   (`web-app/src/components/sessions/ReviewQueuePanel.tsx:858-883`), which opens a modal
   requiring the human to review/edit a prompt and click "Run"
   (`onRunOneShot` → `server/services/session_service.go:3405` `RunOneShot`). Nothing
   invokes `RunOneShot` automatically on Complete. Live state observed 2026-07-17: Review
   Queue had 18 items, 15 stale, only 2 Complete — both sitting with an unclicked
   Create-PR button, oldest stale item 10h old. This is the *primary* blocker.

2. **Fix loops are not automatic.** When a created PR's CI fails or it develops a merge
   conflict with `main`, no automated path respawns the implementing session to fix it.
   An `AutoReopenForPRFix` / `ReconcilePRPending` pattern already exists
   (`session/backlog_lifecycle.go:1590-1629`) for `pr_pending` items, but it only fires
   for items that actually reach `pr_pending` status — and a known status desync leaves
   some items stuck in `review`/`BOUNCING` so the reconciler never runs against them
   (PR #157's item is the live example).

3. **Notification fires on the wrong signal.** The operator is (or should be) told
   "ready to merge" — but the meaningful signal is "CI green AND no merge conflict AND
   about to auto-merge", not merely "a PR exists". GitHub `allow_auto_merge` was enabled
   on the repo this session and `session/backlog_lifecycle.go:1468-1484` already calls
   `EnablePRAutoMerge` after PR creation, so once an item is genuinely mergeable the merge
   now happens without further action — the notification is a *status surface*, not a
   merge trigger.

The net effect: 15 stale Review Queue items, 6 backlog items stuck via
`ListStuckBacklogItems` (3 abandoned-review, 2 bouncing, 2 rework-cap, 1 stale-work), and
the one item that reached a PR (#157) sitting dead with green CI but a `DIRTY`/`CONFLICTING`
merge state and no session left to rebase it.

This is a **trust-boundary change**: enabling it means LLM-authored PRs get created and
(via existing auto-merge) merged without a human reading the generating prompt first.

## Users / Consumers

- **Primary: the single operator** running Stapler Squad's backlog "software factory",
  who wants finished work to reach a merged PR without babysitting each click, while
  retaining the ability to opt individual items OUT and to globally halt automation.
- **The backlog automation subsystem itself** — the reconciliation loop
  (`ReconcilePRPending`, `AutoReopenForPRFix`, the stuck-item detectors) is the machine
  consumer that reads the new policy flag and acts on it.
- **Downstream: GitHub** — receives auto-created PRs and, via `allow_auto_merge`, merges
  them when mergeable.

## Success Metrics

- With the policy enabled on an item, a session reaching `TASK_COMPLETE` results in a PR
  being created automatically (no human click), observable end-to-end.
- A created PR whose CI fails or that develops a merge conflict is automatically driven
  back through a fix session and re-attempted, looping without surfacing to the operator
  during the loop, until CI is green and there is no conflict.
- The operator receives exactly one "ready to merge" notification per item, fired only
  when CI is green AND no merge conflict exists (not on mere PR existence).
- The steady-state count of Review-Queue-stale and `ListStuckBacklogItems` items
  attributable to unclicked Create-PR / un-rebased conflicts trends to zero for
  policy-enabled items.
- No policy-enabled item merges without the fix-loop escalation / rework-cap controls
  having a defined terminal state (escalate to operator; never loop unbounded).

## Constraints

- **Follow the established opt-in toggle pattern exactly**: a new `bool` field on
  `BacklogItemData` (`session/repository.go:342`), default `false`, wired through
  ent schema → proto → repository Create/Update mapping → RPC handler, and exposed in
  `BacklogItemForm.tsx` — mirroring `SkipReviewGate` / `SkipPlanning` / `AutoSpawnSession`.
- **ent regeneration must use** `go run -mod=mod entgo.io/ent/cmd/ent generate
  --feature sql/upsert ./session/ent/schema` (per `.claude/rules/ent-schema-generation.md`).
  Proto changes require `make proto-gen`.
- **proto3 `bool` has no "unset" wire representation** — every partial `updateBacklogItem`
  call site in `BacklogItemDetail.tsx` must include the new flag (via the existing
  `currentFlags()` helper), or saves will silently reset it to `false` (this exact bug bit
  `AutoSpawnSession`; see task doc bucket [2], commit `b28ace2f`).
- **Trust boundary**: the ADR (Phase 3) must explicitly address risk controls —
  behavior on repeated fix-loop failure and its interaction with
  `maxAutoReworkIterations = 3` (`backlog_service_triage.go:72-97`), operator escalation,
  and whether a **global kill-switch** is required in addition to the per-item opt-in.
- Auto-merge with **no branch protection on `main`** means any policy-enabled PR whose CI
  goes green merges with zero required human review — the design must treat this as the
  actual blast radius.
- Feature registry (`docs/registry/features/`) and an e2e Playwright test are required per
  project rules for any new user-facing feature.
- Must not regress the existing manual paths (Review Queue Create-PR button,
  `SubmitManualReview`, per-item override escape hatches) — the policy is additive.

## Scope

### In Scope

- A per-item opt-in policy flag (working name TBD in planning, e.g.
  `AutoMergePolicy` / `AutoPRPolicy`) on `BacklogItemData`, fully wired
  through ent/proto/repository/RPC/form, default `false`.
- **Behavior 1** — auto-create PR when a policy-enabled session reports Complete, replacing
  the manual `RunOneShot` click for those items (reusing the existing PR-creation machinery,
  not a parallel one).
- **Behavior 2** — auto-fix loop on CI failure or merge conflict for policy-enabled items,
  built on the existing `AutoReopenForPRFix` / `ReconcilePRPending` reconciler, looping
  silently until mergeable or until the rework cap / escalation terminal state is hit.
- **Behavior 3** — a single "ready to merge" notification fired on genuine mergeability
  (CI green AND no conflict), as a status surface, not a merge trigger.
- Risk controls: fix-loop failure escalation, interaction with `maxAutoReworkIterations`,
  and a decision (captured in an ADR) on a global kill-switch.
- An ADR documenting the trust-boundary decision and its controls.
- Tests (Go unit + integration, Jest form behavior, e2e) and feature-registry entries.

### Out of Scope

- The broader "configurable pipeline / PipelineMode per item" wiring (Epic 1.4 in the task
  doc) — the `PipelineMode` field is explicitly a separate initiative and is NOT part of
  this policy.
- The MCP session-controller-wiring gap (MCP-created sessions not drivable until a UI
  client opens them) — a deeper orchestration-layer investigation, noted as a risk/blocker
  but not fixed here.
- Changing the *repo-level* `allow_auto_merge` GitHub setting or adding branch protection —
  those are operator/settings actions, not code in this feature (though the design depends
  on `allow_auto_merge` being on).
- The Epic 5 instance-concurrency initiative (unguarded `Instance` field mutation).
- Retroactively unsticking the currently-stuck 6 items / merging PR #157 by hand — those
  are separate `sdd:quick` maintenance tasks, not this feature (though this feature should
  prevent the *class* going forward).

## Open Questions (for research + planning to resolve)

1. **The `review`/`BOUNCING`-vs-`pr_pending` status desync** (Behavior 2's dependency):
   *why* do some items — e.g. PR #157's — never reach `pr_pending`, so `ReconcilePRPending`
   never runs? Research must trace every write path that sets status to `pr_pending` in
   `backlog_service_lifecycle.go` and identify which transition PR #157's item skipped.
   **Decision needed in planning**: is this a Phase-0 prerequisite bug fix (in scope) or a
   blocking dependency to flag (out of scope)? Behavior 2 is unreliable until it's resolved.
2. Is the correct integration point for Behavior 1 the backlog-automated PR-creation path
   (`session/backlog_lifecycle.go:1468-1484`, which already does push→PR→`EnablePRAutoMerge`),
   the Review-Queue `RunOneShot` path, or a convergence of the two? The two currently
   diverge (backlog path already auto-enables merge; Review Queue path is fully manual).
3. Global kill-switch: config field, RPC, both? Where does it gate — at reconciler entry,
   at PR-creation, at merge-enable? What is its default?
4. Fix-loop terminal behavior: on hitting `maxAutoReworkIterations`, does the item escalate
   to the operator as a stuck-item (existing `notifyReworkCapHit` / `MarkStuck` path), and
   does the "ready to merge" notification ever compete with an escalation notification?
5. What exactly constitutes "no merge conflict" and "CI green" as machine-checkable signals,
   and which existing calls (`mergeStateStatus`, `mergeable`, CI check status via the
   github plugin) already surface them?
   **Resolved (planning, ADR-024 §c / Blocker-1):** "no merge conflict" = the existing
   belt-and-suspenders `PRStatus.HasConflicts` (`mergeStateStatus == "DIRTY"` OR
   `mergeable == "CONFLICTING"`, cli/cli#9583 guard). **"CI green" is NOT `!CIFailing`** — that
   coarse terminal-failure bool treats pending / not-yet-created checks (`CheckConclusion==""`) as
   green, which would let both the ready-notify and the auto-merge arm fire before CI is actually
   green. "CI green" must be a **positive tri-state**: checks present, all concluded, none failing
   and none pending (`ciPassing := !CIFailing && !CIPending`). The plan adds `PRStatus.CIPending`
   and maps the tri-state into `github.PRInfo.CheckConclusion` (`failure`/`pending`/`""`) so
   `prReadyToMergeSolo` blocks on pending. Residual: a truly empty check rollup ("no CI" vs "no
   checks yet") cannot be disambiguated without required-check config (out of scope) — accepted,
   see plan Accepted Risks.
