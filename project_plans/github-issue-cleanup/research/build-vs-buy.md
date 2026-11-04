# Build vs Buy: GitHub Issue Cleanup

_Research date: 2026-11-03_

## 1. GitHub API Client

**Verdict: stay with the existing direct HTTP client pattern (build, not buy).**

The codebase has a fully-wired, purpose-built GitHub HTTP client in `github/http_client.go`:
- `github.HTTPClient()` returns a shared `*http.Client` with a 3-layer transport chain: telemetry (`githubTelemetryTransport`) → OTel (`otelhttp.NewTransport`) → rate-limit awareness (`rateLimitTransport`). Every native REST call that goes through this client automatically records metrics, traces, and respects the global rate-limit state (`DefaultRateLimiter`).
- `go.mod` has no dependency on `google/go-github` or `shurcooL/githubv4`. Adding one for two API calls (PATCH issue + POST comment) is not justified — both already have working implementations in `session/backlog_plugin_github.go` (`CloseIssue` and `PostIssueComment`).
- The `norawghrequest` custom linter (`.claude/rules/norawghrequest.md`) enforces that callers must build requests via `NewConditionalRequest`/`NewConditionalRequestNoCache`/`newGHRequestForHostWithToken` for ETag-eligible calls; for mutating calls (PATCH/POST), direct use of the shared client is correct.

## 2. Issue-Closing API Pattern

**Verdict: REST PATCH is already implemented and correct; no GraphQL or Apps API needed.**

`session/backlog_plugin_github.go:CloseIssue` already implements `PATCH /repos/{owner}/{repo}/issues/{issue_number}` with `{"state": "closed"}`, including:
- Label-merge semantics (labels field fully replaces, so `mergeLabels` combines `existingLabels + closeLabel`).
- Rate-limit detection from response headers.
- Returns the post-close `updated_at` from GitHub's response body for the ADR-003 watermark (avoiding clock-skew).

For MCP-imported items (ExternalURL/ExternalID but no SourceID), the new code will need to call the same underlying REST endpoint, but via a different call path since there is no `PluginConfig`/`SourceID` to decode. The right approach is to extract a lower-level helper (e.g., `closeGitHubIssueByOwnerRepoNumber(ctx, host, token, owner, repo, number)`) or call the REST endpoint directly using `github.HTTPClient()` and a manually constructed request — matching the existing pattern in `fetchIssuesPage`.

GraphQL offers no advantage here: the mutation is `updateIssue(input: {id: ..., state: CLOSED})`, which requires the node ID rather than the issue number, adding an extra lookup RPC. REST is simpler and already in use.

GitHub Apps API would require installing an app and managing app credentials — far more infrastructure than a token-based call.

## 3. Token Resolution

**Verdict: `github.GetKeychainTokenForHost` is the correct pattern for MCP-imported items.**

For source-linked items (SourceID present), `decodeGithubIssuesConfig` (called by `CloseIssue`) already prefers the keychain token over any per-source config token. For MCP-imported items (no SourceID), the host is recoverable by parsing ExternalURL with `ParseGitHubRefWithHosts`, then passing the resulting `Host` field to `github.GetKeychainTokenForHost`. This is the established pattern used throughout the GitHub plugin layer. No alternative authentication approach (OAuth apps, GitHub Apps JWT flow, PAT environment variables) is used anywhere in the codebase, and introducing one would be inconsistent.

Fallback: if `GetKeychainTokenForHost` returns `""` for the parsed host, the close should log and no-op — matching the `disabled` path in `decodeGithubIssuesFetchConfig`.

## 4. URL Parsing for Issue Numbers

**Verdict: `ParseGitHubRefWithHosts` is fully sufficient.**

`github/url_parser.go:ParseGitHubRefWithHosts` returns a `ParsedGitHubRef` with:
- `Type == RefTypeIssue` for issue URLs (`/issues/{number}`)
- `IssueNumber int` (already parsed from the URL)
- `Owner`, `Repo`, `Host` fields

For generating `Closes #N` syntax, the number is `ref.IssueNumber`. For the agent prompt's `Fixes <url>` instruction, the original `ExternalURL` or `ref.HTMLURL()` is the right value. No additional URL parsing library is needed.

The only edge case: `ParseGitHubRefWithHosts` requires the enterprise host list to match GHE URLs. For MCP-imported items whose `ExternalURL` is a GHE URL, the host must be passed. The host list is available from `github.EnterpriseHosts()` (or `config.Config.GitHubEnterpriseHosts`) — the same list passed to `ParseGitHubRefWithHosts` in existing call sites.

## 5. OSS Libraries / Forward-Sync Pattern

**Verdict: extend the existing `StartBacklogGitHubForwardSyncSubscriber` pattern, no OSS library buy.**

`server/services/backlog_github_forward_sync.go` already has:
- An EventBus subscriber that fires on `BacklogStatusDone` transitions.
- `handleForwardSyncClose` which re-fetches the item (to get the Source edge), looks up the plugin via `registry.Get`, and calls `CloseIssue`/`PostIssueComment` through the `externalIssueCloser` interface.
- A `forwardSyncCloseComment` bot comment posted after close.

The gap for MCP-imported items is that `handleForwardSyncClose` early-returns at:
```go
if current.SourceID == "" || current.ExternalID == "" {
    return // locally-created item, nothing to sync
}
```
MCP-imported items have `ExternalURL` and `ExternalID` set, but `SourceID == ""`. The new logic belongs in the same subscriber, as a second branch after the existing SourceID check: if `current.SourceID == ""` but `current.ExternalURL != ""` and `ParseGitHubRefWithHosts` returns `RefTypeIssue`, close the issue directly using the keychain token for the parsed host.

No OSS automation library (e.g., `google/go-github`, `cli/go-gh`) adds value over the existing direct-HTTP pattern. `google/go-github` would add a new indirect dependency and require adapting its auth model (oauth2 token source) to the keychain pattern. `cli/go-gh` is CLI-first and not designed for embedding in a server.

## 6. `Closes #N` Injection: Template vs LLM

**Verdict: inject via template (pure string manipulation in `appendBacklogFooter` / `buildFallbackPRBody`), not by instructing the LLM.**

The `prDescriptionSystemPrompt` in `session/headless/features.go` does not currently instruct the model to include any `Closes #N` line; the prompt specifies a strict three-section Markdown structure (`## Summary`, `## What Changed`, `## Test plan`) with no issue-closing keyword. Extending the prompt to request `Closes #N` is fragile:

- LLMs can omit it, duplicate it, or mis-format it (`Closes #0`, `Closes 42`, missing the `#`), especially when no issue number is part of the diff context.
- The issue number is not in the diff — it must be injected from `item.ExternalURL` (parsed to `ref.IssueNumber`), which is not passed to `DraftPRDescription` today.
- A template produces deterministic output with no additional LLM cost and is trivially testable.

The right injection point is `appendBacklogFooter` and `buildFallbackPRBody`, which both call `prFooter`. `prFooter` already appends a backlog-item line; a `Closes #N` line before the existing footer can be appended here when `item.ExternalURL` parses to a GitHub issue ref. The injection should be in `prFooter` itself (or a thin wrapper), not scattered across both callers.

The `Closes` keyword is documented GitHub convention (`Closes #N`, `Fixes #N`, `Resolves #N` all work) — no library generates it; a bare `fmt.Sprintf("Closes #%d", ref.IssueNumber)` is correct. Cross-repository syntax (`Closes owner/repo#N`) is needed when the PR and issue are in different repos; the right value is `item.ExternalURL` for cross-repo references or `#N` when owner+repo match the PR's repo.

## 7. `resolveProgressTarget` Reuse vs Extend

**Verdict: share `resolveProgressTarget` between the progress-comment path and the new done-close path; do not inline equivalent logic in `handleForwardSyncClose`.**

`server/services/backlog_github_progress_sync.go:resolveProgressTarget` already does exactly the needed lookup for the no-SourceID case:

```go
ref, err := githubpkg.ParseGitHubRefWithHosts(item.ExternalURL, nil)
if err != nil || ref.Type != githubpkg.RefTypeIssue {
    return nil, cfg, "", false
}
raw, err := json.Marshal(map[string]string{"host": ref.Host, "owner": ref.Owner, "repo": ref.Repo})
// ...
return &session.GitHubIssuesPlugin{}, session.PluginConfig{Raw: string(raw)}, fmt.Sprint(ref.IssueNumber), true
```

The `handleForwardSyncClose` path in `backlog_github_forward_sync.go` currently has a hard early-return for `SourceID == ""`. Rather than duplicating the URL-parsing and config-fabrication logic, the two files should share `resolveProgressTarget` (or an extracted `resolveGitHubTarget` variant that is not coupled to `issueProgressCommenter`). The return type difference is minor: `resolveProgressTarget` returns an `issueProgressCommenter`, while the close path needs an `externalIssueCloser`. Since `*session.GitHubIssuesPlugin` satisfies both interfaces, a single helper can return the plugin and the config, leaving the interface assertion to the caller.

The tradeoff for not sharing is that the lookup logic evolves in two places: if GHE host-list handling or keychain fallback changes, it must be updated in both subscribers separately — a maintenance hazard already seen with the `decodeGithubIssuesFetchConfig` / `decodeGithubIssuesConfig` split (which introduced subtle differences in how empty-token is treated).

## 8. GitHub Apps vs PAT

**Verdict: out of scope; PAT via keychain is the right choice for this feature.**

The system uses PATs stored in the keychain (`github.GetKeychainTokenForHost`). GitHub Apps would provide better auditability (bot identity separate from a person) and finer-grained permissions, but require: generating and rotating a private key, implementing JWT-based token exchange (`POST /app/installations/{id}/access_tokens`), and storing App credentials outside the existing keychain model. The `CloseIssue` and `PostIssueComment` calls are write operations that require `issues:write` scope — a PAT already in the keychain for the user's own repo satisfies this. Switching to a GitHub App is a distinct infrastructure decision that affects all GitHub integrations in the codebase, not a per-feature choice.

## Summary

| Question | Verdict |
|---|---|
| GitHub API client library | Build: stay with `github.HTTPClient()` direct REST calls |
| Issue-closing API | Build: REST PATCH already implemented; extract a host+token helper for the SourceID-less path |
| Token resolution | Build: `github.GetKeychainTokenForHost(parsedRef.Host)` — existing pattern |
| URL parsing | Build: `ParseGitHubRefWithHosts` returns `IssueNumber` and `Owner/Repo/Host` — sufficient |
| Forward-sync pattern | Build: extend `handleForwardSyncClose`'s existing subscriber with an MCP-import branch |
| `Closes #N` injection | Build: template in `prFooter`/`appendBacklogFooter` — `fmt.Sprintf("Closes #%d", ref.IssueNumber)`; never delegate to the LLM |
| `resolveProgressTarget` reuse | Reuse: extract shared helper shared by both subscribers; avoid duplicating URL-parse + config-fabricate logic |
| GitHub Apps vs PAT | Out of scope: PAT via keychain is correct for this feature |
