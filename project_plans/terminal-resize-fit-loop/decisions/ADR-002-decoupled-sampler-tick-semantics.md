# ADR-002: Decoupled-sampler tick semantics for the ResizeObserver dead-band (Reading A, bounded give-up)

**Status**: Accepted
**Date**: 2026-07-24
**Project**: terminal-resize-fit-loop

## Context

AC2 requires: "`XtermTerminal`'s `ResizeObserver` only schedules `fitAddon.fit()` when
`proposeDimensions()` reports integer `cols`/`rows` that differ from the terminal's
currently-applied size AND that difference repeats on two consecutive observer ticks."

Two independent Phase 2 research passes (pitfalls §2a/§2b/§2c) flagged this wording as
ambiguous in two consequential ways, and warned that a naive implementation of either
ambiguity either **deadlocks legitimate resizes** or **fails to fix the bug it targets**.
This ADR resolves both ambiguities and records why the chosen resolution is correct.

### Ambiguity 1 — what is a "tick"?

`ResizeObserver` only fires when the observed box's size *changes*. It has no fixed cadence.
If "tick" means "one `ResizeObserver` callback invocation," a clean one-shot resize (e.g. the
user drags a window corner once and releases) produces exactly **one** RO invocation — there
is no second one coming, because the size has stopped changing. Waiting for "the same
candidate on RO invocation N and N+1" would wait forever for a confirming invocation that
structurally cannot occur, permanently blocking legitimate resizes. This is worse than the bug
being fixed.

### Ambiguity 2 — which reading of "differs AND repeats twice"?

- **Reading A ("same value twice")**: the candidate must equal the *same* value on two
  consecutive sample ticks.
- **Reading B ("differs from applied twice")**: both ticks merely need to disagree with the
  currently-applied size, regardless of whether they agree with *each other*.

Reading B does not fix the bug: in a `78/79` oscillation around `applied=80`, tick 1 sees
`78≠80` and tick 2 sees `79≠80` — both disagree with `applied`, so Reading B fires and commits
to `79`, even though `79` is not actually a stable value. The oscillation continues after
committing, and the loop is not broken.

### A related trap — gating the sampler behind the existing debounce

If the confirmation "tick" were implemented by re-using the existing adaptive-debounce
`setTimeout` (`resizeTimeout` in `XtermTerminal.tsx`, which is `clearTimeout`'d and
rescheduled on every qualifying `ResizeObserver` delivery), a container that is still actively
jittering would perpetually cancel the debounce before it ever fires even once —
confirmation state would never advance past zero samples. This is a *third* way to fail,
distinct from both ambiguities above.

## Decision

1. **A "tick" is a sample from a decoupled, fixed-cadence re-sampler** — a `setTimeout` chain
   (`SAMPLE_INTERVAL_MS = 50`) that calls `fitAddon.proposeDimensions()` again on its own
   schedule, independent of whether `ResizeObserver` fires again. The existing adaptive
   debounce (`resizeTimeout`, 10ms for the first 3 resizes then 250ms) is retained *only* to
   decide **when to start** the sampler for a burst of RO deliveries (coalescing rapid
   sub-frame RO churn into one sampler kickoff, preserving today's fast-first-resizes feel).
   Once started, the sampler's own tick chain is never reset by further RO deliveries or by
   the adaptive debounce — this directly resolves the debounce-gating trap above. A genuine
   one-shot resize gets its confirming second tick "for free" from the sampler's own schedule,
   resolving Ambiguity 1.
2. **Reading A is adopted**: a candidate must match the immediately preceding sample's
   candidate exactly (`cols` and `rows` both equal) before `fit()` is scheduled. This is
   implemented as a pure, exported function so it is directly unit-testable without mounting
   a component or driving a real/mocked `ResizeObserver`:

   ```ts
   // web-app/src/lib/terminal/types.ts (shared — see architecture-review.md Concern 1 /
   // plan.md Task 1.1.1: ResizeDimensions is hoisted here so useTerminalFlowControl.ts can
   // import it without creating a hook -> leaf-component dependency)
   export interface ResizeDimensions { cols: number; rows: number; }

   // web-app/src/components/sessions/XtermTerminal.tsx (imports ResizeDimensions from the
   // shared module above)
   export interface ShouldScheduleFitResult {
     schedule: boolean;
     nextPending: ResizeDimensions | null;
   }

   export function shouldScheduleFit(
     proposed: ResizeDimensions | undefined,
     applied: ResizeDimensions,
     pending: ResizeDimensions | null
   ): ShouldScheduleFitResult {
     if (!proposed) return { schedule: false, nextPending: null };
     if (proposed.cols === applied.cols && proposed.rows === applied.rows) {
       return { schedule: false, nextPending: null };
     }
     if (pending && pending.cols === proposed.cols && pending.rows === proposed.rows) {
       return { schedule: true, nextPending: null };
     }
     return { schedule: false, nextPending: proposed };
   }
   ```

3. **Sustained oscillation (candidate never repeats): documented tradeoff, bounded give-up.**
   The sampler caps itself at `MAX_SAMPLES = 20` ticks (~1s of wall-clock sampling at 50ms
   intervals) without a confirmation. On reaching the cap, it logs
   `console.warn('[XtermTerminal] Resize did not converge after 20 samples; giving up')` and
   stops sampling **via the same `stopSampler()` reset used by the other two exit paths**
   (`samplerActive = false`, `pendingProposedDims = null`, `sampleCount = 0`, clearing any
   pending `sampleTimeout`) — `fit()` is never called for that burst, and the terminal remains
   at its last-applied size. This reset is load-bearing, not optional: give-up abandons
   confirming *this* candidate only; it must not leave the sampler permanently inert, since
   `startSamplerIfNeeded()` is a no-op whenever `samplerActive` is already `true` (an omitted
   reset here would silently and permanently disable resizing for the rest of the mount's
   lifetime — see architecture-review.md's Blocker finding, closed in this ADR revision).
   **This give-up itself is an intentional, accepted tradeoff**: loop-safety (bounded CPU,
   bounded RPC traffic) is more important than guaranteed eventual convergence for a
   pathologically oscillating measurement, and the user's manual "Fit" button
   (`TerminalOutput.tsx`'s `handleManualResize`, ~line 496) — which calls
   `resize(cols, rows, true)` with `force: true`, bypassing this entire mechanism — is the
   documented escape hatch. The sampler restarts fresh (new 20-sample budget) the next time a
   qualifying `ResizeObserver` delivery arrives, so a container that later stops oscillating
   will still converge on its own without user intervention.

## Given-When-Then pinning the sustained-oscillation behavior

```
Given XtermTerminal's applied size is {cols: 94, rows: 24}
  and fitAddon.proposeDimensions() is stubbed to alternate between {cols: 95, rows: 24}
  and {cols: 96, rows: 24} on every successive sampler tick (never repeating the same
  value twice in a row, and never equal to applied)
When the ResizeObserver fires once (kicking off the sampler) and 20 sampler ticks elapse
  (jest.advanceTimersByTime(20 * 50) inside act(), asserted in bounded increments)
Then fitAddonRef.current.fit() is called 0 times
  and console.warn is called exactly once with a message matching /did not converge/
  and the sampler stops (no further setTimeout is pending)
  and terminal.cols / terminal.rows remain unchanged at 94/24
And, when a subsequent ResizeObserver delivery arrives whose proposeDimensions() converges
  cleanly on its next two sampler ticks (e.g. {cols: 100, rows: 24} confirmed twice), the
  sampler restarts successfully (samplerActive was reset to false by the give-up path above)
  and fitAddonRef.current.fit() is called exactly once for this new sequence — proving
  stopSampler()'s reset in the give-up branch is a true reset, not a one-shot latch
```

## Consequences

- A slow, continuous multi-second window drag will **not** progressively resize the terminal
  on every intermediate frame; it snaps to the final size once the container's size stops
  changing for two consecutive 50ms samples (~100ms after drag-end), or is abandoned if it
  never stabilizes within the 20-sample budget. This is a deliberate, minor behavior change
  from the pre-fix implementation (which called `fit()` on every debounced RO tick,
  progressively resizing during a drag) — progressive resize-during-drag was never a stated
  requirement, and reintroducing it would require calling `fit()` before confirmation, which
  is precisely the mechanism causing the bug. Documented here rather than left as a silent
  side effect.
- Added latency for a genuine settle is one `SAMPLE_INTERVAL_MS` (50ms) beyond the existing
  debounce window — assessed by Phase 2 UX research as well under the ~100ms
  "feels instantaneous" threshold.
- This ADR's chosen constants (`SAMPLE_INTERVAL_MS = 50`, `MAX_SAMPLES = 20`) are defined in
  `web-app/src/components/sessions/XtermTerminal.tsx` and should be changed only alongside a
  re-read of this ADR, since they encode the bounded-give-up tradeoff, not just a debounce
  tuning knob.
- **Background-tab timer throttling is an accepted tradeoff, not a gap.** Browsers clamp
  `setTimeout` to ≥1000ms (and further under sustained backgrounding) for hidden tabs, so if
  the sampler is actively mid-confirmation at the moment a tab backgrounds, its 20-tick budget
  could take longer than ~1s of wall-clock time to exhaust while hidden. This is acceptable:
  `ResizeObserver` rarely delivers while a tab is hidden in the first place, and a sampler that
  is naturally paused/slowed by the browser's own throttling while backgrounded produces no
  incorrect fits — there is no jitter to confirm while nothing is being observed. The ~1s /
  20-sample budget above is a **foreground-tab** budget; a backgrounded tab pausing (rather
  than violating) that budget is expected, not a defect (adversarial-review.md Concern).
