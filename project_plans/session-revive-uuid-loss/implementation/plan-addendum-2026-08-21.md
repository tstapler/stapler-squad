# Plan addendum (2026-08-21 re-triage)

`plan.md` (2026-08-06) predates commit `e156a3f9d` / PR #439 (2026-08-11), which already
shipped Phase 1 of that plan (recovery-before-decision ordering) under different names.
See `requirements.md`'s "Status update" section for the evidence trail. This addendum
does not rewrite `plan.md` — it narrows which of its phases are still live work.

## Phase 1 (Epics 1.1–1.5): SUPERSEDED — do not implement

`prepareColdRestore`/`coldRestoreOutcome`/`RecoverySuppressed` are functionally
equivalent to the already-shipped `recoverConversationBeforeLaunch()`
(`session/instance.go:939`) + `conversationClearedAt` guard
(`session/instance_claude.go:293,354-359`). Implementing Phase 1 as written would
reintroduce a second, parallel recovery-ordering mechanism next to the shipped one —
exactly the "divergent duplicate logic" AC4 warns against. Skip it.

**Two fields Phase 1 still needs to introduce**, now as additions to the shipped
mechanism rather than to a new `prepareColdRestore` helper:

- `EverHadConversationHistory bool` — set `true` in `SetHistoryInfo`
  (`instance_claude.go:464`, per Task 1.2.1b) and in `tryExtractConversationUUID`'s
  direct-mutation path (`instance_claude.go:377-386`, per Task 1.2.1c). Reset `false` in
  `ClearConversationState()` (`instance_claude.go:278-297`, per Task 1.2.1a — alongside
  the existing `conversationClearedAt = time.Now()` write, no new suppression flag
  needed since `conversationClearedAt` already serves that role).
- `LastReviveOutcome ReviveOutcome` — set at the two existing decision points in
  `startLocked` (`instance.go:997-1004`) and `start` (mirror, ~line 1193) immediately
  after the `HasClaudeSession()` check that's already there, not inside a new helper.

## Phase 2 (Durable revive-outcome signal) and Phase 3 (Frontend surface): STILL LIVE

Unchanged from `plan.md` — Epics 2.1–2.3 (proto enum, lifecycle event reason,
`onColdRestoreLostHistory` notification listener) and Epic 3.1 (`RevivedContextBadge`).
These are the actual remaining implementation work for this backlog item.

## Phase 4 (Tests): partially superseded

- Story 4.1.1 (`TestPrepareColdRestore_*`), Story 4.1.2 (`TestColdRestore_
  RecoversUUIDBeforeLaunch_*`), Story 4.1.3 — **superseded**. The ordering behavior they
  target already has coverage from #439 (`TestColdRestore_WithoutUUID_RecoversFromJSONL`,
  `TestKillSessionThenStart_DoesNotRebuildLaunchCommand`,
  `TestTryExtractConversationUUID_ClearedAtGuard`). Re-verify these still pass; do not
  duplicate them.
- New tests still needed, retargeted at the shipped call sites instead of the moot
  `prepareColdRestore` helper:
  - `TestColdRestore_SignalsFreshLostHistory_When_RecoveryFailsButEverHadHistory` —
    asserts `LastReviveOutcome == FRESH_LOST_HISTORY` and the notification fires when
    `DetectByPath` returns nothing but `EverHadConversationHistory` was true.
  - `TestColdRestore_NoSignal_When_GenuinelyNeverHadHistory` — asserts
    `FRESH_EXPECTED`/no notification for first-time sessions and legitimate
    never-had-history fresh starts.
- Story 4.2.1 (`RevivedContextBadge` frontend tests) — unchanged, still needed.

## Revised task list

See the JSON `tasks` array in the triage output for the trimmed ~9-task breakdown
(Phase 2 + Phase 3 + the two fields above + the two retargeted tests), replacing
`plan.md`'s original ~35-task, 4-phase breakdown.
