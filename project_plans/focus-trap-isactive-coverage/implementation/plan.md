# Plan: focus-trap-isactive-coverage

## Architecture

None needed — no new abstraction, no new file, no production code change.
Single new test case in an existing file, using the existing `TrapHarness`.

## Task breakdown

| # | Task | File | Est. |
|---|---|---|---|
| 1 | Add `useFocusTrap_should_RestoreFocusToTrigger_When_IsActiveTogglesFalseWithoutUnmount` test: render `TrapHarness` with `isActive triggerRef={triggerRef}`, `rerender` with `isActive={false}`, assert `document.activeElement === trigger` (mirrors the existing unmount test's assertion, but via rerender instead of `unmount()`) | `web-app/src/lib/hooks/useFocusTrap.test.tsx` | 20m |
| 2 | Run `cd web-app && npx jest --no-coverage --testPathPatterns="useFocusTrap.test"` and confirm all 11 cases pass | — | 5m |
| 3 | Run `cd web-app && npx jest --no-coverage --testPathPatterns="ReviewChangesModal\|BacklogFileBrowserModal"` to confirm no incidental regression from touching a shared hook's test file | — | 5m |

Total: ~30 minutes.

## Test sketch

```tsx
it("useFocusTrap_should_RestoreFocusToTrigger_When_IsActiveTogglesFalseWithoutUnmount", () => {
  const trigger = document.createElement("button");
  document.body.appendChild(trigger);
  const triggerRef = { current: trigger as HTMLElement | null };

  const { rerender } = render(<TrapHarness isActive triggerRef={triggerRef} />);

  act(() => {
    rerender(<TrapHarness isActive={false} triggerRef={triggerRef} />);
  });

  // Container is still mounted — only isActive flipped. Same cleanup path
  // as unmount (React re-runs effect cleanup on any dep change), asserted
  // here explicitly so a future refactor that special-cases unmount vs.
  // deactivation would be caught.
  expect(document.activeElement).toBe(trigger);
  trigger.remove();
});
```

## Adversarial review pass

**Challenge 1: "Is this test redundant with the existing unmount test?"**
No — `useEffect` cleanup on dep-change vs. cleanup on unmount are the same
React mechanism today, but nothing in the hook's contract guarantees they
stay implemented identically (e.g. a future rewrite to
`useLayoutEffect`-plus-manual-unmount-detection, or an early-return guard
keyed off a mount ref, could special-case one and not the other). The test
pins current behavior against exactly the scenario the backlog item names.

**Challenge 2: "Should the modal call sites be changed anyway, since it's a
two-line diff?"**
No — rejected per requirements.md's scope decision. There is no second state
for `isActive` to model at either call site today; wiring one in now is
exactly the "speculative interface"-shaped work this repo's
`interface-pollution-checklist.md` and YAGNI rung flag. Changing it now with
no consumer would also leave it untested by *this* task's own new test until
a modal actually renders with `isActive=false` while mounted — the test
would still only be exercised through the synthetic harness.

**Challenge 3: "Does `rerender` need to go through `act()`?" **
RTL's `render`/`rerender` already wrap state updates in `act` internally in
recent versions, but the existing file wraps `unmount()` in `act(() => {...})`
explicitly (see the 3 existing tests), so match that convention for
consistency rather than relying on implicit wrapping.

**Challenge 4: "What if `document.activeElement` is already `trigger` before
the toggle, making the assertion vacuous?"**
It isn't: `TrapHarness`'s effect moves focus to the *first focusable child*
on activation (`first?.focus()`), so at the point of `rerender`,
`document.activeElement` is the container's `first` button, not `trigger`.
The assertion after toggling to `isActive={false}` is a real state change,
not a no-op check.

## Rollback

Revert the single test-file diff. No production code touched, no migration,
no feature flag.
