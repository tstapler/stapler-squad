# Requirements: focus-trap-isactive-coverage

## Source

Backlog item `892af771-19c7-4f09-89d3-8fc7ab0c8c6f` — "useFocusTrap: isActive
hardcoded true at both modal call sites; no coverage for toggled-in-place
isActive". Surfaced as deferred follow-up during review of PR #638/#641
("useFocusTrap: restore focus to trigger element on modal close").

## Problem statement

`ReviewChangesModal.tsx` and `BacklogFileBrowserModal.tsx`
(`web-app/src/components/backlog/`) call `useFocusTrap(modalRef, true,
triggerRef)` with `isActive` hardcoded to the literal `true`. This is correct
today: both modals only ever mount-when-open / unmount-when-closed
(`{show && <Modal/>}` in `BacklogItemDetail.tsx`), so mount and "open" are
equivalent — there is no intermediate "mounted but inactive" state for
`isActive` to model.

Two related gaps, deliberately left as tracked debt rather than fixed now:

1. **Speculative derivation** — deriving `isActive` from open/visible state
   at these two call sites is YAGNI until a modal actually needs to stay
   mounted while closed (e.g. an exit-transition animation). Doing it now
   would be an unrequested abstraction with no second state to model.
2. **Untested code path** — `useFocusTrap.test.tsx` exercises `isActive` at
   activation and at unmount, but never exercises `isActive` toggling
   `true → false` on an **already-mounted, non-unmounting** container. The
   hook's own cleanup path (`useEffect` cleanup on `[isActive, ref,
   triggerRef]` change) already supports this case in production code — only
   the test coverage is missing.

## Investigation note (this triage)

This worktree's branch (`tstapler/triage-892af771-...`, based on commit
`687391ad4`) predates the commit that added `useFocusTrap` to these two
modals — on this branch, neither file calls `useFocusTrap` at all. Verified
against `origin/main` (commit `90925039d`) that both call sites and the
`{show && <Modal/>}` mount pattern described above are present and current.
This plan targets `origin/main`'s state, not this worktree's stale branch.

## Scope decision for this triage

Per the item's own "When to act" section, deriving `isActive` from open state
at the two modal call sites is **out of scope** until a real mount-while-closed
requirement exists — there is none today, and speculative work here is
exactly what this repo's interface-pollution/YAGNI conventions flag.

The **test coverage gap is in scope now**: it requires no production code
change (the hook already handles the toggle-without-unmount case correctly),
costs one test case, and closes a real regression blind spot — if a future
change ever wires a derived `isActive` into either modal without reading this
item, nothing today would catch a focus-return regression.

## Acceptance criteria

1. `useFocusTrap.test.tsx` has a test case that toggles `isActive` from
   `true` to `false` on an already-mounted container (no unmount) and asserts
   focus returns to `triggerRef.current`.
2. The new test follows the file's existing naming convention
   (`useFocusTrap_should_<effect>_When_<condition>`) and harness patterns
   (`TrapHarness`).
3. No production code in `useFocusTrap.ts`, `ReviewChangesModal.tsx`, or
   `BacklogFileBrowserModal.tsx` changes — this is a test-only addition since
   the underlying behavior is already correct.
4. `cd web-app && npx jest --no-coverage --testPathPatterns="useFocusTrap.test"`
   passes.
5. This backlog item's actual deferred work (deriving `isActive` from open
   state at the two modal call sites) remains explicitly un-actioned and
   documented as conditional on a future mount-while-closed requirement —
   not silently closed out by this test-only change.

## Non-goals

- Refactoring `ReviewChangesModal.tsx` / `BacklogFileBrowserModal.tsx` to
  derive `isActive`.
- Adding exit-transition/animation support to either modal (the trigger
  condition for the deferred work, not requested here).
