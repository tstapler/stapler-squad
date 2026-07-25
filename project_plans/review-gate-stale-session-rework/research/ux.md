# UX Research: review-gate-stale-session-rework

**Date**: 2026-07-24

## Comparable UX patterns already in this product

- The `backlog-stuck` component set already establishes the interaction pattern this feature should follow: a chip (icon + text label, never color-only — `stuckReason.ts`'s own doc comment: "never color-only") in a list/board view, a detail view with "since" duration and "last checked" timestamps, and (for reasons that support it) an inline remediation action. This fix should extend that established pattern, not invent a new one.
- `GateVerdictBox.tsx`'s "Reopen for Revision" flow (feedback text field + submit) is the existing confirmation pattern for a human overriding an automated stall — reuse its interaction shape rather than designing a new one-click "force reopen" affordance, which would reduce the friction this repo has apparently already decided is appropriate before reopening over a live session.

## User mental model

The user's own bug report ("Rework blocked by a stale-but-alive session... check it manually, or use 'Reopen for Revision' once you've confirmed it's actually stuck") shows they already understand the intended mental model correctly — the gap is purely that the *toast* was their only way to learn this, and it was easy to miss among 37 other (false-positive) stale badges. The fix's UX job is not to teach a new concept, it's to make an already-understood state durably findable. This significantly narrows UX design need — no new mental model to introduce, no onboarding/education surface required.

## Accessibility

- Reuses existing, presumably-already-audited `backlog-stuck` components — no new accessibility surface expected if reusing `StuckReasonStaleWork`'s existing chip. If a new `StuckReason` value is added, its icon/label pair must follow the established "icon is decorative, text label is the real signal" convention already documented in `stuckReason.ts` (`STUCK_REASON_ICONS`'s comment: "never the sole signal — text label always accompanies it") — this repo has already made the right accessibility call here; just don't break the pattern for a new entry.
- No new keyboard-navigation or ARIA surface anticipated — this is additive data flowing through existing list/detail components, not new interactive controls beyond what `GateVerdictBox` already provides.

## Error states and edge cases needing graceful UX handling

- **Stale-but-not-actually-hung** (the exact original false-positive problem, now specifically for the rework-block case): if the recalibrated threshold is still occasionally wrong, the UI must make it trivially cheap to dismiss/ignore a false positive without taking the "Reopen for Revision" action — i.e., don't make the durable surfacing itself feel alarming or demand action if it's just informational until the human decides to act. The existing snooze mechanism (`SnoozeStuckItem` RPC, from `backlog-stuck-item-visibility`) may already cover this — confirm during planning whether the new/reused reason should be snoozable via the existing mechanism (likely yes, for consistency).
- **Session ends while the stuck card is open in the UI** — the detail view should reflect the resolved state on next poll/refresh rather than continuing to show a stale "still blocked" card pointing at a session that has since ended or completed.

## Job-to-be-done

- **Functional job**: let the user find, at a glance and without having to have caught a specific toast, which backlog items currently cannot make automated progress and why — this is a strict continuation of the functional job the `backlog-stuck-item-visibility` project already defined for its other 4 reasons; this fix simply closes a 5th (well, sub-)gap in the same job.
- **Emotional job**: reduce the "did I miss something important" anxiety of relying on ephemeral toasts for an automation system the user has delegated real trust to (a system managing 40+ concurrent backlog items). A durable, always-findable list is reassuring in a way a one-shot notification cannot be.
- **Social/no social job** applicable — single-user, self-hosted instance.

## Applicability note

Given the small, mostly-backend-and-reused-component scope of this fix (see architecture.md — the primary UI lift may be a single new label/icon map entry, or zero new UI at all if reusing `STALE_WORK` verbatim), this UX research is intentionally light. Do not let Phase 3 over-invest in new UI design for what is fundamentally a "route an existing signal through an existing, working display mechanism" fix.
