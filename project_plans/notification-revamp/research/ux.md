# UX Research: notification-revamp

Research Agent 5 (UX), Phase 2. Scope: the six in-scope items in
`project_plans/notification-revamp/requirements.md`'s Scope section. Read in
full before this research; not re-derived here.

## Codebase grounding (read before the patterns below)

Some of what comparable products do, this codebase has already half-built:

- `web-app/src/lib/utils/notificationGrouping.ts`'s `groupNotifications()`
  already groups by `(sessionId, notificationType)` and computes
  `count = max(representative.occurrenceCount ?? 0, group.length)` — this is
  **client-side** compensation for the exact server-side bug Failure Mode 1
  describes (`NotificationHistoryStore.Append()` forking rows post-read). The
  IA reboot (item 5) should keep this grouping component but stop needing the
  `group.length` fallback branch once the backend fix (item 1) lands —
  `occurrenceCount` alone becomes authoritative.
- `web-app/src/components/ui/NotificationItem.tsx` already has an
  `AutoHandledSection` (line ~304, label "Auto-handled") — item 5's "extend
  it to include rule-reconciled items" is additive to an existing section,
  not new IA.
- `web-app/src/components/sessions/ReviewQueueBadge.tsx` already renders
  priority with an emoji + text abbreviation + `aria-label` (e.g.
  `aria-label="LOW priority: <reason>"`, line 91) — not color alone. The
  screenshot's "plain white circle" for LOW is very likely a different,
  simpler badge render path (worth confirming in Phase 3 which component the
  live Review Queue page actually uses — `ReviewQueuePanel.tsx` has its own
  status-chip rendering separate from `ReviewQueueBadge.tsx`) rather than a
  missing pattern to invent from scratch.
- `web-app/src/components/ui/NavBadge.tsx:46` caps the nav badge at `99+`
  (`count > 99 ? "99+" : count`) — confirmed the cap is intentional, tested
  (`NavBadge.test.tsx`), and separate from the Review Queue's own uncapped
  count-in-parens patterns (`ReviewQueuePanel.tsx:1313`,
  `` `⏭ Skip all (${skippableItems.length})` ``, no cap). The 152/107 raw
  counts in the audited screenshots are *body copy* (page headers), not the
  capped nav badge — see Mental Models below for why that distinction matters.
- `ReviewQueuePanel.tsx` already has a full grouping engine
  (`GroupingStrategy` from `@/lib/grouping/strategies`, reused from
  `SessionList`) with a `groupSection`/`groupHeading` CSS API — item 6's
  "visually separate priority tiers" can most likely be a new
  `GroupingStrategy.Priority` (or a hardcoded three-tier partition ahead of
  the existing grouping) rather than a bespoke rendering layer.

Net effect on planning: several of the "big lift" IA items are extending
existing, tested primitives, not building new component families from zero.
The gap is consistently "the backend doesn't feed these primitives clean
data" (occurrence counts, idle-ack-suppression, priority tiers actually
varying) more than "the frontend has no vocabulary for this."

## 1. Comparable UX patterns

| Product | Mechanic | Source | Maps to |
|---|---|---|---|
| GitHub Notifications | **Participating vs. Watching** split: notifications you're @mentioned/authored on vs. everything from repos you watch are separable inbox views, not flattened together | [About notifications](https://docs.github.com/en/subscriptions-and-notifications/concepts/about-notifications) | Item 5 (Notifications IA): the "needs a decision" vs. "informational activity" split is structurally the same move — separate by *why you're seeing this*, not just recency. |
| GitHub Notifications | **"Mark as Done" is a distinct action from "Unsubscribe"** — Done clears it from the inbox (can resurface via `is:done` search) while Unsubscribe stops future notifications from that thread entirely. Two different permanence contracts under two different verbs. | [Managing your subscriptions](https://docs.github.com/en/subscriptions-and-notifications/how-tos/managing-subscriptions-for-activity-on-github/managing-your-subscriptions) | Items 2 and 5: the "Skip" vs. dedup-then-collapse distinction needs the same verb discipline — see Mental Models below, this is the single biggest naming risk in the current UI. |
| Linear Triage/Inbox | **Snooze**: hides an item until a chosen time *or* until there's new activity on it, whichever comes first — not a permanent dismissal, and it auto-returns on its own trigger rather than needing the user to remember | [Triage docs](https://linear.app/docs/triage), [Inbox snooze changelog](https://linear.app/changelog/2021-06-17-inbox-snooze-and-easier-issue-merge) | Item 2 (idle-class ack suppression): "acknowledge until state materially changes" (per Success Metrics) is exactly Linear's snooze contract — dismissed-until-condition, not dismissed-forever. This is strong external validation that the requirement's chosen semantics ("stays out until state changes, not permanently") match a proven pattern, rather than inventing a novel contract. |
| Linear Triage | **Single-key triage actions on an opened item** (accept/duplicate/decline/snooze, one keypress each) — the queue is designed to be processed serially, one decision at a time, not scanned as a wall of rows | [Practical guide to Linear Triage](https://www.issuelinker.com/blog/linear-triage) | Item 6 (Review Queue IA): reinforces "needs a decision" items should support fast keyboard/single-tap triage, not just visual sorting — relevant to the existing `ReviewQueuePanel.tsx` bulk-skip and per-row Skip button already present. |
| PagerDuty | **Severity drives incident urgency and notification routing** — severity is a property of the underlying signal (critical/warning/error), and it's what determines whether you get paged at all, not just how a row is colored after the fact | [Alerts](https://support.pagerduty.com/main/docs/alerts), [Incident severity classification](https://www.pagerduty.com/resources/incident-management-response/learn/incident-severity-classification/) | Item 6: reinforces that `Priority`/`RiskLevel` (already threaded through per the Prior Art section) should gate *default visibility/ordering*, not just add a badge color — low priority items should be collapsed by default, matching PagerDuty treating low-severity as non-paging, not merely deprioritized-looking. |
| PagerDuty | **Auto-resolve with configured re-trigger window** — an incident can auto-resolve after a time window, and a re-ack window exists so an auto-resolved item can bounce back if the condition recurs, rather than "auto-resolve" being an unconditional, un-auditable dead end | [Configurable Service Settings](https://support.pagerduty.com/main/docs/configurable-service-settings) | Item 4 (rule-reconciliation auto-resolve): validates the requirement's "visible Auto-resolved by rule: `<name>`" note as the industry-standard shape for an auto-resolve action — it must be inspectable/attributable, not silent, matching `feedback_document_ai_decisions_in_edge_cases` house memory. |
| Gmail | **Collapsed thread view**: a multi-message conversation renders as one row with a count and only expands on click; the "Nudge"/digest treatment groups by conversation, not by individual message-received event | General Gmail UX (no single canonical doc; behavior is directly observable) | Item 5: directly the same move as `groupNotifications()`'s existing `(sessionId, notificationType)` grouping — validates collapsing by *session*, and suggests the collapsed row should be click-to-expand rather than requiring a separate page/filter to see the individual occurrences. |

## 2. Mental models

**Skip vs. Dismiss vs. Mark read vs. Resolve — these are not interchangeable
verbs, and the codebase currently uses them loosely.**

- **"Skip"** (used today in `ReviewQueuePanel.tsx`) reads to a user as
  *session-scoped, temporary, no judgment implied* — "not now," similar to
  Gmail's "Snooze" or Linear's snooze. A user expects a skipped item to come
  back if the underlying condition persists or recurs. This matches the
  Success Metrics requirement almost exactly: "keeps it out of the queue
  until its underlying state materially changes" — so keep calling it Skip,
  but the current bug (idle items reappearing on the *next poll tick alone*,
  not on a real state change) breaks the mental model the word itself
  creates. The fix in item 2 isn't just adding suppression, it's making the
  UI's existing promise ("Skip" implies "will come back only when something
  changes") actually true.
- **"Dismiss"** (not currently used in this codebase's Review Queue/
  Notifications copy, per the grep of `ReviewQueuePanel.tsx`/
  `NotificationItem.tsx`) more strongly implies *permanent, "I've seen this
  and I don't need to see it again in this form"* — closer to GitHub's
  "Mark as Done." Recommend reserving "Dismiss" (if introduced) exclusively
  for the notification-row collapse action, distinct from "Skip"'s
  session-scoped, come-back-later contract, so the two words carry two
  different permanence promises instead of becoming synonyms a user has to
  learn are actually different.
- **"Mark read"** is the weakest-permanence action of the four: it only
  changes visual weight (bold vs. not), never removes anything from a list
  or changes underlying state. Users generally expect it to be reversible
  ("mark unread") and non-destructive.
- **"Resolve"** (relevant for item 4's rule-reconciliation) is the
  strongest-permanence, most consequential of the four — closest to
  PagerDuty's incident resolution. A user expects "resolved" to mean a
  decision was actually made (approve/deny), not just hidden. This is why
  the requirement's insistence on a visible "Auto-resolved by rule: `<name>`"
  audit note matters for mental-model integrity: an *auto*-resolve looks
  identical in the list to a *human* resolve unless the UI marks the
  provenance — a user scanning history later needs to be able to tell "I
  decided this" from "a rule decided this on my behalf," or trust in the
  resolved state erodes.

**Badge count "99+" / "9+".** An uncapped raw count (152 notifications, 107
review-queue items, per the audited screenshots) does not communicate
urgency or magnitude past a small threshold — once a number exceeds what a
person can meaningfully triage in one sitting, more precision adds nothing
actionable and instead reads as "this is broken/unmanaged," i.e., it erodes
trust rather than informing a decision. `NavBadge.tsx`'s existing 99+ cap
(confirmed via its own test suite) already reflects this correctly for the
*nav badge* — the actual regression is that the **page body** (not the badge)
displays raw counts like "152 notifications" as if that were useful
information, which is where the "reads as broken" complaint in the
requirements' screenshots is actually coming from. The fix belongs in item 5
(don't show a raw uncapped total as a headline stat; show a small, capped
"needs a decision" count and let the rest live in a collapsed, unbadged
section) rather than in the nav badge component, which already does the
right thing.

## 3. Accessibility

- **`aria-live` for the "needs a decision" section only, at `polite`, not
  `assertive`.** WCAG 2.1 SC 4.1.3 (Status Messages) requires status changes
  to be programmatically determinable without requiring focus; the
  authoritative guidance is that `assertive` should be reserved for
  time-critical interruptions and overusing it "can disrupt the user's
  workflow by interrupting the screen reader frequently" ([UXPin: ARIA Live
  Regions for Dynamic Content](https://www.uxpin.com/studio/blog/aria-live-regions-for-dynamic-content/)).
  A session going idle→needs-review is not time-critical in the way a
  security alert would be — `polite` is correct so a screen-reader user
  isn't interrupted mid-task every time a background session state changes.
  Do not wrap the whole page (including the collapsed/informational section)
  in a live region — only the count/heading of the actionable section should
  announce changes, and the live region content should be kept short (e.g.
  "3 items need a decision," not the full list re-announced on every change).
- **Don't put an `aria-live` region in the DOM only when content changes.**
  Best practice is to have the live region present (empty) on initial render
  so assistive tech has already registered it before the first update fires;
  a region injected dynamically needs a buffer (commonly ~2s) before the
  first announcement is reliably picked up ([UXPin](https://www.uxpin.com/studio/blog/aria-live-regions-for-dynamic-content/)).
  Relevant here because both pages already poll (~2s interval, per
  requirements' Non-functional Requirements) — the live region should exist
  from first paint, not be conditionally mounted only once the first
  actionable item appears.
- **Priority/severity must not be color-only** (explicitly named in the
  task: the screenshot's LOW badge as "a plain white circle"). Per the code
  grounding above, `ReviewQueueBadge.tsx` already does this correctly —
  emoji + text abbreviation + descriptive `aria-label`, not color alone —
  confirm during Phase 3 which rendering path the live page actually uses,
  since the screenshot suggests a different, simpler chip is in play
  somewhere in `ReviewQueuePanel.tsx`'s own render code. Any *new* priority
  UI (e.g. a `GroupingStrategy.Priority` section) must carry the same
  text/shape encoding, not just reuse the same background-color tokens.
- **Collapsible groups**: use native `<details>/<summary>` where feasible, or
  `aria-expanded` + `aria-controls` on a real button if custom-styled — a
  `div` with a click handler and no ARIA state is invisible to assistive
  tech as "collapsible" at all. Group headings (session name, occurrence
  count) need to be in the accessible name so a screen-reader user gets "3
  events, Session `foo-bar`, collapsed" rather than just a bare count.
- **Contrast**: `Priority.LOW`/`MEDIUM`/`HIGH`/`URGENT` color tokens
  (`ReviewQueueBadge.css.ts`) need a WCAG AA contrast check (4.5:1 body text,
  3:1 for large text/UI components) against both light and dark backgrounds
  — this repo's own `ui-web-design-guidelines` skill and the CI-enforced Axe
  Core gate on `web-app/src/` PRs (per this repo's CLAUDE.md) will catch
  regressions, but a color audit is worth doing proactively during Phase 3
  design rather than relying solely on CI to catch it post-hoc.

## 4. Error/edge-case UX

**Race: rule-reconciliation auto-resolves an item the user has open/mid-review.**
This is a real conflict between two systems both trying to "finish" the same
review item — the backend reconciliation pass (item 4) and the user's own
in-progress decision. Recommended treatment, informed by the requirement's
own house-memory citation (`feedback_document_ai_decisions_in_edge_cases`:
AI actions must be visible, never silent):

- The UI must not silently pull the item out from under the user (e.g. the
  approve/deny buttons vanishing with no explanation while they're reading
  it) — that reads as a bug, not a resolved state.
- On receiving the reconciliation event for an item currently open in the
  UI, show an inline banner *on that item* ("Auto-resolved by rule:
  `<name>` while you were viewing this — no action needed") and disable
  (not hide) the action buttons, replaced with a "why" link/tooltip pointing
  at the rule. This mirrors PagerDuty's re-trigger-window pattern in spirit:
  the system's decision is visible and explained, not just enforced.
  Because `WatchReviewQueue`'s wire protocol is explicitly frozen for this
  project (Out of Scope: "Any change to `WatchReviewQueue`'s wire protocol
  beyond what's needed for the new reconciliation event"), plan for exactly
  one new event type/field carrying the rule name + resolution, not a
  general-purpose diff/patch event.
- If the user has already begun submitting a decision (in-flight
  approve/deny request) when the reconciliation fires, the backend should
  treat the human decision as authoritative if it lands first (last-writer
  problems here should favor the human, not the automation) — this is a
  Phase 3 design/backend concern, not just UI, but the UI needs to surface
  whichever one actually won ("You approved this" vs. "Auto-resolved by rule
  before your approval was received") rather than presenting an ambiguous
  final state.

**Empty state for "needs a decision" — this is the success state.** A
zero-item "needs a decision" section is the entire point of the project
(per the Problem Statement: restoring trust that the page tells you what
actually needs action) and should be designed to feel *earned and calm*,
not like an error page or a broken/empty table:

- Avoid generic empty-state copy ("No items found") that reads the same as a
  broken filter or a loading failure — say something that confirms the
  system is *working*, e.g. "Nothing needs your attention right now" or
  "All caught up" with a small positive affirmation (checkmark icon, not a
  ghost/empty-box illustration that implies absence-as-problem).
  This is the same distinction GitHub Notifications' `is:done` view and Gmail's
  "Inbox Zero" state both make — the visual language for "you cleared
  everything" is deliberately different from "there's nothing here because
  something's wrong."
- Do not collapse the "recent/completed activity" section into the empty
  space where "needs a decision" was — keep it visible below, still
  collapsed by default, so the user can still audit what happened without
  it competing for attention now that there's nothing urgent.
- This state should be reachable and demonstrable in a test/fixture (a
  golden "empty needs-decision, N grouped informational items" screenshot)
  so it doesn't regress silently — visual states like this are exactly the
  kind of thing that erodes without an explicit check when someone later
  changes the section's conditional rendering.

## 5. Jobs-to-be-done

- **Functional job**: "Tell me what needs a decision right now, and let me
  trust that once I've acted on something (skip/approve/deny/dismiss), it
  won't reappear unless something has actually changed." This is the whole
  project in one sentence — every one of the six in-scope items is in
  service of this single job. The corollary functional job for informational
  items is "let me audit what happened without it competing for my
  attention with what needs a decision" (the grouped/collapsed
  recent-activity sections in items 5 and the extended Auto-handled section).
- **Emotional job**: *Trust that a dismissal is safe and permanent
  (within its stated contract)*, and *relief from the anxiety of a growing,
  unbounded badge count*. The current state actively works against both:
  Failure Mode 1 means "I marked this read" doesn't stick past the first
  occurrence, and Failure Mode 2 means "I skipped this" doesn't stick at
  all — both train the user to distrust their own actions on the page,
  which is worse for the emotional job than the page simply being ugly. The
  152/9+ raw counts in the screenshots are themselves an anxiety trigger:
  an uncapped number that only ever grows, with no legible path to zero,
  reads as "this will never be done," which is the opposite of the
  emotional job the page should be doing. Every one of the "visible audit
  note, never silent" requirements (item 4's auto-resolve note, the
  race-condition banner in section 4 above) exists specifically to protect
  this trust — a silent auto-action, even a correct one, is a bigger trust
  cost than a visible one that turns out to need a correction.
- **Social/identity job**: not applicable. This is confirmed, not assumed —
  per requirements ("Users / Consumers: Single operator (Tyler)") and this
  agent's own instructions, there is no team, no shared inbox, no one else
  who sees this UI or is impressed/judged by how the operator triages it.
  Noting this explicitly rather than inventing a social angle (e.g. "feel
  like a competent engineer for keeping inbox zero") that would be
  unfalsifiable set-dressing for a single-operator internal tool.

## Summary of UX-driven inputs to Phase 3 planning

1. Reuse existing components (`groupNotifications`, `AutoHandledSection`,
   `ReviewQueueBadge`, `ReviewQueuePanel`'s `GroupingStrategy` engine) —
   don't rebuild grouping/badge primitives from scratch; confirm in Phase 3
   which review-queue badge render path actually shipped the screenshot's
   plain-white-circle LOW badge, since `ReviewQueueBadge.tsx` itself already
   does the right thing.
2. Keep "Skip" as the item-2 verb (its session-scoped, temporary mental
   model already matches the required semantics) — the bug is that the UI
   doesn't yet keep its own promise, not that the word is wrong. Reserve a
   separate verb ("Dismiss") if a permanent action is ever introduced.
3. Auto-resolve actions (item 4) and the mid-review race condition need a
   visible, attributable UI treatment, not just a backend log line — the
   Observability Requirement's structured log is necessary but not
   sufficient for user trust.
4. The "needs a decision" empty state is a first-class design deliverable,
   not a fallback — it's the page's actual success condition.
5. Badge-count design should follow `NavBadge.tsx`'s existing 99+-cap
   precedent everywhere counts surface, including page-body headline
   numbers, which currently violate it.
