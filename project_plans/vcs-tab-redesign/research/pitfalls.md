# Pitfalls: vcs-tab-redesign

Research for Phase 2. Each item: what could go wrong, why (with citation), and a concrete
mitigation for Phase 3.

## 1. GitHub API rate limits and cost

**Finding — the data for scope items 3–5 is mostly already fetched, not a new call.**
`github.GetPRInfoConditional` (`github/etag_cache.go:53`) does a native-HTTP, ETag-cached
GET on `repos/{owner}/{repo}/pulls/{number}`. Only when that returns 200 (changed) does it
call `GetPRInfoCtx` (`github/client.go:268`), which shells to
`gh pr view --json ...,reviews,reviewDecision,statusCheckRollup` — i.e. `ghReviewItem.Body`
and `ghStatusCheckItem` are **already in the response payload today**; `parseReviewCounts`
(client.go:330) and `getCheckConclusion` (client.go:357) just discard everything but a
count/rollup. Surfacing them in the VCS tab is a parsing/plumbing change, not a new API call.

**Risk — the "why blocked" rollup and itemized checks can go stale silently.**
The conditional fetch's ETag is scoped to the PR *resource* (`pulls/{number}`), not to its
check-runs or reviews sub-resources. GitHub's own `mergeable`/`mergeable_state` computation is
asynchronous and known to get stuck (`null` or stale `blocked`) independent of the PR
resource's own change timestamp — see
[community discussion #126484](https://github.com/orgs/community/discussions/126484) and
[#73849](https://github.com/orgs/community/discussions/73849). If a status check completes or
a review is posted without the PR resource's ETag changing, `fetchAndUpdatePRStatus`
(`session/pr_status_poller.go:346`) takes the 304 path, bumps `LastPRStatusCheck`, and returns
— the itemized checks/review body shown in the tab are silently the last-fetched snapshot,
while the UI's "as of" timestamp (scope item 6) claims to be current.
**Mitigation:** don't let the "as of" timestamp in the UI imply payload freshness beyond what
was actually re-fetched — distinguish "poller confirmed unchanged at T" from "check/review
data last updated at T". If item 6 needs true itemized-check freshness, the poller needs an
explicit periodic full-refetch (bypassing the ETag 304 path) on a longer interval, not just
relying on the PR-resource ETag as a freshness proxy.

**Risk — concurrent sessions each polling their own PR, but this is already bounded.**
`PRStatusPollerConfig.ConcurrentFetches` (default 5, `session/pr_status_poller.go:42`) caps
simultaneous `gh`/HTTP calls workspace-wide, `checkAllSessions` (line 204) bails the whole tick
early if `github.DefaultRateLimiter.IsLimited()`, and `handleFetchError` (line 365) parses
403/429 responses into rate-limit backoff. This existing machinery already covers the
redesign's added fields since no new endpoint is introduced. **Mitigation:** Phase 3 should
explicitly *not* add a second poller or a lazy on-tab-open fetch path that bypasses
`ConcurrentFetches`/`DefaultRateLimiter` — route any new fetch need through the existing
poller and `ETagCache` (`p.ETagCache()`, poller.go:107) rather than a fresh ad hoc call.

## 2. go-git performance/safety pitfalls

**Finding — `session/git/ops.go`'s VCS-tab-relevant functions (`ListShippedCommits:369`,
`BranchAheadBehind:203`, `BehindOriginMain:249`) each call `git.PlainOpenWithOptions` fresh
per invocation, with no per-repo mutex, no caching, and — unlike `FetchBranch` (ops.go:23,
30s timeout) — no `context.Context`/timeout parameter at all.** This matters because
`WorkspaceService.GetVCSStatus` (`server/services/workspace_service.go:132`) calls
`provider.GetStatus()` synchronously with no context propagated into the git layer and no
per-call deadline — only a short-TTL cache (`vcsStatusCacheTTL`, workspace_service.go:153-160)
sits in front of it. A slow git op has nothing to bound it once the cache misses.

**Risk — repo mid-operation (agent actively committing while the VCS tab polls).**
This is a normal, expected condition here: each session is an agent writing/committing files
in its own worktree while the web UI concurrently polls VCS status. `.claude/rules/
prefer-go-git-over-subshells.md` already documents this exact failure mode for `go-git`: "a
torn-read race on ref files that the git CLI's atomic-rename ref updates don't hit," which is
why `getHeadCommitSHA` (`session/git/util.go`) has a documented CLI fallback
(`getHeadCommitSHAViaCLI`) for that specific case. `ListShippedCommits`/`BranchAheadBehind`
have no equivalent fallback — a ref update landing mid-read during `repo.Reference(...)` or
`repo.CommitObject(...)` can surface as a transient error or (worse) a stale read, with no
retry/backoff. The `code-go-git` skill additionally flags (item 12) that `wt.Status()` is
"pathologically slow on repos with large numbers of untracked files" (go-git issue #181, open
since 2020) — not directly used by these three functions, but a real risk if Phase 3 wires in
`DiffShortstat`/working-tree status alongside them.
**Mitigation:** (a) add a `context.Context` parameter with a bounded timeout to
`ListShippedCommits`/`BranchAheadBehind`/`BehindOriginMain` (matching `FetchBranch`'s
pattern), and thread it from `GetVCSStatus`'s request `ctx`; (b) either accept "transient error
during an active commit, retry next poll" (already true for scope item 1's commit list, since
a fresh `git.PlainOpenWithOptions` per call means no crash — see below — just a possible error)
and surface a graceful "updating…" state rather than an error banner, or add the same
CLI-fallback pattern `getHeadCommitSHAViaCLI` uses for the specific ref-read race.

**Note — the MUST-FIX concurrent-map crash (`code-go-git` skill item 2, go-git issue #1121)
does not directly apply here** because each of these functions opens its own fresh
`*git.Repository` per call rather than sharing/caching one across goroutines — that specific
crash requires concurrent calls on the *same* `*git.Repository` object. It becomes a live risk
only if Phase 3 introduces a cached/shared `*git.Repository` per repo path (e.g. to cut
`PlainOpen` I/O cost across the new commit-list + ahead/behind + diff-stat calls) — if so, it
must follow the skill's per-repo `sync.Mutex` (not `RWMutex`) covering the full call+iterator
lifetime, matching the pattern `session/unfinished/gogit_vcs_reader.go` (the backlog scanner's
`GoGitVCSReader`) already implements with singleflight + TTL caching
(`TestGoGitVCSReader_AheadBehind_SingleflightCollapsesParallelCallers`,
`gogit_vcs_reader_limits_test.go:268`).

**Architecture flag — two parallel git-status implementations exist.** `session/git/ops.go`
(simple, no caching, used by the live session VCS poll path) and
`session/unfinished/gogit_vcs_reader.go`'s `GoGitVCSReader` (used today only by the backlog
scanner per requirements.md's scope item 7) implement overlapping ahead/behind and diff-stat
logic, with the latter already having the caching/singleflight/gitignore-walk-cap protections
the former lacks. **Mitigation:** if Phase 3 wires ahead/behind into the live VCS tab, prefer
extending/reusing `GoGitVCSReader`'s already-hardened implementation over adding a second,
unguarded copy in `ops.go` — re-implementing it in `ops.go` would reintroduce exactly the
risks `GoGitVCSReader` was built to avoid.

`ListShippedCommits` is bounded by `listShippedCommitsCap = 100` (ops.go:361) but note this
caps *appended* commits only — ancestor commits skipped via `continue` (ops.go:394-396) don't
count against the cap, and each candidate calls `c.IsAncestor(base)` (ops.go:390), an ancestor
walk per commit. On a branch with many merge commits from upstream, this could still do
meaningfully more work than "100 commits' worth" before the cap trips. Low severity given the
cap exists at all, but worth a benchmark on a repo with a merge-heavy history before shipping.

## 3. Proto/schema evolution pitfalls

**Finding — `ahead_by`/`behind_by` (scope item 7) need no new proto fields.**
`VCSStatus` (`proto/session/v1/types.proto:949`) already has `ahead_by = 5` and
`behind_by = 6` — they're just never populated for live sessions today (per requirements.md,
wired only to the backlog scanner). This shrinks the proto-evolution surface to: a new
`commits` repeated field (scope item 1), an `aggregate_stats` message (item 2), itemized CI
checks (item 3), a multi-reason "why blocked" list (item 4), reviewer body text (item 5), and
a staleness timestamp (item 6) — all genuinely new fields.

**Finding — an additive-only, defensive-parsing precedent already exists in this exact
adapter file.** `web-app/src/lib/vcs/adapters.ts`'s historical-snapshot path (~line 128-182)
already handles a proto message with optional/absent fields gracefully:
`commitsOrLastCommitFallback` (line 157) falls back to a synthesized single-commit summary
when `status.commits` is empty, and `hasSnapshot = status.snapshotAt != null` (line 164) gates
on presence rather than assuming the field exists. **Mitigation:** follow this same pattern for
the new live-session fields in `fromSessionVcs` (adapters.ts:83) — treat every new proto field
as optional/absent-capable (an old cached `Session` snapshot from before this feature shipped,
or a paused/resumed session, will not have populated `commits`/CI-check items/review bodies),
default to today's empty-array/collapsed-badge behavior rather than throwing or rendering
"undefined," and add an adapter test mirroring the existing historical-snapshot fallback tests
(`web-app/src/lib/vcs/adapters.test.ts`) for "old snapshot, new fields absent."

**Gap — no dedicated proto-conventions doc found.** Searched `.claude/docs/*.md` and
`docs/**/*.md` for an explicit "proto fields are additive-only, never renumber/remove" writeup;
none exists as a standalone doc (only scattered ADR mentions of "backward compatible" for
unrelated serialization concerns, e.g. `docs/adr/007-enum-based-state-transitions.md`). The
convention is implicit in protobuf wire-format semantics (new field numbers, never reuse/
renumber, mark deprecated fields `reserved` rather than deleting) but isn't written down for
this repo. **Mitigation:** Phase 3's plan.md should state the field-numbering rule explicitly
(next available field number after 16 in `VCSStatus`/`FileChange`, or wherever the new message
lands) since there's no doc to defer to, and run `make proto-gen` per the standard flow in
CLAUDE.md (`gen/` is gitignored, regenerated by every build target).

## 4. Shared-component regression risk

**Finding — `VcsWidget.test.tsx` already pins the compact-mode contract Phase 3 must not
break.** Relevant existing tests (`web-app/src/components/shared/VcsWidget.test.tsx`):
`"compact mode never renders per-file rows even when fileChanges is populated"` (line 91),
`"compact mode omits VcsWidgetGithubRow but shows the aggregate stat line"` (line 114),
`"caps compact-mode commit list at 5 entries"` (line 138), `"omits the View Diff affordance in
compact mode even when onViewDiff is provided"` (line 223), and
`"VcsWidget_should_OmitBrowseFilesButton_When_CompactModeEvenWithOnBrowseFiles"` (line 246).
These are exactly the guardrails the requirements doc's "must not regress compact-mode
ship-status views" constraint needs — they already exist and should stay green through Phase 3,
not be loosened to accommodate the full-mode change.

**Risk — `VcsWidgetCommitList` is shared and about to render real data in full mode for the
first time.** `VcsWidgetCommitList` (`web-app/src/components/shared/vcs-widget/
VcsWidgetCommitList.tsx`) already accepts `mode="full"` in its own tests
(`VcsWidgetCommitList.test.tsx:32,50,59`) and is rendered unconditionally in `VcsWidget.tsx`
(`<VcsWidgetCommitList commits={data.commits} mode={mode} />`), but since `fromSessionVcs`
hardcodes `commits: []` today, full mode has *never* actually exercised this component with
non-empty data in production. Once scope item 1 populates real commits, full mode starts
exercising code paths (long summary truncation, many-commits rendering, `mode="full"`'s no-cap
behavior vs. compact's 5-item cap) that only had synthetic/empty-array coverage before.
**Mitigation:** add full-mode-specific `VcsWidgetCommitList` tests with realistic data (long
commit messages, >20 commits) before wiring `ListShippedCommits` output through — the
component's compact-mode tests are a poor proxy for full-mode behavior on real data.

**Risk — the aggregate-stat gate at `VcsWidget.tsx:106` is a straightforward mode-gated
branch, low collision risk.** `{mode === "compact" && data.aggregateStats && (...)}` (line
106) only renders in compact mode; adding a sibling `{mode === "full" && data.aggregateStats
&& (...)}` block is additive and shouldn't touch the compact branch. The higher-risk items are
the components that are *literally shared* between modes (`VcsWidgetCommitList`,
`VcsWidgetHeader`), not this mode-gated JSX split.

## 5. "Why is this blocked" rollup UIs — known pitfalls (external)

- **Async/stuck `mergeable_state`:** GitHub's mergeability computation is a background job;
  the REST `mergeable`/`mergeable_state` field can return `null` transiently after a push and
  is known to get stuck on `blocked` even when the PR is actually mergeable, "with no reliable
  way to unstick the PR once it gets into this state" — [community discussion #126484](https://github.com/orgs/community/discussions/126484),
  [#73849](https://github.com/orgs/community/discussions/73849). `deriveMergeabilityState()`
  (`web-app/src/lib/vcs/mergeability.ts`) consumes `PRInfo.Mergeable` directly from this same
  field — a "why blocked" rollup built on top of it inherits this staleness risk and should not
  present a stuck `blocked` value with the same confidence as a definitively failing check.
- **No backend→frontend push for mergeability changes:** GitHub's own PR page has no reliable
  notification when mergeability changes, forcing polling and producing visibly stale UI until
  a manual refresh — [community discussion #183989](https://github.com/orgs/community/discussions/183989).
  This validates the existing poll+ETag design here over trying to add a push mechanism, but
  reinforces mitigation #1 above: the "as of" timestamp (scope item 6) must reflect *actual*
  last-fetch time, not last-poll-attempt time, or it inherits this exact confusion.
- **Stale-but-green CI treated as still valid:** teams have hit real incidents where GitHub's
  required-checks accept a CI result that passed "months old" against a since-diverged branch,
  because there's no staleness threshold on a passing check by default — documented in
  [Mixpanel Engineering's writeup](https://medium.com/mixpaneleng/enforcing-required-checks-on-conditional-ci-jobs-in-a-github-monorepo-8d4949694340).
  Directly relevant to scope item 3 (itemized CI checks): each check item should show *when*
  it last ran/concluded, not just its conclusion, so a stale-green check doesn't read as
  "currently passing."

## 6. Reviewer body text — untrusted/PII-adjacent content

**Finding — React's default JSX text rendering already escapes this safely; no new sanitizer
needed if rendered as plain text.** `ghReviewItem.Body` (`github/client.go:122`) is free text a
human reviewer typed on GitHub — arbitrary Markdown/HTML-looking strings, but rendering it via
`{reviewBody}` in JSX auto-escapes and is XSS-safe by default.

**Risk — a `dangerouslySetInnerHTML` pattern already exists nearby and could be miscopied.**
`SessionCard.tsx:993` renders terminal-preview output via
`dangerouslySetInnerHTML={{ __html: snapshotHtml }}`, explicitly commented `"Safe: content is
rendered by ansi-to-html with escapeXML enabled, or escaped manually in the plain-text
fallback path"` — i.e. that specific call site is safe *because of* a deliberate
escapeXML-enabled rendering step upstream, not because `dangerouslySetInnerHTML` itself is
safe. If a future engineer wants to render reviewer body text with Markdown formatting (bold,
code blocks — reviewers commonly use both) and reaches for this same pattern without the
escaping step, that reintroduces stored-XSS risk from arbitrary GitHub review text.
**Mitigation:** if Phase 3 wants any Markdown rendering of review bodies, use a
sanitizing Markdown renderer (e.g. one that strips raw HTML) rather than
`dangerouslySetInnerHTML` on the raw body; if plain text is sufficient, render via ordinary JSX
text interpolation and skip the escaping question entirely.

**Gap — no existing PII-scanning coverage applies to this content.** Searched
`project_plans/pii-scanning/` (found `decisions/ADR-001-pii-scan-defaults-enabled-escalate.md`)
— that scanner (`server/services/pii_scanner.go`) runs on **agent-generated content passing
through the approval-hook path**, not on GitHub-sourced content flowing into the read-only VCS
tab. Reviewer body text (which can legitimately contain a reviewer's name/email in a quoted
signature, or paste a customer's data while explaining a bug) is out of that scanner's scope
entirely — this is a gap, not a violation, but worth naming explicitly in plan.md as "not
covered by existing PII controls" rather than silently assuming it is, given how directly
adjacent the two features are in this codebase.
