# UX Design: backlog-item-detail-ux

**Phase**: 3 (UX Design), pre-implementation
**Date**: 2026-07-21
**Inputs**: `project_plans/backlog-item-detail-ux/requirements.md`,
`project_plans/backlog-item-detail-ux/research/ux.md`,
`project_plans/backlog-item-detail-ux/implementation/plan.md`

This document is the wireframe/flow/acceptance-criteria layer between the research (`research/ux.md`)
and the implementation plan (`implementation/plan.md`). It uses the plan's exact component names —
`LifecycleSummary`, `StageTracker`, `BlockerChip`, `LivenessLine`, `CollapsibleGroup`,
`CollapsibleSection`, `useShowMore`, `SessionDiagnosticPanel`, `BlockedNotice` — so a reviewer can
trace every box in a wireframe to the component that renders it. Where a wireframe implies a
rendering rule not literally spelled out in plan.md, the rule is footnoted back to the exact
plan.md acceptance criterion it derives from.

Note on naming: the shared disclosure primitive is **two** components, not one —
`CollapsibleSection` wraps a single section's header/body; `CollapsibleGroup` wraps a set of
sibling `CollapsibleSection`s in one shared Radix `Accordion.Root` so Home/End/Arrow keyboard
navigation works across their headers (ADR-027, plan.md Task 1.1.1c). Any reference below to a
section being "a Collapsible" means it renders as a `CollapsibleSection`; where multiple sibling
sections are wired together (as they are in the main detail panel, per Task 3.1.4i), they share one
`CollapsibleGroup`.

---

## Surface Inventory

| # | Surface | Component(s) | States covered |
|---|---|---|---|
| 1 | Redesigned detail panel — desktop | `LifecycleSummary`, `CollapsibleSection`-wrapped sections sharing one `CollapsibleGroup`, `SessionsSection` | loading, loaded, load error, item-not-found |
| 2 | Redesigned detail panel — mobile | Same components, `detailPane`'s `@media (max-width: 768px)` overlay | loading, loaded, load error, item-not-found |
| 3 | `SessionDiagnosticPanel` — Headless Diagnostic sub-state | `TriageReviewPanel readOnly` / `GateVerdictBox readOnly` | has-output (2 variants: triage vs re-review) |
| 4 | `SessionDiagnosticPanel` — Blocked Guardrail sub-state | `BlockedNotice` | blocked-before-start, with/without recorded summary |
| 5 | `SessionDiagnosticPanel` — Manual Review Marker sub-state | `BlockedNotice` | manual override recorded |
| 6 | `SessionsSection` zero-sessions state | `SessionsSection` | empty, status-appropriate copy |
| 7 | Board card blocker chip | `BacklogItemCard`, `BlockerChip` (compact) | present, absent, stale-data banner |
| 8 | Board-wide stuck-data fetch failure | `useStuckBacklogItems()` consumers (`LifecycleSummary`, `BacklogItemCard`) | error, retained-stale-data |
| 9 | "Show N more" expansion (Sessions / Workflow History / Progress History) | `useShowMore`, `SessionsSection`, `WorkflowHistorySection`, `ProgressHistorySection` | capped (default), expanded, expanded-and-persisted-across-reopen |

---

## Surface 1 & 2: Redesigned `BacklogItemDetail` Panel

### 1a. Desktop wireframe (resizable aside, 240–800px, per `backlog.css.ts:206-221`)

```
┌──────────────────────────────────────────────────────────┐ ◄ resizable border-left drag handle
│ ← Back to list         Fix flaky WIP-cap test        [×]  │   (header, unchanged)
├──────────────────────────────────────────────────────────┤
│ data-testid="lifecycle-summary"   ▼ ALWAYS VISIBLE, never collapsed
│                                                            │
│  (Idea)──(Ready)──[●In Progress · Queued]──(Review)──(Done)│ ◄ StageTracker
│                     ▲ active node = item.status, ground truth
│                                                            │
│  🟠 Stuck 4h — Stale work session     Pipeline: Fast Track │ ◄ BlockerChip(full) · pipeline badge
│  (only rendered when useStuckBacklogItems() flags item)     (only when non-default pipeline)
│                                                            │
│  Last activity 2m ago                                     │ ◄ LivenessLine
├──────────────────────────────────────────────────────────┤
│ ▸ Description                                    [collapsed]
├──────────────────────────────────────────────────────────┤
│  ACTIONS                                    (always visible, not a CollapsibleSection)
│  [ Start Session ]  [ Archive ]  [ Edit ]                 │
├──────────────────────────────────────────────────────────┤
│ ▸ Plan Artifacts                                 [collapsed]
├──────────────────────────────────────────────────────────┤
│ ▾ Version Control                    [expanded — status ∈ {in_progress,review,pr_pending}]
│    branch: feat/wip-cap · commit abc1234                  │
│    [ View PR → ]  (single source: links to PullRequestSection, D4)
├──────────────────────────────────────────────────────────┤
│ ▾ Sessions (3)                                    [expanded — default true]
│    🖥  work · a1b2c3d4          started 2m ago      →      │ ◄ real session → <a href="/?session=...">
│    ▸ 🩺 headless-triage-7f2a…   3 suggestions              │ ◄ synthetic → CollapsibleSection → SessionDiagnosticPanel
│    ▸ 🚫 review-blocked-9c1d…   blocked pre-flight          │ ◄ synthetic → CollapsibleSection → SessionDiagnosticPanel
│    (only 3 sessions here — under the cap of 5, so no      │ ◄ see Surface 9 for the "Show N more" state
│     "Show N more" button; see Surface 9 for that case)    │
├──────────────────────────────────────────────────────────┤
│ ▸ Workflow History                               [collapsed]
├──────────────────────────────────────────────────────────┤
│ ▸ Progress History                               [collapsed]
├──────────────────────────────────────────────────────────┤
│ ▸ Notes                                          [collapsed — item.notes is empty]
└──────────────────────────────────────────────────────────┘
   ▲ panel root: height:100%; overflow-y:auto (Page Scroll Convention, Task 3.1.4h)
```

Notes on this wireframe:
- `ReviewingSection` and `PullRequestSection` are **not shown** here because `item.status ===
  "in_progress"` — they only render (guard preserved verbatim, Story 3.1.2) when status is
  `review` / `pr_pending` respectively. See the "status-gated sections" note in the interaction
  flow below.
- Every `▸`/`▾` glyph is decorative only — the real state is `aria-expanded` on the header
  `<button>`, per `CollapsibleSection`'s contract (Story 1.1.1). All sibling sections shown here
  (`Plan Artifacts` through `Notes`) share one `CollapsibleGroup`, so a keyboard user can move
  directly between their headers with Arrow/Home/End without tabbing through collapsed bodies —
  see Accessibility AC 25.
- The chevron + section title is a single full-width `<button>` (not a chevron-only click target),
  satisfying the ≥44×44px touch-target requirement even though this is the desktop layout — one
  component serves both breakpoints.

### 1b. Mobile wireframe (full-screen overlay, `position:fixed; inset:0`, `z-index:500`, per
`backlog.css.ts:206-224`)

```
┌────────────────────────────────┐  100vw × 100vh overlay over the list
│ [←]   Fix flaky WIP-cap test    │  sticky header, back tap target ≥44×44px
├────────────────────────────────┤
│ (Idea)→(Ready)→[●In Prog·Queued]│  ◄ StageTracker — wraps via flex-wrap,
│  →(Review)→(Done)               │    never truncated or scroll-clipped
│                                  │
│ 🟠 Stuck 4h — Stale work session │  ◄ BlockerChip(full) — stacks under tracker
│ Last activity 2m ago             │  ◄ LivenessLine
│ Pipeline: Fast Track             │  ◄ pipeline badge, own line on narrow width
├────────────────────────────────┤
│ ▸ Description                   │  full-width button, ≥44px tall
├────────────────────────────────┤
│ ACTIONS                         │
│ [ Start Session ]                │  full-width primary action
│ [ Archive ]      [ Edit ]        │  secondary actions side-by-side, each ≥44×44px
├────────────────────────────────┤
│ ▸ Plan Artifacts                 │
├────────────────────────────────┤
│ ▾ Version Control                │
│   branch: feat/wip-cap           │
│   commit abc1234                 │
│   [ View PR → ]                  │
├────────────────────────────────┤
│ ▾ Sessions (3)                   │
│   🖥 work · a1b2c3d4         →   │
│   ▸ 🩺 headless-triage-7f2a…     │
│   ▸ 🚫 review-blocked-9c1d…      │
├────────────────────────────────┤
│ ▸ Workflow History                │
├────────────────────────────────┤
│ ▸ Progress History                │
├────────────────────────────────┤
│ ▸ Notes                           │
└────────────────────────────────┘  ◄ scrolls vertically within the overlay;
                                       overlay itself has no horizontal scroll
```

Mobile-specific rules:
- `LifecycleSummary`'s internal layout is `flex-wrap` (per `LifecycleSummary.css.ts`, "layout only;
  no color logic duplicated from child components," Task 2.1.4b) so the StageTracker, BlockerChip,
  pipeline badge, and LivenessLine each drop to their own line rather than being clipped or forcing
  horizontal scroll — the panel body must never scroll sideways.
- The overlay's `[×]`/`[←]` close affordance is the only way to dismiss on mobile (no drag-resize
  handle, unlike desktop) — it must remain reachable at the top of the sticky header regardless of
  scroll position within the overlay.
- Section order is identical to desktop — no mobile-only reordering — so the item's spatial layout
  is predictable across form factors (Recognition Rather Than Recall).

### Interaction Flow

**A. Expand/collapse a section (`CollapsibleSection` header click/tap)**
1. User clicks/taps a section header button (e.g. "Version Control").
2. System: Radix Accordion toggles `data-state`; the header's `aria-expanded` flips
   `"false"→"true"` (or reverse); the panel body mounts/unmounts (not `display:none` — actually
   removed from the DOM per Task 1.1.1a, so collapsed content leaves the Tab sequence);
   `useSectionExpandState()` writes `localStorage["backlog-detail-section-${itemId}-${sectionKey}"]`
   synchronously.
3. Focus remains on the header button that was activated — the newly-revealed content is **not**
   auto-focused (this would be jarring for a scanning/monitoring task).
4. No scroll-jump: the header stays at its current viewport position; content grows/shrinks below
   it.
5. Reloading the page or reopening the same item later restores this exact expand state from
   `localStorage` (Story 1.1.1's second acceptance criterion).
6. **Keyboard nav between sibling headers**: because every `CollapsibleSection` on this panel
   (`Plan Artifacts` through `Notes`, plus `Reviewing`/`Pull Request` where rendered) shares one
   `CollapsibleGroup` (Task 3.1.4i), a user with focus on any section header can press Down/Right
   Arrow to move to the next header, Up/Left Arrow to the previous, Home to jump to the first
   header, and End to jump to the last — all without Tab-ing through intervening (possibly
   DOM-absent) section bodies. This is the concrete, testable form of ADR-027's justification for
   choosing Radix Accordion over `@radix-ui/react-collapsible` — see Accessibility AC 25.

**B. Click a real session row (work or review, `classifySessionKind` = `"work"`/`"review"`)**
1. User clicks the row, rendered as `<a href="/?session=<id>">` — unchanged from today.
2. System navigates to the full interactive terminal view (`SessionMonitor`), same as the
   pre-redesign behavior. Not part of this project's scope beyond preserving it verbatim.

**C. Click a synthetic session row (`headless_diagnostic` / `blocked_guardrail` /
`manual_review_marker`)**
1. User clicks/taps the row — now a `CollapsibleSection` header button (`defaultExpanded={false}`),
   not a dead `<span>` and not a broken `<a>`.
2. System expands **inline**, within the Sessions list — no navigation away from the panel.
   `SessionDiagnosticPanel` dispatches by kind:
   - `headless_diagnostic` with `triageResult` populated → `TriageReviewPanel readOnly`
   - `headless_diagnostic` with `reviewVerdict` populated → `GateVerdictBox readOnly`
   - `blocked_guardrail` / `manual_review_marker` → `BlockedNotice`
3. All three read-only presentations hide action buttons (Apply/Skip/Approve/Override/etc.) but
   keep every informational field, so this is strictly additive over today's dead text — never a
   dead end (see acceptance criteria below).

**D. 5-second poll refresh while the panel is open (Story 3.1.5 — the sharpest risk in this
redesign)**
1. Every 5s (existing polling cadence, unchanged), `load()` refetches the item and
   `useStuckBacklogItems()` refreshes on its own 60s interval.
2. System re-renders `LifecycleSummary` with fresh `StageTracker`/`BlockerChip`/`LivenessLine`
   values — this element is *always* live, on every tick, with no persisted "user closed it" state
   (there's nothing to collapse).
   - **`LivenessLine` is deliberately plain, non-`aria-live` text.** Unlike `SessionDiagnosticPanel`
     (Surface 3), which legitimately wraps itself in `role="status"` because expanding it reveals
     genuinely new content the user just asked for, `LivenessLine`'s "Last activity Nm ago" string
     re-renders on every 5-second poll tick as a normal, silent DOM update — it must **not** be
     wrapped in its own `aria-live`/`role="status"` region. A screen reader that announced "Last
     activity 2m ago" → "Last activity 7m ago" → "Last activity 12m ago" every 5 seconds while the
     panel merely sits open would be noise, not help — the user didn't ask for a running commentary.
     `LivenessLine` should only ever be encountered by assistive tech when the user actively
     navigates focus to it, exactly like any other static text on the page. See Accessibility AC 20.
3. Every `CollapsibleSection`'s expand/collapse state is **untouched** by the poll tick.
   `defaultExpanded` is computed once via `useMemo` keyed only on `itemId` (Task 3.1.5a) — a poll
   tick that returns a new `item` object for the *same* `itemId` cannot re-trigger the default,
   whether or not the user manually collapsed/expanded a section in the meantime.
4. **Known, intentional exception** — sections gated on `item.status` itself (`ReviewingSection`
   only renders while `status === "review"`; `PullRequestSection` only while `status ===
   "pr_pending"`) fully unmount when a poll tick reports the item has left that status, even if the
   user had the section open. This is *preserved-verbatim legacy behavior* (Story 3.1.2's explicit
   "original status guards are preserved verbatim during extraction" AC), distinct from a
   `CollapsibleSection` being force-collapsed while its content is still relevant. It is not a
   violation of "never re-collapse a section the user opened" — the content genuinely no longer
   exists for this item's current state, the same way a "Sessions (3)" list wouldn't keep showing a
   session that was deleted. Flagged here so a future maintainer doesn't mistake it for a
   regression of the Story 3.1.5 guarantee.
5. The one legitimate auto-expand case in this codebase's implementation is **mount-time only**:
   each `CollapsibleSection`'s `defaultExpanded` is evaluated once against the item's state *as of
   first render for that `itemId`* (e.g. opening an item that is already in `review` opens
   `ReviewingSection` immediately). It does not re-fire later in the same viewing session if the
   item transitions into `review` after the panel was already open — this is a deliberate
   simplification versus `research/ux.md §4`'s more elaborate "one-shot nudge on a genuine
   transition" proposal, chosen because it removes an entire class of "did my manual collapse win
   or lose the race against this transition" edge cases for a one-shot mechanism that a solo user
   would see rarely in practice. If this simplification proves insufficient in practice, revisit
   before generalizing to more sections.

### Error and Edge-Case Handling

**Item fails to load** (`GetBacklogItem` RPC error — e.g. network failure, backend 500)
- On the very first load only (`loading && !item`), the panel shows a `role="status"` loading
  state ("Loading…") — unchanged pre-existing behavior, not new for this project.
- On a load failure, `error` is set to the RPC's message (falls back to `"Failed to load item."`)
  and rendered via the existing `InlineError` component pattern inside the still-mounted panel
  chrome (header + close button remain interactive) — the user is never left looking at a blank
  white pane with no way out. **Exit path**: the panel's `[×]`/`[←]` close control remains
  reachable in the header regardless of load error, and `InlineError`'s retry affordance (already
  used elsewhere in this component, e.g. line 823) re-attempts `load()`.
- **Item not found** (deleted/archived-and-purged between list-click and load) shows the distinct
  message `"Item not found."` rather than the generic failure message, so the user isn't left
  wondering whether it's a transient network blip worth retrying — same close/back exit path
  applies.

**Blocker Chip during the `useStuckBacklogItems()` loading race** (extends AC 2's "0-click,
absence=not-blocked" contract)
- While `useStuckBacklogItems()`'s own `isLoading` is `true` — the brief window before that hook's
  first fetch resolves, distinct from `LifecycleSummary`'s own `loading`/`item` state for the
  `GetBacklogItem` call — the `BlockerChip` renders **nothing**: not a loading spinner, not a
  neutral "OK"/"not blocked" placeholder. This is the same visual contract as the "not flagged"
  case (absence = not blocked), applied to the "don't know yet" case too.
- Rationale: the difference between "definitely not blocked" and "haven't checked yet" is real,
  but not worth a dedicated visible loading state — this is a local, single-user tool where
  `useStuckBacklogItems()` resolves in well under a second, so a skeleton/spinner for that window
  would be pure visual noise for a case a user will essentially never see mid-render. If this
  assumption stops holding (e.g. the hook's data source becomes slow or unreliable), revisit before
  adding a distinct loading treatment.

**Zero linked sessions** (Surface 6)
- `SessionsSection` remains visible (default-expanded, per Story 3.1.4c) even when
  `item.linkedSessions.length === 0` — it is never omitted. Renders one status-appropriate line
  instead of a blank body, e.g.:
  - `idea` / `refining` / `ready`: `"No sessions yet — triage hasn't started."`
  - `queued`: `"No sessions yet — waiting for a WIP slot."`
  - Any other status with zero sessions (an inconsistent/edge state): `"No sessions recorded for
    this item."`
- This reuses `getStatusLabel`-driven phrasing (per `research/ux.md`'s explicit guidance) so an
  empty section reads as *expected*, not as a possible fetch failure — directly serving the
  emotional job-to-be-done ("trust nothing silently broke").

**Synthetic session sub-states** (Surfaces 3–5 — see dedicated wireframes below for full detail)
- The UI does **not** uniformly make every synthetic row clickable-to-a-transcript. Only rows
  classified `headless_diagnostic` show structured output (Story 4.1.2); `blocked_guardrail` and
  `manual_review_marker` rows show `BlockedNotice`'s plain explanation (Story 4.1.3) via a
  `CollapsibleSection` — never an affordance implying a transcript exists when it doesn't.

---

## Surface 3: `SessionDiagnosticPanel` — Headless Diagnostic Sub-State

Two variants exist depending on which field is populated on the `LinkedSession` — never both
render for the same row.

### 3a. `headless-triage-*` (has `triageResult`)

```
Sessions (3)
  ▾ 🩺 headless-triage-7f2a9c1d          [expanded]
  ┌────────────────────────────────────────────┐
  │ role="status": "Triage completed — 3 suggestions" │
  ├────────────────────────────────────────────┤
  │ Summary: Refined 3 acceptance criteria for  │
  │ clarity; flagged 1 as ambiguous.            │
  │                                              │
  │ Suggestions:                                │
  │  • Split AC #2 into two testable statements │
  │  • Add explicit error-message text to AC #4 │
  │  • Clarify "recent" in AC #1 (define window)│
  │                                              │
  │ Tasks:                                      │
  │  ☑ Reworded AC #2                           │
  │  ☐ AC #4 error text — not yet applied        │
  │                                              │
  │  (no Apply / Skip / Refine buttons — readOnly)│
  └────────────────────────────────────────────┘
```
Component: `<TriageReviewPanel item={item} triageResult={result} readOnly onApply={noop}
onSkip={noop} />` — action buttons absent from the DOM (not disabled — absent), all informational
content (summary, suggestions, task checklist) present, per Story 4.1.2's acceptance criteria.

### 3b. `headless-re-review-*` (has `reviewVerdict`)

```
Sessions (3)
  ▾ 🩺 headless-re-review-4b8e2f          [expanded]
  ┌────────────────────────────────────────────┐
  │ role="status": "Re-review completed — FAIL" │
  ├────────────────────────────────────────────┤
  │ Verdict: ❌ FAIL                             │
  │ Summary: 2 of 5 criteria still not met after │
  │ rework.                                      │
  │                                               │
  │ Per-criterion outcomes:                      │
  │  ✅ AC1 — Passed                              │
  │  ❌ AC2 — Failed: error not surfaced to user  │
  │  ✅ AC3 — Passed                              │
  │  ❌ AC4 — Failed: missing test coverage       │
  │  ✅ AC5 — Passed                              │
  │                                               │
  │  (no Approve / Reopen / Override / Skip /     │
  │   Re-review buttons — readOnly)               │
  └────────────────────────────────────────────┘
```
Component: `<GateVerdictBox verdict="FAIL" summary="..." criteria={[...]} readOnly ... />` —
action-button row entirely absent, verdict card + per-criterion list present, per Story 4.1.2's
second acceptance criterion.

Both variants use `role="status"` for the one-line state summary at the top (matches
`StatusBadge.tsx`'s existing `role="status"` convention) — this is a single current-state
announcement, not a scrolling log, so `role="log"` does not apply here (there is no raw transcript
for these rows — confirmed in plan.md's Critical Reconciliation: "no raw transcript exists for
synthetic sessions... the diagnostic content is already-fetched structured JSON").

---

## Surface 4 & 5: `SessionDiagnosticPanel` — Blocked-Before-Start / Manual Review Marker

Both `blocked_guardrail` (`review-blocked-*`, `diff-error-*`) and `manual_review_marker`
(`manual-review-*`) render the identical `BlockedNotice` treatment — same shape, different
icon/label — because from the user's perspective both answer the same question ("why is there no
transcript to open") with the same kind of answer (a recorded verdict summary, not a session log).

```
Sessions (3)
  ▾ 🚫 review-blocked-9c1d4a          [expanded]
  ┌────────────────────────────────────────────┐
  │ role="status"                                │
  │ 🚫 Blocked before starting                   │
  │                                               │
  │ Review blocked by security check: secret     │
  │ detected in diff at path/to/file.go:42.       │
  │ Override required to proceed.                │
  │                                               │
  │  (no "open session" affordance of any kind —  │
  │   there was never a session to open)          │
  └────────────────────────────────────────────┘

Sessions (3)
  ▾ ✍️ manual-review-a1b2c3d4          [expanded]
  ┌────────────────────────────────────────────┐
  │ role="status"                                │
  │ ✍️ Manual review                             │
  │                                               │
  │ Manual review: verified fix locally           │
  │                                               │
  │  (no "open session" affordance)               │
  └────────────────────────────────────────────┘
```

- Icon/label pairing per kind (never color-only): `blocked_guardrail` → "🚫 Blocked before
  starting"; `manual_review_marker` → "✍️ Manual review" (exact glyphs at implementer's discretion,
  but the icon+text pairing itself is required — see Accessibility criteria).
- If `reviewVerdict?.summary` is absent (a row that exists but was never given a summary — a
  defensive edge case beyond the 3 documented fixtures), `BlockedNotice` renders `"No summary
  recorded."` rather than a blank body, per Task 4.1.3a's exact fallback text — this keeps the
  panel from ever showing an empty box that looks broken.
- Text is rendered verbatim from the backend, **pending Story 4.1.1's security review**
  (`RunPreGateSecurityCheck`'s error string) confirming no raw secret substring can appear in this
  now-more-discoverable surface. This UX design assumes that review passes; if it does not, the
  fallback is redaction at the source (`session/backlog_review.go`) before this panel ships the
  text, not a UI-layer workaround — do not implement a client-side redaction heuristic as a
  substitute for that backend fix.

---

## Surface 6: `SessionsSection` — Zero Sessions

Covered in the Surface 1/2 "Error and Edge-Case Handling" section above (status-appropriate empty
copy, section never omitted).

---

## Surface 7: Board Card Blocker Chip (`BacklogItemCard.tsx`)

### Wireframe — before/after, full card (header status label + footer)

```
BEFORE (today)                          AFTER (this project)
┌───────────────────────┐               ┌───────────────────────┐
│ Fix flaky WIP-cap test │               │ Fix flaky WIP-cap test │
│                         │               │ Review                 │ ◄ status label (Story 5.1.0) —
│ ...card body...        │               │ ...card body...        │   getStatusLabel(item.status),
├───────────────────────┤               ├───────────────────────┤   header region, new element
│ 3/5 AC     [Resume →]  │               │ 3/5 AC   [View Review →]│ ◄ action button text (unchanged
└───────────────────────┘               ├───────────────────────┤   getActionSpec() — footer)
                                         │ 🟠 Stale work session   │ ◄ BlockerChip(compact) — footer
                                         └───────────────────────┘
                                          (status label only rendered
                                           header-side; BlockerChip only
                                           rendered for items in
                                           useStuckBacklogItems()'s open
                                           list — otherwise byte-identical
                                           to today's footer when absent)
```

- **Three independent status-bearing elements now coexist on one card** — `BacklogItemCard`'s new
  canonical status label (header, Story 5.1.0, sourced from `getStatusLabel(item.status)` — the
  same vocabulary `BacklogItemBadge.tsx` and the Stage Tracker's active-node label use), the
  existing action-button text (footer, `getActionSpec()`, unchanged), and the compact `BlockerChip`
  (footer, Story 5.1.1, only when stuck). None may read as contradictory (see Consistency AC 28).
  They *can* read as somewhat redundant in specific cases — e.g. status label `"Review"` sitting
  near an action button reading `"View Review"` — which is a **known, flagged risk** (pre-mortem
  finding #5) to watch during implementation, not something this document mandates a specific
  layout/copy fix for; see Task 5.1.0's own framing of the same trade-off.
- Compact `BlockerChip` variant = icon + label only (no duration, per Story 1.1.4's distinction
  from the full variant used in `LifecycleSummary`) — a card is scanned in a grid, not read line by
  line, so the duration ("3d") is opt-in detail available one click away in the detail panel, not
  needed at a-glance on the card.
- `cardFooter` gains `flex-wrap` (Task 5.1.1c) so the chip drops to its own line on narrow/mobile
  card widths rather than overflowing or clipping the AC fraction / action button.
- `useStuckBacklogItems()` is called **once** at `board/page.tsx` level and the resolved
  `StuckBacklogItem | undefined` passed down per card (Task 5.1.1a) — not one independent 60s poll
  per card, which would both waste requests and risk cards updating out of sync with each other on
  the same screen.

### `/backlog` non-board list row (`BacklogItemBadge.tsx`) — explicit open decision

Per plan.md Story 5.1.2, whether the narrower list row also gets the compact `BlockerChip` is an
implementation-time width measurement, not a design-time call this document can make in the
abstract. This design's requirement either way: **if implemented**, it must be the same
`BlockerChip` component/compact variant, not a second bespoke chip (per the D1-style duplication
concern this whole project exists to close). **If deferred**, the stuck item must still be
discoverable from `/backlog`'s list view via some existing path (e.g. clicking into the detail
panel, where `LifecycleSummary`'s full-variant chip always renders) — deferring the card-level
chip must never mean a stuck item becomes invisible from that surface entirely, only that it takes
one more click to see the reason, which is acceptable since `/unfinished` (a separate, already-
shipped project) is the primary triage surface for stuck items, not `/backlog`'s list view.

---

## Surface 8: Board-Wide Stuck-Data Fetch Failure

`useStuckBacklogItems()` retains the previous `items` and populates `error` on a failed refresh
(hook's own documented contract, `useStuckBacklogItems.ts`) rather than blanking the list — this
matters for both `LifecycleSummary` (Surface 1/2) and `BacklogItemCard` (Surface 7), since both are
downstream consumers of the same hook.

```
Board page, top of list (existing "may be out of date" banner convention)
┌──────────────────────────────────────────────────┐
│ ⚠ Stuck-item status may be out of date — retrying  │
└──────────────────────────────────────────────────┘

Each card and the detail panel's BlockerChip keep showing their LAST KNOWN blocker state
underneath — never a false "all clear" caused by a fetch error being misread as "no items stuck."
```

- **Never a false-confidence empty state**: if the fetch fails, the UI must not interpret "no data
  returned" as "no items are stuck" — the retained-stale-data contract already implemented in the
  hook is what this design relies on; this project's job is to make sure `LifecycleSummary` and
  `BacklogItemCard` both consume `error`/`items` correctly (i.e. don't independently re-derive an
  "is anything stuck" boolean from a fresh empty array on error) rather than re-implement the
  retry/backoff logic itself, which is out of scope and already shipped.
- **Exit path**: the banner is informational only, not a dead end — the underlying board/list and
  detail panel remain fully interactive using last-known data while a retry is pending in the
  background (existing hook behavior, unchanged by this project).

---

## Surface 9: "Show N More" Expansion (Sessions / Workflow History / Progress History)

Blocker C fix + pre-mortem finding #2 (`useShowMore`, Task 3.1.4c2/d2/e2). Three sections —
`SessionsSection`, `WorkflowHistorySection`, `ProgressHistorySection` — cap their default rendering
to the most recent N entries (5 sessions, 8 workflow events, 8 progress notes respectively) and
reveal the rest via a "Show N more" button, so a single already-expanded section can't itself
reproduce the "everything visible, nothing prioritized" problem this whole project exists to fix,
one level down, for the heavily-cycled items that most need this.

### Capped view (default — `SessionsSection`, `df0d5872`-shaped item, 11 total linked sessions)

```
Sessions (11)                                        [expanded — default true]
  🖥  work · a1b2c3d4          started 4d ago      →
  ▸ 🩺 headless-re-review-9f0a  FAIL — 2/5 criteria
  ▸ 🚫 review-blocked-7c2e     blocked pre-flight
  ▸ 🩺 headless-triage-4b1d    3 suggestions
  ▸ ✍️ manual-review-e8d3      verified fix locally
  ┌──────────────────────────────────────────────┐
  │         Show 6 more                           │ ◄ data-testid="sessions-show-more"
  └──────────────────────────────────────────────┘   real <button>, ≥44×44px, Enter/Space activates
     (5 most recent shown; 6 older entries hidden — nothing summarized or truncated, just deferred)
```

### Expanded view (after clicking "Show N more")

```
Sessions (11)                                        [expanded — default true]
  🖥  work · a1b2c3d4          started 4d ago      →
  ▸ 🩺 headless-re-review-9f0a  FAIL — 2/5 criteria
  ▸ 🚫 review-blocked-7c2e     blocked pre-flight
  ▸ 🩺 headless-triage-4b1d    3 suggestions
  ▸ ✍️ manual-review-e8d3      verified fix locally
  ▸ 🩺 headless-triage-2a9c    1 suggestion
  ▸ 🩺 headless-triage-1f7b    2 suggestions
  ▸ 🚫 diff-error-6d4a         blocked pre-flight
  🖥  work · b3c4d5e6          started 4d ago      →
  ▸ 🩺 headless-re-review-3e8f  FAIL — 1/5 criteria
  ▸ ✍️ manual-review-9a2b      verified fix locally
     (all 11 shown inline, in the same list — no pagination, no route change, no "Show more"
      button remains once fully expanded)
```

### Persistence — the one detail that distinguishes this from a plain toggle

- The "show all" state is **not** a plain `useState` that resets to the capped view on every mount.
  It is `localStorage`-backed via the shared `useShowMore` hook (same pattern/key convention as
  `useSectionExpandState`, key `backlog-detail-showmore-${itemId}-${sectionKey}`), per pre-mortem
  finding #2.
- **Given** the user clicks "Show 6 more" on `SessionsSection` for `itemId="itm_df0d5872"`, **when**
  the user later navigates away and re-opens the same item (a fresh mount), **then**
  `SessionsSection` renders already expanded to all 11 sessions — the button does not need to be
  clicked again, and `localStorage["backlog-detail-showmore-itm_df0d5872-sessions"]` reads `"true"`.
- This matters most for exactly the chronically-stuck, heavily-cycled items this project exists to
  make inspectable: those are the items the user re-opens most often to check status, so a
  plain-`useState` cap would make them re-pay the same click every single visit — a direct
  regression against the project's own success metric for its hardest cases.
- Items with fewer entries than the cap never show a "Show N more" button at all — the cap only
  ever adds a control, never removes information a short list would otherwise show outright.
- `WorkflowHistorySection` (cap 8, `data-testid="workflow-show-more"`) and `ProgressHistorySection`
  (cap 8, `data-testid="progress-history-show-more"`) follow the identical capped/expanded/
  persisted-on-reopen pattern shown above for `SessionsSection` — same button semantics, same
  `useShowMore` hook, different `sectionKey`/cap/testid.

---

## UX Acceptance Criteria

Each item below is independently testable by a human exercising the running app.

### Task efficiency

1. **Identify current lifecycle state** — from first render of the detail panel (desktop or
   mobile), before expanding anything, the user can name the item's exact backend status by
   reading the `StageTracker`'s highlighted node. **0 clicks.**
2. **Identify current blocker** — from first render, if the item is flagged by
   `useStuckBacklogItems()`, the user can read the blocker reason (icon + label + duration) without
   expanding anything. **0 clicks.** If not flagged, the absence of a chip is itself legible as
   "nothing blocking" (no neutral placeholder chip to misread). This same absence-is-legible
   contract also covers the brief window before `useStuckBacklogItems()` itself has resolved — see
   Error and edge-case coverage AC 14.
3. **Judge liveness ("is this actually still working or silently dead")** — from first render, the
   user can read "Last activity Nm/h/d ago" without expanding anything or cross-referencing
   Progress History. **0 clicks.**
4. **Inspect any linked session's outcome** (work, triage, review, headless-review, blocked,
   manual-override) — the user can see meaningful content for every row in the Sessions list.
   **≤2 clicks**: 1 to expand "Sessions" if collapsed (it defaults to expanded, so typically 0), + 1
   to expand the specific synthetic row (real sessions: 1 click total, straight to the terminal
   view). For items with more than 5 linked sessions, add ≤1 click for "Show N more" (AC 7) before
   the specific row is reachable at all.
5. **Find why an item is stuck without inferring it from Progress History text** — the
   `BlockerChip`'s label is sufficient on its own; Progress History is never the only place this
   information exists. **0 clicks** beyond metric #2.
6. **Cross-reference two sections simultaneously** (e.g. "which session produced this PR" —
   Sessions vs. Version Control) — both can be expanded at once; expanding one never force-collapses
   the other (multiple-open accordion, not single-open).
7. **Inspect a long-running/heavily-cycled item's full Sessions, Workflow History, or Progress
   History without re-paying a "Show more" click on every visit** (Surface 9) — for a section whose
   entry count exceeds its cap (5 sessions / 8 workflow events / 8 progress notes), revealing the
   rest costs **≤1 click** ("Show N more"), the same as metric #4's cost model. Because
   `useShowMore`'s expanded state is `localStorage`-persisted per item/section (not a plain
   in-memory toggle), re-opening the *same* item on a later visit shows the section already
   expanded — **0 clicks** on every visit after the first. This matters specifically for the
   chronically-stuck items this project exists to make inspectable, since those are exactly the
   items a user re-opens most often to check status.

### Error and edge-case coverage

8. Item load failure shows a specific message (`"Item not found."` for a missing item, or the
   RPC's own error text / `"Failed to load item."` fallback for other failures) — never a blank
   pane, never a silent no-op.
9. Every error state has a working exit path: the close/back control in the panel header remains
   clickable regardless of load error state; `InlineError`'s retry affordance re-attempts the
   failed operation. **No dead ends.**
10. An item with zero linked sessions shows an explicit, status-appropriate one-line explanation
    ("No sessions yet — triage hasn't started." etc.) — the Sessions section is never silently
    omitted or left blank in a way indistinguishable from a fetch failure.
11. A `blocked_guardrail`/`manual_review_marker` row never presents a clickable "view session"
    affordance that leads to an empty or broken screen — the row's `CollapsibleSection` reveals
    `BlockedNotice`'s explanation text directly, with no intermediate dead click.
12. A `headless_diagnostic` row always resolves to exactly one of `TriageReviewPanel readOnly` /
    `GateVerdictBox readOnly` based on which field (`triageResult` vs. `reviewVerdict`) is
    populated — never both, never neither with a blank panel.
13. Stuck-item data fetch failure never presents as "nothing is stuck" — the UI shows a visible
    "may be out of date" signal and continues showing the last-known `BlockerChip` state rather
    than silently clearing it.
14. While `useStuckBacklogItems()`'s own `isLoading` is `true` (the brief window before that hook's
    first fetch resolves — distinct from the item's own `GetBacklogItem` load state), the
    `BlockerChip` renders **nothing**: no loading spinner, no neutral "OK" placeholder — the same
    "absence = not (yet known to be) blocked" contract as AC 2, deliberately not distinguished with
    a dedicated loading state because the hook resolves in well under a second on this local
    single-user tool.
15. A manual expand/collapse choice on any `CollapsibleSection` survives at least one 5-second
    poll refresh unchanged — verified by: expand a section, wait 5+ seconds (or trigger a manual
    refresh), confirm `aria-expanded` is unchanged from what the user last set.
16. Switching from item A to item B (clicking a different item in the list) fully resets per-item
    UI state — no leftover open manual-review form, no leftover section-expand override from item
    A bleeding into item B's *first-render* defaults (item B's own persisted `localStorage` state,
    if any, applies instead).

### Accessibility

17. Every `CollapsibleSection` header is a real `<button aria-expanded="true"|"false">` — never a
    `<div onClick>` — verifiable via the browser accessibility tree or `axe-core` (this repo's CI
    already runs Axe Core on PRs touching `web-app/src/`, blocking on WCAG AA violations).
18. Collapsed section content is absent from the accessibility tree and the Tab sequence (verified:
    `Tab` from the header of a collapsed section moves to the *next* section's header, not into
    hidden interactive content) — not merely visually hidden via `display:none` on a still-focusable
    subtree.
19. `SessionDiagnosticPanel`'s one-line state summary uses `role="status"`; it does not additionally
    wrap itself in `role="log"` (no raw transcript exists for these rows, so `role="log"`'s
    implicit-`aria-live` scrolling-content semantics do not apply here — confirmed against
    plan.md's Critical Reconciliation).
20. `LivenessLine` is **not** wrapped in its own `aria-live`/`role="status"` region — verified by:
    inspecting `LivenessLine`'s rendered DOM (and its container within `LifecycleSummary`) and
    confirming no explicit or inherited `aria-live` attribute is present. This is the deliberate
    inverse of AC 19: `SessionDiagnosticPanel` legitimately announces content the user just asked
    to reveal (a one-time expand), while `LivenessLine` re-renders silently on every 5-second poll
    tick — wrapping it in `aria-live` would re-announce "Last activity Nm ago" to screen reader
    users every single tick while the panel merely sits open, which is noise, not help. A screen
    reader must only encounter this text when the user actively navigates focus to it.
21. Every status/blocker/session-kind indicator pairs an icon with a text label — verified by:
    disabling color (e.g. via a grayscale browser filter or forced-colors mode) and confirming the
    meaning is still legible from text/icon shape alone. Applies to: `StageTracker` node labels,
    `BlockerChip` (both variants), `SessionDiagnosticPanel`'s dispatch icons (🩺/🚫/✍️), and
    `BlockedNotice`'s kind label.
22. Text contrast for all new/changed UI elements (`StageTracker` node labels, `BlockerChip` text,
    `LivenessLine`, `BlockedNotice` body text, `CollapsibleSection` header text) meets WCAG AA —
    ≥4.5:1 for normal text, ≥3:1 for large text (≥18pt/24px or ≥14pt/19px bold) — against both the
    light and dark theme backgrounds this codebase supports.
23. Every interactive header/button/link introduced or changed by this project (`CollapsibleSection`
    headers, session row links, `Actions` buttons, and the `useShowMore` "Show N more" buttons in
    Sessions/Workflow History/Progress History) has a touch target ≥44×44px on the mobile layout —
    verified by inspecting computed `min-height`/`min-width` or padding box on the rendered element
    at ≤768px viewport width.
24. Keyboard-only navigation can reach and operate every new affordance — every `CollapsibleSection`
    header and every `useShowMore` "Show N more" button is reachable via `Tab` and operable via
    `Enter`/`Space`; no new affordance is mouse/touch-only.
25. Arrow-key navigation moves focus directly between `CollapsibleSection` headers within a shared
    `CollapsibleGroup`; `Home`/`End` jump to the first/last header — without needing to `Tab`
    through any intervening (possibly DOM-absent) section body content. This is the concrete,
    testable form of ADR-027's justification for choosing Radix Accordion over
    `@radix-ui/react-collapsible` (Task 1.1.1c) — verified by placing focus on any section header
    inside `BacklogItemDetail`'s shared `CollapsibleGroup` (Task 3.1.4i) and pressing Down Arrow /
    Up Arrow / Home / End.
26. Focus is never programmatically moved into newly-revealed `CollapsibleSection` content on
    expand — the header retains focus after activation, matching standard toggle-button behavior
    and avoiding a jarring jump for a scanning-oriented task.

### Consistency (board ↔ detail)

27. An item flagged by `useStuckBacklogItems()` shows the *same* `StuckReason` label/icon in both
    the board card's compact `BlockerChip` and the detail panel's full `BlockerChip` — verified by
    opening the same item's card and detail view side by side (or in sequence) and confirming the
    label text is character-for-character identical (duration is the only difference, per the
    full/compact variant contract).
28. No status text appears in more than one place within the redesigned detail panel that could
    contradict itself — verified by confirming the old standalone status badge markup
    (`BacklogItemDetail.tsx:710-714`, pre-redesign) no longer exists anywhere in the render tree,
    and `AcCriteriaList` vs. `GateVerdictBox`'s criteria list are visually distinguished by heading
    text as two different questions ("checklist" vs. "review outcome"), not two copies of the same
    list. This extends to the board card (Surface 7): `BacklogItemCard`'s canonical status label
    (Story 5.1.0, header), its existing action-button text (footer, `getActionSpec()`), and its
    compact `BlockerChip` (footer, when stuck) must not read as contradictory. They may read as
    somewhat redundant in specific cases — e.g. a `"Review"` status label next to a `"View Review"`
    action button — which is a **known risk to watch during implementation** (pre-mortem finding
    #5), not a specific layout/copy fix this document mandates.
