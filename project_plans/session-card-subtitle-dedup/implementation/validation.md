# Validation Plan: session-card-subtitle-dedup

**Date**: 2026-08-06

## Happy Path Scenario

Given a session whose `title` equals its `branch` (e.g. both `"fix-auth"`), when `SessionCard`
renders, then the `Branch:` info row is suppressed while `Program:` and any non-matching rows
(Path, Working Dir, Goal, Repository, Pull Request) render exactly as they do today.

## Pure client-side confirmation

Confirmed genuinely true, not assumed: `isRedundantWithTitle` and `basenameOf` (plan.md Story
1.1.1) are plain string functions with no `fetch`, no ConnectRPC call, no Redux dispatch, no
`localStorage`/`sessionStorage` read, and no timer. The 5 call sites (plan.md Stories
1.2.1–1.2.5, `SessionCard.tsx:705,711-716,717,757,765`) only gate existing JSX conditionals — no
new data source is introduced. All "integration test" rows below are correctly `N/A`.

## Requirement → Test Mapping

AC1–AC8 below are plan.md's Refined Acceptance Criteria (supersedes requirements.md's initial
list per that document's own note).

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1 (Branch, exact case-sensitive trim match) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `isRedundantWithTitle_should_ReturnTrue_When_ValueExactlyMatchesTitle` | Unit | `isRedundantWithTitle("fix-auth", "fix-auth")` → `true` |
| AC1 (Branch, exact case-sensitive trim match) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `isRedundantWithTitle_should_ReturnFalse_When_ValueDiffersOnlyByCase` | Unit (near-miss) | `isRedundantWithTitle("Fix-Auth", "fix-auth")` → `false` (no case-folding) |
| AC2 (Path/Working Dir/Cloned To, basename-vs-title trim-exact match) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `basenameOf_should_ReturnLastPathSegment_When_GivenAPlainAbsolutePath` | Unit | `basenameOf("/home/user/worktrees/fix-auth")` → `"fix-auth"` |
| AC2 (Path/Working Dir/Cloned To, basename-vs-title trim-exact match) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `isRedundantWithTitle_should_ReturnFalse_When_BasenameIsANearMissSubstring` | Unit (near-miss) | `isRedundantWithTitle(basenameOf("/home/user/worktrees/fix-auth-2"), "fix-auth")` → `false` (`"fix-auth-2" !== "fix-auth"`, no substring match) |
| AC3 (Program row excluded from dedup — always renders) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_RenderProgramRowUnchanged_When_TitleMatchesProgram` | Unit (full-render, happy path) | `title="claude"`, `program="claude"` → `screen.getByText("claude")` still present in a `Program:` row |
| AC3 (Program row excluded from dedup — always renders) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_RenderProgramRowUnchanged_When_TitleDoesNotMatchProgram` | Unit (near-miss / control) | `title="fix-auth"`, `program="claude"` → Program row still renders identically (proves Program's render path takes no dedup branch at all, not just that it happens to survive a match) |
| AC4 (No duplicates → no visual regression) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_RenderAllRowsUnchanged_When_NoFieldMatchesTitle` | Unit (full-render, happy path) | plan.md's own AC4 fixture: `title="implement-oauth"`, `branch="feature/sso"`, `path=".../implement-oauth-work"`, `program="claude"`, `goal.goalText="Ship SSO login"` → every row present |
| AC4 (No duplicates → no visual regression) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_RenderPathRowUnchanged_When_BasenameIsNearMissOfTitle` | Unit (near-miss) | `title="fix-auth"`, `path="/home/user/worktrees/fix-auth-2"` → `Path:` row still renders with its `title=` tooltip intact |
| AC5 (Pure, directly-unit-testable helper) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `isRedundantWithTitle_should_ReturnFalse_When_ValueIsUndefinedOrNull` | Unit (happy path — no-render import) | `isRedundantWithTitle(undefined, "fix-auth")` and `isRedundantWithTitle(null, "fix-auth")` → both `false`, imported directly from `../SessionCard` with no component render (mirrors `SessionCard.pending-program.test.tsx`'s no-render style) |
| AC5 (Pure, directly-unit-testable helper) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `basenameOf_should_ReturnWholeTrimmedPath_When_PathHasATrailingSlash` | Unit (error/near-miss — documented quirk) | `basenameOf("/home/user/worktrees/fix-auth/")` → `"/home/user/worktrees/fix-auth"` (documented inherited quirk, not a new bug — Task 1.1.2a) |
| AC6 (Whitespace-trim resilience, still not case-insensitive) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `isRedundantWithTitle_should_ReturnTrue_When_ValueHasSurroundingWhitespace` | Unit (happy path) | `isRedundantWithTitle(" fix-auth ", "fix-auth")` → `true` |
| AC6 (Whitespace-trim resilience, still not case-insensitive) | `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` | `isRedundantWithTitle_should_ReturnFalse_When_ValueDiffersOnlyByCase` (same case as AC1's negative — restated per plan.md to make the trim-vs-fold boundary explicit) | Unit (near-miss) | `isRedundantWithTitle("Fix-Auth", "fix-auth")` → `false` |
| AC7 (Existing SessionCard tests continue to pass unmodified) | N/A — verification step, not a new test | `cd web-app && npx jest --no-coverage --testPathPatterns="SessionCard"` | N/A (test-run verification, not a test case) | Confirms `SessionCard.approval-suppression.test.tsx`, `SessionCard.click.test.tsx`, `SessionCard.pending-program.test.tsx` all pass unedited alongside the 2 new files |
| AC8 (No unintended accessibility information loss beyond the documented tradeoff) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_SuppressClonedToRow_When_BasenameMatchesTitle` | Unit (full-render, happy path) | `title="shared-fixes"`, `clonedRepoPath="/tmp/clones/shared-fixes"` → `screen.queryByText(/Cloned To:/)` is null; `session.title`/aria-label text (which already contains the basename) remains queryable, confirming the basename itself is not lost, only the `/tmp/clones/` prefix |
| AC8 (No unintended accessibility information loss beyond the documented tradeoff) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_KeepClonedToRow_When_ClonedRepoPathBasenameDoesNotMatchTitle` | Unit (near-miss / control) | `title="fix-auth"`, `clonedRepoPath="/tmp/clones/other-repo"` → `Cloned To:` row still renders in full, proving suppression is scoped to the matching case only, not a blanket hide |

### Adversarial-review-mandated additions (beyond plan.md's own Story 1.1.2/1.3.1 scope)

The adversarial review (`implementation/adversarial-review.md`) flags two gaps in the plan's
original test strategy that this validation plan closes explicitly:

| Gap flagged | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| **No test exercises the actual 5 JSX call sites — only the isolated predicates are unit-tested.** (adversarial-review.md, 3rd concern) A wiring bug at any call site — e.g. passing `session.path` instead of `basenameOf(session.path)` — would compile, pass the isolated predicate tests, and pass every pre-existing `SessionCard` test (none assert on row label/value text), yet AC1/AC2/AC3/AC4/AC8 all specify rendered-DOM outcomes nothing currently checks end-to-end. | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_SuppressBranchRow_When_BranchExactlyMatchesTitle` | Unit (full-render, happy path) | `title="fix-auth"`, `branch="fix-auth"` → `screen.queryByText(/Branch:/)` is null |
| (same gap, Path call site) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_SuppressPathRow_When_PathBasenameExactlyMatchesTitle` | Unit (full-render, happy path) | `title="fix-auth"`, `path="/home/user/worktrees/fix-auth"` → `screen.queryByText(/^Path:/)` is null |
| (same gap, Working Dir call site) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_SuppressWorkingDirRow_When_WorkingDirBasenameExactlyMatchesTitle` | Unit (full-render, happy path) | `title="my-project"`, `workingDir="/repos/my-project"` → `screen.queryByText(/Working Dir:/)` is null |
| (same gap, Cloned To call site) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_SuppressClonedToRow_When_BasenameMatchesTitle` | Unit (full-render, happy path) | See AC8 row above — same test covers both the wiring check and AC8 |
| (same gap, Goal call site — raw-text-before-truncation comparison) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_SuppressGoalRow_When_RawGoalTextExactlyMatchesTitle` | Unit (full-render, happy path) | `title="Fix login bug"`, `goal.goalText="Fix login bug "` (trailing space) → `screen.queryByText(/Fix login bug/)` restricted to the title element only, `Goal` label absent |
| (same gap, Goal near-miss — proves comparison happens pre-truncation) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_KeepGoalRow_When_RawGoalTextDivergesFromTitleDespiteTruncatedPrefixLookingSimilar` | Unit (near-miss) | `title="fix-auth"`, `goal.goalText="fix-auth and also update the docs and changelog entries so nothing regresses"` (>61 chars) → `Goal` row still renders (raw text `!==` title, so no false suppression from the truncated display string coincidentally starting with the title) |
| **The "all-fields-redundant" edge case (ux.md State C) is never exercised by a test — the plan's mitigation ("Program always renders, so the info block can never render fully empty") is a structural fact today but untested, and would silently break if a future PR "completes" AC3 by adding a Program guard.** (adversarial-review.md, 2nd concern; also closes features.md's "title matches multiple fields simultaneously / idempotency" case with the same fixture) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_SuppressAllFiveDedupRowsAndKeepProgramRow_When_EveryDedupEligibleFieldMatchesTitle` | Unit (full-render, edge case) | `title="fix-auth"`, `branch="fix-auth"`, `path="/home/user/worktrees/fix-auth"`, `workingDir="/home/user/worktrees/fix-auth"`, `clonedRepoPath="/tmp/clones/fix-auth"`, `goal.goalText="fix-auth"`, `program="claude"`, no `githubOwner`/`githubRepo`/`githubPrNumber` → asserts `Branch:`, `Path:`, `Working Dir:`, `Cloned To:`, `Goal` are all absent (`screen.queryByText(...)` → null, all 5) **and** `Program:` / `claude` is still present and non-empty in the same render — proving the 5 guards fire independently (not "first match wins") and the info block never collapses to zero rows |

## UX Acceptance Tests

Design source: `project_plans/session-card-subtitle-dedup/design/ux.md` Step 3 (9 UX acceptance
criteria), states A/B/C from Step 1. Repo precedent (`SessionCard.click.test.tsx`,
`SessionCard.approval-suppression.test.tsx`) uses `@testing-library/react` + Jest for all
component-level UX checks in this codebase — no Playwright component tests exist for
`SessionCard`, so RTL is the correct tool for every criterion below except #8, which is N/A, and
#2/#9, which are noted as needing a manual/visual fallback since RTL cannot assert computed
pixel height or animation/paint timing.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. No orphaned labels (label with no value, in states A/B/C) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_NeverRenderLabelWithoutAValue_When_RowIsSuppressedInAnyState` | RTL/Jest | Render States A (`fix-auth` fixture), B (`implement-oauth` fixture), and C (all-fields-redundant fixture); for each, assert every row rendered has both a `label` span with non-empty text AND a sibling `value` span with non-empty text — i.e. `screen.queryAllByText(/:$/)` (label pattern) never appears without an adjacent populated value, and no suppressed row leaves a bare label node in the DOM |
| 2. No dead visual gap in State C (info block shows only Program row, normal spacing) | N/A in RTL — computed layout/gap spacing (`SessionCard.css.ts`'s `gap: 6px` behavior) is not observable via jsdom, which does not run layout. **Fallback**: manual/visual verification, noted as the plan's own "Unresolved Questions" non-blocking follow-up (source-verified independently: `info`'s style is `flexDirection: column; gap: 6px` with no padding/border/min-height, so `gap` never inserts space around an empty set) | Manual (visual, non-blocking per plan.md) | During `sdd:6-verify`'s manual click-through, load a State C session card and visually confirm no phantom blank region between the title and status footer |
| 3. Label column stays aligned across mixed states (per-card, unaffected by dedup — row structure unchanged, only row count) | N/A in RTL — this is explicitly a computed-CSS/visual claim (`minWidth: 100px` rendering) that ux.md itself notes is "unaffected by dedup since row structure is unchanged." Covered indirectly by criterion 1's DOM-structure assertion (no row's `label`/`value` markup changes shape) | N/A — no dedicated test; covered by unchanged JSX structure + criterion 1 | — |
| 4. Predictability — no partial/fuzzy suppression (near-miss never suppressed) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_KeepBranchRowInFull_When_BranchIsANearMissOfTitle` | RTL/Jest | `title="fix-auth"`, `branch="fix-auth-2"` → `screen.getByText(/Branch:/)` and `screen.getByText("fix-auth-2")` both present (full value, not partially redacted) |
| 5. "No dead ends" | N/A — no navigation entry/exit point exists in this passive rendering rule (design/ux.md Step 3, item 5) | N/A | — |
| 6. Keyboard navigation unchanged (Repository/PR links, the only focusable elements in this block, are excluded from dedup scope) | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_KeepRepositoryAndPullRequestLinksFocusable_When_OtherRowsAreSuppressed` | RTL/Jest | Render State A/C fixture with `githubOwner`/`githubRepo`/`githubPrNumber`/`githubPrUrl` set (and unrelated dedup-eligible rows suppressed) → `screen.getByRole("link", { name: /GitHub repository/i })` and `screen.getByRole("link", { name: /Pull request/i })` both present and not `disabled`/`aria-hidden` |
| 7. Screen-reader information not lost beyond the documented, bounded `Cloned To` prefix tradeoff | `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` | `SessionCard_should_RetainTitleInAriaLabel_When_ClonedToRowIsSuppressed` | RTL/Jest | `title="shared-fixes"`, `clonedRepoPath="/tmp/clones/shared-fixes"` → the card's root element (`screen.getByLabelText(...)` targeting the existing `aria-label` at `SessionCard.tsx:411`, which already includes `session.title`) still contains `"shared-fixes"` in its accessible name, confirming the basename remains announced via the title even though the `Cloned To:` row itself is gone |
| 8. Color contrast | N/A — no new colors/styles introduced (design/ux.md Step 3, item 8) | N/A | — |
| 9. No dedup-caused layout shift/reflow flash (dedup computed pre-render, single paint) | N/A in RTL — jsdom does not paint or animate, so a "flash before collapse" cannot be observed via RTL; the guarantee instead follows structurally from dedup being a plain JSX conditional evaluated during the same render pass (verified by reading `SessionCard.tsx:705-777` — no `useEffect`/post-render mutation exists in this block) | Manual (visual, non-blocking) | Hard-refresh the session list in a running instance and visually confirm no flicker/collapse animation on a State A/C card |

## Test Stack
- **Unit**: Jest + `@testing-library/react` (RTL), matching the existing 3 `SessionCard` test
  files. Two new files:
  - `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx` — no-render,
    imports `isRedundantWithTitle`/`basenameOf` directly (mirrors
    `SessionCard.pending-program.test.tsx`'s pattern of testing an exported predicate without
    mounting the component).
  - `web-app/src/components/sessions/__tests__/SessionCard.dedup-integration.test.tsx` — full
    render via RTL's `render()`, reusing the exact mock block (`@connectrpc/connect`,
    `@connectrpc/connect-web`, `ReviewQueueContext`, `lib/store`, `sessionsSlice`,
    `useTerminalSnapshot`, `useFocusTrap`, `AppLink`, `Modal`, `useSessionActions`) already
    established in `SessionCard.click.test.tsx`/`SessionCard.approval-suppression.test.tsx`, plus
    a local `makeSession(overrides)` fixture builder following the same shape as those files'
    `makeSession`/`minimalSession` helpers. This is the file that closes the adversarial review's
    "no test exercises the actual JSX wiring" and "all-fields-redundant edge case untested" gaps.
- **Integration**: N/A — no data store, no ConnectRPC/backend call, no persistence. Confirmed by
  inspection of `isRedundantWithTitle`/`basenameOf` (plan.md Story 1.1.1) and all 5 call sites
  (plan.md Epic 1.2): pure string comparisons against props already present on the in-memory
  `Session` object passed to `SessionCard`. Nothing in this feature crosses a process or network
  boundary.
- **E2E / UX**: RTL full-render tests (`SessionCard.dedup-integration.test.tsx`) serve as the
  "UX/behavioral acceptance" layer per repo convention — no Playwright spec is added, matching
  the adversarial review's Minor #2 conclusion that this display-only change carries no
  `// +feature:` marker and no existing e2e precedent requires one. Two UX criteria (#2 visual
  gap, #9 no-flash) fall back to a manual/non-blocking check during `sdd:6-verify`, as jsdom
  cannot observe computed layout or paint timing.

## Migration Plan
N/A — plan.md contains no Migration Plan section (Step 5 skipped per instructions). No schema,
proto, or persisted-data change exists in this feature; `SessionCard.tsx` is display-only and
`Session` proto fields are explicitly untouched (requirements.md "Out of scope").

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| TypeScript/Jest | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}' --testPathPatterns="SessionCard"` | ≥80% line coverage on `SessionCard.tsx`'s new code (`isRedundantWithTitle`, `basenameOf`, and the 5 modified conditional guards) |

- All public service methods: N/A — no service layer in this feature; both new functions
  (`isRedundantWithTitle`, `basenameOf`) are covered happy-path + near-miss/error-path in
  `SessionCard.subtitle-dedup.test.tsx` per the Requirement → Test Mapping table above.
- All external integrations: N/A — confirmed no external call exists (see Test Stack →
  Integration).
- UX acceptance criteria: 9 of 9 criteria in `design/ux.md` Step 3 have either an RTL test, an
  explicit N/A (criteria 3, 5, 8 — structurally inapplicable or covered indirectly), or a
  documented manual fallback (criteria 2, 9 — jsdom cannot observe layout/paint).
