# Research: UX for Review Queue Severity (P0/P1/P2)

Grounded in the actual current UI — `web-app/src/components/sessions/ReviewQueuePanel.tsx`
(renders approval items inline via `queueItem.metadata?.["pending_approval_id"]`),
`ReviewQueueBadge.tsx`/`.css.ts`, `ApprovalCard.tsx`, `ApprovalAnalyticsPanel.tsx`, and
`ApprovalRulesPanel.tsx` (which already threads `riskLevel` through `upsertRule` calls but
never renders it). No severity/risk badge exists in the frontend today — this is greenfield
UI on top of already-wired-through-partway data.

## 1. Comparable triage-product patterns

- **PagerDuty (incident urgency + severity)**: two independent axes — `urgency`
  (high/low, drives notification behavior) and a free-text severity field teams
  customize (SEV1–SEV5). The **badge is always colour + text label together**, never a
  bare colour dot, and severity is a **primary, always-visible sort/group key** on the
  incident list, not something buried in a filter drawer. Lower-severity items still
  render, just deprioritized — nothing is hidden by default.
- **Sentry (issue level)**: `fatal`/`error`/`warning`/`info`/`debug`, rendered as a small
  coloured square + text label to the left of the issue title, consistent across list and
  detail view. Sentry's list defaults to recency, not severity, but severity is always a
  one-click filter facet in the sidebar with per-level counts shown (`error (42)`) — this
  repo already does exactly this shape for `Priority`/`AttentionReason` filters in
  `ReviewQueuePanel.tsx` (`getPriorityLabel`, `byPriority.get(priority)` counts in the
  filter buttons at lines ~1043-1062).
- **GitHub Dependabot/CodeQL alerts**: severity (`critical`/`high`/`moderate`/`low`) shown
  as a coloured pill *and* the alert list defaults to a severity-first sort, with
  same-severity ties broken by recency. This is the precedent for requirements.md's open
  question about "hard primary sort key vs. secondary tiebreaker" — GitHub's answer is
  severity primary, recency secondary, and it works because both signals are visible
  together (severity pill + "opened N days ago").
- **Gastown P0/P1/P2** (from the original issue): a 3-tier scheme where P0 is business-critical/customer-facing,
  mapped by convention to "drop everything." This is an *incident-response* vocabulary,
  not a *tool-risk* vocabulary — see mental-model conflict in section 2.

**Common thread across all four**: severity is always shown as colour + text/icon
together (never colour alone), it's a first-class filter facet with visible counts, and
in every triage-style (not incident-response) product the *default* list ordering is
severity-first. Nothing in any of these tools hides low-severity items by default —
filtering is opt-in, not a default suppression.

## 2. Mental model: P0/P1/P2 vs. `RiskLevel` — conflict, and recommendation

**These conflict, not just in wording but in direction of the scale's mental anchor:**

- The issue's own vocabulary (`P0/P1/P2`, borrowed from Gastown's Deacon/Mayor
  incident-response pattern) uses **P0 = most severe**, following the universal
  incident-management convention (P0/P1/P2/P3, SEV1/SEV2, Jira Priority "Highest").
  Users who have ever touched an incident tool have this baked in: **lower number = more
  urgent**.
- `pkg/classifier.RiskLevel` (`pkg/classifier/classifier.go:16-24`) is an `iota`-ordered
  Go const — `RiskLow=0 < RiskMedium=1 < RiskHigh=2 < RiskCritical=3` — where **higher
  number/later-alphabetically-sounding word = more severe**. This is the opposite
  numeric direction from P0/P1/P2 (if you naively mapped `RiskCritical→P0`,
  `RiskHigh→P1`, `RiskMedium→P2`, you get a *3-level collapse that drops RiskLow
  entirely* and a numbering that only works if nobody looks at the underlying enum
  ordinal).

Requirements.md's recommendation (open question, already leaning this way) is to **keep
the existing 4-level `RiskLevel` internally and choose display labels in planning,
rather than remapping to P0/P1/P2**. UX research supports this strongly, for a reason
requirements.md doesn't fully spell out: **`ApprovalRulesPanel.tsx` already threads
`riskLevel` as a string through rule create/edit (`riskLevel: rule.riskLevel` at line 253)
and `ClassificationAnalytics` already stores it by the same name.** Introducing a
*second* vocabulary (P0/P1/P2) for the same underlying value in a sibling UI (the review
queue) sitting one click away from the rules UI would force users to mentally re-map
`RiskCritical` in one screen to "P0" in another — the exact "two priority notions"
confusion requirements.md flags as a *non-goal* for `queue.Priority` vs. approval risk,
except here it'd be the *same* concept wearing two labels.

**Recommendation**: label the badge with the plain `RiskLevel` word (**Low / Medium /
High / Critical**), not P0/P1/P2. This:
1. Matches the existing (if currently invisible) `riskLevel` vocabulary already in
   `ApprovalRulesPanel`/`SuggestedRuleCard`/backend — one label, one place.
2. Avoids inventing a numbering whose direction (P0=worst) inverts the enum's ordinal
   direction (Critical=highest number) — a classic source of off-by-one/reversed-sort
   bugs if a future contributor assumes `Priority`-style "lower number sorts first."
3. Sidesteps the lossy 4→3 collapse the issue's P0/P1/P2 language would force.

If stakeholders still want the P0/P1/P2 *framing* for scanning speed, treat it as a
**secondary abbreviation inside the badge** (e.g. "Critical · P0"), not a replacement —
same pattern Dependabot uses pairing a colour pill with a text word rather than a bare
code.

## 3. Accessibility — colour is not enough

Colour-only severity coding fails WCAG 1.4.1 (Use of Color) for the ~8% of men with
red-green colour vision deficiency, who cannot reliably distinguish a "red = critical"
vs. "green = low" badge from colour alone, especially at the small badge sizes used
throughout this UI (12px in `ApprovalAnalyticsPanel.css.ts`'s `gapBadgeHigh` etc.).

**This repo already has the correct pattern in two places — reuse it, don't invent a
third:**
- `ReviewQueueBadge.tsx` pairs an emoji (`🔴`/`🟡`/`🔵`/`⚪`) *and* a text abbreviation
  (`URG`/`HIGH`/`MED`/`LOW`) with the colour class, with `aria-label` spelling out the
  full word (`Urgent priority: ...`) — colour is decorative, not load-bearing.
- `ReviewQueuePanel.tsx`'s `ESCALATION_REASON_EMOJI` map (line ~141) has an explicit code
  comment: `// Category -> emoji prefix for the escalation reason line (WCAG 1.4.1 — not
  color-only).` This is a named, already-established convention in this exact file.

**Recommendation for the severity badge**: icon + text label + colour, same three-signal
structure as `ReviewQueueBadge`. Suggested icon set (distinct shapes, not just
colour-differentiated dots, so it also works for users with low vision / small screens):
`Critical` = 🔴 (or ⛔), `High` = 🟠, `Medium` = 🟡, `Low` = ⚪/🔵. Always render the word
(`Critical`/`High`/`Medium`/`Low`), never the icon alone, matching `getPriorityAbbr`'s
compact-mode fallback (icon + 3-4 char abbreviation) for space-constrained contexts like
the `compact={true}` badge already used in `itemHeader`.

Colour tokens: per `.claude/rules/css-architecture.md`, do not hardcode hex — reference
`vars.color.xxx` (new `.css.ts`) or existing CSS-module tokens (`--error`, `--error-bg`,
`--warning`, `--warning-bg`, `--success`, `--success-bg` from `globals.css`). This repo's
existing severity-shaped precedent (`gapBadgeHigh`/`gapBadgeMed`/`gapBadgeLow` in
`ApprovalAnalyticsPanel.css.ts`) currently reuses only `warning`/`success` tokens for a
2.5-level scale — a true 4-level `RiskLevel` badge needs a 4th tier. `--error`/`--error-bg`
(already defined per `css-architecture.md`'s token list) is the natural `RiskCritical`
tier; confirm/add a 4th named tier rather than overloading `--warning` for both High and
Medium (which would itself become a colour-only ambiguity within the badge's own tier
set).

**Automated a11y coverage**: `tests/e2e/accessibility.spec.ts` already runs Axe Core in
CI (per project CLAUDE.md's "UX analysis CI" section, blocking on WCAG AA violations) —
a colour-only badge risks failing that gate automatically if Axe's `color-contrast` or a
custom color-only check is configured; more importantly, follow the existing
icon+text+colour convention so it never gets to that check in the first place.

## 4. Error/edge-case UX

**No computed severity (legacy/orphaned/bypassed item)**: requirements.md's acceptance
criterion #6 covers persistence surviving restart, but acceptance criterion doesn't
explicitly cover the case where `RiskLevel` is genuinely absent (e.g., an approval
persisted *before* this feature ships, or a future code path that constructs a
`PendingApproval` without going through the classifier). Precedent already exists in
`ReviewQueuePanel.tsx` for exactly this "field predates this feature" case:

```tsx
{queueItem.metadata["escalation_reason"]
  ? `${emoji} ${queueItem.metadata["escalation_reason"]}`.trim()
  : "Reason not recorded — this request predates escalation-reason tracking."}
```

**Recommendation**: mirror this exact pattern for severity — render a neutral "Severity
not recorded" badge/state (distinct grey, no icon implying risk level, since implying
"Low" would be actively misleading for an item that simply predates classification —
under-communicating risk is worse than over-communicating it) rather than defaulting
silently to `RiskLow` or omitting the badge entirely. Silently defaulting to Low risks
an unclassified-but-actually-dangerous item sorting to the bottom of a severity-first
queue — the opposite of this feature's goal. Since sort is severity-first, an
"unknown severity" item should sort **as if High/Critical** (fail-safe: surface it near
the top for a human to triage) rather than falling to the bottom with Low — the queue's
job is "don't let something dangerous hide," and an unlabeled item is exactly the thing
that historically got missed (this is literally the problem statement in
requirements.md's "Problem" section, one level up).

**Empty state for "no items at this severity" when filtered**: `ReviewQueuePanel.tsx`
already has this exact empty state implemented for the priority/reason filters (lines
1221-1235):

```tsx
hasActiveFilter ? (
  <div className={emptyClass}>
    <p>No items match the current filter.</p>
    <p className={emptySubtext}>{totalItems} {totalItems === 1 ? "item" : "items"} in queue</p>
    <Button onClick={clearAllFilters}>Clear filter</Button>
  </div>
)
```

**Recommendation**: the severity filter should compose into this same `hasActiveFilter`/
`allFilteredItems` pipeline (join `priorityFilter`-style `Set<RiskLevel>` state, same
toggle/URL-persistence pattern as `priorityFilter`/`reasonFilter`) rather than a
parallel empty-state branch — reuses the existing "No items match... N items in queue...
Clear filter" copy and behavior for free, and keeps severity filtering consistent with
every other filter dimension already in this panel (program/category/tag/PR/diverged all
follow the identical Set-based toggle pattern at lines 232-241).

## 5. Job-to-be-done

**Functional job**: "Let me find the one dangerous request in a queue of forty routine
ones without opening each item." Per requirements.md's problem statement, the failure
mode today is FIFO position — a `rm -rf` sits in the same visual weight as a test-file
edit, forcing a human to open every card to discover which one matters. Severity-first
sort + a scannable badge directly serves "triage in under 10 seconds," the same
functional job PagerDuty/Sentry/Dependabot all optimize for: **glance, not read**. The
badge needs to be legible at list-scan speed (icon shape + colour recognizable before
the text is even read), with the text label as the fallback/confirmation for anyone who
can't rely on colour or shape alone (section 3).

**Emotional job**: reducing the anxiety of "did I just rubber-stamp something dangerous
because it looked like all the other forty items?" — the cost of a missed `RiskCritical`
item isn't just delay, it's an actual security/safety incident (force-push, `rm -rf`).
Severity sort converts an undifferentiated backlog into an implicitly triaged one, so the
human's default action ("work top-to-bottom") is *already* the safe action, rather than
requiring them to remember to specifically hunt for risk. This is the same "don't make
the safe path require extra vigilance" principle behind the existing
`ESCALATION_REASON_EMOJI` / countdown-urgency colour scheme in `ApprovalCard.tsx`
(`countdownUrgent` at ≤10s remaining) — the UI should make the risky thing visually loud
without the user having to go looking for it.

**Analytics breakdown job**: distinct from the live queue's "triage now" job,
`GetApprovalAnalyticsResponse`'s severity breakdown (in scope, acceptance criterion #5)
serves a *retrospective* job — "is our rule coverage keeping up, or are we manually
reviewing a rising share of high-risk requests?" This maps directly onto the existing
`ApprovalAnalyticsPanel.tsx` summary-card pattern (`cardAllow`/`cardDeny`/`cardManual` at
lines 16, 124-128) and the `gapBadgeHigh/Med/Low` visual vocabulary already in that file
— the severity breakdown should render as a peer of those existing cards/bars (reusing
the `Bar`/`StackedBar` helper components at lines 60-83), not a new visualization
paradigm, and should sit near the existing "Escalation Reasons" section since both answer
"why is manual review happening and how urgent is it," using the same
label-map-with-fallback idiom as `ESCALATION_CATEGORY_LABELS` (lines 98-105) for the
four `RiskLevel` values.

## Summary of concrete recommendations

1. **Label as `RiskLevel` words (Low/Medium/High/Critical)**, not P0/P1/P2 — avoids a
   lossy 4→3 collapse and a direction-inverted numbering, and stays consistent with the
   already-wired-but-unrendered `riskLevel` field in `ApprovalRulesPanel.tsx`.
2. **Badge = icon + text + colour**, following `ReviewQueueBadge.tsx`'s existing
   pattern exactly (compact and full variants), not a bare colour dot/pill.
3. **Default sort: severity primary, then existing FIFO/age as tiebreaker** — matches
   Dependabot/CodeQL convention and requirements.md's own default recommendation; wire
   through the same `sortField`/`SORT_FIELDS` mechanism already in `ReviewQueuePanel.tsx`
   (add `"severity"` alongside `"priority"`/`"age"`/`"diffSize"`/`"name"`) but make it
   the *default* selected state rather than requiring the user to pick it from the
   dropdown, since acceptance criterion #3 requires severity-first by default.
4. **Severity filter**: reuse the existing `Set<T>`-based multi-select filter pattern
   (`priorityFilter`/`reasonFilter`) with per-level counts in the filter button labels,
   composed into the existing `hasActiveFilter`/empty-state pipeline — no new empty-state
   UI needed.
5. **Missing/unclassified severity**: render a distinct "Severity not recorded" state
   (mirroring the existing escalation-reason fallback string) and sort it as high-priority
   (fail-safe near the top), never silently default to Low.
6. **Analytics breakdown**: extend `ApprovalAnalyticsPanel.tsx` with a severity
   breakdown card/table reusing the existing `Bar`/summary-card components, positioned
   near the Escalation Reasons section.
