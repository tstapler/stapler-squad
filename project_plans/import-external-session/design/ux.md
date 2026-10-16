# UX Design: import-external-session

**Date**: 2026-07-16
**Status**: Ready for Phase 4 validation / Phase 5 implementation
**Inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md` (Phases 1–3), ADR-001

This document is the concrete design artifact for the six user-facing surfaces implied by the
plan's Phase 1–3 stories. It builds directly on `research/ux.md`'s recommendations rollup and
does not introduce new visual/interaction paradigms beyond what that research already
identified as the two components to imitate:

- **`ResumeSessionModal.tsx`** — `role="dialog" aria-modal="true" aria-labelledby="..."` focus-trap
  modal shape, used for both preview/confirm-import and confirm-kill dialogs.
- **`ApprovalAnalyticsPanel.tsx`** — header "select all" checkbox + row checkboxes table shape,
  used for the discovery/browse table and batch selection.

All new components live under `web-app/src/components/sessions/` with colocated
`*.css.ts` files per `.claude/rules/css-architecture.md` (no raw `var()` strings, tokens only,
named `zIndex` slot for the modal layer, `createPortal` for overlays). All interactive elements
use `data-testid` or ARIA role locators per `.claude/rules/e2e-test-conventions.md`.

---

## Surface Map

| # | Surface | Component(s) | Plan story |
|---|---|---|---|
| 1 | Discovered-sessions browse table (mux + plain-tmux + manual entry) | `ImportExternalSessionsPanel.tsx` | 1.4.1, 2.3.1, 3.1.3 |
| 2 | Import preview / confirm-import dialog (incl. disambiguation) | `ImportPreviewDialog.tsx` | 1.4.2 |
| 3 | Per-row import progress + status machine | `ImportExternalSessionsPanel.tsx` (row state) | 1.4.3 |
| 4 | Confirm-before-kill dialog (single session) | `ConfirmKillDialog.tsx` | 1.4.3, 2.2.2 |
| 5 | Batch import progress + summary (multi-session) | `BatchImportSummary.tsx` | 3.1.1, 3.1.2 |
| 6 | Batch kill-confirmation (successfully-imported rows only) | `ConfirmKillDialog.tsx` (batch mode) | 3.1.2 |

Surfaces 1–4 ship in Phase 1 (mux path only; Phase 2 adds the "Plain tmux" badge + manual entry
to Surface 1, no new surface). Surfaces 5–6 ship in Phase 3.

---

## Surface 1 — Discovered-Sessions Browse Table

### Wireframe

```
┌─ Import External Sessions ──────────────────────────────────────────────────┐
│ aria-live="polite": "3 external sessions found"                             │
│                                                                               │
│ [+ Add manual candidate...]                              [Import 2 selected]│
│                                                                               │
│ ┌───┬──────────┬──────────────────┬─────────────┬────────────────┬────────┐ │
│ │[✓]│ Program  │ Path             │ Last active │ Status         │ Action │ │
│ ├───┼──────────┼──────────────────┼─────────────┼────────────────┼────────┤ │
│ │[✓]│ ● Claude │ ~/proj/api       │ 2m ago      │ Ready          │ Import │ │
│ │   │          │ [ssq-mux]        │             │                │        │ │
│ ├───┼──────────┼──────────────────┼─────────────┼────────────────┼────────┤ │
│ │[✓]│ ● Claude │ ~/proj/web       │ 5m ago      │ Needs choice   │ Import │ │
│ │   │          │ [Plain tmux]     │             │ (2 candidates) │        │ │
│ ├───┼──────────┼──────────────────┼─────────────┼────────────────┼────────┤ │
│ │[ ]│ ◆ Antigravity│ ~/proj/infra │ 1h ago      │ Ready          │ Import │ │
│ │   │          │ [ssq-mux]        │             │                │        │ │
│ └───┴──────────┴──────────────────┴─────────────┴────────────────┴────────┘ │
│                                                                               │
│ (empty state, shown instead of the table when zero rows)                    │
│ ┌───────────────────────────────────────────────────────────────────────┐   │
│ │ No unmanaged Claude or Antigravity sessions found.                    │   │
│ │ Sessions must be running locally and, for IDE terminals, wrapped      │   │
│ │ with ssq-mux.  [Learn how to set up ssq-mux →]                        │   │
│ └───────────────────────────────────────────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────────────────┘
```

Header checkbox (`aria-label="Select all discovered sessions"`) sits in the `[✓]` column header
(not drawn above for space) and drives the row checkboxes — tri-state `indeterminate` when some
but not all rows are checked, matching `ApprovalAnalyticsPanel.tsx` exactly.

The `[+ Add manual candidate...]` control (Phase 2) opens an inline row/small form (path input)
rather than a separate page — submitting it runs `PreviewImportExternalSession` with
`SourceKind: PlainTmux` and no PID, and on success the typed path becomes a new row in the same
table with a `[Plain tmux]` badge, per the git-GUI "untracked but visually distinguished" pattern.

### Interaction Flow

1. User opens the panel (behind `STAPLER_SQUAD_ENABLE_SESSION_IMPORT`; hidden entirely if the
   flag is off, not shown-but-disabled).
2. Panel mounts, calls discovery (mux `Discovery.Scan()` + plain-tmux enumeration), renders rows.
   Live updates (new session appears / disappears) patch the table in place; each change is
   announced via the `aria-live="polite"` count region ("4 external sessions found").
3. User either:
   - Clicks a single row's **Import** button → opens Surface 2 (preview dialog) for that row.
   - Checks 2+ row checkboxes → **Import N selected** bulk bar becomes visible/enabled →
     clicking it opens Surface 5 (batch progress).
   - Clicks **Add manual candidate…** → types a directory path → submits → new row appears with
     `[Plain tmux]` badge, `Ready` or `Needs choice` status depending on correlation result.
4. Row status is a visible column, one of: `Ready`, `Needs choice (N candidates)`,
   `Importing…`, `Imported`, `Import failed`, `No longer available`.

### Error / Edge-Case Handling

- **No sessions discovered**: empty state (shown above) replaces the table entirely — never a
  bare "0 rows" table. Names what was searched and links to `ssq-mux` setup docs.
- **Discovery source itself errors** (e.g. mux socket unreachable): a dismissible banner above
  the table — "Could not reach ssq-mux discovery: <error>. Plain-tmux and manual entry still
  work." — the whole panel does not go blank/unusable because one discovery source failed.
- **A selected row disappears mid-selection** (process exited while the checkbox was checked):
  row transitions to `No longer available` (distinct visual state, not silently removed) with a
  toast/inline note; it is automatically excluded from the in-flight batch selection count, and
  the user must acknowledge (a brief "1 session is no longer available and was skipped" line in
  the bulk bar) before the batch proceeds without it — never a silent renumbering that looks like
  nothing happened.
- **Ambiguous correlation (`Needs choice`)**: `Import` button remains present but the row cannot
  reach the "Confirm Import" state without visiting Surface 2's disambiguation sub-list first —
  clicking `Import` on a `Needs choice` row still opens the preview dialog, it just starts in the
  disambiguation-required state.

---

## Surface 2 — Import Preview / Confirm-Import Dialog

### Wireframe (resolved match)

```
┌─ role="dialog" aria-modal="true" aria-labelledby="import-preview-title" ────┐
│  Import Claude session from ~/proj/api?                    (title, focused) │
│  ──────────────────────────────────────────────────────────────────────────  │
│  Program:            Claude Code                                            │
│  Path:                ~/proj/api                                            │
│  Matched by:          Process ID   ← "Matched by process ID" text, always   │
│                                        shown, never omitted                 │
│  Turns:               42                                                    │
│  Last message:        "...let's add a retry with backoff to the client..." │
│                                                                               │
│                                          [ Cancel ]   [ Confirm Import ]     │
└───────────────────────────────────────────────────────────────────────────────┘
```

### Wireframe (ambiguous match — disambiguation sub-state)

```
┌─ role="dialog" aria-modal="true" ────────────────────────────────────────────┐
│  Import Claude session from ~/proj/web?                                     │
│  ──────────────────────────────────────────────────────────────────────────  │
│  Program:            Claude Code                                            │
│  Path:                ~/proj/web                                            │
│  Matched by:          ⚠ Best guess — 2 possible conversations found.        │
│                          Choose the correct one before importing:           │
│                                                                               │
│   ( ) session-a1b2.jsonl — last modified 3m ago                             │
│       "...fix the flaky test in auth_test.go..."                            │
│   ( ) session-c3d4.jsonl — last modified 40m ago                            │
│       "...refactor the login handler to use..."                             │
│                                                                               │
│                              [ Cancel ]   [ Confirm Import ] (disabled       │
│                                             until one option is chosen)      │
└───────────────────────────────────────────────────────────────────────────────┘
```

### Interaction Flow

1. Dialog opens (from Surface 1's per-row **Import** button), focus moves to the dialog title;
   `PreviewImportExternalSession` has already run (Surface 1 pre-fetches on hover/click) or runs
   on open with a brief loading state inside the dialog body (spinner + "Checking conversation
   history…" — never a blank dialog while waiting).
2. **Resolved case**: confidence text ("Matched by process ID" or "Matched by best guess — verify
   this is correct" for path-heuristic matches with only one candidate) is always shown, turn
   count and last-message excerpt are shown. **Confirm Import** is enabled immediately.
3. **Ambiguous case**: a radio-button sub-list of candidate JSONLs renders, each labeled with
   last-modified time and an excerpt of its last message. **Confirm Import** stays disabled until
   exactly one radio is selected.
4. **NotFound case**: dialog shows "No matching conversation history found for this session yet.
   You can still import — it will start with no prior history." with **Confirm Import** enabled
   (a valid, non-error state per the plan's `CorrelationResult.NotFound` semantics) but with a
   visually distinct (not red/error-colored — informational) notice, since this is expected for a
   session that hasn't written its JSONL yet.
5. User clicks **Confirm Import** → button enters a loading/disabled state ("Importing…") →
   `CommitImportExternalSession` (or `CommitImportExternalSession` with `disambiguation_choice`)
   is called.
6. **On success**: dialog closes, Surface 1's row status becomes `Imported`, and the new session
   appears in the main session list without a page reload. A toast/inline confirmation ("Imported
   as a new session") reinforces the "copy, not move" mental model from the research.
7. **On failure**: dialog stays open, shows an inline error at the bottom of the dialog body (not
   a separate toast that could be missed) with the specific error text and a **Retry** button
   that re-attempts commit without re-fetching preview or losing the user's disambiguation choice.

### Error / Edge-Case Handling

- **Correlation changes between preview and commit** (e.g. the JSONL that was `Resolved` at
  preview time is now ambiguous because a second file appeared): commit re-runs correlation
  server-side per the plan's double-checked-pattern decision; if the result changed, the dialog
  re-renders into the disambiguation sub-state with a note "The match changed since you opened
  this — please confirm again" rather than silently committing to a stale choice.
- **Commit succeeds but the row was deselected/closed elsewhere**: not applicable — this dialog
  is modal and blocks other row interactions while open (focus trap), so no race with itself.
- **Compensating-delete failure path** (start fails after DB row created): surfaces as a normal
  commit failure per point 7 above — the user never sees a "half-imported" state; from their
  perspective it is simply "Import failed: <reason>," retryable.

---

## Surface 3 — Per-Row Import Progress (single-item status machine)

### State diagram

```
 idle ──(click Import, preview opens)──> previewing ──(confirm)──> importing
   ▲                                                                   │
   │                                                    ┌──success────┤
   │                                                    │              │
   │                                              imported        (failure)
   │                                                    │              │
   │                                          (kill offered)      failed
   │                                                    │              │
   └────────────(dismiss error, retry)───────────────────┘◄────────────┘
```

### Wireframe (row states, rendered in Surface 1's Status/Action columns)

```
Ready            [ Import ]
Importing…       [spinner]                     (button disabled during commit)
Imported         [ End original session ]      (green check icon + text, never color-only)
Import failed    ⚠ "Import failed: <reason>"   [ Retry ]
No longer available   "Process exited before import" (no actions)
```

### Interaction Flow

- Each row is independently driven by its own dialog interactions (Surface 2) — there is no
  global "importing" lock across rows; a user can open a second row's preview while row 1 is
  still `Importing…` (matches the requirement that batch/individual items are independent).
- `Imported` is a terminal success state that unlocks exactly one new action: **End original
  session** (Surface 4). This button is not rendered at all — not merely disabled — for any row
  not yet in `Imported`, per the plan's explicit "not auto-offered before import succeeds"
  acceptance criterion.
- `Import failed` shows the specific error inline in the row (not just in the now-closed dialog)
  so the user doesn't have to remember or reopen anything to see why, plus a **Retry** button that
  reopens Surface 2 pre-filled with the same candidate.

### Error / Edge-Case Handling

- **Import succeeded, but the underlying session later becomes unreachable** (e.g. tmux server
  restarts): out of scope for this row's status — that is ordinary session-health surfaced by the
  existing session list, not a special import-row state.
- **Row's underlying external process exits while status is `Ready`**: row transitions to
  `No longer available`; if it was checked for a pending batch, see Surface 1's handling above.

---

## Surface 4 — Confirm-Before-Kill Dialog (single session, no undo)

### Wireframe

```
┌─ role="dialog" aria-modal="true" aria-labelledby="kill-confirm-title" ──────┐
│  End external session in ~/proj/api (PID 4821)?          (named target,     │
│  ──────────────────────────────────────────────────────  never generic)     │
│                                                                               │
│  This ends the original terminal/tmux session that was imported. This       │
│  cannot be undone. The imported session in stapler-squad is not affected.   │
│                                                                               │
│                              [ Cancel ] (initial focus)   [ End Session ]    │
│                                                              (--error token) │
└───────────────────────────────────────────────────────────────────────────────┘
```

### Interaction Flow

1. Rendered only from an `Imported` row's **End original session** button (Surface 3).
2. On open: focus moves into the dialog, landing on **Cancel** (never on the destructive button),
   per the repo's stricter-than-`ResumeSessionModal` destructive-dialog convention. `Escape`
   cancels and returns focus to the triggering **End original session** button.
3. User clicks **End Session** → button shows a brief loading state → `ConfirmKillExternalSession`
   re-verifies `PIDIdentity` server-side immediately before signaling, then acts.
4. **On `Killed`**: dialog closes, row shows "Session ended" (terminal state, no further actions).
5. **On `AlreadyGone`** (process exited on its own, or PID was reused by an unrelated process and
   the identity check caught it): dialog closes, row shows "The original session had already
   exited" — informational, not an error, since the user's goal (original session no longer
   running) is already satisfied.
6. **On `Failed`**: dialog closes; row shows a dismissible error "Import complete. Could not end
   the original session (process no longer found / permission denied) — you may need to close it
   manually." The row's `Imported` status is untouched — a kill failure never downgrades or
   rolls back the import.

### Error / Edge-Case Handling

- **External process already exited before kill-confirm is even opened**: the **End original
  session** button remains present (its absence would look like a bug — "why did the button
  disappear"); clicking it and confirming simply resolves to `AlreadyGone` per step 5 above, which
  is a graceful, expected outcome, not an error path the user needs to route around.
- **User double-clicks End Session**: button becomes disabled immediately on first click
  (`isSubmitting` guard, same pattern as `ResumeSessionModal.tsx`) — no duplicate kill RPC calls.

---

## Surface 5 — Batch Import Progress + Summary

### Wireframe (in progress)

```
┌─ Importing 3 selected sessions… ────────────────────────────────────────────┐
│                                                                               │
│  ✓  ~/proj/api      Imported                                                │
│  ⏳ ~/proj/web       Importing…                                             │
│  ·  ~/proj/infra     Waiting                                                │
│                                                                               │
└───────────────────────────────────────────────────────────────────────────────┘
```

### Wireframe (complete, mixed outcome)

```
┌─ Import Summary — 2 of 3 imported ──────────────────────────────────────────┐
│                                                                               │
│  ✓  ~/proj/api      Imported          [ End original session ]              │
│  ✓  ~/proj/infra    Imported          [ End original session ]              │
│  ✗  ~/proj/web      Failed: disambiguation required     [ Resolve & Retry ] │
│                                                                               │
│                                                             [ Close summary ]│
└───────────────────────────────────────────────────────────────────────────────┘
```

### Interaction Flow

1. Triggered from Surface 1's **Import N selected** bulk bar; opens as a dialog/panel (not a
   silent background operation) so the user watches per-item progress, matching the email-import
   wizard analog from research — never a single aggregate spinner.
2. Items process sequentially (per plan's non-parallel decision to avoid racing writers); each
   row transitions `Waiting → Importing… → Imported | Failed` live, each transition announced via
   `aria-live="polite"` ("2 of 3 imported so far").
3. On completion, the header becomes a summary ("2 of 3 imported") and every row exposes exactly
   the actions appropriate to its own outcome — success rows get **End original session**
   (Surface 6, one dialog per session, per the plan's resolved default); failed rows get
   **Resolve & Retry**, which reopens Surface 2 for just that one candidate without touching the
   others' already-committed state.
4. **Close summary** dismisses the panel; already-imported sessions remain in the main session
   list regardless (closing the summary never undoes anything).

### Error / Edge-Case Handling

- **Partial batch failure**: never all-or-nothing — successes are never rolled back because a
  later item failed (must-not-happen #6 from the plan). The summary line always states exact
  counts ("2 of 3 imported — 1 failed"), never a bare "Batch failed."
- **A row needing disambiguation with no choice supplied**: shows as `Failed: disambiguation
  required` with **Resolve & Retry**, not silently skipped and not blocking the other items in
  the batch — matches the plan's Story 3.1.1 acceptance criterion.
- **All items fail**: header still reads "0 of N imported" with every row's specific error visible
  — no generic "something went wrong."

---

## Surface 6 — Batch Kill-Confirmation

### Wireframe

Uses Surface 4's exact dialog shape, one instance per successfully-imported row (per the plan's
resolved Unresolved Question — one confirm dialog per session, not one dialog naming all of
them). The only batch-specific UI is *which rows offer the button at all*:

```
  ✓  ~/proj/api      Imported          [ End original session ]   ← offered
  ✓  ~/proj/infra     Imported          [ End original session ]   ← offered
  ✗  ~/proj/web       Failed             (no kill option at all)   ← never offered
```

### Interaction Flow

1. From Surface 5's summary, clicking any success row's **End original session** opens exactly
   the Surface 4 dialog for that one session (named target, initial focus on Cancel, no undo
   copy) — identical component, reused, not a new "kill all" bulk action.
2. There is intentionally no "End all N original sessions" bulk button in v1 — every kill remains
   individually confirmed, per the Slack-analog principle that killing something running
   elsewhere is never a bundled/bulk action a user could click through without seeing each named
   target.

### Error / Edge-Case Handling

- **Failed-import rows never render a kill affordance, under any state** — this is the
  must-not-happen #6 guard from the plan; verified structurally (component logic gates on
  `outcome.Status === Success`, not on any UI-only flag that could drift from the real outcome).
- **One kill in the batch fails while others succeed**: each row's kill outcome is independent,
  same handling as Surface 4 §"On Failed" — a kill failure on session A never blocks or reverts
  the kill (or import) of session B.

---

## UX Acceptance Criteria

### Task efficiency

1. A user can preview and import a single unambiguous mux-discovered session in ≤ 3 steps:
   (1) click **Import** on its row, (2) review preview, (3) click **Confirm Import**.
2. A user can end the original session after a successful import in ≤ 2 steps: (1) click
   **End original session**, (2) click **End Session** in the confirm dialog.
3. A user can select and import 5 discovered sessions in ≤ 3 steps: (1) check the header
   "select all" (or individual boxes), (2) click **Import N selected**, (3) watch batch progress
   to completion with no further input required for successful items.
4. A user resolving an ambiguous correlation can do so in the same dialog flow as a normal
   import — no separate page/modal — adding at most 1 extra step (select a radio option) versus
   the unambiguous path.

### Error and edge-case messaging

5. The empty discovery state names both searched sources (ssq-mux socket discovery, plain-tmux
   enumeration) and links to `ssq-mux` setup docs — never a bare "No sessions" message.
6. Every error state (commit failure, kill failure, discovery-source failure, ambiguous-without-
   choice, already-gone process) shows a specific, human-readable message naming what failed and
   offers a concrete next action (Retry, Resolve & Retry, dismiss, or "close manually" guidance)
   — no dead-end error with only an OK/dismiss button and no path forward.
7. No error state ever silently reverts or hides a prior success: an import failure never removes
   a previously-imported row from the session list; a kill failure never changes an `Imported`
   row's status; a partial batch failure never rolls back sibling successes.
8. Killing the wrong process is structurally prevented, not just discouraged: the kill dialog
   always names path + PID, always re-verifies process identity server-side immediately before
   signaling, and the destructive button is never the dialog's initial focus target.
9. No dead ends — every terminal error state (Import failed, kill Failed, discovery-source error)
   has at least one visible, reachable exit path: Retry, Resolve & Retry, dismiss-and-continue, or
   (for kill failures) explicit "close manually" guidance plus a working Dismiss control.

### Accessibility

10. Every interactive control (row checkbox, Import button, dialog buttons, radio options in the
    disambiguation sub-list) is reachable via `Tab`/`Shift+Tab` and operable via `Space`/`Enter`
    with no mouse.
11. Every dialog (`ImportPreviewDialog`, `ConfirmKillDialog`) sets `role="dialog"
    aria-modal="true"` with `aria-labelledby` pointing to a heading that names the specific
    target (path, PID, or program) — never a generic "Are you sure?" heading.
12. Focus moves into each dialog on open and returns to the triggering control on close/cancel/
    Escape, verified for both the preview dialog and the kill dialog.
13. The destructive action in `ConfirmKillDialog` is never the initial-focus element; initial
    focus is always **Cancel**.
14. Discovery-count changes, batch-progress transitions, and selection-count changes are each
    announced via a dedicated `aria-live="polite"` region — verified by at least one test per
    region asserting the announced text updates on the triggering event.
15. The header "select all" checkbox exposes tri-state selection via both the DOM
    `indeterminate` property and `aria-checked="mixed"` when some-but-not-all rows are selected.
16. No status is conveyed by color or icon alone (WCAG 1.4.1): every status icon (✓/✗/⏳/⚠) is
    paired with text in the same accessible name or adjacent visible text.
17. All destructive/status controls using `--error`/`--error-bg` tokens meet ≥ 4.5:1 contrast
    against their background, reusing the existing tokens rather than introducing new colors.
18. Every row, dialog, and control has a stable `data-testid` or accessible ARIA role/name
    sufficient for a Playwright locator with no reliance on CSS class or DOM position, per
    `.claude/rules/e2e-test-conventions.md`.

### Mental-model integrity ("import is copy, not move")

19. No single primary button ever combines import and kill in one action (no "Import & Close");
    the two are always temporally and visually separate steps, verified by absence of any control
    whose accessible name implies both actions.
20. The kill-confirmation checkbox/toggle, wherever it appears, is never pre-checked/defaulted to
    an active state — the user must explicitly opt in on every occurrence.
