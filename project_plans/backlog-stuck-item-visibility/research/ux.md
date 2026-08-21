# UX Research: Backlog Stuck-Item Visibility

## 1. Comparable UX patterns (single-user framing)

The pattern that recurs across every "needs attention" surface examined — GitHub's own
review-required badges, Dependabot/Renovate dashboards, CI "action required" views, and
Linear/Jira blocked swimlanes — decomposes into three layers that are worth separating
explicitly:

- **A persistent counter badge** near primary navigation (GitHub's PR review-required
  count, a CI status dot, Renovate's dashboard issue). It answers "is there anything at
  all?" in under a second, with zero navigation. This repo already has this exact
  pattern twice: `ReviewQueueNavBadge` and `UnfinishedNavBadge` (both
  `web-app/src/components/*/*.tsx`), both hidden/near-zero-friction, both driving into a
  full view on click.
- **A browsable list view grouped by reason/category**, not a flat list. Linear's
  "Blocked" swimlane groups by blocking relationship; Dependabot dashboards group by
  ecosystem; GitHub Actions groups failed runs by workflow. For this feature, group by
  the 4 stuck-reason classes (merge-ready-no-signal, rework-cap-exhausted,
  restart-amnesia-drift, in_progress↔review flapping) rather than by repo or item —
  the reason is the actionable unit, the item is secondary.
- **No assignment/ownership chrome.** Team tools (Jira swimlanes, GitHub's "Assigned to
  you" filters) spend significant UI real estate on who-owns-what. None of that applies
  here — single user, single owner. Every pixel spent on avatars/assignee pickers would
  be pure overhead. Skip it.

What works well specifically for a **solo** tool, evidenced by this repo's own
`ReviewQueueNavBadge` + `GitHubPRsSection` combo: badge for existence, section header
with an inline count (`{prs.length}`) for scale, filter chips for narrowing
(`FilterBar` in `GitHubPRsSection.tsx`), collapsible grouping so a quiet day collapses
to nothing. This is a proven, already-shipped pattern in this codebase — reuse it
rather than inventing a new one.

## 2. User mental model: solo developer checking "what needs my attention"

Tyler's stated workflow (per requirements.md) is a single operator scanning before a
context switch. The mental model is **triage, not investigation**: a fast glance that
answers "is anything on fire, and if so, which fire is worth my next 5 minutes." This
implies:

- The default view must be scannable in the same motion as checking the
  `ReviewQueueNavBadge` — a count + one-line reason per item, not a deep report. Detail
  (why exactly is item X stuck, since when, what was the last state transition) belongs
  behind a click/expand — mirroring the existing `UnfinishedItem` → `UnfinishedItemDetail`
  expand-on-click pattern (`web-app/src/components/unfinished/UnfinishedItem.tsx` lines
  54–119), not inline in the list.
- Sort/priority matters more than search here (only ~6-10 items expected at once per the
  live investigation baseline), so a "stuck the longest" or "most severe reason first"
  default ordering beats requiring the user to filter.
- A stuck item that has been sitting for 3 days (PR #148) is the highest-value single
  fact this feature can surface — duration-since-stuck must be visible at the glance
  level, not just in the detail view, exactly as `UnfinishedTab.tsx` already surfaces
  "Last scanned Ns ago" and PR cards surface `formatRelativeTime`.

## 3. Accessibility requirements (repo-consistent)

This repo enforces accessibility conventions via both existing UI code and CI gates
(CLAUDE.md: "UX analysis CI... Axe Core (blocks on WCAG AA violations), Lighthouse CI").
Concretely, any new component must:

- Use `data-testid` and/or ARIA roles exclusively for locators — per
  `.claude/rules/e2e-test-conventions.md` rule 3, CI-enforced for all specs in
  `tests/e2e/`. New Playwright coverage for this feature must follow the same rule.
- Announce live state changes with `aria-live` where appropriate. `NotificationToast.tsx`
  sets `aria-live="assertive"` for approval-needed and `"polite"` otherwise (line 166) —
  a stuck-item view that updates in place (e.g., an item resolves while the user is
  looking) should use `aria-live="polite"` on the count/summary region so screen reader
  users aren't interrupted for a routine background reconciliation update.
  Use `role="alert"` only for something actionable/urgent, matching the toast's
  existing convention (line 165).
  - Expandable cards need `aria-expanded` + keyboard support (Enter/Space to toggle,
    Escape to collapse) — already implemented in `UnfinishedItem.tsx` (lines 31-42,
    56-63) and should be copied verbatim for the new item cards.
- Badges must carry `aria-label` with the full count phrase, not just the numeral —
  see `UnfinishedNavBadge.tsx` line 24 (`` `${count} unfinished item${count !== 1 ? "s" : ""}` ``)
  and `ReviewQueueNavBadge.tsx` line 44. Follow the same phrasing convention for a new
  "N items stuck" badge.
- Filter/toggle controls use `aria-pressed` (see `UnfinishedTab.tsx` line 138,
  `NotificationPanel.tsx` line 312) and grouped filters get `role="group"` with an
  `aria-label` (`UnfinishedTab.tsx` line 132, `NotificationPanel.tsx` line 306) — reuse
  for any reason-class filter chips.
- No color-only signaling: every status chip in this codebase pairs a color class with
  a text label (`BacklogItemBadge.tsx`'s `STATUS_CLASS` map + `getStatusLabel`,
  `GitHubPRsSection.tsx`'s `chipSuccess`/`chipError` text like "✓ CI" / "✗ CI") — the 4
  stuck-reason chips must follow the same text+color pairing, not rely on a colored dot
  alone.

## 4. Error states and edge cases

- **Reconciler can't determine PR status (GitHub API error/rate limit):** Model this as
  a distinct, visible state — not a silent omission and not conflated with "not stuck."
  Precedent: `GitHubPRsSection.tsx`'s `DeviceAuthBanner` renders an explicit
  `authUnavailable` banner state (line 653, `authState && !authState.available`) rather
  than just showing an empty PR list when GitHub auth/API is unavailable. Apply the same
  principle: an "unknown / couldn't check" reason class (5th implicit state) with its
  own chip, so the user isn't misled into thinking a PR is fine when the reconciler
  simply failed to ask.
- **Zero stuck items (success/empty state):** Every list view in this codebase has a
  deliberately reassuring empty state, not a bare blank area:
  `UnfinishedTab.tsx` line 152 ("No unfinished work found. All repos are clean."),
  `NotificationPanel.tsx` lines 329-337 (icon + text + subtext, contextual to whether a
  filter is active), `GitHubPRsSection.tsx` line 680 ("No open pull requests found.").
  The new view should say something equivalently confidence-building, e.g. "Nothing
  stuck — all backlog items are progressing," and must distinguish "empty because
  nothing is stuck" from "empty because a filter is active" (both existing empty states
  do this distinction already — copy it).
- **Stuck reason changes while the user is looking (live update vs. stale snapshot):**
  This repo already solves the general "background reconciliation changes what's on
  screen while open" problem via context + polling (`useUnfinishedWork`,
  `useReviewQueueContext`, `useNotifications`). The precedent is: don't snapshot-lock the
  view; let it re-render on the next poll/event, but avoid jarring reflow — e.g. keep
  the item in place with an updated chip rather than having it jump position/disappear
  mid-glance. Consider a brief transition (this repo already has toast enter/exit
  animation classes as precedent — `NotificationToast.css` `visible`/`exiting`) if an
  item resolves out of the list while expanded, so it doesn't vanish out from under an
  open detail panel.
- **Restart-amnesia specifically requires this view to be backed by persistent storage**
  (per requirements.md root cause 3), not the in-memory approach the current stuck-item
  bookkeeping uses — otherwise the UI would itself suffer the same amnesia after a
  service restart and mislead the user into thinking a rework-cycle item is now healthy.
  That's a backend concern, but the UX implication is: the "since when" duration shown
  must be sourced from a persisted first-seen timestamp, not from process-uptime-relative
  state, or the displayed duration will reset every restart (this instance restarts
  many times/day per CLAUDE.md) and silently misinform the user exactly like the toast
  problem this feature is meant to fix.

## 5. Jobs-to-be-done

- **Functional job:** "Tell me, in the 10 seconds before I switch away from this
  instance, whether anything needs a decision from me — and if so, which specific
  decision (merge this PR / abandon this item / investigate this loop) — without me
  having to open sqlite or grep logs."
- **Emotional job:** Confidence that nothing is silently rotting. The problem statement's
  own evidence (PR #148 sat 3 days, 6 parked review items) is a trust violation — the
  tool went silent when it should have spoken up. This feature's job is to restore trust
  that "if the toast didn't catch it, I'll still find it here" — i.e., the persistent
  view is an insurance policy against missed ephemeral signals, not just a nicer
  notification.
- **Social job:** None — single user, self-hosted, no team visibility/accountability
  dimension applies. Any pattern from team tools that implies "showing this to others"
  (e.g., a shareable status page, an assignee avatar) is out of scope per the explicit
  out-of-scope note on remediation actions and the single-user framing in Users/Consumers.

## Codebase-specific findings

- **`/unfinished` page** (`web-app/src/app/unfinished/UnfinishedTab.tsx`): toolbar with
  title + scan-freshness text + manual refresh button, filter chip row
  (`role="group"`), a `GitHubPRsSection` (collapsible, its own stats bar + filter/sort +
  search), then repo-grouped worktree cards. Cards (`UnfinishedItem.tsx`) are
  click-to-expand with hover-reveal action buttons (dismiss ×, snooze), status shown as
  colored chips (`chipUncommitted`, `chipAhead`, `chipBehind`, `chipTimeout`). This is
  the strongest visual/structural precedent to extend if the new stuck-items view is
  added as a section of this page rather than a wholly new page — it already handles
  scan-freshness display, grouping, filter chips, and expand/collapse consistently.
- **Notification-to-persistent upgrade path**: `NotificationToast.tsx` is the ephemeral
  signal (auto-close, auto-minimize per `lib/notification-policy.ts`, `role="alert"`);
  `NotificationPanel.tsx` is its "upgrade to persistent" sibling — same data
  (`NotificationData`), same type/priority taxonomy
  (`lib/utils/notificationMapping.ts`: `notificationTypeIcon`, `notificationTypeLabel`,
  `priorityColor`), but rendered as a searchable/filterable list with read/unread state,
  a collapsible "auto-handled" section, and deep links (`View in Backlog`,
  `View Session`). This is the direct template for "what does upgrading a toast to
  persistent look like visually" — the new stuck-item view should reuse this same
  toast→panel duality: keep existing toasts for the moment something becomes stuck, but
  guarantee every stuck condition also lands in a queryable, always-there list (this
  repo's `NotificationPanel` already proves the query/filter/search chrome pattern:
  search input line 298-305, type filter pills 306-317, empty states 328-338).
- **Status/reason chip conventions**: `BacklogItemBadge.tsx` maps a status string to a
  CSS class via a lookup table (`STATUS_CLASS`) plus a human label helper
  (`getStatusLabel` from `lib/backlog/status`) — the correct pattern to follow for the 4
  new stuck-reason chips (e.g. `STUCK_REASON_CLASS` + `getStuckReasonLabel`), keeping
  color and text label together per accessibility conventions above.
- **CSS architecture**: any new component must be `.css.ts` (vanilla-extract), colocated
  with the component, referencing `vars.*` tokens from the shared theme contract per
  `.claude/rules/css-architecture.md` — no new `.module.css` files, no hardcoded hex/
  `var('--x')` strings. Existing `.css.ts` siblings (`UnfinishedItem.css.ts`,
  `NotificationPanel.css.ts`, `BacklogItemBadge.css.ts`) are the direct style templates
  to follow for chip variants, card layout, and collapsible section styling.
