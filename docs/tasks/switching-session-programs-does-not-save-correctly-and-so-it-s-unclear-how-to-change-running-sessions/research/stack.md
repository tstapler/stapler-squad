# Tech Stack Research: Switching Session Programs Bug

## Stack Versions

| Component | Version |
|-----------|---------|
| Go | 1.25.0 |
| React | ^19.0.0 |
| TypeScript | ^5.9.3 |
| ent ORM | v0.14.5 |
| Protobuf (buf) | see `buf.gen.yaml` |

---

## Session Program Data Model

### Proto definition

`proto/session/v1/types.proto` — `Session` message, field 7:
```protobuf
// Program running in session (e.g., "claude", "aider").
string program = 7;
```

`proto/session/v1/session.proto` — `UpdateSessionRequest`, field 5:
```protobuf
// Update program command.
optional string program = 5;
```

`proto/session/v1/session.proto` — `CreateSessionRequest`, field 5:
```protobuf
// Optional: Program to run (default: "claude").
string program = 5;
```

### ent ORM schema

`session/ent/schema/session.go`, `Fields()`:
```go
field.String("program").NotEmpty()
```

DB column: `program TEXT NOT NULL` (confirmed in `session/ent/migrate/schema.go`). Not nullable — every session row must have a non-empty program value.

### Go domain model

`session/instance.go`, `Instance` struct:
```go
// Program is the program to run in the instance.
Program string
```

Serialization path: `session/instance_serialization.go`
- `ToInstanceData()` (line 34): `Program: i.Program`
- `FromInstanceData()` (line 178): `Program: data.Program`

The `Program` field is also included in checkpoint snapshots (`instance_checkpoint.go:109`) and hibernate snapshots (`instance_hibernate.go:54`).

### Storage persistence

`session/storage.go` `SaveInstances()` → `saveInstancesToRepo()`:
- Calls `inst.ToInstanceData()` then `repo.Update(ctx, data)` (create on not-found).
- This is the deprecated JSON-backed path still used by `UpdateSession`.

`session/storage.go` `SaveSession()` → `repo.UpdateSession()` / `repo.CreateSession()`:
- The newer ent-backed path. Used by newer code paths but NOT by `UpdateSession` in the service.

`UpdateSession` in `server/services/session_service.go` (line 1533) calls `s.storage.SaveInstances(instances)` — the older path — after mutating `instance.Program` in memory.

---

## Config Defaults for Program Selection

`config/config.go`:

```go
const defaultProgram = "proxy-claude"
```

`Config` struct fields:
- `DefaultProgram string` — persisted in `config.json` as `"default_program"`.
- `AvailablePrograms []string` — persisted as `"available_programs"`. Populated at startup by `GetAvailablePrograms()`.

`GetAvailablePrograms()` probes the user's shell for these candidates in order:
```go
candidates := []string{"proxy-claude", "claude", "claude-code", "gemini", "agy"}
```

Returns full resolved paths for any candidates found via `which`. The first resolved candidate also becomes `DefaultProgram`.

---

## Update Flow (end-to-end)

1. **UI**: `SessionDetailView.tsx` renders an inline `<select>` + text input when `isEditingProgram` is true (line 947). Available options come from `useAvailablePrograms()`.
2. **Save action**: `handleSaveProgram` calls `actions.update({ program: v })` via `useSessionActions` → `useSessionService.updateSession()`.
3. **RPC**: `useSessionService.ts` (line 295) passes `program` to `clientRef.current.updateSession()` → `UpdateSessionRequest.program` (optional string field 5).
4. **Backend handler**: `session_service.go` `UpdateSession()` (line 1429–1441):
   - Guards: only applies if `req.Msg.Program != nil && *req.Msg.Program != "" && instance.Program != *req.Msg.Program`.
   - Mutates `instance.Program` in memory.
   - If `instance.Status == session.Active`, calls `instance.Restart(true)` — this rebuilds and re-launches the tmux command with the new program.
5. **Persist**: `s.storage.SaveInstances(instances)` at line 1533 upserts the full instance to the ent DB via `ToInstanceData()`.

---

## Key Bug Surface Areas

- The `UpdateSession` guard at line 1430 requires `program != ""` — an empty string clears nothing and is silently ignored.
- `SaveInstances` skips instances where `!inst.Started()` (line 247 of storage.go) — a paused session that is not "started" may not persist the program change.
- The UI `<select>` in `SessionDetailView.tsx` is populated from `useAvailablePrograms()` which calls the `GetAvailablePrograms` RPC; if that list is stale or missing the currently-set program, the displayed value may not match what is stored.
- The restart path (`instance.Restart(true)`) rebuilds the tmux launch command from `instance.Program` — if the save fails or restores the old value, the running program and DB value diverge.
