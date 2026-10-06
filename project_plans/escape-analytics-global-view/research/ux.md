# UX Research: escape-analytics-global-view

**Agent**: Research Agent 5 (UX)
**Date**: 2026-08-11

## Scope recap

Requirements (`project_plans/escape-analytics-global-view/requirements.md`) call for a
"Per-Session | All Sessions" tab/toggle on the existing `EscapeAnalyticsPage`
(`web-app/src/components/analytics/EscapeAnalyticsPage.tsx`). Selecting "All Sessions" hides
the session dropdown and event table and shows an aggregate histogram/mangle-rate summary plus
a per-session breakdown table so a user can spot the outlier session without clicking through
sessions one at a time.

---

## 1. Per-item vs. aggregate-across-all-items toggle — interaction pattern

**Recommendation: two-tab `role="tablist"`, not a dropdown or checkbox toggle.**

Comparable dev-tool/observability UIs (Grafana's per-host vs. fleet dashboards, Datadog's
host-map vs. single-host view, GitHub Actions' per-job vs. all-jobs summary) converge on the
same shape: a **small, fixed set of mutually exclusive, persistent views** rendered as tabs or
a segmented control sitting directly above the content they govern. Reasons this beats the
alternatives for this feature specifically:

- **Not a dropdown**: dropdowns are the right control when the option set is large, dynamic,
  or drawn from data (this repo's own session-selector `<select>` is exactly that case — it
  lists N sessions). "Per-Session" vs. "All Sessions" is a fixed 2-way mode switch, not a
  data-driven list, so a dropdown adds a click-to-open step for no benefit and buries the
  current mode inside a closed control instead of showing it at a glance.
- **Not a plain toggle/checkbox**: a checkbox reads as "on/off feature flag," not "which of
  two views am I looking at." Tabs communicate view identity (each has a label) whereas a
  toggle only communicates a boolean, which is a worse match for two named, differently-shaped
  views (one has a session dropdown + event table; the other has a per-session breakdown
  table instead).
- **Tabs vs. segmented control**: functionally these are the same widget with different visual
  chrome. This repo already has a canonical, accessible tab implementation to reuse directly:
  `web-app/src/components/sessions/SessionDetailView.tsx:571-612` (`role="tablist"` container
  with `ArrowLeft`/`ArrowRight` roving focus, `role="tab"` + `aria-selected` per button, active
  state driven by a single `activeTabId` string). Reusing this pattern over inventing a new
  segmented-control component keeps the codebase's tab affordance consistent and gets the
  keyboard/ARIA wiring for free — see §3.

**Recommended structure**: `viewMode: "per_session" | "all_sessions"` state (mirrors the
existing `selectedSessionId` state shape), two `role="tab"` buttons in a `role="tablist"`
positioned where `sessionSelectorRow` currently sits, immediately above the conditionally
rendered per-session or aggregate content block.

---

## 2. Mental model for spotting an "outlier session" in a per-session breakdown table

The job here is comparative scanning across rows, not reading any single row in isolation — so
the table should optimize for **relative** signal, not just absolute numbers.

- **Sortable by mangle rate (descending) by default.** The requirement's own success metric
  is "see each session's own totals side-by-side to spot outliers" — sorting worst-first means
  the outlier is the first row a user sees, with zero interaction required. This matches the
  standard observability pattern (e.g. "top offenders" tables in APM tools) where the sort
  order itself *is* the primary signal, not a nice-to-have.
- **Sortable columns generally** (session, total sequences, total mangled, mangle rate) via
  clickable `<th>` with `aria-sort`, so a user can re-rank by absolute volume when "who has
  the most mangled events" and "who has the worst rate" diverge (a session with 2 mangled out
  of 3 total has a 67% rate but is noise; a session with 500/10,000 has a lower rate but is the
  real systemic contributor). Exposing both dimensions side-by-side, sortable independently,
  lets the user reconcile "high rate" vs. "high volume" outliers rather than the UI silently
  picking one framing.
- **Visual highlighting above a threshold**, not just numeric sorting — a row whose mangle
  rate exceeds a fixed or dynamic threshold (e.g. >2x the aggregate mangle rate, or a flat
  threshold like >5%) should get a distinguishing treatment (background tint using
  `vars.color.errorBg`/`vars.color.warningBg` from the existing token set, or a small icon
  badge) so the outlier is visible even without explicit sorting — useful when a user opens
  the tab already sorted by session name/ID rather than rate. This mirrors
  `MangleRateIndicator`'s existing job in the per-session view (worth checking whether it
  already has rate-threshold color logic to reuse directly rather than re-deriving thresholds).
- **Anchor the comparison** by showing the aggregate mangle rate as a persistent reference
  value near/above the table (already in scope as the "aggregate histogram/mangle-rate
  summary") so relative highlighting/sorting has a baseline to compare against — "3.2% here vs.
  0.4% fleet-wide" is a stronger signal than "3.2%" alone.
- **Don't paginate for the MVP** (per the requirements doc's own open question) — a sortable,
  fully-visible table is what makes "scan for the outlier" fast; paginating breaks the ability
  to eyeball the whole distribution in one view. Revisit only if real session counts prove this
  wrong.

---

## 3. Accessibility requirements — tab control and data table

### Tab/toggle control (WCAG 2.1 / WAI-ARIA Tabs Pattern)

Reuse `SessionDetailView.tsx`'s existing implementation as the template — it already satisfies:

- `role="tablist"` on the container, `role="tab"` on each button, `aria-selected={boolean}`
  reflecting current mode.
- **Roving tabindex + arrow-key navigation**: `ArrowLeft`/`ArrowRight` move focus and activate
  (this repo's existing pattern activates on arrow-move rather than requiring a separate
  Enter/Space press — acceptable per the WAI-ARIA Tabs Pattern's "automatic activation" model,
  appropriate here since switching tabs is cheap, not a destructive/expensive action).
- Each tab should have `id="tab-<mode>"` and, if the tab panel is given `role="tabpanel"`,
  `aria-controls`/`aria-labelledby` should link tab ↔ panel (SessionDetailView's version above
  doesn't visibly wire `aria-controls` in the excerpt read — worth confirming/adding when
  implementing so screen readers announce the tab-panel relationship, not just tab state).
- Disabled-tab support already exists (`aria-disabled`) — not needed here since both modes are
  always available, but note it's there if a future disabled state (e.g. "All Sessions"
  disabled while aggregate RPC is in flight for the first time) is wanted.

### Data table (per-session breakdown)

- Use native `<table>` with `<thead>`/`<tbody>`, `<th scope="col">` per column — screen readers
  need real table semantics, not `<div>` grids with visual-only styling (this repo's
  `EscapeEventTable` component should be checked for its existing table markup pattern and
  matched, rather than introducing a second table convention).
- **Sortable column headers**: each sortable `<th>` should be a `<button>` (not a bare
  clickable `<div>`) with `aria-sort="ascending" | "descending" | "none"` on the `<th>` itself,
  updated as sort state changes — this is the standard accessible-sortable-table pattern and
  lets screen reader users know both that a column is sortable and its current direction.
- **Outlier highlighting must not be color-only** (WCAG 1.4.1 Use of Color) — pair any
  background-tint highlighting with a text/icon cue (e.g. a "⚠" glyph + `aria-label` or a
  visually-hidden "(above threshold)" span) so the signal survives for colorblind users and
  screen readers, not just sighted users scanning for tinted rows.
- **Keyboard access to sort controls** falls out for free from using real `<button>` elements
  inside `<th>` (native tab-order + Enter/Space activation) — no custom key handling needed
  here, unlike the tablist which needs arrow-key roving focus.
- Focus management on tab switch: when moving from "Per-Session" to "All Sessions", make sure
  focus lands somewhere sensible in the new panel (e.g. the tab button itself retains focus per
  the ARIA Tabs pattern — don't forcibly move focus into the table) so screen reader/keyboard
  users aren't disoriented by the content swap.

---

## 4. Error / empty states

Four distinct states need explicit handling, mirroring the existing per-session view's
`summaryError`/`eventsError` pattern (`role="alert"` banners using `styles.errorBanner`):

1. **Zero sessions have any escape events at all** (fleet-wide empty state). This is different
   from "no session selected" (today's `noSessionMessage` for the per-session tab) — the
   message should say something like "No escape sequence events recorded across any session
   yet" rather than reusing the per-session tab's "select a session above" copy, since there's
   no session-selection action available to take in this view. Render this in place of the
   histogram/table, not as an error — it's a legitimate, non-error data state.

2. **One session dominates ~99% of events.** Not an error state to block on, but a UX trap: if
   the per-session breakdown table's rows are sorted by absolute mangled count rather than
   rate, one dominant session's row will visually swamp the table while smaller sessions with
   a *worse rate* but low volume scroll below the fold. This is the direct argument for
   defaulting the sort to mangle rate (§2) rather than volume, and/or showing both columns
   sortable — the aggregate summary card (fleet-wide totals) should also make clear whether the
   fleet-wide rate is being driven by one outlier ("this aggregate is dominated by session X:
   99% of mangled events" as an inline note) so a user doesn't mistake one bad session's numbers
   for a systemic fleet issue. This is worth a lightweight annotation (e.g. "top contributor:
   <session>, N% of mangled events") directly in the aggregate summary card, not just left for
   the user to infer from the breakdown table.

3. **RPC failure** (`GetEscapeAnalyticsGlobalSummary` errors). Follow the existing
   `summaryError`/`eventsError` convention exactly: `role="alert"` banner with
   `styles.errorBanner`, message text `Failed to load global summary: {error.message}`, and
   critically — **don't silently fall back to stale per-session data**. Per the requirements
   doc's own rabbit hole about cleanly suspending per-session hooks when switching tabs, the
   aggregate RPC's own loading/error/data states must be entirely independent of the
   per-session hooks' states so a failure in one tab's data fetch never bleeds into or gets
   confused with the other tab's last-known state.

4. **(Implicit) time-range filter yields zero matching sessions/events.** Distinct from state 1
   (globally empty) — this is "empty *for this filter*," so the empty-state copy should
   reference the active time range ("No escape events in the selected time range") rather than
   implying there's never been any data, so a user knows to widen/clear the filter rather than
   assuming the feature is broken.

All four states should be visually distinguishable from each other (empty-but-valid vs.
actual RPC error) — reuse `errorBanner`/`role="alert"` only for state 3; states 1, 2's
soft-annotation, and 4 are informational, not alerts, and should use a neutral style
(e.g. extend `noSessionMessage`'s treatment) so `aria-live` regions aren't triggered for
non-error, non-urgent information.

---

## 5. Job-to-be-done

**Functional job**: "Tell me, in one glance, whether a mangle-rate spike I'm investigating is
isolated to the session I happened to be looking at, or is happening across the fleet" —
replacing a manual, memory-intensive process (open session A's summary, note the rate, close,
open session B, note the rate, repeat, try to remember/compare mentally) with a single view that
does the comparison for the user.

**Emotional job**: confidence and closure on scope, specifically resolving the anxiety of "is
this bug bigger than I think?" A developer debugging terminal rendering who finds a mangled
sequence wants to know quickly whether they're chasing (a) a one-off/session-specific glitch —
scope the fix narrowly, maybe even dismiss as session-specific noise — or (b) a systemic
regression in the escape-sequence parser itself — escalate urgency, look for a shared root
cause (parser version, terminal type, recent change) across all affected sessions. Today's
one-by-one click-through actively works against this job: by the time a user has checked 5+
sessions manually, working memory of exact rates degrades, and the "is this systemic" judgment
becomes unreliable exactly when it matters most (many sessions to compare = likely systemic
issue = the case where the answer matters more). The aggregate view directly fulfills this job
by making the fleet-wide rate and the full distribution of per-session rates simultaneously
visible, turning a multi-step recall task into a single perceptual comparison.

---

## Summary of concrete recommendations for Phase 3 planning

- Two-tab `role="tablist"` (reuse `SessionDetailView.tsx:571-612` pattern), state shape
  `viewMode: "per_session" | "all_sessions"`, positioned where the session-selector row is today.
- Per-session breakdown table: native `<table>`, sortable columns via `<button>`-in-`<th>` +
  `aria-sort`, default sort = mangle rate descending, non-color-only outlier highlighting
  (icon/text + tint) above a rate threshold, no pagination for MVP.
- Aggregate summary card should call out the top contributor session inline when one session
  dominates the aggregate, so the "systemic vs. isolated" question is answered explicitly, not
  left implicit in the breakdown table.
- Four distinct states to design: global-empty, filtered-empty, RPC-error (`role="alert"`,
  matching existing `errorBanner` convention), and a dominant-outlier annotation — keep the
  aggregate tab's loading/error state fully independent from the per-session tab's hooks.
