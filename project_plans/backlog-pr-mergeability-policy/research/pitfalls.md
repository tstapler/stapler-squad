# Research: Pitfalls, Risks & Trust-Boundary Controls

**Agent**: Research Agent 4 (PITFALLS)
**Date**: 2026-07-17
**Feature**: backlog-pr-mergeability-policy (LLM-authored PRs auto-created and auto-merged without human review)

This is a **trust-boundary change**: enabling the policy means an LLM-authored PR is created
and — via the already-live `allow_auto_merge` + no branch protection on `main` — merged to
`main` with zero human review. Every risk below is scored **BLOCKER-class** (must be designed
against before code), **high**, or **medium**.

---

## 1. The proto3 bool silent-reset trap — **BLOCKER-class**

### Mechanism (confirmed)

Plain proto3 `bool` has no "unset" wire representation: an omitted field arrives as the zero
value `false`, indistinguishable from an explicit `false`. The backend `UpdateBacklogItem`
handler wraps every flag **unconditionally** into a non-nil pointer, so every partial update
writes the flag:

`server/services/backlog_service_lifecycle.go:232-237`
```go
skipRG := req.Msg.SkipReviewGate
update.SkipReviewGate = &skipRG       // always written
skipP := req.Msg.SkipPlanning
update.SkipPlanning = &skipP          // always written
autoSpawn := req.Msg.AutoSpawnSession
update.AutoSpawnSession = &autoSpawn  // always written
```

The `BacklogItemUpdate` struct uses `*bool` (`session/repository.go:438`), so a non-nil
pointer to `false` **is a write**, not a no-op. Any frontend call that omits the field sends
`false` on the wire → backend writes `false` → flag silently reset. This exact bug bit
`AutoSpawnSession` (task doc bucket [2], commit `b28ace2f`).

### The mitigation already in place (must be extended)

`web-app/src/components/backlog/BacklogItemDetail.tsx:306-313` defines `currentFlags()`:
```ts
const currentFlags = useCallback(() => ({
  skipPlanning: item?.skipPlanning ?? false,
  skipReviewGate: item?.skipReviewGate ?? false,
  autoSpawnSession: item?.autoSpawnSession ?? false,
}), [item]);
```

**Every partial `updateBacklogItem` call site in `BacklogItemDetail.tsx` that must spread the new flag** (confirmed by grep):

| Line | Call site | Spreads `currentFlags()`? |
|---|---|---|
| `319` | `handleSaveNotes` — save notes | ✅ `{ ...currentFlags(), notes: notesValue }` |
| `386` | apply triage suggestions — `acCriteria: newAcCriteria` | ✅ `{ ...currentFlags(), acCriteria }` |
| `406` | undo/pre-apply triage — `acCriteria: preApplyCriteria` | ✅ `{ ...currentFlags(), acCriteria }` |
| `440` | gate-reopen feedback — append note | ✅ `{ ...currentFlags(), notes }` |
| `330` | `handleUpdateItem(data)` — full edit form | passes full `data` from `BacklogItemForm` (form must carry the flag) |

**The 4 partial calls (319, 386, 406, 440) each spread `currentFlags()`. The new policy flag
MUST be added to the `currentFlags()` object at line 306-313 — a single edit fixes all four
call sites at once.** The full-form path (330) additionally requires the flag to be present in
`BacklogItemForm`'s emitted `BacklogItemInput`, or the form save resets it.

### Strong recommendation: use the presence-gated pattern instead

`PipelineMode` already demonstrates the **safe** alternative at
`backlog_service_lifecycle.go:238-245` — an `optional` wire field, presence-gated so an omitted
value never clobbers:
```go
// Deliberately NOT an unconditional wrap like SkipReviewGate/SkipPlanning/AutoSpawnSession
if req.Msg.PipelineMode != nil {
    update.PipelineMode = req.Msg.PipelineMode
}
```
The requirements (line 74-77) mandate "follow the established opt-in toggle pattern exactly"
(the *unconditional* bool). That instruction **reintroduces the trap** unless `currentFlags()`
is updated. Planning should explicitly decide: mirror the `AutoSpawnSession` bool + extend
`currentFlags()` (matches convention, one extra failure surface) **or** adopt the safer
`optional bool` presence-gated pattern (structurally immune, diverges from the three existing
flags). For a **trust-boundary flag whose silent reset to `false` fails *open* toward "not
auto-merging"** the downside is limited — but a silent reset to `false` on an item the operator
*intended* to be hands-off means it silently stops progressing, i.e. the exact "nothing gets
merged" failure this feature exists to kill. Presence-gating is the lower-risk choice.

---

## 2. Unbounded fix-loop / churn reintroduction risk — **BLOCKER-class**

### The cap and its terminal state (confirmed)

- `maxAutoReworkIterations = 3` — `backlog_service_triage.go:136`. Same cap covers both
  review-rework (`AutoReopenAfterFailedReview`, line 521) and PR-fix
  (`AutoReopenForPRFix`, line 609).
- Terminal state when the cap is hit (`AutoReopenForPRFix`, lines 609-613):
  ```go
  if workCount >= maxAutoReworkIterations {
      s.notifyReworkCapHit(ctx, itemID, item.Title, ..., "while fixing PR #"+...)
      return nil   // stop — leave in pr_pending for manual action
  }
  ```
- `notifyReworkCapHit` (`backlog_service_triage.go:75-99`) writes a **durable**
  `StuckReasonReworkCap` row via `MarkStuck` (threshold 0 — marked the instant the cap is hit),
  calls `MarkStuckNotified` for DB-backed notify-once, then publishes a `WARNING`/`MEDIUM`
  notification. Restart-surviving; not lost on a missed toast.

### How a NEW auto-fix loop reintroduces the churn bug (this is the danger)

The item-#10 churn bug (task doc bucket [1], row 10) was: `ReconcilePRPending`'s 60s tick
called `AutoReopenForPRFix` unconditionally, transitioned `pr_pending→in_progress`, the spawn
was rejected by the `hasActiveWorkSession` guard, and it rolled back — writing **2
`BacklogStatusEvent` rows every tick forever**, growing the table unboundedly, even while a
legitimate 4-hour autonomous session was still working.

Two guards were required and **both must be respected by any new auto-fix entry point**:

1. `tombstoneOrphanWorkSessions` (commit `af426f27`, `backlog_service_triage.go:597`,
   also `:289`) — ends a work session confirmed dead via `IsSessionLive` before the guard runs.
   **Necessary but insufficient alone** (task doc: loop continued because the blocker was a
   genuinely-alive session, not a dead one).
2. `hasActiveWorkSession`-before-transition (commit `f8f788ab`,
   `backlog_service_triage.go:598-601`) — checks for an active fix **before any status
   transition** and returns early with **zero churn** when a fix is in flight:
   ```go
   s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
   if hasActiveWorkSession(sessions) {
       return nil   // no transition, no BacklogStatusEvent rows
   }
   ```

**Reintroduction vectors to design against:**
- **Behavior-1 auto-create-PR must not add a second, independent spawn path** that skips the
  tombstone+active-session guards. The requirements (line 104-109) correctly say "reuse the
  existing machinery, not a parallel one" — enforce that literally.
- Any new trigger (e.g. a policy-gated tick that fires more aggressively than 60s) multiplies
  the churn rate if it transitions before the guard. The guard order is load-bearing:
  `tombstone → hasActiveWorkSession → (only then) transition`.
- The cap counts `SessionRoleWork` sessions (`:604-608`). If Behavior-1's auto-PR spawn creates
  work-role sessions that don't count, or the fix loop creates sessions of a different role, the
  cap silently stops bounding the loop → **unbounded fix attempts**. Verify the new path's
  sessions increment the same `workCount`.

---

## 3. Auto-merge blast radius — **BLOCKER-class**

### The mechanism (confirmed)

- `EnablePRAutoMerge` runs `gh pr merge <n> --auto --squash`
  (`session/git/worktree_git.go:559-572`). With `allow_auto_merge` on and **no branch protection
  on `main`**, the *only* gate GitHub enforces is "required status checks pass". With no required
  checks configured, `--auto` can merge essentially immediately.
- Called after PR creation at `session/backlog_lifecycle.go:1475`. **Zero human review sits
  between "CI green" and "merged to main".**

### Failure modes

1. **Bad-but-green PR** — CI passes but the change is wrong (LLM misread the AC, subtle
   regression, deleted-but-compiling code). With no reviewer and no required-approval rule,
   it merges. `prReadyToMergeSolo` (`backlog_lifecycle.go:1604`) *deliberately* does **not**
   require `ApprovedCount>0` (pre-mortem F1) because a solo self-authored PR can never get an
   approval — so approval is structurally impossible as a gate here. **CI green is the entire
   safety bar.** This is the core trust-boundary exposure and the strongest argument for a
   kill-switch (§4).
2. **PR becomes conflicting after auto-merge is armed** — `--auto` holds until mergeable; if the
   branch develops a conflict with `main`, GitHub won't merge it and it sits. `ReconcilePRPending`
   detects `HasConflicts` (`backlog_lifecycle.go:1597, 1624-1628`) and routes to
   `AutoReopenForPRFix` to rebase — **but only if the item is in `pr_pending`** (see §5 desync).
   An armed-but-conflicting PR whose item is stuck in `review`/`BOUNCING` is invisible to the
   reconciler and sits forever with auto-merge silently pending.
3. **Auto-merge silently failing** — `EnablePRAutoMerge` returns an error when the repo lacks the
   setting; the caller at `backlog_lifecycle.go:1475-1485` now **notifies** on failure (WARNING,
   commit history references the fix). But the underlying `github_service.go` `MergePR` /
   auto-merge path was historically "best-effort silent fallback" (task doc bucket [2],
   `github_service.go:167`). The *silent* part was fixed separately (commit `47bbe05d`), but
   verify the Behavior-3 notification does not re-swallow: a policy-enabled item whose auto-merge
   arm fails must surface, or the operator believes it's hands-off when it is not.

---

## 4. Kill-switch necessity — **BLOCKER-class (argue FOR)**

**The case FOR a global kill-switch in addition to the per-item opt-in is strong. Recommend
requiring one.**

Given §1-§3: once the policy is on, the safety bar for merging LLM work to `main` is "CI green"
and nothing else. The per-item flag is the *arming* control; it is not a *halting* control:

- **The 2am failure mode with ONLY a per-item flag**: the operator has 15+ policy-enabled items
  in flight (the requirements cite 18 review-queue items, 6 stuck). A systemic problem appears —
  a bad dependency bump makes CI go green on broken code, or the fix-loop starts churning, or a
  prompt-injection / fabrication (see §7) drives a run of bad PRs. To stop it, the operator must
  toggle **every item individually** through `UpdateBacklogItem`, racing the 60s
  `ReconcilePRPending` tick and any in-flight auto-merges that are already armed on GitHub. There
  is no single lever. Meanwhile each armed `--auto` PR merges the moment its checks flip green,
  with no human in the loop, while the operator is clicking item-by-item at 2am.
- A global switch gives **one atomic halt**. Design question 3 (requirements line 145) is the
  right framing: it should gate at **reconciler entry** (stop new spawns *and* stop new
  auto-merge arming) so a single flip freezes the whole subsystem, not just future PR creation.
  Gating only at PR-creation leaves already-armed `--auto` PRs live.

**Case AGAINST (weaker)**: added config surface + another flag to keep in sync; a global switch
that defaults wrong (on) would itself be a blast-radius amplifier. Mitigation: default **off**,
and make the switch *fail-safe* — when set, the reconciler treats every item as
non-policy-enabled regardless of its per-item flag.

**Recommendation**: require the kill-switch, default off, gating at reconciler entry, with a
documented "does it also cancel already-armed GitHub auto-merges?" answer in the ADR (it likely
cannot un-arm GitHub-side `--auto`, which is itself a reason to gate *before* arming).

---

## 5. MCP controller-wiring gap — **NOT a blocker for this feature (verified negative)**

Task doc bucket [2] documents that `mcp__stapler-squad__create_session` logs
`"skipping controller startup, will be started after wiring"` (`session/instance.go:1209`,
also `:977`) and the controller never wires until a UI client opens the session —
`steer_session`/`run_command` then fail with `"cannot send keys to instance that has not been
started or is paused"` (`session/instance_tmux.go:489`).

**Does the auto-fix loop hit this gap? No — verified.** The loop spawns via
`SpawnSessionFromItem` → `s.sessionCreator.CreateWorktreeSession` / `CreateDirectorySession`
(`backlog_service_triage.go:332,335`). Those satisfy the internal `SessionCreator` interface and
**explicitly call `StartController()` after wiring** the status manager:

`server/services/session_service.go:779-788`
```go
if err := instance.Start(true); err != nil { ... }
if s.statusManager != nil {
    instance.SetStatusManager(s.statusManager)
    if ctrlErr := instance.StartController(); ctrlErr != nil {  // controller IS started
        log.Warn(...)
    }
}
```

The MCP gap is specific to the **MCP tool path**, not the server-side `SessionCreator` path the
backlog auto-fix loop uses. **Residual medium risk**: the `StartController()` call is gated on
`s.statusManager != nil`. If the status manager is not wired (degraded startup), the auto-fix
session is created but never becomes drivable — same symptom as the MCP gap, silently. The
warning at line 785 is logged but not surfaced to the operator. Planning should confirm
`statusManager` is always wired in the deployed configuration, or add a surfaced signal when it
is nil.

---

## 6. Silent error-swallowing class — **high**

Task doc bucket [1] documents a recurring `_ = s.storage.Update(...)` pattern that swallowed
errors and left items silently stuck (bugs #2, #5, #6, #7, all fixed individually; the *class*
remains open — no lint rule / wrapper yet). Any new status-transition or notify code in this
feature must not repeat it. Concrete places the new code touches that historically swallowed:

- `AutoReopenForPRFix` note set/restore (`backlog_service_triage.go:642-646, 654-658`) —
  currently logs at `Warning`, does not swallow silently. Good pattern to follow.
- The `pushAndCreatePR` PR-field cache write (`backlog_lifecycle.go:1460-1465`) and the
  `pr_pending` transition (`:1489-1492`) log on failure. The auto-merge-arm failure now
  **notifies** (`:1475-1485`) — the established correct pattern.
- **Requirement for new code**: every `TransitionBacklogItemStatus`, `MarkStuck`,
  `UpdateBacklogItem`, and notify call on the policy path must either surface via
  `notify(...)`/`eventBus.Publish` (operator-visible) or log at `Warning`/`Error` — never `_ =`.
  For a trust-boundary path, a swallowed transition failure means an item the operator thinks is
  auto-merging is actually dead. Mirror `notifyReworkCapHit` / `notifyTriagePersistFailure`
  (`backlog_service_triage.go:75, 106`) which consolidate failures into one operator
  notification. Recommend the ADR call for the structural fix (lint rule banning `_ =
  s.storage.` assignments) so this class is closed rather than patched instance-by-instance.

---

## 7. Fabrication risk — status feed must be independently verifiable — **high**

Task doc references a subagent that fabricated a false "spawned 2 sessions" report. Any
status-report or stuck-item feed that **feeds automation** (as opposed to merely informing a
human) must be independently verifiable against ground truth, because an LLM in the loop can
assert success that did not happen.

Implications for this feature:
- **Behavior-3 "ready to merge" notification must derive from GitHub-side truth, not an agent's
  self-report.** `ReconcilePRPending` already does this correctly: it re-queries `IsPRMerged`
  (`backlog_lifecycle.go:1525`) and `GetPRStatus` (`:1545`) on every tick rather than trusting a
  session's claim. Machine-checkable signals (requirements Q5): `prStatus.Mergeable`,
  `prStatus.CIFailing`, `prStatus.HasConflicts`, `prStatus.IsClosed`
  (`backlog_lifecycle.go:1595-1601`) — all sourced from `gh`, not from the agent. **The design
  must keep the merge/notify decision anchored to these polled GitHub signals; never let a
  session's `TASK_COMPLETE` self-report or an agent's "PR is ready" claim be the trigger.**
- The `prReadyToMergeSolo` predicate (`backlog_lifecycle.go:1604`, `session/stuck_decisions.go`)
  is the right shape: a pure function over independently-fetched `PRInfo`.
- **Corollary for the fix loop**: `AutoReopenForPRFix`'s decision to stop looping must be driven
  by observed GitHub state (CI green + no conflict), not by a fix session reporting "I fixed it."
  A fix session can claim success while CI still fails; only the next poll cycle's independent
  `GetPRStatus` should decide whether the loop is done or re-spawns.

---

## Summary table

| # | Risk | Class | Primary evidence |
|---|---|---|---|
| 1 | proto3 bool silent-reset of the policy flag | BLOCKER | `backlog_service_lifecycle.go:232-237`; `BacklogItemDetail.tsx:306-313,319,386,406,440` |
| 2 | Fix-loop churn reintroduction / unbounded loop | BLOCKER | `backlog_service_triage.go:136,597-613`; guards `af426f27`+`f8f788ab` |
| 3 | Auto-merge to unprotected `main` on CI-green-only | BLOCKER | `worktree_git.go:559-572`; `backlog_lifecycle.go:1475,1604` |
| 4 | No global halt lever (per-item only) | BLOCKER (recommend kill-switch) | §1-3 compounded; requirements Q3 |
| 5 | MCP controller-wiring gap | NOT a blocker (verified) — residual medium via `statusManager==nil` | `session_service.go:779-788`; `instance.go:1209` |
| 6 | Silent error-swallowing on new status/notify code | high | task doc bucket [1]; `notifyReworkCapHit` pattern `:75-99` |
| 7 | Fabricated status feeding automation | high | `backlog_lifecycle.go:1525,1545,1604` (poll-anchored, correct) |
