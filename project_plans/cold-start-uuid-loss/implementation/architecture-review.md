# Architecture Review: cold-start-uuid-loss
**Date**: 2026-08-06
**Verdict**: CONCERNS

> This review re-assesses the rewritten `implementation/plan.md`, which
> repairs `project_plans/session-revive-uuid-loss/implementation/plan.md`'s
> CONCERNS verdict (1 blocker) against the exact same underlying bug,
> retriaged under this project's name. The blocker is resolved below; the
> 3 Concerns and Nitpicks are carried forward with their resolution status
> noted, plus new context introduced by the scope expansion (adversarial
> Blocker 1's fix — adding `Resume()`/`Restart()` as call sites).

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` still does not exist in
this repo. No constitution constraints apply.

## Former Blocker (from session-revive-uuid-loss) — resolution status

- [x] **RESOLVED — new field documented as lock-protected, but written
  unlocked from a path that already races against a real concurrent
  reader.** The prior plan's Task 1.1.1b claimed `everHadConversationHistory`
  was "guarded by claudeSessionMu" while Task 1.2.1c wrote it from
  `tryExtractConversationUUID()`'s existing **unlocked** direct-mutation
  block, explicitly instructing "do not introduce a new lock here." The
  rewrite picks remediation option (a), as recommended: Task 1.2.1c now
  extracts `recoverConversationUUIDByPath()` with its direct-mutation write
  (`ConversationUUID`, `HistoryFilePath`, and the new
  `everHadConversationHistory`) under `claudeSessionMu.Lock()` + nested
  `i.mu.Lock()`, identical to `ClearConversationState()`/`SetHistoryInfo()`'s
  existing pattern, and the same locking is applied to
  `tryExtractConversationUUID()`'s own live-PID-branch write (previously
  also unlocked). This closes the concurrent-read race the blocker's
  evidence named (`HistoryLinker.correlateSession`/`scanAllSessions`'s 5s
  poller calling the lock-protected `HasClaudeSession()` concurrently with
  what used to be an unlocked write) for all three fields this fix touches,
  not just the newly-added one — going slightly further than the
  blocker's minimum ask ("closes the race for the two fields this fix
  needs to be correct"), because Task 1.2.1c was already restructuring this
  exact block to extract the path-only helper for an independent reason
  (adversarial Blocker 1's fix requiring pane-liveness independence — see
  below). The doc comment on `Instance`'s new fields (Task 1.1.1b) now
  states an invariant that is actually true, rather than an aspirational
  one. Verified: no caller of `tryExtractConversationUUID`/
  `recoverConversationUUIDByPath` (5 existing call sites, checked directly)
  already holds `claudeSessionMu` at the call site, so the added internal
  locking introduces no self-deadlock risk.

## Concerns

- [x] **RESOLVED — `coldRestoreOutcome.Resume bool` was a derivable
  duplicate of `Outcome`.** The rewrite drops the `Resume` field entirely;
  `coldRestoreOutcome` now carries only `Outcome ReviveOutcome`, and a new
  `func (o ReviveOutcome) ShouldResume() bool` method (returning true for
  `ResumeLive`/`ResumeRecovered`) is the single source of truth both
  `startLocked`/`start`'s log-branch logic and `Resume()`/`Restart()`'s
  UUID-capture logic now read from. No illegal-state combination is
  representable. Verified via Task 4.1.1i.

- [x] **RESOLVED, and superseded by a stronger fix than either of the two
  originally-offered remediation options — `prepareColdRestore()`'s
  correctness depended on an unenforced, comment-only "pane is dead"
  precondition.** The review offered two remediations: (a) a defensive
  `if i.pm().IsAlive() { log.Error(...) }` guard, or (b) extract a
  path-only helper. The rewrite took (b), and this turned out to be the
  *only* correct choice once adversarial Blocker 1's fix is folded in:
  `Restart()` (Epic 1.7) now calls `prepareColdRestore()` **before**
  `KillSession()`, meaning the pane can legitimately still be alive at call
  time (e.g. `SwitchProgram`'s `Restart(true)` on an `Active` session) —
  option (a)'s defensive `log.Error` would have been a **false positive**
  on that now-real, non-hypothetical path, not just a safety net for a
  theoretical future misuse. `recoverConversationUUIDByPath()` (Task
  1.2.1c) is a pure path-based scan with no dependency on
  `i.pm().IsAlive()` at all, so the precondition question is now moot by
  construction rather than defended against. Verified via Task 4.1.1h.

- [x] **RESOLVED — `onColdRestoreLostHistory` reused
  `rateLimitLinkedItemID`, a feature-specific name, for an unrelated
  notification.** Renamed to `linkedItemIDForInstance` (Task 2.3.1a),
  updating both existing call sites (`onRateLimitDetected`,
  `onRateLimitRecovery`) alongside the new one. Purely mechanical, no
  behavior change.

- [ ] **STILL OPEN, carried forward as a fast-follow recommendation, not
  required for this PR — three new conversation-recovery fields (now five,
  after this rewrite) are mutated from five separate methods, continuing a
  pattern of enforcing a shared invariant by convention rather than by
  type.** The rewrite's Domain Glossary/Unresolved Questions explicitly
  note that `everHadConversationHistory`, `recoverySuppressedGeneration`,
  and `startCycleGeneration` are now mutated from
  `ClearConversationState`, `SetHistoryInfo`, `recoverConversationUUIDByPath`,
  `tryExtractConversationUUID`, and `prepareColdRestore` — one more mutator
  than the prior plan's four, since the field extraction split
  `tryExtractConversationUUID`'s single mutation point into two. The
  original recommendation (a small `ConversationRecoveryState` value type
  with `MarkCaptured()`/`MarkCleared()`/`ConsumeSuppression()` methods) is
  carried forward unchanged and, if anything, applies with slightly more
  force now. Not a blocker — this plan's own expanded test suite (Epic
  4.1, 18 backend test tasks across Stories 4.1.1-4.1.3) directly covers the
  interactions that would break if a mutator forgot a field, including the
  specific staleness scenario (Task 4.1.1d) the adversarial review's
  Blocker 2 was about.

## New observations from this rewrite (not present in the prior review)

- The scope expansion from 2 to 4 call sites (adversarial Blocker 1's fix)
  is architecturally clean: `prepareColdRestore()`'s signature and
  contract did not need to change to accommodate `Resume()`/`Restart()` as
  new callers — only its internal precondition assumption did (see the
  resolved Concern above), which is a strictly better outcome than
  threading caller-specific parameters through it.
- The deliberate decision to *not* fire `EventStarted` from
  `Resume()`/`Restart()` (see the "User-visible signal transport" Pattern
  Decisions row and the new Unresolved Question in plan.md) is the correct
  call architecturally: `EventStarted` already has consumers
  (`BacklogLifecycleListener.onSessionStarted`,
  `review_queue_poller.go`'s reconciliation logic) whose behavior under a
  `Resume()`/`Restart()`-sourced firing was never specified or tested.
  Widening `EventStarted`'s semantics to cover two more call sites would
  have been a second, unreviewed behavioral change riding along with this
  bug fix. Setting `LastReviveOutcome` directly (read by the frontend badge
  via the existing proto/adapter/snapshot plumbing, independent of any
  event) is the right narrower mechanism for those two call sites.

## Nitpicks (carried forward unchanged from session-revive-uuid-loss)

- `startLocked` and `start()` remain two near-duplicate ~90-line functions
  requiring mirrored edits (now joined by `Resume()`/`Restart()` as two
  more call sites needing careful, independent-but-parallel edits — Epics
  1.4-1.7). The plan already identifies and accepts this cost explicitly
  in Step 0.5's alternative-(a) analysis; flagging only as a candidate
  follow-up (unify into fewer functions + thin wrappers), now with a
  slightly larger surface than when this nitpick was first raised.
- Pattern selection elsewhere in the plan remains sound: reusing
  `LifecycleEvent`/`fireLifecycleEvent` + `onRateLimitRecovery`'s shape,
  the proto-enum + `assertNever`-style frontend switch, and the
  additive-only storage/proto migration are all consistent, low-risk
  reuses of existing precedent — no changes needed there.
- Build-vs-buy conclusion remains consistent with the plan as rewritten —
  confirmed no new library, SDK, or service is introduced anywhere in the
  expanded task list.
