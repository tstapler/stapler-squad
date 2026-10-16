# Research: focus-trap-isactive-coverage

Scoped research — this is a one-test-case, no-production-diff change, so the
usual 6-dimension parallel fan-out is skipped in favor of direct inspection
(proportionality: this repo's CLAUDE.md, "Proportionality" section).

## 1. Current implementation (`web-app/src/lib/hooks/useFocusTrap.ts`, origin/main)

```ts
export function useFocusTrap(ref, isActive, triggerRef?) {
  useEffect(() => {
    if (!isActive || !ref.current) return;
    // ... wire up Tab-cycling keydown listener ...
    const triggerEl = triggerRef?.current;
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      triggerEl?.focus();   // <-- runs on ANY dep change, not just unmount
    };
  }, [isActive, ref, triggerRef]);
}
```

Key fact: the effect's cleanup fires whenever `isActive` (or `ref`/`triggerRef`
identity) changes, **not only on unmount**. React runs cleanup before every
re-run of an effect whose deps changed, so `isActive: true → false` on a
mounted component already exercises the same `triggerEl?.focus()` cleanup
path as unmounting — no code change needed to support it. This is standard
`useEffect` semantics, not something specific to this hook.

## 2. Existing test coverage (`useFocusTrap.test.tsx`, origin/main)

10 test cases, all either:
- activation behavior (`isActive` true from first render), or
- **unmount**-triggered cleanup (`unmount()` called), or
- Tab-cycling behavior while active.

None re-render the harness with `isActive={false}` while the container stays
mounted. `TrapHarness` (the reusable harness, see requirements.md) already
accepts `isActive` as a prop, so re-rendering it with a new value via
`rerender()` (from RTL's `render()` return) is sufficient — no new harness
needed.

## 3. Call site survey (confirms requirements.md's premise)

`grep -rn "useFocusTrap(" web-app/src` on `origin/main`:
- `ReviewChangesModal.tsx:34` — `useFocusTrap(modalRef, true, triggerRef)`
- `BacklogFileBrowserModal.tsx:41` — `useFocusTrap(modalRef, true, triggerRef)`
- 8 other call sites elsewhere in the app already pass a real boolean
  (`showOverflow`, `isRestartConfirmOpen`, `isOpen`, etc.) — i.e. deriving
  `isActive` from state is the established pattern everywhere except these
  two literal-`true` sites, which is exactly what the backlog item flags.

Both modals are rendered via `{show && <Modal/>}` in `BacklogItemDetail.tsx`
(`showFileBrowser && latestWorkSession && <BacklogFileBrowserModal .../>`,
similarly for `ReviewChangesModal`), so mount and open are currently
equivalent — the literal `true` is behaviorally correct today.

## 4. Risk if left as-is

None today. The risk is latent: if either modal is later changed to stay
mounted across open/close (e.g. `visible ? modal : null` inside a
CSS-transition wrapper, instead of conditional mount), `isActive` would need
to become `visible` — and without this test, a regression in focus-return
during that refactor would not be caught by any existing test, since none of
the 10 current cases toggle `isActive` without unmounting.

## 5. Recommendation

Add the one test case now (cheap, no risk, closes a real blind spot).
Do not touch the two modal call sites — no second state exists for them to
model yet; derivation is speculative until an exit-transition requirement
lands, per this repo's YAGNI/interface-pollution convention.
