# Validation Plan: PR Panel Actions & Extended Information

**Project:** pr-panel-actions  
**Date:** 2026-10-19

---

## Test Coverage Matrix

| Requirement | Test Type | Test Case |
|---|---|---|
| FR-1: UserPR body, labels, stats in proto | Unit | `TestUserPRExtendedFields_MappedFromGraphQL` |
| FR-1: node_id propagated | Unit | `TestUserPRNodeID_SetFromGraphQL` |
| FR-1: Reviewer list propagated | Unit | `TestUserPRRequestedReviewers_MappedFromGraphQL` |
| FR-2: MergePRDirect RPC | Unit | `TestMergePRDirect_Success`, `TestMergePRDirect_InvalidArgs`, `TestMergePRDirect_RefreshCalledOnSuccess` |
| FR-2: Merge method selector | Unit | `TestMergePRDirect_DefaultMethod_Ismerge`, `TestMergePRDirect_Squash`, `TestMergePRDirect_Rebase` |
| FR-3: ClosePRDirect RPC | Unit | `TestClosePRDirect_Success`, `TestClosePRDirect_InvalidArgs`, `TestClosePRDirect_RefreshCalledOnSuccess` |
| FR-4: ConvertPRToDraft RPC | Unit | `TestConvertPRToDraft_Success`, `TestConvertPRToDraft_EmptyNodeID_ReturnsPreconditionFailed` |
| FR-4: MarkPRReady RPC | Unit | `TestMarkPRReady_Success`, `TestMarkPRReady_EmptyNodeID_ReturnsPreconditionFailed` |
| FR-1 (frontend): body/labels/stats render | Unit | `PRCard: renders expanded info when detailsLoaded=true` |
| FR-1 (frontend): collapsed by default | Unit | `PRCard: expanded info is collapsed by default` |
| FR-2 (frontend): Merge button present on open non-draft PR | Unit | `PRCard: merge button present on open non-draft PR` |
| FR-2 (frontend): Merge button absent on draft | Unit | `PRCard: merge button absent on draft PR` |
| FR-2 (frontend): inflight state disables button | Unit | `PRCard: merge button disabled during inflight` |
| FR-3 (frontend): Close with confirmation | Unit | `PRCard: close button shows confirmation dialog` |
| FR-3 (frontend): Close success → toast | Unit | `PRCard: close success fires toast and calls onNothingToFix` |
| FR-3 (frontend): Close error → inline alert | Unit | `PRCard: close error shows inline role=alert` |
| FR-4 (frontend): Mark Ready on draft PR | Unit | `PRCard: mark ready button on draft PR` |
| FR-4 (frontend): Convert to Draft on non-draft PR | Unit | `PRCard: convert to draft button on non-draft PR` |
| FR-4 (frontend): neither button on merged/closed | Unit | `PRCard: no draft toggle on merged PR` |
| AC-7: RPCs in GitHubUserService not GitHubService | Integration | `TestMergePRDirect_ServerRouting_GitHubUserService` |
| AC-8: Reuses github.MergePR/ClosePR | Unit | `TestMergePRDirect_DelegatesToGithubMergePR` |
| AC-10: Session-scoped MergePR/ClosePR unchanged | Unit | `TestMergePR_SessionScoped_StillRequiresSession` |
| AC-12: Feature registry updated | Static | `make registry-diff` output includes new RPCs |

---

## Unit Tests — Backend (Go)

### `github/client_graphql_test.go`

```
TestConvertPRToDraft_GraphQLMutationBody
  - mock GraphQL endpoint captures request body
  - assert mutation name is "convertPullRequestToDraft"
  - assert variable pullRequestId equals supplied node_id

TestMarkPRReadyForReview_GraphQLMutationBody
  - same pattern for "markPullRequestReadyForReview"

TestConvertPRToDraft_HostTokenSelection
  - assert newGHGraphQLRequestForHostWithToken is called with correct host
```

### `server/services/github_user_service_test.go`

```
TestMergePRDirect_Success
  - req: owner="tstapler", repo="logseq", number=1, host="github.com"
  - mock github.MergePR returns nil
  - assert response.Success=true
  - assert cache.Refresh called once

TestMergePRDirect_DefaultMethod
  - omit method field → assert github.MergePR called with method="merge"

TestMergePRDirect_EmptyOwner_ReturnsInvalidArgument
TestMergePRDirect_EmptyRepo_ReturnsInvalidArgument
TestMergePRDirect_ZeroNumber_ReturnsInvalidArgument

TestClosePRDirect_Success
TestClosePRDirect_EmptyOwner_ReturnsInvalidArgument
TestClosePRDirect_RefreshCalledOnSuccess

TestConvertPRToDraft_Success
TestConvertPRToDraft_EmptyNodeID_ReturnsFailedPrecondition
TestMarkPRReady_Success
TestMarkPRReady_EmptyNodeID_ReturnsFailedPrecondition
```

### `github/user_pr_cache_test.go`

```
TestUserPRCacheAnnotate_ExtendedFields
  - mock GraphQL response with body, labels, additions, deletions, changedFiles, reviewRequests
  - assert UserPR.Body, Labels, Additions, Deletions, ChangedFiles, RequestedReviewerLogins set

TestUserPRCacheAnnotate_NodeID_Set
  - mock GraphQL response with id="PR_kwDO123"
  - assert UserPR.NodeId="PR_kwDO123"
```

---

## Unit Tests — Frontend (Jest)

### `web-app/src/components/unfinished/prs/PRCard.test.tsx`

```
PRCard extended info
  ✓ renders disclosure button when detailsLoaded=true
  ✓ body text visible after expanding
  ✓ body truncated at 300 chars with "Show more"
  ✓ labels render as chips
  ✓ stats line shows +N/-N/N files
  ✓ requested reviewers listed
  ✓ section collapsed by default

PRCard merge action
  ✓ merge button present on open non-draft PR
  ✓ merge button absent on draft PR
  ✓ merge button absent on closed/merged PR
  ✓ method select defaults to "merge"
  ✓ method select persists selection to localStorage
  ✓ button shows "Merging…" during inflight; disabled
  ✓ success → toast fired; onNothingToFix called
  ✓ error → inline role=alert shown inside card

PRCard close action
  ✓ close button present on open PR
  ✓ confirmation dialog rendered after click
  ✓ dialog has role=dialog; Cancel button auto-focused
  ✓ close fires ClosePRDirect RPC on confirm
  ✓ success → toast fired; onNothingToFix called
  ✓ error → inline role=alert inside card
  ✓ cancel dismisses dialog without RPC call

PRCard draft toggle
  ✓ "Mark as ready" on draft PR
  ✓ "Convert to draft" on open non-draft PR
  ✓ neither on merged/closed PR
  ✓ inflight disables button
```

---

## Integration Tests

```
TestMergePRDirect_E2E (server/services integration test)
  - spawn real GitHubUserService with mock github.MergePR
  - call MergePRDirect via ConnectRPC
  - assert WatchUserPRs stream emits updated snapshot within 2 poll cycles

TestClosePRDirect_E2E
  - same shape as above

TestConvertPRToDraft_E2E_NodeIDRequired
  - call ConvertPRToDraft with empty node_id
  - assert FailedPrecondition returned
```

---

## UX Acceptance Criteria

| ID | Criterion |
|---|---|
| UX-1 | All action buttons have min-height 44 px on mobile viewport (375 px wide). |
| UX-2 | Merge and close buttons are keyboard-reachable (Tab order in `prActions` div). |
| UX-3 | Confirmation dialog for Close is portal-rendered; focus trapped inside; Escape dismisses. |
| UX-4 | In-flight spinner/text visible on all action buttons during RPC call. |
| UX-5 | Success toast visible for ≥3 s; screen-reader-announced via `role="status"`. |
| UX-6 | Inline error message uses `role="alert"` for screen-reader announcement. |
| UX-7 | Body/labels/stats section is collapsed by default; disclosure button visible. |
| UX-8 | Labels render as distinct chips (not a comma-separated string). |
| UX-9 | Merge method persists across page reloads (localStorage). |
| UX-10 | Axe Core audit on PRCard with actions enabled: zero WCAG AA violations. |

---

## Pre-Mortem: Top Risks

| Priority | Risk | Prevention |
|---|---|---|
| P1 | node_id absent → `ConvertPRToDraft`/`MarkPRReady` fail silently | Story 1.1 gated before Story 2.2; frontend disables draft-toggle if `pr.nodeId` falsy; RPC returns `FailedPrecondition` for empty node_id. |
| P1 | Double-tap fires two RPCs → error toast confuses user | `inflight` boolean per button; button disabled immediately on first click. |
| P1 | 2-min stale window after merge/close | `cache.Refresh` called in RPC handler; optimistic removal of PR card on frontend success. |
| P2 | jscpd threshold breach from new jest mock blocks | Run `make ready-duplication-gate-web` locally; extract shared mock factory if needed. |
| P2 | Proto regeneration not run before tests | CI `make build` always runs `ent-gen` + `proto-gen`; document in PR checklist. |
| P3 | GraphQL draft mutation requires owner-level permissions the user may not have | Return clear error message; surface GitHub's error text in the inline alert. |

---

## Implementation Readiness Gate

| # | Criterion | Status |
|---|---|---|
| 1 | Every FR has ≥1 test case | ✅ |
| 2 | No TODO/TBD in plan.md architecture/task sections | ✅ |
| 3 | All ADRs referenced exist on disk | ✅ (none referenced — no ADR warranted) |
| 4 | No BLOCKER items in adversarial-review.md | ✅ (not present; self-review: no blockers) |
| 5 | No BLOCKER items in architecture-review.md | ✅ (not present; self-review: no blockers) |
| 6 | Schema changes have reversibility/zero-downtime strategy | ✅ (Migration Plan: additive fields, zero-downtime) |
| 7 | No P1 items open in pre-mortem.md | ✅ (all P1s have prevention in plan) |
| 8 | No import cycles; no layer violations | ✅ (`github_user_service.go` calls `github` package only; no cycles introduced) |

**Gate verdict: PASS**
