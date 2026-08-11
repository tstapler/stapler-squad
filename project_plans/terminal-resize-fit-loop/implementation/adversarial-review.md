# Adversarial Review: terminal-resize-fit-loop (re-review after repair pass)
**Date**: 2026-07-24
**Verdict**: CLEAN

## Blockers

None remaining.

## Concerns

None remaining.

## Resolved in this pass

- **Blocker 1 (CanvasAddon unguarded construction)** — Resolved structurally, not just in
  prose. Task 3.1.2 now explicitly routes `onContextLoss` through a single
  `triggerCanvasFallback()` function instead of constructing `CanvasAddon` inline, and states
  "this is also where the try/catch around `CanvasAddon` construction lives." Task 3.2.3 wraps
  `terminal.loadAddon(new CanvasAddon())` in `try/catch`, explicitly "mirroring the existing
  `WebglAddon` construction pattern at `XtermTerminal.tsx:149-155`," with a defined catch branch
  (`console.error(/Canvas renderer also failed/)`, latch stays tripped, no fit retried, DOM
  renderer left active — no explicit code path needed). A new Task 4.1.5a adds a dedicated test
  asserting the catch path: no uncaught exception, `console.error` message match, latch stays
  `true`, `fit()` not called again. The AC5 GWT block (plan.md ~line 554) also pins this
  scenario. There is now exactly one `CanvasAddon` construction call site in the plan (Task
  3.2.3), eliminating the original two-uncaught-call-sites risk.

- **Blocker 2 (AC1 cross-instance perturbation has no coverage; tabs are the wrong proxy)** —
  Resolved structurally. `requirements.md`'s Constraints section now explicitly requires
  "multiple concurrently-mounted `XtermTerminal`/`TerminalOutput` instances **on the same page,
  sharing DOM layout**... explicitly **not** separate browser tabs." Plan's GWT-AC1 (~line 253)
  is rewritten around same-page shared-parent mounts with an explicit two-instance perturbation
  scenario (instance 1's `fit()` resizing the shared parent, triggering siblings' independent
  2-tick cycles). A new automated test, **Task 4.1.6**, mounts 2-3 `XtermTerminal` instances in
  one shared parent in a single `render()` call, gives instance 1's mocked `fit()` a side effect
  that shrinks the shared parent's mocked `getBoundingClientRect()`, fires the mocked
  `ResizeObserver` for all instances, and asserts **total `fit()` calls summed across all
  mounted instances** settle to a bounded count rather than growing unboundedly — directly
  matching the ask for "an automated test asserting bounded fit() calls across multiple
  concurrently-mounted instances sharing a parent." Epic 5's manual checklist (Task 5.2) is also
  corrected to use same-page session cards instead of separate tabs, with the checklist steps
  now explicitly checking that "none of the 3 re-triggers a further resize/fit cycle in the
  others."

- **Concern 1 (async `currentPaneRequest` follow-up has no error handling)** — Task 2.1.4 now
  explicitly extends the `try/catch` boundary to the `setTimeout(..., 100)` follow-up
  `pushMessage` call, routing it through the same `onError`/`handleError` path as the
  synchronous send, "rather than leaving it as a separate uncaught gap."

- **Concern 2 (Task 4.1.4 under-asserts the give-up GWT)** — Task 4.1.4 now asserts all three
  ADR-002 GWT conditions, not just `fit()` count: (a) `fit()` called exactly 0 times, (b)
  `console.warn` called once matching `/did not converge/`, and (c) **no sampler `setTimeout`
  remains pending** via `jest.getTimerCount()` — explicitly called out as needed because "a
  regression where `sampleTick()` keeps rescheduling itself past `MAX_SAMPLES` must fail this
  test even though `fit()`'s call count alone would stay 0." The task also adds a
  give-up-then-recovery assertion (a later genuine resize still converges and calls `fit()`
  once), proving the reset isn't a one-shot latch.

- **Concern 3 (background-tab timer throttling unaddressed)** — Addressed via a reasoned,
  documented tradeoff rather than a code change (appropriate for a Concern, not a correctness
  bug). ADR-002 gained a new "Background-tab timer throttling is an accepted tradeoff, not a
  gap" paragraph in its Consequences section, explaining that `ResizeObserver` rarely delivers
  while hidden and that the sampler's budget is explicitly a foreground-tab budget; browser
  throttling pausing (not violating) that budget is called out as expected behavior. Plan.md's
  §9 Unresolved Questions restates this as a residual, explicitly-accepted note (item 3), so it
  isn't silently glossed over.

- **Concern 4 (ADR-001 peer-dependency evidence weakly cited)** — Tightened. ADR-001's Context
  section now cites specific installed versions cross-checked directly against
  `package-lock.json`: `@xterm/addon-canvas@0.7.0`'s peer range `^5.0.0` against the installed
  `@xterm/xterm@^5.5.0`, plus a full cross-check against every other installed `@xterm/addon-*`
  package's version (fit `^0.10.0`, search `^0.15.0`, web-links `^0.11.0`, webgl `^0.18.0`) in
  the Rationale section (item 3), explicitly stating "no version pin conflicts."

- **Concern 5 (Task 3.2.3's `Number.isFinite` guard underspecified / no-op-implementable)** —
  Resolved via a single canonically-named, type-guarded function. `isFiniteResizeDimensions`
  (`(d: ResizeDimensions | undefined): d is ResizeDimensions`) is now defined once in the shared
  `web-app/src/lib/terminal/types.ts` module (Task 1.1.1), explicitly described as "the single
  canonical `Number.isFinite` guard on cols/rows... so the guard isn't re-described inline at
  each call site and can't be implemented as a no-op." Task 3.2.3 wires it into the
  post-Canvas-fallback RAF callback with a defined failure branch (skip the fit cycle, emit a
  specific `console.warn`), and it's also reused as the sole guard in front of the Epic 1
  sampler's confirmed-fit path per the glossary.

## Minors

- Task 4.1.6 (cross-instance test) bounds summed `fit()` calls across instances but does not
  itself assert a bound on `TerminalResize` RPC calls across multiple instances — RPC dedup is
  only tested per-instance in `useTerminalFlowControl.test.ts` (Epic 4, Story 4.3). This is a
  reasonable scoping choice (`XtermTerminal` doesn't call `resize()` directly — it goes through
  `onResize` → `TerminalOutput` → the hook), but the reviewer's original ask was "bounded
  fit()/RPC calls" and only the fit() half is asserted at the multi-instance level.
- No test directly exercises `isFiniteResizeDimensions()`'s guard-fails branch inside the
  post-fallback RAF callback (i.e., `proposeDimensions()` returning `undefined` right after a
  successful `CanvasAddon` load) — Task 4.1.5 tests `isSustainedMismatch()` and
  `triggerCanvasFallback()`'s trigger/disposal sequencing, and Task 4.1.5a tests the
  `CanvasAddon`-throws catch path, but the "guard passes vs. guard fails" branch of the
  post-swap `fit()` gate itself isn't independently pinned by a dedicated test — only implied by
  the AC5 GWT's happy-path assertion that the guard is checked before `fit()`.
