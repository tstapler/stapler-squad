# Implementation Plan: github-issue-cleanup

**Feature**: Close GitHub issues and inject closing keywords when backlog items imported via MCP or web UI transition to done
**Date**: 2026-11-03
**Status**: Ready for implementation
**ADRs**: None (all choices use existing patterns)

---

## Alternatives Explored (Step 0.5 — Creative Pass)

Three approaches were considered before committing to the design below:

| Approach | Strength | Weakness | Verdict |
|----------|----------|----------|---------|
| **A — Extend `handleForwardSyncClose` with an MCP-import branch** (chosen) | Mirrors the proven `resolveProgressTarget` pattern already in the codebase; no new goroutines, no new event types; watermark advancement is mechanically identical to the existing SourceID path. | `handleForwardSyncClose` grows to ~180 lines; must be careful not to break the SourceID early-return semantics. | **Chosen** |
| **B — Add a second EventBus subscriber dedicated to MCP-import closes** | Clean separation; MCP path can't accidentally break SourceID path. | Two subscribers consume the same event; no shared state means watermark logic must be duplicated; subscriber count grows; `StartBacklogGitHubForwardSyncSubscriber`'s wiring becomes non-obvious. | Rejected — duplication without clear gain |
| **C — Handle MCP closes inside `import_github_issue` on item completion poll** | Keeps "imported via MCP" logic co-located with import code. | No completion hook exists on MCP import; polling the item from MCP context would require a new storage query path and race with the EventBus subscriber; two code paths closing the same issue is a loop-prevention problem. | Rejected — introduces race and duplication |

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name.)*

| Term | Definition | Notes |
|------|-----------|-------|
| **BacklogItemData** | The Go struct representing a backlog item, including `ExternalURL`, `ExternalID`, `SourceID`, `PrURL`, and `GitHubSyncedIssueUpdatedAt`. | `session/repository.go` |
| **ExternalURL** | The browser-facing URL of the linked external issue or PR (e.g. `https://github.com/owner/repo/issues/42`). Set by `import_github_issue` and the web UI import; always absent for locally-created items. | `BacklogItemData.ExternalURL` |
| **SourceID** | The ID of the `ItemSource` row for items ingested by the periodic backward sync. MCP-imported items have `ExternalURL` but no `SourceID`. | `BacklogItemData.SourceID` |
| **externalIssueCloser** | Interface implemented by `*GitHubIssuesPlugin` exposing `CloseIssue` and `PostIssueComment`; type-asserted in `handleForwardSyncClose`. | `server/services/backlog_github_forward_sync.go` |
| **issueStateReader** | New interface exposing `ReadIssueState`, used to perform the skip-if-closed pre-GET check before calling `CloseIssue`. Separated from `externalIssueCloser` so the fake test double can implement it independently and the type assertion degrades gracefully for other plugin types. | To be added to `server/services/backlog_github_forward_sync.go` |
| **resolveCloseTarget** | New helper mirroring `resolveProgressTarget`; resolves the plugin, config, and externalID for a done-transition close, covering both SourceID-linked items (Path A) and MCP-imported items (Path B). | To be added to `server/services/backlog_github_forward_sync.go` |
| **resolveProgressTarget** | Existing helper in `backlog_github_progress_sync.go` that is the reference implementation for token resolution on MCP-imported items. | `server/services/backlog_github_progress_sync.go:171` |
| **forwardSyncCloseCommentWithPR** | New helper composing the close comment, optionally appending the implementing PR URL when `item.PrURL != ""`. | To be added to `server/services/backlog_github_forward_sync.go` |
| **closingKeywordFor** | Helper returning `"Closes"`, `"Related:"`, or `""` based on whether `ExternalURL` contains `/issues/` or `/pull/`. Used by `buildSessionInitialPrompt` only (full-URL form is fine in agent instructions — no same-repo check needed there). `buildFallbackPRBody` and `appendBacklogFooter` use `closingKeywordWithRepo` instead. | To be added to `session/backlog_context.go` |
| **closingKeywordWithRepo** | New helper that parses both the issue URL and the PR's remote URL with `ParseGitHubRefWithHosts`, then returns `"Closes #<N>"` (bare number) when owner+repo match, or `""` (omit) when cross-repo or either URL fails to parse. Used by `buildFallbackPRBody` (Task 1.1.1b) and `appendBacklogFooter` (Task 1.1.1d). | To be added to `session/backlog_context.go` |
| **GitHubSyncedIssueUpdatedAt** | Loop-prevention watermark: the GitHub `updated_at` timestamp after a write, stored to prevent backward sync re-ingesting the item on the next poll. | `BacklogItemData.GitHubSyncedIssueUpdatedAt` |
| **watermark** | Local variable name used throughout the forward-sync paths for the `time.Time` value advanced after a write to GitHub. Matches existing code style. | Convention from `handleForwardSyncClose` |
| **buildFallbackPRBody** | Function in `session/backlog_lifecycle_pr.go` that constructs a PR body from item data when no LLM call is available. AC-3 adds a `Closes #N` closing keyword here (same-repo only, bare number form; omitted cross-repo) using `closingKeywordWithRepo`. Requires the PR's remote URL to be passed in from `pushAndCreatePR`. | `session/backlog_lifecycle_pr.go:477` |
| **appendBacklogFooter** | Function appending the LLM-drafted body with a footer. FR2 requires this function to also inject a closing keyword; Task 1.1.1d adds the same `closingKeywordWithRepo` logic here. | `session/backlog_lifecycle_pr.go:464` |
| **BuildSessionInitialPrompt** | Renders the full context prompt injected into an agent session's initial message. AC-4 adds a "Linked GitHub Issue" fact line and a "Fixes" instruction line here. | `session/backlog_context.go:222` |
| **BuildTokenBudgetedPrompt** | Wraps `BuildSessionInitialPrompt` with token-budget enforcement; both passes call `buildSessionInitialPrompt`, which is where the new lines must be inserted to survive truncation. | `session/backlog_context.go:312` |
| **ParseGitHubRefWithHosts** | Parses a GitHub URL into `{Host, Owner, Repo, IssueNumber, Type}`. Passing `nil` for the host list accepts only `github.com`. Used to decode `ExternalURL` in the MCP-import path. | `github.ParseGitHubRefWithHosts` |
| **RefTypeIssue / RefTypePR** | Constants from the `github` package distinguishing issue URLs (`/issues/`) from PR URLs (`/pull/`). Used by both `resolveCloseTarget` and `closingKeywordFor`. | `github.RefTypeIssue`, `github.RefTypePR` |
| **forwardSyncCloseCommentFor** | Existing helper composing the close comment with optional host suffix. `forwardSyncCloseCommentWithPR` delegates to it. | `server/services/backlog_github_forward_sync.go:50` |
| **MCP-imported item** | A `BacklogItemData` created by `import_github_issue` (or the web UI import action): has `ExternalURL` and `ExternalID` but `SourceID == ""`. | Contrast with source-linked item (periodic backward sync). |
| **source-linked item** | A `BacklogItemData` ingested by the periodic backward sync: has `SourceID`, `ExternalID`, and usually `ExternalURL`. The existing `handleForwardSyncClose` SourceID path covers these. | Existing behavior — must not regress. |
| **readIssueUpdatedAt** | Existing private function in `session/backlog_plugin_github.go` that GETs an issue and returns its `updated_at`. Extended (renamed `readIssueState`) to also return `state` for the skip-if-closed check. | `session/backlog_plugin_github.go:432` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `resolveCloseTarget` | **Template Method** (Path A / Path B dispatch in one function) | PoEAA §10 (Gateway) + existing `resolveProgressTarget` | Separate functions per path | Single function keeps both paths visibly consistent; mirrors existing precedent exactly |
| `forwardSyncCloseCommentWithPR` | **Decorator** (wraps `forwardSyncCloseCommentFor`, appends PR suffix) | GoF Decorator | Inline string building at call site | Isolates comment format in one place; testable independently |
| `closingKeywordFor` | **Pure function classifier** (string → string), used by `buildSessionInitialPrompt` only | Type-driven design (total function, no side effects) | Inline `strings.Contains` at call site | Single call site in `buildSessionInitialPrompt`; PR-body injection uses `closingKeywordWithRepo` instead. The agent instruction embeds the full URL (no same-repo check needed), and uses `"Fixes"` (a valid GitHub closing keyword that reads naturally as an instruction verb). |
| `handleForwardSyncClose` refactor | **Extend as-is** (add MCP branch after existing SourceID-not-empty check) | "Extend as-is" architecture recommendation | Extract full refactor of function | Function is 102 lines, below cyclomatic threshold; adding a branch is surgical and low risk |
| Watermark advancement | **Same pattern as existing SourceID path** (GitHub `updated_at` → fallback `time.Now().UTC()`) | ADR-003 (loop-prevention watermark) | Local time only | GitHub's own timestamp guarantees the watermark post-dates the event the backward sync would see |
| Token-budget survival (AC-4) | **Insert into `buildSessionInitialPrompt` (not callers)** | Existing pattern for `item.PlanArtifactsPath` check at line 301 | Inject in `BuildSessionInitialPrompt` wrapper | Both truncation passes call `buildSessionInitialPrompt`; inserting there guarantees the new lines survive pass 1 and pass 2 |
| New test file location | **Same package as changed file** (`_test.go` sibling) | Go standard | Separate integration package | Keeps test helpers close to implementation; existing tests follow this pattern |
| Skip-if-closed interface | **Separate `issueStateReader` interface** (optional type assertion) | Interface segregation; existing `issueProgressCommenter`/`externalIssueCloser` precedent | Add `ReadIssueState` to `externalIssueCloser` | Adding to `externalIssueCloser` would break all existing fakes; a separate optional interface degrades gracefully for plugin types that don't support state reads | 
| Skip-if-closed pre-GET timing | **Pre-GET before `CloseIssue` PATCH** | FR3 explicit requirement; avoids wasted write call on idempotent close | Check PATCH response state post-close | FR3 and AC-2 require the close call be skipped entirely ("no second close is attempted"), not just the comment. Post-close checking cannot skip the PATCH. |
| `Closes #N` keyword in PR body | **`closingKeywordWithRepo`**: same owner+repo → `Closes #N` bare number; cross-repo or parse failure → `""` (omit) | AC-3 (bare `#N` required for same-repo) + AC-4 (keyword omitted cross-repo) | `Closes <full-url>` for all items | GitHub's full-URL closing keyword works across repos — it would auto-close a cross-repo issue, violating AC-4. Bare `#N` is the GitHub-canonical same-repo form; cross-repo items get no keyword at all. `closingKeywordFor` (full-URL classifier) is kept for `buildSessionInitialPrompt` only, where the agent instruction embeds the full URL — no same-repo check needed. |

---

## Tech Debt Disposition
*(Every hotspot/architecture violation touched by this feature.)*

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `handleForwardSyncClose` SourceID early-return | Line 142: `if current.SourceID == "" \|\| current.ExternalID == "" { return }` silently drops all MCP-imported items | **Fix in this feature** — replace the single early-return with a `resolveCloseTarget` call that covers both paths; also add `context.WithTimeout(ctx, forwardSyncCloseTimeout)` at the top of the function (Task 1.1.2a-timeout) to bound the new live API-call sequence | The gap is the primary AC-1 bug; the timeout mirrors `handleProgressComment`'s `progressCommentTimeout` guard |
| `forwardSyncCloseComment` has no PR URL | Existing close comment is a fixed string with no reference to the implementing PR | **Fix in this feature** — add `forwardSyncCloseCommentWithPR` and pass `current.PrURL` at the call site | Zero risk of breaking existing behavior when `PrURL == ""` (string unchanged) |
| `buildFallbackPRBody` has no closing keyword | PR body carries no "Closes" line; GitHub does not auto-link the issue | **Fix in this feature** — insert `closingKeywordWithRepo` check after Summary section (Task 1.1.1b); requires passing PR remote URL from `pushAndCreatePR` (Task 1.1.1c); same-repo → `Closes #N`; cross-repo → omit | AC-3/AC-4; guarded by `ExternalURL != ""` so no regression for plain items |
| `appendBacklogFooter` has no closing keyword | LLM-drafted PR body also carries no "Closes" line; FR2 requires both paths | **Fix in this feature** — insert the same `closingKeywordWithRepo` check in `appendBacklogFooter` (Task 1.1.1d); same-repo → `Closes #N`; cross-repo → omit | FR2; `appendBacklogFooter` already receives backlog item data; needs PR remote URL threaded in from its call site too |
| Agent prompt omits `ExternalURL` | `BuildSessionInitialPrompt` renders no "Linked GitHub Issue" line or "Fixes" instruction | **Fix in this feature** — add fact line and instruction line inside `buildSessionInitialPrompt` after the AC checklist, before `--- END BACKLOG ITEM DATA ---` | AC-4; inserting before the end marker means it always appears regardless of truncation pass |
| `handleForwardSyncClose` no skip-if-closed guard | The forward-sync close never checks whether the issue is already closed, causing a redundant write and duplicate comment when GitHub's own PR-merge auto-close runs before the done transition | **Fix in this feature** — add `ReadIssueState` to `*GitHubIssuesPlugin`, add `issueStateReader` interface, pre-GET before `CloseIssue` call; skip close+comment and advance watermark when `state == "closed"` | FR3/AC-2: AC-2 requires "no second close is attempted"; the GET response also provides the watermark `updated_at` to advance |
| `readIssueUpdatedAt` only returns `updated_at` | The existing private function GETs the issue but discards `state`; must be widened to return `state` for FR3 | **Fix in this feature** — rename private function to `readIssueState` returning `(state string, updatedAt time.Time, err error)`, update callers | No behavior change for callers that only use `updatedAt`; `state` is a free extra from the same GET response body |

---

## Migration Plan
*(N/A — no schema or data changes.)*

No new ent schema fields. No new DB migrations. `ExternalURL`, `PrURL`, and `GitHubSyncedIssueUpdatedAt` already exist in `BacklogItemData` and the ent schema. No generated `session/ent/` files to touch.

---

## Observability Plan

- **Logs**:
  - `log.Warn("backlog_github_forward_sync: no token for MCP-imported item", "host", host, "item", item.ID)` — emitted when `resolveCloseTarget` Path B finds no keychain token. **Warn not Debug**: matches the SourceID path's decrypt-failure handling and makes silent skips diagnosable without log-level changes.
  - `log.Info("backlog_github_forward_sync: closed MCP-imported issue", "item", item.ID, "external_url", item.ExternalURL)` — emitted after a successful close on Path B.
  - `log.Debug("backlog_github_forward_sync: issue already closed, skipping close", "item", item.ID, "external_url", item.ExternalURL)` — emitted when `ReadIssueState` returns `state == "closed"`; **Debug not Warn**: this is a normal, expected condition when GitHub's own PR-merge keyword already closed the issue.
  - Existing `log.Warn("backlog_github_forward_sync: post comment failed", ...)` already covers comment post failures on both paths.
- **Metrics**: No new Prometheus counters. **Production success signal for AC-1**: count `log.Info("closed MCP-imported issue", ...)` lines per time window via the log-clustering monitor. If the count stays at zero after ship, the guard condition or token-miss scenario is broken.
- **Alerts**: No new alerts. The `log.Warn("no token for MCP-imported item", ...)` line is the operator signal for consistent skip conditions.

---

## Risk Control

- **Feature flag**: None. AC-1's MCP-import close is always-on once a token is available, per the requirements. The existing `github_issue_progress_comments` flag does not gate this path. If a token is absent, the close is skipped (Warn log only), limiting blast radius to a missed close.
- **Rollback procedure**: Revert the `handleForwardSyncClose` change and redeploy. The watermark advancement means a reverted item will not be re-closed by backward sync. No DB state rollback needed.
- **Staged rollout**: Not applicable (no flag). The change is gated naturally by `ExternalURL` presence; items without it are entirely unaffected. The `resolveCloseTarget` Path B branch is only reached when `SourceID == ""` and `ExternalURL != ""`.

---

## Unresolved Questions

None. All design decisions are covered by requirements, research, and existing precedent.

---

## Dependency Visualization

```
Task 1.1.1a (closingKeywordFor helper — buildSessionInitialPrompt only)

Task 1.1.1c (closingKeywordWithRepo helper + pass prRemoteURL to buildFallbackPRBody)
    └── Task 1.1.1b (buildFallbackPRBody — uses closingKeywordWithRepo)
    └── Task 1.1.1d (appendBacklogFooter — uses closingKeywordWithRepo)

Task 1.1.2a (forwardSyncCloseCommentWithPR helper)

Task 1.1.2a-timeout (forwardSyncCloseTimeout constant)

Task 1.1.2b (resolveCloseTarget helper — 5-tuple with closeLabel)

Task 1.1.2e (ReadIssueState on *GitHubIssuesPlugin + issueStateReader interface)

[1.1.2a, 1.1.2a-timeout, 1.1.2b, 1.1.2e must all complete before 1.1.2c]
    └── Task 1.1.2c (handleForwardSyncClose — uses all helpers + timeout + skip-if-closed)

Task 1.1.3a (BuildSessionInitialPrompt — uses closingKeywordFor)

Tasks 1.1.4a–b (tests — depend on all production changes above)

[1.1.1a and 1.1.2a can run in parallel]
[1.1.1c can run in parallel with 1.1.1a and 1.1.2a]
[1.1.3a can run in parallel with 1.1.2x]
[1.1.1b depends on 1.1.1c]
[1.1.1d depends on 1.1.1c]
[1.1.1b and 1.1.1d can run in parallel (both depend on 1.1.1c only)]
[1.1.2b depends on 1.1.2a]
[1.1.2a-timeout is independent of 1.1.2b and 1.1.2e (can run in parallel with both)]
[1.1.2e is independent of 1.1.2b (can run in parallel)]
[1.1.2c depends on 1.1.2b, 1.1.2a-timeout, and 1.1.2e]
[1.1.4a depends on 1.1.2c]
[1.1.4b depends on 1.1.1b, 1.1.1d, and 1.1.3a]
```

---

## Phase 1: GitHub Issue Lifecycle Completion

### Epic 1.1: Close, comment, and prompt linking for GitHub-backed backlog items
**Goal**: When a backlog item with a linked GitHub issue transitions to `done`, the issue is closed with a comment naming the implementing PR, the fallback PR body and LLM-drafted PR body each include a `Closes #N` line (same-repo) or no keyword (cross-repo), and the agent prompt includes a `Fixes <url>` instruction — covering both SourceID-linked and MCP-imported items, with a pre-GET skip when the issue is already closed.

---

#### Story 1.1.1: PR body carries GitHub closing keyword (AC-3, AC-4 — both `buildFallbackPRBody` and `appendBacklogFooter`)
**As a** GitHub issue watcher, **I want** the PR that implements my issue to include a `Closes #N` link in its body, **so that** GitHub automatically links and closes the issue on merge; and I want no such keyword when the issue is on a different repo, to avoid cross-repo misfires.

**Acceptance Criteria**:
- When `buildFallbackPRBody` is called for a same-repo item whose `ExternalURL` is `https://github.com/acme/widget/issues/42` and the PR remote URL is also `acme/widget`, the returned body includes the line `Closes #42` (bare number, not full URL).
  - *Given* `ExternalURL = "https://github.com/acme/widget/issues/42"`, PR remote URL `https://github.com/acme/widget.git`, *When* `buildFallbackPRBody` is called, *Then* the returned string contains `"Closes #42"`.
- When `appendBacklogFooter` is called for a same-repo item, the returned body also includes `Closes #42`.
  - *Given* same inputs as above, *When* `appendBacklogFooter` is called, *Then* the returned string contains `"Closes #42"`.
- When the PR is in a different repo than the issue (cross-repo), neither `buildFallbackPRBody` nor `appendBacklogFooter` emits a closing keyword.
  - *Given* `ExternalURL = "https://github.com/acme/widget/issues/42"` and PR remote URL `https://github.com/other/project.git`, *When* either function is called, *Then* the returned string contains neither `"Closes"` nor `"Related:"`.
- When `ExternalURL` is empty, the body is unchanged (no keyword added).
  - *Given* `ExternalURL = ""`, *When* either function is called, *Then* the returned string contains neither `"Closes"` nor `"Related:"`.

**Files**: `session/backlog_context.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_pr_test.go`

##### Task 1.1.1a: Add `closingKeywordFor` helper (XS)
- In `session/backlog_context.go`, add a package-level pure function:
  ```go
  // closingKeywordFor returns the GitHub closing keyword for url: "Closes" for
  // issue URLs (/issues/), "Related:" for PR URLs (/pull/), or "" for unrecognized
  // or empty URLs. Used by buildSessionInitialPrompt only (full-URL form in agent
  // instructions; no same-repo check needed there). PR-body injection uses
  // closingKeywordWithRepo instead.
  func closingKeywordFor(url string) string {
      if strings.Contains(url, "/issues/") {
          return "Closes"
      }
      if strings.Contains(url, "/pull/") {
          return "Related:"
      }
      return ""
  }
  ```
- Place after `sanitizeField` / `truncateField` helpers (before line 39).
- Files: `session/backlog_context.go`

##### Task 1.1.1b: Insert closing keyword into `buildFallbackPRBody` using `closingKeywordWithRepo` (S)
- Add `prRemoteURL string` as a parameter to `buildFallbackPRBody` (called from `pushAndCreatePR`, which already has `wt.RepoPath` available — pass the remote URL resolved from that path).
- Inside `buildFallbackPRBody` (line 477), after the Summary `fmt.Fprintf` call and before `sb.WriteString("\n" + prFooter(...))`, insert:
  ```go
  if line := closingKeywordWithRepo(item.ExternalURL, prRemoteURL); line != "" {
      fmt.Fprintf(&sb, "\n%s\n", line)
  }
  ```
  Note: `closingKeywordWithRepo` returns the full formatted line (e.g. `"Closes #42"`), not just the keyword.
- Add a table-driven unit test in `session/backlog_lifecycle_pr_test.go` covering:
  1. Same-repo issue (`ExternalURL` and `prRemoteURL` share `owner/repo`) → body contains `"Closes #42"` (bare number).
  2. Cross-repo issue (different repos) → body contains neither `"Closes"` nor `"Related:"`.
  3. `ExternalURL` empty → body is unchanged.
- Files: `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_pr_test.go`

##### Task 1.1.1c: Add `closingKeywordWithRepo` and pass PR remote URL to `buildFallbackPRBody` (S)
- In `session/backlog_context.go`, after `closingKeywordFor`, add:
  ```go
  // closingKeywordWithRepo returns "Closes #<N>" when issueURL and prRemoteURL
  // resolve to the same GitHub owner+repo, or "" when they differ, either URL
  // fails to parse, or issueURL is not a GitHub issue URL.
  // This is the PR-body injection form (bare #N, not full URL); GitHub only
  // supports cross-repo closing keywords for full URLs, but that would auto-close
  // unrelated issues — so cross-repo items get no keyword at all (AC-4).
  func closingKeywordWithRepo(issueURL, prRemoteURL string) string {
      issueRef, err := githubpkg.ParseGitHubRefWithHosts(issueURL, nil)
      if err != nil || issueRef.Type != githubpkg.RefTypeIssue {
          return ""
      }
      prRef, err := githubpkg.ParseGitHubRefWithHosts(prRemoteURL, nil)
      if err != nil {
          return ""
      }
      if issueRef.Owner != prRef.Owner || issueRef.Repo != prRef.Repo {
          return ""
      }
      return fmt.Sprintf("Closes #%d", issueRef.IssueNumber)
  }
  ```
- In `pushAndCreatePR` (the call site for `buildFallbackPRBody`), resolve the PR's remote URL from `wt.RepoPath` using the existing `git.GetRemoteURL` helper and pass it to `buildFallbackPRBody`.
- Files: `session/backlog_context.go`, `session/backlog_lifecycle_pr.go`

##### Task 1.1.1d: Inject closing keyword into `appendBacklogFooter` using `closingKeywordWithRepo` (S)
- `appendBacklogFooter` (line 464) also generates PR body text and is in scope per FR2. Apply the same injection pattern as Task 1.1.1b:
  - Add `prRemoteURL string` as a parameter to `appendBacklogFooter`.
  - After the existing footer text, before `return`, insert:
    ```go
    if line := closingKeywordWithRepo(item.ExternalURL, prRemoteURL); line != "" {
        body += "\n" + line + "\n"
    }
    ```
  - Thread `prRemoteURL` through from `appendBacklogFooter`'s call site (same `pushAndCreatePR` path that calls `buildFallbackPRBody`; use the same resolved remote URL).
- Add a table-driven unit test in `session/backlog_lifecycle_pr_test.go` covering:
  1. Same-repo issue → footer contains `"Closes #42"`.
  2. Cross-repo issue → footer contains neither `"Closes"` nor `"Related:"`.
  3. `ExternalURL` empty → footer is unchanged.
- Files: `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_pr_test.go`

---

#### Story 1.1.2: MCP-imported items are closed on done; skip if already closed; comment names the PR (AC-1, AC-2, AC-5, AC-6)
**As a** GitHub issue watcher, **I want** issues imported via `import_github_issue` to be automatically closed when the backlog item is marked done, skipping silently when GitHub already closed the issue via PR merge, and to see the implementing PR URL in the close comment.

**Acceptance Criteria**:
- AC-1: When a backlog item with `SourceID = ""`, `ExternalURL = "https://github.com/acme/widget/issues/42"` transitions to `done`, `CloseIssue` is called on `*GitHubIssuesPlugin` and `PostIssueComment` is called with a comment that includes `"Closed automatically"`.
  - *Given* `SourceID = ""`, `ExternalURL = "https://github.com/acme/widget/issues/42"`, `ExternalID = "42"`, `PrURL = ""`, transitioning to `done`, *When* `handleForwardSyncClose` runs, *Then* `CloseIssue("42", ...)` and `PostIssueComment("42", "Closed automatically — ...")` are called.
- AC-2: When the backing issue is already `state == "closed"` (read from the pre-GET), `CloseIssue` is NOT called, `PostIssueComment` is NOT called, a DEBUG log is emitted, and the watermark is advanced from the GET response's `updated_at`.
  - *Given* `ReadIssueState` returns `("closed", 2026-11-01T00:00:00Z, nil)` for issue `"42"`, *When* `handleForwardSyncClose` runs, *Then* `CloseIssue` is NOT called, `PostIssueComment` is NOT called, `log.Debug("... already closed ...")` is emitted, and `UpdateBacklogItem` is called with `GitHubSyncedIssueUpdatedAt = 2026-11-01T00:00:00Z`.
- When `item.PrURL = "https://github.com/acme/widget/pull/99"`, the comment is `"Closed automatically — the linked backlog item was marked done in stapler-squad. Implemented in: https://github.com/acme/widget/pull/99."`.
  - *Given* `PrURL = "https://github.com/acme/widget/pull/99"`, *When* `handleForwardSyncClose` runs (either Path A or Path B), *Then* `PostIssueComment` receives the comment ending with `"Implemented in: https://github.com/acme/widget/pull/99."`.
- When no keychain token is available for the host, the close is skipped and a Warn log is emitted; `CloseIssue` is NOT called.
  - *Given* `ExternalURL = "https://github.com/acme/widget/issues/42"` and no token in the keychain for `github.com`, *When* `handleForwardSyncClose` runs, *Then* `CloseIssue` is NOT called and a Warn log appears.
- AC-5: After a successful close, `storage.UpdateBacklogItem` is called with `GitHubSyncedIssueUpdatedAt` from the GitHub API.
  - *Given* `CloseIssue` returns `updatedAt = 2026-11-01T00:00:00Z`, *When* `handleForwardSyncClose` runs, *Then* `UpdateBacklogItem` is called with `GitHubSyncedIssueUpdatedAt = 2026-11-01T00:00:00Z`.
- AC-6: Existing SourceID-path behavior is unchanged (no regression).

**Files**: `server/services/backlog_github_forward_sync.go`, `server/services/backlog_github_forward_sync_test.go`, `session/backlog_plugin_github.go`

##### Task 1.1.2a: Add `forwardSyncCloseCommentWithPR` helper (XS)
- In `server/services/backlog_github_forward_sync.go`, after `forwardSyncCloseCommentFor` (line 55), add:
  ```go
  // forwardSyncCloseCommentWithPR composes the close comment, appending the
  // implementing PR URL when prURL is non-empty.
  func forwardSyncCloseCommentWithPR(hostID, prURL string) string {
      base := strings.TrimSuffix(forwardSyncCloseCommentFor(hostID), ".")
      if prURL == "" {
          return base + "."
      }
      return base + fmt.Sprintf(" Implemented in: %s.", prURL)
  }
  ```
- Files: `server/services/backlog_github_forward_sync.go`

##### Task 1.1.2a-timeout: Add `forwardSyncCloseTimeout` constant (XS)
- In `server/services/backlog_github_forward_sync.go`, add a constant after `const forwardSyncCloseComment = "..."`:
  ```go
  // forwardSyncCloseTimeout bounds the state-read + close + comment network sequence,
  // mirroring progressCommentTimeout in backlog_github_progress_sync.go (15s for one
  // POST; 30s for GET + PATCH + POST).
  const forwardSyncCloseTimeout = 30 * time.Second
  ```
- At the top of `handleForwardSyncClose`, immediately after the nil/empty guards (before the storage.GetBacklogItem call), add:
  ```go
  ctx, cancel := context.WithTimeout(ctx, forwardSyncCloseTimeout)
  defer cancel()
  ```
  **Exception**: The watermark `storage.UpdateBacklogItem` call at the end of `handleForwardSyncClose` MUST use `context.Background()` (not the timeout-derived ctx) so the local DB write is never cancelled after `CloseIssue` has already committed on GitHub.
- Files: `server/services/backlog_github_forward_sync.go`

##### Task 1.1.2b: Add `resolveCloseTarget` helper (S)
- In `server/services/backlog_github_forward_sync.go`, add after `forwardSyncCloseCommentWithPR`:
  ```go
  // resolveCloseTarget finds the plugin, config, and externalID for a done-transition
  // close. Mirrors resolveProgressTarget in backlog_github_progress_sync.go.
  //
  // Path A (SourceID != ""): uses item's ItemSource config and plugin.
  // Path B (SourceID == "", ExternalURL != ""): builds config from ExternalURL;
  //   token is resolved by GitHubIssuesPlugin from the keychain at call time.
  //   Only github.com issue URLs are supported (no host list → GHE skipped).
  // ok is false when no closeable GitHub issue target can be resolved.
  func resolveCloseTarget(ctx context.Context, registry *session.PluginRegistry, syncLoop *session.SyncLoop, storage *session.Storage, item *session.BacklogItemData) (plugin externalIssueCloser, cfg session.PluginConfig, externalID, closeLabel string, ok bool) {
      if item.SourceID != "" && item.ExternalID != "" {
          source, err := storage.GetItemSourceByID(ctx, item.SourceID)
          if err != nil {
              return nil, cfg, "", "", false
          }
          if !source.ForwardSyncEnabled {
              return nil, cfg, "", "", false
          }
          p, found := registry.Get(source.PluginID)
          closer, isCloser := p.(externalIssueCloser)
          if !found || !isCloser {
              return nil, cfg, "", "", false
          }
          raw, err := syncLoop.DecryptConfigToken(source.Config)
          if err != nil {
              log.Warn("backlog_github_forward_sync: decrypt token failed", "source_id", source.ID, "err", err)
              return nil, cfg, "", "", false
          }
          return closer, session.PluginConfig{Raw: raw}, item.ExternalID, source.ForwardSyncCloseLabel, true
      }

      // Path B: MCP-imported item — resolve from ExternalURL.
      ref, err := githubpkg.ParseGitHubRefWithHosts(item.ExternalURL, nil)
      if err != nil || ref.Type != githubpkg.RefTypeIssue {
          return nil, cfg, "", "", false
      }
      raw, err := json.Marshal(map[string]string{"host": ref.Host, "owner": ref.Owner, "repo": ref.Repo})
      if err != nil {
          return nil, cfg, "", "", false
      }
      return &session.GitHubIssuesPlugin{}, session.PluginConfig{Raw: string(raw)}, fmt.Sprint(ref.IssueNumber), "", true
  }
  ```
- Add `encoding/json` and `githubpkg "github.com/tstapler/stapler-squad/github"` imports (verify these are not already in `backlog_github_forward_sync.go`'s import block before adding; they are already in `backlog_github_progress_sync.go`).
- Files: `server/services/backlog_github_forward_sync.go`

##### Task 1.1.2e: Add `ReadIssueState` to `*GitHubIssuesPlugin` and `issueStateReader` interface (S)

This task adds the skip-if-closed pre-GET plumbing. It has two parts:

**Part 1 — extend `session/backlog_plugin_github.go`:**
- Rename the private `readIssueUpdatedAt` to `readIssueState`, changing its signature to:
  ```go
  // readIssueState GETs a GitHub issue and returns its current state ("open"/"closed")
  // and updated_at. Used by PostProgressComment (updated_at only) and ReadIssueState
  // (state + updated_at for the skip-if-closed guard).
  func readIssueState(ctx context.Context, cfg githubPluginConfig, externalID string) (state string, updatedAt time.Time, err error) {
      // ... same GET request as before ...
      return issue.State, updatedAt, nil
  }
  ```
- Update `PostProgressComment`'s call site: replace `readIssueUpdatedAt(ctx, cfg, externalID)` with `_, updatedAt, err := readIssueState(ctx, cfg, externalID)` — identical behavior, just threading the extra return value.
- Add `ReadIssueState` as an exported method on `*GitHubIssuesPlugin`:
  ```go
  // ReadIssueState reads the current state and updated_at of a GitHub issue.
  // Used by the forward-sync close guard to skip idempotent closes
  // (issueStateReader interface in backlog_github_forward_sync.go).
  func (g *GitHubIssuesPlugin) ReadIssueState(ctx context.Context, config PluginConfig, externalID string) (state string, updatedAt time.Time, err error) {
      cfg, err := decodeGithubIssuesConfig(config)
      if err != nil {
          return "", time.Time{}, err
      }
      return readIssueState(ctx, cfg, externalID)
  }
  ```
- Files: `session/backlog_plugin_github.go`

**Part 2 — add `issueStateReader` interface in `server/services/backlog_github_forward_sync.go`:**
- After `externalIssueCloser`, add:
  ```go
  // issueStateReader is an optional extension of externalIssueCloser: plugins
  // that implement it allow handleForwardSyncClose to skip a redundant close
  // when the issue is already closed (FR3/AC-2). Separated from
  // externalIssueCloser so the type assertion degrades gracefully.
  type issueStateReader interface {
      ReadIssueState(ctx context.Context, config session.PluginConfig, externalID string) (state string, updatedAt time.Time, err error)
  }
  ```
- Files: `server/services/backlog_github_forward_sync.go`

##### Task 1.1.2c: Refactor `handleForwardSyncClose` to use `resolveCloseTarget`, `forwardSyncCloseCommentWithPR`, skip-if-closed guard, and Linear/JIRA fallback (S)

This task replaces the core of `handleForwardSyncClose`. Exact changes:

1. **Replace early-return + source/plugin lookup block** (lines 142–189) with:
   ```go
   plugin, config, externalID, closeLabel, ok := resolveCloseTarget(ctx, registry, syncLoop, storage, current)
   if !ok {
       // Fallback: state-updater path (Linear/JIRA). Re-fetch source; resolveCloseTarget does
       // not return it on !ok when the SourceID path failed a type assertion.
       if current.SourceID != "" && current.ExternalID != "" {
           source, srcErr := storage.GetItemSourceByID(ctx, current.SourceID)
           if srcErr == nil && source.ForwardSyncEnabled {
               if p, found := registry.Get(source.PluginID); found {
                   if updater, isUpdater := p.(externalIssueStateUpdater); isUpdater {
                       // ... existing Linear/JIRA state-updater logic unchanged ...
                   }
               }
           }
       }
       return
   }
   ```
   The re-fetch of source for the state-updater fallback is an O(1) DB read and matches the original code's structure for Linear/JIRA items.

2. **Add skip-if-closed pre-GET** (FR3/AC-2) immediately after `resolveCloseTarget` returns `ok`:
   ```go
   // Skip-if-closed guard (FR3/AC-2): read current issue state before attempting
   // the close. If the issue is already closed (GitHub's PR-merge keyword ran first),
   // advance the watermark and skip the write to avoid a duplicate comment and a
   // redundant PATCH. ReadIssueState failure is non-fatal: proceed with close.
   if reader, canRead := plugin.(issueStateReader); canRead {
       state, stateUpdatedAt, stateErr := reader.ReadIssueState(ctx, config, externalID)
       if stateErr != nil {
           log.Debug("backlog_github_forward_sync: read issue state failed, proceeding with close", "item", current.ID, "err", stateErr)
       } else if state == "closed" {
           log.Debug("backlog_github_forward_sync: issue already closed, skipping close", "item", current.ID, "external_url", current.ExternalURL)
           watermark := stateUpdatedAt
           if watermark.IsZero() {
               watermark = time.Now().UTC()
           }
           if _, wErr := storage.UpdateBacklogItem(context.Background(), current.ID, session.BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}, nil); wErr != nil {
               log.Warn("backlog_github_forward_sync: failed to persist watermark (already-closed skip)", "item", current.ID, "err", wErr)
           }
           return
       }
   }
   ```

3. **Replace comment** assembly:
   ```go
   comment := forwardSyncCloseCommentWithPR(current.ClaimingHostID, current.PrURL)
   ```

4. **Invoke close**: replace the `update` func closure with a direct call, using `closeLabel` from `resolveCloseTarget`'s 5-tuple return (Path A sets it to `source.ForwardSyncCloseLabel`; Path B sets it to `""`):
   ```go
   issueUpdatedAt, closeErr := plugin.CloseIssue(ctx, config, externalID, current.Labels, closeLabel)
   ```

5. **Watermark persistence** (after close): use `context.Background()` (not the timeout ctx) for the final `storage.UpdateBacklogItem` call, to prevent a cancelled context from dropping the watermark after a successful GitHub close.

- Files: `server/services/backlog_github_forward_sync.go`

---

#### Story 1.1.3: Agent prompt includes linked issue URL and closing instruction (AC-4)
**As an** AI agent working on a backlog item, **I want** my initial prompt to tell me the linked GitHub issue URL and instruct me to include `Fixes <url>` in my PR body, **so that** every agent-created PR automatically closes the issue.

**Acceptance Criteria**:
- When `BuildSessionInitialPrompt` is called for an item with `ExternalURL = "https://github.com/acme/widget/issues/42"`, the returned prompt contains:
  - **FACT LINE** inside the `--- BACKLOG ITEM DATA ---` block: `Linked GitHub Issue: https://github.com/acme/widget/issues/42`
  - **INSTRUCTION LINE** outside the `--- END BACKLOG ITEM DATA ---` fence: `When opening a pull request, include this exact line in the PR body:\nFixes https://github.com/acme/widget/issues/42`
  - *Given* `item.ExternalURL = "https://github.com/acme/widget/issues/42"`, *When* `BuildSessionInitialPrompt(item, nil)` is called, *Then* the output contains the fact line before `--- END BACKLOG ITEM DATA ---` and the instruction line after it.
- When `ExternalURL` contains `/pull/` (e.g. `https://github.com/acme/widget/pull/7`), the fact line reads `Linked GitHub PR: https://github.com/acme/widget/pull/7` and the instruction reads `Related: https://github.com/acme/widget/pull/7`.
- When `ExternalURL` is empty, neither line appears.
- Both truncation passes in `BuildTokenBudgetedPrompt` preserve the new lines.

**Files**: `session/backlog_context.go`, `session/backlog_context_test.go`

##### Task 1.1.3a: Insert linked issue fact line and PR instruction into `buildSessionInitialPrompt` (S)
**Placement is critical and must be exact:**
- The **FACT LINE** goes **INSIDE** the `--- BACKLOG ITEM DATA ---` block — after the Notes section and **before** `sb.WriteString("--- END BACKLOG ITEM DATA ---\n\n")`.
- The **INSTRUCTION LINE** goes **OUTSIDE** the `--- END BACKLOG ITEM DATA ---` boundary — after the closing fence and before `sb.WriteString(escalationNotice(...))`.

In `session/backlog_context.go`, inside `buildSessionInitialPrompt`, replace the existing `sb.WriteString("--- END BACKLOG ITEM DATA ---\n\n")` call with:
  ```go
  if item.ExternalURL != "" {
      kw := closingKeywordFor(item.ExternalURL)
      // Fact line goes inside the inert-data fence: it describes an item property.
      if strings.Contains(item.ExternalURL, "/issues/") {
          fmt.Fprintf(&sb, "\nLinked GitHub Issue: %s\n", item.ExternalURL)
      } else if strings.Contains(item.ExternalURL, "/pull/") {
          fmt.Fprintf(&sb, "\nLinked GitHub PR: %s\n", item.ExternalURL)
      }
      sb.WriteString("--- END BACKLOG ITEM DATA ---\n\n")
      // Instruction line lives outside the inert-data fence: it is a behavioral instruction to the agent, not an item data fact.
      if kw != "" {
          fmt.Fprintf(&sb, "When opening a pull request, include this exact line in the PR body:\n%s %s\n\n", kw, item.ExternalURL)
      }
  } else {
      sb.WriteString("--- END BACKLOG ITEM DATA ---\n\n")
  }
  ```

**Verification**: After the edit, `grep -n "END BACKLOG ITEM DATA"` must show exactly one occurrence in `buildSessionInitialPrompt`.
- Add table-driven tests in `session/backlog_context_test.go`:
  1. `ExternalURL` with `/issues/` → both fact line and `"Fixes"` instruction present, in correct order relative to `--- END BACKLOG ITEM DATA ---`.
  2. `ExternalURL` with `/pull/` → `"Linked GitHub PR:"` and `"Related:"` instruction present.
  3. `ExternalURL = ""` → neither `"Linked GitHub"` nor `"Fixes"` nor `"Related:"` present.
  4. `BuildTokenBudgetedPrompt` with oversized item and `/issues/` URL → instruction line survives both truncation passes.
- Files: `session/backlog_context.go`, `session/backlog_context_test.go`

---

#### Story 1.1.4: Tests for all new paths (AC-1, AC-2, AC-5, AC-6, AC-7)
**As a** maintainer, **I want** unit tests covering the new MCP-import close path, skip-if-closed guard, PR-URL comment, and `ReadIssueState`, **so that** future regressions are caught immediately by `make test`.

**Acceptance Criteria**:
- `make build && make test` passes for `server/services`, `session` packages.
- Tests cover: MCP-import path closes issue and posts comment; issue-already-closed skips CloseIssue AND PostIssueComment; PR URL is included in comment when non-empty; token-miss logs Warn and skips close; SourceID path still works (no regression); watermark is advanced after close (and after already-closed skip); Linear/JIRA state-updater path still works after refactoring.

**Files**: `server/services/backlog_github_forward_sync_test.go`, `session/backlog_plugin_github_test.go`

##### Task 1.1.4a: Add `resolveCloseTarget`, skip-if-closed, and `handleForwardSyncClose` tests (M)
- In `server/services/backlog_github_forward_sync_test.go`, add:
  1. `TestResolveCloseTarget_SourceIDPath` — SourceID-linked item: returns plugin, decrypted config, ExternalID, closeLabel.
  2. `TestResolveCloseTarget_MCPImportPath` — `SourceID = ""`, `ExternalURL = "https://github.com/acme/widget/issues/42"`: returns `*session.GitHubIssuesPlugin`, config with host/owner/repo, externalID `"42"`, closeLabel `""`.
  3. `TestResolveCloseTarget_PRURLSkipped` — `ExternalURL` with `/pull/`: returns `ok=false`.
  4. `TestResolveCloseTarget_EmptyExternalURL` — returns `ok=false`.
  5. `TestForwardSyncCloseCommentWithPR_WithPR` — ends with `"Implemented in: https://github.com/acme/widget/pull/99."`.
  6. `TestForwardSyncCloseCommentWithPR_NoPR` — equals `forwardSyncCloseComment`.
  7. `TestHandleForwardSyncClose_MCPImport_ClosesIssue` — integration-style: `CloseIssue` and `PostIssueComment` are called, watermark is advanced via `context.Background()`.
  8. `TestHandleForwardSyncClose_MCPImport_TokenMiss_Skips` — no token: `CloseIssue` NOT called.
  9. `TestHandleForwardSyncClose_SourceID_NoRegression` — existing SourceID behavior unchanged.
  10. **`TestHandleForwardSyncClose_IssueAlreadyClosed_SkipsCloseAndComment`** (AC-2) — `ReadIssueState` returns `("closed", timestamp, nil)`: `CloseIssue` is NOT called, `PostIssueComment` is NOT called, `UpdateBacklogItem` is called with `GitHubSyncedIssueUpdatedAt = timestamp`.
  11. **`TestHandleForwardSyncClose_ReadStateError_ProceedsWithClose`** — `ReadIssueState` returns an error: `CloseIssue` IS called (error is non-fatal, proceed with close).
  12. `TestHandleForwardSyncClose_LinearStateUpdater_NoRegression` — SourceID-linked item with Linear/JIRA `externalIssueStateUpdater` plugin: state-updater path still fires after the `resolveCloseTarget` refactoring (pre-mortem P2-3: zero coverage on Linear/JIRA fallback after Task 1.1.2c's refactoring).
- For tests 10–11, the `fakeForwardSyncCloserPlugin` fake in the test file must implement `issueStateReader` to allow controlling the state-read response:
  ```go
  type fakeForwardSyncCloserPlugin struct {
      // ... existing fields ...
      stateReaderState     string
      stateReaderUpdatedAt time.Time
      stateReaderErr       error
  }

  func (p *fakeForwardSyncCloserPlugin) ReadIssueState(_ context.Context, _ session.PluginConfig, _ string) (string, time.Time, error) {
      return p.stateReaderState, p.stateReaderUpdatedAt, p.stateReaderErr
  }
  ```
- Files: `server/services/backlog_github_forward_sync_test.go`

##### Task 1.1.4b: Add `ReadIssueState` unit test on `*GitHubIssuesPlugin` (XS)
- In `session/backlog_plugin_github_test.go` (or a new `session/backlog_plugin_github_state_test.go`), add:
  1. `TestReadIssueState_ReturnsStateAndUpdatedAt` — using `httptest.NewServer`, verify `ReadIssueState` returns `state = "open"` and correct `updated_at` from a mock GitHub API response.
  2. `TestReadIssueState_NonOKStatus_ReturnsError` — 404 response → error returned.
- Files: `session/backlog_plugin_github_test.go`

##### Task 1.1.4c: Run CI gate (XS)
- Run `make build && make test` for `server/services`, `session` packages; confirm exit 0.
- Run `make lint`; confirm no new lint errors.
- Run `make nil-safety` — changed files touch nil-sensitive paths (`resolveCloseTarget` returns a pointer interface value; `ReadIssueState` optional type assertion).
- Files: none (verification step)

---

## Effort Estimate

**Cost class breakdown** (token type weighting per ESTIMATION.md):

| Wave | Work | Cost Class |
|------|------|-----------|
| Wave 1 (parallel) | Tasks 1.1.1a + 1.1.1c + 1.1.2a + 1.1.2a-timeout + 1.1.2e (Part 2 — interface only) | XS + S + XS + XS + XS — five helper/constant/interface additions; 1.1.1c is S because it also threads prRemoteURL to call sites |
| Wave 2 (parallel) | Tasks 1.1.1b + 1.1.1d + 1.1.2b + 1.1.2e (Part 1 — plugin method) | XS + XS + S + S — four file edits, low risk; 1.1.1b and 1.1.1d depend on 1.1.1c (Wave 1) |
| Wave 3 (parallel) | Tasks 1.1.2c + 1.1.3a | S + S — surgical edits to two existing functions; each requires careful preservation of surrounding logic |
| Wave 4 | Tasks 1.1.4a + 1.1.4b | M + XS — writing ~12 test cases; highest output-token cost of the feature |
| Wave 5 | Task 1.1.4c | XS — command run, no output tokens |

**Wall-clock blockers**: None. All changes are in existing packages with no external dependencies, no schema migrations, no CI infrastructure changes.

**Total**: ~5 agent waves, dominated by Wave 4 test writing. No owner decisions required before implementation begins.
