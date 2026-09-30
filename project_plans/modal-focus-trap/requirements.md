# Requirements: modal-focus-trap

**Source**: Backlog item `ca77e4b7-fcea-4f12-9fcc-e328c84b6f06` — "Backlog modals (ReviewChangesModal, BacklogFileBrowserModal) don't trap keyboard focus"
**Date**: 2026-08-29

## Problem

The backlog's review/diff modals don't trap keyboard focus, letting `Tab`/`Shift+Tab` move
focus out of an open modal into the backgrounded page — a WCAG 2.1.2 (No Keyboard Trap,
intent inverted here: focus escapes when it shouldn't) / 2.4.3 (Focus Order) violation.

`ReviewChangesModal.tsx:59-68` handles `Escape` in its keydown listener but never intercepts
`Tab`/`Shift+Tab`, despite declaring `role="dialog" aria-modal="true"` (lines 82-84) —
`aria-modal="true"` asserts to assistive tech that focus *is* contained, which isn't true
here. `BacklogFileBrowserModal.tsx:62` has the same pattern: initial focus is set on mount
but nothing constrains it afterward.

The codebase already has a shared `useFocusTrap` hook
(`web-app/src/lib/hooks/useFocusTrap.ts`) adopted by 7 components/pages
(`ResumeSessionModal`, `WorkspaceSwitchModal`, `TagEditor`, `SessionActionsOverflow`,
`DebugMenu`, `app/page.tsx`'s delete-confirm dialog, `app/review-queue/page.tsx`'s modal —
corrected 2026-08-30 by grep, the prior count of 5 missed the two page-level adopters).
Verified by inspection (2026-08-29): the hook computes its focusable-element
snapshot once per activation inside the effect body (not on each keypress), so any adopter
whose content changes after mount would get a partial trap.

## Ask

1. Add a focus-trap effect cycling `Tab`/`Shift+Tab` within each modal's focusable elements
   for `ReviewChangesModal` and `BacklogFileBrowserModal` — reuse the existing
   `useFocusTrap` hook rather than hand-rolling a new one or adding a dependency.
2. Audit every other backlog-scoped modal declaring `aria-modal="true"` and apply the same
   fix to any that don't yet trap focus.

Verified by grep (2026-08-29), the full set of `aria-modal="true"` components under
`web-app/src/components/backlog/` and `web-app/src/components/unfinished/` not already on
`useFocusTrap`:

| Component | Current focus handling |
|---|---|
| `ReviewChangesModal.tsx` | Escape only; mount-focus effect; no Tab trap |
| `BacklogFileBrowserModal.tsx` | Escape only; mount-focus effect; no Tab trap |
| `VaguenessPromptModal.tsx` | Hand-rolled Tab-cycling `onKeyDown` (2-button dialog); no Escape by design |
| `GateVerdictBox.tsx` (skip-confirm `alertdialog`) | Hand-rolled Tab-cycling `handleSkipConfirmKeyDown`; manual trigger-focus restore |
| `CommitPushModal.tsx` | Escape + Cmd/Ctrl+Enter only; no Tab trap |
| `WorktreeDiffModal.tsx` | Escape only; zero refs; no Tab trap |
| `BacklogQueueSection.tsx` (import dialog) | No Tab trap, no Escape handling at all |

## Acceptance Criteria

1. `ReviewChangesModal` traps `Tab`/`Shift+Tab` within its dialog via `useFocusTrap`;
   existing `Escape`-to-close behavior is unchanged. **Amendment (2026-08-30):** already true
   on `main` by the time this branch was rebased onto it — someone else's independent work
   wired this in (with a `triggerRef` prop this project's original diff didn't have), see
   AC4's amendment for detail. No behavior change needed here; this project adds AC5's test
   coverage, which didn't exist yet.
2. `BacklogFileBrowserModal` traps `Tab`/`Shift+Tab` within its dialog via `useFocusTrap`;
   existing `Escape`-to-close behavior is unchanged. Same amendment as AC1.
3. Every other backlog-scoped modal/dialog with `aria-modal="true"` that doesn't already
   trap focus (`VaguenessPromptModal`, `GateVerdictBox`'s skip-confirm alertdialog,
   `CommitPushModal`, `WorktreeDiffModal`, `BacklogQueueSection`'s import dialog) is wired
   onto the same shared `useFocusTrap` hook, replacing any hand-rolled equivalent.
4. `useFocusTrap` re-queries focusable elements on each `Tab` keypress (not just once on
   activation) so modals whose content loads asynchronously after mount (e.g.
   `ReviewChangesModal`, `WorktreeDiffModal`, `BacklogFileBrowserModal`) get a fully
   correct trap. **Amendment (2026-08-30, rebuilt on current main):** this fix already
   existed on `main` independently by the time this branch was rebased onto it (along with
   `useFocusTrap` already being wired into `ReviewChangesModal` and `BacklogFileBrowserModal`
   themselves, AC1/AC2) — confirmed via `useFocusTrap.ts`'s `getFocusable()` re-query and its
   dedicated `useFocusTrap.test.tsx` suite (11 tests, including a `MutableSetHarness`-based
   regression test for exactly this re-query behavior). This item's actual remaining AC3 work
   was wiring the other 5 backlog/unfinished modals onto it. Adopter count as of this rebase:
   11 pre-existing (`app/page.tsx`, `app/review-queue/page.tsx`,
   `sessions/ConfirmKillDialog.tsx`, `sessions/CreatePullRequestModal.tsx`,
   `sessions/ImportPreviewDialog.tsx`, `sessions/MoveToMenu.tsx`,
   `sessions/ResumeSessionModal.tsx`, `sessions/SessionActionsOverflow.tsx`,
   `sessions/TagEditor.tsx`, `sessions/WorkspaceSwitchModal.tsx`, `ui/DebugMenu.tsx`) + 7
   from this item (`ReviewChangesModal`, `BacklogFileBrowserModal`, `VaguenessPromptModal`,
   `GateVerdictBox`, `CommitPushModal`, `WorktreeDiffModal`, `BacklogQueueSection`) = 18 total,
   all showing no behavior change.
5. New/updated automated tests (Jest unit tests per component, plus e2e Tab-loop coverage
   per `tests/e2e/accessibility.spec.ts`'s existing pattern for the two named modals) prove
   focus cannot leave the open dialog via keyboard — a regression here must fail CI, not
   just an Axe static scan (which cannot detect a Tab-escape).

   **Amendment (2026-08-30, revised same day after review PARTIAL verdict, then again after
   rebasing onto main — see AC4's amendment above):** `ReviewChangesModal.test.tsx` and
   `BacklogFileBrowserModal.test.tsx` already mock `useFocusTrap` entirely on main (its own
   Tab-wrap behavior has a dedicated 11-test suite, `useFocusTrap.test.tsx`, run against the
   real hook) — so rather than re-test Tab-wrap at the component level, each gets a spy-based
   test (`ReviewChangesModal_should_activateFocusTrapOnItsDialogRef_When_
   ReturningFocusToSuppliedTrigger` / the equivalent in `BacklogFileBrowserModal.test.tsx`)
   proving the component wires `useFocusTrap` onto its own dialog ref with the trap active
   and the caller's `triggerRef` passed through — what's actually this component's own
   responsibility, as opposed to the hook's. `useFocusTrap.test.tsx` also gained the missing
   "exactly one focusable element" edge case
   (`useFocusTrap_should_KeepFocusOnSoleElement_When_OnlyOneFocusableElementExists`) — the
   zero-element case was already covered, but not this one.

   `ReviewChangesModal` additionally has real Playwright coverage
   (`tests/e2e/accessibility.spec.ts`'s "Tab wraps within ReviewChangesModal instead of
   escaping to the page (modal-focus-trap AC5)") — its real "View Changes" trigger
   (`ReviewingSection.tsx`) only needs a truthy work session, reachable via a small new debug
   seed endpoint (`handleSeedWorkItemSession`, mirroring the existing
   `handleSeedHeadlessTriageSession` pattern almost verbatim) with no backing Session/Worktree
   DB row.

   **Second revision (2026-08-30, same day, after a second PARTIAL verdict):**
   `BacklogFileBrowserModal` now also has real Playwright coverage. Its real trigger
   (`VcsWidgetHeader`'s "Browse files in this worktree" button) needs a linked work session
   with a truthy `worktreePath` — unlike `ReviewChangesModal`'s trigger, this requires a real
   Session+Worktree DB row (resolved via `GetWorktreeDataBySessionUUID`, which joins to an
   actual `ent.Session`/`ent.Worktree` row, not just `ItemSession`) — built via a second debug
   endpoint (`handleSeedWorkSessionWithWorktree`: a real `git init`-ed temp directory plus a
   `Storage.CreateInstanceData` write).

   That test intentionally does **not** do a full forward Tab-loop the way
   `ReviewChangesModal`'s does. Running one against the real (unmocked) `FileTree` surfaced a
   genuine, pre-existing, out-of-scope bug: `FileTree` embeds a real `react-arborist` `<Tree>`
   whose own re-renders rewrite a tree row's `tabindex` attribute to `"-1"` the instant that
   row receives real DOM focus — so a generic focus-trap's `[tabindex]:not([tabindex="-1"])`
   re-query (`useFocusTrap.ts`'s own technique) loses track of "first"/"last" once focus enters
   the tree, and in a longer Tab-loop this occasionally dropped focus to `document.body`
   (outside the dialog entirely). That's a bug in `FileTree`/react-arborist's own focus
   bookkeeping — not in this fix's `useFocusTrap` wiring, which is what AC1/AC2 scope this fix
   to — so it's filed as its own follow-up rather than fixed here: backlog item
   `4a1f73c4-5558-41f8-9860-8508fb874fcc`. The Playwright test instead proves the one thing
   squarely in scope against the real `FileTree`: `useFocusTrap` moves focus to the dialog's
   first focusable element (the "Open in Terminal" link) on activation.
6. `cd web-app && npx jest --no-coverage` passes for every touched file.

## Out of Scope

- The unrelated flaky `accessibility.spec.ts` test (#424) and the SessionDetail a11y
  regression (#311) — different components, different root causes.
- Making the rest of the page `inert`/`aria-hidden` while a modal is open — a separate,
  larger architectural change, not required to close the Tab-escape gap.
- Migrating any modal to a new dependency (`focus-trap-react`, native `<dialog>`, etc.) —
  the existing in-house hook already solves this.

### Known Out-of-Scope Modals (same bug, not fixed by this project)

**Amendment (2026-08-30, re-verified against current main after this branch was rebuilt on
top of it):** verified via `comm -23 <(grep -rl 'aria-modal="true"' web-app/src | sort)
<(grep -rl 'useFocusTrap(' web-app/src --include='*.tsx' | grep -v test | sort)` — 16
components/pages currently declare `aria-modal="true"` without `useFocusTrap` wired in yet,
the identical untrapped-Tab defect this item fixes, but sit outside
`web-app/src/components/{backlog,unfinished}/` and are **not** touched by this fix (grown
from 14 to 16 since this branch's original count, as other unrelated main-line work added
`settings/BackwardSyncConfirmDialog.tsx`, `settings/HostKeyTrustDialog.tsx`, and
`sessions/BoardCompleteConfirmDialog.tsx`):

`app/backlog/page.tsx`, `app/history/page.tsx`, `insights/SessionDetailDrawer.tsx`,
`pane/PaneTilingContainer.tsx`, `history/HistoryMessagesModal.tsx`,
`shared/ShortcutHelpOverlay.tsx`, `rules/TemplateLibrary.tsx`,
`sessions/BoardCompleteConfirmDialog.tsx`, `sessions/NewShellDialog.tsx`,
`sessions/ReviewQueuePanel.tsx`, `sessions/SessionPeekModal.tsx`, `sessions/Omnibar.tsx`,
`settings/BackwardSyncConfirmDialog.tsx`, `settings/HostKeyTrustDialog.tsx`,
`ui/KeyboardShortcutOverlay.tsx`, `ui/NotificationPanel.tsx`.

This project closes the backlog/unfinished-page slice of the WCAG violation, not the
app-wide issue. **File a follow-up backlog item** enumerating this list before this item is
considered to have resolved keyboard-trap accessibility globally.

## Suggested Entry Point

`/sdd:fix-bug`
