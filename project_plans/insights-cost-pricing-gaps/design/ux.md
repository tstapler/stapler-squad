# UX Design: Insights Cost Pricing Gaps

Builds on `research/ux.md` (do not re-derive the pattern survey, mental-model, or
accessibility reasoning there — this doc is the concrete before/after spec for the three
surfaces the plan (`implementation/plan.md`) actually touches: Epic 2.1
`ModelBreakdownChart.tsx`, Epic 2.2 `SummaryCards.tsx`, Epic 2.3 `SessionsTable.tsx`).

## Scope confirmation

**Addendum (plan repair, 2026-07-27): this section is stale regarding `ModelOverTimeChart.tsx`.**
The claim below that it is "not touched by the plan" was accurate against the plan as
originally scoped, but Epic 2.4 (added in a later plan-repair pass, per
`adversarial-review.md`'s BLOCKER finding that three cost-rendering surfaces were missed)
now touches `ModelOverTimeChart.tsx` directly, plus `DailySpendChart.tsx` and
`ProjectedCostCard.tsx`. `TopNTables.tsx` remains genuinely out of scope — it has no cost
column at all (confirmed in `implementation/plan.md`'s Epic 2.3 scope note). See the new
"Epic 2.4 surfaces" section below for the before/after treatment of the three surfaces
Epic 2.4 adds.

Per the plan (as of this repair pass), six user-facing surfaces change in total. No new
component, no new page, no modal/dialog, no CTA on any of them.

| # | Surface | File | Plan reference |
|---|---|---|---|
| 1 | Model breakdown chart legend | `web-app/src/app/insights/ModelBreakdownChart.tsx` | Epic 2.1 |
| 2 | Total Cost summary tile | `web-app/src/app/insights/SummaryCards.tsx` | Epic 2.2 |
| 3 | Sessions table cost column | `web-app/src/app/insights/SessionsTable.tsx` | Epic 2.3 |
| 4 | Daily spend chart footnote | `web-app/src/app/insights/DailySpendChart.tsx` | Epic 2.4 |
| 5 | Model-over-time chart legend | `web-app/src/app/insights/ModelOverTimeChart.tsx` | Epic 2.4 |
| 6 | Projected cost card caveat | `web-app/src/app/insights/ProjectedCostCard.tsx` | Epic 2.4 |

---

## Surface 1 — `ModelBreakdownChart.tsx` legend (primary fix for the reported bug)

### Before

```
┌ Cost by Model Family ─────────────────────────────────────┐
│  $0.045 ┤                    ┌───┐                         │
│  $0.030 ┤          ┌───┐     │   │                         │
│  $0.015 ┤  ┌───┐    │   │     │   │                         │
│  $0.000 ┼──┴───┴────┴───┴─────┴───┴──   (claude-sonnet-5    │
│           sonnet-4  opus-4    haiku-4    bar is 0px tall,   │
│                                           invisible here)    │
│                                                              │
│  ● claude-sonnet-4   ● claude-opus-4   ● claude-haiku-4     │
│  ● claude-sonnet-5                                          │
└──────────────────────────────────────────────────────────────┘
```
The `claude-sonnet-5` legend dot is present (legend always renders, per
`research/ux.md` §2) but reads identically to a real `$0` — nothing distinguishes
"unpriced" from "genuinely free this period."

### After

```
┌ Cost by Model Family ─────────────────────────────────────┐
│  $0.045 ┤                    ┌───┐                         │
│  $0.030 ┤          ┌───┐     │   │                         │
│  $0.015 ┤  ┌───┐    │   │     │   │                         │
│  $0.000 ┼──┴───┴────┴───┴─────┴───┴──                       │
│           sonnet-4  opus-4    haiku-4                       │
│                                                              │
│  ● claude-sonnet-4   ● claude-opus-4   ● claude-haiku-4     │
│  ● claude-sonnet-5 (pricing unavailable)                    │
└──────────────────────────────────────────────────────────────┘
```

Diff (legend row only, per Task 2.1.1b):
```diff
- ● claude-sonnet-5
+ ● claude-sonnet-5 (pricing unavailable)
```

- The suffix is **plain text**, not an icon-only or color-only signal (satisfies
  `research/ux.md` §4's "don't rely on color alone" — a colorblind or non-sighted-context
  user still gets the full meaning from the text itself).
- No change to bar rendering (0-height bar stays 0-height — the plan explicitly treats a
  forced minimum bar height as an optional nicety, not required; this design doesn't
  add it, since the legend fix alone resolves the ambiguity per the accessibility
  reasoning in `research/ux.md` §4).
- Styling: `unpricedLabel` reuses the existing `vars.color.warningText` token (same token
  `SessionsTable.css.ts`'s `orphanBadge` already uses) — no new color introduced,
  consistent with `css-architecture.md`'s token-only rule.

### Interaction flow

Read-only. No click target, no hover-required disclosure (the text is always visible,
not gated behind a tooltip), no dismiss/acknowledge affordance. Confirmed by
`research/ux.md` §3: "No dismiss/acknowledge/snooze affordance is needed... a factual
annotation that should simply disappear on its own once the pricing table is updated."

### Edge case — every model in the period is unpriced

```
┌ Cost by Model Family ─────────────────────────────────────┐
│  $0.000 ┤                                                   │
│  $0.000 ┼──────────────────────────────────                │
│           sonnet-5   opus-6                                 │
│           (all bars 0-height, indistinguishable from        │
│            "no data" by bar shape alone)                    │
│                                                              │
│  ● claude-sonnet-5 (pricing unavailable)                    │
│  ● claude-opus-6 (pricing unavailable)                      │
└──────────────────────────────────────────────────────────────┘
```
No special-case handling needed or added: `toDataPoints()` still produces one entry per
`ModelBreakdown`, so the legend renders one badged row per family regardless of how many
(or what fraction) are unpriced — the existing `models.length === 0` branch (chart
already has an explicit "No data" empty state for the *zero-models* case, distinct from
*all-models-unpriced*) is untouched and doesn't need to change. The chart still "renders
sensibly" in the all-unpriced case: every bar is flush at $0 (visually flat, not broken),
and every legend row is badged, so nothing reads as silently free — the same flat
baseline a real all-zero-cost period would show, but every row is now labeled.

---

## Surface 2 — `SummaryCards.tsx` Total Cost tile

### Before

```
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│ Total Cost     │  │ Input Tokens  │  │ Output Tokens │
│ $0.045         │  │ 1.2M          │  │ 340K          │
│ 12 sessions    │  │ total input   │  │ total output  │
└───────────────┘  └───────────────┘  └───────────────┘
```
No indication that `$0.045` excludes any unpriced usage — reads as the complete total
even when it isn't.

### After (1+ models unpriced)

```
┌───────────────────────┐
│ Total Cost             │
│ $0.045                 │
│ 12 sessions             │
│ excludes 1 unpriced model│
└───────────────────────┘
```

Diff (Total Cost tile body only, per Task 2.2.1a):
```diff
  <span className={cardLabel}>Total Cost</span>
  <span className={cardValue}>{fmtCost(summary.totalCostUsd)}</span>
  <span className={cardSub}>{sessionCount} session{sessionCount !== 1 ? "s" : ""}</span>
+ {summary.unpricedModels.length > 0 && (
+   <span className={cardSub}>
+     excludes {summary.unpricedModels.length} unpriced model{summary.unpricedModels.length !== 1 ? "s" : ""}
+   </span>
+ )}
```

### After (0 models unpriced — unchanged)

```
┌───────────────┐
│ Total Cost     │
│ $0.045         │
│ 12 sessions    │
└───────────────┘
```
No footnote line at all — the tile is byte-identical to today's rendering when
`summary.unpricedModels.length === 0`. This is a deliberate "absence is the default"
design: the footnote only exists to explain an incompleteness that's actually present.

### Interaction flow

Read-only caption, reuses the existing `cardSub` styling and position (stacks below the
existing session-count `cardSub` line, doesn't replace it — both are visible
simultaneously when applicable). No click target.

### Edge case — every model unpriced (Total Cost is $0 and 100% incomplete)

```
┌───────────────────────┐
│ Total Cost             │
│ $0.000                 │
│ 12 sessions             │
│ excludes 2 unpriced models│
└───────────────────────┘
```
Same rendering path as the 1+ case — `summary.unpricedModels.length` (2, plural "models")
just happens to equal the count of all models present that period. No separate "100%
unpriced" wording is needed; "excludes N unpriced models" already communicates the
severity proportionally once the reader also sees `$0.000`, and this repo's UX research
(`research/ux.md` §1, pattern 4) explicitly rules out escalating to a page-level banner
for what's still a per-model-count annotation, even at N=100%.

---

## Surface 3 — `SessionsTable.tsx` cost column

### Before

```
┌──────────┬───────────┬──────┬───────┬────────┬───────┬─────────┐
│ Session  │ Model     │ Path │ Input │ Output │ Cache │ Cost    │
├──────────┼───────────┼──────┼───────┼────────┼───────┼─────────┤
│ a1b2c3   │ sonnet-5  │ proj │ 500K  │ 200K   │ 40%   │ $0.0000 │
│ d4e5f6   │ sonnet-4  │ proj │ 300K  │ 100K   │ 55%   │ $0.0021 │
└──────────┴───────────┴──────┴───────┴────────┴───────┴─────────┘
```
Row 1's `$0.0000` is visually and semantically indistinguishable from a session that
really did cost nothing (e.g. a trivial one-turn session on a priced model).

### After

```
┌──────────┬───────────┬──────┬───────┬────────┬───────┬────────────────────┐
│ Session  │ Model     │ Path │ Input │ Output │ Cache │ Cost               │
├──────────┼───────────┼──────┼───────┼────────┼───────┼────────────────────┤
│ a1b2c3   │ sonnet-5  │ proj │ 500K  │ 200K   │ 40%   │ $0.0000 [unpriced] │
│ d4e5f6   │ sonnet-4  │ proj │ 300K  │ 100K   │ 55%   │ $0.0021            │
└──────────┴───────────┴──────┴───────┴────────┴───────┴────────────────────┘
```

Diff (Cost cell only, per Task 2.3.1b):
```diff
- <td className={tdRight}>{fmtCost(s.estimatedCostUsd)}</td>
+ <td className={tdRight}>
+   {fmtCost(s.estimatedCostUsd)}
+   {s.unpricedModels.length > 0 && <span className={unpricedBadge}>unpriced</span>}
+ </td>
```

`unpricedBadge` visually mirrors the existing `orphanBadge` pattern already used one
column over in this same table (`SessionsTable.tsx:137`, "orphan" badge on the Session
cell) — same badge shape/token, different text, so the row-level visual language for
"this row has an asterisk" is already familiar to the one user of this dashboard before
this feature ships.

### Interaction flow

Read-only badge inline in the existing cell — no separate row, no new column, no
click/hover requirement to reveal it. Clicking the row (where `onSessionClick` is wired)
behaves exactly as before; the badge does not intercept the click (it's plain inline
content, not a button).

### Edge case — a session used multiple models, only some unpriced

`SessionTokenSummary.unpricedModels` is a list (can be `["claude-opus-6"]` while the
session's `primaryModel` column still shows e.g. `claude-sonnet-4`, since primaryModel
is whichever model had the most turns). The badge condition is `unpricedModels.length > 0`
— i.e. it fires whenever *any* portion of that session's cost is unpriced, even if the
displayed `primaryModel` column looks priced. This is intentional: the Cost cell's number
is an aggregate across all models used in the session, so the badge belongs to the
aggregate, not to whichever model happens to be displayed in the adjacent Model column.
No design change needed for this case beyond what Task 2.3.1b already specifies — flagged
here only so a reviewer doesn't mistake it for a bug ("the Model column says sonnet-4 but
it's badged unpriced").

### Edge case — filtering/search with unpriced sessions

The `modelFilter` dropdown (`uniqueModels`, built from `s.primaryModel`) is unaffected —
it still lists primary models exactly as today, unpriced or not. No new filter ("show
only unpriced sessions") is introduced; not requested by requirements.md and would be
scope creep beyond what G2/AC-3 ask for (consistent with `research/ux.md`'s explicit
recommendation to skip building any new interactive affordance around this signal).

---

## Epic 2.4 surfaces — addendum (plan repair)

Added after the original three-surface scope above was set; follows the same treatment
(text-only footnote/legend-suffix additions, no new wireframe, no new interaction
pattern) as Surfaces 1–3. Full task-level detail is in `implementation/plan.md`'s
Epic 2.4 (Stories 2.4.1–2.4.3).

### `DailySpendChart.tsx` — unpriced-days footnote

**Before**: chart title with no annotation, even when one or more plotted days include
unpriced-model usage.

**After**: a footnote line directly under the chart title, shown only when at least one
day in the plotted range has non-empty `unpricedModels`: `"N day(s) include unpriced
model usage"` (correct singular/plural). No footnote when zero days are affected —
identical to today's rendering. Same styling family as Surface 1's `unpricedLabel`
(reuses `vars.color.warningText`, plain text, no icon/color-only signal).

### `ModelOverTimeChart.tsx` — legend annotation per unpriced family

**Before**: each per-family legend entry renders as the bare family name (e.g.
`claude-opus-6`), identical in appearance whether that family is priced or unpriced.

**After**: an unpriced family's legend entry gets the same `" (pricing unavailable)"`
text suffix Surface 1 (`ModelBreakdownChart`) already uses — e.g. `claude-opus-6
(pricing unavailable)` — so the two charts give a consistent signal for the same
underlying gap. A priced family's entry is unchanged.

### `ProjectedCostCard.tsx` — projection caveat

**Before**: the card shows a projected monthly total and a `"Based on N of M days"`
sub-line with no indication the projection may be undercounting.

**After**: when any day in the current-month projection window had unpriced usage, a
caveat line appears directly below the existing `"Based on N of M days"` sub-line:
`"Projection excludes unpriced usage"`. No caveat when no day in the window is affected.
This is the highest-stakes of the three additions — the projection feeds
`ProjectedCostCard`'s over-budget warning, so silently excluding unpriced usage doesn't
just look wrong, it under-warns the user about real spend (see `implementation/plan.md`
Story 2.4.3 for the full reasoning).

All three follow the existing accessibility posture from the original three surfaces:
plain text (no color-only or icon-only signal), no `role="alert"` (informational, not
urgent), no dismiss/acknowledge affordance, no new CTA.

---

## Cross-surface consistency check

| Surface | Trigger condition | Wording | Style source |
|---|---|---|---|
| Chart legend | `ModelBreakdown.pricingUnavailable === true` (per family) | `<family> (pricing unavailable)` | new `unpricedLabel`, reuses `vars.color.warningText` |
| Summary tile | `GetInsightsSummaryResponse.unpricedModels.length > 0` (aggregate) | `excludes N unpriced model(s)` | existing `cardSub` |
| Sessions table | `SessionTokenSummary.unpricedModels.length > 0` (per session) | `unpriced` badge | new `unpricedBadge`, mirrors existing `orphanBadge` |

Wording is deliberately not identical word-for-word across the three surfaces (chart says
"pricing unavailable," table says "unpriced," summary says "unpriced model(s)") — each
is scoped to its own grain (per-family / per-session / aggregate) and matches the
existing verbosity budget of its surrounding UI (a legend row has room for a full phrase;
a compact badge next to a dollar figure doesn't). All three consistently use the word
"unpriced"/"pricing," never a bare symbol or color swatch alone, satisfying the
proportionate accessibility bar from `research/ux.md` §4 without introducing three
different vocabularies for the same concept.

---

## UX Acceptance Criteria

Each is human-testable by loading the Insights page with fixture data containing at
least one unpriced model family (e.g. via a manual test server per this repo's
"Manual/interactive testing" convention in `CLAUDE.md`, pointed at data containing a
`claude-opus-6`-style unknown family) and visually inspecting the three surfaces.

| ID | Criterion | Surface |
|---|---|---|
| UX-AC-1 | Every model-family legend entry for an unpriced model visibly shows the text `(pricing unavailable)` next to the family name — not just a distinct dot color, pattern, or icon with no accompanying text. | Chart legend |
| UX-AC-2 | A priced model's legend entry is rendered exactly as it is today — no suffix, no visual change — when `pricingUnavailable` is false or absent. | Chart legend |
| UX-AC-3 | The bar for an unpriced model does not need to be visible/tall for the unpriced state to be understood — a user can identify every unpriced family from the legend alone, with the chart area covered/scrolled out of view. | Chart legend |
| UX-AC-4 | When all model families in the period are unpriced, the chart still renders (axes, bars at $0, full legend) rather than falling back to an unrelated "No data" empty state — the all-unpriced case is visually distinct from the zero-sessions case. | Chart |
| UX-AC-5 | The Total Cost tile shows a footnote reading `excludes N unpriced model(s)` (correct singular/plural) when `unpricedModels.length >= 1`. | Summary tile |
| UX-AC-6 | The Total Cost tile shows no footnote at all — output identical to pre-feature rendering — when `unpricedModels.length === 0`. | Summary tile |
| UX-AC-7 | The footnote text is legible as a factual statement, not styled or worded as an error/alert (no red/error coloring, no `role="alert"`, no exclamation-heavy copy) — matching this being informational, not urgent, per `research/ux.md` §4. | Summary tile |
| UX-AC-8 | A session row whose cost includes any unpriced-model usage shows an inline `unpriced` badge next to its Cost cell value, visually consistent with (but distinguishable in text from) the existing `orphan` badge pattern already used in this table. | Sessions table |
| UX-AC-9 | A session row with no unpriced-model usage shows only the plain cost figure — no badge — identical to current behavior. | Sessions table |
| UX-AC-10 | No dead ends: none of the three indicators require or offer a click/dismiss/acknowledge/snooze action. They are purely informational and disappear on their own (next page load, once the underlying pricing table is updated) with zero required user interaction. | All three |
| UX-AC-11 | No indicator relies on color alone to convey meaning — every one of the three has accompanying text (`(pricing unavailable)`, `excludes N unpriced model(s)`, `unpriced`) that fully conveys the state without needing to perceive a specific color or icon shape. | All three |
| UX-AC-12 | Clicking a session row in `SessionsTable` that has an `unpriced` badge still triggers `onSessionClick` normally (the badge doesn't intercept or block the existing row-click behavior). | Sessions table |
| UX-AC-13 | No new CTA, link, tooltip-only disclosure, modal, or "configure pricing" affordance is introduced anywhere in the Insights UI as part of this feature (per `research/ux.md` §3's explicit finding that the override mechanism is an operator/config-file concern, not a UI flow). | All three (negative check) |

### Accessibility note (proportionate, not a full audit)

Per `research/ux.md` §4, this is a single-operator internal tool where full WCAG AA
rigor is not warranted, and this repo's e2e CI already runs Axe/Lighthouse against any
PR touching `web-app/src/` (catches contrast regressions automatically — see
`CLAUDE.md`'s E2E Tests section). The one criterion worth confirming by hand rather than
relying on CI is UX-AC-11 above (text-based, not color/icon-only) — that's the cheap,
high-value check this feature specifically needs given the root bug (a 0-height bar) is
itself a "you can't see the marker at all" usability defect independent of color vision.
No separate screen-reader walkthrough, keyboard-nav audit, or manual contrast-ratio
measurement is required for this feature.
