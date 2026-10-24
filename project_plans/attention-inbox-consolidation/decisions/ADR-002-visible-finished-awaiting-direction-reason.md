# ADR-002: Add visible reasons instead of superseding notification-revamp ADR-002

**Status**: Proposed (confirmed or reversed by Spike 0.1.3; owner decision). Amended in the Phase 4 patch: `AwaitingDirection` is delivered, not silent (OWNER TO CONFIRM, see plan.md Owner-to-confirm).
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Context

notification-revamp ADR-002 (Accepted 2026-09-04, `project_plans/notification-revamp/decisions/ADR-002-idle-items-removed-from-review-queue.md`) hides `AttentionReason.IDLE` from the Review Queue page, badge and nav (`web-app/src/lib/utils/reviewQueueVisibility.ts:9`) and shows idle as a `SubStatus.IDLE` chip. Requirement 2 ("finished and waiting for more context") needs those sessions visible. Architecture research (D15) names that filter as the strongest single cause of gaps 1 and 2.

## Options

1. **Supersede ADR-002**: make `IDLE` visible again. Re-admits every shell/vim prompt and every routine pause (project memory: `StatusIdle` is shared with `command_prompt` / `vim_normal_mode`), recreating the flood ADR-002 removed.
2. **Add new visible reasons; keep ADR-002 untouched (chosen)**: `ReasonProseQuestion` (Medium) and `ReasonAwaitingDirection` (Low), both produced only from a turn-ended episode (ADR-003), never from bare idle. Plain `IDLE` stays a chip.

## Decision

Option 2. ADR-002's rationale (GitHub participating vs watching; Vibe Kanban "Needs Attention" vs "Idle") still holds: a *turn-ended-with-evidence* session is "participating", a bare idle prompt is "watching". `isReviewQueueVisible` keeps hiding `IDLE` and shows the two new reasons. ADR-002 gains a one-line "refined by attention-inbox-consolidation ADR-002" note; it is not marked Superseded.

## Consequences

- Needs proto wiring (`ATTENTION_REASON_PROSE_QUESTION`, `ATTENTION_REASON_AWAITING_DIRECTION` in `proto/session/v1/types.proto` after value 10), not only the domain enum (project memory `instinct_alias_session`).
- New reasons are live-flag-gated (`attention_prose_question_detection`); with the flag off behavior is byte-identical to today (golden fixtures prove it).
- `AwaitingDirection` produces a durable list row AND one delivered notification per turn-end episode: low urgency, tray record plus web push, NO toast and NO Slack by default, deduped by `EpisodeID`, with at most one web push per session per 5 minutes (further episodes inside the window still get a tray record). The operator's complaint is that finished and waiting sessions do not tell them; a silent row would not fix it. Hidden sessions are still suppressed by the delivery gate. `ProseQuestion` is published as `INPUT_REQUIRED` and reaches every sink that class reaches today (tray, toast, web push, Slack; Spike 0.1.4b confirms), below approvals in priority. Neither reason inflates the nav badge unless it is needs-human (badge counts needs-human reasons only; plan.md Attention Reason Classification).
- Refined by ADR-007 (triad repair iteration 1): a global push cap with a digest, mute (session and global), an always-on non-alerting "N new finished" nav cue that does not depend on push, server-derived Seen, a prose-question burst guard and opt-out (iteration 2), and an unconfirmed-detection signal now bound this delivery.
- Reversal path: if the Spike 0.1.3 review shows the owner wants idle itself visible, only `reviewQueueVisibility.ts` and one test change.

## Ratification brief (Task 0.1.3a; status stays Proposed until the owner replies)

**Code check (2026-10-24, branch aic-spikes2)**:
- `web-app/src/lib/utils/reviewQueueVisibility.ts:9` is `item.reason !== AttentionReason.IDLE`; it is applied once at `web-app/src/lib/hooks/useReviewQueue.ts:178`, so list, nav badge, BottomNav and DrawerNav share it. Confirmed as described in Context.
- The plan cites `SessionRow.tsx:352` for the idle chip; at this commit that line is the select checkbox button. The chip is at `web-app/src/components/sessions/SessionRow.tsx:398-410`: `SubStatusChip` renders for `ACTIVE` sessions whose `subStatus` is neither `UNSPECIFIED` nor `READY` (a `SubStatus.IDLE` case exists at `SubStatusChip.tsx:139`). Behavior matches the ADR; only the line reference is stale (INFERRED correct fix for the plan: cite `:398-410`).

**Question to the owner**: keep `IDLE` hidden and add two visible, turn-end-evidenced reasons (`ProseQuestion` Medium, `AwaitingDirection` Low, delivered as tray record plus web push, no toast or Slack, one push per session per 5 minutes), all behind `attention_prose_question_detection` (default off)?

| Reply | What flips |
|---|---|
| "accept" | Status Accepted. Story 3.2.1 is unblocked; `reviewQueueVisibility.ts` keeps hiding `IDLE` and shows the two new reasons; proto gains `ATTENTION_REASON_PROSE_QUESTION` and `ATTENTION_REASON_AWAITING_DIRECTION`; notification-revamp ADR-002 gets only a one-line "refined by" note |
| "change: make IDLE itself visible" | Option 1; only `reviewQueueVisibility.ts` and its test change, but every shell/vim prompt and routine pause re-enters the list (the flood notification-revamp ADR-002 removed); the new reasons become unnecessary for visibility; ADR-007 caps still apply |
| "change: AwaitingDirection silent (row only)" | Amended: the tray/push delivery in Consequences and the nav "N new finished" cue's alert dependence are dropped; the operator complaint (finished sessions do not tell me) is not addressed by a silent row |
| "change: also toast/Slack for AwaitingDirection" | Amended: delivery policy and ADR-007 push-cap/digest bounds are re-derived for the louder sink |
