# BUG-061: `create_backlog_item`/`import_github_issue` MCP Tools Never Trigger Auto-Triage, and the Catch-Up Sweep Cannot Reach Items That Were Never Attempted [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-08-05, investigating six backlog items self-filed by agent sessions (per `.claude/rules/fix-flaky-tests-dont-defer.md`) that sat in `idea` status for 24-27+ hours with no plan.
**Impact**: Medium. Any backlog item created via the `create_backlog_item` or `import_github_issue` MCP tools — i.e. every item an agent session self-files as a side-finding, as opposed to one filed through the web UI — never got triaged automatically. Silent: no error, no notification, the item simply sat in `idea` forever unless a human noticed and manually re-triggered triage.

## Root Cause

Two independent gaps combined to produce the symptom.

**1. The MCP creation paths bypassed the auto-triage gate entirely.** `BacklogService.CreateBacklogItem` and `BacklogService.ImportGitHubIssue` (the RPC handlers backing the web UI's "New Idea" form and "Import from GitHub" action, `server/services/backlog_service_lifecycle.go` and `backlog_service_sync.go`) both called `storage.CreateBacklogItem` and then separately called `s.TriggerTriage(...)`, gated on `!skipTriage && created.RepoPath != "" && s.headlessPool != nil`. The MCP tools of the same name (`server/mcp/tools_backlog.go`'s `createBacklogItem`/`importGitHubIssue`) called `h.storage.CreateBacklogItem` directly — the bare storage-layer function — and never called `TriggerTriage` at all. `backlogHandlers` (the MCP tool handler struct) held only a `*session.Storage` reference, not a `*services.BacklogService`, so it had no way to reach `TriggerTriage` even if someone had remembered to call it.

**2. The catch-up sweep has a chicken-and-egg gap: it can only detect items that already failed once.** `reconcileOrphanedTriageItems` (`session/backlog_lifecycle.go:2536`) is the periodic reconciler that would otherwise have caught this — but it explicitly skips any item with no prior triage-role `ItemSession`:

```go
latestTriage := latestTriageSession(sessions)
if latestTriage == nil {
    continue // no triage session has ever run for this item
}
```

This is correct for the shape it was designed for (a triage session started, then crashed/errored/never-completed — see the function's own doc comment, shapes 1-3), but it means the sweep can never *originate* a first triage attempt for an item that has zero sessions. Confirmed live: of the six affected items, the four with zero `item_sessions` rows (`2668d886`, `6be30c82`, `2d7fac56`, `f42d895d`) never got a single triage attempt in their 24-27+ hours in `idea`. The other two (`8f0a9916`, `cfb91f0e`) each got 1-3 triage attempts starting only within the last hour of this investigation — consistent with those two having their *first* triage attempt kicked off manually while investigating this exact bug, after which `reconcileOrphanedTriageItems` + `reconcileOrphanedTriageRemediation`'s backoff-gated retry legitimately picked them up for the subsequent attempts (their timestamps show a 16-33s gap for the first two attempts, then a ~31min gap before a third — the ~31min gap matches the documented first backoff tier, the sub-minute gaps do not match any automated cadence in this codebase). This detail is inferred, not verified against an audit log — the repo's `backlog_status_events` table has no rows for either item, so there is no durable record of who/what triggered the first attempt.

## Fix

Added `BacklogService.MaybeTriggerTriage(ctx, itemID, skipTriage, repoPath) bool` (`server/services/backlog_service_triage.go`) as the single "should this newly created item get auto-triaged" decision, and pointed every creation entry point at it:

- `BacklogService.CreateBacklogItem` and `BacklogService.ImportGitHubIssue` (RPC) now call the shared helper instead of duplicating the inline gate — no behavior change for these two, just deduplication.
- `server/mcp/tools_backlog.go`'s `backlogHandlers` struct gained a `backlogSvc *services.BacklogService` field, threaded through `NewCore`/`NewHTTPHandler`/`RunServer` (`server/mcp/server.go`) from `deps.BacklogService` (`server/server.go`), mirroring the existing `autoReopener` param's threading. The stdio fallback path (`main.go`, `buildMCPDeps`) has no `*services.BacklogService` available (Phase-1-only deps) and passes `nil` — auto-triage is silently skipped on that path only, same pre-existing limitation `autoReopener` already has for `submit_review_verdict`.
- `createBacklogItem`/`importGitHubIssue` MCP tool handlers now call `h.backlogSvc.MaybeTriggerTriage(...)` after creating the item (nil-guarded — a nil `backlogSvc` preserves the old create-only behavior rather than panicking).
- Added an optional `skip_triage` boolean parameter to both MCP tool schemas (default `false`, matching the RPC handlers' default of auto-triaging), so a caller filing several related items to triage together later has an explicit opt-out.
- The returned tool result text now says whether triage was started or not, so the calling agent session gets an explicit signal instead of silence.

The chicken-and-egg gap in `reconcileOrphanedTriageItems` (root cause #2) is **not** touched by this fix — root cause #1 (the actual bypass) being closed means the gap in #2 is no longer reachable via these two creation paths. It remains a real, narrower gap for any *other* future code path that might create a backlog item without triggering triage; see "Recurring shape" below.

## Regression Tests

`server/mcp/tools_backlog_test.go`:
- `TestCreateBacklogItem_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet` — creates an item via the MCP tool with a wired `*services.BacklogService` and a `repo_path`, asserts the result text confirms triage started and a triage-role `ItemSession` exists.
- `TestCreateBacklogItem_should_NotTriggerTriage_When_BacklogSvcNil` — same call with `backlogSvc` left nil (the stdio-fallback shape), asserts item creation still succeeds with no triage session (no panic, no regression from the pre-fix create-only behavior).
- `TestImportGitHubIssue_should_TriggerTriage_When_BacklogSvcWiredAndRepoPathSet` — same coverage for the import path.

`go test ./server/mcp ./server/services ./session` all pass; `go build ./...` and `make lint` are clean.

## Phase D — Classification (per `quality:reflect-and-fix`)

**Classification**: Integration Gap. Two creation entry points (RPC and MCP tool) for "the same logical operation" (create a backlog item) implemented the same post-create invariant ("new items with a repo_path get auto-triaged") independently, and one of the two implementations was never written.

**Earliest enforcement point**: A compile-time or lint-time check can't express "every code path that calls `storage.CreateBacklogItem` must also decide about triage" in Go without a much heavier abstraction (e.g. making `storage.CreateBacklogItem` itself triage-aware, which would leak triage/headless-pool concerns into the storage layer — a layering violation, and the reason `TriggerTriage` lives on `BacklogService` instead). The regression test above is the earliest achievable level given that constraint. The systemic fix here is behavioral, not mechanical: collapsing four independent copies of the same gate (RPC CreateBacklogItem, RPC ImportGitHubIssue, MCP create_backlog_item, MCP import_github_issue) down to one shared method (`MaybeTriggerTriage`) means any *future* fifth creation path that reuses it inherits the invariant for free, and any future change to the gate's conditions only needs to happen once.

**Recurring shape**: This is the same shape `.claude/rules/session-creation-registry.md` and `.claude/rules/feature-testing-registry.md` already exist to guard against for session-creation modes and omnibar features respectively: multiple entry points for one logical operation drifting apart because there is no single place that owns "what must happen after X." Backlog-item creation is a third instance of this shape in this codebase (RPC vs. MCP tool, rather than those two rules' web-UI-vs-backend or omnibar-specific splits). Given there are currently only two creation entry points (not the 6-7+ touchpoints the existing registries manage), a full "N touchpoints" registry doc for backlog-item creation would be over-engineering for what this fix already collapsed into one shared method — but if a third backlog-item creation path is ever added (e.g. a bulk-import tool, a webhook-driven creator), this is worth revisiting as a candidate for the same registry treatment. Flagging for the user's judgment call rather than building it now.

## Related

- `session/backlog_lifecycle.go:2536` (`reconcileOrphanedTriageItems`) — the catch-up sweep whose chicken-and-egg gap (root cause #2) is described but not modified here.
- `.claude/rules/session-creation-registry.md`, `.claude/rules/feature-testing-registry.md` — existing registries for the same "multiple entry points, one invariant" shape in other subsystems.
