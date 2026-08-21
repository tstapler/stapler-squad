# Pitfalls Research — session-card-subtitle-dedup

Scope: `web-app/src/components/sessions/SessionCard.tsx`, info rows at
[SessionCard.tsx:700-778](web-app/src/components/sessions/SessionCard.tsx#L700-L778).
Precedent helper: `hasPendingProgramChange` at
[SessionCard.tsx:22-30](web-app/src/components/sessions/SessionCard.tsx#L22-L30).

## 1. Accessibility pitfall — suppressing a row can silently drop a tooltip, not just visual text

Two of the eight rows carry information in a `title=` attribute that exists
*only* on that row's `<span>`, with no other element in the card exposing the
same string:

- Path row: `<span className={value} title={session.path}>` —
  [SessionCard.tsx:713](web-app/src/components/sessions/SessionCard.tsx#L713)
- Cloned To row: `<span className={value} title={session.clonedRepoPath}>` —
  [SessionCard.tsx:760](web-app/src/components/sessions/SessionCard.tsx#L760)

If dedup removes the whole row when `session.path === session.title` (or the
same for `clonedRepoPath`), sighted mouse users lose the hover-tooltip
affordance for the full untruncated path — CSS on the `value` class likely
truncates with ellipsis for long paths, and the `title` attribute is the only
way to recover the full string without a wider layout. This is a real (if
minor) UX regression, not just a text duplication cleanup, because the title
row itself (`aria-label` at
[SessionCard.tsx:411](web-app/src/components/sessions/SessionCard.tsx#L411))
is `${session.title}, press F2 to rename` — it does not carry a `title=`
tooltip, so it won't reveal a truncated full path/value on hover the way the
Path row's span does.

The other affected rows (Repository, Pull Request) attach `aria-label` to an
`<a>` link, not the row's static text, and describe link *behavior*
("...opens in new tab") rather than duplicating the visible value alone — those
are lower risk to suppress since the value itself remains in the link's own
text content.

**No existing accessibility-specific convention in this repo governs
title-vs-visible-text duplication** — `.claude/rules/css-architecture.md`
covers vanilla-extract token usage only, not ARIA/tooltip conventions, and no
`.claude/docs/*` file addresses this. There is no house rule to cite beyond
observing the existing pattern of `aria-label` + `title` co-occurring in this
same file.

**Recommendation for the plan:** when suppressing the Path or Cloned To row
because it duplicates the title, decide explicitly whether to (a) keep
rendering the row's `title=` tooltip host element with `display: none`-safe
semantics (not just deleting the JSX block), (b) move the tooltip onto the
title element itself, or (c) accept the loss as intentional since the value is
now visible verbatim in the (untruncated, non-tooltip) title text above. Silent
row deletion without this decision is the pitfall.

## 2. Over-matching pitfall — exact-equality is safe by default, but any normalization needs a substring guard

Because the row value fully duplicates the title only when the two strings are
byte-for-byte the *same conceptual value* (e.g. title generated directly from
branch name, or title left as default = path basename), a naive
`value === title` (post-trim) equality check is the safe default — it produces
false negatives (row stays visible when it arguably could be suppressed) but
never false positives.

The risk enters as soon as normalization gets "smarter" than trim + exact
match:

- **Basename/substring matching** (e.g. suppress the Path row if
  `title` matches the last path segment) is explicitly named as out-of-scope
  risk in the task brief's own example: title `"fix-auth"` must NOT suppress
  a path row whose basename is `"fix-auth-2"`. A `.includes()` or prefix-match
  implementation would incorrectly treat `"fix-auth-2".startsWith("fix-auth")`
  as a match. **Use exact equality only, not substring/prefix containment.**
- **Case-folding** (`toLowerCase()`) risks suppressing a Branch row like
  `Fix-Auth` against a title `fix-auth` even though branch names are
  case-sensitive in git and the visual distinction may matter to the user
  (e.g. two branches differing only in case, unusual but not impossible).
  Given this is a display-dedup feature (not an identity/routing decision),
  case-insensitive compare is a defensible choice, but it should be a
  deliberate, documented decision in the helper's doc comment (following the
  `hasPendingProgramChange` precedent of documenting *why*), not an implicit
  side effect of using `.toLowerCase()` for "safety."
- **Whitespace/trim** is the only normalization that's unambiguously safe —
  apply `.trim()` to both sides before comparing, since titles are
  user-editable text (inline rename at
  [SessionCard.tsx:437](web-app/src/components/sessions/SessionCard.tsx#L437))
  and could pick up leading/trailing whitespace that has no visual
  significance.
- **Falsy/empty values**: several row values are optional
  (`session.branch`, `session.workingDir`, `session.clonedRepoPath`,
  `session.goal?.goalText`) and already gated by `&&` in JSX. The helper must
  not treat `""  === ""` as a "duplicate" (both title and value empty is
  degenerate, not a real dedup case) — guard on non-empty value before
  comparing.

**Recommendation:** implement as exact string equality after `.trim()` only.
Do not add case-folding or substring/basename matching in the first version;
if a real duplicate case surfaces later that needs case-insensitivity, add it
as a separate, explicitly-tested change.

## 3. Test-breakage pitfall — checked existing fixtures, no exact collisions found today, but assert the guard

Searched all three existing SessionCard test files for title/program/branch/path
overlap:

- `web-app/src/components/sessions/__tests__/SessionCard.approval-suppression.test.tsx:83-90`
  — `title: "Test Session"`, `path: "/tmp/session"`, `branch: ""`,
  `program: "claude"`. No collision.
- `web-app/src/components/sessions/__tests__/SessionCard.click.test.tsx:80-86`
  — same shape (`title: "Test Session"`, `path: "/tmp/session"`, `branch: ""`,
  `program: "claude"`). No collision.
- `web-app/src/components/sessions/__tests__/SessionCard.pending-program.test.tsx:21-67`
  — only sets `program` fields (`"claude"`/`"aider"`), no `title` field
  override visible in the read range; the shared base fixture is likely reused
  from a helper — worth re-checking the actual fixture builder if one exists,
  but no direct `title === program` collision found in this file's own lines.

None of the three files use `getByText`/`queryByText`/`getAllByText` assertions
that match against row label/value text (confirmed via grep across all three
`SessionCard*.test.tsx` files — zero hits) — they test click propagation and
pending-program-change badge behavior, not row-by-row text presence. **So no
existing test currently asserts "Program:" + value is rendered by text
content**, which lowers (but does not eliminate) breakage risk: a future
snapshot test or a `data-testid`-based row-count assertion added by the
dedup feature's own test suite is the actual risk surface, not today's tests.

**Recommendation:** when writing the dedup feature's own tests, include a
regression fixture where `title === path` (or `=== branch`) explicitly, to
prove the row is suppressed, and a near-miss fixture (`title = "fix-auth"`,
`path` ending in `.../fix-auth-2`) to prove it is NOT suppressed (per pitfall
2). Since no current fixture collides, there is no pre-existing test to fix,
but add the guard fixture so a future refactor can't silently reintroduce
over-matching.

## 4. Memoization pitfall — low risk, confirmed shallow-prop `memo()`

`SessionCard` is exported as `export const SessionCard = memo(SessionCardInner);`
at [SessionCard.tsx:893](web-app/src/components/sessions/SessionCard.tsx#L893)
with **no custom comparison function** — i.e. plain shallow prop-equality
memoization, identical to how `hasPendingProgramChange` is already called
inline during render (it's a plain function call, not itself memoized with
`useMemo`).

A new `getVisibleRows`/`shouldSuppressRow`-style pure helper follows the exact
same pattern: it's a cheap string-compare computation over primitive session
fields (`session.title`, `session.path`, `session.branch`, etc.) that are
already part of the `session` prop object driving `memo()`'s shallow
comparison. Adding the helper call does not:

- change what triggers a re-render (still driven by `session` object identity
  changing, same as today),
- require its own `useMemo`/`useCallback` wrapper — `hasPendingProgramChange`
  is called directly inline without memoization and this is consistent with
  it being O(1) string work, not an expensive computation.

**No performance regression expected.** The only latent risk is unrelated to
memoization: if the dedup helper is (wrongly) implemented to loop over *all*
rows and build a Set/array on every render, that's still O(number of rows) ≈
8 comparisons, negligible. Confirm the eventual implementation doesn't do
anything more expensive (e.g. regex compilation per render) — but no evidence
that's planned.

## 5. General pitfalls for title/subtitle dedup in list/card UIs (industry patterns)

- **Over-normalizing beyond what the eye perceives as "the same"** — the
  classic failure mode (also pitfall 2) is treating semantic similarity as
  visual duplication. Users read the *rendered* text, not a normalized form;
  dedup should match the visual dedup a user would themselves eyeball, which
  favors conservative exact-match over fuzzy/substring logic.
- **Under-normalizing leaves stale-looking duplication** — the opposite
  failure: not trimming whitespace or not handling the literal empty string
  vs `undefined` distinction leaves rows that look like duplicates to the user
  but aren't caught by the helper, defeating the feature's purpose. Both
  directions are real risks; exact match + trim strikes the balance without
  the extra semantic risk of case-folding or partial matching.
- **i18n/locale case-folding**: `.toLowerCase()` is locale-sensitive in some
  languages (Turkish `İ`/`i` dotted/dotless-I problem being the canonical
  example) — if case-insensitive comparison is ever added, prefer
  `.toLocaleLowerCase()` or accept exact-case-only matching to sidestep this
  entirely. Since session titles/paths/branches in this app are
  developer-facing identifiers (paths, branch names, program names) rather
  than user-facing prose, locale-sensitive folding is unlikely to matter in
  practice, but it's a known trap if this pattern gets reused for
  user-authored free text elsewhere.
- **Flicker/thrash on rename**: the title is inline-editable
  (`isInlineEditing`, F2-to-rename at
  [SessionCard.tsx:437](web-app/src/components/sessions/SessionCard.tsx#L437)).
  As a user types a new title character-by-character (if dedup were ever
  wired to live keystroke state rather than committed `session.title`), rows
  could flicker in and out of existence on every keystroke that happens to
  match/unmatch a value. This project's dedup is scoped to the *committed*
  `session.title` prop (not live edit-buffer state), so this specific flicker
  risk does not apply as currently scoped — but it's worth stating explicitly
  in the plan/spec that dedup keys off `session.title`, not any local
  editing-state string, so a future implementer doesn't wire it to the
  live input value by mistake.
- **Reversibility / discoverability cost**: hiding a row entirely (vs. e.g.
  dimming or de-emphasizing it) removes the *fact* that the field exists on
  the session, not just its redundant display. If a future feature (filter,
  search, or copy-value affordance) that this card doesn't yet have relies on
  a row being present to click/copy from, full suppression is a stronger UX
  commitment than styling-only de-emphasis. Out of scope for this feature per
  the task brief (no layout redesign), but worth a one-line note in the plan
  that "suppress" means "don't render," not "de-emphasize," since that's the
  precedent `hasPendingProgramChange` sets (it drives a conditional render at
  [SessionCard.tsx:633](web-app/src/components/sessions/SessionCard.tsx#L633),
  not a CSS-only dimming).

## Summary of concrete recommendations for the plan phase

1. Dedup helper should be **exact string equality after `.trim()`**, keyed off
   committed `session.title` — no substring/basename matching, no case-folding
   in v1.
2. Explicitly decide the fate of the `title=` tooltip on Path/Cloned To rows
   before suppressing them (accessibility pitfall #1) — don't let JSX deletion
   silently drop the hover-tooltip affordance without a decision.
3. Add both a positive fixture (`title === path`, row suppressed) and a
   near-miss fixture (`title` is a substring/prefix of the value but not
   equal, row NOT suppressed) to the new feature's test suite — no existing
   test fixture collides today, so this is new coverage, not a fix.
4. No memoization changes needed; call the helper inline during render exactly
   like `hasPendingProgramChange`.
