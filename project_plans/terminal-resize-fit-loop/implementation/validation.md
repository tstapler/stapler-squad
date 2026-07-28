# Validation Plan: terminal-resize-fit-loop

**Date**: 2026-07-27

## Happy Path Scenario
Given a mounted `XtermTerminal` whose container receives a sub-cell pixel wobble (e.g. a 0.3px
reflow with no cell-boundary crossing), when the `ResizeObserver` fires and the 150ms debounce +
double-rAF chain elapses, then `shouldFit` reports no change, `fit()` and the `TerminalResize` RPC
are each skipped, and the component settles to zero further work — while a genuine cell-boundary
resize still produces exactly one `fit()` call and one RPC.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC2: `fit()` only invoked when `proposeDimensions()` integer output differs from live `terminal.cols`/`rows` | `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts` | `describe('shouldFit', ...)` (Task 1.2.1a) | Unit | Given `terminal.cols=84,rows=60` and `proposeDimensions()` returns `{cols:84,rows:60}` (0.3px wobble case), when `shouldFit({cols:84,rows:60},{cols:84,rows:60})` is evaluated, then returns `false`; also covers cols-only diff (true), rows-only diff (true), and `proposed` fields `undefined` (false) — per Story 1.2.1's 4 cases. |
| AC2: gated call site in `ResizeObserver`'s debounced callback | `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx` | `describe('AC5: sub-cell resize wobble is a no-op', ...)` (Task 4.2.2a) | Integration | Given mount settles at `cols:120,rows:40` with `harness.proposedDimensions` matching, when `fireResizeObserver(801,600)` → `fireResizeObserver(802,601)` → `fireResizeObserver(801,600)` are dispatched with the debounce/rAF chain flushed between each, then `harness.fitCalledCount` does not increase past its post-mount value and the `onResize` prop is not called again. (Story 4.2.2 — also the primary automated evidence for AC2's integration point, since Task 2.2.1a has no separate dedicated test task.) |
| AC3: `resize()` skips `TerminalResize` RPC + `currentPaneRequest` follow-up when `(cols,rows)` equals the last value actually sent, independent of the 200ms time throttle | `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts` | `describe('shouldSendResize', ...)` (Task 1.2.2a) | Unit | Covers `lastSent === null` (true), `lastSent` equals next (false), cols-only diff (true), rows-only diff (true) — per Story 1.2.2's 4 cases. |
| AC3: same-value dedup after throttle window elapses | `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` | new case in existing `describe('resize', ...)` block (Task 4.1.1a) | Unit/Integration | Given `resize(100,30)` sent once and 250ms elapsed (past `THROTTLE_MS`), when `resize(100,30)` is called again with no `force`, then `pushMessage`'s mock call count does not increase and the delayed `currentPaneRequest` re-fetch is not scheduled again. |
| AC3 / AC6 (RPC-level regression guard): genuinely new value is never deduped | `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` | new case in `describe('resize', ...)` (Task 4.1.1b) | Unit/Integration | Given `resize(100,30)` then, past 200ms, `resize(150,45)`, then `pushMessage` is called twice total — mirror-image of 4.1.1a, closing the "no regression" half of AC6 at the RPC-dedup layer. |
| AC3: `force=true` bypasses only the value-dedup, not the time-throttle; reconnect scenario | `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` | new case in `describe('resize', ...)` (Task 4.1.1c) | Unit/Integration | `resize(100,30)` (send #1) → advance past 200ms → `resize(100,30,true)` (send #2, reconnect-resync simulation, bypasses AC3 dedup) → advance past 200ms again (isolates from time-throttle) → `resize(100,30)` no force (deduped, no send #3) — proves the forced call re-established `lastSentSizeRef`. |
| AC3 (caller wiring — **gap found, see Step 1 notes below**) | `web-app/src/components/sessions/__tests__/TerminalOutputBug.test.tsx` | *new*: `resize mock called with force=true at reconnect-resync and manual force-resize call sites` | Unit/Integration | **Not specified in plan.md.** Recommend: using the existing `resize: jest.fn()` mock already wired at `TerminalOutputBug.test.tsx:134` for `useTerminalFlowControl`, assert the reconnect-resync path (`TerminalOutput.tsx:664`) and the manual force-resize path (`TerminalOutput.tsx:1160`) each call `resize(cols, rows, true)` — i.e. 3rd arg is literally `true`. Without this, Tasks 3.1.2a/3.1.2b (the caller edits) have no regression test; a future edit that silently drops the 3rd argument would compile fine (parameter is optional) and only fail via the AC3 dedup unexpectedly swallowing a legitimate reconnect resize — a hard-to-diagnose runtime bug, not a build failure. |
| AC4: oscillation/burst detector — `shouldAbandonWebgl` pure predicate | `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts` | `describe('shouldAbandonWebgl', ...)` (Task 1.2.3a) | Unit | Covers: exactly 3 recurrences within 2000ms window (trips true), 3 recurrences spanning >2000ms where the oldest ages out (false), boundary case at exactly `now - e.at === windowMs` (documents inclusive `<=`), alternating A/B/A/B/A sequence where only the most-recent value's count matters — per Story 1.2.3's 4 cases. |
| AC4: burst trips WebGL `dispose()` + `console.warn` "falling back to default renderer" | `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx` | `describe('AC4: oscillation burst falls back off WebGL', ...)` (Task 4.3.1a), case 1 | Integration | Given mock `WebglAddon` loaded, when `harness.setProposedDimensions` toggles `84,60 → 85,60 → 84,60` across 3 `fireResizeObserver` calls each <2000ms apart with debounce flushed, then mock `WebglAddon.dispose` is called exactly once and `console.warn` logs "falling back to default renderer" (not "canvas renderer"). |
| AC4: backstop — no WebGL addon to dispose | same file | `describe('AC4: ...', ...)`, case 2 (Task 4.3.1a) | Integration | Given WebGL never loaded (mocked `WebGL2RenderingContext` guard `undefined`), when the same oscillation sequence occurs, then `console.error` logs "no WebGL addon to dispose" and nothing throws. |
| AC4: repeated burst after fallback does not re-log error | same file | `describe('AC4: ...', ...)`, case 3 (Task 4.3.1a) | Integration | Given WebGL already fell back once (`dispose` call count 1), when a second independent burst occurs >2000ms later, then `dispose` is not called again, `console.error` is not called again, and a single `console.log` containing "persists after WebGL fallback" is emitted. |
| AC4: `webglAddonRef` survives StrictMode double-mount (async-load race) | same file | `describe('webglAddonRef survives StrictMode double-mount', ...)` (Task 4.2.1b) | Integration | Given `<XtermTerminal>` wrapped in `<React.StrictMode>` with a distinguishable mock addon instance per resolved `import()` call, when all pending microtasks are flushed, then the live ref points at the *second* (real) mount's addon instance, not the first (throwaway/cancelled) one. Closes architecture-review.md's async-load-race Blocker. |
| AC4: ADR documenting the renderer-fallback decision | `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md`, `project_plans/terminal-resize-fit-loop/decisions/ADR-001-webgl-oscillation-fallback-to-default-renderer.md` | N/A (documentation artifact, not a test) | Documentation | Required deliverable per requirements.md's explicit ADR mandate (Tasks 5.1.1a, 5.1.1b). Must state the "canvas" → "default DOM renderer" terminology correction with `@xterm/addon-canvas`/`@xterm/xterm ^6.0.0` incompatibility evidence, plus the four Consequences call-outs (session-scoped only, not a root-cause fix, residual false-positive risk on slow pane drags, scope note on the imperative `fit()` path). PR review gate: confirm both files exist and the pointer stub links to the real ADR before merge. |
| AC5: automated regression coverage exists (unit tests for pure functions + a component test exercising real `ResizeObserver` wiring, simulating sub-cell resizes) | `web-app/src/lib/terminal/__tests__/resizeConvergence.test.ts` (Tasks 1.2.1a/1.2.2a/1.2.3a) + `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx` (Tasks 4.2.1a, 4.2.1b, 4.2.2a) | (aggregate — see individual rows above) | Unit + Integration | This AC is satisfied by the sum of the rows above: all 3 pure functions are unit tested end-to-end (happy path + edge paths), and Task 4.2.2a is the literal "AC5"-labeled component test asserting `fit()`/resize RPC are each invoked at most once when proposed cols/rows never change, against the real `ResizeObserver` via `fireResizeObserver()` (not a mocked observer callback). |
| AC6: a real cols/rows change still triggers exactly one `fit()` and one resize RPC | `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx` | `describe('AC6: genuine cols/rows change still converges exactly once', ...)` (Task 4.2.3a) | Integration | Given `terminal.cols=80,rows=24` at mount, when `harness.setProposedDimensions(150,45)` and `fireResizeObserver(1200,900)` (a genuine cell-boundary crossing) with the debounce+rAF chain flushed, then `harness.fitCalledCount` increases by exactly 1, mock terminal `cols`/`rows` become `150`/`45`, and the `onResize` prop fires exactly once with `(150,45)`. |
| AC6 (RPC-level half) | `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` | Task 4.1.1b (same row as AC3 above — dual-mapped) | Unit/Integration | See AC3 row above — asserts `pushMessage` fires once per genuinely distinct value, proving the AC3 dedup gate does not also swallow legitimate resizes. |
| AC1: any resize-triggering event settles to zero further `fit()`/RPC calls within a bounded number of debounce cycles, across concurrently open sessions | `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx` | `AC1/AC7: imperative fit() handle is gated the same as the ResizeObserver path` (Task 4.2.4a) | Unit/Integration | Given `terminal.cols=84,rows=60` and `harness.proposedDimensions={cols:84,rows:60}` (no layout change), when `ref.current.fit()` is called directly (simulating the imperative path, not the `ResizeObserver`), then `harness.fitCalledCount` does not increase. |
| AC1 (continued — no automated multi-tab/CPU equivalent) | None | Manual repro checklist, Story 5.2.1 — **hard merge gate**, split into independent window-resize and tab-background/resume sub-cases | Manual | Given 3 concurrent browser tabs each with its own `XtermTerminal`/session, when the OS window is resized once (e.g. 1200×800 → 1400×900), then Chrome DevTools' Performance panel (5s recording from resize) shows the chain settling (no repeating `[XtermTerminal] Container resized` log lines after 1-2 debounce cycles) and CPU returns to idle (<5%) within 2s, in each tab. Outcome + quantitative pixels-per-column baseline + shortcut-registry-churn observation recorded in the PR description; must pass before merge, not just before PR-description mention. |
| AC7: original manual repro no longer pegs CPU or freezes input, specifically via the tab-background/resume trigger | `web-app/src/components/sessions/__tests__/XtermTerminalResize.test.tsx` | Same `AC1/AC7` describe block (Task 4.2.4a), second case: proposed dims genuinely change, gate does not swallow a real resize | Unit/Integration | Given the same setup but `harness.proposedDimensions` changes to `{cols:90,rows:62}` (a genuine change, as if the container resized while backgrounded), when `ref.current.fit()` is called, then `harness.fitCalledCount` increases by exactly 1 — the AC1/AC7 imperative-path gate doesn't regress legitimate use. |
| AC7 (continued — no automated CPU/input-responsiveness equivalent) | None | Manual repro checklist, Story 5.2.1 — same hard merge gate as AC1, verified as an *independent* sub-case (per pre-mortem.md P1 #1 — do not assume AC1's window-resize pass also covers AC7's background/resume trigger) | Manual | Given the same 3-tab setup, when one tab is backgrounded (≥5s) and resumed, then input in all 3 tabs remains responsive throughout (typing echoes without perceptible stall) and no tab freezes. Recorded in the PR description as its own pass/fail, not folded into AC1's result; do not skip even though it produces no test file. |

## UX Acceptance Tests
N/A — no user-facing surface (confirmed in research/ux.md and plan.md; this is a background
reliability fix with no new UI).

## Test Stack
- **Unit**: Jest (existing stack — `web-app/`, `jest ^30.2.0`, `npm run test` → `jest`)
- **Integration**: N/A as a separate backend/DB layer — the "Integration" rows above are
  React-Testing-Library component tests against the real `ResizeObserver` wiring (mocked
  `@xterm/xterm`/`@xterm/addon-fit`/`@xterm/addon-webgl` per the existing
  `XtermTerminalBug.test.tsx` harness convention), not integration against an external service.
- **E2E / UX**: Manual QA checklist (Story 5.2.1 in plan.md) for AC1/AC7 — no automated way to peg
  CPU and observe convergence across real browser tabs.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="resizeConvergence|useTerminalFlowControl|XtermTerminalResize" --no-coverage` (drop `--no-coverage` to actually collect the report; the two flags are mutually exclusive — use `--coverage` when measuring, omit it for a fast CI run) | New code (`resizeConvergence.ts`'s 3 pure functions, the gated branches in `XtermTerminal.tsx`'s `resizeDisposable`/debounced-fit callback, and `useTerminalFlowControl.ts`'s `resize()` dedup branch) at or near 100% line coverage — it's ~30-50 lines of pure logic plus small integration points |

- All 3 new pure functions (`shouldFit`, `shouldSendResize`, `shouldAbandonWebgl`): happy path +
  error/edge paths covered (undefined inputs, boundary timing, alternating-value history)
- No external integrations in scope (client-side only; no DB/RPC contract change — server-side Go
  handling is explicitly out of scope per requirements.md)
- AC1/AC7: manual checklist, documented in PR description per Story 5.2.1

## Step 1 Notes — Cross-Check Findings

- All 7 ACs map to at least one concrete test or, for AC1/AC7, an explicit manual checklist item
  (Story 5.2.1) — not skipped.
- AC4 additionally requires a non-test documentation artifact (ADR-018 + project-local pointer
  stub); included above as a "Documentation" row so PR review has an explicit checkbox for it.
- **One genuinely missing test** was found while cross-checking Story 3.1.2 (the 2 non-standard
  `resize()` callers in `TerminalOutput.tsx` updated to pass `force=true`): plan.md's Tasks
  3.1.2a/3.1.2b are implementation-only edits with no corresponding test task. The hook-level
  `force=true` behavior *is* tested (Task 4.1.1c, on the hook directly), but nothing asserts
  `TerminalOutput.tsx` actually threads `true` through at its two call sites
  (`TerminalOutput.tsx:664`, `:1160`). Since `force` is an optional 3rd parameter, a future edit
  that drops the argument compiles cleanly and would only surface as AC3's dedup silently
  swallowing a legitimate reconnect resize — recommend adding this to
  `TerminalOutputBug.test.tsx`, which already mocks `resize: jest.fn()` for
  `useTerminalFlowControl` (line 134), making the addition low-cost (see table row above).
- No other coverage gaps found — Phase 4's test tasks were cross-checked 1:1 against every Story's
  Given-When-Then in Phases 1-3 and each has a matching Task ID.

## Step 3 Notes — Test Naming Convention

Plan.md specifies literal `describe(...)` block names for the integration/component-level tests
and they are already descriptive and AC-tagged where relevant (e.g.
`describe('AC5: sub-cell resize wobble is a no-op', ...)`,
`describe('AC6: genuine cols/rows change still converges exactly once', ...)`,
`describe('AC4: oscillation burst falls back off WebGL', ...)`,
`describe('webglAddonRef survives StrictMode double-mount', ...)`,
`describe('shouldFit', ...)` / `describe('shouldSendResize', ...)` / `describe('shouldAbandonWebgl', ...)`).
These are clear and need no renaming.

However, plan.md does **not** spell out literal `it(...)` strings for the individual test cases
within Phase 4's hook and component describe blocks (Tasks 4.1.1a-c, 4.2.2a, 4.2.3a, 4.3.1a are
task-level prose, not test names). Before implementation, recommend the implementer follow this
repo's existing convention (seen in `detector.test.ts` per
`.claude/rules/feature-testing-registry.md`): `<subject>_should_<effect>_When_<condition>`, e.g.:
- `shouldFit_should_returnFalse_When_proposedEqualsCurrentDimensions`
- `resize_should_skipPushMessage_When_valueMatchesLastSentAfterThrottleWindow`
- `resize_should_sendRpc_When_forceTrueBypassesValueDedup`
- `XtermTerminal_should_notCallFit_When_subCellWobbleProducesNoIntegerChange`
- `XtermTerminal_should_disposeWebgl_When_sameSizeRecursThreeTimesWithin2000ms`

This is a minor pre-implementation clarity improvement, not a blocker — the `describe` grouping
already makes intent unambiguous even without it.

## Migration Plan
N/A — no schema/data changes (plan.md explicitly omits this section; Migration Plan section
skipped per plan.md's own "Omit — no schema/data changes" note).
