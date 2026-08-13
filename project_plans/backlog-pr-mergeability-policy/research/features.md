# Research: Feature Landscape — backlog-pr-mergeability-policy

**Agent**: Research Agent 2 (FEATURES)
**Date**: 2026-07-17
**Scope**: the three behaviors this policy adds, the manual paths they replace, the two
divergent PR-creation paths, edge cases, and unstated operator needs.

All line references are absolute-file relative to repo root.

---

## 0. The Two PR-Creation Paths (the central architectural fact)

There are **two entirely separate ways an item's work reaches a GitHub PR today**, and they
do NOT converge. Understanding this is the key to Behavior 1 and to the #157 desync.

### Path A — backlog-automated (`pushAndCreatePR`)
`session/backlog_lifecycle.go:1358-1503`. Triggered when a **review PASS verdict** lands
(`handleReviewSessionExited`, `session/backlog_lifecycle.go:562-570`, case `ReviewVerdictPass`
→ `l.pushAndCreatePR(ctx, item, *workEntry)`), or when `SkipReviewGate` short-circuits review
(work-session-exit path sets `toStatus = BacklogStatusDone` at `:445-447`, so `SkipReviewGate`
never enters review OR pushAndCreatePR — it goes straight to `done` with no PR at all — see
"edge cases").

What Path A does, in order (`:1416-1502`):
1. `GetWorktreeDataBySessionUUID` → fall back to `done` if no worktree (`:1410-1414`).
2. `CommitChanges` (best-effort) → `PushBranch` (fatal → `stayInReviewAndNotify` + durable
   `push_failed` stuck row).
3. `CreatePR(title, body)` — body drafted by headless LLM (`DraftPRDescription`), else
   boilerplate. Reuses an existing PR if `item.PrNumber>0 && item.PrURL!=""` (`:1433-1437`).
4. **Caches `PrURL`+`PrNumber` on the BacklogItem** via `UpdateBacklogItem` (`:1460-1465`).
5. **`EnablePRAutoMerge(prNumber)`** (`:1475`) — best-effort; on failure emits an
   "Auto-merge not enabled" WARNING notification (`:1477-1482`).
6. **`TransitionBacklogItemStatus(... BacklogStatusPRPending ...)`** with precondition
   `ExpectedStatus: review` (`:1487-1492`). **This is the ONLY write path that sets an item
   to `pr_pending`** (confirmed: grep for `BacklogStatusPRPending` transitions — only here and
   the reconciler's rollback in `AutoReopenForPRFix`).
7. Resolves open `push_failed`/`abandoned_review` stuck rows (`:1499-1502`).

The `prCreator` interface (`session/backlog_lifecycle.go:63-68`) is the consumer-scoped seam:
`CommitChanges / PushBranch / CreatePR / EnablePRAutoMerge`. Behavior 1 should reuse this
machinery, not a parallel one (requirements Open Q #2).

### Path B — Review-Queue manual (`RunOneShot`)
`server/services/session_service.go:3405-3495`. Triggered by the human clicking
**🔀 Create PR** in `ReviewQueuePanel.tsx:858-883`, which opens a modal
(`ReviewQueuePanel.tsx:1240-1337`) pre-filled with `DEFAULT_PR_PROMPT`; the operator edits the
prompt and clicks **Run** → `onRunOneShot(sessionId, prompt)` (`:1312-1328`).

What Path B does (`session_service.go:3416-3494`):
1. Finds the `*Instance` by `sessionId` (not by backlog item).
2. Runs `claude -p <prompt>` in the session's worktree via `headlessPool.CallBlocking`
   (feature key `Custom`), fallback direct `claude` subprocess (`:3442-3469`). Timeout default
   **900 s**, max 1800 s (`:3427-3433`).
3. `extractPRURL(output)` scans last 10 lines for a `github.com/…/pull/NNN` URL
   (`:3497-3516`); `checkBranchDivergence(workDir)` (`:3471`).
4. **Persists the PR URL to the SESSION INSTANCE only** (`inst.SetGitHubPR` + `SaveInstances`,
   `:3475-3486`) — emits `session_updated` event. **It does NOT touch the BacklogItem: no
   `PrURL`/`PrNumber` on the item, and NO status transition to `pr_pending`.**

### The divergence — and the #157 desync mechanism (Open Q #1)
Path B (`RunOneShot`) creates a real PR but **leaves the backlog item wherever it was
(`review` / `BOUNCING`)** — it writes the PR only to the session record. Because
`ReconcilePRPending` iterates **only `FindPRPendingItems`** (`session/backlog_lifecycle.go:1509`,
filtered by status `pr_pending` AND `PrNumberGT(0)`), an item that got its PR via Path B is
**structurally invisible to the reconciler forever** — CI failures, conflicts, and merges on
that PR are never observed. This is exactly PR #157's state: green CI, `DIRTY`/`CONFLICTING`
merge state, no session, item never in `pr_pending`.

So the #157 desync is not (only) a bug in a status-write path — it is the **direct consequence
of the operator using Path B instead of Path A**. Any item whose PR was created by the manual
Review-Queue button will never reach `pr_pending`. Behavior 1 (auto-create on Complete via
Path A) inherently closes this gap for policy-enabled items because Path A *is* the write path
that sets `pr_pending`. **Recommendation for planning**: Behavior 1 should route through Path A
(`pushAndCreatePR`), NOT reuse `RunOneShot`. If the design instead extended `RunOneShot`, it
would reproduce the desync. (Planning still owes a decision on whether to *also* backfill/repair
Path-B-created PRs onto their items — requirements marks the one-off #157 repair out of scope.)

### Which sessions get which button
- The Review-Queue **Create PR** button shows for `queueItem.reason === TASK_COMPLETE &&
  !queueItem.githubPrUrl && onRunOneShot` (`ReviewQueuePanel.tsx:859-861`). It is keyed on
  `sessionId`, NOT on a backlog item — so it serves **both** backlog-linked TASK_COMPLETE
  sessions AND standalone (non-backlog) sessions that reported complete. A `⚠ Diverged from
  main` badge renders when `branchDivergedFromBase` (`:863-867`).
- Backlog items with review enabled never *need* this button — Path A fires automatically on
  PASS. The button is the fallback the operator reaches for when Path A didn't run (review
  never passed, or the session isn't backlog-linked). Live state (requirements:18-20) shows the
  operator using it as the *primary* path — which is precisely why nothing reaches `pr_pending`.

---

## 1. Behavior 1 — Auto-create PR on Complete

**Manual path replaced**: the Review-Queue Create-PR button + modal (Path B above). Nothing
invokes `RunOneShot` automatically; a human must click, review the prompt, and click Run.

**Recommended integration point**: Path A (`pushAndCreatePR`), gated on the new per-item policy
flag. The trigger for a policy-enabled item reaching `TASK_COMPLETE` should converge to the
same call `handleReviewSessionExited`'s PASS case already makes.

**Design tension — what "Complete" means for a policy item**:
- Today `TASK_COMPLETE` (a session-attention reason, `AttentionReason.TASK_COMPLETE`,
  `ReviewQueuePanel.tsx:532`) and a backlog item reaching `review` status are related but not
  identical events. A backlog work session exiting transitions the item `in_progress → review`
  (`backlog_lifecycle.go:444-457`) and then spawns the review gate.
- For a policy-enabled item, planning must decide: does auto-PR fire **at work-session
  completion** (bypassing the review gate, like `SkipReviewGate`), or **only after a PASS
  verdict** (keeping review, just removing the human Create-PR click)? These are materially
  different trust postures. The requirements frame this as "session reaching TASK_COMPLETE
  results in a PR" (metrics:59) — but if review is retained, the honest trigger is "PASS
  verdict," which Path A already automates. If the policy is meant to *also* skip review, it
  overlaps `SkipReviewGate` and needs de-confliction (a `SkipReviewGate` item never enters
  review/pushAndCreatePR at all — it goes straight to `done`, see §4).

**Reuse, don't reinvent**: the `prCreator` interface + headless PR-description drafting already
exist. Behavior 1 is mostly a *policy gate* on an existing code path, not new machinery.

---

## 2. Behavior 2 — Auto-fix loop on CI failure / merge conflict

**Existing machinery** (all already built — Behavior 2 is largely "make sure items reach it"):

`ReconcilePRPending` (`session/backlog_lifecycle.go:1508-1630`), run every stuck-sweep tick
(wired at `:912-914`), for each `pr_pending` item with `PrNumber>0`:
1. `IsPRMerged` → merged ⇒ `pr_pending → done`, resolve `pr_ready_unmerged` (`:1524-1542`).
2. `GetPRStatus(prNumber)` → `*git.PRStatus` (`session/git/worktree_git.go:330-355`), fields:
   `CIFailing`, `HasBlockingReviews`, `HasConflicts`, `IsClosed`, `IsDraft`, `Mergeable`,
   `FeedbackText`, `ChangesRequestedCount`, `ApprovedCount`.
3. **Closed-without-merge** (`IsClosed`, `:1558-1582`): clear cached PR fields, spawn fix via
   `AutoReopenForPRFix` with a "closed without merging" context.
4. **Healthy** (`!CIFailing && !HasBlockingReviews && !HasConflicts`, `:1584-1610`): wait for
   merge; flag/resolve `pr_ready_unmerged` (Behavior 3).
5. **Unhealthy** (CI failing OR blocking review OR conflict, `:1618-1628`): spawn fix session
   via `AutoReopenForPRFix(ctx, itemID, fixCtx)` where `fixCtx` embeds `prStatus.FeedbackText`.

`AutoReopenForPRFix` (`server/services/backlog_service_triage.go:565-670`) — the fix respawn:
- Guards item is `pr_pending` (`:577-579`).
- **Churn avoidance (the hard-won fix, task doc bucket [1] #10)**:
  `tombstoneOrphanWorkSessions` (`:597`, defined `:1283+`) ends confirmed-dead work sessions
  via `IsSessionLive`; then `hasActiveWorkSession(sessions)` (`:598`, `:435-437`) — if a fix is
  **already in flight, return early with ZERO status transition** (`:598-601`). This is what
  stopped the every-60s `pr_pending↔in_progress` churn that grew `BacklogStatusEvent`
  unboundedly (commits `af426f27` + `f8f788ab`).
- **Rework cap**: `workCount >= maxAutoReworkIterations` (3, `backlog_service_triage.go:72`) ⇒
  leave in `pr_pending`, `notifyReworkCapHit`, no spawn (`:609-613`). **Terminal state.**
- Transition `pr_pending → in_progress` (precondition-guarded, `:615-623`); resolve
  `rework_cap`/`pr_ready_unmerged`/`push_failed` rows (`:628-632`).
- Prepend PR-failure context to item notes so the spawned prompt includes it, spawn autonomous
  session, restore notes (`:634-658`). On spawn failure roll back to `pr_pending` (`:660-666`).

`AutoReopenAfterFailedReview` (`server/services/backlog_service_triage.go:494-563`) — the
sibling loop for **review** failures (FAIL/PARTIAL/UNVERIFIABLE verdict, or review session
exited with no verdict; `backlog_lifecycle.go:521-561`): same `maxAutoReworkIterations` cap +
`notifyReworkCapHit`, transitions `review → in_progress`, spawns autonomous rework session,
rolls back on failure. Note: this one does **not** call `tombstoneOrphanWorkSessions`/
`hasActiveWorkSession` — the churn guard was only added to the PR-fix path.

**Key gap Behavior 2 must close**: the entire loop is gated on the item being `pr_pending`. Per
§0, Path-B PRs never reach it. Behavior 2's reliability is therefore *entirely dependent on
Behavior 1 routing through Path A*. This is the requirements' Open Q #1 ("is the desync a
Phase-0 prerequisite?") — the finding is that Behavior 1 done correctly (Path A) largely
*subsumes* the desync fix for new policy items, but pre-existing Path-B items remain orphaned.

---

## 3. Behavior 3 — Notify only when truly ready

**Existing signal**: `StuckReasonPRReadyUnmerged` (`session/domain/backlog.go`), set by
`markPRReadyUnmerged` (`session/backlog_lifecycle.go:1637-1666`), called from the *healthy*
branch of `ReconcilePRPending` (`:1604-1605`) **only when `prReadyToMergeSolo(info)` is true**.

`prReadyToMergeSolo` (`session/stuck_decisions.go:65-95`) is the machine-checkable "genuinely
mergeable" predicate (requirements Open Q #5): not merged/closed, not draft,
`ChangesRequestedCount==0`, `CheckConclusion ∈ {success, ""}`, AND
`Mergeable == "MERGEABLE"`. Deliberately **drops the `ApprovedCount>0` gate** that
`github.DerivePRPriority` requires — a solo operator cannot self-approve, so that gate is a
permanent false-negative (pre-mortem F1, `stuck_decisions.go:65-73`).

**When the notification fires** (`markPRReadyUnmerged:1637-1666`):
- `MarkStuck(PRReadyUnmerged, pr_pending, "PR is green, mergeable, and unmerged")`.
- Fires the "**PR ready to merge**" notification ONLY if the row's `NotifiedAt==nil` (DB-backed
  notify-once dedup, survives restart) AND `stuckPRReady(FirstDetectedAt, now)` — i.e. it has
  been green+mergeable for **> `prReadyThreshold` = 30 min** (`stuck_decisions.go:17-42`). Then
  `MarkStuckNotified`.
- The **resolve** side: `resolveStuckLogged(... PRReadyUnmerged ...)` clears the row whenever
  the PR stops being solo-ready (`:1607`, `:1616` "unhealthy" poll-shaped resolve, `:1539`
  on merge, `:1571` on close). This is what makes "exactly one notification per genuine-ready
  episode" work — the row is cleared and can re-arm.

**How this satisfies "not on mere PR existence"**: the WARNING fires on the durable stuck row
crossing the 30-min-solo-ready threshold — never on PR creation. The existing behavior already
matches requirements metric (metrics:64) closely; Behavior 3 is mostly *confirming/tuning* this,
not building new. Open design question for planning: the 30-min threshold means "ready to merge"
lags true readiness by ≤30 min + one sweep tick; with `allow_auto_merge` on, the PR may merge
*before* the notification ever fires (then it resolves as `done`), so for policy items the
notification is genuinely a *status surface for the stall case*, exactly as requirements:34-36
frames it.

**Notification plumbing**: `l.notify(itemID, title, body, type, priority)` — WARNING(8)/
MEDIUM(2) for "PR ready to merge"; ERROR(7)/HIGH(3) for failures. `notifyReworkCapHit` is the
escalation channel for the terminal cap-hit case.

---

## 4. Edge Cases & Failure Modes the Design Must Handle

| # | Scenario | Current behavior / gap |
|---|---|---|
| E1 | **PR created but session ends before merge** | Path A caches PR on the item and sets `pr_pending`; `ReconcilePRPending` polls it independent of any session. This is the *designed* steady state and works — *if* the item reached `pr_pending`. The failure is the Path-B variant (§0) where it never does. |
| E2 | **CI pending — not yet green, not yet failed** | `PRStatus.CIFailing` is only true on a *terminal* failure (`worktree_git.go:331-332`). A pending/running check is neither `CIFailing` nor green. In the *healthy* branch check (`:1584`), pending CI with `Mergeable=="MERGEABLE"` would pass `!CIFailing && !HasConflicts` but `prReadyToMergeSolo` requires `CheckConclusion ∈ {success,""}` — a pending conclusion (not "success") returns false, so it neither flags ready NOR spawns a fix. **Correct**: it simply waits. Planning must confirm pending-CI maps to "keep waiting," never to a spurious fix spawn or a premature ready notification. |
| E3 | **Conflict appears AFTER green CI** | `HasConflicts` derives from `mergeStateStatus=="DIRTY"` OR `mergeable=="CONFLICTING"` (`worktree_git.go:335-339`), belt-and-suspenders because gh's `mergeable` is observed stale (cli/cli#9583). A conflict after green CI flips the item from the healthy branch to the unhealthy branch on the next tick → `pr_ready_unmerged` resolved (`:1616`) → `AutoReopenForPRFix` spawns a rebase session. Handled. |
| E4 | **Item merged externally (human clicks merge, or auto-merge fires)** | `IsPRMerged` → `pr_pending → done` (`:1524-1542`), resolves `pr_ready_unmerged`. Handled. With `allow_auto_merge` on, this is the happy path and may pre-empt the Behavior-3 notification (see §3). |
| E5 | **Concurrent reconciler ticks / double-spawn** | The stuck sweep is single-goroutine per tick, but `AutoReopenForPRFix` is also reachable from the closed-PR branch AND the unhealthy branch in the same pass. The `hasActiveWorkSession` early-return (`backlog_service_triage.go:598-601`) + precondition-guarded transitions (`ExpectedStatus`+`ExpectedUpdatedAt`, `:615-623`) are the guards. `AutoReopenAfterFailedReview` lacks the `hasActiveWorkSession` guard — a risk if a policy item can be in both review-rework and PR-fix loops. Planning should verify the two loops are mutually exclusive by status (`review` vs `pr_pending`) — they are, today. |
| E6 | **Item never reaching `pr_pending` (#157)** | §0 — the central failure. Path-B PRs, and any status-write path that skips `TransitionBacklogItemStatus` (task doc bucket [1] #9: `ReconcileStuckItems`/`ArchiveBacklogItem` mutate status directly and skip `BacklogStatusEvent`). Behavior 2 is unreliable until every policy item's PR flows through Path A. |
| E7 | **`SkipReviewGate` interaction** | A `SkipReviewGate` item transitions `in_progress → done` directly (`backlog_lifecycle.go:445-447`) — **it never enters review AND never calls `pushAndCreatePR`, so it gets NO PR at all**. If the new policy is meant to auto-PR, it *conflicts* with `SkipReviewGate` (done-without-PR). Planning must define precedence when both flags are set. |
| E8 | **Rework cap terminal state** (requirements Open Q #4) | `workCount >= 3` ⇒ `notifyReworkCapHit`, leave in `pr_pending`/`review`, no further spawn (`backlog_service_triage.go:521-524`, `609-613`). This IS the "never loop unbounded" guarantee (metrics:69). But note: the cap counts *all* work sessions ever (`workCount` over `SessionRoleWork`), so a review-rework loop and a PR-fix loop share one budget of 3 — an item that took 2 rework iterations then hits a CI failure gets only 1 fix attempt. Planning should confirm this shared budget is intended, or the policy needs its own counter. |
| E9 | **Escalation vs "ready" notification competing** (Open Q #4) | They are mutually exclusive by construction: `pr_ready_unmerged` only fires from the *healthy* branch; `notifyReworkCapHit` only from the unhealthy/cap path; the unhealthy branch *resolves* `pr_ready_unmerged` first (`:1616`). No competition today, but a new global kill-switch that halts the reconciler mid-loop could strand an item with a stale `pr_ready_unmerged` row — planning must define kill-switch resolve semantics. |
| E10 | **Auto-merge enable fails** (no branch protection / setting off) | Best-effort; WARNING notification (`backlog_lifecycle.go:1475-1482`). The PR then sits green+mergeable and *nothing initiates the merge* → Behavior 3's "PR ready to merge" notification is the operator's cue to merge manually. This is the intended fallback and must be preserved. |
| E11 | **PR number missing despite PR URL** | Self-healed by `BackfillMissingPRNumbers` each sweep (`backlog_lifecycle.go:860-867`) — otherwise `PrNumberGT(0)` filter hides the item from the reconciler. A Path-B-created PR that stored only a URL on the session (not the item) is NOT covered by this backfill (it backfills item rows, not session rows). |

---

## 5. Unstated Operator Needs (beyond the 3 explicit behaviors)

1. **Visibility into WHY a policy item is looping.** The fix context is written to item notes
   then *restored/erased* after spawn (`backlog_service_triage.go:634-658`) — so the operator
   cannot see, after the fact, what CI failure triggered iteration N. There is no per-item
   audit of "attempt 1 failed on lint, attempt 2 failed on conflict." The `BacklogStatusEvent`
   rows record status flips but not the failure reason. A policy that loops silently (metrics:62)
   **needs a durable, per-attempt failure log** so a rework-cap escalation is diagnosable — else
   the operator gets a "hit the cap" notification with no way to know why 3 attempts failed.

2. **Ability to pause / opt one item out mid-loop.** The per-item opt-in flag turns policy ON;
   there is no described way to turn it OFF *for an item already in a fix loop* without racing
   the reconciler. The existing manual escape hatches ("Return to Triage"/"Back to Ready",
   `BacklogItemDetail.tsx:997-1019`) are deliberately left manual (task doc bucket [2]) but they
   don't stop the PR-fix reconciler, which keys on `pr_pending` status, not on the flag. Planning
   should define: does clearing the flag mid-loop stop the next `AutoReopenForPRFix`? (It must,
   or the opt-out is a lie.) This is the requirements' global kill-switch question (Open Q #3) at
   per-item granularity.

3. **Global kill-switch** (Open Q #3, requirements:86). Where it gates matters: at reconciler
   entry (stops all fix spawns but lets in-flight sessions finish), at PR-creation (stops new
   PRs but lets existing ones merge), or at merge-enable (stops `EnablePRAutoMerge`). The
   `SetEnabled(false)` on the listener (`backlog_lifecycle.go:167`) already exists as a coarse
   "process no events" switch but it also disables review-gate spawning and status transitions —
   too broad for a policy-only halt.

4. **Trust-boundary audit trail.** With `allow_auto_merge` on and no branch protection
   (requirements:89-91), a policy PR merges with zero human review. The operator needs an
   after-the-fact record of *which* items merged autonomously, when, and from which prompt —
   the `BacklogStatusEvent` audit table is the natural home but bucket [1] #9 notes that some
   status paths skip writing to it (already in flight on another session). A policy that expands
   what merges without review makes closing that audit gap more load-bearing.

5. **Distinguish "waiting on CI" from "stuck."** There is no operator-facing surface that says
   "policy item X: fix attempt 2/3, CI running, ETA unknown" vs "stalled." The Review Queue
   shows session attention reasons, not backlog-item policy-loop state. UX-wise the operator
   currently learns of loops only via the churn symptom (task doc bucket [1] #10: "activity
   history cycling every couple minutes") — a designed policy needs a *positive* status surface
   for a healthy loop, not just the absence of a notification.

---

## Key File/Line Index

| What | Location |
|---|---|
| Review-Queue Create-PR button (Path B trigger) | `web-app/src/components/sessions/ReviewQueuePanel.tsx:858-883` |
| Create-PR modal + `onRunOneShot` | `web-app/src/components/sessions/ReviewQueuePanel.tsx:1240-1337` |
| `RunOneShot` handler (Path B) | `server/services/session_service.go:3405-3495` |
| `pushAndCreatePR` (Path A) | `session/backlog_lifecycle.go:1358-1503` |
| `prCreator` interface (reuse seam) | `session/backlog_lifecycle.go:63-68` |
| PASS-verdict → pushAndCreatePR | `session/backlog_lifecycle.go:562-570` |
| work-exit → review (or done if SkipReviewGate) | `session/backlog_lifecycle.go:444-481` |
| `ReconcilePRPending` (Behavior 2 loop) | `session/backlog_lifecycle.go:1508-1630` |
| `AutoReopenForPRFix` (fix respawn + churn guard) | `server/services/backlog_service_triage.go:565-670` |
| `hasActiveWorkSession` / `tombstoneOrphanWorkSessions` | `server/services/backlog_service_triage.go:435-437, 1283+` |
| `AutoReopenAfterFailedReview` (review rework loop) | `server/services/backlog_service_triage.go:494-563` |
| `maxAutoReworkIterations = 3` | `server/services/backlog_service_triage.go:72` |
| `markPRReadyUnmerged` (Behavior 3 notify) | `session/backlog_lifecycle.go:1637-1666` |
| `prReadyToMergeSolo` predicate | `session/stuck_decisions.go:65-95` |
| `prReadyThreshold = 30m` / `stuckPRReady` | `session/stuck_decisions.go:17-42` |
| `git.PRStatus` struct (CI/conflict signals) | `session/git/worktree_git.go:330-355` |
| `BacklogItemData` opt-in flags (pattern to mirror) | `session/repository.go:342-365` (+ `BacklogItemUpdate` `:438-445`) |
| `BackfillMissingPRNumbers` (E11 self-heal) | `session/backlog_lifecycle.go:860-867` |
