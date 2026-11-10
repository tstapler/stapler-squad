# Technology Stack Research: GitHub Issue Cleanup

## 1. GitHub API Call Pattern

**Location:** `session/backlog_plugin_github.go`

The plugin layer uses plain `http.NewRequestWithContext` (not `NewConditionalRequest`) for
write-path calls (PATCH close, POST comment), since ETag caching is only relevant for
conditional GET reads. The pattern is:

1. Decode config: `decodeGithubIssuesConfig(config PluginConfig) (githubPluginConfig, error)` — JSON-unmarshal `config.Raw` into `{host, owner, repo, token}`, then override `token` from keychain via `github.GetKeychainTokenForHost(cfg.Host)` (keychain token wins, raw-config token is the pre-migration fallback).
2. Build URL: `githubAPIURL(host, path)` — joins `github.RestBaseURLForHost(host)` or `githubAPIBaseURL` ("https://api.github.com") with the REST path.
3. Build request: `http.NewRequestWithContext(ctx, method, url, body)` with `Authorization: token <tok>` and `Content-Type: application/json`.
4. Execute: `github.HTTPClient().Do(req)` — the shared rate-limited HTTP client.
5. Handle 429/403+RateLimit-Remaining=0 as a distinct rate-limit error; treat HTTP ≥ 300 as a failure.

**Key method signatures (all on `*GitHubIssuesPlugin`):**
- `CloseIssue(ctx, config PluginConfig, externalID string, existingLabels []string, closeLabel string) (time.Time, error)` — returns post-close `updated_at` from GitHub (zero on parse failure, not an error).
- `PostIssueComment(ctx, config PluginConfig, externalID string, body string) error` — posts comment body to `POST /repos/{owner}/{repo}/issues/{number}/comments`.
- `PostProgressComment(ctx, config PluginConfig, externalID, body string) (time.Time, error)` — calls `PostIssueComment` then reads back updated_at.

**Note on `norawghrequest` linter:** The rule (`.claude/rules/norawghrequest.md`) applies to native REST callers that bypass ETag plumbing. `CloseIssue`/`PostIssueComment` in `backlog_plugin_github.go` are legitimate write calls, not conditional-GET bypasses — the linter's exemption set covers `newGHRequestForHostWithToken` and similar read-path builders, not the plugin layer's write helpers. No change needed.

## 2. EventBus Subscriber Pattern

**Reference implementation:** `server/services/backlog_github_forward_sync.go` (also mirrored in `backlog_github_progress_sync.go` and `server/analytics/subscriber.go`).

Pattern:
```go
func StartBacklogGitHubForwardSyncSubscriber(ctx context.Context, bus *events.EventBus, ...) <-chan struct{} {
    done := make(chan struct{})
    if bus == nil || ... {
        close(done); return done  // nil-guard: returns a closed channel
    }
    ch, _ := bus.Subscribe(ctx)
    go func() {
        defer close(done)
        for {
            select {
            case event, ok := <-ch:
                if !ok { return }
                // filter by event.Type == events.EventBacklogItemChanged
                // filter by payload.Kind == events.BacklogChangeStatusTransition
                // filter by payload.NewStatus == string(session.BacklogStatusDone)
                handleForwardSyncClose(ctx, ...)
            case <-ctx.Done():
                return
            }
        }
    }()
    return done
}
```

**Wiring:** Called from `server/server.go` (or `server/dependencies.go`), passing `deps.BacklogService.Registry()` and `deps.BacklogService.SyncLoopForForwardSync()`. The returned `<-chan struct{}` is the done channel for graceful shutdown.

**Event filter for done-transition:** `event.Type == events.EventBacklogItemChanged && payload.Kind == events.BacklogChangeStatusTransition && payload.NewStatus == string(session.BacklogStatusDone)`.

**Important caveat:** The event payload's `Item` snapshot can have an empty `SourceID` even for source-linked items (TransitionBacklogItemStatus reloads with a plain `.Get`, not `.WithSource()`). Always re-fetch with `storage.GetBacklogItem(ctx, item.ID)` to get the eager-loaded Source edge.

## 3. Token Resolution for MCP-Imported Items (`resolveProgressTarget`)

**Location:** `server/services/backlog_github_progress_sync.go:171`

Two distinct paths:

**Path A — Source-linked items** (`item.SourceID != "" && item.ExternalID != ""`):
1. `storage.GetItemSourceByID(ctx, item.SourceID)` → `source`
2. `registry.Get(source.PluginID)` → plugin (type-assert to `issueProgressCommenter`)
3. `syncLoop.DecryptConfigToken(source.Config)` → decrypted raw config JSON
4. Build `session.PluginConfig{Raw: raw}` and return `(plugin, cfg, item.ExternalID, true)`

**Path B — MCP/web-imported items** (`item.SourceID == ""`, `item.ExternalURL != ""`):
1. `githubpkg.ParseGitHubRefWithHosts(item.ExternalURL, nil)` → `ref` (must be `RefTypeIssue`)
2. Build a minimal config JSON: `json.Marshal(map[string]string{"host": ref.Host, "owner": ref.Owner, "repo": ref.Repo})` — no token in config
3. Return `(&session.GitHubIssuesPlugin{}, session.PluginConfig{Raw: string(raw)}, fmt.Sprint(ref.IssueNumber), true)`
4. The plugin's `decodeGithubIssuesConfig` calls `github.GetKeychainTokenForHost(cfg.Host)` to supply the token from the keychain

**Key insight:** For MCP items, the config has no token field — the keychain is the sole token source. `decodeGithubIssuesConfig` enforces this: `cfg.Token == ""` after the keychain lookup → error "token is required". GHE hosts are excluded (`nil` host list to `ParseGitHubRefWithHosts`).

## 4. PR Body Construction Pipeline

**Location:** `session/backlog_lifecycle_pr.go`

```
buildFallbackPRBody(item, origin)       // used when no LLM draft available
appendBacklogFooter(drafted, origin, item) // used when LLM draft IS available
    └── calls prFooter(origin, item)
```

**`buildFallbackPRBody`** (line 477):
- Opens with `## Summary\n{item.Description truncated to 1000}`
- Appends `prFooter(origin, item)` (backlog item ID, host, provenance comment)
- Appends `## Test plan\n` with AC criteria as a checklist (checked if satisfied)
- **No existing "Closes #N" link** — this would be a new addition for MCP-imported items

**`appendBacklogFooter`** (line 464):
- Trims trailing newlines from drafted, adds `\n\n`, appends footer

**`prFooter`** (line 440-ish):
- Writes `Backlog item: {item.ID}`
- Writes host provenance comment via `FormatPRProvenanceComment(hostID, deepLink)`
- Currently has no external-issue cross-reference

**Where to add "Closes #N":** Insert before `prFooter(origin, item)` in `buildFallbackPRBody`, conditioned on `item.ExternalURL != ""` and the ref being an issue. For the LLM-drafted path (`appendBacklogFooter`), could append a "Closes" line to the footer or instruct the agent via the system prompt.

## 5. Feature Flag Pattern

**Single source of truth:** `knownFeatureFlags` slice in `server/services/feature_flag_service.go:215`.

**Registration:**
```go
var knownFeatureFlags = []struct {
    name         string
    description  string
    defaultValue bool  // false if omitted (zero value)
    scopes       []string  // nil: global only
}{
    // ...
    {
        name:        "github_issue_cleanup_on_done",  // new flag
        description: "...",
        // defaultValue omitted → false (default off, standard for new flags)
    },
}
```

**Runtime read (simplest pattern — from progress_sync.go):**
```go
const myFlagName = "github_issue_cleanup_on_done"

func MyFlagEnabled() bool {
    return config.LoadConfig().GetFeatureFlagWithDefault(myFlagName, featureFlagDefault(myFlagName))
}
```

`featureFlagDefault(name)` (line 557) looks up `name` in `knownFeatureFlags` and returns its `defaultValue`. Unregistered names return `false`.

**Subscriber pattern:** The flag is passed as a `flagEnabled func() bool` parameter (like `StartBacklogGitHubProgressSyncSubscriber`) — read per event, not at startup, so a live settings toggle applies without a restart.

**Default policy:** New flags default to `false` (opt-in). The `github_issue_progress_comments` flag (`githubIssueProgressCommentsFlagName`) is the direct precedent — no `defaultValue` field, default off.

## Summary of What to Build

For this feature:

1. **Forward-sync close for MCP items** (`backlog_github_forward_sync.go`): The existing `handleForwardSyncClose` already checks `current.SourceID == "" || current.ExternalID == ""` and returns early. MCP items have `ExternalURL` but no `SourceID`/`ExternalID`. To cover them, add a second check: if `SourceID == ""` but `ExternalURL != ""`, parse the URL with `githubpkg.ParseGitHubRefWithHosts`, build a minimal config (host/owner/repo), use `&session.GitHubIssuesPlugin{}` directly — the same pattern as `resolveProgressTarget`'s Path B above.

2. **"Closes #N" in fallback PR body** (`session/backlog_lifecycle_pr.go`): In `buildFallbackPRBody`, after the Summary section, add a `Closes {ExternalURL}` line when `item.ExternalURL` resolves to a GitHub issue ref.

3. **PR URL in done-close comment** (`server/services/backlog_github_forward_sync.go`): `forwardSyncCloseCommentFor` currently produces a fixed string. Extend it (or add a new variant) that accepts `item.PrURL` and appends it when non-empty.

4. **"Fixes <url>" in agent prompt** (`session/backlog_context.go`): In `buildSessionInitialPrompt`, after the item data block, append a `Fixes {item.ExternalURL}` line when `ExternalURL != ""` — to guide the agent to include the reference in its own commit/PR.
