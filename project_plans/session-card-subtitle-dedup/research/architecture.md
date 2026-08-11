# Architecture Research: session-card-subtitle-dedup

## Prior SDD research overlap

Checked `project_plans/memory-pressure-ux/research/architecture.md` and
`project_plans/bulk-select-ux/research/architecture.md` (backlog-item-detail-ux has no
`research/architecture.md`). Neither touches title/subtitle deduplication:

- memory-pressure-ux's architecture.md only discusses the RAM badge (`memoryBadge`,
  `SessionCard.css.ts` amber-border pattern) — unrelated field.
- bulk-select-ux's architecture.md is about `SessionRowProps` missing bulk-select props
  (`web-app/src/components/sessions/SessionRow.tsx:33-56`) — unrelated to this file's info
  rows.

**None found relevant.** This is a clean-slate research task for this file.

## 1. Integration shape: (a) helper function vs (b) inline guards vs (c) subtitle builder

**Recommendation: (a) — one helper, following the `hasPendingProgramChange` precedent
almost exactly, but shaped as a per-field predicate rather than a single boolean.**

The three options, weighed:

- **(b) inline per-row checks** (`session.branch !== session.title && (...)`) is what the
  codebase already does for *presence* guards (`session.branch &&` at
  `web-app/src/components/sessions/SessionCard.tsx:705`,
  `session.workingDir &&` at :717, `session.clonedRepoPath &&` at :757). Adding a second
  inline comparison (`session.branch && session.branch !== session.title &&`) to each of
  these JSX conditions is the *smallest* diff, but the existing file already rejected this
  shape once: `hasPendingProgramChange` (:22-30) — a *harder* condition than a straight
  string comparison — was pulled out into a named, exported, doc-commented, independently
  tested predicate specifically so the multi-clause boolean isn't buried in JSX and so it's
  testable without rendering the full card. A title-equality check is simpler than
  `hasPendingProgramChange`'s logic, but the same testability argument applies, and
  mixing "some rows use inline guards, some call a named predicate" in one component is
  inconsistent for no real savings — the named-predicate version is barely longer.
- **(c) consolidated subtitle-builder** (herdr-web's `paneMeta()` — replace N rows with one
  joined string) is out of scope per requirements.md ("Out of scope: ... layout redesign
  beyond row suppression"). It would also change the row *count/shape*, not just suppress
  duplicates, and loses the per-field `label:` structure (`Program:`, `Branch:`, etc.) that
  the rest of the info block relies on for scanability. Rejected.
- **(a) named helper** is the closest fit to the file's own precedent. Given there are up to
  8 candidate fields (see §3), a single function that takes `session` and returns which of a
  small field-name set are "redundant with the title" is the right shape — mirrors
  `hasPendingProgramChange`'s "pure predicate over a `Pick<Session, ...>` slice" pattern, but
  needs to answer "which fields," not one boolean. Two viable signatures:
  - `isRedundantWithTitle(value: string | undefined, title: string): boolean` — one tiny
    generic predicate, called inline at each candidate row:
    `{session.branch && !isRedundantWithTitle(session.branch, session.title) && (...)}`
  - A single `getSuppressedFields(session): Set<'branch' | 'path' | ...>` — one call up top,
    then `{session.branch && !suppressed.has('branch') && (...)}` per row.

  The single-value predicate (`isRedundantWithTitle`) is simpler, matches
  `hasPendingProgramChange`'s "narrow pure function, called per-callsite" shape more closely,
  and avoids inventing a field-name string union that has to be kept in sync with the JSX.
  **Recommend `isRedundantWithTitle(value, title)` as the primitive**, called once per
  candidate row exactly like the existing `session.branch &&` guards are extended with one
  more clause. This keeps the diff mechanically identical to the existing guard pattern
  (option b's minimalism) while getting the standalone-testable, doc-commented, named-export
  shape the file has already established (option a's precedent fit).

## 2. Where the helper should live

**Recommendation: colocate in `SessionCard.tsx`, exported above the component — not a new
file in `web-app/src/lib/sessions/`.**

Precedent check:

- `hasPendingProgramChange` (SessionCard.tsx:22) — colocated, SessionCard-specific derived
  logic that touches only `Session` fields and is only consumed by `SessionCard.tsx`.
- `formatPauseReason` (`web-app/src/lib/sessions/formatPauseReason.ts`) — a standalone file.
  But note its shape: a general string-formatting function (reason code → human label) with
  **no dependency on the `Session` type** or on `SessionCard`'s specific rendering
  logic — it's imported by `SessionCard.tsx:15` but is a generic "map this enum-like string
  to display text" utility, the kind of thing that could plausibly be reused elsewhere
  (tooltips, notifications, etc.).
- `truncateGoal` (`web-app/src/lib/utils/string.ts`, imported at SessionCard.tsx:92) — same
  pattern: a generic string utility (truncate-with-ellipsis), not session-shaped, reusable
  outside this component.

The dividing line in this codebase is **not** "is it exported and tested" (both patterns
are) but **"is the logic generic/reusable vs. specific to this component's session-field
semantics."** A title-vs-field-value string comparison is trivial and generic-looking, but
what makes it worth a helper at all is the SessionCard-specific *set of fields* it needs to
be applied to and the *doc comment* explaining why each is a candidate (see §3) — that
context belongs next to the JSX it drives, exactly like `hasPendingProgramChange`. A new
`web-app/src/lib/sessions/sessionDisplayHelpers.ts` file (the issue's alternate proposal) is
justified when there's a second consumer or enough logic to warrant its own file; today
there's one consumer and ~3-5 lines of logic, so a new file would be a speculative
extraction with no second call site — exactly the "unjustified generic/premature
abstraction" pattern this repo's `.claude/rules/interface-pollution-checklist.md` flags for
Go, and the same reasoning transfers: don't create the file until something else needs it.

**Verdict: add `isRedundantWithTitle` directly in `SessionCard.tsx`, colocated above the
component next to `hasPendingProgramChange` (same doc-comment style), not a new
`lib/sessions/` file.** If a second component later needs the same check, promote it then.

## 3. Which of the 8 rows are realistic dedup targets

All 8 rows live in `web-app/src/components/sessions/SessionCard.tsx:700-778`:

| Row | Line | Value shape | Realistic dedup target? |
|---|---|---|---|
| Program | :702-703 (always rendered, no guard) | short command/binary name (e.g. `claude`, `aider`) | **Unlikely but cheap to guard** — a user could theoretically title a session after its program, but low-value; low cost to include for consistency. Note: this row has **no presence guard today** (`session.program` always renders), so adding suppression here changes an unconditional row to a conditional one — slightly bigger behavioral change than the others. |
| Branch | :705-710 | git branch name, often auto-generated (`feature/foo-bar`) or freeform | **Yes — primary target.** Users very plausibly title a session the same as its branch (common for `new_worktree` sessions where title defaults to/mirrors branch intent). |
| Path | :711-716 (always rendered, no guard) | absolute filesystem path | **Yes — primary target**, though exact string equality is the only case (a title equal to a full path is less common than branch/goal, but happens, e.g. one-off sessions per `.claude/rules/session-creation-registry.md`). Also has **no presence guard today**. |
| Working Dir | :717-722 | absolute path, subdir of Path | **Yes**, same reasoning as Path — less likely to literally equal title than Branch/Goal but cheap and consistent to include. |
| Repository | :723-739 | `owner/repo` pair, rendered as a link with `{owner}/{repo}` text | **No — exclude.** Requirements.md itself calls out PR/repo pairs as "never plausibly equal a title." A title matching `owner/repo` exactly is implausible, and this row is a hyperlink (not a "value matches title" plain-text comparison) — suppressing a *link* because it textually matches the title would also remove the only way to click through to GitHub, which is a UX regression, not a benefit. |
| Pull Request | :740-756 | `#123` (PR number, rendered as a link) | **No — exclude**, same reasoning as Repository: numeric/short, never plausibly equals a free-text title, and is a functional link. |
| Cloned To | :757-764 | absolute filesystem path | **Yes**, same class as Path/Working Dir. |
| Goal | :765-777 | free-text goal string, **already passed through `truncateGoal(text, 61)`** (`web-app/src/lib/utils/string.ts`, imported at :92) plus an appended task-fraction suffix (`· N/M done`) | **Yes — primary target, but needs care.** Goal is the single most likely field to literally equal or closely restate the title (both are free-text descriptions of the same work). Comparison must happen against the **raw** `session.goal.goalText`, not the truncated display string, otherwise a truncated goal that happens to still differ from the title would falsely compare unequal to a truncated version of an equal string in edge cases — always compare full strings, truncate only for display *after* the row is confirmed non-redundant. |

**Scoped set for the architecture: Branch, Path, Working Dir, Cloned To, Goal** (5 of 8) are
the real dedup targets. Program is a low-value "include for consistency" candidate — worth a
one-line mention in the plan phase but not a hard requirement. Repository and Pull Request
are explicitly out of scope (functional links, not plausible title matches).

All five real targets are simple string fields on `Session` (`branch`, `path`, `workingDir`,
`clonedRepoPath`, `goal.goalText`) — a single `isRedundantWithTitle(value, title)` predicate
handles all of them uniformly; no per-field special-casing is needed beyond Goal's
raw-vs-truncated-string care noted above.

**Comparison semantics** (for the plan phase to nail down, flagged here as an open question):
exact string equality after trim, or case-insensitive, or substring-contains? Requirements.md
only says "matches the title" — recommend starting with `value.trim() === title.trim()`
(exact match after whitespace trim) as the least surprising default, consistent with
`hasPendingProgramChange`'s own precedent of a single unambiguous condition
(`startsWith`, not fuzzy). A looser (substring/case-insensitive) match risks false-positive
suppression of genuinely distinct information; that's a decision for `sdd:3-plan`, not
architecture.

## 4. Data flow / state complexity

**100% synchronous, prop-derived — no state management needed.**

`SessionCard` receives a fully-hydrated `session: Session` prop (protobuf-generated type,
`@/gen/session/v1/types_pb`, imported at SessionCard.tsx:4). All 8 info-row fields
(`program`, `branch`, `path`, `workingDir`, `githubOwner`/`githubRepo`,
`githubPrNumber`/`githubPrUrl`, `clonedRepoPath`, `goal.goalText`) are plain fields already
present on that object by render time — same as `session.title`, which is read directly at
:454 with no loading/async state. `hasPendingProgramChange` (:22-30) is the existing proof
of this: it's a plain synchronous function called once per render (:176) with no `useEffect`,
`useState`, or async fetch involved, and reads fields (`status`, `program`, `launchCommand`)
that are peers of the fields this feature needs. `isRedundantWithTitle` follows the identical
pattern — a pure function of already-available props, called inline during render. No new
hooks, context, or async data flow required.

## Summary recommendation for `sdd:3-plan`

1. Add `isRedundantWithTitle(value: string | undefined, title: string): boolean` — a small
   pure exported function, colocated in `SessionCard.tsx` directly above or below
   `hasPendingProgramChange` (:22-30), with a doc comment explaining the exact-match-after-trim
   semantics and citing which rows use it.
2. Apply it as an added clause to the existing presence guards for **Branch** (:705),
   **Working Dir** (:717), and **Cloned To** (:757) (all three already have `session.X &&`
   guards — just AND in `!isRedundantWithTitle(session.X, session.title)`).
3. **Path** (:711) and **Program** (:702) currently render unconditionally — decide in the
   plan phase whether to add a new conditional guard (behavior change: row can now disappear
   entirely) or leave Program unguarded (lower value target, per §3) and only gate Path.
4. **Goal** (:765) needs the comparison against `session.goal.goalText` (raw), with
   `truncateGoal` applied only after the redundancy check passes.
5. Add a colocated test file `web-app/src/components/sessions/__tests__/SessionCard.subtitle-dedup.test.tsx`
   mirroring `SessionCard.pending-program.test.tsx`'s shape (imports the predicate from
   `../SessionCard`, tests it standalone without rendering the component).
6. Do **not** create a new `web-app/src/lib/sessions/` file for this — single consumer,
   small logic, colocation matches the stronger precedent (`hasPendingProgramChange`) for
   session-field-specific derived logic.
