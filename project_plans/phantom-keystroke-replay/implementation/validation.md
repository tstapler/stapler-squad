# Validation Plan: Phantom Repeated "1" Keystroke Fix

**Date**: 2026-07-27
Backlog item: `04089969-0f19-499c-be34-2e8bcfc4f13e`
Requirements: `../requirements.md` | Plan: `implementation/plan.md` | UX: `../design/ux.md` | ADR: `../decisions/ADR-001-startup-dialog-answer-latch.md`

## Happy Path Scenario

Given a fresh session whose `Preview()` shows the trust-folder startup dialog
on the first poll tick, when `runSessionDriverWithPrompt` observes the same
`DialogContentHash` across subsequent poll ticks (the dialog hasn't actually
changed), then `answerDialogOnce` transitions the `DialogAnswerLatch` from
`dialogUnanswered` to `dialogAwaitingDismissal` after exactly one successful
`SendKeys("1\n")`, no further `SendKeys` calls fire for that hash, and once
the dialog actually clears (hash changes, or the latch falls through after
`dialogAwaitingDismissal`) the driver proceeds normally to Ready-detection and
the `NeedsApproval` check — never re-sending the phantom "1" even while the
connection is reconnecting/flapping around it.

## Requirement → Test Mapping

| AC | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1 | `research/phase0-findings.md` | (already complete) | — | Phase 0 go/no-go gate confirmed with a live-executing test before Phase 1/2 began; no new test required here. |
| AC2 | `session/session_driver_test.go` | `TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded` | Integration | `stuckDialogProcessManager` fake returns unchanging trust-dialog text for `driverPollInterval*6 + 500ms` (6 ticks); asserts `sendKeysCount.Load() <= maxDialogAnswerAttempts` — the permanent regression proof replacing Phase 0's obsolete "expect repeated sends" assertion (Task 1.2.1/1.2.2). |
| AC2 | `session/session_driver_test.go` | `TestAnswerDialogOnce_NoResend_WhenHashUnchanged` | Unit | Table-driven case (a): same hash sent twice in a row → second call is a no-op, `sendCallCount` stays 1 (Task 1.2.3a). |
| AC2 | `session/session_driver_test.go` | `TestAnswerDialogOnce_Resends_WhenHashChanges` | Unit | Table-driven case (b): hash changes between calls (genuinely new/different dialog) → second call sends again, proving the latch doesn't swallow a legitimate new dialog (Task 1.2.3b). |
| AC2 | `session/session_driver_test.go` | `TestAnswerDialogOnce_GivesUp_AfterMaxAttempts` | Unit | Table-driven case (c): `send` errors `maxDialogAnswerAttempts` times in a row → status becomes `dialogGaveUp`; a further call with the same hash does not call `send` again (Task 1.2.3c). Error path for AC2. |
| AC2 | `session/session_driver_test.go` | `TestAnswerDialogOnce_ReachesAwaitingDismissal_AfterOneRetry` | Unit | Table-driven case (d): `send` fails once then succeeds → status reaches `dialogAwaitingDismissal`, total 2 calls to `send` (Task 1.2.3d). |
| AC2 | `session/session_driver_test.go` | `TestAnswerDialogOnce_NoResend_WhenOnlyWhitespaceJitterDiffers` | Unit | Table-driven case (e): same logical dialog text across two calls but with incidental whitespace/line-wrap differences (different column-width wrapping, trailing spaces) between them → normalized-before-hash comparison recognizes them as unchanged, `sendCallCount` stays 1 — proves the normalization step (Task 1.1.2) actually defeats the jitter scenario, not just the byte-identical case (Task 1.2.3e, closes adversarial-review.md Blocker 2). |
| AC2 | `session/session_driver_test.go` | `TestSessionDriver_TailSliceBoundsDialogMatchAndHash` | Integration | **Added post-validation** (Task 1.2.3 cases f/g, pre-mortem.md P1 fix): fake `ProcessManager` returns *growing* content each tick (dialog text fixed at the tail, new unrelated lines prepended, simulating real Claude output after the dialog appeared) — asserts `sendKeysCount.Load() <= maxDialogAnswerAttempts` still holds in this ordinary, non-flapping case, not just the byte-identical-forever case Task 1.2.2 covers. |
| AC2 | `session/session_driver_test.go` | `TestSessionDriver_DialogGaveUp_FallsThroughToInactivityEscalation` | Unit/Integration | **Gap closed** (Task 1.2.5, added after this validation pass): control-flow regression for Task 1.1.3's Blocker-1 fix, proving `dialogGaveUp` reaches `handleDriverFailure`/inactivity-timeout escalation rather than just "count stays bounded" — see plan.md's Phase 4 Repair Round item 16 and Task 1.2.5's fallback clause. |
| AC2 | `session/session_driver_test.go` | `TestSessionDriver_StuckApprovalPromptAnswersBoundedNotUnbounded` | Integration | Task 1.1.4's approval-prompt (`NeedsApproval`/`shouldApprovePrompt`) branch, mirroring Task 1.2.2's shape: fake `ProcessManager` + a real/faked `InstanceStatusManager` reporting `detection.StatusNeedsApproval` from static unchanging text; asserts `sendKeysCount.Load() <= maxDialogAnswerAttempts` over 6 ticks (Task 1.2.4, **preferred** path). If the `InstanceStatusManager` scaffolding proves disproportionate, this task is explicitly permitted to fall back to "fixed by inspection/shared-helper coverage" — record which path was taken in the PR description per Task 1.2.4. |
| AC3 | `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts` | `drops buffered messages that were queued before close() is called` | Unit | Push a message with no active iterator pump (lands in `this.queue`), call `close()`, assert return value `=== 1`, then `for await` yields zero further messages (Task 2.1.2). |
| AC3 | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `overlapping connect() only lets the newer generation's state win` | Unit | Two fake async-iterable streams; call `connect()` twice before the first stream's first message resolves; assert `terminalState`/`isConnected` reflect only the newer generation's messages — the epoch/`ConnectionGeneration` guard test (Task 2.2.5). |
| AC3 | `web-app/src/components/sessions/__tests__/useDropEpisodeCoalescer.test.ts` | `useDropEpisodeCoalescer_should_flushSummedCount_When_multipleReportsWithinWindow` | Unit | Fake timers: 3 calls to `report(1)` within 400ms → exactly one `onFlush(3)` call (Task 2.3.3b-a). |
| AC3 | `web-app/src/components/sessions/__tests__/useDropEpisodeCoalescer.test.ts` | `useDropEpisodeCoalescer_should_flushIndependently_When_reportArrivesAfterWindowClosed` | Unit | A `report()` after the window already flushed produces a second, independent `onFlush` call (not merged with the prior episode's count) — matches `design/ux.md` §2.3 Case C "replace, don't merge across episodes" (Task 2.3.3b-b). |
| AC3 | `web-app/src/components/sessions/__tests__/InputDropBadge.test.tsx` | `InputDropBadge_should_renderAlertRole_When_visible` | Unit | Renders with `role="alert"`, `aria-live="assertive"` present (Task 2.3.4). |
| AC3 | `web-app/src/components/sessions/__tests__/InputDropBadge.test.tsx` | `InputDropBadge_should_useSingularText_When_countIsOne` | Unit | Singular vs. plural badge text ("1 keystroke dropped..." vs "N keystrokes dropped...") (Task 2.3.4). |
| AC3 | `web-app/src/components/sessions/__tests__/InputDropBadge.test.tsx` | `InputDropBadge_should_autoDismiss_When_timeoutElapses` | Unit | Auto-dismiss after the configured `DEFAULT_TOAST_MS` timeout, fake timers (Task 2.3.4). |
| AC3 | `web-app/src/components/sessions/__tests__/InputDropBadge.test.tsx` | `InputDropBadge_should_produceDistinctAnnouncementText_When_consecutiveEpisodesHaveSameCount` | Unit | Two consecutive episodes both reporting `count === 1` still produce a distinct/changed underlying text-node on the live region (nonce-suffix dedup fix) — proves the same-count-twice re-announcement bug is actually fixed (Task 2.3.4, closes adversarial-review.md concern 5). |
| AC3 | `web-app/src/components/sessions/__tests__/InputDropBadge.test.tsx` | `InputDropBadge_should_notMoveFocus_When_onInputDroppedFires` | Unit | `document.activeElement` unchanged immediately before/after `onInputDropped`/`report()` fires (Task 2.3.4, closes UX §3.1 non-focus-steal requirement). |
| AC4 (Go) | `server/services/connectrpc_websocket_test.go` | `TestRunInputReadLoopExitsPromptlyOnConnectionClose` | Integration | `createTestWebSocketPair` helper; starts `runInputReadLoop` in a goroutine, writes one input envelope (recorded), closes the client connection, asserts `<-done` fires within `2*time.Second`, and asserts the recorded-input slice length does not grow after close even though the client attempts (and fails) to write again (Task 3.1.2). |
| AC4 (Jest — overlapping-connect) | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `overlapping connect() only lets the newer generation's state win` | Unit | Same test as AC3's epoch-guard row above — this row cross-references it explicitly as satisfying AC4's "overlapping-connect epoch guard test" requirement (Task 2.2.5). |
| AC4 (Jest — queued-message-drop-on-close, isolated unit half) | `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts` | `drops buffered messages that were queued before close() is called` | Unit | Isolated `MessageQueue.close()` unit test, no live reconnect (Task 2.1.2). |
| AC4 (Jest — queued-message-drop-on-close, **interleaving half — gap closed**) | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `a message pushed to the live MessageQueue right as a reconnect closes it is dropped, not delivered to either connection` | Integration | **Gap closed** (Task 2.2.8, added after this validation pass): the actual interleaving scenario AC4 names literally — a message pushed in the same tick a reconnect closes the queue; asserts it's dropped on both generations and `onInputDropped` fires. Task 2.1.2 alone (row above) does not exercise a live reconnect; this test does. See plan.md's Phase 4 Repair Round item 10. |
| AC4 (Jest — triple-rapid-connect) | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `three rapid connect() calls do not throw or leak` | Unit | Three synchronous `connect()` calls in the same tick (StrictMode double-invoke + genuine reconnect); await pending microtasks; assert no unhandled rejection and that only the third `MessageQueue` instance is referenced going forward, with the first two having `.close()` called (Task 2.2.6). |
| AC4 (Jest — disconnect()-vs-connect() race, not literally required by AC4's text but closes an adjacent tested gap) | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `disconnect() racing a concurrent connect() does not tear down the newer generation's queue/controller` | Unit | `connect()` (gen N), then `disconnect()` + `connect()` (gen N+1) back-to-back before gen N's stream resolves; asserts the final `messageQueueRef`/`abortControllerRef` are gen N+1's, not torn down or null (Task 2.2.7). Scoped as documenting current behavior under this interleaving, not a race-freedom guarantee — see plan.md Risk Control table. |
| AC5 | Manual — no automated test | `Task 4.1.1` procedure | Manual | Induce the ticket's specific not-started/paused flap (via pause/resume or `tmux kill-session`) against a fresh session showing the trust dialog; tail `stapler-squad.log`, confirm "SessionDriver: answered startup dialog" appears at most `maxDialogAnswerAttempts` (3) times **and** that those timestamps cluster inside the induced flap window (not just a low total count); confirm no literal repeated `1` chars post-recovery; confirm `InputDropBadge` appeared and announced once if input was dropped; record pass/fail on the backlog item. |
| AC6 | No test — documentation confirmation only | `Task 4.3.1` | N/A | Verification that no Epic 1-3 task introduces cross-tab/multi-connection coordination beyond the single-session generation guard; requirements.md's Non-Goals section already states this explicitly. Not a testable code path — a future multi-tab report must not be treated as a regression per this doc. |

### GAPs flagged

1. **AC2 control-flow starvation fix has no dedicated unit test.** Row
   `TestSessionDriver_ApprovalLatchFallsThrough_WithoutStarvingLoop` above is
   **new, not currently named in plan.md's task list** — Task 1.1.3 and
   ADR-001 both describe the required branching behavior (`dialogUnanswered`
   → `continue`; `dialogAwaitingDismissal`/`dialogGaveUp` → fall through) in
   prose, and Task 1.2.2's integration test only proves the *count* stays
   bounded over 6 ticks, not that the *loop body* (Ready-detection,
   `driverInactivityTimeout` → `handleDriverFailure` escalation,
   `NeedsApproval` check) actually executes once the latch reaches
   `dialogGaveUp`/`dialogAwaitingDismissal`. Without this test, a regression
   that silently restores the old unconditional `continue` would still pass
   `TestSessionDriver_StuckDialogAnswersBoundedNotUnbounded` (send count would
   still be bounded — it just wouldn't fall through), defeating ADR-001's "not
   a dead end" claim without any test catching it. Recommend adding this as a
   Go unit test on `runSessionDriverWithPrompt`'s call-site branching (or, if
   that's awkward to isolate, a table-driven test directly on the `if
   status == dialogUnanswered { continue }` conditional extracted as its own
   tiny helper) before Phase 5 implementation closes out Task 1.1.3.
2. **No cross-layer "input reaches tmux exactly once" test exists.** This is
   an intentional scope decision already documented in plan.md's Acceptance
   Criteria Coverage Summary (AC4 row) and `research/pitfalls.md` §5 — flagged
   here again for visibility, not as a new gap: AC4 is satisfied by two
   independent, layer-scoped suites (client epoch/queue Jest tests + server
   read-loop Go test), not one true end-to-end test driving a keystroke
   through both the WebSocket client and the server read-loop into a real
   tmux sink.

## UX Acceptance Tests

(`design/ux.md` Step 5 — testable UX criteria for `InputDropBadge`.)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UX-AC-1: Badge visible within 500ms of drop (400ms coalescing + ≤100ms mount) | `web-app/src/components/sessions/__tests__/useDropEpisodeCoalescer.test.ts` + `InputDropBadge.test.tsx` | `useDropEpisodeCoalescer_should_flushSummedCount_When_multipleReportsWithinWindow` (timing half) | Jest fake timers | Advance fake timers by 400ms after `report()`, assert `onFlush` fired; combined with `InputDropBadge`'s render-on-flush being synchronous, bounds total to ≤500ms. |
| UX-AC-2: Badge never overlaps the xterm cursor's bounding rect in a ≥600px viewport | `tests/e2e/input-drop-badge.spec.ts` (new) | `input-drop-badge should not overlap terminal cursor` | Playwright | Induce a drop episode in a live session, screenshot/measure both bounding rects, assert no intersection. |
| UX-AC-3: Badge auto-dismisses within 8–8.5s of becoming visible | `InputDropBadge.test.tsx` | `InputDropBadge_should_autoDismiss_When_timeoutElapses` | Jest fake timers | Advance fake timers to `DEFAULT_TOAST_MS` (8000ms) + fade transition, assert badge no longer rendered/visible. |
| UX-AC-4: Singular vs. plural badge text | `InputDropBadge.test.tsx` | `InputDropBadge_should_useSingularText_When_countIsOne` | Jest + RTL | Render with `count=1`, assert "1 keystroke dropped..."; render with `count=3`, assert "3 keystrokes dropped...". |
| UX-AC-5: A second episode while the first is visible replaces (not merges) the count and restarts the hold timer | `InputDropBadge.test.tsx` (new case) | `InputDropBadge_should_replaceCount_When_secondEpisodeArrivesDuringFirstsHold` | Jest fake timers | Fire `onInputDropped(2)`, advance partway through the 8s hold, fire `onInputDropped(1)` again, assert displayed text becomes "1 keystroke..." (not "3") and the dismiss timer restarts from the second episode's flush time. |
| UX-AC-6: Drop announced via `role="alert"`/`aria-live="assertive"` exactly once per coalesced episode | `InputDropBadge.test.tsx` | `InputDropBadge_should_renderAlertRole_When_visible` (existence) + `useDropEpisodeCoalescer.test.ts` (once-per-window) | Jest + RTL | Assert one `onFlush`/announcement per 400ms window regardless of how many individual `report()` calls landed inside it. |
| UX-AC-7: Two consecutive same-count episodes each produce a distinct DOM text mutation | `InputDropBadge.test.tsx` | `InputDropBadge_should_produceDistinctAnnouncementText_When_consecutiveEpisodesHaveSameCount` | Jest + RTL | Fire two episodes both with `count=1` back-to-back (after the first's window fully closes); assert the live region's underlying text differs between the two renders (nonce suffix) even though the human-readable count is identical. |
| UX-AC-8: `LiveRegion` DOM node is never unmounted/remounted across appear→hold→dismiss | `InputDropBadge.test.tsx` (new case) | `InputDropBadge_should_keepLiveRegionMounted_When_badgeFadesOut` | Jest + RTL | Capture the `LiveRegion` element/ref identity at mount, advance through the full 8.5s lifecycle, assert the same DOM node reference persists (only its text content changed). |
| UX-AC-9: Badge is never a focus target | `InputDropBadge.test.tsx` | `InputDropBadge_should_notMoveFocus_When_onInputDroppedFires` | Jest + RTL | Assert `document.activeElement` unchanged immediately before/after `onInputDropped` fires. |
| UX-AC-10: No interactive elements; `pointer-events: none` on root | `InputDropBadge.test.tsx` (new case) | `InputDropBadge_should_haveNoInteractiveElements_When_rendered` | Jest + RTL | Query for `button`/`a`/`[tabindex]` inside the badge root, assert none found; assert computed `pointer-events` style is `none`. |
| UX-AC-11: Badge never requires user action to disappear | `InputDropBadge.test.tsx` | `InputDropBadge_should_autoDismiss_When_timeoutElapses` (same as UX-AC-3) | Jest fake timers | No click/keypress simulated; dismissal happens purely via timer advancement. |
| UX-AC-12: Badge text meets 4.5:1 contrast in both light and dark themes | Manual / axe-core | `axe-core contrast audit — InputDropBadge` | axe-core / Lighthouse + manual computation | Run axe-core against the rendered badge in both themes; cross-check the recorded manual computation (`#92400e` on `#fef3c7` ≈6.8:1 light; `#fbbf24` on `#78350f` ≈5.4:1 dark) in `design/ux.md`. |
| UX-AC-13: Badge not in tab order, no `:focus` styling | `InputDropBadge.test.tsx` (new case) | `InputDropBadge_should_haveNoTabIndex_When_rendered` | Jest + RTL | Assert badge root has no `tabIndex` attribute or `tabIndex="-1"`/absent; no `0`. |
| UX-AC-14: No focus trap — Tab moves focus identically with badge present vs. absent | Manual | `input-drop-badge focus-trap manual check` | Manual / browser accessibility inspector | Press Tab with the badge visible vs. a control run with it absent; verify identical focus destination. |

**Gap closed** (Task 2.3.5, added after this validation pass): plan.md now
adds `tests/e2e/input-drop-badge.spec.ts` (`// +feature: input-drop-badge`
marker, `frontend-features.json` registry entry) as its own task, promoting
UX-AC-2's bounding-rect check into that spec per `.claude/rules/feature-
registry.md`'s e2e-required convention. See plan.md's Phase 4 Repair Round
item 17.

## Test Stack

- **Go unit/integration**: standard `testing` package + `testify`
  (`assert`/`require`), this repo's existing convention in
  `session/session_driver_test.go` and `server/services/connectrpc_websocket_test.go`;
  no `goleak` in this package (per plan.md's Pattern Selection table) — hand-rolled
  channel + `time.After` timeout assertions via the existing
  `createTestWebSocketPair(t)` helper.
- **Jest/RTL (frontend unit)**: Jest with `@testing-library/react` and fake
  timers (`jest.useFakeTimers()`) for all coalescing/auto-dismiss timing
  assertions; fully-mocked-stream style for `useTerminalStream.test.ts` per
  `research/stack.md` §4.
- **E2E / UX**: Playwright (`tests/e2e/`) for the one cross-cutting visual
  check (UX-AC-2 badge/cursor overlap); axe-core/Lighthouse for contrast
  (UX-AC-12); manual checklist (Task 4.1.1, UX-AC-14) for anything requiring a
  live tmux session, a real screen reader, or literal keyboard-focus
  comparison against a control run.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/services/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line on touched files (`session/session_driver.go`, `server/services/connectrpc_websocket.go`) |
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="MessageQueue|useTerminalStream|InputDropBadge|useDropEpisodeCoalescer"` | ≥80% line on touched files (`MessageQueue.ts`, `useTerminalStream.ts`, `InputDropBadge.tsx`, `useDropEpisodeCoalescer.ts`, `LiveRegion.tsx`) |

- No Migration Plan section exists in plan.md (no schema/DB/proto wire-format
  changes) — migration test step correctly omitted.
- All public functions touched by this fix (`answerDialogOnce`,
  `MessageQueue.close`, `useTerminalStream.connect`/`disconnect`,
  `runInputReadLoop`, `useDropEpisodeCoalescer`) have happy-path + error-path
  coverage per the mapping table above.
- Both flagged GAPs (control-flow starvation unit test; missing Playwright
  e2e spec for `InputDropBadge`) should be resolved before Phase 5
  implementation is considered complete, not deferred silently.
