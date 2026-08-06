# ADR-001: Instrument First, Then Fix Both Root-Cause Candidates

**Status**: Accepted
**Date**: 2026-08-06

## Context

Research for `terminal-redraw-corruption` produced two independently code-verified root
causes for the same symptom (stale tail glyphs from a previous, longer status line
surviving under a new, shorter redrawn line), and the research tracks disagree on which
is primary:

- **Candidate A**: `RedrawThrottler.process()` in
  `web-app/src/lib/terminal/TerminalStreamManager.ts:42-92` unconditionally overwrites
  any pending, undelivered "full redraw" chunk (`this.pendingRedraw = chunk`) with a new
  one, dropping the old one's erase entirely if the throttle timer hasn't fired yet.
  `research/stack.md` and `research/pitfalls.md` (§4a) rate this HIGH confidence based on
  the exact code pattern. `research/architecture.md` downgrades it because the
  classifying regex (`/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/`) does not match bare `\x1b[K`,
  which this repo's own escape-code test fixtures and the likely Ink/spinner redraw idiom
  use — so the classifier may never even fire for this bug's actual byte stream.
- **Candidate B**: `broadcastControlModeUpdate` in `session/tmux/control_mode.go:683-697`
  silently drops an entire `%output` control-mode line via a non-blocking
  `select`/`default` if a subscriber's 100-slot channel is full, with only a `log.Warn`
  and no gap-detection or resync. `research/architecture.md` rates this the primary
  candidate, having also confirmed (by grep) that the existing
  `TerminalData_FlowControl` mechanism is unreachable from browser clients — there is no
  backpressure anywhere on this path today.

Neither research doc has a live-captured transcript or packet trace of the actual
corruption event; both conclusions are inferred from static code reading plus general
priors about which bug class better explains the symptom. Picking one candidate now,
purely from re-reading the same code both docs already read, would not resolve the
disagreement — it would just prefer one doc's judgment call over the other's without new
evidence.

## Decision

1. Add debug-gated (production-safe) instrumentation to **both** mechanisms first
   (Phase 1 of the implementation plan): a discard-event log with erase-footprint
   metadata in `RedrawThrottler.process()`, and a drop counter with structured fields in
   `broadcastControlModeUpdate`.
2. Implement fixes for **both** candidates regardless of what the instrumentation shows,
   because both are independently real, code-verified reliability gaps per the research
   pitfalls list:
   - Pitfall #4: any coalescing/throttling layer must never silently discard a pending
     frame whose erase footprint differs from the frame replacing it. This is true of
     `RedrawThrottler` today regardless of whether it's the cause of *this specific*
     screenshot.
   - Pitfall #7: a silent-drop backpressure path should have at least gap-detection/
     telemetry sufficient to correlate a drop with a client-visible corruption event.
     This is true of `broadcastControlModeUpdate` today regardless of whether it's the
     cause of *this specific* screenshot.
3. Use the Phase 1 telemetry to determine **sequencing/priority** between the Phase 2
   (Candidate A) and Phase 3 (Candidate B) fix epics when a real Phase 5 implementation
   session runs — whichever telemetry fires more frequently, or fires in temporal
   proximity to a reported corruption event, ships and is verified first. This does not
   change *what* gets built, only the order.

## Rationale

- Fixing both is not wasted effort: each fix closes a reliability gap that is real on
  its own evidence (unconditional overwrite in a coalescer; unlimited silent drop with no
  observability in a fan-out broadcast), independent of which one produced the specific
  screenshot in the requirements doc.
- Deferring the "which one is primary" decision to runtime telemetry, rather than to
  static-analysis judgment calls, is directly responsive to this repo's "no fix without
  root cause" discipline (`CLAUDE.md` / `.claude/CLAUDE.md` evidence rules) — asserting a
  primary cause without new evidence would be exactly the kind of overclaim those rules
  warn against.
- Instrumentation is cheap (a handful of log lines gated behind an existing debug flag /
  a single atomic counter) relative to the cost of committing to the wrong candidate and
  discovering post-fix that the symptom persists.

## Consequences

- Phase 1 adds two small, low-risk instrumentation changes before any behavioral fix
  ships — slightly increases total task count versus picking one candidate outright, but
  each task is small (3-5 minutes) per the plan's sizing constraint.
- The real Phase 5 implementation session must actually exercise the instrumentation
  (or accept it may not fire in that session, per the "production-safe if live repro
  isn't feasible" fallback in the planning brief) before treating this ADR's sequencing
  guidance as settled — this ADR documents *why* both are being fixed, not a guarantee
  that live evidence will be captured in the same session.
- If neither candidate's telemetry fires under realistic load in a later session, both
  fixes still stand as legitimate hardening — they do not need to be reverted, since they
  address pitfalls #4 and #7 independently of this specific bug's root cause.

## Alternatives Considered

- **Pick Candidate A (RedrawThrottler) as primary, skip Candidate B**: rejected —
  `architecture.md`'s regex-gap argument is a real, code-verified reason to doubt this
  classifier fires for Claude Code's actual bytes; picking A anyway on `pitfalls.md`'s
  confidence rating alone would be overclaiming past what either doc's evidence supports.
- **Pick Candidate B (control-mode drop) as primary, skip Candidate A**: rejected — the
  `RedrawThrottler` overwrite is real and independently defective regardless of whether
  it's this bug's cause; leaving it unfixed would leave a known-lossy coalescer in place.
- **Attempt a live repro before writing any plan**: considered, but the planning brief
  explicitly scopes this SDD run to phases 1-4 (no implementation), and a live Claude
  Code session repro requires the interactive/manual-testing setup described in
  `CLAUDE.md`'s "Manual/interactive testing" section, which is out of scope for a
  planning-only agent context. Production-safe instrumentation lets a *future* session
  gather the same evidence without requiring it now.
