# ADR-027: Default Mailbox Buffer Capacity = 32

**Status**: Accepted
**Date**: 2026-06-30
**Deciders**: Tyler Stapler
**Relates to**: `project_plans/instance-actor-concurrency/requirements.md` (R2.7), ADR-026

---

## Context

ADR-026 settled the mailbox's data structure (a plain buffered Go channel), but none of the four
research passes pinned an exact default capacity. `research/architecture.md` §5 recommends only
a range — "small buffered channel (16-32), blocking `send`/`sendSync` for ordinary callers... a
context-bounded `sendCtx` for background pollers" — with reasoning for *why* a small buffer is
fine (a `command` is "a small struct + closure pointer, not a memory concern at this size") but
no derivation of a single number within that range. This is a genuine gap left open by the
research for planning to resolve, distinct from the snapshot field-set scope (settled explicitly
and exhaustively in `research/architecture.md` §1.2) and the command type shape (settled as
closure-over-state vs. struct-per-command in `research/architecture.md` §1.1).

## Decision

Pin the default mailbox capacity to **32** buffered commands per `Instance`, as a single named
constant (e.g. `defaultMailboxCapacity = 32`) defined once, not a magic number repeated at each
`Instance` construction site.

32 is chosen at the upper end of the recommended 16-32 range because the cost of the larger
buffer is negligible — `research/pitfalls.md` §5 establishes that a `command` (closure pointer +
small struct) at "tens of commands/sec process-wide is noise against Go's young-gen GC
throughput," so the marginal cost of 32 vs. 16 slots is immaterial — while the larger buffer gives
more headroom to absorb bursts of concurrent callers touching the same `Instance` (e.g. several
RPC handlers plus a poller tick landing close together) without an ordinary caller's blocking
`send` becoming the visible backpressure signal under everyday load. R2.7 already establishes
that this workload does not need a lock-free queue's throughput; a 32-slot buffer is sized for
"absorb an ordinary burst," not "sustain a high-throughput stream."

This is paired with the diagnostic already specified in `research/architecture.md` §5: log at
Warn when a `send` blocks more than 1 second waiting for mailbox room, including `command.name`.
That log line is the feedback mechanism for revisiting this number — exactly the signal that
would have shortened the original "UI not loading" investigation to a single log line — rather
than guessing at a larger number speculatively now.

## Consequences

### Positive
- Removes an implementation-time ambiguity ("pick a number in the recommended range") that would
  otherwise be decided ad hoc, inconsistently, at whichever PR first constructs an `Instance`'s
  mailbox.
- A single named constant makes a future profiling-driven adjustment a one-line change, not a
  multi-site grep-and-replace.
- Consistent with this migration's standing rule (R2.7, `research/pitfalls.md` §5): don't
  pre-optimize for a throughput problem with no profiling evidence; ship the simple default and
  let the Warn-level diagnostic surface a real problem if one exists.

### Negative / Accepted risk
- 32 is a judgment call within the research's recommended range, not a number derived from a load
  test against this codebase's actual poller fan-out (`CapacityMonitor`, `ReviewQueuePoller`,
  `PRStatusPoller` all ticking on independent intervals across hundreds of `Instance`s). If
  production load proves bursts routinely exceed 32 in-flight commands for a single `Instance`,
  the Warn log is expected to surface it, at which point the constant should be revisited —
  this ADR does not claim 32 is permanently correct, only that it is a reasonable, low-risk
  starting point consistent with the research's stated range and reasoning.
