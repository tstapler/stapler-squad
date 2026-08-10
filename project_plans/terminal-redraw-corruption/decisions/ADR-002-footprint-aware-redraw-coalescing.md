# ADR-002: Footprint-Aware Flush-Before-Replace for RedrawThrottler Coalescing

**Status**: Accepted
**Date**: 2026-08-06

## Context

`RedrawThrottler` (`web-app/src/lib/terminal/TerminalStreamManager.ts:42-92`) coalesces
rapid full-screen redraw chunks to avoid flicker, holding at most one pending chunk and
flushing it after a 33ms timer. Its current replacement logic
(`this.pendingRedraw = chunk`) is unconditional: if a second full-redraw chunk arrives
before the timer fires, the first is discarded outright with no comparison of what each
chunk actually erases.

Pitfall #4 (`research/pitfalls.md` §6) identifies three options for fixing this class of
bug:

- **(a)** Always flush the pending frame before replacing it, if its erase footprint
  differs from the replacement's.
- **(b)** Merge frames by applying each one's erase set (i.e. compute a union frame that
  erases everything either original frame would have erased, then apply the newest
  frame's content on top).
- **(c)** Abandon content-sniffing coalescing entirely and rely on `requestAnimationFrame`
  (rAF)-based batching instead — batch by time, not by regex-classified content type.

## Decision

Adopt **option (a)**: when a new full-redraw chunk would replace a pending one, compare
their erase footprints (which erase-sequence families — EL variants, ED variants — each
one contains, via the same `summarizeEraseFootprint` helper introduced for
instrumentation in Phase 1 of the implementation plan). If the new chunk's footprint does
not fully cover the pending chunk's footprint, flush the pending chunk synchronously
before storing the new one. If it does fully cover it (the legitimate "these are
genuinely redundant, no data lost" case), keep the existing replace-and-re-throttle
behavior unchanged.

## Rationale

- **Correctness with minimal new surface area**: option (a) only adds a footprint
  comparison and a conditional synchronous flush — both built from a `string[]` of known
  VT sequence labels, not new parsing logic. It reuses the classification approach the
  plan already needs for instrumentation (Task 1.1.1a / Task 2.1.1a), rather than
  introducing a second, independent mechanism.
- **Avoids pitfall #1** ("no new regex to patch parsing — trust xterm.js's real parser"):
  this is a *frame-selection* decision (which of two already-received, complete chunks
  gets sent to the terminal, and in what order), not a re-implementation of VT parsing.
  The actual escape sequences are still delivered to xterm.js's parser unmodified; this
  logic never rewrites or strips bytes within a chunk.
- **Option (b) rejected**: merging two frames into a synthetic union frame requires
  correctly interleaving two different cursor-position/content streams into one coherent
  sequence — this is exactly the kind of custom escape-sequence synthesis pitfall #1 and
  pitfall #3 warn against (risk of producing an invalid or semantically wrong merged
  sequence, and of splitting mid-escape-sequence during construction). It also doesn't
  clearly preserve visual correctness if the two frames also differ in cursor-position
  targets, not just erase extent.
- **Option (c) rejected for this plan's scope**: removing content-sniffing coalescing
  entirely and relying purely on rAF batching is a larger architectural change (removing
  the `RedrawThrottler` class's core mechanism, not patching a specific defect in it) and
  risks reintroducing the flicker `RedrawThrottler` was originally built to prevent
  (`terminal-jank.md` Story 1/2 context). It may be worth revisiting in a future
  dedicated redesign, but it is disproportionate to this specific, narrowly-scoped bug
  fix and was explicitly out of scope per the requirements' constraint against
  resurrecting/duplicating unrelated sibling-plan redesigns.
- **Flush must be synchronous, not scheduled**: scheduling the flush (e.g. via another
  timer or microtask) would reintroduce a window where a third incoming chunk could again
  race the flush; flushing inline within `process()` before storing the new chunk
  guarantees ordering (old frame's effects always land before the new frame's) without
  adding a second timer to reason about.

## Consequences

- In the case where two genuinely-differing-footprint redraws arrive within the same
  33ms window, both are now written to the terminal (two `onFlush` calls instead of one)
  rather than one being silently dropped. This trades a small amount of the throttler's
  coalescing benefit (slightly more writes in this specific case) for correctness — which
  is the right tradeoff since the whole point of this bug fix is that dropped writes
  produce visible corruption, while an extra write only costs a negligible amount of
  render work.
- The footprint-subset check (`isFootprintCovered`) must be kept in sync with
  `summarizeEraseFootprint`'s label set — if a new erase-sequence family is added to one,
  it must be added to the other. This is a small, contained coupling documented in code
  comments per the implementation plan (Task 2.1.2a).
- This does not fix Candidate B (the control-mode channel drop) — that is addressed
  independently in Phase 3 of the implementation plan per ADR-001's "fix both" decision.

## Alternatives Considered

See "Decision" and "Rationale" above for options (b) and (c); both were considered and
rejected in favor of (a) for the reasons stated.
