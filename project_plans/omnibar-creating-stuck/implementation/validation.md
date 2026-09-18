# Validation Plan: omnibar-creating-stuck

**Date**: 2026-08-11
**Complexity**: 1 (quick task) — per SDD Phase-4 calibration, 1 test case per requirement is
sufficient rather than the full unit+error+integration triad. `plan.md`'s Epic 1.2 already
specifies 5 concrete Jest/RTL test cases (Tasks 1.2.2a, 1.2.2b, 1.2.3a, 1.2.3b, 1.2.4a) plus
Epic 1.3's 2 MCP-tool-call tasks (1.3.1a, 1.3.2a); this document adds one more test case (for
Requirement 3, which plan.md left as an implicit gap — see below) and maps all of them to the
7 requirements.

## Happy Path Scenario
Given the Omnibar modal is open and the user has typed an `@alias` invocation (e.g.
`@ssq my-feature`), when the user submits via `Ctrl+Enter` and `onCreateSession` resolves
successfully, then `onClose()` is called, `isSubmitting` resets to `false` (both "Create
Session" buttons re-enable), and no error message appears.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: Alias branch resets `isSubmitting` on success regardless of `onClose()` behavior | `Omnibar.submitReset.test.tsx` | `it("re-enables both Create Session buttons after a successful alias create, even when onClose is a no-op")` (Task 1.2.3a) | Unit/Integration (RTL) | Happy path, `onClose` mocked as no-op |
| REQ-2: SpawnShell branch resets `isSubmitting` on success regardless of `onClose()` behavior | `Omnibar.submitReset.test.tsx` | `it("allows a second submission after a successful SpawnShell create, even when onClose is a no-op")` (Task 1.2.2a) | Unit/Integration (RTL) | Happy path, `onClose` mocked as no-op, resubmission-call-count assertion (see plan.md Pattern Decisions row 4 for why button-`disabled` assertion doesn't work for this branch — SpawnShell never leaves discovery mode, so no Create button is rendered at all) |
| REQ-3: No behavior change on the happy path — modal still closes, no new error toast/flash | `Omnibar.submitReset.test.tsx` | `it("does not show an error message when the Alias submission succeeds normally")` — **new test added by this validation pass**, see below | Unit/Integration (RTL) | Happy path, asserts `onClose` called once and no error text rendered |
| REQ-4: Failure path unchanged — error surfaces, button re-enables | `Omnibar.submitReset.test.tsx` | SpawnShell: `it("still allows resubmission after onCreateSession rejects (pre-existing catch-path reset, must not regress)")` (Task 1.2.2b); Alias: `it("surfaces the error and re-enables both buttons when onCreateSession rejects (must not regress)")` (Task 1.2.3b) | Unit/Integration (RTL) | `onCreateSession` rejects, resubmission/error-text assertions |
| REQ-5: Regression tests exist and are runnable via the standard test command | `Omnibar.submitReset.test.tsx` | Scaffold: Task 1.2.1a (file creation + shared mocks/helpers); verified green via Task 1.2.5a (`cd web-app && npx jest --no-coverage --testPathPatterns="Omnibar.submitReset"`) | Infra + verification | N/A — this requirement is about the test suite's existence/runnability, not a single behavior |
| REQ-6: Defense-in-depth — `!isOpen` reset effect also clears `isSubmitting` | `Omnibar.submitReset.test.tsx` | `it("does not leave isSubmitting stuck if the modal is closed while a submission is in flight")` (Task 1.2.4a) | Unit/Integration (RTL) | Close/reopen cycle with an unresolved held promise; Default/LocalPath branch |
| REQ-7: Root cause of `onClose()` not dismissing the modal filed as its own tracked backlog item, not fixed inline | N/A — MCP tool call | `mcp__stapler-squad__create_backlog_item` call per Task 1.3.1a (modal-doesn't-dismiss investigation, two candidate mechanisms named, neither asserted as confirmed); companion follow-up per Task 1.3.2a (SpawnShell silent-error-path gap) | Process verification | Verify the call returns a new backlog item ID distinct from `a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7` (and, for 1.3.2a, distinct from the 1.3.1a item too) |

### On Requirement 3's gap (Step 1 decision)

`plan.md`'s Unresolved Questions section does not list REQ-3 as unresolved, but none of the 5
existing Epic 1.2 test cases assert the *absence* of an error message on the happy path — the
success tests (1.2.2a, 1.2.3a) assert resubmission/`disabled`-state, which only proves
`isSubmitting` reset, not that no error text rendered. Since this fix touches the same `catch`
block that produces error text, and Complexity-1 calibration still expects each requirement to
have at least one directly-mapped test rather than relying on "would have failed some other
assertion" inference, **one test case is added** rather than accepting the gap:

```ts
// New describe block or appended to "Omnibar Alias submit resets isSubmitting"
it("does not show an error message when the Alias submission succeeds normally", async () => {
  const onCreateSession = jest.fn().mockResolvedValue(undefined);
  const onClose = jest.fn();
  const { input } = renderOmnibar({ onCreateSession, onClose });

  await typeAndDetect(input, "@ssq my-feature");
  await act(async () => {
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
  });

  // onClose fired exactly once — no retry/re-entry into the error branch.
  expect(onClose).toHaveBeenCalledTimes(1);
  // The only place error text renders is OmnibarCreationPanel.tsx:839's
  // {error && <div className={errorClass}>{error}</div>}; assert it's absent.
  expect(screen.queryByText(/failed to create session/i)).not.toBeInTheDocument();
});
```

This uses the Alias branch (not SpawnShell) because `OmnibarCreationPanel` — the only render
site for `error` — is mounted there (Alias is a `CREATION_TYPE`; SpawnShell is not, per
`plan.md`'s Pattern Decisions row 4), so a positive assertion that no error text exists is
meaningful rather than vacuously true because the component never mounted.

## UX Acceptance Tests
N/A — no new user-facing surface; this fix corrects existing internal state-reset logic in an
existing modal, not a new UX flow.

## Test Stack
- **Unit/Integration**: Jest 30 + React Testing Library 16 (per `research/stack.md`)
- **E2E / UX**: N/A for this fix

## Coverage Targets and How to Measure
| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --coverage --testPathPatterns="Omnibar" --coverageThreshold='{"global":{"lines":80}}'` | Existing repo baseline — no new threshold introduced by this fix |

- All 7 requirements: covered by a specific named test or MCP tool-call task —
  REQ-1 → Task 1.2.3a; REQ-2 → Task 1.2.2a; REQ-3 → new test (this document); REQ-4 → Tasks
  1.2.2b + 1.2.3b; REQ-5 → Task 1.2.1a (scaffold) + Task 1.2.5a (green run); REQ-6 → Task
  1.2.4a; REQ-7 → Task 1.3.1a (+ companion Task 1.3.2a).
- Migration test: N/A — no Migration Plan section exists in `plan.md` (Complexity 1).
