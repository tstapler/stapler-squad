# Research: Technology Stack — CI/CD Status in Diff Viewer

## Summary

No new dependency is needed, Go or JS. Everything the acceptance criteria require —
GitHub check-status fetch, rule-engine extension point, and real-time transport — already
exists in this codebase. The work is consolidation and wiring, not net-new integration.

## GitHub API access: hybrid `gh` CLI-style JSON parsing + raw REST/GraphQL over `net/http`

No SDK (`google/go-github`, `shurcooL/githubv4`, etc.) is a dependency — confirmed absent
from `go.mod` and no import of any such package anywhere in the tree.

Three parallel access patterns currently coexist in `github/` and `session/`:

1. **`gh` CLI JSON shape, replicated via native HTTP** — `github/client.go` defines
   `PRInfo`, `ghPRResponse`, `ghStatusCheckItem` structs that mirror
   `gh pr view --json statusCheckRollup` output, but the actual fetch goes over
   `net/http` (`github/http_client.go`), not `os/exec` of the `gh` binary. `getCheckConclusion()`
   (`github/client.go:335`) derives a single `success`/`failure`/`pending`/`""` conclusion from
   the `statusCheckRollup` array — this is the most reusable status-rollup logic in the repo.
2. **GraphQL** — `github/user_pr_cache.go:550-631` POSTs a GraphQL query to fetch the
   authenticated user's PRs in bulk, including `commits.nodes[0].commit.statusCheckRollup.state`,
   normalized via `normalizeCheckState()`. Built by hand (raw query string + `json.Marshal`
   of `{"query": ...}`), not a GraphQL client library.
3. **REST Check Runs API** — `session/backlog_plugin_github_prs.go:161-187` (`fetchCILabel`)
   calls `GET /repos/{owner}/{repo}/commits/{sha}/check-runs?per_page=50` directly via
   `net/http`, decodes into a local `githubCheckRun{Conclusion string}` struct, bounded to
   `githubCILabelConcurrency = 5` concurrent calls via `golang.org/x/sync/errgroup`.

**Auth pattern** (`github/http_client.go:28-51`, `getGHToken`): precedence is
`GITHUB_TOKEN` env → `GH_TOKEN` env → OS keychain (`GetKeychainToken()`, cached 1 minute
via `atomic.Value`/`atomic.Int64`). This is a PAT/env-token model, not a shell-out to
`gh auth token` — despite the JSON shapes being modeled on `gh`'s output, no code here
actually execs the `gh` binary for these paths (`os/exec` is imported in `client.go` but
that's for other command wrapping, not the status-fetch path). New CI-status code should
reuse `getGHToken`/`newGHRequest` rather than adding its own auth resolution.

**Supporting infra already present and reusable:**
- `github/rate_limit.go` — `DefaultRateLimiter`, updated automatically by a
  `rateLimitTransport` on every response (primary vs. secondary GitHub rate-limit
  detection, `Retry-After` handling capped at 60s). Any new poller must route requests
  through `ghHTTPClient` (or an equivalent transport wrapping) to stay covered by this.
- `github/etag_cache.go` — `ETagCache`, a `sync.Map`-backed conditional-request
  (`If-None-Match`) cache keyed by (owner, repo, prNumber), so repeated polls of an
  unchanged PR cost zero rate-limit quota (`304 Not Modified`). This is the direct
  answer to the open question "poll interval / caching strategy to avoid rate-limit
  exhaustion" — reuse this cache rather than inventing a new one.
- `session/pr_status_poller.go` / `session/worktree_pr_poller.go` — existing poller(s)
  that already populate `Instance.LastPRStatusCheck` and drive the frontend
  `PRStatusPoller` → `GitHubBadge` props (`prPriority`, `checkConclusion`, etc., see
  `web-app/src/components/sessions/GitHubBadge.tsx:23-29`). **`GitHubBadge.tsx` already
  renders a `checkConclusion` value in its tooltip** (`GitHubBadge.tsx:116`) but does not
  surface it as a first-class, always-visible badge state — the diff-viewer badge required
  by this feature should either extend this component or share its CSS variants
  (`prBadgeBlocking`/`prBadgeReady`/etc. in `GitHubBadge.css.ts`) rather than building a
  parallel badge from scratch.

**Recommendation:** consolidate on the REST Check Runs pattern
(`session/backlog_plugin_github_prs.go`'s `fetchCILabel` shape) as the canonical CI-status
fetch, since it's the most granular (per-check, not just rollup) and already
concurrency-bounded — but reuse `getCheckConclusion()`'s success/failure/pending rollup
logic to collapse multiple check runs into the single badge state the UI needs. Whatever
is chosen, use `github.ETagCache` for caching and `github.DefaultRateLimiter` awareness
before dispatching new poll work.

## Auto-approve rule engine: `pkg/classifier`, NOT `session/approval_policy.go`

Confirmed live via wiring: `pkg/classifier.RuleBasedClassifier` /
`server/services/rules_store.go`'s `RuleSpec`/`RulesFile` (backing `auto_approve_rules.json`)
is instantiated and consumed by `server/services/approval_handler.go` (`h.classifier.Classify(...)`,
`classifier.AutoAllow`/`AutoDeny`/`Escalate`) and `server/services/rules_service.go`. This is
the engine that actually runs in the deployed server.

Confirmed dead: `session/approval_policy.go`'s `PolicyEngine`/`ApprovalPolicy`/`PolicyCondition`
and `session/approval_automation.go`'s `ApprovalAutomation` are **not constructed anywhere**
outside `session/approval_automation_test.go` — `grep -rn "NewApprovalAutomation"` across the
tree returns only the test file. `server/server.go` and `server/dependencies.go` never
reference `ApprovalAutomation`, `PolicyEngine`, or `NewApprovalAutomation`. Do not add
`ci_passing` here; it will not run in production.

**Caveat / mismatch with requirements framing:** `pkg/classifier`'s live engine classifies
*Bash tool-permission requests* (`auto_allow`/`auto_deny`/`escalate` on individual tool
calls inside a session, via `CommandCriteria` matching program/subcommand/flags — see
`pkg/classifier/classifier.go:167-198`), not "approve this session's diff for merge/review."
The requirements doc's goal 3 ("rules can require green CI before auto-approving") and
acceptance criterion 6 talk about the review-queue-level session approval
(`Instance.Approve()`, `session/instance_state.go:292`), which is a **different, currently
rule-less** action — no rule engine gates `Instance.Approve()` today at all. The plan phase
needs to decide: (a) add a `ci_passing` `CommandCriteria`-style condition to
`pkg/classifier` and find/build a call site that invokes classifier logic before
`Instance.Approve()` fires (new integration point, since none exists), or (b) treat AC #5's
"configurable rule blocks manual Approve when CI red" as the real mechanism (a simpler
boolean gate in `session/review_queue_determiner.go` or `server/review_queue_manager.go`,
independent of `pkg/classifier`) and treat AC #6's "rule engine ci_passing condition" as
governing a narrower thing (e.g. an auto-approve-on-tool-call rule that happens to check CI).
This ambiguity should be resolved explicitly in the plan doc, not assumed.

## Frontend: React/Next.js/ConnectRPC-web, existing badge component family — no new deps

- **Framework**: React `^19.0.0`, Next.js `15.3.2` (`web-app/package.json`).
- **RPC/streaming**: `@connectrpc/connect ^2.1.1`, `@connectrpc/connect-web ^2.1.1` — the
  same client library already driving `WatchSessions`/`WatchReviewQueue` server-streaming
  RPCs (`server/services/session_service.go:2061-2087`, `2671-2677`,
  `server/services/review_queue_service.go:210-213`). A new CI-status field on
  `Instance`/session proto should ride these existing streams; no new transport package
  needed.
- **CSS**: vanilla-extract (`@vanilla-extract/recipes ^0.5.7`), per ADR-009 / this repo's
  CSS architecture rule — new badge variant styles go in a co-located `.css.ts` file (see
  `GitHubBadge.css.ts` for the existing pattern of state-variant classes like
  `prBadgeBlocking`/`prBadgeReady`/`prBadgeError`), not a new CSS module.
- **Existing badge component to extend**: `web-app/src/components/sessions/GitHubBadge.tsx`
  already accepts a `checkConclusion` prop and threads it into the PR badge's tooltip
  (line 116) but has no dedicated visual state for it — this is the natural extension point
  for AC #1/#2 (CI status badge with link-out) rather than a brand-new component. Sibling
  badges in the same directory (`ReviewQueueBadge.tsx`, `StatusBadge.tsx`, `SourceBadge.tsx`,
  `SubStatusChip.tsx`) show the established prop-driven, CSS-variant-per-state pattern to
  follow for any new/extended component.
- **Testing**: Jest + RTL already used for badge components (`Badge.test.tsx`,
  `GitHubBadge` has no dedicated test file currently — `StatusBadge.test.tsx` /
  `SubStatusChip.test.tsx` under `web-app/src/components/sessions/__tests__/` are the
  closest precedent for the required unit coverage). E2E via Playwright per
  `.claude/rules/e2e-test-conventions.md` — no new test infra needed.

## Go/toolchain versions (from `go.mod`)

- `go 1.26.3` (module Go version)
- `connectrpc.com/connect v1.19.0`, `connectrpc.com/otelconnect v0.8.0`
- `github.com/go-git/go-git/v5 v5.14.0` (irrelevant here — no git-object work needed for
  this feature)
- `golang.org/x/sync` (module present, used for `singleflight` in `github/http_client.go`
  and `errgroup` in `session/backlog_plugin_github_prs.go`) — reuse `errgroup` if the new
  CI-status fetch needs bounded concurrency across multiple sessions' PRs, matching the
  `githubCILabelConcurrency = 5` precedent.

## Answers to the open questions in requirements.md

1. **Which rule engine is live?** `pkg/classifier` (see above) — `session/approval_policy.go`
   is dead code. But note the domain mismatch caveat above; this needs a plan-phase decision,
   not just "use pkg/classifier."
2. **Poll interval / caching to avoid rate-limit exhaustion?** Reuse `github.ETagCache`
   (conditional `If-None-Match` requests, zero-quota `304`s) and respect
   `github.DefaultRateLimiter.IsLimited()` before dispatching — both already exist and are
   used by other pollers in this codebase. No new caching layer needed.
3. **Lazy vs. proactive fetch?** The existing `session/pr_status_poller.go` /
   `session/worktree_pr_poller.go` already proactively poll PR status for sessions with a
   PR (driving today's `GitHubBadge` `checkConclusion` prop) — AC #5 (blocking manual
   Approve on red CI) requires this proactive model to continue, since the block must be
   evaluated even if the user never opens the diff viewer. Lazy, diff-viewer-open-only
   fetching would satisfy AC #1/#2 alone but not AC #5; the research above supports
   extending the existing proactive poller rather than adding a lazy fetch-on-open path.
