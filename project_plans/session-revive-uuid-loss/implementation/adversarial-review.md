# Adversarial Review: session-revive-uuid-loss

**Date**: 2026-08-06
**Verdict**: BLOCKED

## Blockers

- [ ] **Two more launch-command call sites reproduce the exact bug and are never touched by this plan.**
  `Instance.Resume()` (`session/instance.go:1386-1451`) and `Instance.Restart()`
  (`session/instance.go:1509-1573`) each build `i.buildLaunchCommand(claudeSessionID)`
  directly from `i.claudeSession.ConversationUUID` — with **no** `DetectByPath`/
  `tryExtractConversationUUID`/`prepareColdRestore` recovery attempt — exactly the pattern
  `startLocked`/`start` had before this fix. Confirmed reachable in production:
  - `Resume()` is called from the `Resume` RPC handler
    (`server/services/session_service.go:1863`) — this is literally the "revive a paused
    session" user action the project is named after. Its own comment at
    `instance.go:1443-1446` ("Tmux session is dead (killed on pause to free memory)... 
    Rebuild the TmuxSession object with the current Claude UUID") describes precisely the
    dead-pane/no-recovery scenario AC1 is meant to fix, and does not fix it.
  - `Restart()` is called from `handleDriverFailure`'s non-`Stopped` branch
    (`session/session_driver.go:551`, `inst.Restart(false)`) — and `research/features.md:14`
    already identifies `handleDriverFailure`/`SessionDriver` as "the actual *trigger* that
    repeatedly calls restart faster than UUID capture can complete in the field-observed
    failure." The plan's own research named the trigger mechanism but never traced that its
    non-`Stopped` branch calls a code path (`Restart`) outside the fix's scope. `Restart()`
    is also called from `SwitchProgram` (`session/instance_program.go:76`) after that same
    function's own `ClearConversationState()` call (line 66).
  - `research/architecture.md` has zero mentions of `Restart` or `Resume()` — this gap was
    never surfaced during Phase 2 research despite `features.md` naming the exact caller.
  requirements.md explicitly scoped the fix to only `startLocked` (~L867-921) and `start`
  (~L1067-1127); AC4's "symmetric... two cold-restore call sites" language and the Domain
  Glossary's `ColdRestore` definition both codify this incomplete scope. As written, this
  plan will ship a fix that does not apply to what is very plausibly the dominant
  field-observed trigger (SessionDriver-initiated restarts and paused-session resume).
  — **Recommendation**: expand requirements.md's scope (and the plan's Epics 1.4/1.5) to add
  a `prepareColdRestore()`-equivalent call in `Resume()` and `Restart()` before each builds
  its launch command, or explicitly re-scope this item to only "the `startLocked`/`start`
  cold-restore path" with a named, tracked follow-up backlog item for `Resume()`/`Restart()`
  — do not leave the gap silent.

- [ ] **`RecoverySuppressed` is only consumed inside `prepareColdRestore()`, which (per the
  blocker above) is unreachable from `Restart()`/`Resume()` — so the one-shot flag can sit
  armed indefinitely and wrongly suppress a later, unrelated, legitimate recovery.** Traced
  concretely, per the review's explicit ask:
  1. `SwitchProgram` calls `i.ClearConversationState()` (sets `recoverySuppressed = true`,
     `everHadConversationHistory = false`) then, if `i.Status == Active`, calls
     `i.Restart(true)` (`session/instance_program.go:66-79`) — **not** `Start`/
     `StartWithCleanup`. `Restart()` never calls `prepareColdRestore()`, so the flag is not
     consumed here.
  2. The session runs normally afterward; a new, legitimate conversation eventually starts
     and captures a fresh UUID (`SetHistoryInfo`/`tryExtractConversationUUID` correctly set
     `everHadConversationHistory = true` again per Task 1.2.1b/c) — but nothing ever resets
     `recoverySuppressed` back to `false`, since only `prepareColdRestore()`'s branch 2 does
     that, and no `startLocked`/`start` cold-restore has occurred since step 1.
  3. Later, this *new* conversation's UUID is itself lost to a watchdog restart race (the
     exact class of bug this whole project fixes) and the session finally does hit
     `startLocked`/`start`'s `ColdRestore` branch. `prepareColdRestore()` now runs: branch 1
     (`HasClaudeSession()`) is false, so branch 2 fires on the **stale, unrelated**
     `recoverySuppressed` flag from step 1 and returns `FreshExpected` — silently discarding
     a real, recoverable JSONL for a conversation that has nothing to do with the original
     program switch. This is the exact silent-fresh-start failure mode requirements.md
     describes, reintroduced by the fix meant to prevent it.
  The plan's Unresolved Questions section only defends `EverHadConversationHistory`'s
  re-earning behavior ("already handles this correctly by construction") — it does not make
  the same claim for `RecoverySuppressed`, and the trace above shows that claim would be
  false if it were made. — **Recommendation**: don't model suppression as a bare
  one-shot bool consumed only inside `prepareColdRestore`. Either (a) clear
  `recoverySuppressed` unconditionally at the start of *every* subsequent start/restart
  attempt regardless of which branch/method handles it (requires fixing the first blocker
  first so there's a single choke point), or (b) scope suppression to a specific start-cycle
  generation number captured at `ClearConversationState()` time and compared, not a bool that
  can silently outlive many intervening cycles.

## Concerns

- [ ] **Notification ID has no per-occurrence entropy — a second `FRESH_LOST_HISTORY` for the
  same session is silently dropped once the first has been read.**
  `onColdRestoreLostHistory`'s `notifID := fmt.Sprintf("cold-restore-lost-history-%s",
  inst.UUID)` (Task 2.3.1b) is a pure function of the stable instance UUID.
  `NotificationHistoryStore.Append()` (`server/notifications/store.go:142-147`) checks
  `existing.ID == record.ID` **first**, unconditionally, and no-ops the append if it matches
  — this check runs *before* the `findUnreadDuplicate`/ADR-003 logic (`store.go:149-165`,
  comment at `store.go:24-27`) whose entire purpose is "if the existing record is already
  read, a new unread record is created instead." Because the ID never changes for a given
  session, a second occurrence — plausible precisely because SessionDriver restart churn is
  this project's own named root cause (`research/features.md:14`) — collides with the first
  record's ID and is dropped regardless of read state, defeating Goal 3's "not just a log
  line" requirement for repeat offenders. The mirrored precedent, `onRateLimitRecovery`
  (`server/services/session_service.go:4001-4029`), avoids this by parameterizing its
  `notifID` with a `sessionID` **function argument** (a per-rate-limit-cycle Claude session
  ID), not a stable instance-level field — the new code diverges from the precedent it
  claims to mirror in exactly the field that matters here.
  — **Recommendation**: include a per-occurrence component (timestamp, or a start-cycle
  counter) in the notification ID.

- [ ] **pitfalls.md design-against-checklist item #7 (corrupt/0-byte JSONL sanity check) is
  silently dropped, unlike item #5.** `prepareColdRestore`'s decision body (Task 1.3.1b) does
  no size/content check on the file `tryExtractConversationUUID`/`DetectByPath` picks, even
  though pitfalls.md (f) explicitly flags this fix as "the first caller to hit this on the
  critical revive path rather than a lazy background correlation loop" and recommends "a
  minimal sanity check... (non-zero size, or at least one parseable JSON line)." Compare to
  the same-directory-collision risk (item #5), which the plan explicitly accepted with a
  Pattern Decisions row and a dedicated regression test (Task 4.1.1f) — item #7 gets neither
  a Pattern Decisions entry nor an Unresolved Question, so it reads as an oversight rather
  than a considered deferral. If Claude CLI's failure text for a corrupt-file `--resume`
  doesn't match `staleResumePattern` ("No conversation found with session ID"),
  `recoverFromStaleResume`'s auto-heal won't fire and the session could get stuck cycling.
  — **Recommendation**: either add the minimal non-zero-size check, or add an explicit
  Pattern Decisions row / Unresolved Question documenting the deferral, matching how item #5
  was handled.

- [ ] **pitfalls.md item (d) — no timeout on the now-earlier `DetectByPath` scan inside the
  actor's serialized command queue — is also dropped without an explicit deferral note.**
  pitfalls.md is explicit that moving the scan earlier "moves this already-existing risk...
  onto the critical path of every revive" for the whole instance's actor loop, and that
  `DetectByPath` takes no `ctx`. No task in Phase 1 adds a timeout or documents why one isn't
  needed. Likely low-impact in practice (small per-project JSONL directories) but pitfalls.md
  flagged it by name and the plan should say explicitly "considered, not needed because X"
  rather than leave it unaddressed.

## Minors

- The deliberate decision not to add a "resumed via low-confidence path-fallback" signal
  (Pattern Decisions row, citing pitfalls.md §c) is reasoned and documented — flagging only
  as a reminder that a same-directory collision (accepted, tested in 4.1.1f) is completely
  invisible to the end user when it actually happens, unlike `FRESH_LOST_HISTORY` which is
  loud by design. Not a plan defect, just an asymmetry worth remembering during review.
- Task 1.5.1's acceptance criterion leans on `grep -c "func (i \*Instance) prepareColdRestore"
  session/instance_claude.go` equals `1` as a proxy for "no duplicated divergent logic" —
  fine as a supplement to the real behavioral assertion in the same task, but weak evidence
  on its own if read in isolation.
- Minor task-numbering drift: "Task 3.1.2a" appears under Story 3.1.1's task list rather than
  a matching "Story 3.1.2" — cosmetic, doesn't affect execution.
