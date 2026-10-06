# Research: Feature Landscape — vcs-tab-redesign

Scope note: two sibling agents cover IntelliJ/JetBrains and vim-fugitive deep-dives; a third
already ran a broad state-of-the-art UI survey this session — none of that is repeated here.
This file focuses on: (a) what `VcsWidget` already supports that the live-session tab doesn't
use, (b) the mergeability-state model, (c) domain-specific edge cases for AI-agent-produced
PRs, (d) unstated needs. A predecessor project, `unified-vcs-widget`
(`project_plans/unified-vcs-widget/research/`), designed and shipped the `VcsWidget` component
itself — its `features.md`/`pitfalls.md` are cited below where still relevant, not repeated.

## (a) Already-built-but-unwired: what VcsPanel.tsx leaves on the table

`VcsPanel.tsx` (`web-app/src/components/sessions/VcsPanel.tsx:62-71`) calls
`fromSessionVcs(status, session)` and renders `<VcsWidget mode="full" ... />` with only
`onNavigateToFile` and `onRefresh` wired. Every finding below is a component capability that
already exists and renders correctly given the right data — the gap is upstream, in
`fromSessionVcs()` (`web-app/src/lib/vcs/adapters.ts:83-96`) and in what `GetVCSStatus` returns
from the backend.

1. **Commit list — hardcoded empty, not just unpopulated.**
   `fromSessionVcs()` sets `commits: []` unconditionally
   (`web-app/src/lib/vcs/adapters.ts:92`). `VcsWidgetCommitList.tsx` is fully built — expand/
   collapse per commit, a 20-commit cap in full mode with a "Show all N commits" button
   (`VcsWidgetCommitList.tsx:12-13,39-58`) — but it renders nothing for a live session because
   it's never given commits. This is the single highest-value "unwire" in the redesign, but note
   it's not purely a frontend fix: `VCSStatus` (`proto/session/v1/types.proto:949`) has no
   commits field at all, and `GetVCSStatus` (`server/services/workspace_service.go:132-188`)
   never calls a commit-listing helper. The backend piece already exists and is proven
   elsewhere: `git.ListShippedCommits(repoPath, baseSHA, headSHA)`
   (`session/git/ops.go:369`, capped internally — see §(c).5) is exactly what
   `backlog_service_ship_status.go:131` calls for the historical/ship-status path. Plumbing this
   into the live path means (i) adding a `repeated ShippedCommit commits` (or equivalent) field
   to `VCSStatus`, (ii) resolving a base SHA for a live worktree (merge-base with the session's
   base branch — check `session/git` for an existing helper before adding one), (iii) calling
   `ListShippedCommits` in `GetVCSStatus`, (iv) mapping it in `fromSessionVcs()` via the existing
   `toCommitSummary` pattern already used by `fromShipStatus()` (`adapters.ts:102-109`).

2. **Aggregate diff-stat line — currently compact-mode-only, not a full-mode gap in the
   component, but a full-mode gap in the *product requirement*.** `VcsWidget.tsx:106-112` only
   renders `data.aggregateStats` when `mode === "compact"`; full mode has no equivalent line at
   all today (full mode instead shows the itemized `VcsWidgetFileList`, which already has
   per-file +/- stats). Requirement #2 in requirements.md asks for an aggregate line *in full
   mode specifically* — that's new rendering, not just new data: either add a full-mode
   aggregate summary row above/alongside the file list (sum of `fileChanges` additions/
   deletions, computable client-side, no backend change needed), or extend
   `VcsWidget.tsx`'s conditional to also show `aggregateStats` in full mode when populated.
   Also note: `fromSessionVcs()` never sets `aggregateStats` at all (only
   `fromUnfinishedWorktree()` does, `adapters.ts:214-218`) — so even the compact-mode line
   would currently be blank for a live session; it'd need to be computed from `flattenFileChanges`
   or from git status counts.

3. **Reviewer "changes requested" reason text — fetched from GitHub, then discarded.**
   `github/client.go`'s `ghReviewItem.Body` (`client.go:117-123`) carries the review body text
   from `gh pr view --json reviews`, but `parseReviewCounts()` (`client.go:330-354`) reads only
   `.State` and throws `.Body` away — only a bare int count survives to
   `PRInfo.ChangesRequestedCount` → `Session.github_changes_req_count`
   (`proto/session/v1/types.proto:124`) → `GithubSummary.changesReqCount`. There is no
   `GithubSummary` field for review text at all, and `VcsWidgetGithubRow.tsx` only ever renders
   the count (`VcsWidgetGithubRow.tsx:69-77`), never a reason. Getting requirement #5 means:
   either persist the latest CHANGES_REQUESTED review's body alongside the count (new
   `Session` field + poller change), or fetch it lazily via a new/existing endpoint when the tab
   is open (see open question #2's tradeoff, same shape as itemized checks below).

4. **Itemized CI checks — fetched from GitHub, then collapsed into one string.**
   `github/client.go`'s `ghPRResponse.StatusCheckRollup []ghStatusCheckItem`
   (`client.go:113,126-132`) carries per-check `Name`/`Context`/`State`/`Status`/`Conclusion`
   from `gh pr view --json statusCheckRollup`, but `getCheckConclusion()`
   (`client.go:356-380ish`) immediately collapses the whole slice into a single
   `(conclusion, status string)` pair — the itemized slice is never returned from `GetPRInfo`,
   never reaches `PRInfo`, and is not stored anywhere. Only the single rollup string survives to
   `Session.github_check_conclusion` (`types.proto:127`) → `GithubSummary.checkConclusion` →
   `VcsWidgetGithubRow`'s single `CI: {conclusion}` span (`VcsWidgetGithubRow.tsx:81-83`). This
   directly answers open question #2's framing: the itemized data is a **field GitHub already
   returns to the existing `gh pr view` call** — no new GitHub API surface is needed, only (i)
   keeping `[]ghStatusCheckItem` on `PRInfo` instead of discarding it, (ii) a new repeated field
   on whichever proto message carries it to the frontend, (iii) new UI (a `VcsWidgetCheckList`
   sibling to `VcsWidgetCommitList`). Whether to thread it through the always-on 60s poller
   (adds ~N small strings to every `Session` push, N = check count) or lazy-fetch on tab open
   (extra round-trip, but zero cost to the poller's payload/cadence) is a real tradeoff for
   planning — see §(d) staleness note below for why lazy-fetch may be preferable.

5. **Live "as of" staleness timestamp — the source field already exists on `Session`.**
   `Session.last_pr_status_check` (`types.proto:130`, a `google.protobuf.Timestamp`) is set by
   `PRStatusPoller.applyPRUpdate` but is not present in `GithubSummary`/`VcsWidgetData` at all —
   `VcsWidget.tsx:60-64` only shows a snapshot timestamp for `kind === "historical"`
   (`data.snapshotAt`), which live data never has. `VcsWidgetHeader`/`VcsWidgetGithubRow` have no
   staleness affordance for the live path. Requirement #6 is achievable by adding
   `lastPrStatusCheck` to `GithubSummary`, mapping `session.lastPrStatusCheck` in
   `fromSessionGithub()` (`adapters.ts:68-81`), and rendering it with the same
   `formatRelativeTime` helper `VcsWidget.tsx` already imports (`VcsWidget.tsx:11,62`) —
   almost entirely a wiring task, no new backend field required. Also worth showing local
   VCS-status freshness separately, since it's a different cadence (see §(d) below) — the two
   can legitimately disagree (e.g. GitHub said CI passed 2 minutes ago, but the local dirty-file
   count is 20s fresh).

6. **Ahead/behind vs. base — already rendered, this one is not a gap.**
   `VcsWidgetHeader.tsx:59-64` already renders `↑{aheadOfMain} ahead` / `↓{behindMain} behind`
   whenever either is nonzero, and `fromSessionVcs()` already maps `status.aheadBy`/
   `status.behindBy` (`adapters.ts:89-90`). Requirement #7 is effectively done already; confirm
   during planning rather than re-scoping work here.

7. **Worktree path / browse-files / copy — already full-mode-only and already wired via
   `VcsPanel`'s sibling props**, but `VcsPanel.tsx` never passes `worktreePath` or
   `onBrowseFiles` to `VcsWidget` today (`VcsPanel.tsx:64-69` passes neither). Not asked for by
   name in requirements.md, but since the component already supports it (`VcsWidgetHeader.tsx:
   45,66-88`) and `BacklogItemDetail`'s ship-status view presumably wires it, it's a very cheap
   add-on worth flagging to the user during planning as bundled-for-free scope.

## (b) Mergeability state enumeration (`mergeability.ts`, `deriveMergeabilityState()`)

`MergeabilityState` (`web-app/src/lib/vcs/mergeability.ts:3-13`) has exactly 10 members,
computed by **first-match precedence** (`mergeability.ts:20-35`), not a multi-reason rollup:

| Order | State | Condition |
|---|---|---|
| 1 | `shipped` | `data.shipped === true` (always wins, even over a stale "closed" GitHub state — ADR-003) |
| 2 | `snapshot_unavailable` | historical + `snapshotCaptureFailed === true` (checked before every GitHub branch so it can't be misread as "no PR" or "CI running") |
| 3 | `no_pr` | `!data.github` |
| 4 | `draft` | `github.isDraft` |
| 5 | `conflicted` | any `fileChanges` with `section === "conflict"` (**local** git conflict markers only — see §(c).6) |
| 6 | `changes_requested` | `github.changesReqCount > 0` |
| 7 | `ci_failing` | `github.checkConclusion === "failure"` |
| 8 | `closed_unshipped` | `github.prState === "closed"` |
| 9 | `ci_pending` | `checkConclusion` is `"pending"` or `""` |
| 10 | `ready_to_merge` | fallthrough |

This is the direct input for requirement #4 ("why blocked" rollup showing ALL reasons, GitLab
merge-checks style): today a PR that is simultaneously a draft, has changes requested, AND has
failing CI shows only **"Draft"** (state 4 wins, states 6-7 never surface). A rollup design
needs to evaluate each of these 7 non-terminal predicates (3-9) independently rather than
short-circuiting, and decide how the single-pill summary (kept for compact mode / at-a-glance
scanning) coexists with a full itemized list (full mode only, per requirements' "richer view in
full mode" framing). Note state 5 (`conflicted`) is a legacy source-mismatch risk in its own
right — see §(c).6.

## (c) Domain-specific edge cases (AI-agent sessions producing PRs)

1. **Session's branch force-pushed by the agent mid-session.** Nothing in `VCSStatus` or
   `GithubSummary` detects a rewritten history — `aheadBy`/`behindBy` and any future commit list
   are computed fresh on every `GetVCSStatus`/`ListShippedCommits` call against the *current*
   HEAD, so a force-push just produces a new correct answer on the next poll — there's no
   "stale SHA" bug here structurally, because nothing is cached across the rewrite (the 15s
   `vcsStatusCacheTTL` in `workspace_service.go:60` self-invalidates well within any human-visible
   window). The one real risk: if requirement #1's commit list is added by resolving a
   **merge-base SHA once and reusing it**, a force-push that changes the merge-base (e.g. agent
   rebases onto a moving base branch) must re-resolve the merge-base every call, not cache it —
   mirror `ListShippedCommits`'s existing base..head contract (recomputed per call, no stored
   base) rather than introducing a new cached "base SHA" that could go stale across a rewrite.
   Recommend: no special-case force-push handling needed if commit list continues to be computed
   live per request; call this out explicitly in the plan so a reviewer doesn't ask for a
   "detect rewritten history" feature that isn't actually necessary.

2. **PR closed-without-merge while the session is still "active."** Already correctly
   distinguished at the mergeability layer: `deriveMergeabilityState()` checks `data.shipped`
   (git-ancestry-derived, `IsCommitOnMain`) before `prState === "closed"`
   (`mergeability.ts:21,30`), so a closed-but-actually-merged-via-squash PR still shows
   `shipped`, and a genuinely abandoned PR shows `closed_unshipped`, not the historical
   "Merged" mislabel the predecessor project's `pitfalls.md` flagged for `GitHubBadge`'s
   `priorityLabel()` (`github/priority.go:29-31` — that bug is in a *different* component, the
   session-list `GitHubBadge`, not in `VcsWidget`; confirm it's genuinely out of scope for this
   redesign or note it as adjacent debt). What's missing for a **still-active** session in this
   state: nothing in the UI currently says "this session is still running/dirty against a PR
   GitHub considers closed" — that's a distinct fact from "was it shipped." Recommend the
   redesign surface both facts side by side (mergeability pill for PR/ship state, dirty-status
   row for local activity) rather than conflating "session activity" into the PR-derived pill,
   which is already the existing layout's separation of concerns (`VcsWidgetHeader` vs.
   `VcsWidgetGithubRow`/`MergeabilityPill`) — just confirm it holds up when closed_unshipped
   + still-dirty co-occur, since no existing test exercises that combination
   (`MergeabilityPill.test.tsx` and `VcsWidgetGithubRow.test.tsx` both test states in isolation).

3. **Concurrent sessions on the same repo.** Low risk by construction, not zero: `GetVCSStatus`
   keys its 15s cache by `workDir` = `instance.Workspace().EffectivePath`
   (`workspace_service.go:145,153`), and every session gets its own git worktree
   (`~/.stapler-squad/worktrees/`), so two sessions never share a cache entry or read each
   other's working directory. The residual risk is at the **GitHub PR** layer, not the local
   layer: two sessions could plausibly push to branches that both open PRs against the same
   base, or (more concerning) two work sessions could be linked to the *same* PR number (e.g. a
   rework session continuing an existing PR) — `PRStatusPoller` polls per-`Instance`
   (`session/pr_status_poller.go:204` `checkAllSessions`), so each session's `Session.github_*`
   fields are independently fetched and could, in principle, show identical PR data twice
   without anything in the UI clarifying "this is the same PR another session is also tracking."
   Not urgent to solve in this redesign (no evidence today's users are confused by it), but worth
   a one-line note in the plan's out-of-scope section rather than silently ignoring it.

4. **Very large diffs (100+ files) from a long-running session, interacting with the commit-list
   cap.** Two independent caps already exist and don't currently interact because commit list is
   empty for live sessions (finding (a).1): `VcsWidgetFileList`'s per-section cap is 20
   (`VcsWidgetFileList.tsx:44`, "Show all N files" button, same pattern as commit list) and
   `session/git/ops.go:358` documents a `listShippedCommitsCap` bounding `ListShippedCommits`
   itself (not just a UI-side slice — the backend caps before the data ever reaches the wire).
   Once commit list is wired up (finding (a).1), a session with >100 shipped commits will see
   the backend cap silently truncate rather than the frontend's "Show all" affordance kicking in
   — **the two caps are inconsistent in kind**: file list caps client-side (data is all there,
   UI shows a subset with an escape hatch), commit list would cap server-side (data past the cap
   never arrives, no client-side "show all" is possible). Recommend the plan either raise/remove
   the backend cap for the live-session path specifically (historical ship-status has less need
   for "all commits" since it's a closed chapter; live in-progress work benefits more from seeing
   the full list) or add explicit truncation messaging ("showing first 100 of N+ commits") so a
   hard cutoff doesn't read as "that's all the commits" when it isn't.

5. **Local uncommitted changes AND an open PR simultaneously.** Already correctly handled by
   the existing layout, not a gap: `VcsWidgetHeader` always renders clean/dirty independent of
   `github` state (`VcsWidgetHeader.tsx:53-55`), and `VcsWidgetGithubRow` renders independent of
   `isClean` (`VcsWidgetGithubRow.tsx:33-90` never reads `data.isClean`). Both already show
   simultaneously today in `VcsPanel`'s full mode. The one thing to verify during
   implementation/testing rather than design: that this combination gets an explicit
   `VcsWidgetHeader`/`VcsWidgetGithubRow` interaction test, since — per finding above — no
   existing test in either component's `.test.tsx` exercises dirty+PR-present together.

6. **GitHub-side merge conflict vs. local git conflict markers — two different "conflicted"
   concepts, only one is wired anywhere.** `deriveMergeabilityState()`'s `conflicted` state
   (`mergeability.ts:27`) fires only from `fileChanges` sections tagged `"conflict"` — i.e. an
   in-progress local merge/rebase with unresolved markers in the working tree. GitHub's own
   mergeability signal — "this PR can't be merged because the base branch has diverged," which
   the agent's local worktree may not even know about if it hasn't fetched/rebased — is a
   **separate fact**, `PRInfo.Mergeable` (`"mergeable"`/`"conflicting"`/`"unknown"`,
   `proto/session/v1/types.proto:766-767`, populated via `GetPRInfo`, `client.go:64,116`). This
   field is fetched by the `GetPRInfo` RPC (used by `PullRequestSection.tsx` for backlog items,
   per the grep in `docs/registry/features/...`) but is **not** part of the `Session` poller's
   fields (`types.proto:84-140` has no `mergeable` field) and has no `GithubSummary` equivalent.
   A PR can be `ready_to_merge` by today's local-only logic while GitHub itself reports it
   unmergeable due to base drift — exactly the class of bug requirement #4's "why blocked"
   rollup exists to close. Recommend adding GitHub's `mergeable` signal as its own rollup entry,
   distinct from the local `conflict` file-section signal, both surfaced under the "why blocked"
   umbrella rather than merged into one boolean.

7. **A session with no PR yet.** Confirmed empirically: `VcsWidgetGithubRow` returns `null` when
   `!data.github && !captureFailed` (`VcsWidgetGithubRow.tsx:36`) — for a live session with no
   PR, `captureFailed` is always `false` (that flag only exists on `kind: "historical"` data), so
   the row renders nothing at all, and `MergeabilityPill` shows the generic `"No PR"` state. Combined
   with finding (a).1 (commits always empty), **a session with local commits but no PR yet
   currently shows almost nothing useful**: branch name, clean/dirty, ahead/behind counts, and a
   "No PR" pill — no commit list, no diff stat. This directly motivates open question #4's
   IntelliJ-Local-Changes-style framing: once commit list (a).1 and a full-mode aggregate stat
   line (a).2 are wired, a no-PR session becomes genuinely informative without needing any
   GitHub data at all — this is likely the single highest-leverage slice for a session that just
   started (every session begins in this state before its first PR).

## (d) Unstated user needs (inferred)

- **Two independent staleness clocks, not one.** Local VCS status is cache-bounded at 15s
  server-side (`vcsStatusCacheTTL`, `workspace_service.go:60`) and fetched event-driven (Redux
  `watchSessions` push) plus a 60s client-side fallback poll only while the tab is visible
  (`useSessionVcs.ts:142-155`, `!document.hidden` guard at line 151). GitHub PR/CI/review data
  is refreshed independently by `PRStatusPoller` on a fixed **60s** server-side interval
  (`session/pr_status_poller.go:41` `DefaultPRStatusPollerConfig`), decoupled from any specific
  session's activity. A single "as of" timestamp (finding (a).5) would conflate two different
  freshness guarantees; consider labeling them separately ("local status: live" vs. "PR status:
  as of 47s ago") rather than one blended timestamp, since local status is near-real-time and PR
  status genuinely lags up to a minute.
- **Lazy-fetch is the better default for itemized CI checks and review-reason text**, not
  threading them through the always-on poller. The poller already pushes a `Session` update on
  every 60s tick for every session with a PR (`pr_status_poller.go:204` `checkAllSessions`
  iterates all instances) — adding a variable-length itemized-checks array and review-body
  strings to that payload multiplies its size by however many checks/PRs a repo has, for every
  session, every 60s, even when no user is looking at that session's VCS tab. Given
  `VcsWidgetCommitList`'s and `VcsWidgetFileList`'s existing "cap + explicit fetch-more/show-all"
  pattern, a tab-open lazy-fetch (new RPC or extending `GetPRInfo`) for itemized checks matches
  the codebase's existing bandwidth-consciousness (see `github/etag_cache.go`,
  `github/rate_limit.go` cited in the predecessor project's `pitfalls.md` §3) better than
  growing the steady-state poller payload. This directly informs open question #2.
- **The redesign should reuse `GetPRInfo`'s already-fetched `Mergeable`/`StatusCheckRollup`/
  `Reviews[].Body` rather than adding new GitHub API calls** — every one of findings (a).3,
  (a).4, and (c).6 above is data the backend already round-trips to GitHub in a single `gh pr
  view --json ...` call and then throws away before it reaches the frontend. The redesign's
  backend work is overwhelmingly "stop discarding fields already in hand," not "call GitHub
  more."
- **`GetPRComments` (open question #1) is fully implemented end-to-end already** — RPC defined
  (`proto/session/v1/session.proto:84,1243-1251`), handler implemented
  (`server/services/github_service.go:90-120+`, calls `instance.GetPRComments()`), but grepping
  the entire `web-app/src` tree found zero callers outside generated code
  (`web-app/src/gen/`) and `web-app/src/lib/features/features/pr.ts` (a feature-registry
  marker file, not a UI consumer). Including inline PR comments in this redesign's scope is a
  pure frontend task with no backend work required — the risk/cost calculus for open question #1
  is almost entirely "how much UI space does a comment thread deserve in an already-dense tab,"
  not feasibility.

## Sources consulted

- `web-app/src/components/shared/VcsWidget.tsx` (whole file)
- `web-app/src/components/shared/vcs-widget/{VcsWidgetHeader,VcsWidgetGithubRow,VcsWidgetCommitList,VcsWidgetFileList,MergeabilityPill}.tsx` (whole files) and their `.test.tsx` siblings
- `web-app/src/lib/vcs/{adapters,mergeability,types}.ts` (whole files)
- `web-app/src/components/sessions/VcsPanel.tsx` (whole file)
- `web-app/src/lib/hooks/useSessionVcs.ts` (polling/caching behavior, lines 1-160)
- `server/services/workspace_service.go:60,132-188` (`GetVCSStatus`, cache TTL)
- `session/pr_status_poller.go:25-41,204` (poll cadence, per-instance loop)
- `session/git/ops.go:358-369` (`ListShippedCommits`, cap)
- `server/services/backlog_service_ship_status.go:131` (existing `ListShippedCommits` call site)
- `github/client.go:60-132,280-380` (`PRInfo`, `ghPRResponse`, `ghStatusCheckItem`, `ghReviewItem`, `getCheckConclusion`, `parseReviewCounts`)
- `github/priority.go` (`DerivePRPriority`, closed/merged conflation — different component, noted as adjacent)
- `proto/session/v1/types.proto:84-140` (`Session` github_* fields), `:740-777` (`PRInfo` message incl. `mergeable`), `:949-990+` (`VCSStatus`)
- `proto/session/v1/session.proto:84,1243-1251` (`GetPRComments` RPC)
- `server/services/github_service.go:90-120` (`GetPRComments` handler)
- `project_plans/unified-vcs-widget/research/{features,pitfalls}.md` (predecessor project — component design precedent, not re-derived here)
- `project_plans/pr-comment-check-runs/requirements.md` (confirms itemized check-run reads already exist in `session/backlog_plugin_github_prs.go`; that project is about *reducing comment noise*, unrelated to this one's display concern, cited only for the "already reads Check Runs when polling CI" confirmation)
