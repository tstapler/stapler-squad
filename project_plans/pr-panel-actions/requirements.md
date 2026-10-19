# Requirements: PR Panel Actions & Extended Information

**Project name:** pr-panel-actions  
**Complexity:** 3  
**Item ID:** 1d1cc332-0f88-41e5-b8cf-8b378fd9bec5

## Problem Statement

The "Open Pull Requests" panel in stapler-squad (Up Next tab) shows a list of the authenticated user's PRs with CI status, merge-conflict badges, and "+ Session" buttons. Users want to take direct actions on PRs from that panel — merge, close, convert to draft, mark ready for review — and see richer PR metadata (description preview, labels, reviewer list, change stats) without leaving the app.

Current limitations:
- Existing `MergePR` / `ClosePR` RPCs in `GitHubService` are **session-scoped** (require a linked session and `IsPRSession()` check). They cannot be called for a PR in the list that has no session.
- No `ConvertToDraft` or `MarkReadyForReview` capabilities exist anywhere in the codebase.
- `UserPR` proto type is missing: `body`, `labels`, `additions`, `deletions`, `changed_files`, reviewer list — all of which are already fetched by `PRInfo` in `github/client.go`.

## User Story

> As a developer using the stapler-squad PR panel on mobile/desktop, I want to see the PR description, labels, reviewer status, and change stats at a glance, and be able to merge, close, or toggle draft status directly from the panel — without needing to create a session or switch to github.com.

## Functional Requirements

### FR-1: Extended PR Information
- Show PR description/body (first 300 chars, expandable).
- Show labels as chips.
- Show change stats: `+N / -N / N files`.
- Show reviewer list (approved, changes-requested, pending).

### FR-2: Merge PR Action
- Merge button available on any open, non-draft PR (with or without a session).
- Support merge methods: merge, squash, rebase (selector).
- Confirm before merging (destructive).
- Post-merge: PR disappears from the list; show a success toast.

### FR-3: Close PR Action
- Close button on any open PR (with or without a session).
- Confirm before closing.
- Post-close: PR disappears from the list.

### FR-4: Draft Toggle
- "Mark as ready" button on draft PRs.
- "Convert to draft" button on non-draft, non-merged PRs.
- No confirmation dialog required (reversible action).
- Post-toggle: PR card updates its draft badge immediately.

## Non-Functional Requirements

- Actions must route through the `GitHubUserService` (not `GitHubService` session service) so they work without a session.
- Must use `NewConditionalRequest`/`newGHRequestForHostWithToken` pattern (not raw `http.NewRequest`) per `norawghrequest` rule.
- Must respect existing ETag/rate-limit patterns from `github/http_client.go`.
- Mobile-first: touch targets ≥ 44×44 px; confirmation dialogs must be accessible (ARIA, keyboard).
- Actions must use `github.WithGitHubCallOrigin(ctx, github.OriginInteractive)`.
- `ConvertToDraft` and `MarkReadyForReview` require GitHub GraphQL (no REST endpoint for these).

## Acceptance Criteria

1. `UserPR` proto type gains `body`, `labels` (repeated string), `additions`, `deletions`, `changed_files`, and `requested_reviewers` (repeated string) fields; the WatchUserPRs stream sends populated values.
2. PR cards in the panel display description preview, labels, change stats, and reviewer list when present.
3. A Merge button appears on open non-draft PRs (with method selector); clicking it prompts confirmation, calls a new session-less `MergePRDirect` RPC, and on success removes the PR from the list.
4. A Close button appears on open PRs; clicking it prompts confirmation, calls a new `ClosePRDirect` RPC, and on success removes the PR from the list.
5. A "Mark as ready" button appears on draft PRs; clicking it calls `MarkPRReady` and updates the card.
6. A "Convert to draft" button appears on open non-draft PRs; clicking it calls `ConvertPRToDraft` and updates the card.
7. All four new RPCs live in `GitHubUserService` (not `GitHubService`) and accept `(owner, repo, number, host)` rather than a session ID.
8. `MergePRDirect` and `ClosePRDirect` reuse the existing `github.MergePR` and `github.ClosePR` standalone functions in `github/client.go`.
9. `ConvertPRToDraft` and `MarkPRReady` are implemented via GitHub GraphQL mutations.
10. Existing `MergePR`/`ClosePR` session-scoped RPCs remain unchanged.
11. All new actions display an appropriate error message inline if the GitHub call fails.
12. Feature registry updated (`make registry-generate`); jscpd and dupl gates stay green.

## Open Questions

- Should action buttons be gated behind a `github:pr-panel-actions` feature flag (off by default, flip after validation) to allow safe rollout?
- For `ConvertPRToDraft` / `MarkPRReady`, should these use the existing GraphQL client in `github/client_graphql.go` or the `gh` CLI (`gh pr edit --draft`)?
- Should the extended `UserPR` fields (`body`, stats, reviewers) be fetched eagerly on every poll or lazily on card expand?
