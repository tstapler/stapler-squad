# Research: Similar Features, Prior Art, Edge Cases — CI/CD Status in Diff Viewer

## 1. Existing CI-status fetching/normalization in this codebase

There are **four** separate places that already fetch and normalize GitHub CI/check
status, each with a different shape. Any new work must reuse one of these, not add a
fifth.

### 1a. `session/backlog_plugin_github_prs.go:161` — `fetchCILabel`
- Fetches via REST **Check Runs API** (`GET /repos/{o}/{r}/commits/{sha}/check-runs`),
  not the PR-level `statusCheckRollup`.
- Collapses everything down to a single boolean label: returns `"pr:ci-failing"` if any
  check run has `conclusion == "failure"` or `"timed_out"`, else `""`. No representation
  of pending/success/no-checks — this is backlog-labeling-shaped, not status-badge-shaped.
- Bounded concurrency via `errgroup.SetLimit(githubCILabelConcurrency)` (5)  — the
  concurrency-bounding pattern worth reusing for any new per-session CI fetch loop.
- Best-effort/silent-failure: `fetchAndUpdatePRStatus`-style errors just return `""`,
  no error surfaced to caller.

### 1b. `github/client.go:335` — `getCheckConclusion`
- Fetches via `gh pr view --json statusCheckRollup` (`ghStatusCheckItem` has
  `State`/`Status`/`Conclusion`/`Name`/`Context` — the richest of the three).
- Aggregates a **list** of checks into one `(conclusion, status)` pair with real
  precedence: failure/error/action_required/timed_out > in_progress/queued/pending >
  success > neutral (default/mixed). This is the most complete normalization and is
  **already the one wired into the live polling path** (`PRStatusPoller` →
  `applyPRUpdate` → `Instance.GitHubCheckConclusion`, see §2).
- Returned values: conclusion ∈ `{"failure","pending","success","neutral",""}`,
  status ∈ `{"completed","in_progress",""}`.

### 1c. `github/user_pr_cache.go:663` — `normalizeCheckState`
- Fetches via **GraphQL** (`Commits.Nodes[0].Commit.StatusCheckRollup.State`) — a single
  already-aggregated `State` enum from GitHub's own GraphQL rollup, not a list of checks.
- Maps `SUCCESS→success`, `FAILURE|ERROR→failure`, `PENDING|EXPECTED→pending`, else
  lowercased passthrough. Used for the cross-workspace "my open PRs" cache
  (`UserPRCache`), a different feature (global PR list) from per-session status.

### 1d. `github/client.go:279` (`GetPRInfoCtx`) is the function that actually calls
`getCheckConclusion` (1b) and populates `PRInfo.CheckConclusion`/`CheckStatus`. This
`PRInfo` is the shared DTO consumed by the poller (§2).

**Design implication:** three different normalization vocabularies exist
(`pr:ci-failing` label / `failure|pending|success|neutral` / `success|failure|pending|<raw>`).
The plan phase should pick 1b's vocabulary (`success|failure|pending|neutral|""`) as
canonical since it's already the one on the wire (`Instance.GitHubCheckConclusion`,
proto `GithubCheckConclusion`), and treat `""` as "no checks found" per AC1. Do not
introduce a fourth normalization; do not resurrect 1a's binary label for this feature
(it discards too much state — no pending/no-checks distinction).

## 2. A full CI/PR status polling system already exists — this is the most important finding

`session/pr_status_poller.go` (`PRStatusPoller`) is a **complete, production-wired**
background poller that already does almost everything AC1–AC4 ask for, just not
exposed in the diff viewer specifically:

- Single shared ticker (not per-session goroutines), default 60s interval
  (`DefaultPRStatusPollerConfig`), bounded concurrency (5), ETag-cached (HTTP 304 costs
  zero rate-limit quota) via `github.ETagCache` and a `sync.Map` of list ETags.
- Auto-discovers the PR for a branch when `GitHubPRNumber` is unknown
  (`GetPRForBranchConditional`), backs off re-checking "no PR yet" branches for 5 min
  (`NoPRBackoff`) so it doesn't hammer branches without a PR.
- Skips terminal PRs (`GitHubPRStatusTerminal`, set once merged/closed) and fork
  sessions (explicit TODO: "upstream PR lookup Phase 2").
- Auth-checked and rate-limit-aware: bails the whole tick if
  `github.DefaultRateLimiter.IsLimited()`, caches `gh auth` result for 5 min
  (`isAuthOK`), and on a detected 401 invalidates that cache early
  (`handleFetchError`).
- On change, calls `Instance.UpdatePRStatus(...)` (`session/instance_terminal.go:326`),
  which sets `GitHubCheckConclusion`, `LastPRStatusCheck`, etc. under the actor lock,
  persists via `Storage.UpdateInstancePRStatus`, and — **only when derived priority
  changed** — invokes the `onUpdated` callback.
- That callback is wired in `server/dependencies.go:585`:
  `eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority",
  "github_pr_state"}))`. `NewSessionUpdatedEvent` embeds the **entire** `*session.Instance`
  snapshot (`pkg/events/types.go:143-150`), so `GithubCheckConclusion` already rides
  along on every such event even though it isn't named in `updatedFields` — this event
  already flows to the frontend over `WatchSessions`.
- `Instance.GitHubCheckConclusion` is already serialized end-to-end: stored
  (`session/storage.go:64`), snapshotted (`session/instance_snapshot.go:55,188`), and
  surfaced to proto (`server/adapters/instance_adapter.go:69-70`,
  `GithubCheckConclusion` + `LastPrStatusCheck` fields already exist on the wire).

**Design implication:** AC3 ("reuse existing check-status fetch code") and AC4 ("reach
frontend via existing WatchSessions... infra") are **almost entirely already done** at
the backend/transport layer. The gap is narrower than the requirements doc implies —
it's primarily a **frontend wiring gap** (diff viewer doesn't read this field yet) plus
two real gaps:
  - **`onUpdated` only fires when derived *priority* changes** (`result.PriorityChanged`
    in `applyPRUpdate`), not whenever `checkConclusion` changes in isolation. A CI
    result flipping from pending→failure does not by itself change `PRPriority` unless
    it also crosses a priority boundary (check `github/priority.go`'s
    `DerivePRPriority` — worth confirming in the plan phase whether check-conclusion
    changes need their own change-detection branch so the diff-viewer badge doesn't go
    stale between poll ticks that don't happen to flip priority).
  - **No link to the specific Actions run/check page** (AC2) — `PRInfo`/`Instance`
    currently store only the aggregate conclusion, not a check-run URL. `ghStatusCheckItem`
    (§1b) has per-check `Name`/`Context` but `getCheckConclusion` discards them when
    collapsing to one conclusion; a run URL isn't captured anywhere in this pipeline
    today. The plan needs to either persist one representative check-run URL (e.g. the
    first failing/first check) or link to the PR's Checks tab
    (`{prUrl}/checks`) as a lower-fidelity fallback — GitHub renders that page for any
    conclusion.

## 3. Existing frontend badge components — reuse, don't rebuild

CI conclusion is **already rendered in two places** in the frontend, both driven by the
same `Instance.GitHubCheckConclusion` field:

- `web-app/src/components/sessions/GitHubBadge.tsx` — accepts a `checkConclusion` prop
  and folds it into the PR badge's tooltip text (`CI: ${checkConclusion}`) — not a
  separate visible element, and not a link (the badge's `href` goes to the PR, not to
  Actions). Used by `SessionCard.tsx`, `SessionRow.tsx`, `SessionDetailView.tsx`.
- `web-app/src/components/shared/vcs-widget/VcsWidgetGithubRow.tsx` — renders a
  standalone `<span>CI: {conclusion}</span>` color-coded via `ciClassName`
  (`success`→green, `failure`→red, everything else→pending/yellow styling) — also not a
  link, and collapses `neutral`/`""`/`pending` all into the same "pending" visual
  bucket (no distinct "no checks found" treatment, contradicting AC1's 4-state
  requirement). Used by `VcsPanel.tsx`, `UnfinishedItemDetail.tsx`, and
  `BacklogItemDetail`'s `VersionControlSection`.

**Design implication:** the new diff-viewer badge should be a third consumer of the
same `checkConclusion` data (via `SessionVcsContext`, which `DiffViewer.tsx` already
reads for diff content — see `web-app/src/components/sessions/DiffViewer.tsx:12`), not
a new fetch path. Two real product gaps to close, common to both existing renderers:
(1) neither is a clickable link to the check page itself, and (2) neither distinguishes
"no checks configured" from "pending" — both requirements this feature explicitly
needs (AC1, AC2) that the two existing components don't yet solve. Fixing
`VcsWidgetGithubRow`'s conclusion→style mapping to add a distinct no-checks state would
also fix it for the other three call sites for free.

## 4. Which auto-approve engine is live — confirms the Open Question

Two candidate engines exist; only one is wired to production:

- **`session/approval_policy.go`'s `PolicyEngine`** (wrapped by
  `session/approval_automation.go`'s `ApprovalAutomation`) is **dead code**. `grep` for
  `NewApprovalAutomation(` finds call sites only in
  `session/approval_automation_test.go` — nowhere in `server/`, `session_service.go`,
  or `server.go`. Do not add `ci_passing` here.
- **`pkg/classifier`'s `RuleBasedClassifier`** (`RuleSpec` persisted via
  `server/services/rules_store.go`, exposed via `RulesService`
  (`server/services/rules_service.go`)) **is live**: `server/server.go:467` wires
  `approvalHandler.SetClassifier(deps.SessionService.GetClassifier())`, and
  `server/services/approval_handler.go:283` calls `h.classifier.Classify(payload,
  classCtx)` on every incoming Claude Code tool-approval hook request. **This is the
  engine AC6 must extend.**

**Important architectural mismatch for the plan phase:** `RuleBasedClassifier` operates
per **tool-use approval request** (`PermissionRequestPayload{ToolName, ToolInput,
SessionID, Cwd, ...}` — see `pkg/classifier/classifier.go:50-58`), not per **session**.
It has no concept of "this session's PR/branch/CI state" in its matching context
(`ClassificationContext{Cwd, IsGitRepo, RepoRoot, IsWorktree, Env}` —
`pkg/classifier/classifier.go:61-72`) today. `PermissionRequestPayload.SessionID` *is*
present, so a `ci_passing` `CommandCriteria`/`Rule` condition is technically pluggable
(classifier would need a session→CI-status lookup injected, e.g. a callback populated
into `ClassificationContext` by `approval_handler.go` before calling `Classify`), but
AC6's phrasing ("before a rule auto-approves **a session**") doesn't map 1:1 onto what
this engine actually gates (individual bash/tool calls, not "approve this session").
The plan phase must decide: does `ci_passing` gate *every* tool-use auto-approval in a
session with a PR (broad, changes behavior for unrelated tool calls), or does it only
apply to a narrower "approve the whole session" action distinct from per-tool
classification (matches AC5's block-manual-approval framing better, and is a different
code path — `Instance.Approve()` / `ReactiveQueueManager.handleApprovalResponse`
(`server/review_queue_manager.go:300`), not the classifier)? These are two different
gates in the existing code and the requirements doc's ACs 5 and 6 map to each
separately — AC5 → `Instance.Approve()` path, AC6 → `RuleBasedClassifier` path. Treat
them as two independent integration points, not one.

## 5. Industry / competitor prior art

- **Emdash** (open-source, YC W26 — github.com/generalaction/emdash): diff view shows
  changes across agents side-by-side; a distinct "CI/CD checks" panel monitors GitHub
  Actions check runs inside the tool, alongside PR creation/merge from the same screen.
  Validates the requirement's framing (CI status co-located with diff review, not a
  separate dashboard).
- **Aizen** (macOS workspace app — github.com/vivy-company/aizen): shows GitHub Actions
  *and* GitLab CI runs from the worktree sidebar (not inline in the diff itself) —
  confirms sidebar/header placement is a viable alternative to inline-in-diff if the
  plan phase wants a less invasive layout than embedding in `DiffRenderer`.
- **Symphony** (OpenAI, github.com/openai/symphony): frames CI status as part of
  "proof of work" alongside PR review feedback and complexity analysis — i.e. CI status
  is one signal among several presented together when a reviewer is deciding whether to
  trust an agent's output, reinforcing AC5's premise (CI-red should visibly weigh
  against approval, not be a silent detail).
- **GitHub's own Checks API model** (canonical vocabulary this codebase should track):
  `status` transitions `queued → in_progress → completed`, and only on `completed` is a
  `conclusion` set from `{success, failure, neutral, cancelled, timed_out,
  action_required, stale, skipped}`. This confirms AC1's 4-bucket UI simplification
  (passing/failing/pending/no-checks) is a reasonable collapse of the real state space,
  matching what `getCheckConclusion` (§1b) already does — but note GitHub's real
  vocabulary also has `cancelled`, `skipped`, and `stale`, which `getCheckConclusion`
  currently folds into the `neutral` catch-all. Confirm in the plan phase whether
  "cancelled" should visually read as "no checks found"/neutral (current behavior) or
  get its own treatment — a cancelled run reads very differently to a reviewer than an
  uninstrumented repo.
- **GitLab CI / CircleCI badges** — out of scope per requirements' explicit non-goal
  (GitHub Actions only); no further action needed beyond confirming the badge-with-
  link-out pattern (a static SVG/text badge linking to the full run) is the same shape
  CircleCI/GitLab use, which the existing `VcsWidgetGithubRow` text-badge approach
  already roughly follows minus the link.

## 6. Edge cases and failure modes to handle

Ranked by whether existing infra already covers them or the plan needs new work:

**Already handled by `PRStatusPoller`/`github` package (reuse, don't re-solve):**
- *Rate limiting*: `github.DefaultRateLimiter.IsLimited()` check skips entire poll
  ticks; `handleFetchError` detects 429/"rate limit" in error text.
- *No CI configured*: `getCheckConclusion` returns `("", "")` for an empty checks list
  — already distinguishable from `"neutral"`/`"pending"` if the frontend chooses to
  render `""` as "no checks found" (today's `VcsWidgetGithubRow` doesn't — see §3 gap).
- *CI still queued*: `status ∈ {queued, in_progress}` → `"pending"` conclusion, already
  covered by `hasInProgress` branch in `getCheckConclusion`.
- *Branch deleted / PR closed/merged*: `GitHubPRStatusTerminal` flag set once
  state∈{merged,closed}; poller then permanently skips that session
  (`checkAllSessions` continue). `ErrNoPR` path (`applyNoPR`) handles "branch exists,
  no PR yet" with backoff.
- *Private repo / auth failures*: `CheckGHAuth()` cached 5 min; poller skips the whole
  tick (`isAuthOK`) rather than erroring per-session; 401/403 detected and cached to
  avoid hammering a broken token.
- *Session with no PR yet (one-off/directory sessions)*: `checkAllSessions` skips
  instances with empty `GitHubOwner`/`GitHubRepo` outright — maps directly onto AC7
  ("no CI badge, not an error state").

**Not yet handled — real gaps for the plan phase:**
- *Force-push invalidating a previous run*: ETag caching (`github.ETagCache`) keys on
  the PR API response, not on head SHA. A force-push changes `PRInfo` (new SHA
  indirectly reflected in a new `statusCheckRollup`), so the next poll tick should
  naturally pick up a changed ETag and refresh — but confirm the ETag is scoped to the
  PR resource (which includes head SHA in its representation) and not e.g. cached
  stale for the poll interval; worth an explicit test case since a force-push mid-poll-
  interval means the diff viewer could show a stale conclusion for up to
  `PollInterval` (60s default) — acceptable per AC4's "reasonably fresh," but should be
  stated as the freshness bound in the plan, not left implicit.
- *Fork sessions*: explicitly unhandled today — `checkAllSessions` skips forks with a
  log line citing "upstream PR lookup Phase 2" as future work. The plan phase should
  either explicitly scope fork sessions as "no CI badge" (consistent with AC7's
  no-error framing) or flag upstream-PR lookup as now-in-scope; don't silently ignore
  it.
- *`ci_passing` rule evaluation timing*: since CI status is polled (not synchronous),
  a `ci_passing` auto-approve rule (AC6) evaluated at tool-use time reads whatever
  `GitHubCheckConclusion` was as of the last poll tick — up to 60s stale. An
  auto-approve rule acting on stale-green CI when the branch was just force-pushed
  with new (possibly red) commits is a real correctness risk specific to this feature,
  not present in any of the existing consumers (which are read-only displays, not
  gates). The plan should consider whether `ci_passing` rules need a tighter
  freshness bound or an explicit "last checked N seconds ago" surfaced alongside the
  gate decision.
- *Diff viewer open with no session-to-branch CI state yet fetched* (cold start): first
  poll tick fires immediately on `Start()` (`pollLoop` calls `checkAllSessions()` before
  entering the ticker loop), so cold-start latency is bounded by one fetch round trip,
  not a full `PollInterval` — but the diff viewer opening should not itself trigger an
  out-of-band fetch (per the Open Questions section's "lazy vs proactive" question,
  and AC4's "not a newly introduced polling loop on the frontend") — it should just
  read whatever the shared poller last wrote, even if that's a few seconds stale on a
  brand new session.

## 7. Unstated user needs beyond the explicit ACs

- **Reviewers need to know *how stale* the badge is**, not just its color — `LastPRStatusCheck`
  already exists on the wire (`server/adapters/instance_adapter.go:70`) but no existing
  frontend consumer surfaces it. A "CI: failing (checked 45s ago)" affordance is cheap
  to add given the field already exists end-to-end, and directly addresses the
  force-push-staleness edge case above by making staleness visible instead of silent.
- **The "why is Approve blocked" explanation (AC5) needs to point at the actual failing
  check(s), not just say "CI is red"** — otherwise a reviewer overriding the block has
  no faster path than opening the PR in a new tab, which defeats the "stay in the diff
  viewer" premise of the whole feature. This argues for capturing at least the failing
  check's `Name`/`Context` (available in `ghStatusCheckItem`, §1b) even though
  `getCheckConclusion` currently discards it.
- **A reviewer who overrides the CI-red block should have that override distinguishable
  later from a normal approval** — none of the ACs ask for an audit trail, but
  `PolicyAuditEntry`-style logging already exists as a pattern elsewhere in this
  codebase (dead-code `PolicyEngine`, §4) and `AutoApprovalLog`
  (`h.autoApprovalLog.AppendAutoApproved`, referenced in `approval_handler.go`) is a
  live analog for the classifier path — worth considering whether a CI-red override
  should append a similar log entry, since "reviewer approved despite red CI" is
  exactly the kind of decision a team would want to audit after an incident.
- **Rule authors composing `ci_passing` AND a regex/command condition (AC6 example)
  need the rule UI to show *why* a rule didn't match** — if `ci_passing` fails silently
  the same way an unmatched regex does today, a user debugging "why didn't my
  auto-approve rule fire" has no way to tell CI was the blocker versus the command
  pattern. `ClassificationResult.Reason` already exists as the field to populate for
  this; the plan should ensure a `ci_passing`-caused non-match produces a distinguishable
  reason string, consistent with how existing criteria failures are (not) currently
  differentiated (`matchesRule` today just returns bool, no per-criterion reason).
