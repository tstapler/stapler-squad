# Validation Plan: backlog-session-item-linking

**Date**: 2026-08-16

## Happy Path Scenario

Given a session with no `ItemSession` row for backlog item `I` (status `in_progress`) — because
it picked up in-flight work after a prior session already advanced the item — and stale
`.claude/commands/backlog/*` files on disk referencing a different, previously-linked item, when
the session calls `link_session_to_item(item_id=I)`, then an `ItemSession` row (role=work) is
created linking the session to `I`, this session's slash-command files are regenerated via
`WriteSlashCommands` to reference `I` (verified by reading `done-0.md`'s content), and a
subsequent `report_progress(item_id=I, ...)` call from the same session succeeds instead of
returning `PERMISSION_DENIED`.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: `link_session_to_item` MCP tool exists, registered, calls `AttachSessionToItem` | `server/mcp/tools_backlog_test.go` | `TestRegisterBacklogTools_should_RegisterLinkSessionToItemTool_When_ToolsRegistered` | Unit | Happy path — tool present in `registerBacklogTools`' output with expected name/schema |
| AC1: (same) | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_ReturnUnavailable_When_AttacherNil` | Unit | Error path — `backlogHandlers{attacher: nil}` (stdio fallback) returns `ErrUnavailable`, not a panic |
| AC1: (same) | `server/mcp/server_integration_test.go` | `TestNewCore_should_WireNonNilAttacher_When_BacklogServiceProvided` | Integration | `NewCore` given a non-nil `*services.BacklogService` produces a `backlogHandlers.attacher` that is non-nil and a `link_session_to_item` call over that wiring succeeds against a real `*session.Storage`-backed item |
| AC2: Linking to a valid unlinked item succeeds and unblocks subsequent calls | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_CreateItemSession_When_ValidUnlinkedItem` | Unit | Happy path — `already_linked:false`, `ItemSession` row created (fake `SessionAttacher`) |
| AC2: (same) | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_ReturnConflict_When_ItemClaimedByOtherLiveSession` | Unit | Error path — a *different* live work-role session already holds the item; `ErrConflict`, no row created for caller |
| AC2: (same) | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_UnblockReportProgress_When_CalledAgainstRealStorage` | Integration | Real `*session.Storage` test helper: link then `reportProgress` against the same item/session no longer returns `PERMISSION_DENIED` |
| AC3: Rejects a nonexistent item id and honors `AttachSessionToItem`'s status constraints | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_ReturnItemNotFound_When_ItemIdDoesNotExist` | Unit | Happy path (of the rejection) — fake attacher returns `connect.CodeNotFound` → `ErrItemNotFound` |
| AC3: (same) | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_ReturnFailedPrecondition_When_ItemInTerminalStatus` | Unit | Error path — item `status="done"`; fake attacher returns `connect.CodeFailedPrecondition` → `ErrFailedPrecondition` naming actual status |
| AC3: (same) | `server/services/backlog_service_sync_test.go` | `TestAttachSessionToItem_should_RejectViaConnectError_When_ItemStatusGuardFails` | Integration | Real `BacklogService.AttachSessionToItem` against real storage with a terminal-status item, confirming the `connect.CodeFailedPrecondition` the MCP layer depends on actually propagates from the service layer |
| AC4: The 5 `PERMISSION_DENIED: not linked` sites are actionable | `server/mcp/tools_backlog_test.go` | `TestActionablePermissionDenied_should_NameBothItems_When_LinkedToDifferentItem` | Unit | Happy path — caller linked to `I1`, calls against `I2`; message/remediation name both `I1` and `I2` and `link_session_to_item` |
| AC4: (same) | `server/mcp/tools_backlog_test.go` | `TestActionablePermissionDenied_should_NameNoLinkMessage_When_SessionHasNoLink` | Unit | Error path — caller has zero `ItemSession` rows; simpler "not linked to any item" message, still names `link_session_to_item` |
| AC4: (same) | `server/mcp/tools_backlog_test.go` | `TestReportProgress_should_ReturnActionableHint_When_SessionNotLinked` | Integration | Real storage, drives the fix through one of the 5 real call sites (`reportProgress`) end to end plus a `grep -c` regression check that the old bare literal string appears 0 times in `tools_backlog.go` |
| AC5: Successful (re)link regenerates this session's slash-command files with the correct item id | `session/backlog_commands_test.go` | `TestPruneStaleSlashCommandFiles_should_RemoveExtraFiles_When_NewItemHasFewerCriteria` | Unit | Happy path — 8 AC's worth of `done-N.md`/`fail-N.md` on disk, regenerate for a 3-AC item, files 3-7 removed |
| AC5: (same) | `session/backlog_commands_test.go` | `TestPruneStaleSlashCommandFiles_should_PreserveAllFiles_When_NewItemHasMoreOrEqualCriteria` | Unit | Error/edge path — regenerating with equal-or-more criteria never deletes a file about to be rewritten |
| AC5: (same) | `server/services/backlog_service_sync_test.go` | `TestAttachSessionToItem_should_RegenerateSlashCommandsWithNewItemID_When_RelinkedToDifferentItem` | Integration | Real `Instance` + temp worktree path; calls `AttachSessionToItem` for item 1 then item 2, reads `done-0.md` from disk via `os.ReadFile`, asserts it contains item 2's id, not item 1's — the literal AC5 "read file contents post-call" requirement |
| AC6: Read-only introspection of current session↔item linkage without SQLite access | `server/mcp/tools_backlog_test.go` | `TestGetLinkedItem_should_ReturnMostRecentLink_When_NoItemIdProvided` | Unit | Happy path — two `ItemSession` rows for one session, no `item_id` arg, returns the most recent by `created_at` |
| AC6: (same) | `server/mcp/tools_backlog_test.go` | `TestGetLinkedItem_should_ReturnNotLinked_When_SessionHasNoItemSessionRows` | Unit | Error/negative path — zero rows → `{"linked":false}` as an HTTP-level success, not a `PERMISSION_DENIED` error |
| AC6: (same) | `server/mcp/tools_backlog_test.go` | `TestGetLinkedItem_should_ReturnLinkedFalseForSpecificItem_When_SessionLinkedToDifferentItem` | Integration | Real storage: session linked to `I5` but not `I6`; `get_linked_item(item_id=I6)` returns `{"linked":false,"item_id":"I6"}` |
| AC7: Test coverage for idempotency and the slash-command regeneration side effect | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_ShortCircuitToNoOp_When_AlreadyLinkedToSameItem` | Unit | Happy path — second call with the same `(session_uuid, item_id)` returns `already_linked:true`, no duplicate row |
| AC7: (same) | `server/mcp/tools_backlog_test.go` | `TestLinkSessionToItem_should_NotCallAttacher_When_AlreadyLinkedToSameItem` | Unit | Error/negative path — fake `SessionAttacher`'s call count is 0 on the idempotent short-circuit (attach must not be re-triggered) |
| AC7: (same) | `session/backlog_commands_test.go` | `TestWriteSlashCommands_should_MatchExactFileSet_When_CalledTwiceWithDifferentCriteriaCounts` | Integration | `WriteSlashCommands` called once for an 8-AC item then again for a 3-AC item at the same `worktreePath`; resulting `os.ReadDir` listing matches the 3-AC item's file set exactly and `done-0.md` content carries the second item's id |
| AC8: Normal-mode initial prompt includes `item_id` (Epic 1.5, added post-triad-review) | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_should_IncludeItemID_When_Called` | Unit | Happy path — output contains `item_id: <test item's ID>` |
| AC8: (same) | `session/backlog_context_test.go` | `TestBuildSessionInitialPrompt_should_IncludeItemID_When_ItemFieldsEmpty` | Unit | Error/edge path — item with empty Description/Notes/AcceptanceCriteria still has `item_id` present in the output (the id line doesn't depend on other fields being populated) |

## Test Stack
- **Unit**: Go stdlib testing + testify (repo convention)
- **Integration**: Go stdlib testing against a real `*session.Storage` test helper (repo convention, see existing tools_backlog_test.go)
- **E2E / UX**: N/A — backend-only feature

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public service methods: happy path + error paths covered
- All external integrations: unit mocked + at least one integration test
