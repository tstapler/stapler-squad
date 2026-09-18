# UX Research: backlog-bounce-escalation

## 1. Comparable escalation patterns

**Incident-management severity levels (SEV1–SEV5)** and **SLA breach badges** (Jira/ServiceNow)
converge on the same shape: a small, fixed number of discrete states (on-track → at-risk →
breached), each rendered as a label + color, never color alone, and escalation is
**threshold-based, not continuous** (e.g. ServiceNow/Jira SLA automations commonly fire at 50%
of budget as an early warning and again at 75–100% as a stronger one — see
[Atlassian: 15 ways to connect SLAs to Jira Automation](https://community.atlassian.com/forums/App-Central-articles/15-ways-to-connect-SLAs-to-Jira-Automation-and-stop-fighting/ba-p/3197762),
[ServiceNow: SLA breach notifications](https://www.servicenow.com/community/itsm-articles/how-to-trigger-sla-breach-notifications-in-servicenow-and-show/ta-p/3499319)).
The key non-alarming design choice these sources converge on: **the best alert is the one that
gives you time to act, not one that announces failure** — i.e. the badge should read as "this
now needs a different kind of attention" rather than "something broke."

**GitHub Actions repeated-failure banners** apply a similar idea passively: the banner only
appears after N consecutive failures (not on the first), and it's a persistent inline element
on the workflow page, not a push notification — visibility without interruption.

**Applicability to this project**: the requirements doc's own framing ("distinguished... without
requiring the user to cross-reference the stuck-state table by hand") matches the SLA-badge
job exactly. But this is a **single-user, non-interruptive** tool — there is no "the right
people react" audience to page. That argues against SEV-style paging/notification escalation
and toward a **persistent, queryable visual marker** (a badge/count), which is also what
requirement item 2 explicitly asks for ("durable... not a one-time toast").

## 2. Existing pattern to extend (do not replace)

The prior `backlog-stuck-item-visibility` project already built almost every primitive this
feature needs, in `web-app/src/components/backlog-stuck/`:

- **Multi-reason cross-reference badge already exists.** `StuckItem.tsx` (lines 23-24, 315-323)
  takes `otherReasonsCount`/`otherReasonLabels` props and renders `· also stuck for {N} other
  reason{s} ⓘ` next to the identity line, with the full reason list in a `title` tooltip. This is
  **exactly** requirement item 1's "N simultaneous reasons" signal — it currently just labels the
  count without escalating severity. The natural extension is a threshold on this same count
  (e.g. `otherReasonsCount >= 1`, i.e. 2+ total reasons) that swaps in a distinct visual
  treatment, not a new component.
- **Capped-while-bouncing already has a hook point.** `isParked` (`remediationAttempts >=
  MAX_REMEDIATION_ATTEMPTS`, `StuckItem.tsx:76,209`) already gates the "Retry now" button and its
  `aria-label`/`title` text ("Automated retries have been exhausted... use Reset to try again").
  Requirement item 2 wants this differentiated specifically when `reason === BOUNCING` — a
  narrower condition than plain `isParked`, reusing the same field.
- **Resolution-while-viewing already has a pattern.** `StuckItemsSection.tsx`'s `resolvedGhosts`
  effect (lines 91-121) + `StuckItem.tsx`'s `justResolved`/`resolvedMessage` props/banner
  (`cardResolved` class, "✓ ... It will be removed from this list shortly", auto-removed via
  `setTimeout`) is the existing answer to research question 4 — see below.
- **Severity/color convention**: `stuckReason.ts`'s doc comment is explicit — reason
  label/icon/class maps intentionally avoid ranking reasons by danger ("never color-only"), and
  `StuckItemsSection.tsx`'s `GROUP_ORDER` comment states display order is "by typical
  actionability, NOT severity ... must never be read as a danger/severity ranking." **A new
  escalation signal must not repurpose the existing reason-chip colors for severity** — it needs
  its own, additive visual element (e.g. a separate badge/border), or it will contradict this
  explicit prior design decision.
- **No push/toast infra found** in `useStuckBacklogItems.ts` or `StuckItemsSection.tsx` — the
  "one-time toast" the requirements doc references is server-emitted (see
  `session/backlog_lifecycle_review.go:208,660` per requirements doc), separate from this
  component tree. The durable escalation marker the requirements doc wants is a natural fit as
  **another field on the existing `StuckBacklogItem`/stuck-state row** (queryable, survives
  restarts) rather than a new notification channel — consistent with the "visibility, not a
  control panel" precedent this rule enforces.

## 3. Accessibility

- **Never color-only.** Every existing chip pairs an icon with a text label (`stuckReason.ts`
  line 14 comment) — any new severity indicator must follow the same rule: text, not just a
  red/orange dot.
- **aria-live discipline already established and must be matched, not escalated.** Grep across
  `web-app/src/components/backlog{,-stuck}/` shows a consistent split: `aria-live="polite"` +
  `role="status"` for informational/count updates (`StuckItemsSection.tsx:361`,
  `GateVerdictBox.tsx:255`, `TriageLoadingIndicator.tsx:45`), reserving `role="alert"` +
  `aria-live="assertive"` exclusively for genuine transient errors (`InlineError.tsx:56,106`).
  `StuckItemsSection.test.tsx:157` has a standing test asserting the item-count region uses
  `aria-live="polite"`, "never `role=alert`." An escalation badge is a state change, not an
  error — it must use `polite`, matching the existing convention, not `assertive`/`alert` (which
  would read as "urgent, single-user tool" alarm-fatigue territory the SLA research above warns
  against).
- **Tooltip content needs a non-hover path.** `otherReasonsCount`'s reason list is currently only
  exposed via the `title` attribute (hover tooltip) — not accessible via keyboard/touch/screen
  reader without also being in the accessible name or an expandable region. Any new escalation
  detail (e.g. "why is this elevated") should follow `StuckItemDetail.tsx`'s existing
  expand-on-click pattern (already keyboard-accessible via `StuckItem.tsx`'s
  `role="button"`/`tabIndex`/`aria-expanded`/Enter-Space-Escape handling) rather than adding a
  second hover-only tooltip.
- **Focus preservation.** `StuckItem.tsx:137-142` returns focus to the card's own toggle on
  collapse — any new escalation-detail expansion must preserve this, not introduce a new focus
  trap.

## 4. Edge case: item resolves while an elevated marker is showing

The existing `justResolved` ghost pattern (`StuckItemsSection.tsx:91-121`,
`StuckItem.tsx:385-390`) is the precedent to reuse verbatim: when an item drops out of the
live stuck-items list while its card is expanded, it's kept rendered as a ghost with a
"✓ ... was just resolved. It will be removed from this list shortly." banner, then removed via a
timer. For an escalated/severity-elevated item, the same mechanism should apply — **and should
explicitly also clear the elevation, not just the underlying reason**: if the escalation trigger
was "N simultaneous reasons" and one clears (dropping the count below threshold), the badge
should downgrade or disappear on the very next poll, using the same non-abrupt confirmation
banner rather than silently vanishing. If the *last* reason clears entirely, the existing
whole-card resolution ghost already covers it. The one net-new case: an item can de-escalate
(4 reasons → 1 reason) without fully resolving — that transition currently has no analog in the
existing code and needs the same "was just resolved"-style confirmation treatment applied to the
severity badge specifically (e.g. "no longer critical — down to 1 open reason"), not just to
full-item disappearance.

## 5. Job-to-be-done

The requirements doc's own "Baseline" section is the strongest evidence here: the gap it
documents was found by **manually querying the `backlog_stuck_states` table** — i.e. the
signal already existed in durable storage, but wasn't surfaced anywhere the user would
naturally look. Combined with:

- Tyler already gets a one-time toast/notification when `MaxRemediationAttempts` is hit
  (existing infra, per Baseline).
- Success Metrics explicitly require the new signal "persists (queryable) rather than being a
  one-time toast."
- The Alternatives-Considered section explicitly rejects both "rely on manual DB queries" and
  "auto-escalate by taking a remediation action" — ruling out both the status quo and an
  urgency-signaling/paging interpretation.

This points at **visibility persistence, not urgency signaling**, as the actual job: the user
already gets notified once; what's missing is that the signal doesn't survive past that single
notification instant — if Tyler doesn't act on the toast immediately (or the browser tab wasn't
open when it fired), there's currently no way to tell, just by looking at the board later, that
"this item is in a qualitatively worse state than an ordinary stuck item." The job is closer to
"don't make me remember/re-derive that this one needs different handling" than "get my
attention right now" — which argues for the `otherReasonsCount`-badge-style persistent marker
this doc recommends extending, over any modal/toast/push mechanism.

## Sources

- [Atlassian Community: 15 ways to connect SLAs to Jira Automation](https://community.atlassian.com/forums/App-Central-articles/15-ways-to-connect-SLAs-to-Jira-Automation-and-stop-fighting/ba-p/3197762)
- [ServiceNow Community: How to Trigger SLA Breach Notifications](https://www.servicenow.com/community/itsm-articles/how-to-trigger-sla-breach-notifications-in-servicenow-and-show/ta-p/3499319)
- [UptimeRobot: Severity Levels Explained (SEV1-SEV5)](https://uptimerobot.com/knowledge-hub/monitoring/severity-levels-explained/)
- Codebase (VERIFIED via Read/Grep, this worktree):
  `web-app/src/components/backlog-stuck/stuckReason.ts`,
  `web-app/src/components/backlog-stuck/StuckItem.tsx`,
  `web-app/src/components/backlog-stuck/StuckItemsSection.tsx`,
  `web-app/src/components/backlog-stuck/StuckItemsSection.test.tsx:157`,
  `web-app/src/components/backlog/InlineError.tsx`
