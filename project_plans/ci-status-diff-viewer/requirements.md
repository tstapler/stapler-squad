# Requirements: CI/CD Status in Diff Viewer

item_id: 3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d
Source: https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/37 (migrated), labels `enhancement`, `p0`

## Problem

The per-session diff viewer (`web-app/src/components/sessions/DiffViewer.tsx`) shows
code changes for a session's worktree branch but has no visibility into whether GitHub
Actions CI is passing on that branch/PR. Reviewers can approve a diff that is about to
fail CI, defeating the purpose of the review queue gate. Competitor tools (Aizen, Emdash,
Symphony) all surface CI status inline with the diff/review flow.

## Goals

1. Surface GitHub Actions CI status (passing / failing / pending / no-checks) for a
   session's branch inline in the diff viewer header.
2. Let reviewers optionally block manual approval when CI is red, as a configurable rule
   (not a hard-coded always-on gate — some sessions have no PR/CI at all, e.g. one-off
   sessions).
3. Extend the existing auto-approve rule engine with a `ci_passing` condition so rules can
   require green CI before auto-approving, alongside existing regex/command conditions.
4. Keep CI status reasonably fresh without polling storms — piggyback on existing
   real-time infra rather than inventing new transport.

## Non-goals

- Do not build a full CI dashboard/log viewer — a status badge (state + link out to the
  GitHub Actions run) is sufficient; no in-app log tailing.
- Do not add GitLab CI support — item explicitly scopes to GitHub Actions (GitLab is
  mentioned only as competitor context, not a requirement).
- Do not change how PRs are created/merged — this is read-only status surfacing.
- Webhook-driven push updates are explicitly optional ("real-time polling **or**
  webhook-driven") — polling reusing existing infra satisfies the requirement; a GitHub
  webhook receiver is out of scope unless polling proves insufficient.

## Existing building blocks (confirmed via codebase research)

- **Diff viewer**: `web-app/src/components/sessions/DiffViewer.tsx:11`, backend RPC
  `SessionService.GetSessionDiff` (`server/services/session_service.go:2586`, proto
  `proto/session/v1/session.proto:35`).
- **GitHub CI status already fetched elsewhere in this codebase**:
  - `github/client.go:101` — `StatusCheckRollup` from `gh pr view --json statusCheckRollup`,
    `getCheckConclusion()` at `github/client.go:335`.
  - `github/user_pr_cache.go:563-631` — GraphQL query for `StatusCheckRollup.State`,
    normalized via `normalizeCheckState`.
  - `session/backlog_plugin_github_prs.go:46-49` — `githubCheckRun{Conclusion string}`,
    fetched via GitHub Check Runs API (`fetchCILabel`, concurrency-bounded).
  - `session/instance.go:210` — `LastPRStatusCheck time.Time` already exists on
    `Instance`, surfaced to proto via `server/adapters/instance_adapter.go:70`.
  - This means CI status fetching does **not** need to be built from scratch — it needs
    to be consolidated/reused and exposed on the diff viewer's data path specifically.
- **Branch/PR association**: `Instance.Branch` (`session/instance.go:114-115`),
  `GitHubPRNumber` (`session/instance.go:176,490`), `HasAssociatedPR()` (`:740`).
- **Review/approval flow**: `Instance.Approve()` (`session/instance_state.go:292`),
  `ReactiveQueueManager.handleApprovalResponse` (`server/review_queue_manager.go:300-312`),
  queue membership decided in `session/review_queue_determiner.go` — candidate hook point
  for a "block approval when CI red" rule.
- **Auto-approve rules**: persisted `RuleSpec` (`server/services/rules_store.go:20-49`),
  runtime `CommandCriteria`/`Rule` (`pkg/classifier/classifier.go:167-198,343`), evaluated
  in `RuleBasedClassifier.classifySingle`/`matchesRule` (`:506,679`). A separate, possibly
  dead, `ApprovalPolicy`/`PolicyCondition` engine exists at `session/approval_policy.go:12-34`
  — research phase must confirm which engine is actually live before adding `ci_passing` to
  either one, to avoid wiring a condition into unused code.
- **Real-time transport to reuse**: `WatchSessions` / `WatchReviewQueue` server-streaming
  RPCs backed by `ReactiveQueueManager` + seq-based EventBus
  (`proto/session/v1/events.proto:24-31`) — CI status changes should publish through this,
  not a new SSE/websocket channel.

## Acceptance Criteria

1. The diff viewer header shows a CI status badge (passing / failing / pending / no
   checks found) for the session's branch when the session has an associated PR.
2. The badge links out to the corresponding GitHub Actions run/check page.
3. CI status is fetched via the existing `gh`/GitHub API integration (no new external
   dependency), reusing/consolidating existing check-status fetch code rather than adding
   a parallel implementation.
4. CI status updates reach the frontend via the existing `WatchSessions`/`WatchReviewQueue`
   streaming/event-bus infra (or documented equivalent), not a newly introduced polling
   loop on the frontend.
5. A configurable rule (default: off) blocks the manual "Approve" action in the review
   queue when CI status for the session's branch is failing; the block is visibly
   explained in the UI (not a silent no-op).
6. The auto-approve rule engine (whichever engine is confirmed live) supports a
   `ci_passing` condition that can be combined with existing conditions (e.g. regex
   command match AND CI passing) before a rule auto-approves a session.
7. Sessions with no associated PR (one-off sessions, directory sessions without a
   worktree/branch) show no CI badge (not an error state) and are unaffected by the
   CI-blocking rule.
8. Feature has unit test coverage for the new `ci_passing` rule condition and an e2e test
   (per `.claude/rules/e2e-test-conventions.md`) covering the CI badge rendering states.
9. Session creation/session-type registry touchpoints
   (`.claude/rules/session-creation-registry.md`) are unaffected — confirm no new session
   type is introduced (this feature only augments existing diff-viewer/review-queue UI).
10. Feature registry entries added per `.claude/rules/feature-registry.md` for the new RPC
    field(s)/UI badge, and `make registry-generate` run with no unexplained coverage-gap
    increase.

## Open Questions (for research/plan phases)

- Which approval-rule engine is actually live: `pkg/classifier` (`RuleSpec`/`CommandCriteria`)
  or `session/approval_policy.go`'s `PolicyEngine`? (Flagged above — confirm in research.)
- Poll interval / caching strategy for GitHub API calls to avoid rate-limit exhaustion
  across many concurrent sessions polling the same repo's Actions API.
- Should CI status be fetched lazily (only when diff viewer is open) or proactively
  for all sessions with a PR (needed if the "block approval" rule must reflect current
  CI state before the user opens the diff viewer)?
