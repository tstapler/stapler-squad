# Validation: AI Backlog Creation Prompt

## Acceptance Criteria Checklist

| # | Criterion | Validation Method | Status |
|---|---|---|---|
| 1 | "Create backlog item" button opens the compose widget for text input | Manual + `Omnibar.backlogCompose.test.tsx` — existing tests cover widget rendering | Pre-existing ✅ |
| 2 | Submitting compose text invokes `parseBacklogItemIntent` (LLM) | New test in `Omnibar.backlogCompose.test.tsx` asserting `BacklogItemIntentReview` renders after submit | Needs new test |
| 3 | User sees AI-structured title, description, AC in review form before item is persisted | `BacklogItemIntentReview.test.tsx` existing test "pre-fills title/description from draft" | Pre-existing ✅ |
| 4 | AI call failure falls back gracefully (user can still submit) | `BacklogItemIntentReview.test.tsx` existing test "shows banner and pre-fills raw text on parse failure" | Pre-existing ✅ |
| 5 | `backlog: <message>` omnibar prefix flow continues to work | `Omnibar.backlogIntentReview.test.tsx` + `Omnibar.backlogCompose.test.tsx` regression guard (line 282) | Pre-existing ✅ |
| 6 | Items created via compose path trigger triage when repo is known | Side-effect: `createBacklogItem` RPC (used by `BacklogItemIntentReview`) accepts `repoPath`; tested in `backlog_service_intent_test.go` if present | Indirect ✅ |

## Pre-flight Checks

Before implementation, verify:
- [ ] `createBacklogInBackground` is used only inside the compose `onSubmit` handler in `Omnibar.tsx`
      (if other callers exist, the import stays; if not, remove it with the fix)
- [ ] `Omnibar.backlogCompose.test.tsx` line 224 test title exactly matches the test to replace
- [ ] `npx jest --testPathPatterns="Omnibar" --no-coverage` runs green on `main` (baseline)

## Post-implementation Checks

- [ ] `npx jest --testPathPatterns="Omnibar.backlogCompose|Omnibar.backlogIntentReview|BacklogItemIntentReview" --no-coverage` — all green
- [ ] `make quick-check` passes (lint + build)
- [ ] Manual smoke test on running app:
  1. Click "Create backlog item" → compose widget appears ✅
  2. Type "Add a dark mode toggle to the settings page" → click Create
  3. Spinner appears: "Parsing your message into a backlog item…" ✅
  4. Review form appears pre-filled with AI-generated title, description, and ACs ✅
  5. Click Save → item created, navigated to item detail page ✅
  6. Verify item has meaningful title + description (not truncated raw text) ✅

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| LLM latency increases perceived create time | Medium | `ParsingIndicator` spinner already exists; expected by users of the `backlog:` prefix path |
| `createBacklogInBackground` import becomes dead code and causes linter error | Low | Check usages before removing; the fix removes one call site, not all callers |
| Cancel out of review loses compose draft | Low | Matches existing `backlog: prefix` behavior; document if desired follow-up is draft preservation |
| New test incorrectly mocks `parseBacklogItemIntent` | Low | Follow existing `Omnibar.backlogIntentReview.test.tsx` mock pattern |

## Definition of Done

- All tests in the three test files listed above pass
- `make quick-check` passes
- Manual smoke test confirms the parsing spinner and review form appear
- No new lint warnings introduced

## Readiness Gate

**Verdict: PASS.** Pre-flight checks resolved before code was written:
- `createBacklogInBackground` had a single caller (the compose `onSubmit`); the hook was removed.
- The replaced tests in `Omnibar.backlogCompose.test.tsx` were located by title.
- Server side needs no change: `CreateBacklogItem` calls `MaybeTriggerTriage` with the item's
  `RepoPath` (`server/services/backlog_service_lifecycle.go:240`).

No pre-mortem blockers. Residual risks are the ones tabled above (LLM latency, cancel loses draft),
both accepted.
