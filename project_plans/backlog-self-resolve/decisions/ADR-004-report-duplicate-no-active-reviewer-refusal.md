# ADR-004: `report_duplicate` Does Not Gain FR2's Active-Reviewer Refusal

## Status
Proposed — flagged for owner confirmation (see Consequences)

## Context

FR2 requires `request_review` to refuse when its source status is `pr_pending` and an active
(unended) review-role `ItemSession` already exists for the item — to avoid re-routing a
pr_pending item out from under a running reviewer.

A synthesized research finding (this plan's source critical-findings list, item #8) discusses
FR2 and FR6 together as sharing the same active-review-session detection primitive, and
mentions "a one-line hint in the refusal message" — worded in a way that could be read as
implying `report_duplicate` should carry an analogous hard refusal.

But `requirements.md`'s FR6 — the actual, ratified acceptance-criteria text for
`report_duplicate`'s refusal conditions — enumerates exactly three:

> `report_duplicate` must refuse (with no state mutation on any refusal path):
> - items with `SkipReviewGate == true`
> - callers whose session role is not `work`
> - callers whose session is not linked to the target item

No fourth condition ("active review session exists") appears. And FR5, a separate, explicit
requirement, only makes sense if `report_duplicate` *can* succeed while a review session is
already active:

> If a review-role session is already active for the item when `report_duplicate` succeeds,
> the success text must say the duplicate evidence will land on the next review pass...

FR5's premise (`report_duplicate` **succeeds** in that state) directly contradicts a reading
where `report_duplicate` refuses in that same state.

## Decision

**`report_duplicate` does not add an FR2-style hard refusal for an active review session.**
It always attempts the transition (subject to all of FR6's actual three conditions plus the
whitelist/CAS checks shared with `request_review`); the *only* effect of an active review
session on `report_duplicate`'s behavior is the FR5 success-message branch (Story 3.3.3 in
plan.md) — "next review pass" wording instead of implying the current reviewer sees it live.

This resolves the apparent tension in favor of the literal, ratified requirements text (FR5,
FR6) over a looser synthesis note, on the grounds that requirements.md is the authoritative
acceptance-criteria source this entire plan is contractually built against, and a refusal
here would make an explicitly-numbered requirement (FR5) permanently unreachable — which
would itself be a plan defect, not a faithful implementation of "every FR."

## Consequences

### Positive
- FR5 is implementable and testable exactly as written (Task 4.2.5a).
- `report_duplicate`'s refusal surface matches FR6's literal, reviewable list — no invented
  fourth condition to justify or maintain.

### Negative / Risk
- **This is a judgment call under genuine ambiguity, not a fact derived from unambiguous
  requirements text — flagged in Unresolved Questions (UQ-1) for the item owner to confirm
  before or during Phase 3.** If the owner's actual intent was closer to the synthesized
  finding (extend FR2's refusal to `report_duplicate` too), the fix is small and isolated:
  add a refusal branch to Story 3.1.2/Task 3.1.2b mirroring Task 2.2.1b's guard, and FR5's
  acceptance criterion (and Task 4.2.5a) would need to be rewritten or dropped as
  unreachable — a contained, single-story change, not a plan-wide rework.

### Neutral
- The active-review-session detection primitive (`hasActiveReviewSession`, Task 2.2.1a) is
  built once regardless of which reading is correct — it's reused by FR2's refusal
  (`request_review`) and FR5's messaging (`report_duplicate`) either way, so this decision
  doesn't add or remove any shared infrastructure, only whether `report_duplicate` consults
  it for a hard-refusal branch versus message-wording only.
