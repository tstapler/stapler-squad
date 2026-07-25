# Validation Plan: bulk-select-ux

**Date**: 2026-06-23

---

## Happy Path Scenario

Given a session list in row mode displaying 5 running sessions with no selection active, when the user hovers the first row (revealing the ghost checkbox), clicks it to enter select mode, Shift+clicks the third row to extend the range, then clicks "Delete 3" in the BulkActions toolbar, then the 3 sessions disappear from the list immediately, an undo toast appears within 200ms reading "Deleted 3 sessions" with a visible Undo button, and no `DeleteSession` RPC has been called.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Wire `selectMode`, `isSelected`, `onToggleSelect` into `SessionRow` | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `SessionRow_should_renderCheckboxWithTabIndexNegativeOne_When_selectModeIsFalse` | Unit (happy) | Renders checkbox cell hidden with `tabIndex={-1}` when `selectMode=false` |
| Wire `selectMode`, `isSelected`, `onToggleSelect` into `SessionRow` | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `SessionRow_should_renderCheckedCheckbox_When_selectModeAndIsSelectedAreTrue` | Unit (happy) | `checked` attribute is true when `isSelected=true, selectMode=true` |
| Wire `selectMode`, `isSelected`, `onToggleSelect` into `SessionRow` | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `SessionRow_should_notCallOnToggleSelect_When_selectModeIsFalseAndCheckboxCellClicked` | Unit (error) | Clicking hidden checkbox cell (pointer-events: none) does not fire `onToggleSelect` |
| Wire `selectMode`, `isSelected`, `onToggleSelect` into `SessionRow` | `tests/e2e/bulk-select.spec.ts` | `row mode — first checkbox click enters select mode and selects session` | E2E | Click first row checkbox, verify `selectMode` active via BulkActions toolbar visible |
| Hover-reveal checkbox affordance on `SessionRow` | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `SessionRow_should_applyCheckboxCellClass_When_rendered` | Unit (happy) | Row DOM has element with `checkboxCell` class, visibility controlled by CSS |
| Hover-reveal checkbox affordance on `SessionRow` | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `SessionRow_should_haveAriaHiddenTrue_When_selectModeIsFalse` | Unit (error) | Checkbox wrapper has `aria-hidden="true"` when `selectMode=false` |
| `buildRowGridTemplate` checkbox column support | `web-app/src/components/sessions/__tests__/session-columns.test.ts` | `buildRowGridTemplate_should_returnOriginalString_When_calledWithoutReserveCheckboxOption` | Unit (happy) | Existing callers with no second arg return unchanged output |
| `buildRowGridTemplate` checkbox column support | `web-app/src/components/sessions/__tests__/session-columns.test.ts` | `buildRowGridTemplate_should_prependTwentyFourPxColumn_When_reserveCheckboxIsTrue` | Unit (happy) | Output starts with `"24px 8px"` when `{ reserveCheckbox: true }` passed |
| `buildRowGridTemplate` checkbox column support | `web-app/src/components/sessions/__tests__/session-columns.test.ts` | `buildRowGridTemplate_should_notPrependCheckboxColumn_When_reserveCheckboxIsFalse` | Unit (error) | Output does NOT start with `"24px"` when option omitted or false |
| Shift+click range select | `web-app/src/lib/utils/__tests__/rangeSelect.test.ts` | `computeRangeIds_should_returnAllSessionsBetweenAnchorAndTarget_When_validRange` | Unit (happy) | Returns all session IDs between anchor and target in `flatItems` order, skipping headers |
| Shift+click range select | `web-app/src/lib/utils/__tests__/rangeSelect.test.ts` | `computeRangeIds_should_returnTargetIdOnly_When_anchorIsNotInFlatItems` | Unit (error) | Falls back to `[targetId]` when anchor ID not found (filtered out) |
| Shift+click range select | `web-app/src/lib/utils/__tests__/rangeSelect.test.ts` | `computeRangeIds_should_excludeGroupHeaders_When_rangeSpansGroupBoundary` | Unit (happy) | Group header `FlatItem`s are excluded from the returned IDs |
| Shift+click range select | `web-app/src/lib/utils/__tests__/rangeSelect.test.ts` | `computeRangeIds_should_contractRange_When_secondShiftClickIsCloserThanFirst` | Unit (happy) | Range contracts when second Shift+click target is between anchor and first Shift+click target |
| Shift+click range select wired into `handleToggleSession` | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleToggleSession_should_replaceSelectionWithRange_When_shiftKeyHeldAndAnchorExists` | Unit (happy) | `selectedSessions` becomes exactly the computed range Set |
| Shift+click range select wired into `handleToggleSession` | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleToggleSession_should_fallbackToSingleSelect_When_shiftKeyHeldAndAnchorFilteredOut` | Unit (error) | Anchor filtered out: only target ID added to selection, no error thrown |
| Shift+click range select | `tests/e2e/bulk-select.spec.ts` | `shift+click range select — plain click row 1, shift+click row 3, rows 1-3 are selected` | E2E | Plain click row 1, Shift+click row 3, all 3 checkboxes are checked |
| Cmd/Ctrl+A selects all visible sessions | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleSelectAll_should_addAllFilteredSessionIds_When_cmdAPressed` | Unit (happy) | All `filteredSessions` IDs present in `selectedSessions` after Cmd+A |
| Cmd/Ctrl+A selects all visible sessions | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleSelectAll_should_notFire_When_focusIsInsideTextInput` | Unit (error) | Keyboard guard: `inInput` true prevents Cmd+A from selecting sessions |
| Escape exits select mode | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleClearSelection_should_clearSelectedSessionsAndExitSelectMode_When_escapePressedWithNoModalOpen` | Unit (happy) | `selectMode=false`, `selectedSessions` empty after Escape |
| Escape exits select mode | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleClearSelection_should_notFire_When_modalIsOpenAndStopPropagationCalled` | Unit (error) | `stopPropagation()` in modal prevents SessionList Escape handler from running |
| Escape exits select mode | `tests/e2e/bulk-select.spec.ts` | `escape exits select mode — enter select mode, press Escape, checkboxes hidden and toolbar gone` | E2E | Enter select mode, press Escape, BulkActions toolbar gone |
| Undo toast for bulk delete | `web-app/src/components/ui/__tests__/NotificationToast.test.tsx` | `NotificationToast_should_renderUndoButton_When_notificationTypeIsUndoAndOnUndoProvided` | Unit (happy) | "Undo" button is visible in the toast when `notificationType: "undo"` |
| Undo toast for bulk delete | `web-app/src/components/ui/__tests__/NotificationToast.test.tsx` | `NotificationToast_should_callOnUndoAndDismissToast_When_undoButtonClicked` | Unit (happy) | `onUndo()` called and `removeNotification` called on Undo click |
| Undo toast for bulk delete | `web-app/src/components/ui/__tests__/NotificationToast.test.tsx` | `NotificationToast_should_notRenderUndoButton_When_notificationTypeIsNotUndo` | Unit (error) | No "Undo" button rendered for `notificationType: "info"` |
| Pending-delete pattern in `SessionList` | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleConfirmBulkDelete_should_removeSessionsFromDisplayImmediately_When_deleteTriggered` | Unit (happy) | Sessions in `pendingDeleteIds` are excluded from `filteredSessions` memo before any RPC |
| Pending-delete pattern in `SessionList` | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleConfirmBulkDelete_should_notCallDeleteSessionRpc_When_undoWindowIsActive` | Unit (happy) | Zero RPC calls in the first 5 seconds after a bulk delete |
| Pending-delete pattern in `SessionList` | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `flushPendingDeletes_should_callDeleteSessionRpcForEachPendingId_When_timerExpires` | Unit (happy) | `deleteSessionRpc` called once per session ID after 5s timer fires |
| Pending-delete pattern in `SessionList` | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `flushPendingDeletes_should_immediatelyFlushPreviousPending_When_newBulkDeleteTriggered` | Unit (error) | Second bulk delete flushes first pending batch (RPCs fire), then starts new undo window |
| Undo restore — no RPC | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `undoFn_should_clearPendingDeleteIds_When_undoClickedBeforeTimerExpires` | Unit (happy) | `pendingDeleteIds` becomes empty, sessions reappear, `deleteSessionRpc` never called |
| Undo restore — no RPC | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `undoFn_should_beNoOp_When_pendingDeleteRefIsNull` | Unit (error) | Calling undo when `pendingDeleteRef.current` is null does not throw |
| Undo restore | `tests/e2e/bulk-select.spec.ts` | `undo restores deleted sessions — delete 2 sessions, click Undo in toast, sessions reappear` | E2E | Sessions reappear in list after Undo click, no network RPC to `DeleteSession` |
| Unmount flush (`useEffect` cleanup) | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `SessionList_should_flushPendingDeletes_When_componentUnmounts` | Unit (happy) | Unmounting with active pending deletes fires `deleteSessionRpc` for all IDs |
| Unmount flush (`useEffect` cleanup) | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `SessionList_should_notThrow_When_unmountedWithNoPendingDeletes` | Unit (error) | Unmounting with `pendingDeleteRef.current === null` is a no-op |
| `activeSelection` derived intersection | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `activeSelection_should_reflectOnlyFilteredSessions_When_filterReducesVisibleSet` | Unit (happy) | 5 selected, filter hides 2 → `activeSelection.size === 3` |
| `activeSelection` derived intersection | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `activeSelection_should_excludePendingDeleteIds_When_sessionsAreOptimisticallyRemoved` | Unit (happy) | Sessions in `pendingDeleteIds` excluded from `activeSelection` |
| `activeSelection` derived intersection | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `handleDeleteSelected_should_onlyTargetActiveSelectionIds_When_filterHidesSomeSelected` | Unit (error) | Delete operates on intersection, not full `selectedSessions` |
| ARIA attributes | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `SessionRow_should_setAriaSelectedTrue_When_isSelectedIsTrue` | Unit (happy) | `aria-selected="true"` on row root element when `isSelected=true` |
| ARIA attributes | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `SessionRow_should_setAriaSelectedFalse_When_isSelectedIsFalse` | Unit (happy) | `aria-selected="false"` on row root element when `isSelected=false` |
| ARIA attributes on list container | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `SessionList_should_setDataSelectModeTrue_When_selectModeIsActive` | Unit (happy) | `data-select-mode="true"` on the virtualizer scroll container |
| ARIA live region in BulkActions | `web-app/src/components/sessions/__tests__/BulkActions.test.tsx` | `BulkActions_should_haveAriaLivePolite_When_countSpanRendered` | Unit (happy) | Count span has `aria-live="polite"` and `aria-atomic="true"` |
| Keyboard hint labels in BulkActions | `web-app/src/components/sessions/__tests__/BulkActions.test.tsx` | `BulkActions_should_showCmdAHint_When_onMacOS` | Unit (happy) | Select All button label contains `(⌘A)` on macOS platform |
| Keyboard hint labels in BulkActions | `web-app/src/components/sessions/__tests__/BulkActions.test.tsx` | `BulkActions_should_showCtrlAHint_When_onNonMacOS` | Unit (happy) | Select All button label contains `(Ctrl+A)` on non-macOS platform |
| Row mode bulk delete E2E | `tests/e2e/bulk-select.spec.ts` | `bulk-delete in row mode — selects 2 sessions, clicks Delete, undo toast appears, sessions removed from list` | E2E | 2 sessions selected, Delete clicked, both absent from list, undo toast visible |
| Row mode bulk pause E2E | `tests/e2e/bulk-select.spec.ts` | `bulk-pause in row mode — selects 2 active sessions, clicks Pause Selected, sessions show paused status` | E2E | 2 running sessions selected, Pause clicked, both show paused status label |

---

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-01: Hover-reveal entry into select mode without "Select" button | `tests/e2e/bulk-select.spec.ts` | `AC-01 — hover row reveals checkbox; clicking it enters select mode without pressing Select button` | Playwright | Hover session row → verify checkbox visible; click checkbox → verify BulkActions toolbar visible |
| AC-02: Card view checkbox enters select mode identically | `tests/e2e/bulk-select.spec.ts` | `AC-02 — card mode checkbox click enters select mode` | Playwright | Switch to card view → hover card → click checkbox → verify select mode active and card has selected background |
| AC-03: Escape clears selection and exits select mode | `tests/e2e/bulk-select.spec.ts` | `AC-03 — Escape with no modal open clears selection and hides toolbar` | Playwright | Enter select mode → select 2 sessions → press Escape → verify toolbar gone and no sessions selected |
| AC-04: Escape with modal open closes modal only | `tests/e2e/bulk-select.spec.ts` | `AC-04 — Escape with modal open closes modal only; select mode and selection unchanged` | Playwright | Enter select mode → open a modal → press Escape → verify modal gone, toolbar still visible, selection unchanged |
| AC-05: Mobile browse mode shows no checkboxes; Select button is entry point | `tests/e2e/bulk-select.spec.ts` | `AC-05 — mobile browse mode has no visible checkboxes; Select button enters select mode` | Playwright | Set viewport 375×812 → verify no checkbox visible → tap Select button → verify checkboxes visible |
| AC-06: Mobile select mode — all checkboxes persistently visible | `tests/e2e/bulk-select.spec.ts` | `AC-06 — mobile select mode shows all checkboxes without hover` | Playwright | Set viewport 375×812 → tap Select → verify all row checkboxes visible without hover gesture |
| AC-07: 10 sessions selectable in 4 or fewer interactions | `tests/e2e/bulk-select.spec.ts` | `AC-07 — select 10 sessions and delete in 4 interactions` | Playwright | Click row 1 checkbox (1), Shift+click row 10 (2), click Delete (3), verify 10 sessions removed from list |
| AC-08: Shift+click selects inclusive range; anchor does not reset | `tests/e2e/bulk-select.spec.ts` | `AC-08 — Shift+click selects contiguous range inclusive of anchor; anchor unchanged` | Playwright | Click row 1, Shift+click row 3 → rows 1–3 checked; verify `lastAnchorId` still row 1 (another Shift+click at row 2 gives rows 1–2) |
| AC-09: Second Shift+click contracts range | `tests/e2e/bulk-select.spec.ts` | `AC-09 — second Shift+click with same anchor contracts selection` | Playwright | Click row 1, Shift+click row 4 (rows 1–4 selected), Shift+click row 2 → only rows 1–2 selected; rows 3–4 deselected |
| AC-10: Plain click resets anchor; previous range replaced | `tests/e2e/bulk-select.spec.ts` | `AC-10 — plain click resets anchor and replaces any previous Shift-range` | Playwright | Shift+click range rows 1–3, then plain click row 5 → only row 5 selected, anchor now row 5 |
| AC-11: Shift+click across group boundaries selects flat range | `tests/e2e/bulk-select.spec.ts` | `AC-11 — Shift+click across group headers selects all sessions skipping headers` | Playwright | With group view enabled, click session in group A, Shift+click session in group B → all sessions between endpoints selected, group headers not in selected count |
| AC-12: Cmd+A selects all filtered sessions; does not fire inside text input | `tests/e2e/bulk-select.spec.ts` | `AC-12 — Cmd+A selects all filtered sessions; guarded inside text input` | Playwright | Focus list area, press Cmd+A → all sessions selected; then focus a search input, press Cmd+A → selection unchanged |
| AC-13: Master checkbox indeterminate state and click-to-select-all | `tests/e2e/bulk-select.spec.ts` | `AC-13 — master checkbox shows indeterminate when some selected; click selects all` | Playwright | Select 2 of 5 sessions → verify master checkbox `indeterminate` DOM property is true → click master checkbox → all 5 selected |
| AC-14: X key on focused row toggles selection | `tests/e2e/bulk-select.spec.ts` | `AC-14 — X key on keyboard-focused row toggles selection and enters select mode` | Playwright | Tab to session row, press X key → session selected, BulkActions visible; press X again → session deselected |
| AC-15: Selected row has distinct background tint | `tests/e2e/bulk-select.spec.ts` | `AC-15 — selected row background is distinct from idle, active, and hover states` | Playwright | Select a row → verify computed background-color matches `rgba(99,102,241,0.08)`; verify active session row has different computed background |
| AC-16: Active and selected sessions use categorically different indicators | `tests/e2e/bulk-select.spec.ts` | `AC-16 — active session shows left border not background tint; selected shows background tint not border` | Playwright | Open a session (makes it active), then select a different session → active has `border-left` style, non-active selected has background tint; select the active session too → verify both indicators present simultaneously |
| AC-17: BulkActions count = activeSelection not selectedSessions | `tests/e2e/bulk-select.spec.ts` | `AC-17 — selection count reflects filtered intersection after filter applied` | Playwright | Select 5 sessions, apply filter that hides 2 → toolbar shows "3 sessions selected", not "5 sessions selected" |
| AC-18: Selection count announced via aria-live | `web-app/src/components/sessions/__tests__/BulkActions.test.tsx` | `AC-18 — selection count span has aria-live polite for screen reader announcements` | Jest/RTL | Render BulkActions → query count element → assert `aria-live="polite"` and `aria-atomic="true"` attributes present |
| AC-19: Checkbox not reachable via Tab when selectMode is false | `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | `AC-19 — checkbox tabIndex is -1 when selectMode is false` | Jest/RTL | Render `SessionRow` with `selectMode=false` → query checkbox input → assert `tabIndex={-1}` |
| AC-20: No layout shift when entering select mode | `tests/e2e/bulk-select.spec.ts` | `AC-20 — entering select mode causes no horizontal layout shift in session list` | Playwright | Record row grid width before and after clicking first checkbox → assert grid width unchanged (24px column was always reserved) |
| AC-21: Deleted sessions disappear within one render cycle | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `AC-21 — sessions excluded from filteredSessions immediately after bulk delete triggered` | Jest/RTL | Trigger bulk delete → in the same synchronous tick, verify sessions absent from rendered list; assert no async timer needed |
| AC-22: Undo toast appears within 200ms | `tests/e2e/bulk-select.spec.ts` | `AC-22 — undo toast visible within 200ms of bulk delete action` | Playwright | Click Delete → within 200ms (use `toBeVisible` with timeout 200) → toast with `data-testid="undo-toast-button"` visible |
| AC-23: Undo restores sessions with no DeleteSession RPC | `tests/e2e/bulk-select.spec.ts` | `AC-23 — Undo within window restores sessions and no DeleteSession RPC is called` | Playwright | Delete 3 sessions → intercept network → click Undo → verify sessions reappear → verify zero `DeleteSession` requests in intercepted network |
| AC-24: Timer expiry fires DeleteSession RPCs | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `AC-24 — DeleteSession RPC called for each pending session after 5s timer expires` | Jest/RTL (fake timers) | Trigger bulk delete → `jest.advanceTimersByTime(5000)` → assert `deleteSessionRpc` call count equals pending session count |
| AC-25: Only one undo toast at a time; second bulk delete flushes first | `tests/e2e/bulk-select.spec.ts` | `AC-25 — second bulk delete flushes first pending deletes and shows new toast` | Playwright | Delete batch A → while toast visible, delete batch B → verify exactly one toast visible; intercept network to confirm batch A RPCs fired before batch B undo window starts |
| AC-26: Tab-close behavior is documented known limitation | _(no automated test — known limitation per plan.md Blocker 1 fix; add a comment in the spec file)_ | `// Known limitation: tab close during undo window may skip DeleteSession RPCs — documented, not tested` | N/A | Documentation comment in `bulk-select.spec.ts` |
| AC-27: Context-aware toolbar action buttons | `web-app/src/components/sessions/__tests__/BulkActions.test.tsx` | `AC-27 — BulkActions shows Resume All N when all selected are paused; hides Pause button` | Jest/RTL | Render BulkActions with all selected sessions having `status="paused"` → assert "Resume All N" visible, "Pause" button absent or hidden |
| AC-28: WCAG 2.1 AA accessibility attributes | `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | `AC-28 — aria-multiselectable true and aria-selected on rows meet WCAG 2.1 AA` | Jest/RTL | Enter select mode → query list container for `aria-multiselectable="true"`; query selected row for `aria-selected="true"`; query unselected row for `aria-selected="false"` |

---

## Test Stack

- **Unit**: Jest + React Testing Library (`web-app/src/`)
- **Integration**: Jest + RTL with mocked ConnectRPC transport and `jest.useFakeTimers()` for pending-delete timer logic
- **E2E / UX**: Playwright (`tests/e2e/`), running against `http://localhost:8544`

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --coverage` | ≥80% line coverage on new files: `session-columns.ts`, `rangeSelect.ts`, `SessionRow.tsx` (new props), `SessionList.tsx` (new logic), `NotificationToast.tsx` (undo branch) |
| E2E | `cd tests/e2e && npm test` | All 28 UX acceptance criteria covered; all 5 core E2E scenarios in `bulk-select.spec.ts` pass |

---

## Test File Index

| File | Test count (unit) | Test count (UX/E2E) |
|---|---|---|
| `web-app/src/components/sessions/__tests__/session-columns.test.ts` | 3 | — |
| `web-app/src/lib/utils/__tests__/rangeSelect.test.ts` | 4 | — |
| `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | 7 | — |
| `web-app/src/components/sessions/__tests__/SessionList.test.tsx` | 17 | — |
| `web-app/src/components/sessions/__tests__/BulkActions.test.tsx` | 5 | — |
| `web-app/src/components/ui/__tests__/NotificationToast.test.tsx` | 3 | — |
| `tests/e2e/bulk-select.spec.ts` | — | 25 (+ 1 documented limitation comment) |
| **Total** | **39** | **25** |

---

## Requirements Coverage Summary

| In-Scope Requirement | Unit Tests | E2E Tests | Coverage |
|---|---|---|---|
| `SessionRow` checkbox wiring | 5 | 1 | Covered |
| Hover-reveal affordance | 2 | 1 (AC-01) | Covered |
| `buildRowGridTemplate` checkbox column | 3 | 0 | Covered (pure function, no E2E needed) |
| Shift+click range select | 6 | 2 (AC-08, AC-09) | Covered |
| Cmd/Ctrl+A select all | 2 | 1 (AC-12) | Covered |
| Escape exits select mode | 2 | 1 (AC-03) | Covered |
| Undo toast (NotificationContext extension) | 3 | 1 (AC-22) | Covered |
| Pending-delete pattern | 5 | 2 (AC-23, AC-25) | Covered |
| Undo restore (no RPC) | 2 | 1 (AC-23) | Covered |
| Unmount flush | 2 | 0 | Covered (unmount difficult to assert in E2E; unit sufficient) |
| `activeSelection` derived intersection | 3 | 1 (AC-17) | Covered |
| ARIA attributes | 4 | 1 (AC-28) | Covered |
| Keyboard hint labels in BulkActions | 2 | 0 | Covered (platform detection unit-testable via `navigator.platform` mock) |
| Playwright E2E coverage | — | 5 core + 20 AC scenarios | Covered |

**Requirements coverage fraction: 14 / 14 in-scope requirements (100%)**

**UX acceptance criteria coverage: 27 / 28 (AC-26 is a documented known limitation with no automated test, per the implementation plan's Blocker 1 resolution)**
