# Adversarial Review — Phase 2 Plan
**Date**: 2026-10-12

## Finding 1 (BLOCKER): PR badge duplicates PullRequestSection for `pr_pending` status

**Claim**: Task P2.1.1b renders `<GitHubBadge>` in LifecycleSummary whenever `prUrl` is set,
including when `item.status === "pr_pending"` — but `PullRequestSection` already renders
(expanded by default) showing the same badge with additional context (Jules provenance marker,
"Mark Done" button). Two badges pointing to the same PR URL in one view is noise, not signal.

**Risk**: The plan's own acceptance criterion confirms this — it says the badge "ALSO" renders
for `pr_pending`. This is intentional (to avoid conditional logic in the header), but it creates
the duplication problem the original plan set out to eliminate.

**Resolution**: Task P2.1.1b should suppress the header badge when `PullRequestSection` is also
rendered — i.e., add an `hidePrBadge` prop or use the status guard directly:
`prUrl && item.status !== "pr_pending"` in the header (the `pr_pending` status already gets
a prominent, always-expanded `PullRequestSection` in the scroll area that is already the first
collapsible item and close to ActionsSection after the P2.2 reorder). The header badge is most
valuable for `done`/`archived`/`review` where no dedicated PR section renders. The acceptance
criterion in P2.1.1 story should be updated to say: "does NOT render when `status === 'pr_pending'`"
rather than "ALSO renders."

**Verdict**: Fix before implementing. Change the condition to `prUrl && item.status !== "pr_pending"`.

---

## Finding 2 (P2): Scroll area reorder breaks the TriageReviewPanel/Actions visual contract for `idea` items

**Claim**: For `item.status === "idea"` with completed triage, moving ActionsSection to the
TOP means the user sees the "Trigger Triage" / "Mark Ready" buttons BEFORE seeing the triage
output (TriageReviewPanel, ChatRefinementPanel). This is an odd flow — the actions reference
content (apply suggestions, skip, etc.) that won't have rendered yet when the user first sees
the buttons.

**Risk**: Low severity for the common case (triage panel always expands immediately above the
action that references it — just now ActionsSection is above it, not below it). The existing
flow for `idea` is: user sees triage result → decides to apply or skip → clicks action. Moving
actions BEFORE triage results inverts this decision-making sequence for idea items.

**Resolution**: The reorder is still net-positive for all non-idea statuses (review, ready, done,
etc. — the main complaint cases). For `idea` status specifically, the order matters more because
the triage content IS the context for the action. Two options:
1. Keep the reorder but acknowledge idea-status is slightly worse (still an improvement over the
   current deep-scroll to actions for review items — the trade-off is acceptable).
2. Conditional reorder: only move ActionsSection to top for non-idea statuses. This adds
   conditional complexity to BacklogItemDetail.tsx's JSX order.

**Verdict**: Option 1. The triage result for an idea item is presented in TriageReviewPanel
(which renders actions of its own — Apply/Skip/Refine), so the user typically acts via
TriageReviewPanel's own buttons, not ActionsSection. ActionsSection for idea status mainly
shows "Trigger Triage" (if triage hasn't run) or a limited fallback set. The regression is
minimal. Accept as a known trade-off, document in the implementation task.

---

## Finding 3 (P3): `!!item.prUrl` as VCS auto-expand condition is too broad

**Claim**: Task P2.3.1a uses `!!item.prUrl` as the condition to auto-expand VersionControlSection.
But `item.prUrl` is set for `pr_pending`, `done`, and any status where a PR was manually linked.
For `in_progress` + prUrl, the VCS section was already auto-expanding via the existing condition.
The new `!!item.prUrl` condition adds no visible change for `in_progress`/`review`/`pr_pending`
(all already in the expand set) but DOES add auto-expand for `done`/`archived` — which is the
intended fix.

However, it also auto-expands for `idea`/`ready`/`queued` if prUrl is somehow set
(unlikely but possible if a PR was linked manually while the item was at these early stages).

**Risk**: Very low. `prUrl` at `idea`/`ready`/`queued` is atypical, and auto-expanding VCS in
that case is arguably correct anyway (if a PR exists, the VCS context is relevant regardless of
lifecycle stage).

**Resolution**: Accept. The condition is correct for all realistic cases. If `idea`/`ready`/`queued`
auto-expand becomes a problem in practice, a future follow-up can add a status exclusion list.

---

## Summary

| Finding | Severity | Action |
|---|---|---|
| 1 — PR badge duplicates PullRequestSection for pr_pending | BLOCKER | Change P2.1.1 condition to `prUrl && item.status !== "pr_pending"` |
| 2 — Reorder breaks idea-status flow | P2 (acceptable trade-off) | Accept + document |
| 3 — `!!item.prUrl` expand condition too broad | P3 | Accept |
