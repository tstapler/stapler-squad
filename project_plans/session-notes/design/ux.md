# UX Design: Session Notes

Source: `project_plans/session-notes/requirements.md`, `research/ux.md`,
`implementation/plan.md` (component names, testids, and pattern decisions
below are taken directly from the plan — this document does not invent new
component names).

Components referenced: `NotePanel.tsx` (new, mounted in `SessionDetailView.tsx`'s
Info tab, immediately after `<GoalPanel>`), note badge in `SessionCard.tsx`'s
`badges` row (`data-testid="badge-has-note"`), `Tooltip.tsx` (existing,
`label: string` only).

---

## Surface inventory

1. SessionCard note indicator badge (+ tooltip)
2. SessionDetailView Notes panel — read mode (populated)
3. SessionDetailView Notes panel — edit mode
4. SessionDetailView Notes panel — empty state
5. SessionDetailView Notes panel — save-error state
6. Notes-vs-Goal disambiguation (cross-cutting, both panels together)

---

## Surface 1: SessionCard note indicator badge

### Wireframe

```
┌─ SessionCard ──────────────────────────────────────────┐
│ ● fix-flaky-test                              [status] │
│   feature/fix-flaky-test · main                        │
│                                                          │
│   [Goal: "add missing await" 60%] [🤖 autonomous]       │
│   [📝] [⚙ pending-change]        ← badges row          │
│                              ↑                          │
│                    data-testid="badge-has-note"          │
│                    role="img" aria-label="Has a note"    │
└──────────────────────────────────────────────────────────┘
        hover/focus →
        ┌───────────────────────────────────┐
        │ "waiting on CI, don't touch — the │
        │  flaky test needs a rerun before …│  ← Tooltip.label (string,
        └───────────────────────────────────┘     plain-text, ~120 chars)
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Session list/board renders; session's `note` is non-empty after `.trim()` | `📝` badge renders in the badges row, after the `pendingProgramChange` badge |
| 2 | User hovers (mouse) or focuses (keyboard, Tab) the badge | `Tooltip` shows `truncateGoal(note.trim(), 120)` — plain text, markdown syntax characters may appear literally (e.g. a leading `#`) |
| 3 | User clicks the card (anywhere — the badge is not a separate click target) | Card's existing click behavior opens `SessionDetailView`; user lands in the Info tab where the full `NotePanel` is visible (no special "jump to note" behavior — not required, no navigation logic to build) |

### Edge cases

- **Whitespace-only note** (`"   \n"`): badge does NOT render. Gate is `session.note?.trim().length > 0`, never raw truthiness. This is the one behavior in this surface most likely to regress silently, so it must be exercised in both `SessionCard.test.tsx` and the e2e spec.
- **Very long note**: tooltip truncates to ~120 chars + `…`; no scrolling, no markdown rendering inside the tooltip — this is a hard constraint from `Tooltip.label` being typed `string`, not `ReactNode`. Do not attempt to work around it in this feature (see plan Pattern Decision 6 / rejected alternatives).
- **Note containing only markdown syntax that strips to empty visually** (e.g. `"# "`) — not specially handled; `.trim()` on the raw string is non-empty so the badge shows, and the tooltip shows the literal `"# "` text. Acceptable per the "plain-text truncation, syntax chars may appear literally" precedent already set for `GoalPanel`.
- **Loading state**: N/A — the badge is derived synchronously from already-fetched `session.note`; there is no separate loading state for the badge itself (`SessionCard` already waits on the parent list/stream fetch before rendering any card).

---

## Surface 2: NotePanel — read mode (populated)

### Wireframe

```
Info tab
┌─────────────────────────────────────────────┐
│ ▸ Goal & Tasks                    [working]  │  ← GoalPanel (existing, read-only)
│   "add missing await to flaky test" 60%      │
├─────────────────────────────────────────────┤
│ ▾ Notes                             [Edit]   │  ← NotePanel <details open> + <summary>
│                                                │
│   Blocked — see PR #482.                      │  ← data-testid="session-note-rendered"
│   **Waiting on CI** before merging.           │     (ReactMarkdown + remarkGfm,
│   - retry once more                           │      wrapped in markdownBody,
│   - then ping reviewer                        │      headings remapped h1→h5 etc.)
│                                                │
├─────────────────────────────────────────────┤
│   WorkspacePeersPanel …                       │
└─────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | User opens session detail, Info tab | `NotePanel` renders with `session.note` passed as prop; not editing → shows rendered markdown via `ReactMarkdown remarkPlugins={[remarkGfm]}`, heading levels remapped (h1→h5, h2→h6, h3→h6, …) so a user-typed `#` never produces a page-level `<h1>`/`<h2>` collision |
| 2 | User clicks `Edit` button | Panel swaps to edit mode (Surface 3); local `draftValue` seeded from current `note`; focus moves into the textarea |
| 3 | (background) Another browser tab edits and saves the same session's note, `WatchSessions` pushes the update, prop `session.note` changes | Because this panel is NOT editing, its display re-syncs from the new prop value immediately — same guard pattern as `SessionDetailView.tsx:206-207`'s `category`/`workingDir` sync |

### Edge cases

- **Heading collision**: without the remap, a user note starting with `# Heading` would render `<h1>` inside a page that likely already has its own `<h1>`/`<h2>`, breaking the heading outline and failing this repo's axe/Lighthouse CI a11y gate. Remap is not optional — it is the tested behavior for AC2.
- **Long note, read mode**: no truncation in the panel itself (unlike the SessionCard tooltip) — the full rendered markdown displays; the panel scrolls with the rest of the Info tab, it does not need its own internal scroll region.
- **Note updated by another tab while this tab is idle (not editing)**: handled above — re-syncs automatically, no stale-content risk when not mid-edit.

---

## Surface 3: NotePanel — edit mode

### Wireframe

```
┌─────────────────────────────────────────────┐
│ ▾ Notes                                       │
│                                                │
│  ┌───────────────────────────────────────┐  │
│  │ Blocked — see PR #482.                 │  │  ← <textarea
│  │ **Waiting on CI** before merging.      │  │     data-testid="session-note-textarea"
│  │ - retry once more                      │  │     maxLength={10000}
│  │ - then ping reviewer|                  │  │     aria-label="Session note (markdown)"
│  │                                         │  │     aria-describedby="note-hint"
│  └───────────────────────────────────────┘  │
│  Markdown supported.            4821/10000    │  ← id="note-hint" + live char count
│                                                │
│  [ Save ]  [ Cancel ]                         │  ← data-testid="session-note-save-button"
└─────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Panel enters edit mode (from Edit button or "Add note" CTA) | Textarea auto-focused (`ref.current?.focus()` in a `useEffect` keyed on `isEditing` — new behavior for this component, not retrofitted onto older `isEditingX` fields per the plan) |
| 2 | User types/edits text | Local `draftValue` state updates; native `maxLength={10000}` hard-stops further input at the cap — no separate validation error needed for the common case of simply hitting the limit while typing |
| 3 | User clicks `Save` | `onSave(draftValue)` fires (wired to `actions.update({ note: v })`); button shows a busy/disabled state (`saving`) while the promise is in flight |
| 3a | Save succeeds | Panel returns to read mode (Surface 2); focus returns to the `Edit` button (not lost to `<body>`) |
| 3b | Save fails | See Surface 5 (save-error state) — stays in edit mode, text preserved |
| 4 | User clicks `Cancel` | `draftValue` discarded, reverts to the current `note` prop value, exits edit mode without calling `onSave`; focus returns to the `Edit` button |
| 5 | User navigates away (closes tab, switches session) mid-edit without saving | No special handling — standard browser/SPA navigation; unsaved draft is lost. Not a regression path introduced by this feature (matches every other `isEditingX` field's existing behavior in this codebase — no draft-persistence mechanism exists anywhere else either), so no new UX obligation here. |

### Edge cases

- **Mobile keyboard**: textarea must not be obscured by the on-screen keyboard; rely on the page's existing scroll behavior (Info tab already scrolls per `SessionDetailView`'s layout) rather than adding custom viewport-shift logic. Save/Cancel buttons must remain reachable without the user needing to dismiss the keyboard first — verify at implementation time that the button row isn't pinned below the fold when the keyboard is up on a small viewport.
- **Touch targets**: `Edit`/`Save`/`Cancel` buttons must meet the ≥44×44px touch target guidance (mobile+desktop instinct) — reuse existing button component sizing rather than custom small buttons.
- **Paste of very long text** (>10,000 chars): native `maxLength` truncates on paste in most browsers; if a paste event bypasses that in some browser edge case, the Save action must still be blocked/validated client-side before calling `onSave` — do not rely on the backend's `InvalidArgument` rejection as the only guard, since that would surface as a full Surface 5 error for something the UI could have prevented silently.
- **Stream update arrives while editing**: explicitly NOT applied to the draft — see Risk Control in the plan. The textarea keeps the user's in-progress text; the "other tab's" value is not shown until this panel exits edit mode (at which point it re-syncs from the (by-then-current) prop).

---

## Surface 4: NotePanel — empty state

### Wireframe

```
┌─────────────────────────────────────────────┐
│ ▾ Notes                                       │
│                                                │
│   No notes yet — leave yourself a reminder    │  ← muted text, matches
│   about this session.                         │     DescriptionSection.tsx's
│                                                │     empty-state copy pattern
│   [ Add note ]                                 │  ← CTA button (Notes-specific;
│                                                │     DescriptionSection has no
│                                                │     equivalent CTA since it's
│                                                │     edited off-panel)
└─────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | `session.note` is `""` (or unset) | Empty-state copy + `Add note` button render instead of the rendered-markdown block |
| 2 | User clicks `Add note` | Enters edit mode directly (Surface 3) with an empty textarea, focused; functionally identical to the `Edit` button's target state, just reached from the empty state's own CTA wording |

### Edge cases

- **Note becomes empty via Save** (user deletes all text and saves): after save succeeds, panel should show the empty state again, not a "read mode" with a blank rendered body — i.e., the empty-state branch is driven by `note.trim() === ""`, checked the same way both on initial mount and after any save.
- **Whitespace-only save**: if a user saves only whitespace, treat it as empty for display purposes on the next read (trim-check), consistent with the `SessionCard` badge's whitespace handling — avoids a state where the card shows no badge but the panel shows a "populated" (blank-looking) read mode.

---

## Surface 5: NotePanel — save-error state

### Wireframe

```
┌─────────────────────────────────────────────┐
│ ▾ Notes                                       │
│                                                │
│  ┌───────────────────────────────────────┐  │
│  │ my note (preserved, not lost)          │  │  ← textarea still populated,
│  │                                         │  │     still in edit mode
│  └───────────────────────────────────────┘  │
│  Markdown supported.              8/10000    │
│                                                │
│  ⚠ Save failed. Check your connection and     │  ← aria-live="assertive"
│    try again.                                  │     inline, adjacent to textarea
│                                                │     (not a toast — must not be
│  [ Save ]  [ Cancel ]                         │     missable)
└─────────────────────────────────────────────┘
```

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | User clicks `Save`; underlying `updateSession` call rejects (network error, server 500, validation failure) | Panel remains in edit mode; textarea keeps the exact typed text; an inline error element appears with `aria-live="assertive"` so screen-reader users hear it immediately without needing to hunt for a toast |
| 2 | User clicks `Save` again (retry) | Same flow as a normal save attempt; error clears optimistically when a new attempt starts, or on success |
| 3 | User clicks `Cancel` instead of retrying | Draft is discarded (same as the normal Cancel flow), error clears, reverts to whatever the last-known-good `note` prop value was — this is an explicit exit path, not a dead end |

### Edge cases

- **Note exceeds 10,000 chars server-side despite client `maxLength`** (e.g. a race where the cap changed, or a bypassed paste): server returns `connect.CodeInvalidArgument` — this should still route through the same error-display path as a network failure (generic "Save failed" is acceptable; a more specific "Note is too long" message is a nice-to-have but not required by the plan's stated tasks — flag as a possible refinement, not a blocker).
- **Repeated failures**: no retry backoff/lockout specified or needed — this is a manual, user-initiated single save action, not a background sync loop.
- **No dead end**: both `Save` (retry) and `Cancel` (discard-and-exit) remain available and enabled in the error state — the user is never stuck with only a broken control.

---

## Surface 6: Notes-vs-Goal disambiguation (cross-cutting)

This is not a separate screen but a design constraint that must hold across Surfaces 2–4 simultaneously, since `GoalPanel` and `NotePanel` sit adjacent in the same Info tab.

### Layout (both panels together)

```
Info tab
┌─────────────────────────────────────────────┐
│ ▸ Goal & Tasks                    [working]  │  ← has a status chip + progress
│                                                │     fraction; read-only; no Edit
│                                                │     button anywhere in this panel
├─────────────────────────────────────────────┤
│ ▾ Notes                             [Edit]    │  ← no chip, no progress; the ONLY
│   (or "No notes yet" + [Add note])            │     panel of the two with a
│                                                │     visible edit affordance
├─────────────────────────────────────────────┤
│   WorkspacePeersPanel …                       │
└─────────────────────────────────────────────┘
```

### Disambiguation rules (must hold, testable)

- **Labels**: "Goal & Tasks" vs. "Notes" — never "Description"/"Details" for the new panel (would read as a synonym for Goal).
- **Visual asymmetry is intentional, not a bug**: `GoalPanel` shows a status chip (`idle`/`working`/`blocked`/`done`) and task-progress fraction; `NotePanel` must never render either of those — a reviewer seeing `NotePanel` gain a status chip in a future PR should treat it as a regression against this design, not a feature parity gap to close.
- **Editability is the clearest signal**: `NotePanel` has a visible `Edit`/`Add note` button; `GoalPanel` has none (goal is agent-set via MCP `set_session_goal`, not user-editable in this UI). This asymmetry is the primary way a user tells "this one's mine to write" from "this one's the agent's."
- **Empty-state copy reinforces ownership**: "No notes yet — leave yourself a reminder about this session" (first person framing) vs. whatever `GoalPanel` already shows when no goal is set (agent/system framing) — should not accidentally converge on similar wording.

---

## UX Acceptance Criteria

Testable by a human (or Playwright/Jest, mapped to the plan's testids where applicable).

### Task completion

1. A user can attach a note to a session in **3 clicks/interactions or fewer**: open session detail (1, if not already open) → click `Add note` or `Edit` (2) → click `Save` (3), with typing in between not counted as a "step."
2. A user can view an existing note in **1 click**: opening the session detail view (Info tab is default) shows the rendered note with no additional expand/collapse step required beyond what `GoalPanel`'s precedent already establishes (the `<details>` should default to `open`).
3. A user can discard an in-progress edit in **1 click** (`Cancel`), with no confirmation dialog required (low-stakes, single-field, no data loss beyond the current unsaved draft).
4. A user can identify, from the `SessionCard` grid/list alone (no click into detail), which sessions have a note, via the `📝` badge — verifiable by visual scan, no interaction required.

### Error and edge-case handling

5. On save failure, the textarea **retains the user's exact typed text** — verified by asserting the DOM value is unchanged before/after a rejected save promise.
6. On save failure, an inline error message is visible **adjacent to the textarea** (not solely a toast that can be dismissed/missed) and is announced via `aria-live="assertive"`.
7. **No dead ends**: every error state (Surface 5) offers both a retry path (`Save` again) and an exit path (`Cancel`) — neither control is ever disabled in a way that leaves the user with zero next action.
8. The `SessionCard` note badge **never renders for a whitespace-only or empty note** — verified with a session fixture of `note: "   \n"` and `note: ""` both producing no `data-testid="badge-has-note"` element.
9. A note exceeding 10,000 characters cannot be saved as literal overflow — client `maxLength` prevents typing past the cap, and a server-side rejection (if ever reached) surfaces through the same Surface 5 error path, not a silent truncation or a crash.

### Accessibility

10. **Keyboard navigation**: a user can reach and activate `Edit`/`Add note`, type into the textarea, and reach/activate `Save`/`Cancel` using only Tab/Shift+Tab/Enter/Space — no interaction in any surface requires a mouse.
11. **Focus management**: entering edit mode moves focus into the textarea; exiting edit mode (via Save or Cancel) returns focus to the `Edit` button — verified by asserting `document.activeElement` at each transition, not just that focus "went somewhere."
12. **Screen-reader labels present**: the textarea has a programmatic label (`aria-label="Session note (markdown)"` or equivalent `<label>`) and an `aria-describedby` hint ("Markdown supported"); the `SessionCard` badge has `role="img"` + `aria-label="Has a note"` so it announces meaningfully rather than as a bare emoji glyph.
13. **Live regions are correctly scoped**: save-success feedback (if any is added beyond the mode switch itself) uses `aria-live="polite"`; save-error feedback uses `aria-live="assertive"` — polite and assertive regions must not be the same DOM node reused for both severities.
14. **Heading hierarchy preserved**: a note containing `# Heading` syntax renders without producing a page-level `<h1>`/`<h2>` collision — verified by asserting the rendered output's heading tag is remapped (e.g. `<h5>`), and by running the existing axe/Lighthouse CI check against a page containing a heading-laden note.
15. **Color contrast ≥ 4.5:1**: all new text (empty-state copy, error message, character count, button labels) uses existing theme tokens from `markdownBody.css.ts`/the shared theme contract, not new hardcoded colors — no new contrast audit should be needed if this constraint holds; flag any new color value introduced for this feature as a violation of `.claude/rules/css-architecture.md` in addition to this AC.
16. **Touch targets ≥ 44×44px**: `Edit`, `Add note`, `Save`, `Cancel` buttons meet minimum touch target size on mobile viewports — verified via a mobile-width Playwright viewport or manual device check.

### Cross-panel disambiguation (Surface 6)

17. A user shown both `GoalPanel` and `NotePanel` side by side can correctly state which one is agent-written and which is user-written, based on labels and the presence/absence of an Edit affordance alone (no tooltip or help text required to disambiguate) — this is a design-review-time human judgment call, not an automatable test, but should be explicitly checked in design QA before implementation.

---

## Summary of surfaces and criteria

- 6 surfaces designed: SessionCard badge, NotePanel read mode, NotePanel edit mode, NotePanel empty state, NotePanel save-error state, Notes-vs-Goal disambiguation.
- 17 UX acceptance criteria written, covering task completion (4), error/edge-case handling (5), accessibility (7), and cross-panel disambiguation (1).
