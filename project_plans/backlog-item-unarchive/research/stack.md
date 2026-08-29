# Research: Technology Stack — Backlog Item Unarchive

## Summary

No new dependencies are needed. Every piece required — proto codegen toolchain,
ent-generated field clearer, ConnectRPC handler pattern, React action-dispatch
pattern — already exists in the repo and has a directly analogous precedent
(`UnarchiveSession`) to copy. This is a same-shape addition to existing
generated/hand-written code, not a new integration.

## Confirmed versions (go.mod / package.json)

| Component | Version | Source |
|---|---|---|
| Go | 1.26.3 | `go.mod:3` |
| `connectrpc.com/connect` | v1.19.0 | `go.mod:6` |
| `connectrpc.com/otelconnect` | v0.8.0 | `go.mod:7` |
| `entgo.io/ent` | v0.14.5 | `go.mod:8` |
| `go-git/go-git/v5` | v5.14.0 | `go.mod:17` |
| `buf` CLI | 1.66.1 | `buf --version` (installed at `/home/linuxbrew/.linuxbrew/bin/buf`) |
| `react` | ^19.0.0 | `web-app/package.json:83` |
| `typescript` | ^5.9.3 | `web-app/package.json:155` |
| `@connectrpc/connect` / `connect-web` | ^2.1.1 | `web-app/package.json:53-54` |

No version bumps or new packages are implicated by this feature.

## Proto codegen (`make proto-gen`)

- Driven by `buf generate proto` (Makefile target `proto-gen`, line ~`Makefile:`
  the rule under `PROTO_STAMP`/`PROTO_OUT_DIRS`), gated by a stamp file
  (`.proto-gen.stamp`) that's invalidated when any `proto/**/*.proto` file, or
  `protoc-gen-es`, is newer than the stamp.
- Config: `buf.yaml`, `buf.gen.yaml` (Go + TS), `buf.gen.go-only.yaml` at repo root.
- Output dirs: `gen/proto/go/` (Go bindings) and `web-app/src/gen/` (TS bindings,
  e.g. `web-app/src/gen/session/v1/session_pb.ts`).
- Convention for a new RPC: add the message pair (`XRequest`/`XResponse`) and the
  `rpc X(...) returns (...) {}` line to the relevant `.proto` file, then run
  `make proto-gen` (or `buf generate proto` directly) to regenerate both sides.
- Exact precedent for this feature: `UnarchiveSession` in
  `proto/session/v1/session.proto:401` (rpc decl) and `:2553-2557` (empty-ish
  request/response messages — `UnarchiveSessionRequest { string session_id = 1; }`,
  `UnarchiveSessionResponse {}`). The backlog equivalent would add
  `UnarchiveBacklogItem`/`UnarchiveBacklogItemRequest{string item_id = 1;}`/
  `UnarchiveBacklogItemResponse{}` to `proto/session/v1/backlog.proto`, alongside
  the existing `ArchiveBacklogItem` (`backlog.proto:374-378`, `:740`) and
  `TransitionBacklogItemStatus` (`backlog.proto:388-396`, `:746`) definitions.

## ent ORM (schema + repository)

- Schema generation command is pinned in `session/ent/generate.go`:
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`
  (matches the repo-wide rule in `.claude/rules/ent-schema-generation.md`).
- **No schema change is needed for this feature.** `session/ent/schema/backlog_item.go:79-81`
  already declares:
  ```go
  field.Time("archived_at").
      Optional().
      Nillable(),
  ```
  Because it's `Optional().Nillable()`, ent already generated a clearer method —
  confirmed present in generated code:
  `session/ent/backlogitem_update.go:414` (`BacklogItemUpdate.ClearArchivedAt()`)
  and `:1727` (`BacklogItemUpdateOne.ClearArchivedAt()`). So no `ent generate` run
  is required at all; the repository method just needs to call the existing
  generated clearer.
- Repository method precedent: `EntRepository.ArchiveBacklogItem`
  (`session/ent_repository_backlog.go:741-778`) — fetches current status via
  `r.client.BacklogItem.Get`, then `r.client.BacklogItem.UpdateOneID(parsedID).SetArchivedAt(now).SetStatus(...).SetUserModifiedStatusAt(now).Save(ctx)`,
  then `recordStatusEvent(...)` for the audit trail, then best-effort
  `attachItemSessionsForPublish` + `publishItemChanged`. A new `UnarchiveBacklogItem`
  repository method (or a fix inside `TransitionBacklogItemStatus`, per the
  requirements doc's AC1 either-path) should mirror this shape:
  `SetStatus(string(BacklogStatusIdea)).ClearArchivedAt().SetUserModifiedStatusAt(now)`,
  plus the same `recordStatusEvent` call for AC3 (status-event audit history).
- `TransitionBacklogItemStatus` (`session/ent_repository_backlog.go:869+`) already
  does its update via a SQL-level compare-and-swap (`Update().Where(...)` bulk
  builder, not `UpdateOneID`) to avoid a TOCTOU race — see the large comment
  above it citing `BUG-026-backlog-transition-status-toctou-reopen.md`. If the
  plan phase chooses "fix `TransitionBacklogItemStatus` to always clear
  `archived_at`" over "new dedicated RPC," the clear must be added to that
  bulk-update path, not a separate `UpdateOneID` call, to preserve the existing
  race protection.
- State machine: `session/domain/backlog.go`'s `CanTransitionBacklog` already
  permits `archived -> idea` (exercised by
  `TestCanTransition_ArchivedToIdeaIsExplicit`) — no state-machine change needed
  regardless of which implementation path is chosen.

## Go backend handler pattern (ConnectRPC)

Precedent: `SessionService.UnarchiveSession` (`server/services/session_service.go:4234-4250`):
```go
// +api: session:unarchive
func (s *SessionService) UnarchiveSession(
    ctx context.Context,
    req *connect.Request[sessionv1.UnarchiveSessionRequest],
) (*connect.Response[sessionv1.UnarchiveSessionResponse], error) {
    if req.Msg.SessionId == "" {
        return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
    }
    inst := s.FindLiveInstance(req.Msg.SessionId)
    if inst == nil {
        return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
    }
    inst.SetArchivedAt(nil)
    if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
        return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
    }
    return connect.NewResponse(&sessionv1.UnarchiveSessionResponse{}), nil
}
```
Every handler carries a `// +api: <scope>:<action>` marker (feature registry
requirement — `.claude/rules/feature-registry.md`); the equivalent for this
feature would be `// +api: backlog:unarchive`, alongside the existing
`ArchiveBacklogItem` handler in `server/services/backlog_service_lifecycle.go:340`
and `TransitionBacklogItemStatus` at `:486` in the same file, which is where the
new `BacklogService.UnarchiveBacklogItem` handler belongs for consistency.

## React/TypeScript UI wiring

- `BacklogItemDetail.tsx`'s `handleAction` (`web-app/src/components/backlog/BacklogItemDetail.tsx:486-571`)
  is a `useCallback` with a `switch` over action strings; `archive` currently at
  `:538-539` calls `await archiveBacklogItem(item.id)` directly with **no**
  `confirm()` guard, while `delete` at `:541-543` does:
  `if (!confirm("Permanently delete this item and all its history? This cannot be undone.")) return;`
  — this is the exact gap AC0 and the "session pattern" comparison in the
  requirements doc refer to.
- Actions are surfaced via `ActionsSection.tsx` (not yet read in this pass — no
  render branch currently exists for `item.status === "archived"`, per the
  requirements doc); adding the "Unarchive" button there is a UI-only change,
  no new component library or state-management dependency required — it's the
  same pattern as any other conditional-status action button already in that
  file (e.g. `mark_done`, wired at `BacklogItemDetail.tsx:1210` via
  `onMarkDone={() => handleAction("mark_done")}`).
- No new frontend dependency: `@connectrpc/connect`/`connect-web` v2.1.1 already
  provides the RPC client machinery `archiveBacklogItem`/`deleteBacklogItem`
  (destructured at `BacklogItemDetail.tsx:90-91`) use; a new `unarchiveBacklogItem`
  binding will come for free from the regenerated `web-app/src/gen/session/v1/*_pb.ts`
  once `make proto-gen` runs, wired into whatever hook currently supplies
  `archiveBacklogItem`/`deleteBacklogItem` to this component.
- This feature is a single-item detail-view action, not an Omnibar action or
  detector — `.claude/rules/feature-testing-registry.md` and
  `.claude/rules/session-creation-registry.md` (OmnibarAction union,
  DetectorRegistry, 7-touchpoint session-creation registry) do **not** apply
  here; those govern the Omnibar/session-creation surfaces, not backlog item
  detail actions.

## Testing stack (no new tooling)

- Go: standard `go test`, table-driven precedent already in
  `server/services/backlog_service_lifecycle_test.go` (e.g.
  `TestTransitionBacklogItemStatus_should_ArchiveWorkSessions_When_ItemArchived`
  at line 544) — a new
  `TestUnarchiveBacklogItem_should_ClearArchivedAt_When_ItemIsArchived`-shaped
  test fits directly alongside these.
- Frontend: Jest (already the project's test runner per
  `cd web-app && npx jest --no-coverage`), no new test framework needed for the
  new UI action or the new `confirm()` guard on `archive`.

## Feature registry impact

Per `.claude/rules/feature-registry.md`, a new RPC method requires a new
`docs/registry/features/backend/<feature>.json` entry (with `markerFound: true`
once the `// +api: backlog:unarchive` marker is added) and
`make registry-generate` must be run and its output committed. This is a
process/tooling note, not a new dependency.
