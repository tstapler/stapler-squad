# Implementation Plan: PR Panel Actions & Extended Information

**Project:** pr-panel-actions  
**Complexity:** 3  
**Date:** 2026-10-19

---

## Domain Glossary

| Term | Definition |
|---|---|
| UserPR | Proto message in `proto/session/v1/types.proto` representing one open PR authored by the authenticated user; served via `GitHubUserService`. |
| PRInfo | Go struct in `github/client.go` carrying full PR metadata fetched via `gh pr view`; the source of truth for fields not yet in `UserPR`. |
| GitHubUserService | ConnectRPC service in `proto/session/v1/github_user.proto` / `server/services/github_user_service.go` that serves the PR panel without needing a session. |
| GitHubService | Separate ConnectRPC service whose existing `MergePR`/`ClosePR` are **session-scoped** (require a linked session). Not modified by this feature. |
| UserPRCache | In-process cache (`github/user_pr_cache.go`) that polls the GitHub GraphQL API in the background; feeds `WatchUserPRs`. |
| graphQLPRNode | Go struct mirroring the GraphQL PR node returned by the poll query; must be extended with new fields. |
| node_id | GitHub's opaque global node ID (e.g. `PR_kwDOA…`), required by `convertPullRequestToDraft` / `markPullRequestReadyForReview` GraphQL mutations. Not yet present in `UserPR` or `graphQLPRNode`. |
| PRCard | React component at `web-app/src/components/unfinished/prs/PRCard.tsx` rendering one PR in the panel. |
| PRActionBar | New sub-component to be extracted from PRCard if the `prActions` section exceeds ~50 lines (UX agent recommendation). |

---

## Tech Debt Disposition

All three touched modules are **Extend as-is**:

- **`github/user_pr_cache.go`** (1,243 lines, high churn): additions are isolated to four non-entangled sites (GraphQL query string, struct, mapping block, proto conversion). No refactor-first required.
- **`server/services/github_user_service.go`** (551 lines): consistent RPC pattern; new methods fit cleanly.
- **`web-app/src/components/unfinished/prs/PRCard.tsx`** (247 lines, low churn): extend directly; extract `PRActionBar` if the `prActions` section exceeds 50 lines.

---

## Epic / Story Breakdown

### Epic 1 — UserPR Extended Fields (Proto + Backend)

**Goal:** Propagate `body`, `labels`, `additions`, `deletions`, `changed_files`, `requested_reviewers`, and `node_id` through the data pipeline so the frontend has them.

#### Story 1.1 — Add `node_id` to GraphQL poll query and `UserPR` proto

**Why `node_id` first:** Draft-toggle mutations require it; nothing else unblocks until it's in `UserPR`.

Tasks:
- `proto/session/v1/types.proto`: add `string node_id = 26` to `UserPR`.
- `github/client_graphql.go`: add `id` field to `graphQLPRInfoQuery` and `prInfoGraphQLNode` struct.
- `github/user_pr_cache.go`: add `id` to `graphQLPRNode`; map to `UserPR.NodeID` in `userPRsToProto()`.
- Run `make proto-gen` (do not commit `gen/`).
- Unit test: mock GraphQL response includes `id`; assert `UserPR.NodeId` is set.

#### Story 1.2 — Add body, labels, stats, and requested-reviewers fields

Tasks:
- `proto/session/v1/types.proto`: add fields 27–33:
  - `string body = 27`
  - `repeated string labels = 28`
  - `int32 additions = 29`
  - `int32 deletions = 30`
  - `int32 changed_files = 31`
  - `repeated string requested_reviewer_logins = 32`
- `github/client_graphql.go` / `userPRQueryPrefix`: extend the GraphQL query with `body`, `labels(first:20){nodes{name}}`, `additions`, `deletions`, `changedFiles`, `reviewRequests(first:10){nodes{requestedReviewer{...on User{login}}}}`.
- `github/user_pr_cache.go` (`graphQLPRNode` struct, `userPRsToProto()`): map all new fields.
- Run `make proto-gen`.
- Unit test: mock response includes all new fields; assert they appear in the streamed `UserPREvent`.

---

### Epic 2 — Session-less PR Action RPCs (Backend)

**Goal:** Implement four new RPCs in `GitHubUserService` that accept `(owner, repo, number, host)` directly — no session required.

#### Story 2.1 — MergePRDirect and ClosePRDirect RPCs

Tasks:
- `proto/session/v1/github_user.proto`: add to `GitHubUserService`:
  ```proto
  rpc MergePRDirect(MergePRDirectRequest) returns (MergePRDirectResponse) {}
  rpc ClosePRDirect(ClosePRDirectRequest) returns (ClosePRDirectResponse) {}
  ```
  Message definitions:
  ```proto
  message MergePRDirectRequest {
    string owner  = 1;
    string repo   = 2;
    int32  number = 3;
    string host   = 4;  // default "github.com"
    optional string method = 5;  // "merge"|"squash"|"rebase", default "merge"
  }
  message MergePRDirectResponse { bool success = 1; string message = 2; }
  message ClosePRDirectRequest { string owner = 1; string repo = 2; int32 number = 3; string host = 4; }
  message ClosePRDirectResponse { bool success = 1; string message = 2; }
  ```
- `server/services/github_user_service.go`: implement `MergePRDirect` and `ClosePRDirect`:
  1. Validate non-empty fields.
  2. `ctx = github.WithGitHubCallOrigin(ctx, github.OriginInteractive)`.
  3. Call `github.MergePR(ctx, ref, number, method)` / `github.ClosePR(ctx, ref, number)`.
  4. On success: `s.cache.Refresh(ctx)` to push updated state to `WatchUserPRs` subscribers.
- Run `make proto-gen`; run `make registry-generate`.
- Unit tests for both RPCs (mock `github.MergePR`/`ClosePR`; assert `cache.Refresh` is called).

#### Story 2.2 — ConvertPRToDraft and MarkPRReady RPCs (GraphQL mutations)

Tasks:
- `github/client_graphql.go`: implement `ConvertPRToDraft(ctx, ref, nodeID, host string) error` and `MarkPRReadyForReview(ctx, ref, nodeID, host string) error` using GraphQL mutations:
  - `convertPullRequestToDraft(input:{pullRequestId:"<nodeID>"})` 
  - `markPullRequestReadyForReview(input:{pullRequestId:"<nodeID>"})`
  - Use `newGHGraphQLRequestForHostWithToken` (already exempted from `norawghrequest` analyzer).
- `proto/session/v1/github_user.proto`: add:
  ```proto
  rpc ConvertPRToDraft(ConvertPRToDraftRequest) returns (ConvertPRToDraftResponse) {}
  rpc MarkPRReady(MarkPRReadyRequest) returns (MarkPRReadyResponse) {}
  ```
  Both take `(owner, repo, number int32, host, node_id string)` — `node_id` carried through from `UserPR.node_id`.
- `server/services/github_user_service.go`: implement both RPCs; call `s.cache.Refresh(ctx)` on success.
- Unit tests: assert GraphQL request body and that `cache.Refresh` is called.
- Feature flag gate: `github:pr-panel-actions` controls all four new RPCs (off by default; validated before flip).

---

### Epic 3 — PRCard Extended Info UI (Frontend)

**Goal:** Display body preview, labels, change stats, and reviewer list on PRCard.

#### Story 3.1 — Body preview + labels + stats collapsed section

Tasks:
- `web-app/src/components/unfinished/prs/PRCard.tsx`: add a `<PRExpandedInfo>` sub-component rendered with `aria-expanded`/`aria-controls` (same pattern as `FailingChecks`). Collapsed by default.
  - Body: first 300 characters of `pr.body`, "Show more" link if truncated.
  - Labels: chips in `styles.chipNeutral` style, one per label.
  - Stats: `+{pr.additions} / -{pr.deletions} / {pr.changedFiles} files` as a stat line.
  - Requested reviewers: avatar/login list with pending indicator.
- `web-app/src/components/unfinished/prs/PRCard.css.ts`: add styles for the expandable section.
- Only render `<PRExpandedInfo>` when `pr.detailsLoaded` is true (prevents flash on first render before fields arrive).
- Unit test: renders with body/labels/stats when `pr.detailsLoaded` is true; collapses by default.

---

### Epic 4 — PRCard Action Buttons UI (Frontend)

**Goal:** Add Merge, Close, Mark Ready, Convert to Draft buttons to PRCard.

#### Story 4.1 — Close PR action button

Tasks:
- `web-app/src/components/unfinished/prs/PRCard.tsx`: add "Close PR" button to `prActions` div.
  - Disabled while `closingInFlight` (local `useState<boolean>`).
  - On click: show confirmation dialog (portal-rendered modal, `role="dialog"`, Cancel auto-focused, danger button). Pattern from `DeleteSessionConfirmModal`.
  - On confirm: call `ClosePRDirectSchema` RPC via `GitHubUserService` client. Inflight state set during call.
  - On success: fire a `ToastStack` toast "PR #N closed"; call `onNothingToFix?.()` to trigger refetch.
  - On error: show inline `role="alert"` error inside the card.
- `web-app/src/components/unfinished/prs/PRCard.css.ts`: close button styles (danger color from `vars.color.error`, 44 px min-height).
- Unit test: button visible on open non-draft PR; confirmation dialog renders; success removes error state; error renders inline.

#### Story 4.2 — Merge PR action button with method selector

Tasks:
- `web-app/src/components/unfinished/prs/PRCard.tsx`: add "Merge" button + method selector.
  - Method selector: `<select>` with options merge/squash/rebase; selected value persisted to `localStorage` key `pr-panel-merge-method`.
  - No confirmation dialog (non-destructive; PR is closed, not deleted). Instead: button switches to "Merging…" disabled state immediately.
  - On success: toast "PR #N merged"; `onNothingToFix?.()`.
  - On error: inline `role="alert"`.
  - Only shown on open non-draft PRs.
- Unit test: button present on open non-draft; absent on draft; method selector persists to localStorage.

#### Story 4.3 — Draft toggle action buttons

Tasks:
- `web-app/src/components/unfinished/prs/PRCard.tsx`: add "Mark as ready" (on draft PRs) / "Convert to draft" (on open non-draft PRs).
  - No confirmation dialog (reversible).
  - Inflight disabled state.
  - On success: optimistic update to `pr.isDraft`; `onNothingToFix?.()` for cache refresh.
  - Feature flag: only render if `github:pr-panel-actions` is enabled.
- Unit test: "Mark as ready" on draft PR; "Convert to draft" on non-draft; neither on merged/closed.

---

### Epic 5 — Feature Flag, Registry, and Validation

Tasks:
- `server/services/feature_flag_service.go`: register `github:pr-panel-actions` in `knownFeatureFlags`.
- `make registry-generate`: update feature registry for new RPCs.
- Integration test: `MergePRDirect` with real `gh` CLI mock; assert `WatchUserPRs` stream sends updated PR state after `Refresh`.
- E2E smoke test (manual, not automated): merge a PR via the panel on a test repo; verify PR disappears within one poll cycle.
- `jscpd` / `dupl` gates: run `make ready` and fix any new violations before shipping.

---

## Migration Plan

No data migration required. All new proto fields are optional / zero-valued for PRs where the cache hasn't polled yet (`detailsLoaded = false` guards the UI). The four new RPCs are additive.

---

## Observability Plan

- Log `github:pr-panel-actions` RPC calls at `INFO` level with `owner/repo#number` (no body content).
- Rate-limit errors from merge/close/draft RPCs: log at `WARN` with `call_origin=interactive`.
- Counter metric: `github_pr_action_total` with labels `action=merge|close|to_draft|to_ready` and `success=true|false`. Add to `github/telemetry_transport.go` call-site counters.

---

## Risk Control

| Risk | Mitigation |
|---|---|
| node_id missing from UserPR stalls draft mutations | Story 1.1 delivers node_id first; draft RPCs (Story 2.2) depend on Story 1.1. |
| Stale PR card after merge/close (2-min poll gap) | Story 2.1 calls `s.cache.Refresh(ctx)` immediately after action; frontend removes card optimistically on success. |
| Double-tap fires two RPCs on mobile | `inflight` boolean per action button; button disabled while RPC is in flight. |
| GitHub secondary rate limit on rapid actions | `rateLimitTransport` fail-fast already in place; actions use `OriginInteractive` which has reserved headroom. |
| ConvertPRToDraft requires node_id not always present | Guard: RPC handler returns `FailedPrecondition` if `node_id` is empty; frontend disables button if `pr.nodeId` is falsy. |
| jscpd threshold breach from new test mocks | Run `make ready-duplication-gate-web` locally before PR; action components will have similar mock patterns. |

---

## Unresolved Questions (Open after Phase 2 Research)

1. **Feature flag rollout**: Should the four new action RPCs be gated behind `github:pr-panel-actions` (off by default)? **Recommendation:** Yes — this allows production deployment before UI is polished; plan above includes the flag gate.
2. **Lazy vs eager extended fields**: Should `body`, stats, and reviewers be fetched lazily on expand or eagerly in the poll? **Recommendation:** Eagerly in the poll (negligible GraphQL cost per research/architecture.md); `detailsLoaded` guards the UI.
3. **`gh` CLI for draft toggle vs GraphQL**: Research confirms `gh pr convert-to-draft` is not a CLI command; GraphQL is the only option.
