# UX Design: session-card-subtitle-dedup

Source: `requirements.md`, `research/ux.md`, `implementation/plan.md` (chosen approach —
row-level conditional suppression of Branch/Path/Working Dir/Cloned To/Goal rows when their
value exactly matches, or, for path-shaped rows, basename-matches, the session title;
Program/Repository/Pull Request rows are explicitly excluded from dedup).

## Step 1 — Surfaces affected

There is exactly **one** surface: the info-row block inside `SessionCard`'s `body`
(`web-app/src/components/sessions/SessionCard.tsx:699-778`), rendered once per session card in
the session list. No new screen, modal, route, or interactive control is introduced — this is a
rendering-rule change applied at the point the existing 8 info rows are already conditionally
(5 of 8) or unconditionally (3 of 8: Program, Path, Working Dir was previously conditional but
Program/Path are the two unconditional today per plan.md) rendered.

Three states of that one surface are in scope for this design:

| State | Description | Which ACs it covers |
|---|---|---|
| **A — Has duplicate fields** | One or more of the 5 dedup-eligible rows (Branch, Path, Working Dir, Cloned To, Goal) exactly/basename-matches the title; at least one other row does not. | AC1, AC2, AC6 |
| **B — No duplicate fields** | None of the 5 dedup-eligible rows match the title. Card renders identically to today (regression check). | AC4 |
| **C — All-candidate-fields-redundant (edge case)** | Every dedup-eligible row that has a value matches the title; only Program (always renders) and any non-dedup rows (Repository, Pull Request) that happen to be present survive. | AC3, edge-case mitigation from research/ux.md §4 |

All three states are the *same component in the same place* — there is no separate "empty
state" screen to design; the edge case (State C) is a variant of the same block, not a new
surface.

## Step 2 — Wireframes, flow, edge case

### State A — Has duplicate fields

Representative example (matches plan.md's AC1/AC2 examples): `title = "fix-auth"`,
`branch = "fix-auth"`, `path = "/home/user/worktrees/fix-auth"`,
`workingDir = "/home/user/worktrees/fix-auth"`, `program = "claude"`,
`goal.goalText = "Fix the auth regression"` (goal does *not* match title, so it survives).

**BEFORE (current behavior — no dedup):**

```
┌──────────────────────────────────────────────────┐
│ ● fix-auth                                  [···] │  ← title row (bold, primary)
├──────────────────────────────────────────────────┤
│ Program:      claude                              │
│ Branch:       fix-auth                            │  ← duplicates title
│ Path:         /home/user/worktrees/fix-auth       │  ← basename duplicates title
│ Working Dir:  /home/user/worktrees/fix-auth       │  ← basename duplicates title
│ Goal          Fix the auth regression             │
├──────────────────────────────────────────────────┤
│ [diff stats / status footer]                      │
└──────────────────────────────────────────────────┘
```

**AFTER (with dedup applied):**

```
┌──────────────────────────────────────────────────┐
│ ● fix-auth                                  [···] │  ← title row unchanged
├──────────────────────────────────────────────────┤
│ Program:      claude                              │  ← unchanged (excluded from dedup)
│ Goal          Fix the auth regression              │  ← unchanged (no match)
├──────────────────────────────────────────────────┤
│ [diff stats / status footer]                      │
└──────────────────────────────────────────────────┘
```

Branch, Path, and Working Dir rows are gone — not blanked, not grayed out, not replaced with a
placeholder. The `info` block simply has 2 rows instead of 5; the label column (`minWidth:
100px`) still aligns for the rows that remain, and the block's height shrinks by exactly
`3 × (row height + 6px gap)` with no residual gap (per `SessionCard.css.ts:293-297`'s
`flexDirection: column; gap: 6px` — gap only inserts space between surviving children).

### State B — No duplicate fields (regression check)

Representative example: `title = "implement-oauth"`, `branch = "feature/sso"`,
`path = "/home/user/worktrees/implement-oauth-work"`, `program = "claude"`,
`goal.goalText = "Ship SSO login"` (none match — this is plan.md's AC4 fixture).

```
┌──────────────────────────────────────────────────┐
│ ● implement-oauth                           [···] │
├──────────────────────────────────────────────────┤
│ Program:      claude                              │
│ Branch:       feature/sso                         │
│ Path:         .../worktrees/implement-oauth-work  │
│ Goal          Ship SSO login                       │
├──────────────────────────────────────────────────┤
│ [diff stats / status footer]                      │
└──────────────────────────────────────────────────┘
```

BEFORE and AFTER are pixel-identical — this state exists purely to confirm the dedup change is
*inert* for the common case, not to show a visual difference. This is the majority case in
production: most sessions have a title that differs from their branch/path (e.g. auto-generated
titles, or a title edited after creation).

### State C — All-candidate-fields-redundant (edge case)

Representative example (bare local session, no GitHub repo/PR, no divergent goal):
`title = "fix-auth"`, `branch = "fix-auth"`, `path = "/home/user/worktrees/fix-auth"`,
`workingDir = "/home/user/worktrees/fix-auth"`, `program = "claude"`, no `githubOwner`, no
`clonedRepoPath`, no `goal`.

**BEFORE:**

```
┌──────────────────────────────────────────────────┐
│ ● fix-auth                                  [···] │
├──────────────────────────────────────────────────┤
│ Program:      claude                              │
│ Branch:       fix-auth                            │
│ Path:         /home/user/worktrees/fix-auth       │
│ Working Dir:  /home/user/worktrees/fix-auth       │
├──────────────────────────────────────────────────┤
│ [diff stats / status footer]                      │
└──────────────────────────────────────────────────┘
```

**AFTER:**

```
┌──────────────────────────────────────────────────┐
│ ● fix-auth                                  [···] │
├──────────────────────────────────────────────────┤
│ Program:      claude                              │
├──────────────────────────────────────────────────┤
│ [diff stats / status footer]                      │
└──────────────────────────────────────────────────┘
```

This is the case research/ux.md §4 flagged as the "looks broken" risk — an `info` block that
suppresses down to *zero* rows would leave a dead gap between the title and the status footer
with no visible cue that suppression happened. Confirming it doesn't look broken: **the plan's
mitigation is that Program is excluded from dedup and has no presence guard at all**
(`SessionCard.tsx:701-704` renders unconditionally), so the `info` block can never render fully
empty — there is always at least the `Program:` row. The one-row state above is not a bug state;
it reads as "this card's title already tells you everything except which agent is running it,"
consistent with the block's job-to-be-done (differentiation at a glance, not literal
completeness — research/ux.md §5).

### Interaction flow

This is a **passive, render-time computation** — there is no user action, click target, or
control that triggers dedup. It runs identically to the file's existing conditional-render
pattern (e.g. `session.branch && (...)`) every time `SessionCard` renders with a given
`session` object: on initial session-list load, on any session-list refresh/poll, and on title
edit (F2 rename) once the new title is committed and the card re-renders with updated
`session.title`. There is no loading state, no transition/animation between "row present" and
"row absent" specified by this design — rows appear or don't appear as part of the same render
pass the title itself appears in, matching how the other 5 already-conditional rows behave
today (no fade/collapse animation exists for those either). No flow diagram is warranted because
there is no multi-step interaction to diagram.

## Step 3 — UX acceptance criteria

Each of these is testable by a human looking at (or interacting with) the running app — not
just by a unit test.

1. **No orphaned labels.** For every session card, in every state (A/B/C), no row ever renders
   with a label (`Branch:`, `Path:`, `Working Dir:`, `Cloned To:`, `Goal`) and an empty/blank
   value span. A row is either fully present (label + value) or fully absent — never
   label-only. *Verify by*: inspect DOM/visually scan a card in each of the three states above;
   confirm every visible label has non-empty text next to it.
2. **No dead visual gap.** In State C (all dedup-eligible rows redundant, no Repository/PR/Goal
   present), the `info` block shows exactly the `Program:` row with normal spacing to the title
   above and the status footer below — no collapsed-but-still-occupying-space blank region, no
   double gap. *Verify by*: create or find a session matching State C's fixture in a running
   instance (or Storybook/manual test harness) and visually confirm the card's vertical rhythm
   matches a normal single-row card, not a card with a phantom empty block.
3. **Label column stays aligned across mixed states.** When scanning multiple cards in the
   session list where some have had rows suppressed and others haven't, the `label` column
   (fixed `minWidth: 100px`) remains left-aligned per-card — dedup must not introduce per-row
   width jitter within a single card. *Verify by*: view a list containing at least one State A
   card and one State B card side by side; confirm each card's own label column is internally
   aligned (this is unaffected by dedup since row structure is unchanged, only row count).
4. **Predictability — no partial/fuzzy suppression.** A near-miss value (e.g. branch
   `"fix-auth-2"` against title `"fix-auth"`) is never suppressed — only an exact
   (trim-only, case-sensitive) or exact-basename match is. *Verify by*: create a session with a
   title/branch near-miss per plan.md AC2's fixture and confirm the row still renders in full.
5. **"No dead ends" — N/A.** This heuristic (does every screen/state give the user a next
   action) does not apply: dedup is a passive rendering rule with no navigation entry or exit
   point, no empty state requiring a CTA, and no interactive element added or removed. There is
   no "dead end" to check because there is no new destination to reach or leave.
6. **Keyboard navigation — unchanged, no new check needed.** No new interactive elements
   (buttons, links, inputs) are added or removed by this change — the only elements that were
   ever keyboard-focusable in this block (the Repository/Pull Request `<a>` links) are excluded
   from dedup scope and remain exactly as focusable as before. *Verify by*: tab through a card
   in State A/C and confirm the same set of focusable elements exists as on an undeduped card
   with the same Repository/PR data.
7. **Screen-reader information is not lost beyond the documented, bounded tradeoff.** Per
   `implementation/plan.md`'s AC8 and ADR-001, suppressing a row is only acceptable because the
   suppressed value's full text (or its basename) remains available elsewhere in the card's
   accessible content — specifically the card's `aria-label` (`SessionCard.tsx:411`, which
   already includes `session.title`) and the visible title text itself, read in DOM order. The
   one deliberate, bounded exception is the `Cloned To` row's parent-directory prefix (e.g.
   `/tmp/clones/`) when only its basename matches the title — per plan.md AC8, that prefix is no
   longer announced on the card, but the basename remains readable via the title, and the full
   path remains available in `SessionDetailView`. *Verify by*: with a screen reader (or the
   accessibility tree inspector), focus a State A/C card and confirm the announced content still
   includes every suppressed row's value in substance (via the title), with only the documented
   `Cloned To` prefix loss as an exception.
8. **Color contrast — N/A.** No new colors, styles, or visual treatments are introduced; rows
   either render with their existing `label`/`value` styling or don't render at all. There is no
   new contrast ratio to check.
9. **No dedup-caused layout shift/reflow glitch.** Loading a card whose fields dedupe (State
   A/C) does not produce a visible "flash" of the full row set before it collapses — because
   dedup is computed before render (a conditional in the JSX, not a post-render effect), the
   card should paint once, already in its final (deduped) shape. *Verify by*: hard-refresh the
   session list and visually confirm no flicker/row-collapse animation occurs on cards with
   dedup-eligible matches.

## Summary of what this design does *not* introduce

- No new component, screen, modal, or route.
- No new user-triggerable action (no toggle to turn dedup on/off — it is unconditional).
- No new color, animation, or layout primitive beyond removing existing `<div>` nodes from the
  render tree, using the same conditional-render idiom already used for 5 of the file's 8 rows.

This keeps the design proportional to the change: a row-suppression rule on one existing
component, verified against its three possible states.
