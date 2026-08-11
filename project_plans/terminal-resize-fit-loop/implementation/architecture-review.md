# Architecture Review: terminal-resize-fit-loop (re-review after repair pass)
**Date**: 2026-07-24
**Verdict**: CONCERNS

## Blockers

None. The previously-flagged Blocker is resolved (see below).

## Concerns

- [ ] **Sampler tick/give-up state machine is still closure-only, not independently
  unit-testable** (was Concern 4 / "Story 1.2 / Story 3.2 testability"). `shouldScheduleFit()`
  remains correctly isolated as a pure, exported function and is now directly unit-tested
  (Task 4.1.3). But the surrounding orchestration — `startSamplerIfNeeded()`, `sampleTick()`,
  `stopSampler()`, and the `samplerActive`/`sampleCount`/`pendingProposedDims` state itself — is
  still specified as unexported `let`-closures inside the mount effect (plan.md §2 Domain
  Glossary: "`ResizeSampler` ... Not a class — plain closures inside the mount effect, matching
  the file's existing style"). The recommended remediation (factor the sampler into a small
  exported factory, e.g. `createResizeSampler(...)`, so the tick/give-up state machine can be
  unit-tested directly) was not adopted. The only test exercising this exact state machine —
  including the give-up→reset→recovery path where the Blocker lived — is the full
  component-mount + mocked-`ResizeObserver` + fake-timer integration test (Task 4.1.4).
  **Current risk level is much lower than before this repair pass**, because Task 4.1.4 now
  explicitly asserts the give-up-then-recovery sequence end-to-end (see "Resolved in this pass"
  below) — the exact bug class the original Blocker was about is now pinned by a test, just not
  by the fastest/most isolated form of test. Leaving open as a design-preference concern, not a
  correctness gap: if the sampler's state machine grows more branches, its closure-only shape
  will make future regressions of this kind harder to catch quickly.

## Resolved in this pass

- **Blocker (Task 1.2.2 `MAX_SAMPLES` give-up branch silently disabling the sampler)** —
  Fully resolved, on both halves of the required remediation. Task 1.2.2 now explicitly
  specifies: "and **also calls `stopSampler()`** (identical reset to the other two branches:
  `samplerActive = false`, `pendingProposedDims = null`, `sampleCount = 0`, clear the pending
  `sampleTimeout`) — give-up abandons confirming *this* candidate, it must not permanently
  disable the sampler, since `startSamplerIfNeeded()` no-ops whenever `samplerActive` is already
  `true`." ADR-002 §Decision item 3 repeats the identical reset and calls it "load-bearing, not
  optional," explicitly citing "architecture-review.md's Blocker finding, closed in this ADR
  revision." A dedicated regression test now proves recovery, not just the reset code: Task
  4.1.4 drives a 20-tick never-converging oscillation (past `MAX_SAMPLES = 20`), asserts `fit()`
  is called 0 times, `console.warn` fires exactly once matching `/did not converge/`, and no
  sampler `setTimeout` remains pending — **then, in the same test**, fires one more
  `ResizeObserver` delivery that converges cleanly on its next two ticks and asserts `fit()`
  **is** called exactly once for that second sequence, "proving `stopSampler()`'s reset in the
  give-up branch (Task 1.2.2) actually re-arms the sampler rather than leaving it permanently
  inert." ADR-002's own GWT block (lines 116-134) pins the identical scenario at the ADR level
  as well. This satisfies both halves of the original Blocker's remediation: an explicit
  `stopSampler()` call in the give-up branch, plus a regression test proving the sampler
  restarts after give-up — not just prose mentioning it.

- **Concern (`ResizeDimensions` declared in `XtermTerminal.tsx` but used untyped inline
  elsewhere)** — Resolved. `ResizeDimensions` is now defined once in a new shared module,
  `web-app/src/lib/terminal/types.ts` (Task 1.1.1), imported by both `XtermTerminal.tsx` and
  `useTerminalFlowControl.ts`. Task 2.1.1 explicitly closes the exact gap the original concern
  named — the `lastSentDimsRef` declaration is now `useRef<ResizeDimensions | null>(null)`
  importing the shared type, with the task text noting "this hook does not import from
  `XtermTerminal.tsx`, avoiding the hook-depends-on-leaf-component layering smell flagged in
  architecture-review.md Concern 1." The Domain Glossary's "Note on `ResizeDimensions` scope"
  and §3 Pattern Selection both cite this hoisting decision directly back to the concern.

- **Concern (`checkWebglCellMismatch` coupling DOM/xterm-internals extraction with pure
  decision logic)** — Resolved. Task 3.2.2 explicitly splits the check into two functions "per
  architecture-review.md Concern 2": `extractCellMismatchInputs(terminal, containerEl)` (impure
  — reads `(terminal as any)._core?._renderService?.dimensions` and
  `containerEl.getBoundingClientRect()`, returns raw numbers or `null`) and
  `isSustainedMismatch(actualPxPerCol, expectedPxPerCol, tolerance)` (pure, exported,
  `Number.isFinite`-guarded decision logic, no DOM/xterm access). Task 4.1.5 confirms the pure
  function is tested with plain numeric fixtures — including the `Infinity`/`NaN` boundary
  cases (`isSustainedMismatch(Infinity, 8.0, 1)`, `isSustainedMismatch(9.2, NaN, 1)`) — with no
  mounting or `getBoundingClientRect` mocking required, exactly matching the recommended
  remediation.

- **Concern (Task 3.2.3's fallback `fit()` guard vaguely "reuse the guard pattern")** —
  Resolved. The plan now names one concrete, shared function: `isFiniteResizeDimensions(d):
  d is ResizeDimensions`, defined once in `web-app/src/lib/terminal/types.ts` (Task 1.1.1) and
  reused verbatim at the Task 3.2.3 call site ("check the result with
  `isFiniteResizeDimensions()` (the shared guard from `web-app/src/lib/terminal/types.ts`, Task
  1.1.1 — the one canonical `Number.isFinite` check, not a restated inline pattern)"). The
  glossary entry for `isFiniteResizeDimensions` explicitly ties the naming decision back to
  "architecture-review.md Concern 3."

## Nitpicks

- The repair pass also folded in fixes attributed to a separate "adversarial-review.md" pass
  (same-page vs. separate-tab AC1 verification, `CanvasAddon` construction try/catch,
  background-tab timer-throttling documentation in ADR-002's Consequences). These are outside
  the scope of this targeted re-check (they weren't part of the architecture-review Blocker/
  Concerns being verified here) but nothing in them conflicts with or undermines the four items
  re-checked above.
- No new severe issue was found on a full read of the current plan.md and ADR-002; the repair
  pass reads as strictly additive/corrective relative to the prior version's specified content.
