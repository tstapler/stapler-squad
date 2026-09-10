# ADR-001: `permanently_failed` is a new top-level `Status`/`SessionStatus` value, not a flag

**Date**: 2026-08-06
**Status**: Accepted
**Deciders**: session-retry-backoff planning pass

## Context

Two research docs for this project disagree on how to represent "a session has
exhausted its automated retry budget":

- `research/architecture.md` §6 recommends **option (b)**: keep `Status` unchanged
  (stays `Stopped`) and add a `PermanentlyFailed bool` flag on the new `RetryState`
  struct, read alongside `Status` — mirroring how `PauseReason string`
  (`session/instance.go:284`) and `hibernateReason string` (`:280`) already layer a
  sub-reason on top of the existing `Status` enum. Rationale given: narrower blast
  radius, no new exhaustive-switch cases across the codebase.
- `research/ux.md` §1e recommends **option 2**: a new top-level `SessionStatus`
  value (parallel to `STOPPED`/`PAUSED`/`HIBERNATED`), rendered in the primary
  (leftmost, highest scan-precedence) badge slot with its own `error`/`errorBg`
  tokens. Rationale: a flag nested inside `ReviewQueueBadge`/`AttentionReason`
  competes for the same visual slot as routine reasons like `APPROVAL_PENDING` or
  `IDLE` — exactly the "mislabeled `ReasonStale`" anti-pattern
  `research/pitfalls.md` and `research/features.md` §4 both call out as the
  precise anti-pattern this feature exists to fix (today's give-up path already
  reuses `ReasonStale` as "the closest existing reason," per a code comment at
  `session/session_driver.go:587` — a mislabeling this feature must not repeat one
  layer up).

## Decision

**Add a new top-level status value**: Go `PermanentlyFailed Status = 6`
(`session/instance.go`, next after `Restoring Status = 5`) and proto
`SESSION_STATUS_PERMANENTLY_FAILED = 10` (`proto/session/v1/types.proto`, next
free integer after `SESSION_STATUS_RESTORING = 9` — verified by reading the enum
directly; `research/stack.md`'s suggested `= 8` was already claimed by
`SESSION_STATUS_HIBERNATED` at the time of research and is stale, corrected here).

Do **not** implement it as a flag on `RetryState`/`ReviewState`, and do **not**
route it through the existing `AttentionReason`/`ReviewQueue` enum as a new
`ReasonPermanentlyFailed` value (`research/ux.md`'s rejected option 1) — the whole
point of this feature per `requirements.md`'s problem statement is that "exhausted
retries" and "any other reason a human needs to look" are currently the same
bucket, and nesting the new state one layer deeper inside the same bucket
(`AttentionReason`) doesn't fix that; it just moves the ambiguity from "which
`Status`" to "which `AttentionReason`."

## Consequences

**Blast radius accepted**: 3 exhaustive Go `switch` statements over `Status` must
gain a `case PermanentlyFailed:` (confirmed via
`grep -rn "case Stopped:" session/ server/`):
- `session/instance_status.go:136` (`GetStatusDescription`)
- `session/review_queue_poller.go:460` (`reconcileSessions`)
- `session/instance.go:58` (`Status.String()`)

**Correction (architecture-review.md Concerns, 2026-08-29)**: only 2 of the 3 have a real
`default:` fallback (`GetStatusDescription` → `"Unknown"`, `Status.String()` → `Status(%d)`).
`reconcileSessions` (`session/review_queue_poller.go:442`) has **no `default:` clause at all** —
it's a bare `case Active:`/`case Stopped:`/`case Hibernated:` switch; an unhandled status
(including `PermanentlyFailed`, if Task 2.5.1d were ever skipped) matches no case and the switch
is a silent no-op. That happens to be the *correct* behavior here (Task 2.5.1d's own reasoning:
don't auto-revive a permanently-failed session) — but it's safe by coincidence, not by a uniform
`default:` guarantee. A future status addition that actually *needs* a `reconcileSessions` branch
could reasonably skip auditing it if this ADR is read as "all three degrade safely by
construction" — it isn't; two do via `default:`, one does via no-op fallthrough that happens to
be correct today. Re-verify with a qualified grep
(`grep -rn 'case \(session\.\)\?Stopped\b' session/ server/`, not the unqualified literal — see
`implementation/plan.md` Task 2.5.1f's correction, which also caught 3 *additional* wire-boundary
switches this count originally missed) before trusting a count like this for the next status
value added after this one.
Frontend `SessionStatus` consumers (`web-app/src/components/sessions/SessionCard.tsx`,
`SessionActionsOverflow.tsx`, `SessionRow.tsx`, `SessionDetailView.tsx`,
`SessionList.tsx`, `WorkspacePeersPanel.tsx`, `web-app/src/lib/grouping/strategies.ts`,
and others — full list via `grep -rln "SessionStatus\." web-app/src`) need review for
places that branch on "is this session stopped/terminal" — `PermanentlyFailed`
must be treated as terminal-like (no restart via the normal driver path, eligible
for the same actions `Stopped` is) everywhere such a check exists, which is a
plan-time task list, not a one-line change.

**What is preserved from architecture.md's option (b)**: the *reset* mechanics it
describes (clear counters, re-invoke the existing restart path) are unaffected by
this decision — "Retry now" from `PermanentlyFailed` still just transitions the
`Status` back (to `Active` via the existing restart call chain) and clears
`RetryState` counters; only the "what does the terminal state look like while
waiting for that reset" representation changes from architecture.md's proposal.

**Alternative not chosen**: `PermanentlyFailed bool` flag on `RetryState`,
rendered via `ReviewQueueBadge`/`AttentionReason` — rejected because it fails the
UX doc's explicit test ("a `permanently_failed` session that reads as 'just
another NeedsAttention card' is exactly the silent-failure-hiding-in-plain-sight
failure mode... this feature exists to eliminate") and because the codebase
already has one cautionary example of an under-specific reason (`ReasonStale`
reused for "no better reason exists" at `session_driver.go:587`) that this
feature should not add a second instance of, one layer down.

## Verification

- `grep -rn "case Stopped:" session/ server/` → 3 sites, each with a `default:`
  fallback (verified 2026-08-06, see plan.md Phase 2 Epic 2.5 for the task
  updating each).
- `proto/session/v1/types.proto:325-351` (`enum SessionStatus`) read directly —
  next free integer confirmed as `10` (`8`=HIBERNATED, `9`=RESTORING already
  taken), not `8` as `research/stack.md` guessed before this value existed in the
  enum.
