# Research: Build vs. Buy — unified-vcs-widget

Agent: Research Agent 6 (Build vs. Buy)

## 1. Diff rendering

**Current state**: `web-app/src/components/shared/DiffRenderer.tsx` is 100% hand-rolled.
It calls `parseDiff()` (`web-app/src/lib/utils/parseDiff.ts`, 78 lines), which does
line-by-line regex parsing of a raw unified-diff string (`diff --git`, `+++ b/...`,
`@@ -a,b +c,d @@` hunk headers, then `+`/`-`/` ` prefixed lines) into a `DiffFile[]`
structure. `DiffRenderer` then renders that structure itself with plain `<div>`s styled
via vanilla-extract (`DiffRenderer.css.ts`). There is:
- No syntax highlighting (despite `shiki` and CodeMirror language packages already being
  dependencies for other editor surfaces — see package.json — they are not wired into the
  diff view).
- No side-by-side/split view — the "Split" toggle button exists in the UI but is
  `disabled` with a "Split view coming soon" tooltip, i.e. explicitly unimplemented.
- No word-level intraline diffing (only whole-line add/delete/context).

**Options**:

### A. Extend the existing hand-rolled `DiffRenderer`/`parseDiff`
- **Pros**: Zero new dependency; full control over styling to match vanilla-extract token
  system without fighting a library's own CSS/theming assumptions; parser is small (78
  lines) and easy to reason about; already integrated with the existing `DiffViewer.tsx`
  wrapper and consumed by at least one panel.
- **Cons**: Split/side-by-side view and syntax highlighting are real, non-trivial features
  to hand-build correctly (word-diff algorithms, virtualization for large diffs, language
  detection) — this is exactly the kind of "reinvent a solved problem" work a library
  exists to avoid. `parseDiff`'s regex-based hunk parser is also fragile against edge cases
  well-handled by mature parsers (renames, binary files, no-newline-at-EOF markers, mode
  changes) — none of those are currently handled at all.
- **Verdict**: **Viable** for the current unified-view-only scope, but extending it to
  cover the open "inline expandable diff" requirement (which implies richer per-hunk
  expand/collapse plus likely syntax highlighting) means re-deriving logic a library
  already solves.

### B. Adopt a small diff-rendering library (e.g. `diff2html`, `react-diff-view`)
- **Pros**: `react-diff-view` (npm: `react-diff-view`) ships a parser (`gitdiff-parser`)
  that correctly handles renames/binary/no-newline cases, plus a `<Diff>` React component
  supporting both `unified` and `split` render modes and hunk-level expand/collapse
  out of the box — directly matches the "inline expandable diff" open question. It is
  headless/unstyled (you supply CSS), which is compatible with the vanilla-extract-only
  CSS architecture rule (no CSS-in-JS runtime, no bundled stylesheet dependency lock-in).
  `diff2html` is also viable but leans toward server-rendered HTML strings / bundled CSS
  themes, which is a worse fit for a vanilla-extract-only codebase than a headless
  component library.
- **Cons**: New dependency to vet (license, maintenance activity, bundle size — check
  against the existing `size-limit` budgets in `web-app/package.json`, which already caps
  the total JS bundle at 5MB and the main app chunk at 400KB); requires replacing/wrapping
  the existing `parseDiff.ts` + `DiffRenderer.tsx` pair, i.e. a real migration, not an
  additive change; team must learn a new component API.
- **Verdict**: **Recommended** *if and only if* the parallel UX research agent confirms the
  "inline expandable diff" requirement needs side-by-side view and/or syntax highlighting.
  If the requirement is satisfied by unified-view-only with simple expand/collapse of
  context lines, Option A remains sufficient and cheaper. Prefer `react-diff-view` over
  `diff2html` for CSS-architecture fit (headless, no bundled theme CSS to fight vanilla-extract).

## 2. GitHub API client

**Confirmed by grep**: `go.mod` has **no** `github.com/google/go-github` or
`github.com/shurcooL/githubv4` dependency (searched `go.mod`/`go.sum` directly — no hits).
The existing GitHub integration (`session/backlog_plugin_github.go`) is a **hand-rolled
REST client**: raw `net/http` calls to `https://api.github.com` (see `githubAPIBaseURL`,
`githubAPIURL()` helpers), manual `encoding/json` struct decoding (`githubIssue` struct),
manual pagination (`githubIssuesPerPage = 50`), and a bearer token read from plugin config.
`server/services/backlog_github_rpc_test.go` and `session/backlog_plugin_github.go` are the
only two GitHub-API touch points found in `server/` and `session/`.

- **Pros of reusing the existing hand-rolled client**: Already proven against the real
  GitHub API for issues; no new dependency; consistent with how the rest of the codebase
  talks to GitHub; the durable-snapshot feature needs PR/CI/review data which is a similarly
  small, well-defined REST surface (`GET /repos/{owner}/{repo}/pulls/{number}`,
  `.../pulls/{number}/reviews`, `.../commits/{sha}/check-runs` or `/status`) — comparable
  complexity to what's already hand-rolled for issues.
- **Cons**: `go-github` would save boilerplate (typed structs for PR/review/check-run
  responses, built-in pagination helpers, rate-limit header parsing) that will otherwise be
  re-hand-rolled for a second, larger API surface (PRs + reviews + CI checks is bigger than
  the issues-only surface currently covered).
- **Verdict**: **Recommended — reuse/extend the existing hand-rolled client** (mirror the
  `githubAPIURL()`/`githubPluginConfig` pattern in `session/backlog_plugin_github.go` for
  PR/CI/review fetching), per the requirement to confirm rather than assume. Since there is
  zero existing investment in `go-github`/`githubv4` and the codebase has an established,
  tested hand-rolled pattern for this exact API, introducing a new SDK dependency for one
  additional (still small) endpoint surface is not justified. Revisit only if the PR/CI/review
  fetch logic grows materially more complex than issues fetch (e.g. GraphQL needed for
  nested review-thread data) — at that point `shurcooL/githubv4` becomes worth reconsidering.

## 3. ent ORM upsert for durable snapshots

**Confirmed**: ent + `--feature sql/upsert` is already established
(`session/ent/generate.go:3`: `//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate
--feature sql/upsert ./schema`, per `.claude/rules/ent-schema-generation.md`). Existing
upsert call sites already exist and work: `server/services/rules_store.go` (`UpsertRule`)
and `server/services/rules_service.go` (`RulesStore.Upsert`, `BulkUpsertRules` RPC),
exercised by `server/services/rules_service_test.go`.

- **Pros**: Snapshot-on-ship persistence (PR state, CI status, review counts, per-file diff
  stats at time of shipping) is low-volume, per-backlog-item structured data — exactly what
  the existing SQLite-backed ent schema already handles well elsewhere. Adding a new ent
  schema type (e.g. `GitHubSnapshot` or extending the existing ship-status entity) and
  using `client.GitHubSnapshot.Create().OnConflict(...).UpsertX()` follows a proven,
  already-generated code path — no new codegen risk, no new runtime dependency.
- **Cons**: None identified for this data volume/shape. A separate cache/store (Redis, etc.)
  would add an operational dependency (a service to run, back up, monitor) for data that is
  small, infrequently written (on-ship / on-poll), and already fits the relational-ish
  shape ent models well.
- **Verdict**: **Recommended — build on existing ent schema.** **Not recommended: new
  datastore** (e.g. Redis) — no volume, latency, or access-pattern justification exists for
  one; this is small structured data written occasionally and read by the widget on page
  load, which SQLite via ent already serves fine elsewhere in this codebase.

## 4. Icon/badge iconography

**Confirmed by reading `web-app/package.json` `dependencies`**: `lucide-react": "^1.14.0"`
is already an installed dependency. Grep confirms it is already in active use in
session-related components: `web-app/src/components/sessions/FileChipList.tsx`,
`SessionActionsOverflow.tsx`, and `SessionDetailView.tsx` all import from `lucide-react`.

Meanwhile, raw emoji glyphs (🌿, 🔄, ⑂, ⎇, ✓, ✗, etc.) are used as VCS/status indicators in
the exact surfaces this feature touches or is adjacent to: `VcsPanel.tsx`,
`VcsStatusDisplay.tsx` (in `components/shared/`), `ShipStatusDisplay.tsx` (backlog),
`HistoryEntryCard.tsx`, `HistoryFilterBar.tsx`, `SessionActionsOverflow.tsx`,
`SessionDetailView.tsx`, `WorkspaceSwitchModal.tsx`, and `TerminalOutput.tsx`.

- **Pros of adopting `lucide-react` for the new widget's icons**: Zero new dependency —
  it's already installed and already the established icon system for `sessions/` components
  in this exact area of the codebase. SVG icons from a maintained library are inherently
  more accessible than emoji (consistent sizing/color via `currentColor`, no
  platform/font-rendering variance, straightforward to pair with `aria-label`/`aria-hidden`
  and `<title>` for screen readers) if the parallel UX research agent flags emoji
  accessibility gaps (likely, given emoji glyphs carry inconsistent/absent semantics to
  screen readers and vary visually across OS/browser emoji fonts).
- **Cons**: None of substance — the only cost is a mechanical swap of emoji glyphs for
  `lucide-react` icon components in the unified widget (and, opportunistically, in the
  legacy panels being consolidated: `VcsPanel.tsx`, `VcsStatusDisplay.tsx`,
  `ShipStatusDisplay.tsx`). No new package.json entry is needed.
- **Verdict**: **Recommended — adopt already-installed `lucide-react`** for all new VCS
  status glyphs (branch, PR, CI pass/fail, review state, diff add/remove indicators) in the
  unified widget. **Not recommended: hand-built custom SVG icon set** — there's no gap
  `lucide-react` doesn't already cover for standard git/PR/CI iconography (branch, git-pull-request,
  check/x-circle, git-commit, file-diff icons all exist in Lucide's set), so building bespoke
  SVGs would be pure duplication.

## 5. Diff-stat / file-list algorithms: LLM-generated bespoke code vs. battle-tested library calls

**Frontend** (`parseDiff.ts`): Per-file additions/deletions are computed by the hand-rolled
parser itself while walking hunk lines (`currentFile.additions++` / `currentFile.deletions++`
in `web-app/src/lib/utils/parseDiff.ts:57-63`) — this is bespoke parsing of raw diff text,
not a library call, and has no test coverage located during this research (no
`parseDiff.test.ts` found alongside it in a quick check — recommend the parallel testing/QA
research or implementation phase confirm test coverage exists or add it, since this is
exactly the kind of hand-rolled parsing logic prone to off-by-one and edge-case bugs, e.g.
around "no newline at end of file" markers, binary diffs, or renamed files with no hunk body).

**Backend** (`session/git/diff.go`, `GitWorktree.Diff()`): Diff stats are **not** computed
via `go-git`'s diff/patch API (`object.Patch().Stats()`, which returns typed
`[]FileStat{Name, Addition, Deletion}` per file) despite `go-git` already being a project
dependency and the established preference
(`.claude/rules/prefer-go-git-over-subshells.md`). Instead, `Diff()` shells out to
`git --no-pager diff <baseSHA>` and then hand-counts lines by checking `strings.HasPrefix(line, "+")`
/`"-"` while excluding `+++`/`---` headers (`diff.go:101-108`). This is:
- **Aggregate only** — a single Added/Removed count for the whole diff, not per-file. The
  new durable-snapshot requirement needs **per-file** diff stats, which this existing code
  path does not provide at all — new code is required regardless of build/buy choice here.
- **Fragile in the way hand-parsing usually is**: no `+++`/`---` header context loss beyond
  the exclusion check shown; no handling of binary files (which have no `+`/`-` line
  content but do have a diffstat entry); relies on `git --no-pager diff` output format
  staying stable rather than a structured API.

- **Options for the new per-file stat computation needed for durable snapshots**:
  - **A. Shell out to `git diff --numstat <base>`** and parse the simple
    tab-separated `added\tdeleted\tfilename` output — better than the current raw-diff
    line-counting (handles binary files as `-\t-\tfilename`), but still hand-parsed text.
  - **B. Use go-git's `object.Patch().Stats()`** — returns `[]FileStat` with `Name`,
    `Addition`, `Deletion` fields natively, no text parsing at all, aligns with the
    project's stated go-git preference, and is the correct fix per
    `.claude/rules/prefer-go-git-over-subshells.md` (this is exactly a "resolving a ref +
    diffing two commits" operation go-git is designed for, not one of the documented
    exceptions like live merge or credentialed push/fetch).
- **Verdict**: **Recommended — use go-git's `object.Patch().Stats()` for the new per-file
  diff-stat computation** backing durable snapshots, rather than adding a second hand-parsed
  `git diff --numstat` code path alongside the existing hand-parsed aggregate one. This is
  new bespoke logic either way (nothing existing computes per-file stats today), so the
  correctness risk is in *how* it's built, not whether to build it — go-git's typed API
  removes an entire class of text-parsing bugs (binary files, renames, no-newline markers)
  that a third hand-rolled text parser would otherwise reintroduce. Flag for the planning
  phase: consider also migrating the existing aggregate `Diff()` line-counting in
  `session/git/diff.go` to go-git's stats API while touching this code, since it sits right
  next to the new work and shares the identical correctness risk — but that migration is
  optional cleanup, not required scope for this feature.

## Summary table

| Area | Verdict | Choice |
|---|---|---|
| Diff rendering | Viable (extend) / Recommended (adopt) — conditional | Extend hand-rolled `DiffRenderer` if unified+expand/collapse suffices; adopt `react-diff-view` if UX research confirms split-view/syntax-highlight need |
| GitHub API client | Recommended | Reuse/extend existing hand-rolled REST client pattern (`session/backlog_plugin_github.go`) — no `go-github`/`githubv4` dependency exists or is justified |
| Durable snapshot persistence | Recommended | Build on existing ent schema + `sql/upsert`; not recommended: new datastore (Redis etc.) |
| Icon/badge iconography | Recommended | Adopt already-installed `lucide-react`; not recommended: hand-built custom SVG set |
| Diff-stat computation | Recommended | Use go-git's `object.Patch().Stats()` for new per-file stats; avoid a second hand-parsed text-diff code path |
