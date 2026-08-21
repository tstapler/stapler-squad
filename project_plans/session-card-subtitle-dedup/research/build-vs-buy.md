# Build vs. Buy — session-card-subtitle-dedup

## Scope

Feature: in `web-app/src/components/sessions/SessionCard.tsx`, hide a secondary
info-row (subtitle) when its displayed value duplicates the primary title —
following the existing `hasPendingProgramChange` precedent (an exported pure
predicate, unit-tested in isolation, `web-app/src/components/sessions/SessionCard.tsx:22`).

## 1. Existing OSS library — Not recommended

Searched for what a "smart subtitle dedup" search would surface:
`string-similarity`, `fastest-levenshtein`, `leven`, `string-comparison`,
`natural` (NLP toolkit). These are all **fuzzy/distance-based** matchers —
they return a similarity score (0–1) or an edit-distance integer, meant for
"did the user mean X" typo-tolerance, fuzzy search ranking, or dedup of
near-identical records with typos/casing drift across a dataset.

That's the wrong shape for this feature. The requirement here is:
- exact string equality, or
- case-insensitive equality, or
- "one value is the path-basename of the other" (e.g. title = `/home/tstapler/foo`,
  subtitle = `foo`)

All three are deterministic, O(n) string operations with a single correct
answer — no scoring, no threshold tuning, no fuzzy false-positive risk. Pulling
in a similarity library would add a dependency (bundle size, `package.json`
maintenance, transitive audit surface) to replace `===`, `.toLowerCase()`, and
`.split("/").pop()` — three built-in operations already used elsewhere in this
exact codebase (see §3/§4). None of `web-app/package.json`'s current
dependencies include any such library (confirmed: no `string-similarity`,
`fastest-levenshtein`, `leven`, or `natural` in `web-app/package.json`).

**Verdict: Not recommended.** No narrowly-scoped npm package exists for
exact/near-exact label-vs-value dedup; the closest matches (fuzzy-string-distance
libraries) solve a different problem and are overkill for this one.

## 2. SaaS/managed API — Not applicable

This is pure client-side display logic operating on data already present in
the rendered `Session` object (title, program, path, branch, etc.) — there is
no external service, no network call, and no reason one would ever be
involved. Confirmed by reading `SessionCard.tsx`: all fields compared come
from props already passed into the component. **N/A.**

## 3. LLM-generated bespoke implementation vs. battle-tested library (`path.basename`)

Node's `path` module (`path.basename`) is **not usable as-is** in this
codebase's target: `SessionCard.tsx` is a `"use client"` component (line 1),
so this logic runs in the browser bundle, not Node. `path.basename` requires
either Webpack/Next's Node polyfill shim (deprecated/removed by default in
recent Next.js/webback 5 configs) or the `path-browserify` package — neither
of which is presently a dependency (`grep` of `web-app/package.json` for
`"path"` only matched Playwright's `out/_next/...` build-artifact glob
patterns in a CI config block, not an actual npm package entry).

More importantly, it's unnecessary: **the codebase already has a working,
browser-safe basename convention it uses in at least four places**, so there
is nothing to "choose a library" for — the bespoke one-liner is already the
established idiom here, not a risk to hand-roll fresh:

- `web-app/src/app/insights/SessionsTable.tsx:40-42` — dedicated named helper:
  ```ts
  function pathBasename(p: string): string {
    return p.split("/").pop() || p;
  }
  ```
- `web-app/src/app/page.tsx:120` — `s.path?.split("/").pop()?.toLowerCase()`
- `web-app/src/components/sessions/RecentFilesSection.tsx:49` — `path.split("/").pop() ?? path`
- `web-app/src/lib/hooks/useAvailablePrograms.ts:21` — `fullPath.split("/").pop() ?? fullPath`

`.split("/").pop()` is a plain, well-understood JS idiom for the POSIX
basename case this app needs (session paths/branches are always POSIX-style
even on the rare Windows dev box, since they come from git/tmux server-side).
It doesn't need Windows-path (`\`) handling, `path.parse`-style extension
splitting, or any of the edge-case handling `path.basename` provides for a
full filesystem API — pulling in `path-browserify` to get feature parity the
feature doesn't need would be strictly worse than the four lines already
proven out in this repo.

**Verdict: Recommended — hand-roll via the existing `.split("/").pop()`
idiom**, not `path.basename`/`path-browserify`. Safer in practice because it
matches an idiom already reviewed and shipped four times in this codebase,
with zero added bundle weight.

## 4. Fork or adapt an existing internal helper

No single shared/exported utility currently unifies this logic — it's
duplicated ad hoc at each of the four call sites above (three inline,
one as a private, unexported function in `SessionsTable.tsx`). There is
no "normalize display string" or generic dedup helper in
`web-app/src/lib/utils/` (checked: no basename/dedup/normalize helpers found
there). The Go backend's `session/git` basename usage (referenced in the task
context) operates server-side on repo paths for a different purpose (deriving
default branch/session names) and isn't reachable from client TS — no cross-
language reuse opportunity.

**Verdict: Viable, with a recommendation** — this feature is a good trigger to
promote `SessionsTable.tsx`'s private `pathBasename` into a shared exported
helper (e.g. `web-app/src/lib/utils/path.ts`) if the new subtitle-dedup logic
needs basename comparison, so a fifth inline duplicate doesn't get added. This
is optional polish, not a blocker — the predicate can also be written self-
contained in `SessionCard.tsx` (mirroring `hasPendingProgramChange`'s
locality) if the plan phase prefers to keep the diff minimal.

## Summary Table

| Option | Verdict |
|---|---|
| OSS fuzzy-string library (`string-similarity`, `fastest-levenshtein`, etc.) | Not recommended — wrong problem shape (fuzzy vs. exact) |
| SaaS/managed API | Not applicable — no external service involved |
| `path.basename` (Node) / `path-browserify` | Not recommended — unnecessary polyfill dependency for a browser client component |
| Hand-rolled `.split("/").pop()` basename + exact/case-insensitive string compare | **Recommended** — matches existing repo idiom (4 call sites), zero new dependencies |
| Extract shared helper from `SessionsTable.tsx`'s `pathBasename` | Viable — nice-to-have consolidation, not required for this feature |

## Recommended approach

Bespoke pure TypeScript predicate in `SessionCard.tsx`, following the
`hasPendingProgramChange` precedent exactly: exported, typed on a `Pick<...>`
of the fields it needs, unit-tested in its own `__tests__` file. Comparison
logic: exact match, case-insensitive match, and basename match using the
established `.split("/").pop()` idiom — no new dependencies.
