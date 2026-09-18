# UX Design: backlog-custom-workflow-stages

Design Agent — SDD Phase 3 (design), covering Phase 2 (Milestone 2+) epics 2.8–2.10 of
`implementation/plan.md` plus the Phase 1 RPC-only surface (Epic 1.3.2). Extends
`research/ux.md` (read in full) rather than re-deriving its recommendations — this doc
turns that research into concrete wireframes, flows, and testable acceptance criteria.

**Naming, locked per research/ux.md §0**: every surface below says "Stage(s)" or
"Backlog Stage(s)," never "Workflow(s)."

**Scope note on liveness UI**: `implementation/plan.md`'s Milestone Structure explicitly
states Phase 1's `LivenessDefinition` CRUD is "no UI in Phase 1 — operator-callable
only," and Epic 2.8's stories (2.8.1 stage list, 2.8.2 stage form) do not include a
liveness-parameter editor. This design follows the plan: no liveness-editing UI is
designed here. The one exception is the read-only liveness/staleness panel on item
detail (Surface 4), which *displays* a resolved liveness value but never edits one. If
a future milestone adds a liveness editor, it should reuse the same field/label/hint
pattern as `PipelineModeForm.tsx`'s template fields — flagged here, not designed here.

---

## Surfaces designed

1. Backlog Stages list page (`/settings/backlog-stages`) — list-based CRUD
2. Stage form with nested transitions + gate sub-form (edit/create overlay)
3. Read-only generated graph diagram (embedded in the stage form)
4. Item-detail "what's blocking this transition" panel (liveness + gate checklist)
5. BacklogBoard / StageTracker dynamic stage rendering (condensed — extends an existing
   interactive surface rather than introducing a new interaction model)

Plus three condensed non-interactive entries: the Phase 1 Liveness CRUD RPC surface,
structured log output, and the graph-validation RPC contract.

---

## Surface 1: Backlog Stages list page

**Precedent extended**: `web-app/src/app/settings/pipeline-modes/page.tsx` verbatim
structure — header + New button, `role="alert"` load-error banner, flat list of rows,
`role="switch"` enabled toggle, Edit button, slide-in form overlay that dims (not
replaces) the list.

```
┌────────────────────────────────────────────────────────────────────┐
│ Backlog Stages                                       [+ New Stage]  │
│ Configure workflow stages, the transitions between them, and the    │
│ gates that must pass before a transition is allowed.                │
├────────────────────────────────────────────────────────────────────┤
│ [role=alert] Couldn't load stages — <message>          [Retry]      │  ← only on load failure
├────────────────────────────────────────────────────────────────────┤
│ ●  Idea            idea             [Built-in]              [Edit]  │
│ ●  Refining        refining         [Built-in]              [Edit]  │
│ ●  Ready           ready            [Built-in]              [Edit]  │
│ ●  Queued          queued           [Built-in]              [Edit]  │
│ ●  In Progress     in_progress      [Built-in]              [Edit]  │
│ ●  Review          review           [Built-in]              [Edit]  │
│ ●  PR Pending      pr_pending       [Built-in]              [Edit]  │
│ ●  Done            done             [Built-in, terminal]    [Edit]  │
│ ●  Archived        archived         [Built-in, terminal]    [Edit]  │
│ ●  Design Review   design-review    [●on]                   [Edit]  │
│ ○  Legal Review    legal-review     [○off] disabled         [Edit]  │
└────────────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. Page loads → `listStages()` (cached RPC, same caching mandate as `PipelineEngine`) →
   rows render in graph order (entry stages first, then BFS order) so the list reads
   top-to-bottom the way the workflow actually flows.
2. **New Stage** → form overlay (Surface 2) opens blank; list stays visible, dimmed.
3. **Edit** on any row (built-in or custom) → form overlay opens pre-filled; slug input
   is `disabled` for every row, built-in or custom (slugs are never editable post-
   creation — matches `PipelineModeForm.tsx:170`, and built-in slugs like `"idea"` are
   referenced by Go code, not just config, so this is doubly true here).
4. Toggle (`role="switch"`) flips `enabled` via `UpdateStage(id, {enabled})` directly
   from the list row — no form needed, mirrors `pipeline-modes/page.tsx:95-108`.
5. A **built-in** stage's row carries a `[Built-in]` badge instead of an
   enabled/disabled toggle-badge pair; built-in stages can still be edited (name,
   description, transitions, gates) and can still be *disabled* like any custom stage,
   but see Surface 2 for why their Delete button is always inert.

### Error / edge cases

| Case | UI response |
|---|---|
| `listStages()` fails (network/server error) | `role="alert"` banner: `"Couldn't load stages — <ConnectError message>"` + a `[Retry]` button that re-calls `listStages()`. List area shows nothing else (no stale/partial list). |
| Zero stages returned (e.g., an unseeded dev DB) | Empty-state text: `"No stages configured — the built-in 9-stage workflow is active by default."` + the New Stage button still works, since seeding gaps must never block creating a custom stage. |
| Toggle-off fails (e.g., backend rejects disabling an `IsEntry` stage with no other enabled entry stage) | Switch visually reverts to its prior state (not left in an ambiguous half-toggled state) and an inline `role="alert"` appears under that row: `"Can't disable 'Idea' — it's the only enabled entry stage."` No page-level banner; the error is scoped to the row that caused it. |
| Toggle succeeds but the stage has live items on it | No warning needed for *disable* (per research/ux.md §2/§4, disabling is always the safe, encouraged action) — only *delete* is guarded (Surface 2). |

---

## Surface 2: Stage form — nested transitions + gate sub-form

**Precedent extended**: `PipelineModeForm.tsx`'s overlay/slug-immutable/two-step-delete/
inline-`role=alert`-error shape, generalized with one new collapsible section
("Outgoing transitions") per research/ux.md §1/§2's explicit recommendation: no canvas,
a second-level list nested inside the same form.

```
┌ Edit Stage: Design Review ────────────────────────────────[Cancel]─┐
│ Slug: design-review  (immutable after creation)                     │
│ Name:        [Design Review______________________]                 │
│ Description: [textarea_______________________________]             │
│ [ ] Entry stage    [ ] Terminal stage    [x] Enabled                │
│                                                                      │
│ ▾ Outgoing transitions                              [+ Add          │
│                                                        transition]  │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ To: [ready ▾]                                     [Remove]  │   │
│  │  Gates:                                                      │   │
│  │  [x] Human approval                                          │   │
│  │  [ ] Automated review                                        │   │
│  │  [ ] Structural check                                        │   │
│  │  [ ] Custom check                                            │   │
│  └────────────────────────────────────────────────────────────┘   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │ To: [in_progress ▾]                               [Remove]  │   │
│  │  Gates:                                                      │   │
│  │  [ ] Human approval                                          │   │
│  │  [x] Automated review                                        │   │
│  │      Review prompt / mode:  [sdd-feasibility ▾]              │   │
│  │      [ ] Requires diff (unchecked — no code artifact yet)    │   │
│  │  [ ] Structural check                                        │   │
│  │  [x] Custom check                                            │   │
│  │      Skill/command:  [lint-full-repo ▾]  (pre-registered      │   │
│  │        allowlist only — see ADR-003)                         │   │
│  └────────────────────────────────────────────────────────────┘   │
│                                                                      │
│ ▾ Graph preview (read-only) — see Surface 3                         │
│                                                                      │
│ [role=alert error banner, if save failed — see Error table]         │
│ [Save changes]  [Cancel]                     [Delete] or            │
│                                        "3 items on this stage —     │
│                                         disable instead, or move     │
│                                         them off first" (tooltip)   │
└──────────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. **Add transition** → appends a blank row (`{targetStage: "", gates: []}`) to the
   sub-list. Target-stage control is a `<select>` (not a radiogroup) per
   research/ux.md §3.1's stated threshold — "one of N existing stages" is a
   value-lookup task once N exceeds ~5-6, and the built-in set alone is 9.
2. **Check a gate checkbox** → reveals that gate kind's config field(s) inline,
   directly below the checkbox, in the same row — progressive disclosure, mirroring
   `OmnibarCreationPanel.tsx`'s advanced-options pattern. Unchecking hides *and clears*
   the field (no orphaned hidden state that resurfaces stale on next check).
3. **Remove** on a transition row deletes that row from the in-progress form state
   (not yet persisted); removing a transition that already has recorded
   `GateSatisfactionRecord`s is allowed — those rows become orphaned server-side and
   are simply never read again, not a blocking condition client-side.
4. **Save changes** → client-side required-field check first (a checked gate with an
   empty required sub-field is caught *before* the RPC call, not after — see Error
   table), then `CreateStageTransition`/`UpdateStage` calls run; the Epic 2.6 graph
   validator runs server-side on every mutating call.
   - **Success, no warnings** → overlay closes, list (Surface 1) updates in place
     (no refetch), matching `handleSaved` in `pipeline-modes/page.tsx:77-87`.
   - **Success, with warnings** (e.g., a gate-free cycle) → overlay stays open, an
     amber `role="status"` banner appears above the action row summarizing the
     warning (e.g., `"Warning: In Progress → Review → Design Review → In Progress
     forms a cycle with no gates on any edge — an item could loop indefinitely."`)
     with a `[Got it]` button that dismisses the banner and closes the overlay. This
     is a deliberate divergence from the "success = close immediately" pattern,
     because per research/ux.md's synthesis, silent misconfiguration discovered later
     is worse than one extra click now.
   - **Rejected** (`CodeInvalidArgument` from the graph validator — e.g. an
     unreachable stage) → inline `role="alert"` banner, form state fully preserved,
     exact backend message shown verbatim (mirrors `PipelineModeForm.tsx`'s
     `errorMessage()` helper).
5. **Delete** → same two-step confirm-in-place as `PipelineModeForm.tsx:250-283`
   ("Delete" → "Confirm delete?" / "Never mind"), *but* the Delete button itself is
   `disabled` with a `title` tooltip whenever the stage has ≥1 live item on it or is a
   built-in stage — reusing the exact `disabled` + `title` tooltip pattern
   research/ux.md §4 cites from `BacklogItemDetail.tsx`'s Trigger Triage button. A
   built-in stage's tooltip reads `"Built-in stage — disable it instead of deleting."`;
   a stage with live items reads `"3 items are on this stage — disable it instead, or
   move them off first."`

### Error / edge cases

| Case | UI response |
|---|---|
| Checked gate with empty required sub-field (e.g. "Automated review" checked, no prompt selected) | Caught client-side before the RPC fires: the sub-field gets `aria-invalid="true"` and an inline message below it — `"Select a review prompt for this gate."` Save button stays enabled (not disabled-and-mysterious) but clicking it re-focuses the first invalid field instead of submitting. |
| Custom-check gate names a skill not in the pre-registered allowlist (only reachable via direct API, not this `<select>`, since the `<select>` only lists allowlisted names) | Server rejects with `CodeInvalidArgument`; UI shows the standard inline error banner. The `<select>` itself structurally prevents this from the UI path — it is not user-reachable through normal use, consistent with ADR-003's bounded-execution intent. |
| Graph validator rejects (unreachable stage, non-terminal dead end) | Inline `role="alert"` banner with the exact server message (e.g. `"Stage 'design-review' has no outgoing transitions and is not marked terminal."`); form state preserved; user fixes in place (add a transition, or check "Terminal stage") and re-saves — no navigation, no lost work. |
| Editing/removing a transition or gate that live items currently depend on | No special client-side block beyond the Delete-button guard above (editing a *transition's gates*, as opposed to deleting the *stage*, has no referential-integrity break — Epic 2.5's `StageConfigSnapshot` makes history immune, and `ConfiguredWorkflowEngine.PendingGates` always re-evaluates fresh, so an edited gate set applies going forward without corrupting anything already recorded). A soft hint still appears: `"Note: 3 active items are currently on this stage. Changes to its gates apply the next time each item's transition is evaluated."` |
| Malformed/unresolvable gate config discovered only at evaluation time (not save time — e.g. a gate's referenced pipeline mode is deleted *after* this transition was saved) | Not surfaced on this form at all (it was valid when saved) — surfaced instead on the item-detail gate checklist, Surface 4, per the "fail closed and loud" mandate. This form's validator only catches problems that exist *at save time*; it cannot and does not promise to catch a later-introduced dangling reference. |
| Save RPC times out / network failure | Same inline `role="alert"` pattern, generic message (`"Couldn't save — <message>. Your changes are still here; try again."`), form state fully preserved — never a lost edit. |

---

## Surface 3: Read-only generated graph diagram

**Precedent / constraint**: research/ux.md §1/§3 — explicitly *not* the editing surface
(the list-based sub-editor in Surface 2 is), and explicitly *not* a dependency pull
(`build-vs-buy.md` confirms no graph/node-edge library exists in `web-app/package.json`
today and recommends against adding one for this). Rendered as computed inline SVG
using a simple deterministic layered layout (BFS rank from every `IsEntry` stage → each
rank is a column, left to right) — no manual positioning, no drag, nothing to lay out.

```
▾ Graph preview (read-only)
┌───────────────────────────────────────────────────────────────────┐
│  [Idea]───▶[Ready]───▶[Queued]───▶[In Progress]───▶[Review]        │
│                            ▲                            │  🔒2      │
│                            │                            ▼           │
│                    [Design Review]◀────────────────────┘           │
│                                                          │           │
│                                                          ▼           │
│                                              [PR Pending]───▶[Done] │
│                                                                      │
│                                                          [Archived] │
│  🔒N badges mark an edge with N gate(s) attached                    │
└───────────────────────────────────────────────────────────────────┘
```

Screen-reader-equivalent markup (present in the accessibility tree, not
`display:none` — CSS `.sr-only` clip-based hiding only):

```html
<figure aria-label="Workflow stage graph">
  <svg aria-hidden="true">...decorative diagram...</svg>
  <table class="sr-only">
    <caption>Stage transitions and gate counts</caption>
    <thead><tr><th>From</th><th>To</th><th>Gates</th></tr></thead>
    <tbody>
      <tr><td>Idea</td><td>Ready</td><td>0</td></tr>
      <tr><td>Review</td><td>Design Review</td><td>2 (human approval, automated review)</td></tr>
      <!-- one row per edge, exactly matching the SVG -->
    </tbody>
  </table>
</figure>
```

### Accessibility verification (not asserted — reasoned through)

- **2.5.7 Dragging Movements / 2.5.1 Pointer Gestures**: N/A by construction — this
  view has zero pointer/drag interaction of any kind; it is `aria-hidden` decoration.
  The actual editing happens entirely in Surface 2's form controls (native `<select>`,
  `<input type=checkbox>`, `<button>`), which are keyboard-operable by definition. This
  is the concrete payoff of research/ux.md's core recommendation: by never making the
  diagram the edit surface, the WCAG dragging-gesture risk that a canvas editor would
  carry simply does not arise here.
- **1.1.1 Non-text Content**: the SVG is marked `aria-hidden="true"` specifically
  *because* the adjacent `sr-only` table is the actual text alternative — marking the
  SVG itself with a single `aria-label` summarizing "9 stages, 11 transitions" would be
  a materially worse alternative (loses the per-edge gate-count detail the sighted
  view conveys), so the table-fallback pattern is chosen deliberately, not by default.
- **1.4.1 Use of Color**: the `🔒N` gate-count badge is conveyed by a numeral plus a
  lock glyph, not color alone, so a gated vs. ungated edge is distinguishable without
  color perception.
- **1.4.3 Contrast (AA)**: edge lines and node borders reuse this app's existing
  `vars.color.border`/`vars.color.text` design tokens rather than introducing new
  diagram-specific colors — the same tokens already used throughout
  `StageTracker.css.ts`'s stepper, which has shipped and presumably passed this
  project's existing accessibility bar. No new color pair is invented for this
  surface, so no new contrast risk is introduced.
- **Zoom/reflow (1.4.10)**: the diagram sits inside a horizontally-scrollable
  container (`overflow-x: auto`) rather than shrinking node text at high zoom —
  matches this repo's general wide-content convention (tables, code blocks) rather
  than a diagram-specific new pattern.

---

## Surface 4: Item-detail "what's blocking this transition"

**Precedent extended**: `GateVerdictBox.tsx`'s verdict-card pattern
(`role="status" aria-live="polite"`, a `VERDICT_CONFIG`-style map keyed by state),
generalized from *one* verdict for the whole transition into an independent row per
gate — the GitHub-branch-protection shape research/ux.md §1/§4 identifies as the
correct precedent for "N independently-satisfiable requirements."

Per research/ux.md §5's JTBD ranking, the **liveness/staleness panel gets top billing**,
rendered above the gate checklist, not folded into or below it — this is the direct
answer to the motivating bug (a silent structural failure with no UI signal at all).

```
┌ ⏱ Stuck: Orphaned Triage ──────────────────────────────────────────┐
│ This item's headless triage call started 42m ago with no response.  │
│ Expected duration: 45m (sdd mode) · Staleness margin: 10m            │
│ Next automatic retry: in 3h 12m           [Retry now]                │
└──────────────────────────────────────────────────────────────────┘

┌ What's blocking Review → Design Review? ────────────────────────────┐
│ ○ Human approval              Pending — click Approve below          │
│                                   [Approve]   [Reject]                │
│                                                                        │
│ ✓ Automated review             Satisfied (PASS · 2026-09-01 14:02)   │
│                                                                        │
│ ✗ Structural check (ac_complete)  Blocked — 2 of 5 acceptance         │
│                                     criteria incomplete                │
│                                                                        │
│ ⚠ Custom check (lint-full-repo)   Configuration error — this gate    │
│                                     can't be evaluated (skill removed  │
│                                     from allowlist)   [Fix in Stages   │
│                                     settings →]                       │
└──────────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. Item-detail page loads → fetches `PendingGates(itemId, nextCandidateTransition)`
   alongside existing stuck/liveness data it already displays.
2. If the item is currently flagged stuck (existing `BacklogStuckState` data), the
   liveness panel renders first, using the exact `reasonDetail`/threshold values
   `LivenessEngine` resolved (Epic 1.6's Info log line's same data, surfaced to a
   human instead of only a log). `[Retry now]` calls the existing manual-remediation
   RPC (no new RPC needed — reuses whatever `ResetStuckRemediation`-equivalent already
   exists for this).
3. Gate checklist renders one row per `GateStatus` returned:
   - **Human approval, unsatisfied** → `[Approve]`/`[Reject]` buttons call
     `RecordGateApproval(itemId, gateId, approved)`. While pending, both buttons show
     a disabled/loading state; on success the row flips to `✓ Satisfied` (one-shot —
     does not re-ask); on failure, an inline error appears *under that row only*
     (`"Couldn't record approval — <message>. [Try again]"`), buttons re-enable, no
     other row is affected.
   - **Automated review / structural**, already satisfied → static `✓` row, no
     action affordance (nothing to click — these aren't awaiting the operator).
   - **Structural, unsatisfied** → static `✗` row with the exact unmet-precondition
     description (`"2 of 5 acceptance criteria incomplete"`), re-evaluated fresh on
     every page load/refresh — never a stale cached verdict.
   - **Config error** (any gate kind whose backing config no longer resolves) → `⚠`
     row reading exactly `"Configuration error — this gate can't be evaluated
     (<reason>)"`, plus a `[Fix in Stages settings →]` link deep-linking to that
     stage's edit form (Surface 2) — this is the exit path; the row is never a dead
     end.
4. If the item is currently on a since-deleted custom stage (frozen `StageConfigSnapshot`
   in effect, per Epic 2.5.2), a one-line notice renders above the checklist:
   `"This item is on a stage no longer in the current configuration. Transitions
   shown reflect the configuration when it entered this stage."` — the checklist
   below still renders normally against the snapshot, it is never blanked.

### Error / edge cases

| Case | UI response |
|---|---|
| `PendingGates` RPC fails outright | Same `InlineError` transient-with-Retry component already used elsewhere on this page (`BacklogItemDetail.tsx`'s existing pattern) — not a new error component. |
| Multiple gates blocking simultaneously | Each gets its own independent row and status — never collapsed into one generic "Blocked" state (research/ux.md §4's explicit, non-negotiable finding). |
| A gate's config references a deleted pipeline mode / removed skill | `⚠ Configuration error` row, exact wording above — never silently passes (would let an unreviewed item through) and never silently blocks with no explanation (the exact opacity the whole project exists to fix). |
| Approve/Reject clicked twice quickly (double-submit) | Buttons disable immediately on first click (`isPending` local state, same idiom `GateVerdictBox.tsx` already uses for its own action buttons); a second click before the RPC resolves is a no-op, not a duplicate `GateSatisfactionRecord` (also enforced server-side by the `UNIQUE(item_id, gate_id)` constraint, so this is defense in depth, not the only guard). |
| No pending transition at all (item is mid-stage, nothing gated right now) | Section does not render at all — no empty "0 gates" box cluttering item detail for the common case. |

### Accessibility verification

- Each row is a static list item (`<li>`) inside a `<ul role="list" aria-label="Pending
  gates">`; the **list container**, not each row, carries `aria-live="polite"`, so a
  status change (e.g., approval recorded) is announced once as a summary
  (`"Human approval satisfied. 1 gate remaining."`) rather than firing a live-region
  event per row on every render — avoids the over-announcement research literature
  flags for per-item `aria-live` in a list (this is a real, reasoned choice, not a
  copy of `GateVerdictBox.tsx`'s single-card `aria-live`, which doesn't face this
  multi-row problem).
- `[Approve]`/`[Reject]` buttons carry `aria-label="Approve human-approval gate for
  transition Review to Design Review"` (not just "Approve") so a screen-reader user
  navigating by button list, out of row context, still gets an unambiguous label.
- Status icons (✓/✗/○/⚠) are always paired with the text label already shown in the
  wireframe (`"Satisfied"`, `"Blocked"`, `"Pending"`, `"Configuration error"`) and
  marked `aria-hidden="true"` themselves — color/glyph is never the only signal.
- Reuses `vars.color.success`/`warning`/`error`/`errorText` tokens exactly as
  `GateVerdictBox.css.ts` already does (see that file's `verdictLabelFail: {color:
  vars.color.errorText}` vs. `verdictIconFail: {color: vars.color.error}` — two
  distinct tokens for icon vs. text specifically because they need different contrast
  treatment against the same background). Reusing this pair, rather than a single
  color for both, is the concrete mechanism the existing codebase already uses to
  keep gate-status text at AA contrast — not a new claim, an inherited one.

---

## Surface 5: BacklogBoard / StageTracker dynamic rendering (condensed)

This is an extension of an already-interactive, already-accessible surface (the board
supports drag/click today) rather than a new interaction model, so it gets a lighter
treatment focused on the one new state the plan calls out: an unrecognized stage.

```
┌ Idea │ Ready │ ... │ Design Review │ ⚠ Unrecognized stage (1) ┐
│      │       │     │  [card]       │  [card: "some-deleted-  │
│      │       │     │               │   stage" — click for    │
│      │       │     │               │   detail]                │
└──────┴───────┴─────┴───────────────┴──────────────────────────┘
```

- `COLUMNS` becomes a live-fetched, cached stage list (`useBacklogStages()`) instead of
  the hardcoded 5-entry array; a configured custom stage with ≥1 item renders as its own
  column, matching its `Name`.
- Any item whose `status` matches no fetched stage renders in a trailing **"Unrecognized
  stage"** overflow column — never dropped (this is BUG-037's exact failure mode, now
  guarded a third time per research/ux.md §4). Clicking that card opens item detail,
  which shows Surface 4's "stage no longer in the current configuration" notice.
- `StageTracker.tsx`'s `deriveStageDisplay` keeps its existing `default:` dimmed-fallback
  shape (research/ux.md §4) — now driven by the fetched stage list instead of a
  hardcoded switch, with no change to the fallback's visual treatment.

### UX acceptance criteria for this surface

- A custom stage with 1 live item appears as its own board column within one
  `listStages()` cache refresh cycle of being created — no page reload required.
- An item on an unrecognized stage ID is visible on the board in 100% of cases (this is
  a hard regression gate, not a "usually" — BUG-037 already happened once for a
  different root cause).

---

## Condensed non-interactive surfaces

### A. Phase 1 Liveness CRUD (RPC-only, no UI)

```
# Representative operator call (documented in deploy notes, Story 1.5.2a)
grpcurl -d '{
  "stage_slug": "idea",
  "pipeline_mode": "sdd",
  "kind": "LIVENESS_KIND_DURATION_BUDGET",
  "expected_duration_ms": 2700000,
  "staleness_margin_ms": 600000
}' localhost:8543 session.v1.BacklogService/CreateLivenessDefinition
```

**Acceptance criteria**:
- Creating a duplicate `(stage_slug, pipeline_mode)` pair returns `CodeAlreadyExists`
  with a message naming both fields, not a generic "invalid argument."
- `UpdateLivenessDefinition`'s response reflects the new value immediately on the next
  `LivenessFor` call (cache invalidation verified, not assumed) — Story 1.3.2's own AC.
- A malformed kind/field combination (e.g. `LIVENESS_KIND_HEARTBEAT` with
  `expected_duration_ms` set) is rejected by the same validating constructor Story
  1.1.1 defines — the RPC layer does not re-implement this validation separately.

### B. Structured log output

```
INFO [LivenessEngine] resolved liveness for stage=idea mode=sdd kind=duration_budget expected=45m margin=10m
WARN [LivenessEngine] stage=custom-typo-stage mode=sdd liveness config unresolved, falling back to default (35m)
WARN [ConfiguredWorkflowEngine] gate abc123 on transition review->design-review unresolved, blocking transition
DEBUG [ConfiguredWorkflowEngine] cache refreshed: 11 stages, 14 transitions, 6 gates
```

**Acceptance criteria**:
- Every fallback (liveness Warn) and every gate-block (gates Warn) names the specific
  stage/mode/transition/gate ID involved — never a bare "resolution failed."
- The two Warn lines' fallback directions are opposite and both explicit in the log
  text itself (liveness falls back to a *value*; gates fall back to *blocking*) — an
  operator reading only the log, with no UI open, can tell which happened.
- Exactly one Warn line per unresolved call (not per internal retry) — Task 1.6.1b's
  own AC.

### C. Graph-validation RPC contract

```json
// CreateStageTransition response, warning (non-blocking) case
{
  "transition": { "fromStage": "in_progress", "toStage": "review", ... },
  "warnings": [
    "Cycle detected (in_progress -> review -> design_review -> in_progress) with no gates on any edge in the cycle."
  ]
}
```

**Acceptance criteria**:
- A hard-invalid graph (unreachable stage, non-terminal dead end) returns
  `CodeInvalidArgument` and persists nothing — verified by a follow-up `ListStages`
  call showing no partial write.
- A soft warning (gate-free cycle) returns success *and* a non-empty `warnings` array
  in the same response — never silently dropped, never silently blocking.

---

## UX acceptance criteria (cross-surface)

**Task efficiency**
1. An operator can create a new custom stage with zero transitions in ≤ 4 form
   interactions (open New Stage, fill slug, fill name, Save) — no required field
   beyond slug/name.
2. An operator can add one gated transition to an existing stage in ≤ 6 interactions
   from the list page (Edit → Add transition → select target → check one gate
   checkbox → fill its one required sub-field → Save).
3. An operator can identify *which specific gate* is blocking a given item's next
   transition in ≤ 1 click from the board (click card → item detail → gate checklist
   is already rendered, no additional expand/tab click required).

**Error handling**
4. Every error state in this document names the specific offending entity (a stage
   slug, a gate ID, a transition pair) — never a bare "something went wrong."
5. Every error state offers a next action: Retry, Fix in Stages settings, Cancel, or
   (for the two-step delete) Never mind — **no dead ends**. Verified per surface: Table
   in Surface 1 (Retry), Surface 2 (fix-in-place + Cancel), Surface 4 (Retry / Fix in
   Stages settings link).
6. A save that produces a structural violation (unreachable stage) is rejected before
   commit; a save that produces a soft risk (gate-free cycle) commits but is
   acknowledged, never silently absorbed — matches the fail-closed-for-structure /
   warn-for-risk asymmetry in `implementation/plan.md`'s Pattern Decisions table.

**Accessibility**
7. Every interactive control across Surfaces 1, 2, 4, 5 is operable by keyboard alone,
   with no drag-based interaction anywhere in the editing path — verified by
   construction (native `<button>`/`<select>`/`<input>` elements throughout; the one
   non-interactive graph diagram is `aria-hidden` and carries a table-based text
   equivalent, not a second keyboard path to the same functionality, because it is
   never itself an edit surface).
8. Every form control has a programmatically associated label (`<label htmlFor>` /
   `aria-label`) — no placeholder-as-label anywhere (matches `PipelineModeForm.tsx`'s
   existing `htmlFor`/`id` pairing convention throughout).
9. Every status/error color pairing reuses this app's existing `vars.color.success` /
   `warning` / `error` / `errorText` design tokens rather than introducing new colors —
   inherits, rather than re-asserts, this codebase's existing AA-contrast treatment
   (evidenced by `GateVerdictBox.css.ts` already maintaining a separate `error` vs.
   `errorText` token pair for icon-vs-text contrast on the identical PASS/FAIL/PENDING
   pattern this design generalizes).
10. The read-only graph diagram's information is fully available to a screen-reader
    user via the paired `sr-only` table — verified row-for-row equivalence (every SVG
    edge has exactly one corresponding table row), not just an overall summary label.
11. No `aria-live` region fires more than once per user-initiated action (Surface 4's
    list-level, not row-level, `aria-live` placement) — avoids the over-announcement
    failure mode a naive per-row live-region design would introduce.

---

## Summary of files referenced (precedent, read in full or in relevant part)

- `web-app/src/app/settings/pipeline-modes/page.tsx`, `PipelineModeForm.tsx` — list +
  form CRUD pattern, slug immutability, two-step delete, inline `role="alert"` errors.
- `web-app/src/components/backlog/GateVerdictBox.tsx` (+ `.css.ts`) — verdict-card
  pattern, action-pending/error-recovery idiom, `error`/`errorText` token pairing.
- `web-app/src/components/backlog/detail/StageTracker.tsx` — `deriveStageDisplay`'s
  `default:` dimmed-fallback shape for an unrecognized status.
- `web-app/src/components/backlog/BacklogBoard.tsx` (lines 14–58) — hardcoded `COLUMNS`
  and the BUG-037 comment this design's Surface 5 extends.
