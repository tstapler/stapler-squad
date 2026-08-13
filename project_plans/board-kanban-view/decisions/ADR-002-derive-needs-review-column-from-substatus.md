# ADR-002: Derive the "Needs Review" Board Column from `SubStatus`, Not the Review Queue

**Status**: Accepted
**Date**: 2026-08-06
**Project**: board-kanban-view

## Context

The item's proposed 4 columns (Running / Needs Review / Paused / Complete) don't map 1:1 onto
the real `SessionStatus` enum (`session/instance.go:24-47`; proto mirror
`proto/session/v1/types.proto:328-350`). `SESSION_STATUS_NEEDS_APPROVAL` is explicitly
deprecated, with the proto comment stating "NeedsApproval is now a sub-status" — confirming
"Needs Review" must be a derived condition, not a raw enum value.

Research found **two independent existing implementations** of "needs attention" already in
the codebase:

1. **`SubStatus`-derived** (`SessionList.tsx:572`): `session.status === ACTIVE &&
   (session.subStatus === NEEDS_APPROVAL || session.subStatus === INPUT_REQUIRED)`. Cheap —
   computed client-side from fields already present on every `Session` object returned by
   `ListSessions`/`WatchSessions`. This backs the existing "Needs approval" filter checkbox in
   list view today.
2. **The dedicated Review Queue** (`GetReviewQueue`/`WatchReviewQueue` RPCs, consumed via
   `useReviewQueueContext()`), where each `ReviewItem` carries a broader `AttentionReason`
   (`APPROVAL_PENDING`, `INPUT_REQUIRED`, `ERROR_STATE`, `IDLE_TIMEOUT`, `TASK_COMPLETE`, …)
   and its own priority/stream lifecycle. This backs the dedicated Review Queue panel/badge.

These two sets are **not guaranteed to agree** — the queue includes reasons (`TASK_COMPLETE`,
`IDLE_TIMEOUT`) that aren't "needs approval" in the `SubStatus` sense, and can contain items
whose `SessionStatus` isn't even `ACTIVE`.

## Decision

The board's "Needs Review" **column membership** test is the `SubStatus`-derived condition
(option 1 above) — i.e. `getBoardColumnKey` classifies a session as `"needs_review"` iff
`status === ACTIVE && subStatus ∈ {NEEDS_APPROVAL, INPUT_REQUIRED}`.

The Review Queue (option 2) is *not* used for column membership, but a session that also
appears in the Review Queue may still show a supplementary visual badge on its `BoardCard` (a
possible future enhancement, not required by this plan's ACs — see `implementation/plan.md`'s
Unresolved Questions for scope boundary).

Additionally, this ADR fixes the **drag semantics** of the Needs Review column, since it is
not an ordinary status bucket:
- **Dragging out** to "Running" resolves the pending approval via the existing
  `ResolveApproval` RPC (`proto/session/v1/session.proto:122`) — not `UpdateSession` — since
  both source and destination share the same underlying `SessionStatus` (`ACTIVE`); the only
  thing actually changing is the sub-status/approval state.
- **Dragging into** Needs Review via a raw drop is disallowed — no `SessionStatus`/`SubStatus`
  write corresponds to "a user decided this session needs review"; membership is purely an
  observed side-effect of the session's own state, populated automatically when it becomes
  true.
- **Dragging out to any column other than "Running"** (e.g. straight to "Complete") is
  rejected as illegal — `legalBoardTransitions["needs_review"] = []`, with "Running" handled
  as an explicit special case in the drop handler rather than as a table entry (see
  `implementation/plan.md` Epic 3.3).

## Alternatives Considered

| Option | Why rejected |
|---|---|
| Use the Review Queue (`AttentionReason`) as column membership | Broader definition of "needs attention" than pure approval-pending (includes `TASK_COMPLETE`, `IDLE_TIMEOUT`, `ERROR_STATE`), which could disagree with the existing list-view filter's language for the same underlying sessions — a session could show as "needs review" on the board but not match the existing "Needs approval" checkbox filter in list view, undermining requirements.md Goal 4's "compose with existing session-list features" framing. Also pulls in a second stream/poll lifecycle the board would need to additionally subscribe to, beyond the single `watchSessions` connection it already must share. |
| Treat Needs Review as an ordinary status column (symmetric `UpdateSession` drag both ways) | No `SessionStatus` value exists for "in review" — this would either require inventing new backend semantics (explicitly out of scope per requirements.md's Non-Goals: "no changing what a session status transition is allowed to do... beyond wiring the existing update-session status mutation to a drag gesture") or silently no-op on drop, which fails AC5's "visible error indication" requirement by being neither a real move nor a clearly rejected one. |

## Consequences

- `SessionBoard`'s drop handler has one genuinely special-cased column (Needs Review) rather
  than a fully uniform "every drag is `getBoardColumnKey(target) → statusForColumnMove →
  updateSession`" pipeline. This asymmetry is called out explicitly in code (Task 3.3.1a) and
  in the Pattern Decisions table so it isn't mistaken for an oversight during review.
- If the Review Queue's broader `AttentionReason` set is ever wanted as board column
  membership (e.g. a future "surface idle-timeout sessions too"), that is a distinct follow-up
  decision, not a change to this ADR's scope — the board would need to additionally subscribe
  to `WatchReviewQueue`, which this plan does not do.
