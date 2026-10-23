# ADR-001: Attention surface consolidation (staged hybrid D)

**Status**: Proposed (owner ratification is a wall-clock blocker before Epic 6 starts)
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Question (from the owner)

Should we replace the review page (`/review-queue`) with the notification bar (tray) and consolidate into Up Next (`/unfinished`)?

## Answer

Not as stated. Replace the review page's role in stages, not its route, and not with the tray.

1. **Durable attention list = single source of truth.** Keep `ReviewQueuePanel` (and its `/review-queue` route) as the state-based list through the detection/event-log slices. It is fixed and extended (new reasons, a11y, grouping).
2. **Tray = transient alert layer.** Toasts, push and Slack link into the list. The tray never becomes the durable surface.
3. **Per-session event timeline** = one shared `SessionEventDrawer` (right drawer on desktop, bottom sheet on mobile), opened from list rows and from `SessionCard`.
4. **Up Next gets a "Sessions" tab later**, built behind `attention_inbox_consolidation` (default off), made the default tab only when it has needs-me items, and promoted once the coverage metric is met on a soak.
5. **`/review-queue` is kept permanently** as a flag-gated redirect to `/unfinished?tab=sessions` (query params preserved). It is never deleted, so Slack/callback/omnibar/shortcut/onboarding deep links and bookmarks cannot break.

## Options considered

| Option | Verdict | Reason |
|---|---|---|
| A keep + fix | Stage 1 only | Right first move, but leaves three overlapping surfaces permanently |
| B tray-only | Rejected | Tray is a per-event stream (dedup-collapsed by notification-revamp ADR-001, non-modal by tray ADR-006); the operator needs a per-session state view with ranking and ack. Mixing the two yields duplicates and no ranking |
| C merge into Up Next now | Rejected as first step | Moves a list the operator does not yet trust; larger regression surface (69 files reference `/review-queue` per architecture research, 12+ e2e specs) |
| D staged hybrid | Chosen | Matches the pattern in GitHub/PagerDuty/Linear: durable queue + transient alert + per-item timeline |

## Reconciling architecture.md vs ux.md

Both recommend hybrid D with Up Next as eventual home. They differ in four places; resolutions:

| Difference | architecture.md | ux.md | Resolution |
|---|---|---|---|
| Timing of Up Next move | Tab + redirect in the same flagged slice | Move only after soak proves trust | UX wins on sequencing: tab is built (Epic 6) but flag default off; redirect flip is a gated owner action after the coverage soak |
| New read model | `ListAttention` RPC (`AttentionItem` = item + events) | Reuse the existing panel | Do not add `ListAttention`. `WatchReviewQueue`/`GetReviewQueue` stay the state feed; one new RPC `ListSessionEvents` supplies history, "recently finished" and the drawer. "Quiet" is derived client-side from `lastMeaningfulChangeAt` on the session list |
| Tray deep link | `/attention?session=` | Link into the list | No new route. Tray links go to `/review-queue?session=<id>`, which the redirect later maps to the Sessions tab. This avoids a new `page.tsx`/RouteMeta row |
| Default tab | Sessions tab default when count > 0 | Attention first/default | Compatible with up-next-tabs-pr-nudge ADR-003 only if deliberate: default to `sessions` iff the flag is on AND needs-me count > 0; `?tab=` still wins, then stored tab, then `prs` |

Additional UX decisions adopted: implicit "seen" when the operator opens the session, explicit "Done" removes finished items and never clears needs-me items (bulk "mark all finished as seen" only touches informational rows, per tray ADR-008); Up Next tab strip scrolls horizontally on mobile.

## Consequences

- Positive: lowest regression risk; every slice is independently flagged and revertible; trust is measured on a stable surface before it moves.
- Negative: for a while two entry points show overlapping data; mitigated by one shared array feeding nav badge and tab badge (notification-revamp ADR-002's technique).
- Redirect uses `Redirect` from `@/lib/nav` inside a `FeatureGuard`-style flag check; RPCs `GetReviewQueue`/`WatchReviewQueue`/`AcknowledgeSession` are never deleted (registry and RPC-surface gates).

## Evidence

architecture.md section 4; ux.md sections 8-9; pitfalls.md P9; features.md section 3 (tray ADR-006/008, up-next-tabs ADR-003, notification-revamp ADR-001/002).
