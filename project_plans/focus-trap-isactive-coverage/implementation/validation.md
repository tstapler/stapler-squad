# Validation: focus-trap-isactive-coverage

## Requirements → test coverage map

| Acceptance criterion | Covered by |
|---|---|
| 1. Test case toggling `isActive` true→false on mounted container asserts focus returns to trigger | New test, plan.md task 1 |
| 2. Naming/harness convention followed | Test name matches `useFocusTrap_should_<effect>_When_<condition>`; reuses `TrapHarness` unmodified |
| 3. No production code changes | Plan touches only `useFocusTrap.test.tsx` |
| 4. `npx jest --testPathPatterns="useFocusTrap.test"` passes | Plan task 2 |
| 5. Modal-level `isActive` derivation stays un-actioned, documented as conditional | requirements.md "Scope decision" section; no modal files in plan's task list |

No AC is left uncovered.

## Pre-mortem

**Failure mode 1: New test is flaky due to jsdom focus timing.**
Likelihood: low. The existing 3 unmount-based tests already assert
`document.activeElement` synchronously after an `act()` block with no
timers/async waits involved, and the new test follows the identical
synchronous pattern (`rerender` inside `act`, then a direct assertion) — no
new async surface is introduced.

**Failure mode 2: Test passes for the wrong reason (vacuous assertion).**
Addressed directly in plan.md's adversarial pass, Challenge 4 — confirmed the
pre-toggle active element is the container's first focusable child, not the
trigger, so the toggle produces a real, checkable state transition.

**Failure mode 3: Someone later reads this closed-out test as "the modal
derivation work is done" and skips it when an exit-transition need arises.**
Mitigated by requirements.md's explicit non-goals section and by leaving the
original backlog item's "When to act" trigger condition unmodified — this
triage only closes the test-coverage gap, not the item itself.

## Gate

All 5 ACs map to a concrete, checkable step. No blocking unknowns. Ready to
implement as a single ~30-minute task.
