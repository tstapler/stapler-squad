# UX Research: backlog-custom-workflow-stages

Research Agent 5 (UX) — SDD Phase 2

## 0. Naming collision found before anything else (must inform every subsequent decision)

**"Workflow" is already a taken, unrelated term in this codebase's UI.** Two existing surfaces use
it for a different concept than this project's "custom workflow stages":

- `web-app/src/app/workflows/page.tsx` + `web-app/src/components/workflows/WorkflowsPanel.tsx` /
  `WorkflowForm.tsx` — a **cron-scheduled automation routine** editor (backed by the
  `mcp__stapler-squad__create_workflow`/`run_workflow`/`list_workflows` RPCs), unrelated to the
  backlog status state machine.
- `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx` — the existing **status-transition
  audit trail** on item detail, titled "Workflow" in its `CollapsibleSection` header.

This project's `ConfiguredWorkflowEngine` (the Go-level name, fine to keep internally, mirroring
ADR-013) must **not** surface as "Workflow(s)" in any nav label, page title, or button the operator
sees — a user who already knows `/workflows` as "my cron routines" and "Workflow" as "the status
history list" will misread a third meaning under the same word. Recommend **"Stages"** or **"Pipeline
Stages"** as the UI-facing term (e.g. a settings page titled "Backlog Stages," matching the sibling
`pipeline-modes` page's own naming register), reserving "workflow" for Go/ADR-013 internal vocabulary
only. This is a cheap fix to make now, before Phase 3 planning starts writing component names.

## 1. Comparable UX patterns for defining a graph of states + transitions

Reviewed against four reference products (general domain knowledge, not sourced from a specific doc
this session — flagged INFERRED where not grounded in this repo):

- **Jira workflow scheme editor**: a true node-and-edge canvas (drag states, draw transition arrows),
  with a separate properties panel per selected transition holding tabs for conditions/validators/
  post-functions. Known failure mode (widely reported, and observable in Jira Cloud today): the canvas
  is unreadable past ~8–10 states — edges cross, and small transition labels are illegible until
  zoomed. Jira also **splits the same conceptual object across two disconnected editors** (the visual
  diagram vs. a flat "Transitions" table used for actually attaching conditions/validators) — users
  routinely report not realizing a transition's gate config lives in a different view than the arrow
  that represents it.
- **GitHub branch protection rules**: no canvas at all — a single rule is a **flat checklist of
  composable requirements** (required reviewers count, required status checks by name, "require
  branches to be up to date," etc.) attached to one target (a branch pattern), not a graph. This is
  the closest existing precedent to this project's "gate" concept: multiple independently-satisfiable
  requirements gating one transition, rendered as a checklist with each item showing its own
  pass/fail/pending state — not a graph node.
- **Linear / Trello / GitHub Projects custom board columns**: **flat, ordered list** of columns
  (add/rename/reorder/delete), no arrows, no transition-specific conditions — moving a card between
  any two columns is always allowed, with automation (if any) triggered by column entry, not gated
  by explicit prerequisites. This is a materially simpler model than what this project needs (it lacks
  gates entirely) but is the most learnable, lowest-effort-to-build reference point if the appetite
  turns out smaller than "Large."
- **Temporal / Camunda visual workflow designers**: BPMN-style canvases (Camorama/Camunda Modeler,
  Temporal's newer visual tooling) are full node-and-edge graph editors aimed at engineers, not a
  single internal operator — they carry substantial learning curves (BPMN symbol vocabulary: gateways,
  events, boundary events) that would be significant over-engineering for this project's stated
  single-operator, ~8-built-in-plus-a-handful-of-custom-stages scale.

**What goes wrong, synthesized across all four**: (1) a canvas layout that must be manually
arranged/dragged imposes cognitive+motor cost disproportionate to the actual complexity once there are
more than ~6–8 nodes — this project's built-in set alone is already 9 states; (2) splitting "the shape
of the graph" from "the conditions on an edge" into two views (Jira's failure mode) causes users to
forget a gate exists because they aren't looking at the view that shows it; (3) validation errors that
only fire at save time ("transition X has no valid gate configuration") rather than inline, are the
single most commonly cited workflow-editor complaint (again, INFERRED from general product-review
literature, not a source opened this session) — silent misconfiguration (a transition technically
saved but unreachable, or a gate type selected with no backing config filled in) is worse than a
blocking validation error, because it fails at runtime for the operator's *next* item instead of at
edit time.

**Recommendation for this project's scale (9 built-in + likely single-digit custom stages, single
operator, appetite explicitly flagged as "real, substantial UI work" in requirements.md's Rabbit
Holes)**: **do not build a drag-and-drop canvas.** Use a **list-of-stages + list-of-transitions-per-
stage** structure instead — this is directly analogous to how `PipelineModeForm.tsx` already handles
its own "9 named fields, one CRUD form" structure (see §2), and it sidesteps the graph-layout-a11y
problem in §3 entirely by never requiring spatial/canvas interaction to define the graph's shape. A
graph is still rendered — but as a **read-only visualization** derived from the list data (e.g. an SVG
computed layout, or `mermaid-diagrams`-style rendering), not as the editing surface itself. This
mirrors GitHub branch protection's flat-checklist gate model (§ above) combined with Trello/Linear's
flat column-list model for stage definition, while keeping Jira's per-transition conditions concept —
i.e., take the good parts of three simpler products rather than the graph-canvas part of the two
complex ones.

## 2. User mental model — extending the existing PipelineMode precedent, not inventing a new paradigm

Read in full: `web-app/src/app/settings/pipeline-modes/page.tsx`,
`web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`, and
`project_plans/backlog-configurable-pipeline/research/ux.md`. The established pattern this project
should extend:

- **List page + slide-in form overlay**, not a wizard or multi-step flow: `page.tsx` renders a flat
  `styles.list` of rows (name, slug, enabled badge, toggle switch, Edit button, "New Mode" button in
  the header), and clicking New/Edit opens `PipelineModeForm` in a `styles.formOverlay` — the list
  stays visible/dimmed behind it. This is a **CRUD-list-plus-form** pattern, the same one nearly every
  settings page in this app uses (`backlog-sources`, `remotes`).
- **Slug is immutable after creation** (`PipelineModeForm.tsx:161-175`, `disabled={Boolean(mode)}`)
  — same constraint should apply to a stage's identifier once items can reference it, for the same
  reason (referential stability for anything that stored the old slug).
- **Enabled is a `role="switch"` toggle, separate from delete** (`page.tsx:162-170`) — disabling a
  stage without deleting it is the safe default action; this maps directly onto this project's Rabbit
  Hole about editing/deleting a stage/gate while items are actively on it (§4 below) — "disable, don't
  delete" is already the established UI affordance for exactly this class of problem in the sibling
  project, and should be the primary discouragement against destructive edits here too.
- **Delete requires a two-step confirm-in-place** (`PipelineModeForm.tsx:250-283`, "Delete" →
  "Confirm delete?" / "Never mind" swap, no modal dialog) — reuse this exact pattern rather than a
  browser `confirm()` or a separate modal.
- **Inline error banner, not toast** (`role="alert"`, `PipelineModeForm.tsx:154-158`) surfaces
  create/update `ConnectError` messages (e.g. `CodeInvalidArgument`) without losing in-progress form
  state — critical for this project since a transition/gate config is more likely to trip a validation
  rule (e.g. "gate references a stage that doesn't exist," "transition creates an unreachable state")
  than a pipeline mode's freeform text fields ever would.
- **`BacklogItemForm.tsx`'s pipeline-mode field** (lines 102–253) is the second half of the precedent:
  a **`RadioGroup<T>`** (the shared, already-extracted `web-app/src/components/ui/RadioGroup.tsx`,
  generalized out of `OmnibarCreationPanel.tsx`'s `SessionTypeRadioGroup` per the sibling project's own
  ux.md recommendation — confirms that extraction happened) built from `pipelineModeOptions` fetched
  live via `listPipelineModes()`, with an explicit **unresolved-value fallback**
  (`unresolvedPipelineMode`, lines 242–251: if the stored value doesn't match any currently-loaded
  option, it's flagged rather than silently rendering nothing). **This exact fallback shape is the
  direct precedent to reuse for "item is on a stage nobody else has seen"** (§4) — the sibling project
  already solved the "stored value not in current option list" problem once; this project should call
  the same pattern, not re-derive it.

**Extension point, not new paradigm**: A new "Backlog Stages" settings page (`/settings/backlog-
stages` or similar — see §0 on naming) should look and behave like `pipeline-modes/page.tsx` at the
list level (rows = stages, toggle = enabled/disabled, Edit → form overlay). The graph-shaped part
(transitions + gates) is the one genuinely new interaction, and per §1 above should be a **second-level
list nested inside a stage's edit form** — "Outgoing transitions" as a sub-list of
`{ targetStage, gates[] }` rows within `PipelineModeForm`-equivalent for stages — rather than a
separate canvas screen. A user who already learned "click Edit → a form opens with fields and a
Delete button at the bottom" from the pipeline-modes page should find the stages page structurally
identical, with one additional collapsible section ("Transitions") inside each stage's form.

## 3. Accessibility for a graph/node-and-edge editor — and the simpler alternative that avoids the problem

**Bottom line first**: a full drag-and-drop canvas graph editor is a well-documented a11y rabbit hole
(WCAG 2.1's Pointer Gestures 2.5.1 / Dragging Movements 2.5.7 both apply directly to a canvas requiring
drag-to-connect-nodes, and neither is trivially satisfiable without also building a full keyboard-
operable alternative *path* through the same functionality — not just keyboard-focusable nodes, but an
equivalent way to create/move/delete an edge without a pointer). Given §1's finding that a canvas isn't
even the *appetite-appropriate* editing surface for this project's scale, the a11y question mostly
resolves itself: **build the list-based editor from §1/§2, not a canvas, and the WCAG story becomes
"a form," which this codebase already knows how to do accessibly** (see `pipeline-modes` and
`BacklogItemForm.tsx` precedent — labeled inputs, `role="alert"` errors, `data-testid` per
interactive element per `.claude/rules/e2e-test-conventions.md`... actually a skill, not a rule file;
see `e2e-test-conventions` skill).

Concrete requirements for the list-based transition/gate editor, building on the `RadioGroup`/
`PipelineModeForm` precedent:

1. **Each stage's "Outgoing transitions" sub-list is a standard list of rows**, each row a target-stage
   `<select>` (not a second radiogroup — a `<select>` is appropriate here per the sibling ux.md's own
   stated threshold, ">5-6 options," since the choice is "one of N existing stages," a value-lookup
   task, not a small named-preset choice) plus an expandable "Gates" area. Add/remove-transition uses
   plain "+ Add transition" / per-row "Remove" buttons — fully keyboard-operable with no drag at all.
2. **Gate attachment within a transition row is a checkbox-group-with-detail-panel**, mirroring GitHub
   branch protection's own accessible pattern (a checklist, not a canvas): each gate type (human
   approval / automated review / structural check / custom) is a checkbox; checking it reveals type-
   specific fields inline below (e.g. automated review → a `<select>` of which review prompt/pipeline
   mode; structural → a `<select>` of which precondition). This is the `SESSION_TYPES`-style
   progressive-disclosure pattern already used in `OmnibarCreationPanel.tsx` (advanced options revealed
   behind a toggle), applied to gates instead of session types.
3. **The read-only graph visualization** (§1's "still show a graph, just not as the editing surface")
   needs its own, lower a11y bar since it's not interactive: render it as an SVG or `mermaid-diagrams`
   diagram with a **parallel text-equivalent** — e.g. a visually-hidden (`sr-only`, not `display:none`,
   so it's still in the accessibility tree) `<table>` or definition list enumerating "From → To (gates:
   N)" rows, so a screen-reader user gets the same information a sighted user gets from the diagram
   without needing to parse SVG. This is the standard WCAG pattern for a complex non-interactive
   visualization (equivalent to a chart's data-table fallback) and avoids inventing ARIA graph
   semantics that have poor screen-reader support in practice (`role="graphics-document"` /
   `graphics-object` /`graphics-symbol` from the SVG-AAM/Graphics ARIA modules have historically weak,
   inconsistent support — INFERRED from general a11y-tooling knowledge, not verified this session;
   flag for a spike if Phase 3 planning wants the diagram to be interactive rather than purely
   illustrative).
4. **`data-testid` and `role="alert"` conventions** carry over unchanged from `pipeline-modes` — no new
   a11y pattern needed for the CRUD chrome itself, only for the transitions/gates sub-editor described
   above.
5. If Phase 3 planning still wants an optional visual canvas as a *progressive enhancement* on top of
   the accessible list editor (not a replacement for it) — e.g. for the "read-only graph visualization"
   in point 3 to later become click-to-jump-to-that-transition's-row-in-the-list — that is a
   reasonable v2, but the list editor must be feature-complete and independently sufficient first; the
   canvas must never be the only way to create or edit a transition or gate.

## 4. Error states and edge cases

- **A transition is blocked by multiple gates of different types simultaneously** (e.g. human approval
  pending AND automated review not yet run): follow `GateVerdictBox.tsx`'s existing verdict-card
  pattern (`role="status" aria-live="polite"`, lines 239–242; a `VERDICT_CONFIG` map keyed by
  PASS/PARTIAL/FAIL/PENDING/UNVERIFIABLE, each rendering an icon + label + summary + optional criteria
  list) — but generalized to **a checklist of N independent gate rows**, not one verdict for the whole
  transition, since GitHub branch protection's own UI (§1) shows this is the legible way to present
  "multiple, independently-satisfiable requirements": each gate gets its own row with its own status
  (Satisfied / Pending / Blocked / N/A) and, critically, **who/what can satisfy it** next to the status
  — "Automated review: pending (will run automatically)" vs. "Human approval: pending — click Approve
  below" vs. "Structural check: blocked — 2 of 5 acceptance criteria incomplete." This directly answers
  requirements.md's Success Metric ("the item detail UI shows... which gate(s) are blocking it and
  who/what can satisfy each one") — the "who/what" column is not optional framing, it's the specific
  thing that was asked for and that GitHub's own equivalent UI gets right by always naming the
  unsatisfied requirement's owner (a specific check name, a specific required-reviewer count) rather
  than a generic "blocked" state.
- **An item sits on a custom stage no other operator/viewer has ever seen** (stage created, used on one
  item, then the browser loading `BacklogBoard.tsx` predates that stage's creation, or the config
  fetch races the item fetch): this is structurally the same "stored value not in the current options
  list" problem `BacklogItemForm.tsx` already solved for pipeline modes
  (`unresolvedPipelineMod e`, lines 242–251) and `StageTracker.tsx`'s `deriveStageDisplay` already
  solved defensively for *status* (`default:` case, lines ~52-56: unknown/future status renders as a
  dimmed "Archived"-styled fallback rather than crashing a lookup) — **reuse both patterns, don't
  invent a third**: `BacklogBoard.tsx`'s `COLUMNS` (currently hardcoded to 5 fixed stages, per
  `BacklogBoard.tsx:52-58`) must fetch the live stage set (same caching precedent as `PipelineEngine`,
  per requirements.md's NFRs) and render an extra "Unrecognized stage" column (or fold unrecognized
  stages into a single overflow column) for any stage ID present on a live item but absent from the
  fetched config — never silently drop the item off the board (this is exactly BUG-037, cited in
  `BacklogBoard.tsx`'s own comment at lines 14-26, which happened once already for a *different*
  reason — status/stage mismatch — and must not be allowed to recur for a *third* reason, unrecognized
  custom stage).
- **A custom stage or gate is edited/deleted while items are actively on it**: per §2, "disable, don't
  hard-delete" is the existing UI affordance (`pipeline-modes`' enabled toggle) and should be the
  default recommendation surfaced *in the UI itself* — e.g. the Delete button for a stage with ≥1 live
  item on it should be disabled with a `title` tooltip ("3 items are on this stage — disable it
  instead, or move them off first"), reusing the exact `disabled` + `title` tooltip pattern the sibling
  project's own ux.md documents from `BacklogItemDetail.tsx`'s Trigger Triage button
  (`disabled={...} title="Set repository path first"`). If the backend allows the delete anyway (e.g.
  an operator deletes via direct API access, or a race), the fail-closed behavior requirements.md's
  Risk Control section already mandates (fall back to default built-in behavior, loud Warn log) is the
  correct backend contract — the UI-side complement is: an item detail page for an item on a
  since-deleted stage must render the same "Unrecognized stage" fallback state described above, not
  crash or show a blank state. **Editing a stage's liveness parameters while items are on it** is
  lower-risk (no referential integrity break, just a threshold change) but should still be visible —
  e.g. a "3 active items will be affected by this liveness change" hint in the edit form, so an
  operator doesn't accidentally tighten a timeout on in-flight work without realizing it.
- **A transition or gate config that was valid at save time becomes invalid later** (e.g. a gate
  referencing a review prompt/pipeline mode that's since been deleted): same "fail closed and loud"
  contract, surfaced to the operator via the item detail gate-checklist row rendering as
  "Configuration error — this gate can't be evaluated (referenced pipeline mode not found)" rather than
  either silently passing (dangerous — a broken gate that "passes" moves items through unreviewed) or
  silently blocking forever with no explanation (the exact opacity problem this whole project exists to
  fix, per requirements.md's motivating bug).

## 5. Jobs-to-be-done — which gate type earns the most polished treatment

- **Functional job**: "Make sure the right checks actually ran before this item moves forward, without
  me having to remember to check them by hand every time" — this is the direct extension of the
  motivating bug (a timeout nobody could see coming) into the gate model: the functional core is
  *visibility and enforcement of a precondition*, not the precondition's specific type.
- **Emotional job — the dominant one, same conclusion the sibling project's ux.md already reached for
  pipeline modes and directly reinforced by requirements.md's own framing here**: **trust, specifically
  the fear of a silent, structural failure the operator didn't know to look for** — not (primarily) "I
  want to feel confident before something ships" in the sense of a deliberate human sign-off ritual.
  The evidence for this ranking, specific to this project (not just inherited from the sibling): the
  entire motivating bug in requirements.md is a **structural/mechanical failure** (a call-budget vs.
  staleness-sweep mismatch), not a case where a human approval step was skipped or a review was
  rubber-stamped. Twelve items got silently stuck with **no UI signal at all** until someone went
  looking. That is squarely "I want automation to make fewer mistakes about when things are done" —
  the job is *structural integrity of the state machine itself*, and only secondarily "I want to feel
  good approving this by hand."
  - This ranking has a direct design consequence for which gate type gets the most polished UI: the
    **structural/mechanical check gate** and the **liveness/staleness surfacing** (the "what's blocking
    this and why has it been stuck" view) should get first-class, prominent treatment — a clearly
    visible "why is this stuck" panel on any item whose current stage has exceeded its liveness
    threshold, shown *before* any gate-checklist detail, not buried under it. The **human-approval
    gate** is real and requested (Success Metrics explicitly call for it as the example gate type to
    verify against) and should get the same checklist-row treatment as any other gate (§4), but does
    not need bespoke polish beyond a clear Approve/Reject affordance — it is functionally close to
    today's existing "Approve Plan" button, which already exists and already works; the *novel* risk
    this project is solving for is the structural one, not the human-approval one.
  - Practical framing for Phase 3 prioritization if the appetite needs trimming: if any gate type has
    to ship first or get disproportionate design investment, it should be the **liveness/staleness
    view** (arguably not a "gate" at all in the Actors taxonomy, but the visibility surface for *why* a
    transition hasn't happened) and the **structural/mechanical check gate**, not automated-review or
    human-approval — those two already have working precedent (`GateVerdictBox.tsx`,
    "Approve Plan") to extend, while the structural/liveness story is being built from nothing.
- **Social job**: N/A, confirmed single-operator tool (requirements.md's Users section, "single
  operator, per `project_plans/backlog-configurable-pipeline/`'s established threat model") — no
  audience to justify a gate configuration to, consistent with the sibling project's own conclusion.

## Key files referenced

- `web-app/src/app/settings/pipeline-modes/page.tsx`, `PipelineModeForm.tsx` — CRUD-list-plus-form
  pattern, enabled-toggle/disable-don't-delete, two-step confirm-delete, inline `role="alert"` errors.
- `web-app/src/components/backlog/BacklogItemForm.tsx` (lines 102–253) — live-fetched `RadioGroup`
  options with `unresolvedPipelineMode` stale-reference fallback; direct precedent for §4's
  unrecognized-stage handling.
- `web-app/src/components/ui/RadioGroup.tsx` (+ `.css.ts`, `.test.tsx`) — the shared, already-
  generalized radiogroup component (confirms the sibling project's own recommendation to extract it
  from `OmnibarCreationPanel.tsx` was followed).
- `web-app/src/components/backlog/BacklogBoard.tsx` (lines 14–58) — hardcoded 5-column `COLUMNS`
  array and its `stageOf`/`deriveStageDisplay` mapping; the BUG-037 comment is the concrete precedent
  for "never silently drop an item off the board."
- `web-app/src/components/backlog/detail/StageTracker.tsx` — `deriveStageDisplay`'s `default:` fallback
  for an unrecognized status; reuse this defensive shape for unrecognized custom stages.
- `web-app/src/components/backlog/GateVerdictBox.tsx` (lines 239–340) — verdict-card pattern
  (`role="status" aria-live="polite"`, `VERDICT_CONFIG` map) to generalize into a multi-gate checklist.
- `web-app/src/app/workflows/`, `web-app/src/components/workflows/`,
  `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx` — **naming collision**: "Workflow"
  already means cron-scheduled automation routines and the status-history audit trail in this app's
  UI; do not reuse the term for this project's stage/transition editor (§0).
- `session/workflow_engine.go` (lines 7–13) — the narrow 3-method `WorkflowEngine` interface this
  project's UI ultimately drives (`CanTransition`, `ValidateGates`, `AllowedTransitions`); no liveness
  concept exists in it yet, confirming requirements.md's Feasibility Risks framing.
- `session/review_gate.go` — existing PASS/FAIL/UNVERIFIABLE automated-review flow
  (`ReviewGateRunner.Run`), the reusable shape for a custom transition's automated-review gate.
- `project_plans/backlog-configurable-pipeline/research/ux.md` — prior UX research this document
  extends rather than duplicates; read in full before writing the above.
