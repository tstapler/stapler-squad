# Build vs. Buy: touch scroll and keyboard-resize redraw

Evidence labels: VERIFIED (opened/ran), INFERRED (reasoned, not checked), UNVERIFIED (from memory, not checked this session).

## Context
- `web-app/package.json:83-89`: `@xterm/xterm ^6.0.0`, addon-fit `^0.11.0`, webgl `^0.19.0`, canvas `^0.7.0`. No `@use-gesture`, no hammer.js. (VERIFIED)
- Current touch code: `web-app/src/lib/hooks/useTerminalGestures.ts` (374 lines), state machine PENDING / SCROLLING / SELECTING, drag-scroll at ~L225-258; `web-app/src/lib/terminal/touchDrag.ts` has `rafThrottlePoint`. (VERIFIED)
- **Likely root cause of one-directional scroll (INFERRED from code, not yet reproduced):** the rAF callback computes `Math.round(-moveDy / cachedCellH)` and then unconditionally sets `lastY = clientY`. Two defects: (a) JS `Math.round` rounds .5 toward +Infinity, so an upward drag (positive lines) rounds up at half a cell while a downward drag (negative lines) rounds toward zero, which is a real up/down asymmetry; (b) sub-line remainders are discarded every frame, so slow drags (a few px per frame, below half a cell) never scroll at all. Fix is an accumulator (carry the remainder), not a library. Independently, `scrollLines()` only moves xterm's buffer, not the TUI's scrollback.

## 1. OSS options

### 1a. xterm.js 6 native touch scrolling
- Finding: `@xterm/xterm@6.0.0` (`npm pack`, `lib/xterm.mjs`) bundles VS Code's touch `Gesture` class, with `touchstart`/`touchmove` (passive:false)/`touchend` listeners, rolling-window velocity and an `inertia()` loop with `SCROLL_FRICTION`. It dispatches Change/End/Tap gesture events to registered targets. (VERIFIED: strings present in the bundle.) I did not confirm which elements register as targets or that the viewport consumes them (the bundle is minified; `addTarget` is defined, call site not located). (UNVERIFIED)
- Scope: whatever it does, it scrolls xterm's own scrollback viewport. In the alt screen (Claude Code TUI) xterm has no scrollback, and Claude Code's scrollback is the TUI's own, reached by PgUp/PgDn or wheel/mouse events. (INFERRED; requirements.md L9 says the same about `scrollLines()`.)
- Pros: zero new deps; momentum already written and battle-tested by VS Code; 1:1 tracking in normal buffer.
- Cons: cannot target TUI scrollback; no hook to translate gestures into PgUp/PgDn or SGR wheel; competes with the custom state machine (long-press select, double-tap) over the same touch events; behavior would change on xterm upgrades.
- Verdict: **Viable only for the plain-scrollback path; Not recommended as the replacement for the state machine.** Worth a 30-minute spike to see whether it already scrolls the normal buffer correctly on-device, which would let us delete the normal-buffer branch.

### 1b. @use-gesture/react
- Pros: well-maintained, handles drag with velocity/direction/axis lock, `pointer` and touch backends; `useDrag` gives `movement`, `velocity`, `last`. Saves writing velocity math.
- Cons: pointer-event-first (ADR-012 mandates TouchEvent-based handling; it has a `touch` config but adds an abstraction layer); ~10 kB; no line quantization or momentum, so we still write inertia and the TUI-forwarding logic; long-press/selection and tap logic already live in our state machine and would need to be reconciled with its lifecycle; React-hook binding to a DOM node owned by xterm is awkward (our hook attaches to document and the xterm element).
- Verdict: **Not recommended.** Solves the part that is already roughly right (tracking) and none of the hard part (target selection, quantization).

### 1c. hammer.js
- Pros: pan/swipe/press recognizers.
- Cons: unmaintained since ~2016-2019 (UNVERIFIED date; run `oss-health-check` if it matters), no momentum, overlapping with our long-press logic.
- Verdict: **Not recommended.**

### 1d. Momentum helpers (e.g. small kinetic-scroll / inertia libs, or copying VS Code's `Gesture.inertia`)
- Pros: inertia math is about 30 lines (exponential decay of velocity from a short rolling window of samples), easy to unit test deterministically with a fake clock and rAF.
- Cons: tiny libs are usually unmaintained; none know about line quantization.
- Verdict: **Build the ~30 lines ourselves, modelled on VS Code's inertia (decay friction, sample window of last ~100 ms); do not add a dependency.** Quantize to lines with a carried fractional accumulator so momentum and drag share one code path.

### 1e. Fit addon replacement (redraw after keyboard)
- `@xterm/addon-fit` is a one-shot measure-and-resize; it has no ResizeObserver of its own. The standard pattern is ResizeObserver on the container, then `fit()` in rAF, then optionally `terminal.refresh(0, rows-1)`. (INFERRED from known addon API.) Our code already debounces refit 400ms on mobile (`TerminalOutput.tsx` ~L1169 per requirements.md) and `ViewportProvider` rAF-batches `visualViewport` updates.
- Options: (i) keep fit addon, replace fixed debounce with leading-edge fit plus trailing "settled" fit (visualViewport `resize` + rAF idle) and `refresh()`; (ii) `xterm-addon-fit`-alternatives like `@xterm/addon-fit` forks: none materially better. (INFERRED)
- Verdict: **Recommended: keep addon-fit, change orchestration** (leading + trailing fit, force `refresh` and WebGL texture-atlas clear on settle). No replacement library needed. Whether the blank screen is a missed refresh or a zero-size/WebGL-context problem still needs instrumentation (requirements.md Open Questions).

## 2. SaaS
Not applicable. Nothing hosted solves terminal touch handling.

## 3. LLM-generated bespoke gesture/momentum code vs tested library
- Risk: gesture state machines and physics are the class of code where plausible-looking LLM output fails on edge cases (multi-touch, touchcancel, rounding, finger lift mid-frame). The existing bug (half-cell rounding asymmetry, remainder loss) is itself an example of exactly that, in code already in the repo.
- Mitigation if building: pure, framework-free `scrollKinematics` module (accumulator, velocity window, decay) with fake-clock unit tests (property tests: sum of emitted lines within +-1 of total finger travel; symmetric for up/down; zero drift after N frames); keep DOM listener code thin. This is cheaper and safer than adopting `@use-gesture` since correctness lives in the pure module.
- Verdict: **Build the pure kinematics module with tests (Recommended); do not hand-roll untested listener logic.**

## 4. Fork/adapt: how others do it (all UNVERIFIED, from memory; not opened this session)
- **VS Code terminal**: xterm.js viewport with `SmoothScrollableElement` plus the `Gesture` class (the same one now bundled in xterm 6; VERIFIED present in bundle). Touch scroll moves xterm scrollback only. Alt-screen apps get wheel events translated by xterm (mouse-tracking apps get wheel reports; otherwise arrow keys in alt buffer).
- **ttyd**: uses xterm.js and relies on xterm's built-in touch handling; known long-standing complaints about touch scroll in alt-screen/tmux, usually answered by enabling tmux `mouse on`. Resize via fit addon plus window `resize`.
- **wetty**: xterm.js + fit addon on window resize; no custom touch scrolling.
- **Tabby (Electron)**: desktop-first; relies on xterm viewport; no notable mobile gesture code.
- Takeaway: none ship a solution for "touch-drag scrolls the TUI's own scrollback". The closest portable approach is translating drag into wheel input (SGR mouse wheel `ESC[<64;x;yM` / `65` when mouse tracking is on; PgUp/PgDn or arrow keys otherwise), which is what xterm itself does on wheel events in the alt buffer. Recommend dispatching a synthetic `WheelEvent` on the xterm element or calling the same internal path, and verifying empirically with Claude Code which of wheel or PgUp/PgDn drives its scrollback (open question in requirements.md).
- Verdict: **Adapt the idea (drag -> wheel/PgUp-PgDn per xterm mode), not code.** Re-verify the above claims by opening the upstream repos before citing them in the plan.

## Recommendation summary
1. Fix the asymmetry with a fractional-line accumulator (small, certain).
2. Extract a pure, fake-clock-tested kinematics + inertia module (~30-60 lines); no new dependency.
3. Route output by terminal mode: normal buffer -> `scrollLines`; alt-screen with mouse tracking -> SGR wheel; alt-screen without -> PgUp/PgDn or arrow keys (decide after empirical test against Claude Code).
4. Redraw: keep addon-fit; leading+trailing fit on `visualViewport` settle, then `refresh()`; add debug-level instrumentation to prove the root cause.
5. Spike (optional, <1 h): check whether xterm 6's built-in touch scroll already handles the normal buffer correctly on-device.
