# Architecture Research: CI/CD Status in Diff Viewer

No prior `code-hotspot-analysis` or `quality:architecture-review` output exists for this area
(checked `project_plans/*/research/architecture.md` — no overlap with diff viewer / review
queue / classifier found). All findings below are from direct code reading on
`main` at the time of research (2026-08-02).

## Headline finding: most of this feature already exists, scattered

The single biggest architectural fact this research turned up: CI status fetching, caching,
proto plumbing, and a rendered badge component **already exist and are live in production**
for the session list/detail views. The gap is narrower than the requirements doc's "Existing
building blocks" section suggested — it's not "consolidate scattered fetch code," it's
"wire the diff viewer into infrastructure that already ends at a badge component, and add two
new gating hooks." See `Gap Analysis` at the end.

## 1. Integration points

### 1a. Which rule engine is live: confirmed `pkg/classifier`, `session/approval_policy.go` is dead code

- `pkg/classifier.RuleBasedClassifier` is instantiated in
  `server/services/session_service.go:296` and injected into `ApprovalHandler` via
  `SetClassifier` (`server/services/approval_handler.go:122`). It is the classifier that
  runs in `ApprovalHandler.HandlePermissionRequest`
  (`server/services/approval_handler.go:282-283`) — the live HTTP hook handler Claude Code's
  `PreToolUse` hook calls for every tool-use permission request. `RuleSpec` persistence
  (`server/services/rules_store.go:20-49`) round-trips to/from `classifier.Rule` and is
  managed by `RulesService` (`server/services/rules_service.go:33,40`).
- `session/approval_policy.go`'s `PolicyEngine`/`ApprovalPolicy`/`PolicyCondition` and its
  only caller, `session/approval_automation.go`'s `ApprovalAutomation`
  (`NewPolicyEngine()` at `session/approval_automation.go:101`), form a closed island: grep
  across the whole repo shows `ApprovalAutomation` and `PolicyEngine` referenced only from
  within `session/approval_policy.go`, `session/approval_automation.go`, and their own
  `_test.go` files. Nothing in `server/`, `main.go`, or any other `session/*.go` file
  constructs a `PolicyEngine` or `ApprovalAutomation`. **Confirmed dead code** — do not wire
  `ci_passing` into it.

### 1b. What "auto-approve rule engine" and "review queue" actually mean in this codebase

This matters because the requirements doc's AC5 and AC6 read as if they're about two
different things (blocking a session-level "Approve" action vs. an auto-approve rule engine),
but tracing the code shows **they are the same underlying system** operating at the
granularity of individual Claude Code tool-use permission requests, not whole-session
diff approval:

- `pkg/classifier.RuleBasedClassifier.matchesRule` (`pkg/classifier/classifier.go:679`)
  matches a `Rule` against a single `PermissionRequestPayload` — one Bash/Edit/Write/etc.
  tool call, identified by `ToolName`/`CommandPattern`/`Criteria`/`FilePattern`
  (`pkg/classifier/classifier.go:343-369`). It has no concept of "a session" beyond `Cwd`.
- When the classifier returns `Escalate` (no matching allow/deny rule), the request becomes
  a `PendingApproval` and the session enters the review queue with
  `ReasonApprovalPending` (`session/review_queue_determiner.go:128-132`).
- The review queue UI's "Approve" button calls `SessionService.ResolveApproval`
  (`server/services/session_service.go:3002-3008`, doc comment: *"allows the web UI to
  approve or deny a pending Claude Code tool use request"*), which is a thin delegate to
  `ApprovalService.ResolveApproval` (`server/services/approval_service.go:46-103`). That
  method resolves the *specific pending tool-use request* (`approvalStore.Resolve`), then
  publishes an `ApprovalResponseEvent` (`server/services/approval_service.go:94`), which
  `ReactiveQueueManager.handleApprovalResponse` (`server/review_queue_manager.go:299-320`)
  consumes to call `inst.Approve()` (transitions the *Instance*, i.e. the Claude Code
  process, from waiting-for-approval back to `Active`
  — `session/instance_state.go:290-294`) and removes the item from the queue.

So: **`ci_passing` as a `Rule`/`CommandCriteria` condition (AC6) and "block manual Approve
when CI is red" (AC5) are two gates on the same code path** — the former runs automatically
inside the classifier before a request is escalated at all; the latter runs when a human
clicks Approve on an already-escalated request. Both need to reach the same piece of data:
the CI status of the session's branch.

### 1c. Where `ci_passing` attaches in `pkg/classifier`

`Rule` (`pkg/classifier/classifier.go:343-369`) is matched by `matchesRule(rule Rule,
payload PermissionRequestPayload) bool` — **no `ClassificationContext` parameter**. This is
the concrete implementation gap: `ClassificationContext` (built once per request by
`BuildContext(cwd)`, `pkg/classifier/classifier.go:645-674`, populated with `Cwd`,
`IsGitRepo`, `RepoRoot`, `IsWorktree`, `Env`) is threaded through
`Classify` → `classifyInternal` (`:429`) → `classifyCompound` (`:564`) for env-expansion and
`AuditCommand`, but is **dropped** before the final call to `classifySingle(payload)`
(`:495`, `:506`), which is what actually calls `matchesRule`. Adding `ci_passing` therefore
requires:
1. A new field on `ClassificationContext`, e.g. `CIPassing *bool` (nil = unknown/no PR,
   so a rule requiring CI-passing can distinguish "no PR" from "CI red" and both should
   fail the condition) or `CIStatus string` mirroring `Instance.GitHubCheckConclusion`'s
   vocabulary (`success`/`failure`/`pending`/`action_required`/`neutral`/`""`).
2. Populating it in `ApprovalHandler.HandlePermissionRequest`
   (`server/services/approval_handler.go:189` onward) — `ApprovalHandler` already holds
   `storage *session.Storage` (`server/services/approval_handler.go:70`) and already
   resolves `sessionID` (`:198`) before classification, so it can look up the `Instance`
   and read `GitHubCheckConclusion` there, no new dependency needed.
3. Threading `ctx` through `classifySingle`/`matchesRule` (currently payload-only) and
   adding a `RequireCIPassing bool` (or similar) field to `Rule` + `RuleSpec`
   (`server/services/rules_store.go:20-49`) checked in `matchesRule`.
4. **No session-scoping ambiguity to solve**: `ApprovalHandler` already resolves the
   specific session for every permission request via `X-CS-Session-ID` /cwd matching
   (`server/services/approval_handler.go:198`), so "CI passing" naturally means "CI passing
   for *this* session's branch," consistent with AC6's example ("regex command match AND CI
   passing").

### 1d. Where "block manual Approve when CI red" (AC5) attaches

`ApprovalService` (`server/services/approval_service.go:17-21`) currently holds only
`approvalStore`, `notificationStore`, `eventBus` — **no instance/session lookup**. To block
approval when CI is red and visibly explain why (not a silent no-op, per AC5), `ResolveApproval`
(`:46`) needs, before calling `as.approvalStore.Resolve(...)`:
- The target session's `Instance` (to read `GitHubCheckConclusion` and confirm
  `HasAssociatedPR()`, `session/instance.go:740`, so sessions without a PR are correctly
  exempted per AC7) — resolved from `sessionID` already computed at
  `server/services/approval_service.go:69-71`.
- A new configurable toggle, default off per AC5 ("configurable rule (default: off)"). No
  existing settings location is an obvious fit (`config.Config`,
  `config/config.go:229` onward, holds process-wide flags like `AutoYes`; `RulesStore`
  holds per-tool-call `RuleSpec`s, not a single global switch) — this is new surface area,
  most naturally a small dedicated setting (e.g. `BlockApprovalOnCIFailure bool`) rather than
  overloading either existing store. On rejection, return a `connect.Code` with a message
  identifying the failing check (not just `false`), so the frontend can render the "why" per
  AC5 instead of a bare failure.

## 2. Data flow: proactive per-session caching already exists — reuse it, don't add lazy fetch

The architecture question ("lazily fetch per-diff-view request, or proactively
cache/poll per-session with a background goroutine?") is already answered by existing code,
and the answer is unambiguous: **proactive, per-session, background-polled, already
running in production.**

`session/pr_status_poller.go`'s `PRStatusPoller` (constructed
`server/dependencies.go` — wired via `warren.SetAlways(w2, "PRStatusPoller.Instances", ...)`
and `.OnUpdated`, `server/dependencies.go:583-587`) is a single workspace-level ticker
(`session/pr_status_poller.go:169-186`, default 60s interval,
`DefaultPRStatusPollerConfig`, `:39-47`) that:
- Iterates every monitored `Instance`, skips ones with no GitHub owner/repo, skips
  fork sessions and already-terminal (merged/closed) PRs (`:220-230`).
- Fetches PR info **conditionally via ETag** (`github.GetPRInfoConditional`,
  `:332`, backed by `github.ETagCache`) — unchanged PRs cost a 304, zero rate-limit
  quota, directly satisfying goal 4 ("keep CI status fresh without polling storms").
- Bounds concurrency at 5 simultaneous `gh`/API calls (`ConcurrentFetches`, `:42`) and
  respects a global `github.DefaultRateLimiter` circuit-breaker (`:190-193`).
- Applies the result via `inst.UpdatePRStatus(state, priority, checkConclusion, ...)`
  (`:396`, `session/instance.go` — persists to `Instance.GitHubCheckConclusion`,
  `session/instance.go:206-207`, comment already documents the vocabulary:
  *"success/failure/pending/action_required/neutral/"""*) and to SQLite
  (`storage.UpdateInstancePRStatus`, `:399`).
- On a priority change, fires `onUpdated(inst)` (`:405-413`), wired in
  `server/dependencies.go:585-587` to
  `eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_priority",
  "github_pr_state"}))` — i.e. **this already publishes through the EventBus that backs
  `WatchSessions`**, satisfying goal 4's "reuse existing real-time infra" and AC4 directly.

**Conclusion for the diff viewer specifically**: no new caching layer, background goroutine,
or package is needed. The diff viewer should read `GitHubCheckConclusion` off the same
`Session` object the rest of the app already gets from `WatchSessions` (see Gap Analysis),
not issue its own fetch when opened. This also means CI status is available to the
"block approval" rule (§1d) *before* the diff viewer is ever opened, which is what AC5
requires (the block must reflect current CI state independent of viewing habits).

One gap: `NewSessionUpdatedEvent`'s `updatedFields` hint list
(`server/dependencies.go:586`, `["github_pr_priority", "github_pr_state"]`) does not include
`"github_check_conclusion"`. Confirm whether `updatedFields` is purely a UI hint (full
`Session` snapshot is sent regardless — likely, since `SessionUpdatedEvent` embeds `Session`
per `proto/session/v1/events.proto:37`) or gates any downstream diffing logic; if the latter,
add `"github_check_conclusion"` to the hint list so CI-conclusion-only changes aren't
silently dropped by a consumer that filters on it.

## 3. Consistency: no new race, bounded staleness inherited from existing poller

`GitHubCheckConclusion` is written by `PRStatusPoller.applyPRUpdate` via `Instance`'s actor
pattern (`inst.UpdatePRStatus(...)`, an actor-synchronized write per
`.claude/rules/go-double-checked-locking.md`'s sibling pattern in this codebase), and read
by `Instance.Snapshot()` elsewhere (e.g. `session/pr_status_poller.go:276`,
`server/adapters/instance_adapter.go:69`). Both the classifier's `ci_passing` check (§1c)
and the "block manual Approve" check (§1d) would read via the same `Snapshot()`/actor-safe
path — no torn reads, and no race is *introduced* by this feature.

The real property to state plainly: **CI status at approval-decision time can be up to
~`PollInterval` (60s) + one API round-trip stale**, same staleness that already exists
today for every other consumer of `GitHubCheckConclusion` (`SessionCard`, `SessionRow`,
`SessionDetailView`, `VcsPanel` — see Gap Analysis). This is a pre-existing, accepted
trade-off in the codebase, not a new one this feature creates, and matches goal 4's explicit
instruction to avoid inventing new polling. If tighter freshness at the exact moment of
approval is wanted later, `ApprovalService.ResolveApproval` could force a synchronous
`PRStatusPoller.fetchAndUpdatePRStatus`-equivalent single-session refresh before deciding —
call this out as an option, not a requirement; it adds latency to every approval click and
isn't asked for by any AC.

No full EventStorming Event-Command-Policy table is warranted — per the requirements doc's
own scoping (goal note: "fairly contained CRUD-ish feature"), and confirmed by this
research: there is exactly one new Policy ("if `block_on_ci_failure` enabled AND CI ==
failure AND decision == allow, reject with reason") sitting on one existing Command
(`ResolveApproval`), plus one new Condition type (`ci_passing`) on one existing Policy engine
(`RuleBasedClassifier`). No new actor, no cross-aggregate saga, no compensating action.

## 4. Session-creation-registry / non-goals check

Confirmed no new session type is introduced — `session/instance.go`'s `SessionType`
constants and the 7-touchpoint registry (`.claude/rules/session-creation-registry.md`) are
unaffected; this feature only reads existing `Instance` fields
(`GitHubCheckConclusion`, `GitHubPRNumber`, `HasAssociatedPR()`) and augments existing
diff-viewer/review-queue UI and the classifier's rule schema. AC9 is satisfied by
construction, not by any change needed here.

## Gap Analysis: what's actually missing (scope check for the plan phase)

Frontend research turned up more pre-existing infrastructure than the requirements doc's
"Existing building blocks" section captured — worth flagging so the plan phase doesn't
over-scope:

- **Proto**: `github_check_conclusion` (field 38) and `last_pr_status_check` (field 39)
  already exist on `Session` (`proto/session/v1/types.proto:127,130`) and are already
  populated by `server/adapters/instance_adapter.go:69-70`. No proto change needed for the
  badge's core data.
- **Frontend types/adapters**: `web-app/src/lib/vcs/types.ts` already defines
  `CheckConclusion` (`"success" | "failure" | "pending" | ""`) and `GithubSummary.
  checkConclusion`; `web-app/src/lib/vcs/adapters.ts:68-79`'s `fromSessionGithub`/
  `fromSessionVcs` already map a `Session` proto object into this shape, including
  `checkConclusion: toCheckConclusion(session.githubCheckConclusion)`.
- **Rendered badge component**: `web-app/src/components/shared/vcs-widget/
  VcsWidgetGithubRow.tsx:79-81` already renders a color-coded `CI: {checkConclusion}` badge
  (`ciClassName` maps success/failure/default → CSS classes) and links out via
  `github.prUrl` (`:52`). It's already used by `VcsPanel.tsx`, `SessionCard.tsx:481`,
  `SessionRow.tsx:271`, `SessionDetailView.tsx:1159-1162`, and (for shipped/historical data)
  `UnfinishedItemDetail.tsx`/backlog's `VersionControlSection`.
- **The actual gap**: `web-app/src/components/sessions/DiffViewer.tsx:11` reads only
  `{ diff, diffLoading, refreshDiff }` from `useSessionVcsContext()`
  (`web-app/src/lib/contexts/SessionVcsContext.tsx`, backed by
  `useSessionVcs(sessionId, baseUrl)`), which does **not** carry the `Session` object or its
  `githubCheckConclusion` — unlike `VcsPanel.tsx`, which receives `session` as a prop from
  its parent and calls `fromSessionVcs(status, session)` itself
  (`web-app/src/components/sessions/VcsPanel.tsx:17-18,65`). The diff viewer's header badge
  (AC1) is therefore mostly a matter of: (a) passing `session` (or just
  `githubCheckConclusion`/`githubPrUrl`) into `DiffViewer`, the same way `VcsPanel` already
  receives it, and (b) rendering `VcsWidgetGithubRow` (or a slimmer CI-only variant) in the
  diff viewer's header — not building new fetch/cache/display logic from scratch.
- **AC2's specific requirement** ("link out to the corresponding GitHub Actions run/check
  page") is *not* fully satisfied by the existing `VcsWidgetGithubRow` link, which points at
  `github.prUrl` (the PR itself), not specifically the checks tab. `<prUrl>/checks` is a
  valid GitHub URL pattern and the likely minimal fix, but no existing code currently
  constructs it — flag as a small, genuinely new piece of work for the plan phase.
- **Unit test coverage (AC8)** for `ci_passing`: place alongside existing `pkg/classifier`
  rule tests (matchesRule/Criteria patterns already have precedent in
  `pkg/classifier/classifier_test.go`). E2E badge-rendering coverage: new spec under
  `tests/e2e/`, per `.claude/rules/e2e-test-conventions.md`.

## Summary of confirmed facts for the plan phase

| Question | Answer | Evidence |
|---|---|---|
| Live rule engine | `pkg/classifier.RuleBasedClassifier` | Instantiated `session_service.go:296`, wired `approval_handler.go:122` |
| Dead code | `session/approval_policy.go` + `session/approval_automation.go` | No external callers found repo-wide; only self- and test-referenced |
| CI status caching | Already proactive, background-polled (`PRStatusPoller`, 60s, ETag-conditional) | `session/pr_status_poller.go` |
| CI status transport to frontend | Already flows through `WatchSessions`/EventBus | `server/dependencies.go:585-587`, `Session.github_check_conclusion` field 38 |
| CI status storage on Instance | Already exists | `session/instance.go:206-207` |
| Frontend badge component | Already exists, just not used in DiffViewer | `VcsWidgetGithubRow.tsx`, used by `VcsPanel`/`SessionCard`/`SessionRow`/`SessionDetailView` |
| `matchesRule` ctx plumbing | Currently payload-only; `ctx` dropped before `classifySingle` | `pkg/classifier/classifier.go:495,506,679` |
| "Block manual approve" hook point | `ApprovalService.ResolveApproval`, before `approvalStore.Resolve` | `server/services/approval_service.go:46-75` |
