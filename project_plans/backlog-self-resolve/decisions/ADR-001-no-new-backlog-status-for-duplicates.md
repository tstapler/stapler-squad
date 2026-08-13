# ADR-001: Represent duplicate outcomes via existing `review` status, not a new `BacklogStatus`

**Status**: Accepted
**Date**: 2026-08-02

## Context

`report_duplicate` (this project, FR3) needs to route an item to a
terminal-ish, human-reviewable state once a work session proves its item is a
duplicate of an already-shipped PR/issue/commit. The obvious data-modeling
instinct is a new `BacklogStatus` value (e.g. `duplicate`, `closed`) so the
item list/detail UI can show a distinct badge and so `submit_review_verdict`
or a human reviewer can dispatch on it directly.

`session/domain/backlog.go`'s `BacklogStatus` enum and its `validTransitions`
map (`session/domain/backlog.go:328-392`) are read by both the Go backend and
(via generated proto/TS bindings) the React web UI. Adding a value requires:
an ent schema change (the `status` column has no CHECK constraint today, but
every consumer — web-app status badges, `StuckItemsSection.tsx`,
`WorkflowHistorySection.tsx`, the reconciliation sweep's per-status handling —
assumes a closed, known set), a `validTransitions` update, and non-trivial UI
work to render the new state — none of which is in scope per the
requirement's own acceptance criteria (AC8 explicitly calls for zero schema
changes).

## Decision

`report_duplicate` transitions the item to the existing `review` status
(`session.BacklogStatusReview`) — the same terminal-safety gate
`request_review` already uses — and represents the duplicate-specific
information (which PR/issue/commit, and why) via two existing, unmodified
storage paths:

1. **`BacklogStatusEvent.Note`** (`session/ent_repository_backlog.go:869`,
   `BacklogItemPrecondition.Note` field, `session/repository.go:556-558`) —
   already rendered to humans in `WorkflowHistorySection.tsx:44-58`. Populated
   with `"duplicate of <duplicate_ref>: <reason>"`.
2. **`ItemSession.VerificationNotes`** (via the existing
   `UpdateItemSessionVerificationNotes`, `session/storage.go:963`) — already
   the mechanism `request_review` uses to hand evidence to the review-gate
   LLM prompt (`spawnReviewGate`, `session/backlog_lifecycle.go:1208`).

A human (or the review-gate LLM) reading the item sees the duplicate claim
through the same channels they'd see any other review evidence — no new enum
value, no new UI surface, no schema migration.

## Consequences

- `go build ./...` succeeds without `ent generate` — confirmed no schema
  touches this feature (FR8).
- The web UI shows a duplicate-flagged item exactly like any other item
  awaiting review — there is no distinct "duplicate" badge/filter. A human
  reviewer (or `submit_review_verdict`) must actually read the status event
  note / verification notes to notice the claim; it is not surfaced as a
  first-class filterable state. This is an accepted limitation, not an
  oversight — a dedicated `duplicate` status with its own UI treatment is a
  reasonable follow-up if this workflow proves common, but is out of scope
  here (see requirements.md's Non-goals).
- Superseding the caller's own already-open PR on GitHub (closing/commenting
  on it) is *not* done by this feature — a human must still close it
  manually. Flagged as a known limitation in `implementation/plan.md`, not
  attempted here (a separate, riskier GitHub-mutation capability with no AC
  requesting it).

## Alternatives considered

- **New `BacklogStatusDuplicate` enum value**: rejected — requires schema
  awareness in every status-consuming call site (reconciliation sweep,
  `validTransitions`, web UI), directly contradicts AC8, and no acceptance
  criterion asks for a distinct terminal state; `review` already provides the
  correct terminal-safety property (a human/review-gate must look at it
  before it can reach `done`/`archived`).
- **A parallel `duplicate_of` column/flag on `BacklogItemData`**: rejected for
  the same reason — any new column is a schema change, and FR8 rules it out
  even though it's less invasive than a new enum value.
