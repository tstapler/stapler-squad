# Backlog dependencies

An item can wait on other items. A dependent is never auto-started until each blocker is `done`, `archived` or deleted; `DequeueNextQueuedItems` skips it and records a `blocked_by_dependency` stuck reason whose context lists the blocker ids.

## Calling the RPCs directly

Connect-RPC handlers are mounted under `/api`. The bare `/session.v1...` path returns the SPA HTML, not a Connect response.

```bash
curl -s -X POST http://127.0.0.1:8543/api/session.v1.BacklogService/AddBacklogItemDependency \
  -H 'Content-Type: application/json' \
  -d '{"blockerItemId":"<uuid>","blockedItemId":"<uuid>"}'
```

| RPC | Behavior |
|---|---|
| `AddBacklogItemDependency` | Rejects self-edges and cycles (`invalid_argument`); re-adding an edge is a no-op. |
| `RemoveBacklogItemDependency` | Idempotent: a missing edge succeeds. Both items must exist (`not_found`). |
| `ListBacklogItemDependencies` | Returns `blocked_by` and `blocking` with title and status. |
| `GetBacklogItem` | Same two lists on the item (`blocked_by`, `blocking`), all edges. |
| `ListBacklogItems` | `blocked_by` holds only unresolved blockers (one batched query); `blocking` is empty. |

## MCP tools

`add_backlog_item_dependency`, `remove_backlog_item_dependency`, `trigger_triage`, `transition_backlog_item_status` (requires `expected_status`; no override), and an optional `depends_on` list on `create_backlog_item` (validated before the item is created). `get_backlog_item` prints a `## Dependencies` section.
