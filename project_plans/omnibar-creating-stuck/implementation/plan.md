# Implementation Plan: omnibar-creating-stuck

**Feature**: Fix `handleSubmit`'s SpawnShell and Alias branches in `Omnibar.tsx` so `isSubmitting` always resets after a successful `onCreateSession` call, even if `onClose()` doesn't unmount the modal; add a defense-in-depth reset on close; file follow-up items for the still-unfixed "modal doesn't dismiss" symptom and SpawnShell's silent error path.
**Date**: 2026-08-11
**Status**: Ready for implementation — revised post-adversarial-review (see `adversarial-review.md`; verdict CONCERNS, 1 blocker + 3 concerns addressed below, not re-reviewed since none required a code-shape change)
**ADRs**: None — see `decisions/` section below.

**Scope note (adversarial review concern #1):** this fix addresses only the "button stuck on
Creating…" half of the reported bug. The other half — the modal itself not dismissing after a
successful submission — has an unconfirmed root cause (see Epic 1.3) and remains unfixed after
this PR ships. State this explicitly in the PR description and `request_review` message; do not
imply this is a full fix for the original report.

---

## Domain Glossary

N/A — complexity 1, no new domain types introduced. This fix touches only existing `useState`/`useCallback` control flow inside `Omnibar.tsx`; no new types, enums, or data shapes are added.

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `handleSubmit` SpawnShell/Alias branches | Per-branch `try { ... } catch (err) { setError(...) } finally { setIsSubmitting(false) }`, matching the existing Default branch shape (`Omnibar.tsx:1110,1198,1201-1203`) exactly | `research/build-vs-buy.md` §2 ("patch in place, do not extract a helper in this PR") | Extract a shared `submitSession(sessionData, { onSuccessExtra })` helper | `sessionData` construction and success side-effects (`addRecentShellCommand` vs. `saveHistory` vs. none) are genuinely branch-specific; the shared shell is ~6 lines; all 3 branches already sit adjacent in one function for a reviewer to compare. `.claude/rules/interface-pollution-checklist.md`'s "unjustified generic ... a second/third real use" bar isn't met yet. Revisit if a 4th session-creation branch is added (`.claude/rules/session-creation-registry.md`). |
| Repo-wide async-submit state | Leave local `useState(false)` pattern in place in `Omnibar.tsx` | `research/build-vs-buy.md` §3 (no `useAsyncAction`/`useAsyncSubmit` hook exists anywhere in `web-app/src`) | Extract a repo-wide `useAsyncSubmit` hook and apply it to `Omnibar.tsx`, `NewShellDialog.tsx`, `ResumeSessionModal.tsx`, `SessionWizard.tsx` | Whether the same missing-`finally` bug exists in those other 3 files is unverified (out of scope per requirements.md); a repo-wide extraction is a distinct, larger change disproportionate to a Complexity-1 fix. Worth a follow-up once a second broken file is confirmed. |
| `onClose()`-throw error attribution | Keep the existing (already-imperfect) behavior: any throw inside the `try` — including from `onClose()` itself — falls into the generic `catch` and produces "Failed to create session" (or the thrown `Error`'s message) exactly as the Default branch already does today | `research/pitfalls.md` §1 (Default branch, `Omnibar.tsx:1176-1197`, already has this exact latent gap, undocumented) | Add branch-specific error isolation so an `onClose()` throw isn't mis-attributed as a creation failure | Requirement 3 only asks for happy-path parity, not new error-isolation behavior; the Default branch already accepts this risk with no special-casing. Isolating it only in the two new branches would leave 3 branches disagreeing — scope creep beyond a mechanical pattern match. |
| SpawnShell regression-test observability | Assert the fix via **resubmission call-count** (a 2nd `Ctrl+Enter` after a successful 1st submit must reach `onCreateSession` again) | Direct code inspection: `Omnibar.tsx:1482` (`{!isDiscoveryMode && <OmnibarCreationPanel/>}`), `Omnibar.tsx:1620` (`{!isDiscoveryMode && <button>...`), `web-app/src/lib/omnibar/modes/useModeReducer.ts:16-24` (`CREATION_TYPES` set excludes `InputType.SpawnShell`) | Assert on a Create-button's `disabled` attribute, as `research/pitfalls.md`/`research/stack.md` assumed | **Corrects a research assumption, verified by reading the code directly**: both Create-button render sites (`OmnibarCreationPanel`'s footer button and `Omnibar.tsx`'s shortcuts-bar button) are gated behind `!isDiscoveryMode`. `SpawnShell` is not in `CREATION_TYPES`, so `handleSubmit`'s SpawnShell branch always runs in discovery mode, where **no Create button exists at all** — a button-disabled assertion would silently pass on an empty `getAllByRole` result instead of testing anything. The Alias branch *is* in `CREATION_TYPES`, so its regression test correctly uses button-state assertions (matches the existing `Omnibar.alias.test.tsx` precedent). |

---

## Migration Plan
N/A — complexity 1.

## Observability Plan
N/A — complexity 1.

## Risk Control
N/A — complexity 1.

## Unresolved Questions

1. **`Omnibar.alias.test.tsx`'s existing comment (lines 225-227) is stale/incorrect.** It reads: *"the omnibar stays in discovery mode (InputType.Alias is not a CREATION_TYPE)"*. Direct inspection of `useModeReducer.ts:16-24` shows `InputType.Alias` **is** in `CREATION_TYPES` — so Alias detection does enter `"creation"` mode and does render `OmnibarCreationPanel`. This doesn't block the fix (the existing tests still pass either way, since they only assert on `onCreateSession`'s call args, not on mode/button-count), but it does mean this plan's new Alias regression tests correctly use button-state assertions (2 buttons render), and it flags the stale comment as a **candidate nit for a human reviewer to fix in the same PR** (1-line comment correction) — not a blocking unresolved question, and not itself part of the required task list below since it's a pre-existing doc comment, not code behavior.
2. **SpawnShell's error message has no visible DOM surface today**, independent of this fix: `error` state is only rendered via `OmnibarCreationPanel.tsx:839` (`{error && <div className={errorClass}>{error}</div>}`), and `OmnibarCreationPanel` never renders while `SpawnShell` keeps the component in discovery mode. This means requirement 4's phrase "surfaces the error message" cannot be literally verified for the SpawnShell branch via any existing UI. This is **pre-existing, out-of-scope behavior** (not something this fix changes, breaks, or is asked to fix) — the SpawnShell failure-path regression test below verifies the observable part of requirement 4 that *is* testable (resubmission is un-blocked after a failure), and explicitly does not assert error text for that branch. Flagged here rather than silently asserting something false.

3. **Overlapping-submission race, widened slightly by Story 1.1.3 (adversarial review concern #3).** Scenario: submit A in flight (`isSubmitting=true`) → modal closed and reopened while A is still pending → the new `!isOpen` effect force-resets `isSubmitting` to `false` → user submits again as request B → A finally resolves/rejects and its own `finally` unconditionally calls `setIsSubmitting(false)`, clobbering B's in-flight state and re-enabling the Create button while B is still outstanding. This race already exists today in isolation in the Default branch; this PR extends the same shape to two more branches and adds the very reset (Story 1.1.3) that makes the overlap window easier to reach. **Accepted as a known, pre-existing limitation, explicitly out of scope for this Complexity-1 fix** — no submission-id/ref guard is added. A repo-wide fix (tagging each in-flight submission and only resetting `isSubmitting` in `finally` if it's still the latest) is a larger change appropriate for the `useAsyncSubmit` follow-up noted in the Pattern Decisions table above, not this PR.

Resolution: all three items are informational/documented or explicitly accepted, not blockers — proceed with the task breakdown below.

## Dependency Visualization

```
Task 1.1.1a (SpawnShell fix, Omnibar.tsx:1037-1061)   ─┐
Task 1.1.2a (Alias fix, Omnibar.tsx:1072-1105)         ├─→ Task 1.2.1a (test file scaffold)
Task 1.1.3a (!isOpen reset, Omnibar.tsx:622-633)       ─┘         │
                                                                   ├─→ Task 1.2.2a (SpawnShell success-reset test)
                                                                   │        └─→ Task 1.2.2b (SpawnShell failure-then-retry test)
                                                                   ├─→ Task 1.2.3a (Alias success-reset test)
                                                                   │        └─→ Task 1.2.3b (Alias failure test, AC4)
                                                                   └─→ Task 1.2.4a (defense-in-depth !isOpen test, AC6)
                                                                              │
                                                                              ▼
                                                                   Task 1.2.5a (run targeted jest, confirm green)

Task 1.3.1a (file "modal doesn't dismiss" follow-up, AC7)      — independent, no dependency on the above
Task 1.3.2a (file "SpawnShell silent error" follow-up)          — independent, no dependency on the above
```

---

## Phase 1: Fix the stuck-"Creating…" modal

### Epic 1.1: Patch the two broken `handleSubmit` branches + defense-in-depth reset
**Goal**: SpawnShell and Alias branches reset `isSubmitting` unconditionally on success (matching the already-correct Default branch), and the `!isOpen` reset effect clears `isSubmitting` as a second line of defense.

#### Story 1.1.1: SpawnShell branch always resets `isSubmitting` on success
**As a** user who submits a `>shell` command, **I want** the Create button/UI state to reset after the session is created, **so that** the omnibar isn't stuck showing a submitting state even if the parent's `onClose()` doesn't synchronously dismiss the modal.
**Acceptance Criteria**:
- SpawnShell branch: after a successful `onCreateSession` call, `isSubmitting` resets to `false` regardless of `onClose()` behavior. (Requirement 2)
  - *Given* the user types `>shell` (detected by `CommandDetector` as `{ type: InputType.SpawnShell, confidence: 1.0 }`), `onCreateSession` is `jest.fn().mockResolvedValue(undefined)`, and `onClose` is a no-op `jest.fn()` that does not flip `isOpen`, *When* the user submits via `fireEvent.keyDown(input, { key: "Enter", ctrlKey: true })` (wrapped in `await act(...)`) a second time after the first submission's promise has resolved, *Then* `onCreateSession` has been called exactly twice — proving `isSubmitting` returned to `false` after the first success (a stuck `isSubmitting === true` would make the `!canSubmit || isSubmitting` guard at `Omnibar.tsx:1019` silently swallow the second `Ctrl+Enter`, leaving the call count at 1).
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.1.1a: Restructure SpawnShell branch to `try/catch/finally` (~3 min)
- In `web-app/src/components/sessions/Omnibar.tsx`, replace the SpawnShell branch body (currently `Omnibar.tsx:1052-1059`):
  ```ts
  try {
    await onCreateSession(sessionData);
    if (shellCommand) addRecentShellCommand(shellCommand);
    onClose();
  } catch (err) {
    setError(err instanceof Error ? err.message : "Failed to create session");
    setIsSubmitting(false);
  }
  ```
  with:
  ```ts
  try {
    await onCreateSession(sessionData);
    if (shellCommand) addRecentShellCommand(shellCommand);
    onClose();
  } catch (err) {
    setError(err instanceof Error ? err.message : "Failed to create session");
  } finally {
    setIsSubmitting(false);
  }
  ```
  (Drop the branch-local `setIsSubmitting(false)` from `catch` — the `finally` now covers both success and failure unconditionally, matching the Default branch's shape at `Omnibar.tsx:1198-1203`.)
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 1.1.2: Alias branch always resets `isSubmitting` on success
**As a** user who submits an `@alias` invocation, **I want** the Create button state to reset after the session is created, **so that** the omnibar isn't stuck showing "Creating…"/"Creating..." even if `onClose()` doesn't unmount the modal.
**Acceptance Criteria**:
- Alias-invocation branch: after a successful `onCreateSession` call, `isSubmitting` resets to non-submitting regardless of whether `onClose()` actually unmounts the modal. (Requirement 1)
  - *Given* the user types `@ssq my-feature` (with `AliasDetector` registered via `getDefaultRegistry().register(new AliasDetector([SSQ_ALIAS]))`, matching `Omnibar.alias.test.tsx`'s existing fixture, so detection resolves to `InputType.Alias`), `onCreateSession` is `jest.fn().mockResolvedValue(undefined)`, and `onClose` is a no-op `jest.fn()`, *When* the user submits via `Ctrl+Enter` (`await act(...)`), *Then*, after the promise resolves, `screen.getAllByRole("button", { name: /create session/i })` (both `OmnibarCreationPanel.tsx:846-853`'s footer button, label `"Create Session"`/`"Creating..."`, and `Omnibar.tsx:1621-1628`'s shortcuts-bar button, label `"Create Session"`/`"Creating…"`) returns buttons where **none** has the `disabled` attribute — neither button is stuck on its submitting label.
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.1.2a: Restructure Alias branch to `try/catch/finally` (~3 min)
- In `web-app/src/components/sessions/Omnibar.tsx`, replace the Alias branch body (currently `Omnibar.tsx:1095-1103`):
  ```ts
  setIsSubmitting(true);
  setError(null);
  try {
    await onCreateSession(sessionData);
    onClose();
  } catch (err) {
    setError(err instanceof Error ? err.message : "Failed to create session");
    setIsSubmitting(false);
  }
  return;
  ```
  with:
  ```ts
  setIsSubmitting(true);
  setError(null);
  try {
    await onCreateSession(sessionData);
    onClose();
  } catch (err) {
    setError(err instanceof Error ? err.message : "Failed to create session");
  } finally {
    setIsSubmitting(false);
  }
  return;
  ```
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 1.1.3: Defense-in-depth reset on modal close
**As a** developer, **I want** the `!isOpen` reset effect to also clear `isSubmitting`, **so that** the long-lived `Omnibar` instance can never carry a stuck submitting flag into its next open cycle, independent of whether `handleSubmit`'s own `finally` ran.
**Acceptance Criteria**:
- The reset-on-close effect at `Omnibar.tsx:622-633` also explicitly clears `isSubmitting`, since the `Omnibar` component instance is long-lived and persists across open/close cycles. (Requirement 6)
  - *Given* a submission is in flight (`isSubmitting === true`, e.g. `onCreateSession` returns an unresolved held `Promise`) while the modal is showing the Default branch's creation form for a `LocalPath` detection (e.g. `/home/user/projects`), *When* the parent flips `isOpen` from `true` to `false` (simulated via `rerender` with `isOpen={false}`) and then back to `true`, *Then* on re-render, after retyping the same path to re-enter creation mode, the Create Session button is **not** disabled — `isSubmitting` reads `false` even though the original `onCreateSession` promise never resolved and `handleSubmit`'s own `finally` never ran for that call.
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.1.3a: Add `setIsSubmitting(false)` to the `!isOpen` reset effect (~2 min)
- In `web-app/src/components/sessions/Omnibar.tsx`, inside the `useEffect` at `Omnibar.tsx:622-633`, add `setIsSubmitting(false);` alongside the existing unconditional setters (after `setError(null);`, before `lastSuggestedNameRef.current = "";`, matching the ordering/style of the other setters already there):
  ```ts
  useEffect(() => {
    if (!isOpen) {
      setInput("");
      setDetection(null);
      setFormState(INITIAL_FORM_STATE);
      setUIState({ showAdvanced: false, dropdownIndex: -1, dropdownDismissed: false, resultHighlightIndex: -1, atSuggestIndex: -1 });
      setError(null);
      setIsSubmitting(false);
      lastSuggestedNameRef.current = "";
      prevDetectionTypeRef.current = null;
      dispatchMode({ kind: "reset_to_discovery" });
    }
  }, [isOpen, dispatchMode]);
  ```
  No dependency-array change needed — `setIsSubmitting` is a stable `useState` setter, exactly like the other setters already called unconditionally in this effect without being listed in `[isOpen, dispatchMode]`.
- Files: `web-app/src/components/sessions/Omnibar.tsx`

---

### Epic 1.2: Regression tests
**Goal**: New Jest/RTL tests in `web-app/src/components/sessions/__tests__/` prove the fix for both branches, confirm the failure path still works, and confirm the defense-in-depth reset — following `Omnibar.alias.test.tsx`'s exact mocking/timer/`act` conventions per `research/stack.md` and `research/pitfalls.md`.

#### Story 1.2.1: New test file scaffold
**As a** developer, **I want** a new `Omnibar.submitReset.test.tsx` colocated with the other `Omnibar.*.test.tsx` files, **so that** the fix has dedicated regression coverage without bloating `Omnibar.alias.test.tsx`.
**Acceptance Criteria**:
- Regression tests exist and are runnable via the project's standard test command. (Requirement 5)
  - *Given* the new file `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`, *When* `cd web-app && npx jest --no-coverage --testPathPatterns="Omnibar.submitReset"` runs, *Then* all tests in it pass, and reverting only the Epic 1.1 source changes (leaving the test file in place) makes the SpawnShell/Alias success-reset tests fail — proving they're not tautological.
**Files**: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

##### Task 1.2.1a: Create the test file scaffold (~5 min)
- Create `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`.
- Copy the following from `Omnibar.alias.test.tsx` verbatim (do not invent a new style, per `research/stack.md`):
  - The `jest.mock("@/lib/omnibar", ...)` passthrough block (only needed if a test wants to force a detection throw — omit if unused by this file's tests, per `research/pitfalls.md` §4's "reuse is optional" note; the SpawnShell/Alias regression tests here don't need it).
  - The full baseline `jest.mock(...)` block: `next/navigation`, `@/lib/contexts/ThemeContext`, `@/lib/config`, `@/lib/hooks/usePathCompletions`, `@/lib/hooks/usePathHistory`, `@/lib/hooks/useSessionSearch`, `@/lib/hooks/useWorktreeSuggestions`, `@/lib/hooks/useAliases`, `@/lib/hooks/useAliasSuggestions`, `@/lib/hooks/useAtCommandSuggestions`, `@/lib/hooks/useAvailablePrograms`, `@/lib/hooks/useSlashCommands`, `@/lib/hooks/useSlashCommandSuggestions`, `@/lib/store`, `@/lib/store/sessionsSlice`, `@/components/sessions/OmnibarResultList`, `@/lib/api/transport`.
  - The `SSQ_ALIAS` fixture, `defaultCompletions`, `defaultHistory` fixtures.
  - The `renderOmnibar(props)` helper (lines 166-182) and `typeAndDetect(input, value)` helper (lines 184-190).
  - Import `getDefaultRegistry`, `resetDefaultRegistry` from `@/lib/omnibar` and `AliasDetector` from `@/lib/omnibar/detectors/AliasDetector` (needed for the Alias-branch tests below).
- **Also fix, in the same PR (adversarial review minor #1):** `Omnibar.alias.test.tsx:225-227`'s stale comment reads *"the omnibar stays in discovery mode (InputType.Alias is not a CREATION_TYPE)"* — `useModeReducer.ts:16-24` shows `InputType.Alias` **is** in `CREATION_TYPES`. Correct the comment there to match reality (this doesn't change any assertion in that file, only the comment text) so it doesn't mislead whoever reads it alongside this PR's new button-count assertions.
- Add three `describe` blocks (populated in Stories 1.2.2-1.2.4 below), each with the standard `beforeEach`/`afterEach` fake-timer setup matching `Omnibar.alias.test.tsx`'s pattern:
  ```ts
  beforeEach(() => {
    jest.useFakeTimers();
    mockUsePathCompletions.mockReturnValue(defaultCompletions);
    mockUsePathHistory.mockReturnValue(defaultHistory);
    mockUseAliases.mockReturnValue({ aliases: [SSQ_ALIAS], loading: false, error: null, refetch: jest.fn() });
    resetDefaultRegistry();
    getDefaultRegistry().register(new AliasDetector([SSQ_ALIAS]));
  });

  afterEach(() => {
    act(() => { jest.runOnlyPendingTimers(); });
    jest.useRealTimers();
    jest.clearAllMocks();
    resetDefaultRegistry();
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

#### Story 1.2.2: SpawnShell regression tests
**As a** developer, **I want** tests proving SpawnShell's `isSubmitting` resets after success and after failure, **so that** the fix and the pre-existing failure path are both covered.
**Acceptance Criteria**: same as Story 1.1.1's AC (Requirement 2) plus Requirement 4's SpawnShell half — see GWTs below.
**Files**: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

##### Task 1.2.2a: SpawnShell success-reset test (~5 min)
- In `describe("Omnibar SpawnShell submit resets isSubmitting", ...)`:
  ```ts
  it("allows a second submission after a successful SpawnShell create, even when onClose is a no-op", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const onClose = jest.fn(); // no-op: does not flip isOpen, reproducing the bug scenario
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, ">shell");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Terminal", sessionType: "one_off" })
    );

    // Submit again with the same (still-detected) input. If isSubmitting were
    // stuck true (the bug), the `!canSubmit || isSubmitting` guard in
    // handleSubmit (Omnibar.tsx:1019) would silently no-op this second call.
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(2);
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

##### Task 1.2.2b: SpawnShell failure-then-retry test (AC4) (~5 min)
- Same `describe` block:
  ```ts
  it("still allows resubmission after onCreateSession rejects (pre-existing catch-path reset, must not regress)", async () => {
    const onCreateSession = jest
      .fn()
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValueOnce(undefined);
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, ">shell");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);

    // Retry after failure — the branch's catch already called setIsSubmitting(false)
    // before this fix; this asserts that behavior is preserved, not newly added.
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledTimes(2);
  });
  ```
  Note (document as a code comment in the test): SpawnShell's `error` message has no visible DOM assertion available here — `OmnibarCreationPanel` (the only place `error` renders, `OmnibarCreationPanel.tsx:839`) never mounts while SpawnShell keeps the component in discovery mode. This is pre-existing, out-of-scope behavior (see Unresolved Questions #2), not something this test suite needs to assert.
- Files: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

#### Story 1.2.3: Alias regression tests
**As a** developer, **I want** tests proving Alias's `isSubmitting` resets after success and after failure, **so that** the fix and the pre-existing failure path are both covered.
**Acceptance Criteria**: same as Story 1.1.2's AC (Requirement 1) plus Requirement 4's Alias half — see GWTs below.
**Files**: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

##### Task 1.2.3a: Alias success-reset test (~5 min)
- In `describe("Omnibar Alias submit resets isSubmitting", ...)`:
  ```ts
  it("re-enables both Create Session buttons after a successful alias create, even when onClose is a no-op", async () => {
    const onCreateSession = jest.fn().mockResolvedValue(undefined);
    const onClose = jest.fn(); // no-op: does not flip isOpen
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, "@ssq my-feature");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ title: "ssq-my-feature", aliasName: "ssq" })
    );

    // Alias IS a CREATION_TYPE (useModeReducer.ts:23), so both Create Session
    // buttons (OmnibarCreationPanel footer + Omnibar.tsx shortcuts-bar) render.
    const buttons = screen.getAllByRole("button", { name: /create session/i });
    expect(buttons.length).toBeGreaterThan(0);
    for (const btn of buttons) {
      expect(btn).not.toHaveAttribute("disabled");
    }
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

##### Task 1.2.3b: Alias failure test (AC4) (~5 min)
- Same `describe` block:
  ```ts
  it("surfaces the error and re-enables both buttons when onCreateSession rejects (must not regress)", async () => {
    const onCreateSession = jest.fn().mockRejectedValueOnce(new Error("boom"));
    const onClose = jest.fn();
    const { input } = renderOmnibar({ onCreateSession, onClose });

    await typeAndDetect(input, "@ssq my-feature");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });

    // error surfaces via OmnibarCreationPanel.tsx:839 ({error && <div>{error}</div>}),
    // which IS rendered here since Alias is a CREATION_TYPE.
    expect(screen.getByText("boom")).toBeInTheDocument();

    const buttons = screen.getAllByRole("button", { name: /create session/i });
    for (const btn of buttons) {
      expect(btn).not.toHaveAttribute("disabled");
    }
  });
  ```
  This is **new** coverage — no existing test exercises `onCreateSession` rejecting in the Alias branch; the only existing throw test in `Omnibar.alias.test.tsx` (lines 437-471) covers the *detection* effect throwing, not `onCreateSession` itself.
- Files: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

#### Story 1.2.4: Defense-in-depth reset test (AC6)
**As a** developer, **I want** a test proving the `!isOpen` effect's new `setIsSubmitting(false)` actually prevents a stuck state across a close/reopen cycle, **so that** requirement 6 has direct coverage, not just code inspection.
**Acceptance Criteria**: see Story 1.1.3's GWT (Requirement 6).
**Files**: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

##### Task 1.2.4a: `!isOpen` defense-in-depth test (~5 min)
- New `describe("Omnibar defense-in-depth reset on close", ...)` (uses the Default/LocalPath branch, which already renders a Create button unconditionally in creation mode — no alias registration needed in this block's `beforeEach`, just the baseline mocks):
  ```ts
  it("does not leave isSubmitting stuck if the modal is closed while a submission is in flight", async () => {
    let resolveCreate!: () => void;
    const heldPromise = new Promise<void>((resolve) => { resolveCreate = resolve; });
    const onCreateSession = jest.fn().mockReturnValue(heldPromise);
    const { input, rerender } = renderOmnibar({ onCreateSession });

    await typeAndDetect(input, "/home/user/projects");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    // isSubmitting is now true; onCreateSession's promise is still unresolved.

    // Parent force-closes the modal (isOpen: true -> false) without the
    // in-flight submission ever resolving.
    rerender(
      <Omnibar isOpen={false} onClose={jest.fn()} onCreateSession={onCreateSession} onNavigateToSession={jest.fn()} />
    );
    // Reopen.
    rerender(
      <Omnibar isOpen={true} onClose={jest.fn()} onCreateSession={onCreateSession} onNavigateToSession={jest.fn()} />
    );

    const reopenedInput = screen.getByRole("combobox", { name: /session source input/i });
    await typeAndDetect(reopenedInput, "/home/user/projects");

    const buttons = screen.getAllByRole("button", { name: /create session/i });
    for (const btn of buttons) {
      expect(btn).not.toHaveAttribute("disabled");
    }

    // Clean up the dangling promise so it doesn't leak into a later test.
    await act(async () => { resolveCreate(); });
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/Omnibar.submitReset.test.tsx`

#### Story 1.2.5: Verify green
**As a** developer, **I want** to confirm the full new test file passes and the targeted existing suite still passes, **so that** the fix is provably correct before shipping.
**Acceptance Criteria**: Requirement 5's "runnable and passing" bar.
**Files**: N/A (verification only)

##### Task 1.2.5a: Run targeted Jest and type-check, confirm green (~4 min)
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="Omnibar.submitReset"` — all new tests pass.
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="Omnibar.alias"` — existing suite still passes unmodified (confirms no regression to existing Alias namePrefix/detection-throw coverage, and that the stale-comment fix didn't break anything).
- Run a type-check (`cd web-app && npx tsc --noEmit`, or `make quick-check` from repo root) after the `try/catch/finally` restructure (adversarial review minor #2) — low risk given the mechanical nature of the change, but confirms no stray syntax/type issue from moving the `onClose()` call and dropping the branch-local `setIsSubmitting(false)`.
- Files: N/A

---

### Epic 1.3: File follow-up items for what this fix doesn't cover (Requirement 7 + adversarial review concern #2)
**Goal**: Track (a) the still-unconfirmed root cause of `onClose()` not dismissing the modal, and (b) SpawnShell's silent error path, as their own backlog items — without implementing either fix here.

#### Story 1.3.1: File a follow-up backlog item for the modal-doesn't-dismiss symptom
**As a** maintainer, **I want** the still-open "modal doesn't close" symptom tracked as its own backlog item with an honest list of candidate mechanisms, **so that** whoever picks it up investigates rather than being pointed at a single unverified theory.
**Acceptance Criteria**:
- Root cause of `onClose()` failing to dismiss the modal is filed as its own tracked backlog item naming the specific mechanism(s) considered, rather than fixed inline here. (Requirement 7)
  - *Given* code inspection confirms `Omnibar.tsx:1237`'s `if (!isOpen) return null;` fully unmounts the modal whenever `isOpen` reaches `false` — meaning `position: fixed` CSS cannot by itself explain a modal that *never unmounts* (that class of bug produces a mispositioned/clipped-but-still-mounted overlay, not a stuck-mounted one) — *When* `mcp__stapler-squad__create_backlog_item` is called with the title/description below (which states the symptom, lists the CSS/portal theory as one unconfirmed hypothesis alongside an `OmnibarContext.tsx` open/close race as an alternative, and does not assert either as "the" root cause), *Then* a new backlog item ID is returned, distinct from this item's own ID (`a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7`).
**Files**: N/A — MCP tool call, not a code change.

##### Task 1.3.1a: Call `create_backlog_item` for the modal-doesn't-dismiss investigation (~4 min)
- **Revised per adversarial review blocker**: `Omnibar.tsx:1237` (`if (!isOpen) return null;`) plus `OmnibarContext.tsx:275-276`'s unconditional `<Omnibar isOpen={isOpen} .../>` render and `close = useCallback(() => setIsOpen(false), [])` (`OmnibarContext.tsx:148-150`) mean React fully unmounts the modal whenever `isOpen` actually reaches `false`, regardless of CSS. A `position: fixed`-without-`createPortal` bug (per `.claude/rules/css-architecture.md`) produces a *mispositioned or clipped but still-mounted* overlay, not a modal that never unmounts — so it doesn't actually explain the reported symptom on its own. Demote it to "one hypothesis to check" rather than "the" root cause, and name the alternative the reviewer identified: `isOpen` may never actually reach `false` in the mobile repro (a double-fire event, a race with a subsequent `open()`/`toggle()` call, or another state-update issue in `OmnibarContext.tsx`'s `open`/`openInCreationMode`/`toggle` callbacks, `OmnibarContext.tsx:130-153`).
- Call `mcp__stapler-squad__create_backlog_item` with:
  - **Title**: `Omnibar modal sometimes doesn't dismiss after onClose() on mobile — mechanism unconfirmed`
  - **Description**:
    ```
    Confirmed live: a "@pw retirement" alias session was created successfully
    server-side (personal-wiki-retirement, Active, ran for hours) but the
    Omnibar modal stayed open on a mobile browser after submission, with the
    Create button stuck on "Creating...". Filed as a follow-up from
    project_plans/omnibar-creating-stuck/ (backlog item
    a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7), which fixed the isSubmitting-never-
    resets symptom (the button no longer gets stuck) but left the modal itself
    not dismissing unexplained and unfixed — the two are separate bugs that
    happened to be reported together.

    Root cause is NOT yet confirmed. Two candidate mechanisms to investigate,
    neither corroborated yet:

    1. A state-update issue in OmnibarContext.tsx's open()/openInCreationMode()/
       toggle() callbacks (OmnibarContext.tsx:130-153) causing isOpen to never
       actually reach false on this repro path (e.g. a double-fire event, or a
       race with a subsequent open call re-setting isOpen=true right after
       close() sets it false). This would explain the symptom directly, since
       Omnibar.tsx:1237's `if (!isOpen) return null;` fully unmounts the modal
       whenever isOpen genuinely reaches false.
    2. Omnibar.css.ts:22's `overlay` style uses `position: "fixed"` and the
       component is not rendered via createPortal(..., document.body). Per
       .claude/rules/css-architecture.md's "Never Do" list, this class of bug
       causes a still-mounted overlay to become mispositioned/clipped when an
       ancestor sets transform/filter/will-change — NOT a failure to unmount.
       Worth ruling out only if #1 doesn't pan out, or if the actual mobile
       repro shows a visually-misplaced-but-interactive modal rather than one
       that's simply still present.

    Scope: investigate #1 first (add logging/a repro test around isOpen state
    transitions); only pursue a createPortal migration for #2 if #1 is ruled
    out and a mispositioning mechanism is actually confirmed.
    ```
- Files: N/A

#### Story 1.3.2: File a follow-up backlog item for SpawnShell's silent error path
**As a** maintainer, **I want** SpawnShell's no-visible-error-on-failure gap tracked, **so that** it gets the same visibility as the Requirement 7 follow-up rather than living only as a doc note in this plan (adversarial review concern #2).
**Acceptance Criteria**:
- The gap documented in Unresolved Question #2 above (SpawnShell `onCreateSession` failures produce no visible DOM error, since `OmnibarCreationPanel` — the only place `error` renders — never mounts while SpawnShell keeps the component in discovery mode) is filed as its own tracked backlog item.
  - *Given* `OmnibarCreationPanel.tsx:839`'s `{error && <div className={errorClass}>{error}</div>}` is the only render site for the `error` state, and `useModeReducer.ts:16-24`'s `CREATION_TYPES` excludes `InputType.SpawnShell` (so `OmnibarCreationPanel` never mounts for a SpawnShell submission), *When* `mcp__stapler-squad__create_backlog_item` is called, *Then* a new backlog item ID is returned, distinct from both this item's ID and the Story 1.3.1 item's ID.
**Files**: N/A — MCP tool call, not a code change.

##### Task 1.3.2a: Call `create_backlog_item` for SpawnShell's silent error path (~3 min)
- Call `mcp__stapler-squad__create_backlog_item` with:
  - **Title**: `SpawnShell (>shell ...) omnibar failures produce no visible error message`
  - **Description**:
    ```
    Omnibar.tsx's handleSubmit SpawnShell branch (>shell ... commands) sets
    `error` state on a failed onCreateSession call, same as every other branch.
    But error only renders via OmnibarCreationPanel.tsx:839
    ({error && <div className={errorClass}>{error}</div>}), and
    useModeReducer.ts:16-24's CREATION_TYPES set excludes InputType.SpawnShell —
    so OmnibarCreationPanel never mounts for a SpawnShell submission (it stays
    in discovery mode the whole time). A failed `>shell` session creation is
    currently silent to the user: the button becomes clickable again (after the
    fix in backlog item a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7), but no error
    text appears anywhere.

    Scope: give the SpawnShell branch a visible error surface even while the
    omnibar stays in discovery mode — e.g. render the error near the input in
    discovery mode, or reconsider whether SpawnShell should enter creation mode
    on failure specifically to show the existing error UI.
    ```
- Files: N/A

---

## Decisions

No ADRs needed for this fix — see the top of this document. This is a same-file, same-pattern, Complexity-1 mechanical fix with no new architecture, no new dependencies, and no cross-cutting design decision that would outlive this PR (the one design choice made — patch in place vs. extract a helper — is fully captured in the Pattern Decisions table above and cites `research/build-vs-buy.md` directly, which is sufficient rationale for a decision this scoped).
