# sdd:6-verify Report — repo-path-picker-parity

**Date**: 2026-08-02
**Diff scope**: PR #296, `backlog/stapler-squad-repo-path-picker-parity` vs `origin/main`, scoped to this feature's files (`OmnibarCreationPanel.tsx`, `RepoPathInput.tsx`/test, `PathCompletionDropdown.tsx`, `useSessionRepoPaths.ts`/test, `sessionsSlice.ts`/test, `NewShellDialog.test.tsx`, `tests/e2e/repo-path-picker-parity.spec.ts`, registry entry).

## Technology Surface

| Technology | Files | Review approach |
|---|---|---|
| TypeScript/React | OmnibarCreationPanel.tsx, RepoPathInput.tsx, PathCompletionDropdown.tsx, useSessionRepoPaths.ts, sessionsSlice.ts | `ui-react-best-practices` skill (general-purpose agent) |
| Playwright e2e | tests/e2e/repo-path-picker-parity.spec.ts | refactor-candidates review + `.claude/rules/e2e-test-conventions.md` |

## Layer 1 — Idioms (React)

| Finding | Severity | Action |
|---|---|---|
| Existing Worktree Path `RepoPathInput` lacks `required` while its label reads "…Path *" | SUGGEST | Not fixed — pre-existing gap (the `<select>` variant of the same field never had `required` either; `canSubmit` gates submission instead of native HTML validity). Out of this project's scope per requirements.md. Noted as follow-up. |
| `stopImmediatePropagation()` blast radius broader than comment implied | NITPICK | Fixed — comment tightened in `RepoPathInput.tsx` (commit ee8352d6c). |
| `showDropdown` in `useCallback` deps instead of raw inputs | — | Verified correct, no change needed. |

No MUST FIX findings.

## Layer 2 — Architecture

**Verdict: CLEAN — no blockers.** Implementation matches plan.md almost exactly (showDropdown-gated Escape guard, combobox ARIA triad, 3-key comparator, selector swap, both field substitutions). Convergence onto the same `RepoPathInput`/`useSessionRepoPaths`/`sessionsSlice` dependencies as the app's 4 other consumers — not new coupling.

- CONCERN: `stopImmediatePropagation()` idiom now appears 5x in the codebase with no shared abstraction. Plan already considered and rejected extracting a hook as premature; reasonable for now, watch for a 6th occurrence.
- NITPICK: `required` asymmetry (see Layer 1) — confirmed pre-existing, not a regression.
- NITPICK: two independently-tested layers instead of one integration test for the full recency chain — acceptable, both layers are pure functions.

## Refactor Candidates

| Finding | Effort | Action |
|---|---|---|
| Duplicated e2e test bodies across "Parent Directory" / "Existing Worktree Path" describe blocks in `repo-path-picker-parity.spec.ts` | ~30 min | Follow-up — not fixed now. Diff is CI-green; refactoring passing e2e specs pre-ship trades ship risk for cosmetic gain. |
| Mixed formal-ID / free-text `testIds` in registry entry | ~5 min | Not fixed — free-text entries match real `describe` block names per `feature-registry.md` convention; not actually inconsistent. |
| Nested ternary in `sessionsSlice.ts` tiebreak | ~2 min | Fixed — replaced with `localeCompare` (commit ee8352d6c). |

## Layer 3 — Correctness & Tests

Acceptance criteria (plan.md stories → AC1–AC7): all covered, matches requirements.md R1–R7.

**Unit tests**: `cd web-app && npx jest --no-coverage --testPathPatterns="RepoPathInput|useSessionRepoPaths|sessionsSlice|NewShellDialog"` → **62 passed, 0 failed** (pre-existing unrelated `className` console warnings in `RepoPathInput.test.tsx`/`NewShellDialog.test.tsx`, confirmed not touched by this diff). Re-ran after the two Layer 1/2 fixes: **50 passed, 0 failed**.

**E2E tests**: not re-run locally (would require building the Go binary + spinning up the isolated Playwright server, ~10 min). PR #296's CI (`E2E Feature Video Capture`, shards 1/2 and 2/2, run against this exact commit) already ran the full suite including `repo-path-picker-parity.spec.ts`, `session-create-new-project.spec.ts`, and `session-create-existing-worktree.spec.ts` — all green (see `gh pr view 296 --json statusCheckRollup`).

**Security**: no auth/authz, no external HTTP calls, no user input reaching a shell/DB/file-path sink beyond the pre-existing local-path text field — inline scan only, no findings.

**Error handling**: N/A — pure UI state, no new external calls.

**Observability**: N/A — no new service boundary; plan.md has no Observability Plan section (correctly, for a client-only UI change).

## Layer 4 — UX & Behavioral

Skipped local browser re-verification — `design/ux.md`'s criteria are already covered by `tests/e2e/repo-path-picker-parity.spec.ts` (desktop + 390×844 viewport geometry, dropdown overflow, free-text entry, Escape scoping), which passed in CI on this commit.

## Fix Loop Summary

| Layer | Iterations used | Items resolved | Items remaining |
|---|---|---|---|
| L1+L2 | 1/5 | 2 (comment tightening, ternary→localeCompare) | 0 blocking (2 follow-ups noted, not blocking) |
| L3 | 0/5 | — | 0 |
| L4 | 0/5 | — | 0 |

## Verdict

✅ **PASS** — all layers clean, two cheap idiom fixes applied and committed (ee8352d6c). Ready for `/backlog/review`.
