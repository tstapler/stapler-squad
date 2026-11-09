# Validation Plan: github-issue-cleanup

**Date**: 2026-11-03

---

## Happy Path Scenario

Given a `BacklogItemData` with `SourceID = ""`, `ExternalURL = "https://github.com/acme/widget/issues/42"`, `ExternalID = "42"`, `PrURL = "https://github.com/acme/widget/pull/99"`, and a valid keychain token for `github.com`, when the item transitions to `done`, then `CloseIssue("42")` is called, `PostIssueComment` receives `"...Implemented in: https://github.com/acme/widget/pull/99."`, `GitHubSyncedIssueUpdatedAt` is advanced to the GitHub API's returned `updated_at`, the fallback PR body contains `"Closes #42"` (bare number — same-repo case), and the agent session prompt contains `"Linked GitHub Issue: https://github.com/acme/widget/issues/42"` before `--- END BACKLOG ITEM DATA ---` and `"Fixes https://github.com/acme/widget/issues/42"` after it.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC-1 | `server/services/backlog_github_forward_sync_test.go` | `TestResolveCloseTarget_MCPImportPath_IssueURL_ReturnsPlugin` | Unit | `SourceID=""`, `ExternalURL="/issues/42"` → returns `*GitHubIssuesPlugin`, config with host/owner/repo JSON, externalID `"42"`, `ok=true` |
| AC-1 | `server/services/backlog_github_forward_sync_test.go` | `TestResolveCloseTarget_MCPImportPath_PRURLSkipped_ReturnsNotOK` | Unit | `SourceID=""`, `ExternalURL="/pull/7"` → `ok=false` (PR URL is not a closeable issue) |
| AC-1 | `server/services/backlog_github_forward_sync_test.go` | `TestResolveCloseTarget_EmptyExternalURL_ReturnsNotOK` | Unit | `SourceID=""`, `ExternalURL=""` → `ok=false` |
| AC-1 | `server/services/backlog_github_forward_sync_test.go` | `TestHandleForwardSyncClose_MCPImport_ClosesIssueAndPostsComment` | Integration | Stub `externalIssueCloser` + stub `BacklogStorage`; assert `CloseIssue("42", ...)` called once and `PostIssueComment("42", ...)` called once |
| AC-1 | `server/services/backlog_github_forward_sync_test.go` | `TestHandleForwardSyncClose_MCPImport_TokenMiss_LogsWarnAndSkipsClose` | Unit | No keychain token → `CloseIssue` not called; Warn log emitted with `"host"` and `"item"` fields |
| AC-2 | `server/services/backlog_github_forward_sync_test.go` | `TestForwardSyncCloseCommentWithPR_WithPRURL_EndsSentenceWithPRURL` | Unit | `forwardSyncCloseCommentWithPR("", "https://github.com/acme/widget/pull/99")` → result ends with `"Implemented in: https://github.com/acme/widget/pull/99."` |
| AC-2 | `server/services/backlog_github_forward_sync_test.go` | `TestForwardSyncCloseCommentWithPR_NoPRURL_EqualsBaseComment` | Unit | `forwardSyncCloseCommentWithPR("", "")` → equals `forwardSyncCloseComment` constant; no "Implemented in:" |
| AC-2 | `server/services/backlog_github_forward_sync_test.go` | `TestHandleForwardSyncClose_MCPImport_CommentBodyIncludesPRURL` | Integration | Stub records `PostIssueComment` body; assert it contains `"Implemented in: https://github.com/acme/widget/pull/99."` |
| AC-3 | `session/backlog_context_test.go` | `TestClosingKeywordFor_IssueURL_ReturnsCloses` | Unit | `closingKeywordFor("https://github.com/acme/widget/issues/42")` → `"Closes"` |
| AC-3 | `session/backlog_context_test.go` | `TestClosingKeywordFor_PRURL_ReturnsRelated` | Unit | `closingKeywordFor("https://github.com/acme/widget/pull/7")` → `"Related:"` |
| AC-3 | `session/backlog_context_test.go` | `TestClosingKeywordFor_EmptyURL_ReturnsEmpty` | Unit | `closingKeywordFor("")` → `""` |
| AC-3 | `session/backlog_lifecycle_pr_test.go` | `TestBuildFallbackPRBody_IssueURL_ContainsClosesLine` | Unit | `ExternalURL="/issues/42"`, same repo PR remote → body contains `"Closes #42"` (bare number, not full URL) |
| AC-3 | `session/backlog_lifecycle_pr_test.go` | `TestBuildFallbackPRBody_CrossRepo_NoClosesLine` | Unit | `ExternalURL="/issues/42"`, different repo PR remote → body contains neither `"Closes"` nor `"Related:"` |
| AC-3 | `session/backlog_lifecycle_pr_test.go` | `TestBuildFallbackPRBody_EmptyURL_NoKeywordLine` | Unit | `ExternalURL=""` → body contains neither `"Closes"` nor `"Related:"` |
| AC-4 | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_IssueURL_FactBeforeFenceInstructionAfter` | Unit | `ExternalURL="/issues/42"` → `"Linked GitHub Issue: ..."` appears before `"--- END BACKLOG ITEM DATA ---"` in output; `"Fixes https://..."` appears after the fence |
| AC-4 | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_PRURL_LinkedPRAndRelated` | Unit | `ExternalURL="/pull/7"` → `"Linked GitHub PR: ..."` before fence; `"Related: https://..."` after fence; `"Linked GitHub Issue:"` absent |
| AC-4 | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_EmptyURL_PromptUnchanged` | Unit | `ExternalURL=""` → prompt contains none of `"Linked GitHub"`, `"Fixes"`, `"Related:"` |
| AC-4 | `session/backlog_context_test.go` | `TestBuildTokenBudgetedPrompt_IssueURL_InstructionSurvivesTruncation` | Integration | Force a token budget tight enough to drop optional sections; assert `"Fixes https://..."` present in output even if `"Linked GitHub Issue:"` is absent |
| AC-5 | `server/services/backlog_github_forward_sync_test.go` | `TestHandleForwardSyncClose_MCPImport_WatermarkFromAPITimestamp` | Unit | `CloseIssue` returns `updatedAt = 2026-11-01T00:00:00Z`; assert `UpdateBacklogItem` called with `GitHubSyncedIssueUpdatedAt = 2026-11-01T00:00:00Z` |
| AC-5 | `server/services/backlog_github_forward_sync_test.go` | `TestHandleForwardSyncClose_MCPImport_WatermarkFallsBackToNow_WhenAPIReturnsZero` | Unit | `CloseIssue` returns zero `time.Time`; assert `UpdateBacklogItem` called with `GitHubSyncedIssueUpdatedAt` ≈ `time.Now().UTC()` (within 5 s) |
| AC-6 | `server/services/backlog_github_forward_sync_test.go` | `TestHandleForwardSyncClose_SourceID_NoRegression` | Integration | SourceID-linked item (non-empty `SourceID`, `ForwardSyncEnabled=true`): assert `CloseIssue` called with `source.ForwardSyncCloseLabel` and watermark advanced; behavior identical to pre-feature |
| AC-6 | `server/services/backlog_github_forward_sync_test.go` | `TestResolveCloseTarget_SourceIDPath_ReturnsSourcePlugin` | Unit | Non-empty `SourceID`, `ForwardSyncEnabled=true` → returns source plugin, decrypted config, `item.ExternalID`, `ok=true` |

---

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| UAC-1.1 (Case A: PR URL known, no host — exact string) | `server/services/backlog_github_forward_sync_test.go` | `TestForwardSyncCloseCommentWithPR_CaseA_ExactString` | Go unit | `forwardSyncCloseCommentWithPR("", "https://github.com/owner/repo/pull/123")` → assert exact equality: `"Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: https://github.com/owner/repo/pull/123."`; no trailing whitespace, one terminal period |
| UAC-1.2 (Case C: no PR URL — clause absent) | `server/services/backlog_github_forward_sync_test.go` | `TestForwardSyncCloseCommentWithPR_CaseC_NoImplementedInClause` | Go unit | `forwardSyncCloseCommentWithPR("", "")` → result does not contain `"Implemented in:"` or `"(no PR"` or `"N/A"` |
| UAC-1.3 (host ID suffix in correct position) | `server/services/backlog_github_forward_sync_test.go` | `TestForwardSyncCloseCommentWithPR_CaseB_HostSuffixAfterPRSentence` | Go unit | `forwardSyncCloseCommentWithPR("my-host", "https://github.com/owner/repo/pull/123")` → result contains `` on host `my-host`. `` after the PR sentence |
| UAC-1.4 (plain prose: no emoji, no Markdown) | `server/services/backlog_github_forward_sync_test.go` | `TestForwardSyncCloseCommentWithPR_NoEmojiOrMarkdown` | Go unit | Assert output of `forwardSyncCloseCommentWithPR` for all cases matches regexp `^[^*#!\[\]` + `` ` `` + `🤖]+$`; no asterisk, hash, bang, bracket, backtick cluster, or bot emoji |
| UAC-1.5 (no-token: issue stays open, nothing posted) | `server/services/backlog_github_forward_sync_test.go` | `TestHandleForwardSyncClose_MCPImport_NoToken_NothingPostedIssueOpen` | Go unit | Configure stub with no keychain token; assert `CloseIssue` call count = 0 and `PostIssueComment` call count = 0 |
| UAC-1.6 (consistent tone across all three comments) | Manual checklist | — | Manual | In a real or staging GitHub issue, trigger `in_progress`, `review`, `done` transitions on a MCP-imported item; verify all three automated comments are sentence-case, declarative, period-terminated, no exclamation marks |
| UAC-2.1 (keyword on its own line, not duplicated) | `session/backlog_lifecycle_pr_test.go` | `TestBuildFallbackPRBody_IssueURL_KeywordOnOwnLineNotDuplicated` | Go unit | Assert `strings.Count(body, "Closes https://github.com/acme/widget/issues/42") == 1`; assert the line is not embedded in a sentence (preceded by `\n` or start-of-string) |
| UAC-2.2 (pull → "Related:", not "Closes") | `session/backlog_lifecycle_pr_test.go` | `TestBuildFallbackPRBody_PRURL_RelatedNotCloses` | Go unit | Assert body contains `"Related: https://github.com/acme/widget/pull/7"` and does not contain `"Closes"` |
| UAC-2.3 (empty ExternalURL → no keyword) | `session/backlog_lifecycle_pr_test.go` | `TestBuildFallbackPRBody_EmptyURL_NoKeyword` | Go unit | (Same test as AC-3 unit error path; reuse.) Assert body contains neither `"Closes"` nor `"Related:"` |
| UAC-2.4 (bare #N, not full URL — GitHub canonical same-repo form) | `session/backlog_lifecycle_pr_test.go` | `TestBuildFallbackPRBody_IssueURL_UsesBareNumberNotFullURL` | Go unit | Assert body contains `"Closes #42"` and does not contain the full URL `"https://github.com/acme/widget/issues/42"` as the closing keyword argument (the full URL is used in the agent prompt via `buildSessionInitialPrompt`, not in the PR body) |
| UAC-3.1 (fact line exact literal string) | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_IssueURL_FactLineExactLiteral` | Go unit | Assert output contains the exact substring `"Linked GitHub Issue: https://github.com/acme/widget/issues/42"` |
| UAC-3.2 (instruction on its own line) | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_IssueURL_InstructionOnOwnLine` | Go unit | Assert `"Fixes https://github.com/acme/widget/issues/42"` appears as a standalone line (preceded by `\n`, not mid-sentence); assert it is NOT embedded in prose |
| UAC-3.3 (pull → "Linked GitHub PR:" + "Related:") | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_PRURL_LinkedPRAndRelated` | Go unit | (Same test as AC-4 pull path; reuse.) Assert `"Linked GitHub PR:"` and `"Related:"` present; `"Linked GitHub Issue:"` and `"Fixes"` absent |
| UAC-3.4 (empty → prompt byte-identical to baseline) | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_EmptyURL_PromptUnchanged` | Go unit | (Same test as AC-4 error path; reuse.) Diff prompt against baseline item with same fields but `ExternalURL=""`: assert zero additions |
| UAC-3.5 (block in hard-facts section, before END fence) | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_IssueURL_FactBeforeEndFence` | Go unit | Assert index of `"Linked GitHub Issue:"` < index of `"--- END BACKLOG ITEM DATA ---"` in output |
| UAC-3.6 (instruction survives minimum viable truncation) | `session/backlog_context_test.go` | `TestBuildTokenBudgetedPrompt_MinBudget_InstructionLineSurvives` | Go integration | Force `BuildTokenBudgetedPrompt` with a token budget set to the minimum that still returns non-empty; assert `"Fixes https://..."` present even when `"Linked GitHub Issue:"` is absent |

---

## Test Stack

- **Unit**: Go testing + testify (`require`/`assert`) assertions; stubs via local struct implementing `externalIssueCloser`; no network calls, no DB
- **Integration**: Go testing with an in-memory stub `BacklogStorage` and a stubbed `externalIssueCloser`; exercises `handleForwardSyncClose` end-to-end within the process; no real GitHub API calls
- **E2E / UX**: Manual checklist for UAC-1.6 (tone consistency across three automated comments in a live issue thread); all other UX criteria are covered by the unit/integration tests above
- **Truncation tests**: Call `BuildTokenBudgetedPrompt` directly with a constructed `BacklogItemData` and a low integer token budget; assert on the returned string

### Test infrastructure notes

- `externalIssueCloser` stub: a local struct in `backlog_github_forward_sync_test.go` with recorded call arguments, returning a configurable `updatedAt` and `error`.
- `BacklogStorage` stub: implement only `GetItemSourceByID` and `UpdateBacklogItem`; use a recording struct to capture the `BacklogItemUpdate` passed to `UpdateBacklogItem`.
- Token-miss stub: return `("", false)` from the keychain lookup shim, without touching the network.
- `closingKeywordFor` is package-private (`session` package) — test it in `session/backlog_context_test.go` using the same package (`package session`), not `package session_test`.
- `forwardSyncCloseCommentWithPR` is package-private (`server/services`) — test it in `backlog_github_forward_sync_test.go` using `package services`.

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (all changed packages) | `make test-coverage` | ≥ 80% line coverage for `server/services/backlog_github_forward_sync.go`, `session/backlog_context.go`, `session/backlog_lifecycle_pr.go` |
| New helpers specifically | `go test -v -run 'TestClosingKeywordFor\|TestForwardSyncCloseCommentWithPR\|TestResolveCloseTarget' ./session/... ./server/services/...` | 100% line (pure functions with 3–4 branches each) |
| Truncation path | `go test -v -run 'TestBuildTokenBudgetedPrompt' ./session/...` | Both truncation passes covered (at least two distinct budget values exercised) |
| Race detection | `make test-race` | Zero race errors in `server/services` and `session` packages |
| Lint | `make lint` | Zero new lint errors |

---

## Test Count Summary

| Type | Count |
|------|-------|
| Unit (requirement mapping) | 18 |
| Integration (requirement mapping) | 4 |
| Unit (UX acceptance, distinct from requirement tests) | 8 |
| Integration (UX acceptance, distinct from requirement tests) | 1 |
| Manual checklist (UX acceptance) | 1 |
| **Total automated** | **31** |
| **Total including manual** | **32** |

Requirements coverage: **6 / 6 ACs** (100%)
UX acceptance criteria coverage: **12 / 12 UACs** (11 automated, 1 manual)
