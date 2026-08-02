# Validation Plan: phantom-keystroke-replay

**Date**: 2026-08-02
**Backlog item**: `04089969-0f19-499c-be34-2e8bcfc4f13e`
**Source**: `requirements.md`, `implementation/plan.md` (post-repair), `design/ux.md`

## Happy Path Scenario

Given a `stapler-squad` terminal session whose WebSocket stream is flapping
(connect → "session not started or paused" → reconnect, matching the
ticket's `mbr-skills` repro), when the client reconnects while a directory-
approval dialog is mid-redraw and buffered keystrokes are still queued from
the superseded connection, then the approval prompt is answered at most
once (no phantom `"1"` replay), any input that cannot be safely carried
across the reconnect boundary is dropped rather than replayed or silently
held, and the user sees/hears an `InputDropBadge` + assertive announcement
for that drop instead of experiencing silent input loss.

---

## Requirement → Test Mapping

### AC1 — Duplication mechanism confirmed with direct runtime evidence (Phase 0 gate, already satisfied)

No new test — AC1 is a documentation-only confirmation (Story 1.1.1). The
pre-existing test below is cited as the standing runtime evidence that the
edge-triggered latch mechanism the confirmation describes actually exists in
code, not re-derived here.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1: duplication mechanism confirmed | `session/session_driver_test.go` | `TestShouldApprovePromptOnce/returns true when dialog present and not awaiting clear` (pre-existing) | Unit | Happy path — fresh dialog, latch unarmed → approves once |
| AC1: duplication mechanism confirmed | `session/session_driver_test.go` | `TestShouldApprovePromptOnce/returns false while awaiting clear, no matter how long it's been` (pre-existing) | Unit | Edge case — latch armed, same dialog persists across ticks → no resend |
| AC1: duplication mechanism confirmed | — | — | Integration | N/A — Phase 0 gate is a documentation confirmation against already-merged code (`3546c2b12`, `c0e6c4ce6`); no new integration harness needed. AC2's stateful-sequence test below is the closest thing to an integration check for this mechanism. |

### AC2 — Re-emission fix regression-tested against this backlog item

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC2: single keystroke delivered at most once across reconnect/flap | `session/session_driver_test.go` | `TestApprovalAwaitingClearLatch_PreventsPhantomReplayAcrossReconnectChurn` (Task 1.1.2.1) | Unit | Happy path — 6-tick simulated flap (tick 1 dialog visible+unarmed → approve; ticks 2-5 dialog still visible+armed → no resend; tick 6 dialog gone) → exactly 1 of 6 calls returns `true` |
| AC2: single keystroke delivered at most once across reconnect/flap | `session/session_driver_test.go` | `TestApprovalAwaitingClearLatch_ReapprovesNewDialogAfterPriorOneFullyClears` (new, same file/describe area as Task 1.1.2.1) | Unit | Edge case — latch correctly re-arms after a dialog fully clears (`approvalVisible` goes `false`, resetting `approvalAwaitingClear` to `false`) so a genuinely *new* dialog appearing afterward is still approved once — proves the fix isn't a permanent "approve nothing after the first prompt ever" latch, only a same-dialog-resend guard |
| AC2: single keystroke delivered at most once across reconnect/flap | — | — | Integration | N/A — `TestApprovalAwaitingClearLatch_PreventsPhantomReplayAcrossReconnectChurn` already drives the real multi-tick state-threading rule (`approvalAwaitingClear = approvalAwaitingClear && approvalVisible`) exactly as `session_driver.go`'s poll loop does; no external I/O to mock, so a separate integration test would duplicate this one at higher cost. |

### AC3 — Client-side drop-not-replay + drop-and-signal UI (MessageQueue half, epoch-guard half, UI half)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC3 (MessageQueue): buffered-but-unsent input is dropped, not flushed, on `close()` | `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts` | `'should drop buffered messages when close() is called before they are drained'` (Task 2.1.1.2) | Unit | Happy path — 2 messages pushed, `close()` called, `close()` returns `2`, iterator yields 0 messages |
| AC3 (MessageQueue): buffered-but-unsent input is dropped, not flushed, on `close()` | `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts` | `'should not deliver a message pushed in the same synchronous tick close() runs'` (Task 2.1.1.3) | Unit | Edge case — `push()` landing in the same microtask as `close()` (resolve-callback race) must still be discarded, not delivered |
| AC3 (MessageQueue): buffered-but-unsent input is dropped, not flushed, on `close()` | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `useTerminalStream_should_dropQueuedInput_When_reconnectSupersedesQueueWithPendingMessages` (Task 3.2.1.3) | Integration (Jest `renderHook`, exercises the hook's actual usage of `MessageQueue`, not the queue class in isolation — per pitfalls.md §5) | Attempt A's queue has a pending push; attempt B supersedes it; instance A's `.close()` was called and instance B (not A) is installed |
| AC3 (epoch guard): overlapping reconnects don't let a stale attempt mutate state | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `connect_should_ignoreStaleGenerationMessages_When_secondConnectSupersedesFirstBeforeFirstMessage` (Task 3.2.1.1) | Unit | Happy path — `connect()` called twice with no `await` between; only the second (current-epoch) attempt's `firstMessage` branch is allowed to flip `isConnected` |
| AC3 (epoch guard): `disconnect()` doesn't clobber a newer `connect()` | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `disconnect_should_notClobberNewerConnect_When_reconnectCompletesWhileDisconnectStillAwaiting` (Task 3.2.1.4) | Unit | Edge case — `disconnect()`'s pending `await` resolves after a newer `connect()` has already reached `isConnected === true`; stale continuation must not reset state or corrupt decoders |
| AC3 (signal): a drop is visibly/audibly surfaced, silence only on the true no-drop case | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_renderPillWithDropCount_When_droppedInputEventIsSet` (Task 4.2.3.1) | Unit (RTL) | Happy path — non-null `droppedInputEvent` prop renders visible pill with count text |
| AC3 (signal): a drop is visibly/audibly surfaced, silence only on the true no-drop case | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_renderNull_When_droppedInputEventIsNull` (Task 4.2.3.1) | Unit (RTL) | Edge case — `droppedInputEvent === null` renders nothing in the DOM (not CSS-hidden), asserting the "silent on the normal case" constraint |
| AC3 (signal): coalescing across occurrences lives in one owner | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_announceRunningTotal_When_MultipleDropsCoalesceInSameEpisode` (Task 4.2.3.2) | Integration-ish (RTL, exercises component + `useLiveRegion` + timer interplay together) | 3 successive `droppedInputEvent` prop updates within the dwell window accumulate to a running total (`1` → `3` → `4`) and fire one announcement per content change, not per occurrence |

### AC4 — Regression coverage: Go bounded read-goroutine exit test; Jest overlapping-connect, drop-on-close interleaving, triple-rapid-connect tests

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC4 (Go): bounded read-goroutine exit | `server/services/connectrpc_websocket_test.go` | `TestControlModeReadLoop_BoundedExitOnConnClose/exits within bound when underlying connection closes` (Task 5.1.2.1) | Unit (Go, real loopback `*websocket.Conn` pair, `-race`) | Happy path — `clientConn.Close()` unblocks `ReadMessage()`; `waitWithTimeout(&readWG, 2*time.Second)` returns `true` |
| AC4 (Go): bounded read-goroutine exit | `server/services/connectrpc_websocket_test.go` | `TestControlModeReadLoop_BoundedExitOnConnClose/exits immediately on EndStream envelope without closing the connection` (Task 5.1.2.1) | Unit (Go, real loopback conn) | Edge case — an `EndStreamFlag` envelope (not a connection close) also bounds the read loop's exit; `errChan` receives `nil` |
| AC4 (Go): extraction of `controlModeReadLoop` is a pure logic move, zero behavior regression | `server/services/connectrpc_websocket_test.go` | Full pre-existing suite (27 tests, e.g. `TestStreamViaControlMode*`) (Task 5.1.2.2) | Integration/regression suite | All 27 pre-existing tests pass unchanged after the extraction — proves the refactor didn't alter envelope parse/EndStream/error-classification behavior, only its packaging |
| AC4 (Jest): triple-rapid-connect must not throw | `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | `connect_should_notThrow_When_calledThreeTimesInRapidSuccession` (Task 3.2.1.2) | Unit | 3 synchronous `connect()` calls (A, B, C); no exception/unhandled rejection; only C's `firstMessage` flips `isConnected`; A and B were both `.close()`-d |
| AC4 (Jest): overlapping-connect epoch guard | *(same test as AC3 row above — cross-covers both)* | `connect_should_ignoreStaleGenerationMessages_When_secondConnectSupersedesFirstBeforeFirstMessage` | Unit | See AC3 row |
| AC4 (Jest): queued-message-drop-on-close interleaving | *(same test as AC3 row above — cross-covers both)* | `useTerminalStream_should_dropQueuedInput_When_reconnectSupersedesQueueWithPendingMessages` | Integration | See AC3 row |

### AC5 — Manual repro of the ticket's exact flapping condition, recorded against the backlog item

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC5: manual repro no longer reproduces phantom keystrokes | N/A — executed live via MCP tools (Task 6.1.1.1) | "5-10 cycle pause/resume marker repro" | Manual | Happy path — for each of 5-10 rapid `pause_session`/`resume_session` cycles, a distinguishable marker string typed immediately before/during the pause; `read_session_output` confirms each marker appears **exactly once**, never duplicated/replayed |
| AC5: manual repro no longer reproduces phantom keystrokes | N/A — same manual session | "mid-flight pause shows drop signal, not silent loss" | Manual | Edge case — for any cycle where `pause_session` lands while a marker is still mid-flight (not yet acknowledged), the browser shows `InputDropBadge` + assertive announcement for that cycle instead of silently losing or replaying the keystrokes |
| AC5: manual repro no longer reproduces phantom keystrokes | — | — | Integration | N/A — AC5 is itself the end-to-end/integration-level confirmation that Phases 1-5 closed the gap; no separate automated integration test is meaningful here (it requires a live tmux-backed session and real `session_not_started_or_paused` timing, which is exactly what Constraints in requirements.md rules out simulating generically). |

### AC6 — Multi-tab concurrent input documented as out of scope

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC6: multi-tab concurrent input remains out of scope | N/A — static check (Task 7.1.1.1) | `grep -rn "BroadcastChannel\|localStorage" web-app/src/lib/hooks/useTerminalStream.ts web-app/src/lib/terminal/MessageQueue.ts` | Static/grep check | Happy path — zero matches confirms this diff introduces no cross-tab shared state, so multi-tab behavior stays implicitly (and correctly) out of scope |
| AC6: multi-tab concurrent input remains out of scope | — | — | Edge case / Integration | N/A — no code change is made for this AC; it is satisfied entirely by `requirements.md`'s Non-Goals section, unchanged by this session's diff. |

---

## UX Acceptance Tests

`design/ux.md` Step 3 defines **15** UX acceptance criteria across four
groups (Visual ×5, Screen-reader ×4, No dead ends ×3, Keyboard/focus ×3).
One test is designed per criterion below. Most are Jest/RTL
(`InputDropBadge.test.tsx`) since the component's behavior is fully
observable through DOM assertions and fake timers; a few (grayscale/AT
cross-browser behavior) are marked Manual per plan.md's Unresolved
Question 3, which explicitly defers full assistive-technology verification
to a manual follow-up rather than automated CI.

### Visual

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-VIS-1 (placement — inside terminal chrome, never a page-level toast/modal) | `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` | `TerminalOutput_should_renderInputDropBadge_InsideTerminalChrome_When_droppedInputEventSet` | Jest/RTL | Mock `useTerminalStream` to return `droppedInputEvent={count:1, at:...}`; render `TerminalOutput`; assert the badge element is a DOM descendant of `styles.terminal`'s container, not `document.body`-portaled and not inside any `role="dialog"` ancestor |
| AC-VIS-2 (no color-only signal — icon + text both present; grayscale-legible) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_pairAriaHiddenIconWithVisibleText_When_droppedInputEventIsSet` | Jest/RTL + Manual | Jest: assert an `aria-hidden="true"` icon element AND a visible text node both render. Manual supplement: toggle OS-level grayscale/high-contrast mode with a live drop showing; confirm icon+text remain legible and distinguishable from terminal chrome |
| AC-VIS-3 (auto-dismiss within ~4s, no manual dismiss required) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_autoDismissWithinFourSeconds_When_NoFurtherDropsOccur` | Jest (fake timers) | Render with `droppedInputEvent` set; advance fake timers past 4000ms with no further prop updates; assert the badge is no longer in the DOM |
| AC-VIS-4 (no stacking — exactly one instance visible at a time) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_renderExactlyOneInstance_When_ThreeDropsOccurInQuickSuccession` | Jest (fake timers) | Re-render 3 times with incrementing `droppedInputEvent` within the dwell window; after each update, assert the DOM contains exactly one pill/alert element, never two+ |
| AC-VIS-5 (silent default — badge never in DOM with no dropped input) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_renderNull_When_droppedInputEventIsNull` | Jest/RTL | Render with `droppedInputEvent={null}` (initial/clean-session state); assert `container.firstChild === null` |

### Screen-reader

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-SR-1 (assertive, not polite — `role="alert"` + `aria-live="assertive"` + `aria-atomic="true"` together) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_renderRoleAlertWithAssertiveAtomicLiveRegion_When_droppedInputEventIsSet` | Jest/RTL | Render with `droppedInputEvent` set; query the rendered `LiveRegion` element; assert `role="alert"`, `aria-live="assertive"`, and `aria-atomic="true"` are all present on the same element |
| AC-SR-2 (coalesced wording — running total, singular/plural correct, never a stale count) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_announceAccumulatedRunningTotal_When_DropsCoalesceInSameEpisode` | Jest | `count:1` at `t0` → announcement contains "1 keystroke" (singular); re-render `count:2` at `t0+800` → announcement contains "3 keystrokes" (plural, accumulated — not "2"); re-render `count:1` again → announcement contains "4 keystrokes" |
| AC-SR-3 (no spam — announcement count bounded by content changes, not by dropped bytes) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_fireOneAnnouncementPerContentChange_When_NRapidDropsOccurWithinDwellWindow` | Jest | Fire N re-renders with distinct `droppedInputEvent.at` values inside the dwell window; assert the `announce` spy was called exactly N times — not fewer (nothing silently dropped) and not more (no per-byte spam) |
| AC-SR-4 (no duplicate "all clear" — `InputDropBadge` never announces reconnection success itself) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_notAnnounce_When_droppedInputEventTransitionsBackToNull` | Jest | Render with `droppedInputEvent` set (drop episode active), then re-render with `droppedInputEvent={null}` (simulating a clean reconnect after the episode); assert no additional `announce()` call fires from this transition — `ConnectionIndicator`'s pre-existing "Connection restored" announcement is out of this component's scope entirely |

### No dead ends

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-RESOLVE-1 (every badge reaches auto-dismiss or superseded-in-place, never stuck with no active timer) | — (composite of AC-VIS-3 + AC-SR-2 behavioral evidence) + code review | `InputDropBadge — dwell-timer invariant review` | Jest (existing tests) + Manual code review | No single test proves this exhaustively; AC-VIS-3's auto-dismiss test and AC-SR-2's coalesce-and-reset test together demonstrate both reachable end-states work; supplement with a manual review confirming every code path that sets "visible" state also (re)starts the dwell timer ref (no branch sets visible without touching the timer) |
| AC-RESOLVE-2 (unmount safety — no `setState` on unmounted component, no orphaned timer) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_clearPendingTimer_When_UnmountedBeforeDwellTimerFires` (Task 4.2.3.1) | Jest (fake timers + `console.error` spy) | Render with `droppedInputEvent` set (dwell timer pending); call `unmount()`; advance fake timers past 4000ms; assert no React "state update on an unmounted component" warning was logged |
| AC-RESOLVE-3 (no state leakage across remounts — fresh mount starts clean) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_resetRunningTotal_When_RemountedForSameSession` | Jest | Render, accumulate the running total to `3` via two coalesced drops; `unmount()`; mount a fresh instance with `droppedInputEvent={count:1, at:newTimestamp}`; assert the displayed/announced count is `1`, not `4` |

### Keyboard / focus

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-KBD-1 (no focus theft on appear) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_notMoveFocus_When_BadgeAppearsWhileTerminalFocused` (Task 4.2.3.1) | Jest/RTL | Focus a sibling input element; render `InputDropBadge` with `droppedInputEvent` set; assert `document.activeElement` is unchanged immediately before vs. after the render |
| AC-KBD-2 (not in tab order — no implicit or explicit focusability) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_haveNoFocusableTabIndex_When_Rendered` | Jest/RTL | Render with `droppedInputEvent` set; query the pill root element; assert it has no `tabIndex="0"` attribute and is not a naturally-focusable element (not a `<button>`/`<a>`) |
| AC-KBD-3 (no focus theft on update or auto-dismiss) | `web-app/src/components/sessions/InputDropBadge.test.tsx` | `InputDropBadge_should_notMoveFocusOrScroll_When_CountUpdatesOrAutoDismisses` | Jest (fake timers) | Focus a sibling input; render badge, re-render with an incremented count (Surface C) — assert `document.activeElement` unchanged; advance timers to trigger auto-dismiss (Surface B) — assert `document.activeElement` still unchanged and no `window.scrollTo`/`scrollIntoView` call occurred |

---

## Test Stack

- **Go unit / integration**: standard library `testing`, run with `-race`. `session/session_driver_test.go` (AC1/AC2, pure-function table-driven style already established by `TestShouldApprovePromptOnce`). `server/services/connectrpc_websocket_test.go` (AC4, real loopback `*websocket.Conn` pairs via `createTestWebSocketPair(t)` — no mocked transport).
- **Jest unit / integration (TypeScript)**: `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts` (AC3 MessageQueue half), `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` (AC3/AC4 epoch-guard half, via `renderHook` + a module-level `MessageQueue` mock tracking `close()` calls per instance), `web-app/src/components/sessions/InputDropBadge.test.tsx` (AC3 signal half + all 15 UX acceptance tests, via React Testing Library + fake timers), `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` (AC-VIS-1 placement).
- **Manual / human-verifiable**: AC5's live pause/resume repro (via `mcp__stapler-squad__pause_session`/`resume_session`/`read_session_output` against a manually-run instance per `CLAUDE.md`'s "Manual/interactive testing" section — never `make install-service`); AC-VIS-2's grayscale/high-contrast supplement; AC-RESOLVE-1's dwell-timer invariant code review. No new Playwright e2e specs are added by this project — all automatable UX behavior is covered at the Jest/RTL level, consistent with plan.md's Unresolved Question 3 deferring full cross-browser/AT verification to a follow-up.
- **Static check**: `grep -rn "BroadcastChannel\|localStorage" ...` for AC6.

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/services/... -race -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line on touched files (`session_driver.go`, `connectrpc_websocket.go`) |
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="MessageQueue.test\|useTerminalStream.test\|InputDropBadge.test\|TerminalOutput.reconnect.test" --no-coverage=false` | ≥80% line on `MessageQueue.ts`, `useTerminalStream.ts`, `useTerminalFlowControl.ts`, `InputDropBadge.tsx` |

- All public/exported functions touched by this plan (`MessageQueue.close()`, `recordDrop`, `controlModeReadLoop`, `shouldApprovePromptOnce`'s stateful sequence): happy path + error/edge path covered.
- All three silent-drop code paths named in requirements.md's Remaining confirmed gap (`MessageQueue.close()`, `useTerminalStream.connect()`'s epoch-guarded queue swap, `useTerminalFlowControl.sendInput`'s early return) have a test asserting the drop is both non-replayed and signaled.
- Every one of the 15 UX acceptance criteria in `design/ux.md` has a corresponding test or explicit manual step above — no criterion is left unmapped.
- Migration Plan: **N/A** — no schema, proto, or persisted-data changes in this plan (see plan.md's "Migration Plan" section); no `migration_should_be_reversible` test is applicable.
