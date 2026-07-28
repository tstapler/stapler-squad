# Manual Verification Report: terminal-resize-fit-loop

**Date**: 2026-07-27
**Environment**: Unattended backlog-pipeline agent session, no human present. This worktree's
sandbox shares its host with multiple other live, actively-orchestrated `stapler-squad` instances
(confirmed via `ps aux` — several `claude` processes are connected to running `stapler-squad --mcp`
servers on other ports/workspaces, doing unrelated work). Starting a new `stapler-squad` server
instance and driving it with a headless/automated browser in this shared environment carries a real
risk of port/resource contention with that other active work, so a full live-browser CPU-trace
verification was not completed in this pass.

## What was attempted

A first attempt was made to build and run the app directly (`go build . && ./stapler-squad`) and
drive it with Playwright to capture console output around a resize event and a `visibilitychange`
simulation. That attempt did not complete cleanly (the agent doing the attempt stalled without
producing a usable trace or report). Given the shared-environment risk noted above, this was not
retried with a second live-server attempt in this pass.

## What automated evidence exists instead

This is not a substitute for Story 5.2.1's Chrome DevTools Performance-panel pass, but it is the
strongest evidence available from this pipeline run:

- **64/64 automated tests pass** across `resizeConvergence.test.ts` (19 cases), the extended
  `useTerminalFlowControl.test.ts` (3 new AC3 cases + all pre-existing), the extended
  `TerminalOutputBug.test.tsx` (2 new AC3-caller-wiring cases), and the new
  `XtermTerminalResize.test.tsx` (8 cases covering AC1/AC4/AC5/AC6/AC7's imperative-fit path, a
  StrictMode double-mount regression case, and all 3 AC4 oscillation/backstop branches).
- The Phase 4 implementation worker **mutation-tested** the component-test suite against the real
  `XtermTerminal.tsx` (temporarily reverting the AC2 gate, the `cancelled` guard, and the backstop
  branch logic in turn) and confirmed each removal causes the corresponding test to fail — i.e.
  these tests exercise the real fix logic, not vacuous assertions.
- The spec-compliance sweep (see request_review's verification notes) independently confirmed
  AC2–AC6 are satisfied in the diff and that `docs/adr/018-webgl-oscillation-fallback-to-default-renderer.md`
  is line-by-line consistent with the shipped `webglAddonRef`/`webglFallbackTrippedRef` logic.

## AC1 (window-resize trigger)

Not verified live in this pass (see above). Automated evidence: `XtermTerminalResize.test.tsx`'s
`AC5`/`AC6` describe blocks exercise the real `ResizeObserver` wiring end-to-end (mocked xterm.js,
real component code) and confirm `fit()` settles to a bounded call count for both a sub-cell wobble
(no-op) and a genuine cell-boundary crossing (exactly one `fit()`).

## AC7 (tab-background/resume trigger)

Not verified live in this pass. Automated evidence: `XtermTerminalResize.test.tsx`'s
`AC1/AC7: imperative fit() handle is gated the same as the ResizeObserver path` describe block
directly exercises the code path `TerminalOutput.tsx`'s visibility/`visualViewport.resize` handlers
call (Epic 2.4, added specifically because AC7's tab-background/resume trigger routes through this
path, not the `ResizeObserver`) and confirms the same `shouldFit` gate applies there.

## Pixels-per-column baseline

Not captured — no live browser session was driven to completion in this pass. The original bug
report's `8.45px` (actual) vs `8.33px` (expected) mismatch is GPU/DPI-dependent and would need to be
observed on hardware reasonably close to the original reporter's, which this pipeline does not have
visibility into.

## Limitation

This report does **not** satisfy Story 5.2.1 as originally written. It documents why a full live
verification wasn't safely completable in this unattended, shared-host pipeline run, and what
automated evidence exists in its place. **A human reviewer should complete the Chrome DevTools
Performance-panel pass (3 tabs, window resize, tab background/resume, recording CPU and the
resize-log volume) before or shortly after merge**, and record the actual pixels-per-column
values observed. This is flagged explicitly in the PR description per the Product-lens triad
review's own prediction that this exact gap (a human-only verification step inside an
agent-driven pipeline) would need to be handled this way.

## Verdict

**INCONCLUSIVE — deferred to human review.** Code-level fix is implemented, reviewed twice
(architecture + adversarial), pre-mortem'd, triad-reviewed, and covered by 64 passing automated
tests including mutation-verified component tests exercising the real `ResizeObserver` and
imperative-`fit()` wiring. The one remaining gap is the live-browser CPU/input-responsiveness
observation this backlog pipeline could not safely complete unattended.
