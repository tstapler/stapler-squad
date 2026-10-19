# PR Panel Actions — Pitfalls & Risks

## 1. Rate limiting: mutations consume quota with no ETag protection

`rateLimitTransport.RoundTrip` (`github/http_client.go:78`) fails fast when
`DefaultRateLimiter.IsLimited()` is true, and `AdmitOrigin` can throttle
background callers to reserve 10% headroom for interactive traffic
(`backgroundHeadroomPercent = 10`, `rate_limit.go:37`).

**What could go wrong:** Unlike `GetPRInfoConditional`'s GET + `If-None-Match`
path (which GitHub answers with a cost-0 `304` when unchanged), every
merge/close/draft-toggle is a write (POST or GraphQL mutation) — no ETag
mechanism exists, so every call charges against the primary quota. If a user
rapidly re-clicks (or a frontend bug fires two concurrent requests), each fires
a full-cost mutation. At GitHub's standard 5000/hr PAT limit that's not
catastrophic in isolation, but in a multi-account workspace where the
background PR poller is also drawing on the same token, repeated interactive
mutations can accelerate a secondary-rate-limit `429` (burst). The new RPCs
should be annotated with `CallOriginInteractive` (or the equivalent origin
label) so `AdmitOrigin`'s headroom reservation logic gives them priority, but
this does not prevent burst violations.

**Mitigation needed:** Disable action buttons in the UI for the duration of the
RPC (inflight guard, see §4) to prevent accidental repeat calls.

## 2. Stale state: 2-minute polling gap after mutation

`UserPRCacheConfig.PollInterval` defaults to `2 * time.Minute`
(`user_pr_cache.go:177`). After the server executes a merge or close, the
`WatchUserPRs` stream will not reflect the PR's new state until the next
periodic poll fires — up to 2 minutes later. During that window the PR card
shows the pre-action state (still open/unmerged), and may reappear for users
who navigate away and back.

`UserPRCache.Refresh(ctx)` (`user_pr_cache.go:620`) triggers an immediate
coalesced fetch and will propagate a "removed" or "updated" event to all
WatchUserPRs subscribers. The new action RPCs **must** call `cache.Refresh(ctx)`
after a successful mutation, or optimistically emit a synthesized
"removed"/"updated" event to the stream.

Without one of those two mechanisms, the PR card will sit stale and the action
buttons will remain enabled, which can confuse the user into thinking the
action failed.

## 3. Proto field addition: generated output is gitignored, make build required

Fields to be added (body, labels, additions, deletions, changed_files,
requested_reviewers) extend `UserPR` beyond its current field 25
(`types.proto:1815`). Proto codegen output lands in `gen/` which is
gitignored (per CLAUDE.md), so a fresh worktree that has only the schema
change committed will fail to compile until `make build` (which runs
`proto-gen`) is re-run. CI's `make build` step handles this automatically, but
developers pulling the branch must run `make build` before editing Go files
that reference the new fields — otherwise `go build ./...` fails with
"undefined: session.v1.UserPR.Body" before `make build` has regenerated.

**No blocking risk for the overall build pipeline**, but reviewers who run
`go build` directly (instead of `make build`) will hit confusing compile
errors. The PR description should note the `make build` requirement.

## 4. Confirmation dialogs: no inflight guard in PRCard — double-tap fires two RPCs

`PRCard.tsx` currently has no pending/loading state for action buttons (the
file has no `isLoading`, `isPending`, or `disabled` flag tied to an in-flight
RPC). On a mobile browser with a laggy connection, a user can double-tap a
merge button and dispatch two `MergePRDirect` RPCs before the first one
returns. GitHub's merge endpoint is idempotent only in the sense that a
second merge on an already-merged PR returns an error, not a double-merge —
but `close` on an already-closed PR also returns an error, so the UX will
show an error toast on the second tap regardless of outcome.

**Required fix:** Each action button needs a local `useState<boolean>` inflight
flag, set to `true` on click and cleared on RPC completion (success or error),
with `disabled={inflight}` on the button. This is especially important for
mobile where `pointer-events: none` on `disabled` prevents the second touch
from registering mid-flight.

## 5. GraphQL draft mutations require a PR node ID — not present anywhere in the stack

GitHub's `convertPullRequestToDraft` and `markPullRequestReadyForReview`
mutations take a `pullRequestId: ID!` argument, which is the PR's opaque
global node ID (e.g. `PR_kwDOA...`), not its numeric number.

**Current state:**
- `PRInfo` struct (`github/client.go:66`) has no `NodeID` field.
- `graphQLPRInfoQuery` (`github/client_graphql.go:27`) does not fetch `id`.
- `prInfoGraphQLNode` has no `ID` field.
- `UserPR` proto (`proto/session/v1/types.proto:1790`) has no `node_id` field.

**Multi-point gap:** To support draft mutations, the following touchpoints must
all be updated in lock-step:
1. Add `id` to `graphQLPRInfoQuery`.
2. Add `ID string` to `prInfoGraphQLNode` and populate it in `mapPRInfoGraphQLNode`.
3. Add `NodeID string` to `PRInfo`.
4. Add `node_id = 26` (or next available) to `UserPR` proto and regenerate.
5. Populate `NodeID` in the cache's `toProto`/`toUserPR` mapping.
6. Thread `NodeID` through to the `ConvertPRToDraft`/`MarkPRReady` handlers.

REST alternative: `PATCH /repos/{owner}/{repo}/pulls/{pull_number}` accepts
`{"draft": true}` on github.com but has historically been unavailable or
undocumented on some GHES versions. The task description already calls out
GraphQL as required; the node-ID gap must be closed before GraphQL mutations
are implementable.

## 6. norawghrequest rule: GraphQL mutations must use newGHGraphQLRequestForHostWithToken

The `norawghrequest` lint rule (`tools/lint/norawghrequest`, `.claude/rules/norawghrequest.md`)
flags any `http.NewRequest`/`http.NewRequestWithContext` call whose URL
resolves to `GhBaseURL()`/`RestBaseURLForHost()` outside the approved
constructors. **The approved constructors are** (from `http_client.go`):
- `NewConditionalRequest` / `NewConditionalRequestNoCache` — GET reads with optional ETag
- `newGHRequestForHostWithToken` / `newGHRequestForHostWithTokenAndBody` — REST reads/writes
- `newGHGraphQLRequestForHostWithToken` — GraphQL (POST); **exempted by function-declaration containment**

GraphQL mutations (which are POST requests against the GraphQL endpoint) do
not use ETags and must go through `newGHGraphQLRequestForHostWithToken`. That
constructor is already exempted from the analyzer's scope (`norawghrequest.md`
states exemption by function-declaration containment). REST mutations
(merge/close via `PATCH /repos/…/pulls/…`) must use
`newGHRequestForHostWithTokenAndBody`. Using either of these constructors is
compliant and will not trip the analyzer; a bare `http.NewRequestWithContext`
against a GitHub URL will fail `make lint`.

**Existing precedent:** `PostIssueComment` in `github/pr_comments.go:31`
already uses `newGHRequestForHostWithTokenAndBody` — follow that pattern for
the REST mutation RPCs.
