# UX Research: session-card-subtitle-dedup

Agent 5 (UX), sdd:2-research phase.

## Current structure (grounding facts)

- `SessionCard.tsx:700-778` — one `infoRow` `<div>` per field (`Program`,
  `Branch`, `Path`, `Working Dir`, `Repository`, `Pull Request`, `Cloned To`,
  `Goal`), each conditionally rendered only if the underlying value is
  present/truthy. This is already a "hide row if empty" pattern — the issue
  extends the same idea to "hide row if redundant with title."
- `SessionCard.css.ts:293-316` — `info` is a `flexDirection: column` stack
  with `gap: 6px`; each `infoRow` is `display: flex` with a fixed
  `label` (`minWidth: 100px`) and a `value` that is single-line, ellipsis-
  truncated (`overflow: hidden; textOverflow: ellipsis; whiteSpace: nowrap`).
  This is a classic two-column "definition list" layout, not free-flowing
  text — every row commits fixed vertical space (~1 line + 6px gap)
  regardless of value length, and long values are already being truncated,
  not wrapped.
- Title is rendered once, as bold/prominent text in `titleRow` (`:449-455`)
  and duplicated into the card's `aria-label` (`:411`,
  `` `${session.title}, press F2 to rename` ``).

## 1. Comparable UX patterns

Two distinct families exist in mainstream card/list UIs, and they solve
different problems — the distinction matters for which one fits here:

- **Hide-entirely-when-redundant** (Gmail thread list, GitHub PR/issue list
  rows, Linear issue rows, macOS Finder list view): a piece of metadata that
  would just restate the primary label is omitted outright, not grayed out.
  Gmail doesn't show the recipient's own name in a "to me" thread; GitHub's
  PR list doesn't repeat the base branch when it's `main` (the default);
  Linear collapses a status badge in swimlane views that are already grouped
  by that status. The row/field disappears — no ghost placeholder, no dimmed
  label — because these are *dense, high-cardinality lists* where every
  redundant character read by the user is a scanning cost across dozens of
  rows.
- **Merge-into-one-line** (herdr-web's `paneMeta()`, VS Code's editor tab
  subtitle, JetBrains project-view secondary text, Slack channel-list preview
  line): several short facts are joined with a separator (` · `, `—`, `/`)
  into a single subordinate text line under the primary label, and any part
  that duplicates the primary label is dropped from the join *before*
  joining, so the separator count self-adjusts. This pattern is chosen when
  the fields are individually short, are conceptually "the same kind of
  thing" (all facets of "where/what is this"), and the UI has budgeted only
  one line of secondary space per item — exactly the shape of a compact list
  (dozens of items, one line each), not a card with room to spare.

**Assessment for SessionCard specifically**: `paneMeta()`'s one-line-join
model is the better fit for the *problem* (reducing rows-that-restate-the-
title) but not a drop-in fit for the *current layout*, because SessionCard's
`info` block is a labeled key-value grid (`label` column pinned to 100px),
not a single flowing text line. Naively joining `Program`, `Branch`, `Path`
into ` · `-separated text would:
  - discard the `label` prefixes (`Branch:`, `Path:`) that currently let a
    user scan the *column* of labels down the whole card list to find "which
    field is this" without reading values — labels are doing real work here,
    not just decoration;
  - collapse fields with different truncation/interaction needs (the
    `Repository`/`Pull Request` rows are links with their own `aria-label`
    and `onClick` stop-propagation; `Path`/`Cloned To` carry `title=` full-
    path tooltips) into one string, which is awkward once any joined part is
    an `<a>` needing independent click handling.

Given the requirements doc already scoped this as **per-row suppression, not
a layout redesign** (`requirements.md` "Out of scope": *"Redesigning the
card's visual layout beyond removing/collapsing duplicate rows"*), the
recommendation is: **adopt `paneMeta()`'s duplicate-skipping *logic* (drop a
value if it string-matches the title) but keep SessionCard's existing
one-row-per-field rendering**, i.e. conditionally suppress the whole
`infoRow` `<div>` when its value matches the title, the same way rows are
already suppressed when the value is falsy (`session.branch &&`, `:705`).
This is the smallest change consistent with the "hide-entirely" family above
— it needs no new joined-string helper, reuses the existing conditional-
render idiom already in the file, and doesn't touch the link/tooltip rows'
special handling. A `paneMeta()`-style consolidated line is a larger,
separately-justified redesign (better filed as its own future issue) — not
what this task is scoped to build.

## 2. User mental model

SessionCard is a developer tool for managing concurrent AI coding-agent
sessions, and the audience reads these rows as **labeled fields on a status
panel**, closer to a process-manager row (`ps`, `htop`, Docker Desktop
container list) than a marketing card. That mental model has a specific
consequence: users expect *labels that are relevant to be present*, but they
do not expect *every possible label to always appear* — the file already
trains this expectation, since `Branch`, `Working Dir`, `Repository`, `Pull
Request`, `Cloned To`, and `Goal` are ALL already conditionally rendered
today (only shown `session.branch &&`, `session.githubOwner && ... &&`,
etc.). A user who manages 10-50 of these cards has already learned "a
one-off session has no Branch row, a non-GitHub session has no Repository
row" — absence-because-not-applicable is the established norm on this exact
card, not an anomaly.

Extending "absence because not applicable" to also cover "absence because
redundant with the title" is a small delta on an already-familiar pattern,
**not** a new mental model the user has to learn — it reads as "the tool is
already doing this selectively, and now it's doing one more kind of
selection." The risk of "looks broken" is real only for a row a user has
learned to expect on *every* session regardless of config — `Program` and
`Path` are the two candidates, since every session (in-scope types) has a
program and a path today, so their rows are currently unconditional
(`:700-704`, `:711-716`, no `&&` guard). Suppressing `Program`/`Path`
specifically (AC1/AC3 in requirements.md) is the one place where a user
could plausibly notice "why is there no Path row on this card but there is
on that one" — mitigated by the fact that the *reason* is visible: the path
basename is sitting right there in the bold title they're looking at, so the
absence is self-explanatory in context, not a mystery. This is a case for a
clear, consistent rule (always suppress on exact/normalized match, never a
partial heuristic) rather than a fuzzy "looks similar enough" suppression
that would make the presence/absence pattern harder to predict.

## 3. Accessibility (WCAG / ARIA)

Current exposure surface for the values in question:

- Card-level `aria-label` (`:411`): already includes `session.title` (plus
  "press F2 to rename" or selection state) — this is what a screen-reader
  user hears first when focus lands on the card (`role="group"`,
  `tabIndex={0}`).
- `Path` row (`:711-716`) has a `title={session.path}` tooltip on the
  `value` span — `title` attributes are *inconsistently* exposed to
  assistive tech (many screen readers do not announce `title` on a `<span>`
  that isn't a link/form control; it's primarily a mouse-hover affordance),
  so the *reliable* AT exposure of the path today is the visible row text
  itself, read in DOM order as part of the card's accessible content, not
  the `title=` attribute.
- `Cloned To` row (`:757-763`) has the same `title=` pattern.
- Since `role="group"` + `aria-label` does not use `aria-labelledby` /
  `aria-describedby` to reference the info rows, screen readers announce the
  card's `aria-label` on focus, and *separately* walk the row text as normal
  document content if the user navigates into the card (arrow-key/virtual-
  cursor browsing) — the info rows are not just decorative visual sugar,
  they are real accessible-tree content today.

**Is removing a visually-duplicate row acceptable?** Yes, *if and only if*
the removed row's value is a pure substring/exact-match duplicate of text
already present elsewhere in the card's accessible content (the title, which
is both in the card's `aria-label` and in the visible `title` span read in
DOM order). Removing the `Branch:` row when `branch === title` does not
remove any fact from the accessible tree — the branch name is still present,
verbatim, as the title text. This is exactly what AC8 in requirements.md
already specifies (*"don't strip data that's the only source of some other
fact just because it string-matches the title incidentally"*) — the
research finding here is that the AC's own qualifier is doing real work: the
implementation must compare against the **exact rendered value**, not a
normalized/lossy version, or it risks suppressing a row whose full value
(e.g. `Cloned To` = `/home/user/repos/my-app` vs. title = `my-app`) is only
*partially* redundant — AC2 already scopes this to basename comparison, so
the row is suppressed only when the *entire displayed value* is redundant,
not when it merely contains the title as a substring. No new `aria-only`
shadow text is needed as long as suppression logic mirrors the requirements'
"identical after normalization" rule and never fires on partial/substring
matches — implement the comparison in the shared helper (AC5) so this
invariant lives in one tested place rather than being re-derived per call
site.

One implementation-level accessibility note: suppress by **not rendering
the JSX node** (conditional render, matching the existing `session.branch &&`
idiom), not by CSS `display: none` on an otherwise-rendered node. Both
remove the row from the accessible tree in modern browsers/AT, but
conditional-render is the pattern already used throughout this exact block
and keeps the DOM lean — no reason to introduce a second suppression
mechanism.

## 4. Error/edge-case UX: all secondary fields dedupe away

A session named identically to its branch, path basename, and program
(all-fields-redundant case) would, under naive full suppression, leave the
`info` block rendering only the truly-independent rows (`Repository`, `Pull
Request`, `Cloned To`, `Goal` — which are unlikely to string-match a title)
plus whatever rows had no data at all. In the worst case (a bare local
session: no GitHub repo, no PR, no goal set, and title happens to equal
program+branch+path-basename), **all** rows in `info` could suppress,
leaving `body` with an empty `info` `<div>` — visually a dead gap between
the header and the diff-stats/status region below it.

This is a real "looks broken" risk distinct from the single-row case in
Q2, because an *entirely* empty info block has no self-explanatory context
the way a single missing row does (there, the missing value is visibly
sitting in the title; here, the reader sees blank space and no cue that
suppression is even happening). Recommendation: this is an edge case worth
guarding explicitly, not left to fall out of the per-row logic — options in
increasing complexity:
  - **Cheapest, matches the file's existing conditional style**: leave the
    empty `info` `<div>` (it's `display:flex; flex-direction:column` with no
    fixed min-height, so it collapses to zero height and doesn't leave a
    visible gap — verify this against the actual CSS box model before
    relying on it, since `body`'s own padding could still create visible
    whitespace even with a zero-height child).
  - **If the collapsed-`info` case does still leave a visible gap**: fall
    back to always showing exactly one row (prefer `Program`, since it is
    the shortest, most stable field, currently unconditional) even if it
    matches the title, rather than showing zero rows — i.e. dedup logic
    should guarantee "at least one row visible when at least one candidate
    field has a value," not blindly suppress every match. This keeps the
    card's vertical rhythm consistent across the list (every card has *some*
    body content) and avoids a genuinely blank-looking card.
  - This "always show at least one row" guard should be validated visually
    (not just unit-tested) before shipping, since it's a layout claim, not a
    string-matching claim — flag for `sdd:3-plan`/`sdd:4-validate` to decide
    which of the two options above, and for implementation to add a test
    fixture exercising the all-fields-redundant case specifically (not just
    the single-field cases AC1-3 already specify).

## 5. Job-to-be-done

The `info` block's actual job, for a developer scanning 10-50 concurrent
agent-session cards, is **differentiation at a glance**: "which of these
many similarly-shaped cards is the one I want, and what state is it in
(branch, whether it has a PR yet, is it a worktree or a clone)?" — not
literal completeness of every field the data model happens to populate.
Evidence for this from the code itself: labels are fixed-width and
left-aligned (`label { minWidth: 100px }`) specifically so a user scanning
*down* the card list can visually align on a column (e.g. skim every
`Branch:` value down the page) — that's a signal-density/scanability design,
not a "show me everything" design.

Given that, **duplication against the title is a symptom, not the deepest
pain point** — the deeper need is: when a session list has many cards
side-by-side/stacked, every unit of *unique* information should be visible
without extra clicks, and every unit of *repeated* information is pure scan-
cost with zero differentiation value (if the title already told you the
branch, seeing "Branch: same-thing-again" doesn't help you tell this card
apart from its neighbors — it actively costs you a line of vertical space in
a view where vertical space is the scarce resource, per the mobile+desktop
dual-form-factor requirement in this repo's CLAUDE.md). This means the
right framing for the fix isn't "hide exact-string matches" as an end in
itself, but "keep every row that adds *new, differentiating* information
and drop every row that doesn't" — which is precisely what exact/normalized-
match suppression (AC1-3) approximates cheaply without requiring any
semantic understanding of the field. It's a good, low-risk proxy for the
real job (maximize signal density per vertical pixel), and matches what
requirements.md already scoped — no broader "smart truncation" or
relevance-ranking system is needed to serve the actual JTBD; simple
duplicate suppression captures most of the value for the size of the
change.

## Recommendations summary

1. **Per-row suppression, not a consolidated `paneMeta()`-style line** —
   matches the existing `info`/`infoRow` layout, requirements.md's
   explicit out-of-scope boundary, and avoids collapsing link/tooltip rows
   into a single string.
2. **Suppress by conditional-render (no JSX node), not CSS hide** — matches
   the file's existing `session.branch && (...)` idiom and removes the row
   from the accessible tree cleanly.
3. **Exact/normalized whole-value match only, never substring/partial** —
   required both for AC8 (accessibility — don't destroy the only source of
   a fact) and Q2 (predictability — avoid fuzzy heuristics users can't
   reason about).
4. **Guard the all-fields-redundant edge case explicitly** — verify the
   empty-`info` case doesn't leave a visible dead gap; if it does, guarantee
   at least one row (e.g. `Program`) always renders regardless of match, so
   no card body renders totally empty. Flag for plan/validate phases with a
   dedicated test fixture.
5. **No change needed to the card-level `aria-label`** (`:411`) — it already
   carries the title, which is the fact every suppressed row would have
   otherwise duplicated.
