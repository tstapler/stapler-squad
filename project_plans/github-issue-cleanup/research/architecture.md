# Architecture Research: GitHub Issue Cleanup

**Date**: 2026-11-03
**Status**: Complete (research phase — no code changed)
**Scope**: Two gaps in the existing GitHub issue integration:
- FR1: Close GitHub issues for MCP-imported items (ExternalURL only, no SourceID) on `done` transition
- FR2: Inject `Closes #N` into generated PR bodies for items backed by GitHub issues

---

## 1. EventStorming Table

The prior research at `project_plans/backlog-github-two-way-sync/research/architecture.md`
defines row #2 (`BacklogItemTransitionedToDone` → `CloseExternalIssue`). This extends
that row to cover the two item shapes and adds the new PR-creation event.

| # | Event | Actor/System | Policy (if...then) | Command | Handler |
|---|---|---|---|---|---|
| 2a | `BacklogItemTransitionedToDone` — item has `SourceID != ""` **AND** `ExternalID != ""` | User / `AutonomousOrchestrationService` / stuck-detector | IF `source.ForwardSyncEnabled == true` AND plugin implements `externalIssueCloser` or `externalIssueStateUpdater` | `CloseExternalIssue(sourceID, externalID, ...)` | Existing path in `handleForwardSyncClose` (`server/services/backlog_github_forward_sync.go:166–189`); no change |
| 2b | `BacklogItemTransitionedToDone` — item has `ExternalURL != ""` **AND** `SourceID == ""` (MCP/web-imported) | Same | IF `ExternalURL` parses as a github.com issue ref (not GHE — same restriction as `resolveProgressTarget`) AND the keychain yields a token | `CloseExternalIssue(keychainConfig, issueNumber, ...)` | **New branch** in `handleForwardSyncClose`, mirroring `resolveProgressTarget`'s fallback arm (`backlog_github_progress_sync.go:190–198`) |
| 2c | `BacklogItemIssuePRCreated` — item with an issue ExternalURL gets a PR created by `pushAndCreatePR` or `RecordPRCreatedOutOfBand` | `BacklogLifecycleListener` | IF `ExternalURL` parses as a github.com issue ref AND PR repo host/owner/repo matches issue host/owner/repo | Inject `Closes #N` into PR body before `CreatePR` | `prFooter` / `buildFallbackPRBody` / `appendBacklogFooter` in `session/backlog_lifecycle_pr.go:445–465` |

---

## 2. FR1 Integration Design: Where the new branch goes in `handleForwardSyncClose`

### Current guard (lines 142–144)

```go
if current.SourceID == "" || current.ExternalID == "" {
    return // locally-created item, nothing to sync
}
```

This comment is accurate for locally-created items but over-broad: it also silently
skips MCP-imported items that do have a GitHub issue URL in `ExternalURL`.

### Three options

**Option A — `else if` arm after the SourceID guard.**

Add a new branch that fires when `SourceID == ""` but `ExternalURL != ""`. The
early-return guard at line 142 becomes:

```go
if current.SourceID == "" || current.ExternalID == "" {
    // Locally-created items have no ExternalURL either — skip.
    // MCP-imported items have ExternalURL but no SourceID/ExternalID —
    // fall through to the keychain path below.
    if current.ExternalURL == "" {
        return
    }
    handleForwardSyncCloseByExternalURL(ctx, syncLoop, current)
    return
}
```

This keeps both paths in the same function, which is good for readability — a
reviewer sees the full close policy in one place. Downside: `handleForwardSyncClose`
already has non-trivial length (source-lookup, decrypt, switch on plugin type,
comment, watermark persist); the keychain-only arm duplicates pieces of that
boilerplate. The function's complexity does not cross `gocognit`'s gate with a
single new branch, but it is worth verifying with `make lint` after the change.

**Option B — extracted helper `handleForwardSyncCloseByExternalURL`.**

Pull the keychain-only path into a peer function called at the branch point above.
This is what `resolveProgressTarget` does for progress sync: a narrow helper
dedicated to the "no-SourceID" resolution path. The main function's inner `switch`
on plugin type and the comment/watermark boilerplate stay where they are; the helper
only owns the config-derivation step and delegates back to a shared closer.
Recommended — it separates the "how do we get a config?" question from the "how do
we close?" question, matching the structure already in the progress-sync file.

**Option C — new subscriber.**

A separate `StartBacklogGitHubExternalURLCloseSubscriber` consuming the same
`EventBacklogItemChanged` event. Only justified if the two paths diverge
significantly in retry, error-recording, or flag logic. They don't — both paths
call `CloseIssue` on the same plugin and use the same comment/watermark persist.
Not recommended: it adds subscriber registration boilerplate and a second goroutine
for a path that is structurally identical to the existing one.

**Recommendation: Option B.** Extract a `resolveForwardSyncCloseTarget` helper
(symmetric name to `resolveProgressTarget`) that returns `(plugin externalIssueCloser,
config session.PluginConfig, externalID string, ok bool)` by the same two-branch
logic. The SourceID path uses the existing source-lookup+decrypt; the ExternalURL
path mirrors `resolveProgressTarget`'s fallback arm exactly (lines 190–198 of
`backlog_github_progress_sync.go`). `handleForwardSyncClose` calls it once and
proceeds to the `switch p := plugin.(type)` block with no structural change there.

---

## 3. FR2 Integration Design: `Closes #N` in PR bodies

### Available context at the injection point

`prFooter(origin PRFooterOrigin, item *BacklogItemData)` already has:
- `item.ExternalURL` — the GitHub issue URL
- No PR repo context — `origin` holds only the stapler-squad host identity, not
  the git remote URL

`buildFallbackPRBody` calls `prFooter`; `appendBacklogFooter` calls `prFooter`.
Both are called from `pushAndCreatePR` after `wt` (the worktree struct) is in
scope. `wt.RepoPath` is available; `git.RemoteURL(wt.RepoPath, "origin")` can
return the PR repo's remote URL.

### Injection options

**Option A — inside `prFooter` without repo context.**

`prFooter` can call `ParseGitHubRefWithHosts(item.ExternalURL, nil)` and emit
`Closes #N` unconditionally if the parse succeeds. No repo comparison possible
here — if the PR is in a fork or a different repo the reference would still be
valid (GitHub resolves `Closes #N` relative to the PR's target repo, so it only
autolinks if the repos match), and the footer text `Closes #123` without a full
URL is still useful human documentation even when no autolink fires.

Tradeoff: may produce a confusing `Closes #123` in a PR that is against a
different repo (e.g. a fork). The issue number is taken from an external service
and is unambiguous in context, but GitHub won't auto-close it. For the common case
(item's issue and PR are in the same repo), the autolink fires correctly.

**Option B — in `pushAndCreatePR` before calling `buildFallbackPRBody`/`appendBacklogFooter`, with repo comparison.**

Pass the issue reference into `prFooter` via an extended `PRFooterOrigin` or a
separate parameter after comparing the parsed PR remote against the parsed issue
URL. `git.RemoteURL(wt.RepoPath, "origin")` is already available at the call
site; `ParseGitHubRefWithHosts` can parse both URLs and compare
`ref.Owner == issueRef.Owner && ref.Repo == issueRef.Repo`.

Tradeoff: more logic in the lifecycle file, and `git.RemoteURL` adds one go-git
read per PR creation. The benefit is correctness: `Closes #N` is only injected
when it will actually autolink. This is the safer choice if the repo set includes
GHE or if users own items whose PRs land in forks.

**Option C — extend `PRFooterOrigin` with an optional issue reference.**

Add `IssueRef *github.GitHubRef` to `PRFooterOrigin`. The lifecycle sets it
after the same `ParseGitHubRefWithHosts` call with repo comparison. `prFooter`
emits `Closes #N` only when `origin.IssueRef != nil`. This is a clean separation:
the lifecycle owns the "should we link?" decision (repo comparison) and `prFooter`
owns the rendering.

**Recommendation: Option C.** It keeps rendering in `prFooter` (single render
path for both body-building functions), puts the repo-comparison decision where
the worktree context is available (the lifecycle), and extends the data structure
rather than adding a parameter. The repo-comparison step uses
`git.RemoteURL(wt.RepoPath, "origin")` + `ParseGitHubRefWithHosts` and is best
gated: if the parse or remote-URL read fails, `IssueRef` stays nil and no
`Closes` line is emitted (safe default).

For `RecordPRCreatedOutOfBand`: this path has no worktree access, so it cannot do
a repo comparison. It should be left out of scope for FR2 — the agent-driven ship
path runs `/backlog/ship` → `/github:pr-ship`, which writes its own PR body; the
fallback `pushAndCreatePR` path is what FR2 covers.

### Where `Closes #N` renders in the footer

After `prFooter`'s existing item-link line, before the provenance block:

```
Backlog item: <link> (ID <id>)
Closes #<N>

<!-- ssq ... -->
```

This matches GitHub's convention (the autolink line is the last human-readable
line before machine-readable metadata).

---

## 4. Tech Debt Disposition

**Recommendation: Isolate via seam (Option B from §2).**

The prior research flagged `handleForwardSyncClose` as a complexity growth area.
Extracting `resolveForwardSyncCloseTarget` (the config-derivation seam) mirrors
exactly how `backlog_github_progress_sync.go` factored out `resolveProgressTarget`
after the same pattern emerged there. This is not a large refactor — it moves the
source-lookup + decrypt + plugin-cast sequence into a named helper and leaves the
close/comment/watermark sequence in place. The result is a function that reads as
two steps ("resolve where to close" then "close it") rather than one monolithic
blob. The `gocognit` gate (`make lint`) is the definitive check; run it after
extracting.

Extending-as-is (adding a second `else if` arm in the existing function without
extraction) is acceptable for a single change but leaves the function in a shape
that will require extraction when a third config path appears. The seam extraction
is cheap now.

---

## 5. Consistency with `resolveProgressTarget`

`resolveProgressTarget` (`backlog_github_progress_sync.go:171–198`) is the
canonical pattern for keychain-only config derivation from ExternalURL. Its two
branches:

1. `SourceID != "" && ExternalID != ""` → `GetItemSourceByID` → `DecryptConfigToken`
   → cast to `issueProgressCommenter`
2. ExternalURL fallback → `ParseGitHubRefWithHosts(item.ExternalURL, nil)` → marshal
   `{"host": ref.Host, "owner": ref.Owner, "repo": ref.Repo}` → `PluginConfig{Raw: ...}`
   → `&GitHubIssuesPlugin{}` → `fmt.Sprint(ref.IssueNumber)` as externalID

The `resolveForwardSyncCloseTarget` helper for FR1 must mirror this exactly.
Divergence risks:

| Risk | Mitigation |
|---|---|
| GHE host skip: `ParseGitHubRefWithHosts(..., nil)` passes no host list, so GHE URLs return an error and the path skips — same as progress-sync | Match the nil host-list argument; add a comment referencing the progress-sync rationale |
| `externalID` as string: progress-sync uses `fmt.Sprint(ref.IssueNumber)` to convert the int; `CloseIssue` expects the same string form that `ExternalID` holds in source-linked items | Use `fmt.Sprint(ref.IssueNumber)` consistently — do not use `strconv.Itoa`, which is equivalent but diverges stylistically |
| `ForwardSyncCloseLabel`: for keychain-only items there is no `ItemSource.ForwardSyncCloseLabel`. The existing close path passes `source.ForwardSyncCloseLabel` to `CloseIssue`; for the ExternalURL arm, pass `""` (no extra label). `CloseIssue`'s implementation treats `""` as "close with no label change", which is the right behaviour for MCP-imported items with no source configuration | Document this explicitly in the helper; do not silently omit it |
| `externalIssueStateUpdater` interface: the `switch p := plugin.(type)` block handles `externalIssueStateUpdater` for non-GitHub plugins. `GitHubIssuesPlugin` only satisfies `externalIssueCloser`; the keychain-only path will never see `externalIssueStateUpdater`. The extraction makes this self-documenting | The helper should return `externalIssueCloser`, not `plugin.Plugin`, to enforce this at compile time |

---

## 6. File Touch Map

| File | Change |
|---|---|
| `server/services/backlog_github_forward_sync.go` | Add `resolveForwardSyncCloseTarget` helper; modify early-return guard at line 142; call helper before existing switch |
| `session/backlog_lifecycle_pr.go` | Add `IssueRef *github.GitHubRef` to `PRFooterOrigin`; update `pushAndCreatePR`'s `origin` construction; update `prFooter` to emit `Closes #N` |
| `session/backlog_plugin_github.go` | No change (FR1 uses `CloseIssue` as-is) |
| `github/url_parser.go` | No change (FR2 uses `ParseGitHubRefWithHosts` as-is) |
