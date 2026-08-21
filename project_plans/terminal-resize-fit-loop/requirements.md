# Requirements: Terminal Resize/Fit Feedback Loop Fix

**Status**: Draft | **Phase**: 1 — Requirements derived directly from backlog item (no interactive ideation; backlog item 64b2b67b-9887-4771-9ba6-eaca0b425bb7)
**Created**: 2026-07-24

## Problem Statement

In the multi-terminal web UI, `XtermTerminal`'s `ResizeObserver` and `FitAddon.fit()` can enter an unbounded feedback loop: a sub-pixel/sub-cell container-size jitter (amplified by a WebGL glyph-width measurement that doesn't exactly match the rendered cell width) causes `fit()` to run every animation frame without ever reaching a fixed point. Each iteration re-sends a `TerminalResize` RPC to the server and re-requests pane content, so the loop also floods the server and re-renders downstream terminal state. With multiple terminals mounted concurrently, each pane's resize can perturb its neighbors, extending the churn indefinitely. Net effect: CPU pegged, UI unresponsive, input effectively dead.

Confirmed against current code (not the exact line numbers in the original report, which was filed against v1.34.0 — the code has moved since, but the mechanism is intact):

- `web-app/src/components/sessions/XtermTerminal.tsx` (`ResizeObserver` callback, ~line 259): only gates on a >1px container width/height delta, not on whether `proposeDimensions()` actually changed the integer `cols`/`rows`. A sub-cell jitter that never changes cols/rows still re-triggers `fit()` after debounce.
- `web-app/src/lib/hooks/useTerminalFlowControl.ts` (`resize()`, ~line 364): gates sending the `TerminalResize` RPC purely on a 200ms time throttle (`lastResizeTimeRef`), not on whether `(cols, rows)` actually changed from the last value sent. Once the throttle window passes, an unchanged size is still re-sent.
- `web-app/src/components/sessions/TerminalOutput.tsx`: two call sites invoke `resize()` outside the normal `handleTerminalResize` value-changed path and need to keep working after a value-dedup guard is added — post-connection resync (~line 351) and the manual "Fit" button handler (~line 510).
- WebGL cell-width mismatch: `XtermTerminal.tsx` (~lines 188–197) already logs "Actual pixels per column" vs "Expected pixels per column" from `dims.css.cell.width` and warns above a 1px tolerance, but takes no corrective action — it never falls back to the canvas renderer or otherwise stabilizes the metric.
- No `shortcutRegistry.ts` file exists in this codebase today (the ticket's "Duplicate shortcut id" churn was corroborating evidence from the reporter's build, not reproducible against current source) — no code changes are in scope for that symptom; see Out of Scope.
- No literal tiled/split-pane multi-terminal layout exists in this codebase today. Multiple terminals only ever appear as separate concurrently-mounted `XtermTerminal`/`TerminalOutput` instances (e.g., multiple session cards), not panes sharing a resizable split container. This changes how AC1/AC7 are verified (see Scope and Constraints).

## Success Criteria

Directly from the backlog item's acceptance criteria (verbatim numbering preserved for traceability):

1. Opening 3 terminals (concurrently-mounted, since no tiled-pane layout exists) and backgrounding/resuming the tab, or resizing the window once, does not trigger unbounded resize churn — `fit()` calls settle to zero within a bounded number of debounce cycles after the triggering event.
2. `XtermTerminal`'s `ResizeObserver` only schedules `fitAddon.fit()` when `proposeDimensions()` reports integer `cols`/`rows` that differ from the terminal's currently-applied size AND that difference repeats on two consecutive **ticks** (closes both the sub-cell-jitter loop and the boundary-flapping edge case where a container sits near an exact cell-width boundary). *(Note, added after Phase 3 planning: "tick" here is deliberately not "next raw `ResizeObserver` invocation" — a literal reading of that would deadlock a genuine one-shot resize, since `ResizeObserver` never fires a confirming second time for a size that already settled. `ticks` means samples from a decoupled fixed-cadence sampler; see `project_plans/terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md` for the full derivation of why and how.)*
3. `useTerminalFlowControl`'s `resize()` skips sending the `TerminalResize` RPC (and the follow-up `currentPaneRequest`) when the incoming `(cols, rows)` equals the last pair actually sent — independent of, and in addition to, the existing 200ms time throttle. Value-dedup, not just rate-limiting.
4. The two call sites that must bypass value-dedup (post-connection reconnect-resync, manual "Fit" button) pass an explicit `force: true` parameter that bypasses both the value-dedup check and the existing 200ms time throttle uniformly (a reconnect or an explicit user click must never be silently swallowed by either guard), with a regression test asserting the literal third argument at each call site so a future edit can't silently drop it.
5. The WebGL actual-vs-expected pixels-per-column discrepancy is corrected or mitigated so `fit()` converges under WebGL rendering: a sustained mismatch beyond a defined tolerance triggers a one-directional fallback to the canvas renderer, using `Number.isFinite` (not `Number.isNaN`) guards against `proposeDimensions()` returning `Infinity`.
6. Regression coverage (unit/component tests) simulates sub-cell jitter, boundary-flapping, WebGL mismatch escalation, and force-bypass call sites, each asserting `fit()`/RPC/dispose calls fire at most the expected number of times — not once per observed frame.
7. Manual repro from the ticket (multiple terminals open, background/resume tab or a single window resize) no longer pegs CPU or freezes input, verified in a follow-up manual pass — substituting N concurrently-mounted single-terminal instances for the ticket's literal tiled-pane layout, since no such layout exists in this codebase.

## Scope

### Must Have (this item)
- Dead-band `XtermTerminal`'s `ResizeObserver` → `fit()` path on integer `cols`/`rows` from `proposeDimensions()`, with two-consecutive-tick confirmation, replacing the current raw pixel-delta gate (AC2).
- Value-dedup in `useTerminalFlowControl.resize()`: track last-sent `(cols, rows)` and skip the RPC + follow-up `currentPaneRequest` when unchanged, layered on top of (not replacing) the existing time throttle (AC3).
- `force: true` opt-out parameter on `resize()`, wired into the two identified call sites that must always resync regardless of value-dedup (AC4).
- WebGL cell-width convergence fix: define a tolerance for the actual-vs-expected px/col mismatch, detect sustained (not one-off) mismatch, and fall back to the canvas renderer one-directionally (no flapping back to WebGL) when exceeded; guard all math with `Number.isFinite` (AC5).
- Unit/component test coverage for all of the above failure modes plus the force-bypass call sites (AC6).
- Manual verification pass with N concurrently-mounted terminals substituting for tiled panes (AC7).
- Adjacent hardening, same root-cause class as AC3: fix the pre-existing gap where `useTerminalFlowControl.ts`'s `lastResizeTimeRef` is updated *before* confirming the (throwable) RPC send succeeded, and extend error handling to the async `currentPaneRequest` follow-up send, which currently has none (identified during Phase 2 pitfalls research and the Phase 3 adversarial review; small, in-file fix directly adjacent to the AC3 value-dedup work, not a separate initiative).

### Out of Scope
- Building an actual tiled/split-pane terminal layout — does not exist today and is not requested by this item; AC1/AC7 are satisfied via concurrently-mounted instances as the closest available proxy, per the acceptance criteria's own caveat.
- A `shortcutRegistry.ts` dedup fix — no such file exists in current source; this was reporter-side corroborating evidence, not something reproducible or fixable here.
- Any change to the server-side (Go) resize RPC handling — this is a client-side (web-app) convergence bug; the server already accepts whatever the client sends.
- General terminal performance work beyond what's needed to stop the specific feedback loop (e.g., broader streaming/render pipeline optimization).

## Constraints

- **Tech stack**: TypeScript/React web-app (`web-app/src/`), xterm.js (`@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-webgl`). No new dependencies expected.
- **Compatibility**: Must not change the `TerminalResize` protobuf schema or server-side handling. Must preserve existing behavior for legitimate resizes (real user window/pane resizes still need to reach the server).
- **Verification substitute**: Since no tiled-pane layout exists, AC1/AC7 manual/scenario verification uses multiple concurrently-mounted `XtermTerminal`/`TerminalOutput` instances **on the same page, sharing DOM layout** (e.g., a test harness or an existing multi-session dashboard view) as the closest available proxy for the ticket's reported scenario — explicitly **not** separate browser tabs, which are isolated DOM/JS contexts and cannot exhibit the reported cross-pane perturbation mechanism (clarified during plan review, project_plans/terminal-resize-fit-loop/implementation/adversarial-review.md, after an earlier plan draft mistakenly substituted separate tabs).
- **Testing**: Existing test suite uses Jest 30 + `ts-jest` + `@testing-library/react` (not Vitest — corrected after Phase 2 research; config in `web-app/jest.config.js`/`jest.setup.js`), following conventions in `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` — new tests should follow the same patterns (mocked timers for debounce/throttle assertions, spy-based call counting). No `ResizeObserver` polyfill exists in `jest.setup.js` yet; this needs to be added as part of the new `XtermTerminal.tsx` test file.

## Context

### Existing Work / Key Files

| Component | Location | Relevance |
|---|---|---|
| `XtermTerminal` | `web-app/src/components/sessions/XtermTerminal.tsx` | Owns the `ResizeObserver` → `fitAddon.fit()` loop (~line 259) and the WebGL addon init + cell-width diagnostics (~lines 148–197) |
| `useTerminalFlowControl` | `web-app/src/lib/hooks/useTerminalFlowControl.ts` | Owns `resize()` (~line 364), the existing 200ms time-throttle, and the follow-up `currentPaneRequest` after a resize |
| `useTerminalStream` | `web-app/src/lib/hooks/useTerminalStream.ts` | Composes `useTerminalFlowControl`; re-exports `resize` unchanged (~line 350) |
| `TerminalOutput` | `web-app/src/components/sessions/TerminalOutput.tsx` | Consumer of `resize()`; contains `handleTerminalResize` (value-changed path, ~line 257), the post-connection resync call site (~line 351), and the manual "Fit" button handler (`handleManualResize`, ~line 496) — note: the backlog item's own AC4 wording calls this the "Fit" button, but the live UI label is "↔️ Resize" (`aria-label="Resize terminal"`, `TerminalOutput.tsx:596`); this doc follows the AC's naming for traceability, the code follows the UI's actual label |
| Existing tests | `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` | Current coverage for `resize()`; extend rather than replace |

### Acceptance Criteria Traceability

| Requirement | Backlog AC # |
|---|---|
| Bounded churn after trigger event | AC1 |
| Integer cols/rows dead-band + 2-tick confirmation in `XtermTerminal` | AC2 |
| Value-dedup in `useTerminalFlowControl.resize()` | AC3 |
| `force: true` bypass at the two required call sites + literal-arg regression test | AC4 |
| WebGL px/col mismatch tolerance + canvas fallback + `Number.isFinite` guards | AC5 |
| Regression test coverage for jitter/flapping/WebGL escalation/force-bypass | AC6 |
| Manual repro verification (proxy: concurrent instances) | AC7 |
