# UX Design: backlog-configurable-pipeline

Phase 3.5 (design) artifact — precedes implementation of `implementation/plan.md`'s Phase 3
(Frontend). Grounds Phase 3's Epics 3.1–3.4 in concrete wireframes, interaction flows, and
testable acceptance criteria, and gives an explicit recommendation on the two deferred
follow-ups flagged in `research/ux.md` §5.

**Inputs read**: `requirements.md`, `research/ux.md`, `implementation/plan.md` (Domain Glossary +
Phase 3), plus direct inspection of `BacklogItemForm.tsx`, `OmnibarCreationPanel.tsx`,
`BacklogItemDetail.tsx`, `BacklogSourcesSettings.tsx`, `settings/backlog-sources/page.tsx`.

---

## Surface Map

| # | Surface | File(s) | Plan Epic |
|---|---|---|---|
| A | Item-level pipeline-mode selector | `BacklogItemForm.tsx` (create + edit) | 3.2 |
| B | "Overrides" fieldset (3 existing checkboxes, regrouped) | `BacklogItemForm.tsx` | 3.2 |
| C | Pipeline-modes management: list view | `settings/pipeline-modes/page.tsx` | 3.3 |
| D | Pipeline-modes management: create/edit form | `settings/pipeline-modes/PipelineModeForm.tsx` | 3.3 |
| E | Pipeline-modes management: enable/disable/delete actions | `settings/pipeline-modes/page.tsx` | 3.3 |
| F | "What ran" read-only surface | `BacklogItemDetail.tsx` | 3.4 |
| G | Cross-cutting error/edge states | all of the above | 3.2–3.4 |

Six primary surfaces (A–F) plus one cross-cutting concern (G) covering error/edge states that
recur across A, C, D, F.

---

## A. Item-level pipeline-mode selector (`BacklogItemForm.tsx`)

### Wireframe

```
┌─ New / Edit Backlog Item ──────────────────────────────────────────────┐
│ Title *          [________________________________]                   │
│ Description      [________________________________]                   │
│                   ...                                                  │
│ Repository path   [________________________________]  ⓘ hint          │
│ Priority          [ P3 — Medium            ▾]                          │
│                                                                          │
│ Pipeline mode                                                          │
│ ┌──────────┐ ┌──────────┐ ┌──────────┐  ┌────────┐                    │
│ │ Default  │ │ Quick Fix│ │ Full SDD │  │ ▾ More │                    │
│ │  ● sel.  │ │          │ │          │  │        │                    │
│ └──────────┘ └──────────┘ └──────────┘  └────────┘                    │
│ Runs the standard triage → plan → implement → review pipeline.         │  ← live hint,
│                                                                          │    aria-describedby
│ ┌─ Overrides (independent of pipeline mode) ───────────────────────┐  │
│ │ ☐ Skip planning phase                                             │  │
│ │    Go straight to triage without a separate planning pass.        │  │
│ │ ☐ Skip review gate                                                │  │
│ │    Mark work done without an automated review pass first.         │  │
│ │ ☐ Auto-spawn work session                                        │  │
│ │    Skip the manual "Spawn Session" click...                       │  │
│ └────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  ⚠ Full SDD mode requires a repository path — add one above.           │  ← conditional
│                                                                          │    inline warning
│                                    [ Cancel ]   [ Save Item ]           │
└──────────────────────────────────────────────────────────────────────┘
```

`RadioGroup` renders `PIPELINE_MODES` fetched via `listPipelineModes()` (only `enabled: true`
modes, per Epic 3.2.1), with `"Default"` (`value: ""`) always present as the first, permanently
pinned option — it is not fetched from the DB, it is a hardcoded literal so the zero-regression
guarantee holds even if the mode list fails to load (see error state G-4 below).

### Interaction flow

1. Form mounts (create: `pipelineMode = ""`; edit: `pipelineMode = initialValues.pipelineMode ?? ""`).
2. `listPipelineModes()` fires on mount; while pending, the radio group renders with only
   `"Default"` selectable and the rest disabled/skeleton (never blocks form submission — see G-4).
3. User clicks a mode button, or tabs into the group and arrow-keys through options (roving
   tabindex, arrow keys select immediately — matches `SessionTypeRadioGroup`'s existing contract).
4. On change, the hint span below the group updates to that mode's `description` live.
5. If the selected mode has a declared prerequisite the item doesn't satisfy (only known case in
   scope today: a mode's content-template fields reference `{{repo_path}}`-derived context and
   `item.repoPath` is empty), an inline warning renders below the "Overrides" fieldset — same
   `role="alert"` treatment as `errors.repoPath` (line ~186) — but it does **not** block selection
   or submission (mirrors the "warn, don't disable" pattern research/ux.md §4 recommends when the
   control is a choice, not an action).
6. The three "Overrides" checkboxes are unaffected by mode selection — no auto-check, no
   auto-disable, no visual tie to the selected mode beyond the shared fieldset boundary. This is
   the plan's explicit "compose, not subsume" decision (Pattern Decisions table).
7. On submit, `pipelineMode` (string, possibly `""`) is included in the create/update payload
   alongside the 3 existing booleans.

### Error / edge-case handling

- **G-1 — Mode requires something the item lacks** (e.g. repoPath): inline warning, non-blocking,
  as above. Exit path: fix the prerequisite or pick a different mode; nothing traps the user.
- **G-2 — Editing an item whose stored `pipelineMode` no longer matches any currently-enabled
  mode** (deleted, or disabled since the item was created): `RadioGroup`'s `options.find()` won't
  match. Render an **extra, synthetic, disabled radio option** at the position it would have
  occupied — label `"Unknown mode ('<slug>')"`, `aria-checked="true"`, `aria-disabled="true"` —
  rather than silently falling back to "Default" being shown as selected (which would misrepresent
  the item's actual stored state to the user). A hint below reads: *"This item references a
  pipeline mode that no longer exists or is disabled. Choosing a different mode below will replace
  it when you save."* The user must actively pick a live mode (or Default) to change it — the form
  never silently rewrites the field on their behalf.
- **G-3 — `listPipelineModes()` fails (network/server error)**: the radio group still renders with
  only `"Default"` present and selectable; a small inline notice (`role="status"`) reads *"Couldn't
  load pipeline modes — you can still save with Default."* The form remains fully submittable.
  This is the concrete mechanism keeping the "no engineering involvement to add a mode" feature
  from becoming a single point of failure for the base create/edit flow.

---

## B. "Overrides" fieldset

Covered inline in wireframe A above. No new interaction beyond the existing 3 checkboxes;
the only change is visual grouping (`<fieldset><legend>`) and label. No new error states —
these fields' existing validation/behavior is unchanged (per plan.md, "no change to the
checkboxes' own state/logic, purely visual regrouping").

**Clarification on the compose-not-subsume boundary**: a pipeline mode's own content-template
fields never programmatically read or set the 3 `Skip*`/`AutoSpawnSession` checkboxes' values, and
the checkboxes never programmatically read or set the mode's content, per plan.md's "compose, don't
subsume" Pattern Decision — the two are fully independent axes of configuration, always. The only
place a mode author can note an intended pairing (e.g. "this mode is meant to be used with Skip
Review Gate checked") is the mode's own free-text `description` field, shown as a human-readable
hint to whoever selects the mode — there is no enforced or automatic link between the two, and none
should be built. Implementers and future designers should not try to secretly wire the mode
selection and the Overrides checkboxes together.

---

## C. Pipeline-modes management — list view (`/settings/pipeline-modes`)

### Wireframe

```
┌─ Settings ▸ Pipeline Modes ─────────────────────────────────────────────┐
│                                                                            │
│  Pipeline modes let you customize which skills/prompts drive a backlog   │
│  item's triage, work, and review — without a code change.                │
│                                                                            │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │ ● Quick Fix        quick        3 items using this      [Edit] [⋮]  │ │
│  │ ● Full SDD         full         0 items using this      [Edit] [⋮]  │ │
│  │ ○ Legacy Fast      legacy-fast  1 item using this        [Edit] [⋮]  │ │  ← disabled,
│  │                                                            dimmed    │ │     dimmed row
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                            │
│  [ + New Pipeline Mode ]                                                 │
└────────────────────────────────────────────────────────────────────────┘
```

`●`/`○` = enabled/disabled indicator (matches `/settings/backlog-sources`'s existing
enabled/disabled row treatment). `[⋮]` opens a menu with **Enable/Disable** and **Delete**.
The "N items using this" count is a lightweight, valuable trust-building addition — it warns the
operator before they disable/delete a mode that live items depend on. (Not in plan.md's literal
Story text; recommended as a low-cost UI addition using data the backend already needs for the
delete-safety check discussed under G-5 below — flag to implementer as a nice-to-have, not a
blocking requirement.)

### Interaction flow

1. Page loads → `listPipelineModes()` (unfiltered — enabled AND disabled, unlike the item-form
   selector which only shows enabled).
2. Click **"+ New Pipeline Mode"** → navigates to/opens the create form (surface D).
3. Click **[Edit]** on a row → opens the same form pre-filled (surface D).
4. Click **[⋮] → Disable** → optimistic UI update (row dims immediately), calls
   `UpdatePipelineMode(enabled: false)`; on failure, row reverts and a toast/inline error shows.
5. Click **[⋮] → Delete** → confirmation dialog (surface E).

### Error / edge-case handling

- **Empty state** (no modes defined yet — the common case immediately after this feature ships):
  render `"No pipeline modes yet. Items use the built-in Default pipeline until you create one."`
  plus the same **"+ New Pipeline Mode"** button — not a bare empty table.
- **Load failure**: `"Couldn't load pipeline modes. [Retry]"` — retry re-fires the fetch; no dead
  end.

---

## D. Pipeline-modes management — create/edit form (`PipelineModeForm.tsx`)

### Wireframe

```
┌─ New Pipeline Mode ──────────────────────────────────────────────────────┐
│ Slug *        [ quick___________ ]  (lowercase, letters/digits/hyphens,  │
│                                       cannot be changed after creation)  │
│ Name *        [ Quick Fix________ ]                                     │
│ Description   [ For trivial, low-risk changes___________________ ]      │
│ Enabled       [x] Enabled                                                │
│                                                                            │
│ ▾ Slash commands (written into the item's worktree)                     │
│   status.md content     [textarea] ......................                │
│   done.md content       [textarea] ......................                │
│   fail.md content       [textarea] ......................                │
│   review.md content     [textarea] ......................                │
│   ship.md content       [textarea] ......................                │
│   help.md content       [textarea] ......................                │
│                                                                            │
│ ▾ Prompts                                                                │
│   Triage prompt          [textarea] ...................... ⓘ {{item_id}}│
│   Review prompt          [textarea] ......................              │
│   Initial session prompt [textarea] ......................              │
│                                                                            │
│  ⚠ Unknown placeholder '{{bad_token}}' in Triage prompt (row 3)          │  ← inline,
│                                                                            │    field-scoped
│                                    [ Cancel ]   [ Delete ]   [ Save ]     │
└────────────────────────────────────────────────────────────────────────┘
```

The 9 content-template fields are grouped into two collapsible sub-sections ("Slash commands" /
"Prompts") rather than one flat list of 9 textareas — reduces initial visual weight; both default
**expanded** on the create form (so the operator sees the full surface immediately) and default
**collapsed** on edit *only if all fields in that group are empty* (so an edit of a fully-populated
mode still shows everything). Each field has placeholder-hint text listing the substitutions it
supports (e.g. `{{item_id}}`, `{{title}}`), per plan.md's "fixed set of `{{placeholder}}`
substitutions" design.

### Interaction flow

1. Create: all fields blank except `enabled` (defaults checked). Edit: pre-filled from
   `GetPipelineMode`/`ListAll` response; `slug` field rendered `disabled`/read-only (immutable
   after creation per plan.md Story 3.3.2).
2. Submit → `CreatePipelineMode` or `UpdatePipelineMode`. On success: list view (surface C)
   refreshes without a full page reload/navigation; a brief success toast confirms.
3. Any content-template field left blank is valid — no required-field validation on the 9
   template fields, only on `slug`/`name`. **Blank-field semantics (previously undefined,
   specified here explicitly)**: an empty content-template field means "this mode does not
   write/override this particular slash-command file or prompt at all — for this ONE field, the
   item falls back to the built-in default content," not "write an empty file" or "produce an
   empty prompt." Each of the 9 fields resolves independently: a mode can leave `ship.md content`
   blank (falls back to the default `ship.md`) while overriding all 8 other fields, and vice
   versa. This mirrors `plan.md`'s `CachingPipelineEngine`'s per-call, per-field fail-closed-to-
   default resolution, applied at field granularity within a single resolved mode rather than only
   at the whole-mode level.

**Deferred, out of scope for this phase**: live preview of rendered content (showing what a
template will actually produce for a sample item before saving) and duplicate-existing-mode (a
"start from an existing mode's content" create shortcut) are both reasonable future enhancements,
not required now — noted here so they aren't silently forgotten, but neither blocks this phase's
acceptance criteria.

### Error / edge-case handling

- **G-6 — Duplicate slug**: submit with a slug that collides with an existing mode → backend
  returns `CodeAlreadyExists` (or equivalent) → inline error directly under the Slug field:
  *"A pipeline mode with slug 'quick' already exists — choose a different slug."* Focus returns to
  the Slug field. No navigation, no data loss (rest of the form retains what the operator typed).
- **Invalid slug format** (`"Quick Fix!"`, spaces, uppercase): same inline-under-field treatment,
  client-side pre-check before submit *and* server-side (Story 2.3.1's `CodeInvalidArgument`) as a
  backstop — message: *"Slug can only contain lowercase letters, numbers, and hyphens."*
- **G-7 — Malformed/unknown-placeholder content-template field**: per plan.md's Unresolved
  Question (write-time allow-list rejection, default answer), submit is rejected with a
  field-scoped inline error identifying which textarea and which unrecognized token, as shown in
  the wireframe above — never a generic "invalid input" toast with no indication of *where*.
- **Delete blocked / allowed while referenced**: per plan.md's Unresolved Question (default:
  *allow* deletion, rely on fail-closed resolution), the Delete action is never hard-blocked by
  reference count — but per the list view's "N items using this" indicator (surface C), a
  **confirmation dialog** is the safety net:
  - *Given* a mode referenced by 3 items, *When* the operator clicks Delete, *Then* the confirm
    dialog reads: *"Delete 'Quick Fix'? 3 backlog item(s) currently reference this mode — they will
    fall back to Default pipeline behavior. This cannot be undone."* — explicit, specific,
    consequence-stating, not a generic "Are you sure?".
  - *Given* a mode referenced by 0 items, *When* the operator clicks Delete, *Then* the dialog reads
    the same template with *"0 backlog item(s)"* omitted/simplified to *"Delete 'Legacy Fast'? This
    cannot be undone."*

---

## E. Enable/disable/delete actions (detail)

Covered inline above (surfaces C, D). One additional cross-surface rule:

- **Disabling** a mode (not deleting) never affects items that already reference it — it only
  removes the mode from the *selector*'s options in `BacklogItemForm.tsx` for *new* selections
  (existing items keep showing it, per G-2's "Unknown mode" treatment only applies to fully
  *missing* slugs — a disabled-but-still-present mode instead resolves normally in the "what ran"
  surface and shows as a disabled/dimmed but still-selectable-if-reselected option... actually per
  requirements.md's Risk Control, resolution always falls back cleanly, so: a disabled mode
  selected on an existing item is treated identically to G-2's synthetic "Unknown mode" display in
  the *form*, since it's no longer in the enabled-only fetch — but the backend's `PipelineEngine`
  still resolves it correctly for *running* pipelines (disabled only affects new selection, not
  existing resolution), per the cache's `ListEnabled` semantics feeding the selector while
  `GetBySlug` still works for resolution). This distinction (disabled ≠ deleted, and both differ
  from unresolvable) must be visually distinguishable in G-2's synthetic option label — recommend
  *"Legacy Fast (disabled)"* rendered as a **selectable-but-flagged** option (not fully disabled
  like the "unknown slug" case) when the slug still exists but `enabled: false`, reserving
  *"Unknown mode ('slug')"* strictly for slugs with no matching row at all.

---

## F. "What ran" read-only surface (`BacklogItemDetail.tsx`)

### Wireframe

```
┌─ Item Detail: "Refactor auth middleware" ────────────────────────────────┐
│  ...                                                                       │
│  Sessions                                                                 │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │ Triage session — a3f9c2  (2026-07-12 14:02)                          │ │
│  │   role="group" aria-label="Pipeline"                                 │ │
│  │   Pipeline: Quick Fix                                                │ │
│  │                                                                        │ │
│  │ Work session — b71e08  (2026-07-12 14:15)                            │ │
│  │   Pipeline: Quick Fix                                                │ │
│  │                                                                        │ │
│  │ Review session — c40a91  (2026-07-13 09:03)                          │ │
│  │   Pipeline: custom (unrecognized mode: 'legacy-fast')                │ │  ← degraded
│  │                                                                        │ │     fallback
│  └────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
│  [existing GateVerdictBox, AC criteria, Actions panel unchanged below]    │
└────────────────────────────────────────────────────────────────────────┘
```

Positioned per-`ItemSession`, near the existing session/review context blocks (line ~702 region),
not as one summary line for the whole item — this is the concrete mechanism that lets an operator
see a mode *escalation* mid-flight (e.g. triage ran under "Quick Fix," review ran under "Full SDD"
after an operator manually changed the item's mode) without the display overwriting history, per
`research/ux.md` §4's mid-pipeline-mode-change concern. (Mode is immutable-after-first-triage
per plan.md's Pattern Decision, so this scenario is not reachable *within* a single session's
lifecycle today — but is reachable across sessions on the same item if plan.md's snapshot-per-
`ItemSession` design is honored literally, which it is: each session's own snapshot is shown, not
one item-level value.)

### Interaction flow

1. `BacklogItemDetail` already fetches the item's session list. Each session's
   `pipelineModeSnapshot` (added by Epic 1.6) is resolved against the *currently fetched* mode list
   (from `listPipelineModes()`, same call as surface A) purely for **display** (looking up the
   human-readable `name` for a slug) — the underlying value shown is always the frozen snapshot,
   never re-resolved live.
2. No user interaction beyond viewing — this is `role="group"`, not interactive controls.
3. If session data is still loading, render a skeleton/placeholder row, not a blank gap (avoids a
   flash of "no pipeline info" that could be misread as "this session had no mode").

### Visual treatment for the "(content since changed)" drift annotation

`plan.md`'s Story 3.4.1 adds a `" (content since changed)"` suffix when a session's
`pipelineModeSnapshotHash` no longer matches its mode's live `content_hash` (mode content edited
since the session ran). This is informational, not blocking, so it must NOT use a full alert box —
reuse this codebase's existing warning-tier visual language instead of inventing a new one: the
same treatment as `web-app/src/components/backlog-stuck/stuckReason.ts`'s `chipOrphanedTriage`
warning-tier chip class (yellow/amber, paired with a text label, never color-only — matching this
file's own "never the sole signal" convention for `STUCK_REASON_ICONS`), or equivalently
`GateVerdictBox.tsx`'s partial/unverifiable warning-tier styling (`skipGateWarning`-class region).
Concretely: render `"(content since changed)"` as a small inline warning-colored badge or text span
immediately after the mode name inside the `role="group"` "Pipeline" block — not a separate
`role="alert"` box, since nothing here requires the user's immediate attention or blocks any
action, unlike G-1's prerequisite warning.

### Error / edge-case handling

- **G-8 — Snapshot slug not found in current mode list** (deleted or renamed since the session
  ran): render `"custom (unrecognized mode: '<slug>')"` exactly as specified in plan.md Story
  3.4.1 — never blank, never `undefined`, never silently falls back to "Default" (that would
  misrepresent history).
- **G-9 — No snapshot recorded** (session predates this feature, `pipelineModeSnapshot === ""`):
  render `"Pipeline: Default"` — matches the "legacy items behave exactly as today" zero-regression
  guarantee from requirements.md, applied to display as well as behavior.
- **Live-updating consideration**: unlike `GateVerdictBox`'s `role="status" aria-live="polite"`
  (used because verdicts change while a human is plausibly watching), the "what ran" surface does
  **not** need `aria-live` — a session's `pipelineModeSnapshot` is set once at session start and
  never changes thereafter, so there is no transient update to announce. Use plain `role="group"`
  with a static `aria-label="Pipeline"`, not `aria-live`. (This intentionally diverges from
  research/ux.md §3 point 6's suggestion to "mirror" `aria-live` — the mirroring should apply to
  the *labeling convention*, not the live-region behavior, since the underlying data here is
  write-once, not streaming.)

---

## G. Cross-cutting error/edge-case summary

| ID | Scenario | Surface(s) | Behavior |
|---|---|---|---|
| G-1 | Selected mode's prerequisite (e.g. repoPath) missing on item | A | Non-blocking inline warning, `role="alert"` |
| G-2 | Item's stored mode slug doesn't match any current mode (deleted) | A | Synthetic disabled "Unknown mode ('slug')" option; user must actively change it |
| G-3 | `listPipelineModes()` fetch fails | A | "Default" still selectable; form still submittable; inline status notice |
| G-4 | Mode list still loading | A | Only "Default" enabled meanwhile; never blocks submit |
| G-5 | Delete a mode still referenced by items | C, D | Confirm dialog states reference count + fallback consequence explicitly; deletion always allowed (no hard block), per plan.md's default answer |
| G-6 | Duplicate slug on create | D | Inline error under Slug field, focus returned, no data loss |
| G-7 | Unknown `{{placeholder}}` token in a content-template field | D | Field-scoped inline error naming the field + token |
| G-8 | "What ran" snapshot slug unresolvable | F | `"custom (unrecognized mode: '<slug>')"`, never blank |
| G-9 | Legacy item/session with no mode recorded | A, F | Displays/behaves as `"Default"`, never blank |

Every row above has an explicit, stated exit path (no dead ends): G-1/G-3/G-4 never block
submission; G-2 requires an active choice but the choice set (including re-picking the same
missing slug is impossible, but picking Default or any live mode) is always available; G-5's
confirm dialog has both a Cancel and a Confirm path; G-6/G-7 keep the operator on the form with
their input intact and a specific fix instruction; G-8/G-9 are pure display fallbacks with no
action required.

---

## UX Acceptance Criteria

Testable by a human (or Playwright, per `.claude/rules/e2e-test-conventions.md` — `data-testid`/
ARIA-role locators only, no `waitForTimeout`).

### Task completion

1. A user can select a non-default pipeline mode on a **new** backlog item in ≤ 2 interactions
   from an empty form: (1) click/arrow-key to the desired mode button. (Title, priority, etc. are
   pre-existing required steps not attributable to this feature.)
2. A user can change an **existing** item's pipeline mode in ≤ 2 interactions: (1) open the item's
   edit form (already-required step), (2) click the new mode.
3. A user can create a new, immediately-usable pipeline mode in ≤ 4 interactions from the settings
   list view: (1) click "+ New Pipeline Mode", (2) fill Slug + Name (Description/templates
   optional), (3) click Save — the mode is selectable on a backlog item's form on the very next
   visit to surface A, with no deploy/restart.
4. A user can determine which mode drove a specific past session in ≤ 1 click: open the item's
   detail view (already-required navigation) — the "what ran" info is visible without any further
   interaction (no expand/collapse required to see it).
5. A user can disable a mode without deleting it in ≤ 2 interactions from the list view: `[⋮]` →
   "Disable".

### Error states

6. Submitting a duplicate slug shows the exact message *"A pipeline mode with slug '&lt;slug&gt;'
   already exists — choose a different slug."* inline under the Slug field and offers the action
   "change the slug and resubmit" (form data preserved, no navigation).
7. Submitting a content-template field with an unrecognized `{{placeholder}}` shows a message
   naming both the field and the offending token, and offers the action "edit that field and
   resubmit" (form data preserved).
8. Attempting to delete a mode referenced by N ≥ 1 items shows a confirmation dialog stating the
   exact reference count and the fallback consequence ("fall back to Default pipeline behavior"),
   and offers both "Cancel" and "Delete anyway" — never a silent or irreversible action without
   this dialog.
9. An item referencing a deleted/unresolvable mode shows `"Unknown mode ('<slug>')"` (form) or
   `"custom (unrecognized mode: '<slug>')"` ("what ran" surface) — never a blank field, never a
   JS error/crash, never a value silently substituted without indicating substitution occurred.
10. If `listPipelineModes()` fails, the item form remains fully usable and submittable with
    "Default" mode, with a non-blocking notice — a backend outage in the mode-listing RPC never
    prevents creating/editing a backlog item.

### No dead ends

11. Every error state listed in the Error States section and the G-1…G-9 table above has at least
    one visible, labeled action that returns the user to a working state (Cancel, Retry, "change
    the slug", "pick a different mode," etc.) — none require a page reload or browser back button
    to recover.

### Accessibility

12. The pipeline-mode `RadioGroup` (surface A) has an accessible name equal to its visible label
    ("Pipeline mode") via `aria-labelledby` (not a duplicated `aria-label` string) — verifiable via
    `getByRole("radiogroup", { name: "Pipeline mode" })`.
13. The pipeline-mode `RadioGroup`'s accessible description includes the currently-selected mode's
    hint text via `aria-describedby`, updating as selection changes — verifiable via
    `getByRole("radiogroup", { description: /.../  })`.
14. All `RadioGroup` options are reachable via Tab (once, to enter the group) + Arrow keys (to move
    *and* select) — no keyboard trap, no requirement to press Space/Enter to confirm a selection.
15. Every interactive element introduced by this feature (radio buttons, Edit/Delete/Enable-Disable
    buttons, form inputs, confirm-dialog buttons) has a `data-testid` or is targetable via ARIA
    role + accessible name — no new CSS-class-only locators, per
    `.claude/rules/e2e-test-conventions.md`.
16. The "what ran" surface (`role="group" aria-label="Pipeline"`) is reachable and its text content
    exposed to screen readers without requiring focus on an interactive control (it's static
    content, so a `role="group"` landmark with a labeled region is sufficient — no `tabIndex`
    needed).
17. All new text (labels, hints, error messages, confirm-dialog text) meets ≥ 4.5:1 contrast ratio
    against its background in both light and dark theme, consistent with existing form field/error
    styling already in `BacklogItemForm.module.css`/settings page styles (no new colors introduced
    for this feature — reuse existing `--error-text`, `--text-secondary`, etc. tokens per
    `.claude/rules/css-architecture.md`).
18. The `PipelineModeForm`'s 9 content-template `<textarea>` elements each have a programmatically
    associated `<label>` (via `htmlFor`/`id`, not placeholder-only labeling) naming the specific
    target file/prompt (e.g. "Triage prompt", "review.md content") — a screen reader user tabbing
    through the form hears which field they're in without relying on visual position.

---

## Recommendation on the two deferred follow-ups (research/ux.md §5)

`research/ux.md` §5 flagged two low-cost items as "worth flagging to product/planning" but left
them explicitly out of that research pass's own scope-decision authority. `plan.md`'s Phase 3
does not include either as an in-scope story. Recommendation for each, made concretely here:

### (a) Compact pipeline-mode badge on the backlog **list** view (`BacklogBoard.tsx` cards)

**Recommendation: pull this into Phase 3 as a small additional story (Epic 3.5).**

**Superseded note (Product Triad Review repair round)**: `plan.md`'s Out of Scope section
overrides this recommendation — Epic 3.5 is explicitly NOT added in this plan, deferred until both
the Phase-0 adoption spike and Phase 4's real-usage proof succeed. The reasoning below (data
already loaded, pure rendering addition, small scope) is still accurate and remains the reason
Epic 3.5 is cheap to pick up later — but "cheap and valuable" was judged insufficient justification
to add net-new UI scope before this plan's adoption premise is validated. See `plan.md`'s Risk
Control and Out of Scope sections for the current, authoritative decision.

Reasoning:
- The data is already resolved and loaded — `BacklogItemData.PipelineMode` is a plain string field
  on the same struct the list view already fetches for every card (priority, status, title). No
  new RPC, no new backend work, no new loading state to design.
- It's a pure rendering addition: a small chip/badge next to the existing priority badge, reusing
  whatever badge component `BacklogBoard.tsx` cards already use for priority/status (styling
  precedent exists; this is not a new visual language).
- It directly serves the dominant emotional job identified in `research/ux.md` §5 (trust — "will I
  notice something's wrong before it ships") in a way the read-only detail-view surface (Epic 3.4)
  structurally cannot: Epic 3.4 requires a click-through to a specific item's detail view, which
  only helps an operator who already suspects something and goes looking. A list-view badge is
  passively visible during normal triage scanning — the exact gap `research/ux.md` §5 names
  ("the absence was invisible by construction... the same risk applies to a purely passive 'what
  ran' panel that requires proactively opening the item detail view").
- Scope is genuinely small: one `<span>`/badge render per card, keyed off already-fetched data,
  with the same "Default" fallback for `pipelineMode === ""`. This does not touch the caching
  layer, the engine, or any backend code — it is strictly additive to Epic 3.2's existing data
  fetch.

Suggested placement in the plan: **Epic 3.5**, sequenced after Epic 3.2 (needs `pipelineMode` on
the fetched item shape, which 3.2 already requires) and independent of Epic 3.3/3.4 — could ship
in parallel with either. One story, one task, roughly the same size as Epic 3.2.3 (a registry-entry
story), not a multi-day addition.

### (b) `GateVerdictBox` flagging verdicts that passed under a reduced-scrutiny mode

**Recommendation: keep this deferred, out of scope for this plan.**

Reasoning:
- Unlike (a), this is not purely additive rendering — it requires first **defining** "reduced
  scrutiny," a judgment call nothing in `requirements.md` or `plan.md` resolves. Is any non-default
  mode "reduced scrutiny"? Only modes with `skipReviewGate` baked into their template content? A
  mode-level boolean flag that doesn't exist in the current 9-content-template-field schema (would
  require a 10th field or new column, i.e. a schema change)? This is a product-definition question,
  not a UI-wiring task, and `plan.md`'s Migration Plan does not include any such field.
- It touches `GateVerdictBox.tsx`'s verdict-rendering semantics, a component with its own
  established `role="status" aria-live="polite"` contract for a *different* concern (review-in-
  progress state) — bolting on a second, unrelated "was this reduced scrutiny" concern without a
  settled data model risks exactly the kind of half-considered coupling
  `.claude/rules/interface-pollution-checklist.md` warns against elsewhere in this codebase's
  conventions (mixing concerns because the file is already open, not because the concerns belong
  together).
- `research/ux.md` §5 itself already flags this as "a monitoring/alerting concern, not a display
  concern, and is reasonably deferred" — this UX design agrees with that self-assessment.
- The in-scope Epic 3.4 "what ran" surface, plus (a) above if pulled in, already give the operator
  the raw information (which mode ran) at both the list-scan and per-session-detail level. Once
  real modes exist (Phase 4's "define a real 'quick' mode via live UI" milestone) and it's
  observable in practice which modes actually correlate with lighter review, a follow-up project
  can define "reduced scrutiny" concretely and design the `GateVerdictBox` treatment against real
  data instead of a speculative definition now.

**Net**: one of the two follow-ups (the badge) is cheap enough and valuable enough to pull in now;
the other (the `GateVerdictBox` flag) is correctly deferred because it is blocked on an undefined
product concept, not on UI effort.
