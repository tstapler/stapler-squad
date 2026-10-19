# Architecture Research: PR Panel Actions

## 1. GitHubUserService — structure and registration

File: `server/services/github_user_service.go` (551 lines)

`GitHubUserService` is a plain struct that holds a `*githubpkg.UserPRCache` and a list of enterprise hosts. It implements the generated `sessionv1connect.GitHubUserServiceHandler` interface (checked at compile time via a blank-identifier assignment on line 20). Every RPC method follows the same shape:

```go
func (s *GitHubUserService) <RPC>(
    ctx context.Context,
    req *connect.Request[sessionv1.<Req>],
) (*connect.Response[sessionv1.<Resp>], error)
```

Streaming RPCs (WatchUserPRs) use `*connect.ServerStream[sessionv1.UserPREvent]` as the last parameter.

**Registration** (`server/server.go` lines 568–581): The handler is registered once in `(*Server).wireRoutes` via `sessionv1connect.NewGitHubUserServiceHandler(deps.GitHubUserService, ...)`. The result is mounted under `/api` prefix. WatchUserPRs is additionally bridged over WebSocket via `services.NewStreamingWSBridge`. Adding the four new RPCs (MergePRDirect, ClosePRDirect, ConvertPRToDraft, MarkPRReady) requires:
1. Declare them in `proto/session/v1/github_user.proto` with request/response messages.
2. Run `make proto-gen` to regenerate the Connect handler interface.
3. Implement them on `GitHubUserService`.
4. No server.go changes needed — the existing `NewGitHubUserServiceHandler` call automatically picks up the new methods once the generated interface grows them.

## 2. UserPR data pipeline — where new fields slot in

### Current pipeline

```
GitHub GraphQL API
  └─ userPRQueryPrefix + mode fields (user_pr_cache.go:852–924)
  └─ graphQLPRNode (struct, lines 964–1001)
  └─ fetchUserPRsForToken() → []UserPR (lines 1137–1222)
  └─ publish() → atomic snapshot
  └─ Subscribe() channel → WatchUserPRs handler
  └─ userPRsToProto() (github_user_service.go:426–478) → []*sessionv1.UserPR
  └─ WatchUserPRs/ListUserPRs stream
  └─ PRCard.tsx (reads proto-generated TypeScript types)
```

### Fields already available in the github package but not in UserPR/proto

`github.PRInfo` (populated by `GetPRByNumber`, used for session-scoped PR fetches) already has `Body`, `Labels`, `Additions`, `Deletions`, `ChangedFiles`, and `Reviews` — but these come from `gh pr view --json` (a subprocess call), not from the background GraphQL poll.

### GraphQL query additions needed for body/labels/stats/reviewers

The background poll query (`userPRQueryPrefix` in `user_pr_cache.go:852`) currently fetches:
`number`, `title`, `url`, `headRefName`, `baseRefName`, `state`, `isDraft`, `updatedAt`, `closedAt`, `mergedAt`, `repository { owner { login } name }`, `reviewDecision`, `reviews(last:20, states:[APPROVED,CHANGES_REQUESTED]) { nodes { state } }` plus mode-dependent fields (mergeable, reviewThreads, commits/statusCheckRollup).

To add the new fields the following GraphQL fields must be appended to `userPRQueryPrefix` (or as a new const block):
- `body` — PR description
- `labels(first: 20) { nodes { name } }` — labels
- `additions`, `deletions`, `changedFiles` — diff stats (available on PullRequest in GitHub GraphQL v4)
- `reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } ... on Team { name } } } }` — requested reviewers

Each requires:
1. Add JSON fields to `graphQLPRNode` struct.
2. Add corresponding fields to `github.UserPR` struct.
3. Map them in `fetchUserPRsForToken()`'s PR construction block (lines 1192–1219).
4. Add fields to `proto/session/v1/types.proto`'s `UserPR` message (currently ends at field 25).
5. Map them in `userPRsToProto()`.
6. Consume in `PRCard.tsx`.

**Cost note**: `body`, `labels`, `additions`, `deletions`, `changedFiles` are cheap scalar/flat-list fields — negligible GraphQL complexity cost. `reviewRequests` adds a connection fetch but is capped (first:10). The three query modes (widened/degraded/legacy) do not gate these fields — they should go into `userPRQueryPrefix` so all modes get them.

## 3. Session-scoped MergePR/ClosePR pattern → session-less replication

File: `session/pr_tracking.go`

`Instance.MergePR` and `Instance.ClosePR` both follow the same pattern:
1. Read `i.GitHub()` (a snapshot of the session's GitHub state: Owner, Repo, PRNumber, Host).
2. Build a `github.RepoRef` via `github.NewRepoRefWithHost(owner, repo, host)`.
3. Call the package-level `github.MergePR(ctx, ref, prNumber, method)` or `github.ClosePR(ctx, ref, prNumber)`.

For the session-less RPCs, steps 1–2 are replaced by reading (owner, repo, number, host) directly from the request proto — no session lookup needed. The underlying `github.MergePR` and `github.ClosePR` functions already accept a `RepoRef` independent of any session. `github.ConvertPRToDraft` and `github.MarkPRReady` do not yet exist; they need to be added to `github/client.go` using the GitHub GraphQL API (the REST API doesn't support draft conversion for PRs — it requires `convertPullRequestToDraft` / `markPullRequestReadyForReview` GraphQL mutations).

The session-less RPC implementation pattern:
```go
func (s *GitHubUserService) MergePRDirect(ctx context.Context, req *connect.Request[sessionv1.MergePRDirectRequest]) (*connect.Response[sessionv1.MergePRDirectResponse], error) {
    ctx = githubpkg.WithGitHubCallOrigin(ctx, githubpkg.OriginInteractive)
    ref, err := githubpkg.NewRepoRefWithHost(req.Msg.Owner, req.Msg.Repo, req.Msg.Host)
    if err != nil {
        return nil, connect.NewError(connect.CodeInvalidArgument, err)
    }
    if err := githubpkg.MergePR(ctx, ref, int(req.Msg.Number), req.Msg.Method); err != nil {
        return nil, connect.NewError(connect.CodeInternal, classifyGitHubRateLimitError(err))
    }
    // Trigger a cache refresh so WatchUserPRs subscribers see the updated state.
    _ = s.cache.Refresh(ctx)
    return connect.NewResponse(&sessionv1.MergePRDirectResponse{Success: true}), nil
}
```

`classifyGitHubRateLimitError` is already defined in `github_service.go` — import it or inline the same logic.

## 4. Data flow summary

```
GitHub GraphQL poll (every 2 min, via UserPRCache.fetchUserPRsForToken)
  → graphQLPRNode (add body/labels/additions/deletions/changedFiles/reviewRequests)
  → UserPR struct (add Body/Labels/Additions/Deletions/ChangedFiles/RequestedReviewers)
  → publish() → atomic snapshot
  → Subscribe() channels → WatchUserPRs server-streaming RPC
  → userPRsToProto() → sessionv1.UserPR proto (add new fields 26+)
  → Frontend TypeScript (proto-generated; add to PRCard.tsx display)

New session-less action RPCs (MergePRDirect / ClosePRDirect / ConvertPRToDraft / MarkPRReady):
  Frontend action button → ConnectRPC call → GitHubUserService handler
    → github.NewRepoRefWithHost(owner, repo, host)  [from request, not session]
    → github.MergePR / ClosePR / ConvertPRToDraft / MarkPRReady (github/client.go)
    → UserPRCache.Refresh(ctx)  [push updated state to WatchUserPRs subscribers]
```

## 5. Tech debt disposition for touched files

### github/user_pr_cache.go (1243 lines, 10 commits since 2026-01-01)
**High churn, moderate size.** It has had significant recent activity (rate-limit rework, tabbed Up Next, degraded-mode, GHE support). The new fields are purely additive changes to four separate sites (query string, graphQLPRNode struct, UserPR struct, fetchUserPRsForToken mapping) — each is a one-to-three line addition with no logic entanglement. **Extend as-is**: the additions are mechanical and localized, not structural.

### server/services/github_user_service.go (551 lines, 18 commits combined with user_pr_cache since 2026-01-01)
**Moderate churn.** The four new RPCs are each 15–25 lines (validate inputs, build RepoRef, call github package, trigger cache refresh, return response). The file's existing RPC methods have consistent structure — the new ones slot in cleanly. **Extend as-is**.

### web-app/src/components/unfinished/prs/PRCard.tsx (247 lines, 1 commit since 2026-01-01)
**Low churn.** The component is well-factored (small helper functions + one exported `PRCard`). Adding action buttons (Merge/Close/Draft toggle) requires adding new state (`useState` for confirmation or loading) and new button elements inside `prActions`. The PRInfo display (body/labels/stats) is additive. The component is not large enough to warrant a seam or split. **Extend as-is**, but be mindful of the `prActions` section growing — consider extracting a `PRActionBar` component if it reaches ~50 lines.

## 6. One decision needed before implementation

`ConvertPRToDraft` and `MarkPRReady` require GitHub GraphQL mutations (not REST). `github.MergePR` and `github.ClosePR` both use `gh` CLI subprocess calls (`gh pr merge`, `gh pr close`). The `gh` CLI supports `gh pr ready` (mark ready for review) but not `gh pr convert-to-draft`. Options:
- **gh CLI for MarkPRReady** (`gh pr ready <number> --repo <ref>`), raw GraphQL for ConvertPRToDraft — matching the existing client.go pattern for mutations.
- **Raw GraphQL mutation** (`convertPullRequestToDraft` + `markPullRequestReadyForReview`) for both — consistent approach, avoids subprocess, but requires building a new GraphQL mutation path (the existing code uses `newGHGraphQLRequestForHostWithToken` which is already wired).

The raw GraphQL path is recommended to avoid the subprocess + `gh` auth dependency for this case — the existing `newGHGraphQLRequestForHostWithToken` helper handles auth and host correctly.
