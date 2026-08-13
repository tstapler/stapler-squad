# UX Research: backlog-configurable-pipeline

Research Agent 5 (UX) — SDD Phase 2

## 1. Comparable UX patterns

**Named preset/mode selection precedent already exists in this codebase and should be reused, not reinvented.**

`web-app/src/components/sessions/OmnibarCreationPanel.tsx` (`SESSION_TYPES`, lines 26–57) solves
almost exactly this problem: a small, named, extensible set of modes, each with a `label` and a longer
`description` used as contextual hint text. The implementation:

- `SESSION_TYPES` is a `const` array of `{ value, label, description }` objects — not a boolean, not
  a raw enum switch scattered through the JSX. Adding a mode is a one-entry array edit.
- Rendered as a **hand-rolled ARIA radio group** (`role="radiogroup"` on the container,
  `role="radio"` + `aria-checked` on each `<button type="button">`), not native `<input type="radio">`
  elements and not a `<select>`. See lines 103–151.
- Roving tabindex: only the checked item (or the first item, if nothing is checked yet) has
  `tabIndex={0}`; all others are `tabIndex={-1}`. Arrow keys (`ArrowRight`/`ArrowDown`/`ArrowLeft`/
  `ArrowUp`) move selection and call `onChange` directly (`handleKeyDown`, lines 90–101) — this is the
  correct native radio-group keyboard contract (arrow keys *change* the selection immediately, Tab
  moves focus off the group).
- Progressive disclosure: `PRIMARY_TYPES` (3 items) shown by default, `ADVANCED_TYPES` (2 items) behind
  a "▾ More" toggle button that is deliberately *not* part of the radio semantics (`tabIndex={-1}`,
  no `role="radio"`) so screen readers don't announce it as a 6th option.
- Hint/description text lives in a **separate `<span className={hint}>` outside the radio group**,
  looked up by `SESSION_TYPES.find(t => t.value === selected)?.description` (lines 496–502) — the
  description updates live as the user changes selection, and is NOT wired to
  `aria-describedby` on the radiogroup (a gap worth fixing for the new selector, see §3).

**Recommendation**: build `PIPELINE_MODES` as a `{ value, label, description }[]` const, following the
`SESSION_TYPES` shape and the same hand-rolled radiogroup component (or extract
`SessionTypeRadioGroup` into a shared generic `RadioGroup` component parameterized by an options array
— it currently has zero session-specific logic baked into its rendering, only the `SessionTypeValue`
type param, so generalizing costs little and avoids drift between two near-identical
implementations). A `<select>` dropdown (the pattern used for `Priority` in `BacklogItemForm.tsx`,
lines 192–210) is the fallback if the mode count grows large (>5–6) or screen space is tight, but for
an initial set like `default`/`quick`/`full` a radio group is superior because:
- All options are visible without opening/interacting (recognition over recall) — important since the
  user is choosing among *behaviors*, not values they already know the name of.
- Per-option description text can render inline/adjacent, whereas a `<select>` needs a separate
  `title` attribute or JS-driven hint swap on `change` (extra indirection, easy to forget to wire up
  ARIA-wise).
- A segmented control (visually similar to a radio group but styled as adjoining buttons) is a viable
  visual treatment of the *same* semantics already implemented — no new pattern needed, just CSS. Do
  not introduce a third distinct interaction pattern (e.g. a custom dropdown-with-cards or a modal
  picker) — the codebase already has exactly one blessed pattern for this exact problem shape.

## 2. User mental models

The existing three checkboxes (`skipPlanning`, `skipReviewGate`, `autoSpawnSession`, all in
`BacklogItemForm.tsx` lines 214–268) are **independent boolean toggles**, each with its own
`fieldGroup` + `checkboxHint` line, laid out in a `styles.twoColumn` grid. A user who has already
learned "check a box, read the hint below it" will map a new *named-mode* control onto a different
mental model — "pick one of N presets" rather than "flip N independent switches" — so the two
controls read as structurally different even though both configure the pipeline. That's fine and
expected (mode ≠ boolean), but it creates a **composition ambiguity** the UI must resolve explicitly:

- **Does selecting `"quick"` mode implicitly set `skipPlanning=true`?** If pipeline mode is a coarse
  preset and the three checkboxes are fine-grained overrides, the user needs to see the checkboxes
  *reflect* the mode's implied settings (e.g. pre-check/gray-out `skipPlanning` when `quick` is
  selected, with a "(set by Quick mode)" annotation) — otherwise a user who picks `"quick"` and then
  is surprised triage still ran a planning pass will lose trust in the control. Silent composition
  (mode sets defaults but checkboxes remain independently editable with no visual link) is the most
  dangerous option from a mental-model standpoint: it looks like 4 unrelated toggles even though 3 of
  them are semantically subordinate to the 4th.
  - **Recommended pattern**: mode is the primary/first control; the three checkboxes render inside a
    visually grouped sub-section labeled something like "Overrides" that is collapsed/secondary unless
    a value diverges from the mode's default. This avoids "6 independent unrelated toggles" perception
    (the requirement's own words) while keeping escape-hatch granularity power users may want. Do NOT
    remove the checkboxes — requirements.md scope doesn't authorize deleting existing fields, and
    `skipTriage` (vagueness-derived, not a form field) already composes with them, so a fourth
    independent axis is already a precedent in this exact form.
- Given the field is likely optional with a sane default (`"default"` mode preserving today's
  hardcoded pipeline), most users will never touch it — same usage pattern as `autoSpawnSession`
  today (off by default, discoverable via hint text, not requiring a decision on every item).

## 3. Accessibility — concrete requirements for the recommended radio-group pattern

Building directly on `SessionTypeRadioGroup`'s implementation, with two concrete fixes for gaps
observed in that reference implementation:

1. **`role="radiogroup"` container + `role="radio"` buttons + `aria-checked`** (not `aria-selected` —
   that's for listbox/tab semantics, a common mis-application). Copy the existing pattern exactly;
   do not use native `<input type="radio">` unless also dropping the custom button styling, since
   mixing native radio inputs with custom keyboard handling double-fires events.
2. **`aria-label` or `aria-labelledby` on the radiogroup itself** — `SessionTypeRadioGroup` uses
   `aria-label="Session type"` inline (line 106) OR (in the outer form) a `<label id="omnibar-session-
   type-label">` element with the radiogroup NOT referencing it via `aria-labelledby` (a latent gap:
   the visible `<label>` at line 493 is not programmatically associated with the radiogroup at line
   104–109 — screen reader users get "Session type" only because it happens to read the aria-label
   string, not because of the visible label). **For the new pipeline-mode selector, wire this
   correctly**: either drop the redundant aria-label and use `aria-labelledby="pipeline-mode-label"`
   pointing at the visible `<label>`, or keep both but make sure they say the same thing so there's no
   discrepancy for AT users vs. sighted users.
3. **Roving tabindex + arrow-key cycling** exactly as implemented (lines 90-101, 115, 140) — Tab
   enters/exits the group once; arrow keys move *and select* (per the WAI-ARIA APG radio-group
   pattern, arrow-key movement changes the checked state, unlike a listbox where arrow keys only move
   focus). Do not require Space/Enter to confirm — that's the tablist/listbox pattern, not radiogroup.
4. **Fix the missing `aria-describedby` link this new build should not repeat**: `SessionTypeRadioGroup`
   updates a sibling `<span className={hint}>` with the live description (line 500–502) but never
   associates it with the radiogroup via `aria-describedby`, so a screen reader user tabbing to the
   group hears only the label, not the description, and has to explicitly navigate to the next sibling
   to find it (if they even know to). For pipeline mode, give the hint span a stable `id`
   (e.g. `id="pipeline-mode-hint"`) and set `aria-describedby="pipeline-mode-hint"` on the
   `role="radiogroup"` div so the description is announced automatically on focus.
5. **`data-testid` per option** — required by `.claude/rules/e2e-test-conventions.md` (no CSS-class
   locators). Follow `SESSION_TYPES`/`SessionTypeRadioGroup` convention: none of the radio buttons
   currently carry `data-testid` (only the outer hint span does, `data-testid="omnibar-session-hint"`,
   line 500) — this is itself a minor gap. For pipeline mode, add
   `data-testid={`backlog-pipeline-mode-${type.value}`}` per button so e2e tests can target/assert
   individual mode buttons directly rather than relying on `getByRole("radio", { name })`, which is
   also valid per the ARIA-role-or-testid rule but more brittle if labels change.
6. **The read-only "what ran" surface in `BacklogItemDetail.tsx`** is not interactive, so its a11y
   bar is lower, but it must not be a bare `<div>` of text: use a labeled `role="group"` or a `<dl>`
   (definition list: term = pipeline stage, description = skill/command used) consistent with how
   `GateVerdictBox.tsx` uses `role="status" aria-live="polite"` (line 228–229) for verdict summaries —
   if "what ran" can update live (e.g. while triage is in progress), mirror that `aria-live="polite"`
   treatment so status changes are announced without requiring a page reload or manual re-focus.

## 4. Error states and edge cases needing UX handling

- **Item references a mode that no longer exists** (deleted/renamed in a future config change): the
  read-only "what ran" surface must render historical/frozen data independent of the current
  `PIPELINE_MODES` list — i.e. store the mode's *label and description as they were at run time*, or
  at minimum degrade gracefully to showing the raw stored string (e.g. `"custom (unrecognized mode:
  'legacy-fast')"`) rather than crashing a `.find()` lookup that returns `undefined` (the exact
  `SESSION_TYPES.find(...)?.description` pattern at line 501 would silently render nothing if the
  stored value doesn't match any current entry — for a *live* selector that's a tolerable no-op, but
  for a *historical audit* surface it's a silent data-loss bug users won't notice until they need the
  info). If mode selection is mutable in the create/edit form, the same `.find()` fallback needs an
  explicit "Unknown mode" state there too, so a user editing an old item isn't shown a blank/wrong
  selection.
- **Mode requires something the item lacks** (e.g. `"full"` mode needs `repoPath` set, mirroring the
  existing `Trigger Triage` button's `disabled={actionLoading || !item.repoPath}` +
  `title="Set repository path first"` pattern at lines 805–807). Reuse that exact pattern: disable
  the mode option (or disable the action it gates) with a `title` tooltip and `aria-disabled`, not a
  silent no-op click. If the mode selector itself should never be "disabled" (since it's just a choice,
  not an action), the validation should surface as a warning/hint adjacent to the selector — e.g. "Full
  mode requires a repository path — add one above" — following the same inline-error convention as
  `errors.repoPath` (`role="alert"`, lines 185–189).
- **Item is mid-pipeline when its mode changes** (relevant only if mode is mutable post-creation,
  per requirements.md's open question). If allowed, the "what ran" surface needs to distinguish
  *what was configured* from *what actually executed* — e.g. an item created with `"quick"` mode,
  escalated to `"full"` after a failed review, should show two entries/a timeline
  ("Triage: quick mode → Review: escalated to full mode"), not overwrite history with the current
  mode. This is the single highest-risk gap for the emotional job below (trust) — if changing the mode
  retroactively rewrites what the UI claims happened, an operator auditing a surprising outcome will
  get a plausible-looking but wrong explanation.
- **No mode set / legacy items predating this feature**: must default to `"default"` (today's hardcoded
  pipeline) both in stored data (migration) and in display — never show a blank/undefined mode field on
  old items.

## 5. Job-to-be-done analysis

- **Functional job**: "Get this item through triage → plan → implement → review → merge with the
  right amount of process for its size — trivial items shouldn't wait on steps that don't apply, and
  I shouldn't have to remember to manually skip them every time." This is already partially served by
  the three checkboxes; the named-mode selector's incremental functional value is *reducing the number
  of decisions* for the common case (pick one mode instead of configuring 3 checkboxes correctly) and
  *enabling escalation* ("use `/sdd:full` for this one because it's touching the auth layer") for
  atypical items — i.e. both simplification (quick path) and control (full/heavier path) needs, not
  just one direction.
- **Emotional job**: **trust** — specifically, the fear that a large or risky change silently rides
  through a low-scrutiny pipeline because a mode was misconfigured (or defaulted wrong) and nobody
  noticed until after merge. This is the dominant emotional driver given the single-operator context
  (no social/approval dynamic to catch mistakes — the operator IS the only reviewer of the review
  process itself). The read-only "what ran" surface is *necessary* to serve this job but likely
  **not sufficient** on its own: a passive read-only display only helps if the operator thinks to check
  it. Given the audit finding cited in requirements.md ("no UI element anywhere shows... which
  pipeline/skills ran"), the absence was invisible *by construction* — the same risk applies to a
  purely passive "what ran" panel that requires proactively opening the item detail view. Two low-cost
  mitigations worth flagging to product/planning (outside this agent's scope to decide, but worth
  surfacing): (a) show the pipeline mode as a compact badge/chip on the backlog **list** view (not just
  the detail view) so it's visible during triage without a click-through, and (b) if a review verdict
  or gate failure occurs on an item using a non-default (`quick`/skip-heavy) mode, consider whether
  `GateVerdictBox` should visually flag that the failing review ran under reduced scrutiny — this
  directly targets the "won't get rubber-stamped through" fear from requirements.md's Success Metrics
  framing.
- **Social job**: N/A — confirmed single-operator tool per requirements.md; no audience beyond the
  operator to justify a mode choice to.
- **Sanity check on scope**: the in-scope read-only "what ran" surface is the right *first* increment
  and matches the requirement's literal ask, but per the emotional-job analysis above it addresses
  "can I find out what happened" better than it addresses "will I notice something's wrong before it
  ships" — the latter is a monitoring/alerting concern, not a display concern, and is reasonably
  deferred, but should be flagged as a likely follow-up rather than assumed solved by this feature.

## Key files referenced

- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (lines 26–152, 491–503) — canonical
  named-mode radiogroup pattern to reuse/generalize.
- `web-app/src/components/backlog/BacklogItemForm.tsx` (lines 38, 192–210 select pattern; 214–268
  checkbox pattern) — where the pipeline-mode field and composition-with-checkboxes UI would live.
- `web-app/src/components/backlog/BacklogItemDetail.tsx` (lines 786–860+ Actions panel, disabled+title
  pattern for missing prerequisites) — where the "what ran" read-only surface and any mode-requires-
  repoPath validation UX would live.
- `web-app/src/components/backlog/GateVerdictBox.tsx` (lines 228–233 `role="status" aria-live="polite"`)
  — a11y precedent for a live-updating read-only status surface.
- `web-app/src/components/backlog/TriageReviewPanel.tsx` — precedent for Apply/Skip/Refine action UX,
  relevant if pipeline-mode escalation ever needs a similar user-facing decision point.

No existing `pipelineMode`/`skillSet`/similar field exists anywhere in `web-app/src` or `server` —
confirmed via grep; this is a greenfield addition to both the form and detail components.
