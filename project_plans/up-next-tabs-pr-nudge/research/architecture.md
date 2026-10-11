# Architecture Research: up-next-tabs-pr-nudge

Evidence is from reading the cited files in this worktree. Line numbers are from `grep`/`sed` on 2026-10-05. Items marked INFERRED were not executed.

## 1. Frontend integration points
- `web-app/src/app/unfinished/UnfinishedTab.tsx` (192 lines) uses `useSearchParams` (l.34) for the `?item=` deep link (`focusItemId`). It renders `<StuckItemsSection focusItemId>` (l.160), then an "In Progress" `<section>` containing `<GitHubPRsSection />` (l.165) plus worktrees, then `<BacklogQueueSection />` (l.189). The tab shell is a small wrapper change.
- `web-app/src/components/unfinished/GitHubPRsSection.tsx` is 978 lines and holds about 10 components. Only `PRCard` (l.72) and `PRGroupedList` (l.809) concern PR display. The rest is unrelated account and auth UI: `AccountsBar` l.173, `DeviceAuthBanner` l.226, `AddAccountPanel` l.413, `CLIImportSection` l.494, `TokenAuthForm` l.591. The other pieces are `FilterBar` l.691, `applyFilterSort` l.734, `StatsBar` l.134, and the `GitHubPRsSection` container (l.853, `useState` for filter, sort and search).
- It builds its own `GitHubUserService` client via `useGitHubUserClient` (l.26) and reads PRs through `useGitHubPRs` (`web-app/src/lib/hooks/useGitHubPRs.ts`, 67 lines).
- `PRCard` shows only `sessionIds[0]` (see requirements). Showing all linked sessions is a card-local change.
- Churn: 3 commits in the last 90 days (`git log --since=90.days`). It is large but not high-churn, so it is a moderate hotspot by size, not by churn.

## 2. PR data pipeline (server)
- Proto `UserPR` (`proto/session/v1/types.proto:1769`) fields: owner, repo, number, title, html_url, state, head_ref, base_ref, is_draft, `check_conclusion`, approved_count, `changes_req_count`, timestamps, `session_ids` (16), `local_worktree_path` (17). It has no failing-check names and no unresolved-thread count. Next free field number is 18.
- Go mirror: `github/user_pr_cache.go:22` (`UserPR`). `UserPRCache` polls via a viewer GraphQL query (`fetchUserPRsForToken`, l.602, using `userPRGraphQLQuery` and `newGHGraphQLRequestForHostWithToken`). It supports multiple accounts and GHE hosts (`resolveAllLogins`, `collectAllTokens`). It pushes updates to subscribers (`Subscribe`, `SetOnUpdated`).
- The RPCs are served by `server/services/github_user_service.go`: `ListUserPRs` (l.65) returns the cache snapshot, and `WatchUserPRs` (l.80) streams it.
- That poll is a single GraphQL query. It does not use the ETag path. ETag (`github/etag_cache.go`, `GetPRInfoConditional`, `NewConditionalRequest*`) is used by per-PR polling in `session/pr_status_poller.go` for sessions.
- Extending the viewer query with `statusCheckRollup.contexts` (failing check names) and `reviewThreads(first:N){nodes{isResolved}}` or `reviewThreads.totalCount` adds fields to the existing request at no extra request cost, only query-cost points. INFERRED: `statusCheckRollup` is already read for `check_conclusion`, and GitHub GraphQL `reviewThreads.isResolved` exists. Not executed.
- Per-PR detail already exists in `github/client_graphql.go`: `GetPRInfoGraphQL` (l.205) with `mapPRInfoGraphQLChecks` (l.365) and `mapPRInfoGraphQLReviews` (l.341). A lazy on-click fetch could reuse it instead of widening the list poll.
- Unresolved-comment source of truth: review threads (`isResolved`) are the right signal, since review comments have no resolved state. This answers the open question in requirements.

## 3. Session to PR mapping
- `annotateUserPRCache` (`server/dependencies.go:1926`) fills `SessionIDs` and `LocalWorktreePath` in the `onUpdated` callback.
  - Sessions come from `PRStatusPoller.GetInstances()`, using `Snapshot()` fields.
  - Repo resolution falls back in four steps: DB owner/repo, stored PR URL, git remote, then `pr-<n>-` regex on the title.
  - Matching is by branch against `headRef`, with a PR-number fallback (`PRAnnotationSession`, `user_pr_cache.go:48`). Worktrees come from `unfinished.Scanner`.
- `session_ids` hold `inst.Title`, not a UUID. Check this against what `steer_session` expects.
- Consequence: all linked sessions are already available server-side. "Most recently active" needs session metadata that is not in `UserPR`. Either add a field or have the client sort using its session store.
- The known gaps are forks, renamed branches and paused worktrees, as noted in requirements.

## 4. Steering and nudge path
- MCP `steer_session` (`server/mcp/tools_terminal.go:147`, handler l.667) takes `session_id` and `message`. It caps length (`maxInputBytes`), strips NULs, calls `findInstance`, and uses `RunWithResume` for stopped OneShot sessions. Otherwise it sends keys to tmux.
- `diagnose_nudge_session` (`server/mcp/tools_diagnose.go:59`) adds the safeguards: it refuses if the target is no longer idle or its tmux pane identity can't be re-verified. This is the mid-turn and paused safeguard the requirements ask to reuse. It is role-gated to Diagnose & Nudge dispatch (`diagnose_role_gate.go`), so a UI RPC cannot call it directly. Extract the idle and pane-identity check into a shared service function.
- `AutoReopenForPRFix` (`server/services/backlog_service_triage.go:2247`, `autoReopenForPRFix` l.2260) takes a `fixContext` string. The active-session branch is `steerActiveSessionForPRFix` (`backlog_service_pr_fix_steer.go:245`). It uses `buildSteerMessage(program, fixContext)` (l.190), an in-flight guard (l.263), and an invariant refusal for `jules_work` (l.253). This is all keyed by backlog item ID, not PR or session.
- `fixContext` is built in `session/backlog_lifecycle_pr.go:1675`: `"PR #%d (%s) needs fixes:\n\n%s"` with `prStatus.FeedbackText`, which `git.PRStatus` produces. It is backlog-item shaped. The nudge prompt builder should be a new, small, pure function over `UserPR` plus the new check and thread details, reusing the `buildSteerMessage` per-program formatting where possible.
- Recommended new surface: a `NudgeSessionForPR` RPC on `GitHubUserService`, or on `SessionService`. It takes the PR key and a session ID. The server builds the prompt, runs the idle and pane checks, and sends it. Do not let the browser compose or send raw text through `write_to_session`.

## 5. Overlap with prior plans
- `project_plans/pr-fix-steering/research/architecture.md`: same steer primitive and `fixContext` reuse. Its scope is automatic, backlog-item sessions only. This project is the manual, PR-card-triggered counterpart. Reuse its dedup and backoff learnings, but the manual path needs no backoff gate.
- `project_plans/pr-review-followup/research/architecture.md`: `HasReviewFeedback` and `LatestFeedbackAt` on `PRStatus`, and the watermark pattern. It is relevant for deciding "unresolved" and for not re-nudging stale feedback.
- `project_plans/github-pr-status/research/architecture.md` and `project_plans/pr-comment-check-runs/research/architecture.md` cover the PR status and check-run data model. Read both before choosing the GraphQL fields. I did not open these two in full.
- No EventStorming table. The domain is not complex.

## 6. Tech-debt disposition for GitHubPRsSection.tsx
**Isolate via seam.** Do not do a full refactor first. Extract `PRCard` and `PRGroupedList` into their own files (for example `PRsTab`), leave the auth and account components in place, and build the nudge UI in the new module. A pure file split is cheap, keeps the test file `GitHubPRsSection.test.tsx` stable, and does not touch the 3-commits/90-day auth code.

## 7. Risks and recommendations
- Widening the viewer GraphQL query affects every poll across accounts and GHE hosts. Prefer adding `reviewThreads`/check names there only if the query cost stays low. Otherwise fetch lazily through `GetPRInfoGraphQL` on nudge.
- Do not use `http.NewRequest` for GitHub calls (`norawghrequest` lint). Use the existing constructors.
- Tab state: derive the initial tab as `?item=` → Stuck, else `?tab=`, else localStorage, else PRs. Keep `UnfinishedNavBadge` unchanged unless a decision says otherwise.
- Proto changes are additive (fields 18+). Run `make proto-gen` and `make registry-generate`.
