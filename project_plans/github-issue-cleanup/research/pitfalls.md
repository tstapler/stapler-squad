# Pitfalls and Risks: github-issue-cleanup

Researched 2026-11-03 against the codebase at `backlog/stapler-squad-owner-action-existing-surfaces`.

---

## 1. Loop-prevention risk

**Mechanism.** The backward sync checks `alreadyReconciled` in `session/backlog_sync.go` line 486:

```go
alreadyReconciled := existing.GithubSyncedIssueUpdatedAt != nil &&
    !extItem.IssueUpdatedAt.After(*existing.GithubSyncedIssueUpdatedAt)
```

The forward sync writes this watermark _after_ `CloseIssue` returns, using GitHub's own `updated_at` from the close response (not local wall clock — per ADR-003 and `backlog_github_forward_sync.go` lines 214–223). Without this, the backward sync would see the closed issue's `updated_at` as newer than the stored watermark (or no stored watermark at all), and attempt another status transition.

**MCP-imported items and the watermark.** The backward sync is entirely keyed on `ItemSource`. `SyncOne` (`backlog_sync.go` line 343) calls `er.GetBacklogItemByExternalID(ctx, source.ID.String(), extItem.ExternalID)` — a source-scoped lookup. MCP-imported items have no `SourceID` and are therefore _invisible to every backward sync loop_. This means there is no active loop risk for MCP-imported items in Phase 1.

**Risk going forward.** If a future change adds an `ExternalURL`-based backward sync (not currently wired), or if an MCP-imported item is later linked to a source, a missing `GithubSyncedIssueUpdatedAt` on a closed item would immediately create a reconcile loop. The safe approach is to write the watermark for MCP-imported items in the same step as for SourceID items — `storage.UpdateBacklogItem` with `GitHubSyncedIssueUpdatedAt` set to the `updated_at` returned from `CloseIssue`. This costs nothing extra and future-proofs against the loop.

---

## 2. Race conditions

**Re-fetch on subscribe, but no post-fetch status check.** `handleForwardSyncClose` re-fetches the item at line 137 to avoid trusting a stale event payload (the payload's `Item` snapshot can have an empty `SourceID` even for a source-linked item — see the comment block). However, it does NOT re-verify `current.Status == "done"` after the re-fetch. If a concurrent transition moves the item from `done` back to another status between the event firing and the subscriber's re-fetch, the subscriber will still attempt to close the GitHub issue.

**Practical severity.** In practice `done` is terminal and no transition path moves out of it, so this is a theoretical edge case in the existing code. The new MCP-imported path inherits the same gap. The fix is a one-liner: return early if `current.Status != string(session.BacklogStatusDone)` after re-fetching.

**No at-most-once guard.** There is also no idempotency record for the close action — if the subscriber fires twice for the same item (e.g. the EventBus re-delivers), `CloseIssue` will be called twice. GitHub's close endpoint is idempotent (closing an already-closed issue returns a valid response), so this is benign — but `PostIssueComment` is not idempotent and will post a duplicate bot comment. The watermark write after the first call makes the second call's close a no-op at the GitHub level, but it does not prevent the second comment from posting.

---

## 3. Rate limiting

**Call count.** The existing path: `CloseIssue` (1 PATCH) + `PostIssueComment` (1 POST) = 2 write calls. The skip-if-closed path adds `readIssueUpdatedAt` (1 GET) before the PATCH, bringing the total to 3.

**GitHub's secondary rate limit** applies to write operations (POST/PATCH/PUT/DELETE), not to GETs, so the extra GET does not increase secondary rate limit exposure. However, if many items transition to done in a burst (e.g. a bulk status import), the subscriber fires serially per event and the POSTs/PATCHes accumulate quickly. GitHub's secondary rate limit is per-minute, not per-second, so a burst of 10–20 items in one minute can hit it.

**Error handling.** Both `CloseIssue` (lines 337–339) and `fetchIssuesPage` (lines 195–198) detect rate limit responses and return an error immediately with no retry/backoff. The subscriber logs the error and calls `storage.RecordSourceSyncFailure`, but does not reschedule. Rate-limited items are therefore silently dropped unless an operator manually triggers re-sync.

**Recommendation.** For Phase 1 this is acceptable since the subscriber runs at most once per done-transition and normal usage patterns don't involve mass done-transitions. Flag it as a known gap to address before any bulk-import or mass-archive feature.

---

## 4. Cross-repo `Closes #N` pitfall

**GitHub's behaviour.** The bare `Closes #N` keyword in a PR body closes issue number N in the **same repository** as the PR. For a cross-repo reference, the full form `Closes owner/repo#N` (or the full URL) is required; a bare `#N` for an issue in a different repo is silently ignored — no link is created, no error is shown.

**Dangerous wrong-issue scenario.** If a backlog item is linked to `github.com/other-org/other-repo/issues/42` and the PR is opened in `github.com/my-org/my-repo`, injecting `Closes #42` into the PR body does not close the linked issue — but it *does* create a link to `my-org/my-repo#42` if that issue exists, which may be a completely unrelated issue. That issue would then auto-close when the PR merges, silently breaking it.

**Required check.** The PR body injection must compare the parsed owner/repo from the item's `ExternalURL` against the PR's target repo. Only inject `Closes #N` when both match. For cross-repo items, either use the full `Closes owner/repo#N` form (which GitHub does honour across repos in the same GitHub instance) or omit the keyword entirely.

**Parsing the PR repo.** The PR's target repo is derivable from the `git remote` URL or from the `gh pr create` target, which is already known at the call site in `backlog_lifecycle_pr.go`. The item's repo is in `ExternalURL`, parseable via `github.ParseGitHubRefWithHosts`.

---

## 5. Keychain token scope for GHE hosts

`resolveProgressTarget` (`server/services/backlog_github_progress_sync.go` lines 190–198) handles MCP-imported items by building a tokenless plugin config from `ExternalURL` and relying on `github.GetKeychainTokenForHost(cfg.Host)` inside `decodeGithubIssuesConfig` to supply a token.

**Failure path.** When no keychain token is registered for the issue's host (e.g. a GitHub Enterprise instance never configured in Settings), `decodeGithubIssuesConfig` returns `fmt.Errorf("github_issues: token is required")`. `CloseIssue` returns that error; the forward-sync subscriber logs it via `storage.RecordSourceSyncFailure` and returns without closing the issue or posting a comment. No user-visible notification is sent.

**github.com is safe.** A github.com token stored in the keychain is used for all `IsGitHubCom(host)` checks — most users will have this. The gap only affects GHE-hosted issues imported via MCP without a matching GHE host token in the keychain.

**Practical impact.** For Phase 1 this is acceptable, but the error should surface clearly to the user (e.g. in the item's sync-failure log) so they can add the token. The existing `RecordSourceSyncFailure` path achieves this for SourceID-based items; for MCP-imported items (no SourceID), there is no `source.ID` to record against, so the failure may be logged only to the application log and invisible in the UI.

---

## 6. MCP-imported items: ExternalID is stored

From `server/mcp/tools_backlog.go` lines 2417–2426, `import_github_issue` creates the item with:

```go
ExternalID:  strconv.Itoa(ref.IssueNumber),  // numeric issue number as string
ExternalURL: issue.URL,                        // full GitHub URL
// SourceID is NOT set — no ItemSource link
```

Both `ExternalID` and `ExternalURL` are populated. `SourceID` is intentionally absent.

The current forward-sync guard at line 142 of `backlog_github_forward_sync.go`:

```go
if current.SourceID == "" || current.ExternalID == "" {
    return // locally-created item, nothing to sync
}
```

...silently skips ALL MCP-imported items because `current.SourceID == ""`, even though they have a valid `ExternalID` and `ExternalURL`. The new path must handle the `SourceID == "" && ExternalID != "" && ExternalURL != ""` case as a separate branch, using `resolveProgressTarget`'s ExternalURL-parsing logic (already proven correct in `backlog_github_progress_sync.go`) rather than re-implementing it.

---

## 7. `norawghrequest` linter rule

**Rule scope.** The analyzer (`tools/lint/norawghrequest/analyzer.go`) flags `http.NewRequest`/`http.NewRequestWithContext` calls only when their URL argument resolves (via local variable assignment chains, up to depth 5) to `github.GhBaseURL()` or `github.RestBaseURLForHost()` from the `github` package, or to a function in `hostURLHelperFuncNames` (currently only `graphQLURLForHost`).

**Existing calls pass undetected.** `CloseIssue`, `PostIssueComment`, `fetchIssuesPage`, and `readIssueUpdatedAt` all pass `githubAPIURL(cfg.Host, ...)` as the URL arg. `githubAPIURL` is in the `session` package, not `ghPackagePath`, and is not in `hostURLHelperFuncNames`. The analyzer does not follow cross-package calls, so it never sees that `githubAPIURL` internally calls `github.RestBaseURLForHost`. All four existing `http.NewRequestWithContext` calls in `backlog_plugin_github.go` **pass the linter without a nolint annotation**.

**New code following the same pattern will also pass.** Any new call in `backlog_plugin_github.go` using `githubAPIURL(...)` as the URL arg will not trigger the analyzer. This is a de facto exemption through indirection.

**Why the rule doesn't apply semantically for write ops.** The `norawghrequest` rule exists primarily to enforce ETag/conditional-request semantics for GET calls (to get `304 Not Modified` responses that consume zero rate limit quota). `CloseIssue` is a PATCH and `PostIssueComment` is a POST — write operations. ETag caching is not applicable to mutating calls, so the absence of `NewConditionalRequest` here is correct by design, not an oversight.

**Conclusion.** New code in `backlog_plugin_github.go` using the established `githubAPIURL` pattern needs no `//nolint:norawghrequest` annotation and will not trigger the linter. If a new call were added elsewhere that directly used `github.RestBaseURLForHost()` as the URL arg, it *would* be flagged and would need either a wrapper or the annotation.
