# Validation Plan: fix-mouse-input-readiness

**Date**: 2026-10-19

## Happy Path Scenario

Given a mobile terminal that had vim running (which left `mouseTrackingMode !== "none"`), when the user toggles the "Mouse" toolbar button OFF (`mouseMode === 'none'`) and then taps the terminal, then only `terminal.focus()` is called and no X10 or SGR mouse escape sequences are forwarded to the PTY.

---

## Requirement → Test Mapping

| # | Requirement | Test File | Test Name | Type | Scenario |
|---|-------------|-----------|-----------|------|----------|
| AC1-a | Tap with `mouseTrackingMode === "none"` calls only `focus()`, no mouse escapes | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_callFocusOnly_When_mouseTrackingModeIsNone` | Unit | `mode=none, mouseModeOverride=any` → `onSendData` not called, `t.focus` called |
| AC1-b | `mouseTrackingMode === "none"` with override also none → still only focus | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_callFocusOnly_When_mouseTrackingModeIsNone_andOverrideIsNone` | Unit | Both gates say "no tracking" → no `onSendData` |
| AC2-a | `mouseTrackingMode !== "none"` + `mouseModeOverride === 'any'` → sends X10 bytes | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_sendX10MouseBytes_When_trackingActiveAndOverrideIsAny` | Unit | `mode=vt200, override=any` → `onSendData` called with `\x1b[M` press+release |
| AC2-b | X10 payload encodes clamped col/row in 1-indexed terminal coordinates | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_encodeColRowCorrectly_When_sendingX10Sequence` | Unit | Touch at pixel (50, 60) → X10 bytes match expected cell col/row offsets |
| AC3-a | `mouseModeOverride === 'none'` blocks forwarding even when PTY has tracking active | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_callFocusOnly_When_overrideIsNone_evenIfTrackingIsActive` | Unit | `mode=vt200, override=none` → `onSendData` not called, `t.focus` called |
| AC3-b | `mouseModeOverride === 'none'` blocks forwarding for all non-none PTY modes | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_callFocusOnly_When_overrideIsNone_forAllTrackingModes` | Unit | Parameterized over `x10 / vt200 / any / drag` — all blocked when override=none |
| AC3-c (stale closure) | Toggle `mouseModeOverride` from `'none'` to `'any'` mid-session propagates immediately | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_reflectLatestOverride_When_toggledMidSession` | Unit | Rerender hook with `override=none`, tap → blocked; rerender with `override=any`, tap → `onSendData` called; proves `optionsRef.current` read, not stale closure |
| AC3-d (stale closure, reverse) | Toggle from `'any'` to `'none'` mid-session also propagates | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_blockAfterToggle_When_overrideSwitchedFromAnyToNone` | Unit | Tap with `any` → data sent; rerender with `none` → next tap blocked |
| AC4-a | `mouseModeOverride` prop default (`undefined`) preserves backward-compatible `'any'` behavior | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_sendMouseBytes_When_overrideIsUndefined_andTrackingIsActive` | Unit | Hook called without `mouseModeOverride` field → still forwards clicks when tracking active |
| AC4-b | `TerminalOutput.tsx` "Mouse" button toggle changes `mouseMode` state and threads it to `scrollGestureProps` | `web-app/src/components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx` | `MouseButton_should_updateScrollGestureProps_When_toggled` | Integration | Render `TerminalOutput` on mobile, click Mouse button → `scrollGesture.mouseModeOverride` flips from `'any'` to `'none'` |
| AC4-c | `mouseMode` appears in `scrollGestureProps` useMemo deps so stale closure is impossible | `web-app/src/components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx` | `MouseButton_should_propagateImmediately_When_clickedMultipleTimes` | Integration | Three rapid toggles → each tap on terminal reflects the current value |
| AC5-a | `connectionEpoch` bump writes DECSET disable sequences to xterm `terminal.write()` | `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` | `connectionEpoch_should_writeMouseTrackingDisable_When_epochChanges` | Unit | Simulate `connectionEpoch` change → `mockXtermHandle.write` called with string containing `\x1b[?1000l` |
| AC5-b | Disable sequences cover all five tracking DECSET codes | `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` | `connectionEpoch_should_writeAllFiveDisableSequences_When_epochChanges` | Unit | Written string contains `?1003l`, `?1002l`, `?1001l`, `?1000l`, `?9l` |
| AC5-c | `connectionEpoch` bump does NOT route disable sequences to `onSendData` (no PTY corruption) | `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` | `connectionEpoch_should_notCallOnSendData_When_resettingMouseTracking` | Unit | Capture `onSendData` mock → not called during epoch-change effect |
| AC5-d | No write to `terminal.write()` when epoch is unchanged (no unnecessary churn) | `web-app/src/components/sessions/__tests__/TerminalOutput.reconnect.test.tsx` | `connectionEpoch_should_notWriteDisableSequences_When_epochUnchanged` | Unit | Re-render with same epoch → `write` call count does not increase |
| AC5-e (Story 5.1) | `terminal.options.mouseEnabled` set to `false` when `mouseModeOverride === 'none'` | `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx` | `mouseEnabled_should_beFalse_When_mouseModeOverrideIsNone` | Unit | `XtermTerminal` receives `scrollGesture={{ mouseModeOverride: 'none' }}` → `mockTerminal.options.mouseEnabled === false` |
| AC5-f (Story 5.1) | `terminal.options.mouseEnabled` set to `true` when `mouseModeOverride === 'any'` | `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx` | `mouseEnabled_should_beTrue_When_mouseModeOverrideIsAny` | Unit | `scrollGesture={{ mouseModeOverride: 'any' }}` → `mockTerminal.options.mouseEnabled === true` |
| AC5-g (Story 5.2) | Mobile auto-default: `mouseMode` is `'none'` without user touching toolbar | `web-app/src/components/sessions/__tests__/TerminalOutput.toolbarKeys.test.tsx` | `mouseMode_should_autoDefaultToNone_When_onMobileWithoutToolbarInteraction` | Unit | Render `TerminalOutput` with `isMobile=true`, no click → `scrollGesture.mouseModeOverride === 'none'` in first captured props |
| AC6-a | On desktop (`isMobile=false`), `mouseMode` defaults to `'any'` → existing mouse tracking flows unchanged | `web-app/src/components/sessions/__tests__/TerminalOutput.test.tsx` | `mouseMode_should_defaultToAny_When_onDesktop` | Unit | Render with viewport `isMobile=false` → no `mouseModeOverride: 'none'` passed to scroll gesture |
| AC6-b | Desktop tap route is unaffected: no touchstart handler registered on non-mobile | `web-app/src/lib/hooks/__tests__/useTerminalGestures.test.ts` | `handleTap_should_forwardMouseClicks_When_desktopDefaultsAndTrackingActive` | Unit | `mouseModeOverride=undefined` (desktop default) + `mode=vt200` → `onSendData` called |
| AC7-a | `GestureOptions` accepts `mouseModeOverride?: 'none' \| 'any'` without TypeScript error | _(type-level; covered by compile step: `make build`)_ | _(build gate)_ | Build | `tsc --noEmit` passes after adding the field |
| AC7-b | `ScrollGestureProps` threads `mouseModeOverride` to `useTerminalGestures` | `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx` | `scrollGestureProps_should_passMouseModeOverride_When_provided` | Integration | `XtermTerminal` receives `scrollGesture={{ mouseModeOverride: 'none' }}` → gesture hook sees it |

---

## UX Acceptance Tests

N/A — no design/ux.md for this feature (pure mobile-gesture fix, no new UI surfaces added).

Manual checklist for human smoke-test on a real device (not automated):

1. Open a session running `vim` on mobile. Confirm mouse cursor moves in vim on tap (tracking mode active, Mouse ON).
2. Exit vim without `:set mouse=` cleanup. Open `gh auth login`. Tap the terminal. Confirm no garbled `^[[M...` bytes appear in the prompt.
3. Return to vim. Toggle Mouse button OFF. Tap anywhere. Confirm no X10 bytes reach vim (cursor does not jump).
4. Toggle Mouse button back ON. Tap. Confirm vim cursor moves again.
5. Kill the session and start a new one. Open `less`. Verify reconnect reset cleared any stale tracking mode from the previous session.

---

## Test Stack

- **Unit**: Jest + `@testing-library/react` hooks (`renderHook`) — for `useTerminalGestures` and isolated logic tests
- **Integration**: Jest with mock xterm.js terminal handle (existing `mockXtermHandle` pattern in `TerminalOutput.reconnect.test.tsx`) — for `TerminalOutput` and `XtermTerminal` prop-threading tests
- **E2E / UX**: Manual checklist (Playwright not applicable for terminal touch-gesture testing on JSDOM)

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && bunx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |
| Changed files only | `cd web-app && bunx jest --coverage --collectCoverageFrom='src/lib/hooks/useTerminalGestures.ts,src/components/sessions/TerminalOutput.tsx,src/components/sessions/XtermTerminal.tsx'` | ≥90% line on changed files |

---

## Test Implementation Notes

### `handleTap_should_reflectLatestOverride_When_toggledMidSession` (AC3-c)

This test is the critical stale-closure regression test. Pattern:

```ts
const { rerender } = renderHook(
  ({ override }: { override: 'none' | 'any' }) =>
    useTerminalGestures({ ..., mouseModeOverride: override }),
  { initialProps: { override: 'none' as const } },
);
// Tap with override='none' → onSendData not called
fireTouchStart(50, 60); jest.advanceTimersByTime(100); fireTouchEnd(50, 60);
expect(onSendData).not.toHaveBeenCalled();

// Toggle to 'any' — no remount, just prop change
rerender({ override: 'any' });
fireTouchStart(50, 60); jest.advanceTimersByTime(100); fireTouchEnd(50, 60);
expect(onSendData).toHaveBeenCalledTimes(1); // propagated immediately
```

If `mouseModeOverride` is captured from the effect closure instead of read via `optionsRef.current`, the second tap will still call `onSendData` zero times, and this test will fail — catching the exact bug the plan warns about.

### `connectionEpoch_should_writeMouseTrackingDisable_When_epochChanges` (AC5-a/AC5-b)

Reuse the `mockXtermHandle` pattern from `TerminalOutput.reconnect.test.tsx`. Key assertion:

```ts
const EXPECTED_SEQUENCES = ['\x1b[?1003l', '\x1b[?1002l', '\x1b[?1001l', '\x1b[?1000l', '\x1b[?9l'];
const writtenArg: string = mockXtermHandle.write.mock.calls[0][0];
EXPECTED_SEQUENCES.forEach(seq => expect(writtenArg).toContain(seq));
```

The test must also verify `mockXtermHandle.write` is called (not `onSendData`), which proves the reset goes through xterm's VT parser, not PTY input.

### `MouseButton_should_updateScrollGestureProps_When_toggled` (AC4-b)

Because `scrollGestureProps` is passed as a prop to `XtermTerminal` (which is mocked), capture the mock's received props across renders:

```ts
// In TerminalOutput.toolbarKeys.test.tsx — extend or add to existing toolbar suite
// Mock XtermTerminal and capture scrollGesture prop on each render
const capturedScrollGestureProps: ScrollGestureProps[] = [];
jest.mock('../XtermTerminal', () => {
  return { XtermTerminal: React.forwardRef((props: any, ref: any) => {
    capturedScrollGestureProps.push(props.scrollGesture);
    React.useImperativeHandle(ref, () => mockXtermHandle);
    return <div data-testid="mock-xterm" />;
  }) };
});
// After clicking the Mouse button:
const lastProps = capturedScrollGestureProps[capturedScrollGestureProps.length - 1];
expect(lastProps?.mouseModeOverride).toBe('none');
```

---

## Summary

| Metric | Value |
|---|---|
| Total test cases | 21 |
| Unit tests | 17 |
| Integration tests | 4 |
| E2E / UX acceptance tests | N/A (manual checklist only) |
| Migration tests | N/A |
| Requirements covered | 7 of 7 ACs (AC1–AC7) |
| Requirements coverage fraction | 7/7 = 100% |
| New test files needed | 0 (all tests added to existing files) |
| Existing test files modified | 3: `useTerminalGestures.test.ts`, `TerminalOutput.reconnect.test.tsx`, `TerminalOutput.toolbarKeys.test.tsx` |
| XtermTerminal.test.tsx additions | 3 tests (AC7-b, AC5-e, AC5-f) |
| Post-P1-fix additions | 3 tests for Story 5.1 native suppression (AC5-e, AC5-f) and Story 5.2 mobile auto-default (AC5-g) |
