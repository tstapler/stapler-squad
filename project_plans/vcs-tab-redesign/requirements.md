# Requirements: vcs-tab-redesign

**Date**: 2026-08-27
**Type**: feature addition (redesign of an existing, underbuilt UI surface)

## Problem Statement

The VCS tab in a session's detail view (one of Terminal/Diff/VCS/Files/Logs/Info/Browser/Artifacts)
is the place a user checks "where does this session's work stand, git-wise" without leaving the
session. Today it renders almost nothing useful: a PR number + collapsed CI badge, a
closed/merged badge, the branch name, and a bare "Clean"/dirty label. There is no commit history,
no diff stats, no itemized CI checks, no reviewer feedback, and no explanation of *why* a PR is
blocked from merging. A user managing several concurrent agent sessions has to leave the app and
open GitHub to answer basic questions the app already has the data to answer.

This is a whitespace gap relative to the rest of the product: `VcsWidget.tsx` and its
subcomponents are already a rich, capable component — richer views already exist for backlog
ship-status contexts (`compact` mode via `fromShipStatus`/`fromUnfinishedWorktree`) — but the
live-session tab renders a degraded subset of what the component and backend already support.
Confirmed by reading the code, not just observing the UI: `fromSessionVcs()`
(`web-app/src/lib/vcs/adapters.ts:83`) hardcodes `commits: []` and never populates
`aggregateStats`, and `VcsWidget.tsx:106` only renders the aggregate diff-stat line when
`mode === "compact"` — never in `"full"` mode, which is what the session tab actually uses.

## Users / Consumers

- End users: the app's primary user (solo/small-team developers driving AI coding agents through
  stapler-squad), viewing a live session's VCS tab while a session is running or after it's
  produced a PR.
- Indirectly: the same VcsWidget component is reused in backlog ship-status views, so any
  shared-component change must not regress that surface.

## Success Metrics

- The VCS tab surfaces, without leaving the app: commit history for the session's branch, an
  aggregate diff-stat line, itemized (not collapsed) CI check results, and an itemized "why is
  this blocked from merging" rollup (not a single top-precedence pill).
- A user can answer "is this session's PR ready to merge, and if not, exactly why" entirely from
  the VCS tab.
- No regression to the existing `compact`-mode VcsWidget usage on backlog ship-status views.
- Subjectively: the tab should read as "world-class" against the state-of-the-art comparison set
  established in Phase 2 research (VS Code/GitLens, JetBrains VCS tooling, GitLab merge-checks,
  GitHub Desktop, Graphite, vim-fugitive) — the two the user asked to be researched in depth are
  IntelliJ/JetBrains VCS tooling and vim-fugitive.

## Constraints

- No hard deadline given.
- Must reuse the existing `VcsWidget.tsx` / `vcs-widget/` component family rather than building a
  parallel one — the component already supports the richer layout in `compact` mode; the fix is
  largely in wiring `fromSessionVcs()` and `"full"` mode up to feature parity, not a rewrite.
- Must not regress the `compact`-mode usage on backlog ship-status views (`fromShipStatus`,
  `fromUnfinishedWorktree` in `web-app/src/lib/vcs/adapters.ts`).
- Backend data that's already computed but currently discarded should be surfaced rather than
  recomputed where possible (see the specific discard points cataloged below) — avoid redundant
  GitHub API calls or redundant go-git traversals where an existing call already has the data.
- Must follow this repo's standing conventions: `prefer-go-git-over-subshells`,
  `interface-pollution-checklist`, `primitive-obsession-checklist`, mobile+desktop UX
  consideration (per user global memory), and the proto-gen workflow (`make proto-gen`) if the
  `VCSStatus` proto needs new fields.

## Scope

### In Scope

Candidate feature list (ranked by the state-of-the-art synthesis already done this session;
Phase 2/3 may reprioritize based on the IntelliJ/Fugitive deep-dives):

1. Commit list (branch vs. base) in the live-session VCS tab — via `ListShippedCommits`
   (`session/git/ops.go:369`), not currently called from any live-session path.
2. Aggregate diff-stat line rendered in `"full"` mode, not just `"compact"` — requires both a
   data source (a `DiffShortstat`-equivalent for the live session) and fixing the `mode ===
   "compact"` gate in `VcsWidget.tsx:106`.
3. Itemized CI checks (name/context/state/conclusion per check), not one collapsed
   conclusion — `github/client.go:113`'s `ghStatusCheckItem` slice is already fetched per PR and
   currently discarded by `getCheckConclusion()` (`github/client.go:300`).
4. A "why is this blocked" rollup showing all blocking reasons at once (GitLab merge-checks
   style), replacing/augmenting the single top-precedence pill from
   `deriveMergeabilityState()` (`web-app/src/lib/vcs/mergeability.ts`).
5. Reviewer's stated "changes requested" reason text — `ghReviewItem.Body`
   (`github/client.go:116-123`) is fetched via `parseReviewCounts` (`github/client.go:330`) but
   only `Author`/`State` are read; `Body` is discarded.
6. A live "as of" staleness/freshness timestamp for the panel (an equivalent pattern already
   exists for historical/backlog snapshots; needs a live-session equivalent).
7. Anything specifically worth adopting from the IntelliJ VCS tooling and vim-fugitive deep-dives
   (Phase 2) that fits a read-mostly status panel — e.g. ahead/behind-vs-base display (data
   already available via `gogit_vcs_reader.go`'s `AheadBehind`, currently only wired to the
   backlog "unfinished worktree" scanner path, not live sessions), and Fugitive-inspired
   information density (a single scrollable view with collapsible sections) over additional
   chrome.

### Out of Scope

- Any mutating git actions (stage/unstage/commit/stash/rebase/merge) — this is a read-status tab,
  not a full git client. Mutating actions belong in the Terminal tab. (User-confirmed scoping
  from earlier in this conversation.)
- Multi-PR / stacked-PR visualization (Graphite-style) — no PR-stack concept exists anywhere in
  this app yet; introducing one is a separate, larger feature.
- Inline diff viewing/annotate/blame gutters — the Diff tab is the existing dedicated surface for
  that; VCS tab stays a status/summary panel, not a code viewer.
- Any change to the `compact`-mode ship-status views' visual design (only avoid regressing them).

## Open Questions

These are genuine open questions for the user — flagging rather than guessing:

1. **PR comments surfacing**: `GetPRComments` (`github/client.go:556`, RPC at
   `server/services/github_service.go:90`) is a working, already-wired RPC for PR review
   comments, but it's not currently called from the VCS tab. Should inline PR review comments
   (not just the review-body text from item 5 above) be part of this redesign's scope, or is
   that a separate follow-up? Tentatively treating as in-scope-if-cheap given the "world-class"
   bar, but flagging since it wasn't in the explicit ranked list.
2. **Itemized CI checks data source for the live tab**: `getCheckConclusion()` collapses the
   itemized rollup today. Does surfacing itemized checks require adding a field to `PRInfo`
   (`github/client.go:50-76`) and threading it through `UpdatePRStatus`
   (`session/instance_terminal.go:404`) and the `VCSStatus` proto
   (`proto/session/v1/types.proto:949`), or is there a cheaper path (e.g. a separate on-demand
   RPC fetched only when the VCS tab is open, avoiding bloating the poller's payload)? This is a
   design decision for Phase 3, but the user should confirm whether poller-payload growth is an
   acceptable tradeoff for "always fresh" itemized checks, versus a lazy/on-open fetch.
3. **"As of" live staleness timestamp**: what's the actual polling cadence of
   `pr_status_poller.go`'s update loop? Needed to decide whether a staleness indicator is
   meaningful (i.e., if polling is already sub-minute, a timestamp may be noise) — Phase 2
   research should pull this so Phase 3 can decide.
4. **Local-only sessions with no PR yet**: for a session that hasn't opened a PR, the "why
   blocked" rollup and itemized CI have nothing to show. Should the tab instead emphasize local
   commit history + ahead/behind-vs-base + dirty-file summary in that state (mirroring IntelliJ's
   Local Changes view), and is that considered explicitly in scope here, or deferred? Leaning
   in-scope since `ListShippedCommits`/`AheadBehind`/dirty-status don't require a PR to exist.
5. **Mobile layout**: per standing UX guidance, the redesigned panel must work on mobile touch
   targets and narrow viewports. Should IntelliJ/Fugitive-style information density be trimmed
   further on mobile (e.g. collapsed sections default-closed) versus desktop (default-open)? No
   existing precedent in this codebase to point to — flagging as a Phase 3 design decision.
