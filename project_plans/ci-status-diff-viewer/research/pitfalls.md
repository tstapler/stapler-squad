# Pitfalls: CI/CD Status in Diff Viewer

Research question: what commonly goes wrong when surfacing CI status in a review tool,
mapped to this codebase's existing patterns.

## 1. GitHub API rate limiting under concurrent polling

Three existing fetch paths already solve this problem three different ways — none of them
is a drop-in fit for "one CI badge per open diff viewer," so the plan needs to pick one
pattern deliberately rather than adding a fourth:

- `session/backlog_plugin_github_prs.go:20` — `githubCILabelConcurrency = 5`, an
  `errgroup.Group` with `SetLimit()` bounding concurrent `fetchCILabel` calls
  (`:116-124`). This bounds *within one fetch cycle* but has no cross-cycle rate-limit
  awareness and no caching — every `Fetch()` re-hits the Check Runs API per PR.
- `github/user_pr_cache.go` — GraphQL-based, single query per account gets all PRs'
  `StatusCheckRollup.State` in one round trip (`:560-568`), `PollInterval: 2 * time.Minute`
  default (`:99`), refreshes coalesced via `singleflight.Group` (`:120-121`) so concurrent
  callers share one network call. This is the most rate-limit-efficient pattern in the repo
  but is scoped to the *viewer's own* PRs (`Viewer.PullRequests` in the GraphQL query), not
  arbitrary sessions' branches — reusing it for session CI status would require a different
  query shape.
- **`session/pr_status_poller.go` is the closest analog to what this feature needs**: a
  single workspace-level `time.Ticker` (not per-session goroutines, `:50`) with
  `ConcurrentFetches: 5` bounding simultaneous `gh` calls (`:27-28,42,204`), an
  `github.ETagCache` so unchanged PRs return HTTP 304 and cost **zero** rate-limit quota
  (`:51,56,91,332`), and a check against `github.DefaultRateLimiter.IsLimited()` before each
  tick that skips the whole cycle if already limited (`:190-191`). Errors are inspected for
  `"rate limit"`/`"429"` and fed back into that limiter (`:350-358`).

**Verdict**: none of the three fully solves rate-limiting for *this* use case out of the
box, but `pr_status_poller.go`'s architecture (single ticker, bounded concurrency, ETag
cache, shared rate-limit breaker) is the right template — the plan should extend/reuse it
rather than adding session-count-many independent pollers. If CI status is fetched lazily
per open diff-viewer tab instead, the same single-ticker approach still applies: one
workspace-level tick refreshes CI status for the (small) set of sessions with a diff-viewer
currently open or a pending "block on CI" evaluation, not one poller per browser tab.
Naively polling once per open session (as opposed to once per workspace) multiplies
rate-limit consumption linearly with concurrent worktrees, which is exactly what this
question warns against.

## 2. Stale/incorrect CI status (force-push, branch divergence)

None of the three existing fetchers key their cached/reported status by commit SHA in a way
that's exposed to callers — `Instance.LastPRStatusCheck` (`session/instance.go:210-211`) is
a *timestamp* of last fetch, not the SHA that was checked. `github/client.go`'s
`getCheckConclusion()` (`:335`) and `github/user_pr_cache.go`'s `normalizeCheckState`
resolve state from GitHub's `StatusCheckRollup`, which GitHub itself keys to the PR's
current head SHA — so a force-push is *eventually* reflected correctly by the upstream API,
but only after the local poller's next tick (up to `PollInterval`, 60s–2min) or ETag
invalidation. The risk is a narrower window than "shows stale forever," but two concrete
failure modes remain:

- **Display lag**: between a force-push and the next poll tick, the badge shows the
  *previous* commit's CI result labeled as current, with no visual indicator that it's
  stale. The design should carry the checked SHA (or at least `LastPRStatusCheck`) into the
  UI so a badge can show "as of Xs ago" rather than implying live truth.
- **Branch/PR mismatch**: `Instance.Branch` (`session/instance.go:114-115`) is the local
  worktree's tracked branch; `GitHubPRNumber` (`:176,490`) is the associated PR. If a
  session's local branch has diverged from what the PR's `headRefName` currently points to
  (e.g. user manually force-pushed outside stapler-squad, or rebased locally without
  pushing), the CI status fetched for the PR reflects GitHub's HEAD, not the diff currently
  shown in the diff viewer. The diff viewer's `GetSessionDiff` RPC
  (`server/services/session_service.go:2586`) diffs the *local* worktree; the CI badge would
  reflect the *remote* PR head. These can silently disagree with no error surfaced.
  **Mitigation**: badge should be scoped/labeled to the PR head SHA it reflects, and ideally
  compared against the local worktree's current HEAD SHA — if they differ, show
  "CI status may be outdated" rather than a bare green/red badge.

## 3. "Block approval on CI red" as a UX dead-end

`requirements.md` already anticipates this (Goal 2: "not a hard-coded always-on gate").
No existing `.claude/rules/*.md` file addresses gating/blocking UX generally (checked all 13
rule files — closest is `tmux-keep-server-on-restart.md`, unrelated). The concrete failure
modes to design against:

- **No CI configured** (`session/instance.go:740` `HasAssociatedPR()` — sessions with no PR,
  or a PR with no workflow file, or a non-GitHub-Actions CI provider) must resolve to
  "no checks found," a distinct state from "failing," and must **not** block approval by
  default even when the CI-blocking rule is enabled — otherwise every one-off/directory
  session or every repo without Actions configured becomes permanently unapprovable via the
  UI. Acceptance criterion 7 already requires "no CI badge... unaffected by CI-blocking
  rule" for no-PR sessions; the same must extend to has-PR-but-no-checks.
- **Flaky test blocking legitimate work**: a hard block with no override is a foot-gun. The
  requirement (AC 5) requires the block to be "visibly explained... not a silent no-op" —
  this implies the UI needs an explicit override/bypass affordance (e.g. "Approve anyway"
  with the CI-red state acknowledged in an audit trail), not just a disabled button with a
  tooltip. A rule that can never be overridden by the human reviewer who owns the review
  queue re-introduces exactly the kind of hard gate the requirements doc's Goal 2
  explicitly rejects for auto-approval; manual approval should preserve human override even
  when the rule is on.
- **Default-off is the load-bearing mitigation** — the requirements doc specifies "default:
  off" for the blocking rule (AC 5). The plan should not silently flip this default during
  implementation; it's the difference between "opt-in safety net" and "surprise
  wall" for teams whose repos don't run Actions on every branch.

## 4. Auto-approve rule engine: stale-CI race condition

Confirmed via `matchesRule` (`pkg/classifier/classifier.go:679-720`) — `Rule` fields
(`ToolName`/`ToolPattern`/`ToolCategory`/`Criteria`/`CommandPattern`/`FilePattern`, struct at
`:343-360+`) are **all ANDed** when non-nil; a new `CIPassing bool` (or similar) field would
compose the same way as `criteria.Matches()`. That AND-composition model is a reasonable fit
for `ci_passing`, but two race conditions are specific to CI status, unlike the existing
purely-local conditions (regex, file pattern):

- **Evaluation-time vs. fetch-time staleness**: `RuleBasedClassifier.classifySingle`
  evaluates synchronously against whatever CI status is currently cached on the `Instance`
  (or wherever the plan lands the cached value). If that cache was populated by the last
  poller tick (up to `PollInterval`) and CI has since gone red — e.g. a check that was still
  "pending" at fetch time failed a few seconds later — the rule can auto-approve based on a
  now-false "passing" read. This is structurally the same risk class documented in
  `.claude/rules/go-double-checked-locking.md`, but the fix there (return the
  locally-computed value, not the cache slot) doesn't apply here: there's no "locally
  computed" fresher value to fall back to, because CI status is inherently async/network
  sourced, not something the classifier can compute synchronously. The mitigation instead
  has to be either (a) only auto-approve on a "pending" or "unknown" status once it flips to
  definitively "passing" with a bounded staleness window (e.g. refuse to trust a status
  older than N poll intervals), or (b) force a synchronous re-fetch at rule-evaluation time
  for the `ci_passing` condition specifically, accepting the added latency, rather than
  trusting whatever is in the poller cache.
- **"Passing" transiently including in-flight checks**: `normalizeCheckState`
  (`github/user_pr_cache.go`) and `getCheckConclusion()` (`github/client.go:335`) both
  collapse GitHub's `StatusCheckRollup` states — the plan needs to confirm which raw states
  count as "passing" for `ci_passing` (e.g. does `SUCCESS` require *all* checks to have
  reported, or does a rollup state of e.g. `PENDING` for one check but `SUCCESS` rollup
  overall get treated as passing prematurely). Auto-approving on a rollup that hasn't seen
  all configured checks report yet is a second, independent path to the same "approved
  something that then went red" outcome.

## 5. Real-time update / event storm risk

`server/review_queue_manager.go` holds the live `*events.EventBus` (`:65`) and already
publishes `EventNotification`s (`:385`) that flow through `WatchSessions`/`WatchReviewQueue`.
`proto/session/v1/events.proto:29-33` documents the seq-based replay contract (monotonic
`seq`, clients track highest seen and pass `after_seq` on reconnect). Two failure modes to
design against explicitly:

- **Re-publishing unchanged status every tick**: if the CI-status poller publishes an event
  on every poll regardless of whether the state changed, every connected `WatchSessions`
  client re-renders and every reconnect replay grows, even though nothing changed. The
  existing `pr_status_poller.go` avoids the equivalent problem for PR *metadata* using its
  ETag cache — a 304 response means `changed` is `false` (`:332`,
  `GetPRInfoConditional(...) (prInfo, changed, err)`) and downstream code should skip the
  publish when `changed == false`. The CI-status feature must apply the identical
  changed-only-publish discipline (compare newly-fetched state against the last-known state
  on the `Instance` before calling `eventBus.Publish`), not just poll-and-always-publish.
- **Poll interval must not become per-session**: as in pitfall #1, if the design ends up
  scheduling one CI-status poll per open diff viewer (rather than one workspace-level tick
  covering all sessions with a PR), the event volume scales with concurrently open tabs, not
  with actual CI state changes — compounding both the rate-limit risk and the event-storm
  risk from the same root cause.

## 6. go-git cannot fetch CI status — must use `gh`/API

`.claude/rules/prefer-go-git-over-subshells.md` establishes "prefer go-git when it can do
the job" for git *operations* (ref resolution, ancestry checks, etc.). CI status is not a
git-native concept — go-git has no equivalent of GitHub's Check Runs / Actions API, so this
rule does not apply here; the existing `github/client.go` (`gh pr view --json
statusCheckRollup`, `:101`) and `github/user_pr_cache.go` (GraphQL against
`api.github.com/graphql`) are correctly the two viable transports, matching requirements AC
3 ("via the existing `gh`/GitHub API integration"). Worth flagging explicitly in the plan so
a future implementer doesn't try to force this through go-git and hit a dead end, or
conversely doesn't add a new HTTP client from scratch when `github/client.go` and
`github/user_pr_cache.go` already have working, authenticated request plumbing
(`ghHTTPClient`, `githubAPIURL`, `graphQLURLForHost`) to reuse.

## 7. Confirmed: which rule engine is live (resolves requirements.md's open question)

- `pkg/classifier.RuleBasedClassifier` — constructed in
  `server/services/session_service.go:296` and threaded into
  `server/services/rules_service.go:33,40`; this is the live, production-wired engine.
- `session/approval_policy.go`'s `PolicyEngine` — only ever constructed by
  `session/approval_automation.go:97-101` (`NewApprovalAutomation`), and
  `NewApprovalAutomation` itself is **only called from `session/approval_automation_test.go`**
  (verified via repo-wide grep — zero non-test call sites). This confirms the requirements
  doc's suspicion: `ApprovalPolicy`/`PolicyEngine` is dead code in the production path. The
  `ci_passing` condition must be added to `pkg/classifier`'s `Rule`/`CommandCriteria`
  machinery, not `session/approval_policy.go`.

## Summary of design implications

1. Reuse `session/pr_status_poller.go`'s architecture (single workspace ticker, bounded
   concurrency via semaphore/errgroup, `github.ETagCache`, `github.DefaultRateLimiter` guard)
   for CI-status polling rather than building a fourth independent fetch path.
2. Only publish `WatchSessions`/`WatchReviewQueue` events when CI status actually changed
   (changed-only-publish, mirroring the ETag `changed` bool already returned by
   `GetPRInfoConditional`).
3. Carry the checked commit SHA (or at minimum `LastPRStatusCheck` timestamp) into the UI so
   staleness after a force-push is visible rather than silently wrong.
4. Default the "block approval on CI red" rule to off, treat "no checks configured" as a
   distinct non-blocking state, and preserve a human override path even when the rule is on.
5. Add `ci_passing` to `pkg/classifier`'s `Rule` struct (ANDed with existing fields per
   `matchesRule`'s existing composition model) — not to `session/approval_policy.go`, which
   is confirmed dead code. Guard against auto-approving on a stale or partially-reported
   "passing" rollup.
