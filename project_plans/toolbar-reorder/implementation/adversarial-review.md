# Adversarial Review: toolbar-reorder

**Date**: 2026-05-30
**Verdict**: CONCERNS

---

## Blockers

*(None)*

---

## Concerns

- [ ] **Plan's final button order contradicts story requirement** — The parent task spec says the reorder should be: `Copy, Paste, Bottom, Clear → Gallery, Files → Mouse`. The plan's "Final Button Layout Reference" matches this. However, Task 3.1.1a's "Chosen approach" moves Paste *inside* the `secondaryGroup` div at its *start*, then describes `secondaryActions` as `[copy, bottom, clear, mouse]`. The resulting render order inside `secondaryGroup` would then be: `[Paste (hardcoded first)] [Copy] [Bottom] [Clear] [Mouse]` — which gives Paste position 1 and Copy position 2, matching the spec. But the plan text also says "Insert the Paste button JSX at the **start** of the secondary group div (before the `secondaryActions.map()`), and reorder `secondaryActions` to `[copy, bottom, clear, mouse]`." This is internally consistent but creates an implementation trap: the `mobileOverflowRow` at line 1394–1395 also uses `secondaryActions.map(...)`, so Paste will be **absent from the mobile overflow row** (it's hardcoded in the secondary group which is hidden on mobile). The plan acknowledges Resize disappearing from mobile overflow but **does not acknowledge Paste disappearing from mobile overflow**. This is a missing behavior on mobile that must be called out explicitly and either accepted or mitigated.

- [ ] **`vars.color.borderColor` confirmed valid but plan warns unnecessarily** — The adversarial grep shows `vars.color.borderColor` is already used at line 115 of `TerminalOutput.css.ts`. The plan's "Note: check theme-contract.css.ts" warning is inaccurate — the token exists and works. While this is just a note, it could mislead the implementor into unnecessary checking. Recommend removing the caveat or correcting it to say "token confirmed valid."

- [ ] **Paste button moves from `toolbarActions` (outside secondaryGroup) into `secondaryGroup` — must verify no test queries by position** — The `upload.test.tsx` tests the Paste button's behavior indirectly (clipboard API). If any test queries `screen.getByRole("button", { name: /paste from clipboard/i })` and then traverses the DOM to check a parent container, the new parent being `secondaryGroup` (which has `data-testid="toolbar-secondary"`) instead of `toolbarActions` would break. Review `upload.test.tsx` Paste-related tests before implementing.

- [ ] **Analytics test file plan has a placeholder `defaultProps` stub** — Task 4.1.2a says `const defaultProps = { /* ... copy minimal props from existing test files */ }`. This is not a concrete step. The test will not compile until someone fills in actual props. The plan should either (a) call out the exact props to copy from `logstream.test.tsx` by line number, or (b) note that this is the implementor's first step.

- [ ] **`dev_group` analytics track call happens in `handleDevGroupToggle` which is also defined in Task 2.1.1a AND Task 2.1.3a** — The plan defines `handleDevGroupToggle` twice: once in Task 2.1.1a (correctly with `track()`) and again in Task 2.1.3a (as a reference `onClick={handleDevGroupToggle}`). This is fine as intent but could cause an implementor to create two function definitions. Make clear that Task 2.1.1a creates the handler and Task 2.1.3a only *uses* it.

- [ ] **Story numbering discrepancy** — The parent task spec uses Story 1/2/3/4 with Tasks 1.1, 2.1, 2.2, 2.3, 3.1, 3.2, 3.3, 4.1, 4.2. The plan uses a different numbering (Epic 1.1, Story 1.1.1, Task 1.1.1a etc.). This makes it harder to verify task-for-task coverage vs. the original requirements. Low impact but worth noting.

---

## Minors

- The `devGroupButton` CSS export is described as "optional, can be extended here if needed" but the code snippet shows `export const devGroupButton = style([])` — an empty array composition. This will compile but lint tools may flag an unused export. Consider either removing it or not including it in the initial plan.

- The plan says "Log Stream variable is `remoteDebugEnabled`" in the JSX snippet for Task 2.1.3a, but the actual source code (line 1257) uses `logStreamEnabled`. Implementors should use `logStreamEnabled`, not `remoteDebugEnabled`.

- The `kill` and `resize-icon` buttons are listed in Task 1.1.1a for analytics instrumentation, and listed in the acceptance criteria list (15 buttons). However, those buttons are outside the `toolbarExpanded` section (they're always-visible). The plan correctly notes they're in "always-visible layer" in the research. No plan change needed, but the implementor should note these buttons are in a different JSX location than the dev/secondary/upload buttons.

- The `camera` button analytics (Task 1.1.1c) is listed in the 15-button AC, but the analytics test stub (Task 4.1.2a) does not have a camera test written out. Since Camera is mobile-only via CSS pointer media query and JSDOM doesn't evaluate that query (the button IS in the DOM in tests), a `camera` analytics test should be feasible and should be included.

- The plan recommends `setTimeout(() => devToggleRef.current?.focus(), 0)` for focus management. `setTimeout(..., 0)` is a smell in React — prefer `flushSync` + `focus()` or just `requestAnimationFrame`. Not a blocker but worth updating.
