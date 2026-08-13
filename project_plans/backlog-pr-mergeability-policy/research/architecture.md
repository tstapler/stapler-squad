# Research: Architecture & Integration Points (Agent 3)

**Project**: backlog-pr-mergeability-policy
**Date**: 2026-07-17
**Scope**: integration architecture for the new per-item policy flag, and root-cause of the
`review`/`BOUNCING`-vs-`pr_pending` status desync that Behavior 2 depends on.

All paths absolute-relative to repo root. Line numbers as of this worktree's HEAD.

---

## 1. THE KEY INVESTIGATION — the `review`/BOUNCING vs `pr_pending` status desync

### 1.1 Every write path that sets a backlog item to `pr_pending`

I traced all writes of `BacklogStatusPRPending`. **There is exactly ONE production code path
that transitions an item INTO `pr_pending`:**

| # | Site | Precondition to fire | Trigger |
|---|------|----------------------|---------|
| **P1** | `session/backlog_lifecycle.go:1487-1492` (`pushAndCreatePR`) | `BacklogItemPrecondition{ExpectedStatus: "review"}` — item MUST currently be `review`, AND push+`CreatePR` must have already succeeded above it | Called ONLY from `handleReviewSessionExited` `case ReviewVerdictPass:` (`backlog_lifecycle.go:562-569`) and from `spawnReviewGate`'s `onPass` closure (`:672`, same function). i.e. **only on a PASS review verdict.** |

Supporting facts:
- `item.PrNumber` / `item.PrURL` are written to the backlog item in **only two places**, both
  inside `pushAndCreatePR`: set at `backlog_lifecycle.go:1460-1463` (on PR create) and cleared
  at `:1561-1564` (closed-PR path in `ReconcilePRPending`). Nowhere else in non-test code.
- `FindPRPendingItems` (`session/storage_backlog.go:550-577`) filters on
  `backlogitem.Status("pr_pending")` **AND** `PrNumberGT(0)`. An item that is not `pr_pending`,
  or is `pr_pending` with `PrNumber==0`, is structurally invisible to `ReconcilePRPending`
  (which also early-`continue`s on `item.PrNumber == 0 || item.PrURL == ""` at
  `backlog_lifecycle.go:1515`).
- `review → pr_pending` IS a structurally-valid transition (`session/domain/backlog.go:247-254`),
  so the transition table is not the blocker — the missing *trigger* is.

### 1.2 What routes an item AWAY from `pr_pending` so it stays in `review`

`handleReviewSessionExited` (`backlog_lifecycle.go:553-570`) is the fork:

```
verdict == PASS                         → pushAndCreatePR → pr_pending          (P1)
verdict == FAIL/PARTIAL/UNVERIFIABLE    → AutoReopenAfterFailedReview           (bounce)
verdict missing (crash/killed/no-turns) → AutoReopenAfterFailedReview           (bounce)
```

`AutoReopenAfterFailedReview` (`server/services/backlog_service_triage.go:497-563`):
- Counts prior work sessions. If `workCount >= maxAutoReworkIterations` (=3), it
  **deliberately leaves the item in `review`**, fires `notifyReworkCapHit`, and returns nil
  (`:521-525`). The item is now parked in `review` with an open `rework_cap` stuck row and
  **no path back toward `pr_pending` without a human/PASS verdict.**
- Below the cap, it rolls `review → in_progress` (`:535`) and respawns an autonomous work
  session. Each rework that comes back for review and does not PASS is one `in_progress↔review`
  bounce. After enough non-PASS cycles the `bouncing` detector
  (`reconcileBouncingItems`, `backlog_lifecycle.go:1249-1305`; `isBouncing`,
  `session/stuck_decisions.go:58-62`) flags it, and the cap eventually parks it in `review`.

### 1.3 Concrete root-cause hypothesis for PR #157

> **PR #157's item is stuck at `review` because it never received a PASS gate verdict (only
> PARTIAL), so the sole `review → pr_pending` trigger — `pushAndCreatePR`, invoked ONLY on the
> PASS branch of `handleReviewSessionExited` (`backlog_lifecycle.go:562-569`) — never ran.
> After 3 rework iterations `AutoReopenAfterFailedReview` hit `maxAutoReworkIterations` and by
> design left the item parked in `review` (`backlog_service_triage.go:521-525`). The PR that
> exists (#157) was created OUT-OF-BAND via the Review-Queue manual `RunOneShot` path
> (`server/services/session_service.go:3405`), which stamps the PR URL onto the *session
> `Instance`* (`SetGitHubPR`, `session_service.go:3480`) and NEVER touches the backlog item's
> `status`, `PrNumber`, or `PrURL`.**
>
> **`ReconcilePRPending` never runs against this item because `FindPRPendingItems`
> (`storage_backlog.go:550-577`) requires `status == pr_pending` AND `pr_number > 0`, and this
> item satisfies neither — it is `review` with `PrNumber == 0`. Therefore the auto-fix loop
> (Behavior 2) is structurally unreachable for it, exactly as the requirements observe.**

This is a genuine **two-writer split-brain**: GitHub/PR-truth lives on the `Instance` record
(via `RunOneShot`), while the reconciler's world-view is keyed on the backlog-item record. The
two are never reconciled because no code copies the Instance's PR back onto the item, and no
code transitions the item to `pr_pending` absent a PASS verdict.

### 1.4 A second, independent desync trigger worth noting

Even on the automated PASS path, `pushAndCreatePR`'s `review → pr_pending` transition
(`backlog_lifecycle.go:1487-1489`) is preconditioned on `ExpectedStatus: "review"`. If the item
is NOT in `review` at that instant (e.g. a concurrent `AutoReopenAfterFailedReview` already
rolled it to `in_progress`, or a reconciler moved it), the transition fails
`ErrPreconditionFailed`, the code logs and `return`s (`:1490-1491`) — the PR is already created
and auto-merge already enabled on GitHub, but **the item is left off `pr_pending` with its PR
fields possibly already written**. `BackfillMissingPRNumbers` (`backlog_lifecycle.go:863-867`)
only rescues items *already* in `pr_pending` with a missing number — it does not rescue an item
left in `review`/`in_progress` that has a live PR. So there are (at least) two roads into the
same desync.

### 1.5 Evidence that would CONFIRM the hypothesis (for planning)

Run against the live item behind PR #157 (via `get_backlog_item` MCP / RPC):
1. **Item `status` == `review`** (not `pr_pending`) — confirms it never took P1. *(primary)*
2. **Item `PrNumber == 0` and `PrURL == ""`** — confirms the PR was created out-of-band, not by
   `pushAndCreatePR` (which would have stamped them). *(primary)*
3. **`BacklogStatusEvent` audit rows** for the item show `in_progress↔review` bounces and NO
   `*→pr_pending` row — confirms P1 never fired. (Audit rows are written ONLY by
   `TransitionBacklogItemStatus`, `ent_repository_backlog.go:578-590`.)
4. **An open `rework_cap` `BacklogStuckState` row** anchored at `review`, and the most-recent
   review verdict is PARTIAL/non-PASS (`GetMostRecentReviewVerdictForItem`).
5. **Log absence**: grep `~/.stapler-squad/logs/` for
   `pushAndCreatePR item=<id> → pr_pending` — expected ABSENT; and the session driving PR #157
   should show a `RunOneShot`/`extractPRURL` trail instead.

### 1.6 Scope recommendation (the requirements' Open Question #1)

This is a **Phase-0 prerequisite bug fix, in scope.** Behavior 2 (auto-fix loop) is *defined*
as "built on `ReconcilePRPending`", and that reconciler is provably unreachable for the exact
class of item the feature targets. Shipping Behavior 2 without closing the desync delivers a
loop that never engages for the very items (bounced-then-parked, PR-created-out-of-band) that
motivate the feature. The minimal fix is small and localizable (see §2), so the cost of
in-scoping it is low relative to the correctness it buys.

**Minimal Phase-0 fix options (for planning to choose in the ADR):**
- **(a) Converge PR creation** so ALL PR creation flows through `pushAndCreatePR` (or a shared
  helper it calls), which is the ONE path that both stamps `PrNumber`/`PrURL` on the item AND
  transitions to `pr_pending`. This structurally eliminates the out-of-band writer. *(preferred
  — see §2.3)*
- **(b) Add a reconciler that adopts orphaned PRs**: a detector that finds `review`/`in_progress`
  items whose linked session `Instance` has a `github_pr_url` but whose item `PrNumber==0`, and
  promotes them to `pr_pending` (writing item PR fields from the Instance). This is a
  belt-and-suspenders backstop; keep even if (a) is done, to catch legacy/manual PRs.

---

## 2. Integration architecture for the new policy flag

### 2.1 Where the reconciler reads the flag (Behavior 2 gate)

`ReconcilePRPending` (`backlog_lifecycle.go:1508-1630`) already does the full CI-green /
conflict / closed-PR / merged detection and calls `fixSpawner.AutoReopenForPRFix` on
CI-fail/conflict/blocking-review (`:1626`). **The per-item policy read belongs at the top of the
per-item loop body** (`:1514`, right after the `PrNumber==0` guard): skip the
`AutoReopenForPRFix` spawn branches (`:1578`, `:1626`) for items whose policy flag is `false`,
leaving the merged→done and notify branches active for all items. Concretely: load
`item.<PolicyFlag>` (the item is already in hand as `*ent.BacklogItem`) and gate the two
`fixSpawner.AutoReopenForPRFix(...)` calls on it. No new query is needed — `FindPRPendingItems`
already returns the full item.

Note the item objects returned by `FindPRPendingItems` are `*ent.BacklogItem`, so the new ent
column is directly available; the `BacklogItemData` DTO conversion (`backlogItemToData`) must
also carry the field for the RPC/UI layers.

### 2.2 Where Behavior 1's auto-PR-create hooks in

Behavior 1 ("auto-create PR when a policy-enabled session reports Complete") maps onto the
**existing PASS-verdict path** — `pushAndCreatePR` already does push → `CreatePR` →
`EnablePRAutoMerge` → `pr_pending` with zero human clicks (`backlog_lifecycle.go:1416-1493`).
For backlog items that go through the review gate and PASS, **auto-PR already works today.** The
gap Behavior 1 closes is the items where that PASS path never fires:
- `SkipReviewGate` items skip the gate entirely (`review_gate.go:59-62`) and go
  `in_progress → done` directly (per task doc bucket [2]) — no PR at all.
- Items that bounce and never PASS (the §1 class) — a human clicks Create-PR in the Review Queue.

So Behavior 1's hook is **not a new call site** but a **policy-driven redirect**: for
policy-enabled items reaching Complete, drive them through `pushAndCreatePR` regardless of
whether a PASS verdict was produced (or make "policy-enabled + Complete" itself a trigger for
`pushAndCreatePR`). This is exactly the convergence recommended in §2.3.

### 2.3 The two divergent PR-creation paths — map & recommendation

| | **Backlog-automated path** | **Review-Queue manual path** |
|---|---|---|
| Entry | `handleReviewSessionExited` PASS → `pushAndCreatePR` | `ReviewQueuePanel.tsx` "Create PR" → `onRunOneShot` → `RunOneShot` RPC |
| Code | `session/backlog_lifecycle.go:1363-1503` | `server/services/session_service.go:3405-3495` |
| PR creation | `g.CreatePR(...)` via `PRCreator` factory | LLM agent runs a prompt containing `gh pr create`; URL scraped by `extractPRURL` |
| Auto-merge | `g.EnablePRAutoMerge(prNumber)` (`:1475`) | **none** — operator merges manually |
| Writes item `PrNumber`/`PrURL` | **YES** (`:1460-1463`) | **NO** — writes `inst.SetGitHubPR` on the *session* (`session_service.go:3480`) |
| Transitions item `→ pr_pending` | **YES** (`:1487-1489`) | **NO** — item status untouched |
| Reconciler-visible afterward | **YES** | **NO** ← the desync |

**Recommendation: Behavior 1 should converge on the backlog-automated `pushAndCreatePR`
path.** It is the only path that keeps the backlog item and the reconciler consistent (stamps
PR fields, transitions to `pr_pending`, enables auto-merge). `RunOneShot` is a generic
session-scoped one-shot runner with no backlog awareness; extending it to also write backlog
state would duplicate `pushAndCreatePR`'s logic and re-introduce a second writer. The right move
is to make the Review-Queue "Create PR" button (for policy-enabled items) invoke the same
backlog PR-creation machinery rather than `RunOneShot`, OR to auto-fire `pushAndCreatePR` on
Complete for policy-enabled items so the manual button is never needed. Either way, **collapse
to one writer** — this simultaneously fixes §1's desync and satisfies the requirements'
"reuse the existing PR-creation machinery, not a parallel one" constraint.

### 2.4 Where Behavior 3's notification fires — IT LARGELY EXISTS ALREADY

`markPRReadyUnmerged` (`backlog_lifecycle.go:1637-1666`) is **already the "ready to merge"
notification**, and it already fires on the correct signal:
- Called from `ReconcilePRPending` (`:1604-1605`) only when
  `!CIFailing && !HasBlockingReviews && !HasConflicts` (`:1584`) AND `prReadyToMergeSolo(info)`
  is true — i.e. **CI green AND no conflict AND mergeable**, not on mere PR existence.
- It is **durably notify-once**: `MarkStuck(...pr_ready_unmerged...)` opens a `BacklogStuckState`
  row, and it only notifies when `row.NotifiedAt == nil` and past `prReadyThreshold`
  (`:1653`), then calls `MarkStuckNotified` (`:1663`). The dedup survives restarts (DB column,
  not a process timer).

**Behavior 3 is therefore mostly a matter of reachability, not new code**: because
`markPRReadyUnmerged` runs only inside `ReconcilePRPending`, an item that never reaches
`pr_pending` (the §1 class) never gets the notification. Fixing the §1 desync makes Behavior 3
work for the target items almost for free. Planning should decide whether the requirements'
"exactly one 'ready to merge' notification" is satisfied by this existing mechanism (likely
yes) or whether the threshold/copy needs tuning.

---

## 3. Workflow / state-machine layer

`session/workflow_engine.go` — `WorkflowEngine` interface (`CanTransition` / `ValidateGates` /
`AllowedTransitions`) with the single `DefaultWorkflowEngine` implementation. It governs
**status-transition guards only** — structural validity (`validTransitions` map,
`domain/backlog.go:224-274`) plus business-rule guards (`TransitionGuard`,
`domain/backlog.go:324+`). It does NOT govern stage/skill selection (the never-implemented
`ConfiguredWorkflowEngine` of ADR-013 — out of scope per requirements).

**Does auto-PR-on-Complete need a new transition or guard? NO for the transition:**
- `review → pr_pending` already valid (`domain/backlog.go:248`).
- `pr_pending → in_progress` (fix loop) already valid (`:257`).
- `in_progress → review` (bounce) already valid (`:242`).

The transition table is complete for all three behaviors. **The relevant guard** is
`review → done` (`TransitionGuard`, `domain/backlog.go:355-357+`), which enforces
`ErrPRRequired`/`ErrVerdictRequired` — this guards the merge-completion semantics and should be
left intact. No new guard is required for the policy; the policy is a per-item *routing* flag
read at the reconciler / PR-create call sites (§2.1, §2.2), not a new transition rule. If the
ADR chooses to make "Complete + policy-enabled" a distinct signal, it still reuses existing
transitions.

**Positive pattern the task doc calls out (and to reuse):** `workflow_engine.go`'s
**narrow-interface + deep-copy-on-construct** design — `NewDefaultWorkflowEngine`
(`:24-34`) deep-copies `validTransitions` into a private field so no shared mutable global
state leaks; the interface exposes only the three methods consumers need. This matches the
repo's anti-interface-pollution convention (`.claude/rules/interface-pollution-checklist.md`).
Any new policy-reading seam should follow the same shape — read the flag off the concrete item
in the consumer, no speculative `PolicyEngine` interface unless a second implementation is
imminent.

---

## 4. Kill-switch placement options

**A global kill-switch already has a home: the existing feature-flag mechanism.**
- Config layer: `config/config.go:238-241` (`FeatureFlags map[string]bool`), with
  `GetFeatureFlag` (`:853-857`) / `SetFeatureFlag` (`:861-865`). Defaults to `false` when unset
  (`TestGetFeatureFlag_defaultsFalse`).
- RPC/registry layer: `server/services/feature_flag_service.go` — `knownFeatureFlags`
  (`:16-36`) is the authoritative list; `GetFeatureFlags`/`UpdateFeatureFlag` RPCs expose them,
  and `SetFeatureController` (`:64`) wires a runtime `FeatureController` (Enable/Disable) per
  flag. Adding `{name: "backlog:auto-merge-policy", description: ...}` to `knownFeatureFlags`
  gives a UI-toggleable, config-persisted global switch with **zero new plumbing**.

**Precedent for gating a whole loop on a flag:** the entire 60s reconcile ticker is already
gated on `cfg.GetFeatureFlag("backlog")` at startup (`server/dependencies.go:863, 893-894`).

**Where the kill-switch should gate (control-flow options, cheapest → deepest blast-radius):**
1. **Merge-enable** — skip `g.EnablePRAutoMerge` (`backlog_lifecycle.go:1475`). Narrowest: PRs
   still get created but never auto-merge. Good "pause merges, keep creating" mode.
2. **PR-create** — short-circuit `pushAndCreatePR` before push/CreatePR. Stops new automated PRs
   but lets in-flight `pr_pending` items finish.
3. **Reconciler entry / fix-spawn** — gate the `AutoReopenForPRFix` spawn branches in
   `ReconcilePRPending` (`:1578`, `:1626`). Stops the auto-fix loop from spawning sessions.

**Recommendation:** a single global flag checked at **all three** points (defense in depth),
defaulting to `false` (OFF), so flipping it OFF halts new PR creation, new auto-merge, and new
fix-spawns while leaving already-created PRs to be finished/merged by a human. The per-item
opt-in (§2) and the global kill-switch are complementary: item-flag = "which items may
automate", global-flag = "is automation running at all". Given the requirements' emphasis on
"no branch protection on `main`" = full blast radius, the global OFF-by-default kill-switch
should be treated as required, not optional (resolves Open Question #3; the ADR must state the
default and the gate points).

---

## 5. Data-flow / consistency concerns

### 5.1 Audit-row / notify-once durability — current state

- **`BacklogStatusEvent` audit rows are written ONLY by `TransitionBacklogItemStatus`**
  (`ent_repository_backlog.go:578-590`, best-effort/non-fatal). Any status change that bypasses
  this method (direct ent `SetStatus`, e.g. `ReconcileStuckItems` / `ArchiveBacklogItem` per
  task doc bucket [1] #9) leaves no audit trail — a known gap already in flight on another
  session. **Implication for this feature: every status write it introduces MUST go through
  `TransitionBacklogItemStatus`**, never a raw ent update, or it will be invisible to the audit
  trail and to the desync-diagnosis evidence in §1.5.
- **The durable notify-once mechanism already exists and is the right model to reuse:**
  `BacklogStuckState` rows carry `first_detected_at` + `notified_at` (DB columns), and the
  `MarkStuck` → `FindOpenStuckStates` → check `NotifiedAt == nil` + threshold → `MarkStuckNotified`
  sequence (canonically in `markPRReadyUnmerged`, `backlog_lifecycle.go:1637-1666`, and
  `notifyReworkCapHit`, `backlog_service_triage.go:75-84`) gives restart-surviving,
  once-per-condition delivery. The `notify`/`Notify` method (`backlog_lifecycle.go:275`) is the
  **ephemeral** push-toast — non-durable, fires every call; use it only as the delivery
  transport *alongside* a durable `MarkStuckNotified` guard, never as the dedup itself.

### 5.2 How the "ready to merge" notify-once should be made durable

**It already is** — via the `pr_ready_unmerged` `BacklogStuckState` row (§2.4). The
requirements' Behavior 3 ("exactly one 'ready to merge' notification per item, fired only when
CI green AND no conflict") is satisfied by the existing `markPRReadyUnmerged` durable
notify-once, **provided the item is reachable in `pr_pending`** (fixed by §1). Planning's job
is:
1. Ensure §1's fix makes policy-enabled items reach `pr_pending` so `markPRReadyUnmerged` runs.
2. Confirm the "exactly one" guarantee across the fix-loop: after a fix cycle
   (`AutoReopenForPRFix` → `in_progress` → back to `pr_pending`), the `pr_ready_unmerged` row is
   **resolved** on leaving `pr_pending` (`backlog_service_triage.go:628-632`) and re-opened on
   return, which re-arms the notification. The ADR must decide whether a re-notification after a
   fix cycle is "exactly one per item" (arguably one per ready-episode) — a copy/threshold
   decision, not a durability one.
3. **Escalation vs "ready" contention (Open Question #4):** the rework-cap escalation
   (`notifyReworkCapHit`, durable `rework_cap` row) and the "ready to merge" notification
   (durable `pr_ready_unmerged` row) are keyed on **different stuck reasons and different
   anchor statuses** (`rework_cap`/`review` or `pr_pending` vs `pr_ready_unmerged`/`pr_pending`),
   so they cannot double-fire for the same condition, but an item CAN legitimately surface both
   across its lifetime. Planning should define the terminal state: on `maxAutoReworkIterations`
   during PR-fix, `AutoReopenForPRFix` already parks in `pr_pending` + `notifyReworkCapHit`
   (`backlog_service_triage.go:609-613`) — that is the defined escalation, and it does not loop
   unbounded.

### 5.3 Reconciler-tick consistency

`ReconcilePRPending` runs inside the 60s `ReconcileStuck` sweep (`backlog_lifecycle.go:912-914`),
one tick, panic-isolated per detector (`runStuckDetector`). It makes live GitHub API calls per
item (`IsPRMerged`, `GetPRStatus`) — the policy read (§2.1) must be a cheap in-memory field
check that gates the *spawn*, not the *polling*, so merged/closed detection keeps working for
all items regardless of policy (only the auto-fix spawn is policy-gated). The tick cadence means
Behavior 1/2/3 are eventually-consistent with a ≤60s latency floor; `request_review` and
`EventExited` provide the faster event-driven paths, with the tick as the safety net
(`dependencies.go:867-868`).

---

## Summary of concrete file:line anchors for planning

| Concern | Anchor |
|---|---|
| Sole `→ pr_pending` writer (P1) | `session/backlog_lifecycle.go:1487-1492` |
| PASS-verdict fork | `session/backlog_lifecycle.go:553-570` |
| Rework-cap parks in `review` | `server/services/backlog_service_triage.go:521-525` |
| Out-of-band PR writer (desync source) | `server/services/session_service.go:3470-3486` |
| Reconciler visibility filter | `session/storage_backlog.go:550-577` + `backlog_lifecycle.go:1515` |
| Behavior 2 spawn gate points | `session/backlog_lifecycle.go:1578`, `:1626` |
| Behavior 3 (already exists, durable) | `session/backlog_lifecycle.go:1637-1666` |
| Auto-merge enable (kill-switch pt 1) | `session/backlog_lifecycle.go:1475` |
| Transition table (no new transition needed) | `session/domain/backlog.go:224-274` |
| Transition guards | `session/domain/backlog.go:324+` |
| Workflow engine (narrow-iface pattern to reuse) | `session/workflow_engine.go:24-64` |
| Global flag config layer | `config/config.go:238-241, 853-865` |
| Global flag RPC/registry | `server/services/feature_flag_service.go:16-64` |
| Reconcile ticker (gated on `backlog` flag) | `server/dependencies.go:863-884` |
| Audit rows only via this method | `session/ent_repository_backlog.go:578-590` |
| Per-item opt-in flag pattern | `session/repository.go:342` (`AutoSpawnSession` sibling) |
