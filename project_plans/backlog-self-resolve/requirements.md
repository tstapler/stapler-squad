# Requirements: Backlog self-resolve (duplicate detection + request_review CAS fix)

Source: backlog item `da58b867-bf4e-4720-8fe4-9cfcfa5b6eed`. Requirements derived
directly from the item description and its 10 acceptance criteria — no interactive
ideation interview (none was available; this session runs unattended).

## Problem

A work session that discovers, after opening its own PR, that a parallel session
already shipped the same fix (e.g. item `fc63d55b-d6cf-4e11-af02-c76c86637c5e`,
superseded by PR #272 while this item's own PR #281 was still open) has no way to
self-resolve:

- `request_review`'s CAS precondition is hardcoded to `ExpectedStatus: in_progress`,
  so it always rejects once `report_pr_created` has already moved the item to
  `pr_pending`.
- `submit_review_verdict` is review-role-only; a work session cannot call it.
- No tool exists for a work session to say "this is a duplicate of PR #N" and route
  the item to a terminal/reviewable state.

## Functional Requirements (map 1:1 to acceptance criteria)

1. **FR1 — Generalize `request_review`'s CAS precondition.** A work-role session
   whose item is at `in_progress` OR `pr_pending` may call `request_review`. The
   precondition passed to `TransitionBacklogItemStatus` must be pinned to the
   status actually observed on the loaded item (not a hardcoded constant), so the
   CAS check still protects against concurrent modification without blocking the
   legitimate pr_pending → review re-request path.

2. **FR2 — Guard against clobbering an active reviewer.** When the source status
   is `pr_pending`, `request_review` must refuse the call if an active
   (unended) review-role `ItemSession` already exists for the item — a
   pr_pending item already in review-adjacent flight must not be silently
   re-routed out from under a running reviewer. The `in_progress`-sourced path's
   existing behavior must be unchanged.

3. **FR3 — New `report_duplicate` tool.** Work-role sessions get a new MCP tool
   `report_duplicate(item_id, duplicate_ref, reason)`. `duplicate_ref` is a
   PR/issue/commit URL. The reference must be verified against GitHub *before*
   any state mutation. On success the item is routed to `review` (never directly
   to `done` or `archived`) — the same terminal safety property `request_review`
   has via the review gate.

4. **FR4 — Two-channel verification errors.** `report_duplicate` must distinguish
   a definitively-nonexistent/invalid reference (`ErrInvalidArgument`, no retry
   implied) from a transient verification failure — network, auth, rate-limit
   (`ErrInternalError`, explicit "retry" wording in the message) — mirroring
   `report_pr_created`'s existing pattern (`tools_backlog.go` ~L707-715).

5. **FR5 — Accurate "when will this be seen" messaging.** If a review-role
   session is already active for the item when `report_duplicate` succeeds, the
   success text must say the duplicate evidence will land on the **next** review
   pass — never claim the currently-running reviewer will see it, since
   verification notes are rendered into the reviewer's prompt only once, at
   review-gate spawn time (not live-updated mid-session).

6. **FR6 — Refusal preconditions, zero mutation.** `report_duplicate` must refuse
   (with no state mutation on any refusal path):
   - items with `SkipReviewGate == true`
   - callers whose session role is not `work`
   - callers whose session is not linked to the target item

7. **FR7 — Audit trail.** Every status transition this feature causes must be
   recorded through the existing `BacklogStatusEvent`/`recordStatusEvent`
   mechanism with `TriggeredBy="agent"` (introducing that constant if it doesn't
   already exist). `duplicate_ref` and `reason` must be persisted into the item
   session's verification notes (reusing the existing verification-notes
   storage path, e.g. `UpdateItemSessionVerificationNotes`).

8. **FR8 — No schema changes.** Per ADR-001, no new `BacklogStatus` enum value
   and no ent schema/migration change. `go build ./...` must succeed without
   running `ent generate`. The duplicate outcome is represented via existing
   statuses (`review`) plus the persisted verification notes / reason, not a new
   status value.

9. **FR9 — No regression.** The existing `request_review` test suite in
   `server/mcp/tools_backlog_test.go` must pass unmodified for the
   `in_progress`-sourced path.

10. **FR10 — Discoverability of stuck items.** `report_duplicate`'s MCP tool
    description must give explicit agent-facing retry guidance for
    `INTERNAL_ERROR` results. Independently, an item that ends up stuck at
    `pr_pending` after a logged verification-failure warning must eventually
    surface through the existing stuck-item notification path(s) (not require a
    human to grep logs to discover it).

## Non-goals

- No new terminal `BacklogStatus` (e.g. `duplicate`, `closed`) — explicitly
  excluded by AC8/ADR-001.
- No change to `submit_review_verdict`'s review-role gating.
- No UI changes are implied by the acceptance criteria (backend/MCP-tool scope
  only); a UI surfacing of duplicate_ref/reason may be a natural follow-up but is
  out of scope unless research finds it's needed to satisfy an AC.

## Open questions for research phase

- Exact current signature of `TransitionBacklogItemStatus` / `BacklogItemPrecondition`
  and how CAS mismatches are surfaced as errors.
- Whether a `TriggeredByAgent` (or similarly named) constant already exists next to
  `TriggeredBySystem`.
- How to detect "active (unended) review-role ItemSession" for FR2/FR6 — exact
  storage method/fields available today, or whether one needs to be added.
- Whether `session.ParseGitHubURL` / `VerifyPRMatchesBranch` (or an issue/commit
  equivalent) already generalize to non-PR refs, since `duplicate_ref` may be an
  issue or commit URL, not only a PR.
- Where/how the existing stuck-item notification path surfaces `pr_pending` items
  today, to confirm FR10's "eventually surfaces" claim is achievable without new
  plumbing.
