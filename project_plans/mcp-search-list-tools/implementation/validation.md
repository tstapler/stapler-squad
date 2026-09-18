# Validation Plan: mcp-search-list-tools

**Date**: 2026-08-13

## Happy Path Scenario
Given a running stapler-squad MCP server with a live `SessionService`/`BacklogService` and the `backlog` feature flag enabled, when an LLM client calls `list_backlog_items` with `status: ["ready", "in_progress"]`, then the response's `Items` contains only matching, non-`done` backlog items capped at the default limit, with `Success: true`.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: List/filter backlog items via MCP without a raw RPC call | `server/mcp/tools_backlog_test.go` | `TestListBacklogItems_ReturnsFilteredItems` (Task 1.1.2a) | Unit | Seed items across 3+ statuses, filter by `status`, assert only matches returned |
| AC1 (default-limit sub-clause) | `server/mcp/tools_backlog_test.go` | `TestListBacklogItems_AppliesDefaultLimit` (Task 1.1.2d) | Unit | >10 matching items, no `limit` arg → exactly 10 returned, `HasMore: true` |
| AC2: Query notification history via MCP with `type_filter`/`session_id`/`unread_only`/pagination | `server/mcp/tools_notifications_test.go` | `TestGetNotificationHistory_ReturnsFilteredRecords` (Task 1.2.2a) | Unit | Filter by `type_filter`, assert only matching records returned |
| AC2 (combined-filter sub-clause) | `server/mcp/tools_notifications_test.go` | `TestGetNotificationHistory_CombinesUnreadOnlyAndTypeFilter` (Task 1.2.2c) | Unit | 5 unread `TASK_COMPLETE` + 3 read `ERROR`; `type_filter: "TASK_COMPLETE"` + `unread_only: true` → exactly the 5, `UnreadCount: 5` |
| AC2 (pagination default sub-clause) | `server/mcp/tools_notifications_test.go` | `TestGetNotificationHistory_AppliesContextSafeDefaultLimit` (Task 1.2.2d) | Unit | 100 matching records, no `limit` arg → ≤10 returned, `HasMore: true` |
| AC3: Full-text Claude/session-history search via MCP with LLM-context-safe default limit | `server/mcp/tools_history_test.go` | `TestSearchClaudeHistory_ReturnsResults` (Task 1.3.2a) | Unit | `query` provided → matching results returned |
| AC3 (default-limit sub-clause) | `server/mcp/tools_history_test.go` | `TestSearchClaudeHistory_AppliesDefaultLimitOf10` (Task 1.3.2c) | Unit | 50 matching sessions, no `limit` arg → ≤10 returned (not the RPC's native 20), `TotalCount: 50`, `HasMore: true` |
| AC3 (context-safety sub-clause: snippet truncation) | `server/mcp/tools_history_test.go` | `TestSearchClaudeHistory_TruncatesSnippets` (Task 1.3.2d) | Unit | Result with 8 snippet hits → ≤3 snippets included, each truncated via `SanitizeForAgentContext` |
| AC4: `tools_backlog.go:204`'s dangling `list_backlog_items` reference is resolved (tool exists, discoverable via `tools/list`) | `server/mcp/server_test.go` | `TestToolRegistrationCount` (Task 1.4.1a, existing/unmodified) | Unit | Confirms the hardcoded 5-file `AddTool(` scan still yields the expected count — **see Coverage Gap Note below**, this test does not itself scan `tools_backlog.go` |
| AC4 (actual "appears in `tools/list`" coverage) | `server/mcp/server_integration_test.go` | `TestMCPHandshakeSubprocess` (existing, build-tag `integration`, self-computing oracle via `expectedToolCount`) | Integration | Full MCP `initialize` + `tools/list` handshake over stdio against the built binary; oracle count is computed from `NewCore`'s own registration calls, so it automatically includes `list_backlog_items` once wired — no hand-bump required |
| AC5: Argument validation + `errResult` + matching `*_test.go` per new tool | `server/mcp/tools_backlog_test.go` | `TestListBacklogItems_ValidatesStatusValues` (Task 1.1.2b) | Unit | Typo'd `status: ["readdy"]` → `Success: false`, `ErrInvalidArgument` |
| AC5 (priority range) | `server/mcp/tools_backlog_test.go` | `TestListBacklogItems_ValidatesPriorityRange` (Task 1.1.2c) | Unit | `priority: [0]` and `priority: [99]` → `ErrInvalidArgument` for both |
| AC5 (notification type enum) | `server/mcp/tools_notifications_test.go` | `TestGetNotificationHistory_ValidatesTypeFilter` (Task 1.2.2b) | Unit | `type_filter: "NOT_A_REAL_TYPE"` → `Success: false`, `ErrInvalidArgument` |
| AC5 (required query) | `server/mcp/tools_history_test.go` | `TestSearchClaudeHistory_RequiresQuery` (Task 1.3.2b) | Unit | No `query` argument → `Success: false`, `ErrInvalidArgument`, message `"query is required"` |
| AC5 (feature-flag gate, scoped to `list_backlog_items` only) | `server/mcp/feature_flag_test.go` | `TestBacklogHandlers_FeatureDisabled` (existing table test, new row added — Task 1.1.2e) | Unit | `backlogEnabled()` returns `false` → `featureDisabledResult` output, `ErrFeatureDisabled`, `h.backlogSvc` not called |
| AC5 (nil-service fallback, ADR-002) | `server/mcp/tools_backlog_test.go` | `TestListBacklogItems_ReturnsUnavailable_When_BacklogSvcNil` (Task 1.1.2e) | Unit | `backlogHandlers{backlogSvc: nil}` → `ErrInternalError` |
| AC6: New tools registered the same way as existing tools (`tools_*.go` + `server.go` wiring) | `server/mcp/server_test.go` | `TestToolRegistrationCount` (Task 1.4.1a) | Unit | Confirms no unmodified-count regression in the 5 originally-scanned files (see Coverage Gap Note) |
| AC6 (build/lint/registry no-op) | n/a — command execution, not a Go test | `make build && make test && make lint` (Task 1.4.1b/c) + registry marker grep (Task 1.4.1d) | Integration/CI gate | All three green; grep of the 3 new/modified files confirms zero `// +api:`/`// +feature:` markers, so no `docs/registry/` entry is required |

## Coverage Gap Note (found during validation, not a plan.md defect requiring a re-plan)

`TestToolRegistrationCount` (Task 1.4.1a's named test) scans exactly `server.go`, `tools_discovery.go`, `tools_lifecycle.go`, `tools_terminal.go`, `tools_vcs.go` — **not** `tools_backlog.go`, `tools_notifications.go`, or `tools_history.go`, which is where all three new tools are registered. Passing this test after implementation confirms only that the *unrelated* 5-file count wasn't accidentally disturbed; it provides **zero** signal that `list_backlog_items`/`get_notification_history`/`search_claude_history` actually registered successfully. Plan.md's own Task 1.4.1a description already flags this ("shouldn't happen per this plan's file placement"), so this isn't a new finding — but it means AC4's literal claim ("an MCP client sends `tools/list`, then `list_backlog_items` appears in the returned tool set") is **not** verified by the test plan.md names for it.

The actual verification already exists in the repo: `TestMCPHandshakeSubprocess` (`server/mcp/server_integration_test.go`, build tag `integration`) drives a real `initialize` + `tools/list` handshake against the built binary and compares the returned tool count against `expectedToolCount`, an oracle computed by calling `NewCore`'s real registration path directly — so it will automatically catch a missing/failed registration for any of the 3 new tools with no manual count bump, the same way it already does for every other tool in the package. `make test` (Task 1.4.1b) does not run build-tagged `integration` tests by default; confirm whether CI's `make ci` includes `-tags=integration` before treating this test as part of the gate, and if not, run `go test -tags=integration ./server/mcp/...` explicitly as part of Task 1.4.1b/1.4.1c's verification.

## UX Acceptance Tests
N/A — pure infrastructure, no user-facing surface.

## Test Naming Convention Notes

Repo convention: `Test<Thing>_<ExpectedBehavior>_When_<Condition>`. Most of plan.md's own test names use `Test<Thing>_<ExpectedBehavior>` without an explicit `_When_<Condition>` clause. Existing precedent in the same package (`TestListBacklogItems_ReturnsUnavailable_When_BacklogSvcNil`, Task 1.1.2e) already follows the full convention — the other 12 new test names do not. Not a blocker (plan.md's names are used verbatim in the mapping table above per validation-authoring convention), but flagged here so the implementer can rename during Task execution if desired:

| Plan.md name | Suggested corrected name |
|---|---|
| `TestListBacklogItems_ReturnsFilteredItems` | `TestListBacklogItems_ReturnsFilteredItems_When_StatusFilterApplied` |
| `TestListBacklogItems_ValidatesStatusValues` | `TestListBacklogItems_ReturnsInvalidArgument_When_StatusValueUnknown` |
| `TestListBacklogItems_ValidatesPriorityRange` | `TestListBacklogItems_ReturnsInvalidArgument_When_PriorityOutOfRange` |
| `TestListBacklogItems_AppliesDefaultLimit` | `TestListBacklogItems_ReturnsDefaultLimitOf10_When_NoLimitArgGiven` |
| `TestGetNotificationHistory_ReturnsFilteredRecords` | `TestGetNotificationHistory_ReturnsFilteredRecords_When_TypeFilterApplied` |
| `TestGetNotificationHistory_ValidatesTypeFilter` | `TestGetNotificationHistory_ReturnsInvalidArgument_When_TypeFilterUnknown` |
| `TestGetNotificationHistory_CombinesUnreadOnlyAndTypeFilter` | `TestGetNotificationHistory_ReturnsAndedResults_When_UnreadOnlyAndTypeFilterCombined` |
| `TestGetNotificationHistory_AppliesContextSafeDefaultLimit` | `TestGetNotificationHistory_ReturnsDefaultLimitOf10_When_NoLimitArgGiven` |
| `TestSearchClaudeHistory_ReturnsResults` | `TestSearchClaudeHistory_ReturnsMatchingResults_When_QueryProvided` |
| `TestSearchClaudeHistory_RequiresQuery` | `TestSearchClaudeHistory_ReturnsInvalidArgument_When_QueryMissing` |
| `TestSearchClaudeHistory_AppliesDefaultLimitOf10` | `TestSearchClaudeHistory_ReturnsDefaultLimitOf10_When_NoLimitArgGiven` |
| `TestSearchClaudeHistory_TruncatesSnippets` | `TestSearchClaudeHistory_TruncatesSnippets_When_ResultHasManyHits` |

## Test Stack
- **Unit**: Go stdlib `testing`, table-driven where applicable (e.g. `TestBacklogHandlers_FeatureDisabled`'s existing table gains a `list_backlog_items` row)
- **Integration**: Go stdlib `testing`, build tag `integration` — `TestMCPHandshakeSubprocess` builds the real binary and drives a real MCP stdio handshake; no new integration test is added by this feature, the existing one's self-computing oracle covers it for free
- **E2E / UX**: N/A

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./server/mcp/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
