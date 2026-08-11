# Research: Feature Landscape — session-card-subtitle-dedup

## 1. Current info-row rendering block

`web-app/src/components/sessions/SessionCard.tsx`

Primary title, line 454: `{session.title}` — always rendered, unconditionally.

Secondary info rows, lines 700–778 (inside `<div className={info}>`):

| Row | Line | Condition to render | Field(s) that could duplicate title |
|---|---|---|---|
| Program | 701–704 | always (no guard) | `session.program` |
| Branch | 705–710 | `session.branch` truthy | `session.branch` |
| Path | 711–716 | always (no guard) | `session.path` (full path, has `title=` tooltip attr too) |
| Working Dir | 717–722 | `session.workingDir` truthy | `session.workingDir` |
| Repository | 723–739 | `session.githubOwner && session.githubRepo` | rendered as `owner/repo` link text |
| Pull Request | 740–756 | `session.githubPrNumber > 0 && session.githubPrUrl` | rendered as `#N` — unlikely to match title, low priority |
| Cloned To | 757–764 | `session.clonedRepoPath` truthy | `session.clonedRepoPath` (full path) |
| Goal | 765–777 | `session.goal?.goalText` truthy | `session.goal.goalText` (truncated to 61 chars via `truncateGoal`, plus optional ` · N/M done` suffix) |

Additionally, `session.category` (line 640–642) and `session.tags` (line 643–664) are rendered in the header area, above the info block — also plausible dup targets per the requirements' field list, though not in the 700–778 "info row" block itself.

Also outside the row block but literal duplicate-display precedent: `session.workflowId` badge (line 616–626) already does a title-vs-name preference — `title={session.workflowName || session.workflowId}` — not a title-dedup pattern, but shows the codebase already prefers a human label over a raw id when available.

## 2. How `session.title` gets set/defaulted — duplication is the DEFAULT case, not an edge case

**Backend (`server/services/session_service.go`)**: `Title` is always taken verbatim from `req.Msg.Title` (lines 866, 917, 1484) — the server does not itself default title to branch or a directory basename. All "title mirrors another field" behavior originates client-side, at creation-form default state.

**Frontend — `useTitleAsBranch` (default `true`)**: `web-app/src/components/sessions/Omnibar.tsx` line 93 (`useTitleAsBranch: true` in default `OmnibarFormState`) and `OmnibarCreationPanel.tsx` (checkbox at lines 558–565/596–599, label "Use session name as branch name"). At submit time:

- Line 1080: `finalBranch = sessionName.trim();` (regular new_worktree / new_project→new_worktree path)
- Line 1045: `aliasFinalBranch = sessionTitle;` (alias new_worktree path)

Both assign the branch **verbatim** from the session name/title — no slugification, no case change. Since `useTitleAsBranch` defaults to `true` and the checkbox is visible/checked out-of-the-box for the most common creation mode ("New branch (isolated)" / `new_worktree`), **`session.title === session.branch` exactly is the default outcome for new-worktree session creation**, not a rare coincidence. This single case alone justifies branch-row dedup as the highest-value target, well above "trivial edge case."

**Path/workingDir**: No equivalent literal default was found wiring `path` or `workingDir` from title. `namegen.GenerateAndCreate`/`GenerateUnique` (referenced in `server/services/session_service.go:1366` and the one-off flow) generates a random directory name unrelated to title, so one-off sessions won't collide. Worktree paths are constructed from repo/branch by the git worktree scaffolding (`session/git/scaffolding.go`), not searched exhaustively here, but no code path was found assigning `path`/`workingDir` directly equal to title — only **basename-level** matches are plausible (e.g. a worktree dir named after the branch, which itself equals the title under the default above), not full-path equality. This matches the requirement's own framing: "title is the same as the working-directory basename," i.e., a substring/basename match, not full string equality — this needs different comparison logic than the branch case.

**Goal text**: No code path sets `goal.goalText` from title; goals are set independently via `set_session_goal`. Coincidental full match is possible but not systemic — same treatment as any other free-text field (exact-match compare only).

## 3. Existing SessionCard tests referencing these rows

`web-app/src/components/sessions/__tests__/SessionCard.click.test.tsx` is the only test file under `__tests__/` asserting on the specific rows in scope:

- Lines 114, 121–124: `expect(screen.queryByText("Goal")).toBeNull()` — asserts the Goal row is **absent** when `goalText` is empty (this precedent for "suppress row when empty" is exactly the same shape a dedup guard would use).
- Lines 132–136: asserts `Goal` row renders (`getByText("Goal")`) when `goalText` is a long string, using a `makeGoalSummary()` builder with overridable fields (`goalText`, `tasksTotal`, `tasksDone`).
- Lines 146–159: asserts truncation and the `N/M done` suffix render together.

No test in `__tests__/` currently asserts on literal `"Program:"`, `"Branch:"`, `"Path:"`, `"Working Dir:"`, `"Repository:"`, `"Pull Request:"`, or `"Cloned To:"` label text — a grep across `__tests__/*.tsx` for those label strings returned zero matches outside comments. This means **no existing test asserts these rows are unconditionally present**, so a dedup change has no direct test collisions to fix for those rows — only the Goal-row tests in `SessionCard.click.test.tsx` need checking (their `goalText` values, e.g. `"a goal"`, don't collide with any `session.title` value used in the same test file, so they should be unaffected, but should be re-run to confirm after implementation).

`SessionCard.approval-suppression.test.tsx` and `SessionCard.pending-program.test.tsx` were not found to reference these row labels (title strings only in scope for badges/status, not the info block).

No fixture in these tests currently sets `session.title` equal to `session.branch`, `session.path`, `session.workingDir`, or `session.goal.goalText` — so realistic "already duplicated" fixtures will need to be added by the implementation/test-writing phase; they don't already exist to crib from.

## 4. Edge cases that matter

- **Empty/whitespace title**: `session.title` should never be empty in practice (server rejects empty title at creation — `session_service.go:1259` `if req.Msg.Title == "" { ... }` returns `InvalidArgument`), but a title that is only whitespace after trim is not explicitly guarded server-side use the same guard pattern the requirement's referenced `paneMeta()` uses (skip parts equal to title) — normalize via `.trim()` before comparing so `"  my-title  "` still dedups against `"my-title"`.
- **Case-only difference**: branches are often lowercased/kebab-cased by convention even though the code doesn't currently force that (see §2 — no slugification is applied), so a user-typed title like `"My Feature"` typed into a manually-edited branch field as `"my-feature"` would NOT match on exact string compare. Decide explicitly whether comparison is case-sensitive (matches the "trivially match" language in requirements, suggesting case-insensitive is in scope) — but default `useTitleAsBranch` produces an **exact** match, so case-sensitivity is not required for the majority default case, only for the manually-diverged case.
- **Basename-only match for Path/Working Dir**: Because path values are full filesystem paths (e.g. `/home/user/.stapler-squad/worktrees/repo/my-feature`), never expect full-string equality against title — always compare `path.split("/").pop()` (or workingDir's basename) against title, per the requirement's own note. Reuse existing basename logic if any exists in the codebase (`session/instance*.go` or web-app path utils) rather than inventing new splitting logic — this repo research phase did not find an existing shared basename helper on the frontend (`web-app/src/lib/utils/`); check the security/utility research track for one before writing a new one.
- **Title matching multiple fields at once** (e.g., `useTitleAsBranch` true AND path basename also happens to equal title, e.g. worktree dir literally named after branch): each row's dedup check should be independent/idempotent — suppressing Branch doesn't imply suppressing Path — so implement as N independent per-row guards, not a single "first match wins" reducer (that would risk hiding Path when only Branch was the "intended" duplicate).
- **RTL / very long titles**: The `title` display span (line 448–455) has no truncation applied to it currently (unlike Goal, which uses `truncateGoal(...,61)`); dedup logic should compare raw untruncated values, not the (possibly future) truncated display string, to avoid a truncated title falsely mismatching a full-length branch name.
- **Title matching `goal.goalText`**: `goalText` is compared/rendered through `truncateGoal(text, 61)` — if title is compared for equality, compare against the **raw** `goal.goalText`, not the truncated display string, same reasoning as above; a title of exactly 61+ chars that happens to equal a truncated goal display would falsely appear to differ if compared post-truncation.

## 5. Existing dedup-against-title precedent in the codebase

Grepped for `!== session.title`, `=== session.title`, `!== title`, `=== title` across `web-app/src`. Only one hit, and it's unrelated to display dedup:

- `SessionCard.tsx:345` — `if (!trimmed || trimmed === session.title) { ... }` inside the inline-rename submit handler (`handleInlineSave`), used to no-op a rename when the new value is empty or unchanged from the current title. This is a "skip a state update if value === title" pattern, not a "skip rendering a row if value === title" pattern — same comparison shape (equality against `session.title`) but different purpose (avoid a no-op API call, not avoid duplicate display).

**No existing "paneMeta()-style" consolidated subtitle builder, and no existing per-row title-dedup guard, exists anywhere in this codebase.** This confirms the requirements doc's own framing: the referenced herdr-web pattern is not literally portable — there is no single line-builder function to extend, and no established local convention beyond the rename no-op above to mimic for the comparison-against-title shape itself (equality/trim check). The dedup implementation is new, but should reuse the same trim-then-compare idiom already established in `handleInlineSave` for consistency with the one existing local precedent.

## Summary of files relevant to implementation

- `web-app/src/components/sessions/SessionCard.tsx` (lines 640–778 for row block; line 345 for the one local precedent idiom)
- `web-app/src/components/sessions/__tests__/SessionCard.click.test.tsx` (existing Goal-row tests to re-verify post-change; pattern to extend for new dedup test cases)
- `web-app/src/components/sessions/Omnibar.tsx` (line 93 `useTitleAsBranch: true` default; lines 1043–1080 branch-from-title assignment) — evidence for how often title/branch duplication actually occurs
- `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (lines 558–620, the "Use session name as branch name" checkbox UI)
- `web-app/src/lib/utils/string.ts` (`truncateGoal` — the raw-vs-truncated comparison caveat for the Goal row)
