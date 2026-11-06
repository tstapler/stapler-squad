# Architecture research: mobile terminal scroll + keyboard redraw

All paths are under `web-app/src/`. Confidence: VERIFIED = code read this session; INFERRED = hypothesis, not reproduced.

## 1. Integration points

| Concern | Location |
|---|---|
| Gesture state machine (IDLE/PENDING/SCROLLING/SELECTING/TAPPING) | `lib/hooks/useTerminalGestures.ts` (touchstart on container; touchmove/end/cancel on `document`) |
| Hook wiring | `components/sessions/XtermTerminal.tsx:262` (`onSendData` -> `onDataRef`) |
| Mouse-tracking detection | `lib/terminal/mouseTracking.ts` (reads `terminal.modes.mouseTrackingMode`) |
| Cell metrics | `lib/terminal/cellDimensions.ts` (`el.clientHeight / rows`) |
| rAF coalescing helper | `lib/terminal/touchDrag.ts` `rafThrottlePoint` |
| Own touch code in XtermTerminal | selection handles (~L814-886), scrollbar track tap (~L908-928), scrollbar thumb drag (~L957-1010) |
| Container ResizeObserver + decoupled sampler (ADR-002) | `XtermTerminal.tsx:1014-1149` (150ms debounce, then 50ms sampler ticks, `fit()` only when stable) |
| visualViewport refit | `components/sessions/TerminalOutput.tsx:1172-1188` |
| `--keyboard-height` / `--viewport-height` | `components/providers/ViewportProvider.tsx:33-57` (rAF per event); only `--viewport-height` is consumed (many `.css.ts`); `--keyboard-height` has no consumer in `src/` |
| Mobile PgUp/PgDn keys | `TerminalOutput.tsx:1984/2018` send `\x1b[5~` / `\x1b[6~` via `sendKey` |
| Gesture CSS | `XtermTerminal.css.ts:60` `touchAction: "none"` on the terminal container |

## 2. Data flow (a): touch-drag to scroll (VERIFIED)

1. `touchstart` (container) -> `PENDING`, 400ms long-press timer.
2. `touchmove` on document: `|dy| > 15px` -> `SCROLLING`, caches `cellH`, creates a per-frame `rafThrottlePoint`.
3. Each frame: `lines = Math.round(-moveDy / cellH)`; `lastY = clientY`; `terminal.scrollLines(lines)` (`useTerminalGestures.ts:240-247`).
4. `touchend` -> `transitionToIdle()`. No inertia.

Defects found in this path:

- **Sub-cell remainder is discarded.** `lastY` advances every frame even when `lines === 0`, so slow drags (moves under half a cell per frame) never accumulate. This affects both directions, not just down. Fix: accumulate a fractional remainder, or advance `lastY` by `lines * cellH` only.
- **Drag target is xterm's own scrollback only.** `scrollLines()` is a no-op on the alt-screen buffer, which has no scrollback. With a TUI, the gesture has nothing to scroll. The PgUp/PgDn buttons work because they send bytes to the PTY (the TUI scrolls itself). `isMouseTracking()` is used for tap/selection but NOT for scroll routing; there is no alt-buffer check either (`terminal.buffer.active.type === 'alternate'` is the available public signal).
- **The 15px dead-zone movement is dropped.** `lastY` is set to the threshold-crossing point, so the first 15px of travel does not move content (not 1:1).
- **No bounds/overscroll handling.** `scrollLines` clamps silently, so there is no way to tell "at top" from "ignored".
- Why drag-down "does nothing" specifically is NOT established from code alone: the logic is symmetric. Likely candidates (INFERRED): the viewport is at the bottom or in the alt buffer, so only one direction has anywhere to scroll in xterm's buffer, plus the rounding loss above. Needs on-device instrumentation (log `buffer.active.type`, `viewportY`, `baseY`, computed `lines`).

## 3. Data flow (b): keyboard open -> resize -> fit -> refresh (VERIFIED code, INFERRED race)

Two independent, uncoordinated pipelines react to the same visualViewport resize:

1. `ViewportProvider`: `vv resize/scroll` -> rAF -> sets `--viewport-height` -> layout containers shrink (`SessionDetail.css.ts:18/25`) -> container `ResizeObserver` fires in XtermTerminal -> 150ms debounce -> sampler (50ms ticks, up to 20) -> `fit()` -> `terminal.onResize` -> server resize.
2. `TerminalOutput` `onVpResize`: `isFittingRef` guard, then `setTimeout(fit, 400)` on mobile, guard released one rAF after.

Notable problems:

- **No `terminal.refresh()` / renderer reset anywhere in the resize path.** `refresh(0, rows-1)` is only called on theme change and manual clear (`XtermTerminal.tsx:1199/1214`, `TerminalOutput.tsx:1486`). A WebGL canvas that is resized/hidden during the keyboard transition and not repainted is the leading candidate for the black canvas. Note `XtermTerminal.tsx:470-534` has a WebGL mismatch -> canvas fallback latch; WebGL context loss on Android during resize is plausible but not instrumented (no `onContextLoss` handler seen in the grep; verify).
- **Fixed 400ms timer vs. unbounded keyboard animation.** The `isFittingRef` guard drops every vv resize event that arrives during the 400ms window, so the timer's `fit()` reflects whatever size exists at t+400ms. If Android's keyboard animation finishes later, or a final resize lands during the window, the final size is never fit by pipeline 2 (INFERRED race). Pipeline 1 can still catch it, but its `lastContainerSize` dedupe + 1px threshold + sampler `give up after 20` can leave it stale.
- **Pipeline 2 duplicates pipeline 1.** Both call `fit()`; pipeline 2's `fit()` bypasses ADR-002's stability sampler and `lastContainerSize` bookkeeping, so the next RO delivery can be treated as "changed" and trigger a second fit.
- **`fit()` imperative handle is bare** (`XtermTerminal.tsx:1269`: `fitAddonRef.current?.fit()`): no zero-size guard, no refresh, no scroll-to-bottom anchoring.
- **Zero-size RO deliveries are skipped** (`:1144`), so a collapse followed by restore with identical dims to `lastContainerSize` (within 1px) never re-runs fit or repaints; a stale canvas persists.
- `ViewportProvider` rAF-per-event is not coalesced (each event schedules its own rAF), but it is cheap; low priority.

## 4. Duplicate / competing handlers

- `useTerminalGestures` (touchstart on container, touchmove/end on `document`, all `passive:false` for move) coexists with three other touch handler sets in XtermTerminal: selection handles, scrollbar track, scrollbar thumb. They also use document-level touchmove. The handles/thumb live on their own elements, but nothing stops a thumb/handle touch from ALSO starting a gesture in the hook if their elements are inside `containerRef` (touchstart bubbles). Needs a check for `stopPropagation` in those handlers (INFERRED risk, not confirmed).
- Document-level `touchmove` listeners from all four sources each run on every move anywhere on the page (not just in the terminal) while their state is non-IDLE; the hook's `onTouchMove` also fires for touches that never started in the container if `state` is stale. Mitigated by the `IDLE` early exits but worth a unit test.
- The hook's own docstring says it replaced the earlier double-scroll hooks, so hook-vs-hook duplication is already resolved; the remaining duplication is the resize pipelines above.

## 5. Recommended approach

1. **Scroll routing as a pure module** (`lib/terminal/scrollRouting.ts`): `decideScrollTarget(terminal) -> 'xterm' | 'tui-wheel' | 'tui-pgkeys'` plus a pure accumulator (`ScrollAccumulator`: fractional px -> integer lines, momentum decay) and a velocity tracker for inertia. Keep the state machine in the hook; the hook only calls `route(lines)`.
2. **Forward to TUI when** `buffer.active.type === 'alternate'` or mouse tracking is on: emit SGR wheel events (`\x1b[<64;col;rowM` up / `65` down; requires SGR mode, else X10 style like the existing tap code) with PgUp/PgDn bytes as the fallback. Which one Claude Code honors is an open question that needs on-device testing (requirements Open Question 3); keep it behind the routing seam so it is one-line switchable.
3. **Single resize coordinator**: remove the `TerminalOutput` 400ms `setTimeout(fit)` path and instead let the XtermTerminal RO/sampler own `fit()`; add a "post-fit repaint" step (`fit()` -> `terminal.refresh(0, rows-1)`, plus `clearTextureAtlas()` when WebGL is active) in the sampler's confirmed-fit branch and in the imperative `fit`/a new `refit()` handle. Add a visualViewport-driven "settled" signal (resize events stop for N ms, or `visualViewport` `resize` + rAF stability check) as an additional sampler trigger instead of a fixed 400ms. Add `onContextLoss` -> fallback/refresh and a debug log on forced refresh (requirements: Observability).
4. Instrument first (cheap, on-device): log `buffer.active.type`, `modes.mouseTrackingMode`, `viewportY/baseY`, and per-fit size + renderer state, to turn the two INFERRED items above into VERIFIED before locking in fixes.

## 6. Disposition for XtermTerminal.tsx: **Isolate via seam**

Do not refactor the 1378-line file first. Put the new scroll routing/inertia in a pure `lib/terminal/` module consumed by `useTerminalGestures` (already a separate hook), and add one small `refit()`/post-fit-repaint seam in XtermTerminal at the existing sampler confirmed-fit branch (`:1051-1070`) and the imperative handle (`:1269`); both are localized edits that existing ADR-002 tests cover. A refactor-first pass would put ADR-002's carefully tuned resize logic at regression risk for no benefit to either bug.
