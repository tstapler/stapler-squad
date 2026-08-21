# Adversarial Review: cold-start-uuid-loss

**Date**: 2026-08-06
**Verdict**: CONCERNS

> This review re-assesses the rewritten `implementation/plan.md`, which
> repairs `project_plans/session-revive-uuid-loss/implementation/plan.md`'s
> BLOCKED verdict (2 blockers) against the exact same underlying bug,
> retriaged under this project's name. Both former blockers are resolved
> below by the rewrite; the review's 3 Concerns and Minors are carried
> forward with their resolution status noted.

## Former Blockers (from session-revive-uuid-loss) — resolution status

- [x] **RESOLVED — Two more launch-command call sites reproduce the exact
  bug and were never touched by the prior plan.** The rewrite adds Epic 1.6
  (`Instance.Resume()`) and Epic 1.7 (`Instance.Restart()`), each replacing
  a raw `i.claudeSession.ConversationUUID` capture with a call to
  `i.prepareColdRestore()` before building the launch command — mirroring
  Epic 1.4/1.5's pattern exactly, via the same shared helper (AC4 still
  holds: `grep -c "func (i \*Instance) prepareColdRestore"` still resolves
  to `1`). `Restart()`'s integration (Task 1.7.1a) is called before
  `KillSession()`, which meant the original "caller has already confirmed
  `!i.pm().IsAlive()`" precondition baked into the prior plan's
  `prepareColdRestore()` no longer universally holds — the rewrite handles
  this by extracting a pane-liveness-independent `recoverConversationUUIDByPath()`
  helper (Task 1.2.1c) rather than gating on liveness, which also happens
  to resolve the architecture review's Concern 2 (see below) as a side
  effect of fixing this blocker properly rather than superficially.
  Verified via Tasks 4.1.2c/4.1.2d (new integration tests asserting
  `--resume <uuid>` in `i.LaunchCommand` after `Resume()`/`Restart()`) and
  4.1.1h (`prepareColdRestore` works regardless of `i.pm().IsAlive()`).

- [x] **RESOLVED — `RecoverySuppressed` was only consumed inside
  `prepareColdRestore()`, unreachable from `Restart()`/`Resume()`, so the
  one-shot flag could sit armed indefinitely and wrongly suppress a later,
  unrelated legitimate recovery.** The rewrite replaces the bare
  `recoverySuppressed bool` with a generation-scoped design:
  `startCycleGeneration uint64` (incremented on every `prepareColdRestore()`
  call, from any of the four sites) and `recoverySuppressedGeneration
  uint64` (armed to `startCycleGeneration + 1` inside `ClearConversationState()`,
  consumed only when it exactly matches the generation `prepareColdRestore()`
  is currently processing). This is self-correcting by construction: even
  if a hypothetical future fifth call site forgot to route through
  `prepareColdRestore()`, a stale `recoverySuppressedGeneration` simply
  fails the equality check on the next real cycle rather than silently
  applying to it — no call site has to remember to reset anything. The
  exact trace from the original blocker (`SwitchProgram` →
  `ClearConversationState()` → a later, unrelated cold-restore) is now
  additionally moot on two independent grounds: (1) `Restart()` (which
  `SwitchProgram` calls) now itself calls `prepareColdRestore()`, so the
  very next cycle after the clear consumes the suppression correctly
  regardless; (2) even setting that aside, the generation-match design
  means a suppression that somehow *did* skip a cycle would not
  mis-fire on a later, unrelated one. Verified via Task 4.1.1d (replays the
  original trace directly: two `prepareColdRestore()` calls with no
  intervening clear, second one must not be suppressed) and Task 4.1.3c.

## Concerns

- [x] **RESOLVED — Notification ID has no per-occurrence entropy.**
  `onColdRestoreLostHistory`'s `notifID` now includes a `time.Now().Unix()`
  suffix (Task 2.3.1c), so a second `FRESH_LOST_HISTORY` occurrence for the
  same session routes through `NotificationHistoryStore.Append()`'s
  `findUnreadDuplicate` collapse-or-create-new logic instead of colliding
  on the exact-ID-match idempotency check. Verified via Story 2.3.1's
  second AC.

- [x] **RESOLVED — pitfalls.md design-against-checklist item #7
  (corrupt/0-byte JSONL sanity check) was silently dropped.** Task 1.2.1d
  adds a `info.Size() == 0` skip to `DetectByPath`'s candidate loop —
  cheap, and applied at the shared source rather than per-caller, so it
  also protects `tryExtractConversationUUID`'s existing (pre-fix) usage of
  `DetectByPath`, not just the new `prepareColdRestore` path. This is a
  minimal size check, not full JSONL content validation; the plan's
  Pattern Decisions table documents that scope explicitly rather than
  silently. Verified via Task 4.1.1j.

- [ ] **STILL OPEN, carried forward unchanged — no timeout on the
  now-more-frequently-exercised `DetectByPath` scan.** The rewrite adds a
  *third* call site for `recoverConversationUUIDByPath`/`DetectByPath`
  (`Restart()`, now called before `KillSession()` on every driver-triggered
  restart and every `SwitchProgram` restart, not just on confirmed-dead
  panes). This makes the underlying concern marginally more relevant than
  when it was raised against the two-call-site prior plan, not less. The
  rewrite's own Pattern Decisions table names this explicitly as a
  deliberate non-fix ("not added in this pass... candidate follow-up, not
  blocking") rather than silently dropping it a second time, which is the
  main thing this review asked for the first time it was raised — but the
  underlying risk itself is unaddressed. Recommend the implementer take a
  second look at scoping a lightweight timeout (or at minimum confirming
  `DetectByPath`'s typical directory size makes this a non-issue in
  practice) during Epic 1.7's PR review, given it's now on the restart-driver
  hot path.

## New observations from this rewrite (not present in the prior review)

- **Minor**: the rewrite introduces an intentional asymmetry —
  `startLocked`/`start` fire `EventStarted` with
  `ReasonColdRestoreLostHistory` (driving the toast notification via Epic
  2.3), while `Resume()`/`Restart()` set `i.LastReviveOutcome` but
  deliberately do not fire `EventStarted` (avoiding side effects on
  `BacklogLifecycleListener.onSessionStarted` and review-queue
  reconciliation logic that also listen for that event). This means a
  `FRESH_LOST_HISTORY` reached via `Resume()`/`Restart()` gets the durable
  badge but not the toast. The plan documents this explicitly as a Pattern
  Decision and a new Unresolved Question rather than leaving it implicit —
  correctly scoped as "meets AC3's at-minimum bar via the badge alone,"
  but flagging that a reviewer should explicitly sign off on this being
  acceptable rather than assuming symmetry across all four call sites.
- **Minor**: Task 1.7.1b documents, as a deliberate non-change, that
  `Restart()`'s existing `waspaused` worktree-recreation override (which
  raw-clears `claudeSessionID`/`ConversationUUID`/`HistoryFilePath` without
  going through `ClearConversationState()`, and therefore without arming
  `recoverySuppressedGeneration`) is left as-is. This is a reasonable scope
  boundary — no `prepareColdRestore()` call happens again within that same
  cycle, so there's no future-cycle leak to guard against — but it does
  mean this one clear path remains inconsistent with the "all intentional
  clears go through `ClearConversationState()`" pattern the rest of the fix
  establishes. Not a blocker; noted for a reviewer's awareness.

## Minors (carried forward unchanged from session-revive-uuid-loss)

- The deliberate decision not to add a "resumed via low-confidence
  path-fallback" signal is reasoned and documented — flagging only as a
  reminder that a same-directory collision is completely invisible to the
  end user when it actually happens, unlike `FRESH_LOST_HISTORY` which is
  loud by design. Not a plan defect.
- Task 1.5.1's acceptance criterion leans on a `grep -c` proxy for "no
  duplicated divergent logic" — fine as a supplement to the real
  behavioral assertion in the same task, weak evidence in isolation.
- Minor task-numbering drift ("Task 3.1.2a" under Story 3.1.1's task list)
  carried forward unchanged — cosmetic.
