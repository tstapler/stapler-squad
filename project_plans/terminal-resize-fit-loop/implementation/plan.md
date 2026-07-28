# Implementation Plan: Terminal Resize/Fit Feedback Loop Fix

**Project**: terminal-resize-fit-loop | **Phase**: 3 — Plan
**Status**: Ready for Phase 4 (validate) / Phase 5 (implement)
**Created**: 2026-07-24
**Requirements**: `project_plans/terminal-resize-fit-loop/requirements.md`
**Research**: `project_plans/terminal-resize-fit-loop/research/*.md`
**Decisions**: `project_plans/terminal-resize-fit-loop/decisions/ADR-001-add-xterm-addon-canvas-dependency.md`,
`project_plans/terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md`

---

## 0. Creative Pass — Approach Selection

Three candidate approaches for the core dead-band/convergence mechanism:

**(a) Decoupled sampler + Reading-A 2-consecutive-same-value confirm** (the approach research
converges on). *Strength*: directly targets the root cause — no fixed point detector existed
anywhere in the pipeline (architecture research) — by requiring an actual stability observation
before committing, and (via ADR-002's decoupling) works correctly for both the one-shot-resize
case and the sustained-oscillation case. *Weakness*: more moving state (sampler chain, pending
value, sample budget) than the alternatives — more surface to get wrong, more to test.

**(b) Widen the pixel dead-band threshold + a max-fit-calls-per-second circuit breaker.**
*Strength*: trivial to implement (one constant change plus a counter-and-reset), no new
control flow. *Weakness*: doesn't fix anything — it only changes *how fast* the loop pegs the
CPU, not *whether* it exists. A wider pixel threshold still eventually gets crossed by
accumulated sub-cell jitter (the bug is about crossing an integer cell-boundary, not about
pixel magnitude per se), and a circuit breaker that caps call *rate* is exactly the
"call-rate not value-oscillation" mismatch the build-vs-buy research called out for
`lodash.debounce` — it would silently start dropping *legitimate* resizes once the cap is hit
during a real fast-moving drag, with no way to distinguish "still jittering" from "still
being legitimately resized." Rejected: treats the symptom (call frequency), not the cause
(no convergence check).

**(c) Re-entrancy guard — disconnect-and-reobserve the `ResizeObserver` during its own
callback.** *Strength*: cheaply prevents a callback from re-entering itself synchronously
(a real, if narrower, xterm.js/ResizeObserver footgun). *Weakness*: this bug isn't
synchronous re-entrancy — the loop is a debounced `setTimeout` chain, not a callback calling
itself before returning, so disconnect/reobserve around the callback body does nothing to
break the actual chain (RO fires, debounce settles, `fit()` perturbs layout, RO fires again on
the *next* macrotask, long after any observer would have been reconnected). Rejected: solves a
different, adjacent problem than the one in this bug report.

**Chosen: (a).** It is the only option that fixes the underlying absence of a convergence
check rather than changing the operating parameters of a check that doesn't exist. Its extra
state is justified because AC2's wording (verified against the exact `proposeDimensions()`
source in Phase 2 stack research) is precise enough to implement directly, and ADR-002 shows
the two structural traps in a naive version of (a) are both avoidable with a decoupled sampler
— so the "weakness" is addressable, not fundamental. Rejected approaches (b) and (c) are
recorded in the Pattern Decisions table below.

### Pattern Decisions

| Decision | Chosen | Rejected | Why |
|---|---|---|---|
| Core convergence mechanism | (a) Decoupled sampler + Reading-A 2-tick confirm | (b) Wider dead-band + rate circuit breaker; (c) RO re-entrancy guard | (b) fixes symptom not cause and risks dropping legitimate fast resizes; (c) targets a different bug shape (sync re-entrancy) than the actual debounced-chain loop |
| 2-tick "tick" source | Decoupled fixed-cadence sampler (`setTimeout`, 50ms), started by existing debounce, ticking independently thereafter | Confirming tick = next `ResizeObserver` invocation | RO doesn't fire on a fixed cadence; a clean one-shot resize would never get a confirming RO invocation, deadlocking legitimate resizes (pitfalls §2a) — see ADR-002 |
| "Repeats twice" reading | Reading A — same candidate value on two consecutive sampler ticks | Reading B — both ticks merely differ from applied | Reading B commits to still-unstable values (worked trace in pitfalls §2b shows a 78/79 oscillation firing on tick 2) — does not fix the bug — see ADR-002 |
| Sustained oscillation | Bounded give-up after `MAX_SAMPLES = 20` ticks (~1s); documented tradeoff; manual Fit (`force: true`) is the escape hatch | Force-apply after N failed confirmations | Recommended by pitfalls research as the simpler, sufficient option; avoids adding a second commit path with its own correctness questions for a bug-fix-scoped change — see ADR-002 |
| Confirmation state shape | Nullable field: `PendingConfirmation = ResizeDimensions \| null` | Discriminated union / enum (`AtRest \| PendingConfirm`) | Only two states of the *value itself* exist (something pending vs. nothing pending); "is the sampler running" is an orthogonal, independently-varying boolean (`samplerActive`) with no illegal-state interaction with pending — folding both into one enum adds ceremony without eliminating any additional illegal state (see §2 Domain Glossary) |
| WebGL fallback target renderer | Add `@xterm/addon-canvas` (`CanvasAddon`) — real canvas tier | Treat "canvas" loosely as the existing DOM-renderer fallback | AC5's literal wording says "canvas renderer"; xterm.js core only auto-falls-back WebGL→DOM, never to Canvas, so satisfying the literal requirement requires the addon — see ADR-001 |

---

## 1. System Type

Client-side stateful UI stabilization: a debounce/dead-band state machine
(`XtermTerminal.tsx`'s `ResizeObserver` handling), an RPC value-dedup layer
(`useTerminalFlowControl.resize()`), and a defensive rendering-tier fallback
(WebGL→Canvas). No new business-domain model, no server-side change, no persistence change.
Implementation is mostly small pure functions and closure-scoped state machines — see §3 for
why this plan deliberately does not reach for PoEAA/DDD-weight patterns.

---

## 2. Domain Glossary

All names below are the literal identifiers to use in code, so subsequent tasks and tests stay
consistent.

| Term | Kind | Meaning | Defined in |
|---|---|---|---|
| `ResizeDimensions` | `interface { cols: number; rows: number }` | Value type replacing raw positional `(cols, rows)` pairs wherever a size is held as *state* (not as a function argument — see below). Prevents cols/rows argument-order bugs in the new code this plan adds. | `web-app/src/lib/terminal/types.ts` (shared module — imported by both `XtermTerminal.tsx` and `useTerminalFlowControl.ts`; hoisted here per architecture-review.md Concern 1 so the hook doesn't depend on a leaf component — see Task 1.1.1) |
| `AppliedDimensions` | usage of `ResizeDimensions` | The size currently applied to the xterm `Terminal` instance — read as `{ cols: terminal.cols, rows: terminal.rows }`. Authoritative "what the terminal is now." | `XtermTerminal.tsx` (ResizeObserver closure) |
| `ProposedDimensions` | usage of `ResizeDimensions \| undefined` | The return value of `fitAddon.proposeDimensions()` — `undefined` when the terminal is unmounted, has no parent, or cell dimensions are not yet measured (per exact `@xterm/addon-fit@0.10.0` source in research/stack.md). Always integer when defined (via `Math.floor()` internally). | `XtermTerminal.tsx` |
| `PendingConfirmation` | `ResizeDimensions \| null` | The `ProposedDimensions` value observed on the previous sampler tick, not yet confirmed by a matching second tick. `null` means nothing is pending. See Pattern Decisions for why this is a nullable field, not an enum. | `XtermTerminal.tsx` (closure `let pendingProposedDims`) |
| `shouldScheduleFit` | pure exported function | `(proposed: ResizeDimensions \| undefined, applied: ResizeDimensions, pending: ResizeDimensions \| null) => { schedule: boolean; nextPending: ResizeDimensions \| null }`. Encodes the Reading-A dead-band decision. Exported specifically so tests can drive it without mounting a component or a real `ResizeObserver`. | `XtermTerminal.tsx` |
| `ResizeSampler` | conceptual name for a set of closures | The decoupled, fixed-cadence re-sampling loop: `startSamplerIfNeeded()`, `sampleTick()`, `stopSampler()`, backed by `let samplerActive`, `let sampleTimeout`, `let sampleCount`. Not a class — plain closures inside the mount effect, matching the file's existing style (`lastContainerSize`, `resizeCount`). | `XtermTerminal.tsx` |
| `SAMPLE_INTERVAL_MS` | `const = 50` | Fixed cadence (ms) of the decoupled sampler. See ADR-002. | `XtermTerminal.tsx` |
| `MAX_SAMPLES` | `const = 20` | Bounded give-up threshold (~1s of sampling) for sustained oscillation. See ADR-002. | `XtermTerminal.tsx` |
| `LastSentDimensions` | usage of `ResizeDimensions \| null`, held in `useRef` | `lastSentDimsRef.current` — the last `(cols, rows)` pair actually confirmed sent over the `TerminalResize` RPC (i.e., updated only after `pushMessage` returns without throwing). Distinct from `TerminalOutput`'s `lastResizeRef` (which tracks "last size xterm reported," a different question — see §4 Architecture Notes). | `useTerminalFlowControl.ts` |
| `force` | `resize()`'s 3rd parameter, `boolean = false` | Bypasses **both** the value-dedup check and the 200ms time throttle when `true`. Named and shaped to mirror the existing `requestFullResync(urgent: boolean = false)` precedent (`useTerminalFlowControl.ts:94`). | `useTerminalFlowControl.ts` |
| `extractCellMismatchInputs` | function (impure) | Reads `dims = (terminal as any)._core?._renderService?.dimensions` and `containerEl.getBoundingClientRect()`, returning `{ actualPxPerCol: number; expectedPxPerCol: number } \| null` (raw values, may be non-finite; `null` if `dims.css.cell.width` isn't available yet). The only place private xterm.js internals / DOM measurement are touched for this feature — split out per architecture-review.md Concern 2 so the decision logic below can be tested with plain numbers. | `XtermTerminal.tsx` |
| `isSustainedMismatch` | pure exported function | `(actualPxPerCol: number, expectedPxPerCol: number, tolerance: number) => boolean`. `Number.isFinite`-guards both inputs (returns `false` if either is non-finite — this is where the `Infinity`/`terminal.cols === 0` guard actually lives, since `proposeDimensions()` itself never returns `Infinity`), then returns `Math.abs(actualPxPerCol - expectedPxPerCol) > tolerance`. Exported so Task 4.1.5 can drive the `Infinity` boundary case directly with numeric fixtures, no mounting/mocking of `Terminal` internals required. | `XtermTerminal.tsx` |
| `isFiniteResizeDimensions` | pure exported function | `(d: ResizeDimensions \| undefined): d is ResizeDimensions` — the single canonical `Number.isFinite` guard on `cols`/`rows`, used by both the post-Canvas-fallback `fit()` guard (Task 3.2.3) and anywhere else a `ProposedDimensions` value needs validating before being trusted. Named once (architecture-review.md Concern 3 / adversarial-review.md Concern on Task 3.2.3) so the guard isn't re-described inline at each call site and can't be implemented as a no-op. | `web-app/src/lib/terminal/types.ts` (same shared module as `ResizeDimensions`) |
| `WebglMismatchTracker` | conceptual name for closures | `let webglMismatchCount`, `let webglFallbackTriggered` — one-directional latch (never re-arms) counting consecutive/cumulative mismatches and tripping the fallback at `MISMATCH_THRESHOLD = 3`. | `XtermTerminal.tsx` |
| `MISMATCH_TOLERANCE_PX` / `MISMATCH_THRESHOLD` | `const`s (`1` / `3`) | Pixel tolerance for a single mismatch sample, and count of qualifying samples before the fallback trips. | `XtermTerminal.tsx` |

**Glossary term count: 14** (`ResizeDimensions`, `AppliedDimensions`, `ProposedDimensions`,
`PendingConfirmation`, `shouldScheduleFit`, `ResizeSampler`, `SAMPLE_INTERVAL_MS`,
`MAX_SAMPLES`, `LastSentDimensions`, `force`, `extractCellMismatchInputs`,
`isSustainedMismatch`, `isFiniteResizeDimensions`, `WebglMismatchTracker`).

**Note on `ResizeDimensions` scope**: this value type is defined in the shared
`web-app/src/lib/terminal/types.ts` module (Task 1.1.1, per architecture-review.md Concern 1 —
hoisted out of `XtermTerminal.tsx` specifically so `useTerminalFlowControl.ts` can import it
without a hook-depending-on-leaf-component layering smell) and used for *internal state*
(applied / proposed / pending / last-sent) inside both `XtermTerminal.tsx` and
`useTerminalFlowControl.ts`. It is **not** used to change the public signatures of
`resize(cols, rows, force?)` or the
`onResize?: (cols: number, rows: number) => void` prop — those stay positional, because (1)
AC4 explicitly requires asserting a *literal third positional argument* (`toHaveBeenCalledWith(cols, rows, true)`)
at each call site, and (2) `onResize` has exactly one consumer in the codebase
(`TerminalOutput.tsx:645`, confirmed via repo-wide grep — no other file reads
`XtermTerminalProps.onResize`), so widening its signature to an object would be a
speculative, out-of-scope refactor with no current beneficiary.

---

## 3. Pattern Selection

This is closure-based stateful helper logic, not a Domain Model / Repository situation — no
PoEAA/DDD pattern applies here beyond ordinary value-typing. Two deliberate choices:

- **Type-driven design, scoped**: `ResizeDimensions` (in the shared
  `web-app/src/lib/terminal/types.ts` module, along with its `isFiniteResizeDimensions` guard)
  is introduced as a small value type for *state* (see glossary note above), not as a full
  object-signature refactor of every function that currently takes positional `cols, rows`.
  This keeps the type-safety win (no more `{width, height}` vs `{cols, rows}` mix-ups in the
  new sampler/dedup code, and one canonical `Number.isFinite` guard instead of scattered ad hoc
  checks) without touching unrelated call sites.
- **Nullable field over enum** for `PendingConfirmation` (see Pattern Decisions table above)
  — avoids over-modeling a two-state value as a three-or-more-state union when the extra
  states (sampler running vs. not) are already correctly represented by an independent
  boolean with no illegal combinations to rule out.

---

## 4. Architecture Notes (current-state data flow, confirmed against source at plan time)

```
ResizeObserver (XtermTerminal.tsx:259, current) → REPLACED per Epic 1
  pixel dead-band (>1px, current lines 269-272) → REPLACED by shouldScheduleFit() dead-band
  adaptive debounce setTimeout (10ms/250ms, current lines 279-293) → RETAINED, but now only
    decides when to call startSamplerIfNeeded(); no longer calls fit() itself
  fitAddonRef.current.fit() (current line 290, unconditional) → REPLACED: fit() only called
    from sampleTick() after Reading-A 2-tick confirmation
  terminal.onResize({cols, rows}) (registered XtermTerminal.tsx:233-240) → UNCHANGED
  onResizeRef.current(cols, rows) (XtermTerminal.tsx:238) → UNCHANGED (prop, positional)
  → TerminalOutput.handleTerminalResize(cols, rows) (TerminalOutput.tsx:257-328) → UNCHANGED
    own dedup vs. lastResizeRef (TerminalOutput.tsx:260-261) → UNCHANGED, different question
      ("last size xterm reported" vs. "last size actually sent over RPC")
    resize(cols, rows) (TerminalOutput.tsx:327, unforced) → gains value-dedup benefit for free
  → useTerminalFlowControl.resize(cols, rows, force?) (useTerminalFlowControl.ts:364) →
    MODIFIED per Epic 2: value-dedup vs. lastSentDimsRef checked before the existing 200ms
    time throttle (useTerminalFlowControl.ts:370-377); force bypasses both
```

Confirmed exact current line numbers at plan time (may drift by the time implementation
subagents run — re-verify with a targeted grep before each task if line numbers don't line up):
- `XtermTerminal.tsx` is 441 lines. `ResizeObserver` callback: lines 259-295. Mount effect
  cleanup: lines 300-312. WebGL addon creation (currently an un-stored local): lines 150-155.
  Cell-mismatch logging (no `Number.isFinite` guard today): lines 188-197.
- `useTerminalFlowControl.ts` is 503 lines. `resize()`: lines 364-416. `requestFullResync()`
  precedent for the `force`/`urgent` shape: lines 94-150. `lastResizeTimeRef` declared line 70.
- `TerminalOutput.tsx` is 669 lines. Three `resize(...)` call sites: line 327 (automatic,
  inside `handleTerminalResize`, stays unforced), line 351 (post-connection resync effect,
  needs `force: true`), line 510 (inside `handleManualResize`, needs `force: true`).
- `useTerminalStream.ts` is 361 lines. `resize` re-export type at line 46
  (`resize: (cols: number, rows: number) => void;`), re-export assignment at line 350
  (`resize: flowControl.resize,`). Both need the optional third `force?: boolean` parameter
  added to the type, or `resize(cols, rows, true)` calls through this re-export won't
  typecheck.
- Only one consumer of `XtermTerminalProps.onResize` exists in the repo:
  `TerminalOutput.tsx:645` (`onResize={handleTerminalResize}`).
- `web-app/src/lib/terminal/` is an existing shared module directory (`TerminalDimensionCache.ts`,
  `MessageQueue.ts`, `CircularBuffer.ts`, etc.) — `types.ts` (Task 1.1.1) is a new file added
  there, not a new top-level directory, and holds `ResizeDimensions` +
  `isFiniteResizeDimensions` so `useTerminalFlowControl.ts` can import them without depending on
  `XtermTerminal.tsx` (architecture-review.md Concern 1).
- `@xterm/addon-webgl` resolves to exact version `0.18.0` in `web-app/package-lock.json`
  (`node_modules/@xterm/addon-webgl` entry). `@xterm/addon-canvas` is absent from
  `package-lock.json` entirely (not installed) — confirms ADR-001's premise.
- `docs/PROFILING.md` exists in the repo root and is referenced by the manual-verification
  task (Epic 5).

---

## 5. Epics, Stories, Tasks

### Epic 1 — Dead-band `ResizeObserver` with decoupled-sampler 2-tick confirm (AC1, AC2)

#### Story 1.1: Pure decision function + value type

- **Task 1.1.1** — Create `web-app/src/lib/terminal/types.ts` (new shared module, alongside
  this directory's existing `TerminalDimensionCache.ts`/`MessageQueue.ts` files) exporting the
  `ResizeDimensions` interface and the pure `isFiniteResizeDimensions(d: ResizeDimensions | undefined): d is ResizeDimensions`
  guard (per architecture-review.md Concern 1 and adversarial-review.md's Task 3.2.3 concern —
  one canonical guard function, not a restated inline pattern at each call site). In
  `web-app/src/components/sessions/XtermTerminal.tsx`, import `ResizeDimensions` from this new
  module and add (near the top, after imports) the exported `ShouldScheduleFitResult`
  interface (stays local — it's `shouldScheduleFit`-specific, not a shared value type) and the
  pure `shouldScheduleFit()` function exactly as specified in ADR-002 §2. *Files*:
  `web-app/src/lib/terminal/types.ts` (new), `XtermTerminal.tsx`. *Size*: ~7 min.

- **Task 1.1.2** — Add module-level constants `SAMPLE_INTERVAL_MS = 50` and
  `MAX_SAMPLES = 20` next to `shouldScheduleFit`, with a one-line comment each pointing to
  ADR-002 for rationale. *Files*: `XtermTerminal.tsx`. *Size*: ~2 min.

#### Story 1.2: Decoupled sampler replacing the unconditional-`fit()` ResizeObserver

- **Task 1.2.1** — In the mount effect (currently lines 254-297), replace the body of the
  `ResizeObserver` construction: keep `lastContainerSize`/`resizeCount`/`resizeTimeout` and the
  existing `>1px` pixel pre-filter (still useful as a cheap early-out before waking the
  sampler — see Architecture Notes), but change what happens when the debounce settles: instead
  of `fitAddonRef.current?.fit()`, call a new `startSamplerIfNeeded()`. *Files*:
  `XtermTerminal.tsx`. *Size*: ~5 min.

- **Task 1.2.2** — Add the sampler closures (`samplerActive`, `sampleTimeout`, `sampleCount`,
  `pendingProposedDims`) and the three functions `stopSampler()`, `sampleTick()`,
  `startSamplerIfNeeded()` exactly per the algorithm in ADR-002 (§Decision, item 1), placed
  alongside `lastContainerSize`/`resizeCount` in the mount effect closure (plain `let`
  locals, not `useRef` — matches file's established style per features research, and a fresh
  effect run gets fresh state automatically on scrollback changes). `sampleTick()` calls
  `shouldScheduleFit()`, and on `schedule: true` calls `fitAddonRef.current.fit()` then
  `stopSampler()`; on `nextPending === null && !schedule` (at rest or `proposed === undefined`)
  calls `stopSampler()`; otherwise increments `sampleCount` and, if `sampleCount >= MAX_SAMPLES`,
  logs `console.warn('[XtermTerminal] Resize did not converge after 20 samples; giving up')`
  and **also calls `stopSampler()`** (identical reset to the other two branches:
  `samplerActive = false`, `pendingProposedDims = null`, `sampleCount = 0`, clear the pending
  `sampleTimeout`) — give-up abandons confirming *this* candidate, it must not permanently
  disable the sampler, since `startSamplerIfNeeded()` no-ops whenever `samplerActive` is
  already `true` (architecture-review.md Blocker: without this reset, `samplerActive` never
  clears and `fit()` becomes permanently unreachable for the remaining lifetime of the mount,
  including for a later, wholly legitimate one-shot resize); otherwise (sampleCount still under
  budget) reschedules itself via `setTimeout(sampleTick, SAMPLE_INTERVAL_MS)`. *Files*:
  `XtermTerminal.tsx`. *Size*: ~6 min.

- **Task 1.2.3** — Update the mount effect's cleanup function (currently lines 300-312) to
  also call `stopSampler()` (clearing `sampleTimeout` if pending), preventing a leaked
  `setTimeout` firing after unmount. *Files*: `XtermTerminal.tsx`. *Size*: ~2 min.

- **Task 1.2.4** — Verify (read-only, no edit) that the font-size effect
  (`XtermTerminal.tsx` ~343-349, `setTimeout(() => fitAddonRef.current?.fit(), 0)`) and the
  initial double-RAF + secondary 100ms fit block (lines 163-208) are **not** touched — both
  call `fit()` directly and intentionally bypass the `ResizeObserver`/sampler path entirely
  (per features research: "the double-RAF + secondary 100ms fit already bypass the
  ResizeObserver entirely"; the 2-tick gate is scoped to the ResizeObserver callback only).
  Confirm no regression by reading the diff after Tasks 1.2.1-1.2.3. *Files*: none (review
  step). *Size*: ~2 min.

**GWT — AC1** (verbatim: "Opening 3 terminals ... and backgrounding/resuming the tab, or
resizing the window once, does not trigger unbounded resize churn"):

```
Given 3 concurrently-mounted XtermTerminal instances **in one shared parent container on the
  same page** (same-page concurrent mounts — corrected per adversarial-review.md Blocker: the
  requirements.md verification-substitute language ("multiple concurrently-mounted
  XtermTerminal/TerminalOutput instances") anticipated same-page shared-DOM mounts as the
  correct proxy for the ticket's tiled-pane scenario, since separate browser tabs are isolated
  DOM/JS contexts structurally incapable of one instance perturbing another's layout — see
  Task 4.1.6 below and the corrected Epic 5 manual checklist), each at steady-state
  AppliedDimensions {cols: 80, rows: 24} with container size 800px × 480px
When the page is backgrounded and resumed, firing one ResizeObserver delivery per instance
  whose contentRect is unchanged (800px × 480px, same as before backgrounding)
Then startSamplerIfNeeded() runs sampleTick() once per instance; proposeDimensions() returns
  {cols: 80, rows: 24} which equals AppliedDimensions, so shouldScheduleFit returns
  {schedule: false, nextPending: null}; stopSampler() is called immediately
And fitAddonRef.current.fit() is called 0 times across all 3 instances
And no TerminalResize RPC is sent for any instance (ties into AC3 — no size change reached
  useTerminalFlowControl.resize() to even evaluate)

Given the same 3 same-page instances, but this time instance 1's fit() call changes the shared
  flex/grid parent's total size (the actual AC1 failure mechanism per requirements.md's Problem
  Statement: "each pane's resize can perturb its neighbors")
When instance 1's confirmed fit() fires, resizing the shared parent, which in turn delivers a
  new ResizeObserver entry to sibling instances 2 and 3
Then each sibling's sampler runs its own bounded 2-tick (or MAX_SAMPLES-bounded) confirmation
  cycle independently and settles to 0 further fit() calls once its own proposeDimensions()
  stabilizes — the shared-parent perturbation does not cause an unbounded cross-instance
  cascade (this is the scenario Task 4.1.6 automates; it cannot be exercised by separate-tab
  verification since tabs cannot share a DOM parent)
```

**GWT — AC2** (sub-cell jitter and boundary-flapping, Reading A specifically rejecting a
still-jittering candidate — see ADR-002 for the full derivation):

```
Given AppliedDimensions {cols: 80, rows: 24} and a WebGL glyph-width mismatch causing
  proposeDimensions() to return {cols: 81, rows: 24} on sampler tick 1
When sampler tick 1 runs: shouldScheduleFit({81,24}, {80,24}, null) →
  {schedule: false, nextPending: {81,24}} (first sighting, not yet confirmed)
  and sampler tick 2 (50ms later) runs with proposeDimensions() now returning
  {cols: 80, rows: 24} (the jitter passed): shouldScheduleFit({80,24}, {80,24}, {81,24}) →
  {schedule: false, nextPending: null} (equals applied — at rest)
Then fitAddon.fit() is called 0 times (jitter absorbed without committing to the unstable
  {81,24} candidate — this is exactly the case where Reading B would have incorrectly fired
  on tick 1's {81,24}≠{80,24} alone, without waiting for a second matching tick)
```

---

### Epic 2 — RPC value-dedup + `force` bypass (AC3, AC4)

#### Story 2.1: Value-dedup in `resize()`, checked before the time throttle

- **Task 2.1.1** — In `web-app/src/lib/hooks/useTerminalFlowControl.ts`, import
  `ResizeDimensions` from `web-app/src/lib/terminal/types.ts` (the shared module created in
  Task 1.1.1 — this hook does not import from `XtermTerminal.tsx`, avoiding the
  hook-depends-on-leaf-component layering smell flagged in architecture-review.md Concern 1)
  and add `const lastSentDimsRef = useRef<ResizeDimensions | null>(null);` next to
  `lastResizeTimeRef` (line 70). *Files*: `useTerminalFlowControl.ts`. *Size*: ~2 min.

- **Task 2.1.2** — In `resize()` (lines 364-416), add the third parameter
  `force: boolean = false`. Immediately after the existing connected-check
  (`if (!pushMessageRef.current || !isConnectedRef.current) ...`), insert the value-dedup
  check **before** the existing time-throttle block: if `!force` and
  `lastSentDimsRef.current` is non-null and equals `{cols, rows}`, `console.log(...)` a
  "resize skipped, value unchanged" message and `return` — without touching
  `lastResizeTimeRef` (per pitfalls §3: an unchanged value must not keep the throttle window
  "warm"). *Files*: `useTerminalFlowControl.ts`. *Size*: ~5 min.

- **Task 2.1.3** — Update the existing time-throttle condition (lines 374-377) to also
  short-circuit when `force` is `true`:
  `if (!force && timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) { ...; return; }`.
  *Files*: `useTerminalFlowControl.ts`. *Size*: ~2 min.

- **Task 2.1.4** — Move `lastResizeTimeRef.current = now` (currently line 381, set
  unconditionally before the throwable `pushMessage` call) to execute only **after**
  `pushMessage(...)` returns without throwing — i.e., inside the `try` block, after the
  `pushMessage(create(TerminalDataSchema, ...))` call succeeds, not before it. Add
  `lastSentDimsRef.current = { cols, rows };` at the same point (same fix, same root-cause
  class per pitfalls §3/§5: "worth fixing in the same change"). **While touching this
  function's `try/catch` boundary, also extend it to cover the async `currentPaneRequest`
  follow-up** (the `setTimeout(..., 100)` block that calls `pushMessage` a second time, on both
  the plain and `force` paths) — adversarial-review.md Concern: this follow-up currently has no
  error handling and an exception from it is uncaught, never routed through `onError`. Wrap the
  `setTimeout` callback's `pushMessage` call in its own `try/catch` that routes to the same
  `onError`/`handleError` path used by the synchronous send, rather than leaving it as a
  separate uncaught gap. *Files*: `useTerminalFlowControl.ts`. *Size*: ~6 min.

- **Task 2.1.5** — Update `UseTerminalFlowControlResult`'s `resize` type (line 25) to
  `resize: (cols: number, rows: number, force?: boolean) => void;`. *Files*:
  `useTerminalFlowControl.ts`. *Size*: ~1 min.

**GWT — AC3** (verbatim: "skips sending the TerminalResize RPC (and the follow-up
currentPaneRequest) when the incoming (cols, rows) equals the last pair actually sent —
independent of, and in addition to, the existing 200ms time throttle"):

```
Given lastSentDimsRef.current = {cols: 120, rows: 40} (set after a prior successful send)
When resize(120, 40) is called again 5ms later (force=false, well under the 200ms throttle —
  i.e., the time-throttle alone would NOT have blocked this call)
Then pushMessage is called 0 additional times (value-dedup catches it independently of the
  time throttle)
And the setTimeout(..., 100) follow-up CurrentPaneRequest is never scheduled for this call
And lastResizeTimeRef.current is left unchanged (not updated on the no-op call, per Task 2.1.4
  — avoids keeping the throttle window artificially warm)
```

#### Story 2.2: `force` param threaded through `useTerminalStream` + the 2 named call sites

- **Task 2.2.1** — In `web-app/src/lib/hooks/useTerminalStream.ts`, update the `resize` type
  in `UseTerminalStreamResult` (line 46) from `(cols: number, rows: number) => void` to
  `(cols: number, rows: number, force?: boolean) => void`. *Files*: `useTerminalStream.ts`.
  *Size*: ~2 min.

- **Task 2.2.2** — Confirm the re-export at line 350 (`resize: flowControl.resize,`) needs no
  further change beyond the type update in 2.2.1 (the runtime function already accepts the
  extra param after Epic 2 Story 2.1 — this task is verification only, read the file after
  2.2.1 to confirm no separate wrapper needs updating). *Files*: `useTerminalStream.ts`
  (read-only). *Size*: ~1 min.

- **Task 2.2.3** — In `web-app/src/components/sessions/TerminalOutput.tsx`, change the
  post-connection resync call at line 351 from `resize(currentSize.cols, currentSize.rows);`
  to `resize(currentSize.cols, currentSize.rows, true);`. Update the adjacent
  `console.log` at line 350 to mention `force=true` for debuggability. *Files*:
  `TerminalOutput.tsx`. *Size*: ~2 min.

- **Task 2.2.4** — In the same file, change the manual-Fit call inside `handleManualResize`
  (line 510) from `resize(cols, rows);` to `resize(cols, rows, true);`. *Files*:
  `TerminalOutput.tsx`. *Size*: ~2 min.

- **Task 2.2.5** — Confirm (read-only) that the automatic path at line 327
  (`resize(cols, rows);`, inside `handleTerminalResize`) is left **unforced** — it must
  benefit from Epic 2's value-dedup, not bypass it. *Files*: `TerminalOutput.tsx`
  (read-only). *Size*: ~1 min.

**GWT — AC4** (verbatim: "pass an explicit `force: true` parameter, with a regression test
asserting the literal third argument at each call site"):

```
Given TerminalOutput's post-connection-established effect fires after a reconnect, with
  lastResizeRef.current = {cols: 100, rows: 30} (same value already in lastSentDimsRef from
  before the disconnect) and only 50ms elapsed since the last send (under the 200ms throttle)
When the effect calls resize(currentSize.cols, currentSize.rows, true) i.e.
  resize(100, 30, true)
Then a regression test asserts the literal call: expect(resizeSpy).toHaveBeenCalledWith(100, 30, true)
And — per pitfalls §3, not just the literal-arg check — a second assertion confirms the RPC
  was actually observed sent despite matching lastSentDimsRef and being inside the throttle
  window: expect(pushMessageFn).toHaveBeenCalledWith(
    expect.objectContaining({ data: expect.objectContaining({ case: 'resize' }) })
  )
And the analogous test for handleManualResize asserts
  expect(resizeSpy).toHaveBeenCalledWith(cols, rows, true) using the terminal's actual
  post-fit cols/rows
```

---

### Epic 3 — WebGL mismatch tracker + one-directional Canvas fallback (AC5)

#### Story 3.0: Dependency + version check (do first, unblocks the rest of the epic)

- **Task 3.0.1** — Add `"@xterm/addon-canvas": "^0.7.0"` to `web-app/package.json`
  dependencies (alongside the other `@xterm/addon-*` entries), matching ADR-001. Run
  `npm install` (or the repo's package manager equivalent) inside `web-app/` to update
  `package-lock.json`. *Files*: `web-app/package.json`, `web-app/package-lock.json`.
  *Size*: ~3 min.

- **Task 3.0.2** — Version-check task (per pitfalls §4 action item): confirm the resolved
  `@xterm/addon-webgl` version in `web-app/package-lock.json` (`0.18.0` at plan time) postdates
  the historical `WebglAddon.dispose()` no-op bug (xterm.js #2254, fixed via #2548) and the
  GPU-memory-leak-on-dispose bug (#3889, fixed via #3890). This is a quick check, not a deep
  investigation: read `node_modules/@xterm/addon-webgl/package.json` version and cross-check
  against the addon's CHANGELOG/release notes (or the xterm.js monorepo's CHANGELOG.md, since
  addon-webgl versions track xterm.js core releases) for entries corresponding to those two
  fixes. If the version predates either fix, note it in a code comment above the
  `webglAddon.dispose()` call added in Story 3.1 and flag to the user before proceeding — do
  not silently ship a dispose-then-fallback sequence known to leak GPU memory or no-op.
  *Files*: none (verification task). *Size*: ~5 min.

#### Story 3.1: Promote `webglAddon` to a ref; wire `onContextLoss`

- **Task 3.1.1** — In `XtermTerminal.tsx`, add `import { CanvasAddon } from "@xterm/addon-canvas";`
  and add `const webglAddonRef = useRef<WebglAddon | null>(null);` alongside the existing
  `fitAddonRef`/`searchAddonRef` declarations (~line 104). *Files*: `XtermTerminal.tsx`.
  *Size*: ~2 min.

- **Task 3.1.2** — In the WebGL-addon-creation `try` block (currently lines 149-155), change
  `const webglAddon = new WebglAddon();` to assign into `webglAddonRef.current` instead of a
  local `const`, and register
  `webglAddonRef.current.onContextLoss(() => { console.warn('[XtermTerminal] WebGL context lost, falling back to canvas renderer'); triggerCanvasFallback(); })`.
  Route through the single `triggerCanvasFallback()` function defined in Story 3.2 (Task 3.2.3)
  rather than duplicating the dispose/load-`CanvasAddon` sequence inline here — this is also
  where the `try/catch` around `CanvasAddon` construction lives (adversarial-review.md
  Blocker: one call site, one guard, not two un-caught `new CanvasAddon()` invocations to keep
  in sync). Note: `onContextLoss` (real GPU-context-loss) is a distinct trigger from AC5's
  literal "sustained pixel mismatch" condition — wiring it is an intentional, cheap extension
  (per build-vs-buy.md: "both mechanisms should exist side by side... official, cheap, should
  be wired regardless"), not scope creep against AC5, since it shares the same
  `triggerCanvasFallback()` path and one-directional latch rather than introducing a second,
  parallel fallback mechanism. *Files*: `XtermTerminal.tsx`. *Size*: ~4 min.

- **Task 3.1.3** — Update the cleanup function (mount effect return, ~lines 300-312) to also
  set `webglAddonRef.current = null` (the addon itself is disposed via `terminal.dispose()`,
  which disposes all loaded addons — this just clears the ref to avoid a stale reference).
  *Files*: `XtermTerminal.tsx`. *Size*: ~2 min.

#### Story 3.2: Bespoke `Number.isFinite`-guarded mismatch tracker + fallback sequence

- **Task 3.2.1** — Add module-level constants `MISMATCH_TOLERANCE_PX = 1` and
  `MISMATCH_THRESHOLD = 3` next to the Epic 1 sampler constants, with a one-line comment
  flagging them as **provisional values, not yet validated against a real browser** (pre-mortem
  P1 #1: jsdom cannot reproduce real WebGL glyph-width mismatch magnitude, especially under
  fractional OS display scaling — Windows 125%/150%, macOS non-integer zoom — which is exactly
  the condition that produces this mismatch in the first place; the value chosen here is a
  reasonable starting point from the requirements' own "warns above a 1px tolerance" precedent,
  not measured data). Point the comment at Task 5.2 step 7 (added below) for the real-device
  validation/tuning step. *Files*: `XtermTerminal.tsx`. *Size*: ~1 min.

- **Task 3.2.1a** — Add a dev-only forced-fallback trigger so `triggerCanvasFallback()` (Task
  3.2.3) can be exercised manually without waiting for the mismatch heuristic to fire naturally
  (pre-mortem P1 #1 prevention, also closes pre-mortem #4 which is otherwise unverifiable —
  jsdom cannot run real WebGL/Canvas rendering, so this is the only way a human can visually
  confirm the Canvas tier renders correctly before shipping): guard it behind the existing
  `localStorage.getItem('debug-terminal') === 'true'` convention already used elsewhere in this
  codebase (e.g. `TerminalOutput.tsx`'s predictive-echo debug logging) — expose a
  `window.__staplerSquadForceCanvasFallback` function (or equivalent debug-menu hook, matching
  whatever debug-affordance convention is idiomatic here) that calls `triggerCanvasFallback()`
  directly, only registered when the debug flag is set. Referenced by Task 5.2 step 7. *Files*:
  `XtermTerminal.tsx`. *Size*: ~4 min.

- **Task 3.2.2** — Split the mismatch check into two functions (architecture-review.md
  Concern 2: separates DOM/xterm-internals extraction from the pure decision logic, so the
  `Number.isFinite` boundary case is unit-testable without mounting anything):
  (a) `extractCellMismatchInputs(terminal: Terminal, containerEl: HTMLElement): { actualPxPerCol: number; expectedPxPerCol: number } | null` —
  reads `dims = (terminal as any)._core?._renderService?.dimensions`; if
  `!dims?.css?.cell?.width` returns `null`; otherwise returns
  `{ actualPxPerCol: containerEl.getBoundingClientRect().width / terminal.cols, expectedPxPerCol: dims.css.cell.width }`
  (no `Number.isFinite` logic here — pure extraction; `terminal.cols === 0` simply produces
  `Infinity` and is passed through); (b) the pure exported
  `isSustainedMismatch(actualPxPerCol: number, expectedPxPerCol: number, tolerance: number): boolean` —
  returns `false` unless **both** `Number.isFinite(actualPxPerCol)` and
  `Number.isFinite(expectedPxPerCol)` are true (guards the `terminal.cols === 0` / hidden-tab
  `Infinity` case per pitfalls §4 — using `Number.isFinite`, not `Number.isNaN`, per AC5's
  explicit wording), otherwise returns `Math.abs(actualPxPerCol - expectedPxPerCol) > tolerance`.
  The call site (Task 3.2.4/3.2.5) is `extractCellMismatchInputs(...)` then, if non-null,
  `isSustainedMismatch(result.actualPxPerCol, result.expectedPxPerCol, MISMATCH_TOLERANCE_PX)`.
  *Files*: `XtermTerminal.tsx`. *Size*: ~6 min.

- **Task 3.2.3** — Add closure state `let webglMismatchCount = 0` and
  `let webglFallbackTriggered = false` inside the mount effect (same closure as the Epic 1
  sampler state). Add a `triggerCanvasFallback()` function, this being the **single** call
  site used by both the mismatch-threshold path (this story) and the `onContextLoss` handler
  (Task 3.1.2): if `webglFallbackTriggered` is already `true`, return immediately
  (one-directional latch, never re-arms, per pitfalls §4's GPU-leak-toggling concern); else set
  the flag, `webglAddonRef.current?.dispose()`, `webglAddonRef.current = null`, then attempt
  `terminal.loadAddon(new CanvasAddon())` **wrapped in `try/catch`, mirroring the existing
  `WebglAddon` construction pattern at `XtermTerminal.tsx:149-155`** (adversarial-review.md
  Blocker: unlike `WebglAddon`, an earlier draft of this plan left `CanvasAddon` construction
  unguarded — if it also throws, with the latch already tripped, the terminal would be
  permanently blank with no retry path and no user-facing error). On success, run
  `requestAnimationFrame(() => { ... })` (per pitfalls §4: wait one RAF frame after the addon
  swap before calling `fit()`, to avoid measuring against a not-yet-initialized renderer — the
  historical `Infinity`/crash failure mode in xterm.js #1416): inside the RAF callback, call
  `fitAddonRef.current?.proposeDimensions()`, check the result with `isFiniteResizeDimensions()`
  (the shared guard from `web-app/src/lib/terminal/types.ts`, Task 1.1.1 — the one canonical
  `Number.isFinite` check, not a restated inline pattern), and only call
  `fitAddonRef.current.fit()` when the guard passes; if it fails, skip this fit cycle and
  `console.warn('[XtermTerminal] Skipped post-fallback fit: proposed dimensions not finite')`.
  On `catch` (Canvas addon construction/load failed): log
  `console.error('[XtermTerminal] Canvas renderer also failed to load; falling back to xterm's built-in DOM renderer', err)`
  and take no further action — per build-vs-buy.md research, xterm.js core's default DOM
  renderer is already active automatically once `WebglAddon` is disposed and no other render
  addon is loaded, so no explicit DOM-fallback code is needed, only the guarantee that the
  thrown error doesn't propagate uncaught out of a `setTimeout`/`onContextLoss` callback. In
  both the success and catch paths, log the pre-existing
  `console.warn('[XtermTerminal] WebGL cell-measurement mismatch exceeded threshold, falling back to canvas renderer')`
  once, before attempting the addon swap. *Files*: `XtermTerminal.tsx`. *Size*: ~8 min.

- **Task 3.2.4** — Wire the mismatch check into the sampler: in `sampleTick()` (Epic 1 Task
  1.2.2), immediately after a confirmed `fit()` call (the `schedule: true` branch), call
  `const inputs = extractCellMismatchInputs(terminalRef.current, containerRef.current);` and,
  if `inputs` is non-null, evaluate
  `isSustainedMismatch(inputs.actualPxPerCol, inputs.expectedPxPerCol, MISMATCH_TOLERANCE_PX)`;
  if that returns `true` and `!webglFallbackTriggered`, increment `webglMismatchCount`; if
  `webglMismatchCount >= MISMATCH_THRESHOLD`, call `triggerCanvasFallback()`. This makes
  mismatch accumulate across multiple confirmed resize events (not just a single startup
  check), per architecture research point 2. *Files*: `XtermTerminal.tsx`. *Size*: ~4 min.

- **Task 3.2.5** — Update the existing startup mismatch-logging block (lines 188-197) to use
  `extractCellMismatchInputs()` + `isSustainedMismatch()` instead of its inline
  `Math.abs(...) > 1` check (removing the duplicated, unguarded logic), and route a `true`
  result through the same `webglMismatchCount`/`triggerCanvasFallback()` path used by Task
  3.2.4, so the initial-mount measurement also counts toward the threshold. *Files*:
  `XtermTerminal.tsx`. *Size*: ~4 min.

**GWT — AC5** (verbatim: "a sustained mismatch beyond a defined tolerance triggers a
one-directional fallback to the canvas renderer, using Number.isFinite ... guards against
proposeDimensions() returning Infinity"):

```
Given webglMismatchCount = 0, webglFallbackTriggered = false, MISMATCH_TOLERANCE_PX = 1,
  MISMATCH_THRESHOLD = 3, and 3 successive confirmed-fit events each measure
  actualPxPerCol = 9.2 (vs. expectedPxPerCol = 8.0, diff = 1.2 > 1)
When the 3rd mismatch event fires (webglMismatchCount reaches 3)
Then triggerCanvasFallback() runs exactly once: webglFallbackTriggered flips to true,
  webglAddonRef.current.dispose() is called once, terminal.loadAddon(new CanvasAddon()) is
  called once (inside the Task 3.2.3 try/catch — no throw in this scenario, so the catch path
  is not exercised here, see the separate CanvasAddon-failure GWT below), and after one
  requestAnimationFrame, fitAddon.proposeDimensions() is checked via isFiniteResizeDimensions()
  before fitAddon.fit() is called
And a 4th mismatch event occurring afterward does NOT call dispose()/loadAddon() again
  (checked via webglFallbackTriggered short-circuit — asserts call counts stay at 1 each)

Given terminal.cols = 0 (container hidden/backgrounded, e.g. display:none on the tab)
When extractCellMismatchInputs(terminal, containerEl) computes
  actualPxPerCol = containerEl.getBoundingClientRect().width / 0 = Infinity, then
  isSustainedMismatch(Infinity, expectedPxPerCol, MISMATCH_TOLERANCE_PX) is evaluated
Then Number.isFinite(Infinity) === false, so isSustainedMismatch returns false immediately
  and this sample is never added to webglMismatchCount (guards against the Infinity case
  exactly as AC5 requires, using Number.isFinite not Number.isNaN — Number.isNaN(Infinity) is
  false, which would have incorrectly let this sample through a NaN-only guard)

Given CanvasAddon construction throws when triggerCanvasFallback() attempts
  terminal.loadAddon(new CanvasAddon()) (adversarial-review.md Blocker — e.g. a
  canvas-fingerprinting-blocked environment or addon API mismatch)
When triggerCanvasFallback() runs
Then the try/catch added in Task 3.2.3 contains the exception: no uncaught error propagates
  out of the sampler's setTimeout chain or the onContextLoss handler, console.error logs a
  message matching /Canvas renderer also failed/, webglFallbackTriggered remains true (WebGL
  stays disposed, no retry/re-arm), fitAddon.fit() is not called again for this fallback
  attempt, and xterm.js's built-in DOM renderer is left as the active renderer (no code path
  needed to explicitly select it — confirmed by build-vs-buy.md research)
```

---

### Epic 4 — Regression test coverage (AC6)

#### Story 4.1: Shared `ResizeObserver` mock + `XtermTerminal.test.tsx`

- **Task 4.1.1** — Decision: add the `ResizeObserver` mock to `web-app/jest.setup.js` (shared,
  global), not per-test-file. Justification: `jest.setup.js` currently only polyfills
  `TextEncoder`/`TextDecoder` — both are also environment polyfills, not test doubles with
  assertion-relevant behavior, and `ResizeObserver` belongs in the same category (jsdom simply
  doesn't implement it; every future component test that mounts an xterm-based component would
  otherwise need to redefine the same polyfill). Tests that need to *drive* the observer
  (invoke its callback with fabricated entries) still capture the constructor's callback
  argument themselves via a `jest.fn()`-wrapped global assignment local to that test file — the
  shared polyfill only ensures `new ResizeObserver(cb)` doesn't throw in jsdom and exposes
  `observe`/`unobserve`/`disconnect` as no-op-safe methods. Add a minimal
  `class MockResizeObserver { ... }` to `jest.setup.js` assigned to `global.ResizeObserver`.
  *Files*: `web-app/jest.setup.js`. *Size*: ~5 min.

- **Task 4.1.2** — Create `web-app/src/components/sessions/__tests__/XtermTerminal.test.tsx`.
  Add the file-level test setup: mock `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-webgl`,
  `@xterm/addon-canvas`, `@xterm/addon-web-links`, `@xterm/addon-search` (per jest.config.js's
  `ts-jest`/`jsdom` setup — WebGL is unusable in jsdom, so these must be mocked at the module
  boundary, not integration-tested for real rendering, per pitfalls §5). Add a helper to
  capture the `ResizeObserver` callback registered by the component under test so individual
  tests can invoke it with fabricated `ResizeObserverEntry`-shaped objects. *Files*:
  `XtermTerminal.test.tsx`. *Size*: ~5 min.

- **Task 4.1.3** — In `XtermTerminal.test.tsx`, add unit tests for the exported
  `shouldScheduleFit()` directly (no component mount needed): (a) sub-cell jitter case from
  the AC2 GWT above — 2 calls, second equals applied, expect `fit()` not scheduled; (b)
  boundary-flapping / sustained-oscillation case from ADR-002's GWT — simulate 20 calls with
  never-repeating candidates, expect `schedule: false` on every call; (c) a genuine
  convergent resize — `proposed` differs from `applied` and repeats on the immediate next
  call, expect `schedule: true` on the second call only. *Files*: `XtermTerminal.test.tsx`.
  *Size*: ~5 min.

- **Task 4.1.4** — Add a component-level test driving the full sampler via the mocked
  `ResizeObserver` + `jest.advanceTimersByTime(SAMPLE_INTERVAL_MS)` in bounded increments
  (never `runAllTimers()`/`runOnlyPendingTimers()`, per pitfalls §5 — these can hang the Jest
  worker if the implementation under test isn't yet fully fixed). Simulate a 20-tick
  never-repeating oscillation (past `MAX_SAMPLES = 20`) and assert, per ADR-002's own GWT (all
  three, not just the first — adversarial-review.md Concern): (a) `fit()` is called **exactly
  0 times**; (b) `console.warn` is called exactly once with a message matching
  `/did not converge/`; (c) no sampler `setTimeout` remains pending after the budget is
  exhausted (Jest's pending-timer-count APIs, e.g. `jest.getTimerCount()` — a regression where
  `sampleTick()` keeps rescheduling itself past `MAX_SAMPLES` must fail this test even though
  `fit()`'s call count alone would stay 0). **Then, in the same test**, fire one more
  `ResizeObserver` delivery whose `proposeDimensions()` converges cleanly on the next two
  sampler ticks (a genuine, unrelated resize arriving after the give-up), and assert `fit()`
  **is** called exactly once for that second sequence — proving `stopSampler()`'s reset in the
  give-up branch (Task 1.2.2) actually re-arms the sampler rather than leaving it permanently
  inert (architecture-review.md Blocker regression test requirement). *Files*:
  `XtermTerminal.test.tsx`. *Size*: ~8 min.

- **Task 4.1.5** — Add tests for `isSustainedMismatch()` (pure, numeric fixtures — per
  architecture-review.md Concern 2's extraction split, Task 3.2.2) and `triggerCanvasFallback()`
  trigger/disposal sequencing (per pitfalls §5: testing the *decision logic and guard
  conditions in isolation*, explicitly not real WebGL behavior — test names should say so,
  e.g. `'triggers canvas fallback after 3 mismatch samples (mocked renderer, not real WebGL)'`).
  Cover: 3 consecutive mismatches trip the fallback exactly once; a 4th does not re-trigger;
  `isSustainedMismatch(Infinity, 8.0, 1)` and `isSustainedMismatch(9.2, NaN, 1)` both return
  `false` directly (the `terminal.cols === 0` Infinity case, now testable as a plain numeric
  fixture without mounting a `Terminal` or mocking `getBoundingClientRect`). *Files*:
  `XtermTerminal.test.tsx`. *Size*: ~5 min.

- **Task 4.1.5a** — Add a test asserting `triggerCanvasFallback()`'s `catch` path when
  `CanvasAddon` construction/loading throws (adversarial-review.md Blocker): mock
  `terminal.loadAddon` to throw once when passed a `CanvasAddon` instance, trigger the fallback
  (via 3 accumulated mismatches or a simulated `onContextLoss`), and assert: (a) no exception
  propagates out of the test (the `try/catch` added in Task 3.2.3 contains it); (b)
  `console.error` is called with a message matching `/Canvas renderer also failed/`; (c)
  `webglFallbackTriggered` is still latched `true` afterward (the WebGL addon stays disposed —
  no attempt to un-trip the latch and retry WebGL); (d) `fitAddonRef.current.fit()` is **not**
  called again after the failed `CanvasAddon` load (no RAF-guarded fit runs when the addon
  never loaded). Name the test to make the scope explicit, e.g.
  `'falls through to xterm's built-in DOM renderer without crashing when CanvasAddon also fails to load'`.
  *Files*: `XtermTerminal.test.tsx`. *Size*: ~5 min.

- **Task 4.1.6** — Add a component test mounting **2-3 `XtermTerminal` instances inside one
  shared flex/grid parent container** in the same `render()` call (adversarial-review.md
  Blocker: the only proxy in this codebase able to exercise cross-instance perturbation —
  requirements.md's own verification-substitute language already anticipated same-page mounts,
  not separate tabs). Give the mocked `fitAddon.fit()` for instance 1 a side effect that
  shrinks the shared parent's mocked `getBoundingClientRect()` width (simulating instance 1's
  fit changing the shared layout), fire the mocked `ResizeObserver` callback for all instances
  (simulating the shared-parent resize reaching every sibling), and — using
  `jest.advanceTimersByTime(SAMPLE_INTERVAL_MS)` in bounded increments, never
  `runAllTimers()`/`runOnlyPendingTimers()` — assert total `fit()` calls **summed across all
  mounted instances** settle to a bounded count (e.g. at most 1 per instance) rather than
  growing unboundedly across repeated ticks. This is the automated counterpart to the corrected
  Epic 5 manual checklist (Task 5.2) and closes the AC1 cross-instance-perturbation coverage
  gap identified in adversarial-review.md. *Files*: `XtermTerminal.test.tsx`. *Size*: ~8 min.

#### Story 4.2: `TerminalOutput.test.tsx` — force-bypass call-site regression tests

- **Task 4.2.1** — Create `web-app/src/components/sessions/__tests__/TerminalOutput.test.tsx`.
  Mock `useTerminalStream` (returning a `jest.fn()` for `resize` and other required fields) so
  `TerminalOutput` can be rendered without a real ConnectRPC connection, following the mocking
  conventions already used in `useTerminalFlowControl.test.ts` (protobuf/module mocks). *Files*:
  `TerminalOutput.test.tsx`. *Size*: ~5 min.

- **Task 4.2.2** — Test asserting the literal `force: true` argument at the post-connection
  resync call site (line 351): drive the component from disconnected→connected, assert
  `expect(resizeMock).toHaveBeenCalledWith(expect.any(Number), expect.any(Number), true)` for
  that specific invocation. *Files*: `TerminalOutput.test.tsx`. *Size*: ~4 min.

- **Task 4.2.3** — Test asserting the literal `force: true` argument at the manual-Fit call
  site (line 510): trigger `handleManualResize` (via the Fit button's click handler / testid),
  assert `expect(resizeMock).toHaveBeenCalledWith(cols, rows, true)` using the mocked
  terminal's actual post-fit `cols`/`rows`. *Files*: `TerminalOutput.test.tsx`. *Size*: ~4 min.

- **Task 4.2.4** — Test asserting the automatic path (line 327, inside
  `handleTerminalResize`) is called **without** a third argument (or explicitly `false`):
  `expect(resizeMock).toHaveBeenCalledWith(cols, rows)` (2-arg form) or
  `toHaveBeenCalledWith(cols, rows, undefined)` depending on how the mock records omitted
  args — pick the form matching actual `jest` call-recording behavior for an omitted optional
  parameter and assert it explicitly, so a future edit accidentally adding `force: true` here
  is caught. *Files*: `TerminalOutput.test.tsx`. *Size*: ~3 min.

#### Story 4.3: Extend `useTerminalFlowControl.test.ts`

- **Task 4.3.0** — **(Prerequisite, discovered during Phase 4 engineering triad review, fixed
  and verified during planning — `git log`/PR diff will show this predates Epic 1-3
  implementation)**: `useTerminalFlowControl.test.ts`'s `jest.mock('@/gen/session/v1/events_pb', ...)`
  exported plain classes (a protobuf-es v1 shape) but the hook's real code calls
  `create(TerminalDataSchema, init)` from `@bufbuild/protobuf` (a v2-style schema-descriptor
  API, introduced by the "adopt Redux Toolkit + protobuf-es v2" migration commit `53327064`) —
  since the mock never exported `TerminalDataSchema`/`TerminalResizeSchema`/etc., every
  `create(...)` call in the hook received `undefined` as its schema argument, threw, and was
  silently swallowed by the hook's own `try/catch`, meaning `pushMessage` was never actually
  invoked. This was **not caused by this project's changes** but was a real, pre-existing
  defect (6 of 11 tests in this file failed before Epic 1-4 touched anything, verified by
  running the suite directly) that Epic 2 Task 2.1.4 and Epic 4 Tasks 4.3.1-4.3.3 would
  otherwise have silently inherited — new tests added to the same `describe('resize', ...)`
  block would fail for a reason unrelated to whether the new dedup/force logic is correct,
  defeating AC6's "every test passes" gate. Fix: add
  `jest.mock('@bufbuild/protobuf', () => ({ create: (_schema: any, init: any) => init }))`
  above the existing `events_pb` mock, bypassing real schema-based construction entirely (the
  test only needs the resulting plain object's shape, not real protobuf serialization).
  Verified: all 11 pre-existing tests in this file pass after the fix, with no other change to
  the file. *Files*: `useTerminalFlowControl.test.ts`. *Size*: ~3 min (already applied).

- **Task 4.3.1** — Add to the existing `describe('resize', ...)` block (lines 126-177): a test
  for value-dedup — call `resize(120, 40)`, then `resize(120, 40)` again with
  `jest.advanceTimersByTime(201)` in between (past the 200ms throttle, isolating dedup as the
  actual blocker) — assert `pushMessageFn` call count does not increase on the second call.
  *Files*: `useTerminalFlowControl.test.ts`. *Size*: ~4 min.

- **Task 4.3.2** — Add a test mirroring the existing `'should allow urgent resync to bypass
  throttle'` test (line 198) for `resize`'s new `force` parameter: call `resize(100, 30)`,
  then immediately (0ms elapsed, same value) call `resize(100, 30, true)` — assert
  `pushMessageFn` **is** called the second time (force bypasses both guards), and that the
  underlying message is a `TerminalResize` with `cols: 100, rows: 30`. *Files*:
  `useTerminalFlowControl.test.ts`. *Size*: ~4 min.

- **Task 4.3.3** — Add a test for the reordered `lastResizeTimeRef`/`lastSentDimsRef` update
  timing (Task 2.1.4): mock `pushMessageRef.current` to throw once, call `resize(90, 20)`,
  assert the hook's `onError` fires and a subsequent `resize(90, 20)` call (same value) is
  **not** deduped (since the throwing call never actually updated `lastSentDimsRef`) —
  proving the "update only after success" ordering prevents the false-dedup hazard from
  pitfalls §3. *Files*: `useTerminalFlowControl.test.ts`. *Size*: ~5 min.

**GWT — AC6** (verbatim: "Regression coverage ... simulates sub-cell jitter,
boundary-flapping, WebGL mismatch escalation, and force-bypass call sites, each asserting
fit()/RPC/dispose calls fire at most the expected number of times — not once per observed
frame"):

```
Given the 4 scenario categories above are each implemented as a Jest test (Tasks 4.1.3, 4.1.4,
  4.1.5, 4.2.2/4.2.3, 4.3.1/4.3.2)
When the full web-app test suite runs via `cd web-app && npm test`
Then every test in XtermTerminal.test.tsx, TerminalOutput.test.tsx, and the extended
  useTerminalFlowControl.test.ts passes, each asserting a call-count ceiling
  (toHaveBeenCalledTimes(N) with N bounded, e.g. 0 for the pure-jitter case, exactly 1 for the
  fallback-trip case) rather than "eventually settles" — matching the pitfalls-research
  warning that jest.runAllTimers()/runOnlyPendingTimers() must never be used on these tests
  (would hang the worker on a not-yet-fixed implementation instead of failing with a useful
  assertion)
And no test uses jest.runAllTimers() or jest.runOnlyPendingTimers() (grep-verifiable:
  `grep -rn "runAllTimers\|runOnlyPendingTimers" web-app/src/components/sessions/__tests__/ web-app/src/lib/hooks/__tests__/` returns no matches)
```

---

### Epic 5 — ADRs + manual verification (AC7, plus the ADR-writing step)

- **Task 5.1** — (Already done as part of this planning pass — no implementation subagent
  action needed.) ADR-001 and ADR-002 are written to
  `project_plans/terminal-resize-fit-loop/decisions/ADR-001-add-xterm-addon-canvas-dependency.md`
  and
  `project_plans/terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md`.

- **Task 5.2** — Manual verification checklist (not automated — run once after Epics 1-4 are
  merged and `make restart-web` is running). **Corrected per adversarial-review.md Blocker**:
  use same-page concurrent mounts (multiple session cards in one dashboard view at
  `localhost:8543`), not separate browser tabs — separate tabs are isolated DOM/JS contexts and
  structurally cannot exhibit AC1's actual failure mechanism (one pane's resize perturbing a
  sibling sharing a layout parent); same-page multi-session mounts are the only proxy in this
  codebase that can.
  1. In one browser tab, open (or arrange, if the dashboard view supports it) 3 stapler-squad
     session cards **on the same page**, each terminal reaching a steady-state size (e.g.
     `158x42`).
  2. Background the browser tab (switch to another application/tab) for ~10s, then resume.
     Watch the browser console (with `localStorage.setItem('debug-terminal', 'true')` per the
     debug-menu docs) for `[XtermTerminal]` resize/fit log lines across all 3 same-page
     instances — confirm they stop within ~1s of resuming (bounded, not continuous).
  3. Resize the OS browser window once (drag a corner, release), so all 3 same-page instances
     receive a `ResizeObserver` delivery simultaneously. Confirm each terminal reflows to its
     new size within ~1s and stops logging fit activity afterward, **and** that none of the 3
     re-triggers a further resize/fit cycle in the others (the cross-instance-perturbation
     check AC1 is actually about).
  4. During and after both scenarios, confirm typed input in each of the 3 terminals remains
     responsive (no perceptible input lag).
  5. Reference `docs/PROFILING.md` (confirmed present in this repo) for CPU-profiling steps if
     step 2 or 3 shows anything resembling continued churn — capture
     `curl http://localhost:6060/debug/pprof/goroutine?debug=2` output for comparison against
     a pre-fix baseline if available.
  6. Click the manual "Fit" button in each terminal once; confirm it still resizes correctly
     (force-bypass path still works end-to-end, not just in unit tests).
  7. **(Pre-mortem P1 #1)** Using the dev-only trigger from Task 3.2.1a
     (`localStorage.setItem('debug-terminal', 'true')` then invoke the forced-fallback hook),
     force a WebGL→Canvas trip on at least one real display and visually confirm the terminal
     keeps rendering correctly afterward (readable glyphs, correct cursor, correct
     selection/theme colors) — this is the only verification path for `CanvasAddon`'s actual
     rendering correctness, since jsdom cannot run real WebGL/Canvas (pre-mortem #4). If a
     display with fractional OS scaling (Windows 125%/150%, macOS non-integer zoom) is
     available, additionally observe the real (non-forced) `actualPxPerCol`/`expectedPxPerCol`
     console log values on that display and compare them against `MISMATCH_TOLERANCE_PX = 1`
     — if real measured mismatch is consistently far below or far above the constant, file a
     follow-up to retune `MISMATCH_TOLERANCE_PX`/`MISMATCH_THRESHOLD` rather than silently
     shipping unvalidated values (pre-mortem P1 #1). Absence of a fractionally-scaled display in
     the verifier's environment is an acceptable reason to skip only the comparison half of this
     step, not the forced-trip rendering check.

  **(Pre-mortem P1 #3 — hard gate, not a self-reported checkbox)**: this task is not complete
  until the PR description includes actual evidence for steps 2, 3, and 7 — a pasted browser
  console log excerpt (with `debug-terminal` enabled) showing the resize/sample log lines
  settling to zero, and a screenshot or brief description of the post-fallback terminal render
  from step 7. Record pass/fail for all 7 steps. The `/backlog/done-N` criterion corresponding
  to AC7 (per `.backlog-context.md`'s acceptance-criteria numbering) must not be marked complete
  without this evidence present in the PR description or a linked comment — a plain
  "manually verified, LGTM" note without the pasted log/screenshot does not satisfy this task.
  This task has no file changes beyond the PR description itself.

**GWT — AC7** (verbatim: "Manual repro from the ticket ... no longer pegs CPU or freezes
input"):

```
Given 3 stapler-squad session terminals mounted on the same page (same-page concurrent mounts,
  corrected per adversarial-review.md Blocker — not separate browser tabs, which cannot exhibit
  cross-instance perturbation), each terminal at steady-state 158x42
When the OS browser window is resized once (corner-drag + release), delivering a
  ResizeObserver entry to all 3 instances simultaneously
Then browser console shows at most a handful of "[XtermTerminal]" resize/sample log lines per
  instance, settling to none within ~1s of mouse-release (not a continuous stream), and no
  instance's settling re-triggers churn in another
And typing in any of the 3 terminals immediately after the resize registers with no
  perceptible lag
And (if profiling is needed) docs/PROFILING.md's goroutine/CPU capture steps show no runaway
  goroutine count or pegged CPU attributable to resize handling
```

---

## 6. Test Strategy Summary

| Layer | File | New/Extended | Key techniques |
|---|---|---|---|
| Pure function unit tests | `XtermTerminal.test.tsx` | New | Direct calls to `shouldScheduleFit()`, `isSustainedMismatch()`, `isFiniteResizeDimensions()` — no mounting |
| Component/sampler integration | `XtermTerminal.test.tsx` | New | Mocked `ResizeObserver`, `jest.advanceTimersByTime()` in bounded increments, `act()`; includes give-up-then-recovery (Task 4.1.4) |
| Cross-instance perturbation (AC1) | `XtermTerminal.test.tsx` | New | 2-3 instances mounted in one shared parent, mocked shared `getBoundingClientRect()`, summed call-count ceiling (Task 4.1.6) |
| WebGL fallback decision logic | `XtermTerminal.test.tsx` | New | Mocked addon surface (`onContextLoss`, `dispose()`, `loadAddon()` throwing), no real WebGL; includes `CanvasAddon`-construction-failure path (Task 4.1.5a) |
| Call-site force-bypass | `TerminalOutput.test.tsx` | New | Mocked `useTerminalStream`, literal-arg + RPC-observed assertions |
| RPC dedup/throttle/force | `useTerminalFlowControl.test.ts` | Extended | Existing `createTestOptions()` pattern, new `describe` cases in existing `resize`/mirrored `urgent` blocks |

All new async/timer-driven assertions wrap `jest.advanceTimersByTime()` in
`act()`/`await act(async () => {...})`. No test uses `jest.runAllTimers()` or
`jest.runOnlyPendingTimers()`.

---

## 7. Observability Plan

No new metrics, traces, or alerts. This is a client-side bug fix to existing always-on
behavior; the existing `console.log`/`console.warn`/`console.error` pattern already used
throughout `XtermTerminal.tsx` and `useTerminalFlowControl.ts` is extended with a small number
of new log lines (sampler give-up, canvas-fallback trip, value-dedup skip) that follow the
same `[ComponentName] message` convention already in place. No OpenTelemetry spans are added —
this is browser-side code outside the Go backend's OTel instrumentation described in
`CLAUDE.md`, and inventing telemetry for a bug fix with no operational dashboard consumer would
be scope creep.

---

## 8. Risk Control

No feature flag. This fixes existing always-on resize behavior; there is no meaningful
"gradual rollout" for a client-side terminal component used synchronously by every session
view. Standard PR revert is the rollback mechanism if a regression is found post-merge. The
`force: true` bypass at exactly 2 call sites (reconnect resync, manual Fit) is deliberately
narrow and test-pinned (Task 4.2.2-4.2.4) specifically so a future edit can't silently widen or
drop it without a failing test.

---

## 9. Unresolved Questions

None outstanding from the two tensions flagged in requirements.md — both resolved via ADR-001
(add `@xterm/addon-canvas`) and ADR-002 (decoupled sampler, Reading A, bounded give-up). Two
residual, explicitly-accepted (not silently glossed over) behavior notes carried into ADR-002's
Consequences section, restated here for visibility:

1. **No progressive resize during a slow multi-second window drag** — the terminal now snaps
   to its final size once the container stops changing for ~100ms (2 sampler ticks) or the
   20-sample budget is exhausted, rather than resizing on every intermediate debounced frame
   as the pre-fix code did. Not a stated requirement either way; flagged as a deliberate,
   minor trade-off rather than an accidental side effect.
2. **`@xterm/addon-webgl@0.18.0`'s dispose-safety against xterm.js #2254/#3889 is assessed,
   not exhaustively proven** (Task 3.0.2) — the resolved version is recent enough
   (bundled with the `@xterm/xterm@5.5.0` generation) that both historical fixes are very
   likely included, but the plan explicitly calls for a lightweight confirmation step before
   Story 3.1 proceeds rather than assuming it silently.
3. **Background-tab `setTimeout` throttling is an accepted, documented tradeoff, not a gap** —
   see ADR-002's Consequences section (added in this repair pass). Browsers clamp `setTimeout`
   in hidden tabs; the sampler's ~1s/`MAX_SAMPLES=20` give-up budget is a foreground-tab
   budget, and a tab naturally pausing the sampler while backgrounded (via browser throttling)
   is acceptable — no jitter to confirm while nothing is being observed.
4. **Cross-instance perturbation (AC1's core mechanism) is now covered by Task 4.1.6's
   same-page multi-mount component test**, correcting an earlier draft's use of a
   structurally-incapable separate-browser-tabs proxy (adversarial-review.md Blocker). The test
   mocks `fit()`'s downstream layout effect on a shared parent rather than proving it against a
   real rendered flex/grid layout end-to-end — this is the practical ceiling of what a jsdom
   component test can exercise for this scenario; the corrected Task 5.2 manual checklist
   (same-page session cards, not tabs) is the remaining real-browser confirmation.
5. **`MISMATCH_TOLERANCE_PX`/`MISMATCH_THRESHOLD` are provisional, not measured** (pre-mortem
   P1 #1) — jsdom cannot reproduce real WebGL glyph-width mismatch magnitude, so these constants
   could not be validated against real fractional-display-scaling data before implementation.
   Mitigated, not eliminated, by: (a) Task 3.2.1a's dev-only forced-fallback trigger, letting a
   human visually confirm the Canvas tier renders correctly regardless of whether the heuristic
   fires naturally; (b) Task 5.2 step 7's real-display comparison, which surfaces a follow-up
   retuning task if measured mismatch diverges meaningfully from the constant. If step 7 finds a
   large divergence, treat the constants as a known-open follow-up, not a blocker to shipping
   this fix — the loop-prevention mechanism (Epic 1's sampler) is the primary fix and does not
   depend on these constants being perfectly tuned; a mistuned tolerance only affects *when* the
   Canvas fallback trips, not whether the resize loop itself is bounded.
6. **Task 5.2's manual verification now requires pasted evidence, not a self-reported
   checkbox** (pre-mortem P1 #3) — the PR description must include console log excerpts and a
   screenshot before the corresponding `/backlog/done-N` criterion for AC7 can be marked
   complete.
