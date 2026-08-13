# Implementation Plan: omnibar-creation-stuck-modal

**Feature**: Fix the Omnibar SpawnShell and Alias-invocation submit branches so `isSubmitting` cannot get stuck `true` after a successful session creation, by copying the existing correct `try/finally` pattern already used in the default branch.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: None — considered and rejected. This is a small bug fix that copies an already-proven pattern within the same file; it introduces no new technology, dependency, or architecturally significant decision, so no ADR is warranted.

---

## Step 0.5: Creative Pass — Approaches Considered

**A. Minimal `try/finally` copy + reset-effect defense-in-depth (CHOSEN)**
Copy the exact `try { ... } finally { setIsSubmitting(false) }` shape already used in the default branch (`web-app/src/components/sessions/Omnibar.tsx:1073-1169`, and mirrored again at `:1550-1559`) into the SpawnShell branch (`:1003-1027`) and Alias branch (`:1038-1071`), and add `setIsSubmitting(false)` to the existing close-reset `useEffect` (`:587-599`) as a second, independent layer.
*Strength*: Zero new abstractions, mechanically consistent with code already in this file (same author intent, same pattern, easy to review line-by-line), and directly matches the architecture.md/build-vs-buy.md research verdict.
*Weakness*: Duplicates the same 4-line shape three times in one file — acceptable per architecture.md's explicit rejection of a shared wrapper (see below), but a future reader has to trust that all three copies stay in sync.

**B. Extract a shared `submitAndClose` helper/hook**
A single function like `await submitAndClose(sessionData, onCreateSession, onClose)` called from all three branches.
*Strength*: Eliminates the 3x duplication in one motion.
*Weakness*: **Rejected** — architecture.md found the three branches differ enough (different payload shapes, different post-success side effects like `addRecentShellCommand`, different recovery logic like the `R2` path-not-found confirmation dialog in the default branch) that a generic wrapper needs several optional params/callbacks, which research judged strictly worse than the 4-line mechanical duplication for a 2-call-site problem. build-vs-buy.md independently confirms: no async-submit hook exists anywhere in this codebase (`SessionWizard.tsx`, `CheckpointButton.tsx`, `ResumeSessionModal.tsx`, `NewShellDialog.tsx` all hand-roll their own), so introducing one here would be a net-new abstraction for a bug fix, not a natural extraction.

**C. Refactor the three branches into a single unified code path**
Collapse SpawnShell/Alias/default into one generic "build sessionData, submit" flow driven by a discriminated union or config object.
*Strength*: Would prevent this entire class of bug from recurring by construction (only one `try/finally` to get right).
*Weakness*: **Rejected** — requirements.md explicitly scopes this out ("Redesigning the Omnibar's three-branch structure into a single shared code path (flag as follow-up suggestion only, keep diff minimal)"). Blast radius is far larger than a bug fix warrants; flagged as a follow-up suggestion only, not a task in this plan.

**Decision**: Approach A. Recorded in Pattern Decisions below.

---

## Domain Glossary
| Term | Definition | Notes |
|------|-----------|-------|
| `isSubmitting` | `useState<boolean>` at `Omnibar.tsx:205`; drives the disabled/"Creating…" state of the submit button (`:1588-1590`). | The exact piece of state this bug leaves stuck `true`. |
| SpawnShell branch | The `if (detection?.type === InputType.SpawnShell ...)` block in `handleSubmit`, `Omnibar.tsx:1003-1027`. Creates a real terminal session from `>shell [dir] [-- command]` input. | One of the two broken branches (missing `finally`). |
| Alias-invocation branch | The `if (detection?.type === InputType.Alias && detection.metadata?.aliasName)` block, `Omnibar.tsx:1038-1071`. Creates a session from `@aliasname ...` input (the exact path `@pw retirement` hit in the reported bug). | The other broken branch; the one from the bug report. |
| Default branch | The unconditional fallthrough block starting `Omnibar.tsx:1073`, ending with `} finally { setIsSubmitting(false); }` at `:1167-1168`. Already correct — the pattern to copy. | Handles `directory`/`new_worktree`/`existing_worktree`/`one_off`/`new_project`/autonomous session creation. |
| `onClose` / `close` | The prop `Omnibar` receives (`onClose`), backed by `close` defined in `web-app/src/lib/contexts/OmnibarContext.tsx:148-152` — a trivial synchronous `setIsOpen(false)`. Cannot throw, cannot legitimately "fail to fire." | Ruled out as broken *itself* by architecture.md and features.md (it always fires and always sets `isOpen` false). This does not rule out a second, distinct mechanism — a CSS/rendering issue downstream of a correctly-fired `onClose` — see Unresolved Questions for the still-open `position: fixed`/no-`createPortal` hypothesis. |
| Reset-on-close effect | The `useEffect` at `Omnibar.tsx:587-599`, keyed on `[isOpen, dispatchMode]`, that clears `input`/`detection`/`formState`/`uiState`/`error` and dispatches `reset_to_discovery` whenever `isOpen` becomes `false`. Currently does **not** reset `isSubmitting` — this is the actual, provable root cause of the reported symptom. | The Omnibar component instance never unmounts (see next entry), so this effect is the only place stale submission state gets cleaned up. |
| Long-lived Omnibar instance | `OmnibarContext.tsx:275-277` renders `<Omnibar isOpen={isOpen} onClose={close} .../>` unconditionally (no `{isOpen && <Omnibar/>}`); combined with `Omnibar.tsx:1203`'s `if (!isOpen) return null;`, the component instance and all its `useState` (including `isSubmitting`) persist across every open/close/reopen cycle. | Explains why the bug is invisible while the modal is closed and only visible on the *next* open — indistinguishable from "never closed" to the user. |
| `handleSubmit` | The `useCallback` in `Omnibar.tsx` containing all three branches plus the shared dependency array (starts ~`:985`, deps list starts `:1172`). | Single function; this plan edits three of its internal blocks plus deps if needed (no new deps required — `setIsSubmitting` is already stable and already in scope). |
| `aria-busy` / `aria-live` convention | Existing accessibility pattern already used at `Omnibar.tsx:1284`, `:1338`, `:1429` (`aria-live="polite"` / `"assertive"` regions for analogous async/status states). The submit button (`:1584-1591`) currently has neither. | Cheap, same-button addition (Task 1.3.1a); promoted from optional polish into required core scope during Product Triad Review since this repo's CI gates `web-app/src/` PRs on Axe Core WCAG-AA checks — see Epic 1.3's promotion note. |

---

## Pattern Decisions
| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| SpawnShell / Alias branch structure | Hand-rolled `try { await onCreateSession(...); ...; onClose(); } catch (err) { setError(...); } finally { setIsSubmitting(false); }`, copied verbatim in shape from the default branch (`Omnibar.tsx:1073-1169`) | architecture.md ("REJECTED: extracting a shared `submitAndClose` wrapper/hook"), build-vs-buy.md ("Verdict: local hand-fix, no new dependency") | B: shared `submitAndClose` helper/hook | Only 2 broken call sites; branches differ in payload shape and post-success side effects (`addRecentShellCommand`, R2 confirmation-dialog recovery in the default branch); a generic wrapper needs several optional params — worse than 4 lines of duplication per call site. No existing async-submit hook precedent anywhere in the codebase to extend. |
| Overall branch structure (3 near-identical blocks in `handleSubmit`) | Leave as-is; fix each block in place | requirements.md ("Out of scope: Redesigning the Omnibar's three-branch structure into a single shared code path (flag as follow-up suggestion only, keep diff minimal)") | C: unify into one generic branch | Explicitly out of scope; blast radius far exceeds a bug fix. Recorded as a follow-up suggestion (see below), not a task. |
| State type for `isSubmitting` | No change — remains `useState<boolean>` | plan.md Step 3 instruction (no primitive obsession here) | Overloading a status enum (`idle/submitting/error/success`) | `isSubmitting` boolean plus `error: string \| null` already correctly separates "is a request in flight" from "did it fail"; introducing a status enum would be an unrequested refactor with no bug-fix value. |
| Defense-in-depth for stale submission state | Also reset `isSubmitting` in the existing reset-on-close effect (`Omnibar.tsx:587-599`) | pitfalls.md ("supports doing the defense-in-depth fix now"), citing git precedent commit `80f80da22` (same file, same root class: "state resets on wrong trigger") | Rely on the branch-level `finally` alone | The `finally` fix and the reset-effect fix cover two different failure surfaces (a promise that never settles vs. a component instance that outlives its `isOpen` cycle); pitfalls.md's precedent shows this file has hit this exact class of bug before and the fix pattern (reset in the close effect) is already established here. |

---

## Observability Plan
- **Logs**: None added. This is a client-side React state bug with no server-side or structured-logging surface; existing `console.error`/toast paths (if any) are unchanged.
- **Metrics**: None added. No metrics infrastructure exists for Omnibar submission state today, and adding one is out of scope for a bug fix of this size.
- **Alerts**: None. Not applicable to a frontend state-reset fix.

## Risk Control
- **Feature flag**: None needed — this is a strict bug fix (removes a stuck-disabled state), not a behavior change gated by opt-in/opt-out. Both branches keep their existing success/failure UX; only the "successful creation but state variable not reset" gap closes.
- **Rollback procedure**: Standard `git revert` of the PR. The change is additive (`finally` blocks, one extra `setIsSubmitting(false)` line) with no schema/API/data changes, so revert is safe and immediate.
- **Staged rollout**: Not applicable — ships as part of the normal frontend build (`web-app`), no separate deploy stage exists for this kind of change.

## Unresolved Questions
AC6 ("if a concrete root cause for `onClose()` not dismissing the modal is found, it is fixed or filed as a tracked follow-up") is only **partially** resolved, not fully closed:

- **Mechanism 1 (confirmed and fixed)**: `onClose`/`close` (`OmnibarContext.tsx:148-152`) is itself a trivial synchronous `setIsOpen(false)` that cannot throw or fail to fire. The actual, code-confirmed mechanism is the state leak — `isSubmitting` is never reset on close, so the long-lived Omnibar instance (never unmounted, see Domain Glossary) shows a stuck "Creating…" button on its *next* open. Task 1.1.3 (reset-on-close effect) and Tasks 1.1.1a/1.1.2a (`finally`) are the tracked, shipped resolution for this mechanism.
- **Mechanism 2 (named, unresolved, NOT ruled out)**: pitfalls.md §2 independently identifies a second, distinct, concrete mechanism that this plan does not fix: `Omnibar.css.ts:21-22` uses `position: "fixed"` on the overlay with no `createPortal(..., document.body)` wrapping it anywhere in `Omnibar.tsx`. Per this repo's own `.claude/rules/css-architecture.md`, this exact pattern is documented to silently misbehave (mispositioned or fails to visually dismiss) when any ancestor has a CSS `transform`, `filter`, or `will-change` — plausible on mobile Safari during virtual-keyboard show/hide. pitfalls.md explicitly labels this "plausible but unverified" and says not to present it as the confirmed cause — it is equally not something this plan can claim to have ruled out. Per AC6's own wording, a named mechanism that isn't fixed must be filed as a **tracked** follow-up, not merely mentioned in an "ideas, not tasks" list. Story 3.1.2/Task 3.1.2a below files it as a real backlog item, satisfying AC6 for this mechanism.
- **The "next open, not the same open" reinterpretation is itself unverified.** pitfalls.md §1 offers a plausible re-reading of the bug report — that the user saw the stuck button on a subsequent open, not continuously during the original session — as the explanation consistent with mechanism 1. This is a reasonable theory that fits the code, but nothing in the bug report (no repro, screenshot, or browser/OS detail) actually confirms the user reopened the omnibar before observing the stuck state, as opposed to it being visible continuously (which would point more toward mechanism 2). Treat this as a plausible theory, not a settled fact about what the user actually observed.
- **Out of scope, stated explicitly rather than silently omitted**: the `onCreateSession` never-resolving (hung request / dead network) failure mode is not addressed by this plan. Under that mode, `isSubmitting` stays stuck indefinitely regardless of this fix — neither the branch-level `finally` (never reached, since the `await` never settles) nor the reset-on-close effect (never triggered, since nothing drives an `isOpen` transition) can help. No code path in this plan can fix a promise that never settles; requirements.md's ACs only cover the resolve/reject cases, so this is legitimately out of scope, not an oversight.
- **Pre-mortem verification (2026-08-06, addresses pre-mortem.md P1 #1)**: before treating mechanism 2 as merely speculative, the current codebase was checked directly for the ancestor-transform/filter precondition the CSS hypothesis requires. `grep -n "transform\|will-change\|filter" web-app/src/components/sessions/Omnibar.css.ts` shows the only `transform`/`filter` declarations in that file apply to the `modal` div (a *descendant* of `overlay`, via the `scanlineReveal` open-animation, `Omnibar.css.ts:42-58`) and a chevron icon (`:213/:217`) — neither is an ancestor of `overlay`, so neither can break `overlay`'s `position: fixed` containing block. `web-app/src/app/layout.css.ts` and `web-app/src/app/Providers.tsx` (which renders `<OmnibarProvider>` directly under the app root, `Providers.tsx:43`) were also checked and contain no `transform`/`filter`/`will-change` on any ancestor in the render chain from the document root down to `<Omnibar>`. **This narrows, but does not eliminate, mechanism 2**: it rules out a static/build-time CSS cause on desktop, but does not check for a *dynamic*, JS-injected inline style (e.g. a mobile browser's own visual-viewport handling applying a transform to `<html>`/`<body>` during virtual-keyboard show/hide, which would not appear in any `.css.ts` file). Story 3.1.2's backlog item is updated to record this evidence and scope the remaining unverified surface precisely, rather than leaving mechanism 2 as an unexamined guess.

## Dependency Visualization
```
Phase 1: Core Fix
  Epic 1.1: isSubmitting reset resilience
    Story 1.1.1: SpawnShell branch try/finally
      Task 1.1.1a (edit Omnibar.tsx:1003-1027)
    Story 1.1.2: Alias branch try/finally
      Task 1.1.2a (edit Omnibar.tsx:1038-1071)
    Story 1.1.3: Reset-on-close defense-in-depth
      Task 1.1.3a (edit Omnibar.tsx:587-599)  [independent of 1.1.1/1.1.2]
    Story 1.1.4: Verify Escape/overlay-click never gated on isSubmitting
      Task 1.1.4a (grep-verify only, no code change expected)  [independent]
  Epic 1.2: Regression tests
    Story 1.2.1: Alias branch regression test
      Task 1.2.1a (new test in Omnibar.alias.test.tsx)  [depends on 1.1.2]
    Story 1.2.2: SpawnShell branch regression test
      Task 1.2.2a (new test, same file)  [depends on 1.1.1]
  Epic 1.3: Accessibility (promoted from optional to core — Axe CI gates web-app/src/ PRs)
    Story 1.3.1: aria-busy / aria-live on submit button
      Task 1.3.1a (edit Omnibar.tsx:1584-1591)  [independent]
Phase 3: Follow-up filing (no code change) — kept as "Phase 3" for continuity with the section heading below, even though the standalone "optional polish" phase was folded into Phase 1's Epic 1.3
  Epic 3.1: Tracked follow-ups
    Story 3.1.1: File SessionWizard.tsx same-anti-pattern bug
      Task 3.1.1a (create backlog item / note)  [independent]
    Story 3.1.2: File position:fixed/no-createPortal hypothesis (AC6)
      Task 3.1.2a (create backlog item, cites Omnibar.css.ts:21-22)  [independent, no code]

  1.1.1a ─┐
  1.1.2a ─┼─> 1.2.1a, 1.2.2a
  1.1.3a (independent)
  1.1.4a (independent, verify-only)
  1.3.1a (independent, now required — Axe CI gating)
  3.1.1a (independent, no code)
  3.1.2a (independent, no code)
```

---

## Phase 1: Core Fix

### Epic 1.1: `isSubmitting` reset resilience
**Goal**: After a successful `onCreateSession` call in the SpawnShell or Alias branch, `isSubmitting` always resets to `false` regardless of whether `onClose()` results in the modal unmounting or the component staying mounted (long-lived instance case).

#### Story 1.1.1: SpawnShell branch resets `isSubmitting` via `finally`
**As a** user submitting a `>shell` command in the Omnibar, **I want** the Create button to stop showing "Creating…" after the session is created, **so that** I'm not left staring at a disabled button or a stale loading state next time I open the omnibar.
**Acceptance Criteria**:
- AC2 (requirements.md): SpawnShell branch guarantees `isSubmitting` resets after success, matching the Alias branch guarantee.
  - *Given* the user submits `>shell ~/project`, *When* `onCreateSession` resolves successfully and `onClose()` is a no-op mock, *Then* `isSubmitting` becomes `false` (button is no longer disabled/"Creating…") without any error being shown.
- AC4 (requirements.md): Failure path unchanged.
  - *Given* the user submits `>shell ~/project`, *When* `onCreateSession` rejects, *Then* the error message is shown and `isSubmitting` becomes `false` (as today — no regression).
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.1.1a: Wrap SpawnShell branch body in `try/finally` (~3 min)
- In `Omnibar.tsx:1003-1027`, change the body so `setIsSubmitting(false)` runs in a `finally` block instead of only inside `catch`, mirroring the default branch's shape (`:1073-1169`) and the nested callback's shape (`:1550-1559`):
  ```tsx
  setIsSubmitting(true);
  setError(null);
  try {
    await onCreateSession(sessionData);
    if (shellCommand) addRecentShellCommand(shellCommand);
    onClose();
  } catch (err) {
    setError(err instanceof Error ? err.message : "Failed to create session");
  } finally {
    setIsSubmitting(false);
  }
  return;
  ```
- Remove the now-redundant `setIsSubmitting(false)` from inside the `catch` block (previously at `:1024`) since `finally` now covers both paths.
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 1.1.2: Alias-invocation branch resets `isSubmitting` via `finally`
**As a** user submitting `@pw retirement` (or any alias invocation) in the Omnibar, **I want** the Create button to stop showing "Creating…" after the session is created, **so that** the exact bug reported in backlog item `a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7` cannot recur.
**Acceptance Criteria**:
- AC1 (requirements.md): Alias branch guarantees `isSubmitting` resets after success even if `onClose()` doesn't unmount the modal.
  - *Given* the user submits `@pw retirement` and it resolves to an "Existing folder" session type, *When* `onCreateSession` resolves successfully and `onClose()` is mocked as a no-op, *Then* `isSubmitting` becomes `false`.
- AC3 (requirements.md): Happy path unchanged when `onClose()` works correctly — no new error toast/flash.
  - *Given* the user submits `@pw retirement` with a working `onClose`, *When* `onCreateSession` resolves, *Then* the modal closes exactly as it does today, with no new error rendered.
- AC4 (requirements.md): Failure path unchanged.
  - *Given* the user submits `@pw retirement`, *When* `onCreateSession` rejects, *Then* the error message is shown and `isSubmitting` becomes `false` (as today).
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.1.2a: Wrap Alias branch body in `try/finally` (~3 min)
- In `Omnibar.tsx:1038-1071`, change the body so `setIsSubmitting(false)` runs in a `finally` block instead of only inside `catch`:
  ```tsx
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
- Remove the now-redundant `setIsSubmitting(false)` from inside the `catch` block (previously at `:1068`).
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 1.1.3: Reset-on-close effect also clears `isSubmitting` (defense-in-depth)
**As a** user reopening the Omnibar after a prior successful submission, **I want** the Create button to always start in a fresh, non-submitting state, **so that** even if some future code path sets `isSubmitting` without a matching reset, the long-lived component instance self-heals on every close.
**Acceptance Criteria**:
- AC1 + AC2 defense-in-depth, per pitfalls.md and architecture.md's root-cause finding (the Omnibar instance never unmounts; `isSubmitting` is the one field the existing reset effect misses).
  - *Given* `isSubmitting` is somehow `true` while `isOpen` transitions to `false` (e.g. via the reset-on-close effect firing after a successful branch-level submit, or hypothetically from any future code path), *When* the effect at `Omnibar.tsx:587-599` runs, *Then* `isSubmitting` is set back to `false` alongside the other fields it already resets (`input`, `detection`, `formState`, `uiState`, `error`).
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.1.3a: Add `setIsSubmitting(false)` to the close-reset effect (~2 min)
- In `Omnibar.tsx:587-599`, add `setIsSubmitting(false);` alongside the existing `setError(null);` line (the effect already resets `error`, so this follows the same convention) — insert directly after `setError(null);`:
  ```tsx
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
- No dependency-array change needed — `setIsSubmitting` is a stable `useState` setter, same as the other setters already omitted from the deps array.
- Files: `web-app/src/components/sessions/Omnibar.tsx`

#### Story 1.1.4: Verify Escape and overlay-click dismiss paths are never gated on `isSubmitting`
**As a** developer confirming this fix doesn't paper over a worse trap, **I want** written confirmation that the user can always dismiss the modal via Escape or overlay-click even mid-submission, **so that** the UX risk ux.md flagged (user fully trapped) is ruled out rather than assumed.
**Acceptance Criteria**:
- Per ux.md's recommended "quick grep-verify" task (not expected to require a code change).
  - *Given* `isSubmitting` is `true` (a submission in flight), *When* the user presses Escape, *Then* `onClose()` fires unconditionally — confirmed at `Omnibar.tsx:908-910` (`if (isOpen && e.key === "Escape") { onClose(); }`, no `isSubmitting` check).
  - *Given* `isSubmitting` is `true`, *When* the user clicks the overlay backdrop, *Then* `onClose()` fires unconditionally — confirmed at `Omnibar.tsx:1216` (`<div className={overlay} onClick={onClose} ...>`, no `isSubmitting` check; the inner `modal` div at `:1222-1223` stops propagation so only backdrop clicks trigger this).
**Files**: `web-app/src/components/sessions/Omnibar.tsx` (read-only verification)

##### Task 1.1.4a: Grep-verify Escape/overlay-click are not gated on `isSubmitting` (~2 min)
- Confirm via `grep -n "isSubmitting" web-app/src/components/sessions/Omnibar.tsx` that neither the Escape handler (`:908-910`) nor the overlay `onClick={onClose}` (`:1216`) reference `isSubmitting` in any conditional.
- No code change expected. If the grep surfaces a gating bug, file it as a separate finding (not silently folded into this fix's diff) — but research (ux.md) found none.
- Files: `web-app/src/components/sessions/Omnibar.tsx` (verification only, no edit)

---

### Epic 1.2: Regression tests
**Goal**: Prove the fix with Jest/RTL tests that fail on the pre-fix code and pass after, per AC5.

#### Story 1.2.1: Alias branch regression test
**As a** future maintainer, **I want** an automated test proving the alias-submit path resets `isSubmitting` even when `onClose` is a no-op, **so that** this exact bug (backlog item `a6c87dbf-2ebb-4c6c-8fab-032d76fef1e7`) cannot silently regress.
**Acceptance Criteria**:
- AC5 (requirements.md): regression test covers alias branch success path resetting `isSubmitting` with `onClose` mocked as a no-op.
  - *Given* `renderOmnibar({ onClose: jest.fn() /* no-op */, onCreateSession: jest.fn().mockResolvedValue(undefined) })` and an alias detected via `AliasDetector` (same setup pattern as the existing `describe("Omnibar alias namePrefix population", ...)` block), *When* the user submits (e.g. via Cmd+Enter, matching the existing test's submission method) and the `onCreateSession` promise resolves, *Then* the Create button (`screen.getByRole("button", { name: /create session/i })` or equivalent) is no longer disabled and no longer reads "Creating…".
**Files**: `web-app/src/components/sessions/__tests__/Omnibar.alias.test.tsx`

##### Task 1.2.1a: Add alias-submit `isSubmitting` reset regression test (~5 min)
- In `web-app/src/components/sessions/__tests__/Omnibar.alias.test.tsx`, add a new `describe("Omnibar alias submit resets isSubmitting even when onClose is a no-op", ...)` block (or a new `it` inside the existing `describe("Omnibar alias namePrefix population", ...)` block at line 196) that:
  - Reuses `renderOmnibar()` (defined at line 166) with `onClose: jest.fn()` (a no-op that does nothing, simulating the long-lived-instance scenario from architecture.md) and `onCreateSession: jest.fn().mockResolvedValue(undefined)`.
  - Follows the same alias-registration `beforeEach` pattern (lines 197-213: `resetDefaultRegistry()`, `getDefaultRegistry().register(new AliasDetector([SSQ_ALIAS]))`) already present in this file.
  - Uses `typeAndDetect` (line 185) to type `"@ssq retirement"` (or reuse the existing `SSQ_ALIAS` fixture), then submits the same way the existing tests do (Cmd+Enter, per the comment at line 229-230) inside `await act(async () => { ... })`.
  - Asserts, after the submit promise settles: the Create button element found via `screen.getByRole` is not disabled and its text is not `"Creating…"` (i.e. `isSubmitting` is `false`).
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="Omnibar.alias.test"` to verify it passes against the fixed code (and, if feasible, temporarily revert Task 1.1.2a's `finally` locally to confirm the new test fails against the pre-fix code, then re-apply).
- Files: `web-app/src/components/sessions/__tests__/Omnibar.alias.test.tsx`

#### Story 1.2.2: SpawnShell branch regression test
**As a** future maintainer, **I want** the same regression coverage for the SpawnShell branch, **so that** both fixed branches are equally protected.
**Acceptance Criteria**:
- AC2 regression coverage (mirrors AC5's spirit for the SpawnShell branch specifically).
  - *Given* the user submits `>shell ~/project` with `onClose` mocked as a no-op and `onCreateSession` resolving successfully, *When* the submission completes, *Then* the Create button is no longer disabled/"Creating…".
**Files**: `web-app/src/components/sessions/__tests__/Omnibar.alias.test.tsx` (or a new colocated test in the same `__tests__/` directory if a SpawnShell-specific test file already exists — check `web-app/src/components/sessions/__tests__/` for an existing `Omnibar.spawnShell.test.tsx`-style file first; if none exists, add to `Omnibar.alias.test.tsx` as a second `describe` block to avoid duplicating all the `jest.mock(...)` boilerplate at the top of that file).

##### Task 1.2.2a: Add SpawnShell-submit `isSubmitting` reset regression test (~5 min)
- Add a second `describe("Omnibar spawn-shell submit resets isSubmitting even when onClose is a no-op", ...)` block in the same test file used for Task 1.2.1a, following the identical structure but typing a `>shell` command (e.g. `">shell ~/project"`) instead of an alias invocation, and using `onClose: jest.fn()` (no-op) + `onCreateSession: jest.fn().mockResolvedValue(undefined)`.
- Assert the same post-submit button state as Task 1.2.1a.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="Omnibar.alias.test"` (or the new file's pattern, if split out) to verify.
- Files: `web-app/src/components/sessions/__tests__/Omnibar.alias.test.tsx`

---

### Epic 1.3: Accessibility (promoted from optional Phase 2 — see Triad Review, Round 1)
**Goal**: Apply the file's existing `aria-live` convention to the submit button, per ux.md's low-cost recommendation.
**Promotion note (2026-08-06, resolves Product Triad Review UX-lens gap)**: originally scoped as droppable Phase-2 polish since no requirements.md AC requires it directly. Promoted into core (required) scope because this repo's own CLAUDE.md documents that "UX analysis CI runs on PRs touching `web-app/src/`: Axe Core (blocks on WCAG AA violations)" — this PR touches `web-app/src/components/sessions/Omnibar.tsx`, so it is subject to that gate regardless of whether this task is treated as optional in planning. Given the fix is cheap (~4 min, same file/same button already being edited by Epic 1.1) and directly matches an existing in-file convention (`Omnibar.tsx:1284`, `:1338`, `:1429`), there is no minimal-diff justification for leaving it droppable and risking a CI-time surprise instead of a plan-time decision.

#### Story 1.3.1: `aria-busy` and `aria-live` on the submit button
**As a** screen-reader user submitting the Omnibar form, **I want** the "Creating…" state to be announced, **so that** I get the same feedback sighted users get from the button label change — consistent with this file's existing convention at `Omnibar.tsx:1284`, `:1338`, `:1429`.
**Acceptance Criteria**:
- Not one of requirements.md's 6 numbered ACs, but now required (not droppable) per the Axe-CI gating rationale above. Satisfies ux.md's AC-7.
  - *Given* `isSubmitting` becomes `true`, *When* a screen reader user has focus near the submit button, *Then* `aria-busy="true"` is present on the button and the button's text change ("Creating…") is inside an `aria-live="polite"` region so it is announced.
**Files**: `web-app/src/components/sessions/Omnibar.tsx`

##### Task 1.3.1a: Add `aria-busy` and wrap button text in `aria-live` (~4 min)
- In `Omnibar.tsx:1584-1591`, add `aria-busy={isSubmitting}` to the `<button>` element and wrap the label text in a small `aria-live="polite"` span, matching the convention already used elsewhere in this file (e.g. `:1284`, `:1338`):
  ```tsx
  <button
    type="button"
    className={createButton}
    onClick={handleSubmit}
    disabled={!canSubmit || isSubmitting}
    aria-busy={isSubmitting}
  >
    <span aria-live="polite">{isSubmitting ? "Creating…" : "Create Session"}</span>
  </button>
  ```
- Verify no existing CSS selector in `Omnibar.css.ts` targets the button's direct text node in a way the new `<span>` wrapper would break (quick grep for `createButton` usage in the `.css.ts` file before committing).
- Files: `web-app/src/components/sessions/Omnibar.tsx`

---

## Phase 3: Follow-Up Filing (no code change in this plan)

### Epic 3.1: Tracked follow-ups from research
**Goal**: Ensure findings explicitly ruled out-of-scope by requirements.md are not silently dropped — file them so they're picked up on their own, per this repo's `.claude/rules/fix-flaky-tests-dont-defer.md` "name the recurring shape once" discipline (same spirit applied to a structural code smell here).

#### Story 3.1.1: File the `SessionWizard.tsx` same-anti-pattern bug
**As a** maintainer, **I want** the identical try/catch-without-finally anti-pattern found in `SessionWizard.tsx:238-253` (`onSubmit`) tracked as its own item, **so that** it doesn't quietly resurface as an unrelated future bug report.
**Acceptance Criteria**:
- Per features.md: "SessionWizard.tsx:238-253 has the SAME try/catch-without-finally anti-pattern — OUT OF SCOPE for this fix... should be flagged as a follow-up/filed bug."
  - *Given* this plan's fix ships for Omnibar.tsx only, *When* implementation completes, *Then* a tracked backlog item or note exists describing `SessionWizard.tsx:238-253`'s matching anti-pattern, so it is discoverable independently of this plan.
**Files**: None (tracking artifact only — no source file edited by this task).

##### Task 3.1.1a: File a follow-up item for `SessionWizard.tsx:238-253` (~3 min)
- Create a backlog item (e.g. via `mcp__stapler-squad__create_backlog_item`) or a note in the team's usual bug-tracking location, titled something like "SessionWizard.tsx onSubmit has the same try/catch-without-finally pattern fixed in Omnibar.tsx (omnibar-creation-stuck-modal)", citing `web-app/src/components/sessions/SessionWizard.tsx:238-253` and this project's `project_plans/omnibar-creation-stuck-modal/` as precedent/pattern reference.
- Do not edit `SessionWizard.tsx` itself — explicitly out of scope per requirements.md ("Any change to backend session-creation logic" is out of scope, and features.md names this as "different file, not named in requirements... file it, don't fix it inline here").
- Files: None (tracking only).

#### Story 3.1.2: File the `position: fixed` without `createPortal` hypothesis as a tracked follow-up
**As a** maintainer, **I want** the second, unverified candidate mechanism for AC6 (the CSS-driven non-dismissal hypothesis) tracked as its own backlog item instead of only appearing in an "ideas, not tasks" list, **so that** AC6's "fixed or filed as a tracked follow-up with the specific mechanism named" requirement is actually satisfied for this mechanism, and so it doesn't quietly disappear if the stuck-modal report recurs after this fix ships.
**Acceptance Criteria**:
- Per pitfalls.md §2 and this repo's `.claude/rules/css-architecture.md`: a concrete, named mechanism (`position: "fixed"` overlay with no `createPortal`, breaking under ancestor `transform`/`filter`/`will-change`, plausible on mobile Safari virtual-keyboard show/hide) is identified but unverified and not fixed by this plan.
  - *Given* this plan's fix ships (Tasks 1.1.1a/1.1.2a/1.1.3a) addressing only the state-leak mechanism, *When* implementation completes, *Then* a tracked backlog item exists naming the CSS/`createPortal` mechanism explicitly, citing `web-app/src/components/sessions/Omnibar.css.ts:21-22`, `.claude/rules/css-architecture.md`, and the escalation trigger from pitfalls.md §3 (only escalate to a `createPortal` migration if the stuck-modal report recurs independently of the `isSubmitting` leak, i.e. after this fix ships).
**Files**: None (tracking artifact only — no source file edited by this task).

##### Task 3.1.2a: File a follow-up item for the `position: fixed` / no-`createPortal` hypothesis (~3 min)
- Create a backlog item (e.g. via `mcp__stapler-squad__create_backlog_item`) titled something like "Investigate Omnibar modal position:fixed without createPortal as a possible non-dismissal cause (only if the stuck-modal report recurs after omnibar-creation-stuck-modal ships)".
- Name the specific mechanism explicitly in the item body (per AC6's wording): `Omnibar.css.ts:21-22`'s overlay uses `position: "fixed"` with no `createPortal(..., document.body)` anywhere in `Omnibar.tsx`; per `.claude/rules/css-architecture.md`, this silently mispositions or fails to visually dismiss when an ancestor has `transform`/`filter`/`will-change` — plausible on mobile Safari during virtual-keyboard show/hide.
- **Include the verification already done** (see Unresolved Questions above, pre-mortem P1 #1 resolution): static grep of `Omnibar.css.ts`, `layout.css.ts`, and `Providers.tsx` found no ancestor `transform`/`filter`/`will-change` in the current render chain from the app root to `<Omnibar>` — the only `transform`/`filter` in `Omnibar.css.ts` apply to `overlay`'s own descendant (`modal`) or a chevron icon, not an ancestor, so the static-CSS variant of this hypothesis is not currently present. State explicitly in the backlog item that the remaining unverified surface is narrower than originally scoped: a *dynamic*, JS/browser-injected transform on `<html>`/`<body>` during mobile virtual-keyboard show/hide (not checkable via static grep) — this is what a future investigator should target first, not a repeat of the static-CSS check already done here.
- Cite `research/pitfalls.md` (§2 for the mechanism, §3 for the trigger condition: only escalate to a `createPortal` migration if the stuck-modal report recurs *independently* of the `isSubmitting` leak fixed in this plan, i.e. after this fix has shipped).
- Do not implement the `createPortal` migration itself — explicitly out of scope for this plan; this task only files the tracked follow-up.
- Files: None (tracking only).

---

## Follow-Up Suggestions (not tasks — do not implement as part of this plan)
- **ux.md's AC-8 ("Created ✓" duplicate-resubmission confirmation beat) has no implementing task in this plan.** Cross-artifact consistency review and validation.md both flagged this: ux.md frames AC-8 as a UX acceptance criterion, but no task anywhere in Phase 1 (including Epic 1.3's accessibility work) adds a confirmed/"Created ✓" intermediate state — Epic 1.1 only fixes `isSubmitting` reset timing, Epic 1.3 only adds ARIA attributes to the existing idle/loading states, neither introduces a new visual "submitted" state. Explicitly demoted here to a follow-up suggestion, not a blocking gap: requirements.md's 6 ACs don't require it, and pre-mortem.md did not rate the duplicate-resubmission risk as P1 (the `finally` fix + reset-on-close effect together mean the only way a user reaches the pre-fill-and-resubmit state is if `onClose()` also genuinely fails to dismiss — narrowed by this plan's mechanism-2 verification above to a dynamic-mobile-only remaining surface, not the common case). Revisit if Story 3.1.2's follow-up investigation confirms mechanism 2 is real.
- Unify the three `handleSubmit` branches into a single shared code path (Approach C, rejected above) — only worth revisiting if a 4th near-identical branch is ever added.
- (The `position: "fixed"` + no-`createPortal` hypothesis previously listed here is now a tracked task, not just a suggestion — see Story 3.1.2/Task 3.1.2a above.)
- Consider surfacing post-success side-effect failures (e.g. `addRecentShellCommand` throwing) via the `NotificationContext`/`addNotification` toast system instead of the inline error, since the session was actually created (features.md, nice-to-have, not a blocking AC).
- Mobile-viewport Playwright/E2E test exercising the exact `@alias` reproduction path, per `.claude/rules/e2e-test-conventions.md` (ux.md) — deferred given the high spin-up cost for what is a unit-testable state bug.
- `useSessionService.ts`'s single shared `abortControllerRef` (~line 170, ~835-899) as a pre-existing double-submit race risk — noted by features.md, independent of this fix, not addressed here.
