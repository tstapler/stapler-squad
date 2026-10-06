# UX Design: Prompt Library (Template Picker + Save-as-Template)

Design artifact for the prompt-library feature, produced before implementation (SDD Phase 4.5 /
design). Grounds every surface in `research/ux.md`'s prior findings and `implementation/plan.md`'s
Phase 5 (`TemplatePicker.tsx`) and Phase 6 (`SaveAsTemplateModal.tsx`) — this document does not
re-derive component boundaries or ARIA choices already settled there; it specifies layout,
flow, and acceptance criteria on top of them.

Sources: `project_plans/prompt-library/requirements.md`, `project_plans/prompt-library/research/ux.md`,
`project_plans/prompt-library/implementation/plan.md` (Phase 5 lines 477-623, Phase 6 lines 625-676,
Unresolved Questions lines 81-88).

## Surfaces covered

1. "From template" picker (desktop — anchored popover)
2. "From template" picker (mobile — full-screen sheet)
3. Pending-replace / destructive-overwrite confirmation (inline in the picker)
4. Zero-templates empty state
5. Malformed-template skip notice
6. Save-as-template modal (form + slug preview + typo warning)
7. Save-as-template success / failure states

---

## Surface 1: "From template" picker — desktop

### Entry point

A "From template" button sits adjacent to the First Prompt textarea in
`OmnibarCreationPanel.tsx` (existing field at lines 721-760). It is always rendered, regardless
of whether any templates exist — per `research/ux.md` §4, checking template count before
deciding whether to render the button would add a network round-trip to every session-creation
panel render to avoid one dead click; the empty state (Surface 4) handles the zero-template case
inside the picker instead.

```
┌─ New Session ────────────────────────────────────────────────┐
│  ...                                                          │
│  First Prompt                                    [From template ▾]
│  ┌────────────────────────────────────────────────────────┐  │
│  │ (empty, or user-typed text)                             │  │
│  │                                                          │  │
│  └────────────────────────────────────────────────────────┘  │
│  ...                                                          │
└─────────────────────────────────────────────────────────────┘
```

### Picker layout (anchored popover, opens on click)

```
┌─ Templates ──────────────────────────────────────── [Esc ✕] ─┐
│ ┌────────────────────────────────────────────────────────┐   │
│ │ 🔍 Search templates                                     │   │  <input aria-label="Search templates">
│ └────────────────────────────────────────────────────────┘   │
│ [ maintenance ] [ security ] [ review ] [ testing ]           │  role="group" tag chips, aria-pressed
│ ──────────────────────────────────────────────────────────── │
│ ▸ Dependency Audit                              [Global]      │  role="option" aria-selected
│    Run a full dependency audit and file findings              │
│ ▸ PR Review Pass                                [Workspace]   │
│    Structured review pass against CONTRIBUTING.md             │
│ ▸ Test Generator                                [Global]      │
│    Generate missing unit tests for changed files              │
│ ──────────────────────────────────────────────────────────── │
│ ⓘ 2 templates couldn't be loaded — check                      │  role="status" aria-live="polite"
│   ~/.stapler-squad/prompts/                                   │
└─────────────────────────────────────────────────────────────┘
```

- Rows are keyboard-navigable (`ArrowUp`/`ArrowDown`, clamped not wrapped per plan Task 5.1.1d),
  `Enter` commits the active row, `Escape` closes with no change.
- Active row is indicated via `aria-activedescendant` on the search input pointing at the row's
  `id` (per `AliasPalette.tsx:63,103` precedent, plan Task 5.1.1a) — not by moving DOM focus into
  the listbox.
- Scope badges ("Global" / "Workspace") are always visible, not hover-revealed — mobile-safe per
  `research/ux.md` §6 and plan Story 5.1.3.
- Tag chips are `role="group"`, each chip `<button aria-pressed>`; multi-select, no "apply" step
  (per `LevelFilterChips.tsx` precedent).

### Interaction flow — happy path

| Step | User action | System response |
|---|---|---|
| 1 | Clicks "From template" | `usePromptService().listTemplates(path)` fires; picker opens anchored under the button, focus moves to the search input |
| 2 | Types `"audit"` | Fuse.js filters rows to name/description matches in real time (no debounce needed at local-filesystem scale) |
| 3 | Clicks the `"security"` tag chip | Row list narrows further to templates tagged `security`; chip shows `aria-pressed="true"` |
| 4a | First Prompt field is empty; user presses `Enter` or clicks a row | Template body is interpolated (`{{repo}}`, `{{branch}}`, `{{issue_title}}` resolved from Omnibar form state), inserted into First Prompt, picker closes, focus returns to First Prompt textarea. Session is **not** submitted. |
| 4b | First Prompt field has user-typed text; user presses `Enter` or clicks a row | See Surface 3 (pending-replace confirm) — the picker does not close yet |
| 5 | User edits the populated text | Plain textarea editing — no "linked to template" state, no re-sync indicator (per `research/ux.md` §2: template application is a one-shot insert, not a live link) |

### Exit paths (every state)

- `Escape` closes the picker unconditionally, discarding any in-progress search/filter state,
  never touching the First Prompt field.
- Clicking outside the popover closes it (standard popover-dismiss behavior, matching
  `SlashCommandDropdown.tsx`/`QuickOpenPalette.tsx`).
- The `[✕]` control in the header is a secondary, explicit close affordance for users who don't
  discover `Escape` — required per Nielsen's "user control and freedom" heuristic; do not rely on
  `Escape` alone as the only exit path since it is not visually discoverable.

---

## Surface 2: "From template" picker — mobile (full-screen sheet)

Below the `useIsMobile()` breakpoint, the same `TemplatePicker` component renders inside
`Modal.tsx` as a full-screen sheet instead of an anchored popover (plan Story 5.1.4), because an
anchored popover under the First Prompt textarea risks being clipped by the on-screen keyboard.

```
┌─────────────────────────────────────────┐
│ Templates                          [✕]   │  ← Radix Dialog header, close = tap target ≥44×44
├─────────────────────────────────────────┤
│ 🔍 Search templates                      │
├─────────────────────────────────────────┤
│ [maintenance] [security] [review]        │  ← horizontally scrollable chip row if it overflows
├─────────────────────────────────────────┤
│ Dependency Audit            [Global]     │  ← full-width row, ≥44px tall, tap = select
│  Run a full dependency audit...          │
├─────────────────────────────────────────┤
│ PR Review Pass            [Workspace]    │
│  Structured review pass...               │
├─────────────────────────────────────────┤
│                 ...                      │
└─────────────────────────────────────────┘
```

- Radix `Dialog` supplies focus trap, `Escape`-to-close, `aria-modal="true"`, and
  return-focus-on-close for free (per `research/ux.md` §3 — reuse `Modal.tsx`, don't build a
  bespoke portal).
- Tap is the primary interaction; there is no hover state to depend on (per `research/ux.md`
  §6 — hover-only affordances are disallowed for mobile parity).
- All rows and chips meet the ≥44×44px touch target minimum (WCAG 2.5.5/2.5.8), verified in
  `TemplatePicker.css.ts`'s mobile media-query branch (plan Task 5.1.4b).
- Exit path: `[✕]` in the header (primary, always visible — no keyboard `Escape` assumption on
  touch devices) plus swipe-down-to-dismiss if the underlying `Modal.tsx` sheet variant supports
  it; otherwise `[✕]` alone is sufficient and required.

---

## Surface 3: Pending-replace / destructive-overwrite confirmation

This is the resolved decision from `plan.md` Unresolved Questions §1 — documented here as a
concrete UI state, not re-litigated.

### Trigger condition

`formState.firstPrompt.trim() !== ""` at the moment the user selects a template (click or
`Enter`).

### Layout — inline footer inside the picker, not a separate modal

```
┌─ Templates ──────────────────────────────────────── [Esc ✕] ─┐
│  🔍 Search templates                                          │
│  [ maintenance ] [ security ] [ review ] [ testing ]           │
│  ──────────────────────────────────────────────────────────  │
│▸ Dependency Audit  (highlighted — pending selection)  [Global]│
│   Run a full dependency audit...                              │
│  PR Review Pass                                     [Workspace]│
│  ──────────────────────────────────────────────────────────  │
│  This will replace your current draft:                        │
│  "Run a full dependency audit on stapler-squad and file        │
│   findings as backlog items."                                 │
│                                                                 │
│              [ Cancel ]        [ Replace current draft ]      │
└─────────────────────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Selects a template while First Prompt is non-empty | Picker does **not** close. The selected row stays highlighted/active. A footer panel appears showing the interpolated body that *would* replace the current draft (live preview, per `research/ux.md` §2's Raycast/Alfred precedent — the user sees the substitution result before committing, addressing the silent-blank-on-undefined-variable trust gap called out in `research/ux.md` §5) |
| 2a | Clicks "Replace current draft" (or presses `Enter` a second time) | First Prompt is overwritten with the interpolated body, picker closes, focus returns to the textarea |
| 2b | Clicks "Cancel", presses `Escape`, or clicks a different row | Pending-replace state clears with **no change** to First Prompt. Clicking a different row re-triggers step 1 for the newly selected row (does not silently commit the previous one) |

### Why inline, not a second modal

A confirm-dialog-on-top-of-a-popover would be a second context switch for what should be a single
extra click (per plan's own rationale: "preserves single-action speed... adding exactly one extra
confirming step only when a destructive overwrite is actually possible"). The footer-panel
pattern keeps the user in the same visual frame, consistent with `GitHubIssuePicker.tsx`'s
expand-in-place row preview precedent rather than a nested dialog.

---

## Surface 4: Zero-templates empty state

Rendered **inside** the already-open picker (popover or full-screen sheet, same component in both
cases) when `templates.length === 0 && skippedCount === 0`. This is a first-run/discovery moment,
not a failed search — copy must be actionable, not a bare "No results."

```
┌─ Templates ──────────────────────────────────────── [Esc ✕] ─┐
│  🔍 Search templates                                          │
│  ──────────────────────────────────────────────────────────  │
│                                                                 │
│                  No templates yet                             │
│                                                                 │
│      Drop a .md file in ~/.stapler-squad/prompts/, or          │
│      save one from an existing session's prompt.               │
│                                                                 │
└─────────────────────────────────────────────────────────────┘
```

- `role="status" aria-live="polite"` (informational, not an error — matches
  `AliasPalette.tsx:44-54`'s empty-state shape per `research/ux.md` §3/§4).
- The search input and tag-chip row still render (chips row will simply be empty/absent), so the
  layout doesn't visually jump if templates are added later without reopening the picker.
- Exit path: same as Surface 1 — `Escape`, `[✕]`, or click-outside. No dead end: the copy itself
  gives the user a concrete next action (drop a file, or use Save-as-template from a session),
  satisfying "every error/empty state has an exit path."

### Search yields zero results (distinct from zero templates existing)

If templates exist but the current search+filter combination matches none, show a *different*,
narrower message so the user doesn't confuse "nothing exists" with "nothing matches":

```
│         No templates match "dependnecy" + security             │
│              Try a different search or clear filters            │
```

with a `[Clear filters]` action if any tag chips are active — this is the exit path for this
specific sub-state (distinct from the picker's overall close affordance).

---

## Surface 5: Malformed-template skip notice

Non-blocking, non-modal notice inside the picker when `skippedCount > 0` (AC6). This is a
**partial** failure (N of M templates loaded successfully) and must not borrow the visual weight
of a total failure.

```
│  ──────────────────────────────────────────────────────────  │
│  ⓘ 2 templates couldn't be loaded — check                     │
│    ~/.stapler-squad/prompts/                                  │
```

- `role="status" aria-live="polite"` — explicitly **not** `role="alert" aria-live="assertive"`.
  `research/ux.md` §4 is explicit about this distinction: assertive/alert treatment is reserved
  for `AliasPalette.tsx`'s all-or-nothing config-load failure; a partial parse failure here is
  categorically different and should not interrupt or steal focus.
- Coexists with a non-empty template list — the notice sits below the row list, not above the
  search input, so it doesn't push down and reflow the primary content the user is scanning.
- No dismiss button required (it's a passive status line, not a toast) but it must not block any
  other interaction — search, filtering, and selection all work normally with the notice present.
- Exit path: implicit — the notice does not trap focus or require acknowledgment; closing the
  picker via any Surface 1 exit path dismisses it along with everything else.
- System-level detail (which files, what parse error) is logged server-side per AC6's "logged
  warning" requirement — the UI intentionally shows only the count and a pointer to the directory,
  not raw parse errors, to keep this proportional to a power-user, rarely-hit edge case.

---

## Surface 6: Save-as-template modal

### Entry point

A "Save as template" button on an existing session's initial-prompt display (exact host component
to be confirmed at implementation time per plan Task 6.1.2c — likely the same surface that shows
`firstPrompt`/`initialPrompt` in the session detail view).

### Layout (desktop)

```
┌─ Save as template ─────────────────────────────────── [✕] ──┐
│                                                                │
│  Name *                                                       │
│  ┌──────────────────────────────────────────────────────┐    │
│  │ Dependency Audit                                       │    │
│  └──────────────────────────────────────────────────────┘    │
│  Will save as: dependency-audit.md                            │
│                                                                │
│  Description                                                  │
│  ┌──────────────────────────────────────────────────────┐    │
│  │ Audit deps for vulnerabilities                          │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                                │
│  Tags                                                          │
│  ┌──────────────────────────────────────────────────────┐    │
│  │ maintenance × ​ security × ​ [type + Enter to add]      │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                                │
│  Scope                                                          │
│  ( ● ) Global  (~/.stapler-squad/prompts/)                     │
│  ( ○ ) Workspace  (.stapler-squad/prompts/ in this repo)        │
│                                                                │
│  Prompt body (read-only preview)                                │
│  ┌──────────────────────────────────────────────────────┐    │
│  │ Run a full dependency audit on stapler-squad and file  │    │
│  │ findings as backlog items.                              │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                                │
│                                    [ Cancel ]   [ Save ]        │
└────────────────────────────────────────────────────────────┘
```

### Layout (mobile) — same fields, stacked, no side-by-side groups

Per `.claude/rules/css-architecture.md`'s page-scroll convention and `research/ux.md` §6: the
modal body sets `height: "100%"` + `overflowY: "auto"` so the form remains reachable when the
on-screen keyboard reduces viewport height; all fields stack vertically (no side-by-side layout
below the mobile breakpoint).

### Field states and edge cases

- **Name field empty**: "Save" is disabled (or submits and shows inline validation — implementer's
  choice, but *some* blocking indication is required; an unnamed template is not a valid save
  target). Focus returns to the Name field with a visible inline error, e.g. "Name is required."
- **Workspace scope with no resolvable workspace root** (e.g. a one-off session with no git repo):
  the "Workspace" radio option is disabled with an inline tooltip/hint explaining why (per plan
  Task 6.1.1c) — not silently hidden, so the user understands the option exists but isn't
  available here rather than wondering if it's missing entirely.
- **Slug preview**: live-updates under the Name field as the user types (`Will save as:
  <slug>.md`), so the user sees the exact filename before committing — addresses the "which file
  did this become" trust question directly (client-side preview only; server slug logic in
  `promptlibrary/save.go` is authoritative, matching the plan's explicit "server is authoritative,
  this is preview-only" comment).
- **Body preview is read-only** — the modal captures metadata around an existing prompt, it does
  not let the user edit the body inline (per requirements' explicit "not a WYSIWYG" scope
  boundary); if the user wants different body text, they edit the session's prompt first, then
  re-open this modal.

### Interaction flow — happy path

| Step | User action | System response |
|---|---|---|
| 1 | Clicks "Save as template" on a session's prompt | Modal opens, Name/Description/Tags empty, Scope defaults to Global, body preview shows the session's actual initial prompt (read-only, unmodified) |
| 2 | Types Name = "Dependency Audit" | Slug preview updates to `Will save as: dependency-audit.md` |
| 3 | Adds tags `maintenance`, `security` | Tag chips appear inline in the tags field, removable via `×` |
| 4 | Leaves Scope at default (Global) | — |
| 5 | Clicks "Save" | `usePromptService().saveTemplate(...)` fires |
| 6a | RPC succeeds, no unrecognized tokens | Modal closes, `onSaved(savedTemplate)` fires (parent surface — e.g. a toast or the picker's list — can reflect the new template) |
| 6b | RPC succeeds, body contains an unrecognized token like `{{repoo}}` | Modal stays **open**, shows a dismissible warning: `` `{{repoo}}` won't be replaced — did you mean `{{repo}}`? `` — the save has already happened (non-blocking per Unresolved Questions §2), the warning is purely informative so the user can go fix the source prompt/template later if desired |

### Exit paths

- `[✕]` (Radix Dialog default close), `Escape` (Radix Dialog default), and "Cancel" button all
  discard the form with no write. Because Radix `Dialog` supplies this focus-trap +
  Escape-to-close + return-focus-on-close behavior for free (per `research/ux.md` §3), no custom
  exit-path code is needed here — but this document still lists it explicitly to confirm the
  requirement is met without a bespoke implementation drifting from that guarantee.
- The typo-warning state (6b) is dismissible and does not trap the user — closing the modal after
  a warning is shown still exits cleanly since the save already completed; there is no "stuck"
  state where a warning blocks progress.

---

## Surface 7: Save-as-template failure state (RPC error, not validation)

Distinct from the non-blocking typo warning (6b above) — this covers a hard failure: disk write
error, permission denied, path-traversal rejection, etc.

```
┌─ Save as template ─────────────────────────────────── [✕] ──┐
│  ...(form fields, unchanged, values preserved)...             │
│                                                                │
│  ⚠ Couldn't save this template: permission denied writing to   │
│     ~/.stapler-squad/prompts/dependency-audit.md.               │
│     [ Try again ]                                                │
│                                    [ Cancel ]   [ Save ]        │
└────────────────────────────────────────────────────────────┘
```

- Modal stays open, all typed values (name/description/tags/scope) are preserved — no data loss
  on a failed submit.
- Error message names the concrete failure (permission, invalid path, etc. — sourced from the
  RPC error) and offers "Try again" (re-submits with the same field values) as the concrete next
  action, plus the existing Cancel/✕/Escape exit paths remain available.
- No dead end: user can retry, edit fields and retry, or cancel out entirely.

---

## UX Acceptance Criteria

Each criterion is written to be testable by a human tester (or mapped to a Playwright assertion).

### Task completion

1. A user with an empty First Prompt field can select and apply a template in **2 actions**: click
   "From template" → click (or arrow+Enter) a template row. No forced confirmation step when the
   field is empty.
2. A user with a non-empty First Prompt field can apply a template in **3 actions**: click "From
   template" → select a row → click "Replace current draft" (or Enter twice). Exactly one extra
   step versus the empty-field path, never more.
3. A user can narrow a template list by both free-text search and tag filter simultaneously, and
   can clear both to return to the full list, in ≤ 2 additional actions from an already-open
   picker.
4. A user can save an existing session's prompt as a reusable template in **1 modal**, filling
   at minimum a required Name field, in ≤ 5 form interactions (name, description, tags, scope
   choice, Save click) — no multi-step wizard.
5. Retrieval of a previously-saved template (via search or tag filter) takes no more steps than
   retrieval of any other template — saved templates are not segregated into a separate,
   harder-to-reach view.

### Visibility of system status

6. While templates are loading, the picker shows a state distinguishable from "zero templates
   exist" (e.g. a loading indicator or skeleton row) — a user must never interpret an in-flight
   fetch as "no templates were found."
7. The scope of every template (Global vs. Workspace) is visible on the row itself without
   hovering, clicking, or any other secondary action.
8. When any templates failed to parse, the count of skipped templates is visible in the picker
   without requiring the user to open a log file or console.

### Error and edge-case handling

9. Zero-templates state shows the exact copy "No templates yet" plus actionable guidance
   referencing both creation paths (drop a file, or Save-as-template) — never a bare "No results"
   or blank panel.
10. A malformed template file never crashes the picker, never blocks listing of the templates that
    *did* parse successfully, and is surfaced via a non-blocking `role="status"` notice, not a
    modal or blocking alert.
11. A save failure (permission/disk/path error) preserves all user-entered form field values and
    offers a "Try again" action — the user is never forced to re-type the form after a failed
    save.
12. A typo'd variable token (e.g. `{{repoo}}`) never silently fails — the user sees a specific,
    named warning identifying the exact token and suggesting the likely intended token, without
    blocking the save.
13. **No dead ends**: every surface in this document (picker, pending-replace, empty state, skip
    notice, save modal, save-error state) has at least one visible, explicit exit path (`Escape`,
    `[✕]`, "Cancel", or equivalent) that returns the user to their prior state with no unintended
    side effect (verified per-surface above).
14. Selecting a template never submits/creates a session as a side effect — session creation
    remains a separate, explicit action (the Create button) in every case, including immediate
    application to an empty field.

### Accessibility

15. Every interactive control in the picker (search input, tag chips, rows, close button, replace
    confirm) is reachable and operable via keyboard alone, with a visible focus indicator at every
    step — no mouse-only interaction exists anywhere in this feature.
16. The active/highlighted row is announced to screen readers via `aria-activedescendant` on the
    search input (not by moving DOM focus into the listbox), consistent with the existing
    `AliasPalette.tsx` pattern already shipped in this codebase.
17. All status/notice text (skip notice, empty state, search-zero-results) is exposed via
    `role="status" aria-live="polite"` — verified by a screen-reader smoke test (VoiceOver/NVDA)
    announcing the notice without requiring the user to navigate to it manually, and without
    interrupting whatever the user was doing (i.e., never `assertive`).
18. Text and interactive-element color contrast meets WCAG AA (≥ 4.5:1 for body text, ≥ 3:1 for
    large text/UI component boundaries) in both light and dark themes — verified via the
    Axe Core CI gate already wired up per `CLAUDE.md`'s "UX analysis CI" section (blocks on WCAG
    AA violations for any PR touching `web-app/src/`).
19. All tap targets (chip buttons, row selection area, modal close button) measure ≥ 44×44px on
    the mobile viewport — verified via `TemplatePicker.css.ts`'s mobile media-query branch and a
    manual tap-target audit at a ≤ 400px viewport width.
20. The Save-as-template modal correctly traps focus while open and returns focus to the
    triggering "Save as template" button on close (verified by Radix `Dialog`'s built-in behavior
    — flagged here as a criterion to confirm, not a custom implementation to build).

### Mobile/desktop parity

21. Every affordance available on desktop (scope badge visibility, replace-confirm, skip notice,
    tag filtering) is present and operable on the mobile full-screen variant — no feature is
    desktop-only.
22. No interaction in this feature depends on `:hover`/`onMouseEnter` as its only trigger — every
    hover-enhanced affordance has an equivalent tap/click/keyboard path.

---

## Cross-reference: components this document assumes

| Component | Role | File |
|---|---|---|
| `TemplatePicker.tsx` | Picker (desktop popover + mobile sheet), Surfaces 1-5 | `web-app/src/components/sessions/TemplatePicker.tsx` |
| `SaveAsTemplateModal.tsx` | Save form, Surfaces 6-7 | `web-app/src/components/sessions/SaveAsTemplateModal.tsx` |
| `OmnibarCreationPanel.tsx` | Hosts the "From template" button + First Prompt field | `web-app/src/components/sessions/OmnibarCreationPanel.tsx:721-760` |
| `Modal.tsx` | Radix Dialog primitive reused for mobile picker sheet and the save modal | `web-app/src/components/ui/Modal.tsx` |
| `AliasPalette.tsx` | ARIA/empty-state precedent | `web-app/src/components/ui/AliasPalette.tsx` |
| `LevelFilterChips.tsx` | Tag-chip precedent | `web-app/src/components/shared/LevelFilterChips.tsx` |
| `ViewportProvider.tsx` (`useIsMobile`) | Desktop/mobile branch | `web-app/src/components/providers/ViewportProvider.tsx` |

This document does not introduce any new component boundaries beyond what `implementation/plan.md`
Phase 5/6 already specify — it is a layout/flow/acceptance-criteria layer on top of that plan.
