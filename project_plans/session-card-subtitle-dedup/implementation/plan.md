# Implementation Plan: session-card-subtitle-dedup

**Feature**: Suppress `SessionCard` secondary info rows (Branch, Path, Working Dir, Cloned To, Goal) whose value exactly duplicates the visible session title, via a colocated pure predicate — Program/Repository/Pull Request rows are explicitly excluded from dedup.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: ADR-001 (row-level suppression, not value-only, for redundant info rows)

---

## Step 0.5 — Alternatives Considered

**A. Consolidated `paneMeta()`-style single subtitle line** (the pattern literally referenced in the source GitHub issue). *Strength*: matches the cited prior art exactly — one string-builder function, one line of output. *Weakness*: doesn't fit `SessionCard`'s fixed-width label/value row layout, and the Repository/Pull Request rows are clickable `<a>` links that cannot be joined into a single plain-text line without losing their click-through affordance. Rejected — requirements.md itself already flags this as not a literal fit for this codebase, and ux.md independently reaches the same conclusion.

**B. Per-row suppression via a small colocated pure predicate + conditional JSX** (chosen). *Strength*: minimal diff; matches two precedents already in the file (`hasPendingProgramChange` as a colocated exported predicate, and the `session.branch && (...)` conditional-render idiom already used for 5 of the 8 rows); preserves each row's independent semantics (links stay links, tooltips stay tooltips except where explicitly traded off). *Weakness*: touches 5 separate call sites instead of one central function, so slightly more surface area for a copy-paste mistake — mitigated by table-driven unit tests on the shared predicate rather than per-call-site tests.

**C. CSS-only visual de-duplication** (e.g., `text-overflow`/visually-hiding a row when it looks like a repeat). *Strength*: no JS logic to write or maintain. *Weakness*: infeasible — CSS cannot compare two arbitrary runtime string values, and even if it could, hiding via `display:none`/`visibility:hidden` leaves the duplicate node in the accessible tree (screen readers still announce it), directly working against AC8 and ux.md's explicit recommendation to remove the node, not just hide it visually. Rejected outright.

**Chosen: B.**

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SessionCard` | The React component rendering one session's summary card in the session list (`web-app/src/components/sessions/SessionCard.tsx`). | Existing component; this feature only edits it. |
| info row | One `<div className={infoRow}>` block in the card body (`web-app/src/components/sessions/SessionCard.tsx:700-778`) — a `Label:` + value pair. There are 8 today: Program, Branch, Path, Working Dir, Repository, Pull Request, Cloned To, Goal. | Only Branch, Path, Working Dir, Cloned To, Goal are in this feature's dedup scope. |
| dedup scope | The 5 info rows (Branch, Path, Working Dir, Cloned To, Goal) eligible for title-redundancy suppression in this change. | Program, Repository, Pull Request are explicitly excluded — see Pattern Decisions #5 and architecture.md. |
| `isRedundantWithTitle(value, title)` | New exported pure predicate: returns `true` when `value.trim() === title.trim()` (exact, case-sensitive, whitespace-trimmed comparison). Colocated in `SessionCard.tsx` above the component, mirroring `hasPendingProgramChange`. | Does not itself do basename extraction — callers pre-normalize path-shaped values via `basenameOf` before calling it. |
| `basenameOf(pathValue)` | New exported pure helper: `pathValue.trim().split("/").pop() || pathValue.trim()` — the last `/`-separated segment of a trimmed path string. Colocated next to `isRedundantWithTitle`. | Reuses the existing repo idiom already duplicated in `SessionsTable.tsx`, `page.tsx`, `RecentFilesSection.tsx`, `useAvailablePrograms.ts` (build-vs-buy.md) rather than `path.basename` (no Node polyfill in this `"use client"` component). |
| row-level suppression | Hiding an entire info row (label + value + any tooltip) by not rendering its JSX node, vs. rendering the row but leaving its value blank/altered. | This plan chooses row-level suppression everywhere — see ADR-001. |
| trim-exact match | The chosen comparison semantics: `.trim()` both sides, then `===`. No case-folding, no substring/basename matching inside `isRedundantWithTitle` itself. | See Pattern Decisions #2 for why case-insensitive matching was rejected. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall shape | Plain colocated exported functions (Transaction Script style — no class, no interface) | architecture.md, build-vs-buy.md, AC5 | Extract to `web-app/src/lib/utils/string.ts` next to `truncateGoal` (stack.md) | AC5 explicitly requires mirroring `hasPendingProgramChange`'s colocated, exported-above-the-component pattern; single consumer today (interface-pollution-checklist: don't extract until a 2nd consumer exists); `lib/utils/` is reserved for cross-component-reusable logic, and this logic is scoped to exactly which 5 of `SessionCard`'s 8 rows participate — a `SessionCard`-specific decision, not a generic string utility. |
| Comparison semantics | Exact match after `.trim()` only — no case-folding | pitfalls.md, architecture.md, requirements.md AC1 | Case-insensitive / whitespace-normalized (stack.md) | Requirements AC1 explicitly requires case-*sensitive* Branch comparison — directly contradicting case-folding. pitfalls.md's "fix-auth" vs "fix-auth-2" false-positive warning (about substring matching) generalizes: over-matching in *either* dimension (substring or case) risks silently equating two values a user deliberately made different (e.g. title `"API-Fix"` vs branch `"api-fix"`). No AC demands case-insensitivity; AC6 only demands whitespace resilience. Trim-only is the minimal change that satisfies AC6 without overreaching. |
| Basename extraction (Path / Working Dir / Cloned To) | `.trim()` then `.split("/").pop() || trimmed` (existing repo idiom) | build-vs-buy.md | `path.basename` (Node `path` module) | No browser polyfill available in this `"use client"` component; reusing the idiom already duplicated 4x elsewhere means no new edge-case surface (including the trailing-slash quirk) beyond what already ships today. Promoting the 4 existing duplicates into one shared helper is flagged by build-vs-buy.md as optional polish — explicitly out of scope here to avoid expanding this change's blast radius. |
| Row suppression granularity | Hide the entire row (existing `session.branch && (...)` conditional idiom) | architecture.md, ux.md, pitfalls.md, ADR-001 | Keep the row, blank/replace only the value | Matches how every other optional row in the file already behaves (present-with-value or fully absent — never present-with-empty-value); simpler diff, same idiom reused 5x. Full untruncated value (including any parent-directory prefix lost for Path/Working Dir/Cloned To) remains one click away in `SessionDetailView.tsx` (verified: `web-app/src/components/sessions/SessionDetailView.tsx:881-884`, `:906`, `:1180-1183` render `session.path`/`workingDir`/`clonedRepoPath` unconditionally). Full reasoning in ADR-001. |
| Program row | Excluded from dedup scope — always renders, unchanged | architecture.md (flagged as open item) | Add `session.program && !isRedundantWithTitle(...)` guard, matching requirements.md's original AC3 | The Program row today has **no** presence guard at all (renders unconditionally even when empty) — adding one is a materially larger behavior change than "add a check to an existing conditional" (the pattern AC5 asks for). `title === program` collisions are low-value (program is almost always `"claude"`/`"aider"`, rarely title-worthy). Side benefit: keeping Program always-rendered guarantees the `info` block can never render fully empty, which resolves ux.md's "all-fields-redundant empty gap" concern with zero extra code (see next row). |
| Empty-`info`-block edge case | No fallback-row code; rely on Program always rendering (row above) | This plan + `web-app/src/components/sessions/SessionCard.css.ts:293-297` | Dedicated "always show ≥1 row" fallback special-case | Moot given Program always renders. Independently verified via source read (not a runtime/browser claim): `info`'s style is `display:flex; flexDirection:column; gap:6px` only — no padding/border/min-height — so even in a hypothetical zero-children case it collapses to zero height; `gap` only inserts space *between* children, never around an empty set. ux.md itself preferred the simpler option when available. |
| Test strategy | Pure-predicate unit tests only, new `SessionCard.subtitle-dedup.test.tsx`, no full component render | architecture.md, `SessionCard.pending-program.test.tsx` precedent | Full-render integration test (`SessionCard.approval-suppression.test.tsx`'s heavier mock-everything pattern) | Dedup behavior is entirely captured by the two exported pure functions; a full render test would require `SessionCard`'s Redux/analytics mock harness for no added coverage. AC5 asks for a "pure, unit-testable helper," not an integration test. |

---

## Observability Plan
- **Logs**: N/A — pure client-side display logic, no new log points warranted for a conditional-render change.
- **Metrics**: N/A — no user-facing behavior change worth tracking beyond what existing frontend error monitoring already covers.
- **Alerts**: N/A.

## Risk Control
- **Feature flag**: None — display-only change with a narrow, well-tested predicate; a broken predicate fails closed (rows render, matching today's behavior) only if `isRedundantWithTitle` is miscalled, not if it's simply absent, so the blast radius of a bug is "a row that should be hidden isn't," not data loss or a crash.
- **Rollback procedure**: Standard `git revert` of the merge commit — no schema, no proto, no data migration involved.
- **Staged rollout**: Not needed — ship behind normal PR review + existing Jest/e2e CI gates (`make quick-check` equivalent for `web-app`: `cd web-app && npx jest --no-coverage`).

## Unresolved Questions
None blocking implementation. Both research disagreements (comparison semantics, helper location) and all four flagged open items (Program guard, basename semantics, Goal raw-vs-truncated comparison order, tooltip-loss acceptance, empty-info-block edge case) are resolved above with rationale. Optional, non-blocking follow-up noted only: during `sdd:6-verify`'s manual click-through, a maintainer may want to eyeball a real all-else-redundant session card once to confirm the Program-only row doesn't look sparse/odd — not required to ship, since the CSS collapse behavior is independently verified from source (see Pattern Decisions table).

## Dependency Visualization
```
Epic 1.1 (Dedup Helper Functions)
  Story 1.1.1 (add predicates)  ──▶  Story 1.1.2 (unit tests)
            │
            ▼
Epic 1.2 (Apply Dedup to Info Rows)   [each story independent of the others, all depend on 1.1.1]
  Story 1.2.1 (Branch)
  Story 1.2.2 (Path)
  Story 1.2.3 (Working Dir)
  Story 1.2.4 (Cloned To)
  Story 1.2.5 (Goal)
            │  (all five)
            ▼
Epic 1.3 (Regression Verification)
  Story 1.3.1 (run full SessionCard test suite + lint)
```

---

## Refined Acceptance Criteria (authoritative — supersedes requirements.md's initial list)

**AC1 — Branch row, exact case-sensitive trim match.**
- *Given* `session.title = "fix-auth"` and `session.branch = "fix-auth"`, *When* `SessionCard` renders, *Then* the `Branch:` row is not rendered.
- *Given* `session.title = "fix-auth"` and `session.branch = "Fix-Auth"`, *When* `SessionCard` renders, *Then* the `Branch:` row **is** rendered (case-sensitive — no fold).

**AC2 — Path / Working Dir / Cloned To rows, basename-vs-title trim-exact match.**
- *Given* `session.title = "fix-auth"` and `session.path = "/home/user/worktrees/fix-auth"`, *When* rendered, *Then* the `Path:` row is not rendered (`basenameOf(path) === "fix-auth"`).
- *Given* `session.title = "fix-auth"` and `session.path = "/home/user/worktrees/fix-auth-2"`, *When* rendered, *Then* the `Path:` row **is** rendered (near-miss — `"fix-auth-2" !== "fix-auth"`, no substring match).
- *Given* `session.title = "shared-fixes"` and `session.clonedRepoPath = "/tmp/clones/shared-fixes"`, *When* rendered, *Then* the `Cloned To:` row is not rendered (full path, including the `/tmp/clones/` prefix and its hover tooltip, is no longer shown on the card — deliberate tradeoff, see ADR-001; full value remains visible in `SessionDetailView`).

**AC3 — Program row is explicitly excluded from dedup (rewrite of requirements.md's original AC3).**
- *Given* `session.title = "claude"` and `session.program = "claude"`, *When* rendered, *Then* the `Program: claude` row still renders unchanged — Program is intentionally out of scope for v1 dedup (Pattern Decisions table).

**AC4 — No duplicates present → no visual regression.**
- *Given* `session.title = "implement-oauth"`, `branch = "feature/sso"`, `path = "/home/user/worktrees/implement-oauth-work"`, `program = "claude"`, `goal.goalText = "Ship SSO login"` (none match the title), *When* rendered, *Then* every info row appears exactly as it does today — none suppressed.

**AC5 — Dedup logic is a pure, directly-unit-testable helper.**
- *Given* the exported `isRedundantWithTitle` and `basenameOf` functions in `SessionCard.tsx`, *When* imported and called directly from `SessionCard.subtitle-dedup.test.tsx` without rendering the component, *Then* every table-driven case in that suite passes.

**AC6 — Whitespace-trim resilience, still not case-insensitive.**
- *Given* `session.title = "fix-auth"` and `session.branch = " fix-auth "` (surrounding whitespace), *When* rendered, *Then* the `Branch:` row is not rendered (trimmed match).
- *Given* `session.title = "fix-auth"` and `session.branch = "Fix-Auth"`, *When* rendered, *Then* the `Branch:` row **is** rendered (confirms trim-only, not case-insensitive — same example as AC1's negative case, restated here to make the trim-vs-fold boundary explicit).

**AC7 — Existing SessionCard tests continue to pass unmodified.**
- *Given* the existing suite (`SessionCard.approval-suppression.test.tsx`, `SessionCard.click.test.tsx`, `SessionCard.pending-program.test.tsx`), *When* run via `cd web-app && npx jest --no-coverage --testPathPatterns="SessionCard"` after this change, *Then* all pass with no test-file edits required (pitfalls.md confirmed no existing fixture has a title/branch/path/workingDir/goal collision).

**AC8 — No unintended accessibility information loss beyond the documented, bounded tradeoff.**
- *Given* the `Cloned To:` row is suppressed per AC2's third example, *When* a screen-reader user navigates the card, *Then* the only information no longer announced is the parent-directory prefix of the cloned path (`/tmp/clones/`) — the basename itself (`shared-fixes`) remains readable via the title text, and the full path remains available in `SessionDetailView`. This is the specific, deliberate, bounded loss documented in ADR-001 — not an accidental strip of the *only* source of some fact.

---

## Phase 1: Session Card Subtitle Deduplication

### Epic 1.1: Dedup Helper Functions
**Goal**: Add the two pure, colocated, exported predicate functions that every info-row call site will use, and unit-test them directly (AC5).

#### Story 1.1.1: Add `isRedundantWithTitle` and `basenameOf` to `SessionCard.tsx`
**As a** developer wiring dedup into info rows, **I want** two small pure functions colocated with the existing `hasPendingProgramChange` precedent, **so that** every row's suppression check is a single, testable, consistent call.
**Acceptance Criteria**:
- `isRedundantWithTitle` and `basenameOf` are both exported from `SessionCard.tsx`, placed immediately after `hasPendingProgramChange` (currently ending at line 30).
  - *Given* the functions are added, *When* `hasPendingProgramChange({...})` is imported from `../SessionCard` in existing tests, *Then* the existing import continues to work unchanged (no accidental reordering breaks the existing export).
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 1.1.1a: Add `isRedundantWithTitle` (~3 min)
- Insert immediately after `hasPendingProgramChange` (after line 30, before the `import {` block starting at line 31):
  ```ts
  // A secondary info-row value is redundant with the primary title when it is
  // the exact same text (surrounding whitespace aside) — repeating it below the
  // title adds visual noise with no new information. Deliberately NOT
  // case-insensitive (a user who capitalizes a branch/title differently likely
  // meant it) and NOT substring/basename-aware here — callers that need
  // basename comparison (Path/Working Dir/Cloned To) pre-normalize via
  // `basenameOf` before calling this.
  export function isRedundantWithTitle(value: string | undefined | null, title: string): boolean {
    if (!value) return false;
    return value.trim() === title.trim();
  }
  ```
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 1.1.1b: Add `basenameOf` (~2 min)
- Insert directly after `isRedundantWithTitle`:
  ```ts
  // Last "/"-separated segment of a trimmed path string. Mirrors the
  // `p.split("/").pop() || p` idiom already used in SessionsTable.tsx,
  // page.tsx, RecentFilesSection.tsx, and useAvailablePrograms.ts (not
  // `path.basename` — no Node `path` polyfill in this "use client" component).
  export function basenameOf(pathValue: string): string {
    const trimmed = pathValue.trim();
    return trimmed.split("/").pop() || trimmed;
  }
  ```
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 1.1.2: Unit-test the two predicates directly
**As a** reviewer, **I want** table-driven unit tests covering the exact/trim/near-miss cases called out in research, **so that** the dedup behavior is verified without rendering the full component.
**Acceptance Criteria**:
- New file `SessionCard.subtitle-dedup.test.tsx` covers: exact match, case-mismatch (non-match), whitespace-trim match, near-miss substring (`"fix-auth"` vs `"fix-auth-2"`), empty/undefined value, and `basenameOf` on a plain path, a trailing-slash path, and a path with no `/`.
  - *Given* `isRedundantWithTitle("fix-auth-2", "fix-auth")`, *When* called, *Then* it returns `false` (AC2's near-miss case).
  - *Given* `basenameOf("/home/user/worktrees/fix-auth/")` (trailing slash), *When* called, *Then* it returns the full trimmed path (documented inherited quirk from the existing repo idiom, not a new bug).
**Files**: `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx`

##### Task 1.1.2a: Create `SessionCard.subtitle-dedup.test.tsx` (~5 min)
- Mirror the header/doc-comment and no-render style of `SessionCard.pending-program.test.tsx`: import `isRedundantWithTitle`, `basenameOf` from `../SessionCard`, no component render, no mocks needed.
- Table-driven `describe("isRedundantWithTitle")` block with cases: exact match → `true`; case mismatch → `false`; leading/trailing whitespace on either side → `true`; near-miss (`"fix-auth"` vs `"fix-auth-2"`) → `false`; empty string value → `false`; `undefined`/`null` value → `false`.
- Table-driven `describe("basenameOf")` block with cases: `"/home/user/worktrees/fix-auth"` → `"fix-auth"`; `"/home/user/worktrees/fix-auth/"` (trailing slash) → `"/home/user/worktrees/fix-auth"` (documented quirk); `"fix-auth"` (no slash) → `"fix-auth"`; `"  /tmp/clones/shared-fixes  "` (whitespace) → `"shared-fixes"`.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="subtitle-dedup"` to confirm green.
- Files: `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx`

---

### Epic 1.2: Apply Dedup to Info Rows
**Goal**: Wire the two predicates into the 5 in-scope info rows (Branch, Path, Working Dir, Cloned To, Goal), each via row-level conditional suppression matching the existing idiom.

#### Story 1.2.1: Branch row (AC1, AC6)
**As a** user, **I want** the Branch row hidden when it's identical to the title, **so that** the card doesn't repeat itself for the default "New branch (isolated)" creation mode (the highest-value target per features.md — branch defaults to the title verbatim).
**Acceptance Criteria**:
- Branch row suppressed when `isRedundantWithTitle(session.branch, session.title)` is `true`; otherwise unchanged.
  - *Given* `title = "fix-auth"`, `branch = "fix-auth"`, *When* rendered, *Then* the `Branch:` row does not appear in the DOM.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 1.2.1a: Guard the Branch row (~2 min)
- At `web-app/src/components/sessions/SessionCard.tsx:705`, change:
  ```tsx
  {session.branch && (
  ```
  to:
  ```tsx
  {session.branch && !isRedundantWithTitle(session.branch, session.title) && (
  ```
  (closing `)}` at line 710 unchanged).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 1.2.2: Path row (AC2)
**As a** user, **I want** the Path row hidden when its basename matches the title, **so that** an obvious repeat (e.g. a worktree directory named after the session) doesn't clutter the card.
**Acceptance Criteria**:
- Path row (currently unconditional — no existing guard) becomes conditional on `!isRedundantWithTitle(basenameOf(session.path), session.title)`.
  - *Given* `title = "fix-auth"`, `path = "/home/user/worktrees/fix-auth"`, *When* rendered, *Then* the `Path:` row does not appear.
  - *Given* `title = "fix-auth"`, `path = "/home/user/worktrees/fix-auth-2"`, *When* rendered, *Then* the `Path:` row still appears with its `title=` tooltip intact.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 1.2.2a: Guard the Path row (~3 min)
- At `web-app/src/components/sessions/SessionCard.tsx:711-716`, change:
  ```tsx
  <div className={infoRow}>
    <span className={label}>Path:</span>
    <span className={value} title={session.path}>
      {session.path}
    </span>
  </div>
  ```
  to:
  ```tsx
  {!isRedundantWithTitle(basenameOf(session.path), session.title) && (
    <div className={infoRow}>
      <span className={label}>Path:</span>
      <span className={value} title={session.path}>
        {session.path}
      </span>
    </div>
  )}
  ```
  (This is the first time the Path row becomes conditional — previously unconditional. Confirmed no existing test fixture asserts the Path row is always present regardless of content — pitfalls.md.)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 1.2.3: Working Dir row (AC2)
**As a** user, **I want** the Working Dir row hidden when its basename matches the title, **so that** the same redundancy is avoided for sessions where working dir diverges from path.
**Acceptance Criteria**:
- Working Dir row suppressed when `isRedundantWithTitle(basenameOf(session.workingDir), session.title)` is `true`.
  - *Given* `title = "my-project"`, `workingDir = "/repos/my-project"`, *When* rendered, *Then* the `Working Dir:` row does not appear.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 1.2.3a: Guard the Working Dir row (~2 min)
- At `web-app/src/components/sessions/SessionCard.tsx:717`, change:
  ```tsx
  {session.workingDir && (
  ```
  to:
  ```tsx
  {session.workingDir && !isRedundantWithTitle(basenameOf(session.workingDir), session.title) && (
  ```
  (closing `)}` at line 722 unchanged).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 1.2.4: Cloned To row (AC2, AC8)
**As a** user, **I want** the Cloned To row hidden when its basename matches the title, **so that** the same redundancy rule applies consistently to the third path-shaped row.
**Acceptance Criteria**:
- Cloned To row suppressed when `isRedundantWithTitle(basenameOf(session.clonedRepoPath), session.title)` is `true`.
  - *Given* `title = "shared-fixes"`, `clonedRepoPath = "/tmp/clones/shared-fixes"`, *When* rendered, *Then* the `Cloned To:` row does not appear (tooltip loss accepted per ADR-001 — full path remains in `SessionDetailView`).
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 1.2.4a: Guard the Cloned To row (~3 min)
- At `web-app/src/components/sessions/SessionCard.tsx:757`, change:
  ```tsx
  {session.clonedRepoPath && (
  ```
  to:
  ```tsx
  {session.clonedRepoPath && !isRedundantWithTitle(basenameOf(session.clonedRepoPath), session.title) && (
  ```
  (closing `)}` at line 764 unchanged).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 1.2.5: Goal row (AC2's spirit extended to free text — compare raw text, truncate only for display)
**As a** user, **I want** the Goal row hidden only when the raw, untruncated goal text matches the title, **so that** a coincidental short-goal/title match is caught without ever comparing a truncated (and thus falsely-mismatching, or falsely-matching-a-truncation-artifact) string.
**Acceptance Criteria**:
- Goal row suppressed when `isRedundantWithTitle(session.goal?.goalText, session.title)` is `true`, checked **before** `truncateGoal` is called for display — never compare the post-truncation string.
  - *Given* `title = "Fix login bug"`, `goal.goalText = "Fix login bug "` (trailing space), *When* rendered, *Then* the `Goal` row does not appear.
  - *Given* `title = "fix-auth"`, `goal.goalText = "fix-auth and also update the docs and changelog entries so nothing regresses"` (>61 chars, would truncate to something starting with `"fix-auth and..."`), *When* rendered, *Then* the `Goal` row **still appears** — the comparison is against the raw text (`"fix-auth and also..." !== "fix-auth"`), not the truncated display string, so no false suppression from truncation coincidentally matching.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 1.2.5a: Guard the Goal row, comparing raw text before truncation (~4 min)
- At `web-app/src/components/sessions/SessionCard.tsx:765`, change:
  ```tsx
  {session.goal?.goalText && (
  ```
  to:
  ```tsx
  {session.goal?.goalText && !isRedundantWithTitle(session.goal.goalText, session.title) && (
  ```
  Leave the body (lines 766-776, including the `truncateGoal(session.goal.goalText, 61)` call at line 769) untouched — the comparison happens in the guard, against the raw `goalText`, strictly before `truncateGoal` ever runs. Do not introduce a second, truncated comparison anywhere in this block.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

---

### Epic 1.3: Regression Verification
**Goal**: Confirm the change is behaviorally inert for the common (no-duplicate) case and doesn't break any existing test.

#### Story 1.3.1: Full `SessionCard` test suite + lint pass (AC4, AC7)
**As a** reviewer, **I want** confirmation that existing tests and lint are green after the 5 row edits, **so that** the change can ship with evidence, not just a claim.
**Acceptance Criteria**:
- `cd web-app && npx jest --no-coverage --testPathPatterns="SessionCard"` passes with 0 failures across all 4 test files (3 existing + 1 new).
  - *Given* the 5 completed Epic 1.2 tasks, *When* the full `SessionCard`-scoped Jest run executes, *Then* `SessionCard.approval-suppression.test.tsx`, `SessionCard.click.test.tsx`, `SessionCard.pending-program.test.tsx`, and `SessionCard.subtitle-dedup.test.tsx` all report pass, with no edits needed to the 3 pre-existing files.
- `make lint` (or the frontend-scoped lint step it invokes) reports no new violations introduced by this diff.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/__tests__/*`

##### Task 1.3.1a: Run tests and lint, confirm green (~5 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SessionCard"`; paste/confirm 0 failures.
- Run `make lint` (or `cd web-app && npx eslint` if scoped faster); confirm no new violations on the touched file.
- If any existing test fails, root-cause against this diff specifically (per `.claude/rules/fix-flaky-tests-dont-defer.md` — do not wave off as unrelated without checking) before marking this task done.
- Files: N/A (verification only, no edits)
