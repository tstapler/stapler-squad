# UX Design: backlog-stage-execution-costs

SDD Phase 3. Builds on `research/ux.md` (Phase 2) — that doc's comparable-pattern
research, IA decision (extend `PipelineModeForm`, not a new page), and
accessibility-gap findings are not re-derived here. This doc grounds those
recommendations in the concrete field/component names from
`implementation/plan.md` so implementers build exactly this, not a
reinterpretation.

## Surface inventory

| # | Surface | Type | Files |
|---|---|---|---|
| A | Stage executor table (program+model per stage) | Interactive form | `web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx` |
| B | Stage cost bar chart | Interactive chart | `web-app/src/app/insights/StageCostChart.tsx` |
| C | Bar-click cross-filter into `SessionsTable` | Interactive drill-down | `StageCostChart.tsx`, `SessionsTable.tsx`, `InsightsDashboard.tsx` |
| D | Soft budget warning (global + per-item) | Interactive form + passive banner | `ProjectedCostCard.tsx` (existing, unchanged), `ItemBudgetWarning.tsx` (new), `BacklogItemDetail.tsx` |
| E | Validation/error states (bad program/model, Aider+headless) | Error states, embedded in A | `PipelineModeForm.tsx`, `session/pipeline_mode_validation.go` |
| F | Non-interactive: `[BudgetWarning]` structured log line | Log output | `session/budget_warning.go` |
| G | Non-interactive: Aider added to program-detection candidates | Config/CLI-adjacent | `config/config.go` |
| H | Resolved-executor provenance fields (fallback badge + drift indicator) | Read-only, rendered per-session row | `proto/session/v1/backlog.proto`, `server/services/backlog_service_pipeline_mode.go`, `web-app/src/lib/backlog/pipelineModeDisplay.ts`, `web-app/src/components/backlog/detail/SessionsSection.tsx` |
| I | Per-item cost-by-stage table loading/error states | Non-interactive states, embedded in Story 5.2.3 | `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/app/insights/ItemStageCostTable.tsx` |

Surfaces A and E are designed together (E is A's error states). B and C are
designed together (C is B's interaction). F, G get condensed entries per the
task brief. H was originally condensed but is expanded below (repair pass —
see its own section) since it needed a concrete rendering location, not just
a field list. I is a repair-pass addition alongside Story 5.2.3.

---

## Surface A + E: Stage executor table in `PipelineModeForm.tsx`

### Wireframe

Inserted as a new labeled subsection, positioned **above** the existing
`templateFieldsGrid` (9 content textareas) — distinct concern (execution
config vs. prompt content), per `research/ux.md` §2.

```
┌─ Pipeline Mode: cheap-triage ────────────────────────────────────┐
│ Slug: [cheap-triage        ] (immutable after creation)          │
│ Name: [Cheap Triage        ]                                     │
│ Description: [...]                                               │
│ [x] Enabled                                                      │
│                                                                    │
│ ── Stage execution ──────────────────────────────────────────────│
│ Different models per stage means no prompt cache carries over    │
│ between stages.                                                  │
│                                                                    │
│ ┌────────────┬──────────────────────┬───────────────────────┐   │
│ │ Stage      │ Program              │ Model                 │   │
│ ├────────────┼──────────────────────┼───────────────────────┤   │
│ │ Triage     │ [System default  ▾]  │ [claude-haiku-4-5   ] │   │
│ │ Review     │ [System default  ▾]  │ [System default     ] │   │
│ │ Work       │ [aider           ▾]  │ [System default     ] │   │
│ └────────────┴──────────────────────┴───────────────────────┘   │
│                                                                    │
│ ── Prompts & commands (9 fields, unchanged) ─────────────────────│
│ [Triage prompt] [Review prompt] [Work prompt] ...                │
│                                                                    │
│ [ Save ]  [ Delete ]                                              │
└────────────────────────────────────────────────────────────────┘
```

Markup: a real `<table>` with `<th scope="col">Stage</th>`/`Program`/`Model`
(per `research/ux.md` §3 — not CSS-grid `div`s, which lose the
header/cell association for AT). Each `<td>` input carries its own
`aria-label` (e.g. `"Triage stage model"`) so a screen reader announces
"Triage stage model, edit text" even where the visual column header alone
wouldn't associate reliably in every AT. Program/Model inputs use the
existing `PROGRAMS`/`MODEL_AUTOCOMPLETE_OPTIONS` datalist-autocomplete
pattern from `WorkflowForm.tsx` (typeahead, not a rigid `<select>` — model
IDs churn faster than a hardcoded enum should).

### Interaction flow

1. User opens `/settings/pipeline-modes`, selects or creates a mode →
   `PipelineModeForm` renders with `StageExecutorValues` initialized from
   `mode?.stageExecutors ?? {}` (all three rows blank/"System default" for a
   new mode).
2. User types into a stage's Model field. Autocomplete suggests matches from
   `MODEL_AUTOCOMPLETE_OPTIONS` as they type (same behavior as
   `WorkflowForm.tsx`'s model field) but accepts free text — an unlisted but
   valid model ID (e.g. a new release not yet in the static list) is not
   blocked client-side; validation happens server-side on submit (see E
   below).
3. An empty field always displays placeholder text `"System default"`
   (mirrors `PROGRAMS`'s own `{ value: "", label: "System default" }`
   convention) — never a bare empty box that could read as "no model
   configured" vs. "inherits default" ambiguously.
4. User clicks **Save**. Client sends `stage_executors: {triage: {program,
   model}, review: {...}, work: {...}}` alongside the existing 9 fields via
   `createPipelineMode`/`updatePipelineMode`. Empty program/model submit as
   `""` (not an absent key) — the domain layer treats `""` as "inherit"
   uniformly.
5. On success: form closes/returns to the mode list, matching existing save
   behavior for the 9-field submit today (no new success affordance needed
   — consistent with how a content-field save already behaves).
6. On failure (see E): the existing `styles.errorMessage` /
   `data-testid="pipeline-mode-error"` / `role="alert"` banner at the top of
   the form (already rendered for other validation failures) shows the
   server's message. Focus does not move away from the offending row — the
   user can see both the error banner and the field that caused it without
   scrolling.

### Error / edge-case states (Surface E — ADR-002, save-time validation)

| Trigger | System response | User sees | Exit path |
|---|---|---|---|
| Triage or Review row's Program = `"aider"` | `CreatePipelineMode`/`UpdatePipelineMode` returns `CodeInvalidArgument` naming the role and program; no row persisted | Error banner: *"Aider can't run the triage stage headlessly (no machine-readable cost output). Choose Claude or Gemini for Triage, or leave it blank to use the default."* (message must name **"aider"** and the role, per plan Task 1.3.1b's acceptance criteria) | Clear the Program field (revert to "System default") and re-save — no page reload, no lost edits to other fields |
| Work row's Program = `"aider"` | Accepted — work stage has no headless allow-list | Saves normally, no error | n/a (success path) |
| Review row's Model = an unrecognized raw string (e.g. `"claude-opus-9000"`) not a `family:` alias | `CodeInvalidArgument` naming the unrecognized model | Error banner: *"'claude-opus-9000' isn't a recognized model for Review. Use a known model ID or a family alias like family:opus."* | Correct the Model field, re-save, **or** check the "I know this model isn't in the pricing table yet — save anyway" override (see below) and resubmit |
| Same unrecognized-model error, with the override checkbox checked (`force_unknown_model: true`, Task 5.1.1h) | Accepted — the pricing-table cross-check is bypassed for this one save | Saves normally; error banner and override control disappear on success. The stage-executor table row keeps a small persistent marker (e.g. an inline icon + "Unverified pricing" label next to the Model field, at reduced visual weight vs. the error banner it replaces) — not just a one-time checkbox — so a user who checked it once and moved on can still see, later, which stage is running on a model outside the pricing table (mirrors the codebase's existing `pricingUnavailable` "abstain rather than guess" convention). `aria-label` on the marker: `"Triage stage model — unverified pricing, cost may not display"` (per stage) | n/a (success path) — a second parallel exit path alongside "correct the field," not a replacement for it. New models routinely ship before Epic 3.2's pricing table is updated; a hard, un-overridable rejection would block a legitimate save |
| Aider not installed on this machine, selected for Work | No client or server detection exists (`PROGRAMS` is a static list, confirmed in `research/ux.md` §4) — accepted at save time | No warning at save time. Failure surfaces later, at session-spawn time, as whatever error `SwitchProgram`/session launch already produces today | Documented, accepted gap (see "Known limitations" below) — **not** built in this project |
| Both Program and Model left blank for a stage | Always valid — means "inherit pool/session default" | No error; row displays "System default" in both columns | n/a (success path) |

**No dead ends**: every error state above leaves the form open, with all
other field values intact, and a single corrective action (fix one field,
re-save) — never a full-form reset or navigation away.

**Known limitation, explicitly flagged, not silently deferred**: program
"not installed" detection has no hook in this codebase today (confirmed in
Phase 2 research). This project inherits that gap unchanged for the new
per-stage fields — a user can configure an uninstalled program for any
stage and won't find out until it fails to launch. Filed as a candidate
follow-up (`research/ux.md` §4), not a blocker for this project's scope.

---

## Surface B + C: `StageCostChart.tsx` and cross-filter drill-down

### Wireframe

Rendered as a sibling card to `ModelBreakdownChart`, in the same
`InsightsDashboard.tsx` grid section (per `research/ux.md`'s "one chart per
dimension" convention — not a generic pivot control):

```
┌─ Insights ──────────────────────────────────────────────────────┐
│ [Daily Spend]        [Model Breakdown]      [Cost by Stage]      │
│                                              ┌──────────────────┐│
│                                              │ Cost by Stage     ││
│                                              │  ▓▓▓▓▓▓▓▓ $14.30  ││
│                                              │  work             ││
│                                              │  ▓▓ $1.20         ││
│                                              │  review           ││
│                                              │  ▓ $0.50          ││
│                                              │  triage           ││
│                                              │                    ││
│                                              │ ● work  ● review   ││
│                                              │ ● triage           ││
│                                              └──────────────────┘│
│                                                                    │
│ [ Sessions table — searchText: "" | roleFilter: none ]           │
│  Session   Role     Item              Cost                        │
│  sess-9    work     Fix login bug     $2.10                       │
│  sess-8    triage   Fix login bug     $0.02                       │
│  ...                                                              │
└────────────────────────────────────────────────────────────────┘
```

Bars sorted descending by cost (work, review, triage in the example above),
matching `ModelBreakdownChart`'s existing sort convention. A role with zero
sessions in the selected range is **omitted entirely** — never a
zero-height bar (which could misread as "$0 incurred" rather than "no
data"). If all three roles are empty, the card shows the same `"No data"` /
`emptyChart` state `ModelBreakdownChart.tsx` already uses.

### Interaction flow

1. User loads `/insights`. `StageCostChart` renders from
   `GetInsightsSummaryResponse.role_breakdown`.
2. User clicks the "work" bar (or, via keyboard, tabs to the "work" legend
   entry and presses Enter/Space — see Accessibility below).
3. `StageCostChart` calls `onRoleClick("work")`. `InsightsDashboard.tsx`
   (the newly-controlling parent) sets a `roleFilter` prop passed down to
   `SessionsTable`, which applies it as an additional array filter *before*
   its existing Fuse.js text search runs (role name is not fuzzy-matched as
   free text — a separate filter dimension, per plan Task 5.2.2c).
4. `SessionsTable` re-renders showing only `sessionRole === "work"` rows —
   no page navigation, no modal; the table the user is already looking at
   just narrows, matching the AWS Cost Explorer precedent
   `research/ux.md` §1 cites.
5. To drill further into one backlog item's cost, the user uses
   `SessionsTable`'s **existing** search box (unchanged) to filter by item
   title — this project does not add a second, separate item-name filter
   UI; it reuses the table's current search, per requirement.
6. To clear the role filter: clicking the same bar again, or clearing
   `SessionsTable`'s search/filter controls, returns to the unfiltered
   view. (Implementation note for Phase 5: the bar should visually indicate
   "active" filter state — e.g. an outline or the legend dot getting a
   ring — so a user who clicked "work" and then looks away doesn't forget
   the table is filtered. This is a small addition beyond the plan's
   explicit tasks; flagging it here since its absence would violate the
   "no confusing hidden state" spirit of the acceptance criteria below.)
7. The "Filtered to: work ×" chip (Task 5.2.2e) carries `aria-live="polite"`
   on its containing element (a visually-hidden sibling status region if the
   chip itself unmounts/remounts in a way that would suppress the
   announcement) so a screen-reader user is told the result set narrowed —
   e.g. "Filtered to work, showing 4 sessions" — the moment a bar/legend
   click applies the filter, not only sighted users watching the table
   shrink (Nielsen #1, visibility of system status, for non-visual users).
   Clearing the filter announces the reverse: "Filter cleared, showing all
   sessions."

### Error / edge-case states

| Trigger | System response | User sees |
|---|---|---|
| A role has zero sessions in the selected date range | Bar omitted from chart | Chart shows only roles with data; no "$0" bar |
| All roles empty | Whole-chart empty state | `chartCard` with `"No data"` text, matching `ModelBreakdownChart` |
| A session's cost is unpriced (non-Claude program, no pricing table entry) | **Gap identified during this design pass**: `RoleCostBreakdown`/`ItemRoleCost` (plan.md Epic 4.1) carry no `pricingUnavailable`/unpriced signal, unlike `ModelBreakdownChart`'s per-family `pricingUnavailable` field. As specified, an unpriced session's cost is indistinguishable from a genuinely-\$0 session in this chart. | **Recommendation, not yet in plan.md**: either (a) accept this as scoped-out for v1 and note it in the chart's own doc comment, or (b) add a `bool has_unpriced_sessions` to `RoleCostBreakdown` (mirrors the existing `unpriced`/`pricingUnavailable` "abstain rather than guess" convention the codebase already uses in `ModelBreakdownChart`/`SessionsTable`) so the stage's legend entry can carry the same `" (pricing unavailable)"` annotation. Flagging this now so Phase 4/5 implementers make a deliberate choice rather than an accidental silent gap — this is exactly the kind of inconsistency the requirements doc's "trust the drilldown numbers" success metric is meant to prevent. |
| Bar/legend click with no matching sessions (e.g. filtered by date range so "work" bar shows cost but underlying sessions expired from the table's own range) | `SessionsTable` shows its own existing empty-result state | No crash; standard "no results" table state (already exists) |

### Accessibility (Story 5.4.1)

- Chart wrapping `div` (`chartWrap`): `role="img"`, `aria-label` summarizing
  all visible data points, e.g. `"Cost by stage: work $14.30, review $1.20,
  triage $0.50"` — generated from the same sorted data the bars render, so
  it never drifts out of sync with what's visually shown. When a role has
  `unpriced_session_count > 0`, its portion of the label folds that count in
  too, e.g. `"...review $1.20 (1 unpriced), triage $0.50"` — the visible
  unpriced badge (Task 5.2.1, see "Shared emphasis convention" below) is a
  sighted-only affordance unless its count is also in the accessible name,
  which is exactly what AC13 requires the label to match.
- Each legend entry: `tabIndex={0}`, `role="button"`, `onKeyDown` handling
  Enter/Space to fire the identical `onRoleClick(role)` handler a mouse
  click does — mirrors `SessionsTable.tsx`'s existing clickable-row pattern
  (`SessionsTable.tsx:233-238`), not a new interaction paradigm.
- `SessionsTable`'s existing `SessionRole` column remains the
  non-chart-dependent path to the same breakdown — a screen-reader user is
  never required to interact with the chart to get the underlying data,
  per `research/ux.md` §3's "convenience, not the only path" framing.
- Color contrast: reuse the existing `ModelBreakdownChart` `PALETTE` and
  legend-dot pattern, already presumed to meet 4.5:1 (unchanged from a
  shipped, presumably-audited component) — no new color decisions
  introduced by this feature.

### Shared emphasis convention (unpriced badge, rework indicator, fallback badge, executor-drift indicator)

**Repair-pass correction:** the previous revision of this section claimed the
icon "may be the same asterisk/dagger glyph already used elsewhere in
Insights for 'pricing unavailable'-style annotations." Verified against the
real codebase (`ModelBreakdownChart.css.ts:55-58`'s `unpricedLabel` —
`{ color: warningText, fontStyle: "italic" }`, no icon; `ProjectedCostCard.tsx:44-45`'s
`warningText` span — plain colored text, no icon) that **no such glyph
exists anywhere in Insights**. There is a real icon-prefix convention in this
codebase, but it's one file over and for a different purpose:
`SessionsSection.tsx`'s `SYNTHETIC_KIND_ICON` map (`SessionsSection.tsx:60-64`)
prefixes a diagnostic row's title with an `aria-hidden` emoji
(`🩺`/`🚫`/`✍️`, falling back to `🔍`). This section now specifies **new**
icons rather than claiming reuse of a convention that doesn't exist for
warning pills specifically.

Four components in this project need a distinguishable, non-color-only
emphasis treatment: `StageCostChart`'s unpriced badge (Task 5.2.1),
`ItemStageCostTable`'s `session_count` rework indicator (Task 5.2.3d), and
Surface H's new fallback badge and executor-drift indicator (Story 5.2.4,
Tasks 5.2.4c/d). The first two don't crowd the same row as each other or as
Surface H's badges, so they can safely share one glyph; Surface H's two new
badges *do* crowd the same row as each other and as the pre-existing
content-drift badge, so those three need to be mutually distinguishable, not
just distinguishable from plain text. Defined once here, so every
implementer builds the identical triplet instead of independent guesses:

- **Bold text + a small icon + an explicit text label — never color alone**
  (WCAG 1.4.1, use of color).
- **Unpriced badge / rework indicator** (don't co-occur in the same row):
  share a generic warning glyph, `⚠` — e.g. a triage row with 3 runs renders
  **⚠ 3 runs** (bold, icon, text), not a red/orange-tinted "3" with no other
  distinguishing mark.
- **Fallback badge / executor-drift indicator / content-drift badge** (all
  three *can* appear in the same `pipelineGroup` row, per Surface H below —
  see BLOCKER 1 in the prior review round): each gets its **own** icon and
  label text, not a shared glyph:
  - Fallback badge: icon `↩`, label "Ran on different program".
  - Executor-drift indicator: icon `⚙`, label "(executor config since
    changed)".
  - Pre-existing content-drift badge (`pipelineDriftBadge`, unchanged by
    this project): no icon, label "(content since changed)" — its
    distinctiveness from the two new badges comes from them each having an
    icon it lacks, not from retrofitting one onto it (out of scope here).
- The accessible name always states the fact in words, e.g.
  `aria-label="Triage: 3 runs, rework indicator"`,
  `aria-label="Review: 1 unpriced session"`,
  `aria-label="Fell back to Claude: Gemini unavailable"`, or
  `aria-label="Executor config changed since this session ran"` — never
  conveyed by a tooltip alone, which is invisible to a screen-reader user
  who isn't hovering.
- `StageCostChart`'s unpriced badge additionally folds its count into the
  chart's own `role="img"` `aria-label` string (see Surface B+C
  Accessibility above) — the badge and the chart-level label must agree.

---

## Surface D: Soft budget warning

Two distinct, clearly-labeled surfaces — not one mechanism awkwardly
serving two granularities:

### D1 — Global monthly budget (existing, unchanged)

`ProjectedCostCard.tsx` already ships this: a `budgetInput`
(`aria-label="Monthly budget threshold in USD"`), an `isWarning` boolean
(`projectedMonthly > threshold`), and inline `warningText` ("Over budget!").
This project does not modify it — included here only so the two surfaces
are visibly distinguished for the reader of this doc and, more importantly,
for the end user: **D1 answers "is my total monthly spend over budget,"
D2 (below) answers "is this specific item over budget."**

### D2 — Per-item budget (`ItemBudgetWarning.tsx`, new)

#### Wireframe

Wired into `BacklogItemDetail.tsx`'s existing `bannerBar` pattern (the same
slot used for other item-level advisory banners, per
`BacklogItemDetail.tsx:1365,1376`):

```
┌─ Fix login bug ──────────────────────────────────── [ × ] ──────┐
│ Priority: High   Created: 2026-09-10                             │
│ ┌────────────────────────────────────────────────────────────┐  │
│ │ ⚠ Over budget: $5.05 spent, threshold $5.00                 │  │  ← bannerBar, only when over
│ └────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  Budget threshold: [$5.00        ]   (edit form field)           │
│                                                                    │
│  [Description] [Triage result] [Sessions] ...                    │
└────────────────────────────────────────────────────────────────┘
```

When no threshold is configured (`costBudgetThresholdUsd` is `undefined`),
`ItemBudgetWarning` renders `null` — no empty banner, no placeholder text,
nothing. The threshold input itself (in the edit form) is always visible so
a user can opt in.

#### Interaction flow

1. User opens a backlog item's detail view. If `costBudgetThresholdUsd` is
   unset, no warning banner appears anywhere on the page (component returns
   `null`) — the edit form still shows an empty threshold input, inviting
   opt-in without nagging.
2. User sets a threshold (e.g. `$5.00`) via the edit form and saves →
   `UpdateBacklogItem(cost_budget_threshold_usd: 5.00)`.
3. As sessions run against this item and their costs accumulate,
   `ItemBudgetWarning` compares the item's current total cost against the
   threshold on every render (derived from data already fetched for the
   detail view — no new polling mechanism).
4. The moment total cost ≥ threshold, the banner appears — same visual
   language (`warningText`/`isWarning` styling) as `ProjectedCostCard`, so
   a user who's already learned what an "over budget" look means from the
   Insights page recognizes it instantly here too.
5. This is advisory only — nothing is blocked. The item continues through
   its pipeline exactly as before; the user's only action is to notice and,
   if they choose, intervene manually (e.g. pause the item, lower future
   stage costs via Surface A).
6. Server-side, the moment a headless call's `CostSink` fires and pushes an
   item over threshold, a `[BudgetWarning]` log line fires immediately
   (Surface F) — the UI banner and the log are two views of the same
   `EvaluateBudgetThreshold` evaluation, not two independently-computed
   numbers that could disagree.

#### Error / edge-case states

| Trigger | System response | User sees | Exit path |
|---|---|---|---|
| No threshold configured | `ItemBudgetWarning` renders `null` | Nothing (no empty box) | n/a |
| Threshold set, cost below it | No warning | Nothing | n/a |
| Threshold set, cost at/above it | Warning banner shown | `bannerBar` with warning styling, exact amounts | Dismissible only by lowering spend or raising/clearing the threshold — never a "dismiss" button that would hide a still-true fact (matches the non-blocking-but-persistent spirit of `ProjectedCostCard`, which has no dismiss either) |
| Threshold cleared back to blank | Warning disappears immediately (component returns to `null` render) | Nothing | n/a |

---

## UX Acceptance Criteria

**Task completion efficiency**
1. A user can configure a per-stage program and model override for all
   three stages (triage, review, work) of a pipeline mode in a single form
   submission — 0 additional page navigations beyond opening the existing
   `/settings/pipeline-modes` edit form.
2. A user can view the stage/role cost breakdown in ≤ 1 page load
   (`/insights`, no extra click to reveal the chart — it renders alongside
   `ModelBreakdownChart` on initial page load).
3. A user can drill from "which stage costs the most" to "which sessions
   for that stage" in exactly 1 click (bar click) or 1 keyboard activation
   (Tab to legend entry + Enter/Space) — no intermediate modal or page.
4. A user can opt into a per-item budget warning in ≤ 1 form field edit
   (set the threshold input) with no separate "enable budget tracking"
   toggle required.

**Error and edge-case handling**
5. Configuring `"aider"` for the Triage or Review stage's Program field
   shows an error message that names both the rejected program ("aider")
   and the affected stage, and offers a specific corrective action (clear
   the field or choose Claude/Gemini) — never a generic "invalid request."
6. Configuring `"aider"` for the Work stage's Program field succeeds with
   no error (work stage has no headless-only restriction).
7. An unrecognized model string is rejected with a message naming the
   exact rejected value and the affected stage.
8. Every error state identified in this document has a visible exit path
   requiring no page reload and no loss of other in-progress form edits —
   no dead ends.
9. A stage/role with zero sessions in the selected Insights time range is
   omitted from the chart, never shown as a zero-height or "$0" bar that
   could be misread as confirmed zero spend.
10. An item with no configured budget threshold shows no warning UI
    anywhere (not an empty/disabled warning box) — the feature is
    invisible until opted into.

**Accessibility**
11. Every stage-executor table input has a unique, descriptive `aria-label`
    or an associated `<label>`, distinguishable by stage and field type
    (e.g. "Triage stage model" vs. "Review stage model").
12. The stage-executor table uses semantic `<table>`/`<th scope="col">`
    markup, not CSS-grid `div`s styled to look tabular.
13. The new `StageCostChart` wrapping element has `role="img"` and an
    `aria-label` whose content matches the visually-rendered bar values,
    including each role's unpriced-session count when present (verified by
    a test asserting the label string against the same sorted data array
    the bars use).
14. Every clickable chart legend entry is reachable via Tab and activatable
    via both Enter and Space, firing the identical handler a mouse click
    would.
15. `SessionsTable`'s existing role/search-based breakdown remains fully
    usable without ever interacting with the chart — the chart is a
    convenience path, not the only path, to the same information.
16. All new/changed text (error messages, banner text, legend labels,
    "System default" placeholders) meets ≥ 4.5:1 contrast against its
    background, using the same design tokens already governing
    `ProjectedCostCard`/`ModelBreakdownChart`/`PipelineModeForm` (no new
    colors introduced by this feature).

**Consistency**
17. The per-stage cost chart's bar values sum to exactly the existing
    global total cost shown elsewhere on the Insights page for the same
    time range (matches the plan's `sum(role_breakdown[].estimated_cost_usd)
    == total_cost_usd` invariant) — verified visually by summing the bar
    labels against the total-cost card.
18. The global monthly budget warning (`ProjectedCostCard`) and the
    per-item budget warning (`ItemBudgetWarning`) use visually consistent
    warning styling (color, iconography, tone of message) so a user
    recognizes both as "the same kind of signal," while remaining clearly
    labeled as answering different questions (total spend vs. this item's
    spend).

**Repair-pass additions**
19. A session whose configured stage program fell back to Claude at call
    time shows a fallback badge naming both the configured program and the
    fallback reason, visible on the same per-session row as the existing
    pipeline-mode display — never only discoverable via server logs or a
    direct API query (Surface H, plan.md Story 5.2.4).
20. A session whose `executor_snapshot_hash` no longer matches its pipeline
    mode's current per-role executor config shows a drift indicator that is
    **visually distinguishable** from the pre-existing pipeline-mode
    content-drift indicator (own icon and own label text — "(executor config
    since changed)", never the content-drift badge's "(content since
    changed)" string verbatim) while still using the same warning color
    family for family resemblance — see "Shared emphasis convention" above
    (Surface H).
21. The "Filtered to: <role> ×" chip (Task 5.2.2e) announces its state
    change via `aria-live="polite"` so a screen-reader user learns the
    session table narrowed (or the filter cleared) without needing to
    re-scan the table.
22. A stage saved via the `force_unknown_model` override carries a
    persistent, non-tooltip-only marker in the stage-executor table (not
    just a one-time checkbox at save time) so a user can later recognize
    which stage is running on an unverified/unpriced model (Surface E).
23. The unpriced-session badge (`StageCostChart`), the rework-count
    indicator (`ItemStageCostTable`), the fallback badge (Surface H), and
    the executor-drift indicator (Surface H) all use the bold+icon+text
    emphasis triplet defined once in "Shared emphasis convention" above —
    none signals its state by color alone (WCAG 1.4.1), and the two Surface H
    badges plus the pre-existing content-drift badge use mutually distinct
    icons/label text from each other since all three can appear in the same
    row (see AC24).
24. A session with both a non-empty `executor_fallback_reason` and a
    mismatched `executor_snapshot_hash` shows **both** the fallback badge and
    the executor-drift indicator simultaneously — neither is suppressed by
    the other's presence (Surface H, plan.md Story 5.2.4's `resolveExecutorProvenance`).
25. Below the narrow-viewport breakpoint, a session row's pipeline/provenance
    badge group (pipeline-mode display, content-drift badge, fallback badge,
    executor-drift indicator) wraps onto its own line rather than truncating
    or forcing horizontal scroll, per this repo's standing mobile+desktop
    parity requirement (Surface H).

---

## Condensed non-interactive surfaces

### F — `[BudgetWarning]` structured log line

```
[BudgetWarning] item=bl_abc123 stage=triage threshold=5.00 spent=5.05
```

- Fires synchronously at the moment `EvaluateBudgetThreshold` observes a
  crossing, inside the existing `CostSink` callback — not batched, not
  delayed to a periodic scan.
- Names the item ID, stage, configured threshold, and actual cumulative
  spend — sufficient to reconstruct the warning from logs alone with no UI.
- Fires at most once per crossing event (does not repeat on every
  subsequent session once already over) — implementation detail for Phase
  5/backend, called out here because a UX reviewer should confirm log
  volume doesn't become noise for an "always watching" operator.
- Mirrors the existing `[PipelineEngine] unresolved pipeline_mode=...`
  Warn-log convention already in `session/pipeline_engine.go` — same
  bracket-prefix, same structured-field style.

### G — Aider added to `config.GetAvailablePrograms()` candidates

- `candidates := []string{"proxy-claude", "claude", "claude-code", "gemini",
  "agy", "aider"}` — one line added to `config/config.go`.
- Acceptance: with Aider on `$PATH`, `GetAvailablePrograms()` includes
  `"aider"`; without it, the candidate is silently omitted (existing
  behavior for every other candidate).
- This makes Aider visible in the Work-stage Program autocomplete
  (Surface A) when installed — no new UI, just a longer candidate list
  feeding the existing dropdown.
- Acceptance criteria: (1) test asserts `"aider"` is checked when present
  on `$PATH`; (2) test skips gracefully in CI environments without Aider
  installed, matching every other candidate's existing test pattern; (3)
  no regression to the other 5 candidates' detection.

### H — Resolved-executor provenance fields (`resolved_program`,
`resolved_model`, `executor_snapshot_hash`, `configured_program`,
`executor_fallback_reason` on `ItemSession`)

**Repair-pass correction:** the original wording below ("no new UI location
introduced... provenance data riding along on an existing surface") turned
out to describe an assumption, not a built rendering — verified against
`BacklogItemDetail.tsx`/`SessionsSection.tsx`: the existing
`pipeline_mode_snapshot` plumbing (via `resolvePipelineModeDisplay`) is
consumed only to compute a drift name/state, never to render raw field data
generically, and there is no generic "session detail" panel that picks up
new proto fields for free. Plan.md's own stated intent for these fields
("a persisted, UI-visible fallback marker... never only a log line") was
therefore unfulfilled as originally scoped. Fixed by plan.md's new
**Story 5.2.4**, which gives this a concrete, named rendering:

**Repair-pass correction (round 2):** the previous revision here said the
drift indicator should reuse "the identical badge/tooltip styling" as the
content-drift badge and that the fallback badge should reuse "D1/D2's
existing warning visual language" verbatim. Both instructions, taken
literally, would make three semantically different facts (content drifted /
executor config drifted / program fell back at runtime) render as
pixel-identical warning-colored text pills once they can legitimately
co-occur in the same row (verified: `SessionsSection.tsx`'s `pipelineGroup`
already stacks badges above/below the pipeline line — branch/cost/ended
badges at `SessionsSection.tsx:322-339`, the pipeline+drift line at
`:445-462`). Fixed below: each of the three gets its own icon/label
("Distinguishable badge treatments," in the Surface B+C section above),
consistent in *color family* (all warning-toned) but not in *icon or text*.

- `configured_program`/`executor_fallback_reason` (fields 24-25): rendered
  as an inline fallback badge in `SessionsSection.tsx`'s per-session row —
  label "Ran on different program", e.g. accessible name `"Fell back to
  Claude: Gemini unavailable"` — next to the existing `pipelineDisplay`
  badge, visible only when `executor_fallback_reason` is non-empty. Uses its
  own `executorFallbackBadge` style (new, in `BacklogItemDetail.css.ts`) —
  same warning color family as the rest of this surface for family
  resemblance (AC18), but its own icon (`↩`) and label text so it's never
  confused with the drift indicators below.
- `executor_snapshot_hash` (field 23): compared against a new, **dense**
  `PipelineMode.stage_executor_hashes` map — an entry for all 3 stage roles
  on every mode, including unconfigured ones (computed as
  `ComputeExecutorHash("", "")` for a role with no override, matching what
  an unconfigured session's own hash already computes) — not just roles with
  a configured override. A sparse map would compare a "no entry" miss
  against a real per-session hash and flag every ordinary default-executor
  session as falsely drifted; see plan.md Story 5.2.4's Engineering-lens
  design note for the full trace. On mismatch, rendered as its own
  `executorDriftBadge` — icon `⚙`, label "(executor config since changed)" —
  deliberately **not** the identical styling/text as the pre-existing
  content-drift badge (`pipelineDriftBadge`, "(content since changed)"),
  since that string is specifically about the prompt/content templates and
  would misdescribe an executor/program/model change. This both-warning-
  family-but-distinct-icon approach satisfies "Consistency and standards"
  (§4, family resemblance) without sacrificing "Recognition rather than
  recall" (§1, telling the three facts apart at a glance) the way exact reuse
  would have.
- `resolved_program`/`resolved_model` (fields 21-22, pre-existing scope, not
  part of this repair): continue to display as concrete values (not the raw
  `family:sonnet`-style alias or an empty string) wherever a session's "what
  ran" data is shown — this part of the original design was already sound.
- **Fallback and drift are independent facts and can both be true for the
  same session** (fallback is a spawn-time runtime substitution; drift is a
  config-changed-since-spawn fact) — `resolveExecutorProvenance` (Task
  5.2.4b) reports both simultaneously rather than picking one via
  if/else-if priority, and both badges render together when both are true.
  See plan.md Story 5.2.4's both-true acceptance criterion and Given-When-Then.
- **What the user should do upon seeing either badge** (mirroring Surface
  D2's "advisory only" statement): nothing is required. Both badges are
  purely informational — the session already ran; the user's only action is
  to notice and, if they choose, investigate manually (e.g. check whether
  the configured program is still installed for the fallback badge, or
  re-verify the stage's current executor config for the drift indicator).
  Neither badge blocks anything or represents an error state needing
  immediate response.
- **Narrow-viewport treatment:** below the breakpoint `BacklogItemDetail.css.ts`'s
  existing responsive rules use (confirmed at implementation time), the
  `pipelineGroup` badge row wraps onto its own line instead of truncating or
  scrolling horizontally — a session row can now carry up to 3 inline badges
  (pipeline-mode display/content-drift, fallback, executor-drift) plus the
  existing branch/cost/ended badges above it, per this repo's standing
  mobile+desktop parity requirement.
- Sample: `ItemSession{..., configured_program: "gemini", resolved_program:
  "", resolved_model: "claude-haiku-4-5", executor_fallback_reason:
  "gemini_unavailable", executor_snapshot_hash: "a1b2c3d4e5f6..."}`.
- Acceptance: (1) a session spawned with a per-stage override shows the
  concrete resolved program/model, not the raw `family:sonnet`-style alias
  or an empty string; (2) a session that fell back from its configured
  program shows the fallback badge naming both programs; (3) a session
  whose stage executor config has since changed on the mode shows the
  executor-drift indicator, with wording/icon distinct from the content-drift
  badge; (4) a session with both facts true shows both badges; (5) editing
  the pipeline mode after the session ran does not change what that
  historical session displays (hashes are computed at spawn time, not read
  live, except for the mode-side comparison value which is always freshly
  computed and now dense across all 3 roles); (6) no separate reconciliation
  UI needed beyond the badges above.

**Interaction flow** (added — the prior revision described only *what*
renders, not the user's path through it, unlike Surfaces A-D):
1. User opens a backlog item's detail view and scrolls to its Sessions list
   (`SessionsSection.tsx`) — no separate navigation; these badges live on a
   page the user already visits to check session status/cost.
2. For each session row, the user sees the existing pipeline-mode name/badge
   plus, when applicable, the new fallback badge and/or executor-drift
   indicator inline.
3. If a badge is present, the user reads its label/hover state; no click
   target exists on any of the three badges (they are display-only, per the
   "what the user should do" note above) — the user's next action, if any,
   happens elsewhere (e.g. editing the pipeline mode's stage executors on
   `/settings/pipeline-modes`, Surface A).

---

### I — Per-item cost-by-stage table (`ItemStageCostTable` on `BacklogItemDetail.tsx`, Story 5.2.3)

**Repair-pass addition.** This is `BacklogItemDetail.tsx`'s first
Insights-summary data fetch (verified: no existing `GetInsightsSummary`/
`useInsights`/`insightsSummary` reference anywhere in that file) — unlike
Surface B/C, which rides on `/insights`'s pre-existing page-level fetch, so
its loading/error handling can't be assumed to already exist. Three states,
mirroring patterns already used elsewhere on this page rather than
inventing new ones:

| State | Trigger | User sees |
|---|---|---|
| Loading | Fetch in flight, item detail already mounted | The existing shared `Skeleton` component (`@/components/ui/Skeleton`, already used by `InsightsDashboardSkeleton.tsx` for exactly this kind of async-fetch placeholder — e.g. `<Skeleton variant="text" width="100%" height={32} />` per row, mirroring `InsightsDashboardSkeleton.tsx`'s own list-of-rows section), sized to the cost-table's row height, not a layout jump when data arrives and not the "no cost data" empty state |
| Error | Fetch rejects | A small inline error state distinct from "no data" — e.g. "Couldn't load cost data" — never silently rendering nothing, which would be indistinguishable from the legitimate "this item has no sessions yet" empty state below and violates the same "abstain rather than guess" principle the codebase already applies to unpriced costs |
| Empty (legitimate) | Fetch succeeds, no matching `role_breakdown[].items` entries for this item | Table omitted entirely (unchanged from Story 5.2.3's original AC) |

A failed fetch must never be visually identical to "this item genuinely has
no sessions yet" — a silent failure reading as a false-negative "nothing to
see here" is exactly what the requirements' "trust the drilldown numbers"
success metric exists to prevent.

---

## Summary of findings for implementers

- **Resolved, both now in plan.md**: the two gaps originally noted here —
  `RoleCostBreakdown`/`ItemRoleCost` having no `pricingUnavailable`/unpriced
  signal, and the clicked-bar "active filter" affordance being absent from
  the task list — are both now concretely specified: the unpriced indicator
  in Story 5.2.1's AC and Task 5.2.1d, the active-filter chip in Task
  5.2.2e (plus this doc's own aria-live addition, AC21).
- Surfaces D1 (global) and D2 (per-item) budget warnings are intentionally
  kept as two separate components per the architecture research cited in
  the plan — this design confirms they should nonetheless share visual
  language so a user recognizes both as budget warnings, per Acceptance
  Criterion 18.
- **Repair-pass findings (round 1)**: `configured_program`/
  `executor_fallback_reason`/`executor_snapshot_hash` had zero Phase 5
  renderer despite being built specifically for UI visibility — fixed via
  Surface H's rewrite and plan.md's new Story 5.2.4 (AC19-20). The
  `force_unknown_model` override path, a new async fetch on
  `BacklogItemDetail.tsx` (Surface I), and an under-specified shared
  emphasis convention were also fixed in place — see Surface E, Surface I,
  and "Shared emphasis convention" above, and AC19-23.
- **Repair-pass findings (round 2, this revision)**: round 1's fix
  over-corrected — reusing the content-drift badge's exact styling for the
  new executor-drift indicator, and `ItemBudgetWarning`'s exact styling for
  the new fallback badge, made three semantically different facts
  indistinguishable once they could co-occur in one row, and
  `resolveExecutorProvenance`'s if/else-if priority silently dropped the
  drift fact whenever a fallback was also present. Both fixed: each of the
  three badges now has its own icon/label text (see "Shared emphasis
  convention" and Surface H above; AC20, AC23-24), and
  `resolveExecutorProvenance` returns both facts independently instead of
  picking one (AC24). Also folded in this round: a mobile/narrow-viewport
  wrap treatment for the now-more-crowded row (AC25), a concrete loading
  primitive for Surface I (the existing `Skeleton` component, not "confirm
  at implementation time"), and an explicit "what should the user do" note
  for Surface H mirroring D2's advisory-only framing.
