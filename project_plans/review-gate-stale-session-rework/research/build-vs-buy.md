# Build vs. Buy Research: review-gate-stale-session-rework

**Date**: 2026-07-24

## Framing

This is not a "build vs. source an external solution" decision — there is no OSS library, SaaS API, or algorithm-correctness question involved. The relevant version of this question for an internal bug fix is: **reuse existing internal infrastructure vs. build a parallel one**. That framing is answered decisively by the architecture research.

## 1. Existing OSS library or framework

Not applicable. Staleness-threshold comparison and durable state-machine marking are simple, already-implemented internal primitives (`time.Since`, `MarkStuck`/ent). No external library would meaningfully reduce risk or effort here.

**Verdict: Not recommended (not applicable).**

## 2. SaaS / managed API

Not applicable — single-user, self-hosted, internal automation state. No external service has a role here.

**Verdict: Not recommended (not applicable).**

## 3. LLM-generated implementation vs. battle-tested library

The only "implementation" involved is a duration comparison against a `time.Duration` threshold and a call into the existing, already-tested `MarkStuck` repository method. There is no algorithm or data structure novel enough to warrant this comparison — the correct move is maximal reuse of the existing, already-tested `MarkStuck`/`FindOpenStuckStates`/`resolveStuckLogged` machinery (11 existing call sites, dedicated test file `session/ent_repository_backlog_stuck_test.go`) rather than writing new bespoke persistence or dedup logic. Writing a new, parallel "is this stale" implementation instead of calling the existing `staleWork()`-shaped helper (or a close variant) would be the "reckless bespoke code where a tested primitive already exists" case this question is designed to catch.

**Verdict: Recommended — reuse the existing `MarkStuck`/`staleWork()`-family primitives; do not write new bespoke staleness/persistence logic.**

## 4. Fork or adapt existing implementation

This is the actual applicable case, reframed for an internal-reuse context: **adapt the existing `StuckReasonStaleWork` detector/UI machinery** (built for `in_progress` items) to also cover the `review`-status, blocked-rework scenario, rather than building a new parallel `StuckReason` subsystem from scratch. Concretely:

- Adapt (reuse `StuckReasonStaleWork`, widen its status precondition) — lower total surface area, one fewer proto enum value and UI map entry, but risks conflating two UX-distinct situations under one label (see pitfalls.md #4) and risks the two detectors (`reconcileStaleWorkSessions` and the `AutoReopenAfterFailedReview` call site) racing each other if not carefully scoped (pitfalls.md #3).
- Fork (new `StuckReason` value, e.g. `stale_work_blocks_rework`, reusing the *pattern* but not the *reason constant*) — clean separation, distinct copy/urgency framing preserved, at the cost of the standard "new enum value" ceremony (proto change + `make proto-gen` + one Go switch arm + one TS map entry — all small, well-trodden per the 11 existing precedents).

**Verdict: Recommended — adapt the existing pattern (detector shape, `MarkStuck` machinery, UI components) in either sub-flavor; Phase 3 should make the "reuse the reason constant vs. add a new one" call explicitly rather than defaulting to whichever is less code to write today, since the UX distinction (pitfalls.md #4) is a real product consideration, not just an implementation detail.**

## Overall recommendation

Build nothing new at the infrastructure level. This fix is scoped correctly as: (a) a threshold-value correction, (b) wiring one existing gap in an already-built `StuckReason` pipeline, and (c) a prompt-text edit. Any implementation plan that proposes new persistence mechanisms, new UI component families, or a new notification pipeline should be treated as a scope-creep red flag against this research, not as thoroughness.
