# Research: Stack — session-card-subtitle-dedup

## Summary

This is a pure frontend, pure-function change. No new dependencies. Reuse the
existing React 19 / TypeScript 5.9 / Jest 30 stack already in `web-app/`, and
add one new small utility function alongside the existing `truncateGoal`
helper rather than duplicating string-normalization logic — because **no
such normalization utility currently exists** in this codebase (verified by
search, see below).

## Confirmed versions (web-app/package.json)

| Package | Version |
|---|---|
| `react` | `^19.0.0` |
| `next` | `15.3.2` |
| `typescript` | `^5.9.3` |
| `jest` | `^30.2.0` |
| `jest-environment-jsdom` | `^30.2.0` |
| `ts-jest` | `^29.4.11` |
| `@testing-library/react` | `^16.3.0` |
| `@testing-library/jest-dom` | `^6.9.1` |
| `@testing-library/user-event` | `^14.5.2` |

No version bumps are needed for this feature; all tooling required (React
function components, Jest unit tests, RTL for interaction/render tests) is
already present and current.

## No dependencies needed

The feature is a string-comparison predicate plus conditional JSX rendering
— both fully covered by the existing stack. No new npm packages, no new
proto fields (per requirements.md "out of scope"), no backend changes.

## Existing string utilities — what's there, what's missing

`web-app/src/lib/utils/string.ts` currently contains **only** one export:

```ts
// web-app/src/lib/utils/string.ts (full file, 9 lines)
export function truncateGoal(text: string, max: number): string {
  if (!text) return text;
  if (text.length <= max) return text;
  return text.slice(0, max - 1) + "…";
}
```

`truncateGoal` is already imported into `SessionCard.tsx` at line 92
(`import { truncateGoal } from "@/lib/utils/string";`) and used at line 769
for the Goal row.

**There is no existing normalization/comparison helper** (`normalize`,
`.toLowerCase().trim()` pattern, `localeCompare`, or similar) anywhere under
`web-app/src/lib/utils/` — confirmed via grep across the whole utils
directory (excluding `.test.ts` files) with zero matches. `truncateMiddle.ts`
and `parseDiff.ts` are unrelated (path-middle truncation and diff parsing,
respectively). This means the plan phase needs to **add** a new helper
(e.g. `isTitleDuplicate(value: string, title: string): boolean` or
`normalizeForCompare(s: string): string`) to `string.ts` — there is nothing
to reuse for the comparison itself, only the file/module location and the
adjacent-export convention to follow.

Recommended placement: add the new function to the same
`web-app/src/lib/utils/string.ts` file, next to `truncateGoal`, so
`SessionCard.tsx` can extend its existing `import { truncateGoal } from
"@/lib/utils/string"` to a multi-named import (e.g. `import { truncateGoal,
isDuplicateOfTitle } from "@/lib/utils/string";`) rather than adding a new
module.

## No `paneMeta()`-equivalent exists in this repo

Confirmed via repo-wide grep: `paneMeta` appears only in
`project_plans/session-card-subtitle-dedup/requirements.md` (the migrated
issue text itself) — it is not vendored anywhere in this codebase. This
matches the requirements.md note: the referenced pattern from `herdr-web` is
prior art only, not a function to port. The integration point here is
per-row suppression inline in `SessionCard.tsx`'s existing JSX
(`web-app/src/components/sessions/SessionCard.tsx`, lines 700–778), not a
new consolidated subtitle-building function.

## Relevant SessionCard.tsx structure (for the plan phase)

- Title rendered at line 454: `{session.title}` (inside a `<span
  className={title} ...>` starting line 449).
- Secondary info rows live in a `<div className={info}>` block, lines
  700–778, each wrapped in `<div className={infoRow}>`:
  - `Program:` — `session.program` (line 702–704, always rendered)
  - `Branch:` — `session.branch` (705–710, conditional on `session.branch`)
  - `Path:` — `session.path` (711–716, always rendered)
  - `Working Dir:` — `session.workingDir` (717–722, conditional)
  - `Repository:` — `session.githubOwner`/`session.githubRepo` as a link (723–739, conditional)
  - `Pull Request:` — `#session.githubPrNumber` as a link (740–756, conditional)
  - `Cloned To:` — `session.clonedRepoPath` (757–764, conditional)
  - `Goal` — `truncateGoal(session.goal.goalText, 61)` (765–777, conditional)

Each row's existing conditional (`session.branch && (...)`, etc.) is the
natural place to AND-in the new dedup check, e.g. `session.branch &&
!isDuplicateOfTitle(session.branch, session.title) && (...)`. Repository and
Pull Request rows render composite/link values rather than plain strings, so
their dedup comparison target needs a decision in the plan phase (e.g.
compare against `${owner}/${repo}` for Repository, likely skip or compare
PR number as string for Pull Request — flagging for the plan/UX pass, not
resolved here).

## Test conventions to follow

- Existing SessionCard tests live in
  `web-app/src/components/sessions/__tests__/`, one file per behavior area:
  `SessionCard.approval-suppression.test.tsx`,
  `SessionCard.click.test.tsx`, `SessionCard.pending-program.test.tsx`. A
  new `SessionCard.subtitle-dedup.test.tsx` (or similar) in the same
  directory follows the established pattern.
- `string.ts` already has a sibling `string.test.ts` — the new helper's unit
  tests belong there.
- Run frontend tests via `cd web-app && npx jest --no-coverage
  --testPathPatterns="<pattern>"` per root CLAUDE.md.

## Recommendation

1. Add a small, pure comparison helper to `web-app/src/lib/utils/string.ts`
   (case-insensitive, whitespace-normalized match) — no new file, no new
   dependency.
2. Add unit tests to the existing `web-app/src/lib/utils/string.test.ts`.
3. Wire the helper into each conditional in `SessionCard.tsx` (lines
   700–778) per-row, matching the existing conditional-rendering idiom
   already used for `session.branch`, `session.workingDir`, etc.
4. Add a new `SessionCard.subtitle-dedup.test.tsx` under
   `web-app/src/components/sessions/__tests__/` following the existing
   per-behavior test file convention.
