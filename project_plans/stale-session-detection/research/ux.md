# UX Research: stale-session-detection

Agent 5 (UX), SDD Phase 2. Scope per `requirements.md`: (1) visual stale indicator on
`SessionCard.tsx`, (2) a "Stale" grouping/filter strategy, (3) an optional stale-session
notification. This doc covers comparable patterns, mental model, accessibility, edge cases,
and jobs-to-be-done — architecture/data-flow is out of scope here (see `architecture.md`).

## 1. Comparable UX patterns already in the codebase — reuse, don't invent

Two existing "staleness" surfaces set the visual and structural precedent this feature
should match, not diverge from:

- **`backlog-stuck` chip pattern** (`web-app/src/components/backlog-stuck/StuckItem.tsx:252-260`,
  `web-app/src/components/backlog-stuck/stuckReason.ts:14-89`,
  `web-app/src/components/backlog-stuck/stuckReason.css.ts:44-51`). The `STALE_WORK` reason
  already renders as an **icon + text label pill**, never color/icon alone:
  `<span aria-hidden="true">🟠</span> Stale work session`, using vanilla-extract
  `chipStaleWork` (`background: vars.color.warningBg`, `color: vars.color.warningText`,
  `border: 1px solid vars.color.warning`). The doc comment on `STUCK_REASON_ICONS`
  (`stuckReason.ts:33`) states the rule explicitly: "Decorative icon glyph for every
  StuckReason (never the sole signal — text label always accompanies it)." This project's
  new `SessionCard.tsx` indicator should reuse the **same `warningBg`/`warningText`/`warning`
  token triplet** (already defined per `.claude/rules/css-architecture.md`'s token list) and
  the same icon+text shape — not a new amber border or a bespoke color.
- **Review Queue `ReasonStale`** (`session/review_queue_poller.go:38-49`,
  `StalenessThreshold: 5 * time.Minute` — recalibrated in the now-shipped
  `review-gate-stale-session-rework` (PR #219, merged) via
  `project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md`)
  drives `AttentionReason.STALE`, which `NotificationContext.tsx:92-93`
  (`reviewItemToNotificationType`) already maps to notification type `"warning"`. This is the
  exact precedent for jobs-to-be-done point 3 below — the new stale-session notification
  should reuse `notificationType: "warning"`, not invent a new type.
- **Confirmed via Phase 2 re-check (constraint from requirements.md):**
  `review-gate-stale-session-rework` has shipped (`git log`: `0dd9cd226`/`b2a12eac2`, PR
  #219). Current live values: Review Queue badge threshold = 5 min
  (`session/review_queue_poller.go:49`), stuck-item detector `maxWorkSessionStaleness` = 2h
  (`session/backlog_lifecycle.go:2098`). Per requirements.md's open question, this project's
  new **card-level indicator threshold is a third, distinct sensitivity** ("informational
  glance-level badge") — it should NOT silently inherit either existing constant; Phase 3
  needs its own config default, reasoned about independently (5 min is too noisy for a
  glance-scan badge sitting on every card at once; 2h is too slow to catch a session stuck
  for 20 minutes). A sensible starting anchor for planning: something between the two (e.g.
  10-15 min), config-driven per requirements.md scope, not hardcoded.

**Do not invent a new visual language.** Both the icon glyph choice (🟠) and the
warning-token color triplet are already established; the new card badge should look like a
sibling of the `chipStaleWork` pill, sized for the more compact `SessionCard.tsx` header row
(see Section 3 for exact styling recommendation).

## 2. User mental model — scannable at a glance across 5-10 parallel cards

The problem statement (`requirements.md:10-11`) is explicit: running 5-10 parallel sessions
makes it easy to miss the one that silently died. The design implication is that the
indicator's job is **negative-space scanning** — the user's eye should catch the *one* stale
card among many healthy ones without reading each card's text.

- `SessionCard.tsx` already has a header badge row with an established precedence pattern
  (lines 505-548): primary `status` badge, then `rateLimitState` badge, then `StatusBadge`
  (only when `SubStatusChip` has nothing to show — the code comment at line 533 states this
  explicitly: "showing both is duplication"), then `SubStatusChip`. The stale badge is a
  **new peer in this same row**, not a separate section — keeps the single "the state of
  this card" scan-line the user already relies on, rather than adding a second place to
  check.
- The existing "Last Activity" row (`SessionCard.tsx:677-696`, `lastActivityRow`) already
  renders a relative timestamp (`formatTimeAgo`) sourced from the same
  `lastMeaningfulOutput`/`lastTerminalUpdate` fields this feature reuses
  (`SessionCard.tsx:679-683`). The stale badge and this existing timestamp are **the same
  signal at two granularities** (boolean flag vs. exact age) — pair them visually (badge in
  the header row for at-a-glance scanning; the existing "Active 47m ago" text remains the
  drill-down explanation) rather than duplicating a second timestamp render.
- For the grid/list scan specifically: because 5-10 cards may render simultaneously, favor a
  **compact badge, not a full-card border/background treatment**. A colored full-card border
  (as brainstormed in requirements gathering) competes visually with the existing
  `cardExpanded`/hover/focus-visible border treatments already used elsewhere
  (`StuckItem.css.ts:4-20` shows the established `:hover`/`:focus-visible` border-color
  pattern on cards) and risks looking like a selection/focus state rather than a status flag.
  A small icon+text pill in the header badge row is both consistent with `chipStaleWork` and
  avoids that collision.

## 3. Accessibility

- **Color alone is insufficient (WCAG 1.4.1)** — confirmed as already-enforced house style:
  `stuckReason.ts`'s doc comments state this rule for every existing reason chip, and
  `STUCK_REASON_LABELS`/`STUCK_REASON_ICONS` are typed as `Record<StuckReason, T>` specifically
  so a new enum value is a **TypeScript compile error**, not a silently-blank chip, if a label
  is forgotten. The new stale badge must ship with a text label ("Stale") alongside the icon,
  exactly like every existing `backlog-stuck` chip — an amber border alone (as
  `requirements.md`'s phrasing loosely suggests) is not sufficient on its own and must not be
  the only signal. If a border treatment is also desired for peripheral-vision scanning, it
  should be an *addition* to the icon+text badge, never a replacement.
- **`aria-label` / `role="img"` pattern already established** — every existing badge in this
  header row uses `role="img"` with a full-sentence `aria-label`
  (e.g. `SessionCard.tsx:509-510`: `aria-label={`Session status: ${getStatusText(...)} — ${session.creationProgress}`}`).
  The new stale badge should follow suit:
  `aria-label={`Stale — no output for ${formatTimeAgo(lastActivity)}`}` (or equivalent),
  with the icon marked `aria-hidden="true"` per `stuckReason.tsx:258`'s existing pattern.
- **Grouping option keyboard/screen-reader behavior** — `strategies.ts`'s existing
  `GroupingStrategy` enum + `GroupingStrategyLabels` record renders through whatever selector
  component consumes it (a `<select>`-like control per the existing 9 options). Adding
  `GroupingStrategy.Stale = "stale"` with a `GroupingStrategyLabels` entry ("Stale") is
  additive and requires no new ARIA pattern — it inherits the existing selector's
  keyboard/screen-reader behavior automatically, consistent with how `Status`, `Program`, etc.
  already work. No new accessibility surface to design here; just don't forget the
  `GroupingStrategyLabels` entry (a `Partial<Record<...>>` — unlike `stuckReason.ts`'s maps,
  this one is NOT compile-enforced, so it is easy to add the enum value and silently forget
  the label — flag as an explicit implementation checklist item).
- **Contrast**: reusing `vars.color.warningBg`/`warningText`/`warning` inherits whatever
  contrast audit those tokens already passed elsewhere (used across `chipStaleWork`,
  `chipAbandonedReview`, and the general `warning`/`warningBg` tokens listed in
  `.claude/rules/css-architecture.md`'s "Status" token row) — no new contrast verification
  needed if the existing token triplet is reused verbatim, which is another reason not to
  introduce a new bespoke amber value.

## 4. Error / edge-case UX — a paused or archived session must not read as "stale"

This is the sharpest edge case, and it is not yet resolved in requirements.md.

- **Root cause of the risk**: staleness is computed purely from
  `lastMeaningfulOutput`/`lastTerminalUpdate` vs. a threshold
  (`SessionCard.tsx:679-683`'s existing computation). A `SessionStatus.PAUSED` (4) or
  `SessionStatus.HIBERNATED` (8) session has, by design, produced no output for an arbitrary
  length of time — that is the *expected*, healthy state of a paused/hibernated session, not
  a symptom of a stuck agent. Naively applying the threshold to `lastActivity` regardless of
  `session.status` would flag every paused session in the workspace as "stale," which is
  actively misleading (worse than no signal — it teaches the user to ignore the badge).
- **Precedent for the fix**: the codebase already has this exact precedence discipline for
  the adjacent `StatusBadge`/`SubStatusChip` pair (`SessionCard.tsx:533-548`) — an explicit
  comment states the suppression rule ("only shown when SubStatusChip has nothing to
  display... showing both is duplication") and the sub-status chip is gated to
  `session.status === SessionStatus.ACTIVE` only (line 543). The stale badge must follow the
  same discipline: **gate the staleness computation to `SessionStatus.ACTIVE` only** (and,
  per `isPausedOrStopped` already computed at `SessionCard.tsx:23`, explicitly exclude
  `PAUSED`/`STOPPED`, and by extension `HIBERNATED`/`CREATING`). A session that is not
  actively expected to be producing output should never show a staleness badge, full stop —
  this is a correctness requirement for Phase 3's implementation plan, not just a nice-to-have.
- **What the badge should say/do for non-active states**: nothing — suppress entirely, rather
  than showing an alternate "N/A" or grayed-out variant. Adding a second visual state
  ("paused, was stale before pausing") is speculative complexity this project's Complexity-2
  scope (per requirements.md) doesn't need; the existing status badge already communicates
  "Paused"/"Stopped"/"Hibernated" clearly on its own.
- **Grouping strategy analog**: the "Stale" `GroupingStrategy` bucket should apply the same
  `ACTIVE`-only filter when computing membership — a paused session must never land in the
  "Stale" group. Follow the existing `Status` strategy's per-session classification shape
  (`strategies.ts:127`, `case GroupingStrategy.Status`) for where to inject this rule.
- **Notification edge case**: if a session transitions `ACTIVE → PAUSED` at the same moment
  it crosses the staleness threshold (e.g., user pauses a session that also happens to be
  idle), the notification-emitting code must check current status at emission time, not just
  at the moment the threshold was crossed — otherwise a user who deliberately paused a
  session gets a spurious "session went stale" notification for an action they just took on
  purpose. This should reuse the same dedup-key shape already used by
  `NotificationContext.tsx`'s history reconciliation (`sessionId:notificationType` pattern,
  `NotificationContext.tsx:136,160`) so a flapping active/stale transition doesn't spam
  duplicate notifications — consistent with the "notify-once" semantics
  `review-gate-stale-session-rework`'s architecture.md documents for the sibling
  `MarkStuckNotified` mechanism.

## 5. Jobs-to-be-done

- **Functional job**: let the user find the one stuck/silent agent among 5-10 running
  sessions without opening each card individually — directly the problem statement in
  `requirements.md:10-11`. The card badge (glanceable), the "Stale" grouping/filter
  (find-all-at-once), and the optional notification (push instead of pull) are three
  different delivery mechanisms for the identical underlying job — a user who prefers
  glancing at the grid doesn't need the notification, and vice versa. Both should exist but
  neither should be assumed as *the* primary path; `stale_notify` is explicitly optional
  (config flag) per requirements.md scope for this reason.
- **Emotional job**: confidence that nothing was missed — the same "did I miss something
  important" anxiety already named as the emotional job for the adjacent
  `review-gate-stale-session-rework` UX research (`research/ux.md` there: "reduce the... 
  anxiety of relying on ephemeral toasts for an automation system the user has delegated real
  trust to"). A durable, always-visible badge (not just a one-shot toast) is reassuring in the
  same way that project's durable `backlog-stuck` list is — this is the same emotional job
  applied to the main session list instead of the backlog queue.
- **Social job**: "fewer 3am pages from a stuck agent" per the task brief — for this
  single-user, self-hosted deployment (per requirements.md's Users/Consumers section,
  "Constraints" section explicitly states no multi-tenant/auth considerations), this is best
  read as *self*-social: fewer moments of finding out hours later, via an external signal
  (Slack, a manual check), that a session had silently died, versus catching it proactively
  from inside the app. It reinforces why the notification path matters even though it's
  optional — for the specific case of *not currently looking at the session grid*, the badge
  alone can't serve this job; only the notification can.

## Summary for Phase 3 (plan.md) to carry forward

1. Reuse `warningBg`/`warningText`/`warning` tokens + the `chipStaleWork`-style icon(🟠)+text
   ("Stale") pill shape for the new `SessionCard.tsx` badge — don't invent new colors/icons.
2. Place it as a new peer in the existing header badge row (`SessionCard.tsx:505-548`),
   following the same `role="img"`/`aria-label` pattern already used by every sibling badge
   there.
3. Gate staleness computation (badge, grouping-strategy membership, and notification) to
   `SessionStatus.ACTIVE` only — explicitly exclude PAUSED/STOPPED/HIBERNATED/CREATING, per
   the precedent at `SessionCard.tsx:23,543`. This is a correctness requirement, not a
   polish item.
4. The card-badge threshold is a third, distinct, config-driven value — do not silently
   inherit the Review Queue's 5min or the stuck-detector's 2h; both are tuned for a different
   consumer and sensitivity.
5. New `GroupingStrategy.Stale` needs both the enum value AND a `GroupingStrategyLabels`
   entry — the labels map is a `Partial<Record<...>>`, not compile-enforced like
   `stuckReason.ts`'s maps, so this is an easy step to silently skip.
6. Reuse notification type `"warning"` (already used for `AttentionReason.STALE` via
   `reviewItemToNotificationType`, `NotificationContext.tsx:92-93`) and the existing
   `sessionId:notificationType` dedup-key shape to avoid flapping/duplicate notifications.
