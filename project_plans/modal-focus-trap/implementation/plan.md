# Implementation Plan: modal-focus-trap

**Feature**: Wire `ReviewChangesModal`, `BacklogFileBrowserModal`, and 5 other `aria-modal="true"` backlog/unfinished-work components onto the existing `useFocusTrap` hook so `Tab`/`Shift+Tab` can no longer move focus into the backgrounded page while a dialog is open.
**Date**: 2026-08-29
**Status**: Ready for implementation
**ADRs**: [ADR-001: Re-query focusable elements on each Tab keypress in `useFocusTrap`](../decisions/ADR-001-focus-trap-requery-on-tab.md)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `useFocusTrap(ref, isActive, triggerRef?)` | Existing shared hook (`web-app/src/lib/hooks/useFocusTrap.ts`) that, while `isActive` is true, focuses the first focusable descendant of `ref.current` and cycles `Tab`/`Shift+Tab` between the first and last focusable descendants. | No new hook is built — this project only wires 7 more call sites onto it, plus one internal fix (see ADR-001). |
| `ref` (container ref) | A `RefObject<HTMLElement \| null>` pointing at the dialog/modal's outer DOM node — the boundary the trap cycles within. | Named `modalRef`, `dialogRef`, or `skipConfirmDialogRef`/`importDialogRef` per component below, matching each file's existing naming style. |
| `isActive` | Boolean passed to the hook gating whether its `document`-level `keydown` listener is attached. | Either "component only mounts while open" (`true` literal, e.g. `ReviewChangesModal`) or a real boolean state variable for dialogs nested in an always-mounted parent (e.g. `showSkipConfirm` for `GateVerdictBox`, `showImport` for `BacklogQueueSection`). |
| `triggerRef` | Optional `RefObject` to the element that opened the dialog; the hook focuses it back when the trap deactivates. | Only added where a single stable opening control exists (`GateVerdictBox`'s `skipLinkRef`). Omitted elsewhere per Pattern Decision #4 below. |
| focusable element | Any element matching the hook's `FOCUSABLE_SELECTORS` (`a[href]`, non-disabled `button`/`input`/`select`/`textarea`, or `[tabindex]` not `"-1"`) that isn't inside an `aria-hidden="true"` ancestor. | Defined once in the hook; not reimplemented per component. |
| focus trap | The behavior this project adds: while a dialog is `isActive`, `Tab` from the last focusable element moves to the first, and `Shift+Tab` from the first moves to the last — focus cannot leave the dialog via keyboard. | The WCAG 2.4.3 (Focus Order) fix; see `ux.md`. |
| stale snapshot | The pre-fix bug in the hook: the focusable-element list was captured once per activation and never updated, so components that render new focusable controls after mount (async-loaded content) got a trap that silently excluded those controls. | Fixed by ADR-001; not a per-component concern once the hook is patched. |
| Tab-loop pattern | The existing e2e technique at `tests/e2e/accessibility.spec.ts:242-280`: press `Tab` in a loop and assert `document.activeElement` to prove real keyboard reachability, since Axe's static scan cannot detect a Tab-escape. | Reused, not reinvented, for the two new e2e assertions in Phase 4. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Core wiring (all 7 targets) | Adopt existing `useFocusTrap` hook — a reusable-behavior hook, functionally the codebase's Strategy-via-hook pattern (swap in the same focus-management algorithm at each call site without inheritance) | `build-vs-buy.md`, `architecture.md` | (a) `focus-trap-react` / `focus-trap` npm package; (b) migrate to native `<dialog>`; (c) hand-roll a second local trap per component | (a) new dependency with no justification, explicitly out of scope per requirements.md; (b) the portal + vanilla-extract backdrop architecture makes this a multi-component rewrite, not a bug fix — worth its own future ADR, not bundled here; (c) duplicates a shared abstraction the codebase already has and 5 components already use, directly against the requirements' explicit ask to reuse it |
| `useFocusTrap` internals | Re-query focusable elements inside `handleKeyDown` on every `Tab` keypress instead of snapshotting once per activation (ADR-001) | `pitfalls.md` #1, `build-vs-buy.md` | (a) leave the one-time snapshot and accept a partial trap for async-content modals; (b) re-run the full effect via a `MutationObserver` on the container | (a) leaves a real correctness gap in 3 of 7 new adopters (`ReviewChangesModal`, `BacklogFileBrowserModal`, `WorktreeDiffModal`) that this project would otherwise ship knowingly; (b) more moving parts (observer lifecycle, disconnect timing) than the problem needs — `GateVerdictBox`'s existing hand-rolled trap already proves the simpler per-keypress requery works in this codebase |
| `GateVerdictBox` skip-confirm alertdialog | Replace the hand-rolled `handleSkipConfirmKeyDown` Tab-cycling branch + mount-focus effect + manual `skipLinkRef.current?.focus()` Escape call with `useFocusTrap(skipConfirmDialogRef, showSkipConfirm, skipLinkRef)` | `features.md`, `pitfalls.md` #3 | Keep the working hand-rolled trap untouched since "it already works" | Requirements AC3 asks all 7 targets to "trap focus the same way" — leaving one hand-rolled implementation creates two divergent focus-trap code paths in the codebase for future maintainers to reconcile, and (per pitfalls.md #3) any partial removal here creates two competing Tab handlers running simultaneously, which is worse than either alone |
| `VaguenessPromptModal` | Replace the hand-rolled 2-button `handleKeyDown` Tab-cycling branch + `refineButtonRef.current?.focus()` mount effect with `useFocusTrap(dialogRef, true)`; keep the deliberate no-Escape design untouched | `features.md` | Keep hand-rolled trap | Same reasoning as `GateVerdictBox` — one shared implementation, not two; `useFocusTrap` has no Escape handling of its own so the "no Escape by design" behavior is unaffected by the swap |
| `triggerRef` usage | Add `triggerRef` only where one stable, single opening control exists (`GateVerdictBox`'s `skipLinkRef`); omit it for the other 6 targets | `architecture.md`, `SessionActionsOverflow.tsx:153-159` (5 of 8 calls omit `triggerRef`) | Force-add an artificial trigger ref to every component (e.g. wrap `BacklogQueueSection`'s import button in a new ref just to satisfy symmetry) | `ReviewChangesModal`/`BacklogFileBrowserModal` have 2 ambiguous openers in `BacklogItemDetail.tsx`, `CommitPushModal`/`WorktreeDiffModal` weren't audited for a stable trigger, and `VaguenessPromptModal` opens as a side effect of triage logic, not a click — omitting `triggerRef` in all 5 matches the codebase's own established precedent rather than manufacturing refs with no behavioral payoff |
| `BacklogQueueSection` import dialog Escape-to-close | Do not add Escape-to-close as part of this fix | `features.md` | Add Escape handling while the file is already being touched | Not literally asked for by requirements.md ("Add a focus-trap effect… Apply the same fix to any other backlog modal"); adding it is scope creep beyond the stated ask and is flagged here as a deliberate deferral, not an oversight |

---

## Migration Plan

N/A — no schema, data, or persisted-state changes. This is a client-side keyboard-interaction fix only.

## Observability Plan
- **Logs**: None added. No new server-side code path; this is pure client-side focus management.
- **Metrics**: None added. Focus-trap correctness is verified by tests (Jest/RTL unit tests + Playwright Tab-loop e2e), not runtime telemetry.
- **Alerts**: None. No production-facing failure mode this introduces would page anyone — worst case on a bug is a UX regression (focus escapes again), which is caught by the e2e Tab-loop assertions in CI before merge.

## Risk Control
- **Feature flag**: None. This is a behavior-correctness fix to existing, always-on UI (the modals already render `aria-modal="true"`); there is no user-facing toggle to gate it behind, and gating an accessibility fix behind a flag would leave the WCAG violation live for flagged-off users with no benefit.
- **Rollback procedure**: Standard `git revert` of the merged PR. Each component's change is a small, self-contained diff (add a ref + one hook call + delete a superseded effect/handler) with no cross-component coupling except the shared hook change in Phase 1, so a partial revert (e.g. keep the hook fix, revert one component's wiring) is also safe if a single target regresses.
- **Staged rollout**: Not applicable — ships as a normal PR through CI (Jest + Playwright + lint) like any other frontend change. No canary/percentage rollout mechanism exists for this app's web UI.

## Unresolved Questions

None outstanding. Every decision point research surfaced (hook stale-snapshot fix vs. accept, `GateVerdictBox`/`VaguenessPromptModal` replace-vs-keep, `triggerRef` inclusion, `BacklogQueueSection` Escape scope) was resolved above in Pattern Decisions, using the recommendations in `pitfalls.md`, `build-vs-buy.md`, and `architecture.md`. The one item research flagged as "worth a manual/e2e check, not a code fix" — whether a sibling modal's trigger stays reachable while another modal is open — is carried forward as Task 4.1.2a (a verification task, not an open question blocking a story).

## Dependency Visualization

```
Phase 1: Shared Hook Fix
  Task 1.1.1a (re-query focusable elements in handleKeyDown, per ADR-001)
  Task 1.1.1b (regression check: 7 existing adopters (5 components + 2 page-level dialogs, app/page.tsx and app/review-queue/page.tsx)' test suites)
        │
        ├──────────────────────────────┬───────────────────────────────────┐
        ▼                              ▼                                   │
Phase 2: Named Modals (parallel)  Phase 3: Remaining 5 Modals (parallel)    │
  Story 2.1.1 ReviewChangesModal    Story 3.1.1 VaguenessPromptModal        │
    Task 2.1.1a, 2.1.1b               Task 3.1.1a, 3.1.1b                   │
  Story 2.1.2 BacklogFileBrowserModal Story 3.1.2 GateVerdictBox            │
    Task 2.1.2a, 2.1.2b               Task 3.1.2a, 3.1.2b                   │
                                     Story 3.1.3 CommitPushModal             │
                                       Task 3.1.3a, 3.1.3b                  │
                                     Story 3.1.4 WorktreeDiffModal           │
                                       Task 3.1.4a, 3.1.4b                  │
                                     Story 3.1.5 BacklogQueueSection         │
                                       Task 3.1.5a, 3.1.5b                  │
        │                              │                                   │
        └──────────────┬───────────────┘                                   │
                        ▼                                                  │
        Phase 4: Test Coverage (needs Phase 2 done; Phase 3 informs 4.1.2) ◄┘
          Task 4.1.1a  e2e Tab-wrap — ReviewChangesModal
          Task 4.1.1b  e2e Tab-wrap — BacklogFileBrowserModal
          Task 4.1.2a  manual/e2e check — sibling modal reachability
                        │
                        ▼
        Phase 5: Feature Registry (needs all component + test tasks done)
          Task 5.1.1a  create/update per-feature registry JSON
          Task 5.1.1b  make registry-generate; verify coverage-gaps
          Task 5.1.1c  full `cd web-app && npx jest --no-coverage` gate (AC6)
```

---

## Phase 1: Shared Hook Fix

### Epic 1.1: Fix the stale focusable-element snapshot in `useFocusTrap`
**Goal**: Make `useFocusTrap` re-query focusable descendants on every `Tab` keypress instead of once per activation, so components with async-loaded content (3 of the 7 new adopters) get a fully correct trap instead of a partial one — see ADR-001.

#### Story 1.1.1: `useFocusTrap` tracks newly-rendered focusable elements without a remount
**As a** developer wiring `useFocusTrap` into a modal whose content loads asynchronously, **I want** the hook to always cycle Tab among the *current* set of focusable descendants, **so that** controls rendered after mount (e.g. a diff viewer's retry/refresh buttons) are reachable via keyboard instead of silently excluded from the trap.

**Acceptance Criteria**:
- The hook's `Tab`/`Shift+Tab` cycling reflects the container's focusable elements at the moment of each keypress, not just at activation time.
  - *Given* `ReviewChangesModal` is open, activated with `useFocusTrap(modalRef, true)`, and the diff fetch is still `loading` (so the only focusable elements are `[closeButton]`, since `sessionId` is undefined in this example so no terminal link renders), *When* the diff fetch resolves successfully 400ms later and `DiffRenderer` renders a "Refresh" button as a new focusable descendant of `modalRef.current`, and the user then presses `Tab` while focus is on `closeButton`, *Then* focus moves to the newly-rendered "Refresh" button (the current last focusable element), not to background page content — proving the hook re-queried rather than using the stale mount-time snapshot of `[closeButton]` only.
- The 7 existing static-content adopters (`ResumeSessionModal`, `WorkspaceSwitchModal`, `TagEditor`, `SessionActionsOverflow`, `DebugMenu`) show no behavior change.
  - *Given* `SessionActionsOverflow`'s delete-confirm dialog is open via `useFocusTrap(deleteDialogRef, isDeleteConfirmOpen)` with its static 2-button set `[Cancel, Delete]`, *When* the existing Jest test suite for that component is run before and after the hook change, *Then* all assertions pass identically (no new failures, no changed focus-order expectations).
**Files**: `web-app/src/lib/hooks/useFocusTrap.ts`

##### Task 1.1.1a: Move focusable-element query into `handleKeyDown` (~4 min)
- In `useFocusTrap.ts`, move the `container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS)` call and the `.filter(...)`, `first`, `last` derivations (currently lines 26-31, computed once in the effect body) into the top of `handleKeyDown` (currently lines 36-53), so they run fresh on every `Tab` keypress. Keep the one-time `first?.focus()` call on activation (line 34) as-is, using a local computation at effect-setup time — only the *keypress-time* first/last need to be live.
- Confirm `container` (currently `ref.current as HTMLElement`, captured once) is still valid to reuse inside `handleKeyDown` since the container element itself doesn't change across the trap's lifetime, only its descendants do.
- Files: `web-app/src/lib/hooks/useFocusTrap.ts`

##### Task 1.1.1b: Regression-check the 7 existing adopters (5 components + 2 page-level dialogs, app/page.tsx and app/review-queue/page.tsx) (~3 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="ResumeSessionModal|WorkspaceSwitchModal|TagEditor|SessionActionsOverflow|DebugMenu"` and confirm all pre-existing suites still pass unchanged after Task 1.1.1a.
- Files: none changed (verification only)

---

## Phase 2: Named Modals (Primary Ask)

### Epic 2.1: Wire `ReviewChangesModal` and `BacklogFileBrowserModal` onto `useFocusTrap`
**Goal**: Satisfy AC1 and AC2 — the two modals explicitly named in the backlog item trap `Tab`/`Shift+Tab` via the shared hook, with the pre-existing redundant mount-focus effect removed so it doesn't race the hook's own `first?.focus()`.

#### Story 2.1.1: `ReviewChangesModal` traps focus within its dialog
**As a** keyboard user reviewing a backlog item's diff, **I want** `Tab` to stay confined to the "Review Changes" dialog, **so that** I can't accidentally tab into the backgrounded page behind the modal backdrop.

**Acceptance Criteria**:
- `Tab` from the last focusable element wraps to the first; `Shift+Tab` from the first wraps to the last.
  - *Given* `ReviewChangesModal` is open with `sessionId` set, so focusable elements in DOM order are `[Open in Terminal link, Close button (✕)]` while the diff is still `loading` (no `DiffRenderer` buttons rendered yet), *When* the user presses `Tab` while focus is on the Close button (currently the last focusable element), *Then* focus moves to the Open in Terminal link (the first focusable element), not to background page content behind the `backdrop` div.
  - *Given* the same state, *When* the user presses `Shift+Tab` while focus is on the Open in Terminal link (the first focusable element), *Then* focus moves to the Close button (the last focusable element).
- `Escape`-to-close is unchanged.
  - *Given* `ReviewChangesModal` is open, *When* the user presses `Escape`, *Then* `onClose()` fires exactly as it does today via the existing `window.addEventListener("keydown", handleKeyDown, { capture: true })` effect (lines 59-68) — untouched by this change.
- `cd web-app && npx jest --no-coverage --testPathPatterns="ReviewChangesModal"` passes.
**Files**: `web-app/src/components/backlog/ReviewChangesModal.tsx`, `web-app/src/components/backlog/__tests__/ReviewChangesModal.test.tsx`

##### Task 2.1.1a: Wire `useFocusTrap` and remove the redundant mount-focus effect (~4 min)
- Add `import { useFocusTrap } from "@/lib/hooks/useFocusTrap";`.
- Add `useFocusTrap(modalRef, true);` (the existing `modalRef` at line 30 is already attached to the dialog `<div>` at line 80 — no new ref needed, no `triggerRef` per Pattern Decision "trigger ref usage" above).
- Delete the now-redundant `useEffect(() => { modalRef.current?.focus(); }, []);` at lines 70-72 — it races the hook's own `first?.focus()` call in the same commit; the hook's focus-on-activation subsumes it.
- Files: `web-app/src/components/backlog/ReviewChangesModal.tsx`

##### Task 2.1.1b: Add Tab-wrap assertions to the existing Jest suite (~5 min)
- In `web-app/src/components/backlog/__tests__/ReviewChangesModal.test.tsx`, add a test asserting: render the modal, `fireEvent.keyDown` with `key: "Tab"` on `document` while focus is on the last focusable element, assert `document.activeElement` is the first focusable element; repeat for `Shift+Tab` from the first element wrapping to the last.
- Files: `web-app/src/components/backlog/__tests__/ReviewChangesModal.test.tsx`

#### Story 2.1.2: `BacklogFileBrowserModal` traps focus within its dialog
**As a** keyboard user browsing a session's worktree files, **I want** `Tab` to stay confined to the file browser dialog, **so that** I can't accidentally tab into the backgrounded page behind the modal backdrop.

**Acceptance Criteria**:
- `Tab` from the last focusable element wraps to the first; `Shift+Tab` from the first wraps to the last.
  - *Given* `BacklogFileBrowserModal` is open with focusable elements in DOM order `[Open in Terminal link, Close button (✕), first file/folder row rendered by FileTree]`, *When* the user presses `Shift+Tab` while focus is on the Open in Terminal link (the first focusable element), *Then* focus moves to the last focusable element (the last interactive row `FileTree` has rendered), not to background page content.
- `Escape`-to-close is unchanged (existing `window.addEventListener` effect at lines 50-59, untouched).
- `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogFileBrowserModal"` passes.
**Files**: `web-app/src/components/backlog/BacklogFileBrowserModal.tsx`, `web-app/src/components/backlog/__tests__/BacklogFileBrowserModal.test.tsx`

##### Task 2.1.2a: Wire `useFocusTrap` and remove the redundant mount-focus effect (~4 min)
- Add `import { useFocusTrap } from "@/lib/hooks/useFocusTrap";`.
- Add `useFocusTrap(modalRef, true);` (the existing `modalRef` at line 38 is already attached to the dialog `<div>` at line 71).
- Delete the redundant `useEffect(() => { modalRef.current?.focus(); }, []);` at lines 61-63 for the same reason as Task 2.1.1a.
- Files: `web-app/src/components/backlog/BacklogFileBrowserModal.tsx`

##### Task 2.1.2b: Add Tab-wrap assertions to the existing Jest suite (~5 min)
- In `web-app/src/components/backlog/__tests__/BacklogFileBrowserModal.test.tsx`, add the same Tab/Shift+Tab wrap assertions as Task 2.1.1b, adapted to this modal's focusable set (mock `FileTree`/`FileContentViewer` minimally if the existing suite doesn't already, so a stable, small focusable set can be asserted against).
- Files: `web-app/src/components/backlog/__tests__/BacklogFileBrowserModal.test.tsx`

---

## Phase 3: Remaining Backlog-Scoped Modals (Audit Ask)

### Epic 3.1: Apply the same fix to the 5 additional `aria-modal="true"` components identified in the audit
**Goal**: Satisfy AC3 — every other backlog-scoped modal/dialog with `aria-modal="true"` (or `role="alertdialog"` + `aria-modal="true"`) that doesn't yet trap focus via `useFocusTrap` is brought onto the same shared hook.

#### Story 3.1.1: `VaguenessPromptModal` traps focus via `useFocusTrap` (replacing its hand-rolled trap)
**As a** keyboard user who just created a vague backlog item, **I want** `Tab` to stay confined to the "refine or proceed" dialog, **so that** I can't tab past the two explicit choices into the page behind it.

**Acceptance Criteria**:
- `Tab`/`Shift+Tab` wraps between the dialog's focusable elements using the shared hook instead of the current hand-rolled `handleKeyDown` branch.
  - *Given* `VaguenessPromptModal` is open with focusable elements `[refineButtonRef ("Add more detail"), proceedButtonRef ("Run triage anyway")]`, *When* the user presses `Tab` while focus is on the "Run triage anyway" button (the last/only-other focusable element), *Then* focus moves to the "Add more detail" button (the first focusable element) — same externally-observable behavior as today's hand-rolled trap (lines 33-45), now produced by `useFocusTrap(dialogRef, true)` instead.
- No-Escape-dismissal design is preserved (this dialog forces an explicit choice).
  - *Given* the dialog is open, *When* the user presses `Escape`, *Then* nothing happens — the dialog stays open, since `useFocusTrap` has no Escape handling of its own and none is added.
- `cd web-app && npx jest --no-coverage --testPathPatterns="VaguenessPromptModal"` passes.
**Files**: `web-app/src/components/backlog/VaguenessPromptModal.tsx`, `web-app/src/components/backlog/VaguenessPromptModal.test.tsx`

##### Task 3.1.1a: Replace the hand-rolled trap with `useFocusTrap` (~5 min)
- Add `import { useFocusTrap } from "@/lib/hooks/useFocusTrap";` and `const dialogRef = useRef<HTMLDivElement>(null);`.
- Attach `ref={dialogRef}` to the `<div role="dialog" ...>` at line 49; add `useFocusTrap(dialogRef, true);` (no `triggerRef` — this dialog opens as a side effect of triage logic in `app/backlog/page.tsx` ~line 945, not a captured click handler, per Pattern Decision "trigger ref usage").
- Delete the `handleKeyDown` function (lines 33-45) and its `onKeyDown={handleKeyDown}` wiring on the dialog `<div>` (line 54).
- Delete the mount-focus effect (`useEffect(() => { refineButtonRef.current?.focus(); }, []);`, lines 28-30) — the hook's own `first?.focus()` targets the same `refineButtonRef` element since it's the first focusable descendant in DOM order.
- Files: `web-app/src/components/backlog/VaguenessPromptModal.tsx`

##### Task 3.1.1b: Update the existing Jest suite for the new wiring (~4 min)
- In `web-app/src/components/backlog/VaguenessPromptModal.test.tsx`, adjust any test that dispatched `keydown` on the dialog element directly (the old `onKeyDown` handler) to instead dispatch on `document` (the hook's listener target), and confirm the no-Escape-dismissal assertion still passes unmodified.
- Files: `web-app/src/components/backlog/VaguenessPromptModal.test.tsx`

#### Story 3.1.2: `GateVerdictBox`'s skip-confirm alertdialog traps focus via `useFocusTrap` (replacing its hand-rolled trap)
**As a** keyboard user about to skip a gate review (an irreversible action — "This cannot be undone"), **I want** `Tab` to stay confined to the confirm/cancel dialog, **so that** I can't accidentally tab away from the confirmation prompt.

**Acceptance Criteria**:
- `Tab`/`Shift+Tab` wraps between `[Cancel, Confirm — Skip Gate]` using the shared hook instead of the current hand-rolled `handleSkipConfirmKeyDown` Tab branch.
  - *Given* the skip-confirm alertdialog is open (`showSkipConfirm === true`) with focusable elements `[cancelRef ("Cancel"), confirmRef ("Confirm — Skip Gate")]`, *When* the user presses `Tab` while focus is on the "Confirm — Skip Gate" button (the last focusable element), *Then* focus moves to the "Cancel" button (the first focusable element) via `useFocusTrap(skipConfirmDialogRef, showSkipConfirm, skipLinkRef)`, replacing the equivalent behavior currently produced by lines 221-238 of `handleSkipConfirmKeyDown`.
- Focus restores to the trigger element (`skipLinkRef`, the "Skip gate and mark done without review" button) on close, and `Escape`-to-close is unchanged in effect (though its restore-focus call is now delegated to the hook — see below).
  - *Given* the alertdialog is open, *When* the user presses `Escape`, *Then* `setShowSkipConfirm(false)` still fires (kept as a dedicated `Escape`-only branch in `handleSkipConfirmKeyDown`, since `useFocusTrap` has no Escape handling), and focus returns to `skipLinkRef` — now via the hook's cleanup (`triggerEl?.focus()`) rather than the current manual `skipLinkRef.current?.focus()` call at line 218, which must be deleted to avoid a double-focus race.
  - **Behavior change to call out in the PR** (flagged per `features.md`, not silently folded in): today, focus only auto-restores to `skipLinkRef` on `Escape`; after this change it also auto-restores on Cancel/Confirm button clicks, since those close paths now go through the same `showSkipConfirm` state transition the hook observes. This is a net improvement (consistent restore behavior across all three close paths) but is broader than the literal "add a focus trap" ask.
- `cd web-app && npx jest --no-coverage --testPathPatterns="GateVerdictBox"` passes.
**Files**: `web-app/src/components/backlog/GateVerdictBox.tsx`, `web-app/src/components/backlog/GateVerdictBox.test.tsx`

##### Task 3.1.2a: Replace the hand-rolled trap with `useFocusTrap` (~5 min)
- Add `import { useFocusTrap } from "@/lib/hooks/useFocusTrap";` and `const skipConfirmDialogRef = useRef<HTMLDivElement>(null);` alongside the existing refs at lines 105-108.
- Attach `ref={skipConfirmDialogRef}` to the `<div role="alertdialog" ...>` at line 485; add `useFocusTrap(skipConfirmDialogRef, showSkipConfirm, skipLinkRef);`.
- In `handleSkipConfirmKeyDown` (lines 215-240): delete the entire `if (e.key === "Tab") { ... }` branch (lines 221-239) and the `skipLinkRef.current?.focus();` call in the `Escape` branch (line 218) — keep only `if (e.key === "Escape") { setShowSkipConfirm(false); return; }`.
- Delete the mount-focus effect for `cancelRef` (lines 130-135) — the hook's `first?.focus()` targets the same `cancelRef` element since it's the first focusable descendant.
- Files: `web-app/src/components/backlog/GateVerdictBox.tsx`

##### Task 3.1.2b: Update the existing Jest suite for the new wiring and the flagged behavior change (~5 min)
- In `web-app/src/components/backlog/GateVerdictBox.test.tsx`, adjust any test dispatching `keydown` Tab events on the alertdialog element to dispatch on `document` instead; add/confirm a test that Cancel and Confirm clicks now also restore focus to the skip-link trigger (the flagged behavior change above), alongside the existing Escape-restores-focus test.
- Files: `web-app/src/components/backlog/GateVerdictBox.test.tsx`

#### Story 3.1.3: `CommitPushModal` traps focus via `useFocusTrap`
**As a** keyboard user committing and pushing changes from the Unfinished page, **I want** `Tab` to stay confined to the commit dialog, **so that** I can't accidentally tab into the backgrounded page while a commit message is in progress.

**Acceptance Criteria**:
- `Tab`/`Shift+Tab` wraps between the dialog's focusable elements, where today there is no Tab handling at all (only `Escape` and Cmd/Ctrl+Enter, lines 64-67).
  - *Given* `CommitPushModal` is open with focusable elements in DOM order `[commit-message textarea, Cancel button, Commit & Push button]`, *When* the user presses `Tab` while focus is on the "Commit & Push" button (the last focusable element), *Then* focus moves to the commit-message textarea (the first focusable element), not to background page content — closing a genuine gap (no prior Tab handling existed).
- `Escape`-to-close and Cmd/Ctrl+Enter-to-submit remain unchanged (both live in the existing `handleKeyDown`, lines 64-67, untouched).
- `cd web-app && npx jest --no-coverage --testPathPatterns="CommitPushModal"` passes.
**Files**: `web-app/src/components/unfinished/CommitPushModal.tsx`, `web-app/src/components/unfinished/CommitPushModal.test.tsx` (new file — none exists today)

##### Task 3.1.3a: Wire `useFocusTrap` on the inner modal div (~4 min)
- Add `import { useFocusTrap } from "@/lib/hooks/useFocusTrap";` and `const modalRef = useRef<HTMLDivElement>(null);`.
- Attach `ref={modalRef}` to the **inner** `<div className={styles.modal}>` at line 78 (not the outer `overlay`/backdrop div at line 70-77 — the overlay is just the click-to-dismiss backdrop, not the dialog boundary); add `useFocusTrap(modalRef, true);` (no `triggerRef` — not audited for a stable trigger button, per Pattern Decision).
- Confirm the `textareaRef` (line 26, focused via the effect at lines 28-30) is the first focusable element in DOM order inside `modalRef` — it is (the textarea at line 87-95 precedes the Cancel/Commit buttons at lines 101-111) — then delete the now-redundant `useEffect(() => { textareaRef.current?.focus(); }, []);` (lines 28-30), since the hook's `first?.focus()` subsumes it.
- Files: `web-app/src/components/unfinished/CommitPushModal.tsx`

##### Task 3.1.3b: Create a new Jest test file covering the trap and existing behavior (~5 min)
- Create `web-app/src/components/unfinished/CommitPushModal.test.tsx` (matching the co-located `*.test.tsx` convention already used by `GateVerdictBox.test.tsx`/`VaguenessPromptModal.test.tsx` in this same tree) covering: renders with textarea focused on mount, `Tab` from Commit & Push wraps to the textarea, `Shift+Tab` from the textarea wraps to Commit & Push, `Escape` calls `onClose`, Cmd/Ctrl+Enter calls the submit handler when a message is present.
- Files: `web-app/src/components/unfinished/CommitPushModal.test.tsx`

#### Story 3.1.4: `WorktreeDiffModal` traps focus via `useFocusTrap`
**As a** keyboard user reviewing an unfinished session's worktree diff, **I want** `Tab` to stay confined to the diff dialog, **so that** I can't accidentally tab into the backgrounded page.

**Acceptance Criteria**:
- `Tab`/`Shift+Tab` wraps between the dialog's focusable elements, where today there are no refs and no Tab handling at all (only a global `document` `Escape` listener, lines 53-57).
  - *Given* `WorktreeDiffModal` is open, the diff fetch has completed successfully, and focusable elements in DOM order are `[Close button (✕), Refresh button, view-mode toggle button]` (from `DiffRenderer` once `loading` is false), *When* the user presses `Tab` while focus is on the view-mode toggle button (the last focusable element, only reachable because of the ADR-001 hook fix in Phase 1), *Then* focus moves to the Close button (the first focusable element), not to background page content.
- `Escape`-to-close remains unchanged (lines 53-57, untouched).
- `cd web-app && npx jest --no-coverage --testPathPatterns="WorktreeDiffModal"` passes.
**Files**: `web-app/src/components/unfinished/WorktreeDiffModal.tsx`, `web-app/src/components/unfinished/WorktreeDiffModal.test.tsx` (new file — none exists today)

##### Task 3.1.4a: Add a container ref and wire `useFocusTrap` (~4 min)
- Add `import { useFocusTrap } from "@/lib/hooks/useFocusTrap";` and `const modalRef = useRef<HTMLDivElement>(null);`.
- Attach `ref={modalRef}` to the inner `<div className={styles.modal}>` at line 67 (not the outer `overlay` div at lines 60-66); add `useFocusTrap(modalRef, true);` (no `triggerRef`, same reasoning as `CommitPushModal`).
- No existing focus-management code to remove here — this component had zero refs and zero Tab handling before this change (the cleanest of the 7 targets, per `features.md`).
- Files: `web-app/src/components/unfinished/WorktreeDiffModal.tsx`

##### Task 3.1.4b: Create a new Jest test file covering the trap and existing Escape behavior (~5 min)
- Create `web-app/src/components/unfinished/WorktreeDiffModal.test.tsx` covering: `Tab` from the last focusable element (Close button, when diff hasn't loaded and `DiffRenderer` renders no extra controls yet) wraps to the first, and separately (mocking a resolved diff fetch) that a `DiffRenderer`-rendered control is reachable — directly exercising the ADR-001 requery fix — plus `Escape` still calls `onClose`.
- Files: `web-app/src/components/unfinished/WorktreeDiffModal.test.tsx`

#### Story 3.1.5: `BacklogQueueSection`'s import dialog traps focus via `useFocusTrap`
**As a** keyboard user importing a GitHub issue from the "Up Next" section, **I want** `Tab` to stay confined to the import dialog, **so that** I can't accidentally tab into the backgrounded page.

**Acceptance Criteria**:
- `Tab`/`Shift+Tab` wraps between the dialog's focusable elements, where today there are no refs, no Tab handling, and no Escape handling at all (the most incomplete of the 7 targets, per `features.md`).
  - *Given* the import dialog is open (`showImport === true`) rendering `<GitHubIssuePicker onSelect={...} onCancel={...} />`, which always renders at least one input/button in every branch (confirmed by inspection — zero-focusable is theoretical only here), *When* the user presses `Shift+Tab` while focus is on the first focusable element inside the dialog, *Then* focus wraps to the last focusable element rendered by `GitHubIssuePicker`, not to background page content.
- No Escape-to-close is added (Pattern Decision above: out of scope for this fix, default to not adding it).
- `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogQueueSection"` passes.
**Files**: `web-app/src/components/unfinished/BacklogQueueSection.tsx`, `web-app/src/components/unfinished/BacklogQueueSection.test.tsx`

##### Task 3.1.5a: Add a container ref and wire `useFocusTrap` (~4 min)
- Add `import { useFocusTrap } from "@/lib/hooks/useFocusTrap";` and `const importDialogRef = useRef<HTMLDivElement>(null);` alongside the existing `headingId`/`loadRequestIdRef` declarations (~line 56-59).
- Attach `ref={importDialogRef}` to the `<div role="dialog" aria-modal="true" ...>` at line 170; add `useFocusTrap(importDialogRef, showImport);` (no `triggerRef` — matches 5/8 `SessionActionsOverflow` instances that omit it, keeps the diff minimal per Pattern Decision).
- Files: `web-app/src/components/unfinished/BacklogQueueSection.tsx`

##### Task 3.1.5b: Add Tab-wrap assertions to the existing Jest suite (~4 min)
- In `web-app/src/components/unfinished/BacklogQueueSection.test.tsx`, add a test that opens the import dialog (click "+ Import GitHub Issue", `data-testid="import-github-issue-button"`), then asserts `Tab`/`Shift+Tab` wraps among `GitHubIssuePicker`'s rendered focusable elements (mock `GitHubIssuePicker` to a minimal stable set — e.g. one input + Cancel button — if the existing suite doesn't already).
- Files: `web-app/src/components/unfinished/BacklogQueueSection.test.tsx`

---

## Phase 4: Test Coverage — Close the CI Detection Gap

### Epic 4.1: Add Playwright Tab-loop coverage for the two named modals; verify sibling-modal reachability
**Goal**: Satisfy AC5. Per `ux.md`, the blocking Axe Core CI gate (`.github/workflows/ux-analysis.yml`, `continue-on-error: false`) never opens any of these 6 modals and cannot structurally detect a Tab-escape even if it did — only a hand-written `page.keyboard.press('Tab')` loop (the existing pattern at `tests/e2e/accessibility.spec.ts:242-280`) exercises real keyboard reachability. Without a new e2e test using that pattern, a regression here would not be caught by CI.

#### Story 4.1.1: e2e tests prove `Tab` wraps within `ReviewChangesModal` and `BacklogFileBrowserModal`
**As a** developer relying on CI to catch a regression in this fix, **I want** an e2e test that presses `Tab` in a loop inside each named modal and asserts on `document.activeElement`, **so that** a future change that breaks the trap fails CI instead of shipping silently (the gap `ux.md` identified: Axe's static scan cannot detect this).

**Acceptance Criteria**:
- A Playwright test opens `ReviewChangesModal` from `BacklogItemDetail` and proves `Tab` wraps from the last to the first focusable element.
  - *Given* a Playwright test creates a review-status backlog item, opens its detail view, and clicks the control that opens `ReviewChangesModal`, *When* the test presses `Tab` in a loop (matching the up-to-30-iteration pattern at `tests/e2e/accessibility.spec.ts:266-272`) starting from whatever element currently has focus, *Then* `page.evaluate(() => document.activeElement)` eventually reports the modal's first focusable element as active, and continuing to press `Tab` from there cycles back through the same set rather than moving focus outside the modal's DOM subtree (asserted by checking the active element's closest `[role="dialog"]` ancestor matches the modal).
- The same proof for `BacklogFileBrowserModal`.
  - *Given* the same backlog item detail view, *When* the test opens the file browser and repeats the same Tab-loop assertion, *Then* focus wraps within the file browser dialog's focusable set the same way.
- Both tests are added to `tests/e2e/accessibility.spec.ts` (reusing its existing `@feature backlog:item-detail` annotation and Tab-loop helper pattern, rather than a new spec file — this file is the established home for this exact technique).
**Files**: `tests/e2e/accessibility.spec.ts`

##### Task 4.1.1a: e2e Tab-wrap test for `ReviewChangesModal` (~5 min)
- Add a test inside `tests/e2e/accessibility.spec.ts`'s `Accessibility (WCAG 2.1 AA)` describe block: create a review-status backlog item via `createBacklogItemDirect`, navigate to its detail view via `BacklogPage`, open `ReviewChangesModal`, then apply the Tab-loop pattern (lines 266-272 style) to assert wrap-around within the dialog.
- Files: `tests/e2e/accessibility.spec.ts`

##### Task 4.1.1b: e2e Tab-wrap test for `BacklogFileBrowserModal` (~5 min)
- Add the equivalent test for `BacklogFileBrowserModal`, opened from the same or a similarly-seeded backlog item detail view.
- Files: `tests/e2e/accessibility.spec.ts`

#### Story 4.1.2: Verify sibling-modal trigger reachability is not silently broken
**As a** reviewer of this change, **I want** confirmation that `ReviewChangesModal` and `BacklogFileBrowserModal` — independent boolean states (`showChangesModal`/`showFileBrowser`) with no shared mutual-exclusion enum in `BacklogItemDetail.tsx` — don't produce a confusing double-trap if both are somehow opened at once, **so that** this fix doesn't introduce a new interaction hazard even though it's an accepted pattern elsewhere (`SessionActionsOverflow` already runs 8 simultaneous `useFocusTrap` instances).

**Acceptance Criteria**:
- Manual or e2e check confirms the sibling modal's trigger button is not reachable / has no effect while one of the two is open, rather than assuming it structurally can't happen.
  - *Given* `ReviewChangesModal` is open, *When* a tester (manually, or via a Playwright check) attempts to activate the button that would open `BacklogFileBrowserModal` while `ReviewChangesModal`'s backdrop is covering it, *Then* the attempt has no effect (the backdrop intercepts the click, or the trigger is not in the Tab order reachable from within the open modal's trap) — confirming no second simultaneous `document`-level Tab listener is reachable via keyboard from inside the first modal.
- No code change results from this task unless the check reveals a real gap — if it does, that gap is out of scope for this fix (per requirements.md: "do not silently 'fix' this via aria-hidden/inert on the rest of the page, that's a separate larger change") and should be filed as a new backlog item instead.
**Files**: None (verification task; a new backlog item only if a gap is found)

##### Task 4.1.2a: Manual verification and PR note (~3 min)
- Manually open `ReviewChangesModal`, confirm the "Files" trigger behind the backdrop is inert; repeat with `BacklogFileBrowserModal` open and the "Review Changes" trigger. Note the result in the PR description per requirements.md's guidance (recommend a check, don't silently "fix" via aria-hidden/inert).
- Files: none (PR description only)

---

## Phase 5: Feature Registry

### Epic 5.1: Register the touched frontend features per `.claude/rules/feature-registry.md`
**Goal**: Keep `docs/registry/features/frontend/` accurate for the components this project modifies, and confirm the full Jest suite still passes across all touched files (AC6).

#### Story 5.1.1: Registry reflects modal-focus-trap test coverage; full Jest suite gate passes
**As a** future contributor consulting the feature registry, **I want** the components this project touches to have accurate `tested`/`testIds` entries, **so that** `make registry-diff`/`coverage-gaps.json` reflect reality.

**Acceptance Criteria**:
- `BacklogQueueSection`'s existing registry entry gains the new focus-trap test IDs.
  - *Given* `docs/registry/features/frontend/ui/unfinished-backlog-queue.json` currently lists 8 `testIds` for `BacklogQueueSection` and `tested: true`, *When* Task 5.1.1a runs, *Then* the file gains the new Tab-wrap test's name (matching Task 3.1.5b) appended to `testIds` and an updated `lastModified` timestamp.
- The 6 other touched components (`ReviewChangesModal`, `BacklogFileBrowserModal`, `VaguenessPromptModal`, `GateVerdictBox`'s host component, `CommitPushModal`, `WorktreeDiffModal`), which have no pre-existing per-feature frontend registry file today (confirmed by inspection — only `BacklogQueueSection` has one, via `unfinished-backlog-queue.json`), each get a minimal new file following the `alias-manager.json`/`unfinished-backlog-queue.json` shape (`id`, `type: "frontend"`, `component`, `path`, `tested: true`, `testIds` listing the new Tab-wrap test names).
- `make registry-generate` runs clean and `docs/registry/coverage-gaps.json`'s count does not grow (adding `tested: true` entries can only shrink or hold the gap count steady, never grow it).
- `cd web-app && npx jest --no-coverage` passes for every file touched in Phases 1-3 (AC6, the overarching gate for this project).
**Files**: `docs/registry/features/frontend/ui/unfinished-backlog-queue.json`, 6 new files under `docs/registry/features/frontend/ui/` (one per remaining touched component), `docs/registry/backend-features.json`/`frontend-features.json`/`coverage-gaps.json` (generated, not hand-edited)

##### Task 5.1.1a: Create/update per-feature registry JSON files (~5 min)
- Update `docs/registry/features/frontend/ui/unfinished-backlog-queue.json`: append the new test name from Task 3.1.5b to `testIds`, bump `lastModified`.
- Create 6 new minimal entries (one per `ReviewChangesModal`, `BacklogFileBrowserModal`, `VaguenessPromptModal`, `GateVerdictBox`, `CommitPushModal`, `WorktreeDiffModal`) under `docs/registry/features/frontend/ui/`, each with `tested: true` and `testIds` populated from the corresponding Phase 2/3 test tasks.
- Files: `docs/registry/features/frontend/ui/unfinished-backlog-queue.json` + 6 new files

##### Task 5.1.1b: Run `make registry-generate` and verify no coverage-gap growth (~3 min)
- Run `make registry-generate`, diff `docs/registry/coverage-gaps.json` before/after, confirm the count does not increase.
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json` (generated)

##### Task 5.1.1c: Full Jest gate (~3 min)
- Run `cd web-app && npx jest --no-coverage` and confirm zero failures across the full suite, closing out AC6.
- Files: none (verification only)
