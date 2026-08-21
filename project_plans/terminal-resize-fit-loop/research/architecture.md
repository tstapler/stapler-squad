# Architecture Research: Terminal Resize/Fit Feedback Loop Fix

Agent 3 (Architecture), Phase 2. No prior hotspot-analysis or architecture-review artifacts exist for these files — this is the first pass.

## 1. Current data flow (exact, line-referenced)

```
ResizeObserver (XtermTerminal.tsx:259-295)
  → pixel dead-band check (>1px width/height delta only, lines 269-272)
  → adaptive debounce setTimeout (10ms first 3 resizes, else 250ms; lines 279-293)
  → fitAddonRef.current.fit()  (line 290) — unconditional, no proposeDimensions() pre-check
  → terminal.onResize({cols, rows}) fires internally inside xterm.js if fit() actually changed cols/rows
    (registered at mount, lines 233-240) → dedups against lastSizeRef (XtermTerminal's OWN
    cols/rows ref, separate from anything in TerminalOutput/useTerminalFlowControl)
  → onResizeRef.current(cols, rows)  (XtermTerminal.tsx:238, prop `onResize`)
  → TerminalOutput's handleTerminalResize(cols, rows)  (TerminalOutput.tsx:257-328)
    → own dedup against lastResizeRef (TerminalOutput's OWN cols/rows ref — a THIRD
      independent copy of "last known size", parallel to lastSizeRef in XtermTerminal
      and dimensionSyncRef/lastResizeTimeRef in useTerminalFlowControl)
    → saveDimensions() to TerminalDimensionCache (persistence, not resize-loop relevant)
    → initial-connection-only branches (cached dims / stability wait) — early return paths,
      not part of the steady-state loop
    → if already connected and sizeChanged: calls resize(cols, rows)  (line 327)
  → useTerminalFlowControl's resize(cols, rows)  (useTerminalFlowControl.ts:364-416)
    → 200ms time-only throttle against lastResizeTimeRef (lines 370-377) — NO value check
    → pushMessage(TerminalResize RPC)  (line 382-390)
    → setTimeout(100ms) → pushMessage(CurrentPaneRequest)  (lines 393-412) — re-requests
      pane content, which re-renders downstream terminal state and can itself perturb
      container layout (e.g. scrollbar appearing/disappearing), closing the feedback loop
```

Three independent "last known size" caches exist on the path (`lastSizeRef` in
XtermTerminal, `lastResizeRef` in TerminalOutput, and the *absence* of any size-based
guard in `useTerminalFlowControl`, which only has `lastResizeTimeRef`/`dimensionSyncRef`
for timing/resync, not value-dedup). Each guard is necessary-but-not-sufficient on its
own — sub-cell jitter that produces a *new* (cols, rows) pair every tick sails through
all three because each one's job is "did the value change from last time I saw it," not
"has fit() actually converged."

**Root cause restated architecturally**: there is no fixed-point detector anywhere in
this pipeline. Every stage gates on "did X change since the last observation of X," which
is precisely the condition that's satisfied on every tick of an oscillating/non-converging
sequence. The requirements' two correctives — (a) 2-tick confirmation on integer cols/rows
before scheduling `fit()`, and (b) value-dedup in `resize()` independent of time — are the
two places where a fixed-point check is structurally missing today.

## 2. Where new state should live

**Two-consecutive-ticks state (XtermTerminal ResizeObserver): plain closure variable
inside the mount effect, NOT a `useRef`, matching existing style.**

The effect at XtermTerminal.tsx:117-315 already declares `lastContainerSize`,
`resizeCount`, and `resizeTimeout` as plain `let` bindings captured by the
`ResizeObserver` callback closure — not `useRef`s — because the effect only runs once
per mount (deps: `[scrollback]`) and the observer instance itself is the thing that needs
persistent state across its own callback invocations, not something that needs to survive
across React re-renders independent of the effect. `useRef` would work but is
inconsistent with the file's established pattern and adds indirection for no benefit
here. Recommendation: add `let pendingProposedDims: { cols: number; rows: number } | null = null;`
alongside the other closure locals (~line 256-258).

**Algorithm** (replaces the pixel-only gate at lines 269-272, or layers on top of it):
on each observer tick, call `fitAddonRef.current.proposeDimensions()` and compare its
integer `cols`/`rows` to `terminalRef.current.cols`/`.rows` — the terminal's own
`cols`/`rows` properties are already the authoritative "currently-applied size" used
throughout this file (e.g. lines 189, 277, 291); there is no need to introduce a
redundant "last applied" ref that could drift from the terminal's real state.
- If proposed equals applied → at rest; clear `pendingProposedDims = null` (this also
  correctly resets the boundary-flapping case: 80×24 → 81×24 → 80×24 → 81×24 never
  accumulates two matching proposals in a row, so `fit()` never fires).
- If proposed differs from applied and matches `pendingProposedDims` from the prior
  tick → confirmed twice → proceed to schedule (existing debounce timer can remain as
  the actual `fit()` trigger, coalescing bursts within a frame).
- If proposed differs from applied and does NOT match `pendingProposedDims` → first
  sighting; set `pendingProposedDims = proposed`, do not schedule yet.

This subsumes the existing >1px pixel check rather than needing to coexist awkwardly
with it — the cols/rows check is strictly more precise (a pixel jitter that doesn't
cross a cell boundary now produces `proposed === applied`, so it's filtered before ever
reaching the tick-counting logic).

**Value-dedup state (`resize()`): a ref inside `useTerminalFlowControl`, colocated with
`lastResizeTimeRef`.**

Requirements Q2 asks whether this should live in the hook or in `TerminalOutput`. It
must live in the hook, for the same reason the file's own doc comment gives for keeping
`stateApplicatorRef` and `isResyncingRef` together ("Bug Risk 1... preserve synchronous
ref access"): `TerminalOutput`'s `lastResizeRef` (TerminalOutput.tsx:25) already tracks
"last size xterm reported," which answers a different question than "last size actually
sent over the RPC." Those two must stay decoupled — `TerminalOutput` should be allowed to
call `resize()` redundantly (e.g. after a reconnect, where `lastResizeRef` hasn't changed
but the server needs to be told again) and have `useTerminalFlowControl` be the sole
authority on whether an RPC actually goes out. Collapsing them into one ref would
re-couple "did xterm report new dims" with "did we tell the server," which is exactly
the two concerns Q3's `force` parameter needs to be able to pull apart.

Recommendation: add `const lastSentDimsRef = useRef<{ cols: number; rows: number } | null>(null);`
next to `lastResizeTimeRef` (useTerminalFlowControl.ts:70), updated only at the point
the RPC is actually pushed (line 382, alongside the existing `lastResizeTimeRef.current = now`
at line 381).

## 3. `force` parameter design

**Signature**: `resize(cols: number, rows: number, force = false)`.

**Existing precedent to follow**: `requestFullResync(urgent: boolean = false)`
(useTerminalFlowControl.ts:94-149) already solves the identical shape of problem one
function above `resize()` in the same file — a boolean flag that bypasses a time-based
throttle, with a `console.log` distinguishing the bypass path. `resize`'s `force` should
mirror this exactly: `if (!force && timeSinceLastResize < THROTTLE_MS && ...) return;`
alongside `if (!force && dedup-check) return;`. Two independent guards, both bypassable
by the same flag, both left in place for the non-forced path.

**Should `force` bypass the time throttle too, or only the value-dedup?** Both — for
both call sites, per their stated intent:
- Reconnect-resync (TerminalOutput.tsx:348-352): fires once per reconnection, which by
  definition cannot be happening more than once every 200ms in practice (reconnection
  itself takes far longer), so bypassing the time throttle is a no-op in the common case
  but must be safe in the pathological case (rapid reconnect/disconnect flapping) — the
  whole point is "server-side state was lost, tell it the truth regardless." Blocking on
  either guard defeats that.
- Manual Fit button (TerminalOutput.tsx:496-514): an explicit, low-frequency user action.
  A user who clicks "Fit" twice within 200ms (plausible — double-click, or click-then-
  realize-nothing-happened-then-click-again) must not have the second click silently
  eaten by the time throttle; that reproduces the exact "why didn't my click do anything"
  complaint the acceptance criteria is trying to prevent.

Both guards exist to protect the *unforced*, automatic path (ResizeObserver → onResize →
handleTerminalResize → resize()) from the jitter loop. Neither guard serves a purpose on
the two explicit/one-shot call sites, so `force` should bypass both uniformly rather than
having two separate flags — that keeps the call-site code simple (`resize(cols, rows, true)`)
and matches the `urgent` precedent, which also bypasses its one guard unconditionally.

**Regression test for the literal third argument** (req 4): the two call sites are
`resize(currentSize.cols, currentSize.rows)` at TerminalOutput.tsx:351 and
`resize(cols, rows)` at TerminalOutput.tsx:510 — both currently missing a third arg
entirely. Since `TerminalOutput` doesn't have its own test file yet (see §5), this is a
new test file (`TerminalOutput.test.tsx`) asserting, via a mocked `resize` jest.fn(), that
the post-connect effect and `handleManualResize` each call it with `force: true` (or
literal `true`, depending on final param name) as the third argument — a call-signature
assertion, not a behavioral one, so it stays valid even if the dedup logic inside the hook
changes later.

## 4. WebGL → canvas fallback mechanism

**`WebglAddon.dispose()` and `onContextLoss` both already exist and are already used
elsewhere in this codebase** — `src/app/test/terminal-stress/page.tsx:106-125` has a
working precedent:
```ts
const webgl = new WebglAddon();
webgl.onContextLoss(() => {
  console.warn('WebGL context lost, falling back to canvas');
  webgl.dispose();
  setRendererType('canvas');
});
term.loadAddon(webgl);
```
This is the pattern `XtermTerminal.tsx` should follow structurally — but note a naming
trap in that precedent worth flagging for the plan phase: **that code calls the
post-dispose state "canvas" but never loads `@xterm/addon-canvas`.** `@xterm/addon-canvas`
is **not** in `package.json` (only `@xterm/addon-fit`, `@xterm/addon-search`,
`@xterm/addon-web-links`, `@xterm/addon-webgl`, `@xterm/xterm` are listed). Disposing
`WebglAddon` without loading a canvas addon does not "fall back to canvas" — it falls
back to xterm.js's built-in DOM renderer, which is real, functional, and slower under
heavy scrollback but is *not* the literal "canvas renderer" the requirement's wording
(req 5: "fallback to canvas renderer") specifies.

**Decision point for the plan phase** (flagging, not resolving, since this is
architecture research not implementation): either (a) add `@xterm/addon-canvas` as a new
dependency and actually load `CanvasAddon` on fallback — satisfies the literal
requirement text and gives a real perf-tier fallback (DOM < Canvas < WebGL) instead of a
two-tier one; or (b) keep the existing codebase's simpler "dispose → implicit DOM
renderer" pattern and treat "canvas" in the requirement as loose terminology for
"non-WebGL." Given the requirement explicitly names `@xterm/addon-canvas`-style behavior
and this is a CPU-pegging bug fix where render-path performance matters, (a) is the safer
choice, but it's a one-line dependency add + import, not an architectural blocker either
way.

**"Sustained mismatch" state placement**: the existing pixel/cell-width comparison
(XtermTerminal.tsx:188-198) runs exactly once, inside the initial-mount double-RAF +
100ms-setTimeout block, and is diagnostic-only (console.error, no action). To make the
fallback "sustained" (per req 5, not a one-shot trip on the first mismatched sample —
needed to avoid falsely tripping on a single measurement race during startup), this check
needs to be:
1. Extracted into a small reusable function, e.g. `checkWebglCellMismatch(terminal, containerEl): boolean`,
   guarded with `Number.isFinite()` on both the computed `actualPixelsPerCol` (protects
   against divide-by-zero when `terminal.cols === 0` during teardown/hidden-tab states)
   and `dims.css.cell.width` (protects against the private `_core._renderService`
   API returning `undefined` before layout is ready — already partially guarded via
   `dims?.css?.cell` optional chaining at line 178, but the *numeric comparison* at line 194
   is not currently guarded).
2. Called from both the initial-mount block AND after every confirmed `fit()` call inside
   the ResizeObserver (i.e., at the same point where the new 2-tick-confirmed fit actually
   executes) — mismatch samples need to accumulate across multiple resize events to be
   "sustained," not just be checked once at startup.
3. Backed by closure-scoped `let webglMismatchCount = 0;` and `let webglFallbackTriggered = false;`
   in the same mount-effect closure as `pendingProposedDims`, incremented when the
   mismatch exceeds tolerance and reset to 0 when a fit produces a matching measurement
   — "one-directional" (req 5) means once `webglFallbackTriggered` flips true (after N
   consecutive/cumulative mismatches), it must never flip back to loading WebGL again for
   that terminal instance's lifetime (no re-arming), which the boolean latch naturally gives.
4. To dispose the addon from inside the ResizeObserver callback (a different closure than
   where it's currently created at line 150), `webglAddon` needs to be promoted from a
   `const` local inside the mount `try` block to a ref, e.g.
   `const webglAddonRef = useRef<WebglAddon | null>(null);`, set at line 151 alongside
   the existing addon refs (`fitAddonRef`, `searchAddonRef`).

## 5. Integration points / call sites to not break

**Callers of `XtermTerminalHandle.fit()`** (the imperative method exposed at
XtermTerminal.tsx:387-389, `fitAddonRef.current?.fit()`):
- `TerminalOutput.tsx:499` — `handleManualResize()`, the manual "Fit" button. This is the
  one non-ResizeObserver caller in production code and is explicitly named in the
  requirements (force:true call site). It calls `.fit()` directly (bypassing the new
  ResizeObserver dead-band/2-tick logic entirely, since it's a different code path), then
  reads `terminal.cols`/`.rows` synchronously and calls `resize(cols, rows, true)`. This
  path is intentionally exempt from the 2-tick confirmation — it's a single deliberate
  user-triggered fit, not a stream of observer ticks, so there is nothing to "confirm
  twice" here; the fix only needs to ensure this call site's *own* `resize()` call uses
  `force: true`, not that `.fit()` itself gets gated.
- No other production call sites of the exposed `.fit()` handle method exist (confirmed
  via `grep -rn "xtermRef.current.*fit\(\)"` and `useImperativeHandle` usage across `src/`).
  `src/app/test-terminal/page.tsx` and `src/app/test/terminal-stress/page.tsx` call
  `.fit()` directly on their own locally-created `FitAddon` instances, not through
  `XtermTerminalHandle` — they don't use the `XtermTerminal` component at all, so they
  are unaffected by this change and out of scope.
- Internal (non-imperative) callers of `fitAddonRef.current?.fit()` inside XtermTerminal
  itself, also worth not breaking: the initial-mount double-RAF fit (line 184) and its
  100ms-delayed secondary fit (line 204); the font-size-change effect (line 348); the
  font-family-change effect (line 357). None of these go through the ResizeObserver, so
  the 2-tick dead-band change doesn't touch them — but the font-size/font-family effects
  each schedule a `fit()` via `setTimeout(..., 0)` specifically to "avoid synchronous
  resize events," meaning they already anticipate that their own `fit()` call will
  re-trigger the ResizeObserver on the next tick. Under the new dead-band logic this is
  actually safer than today (a single post-font-change fit needs 2 confirming ticks
  before triggering *another* fit, which won't happen because it's a one-shot dimension
  change, not a jitter).

**Callers of `useTerminalFlowControl`'s `resize` return value**:
- `useTerminalStream.ts:350` re-exports `flowControl.resize` verbatim as its own `resize`
  field (typed `(cols: number, rows: number) => void` at line 46 — **this type signature
  will need updating to include the optional third `force` param**, or TypeScript will
  reject `resize(cols, rows, true)` calls made through the `useTerminalStream` re-export).
  This is the one non-obvious ripple: `TerminalOutput.tsx` consumes `resize` from
  whatever `useTerminalStream` returns (need to confirm import chain — `TerminalOutput`
  calls `resize` directly per lines 327/351/510, presumably obtained by destructuring
  `useTerminalStream()`'s return value), so the type must be threaded through that
  interface, not just `UseTerminalFlowControlResult` (useTerminalFlowControl.ts:25).
- Three call sites inside `TerminalOutput.tsx` total: `handleTerminalResize` (line 327,
  unforced — this is the automatic/ResizeObserver-driven path that must get the
  dedup benefit), the post-connect resync effect (line 351, force:true per req 4), and
  `handleManualResize` (line 510, force:true per req 4).
- No other files call `resize` from this hook (confirmed via
  `grep -rn "useTerminalFlowControl(" src/` — only `useTerminalStream.ts` instantiates
  the hook; nothing else calls it directly).

**Test infrastructure gaps to account for in the plan**: neither `XtermTerminal.tsx` nor
`TerminalOutput.tsx` has an existing test file (only `useTerminalFlowControl.test.ts`
exists, using `renderHook`/`act` from `@testing-library/react` with fake timers and a
mocked `pushMessageRef`/`isConnectedRef` — good precedent to extend for the value-dedup
and `force` tests). `jest.setup.js` currently polyfills only `TextEncoder`/`TextDecoder`
— there is no `ResizeObserver` polyfill, so any test that wants to exercise the real
`ResizeObserver` callback in `XtermTerminal.tsx` (for the jitter/boundary-flapping
regression tests in req 6) will need either (a) a `ResizeObserver` mock added to
`jest.setup.js` or a per-test mock, or (b) the 2-tick decision logic extracted into a
small pure/exported function (e.g. `shouldScheduleFit(proposed, applied, pending)`)
that can be unit tested directly without mounting the component or driving a real/mocked
`ResizeObserver` — mirroring how `useTerminalFlowControl.test.ts` tests `resize()`'s
logic via `renderHook` without needing a real xterm `Terminal` instance (it mocks
`getTerminal` to return a plain `{ cols: 80, rows: 24 }` object). The pure-function
extraction is the lower-risk option for testability and is consistent with keeping the
new dead-band logic simple enough to reason about.

## Summary of recommended shapes (for the plan phase)

- `XtermTerminal.tsx` mount effect: add `let pendingProposedDims` closure var next to
  `lastContainerSize`/`resizeCount`; gate `fit()` scheduling on
  `proposeDimensions()` vs `terminal.cols/rows` + 2-tick match, ideally via an
  extracted/exported pure helper for testability. Promote `webglAddon` to a ref;
  add sustained-mismatch tracking (closure counter + one-directional latch) reusing the
  cell-width-mismatch calc, with `Number.isFinite` guards, called after mount AND after
  each confirmed fit.
- `useTerminalFlowControl.ts`: add `lastSentDimsRef` next to `lastResizeTimeRef`;
  change `resize` signature to `resize(cols, rows, force = false)`, mirroring the
  `requestFullResync(urgent)` precedent for how `force` bypasses both the time throttle
  and the new value-dedup uniformly. Update `UseTerminalFlowControlResult['resize']`
  type and thread the optional param through `useTerminalStream.ts:46,350`.
- `TerminalOutput.tsx`: pass `force: true` (or positional `true`) at the reconnect-resync
  call (line 351) and manual-fit call (line 510) only; leave the automatic
  `handleTerminalResize` call (line 327) unforced so it benefits from the new dedup.
- New test files needed: `XtermTerminal.test.tsx` (or a focused test of the extracted
  pure dead-band function) and `TerminalOutput.test.tsx` (call-signature assertions for
  the two `force: true` sites) — both currently absent.
