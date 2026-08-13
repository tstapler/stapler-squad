# Research: Architecture for session `note` field

## 1. Field placement + RPC shape: extend `UpdateSessionRequest`, not a dedicated RPC, not a `SessionGoal`-style sibling table

Two existing precedents point in different directions; the deciding factor is **who writes the field**.

- **`SessionGoal`** (`session/ent/schema/session_goal.go`) is a separate 1:1 ent table
  (`session_uuid` unique index) with its own JSON task-tree payload. It is **agent-written
  only** — set exclusively via the MCP tool `server/mcp/tools_goal.go` (`set_session_goal`),
  never via a user-facing ConnectRPC endpoint. The frontend only *reads* it, denormalized onto
  `GetSessionResponse` as `SessionGoalSummary goal = 59` (`proto/session/v1/types.proto:199`),
  and `GoalPanel.tsx` (`web-app/src/components/sessions/GoalPanel.tsx`) is read-only — no edit
  UI exists for it at all. This shape exists because goal state is complex (status enum + task
  tree) and multi-writer (any agent tool call can update it mid-session).
- **Simple scalar fields that a *user* edits from the UI** — `title`, `category`, `program`,
  `working_dir`, `tags`, `rate_limit_enabled`, `autonomous_mode`, `pause_reason` — all live
  directly as columns on the `Session` ent schema (`session/ent/schema/session.go`) and are
  written through **one shared RPC**, `UpdateSession` (`proto/session/v1/session.proto:579`,
  handled in `server/services/session_service.go:1702` `SessionService.UpdateSession`). Each
  field is `optional` on `UpdateSessionRequest`, checked independently
  (`if req.Msg.Category != nil { instance.SetCategory(...); updatedFields = append(...) }`),
  and setters live on `*Instance` (e.g. `session/instance_actor_setters.go:367`
  `SetCategory`). `RenameSession` is the one dedicated single-field RPC in the service, and it
  exists only because renaming has extra invariants (uniqueness check, is treated as an
  identity change) — not because single-field updates default to their own RPC.

A session note is single-owner (the user), single free-form field, no side effects, no
uniqueness constraint — it matches the `category`/`working_dir` shape exactly, not the
`SessionGoal` shape. **Recommendation: add `note` as a new optional field on the existing
`Session` ent schema and thread it through `UpdateSessionRequest`/`UpdateSessionResponse`,
not a new `SessionGoal`-style table and not a dedicated `UpdateSessionNote` RPC.** This also
avoids the requirements doc's own non-goal of building revisioning/multi-writer machinery —
a dedicated table with its own event/read path is the kind of complexity that shape implies,
even though a dedicated RPC could still enforce last-write-wins the same way.

Field type: use `field.Text("note")` (matches `session/ent/schema/session_summary.go:70`'s
`markdown` field — `field.Text`, not `field.String().MaxLen(...)`), since notes are free-form
markdown with no natural length cap, unlike `SessionGoal.goal` which is capped at 2000 chars
via `field.String(...).MaxLen(2000)`.

## 2. Integration points (touchpoints to update)

| Layer | File | Change |
|---|---|---|
| ent schema | `session/ent/schema/session.go` | Add `field.Text("note").Optional()` to `Session.Fields()` |
| ent regen | — | `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (`.claude/rules/ent-schema-generation.md`) |
| proto | `proto/session/v1/types.proto` | Add `string note = <next tag>;` to the `Session` message (alongside `category` at line 55/596) so it round-trips on read |
| proto | `proto/session/v1/session.proto` | Add `optional string note = 12;` to `UpdateSessionRequest` (`session.proto:579-617`, next free field number after `steer_message = 11`) |
| proto regen | — | `make proto-gen` → regenerates `session/gen/session/v1/*.go` + `web-app/src/gen/session/v1/*_pb.ts` |
| Go instance | `session/instance.go` + `session/instance_actor_setters.go` | Add `Note string` field (mirrors `Category string` at `instance.go:143,477`) and `SetNote(string)` setter (mirrors `SetCategory` at `instance_actor_setters.go:367`) |
| Go handler | `server/services/session_service.go` `UpdateSession` (~line 1755, alongside the `Category` block) | `if req.Msg.Note != nil { instance.SetNote(*req.Msg.Note); updatedFields = append(updatedFields, "note") }` |
| Go adapter | `server/adapters/instance_adapter.go` | Map `instance.Note` → `sessionv1.Session.Note` in the instance→proto conversion (same place `Category` is mapped) |
| Frontend indicator | `web-app/src/components/sessions/SessionCard.tsx` (~line 765, next to the `session.goal?.goalText` block) | Render a small icon/badge when `session.note` is non-empty |
| Frontend panel | New: `web-app/src/components/sessions/NotePanel.tsx` (+ `.css.ts`) | Edit/view toggle: textarea in edit mode (pattern: `BacklogItemForm.tsx:521` `<textarea>`), `ReactMarkdown remarkPlugins={[remarkGfm]}` in view mode (pattern: `DescriptionSection.tsx` / `SessionSummaryPanel.tsx`) |
| Frontend wiring | `SessionDetailView.tsx` (`info` tab block, ~line 836-1248, alongside `<GoalPanel goal={session.goal} />`) | Mount `<NotePanel note={session.note} onSave={...} />`; save calls `updateSession({ id, note })` via `useSessionService.ts` |
| Feature registry | `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/*.json` | New entries per `.claude/rules/feature-registry.md`; run `make registry-generate` |
| e2e test | `tests/e2e/session-notes.spec.ts` | Per `.claude/rules/e2e-test-conventions.md`: `// @feature session:update` header, `data-testid` locators, no `waitForTimeout` |

Note: `react-markdown` (`^10.1.0`) and `remark-gfm` are already dependencies and already used
this way in `DescriptionSection.tsx` and `SessionSummaryPanel.tsx` — reuse that pattern
directly, don't add a new renderer.

## 3. Data flow / consistency: single-user overwrite is fine, and multi-tab live sync is free

The requirements doc explicitly rules out optimistic-concurrency revisioning
("single-user model, last-write-wins overwrite is acceptable") — confirmed as the right call
architecturally: `UpdateSession` has no revision/ETag guard on any of its existing fields
(`title`, `category`, `working_dir`, etc. all overwrite unconditionally), so adding `note` to
the same RPC with the same semantics is consistent with every other field on this entity.

**Multi-tab/multi-client live update already exists and note gets it for free** by riding the
same `UpdateSession` RPC: after any field changes, the handler publishes
`s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, updatedFields))`
(`server/services/session_service.go:1888-1901`), which flows through the `WatchSessions`
server-streaming RPC (`session.proto:29`, `rpc WatchSessions(...) returns (stream
SessionEvent)`) to every connected browser tab/client. This is exactly the same mechanism
`title`/`category`/`tags` updates already use to keep multiple open tabs in sync. **No new
event type or streaming plumbing is needed for the note field** — just add `"note"` to the
`updatedFields` list in the existing handler block, the same as every other scalar field.

This confirms the dedicated-RPC alternative from the requirements doc's own design sketch
(`UpdateSessionNote`) would have to duplicate this event-publish wiring for no benefit —
another point in favor of extending `UpdateSessionRequest` instead.
