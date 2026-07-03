# Stack Research: Switching Session Programs

Backlog item: `c35902a2-8027-4910-a8bd-2c6d0fd564fc` — "Switching session programs does not
save correctly and so it's unclear how to change running sessions."

(This supersedes an earlier draft of this file — line numbers and code paths below were
re-verified against the current working tree on 2026-07-01.)

## Where "program" lives in the stack

The `program` field (which CLI agent — e.g. `claude`, `aider`, `agy`/Antigravity — runs in a
session's tmux pane) flows through five layers:

### 1. Proto definitions (`proto/session/v1/`)

- `types.proto:28-29` — `Session` message: `string program = 7;` (plain, always-present field
  used for read/display).
- `session.proto:486-487` — `CreateSessionRequest.program` (`string program = 5;`, optional at
  creation time — defaults server-side).
- `session.proto:589-590` — `UpdateSessionRequest.program`: **`optional string program = 5;`**.
  This is the field that matters for the bug — it's proto3 `optional`, so it carries explicit
  field presence (nil vs. empty string vs. a value) rather than relying on the zero-value
  convention used elsewhere in the message.

### 2. Go backend

- `session/instance.go:110-111` and `:423-424` — `Instance.Program string` (in-memory struct
  field) and `StartOptions.Program string` (used when first launching an instance).
- `session/instance_actor_setters.go:191-204` — `SetProgram(program string)` mutates the field
  through the actor's sync-locked state (`setProgramLocked`) and rebuilds the instance snapshot
  (`s.inst.snapshot.Store(buildSnapshot(s.inst))`). This is the only sanctioned mutator; it does
  **not** itself persist to storage or restart the tmux session — callers are responsible for both.
- `server/services/session_service.go`:
  - `UpdateSession` (handler for the `UpdateSession` RPC, starts at **line 1400**), program
    handling block at **lines 1472-1505**:
    - Empty string is resolved to `config.LoadConfig().DefaultProgram` (so the ent `NotEmpty`
      constraint on `program` is never violated).
    - If old/new program straddle Claude ⇄ Antigravity (`agy`/`antigravity`), it calls
      `session.PortSessionHistory` to carry conversation history across the switch.
    - If the session is `Active`, it does an **early** `s.storage.SaveInstances(instances)`
      (comment: "Save before restarting so the new program is persisted even if Restart fails"),
      then calls `instance.Restart(true)` — which kills and recreates the tmux session
      (`session/instance.go:1368`, `KillSession()` + tmux relaunch). If `Restart` errors, the
      handler returns `CodeInternal` immediately, **before** reaching the unconditional final
      save at line 1596.
    - Regardless of active/paused status, there is a final `instances[instanceIndex] = instance;
      s.storage.SaveInstances(instances)` at **lines 1594-1598** that covers every field mutated
      earlier in the handler (title, category, tags, program, working_dir, rate_limit_enabled,
      autonomous_mode, status). For a *paused* session, this final save is the only place the
      program change is persisted (there's no early/paused-specific save, but it is still saved).
  - `UpdateSessionProgram(ctx, sessionID, newProgram)` at **lines 3957-3991** is a **second,
    separate code path** that also switches program (used by "auto-transition" callers, e.g.
    provider-limit-triggered auto-switch — see `server/services/gemini_limits_client.go`, which
    is modified in the current working tree). It duplicates the port-history + `SaveInstances` +
    `Restart` logic independently of `UpdateSession`, and publishes its own
    `SessionUpdatedEvent`. Two divergent implementations of "change program + persist + restart"
    is a likely source of the "does not save correctly" symptom if a fix applied to one path is
    missed in the other — e.g. the early-save-before-restart pattern in `UpdateSession` and the
    save-then-restart pattern in `UpdateSessionProgram` are similar but not identical, and any
    future change to error handling in one is easy to forget in the other.

### 3. ent ORM (persistence)

- `session/ent/schema/session.go:53-54`: `field.String("program").NotEmpty()` — program is a
  required (non-empty) column on the `Session` ent schema. This is why the handler must resolve
  empty-string to a default before persisting.
- `session/ent_repository.go`:
  - `Create` (line 139) and `Update` (line 344) both call `.SetProgram(data.Program)`
    unconditionally (unlike many other optional fields in `Update`, e.g. `WorkingDir`, `Branch`,
    `Category`, which are only set `if data.X != ""`). So the ent write path itself looks correct
    — it always writes whatever is in `InstanceData.Program`.
  - `Update` (line 331) looks the session up **by title** (`session.Title(data.Title)`), not by
    UUID/ID, before applying the update. If a title rename and a program change race, or if two
    instances momentarily share a stale title during a rename, this lookup could silently update
    the wrong row — a possible secondary cause of "does not save correctly," though not confirmed
    without a repro.
- `session/storage.go:238-263` — `SaveInstances` calls `s.repo.Update(...)`, and only calls
  `s.repo.Create(...)` as a *fallback* if `Update` errors (e.g. row not found). Errors from
  either are only logged (`log.Error`), not returned to the caller — `SaveInstances` itself
  always returns `nil`. This means a failed DB write during a program switch is invisible to the
  RPC caller: `UpdateSession`'s early save at lines 1496-1498 only logs a warning
  (`log.Warn("[UpdateSession] failed to pre-save before program restart", ...)`) and proceeds to
  restart anyway. **This is the most direct explanation for "does not save correctly": a DB write
  failure during program switch never surfaces as an RPC error, so the UI reports success (its
  toast/optimistic update reflects the response) while the DB still has the old program.**
  - `saveInstancesToRepo` also `continue`s (skips) any instance where `!inst.Started()`
    (line 247) — a session that has never been started would silently skip persistence of a
    program change, though in practice `UpdateSession` operates on already-loaded/started
    instances so this is a lower-probability path.
- `session/instance.go:543` and `session/session.go:27,52,248,328` — `Program` also appears in
  `Instance` construction and the `session.Data`/serialization structs used for JSON
  (`InstanceData.Program`, `session/storage.go:32`, `json:"program"`), confirming the same field
  name is threaded through JSON (de)serialization as well as the ent-backed store.

### 4. Frontend (`web-app/src`)

- Generated types: `web-app/src/gen/session/v1/session_pb.ts` — `Session.program: string` and
  `UpdateSessionRequest.program?: string` (optional, matching proto3 `optional`).
- Two independent UI entry points let a user change a running session's program (both call the
  same `actions.update({ program })` → `useSessionActions.update` →
  `useSessionService.updateSession`):
  1. `web-app/src/components/sessions/SessionActionsOverflow.tsx` — "⚙️ Change Program" menu item
     (line 618) opens a portal dialog (lines 745-798) with a native `<select>` bound to
     `programPickerValue`, populated from `useAvailablePrograms()`. On Save it calls
     `onChangeProgram?.(session.id, programPickerValue)`.
  2. `web-app/src/components/sessions/SessionDetailView.tsx` — inline "Program" field in the
     session detail panel (lines 948-976), also a native `<select>`, wired to
     `actions.update({ program: v })` via a `handleSave`/`handleCancel` pair (lines 355-358).
  - `SessionCard.tsx:841` and `SessionRow.tsx:387` both wire
    `onChangeProgram={(_id, program) => { void sessionActions.update({ program }); }}` into
    `SessionActionsOverflow` — the `void` discards the promise, so any rejected/failed update is
    silently swallowed at this call site (no `.catch`, no UI error surfaced beyond whatever
    `updateSession`'s internal `dispatch(setError(...))` does).
- `web-app/src/lib/hooks/useSessionService.ts:280-316` — `updateSession` builds the RPC request
  by spreading only the known `UpdateSessionRequest` fields (`program: updates.program`, etc.).
  On success it dispatches `upsertSession(response.session)` into the client-side store; on
  failure it dispatches `setError(...)` but the two call sites above (`SessionCard`,
  `SessionRow`) don't read that error state in a way that blocks/reverts the picker UI — so the
  dialog will happily close with "Saving…" → closed even if the backend silently failed to
  persist (per the `SaveInstances`-swallows-errors issue above).
- `web-app/src/lib/hooks/useAvailablePrograms.ts` — merges a static list
  (`web-app/src/lib/constants/programs.ts` → `PROGRAMS`) with a runtime-detected list fetched
  from `/api/server-info` (`config.AvailablePrograms`, populated by
  `config/config.go:525-568 GetAvailablePrograms()`, which shells out to detect installed CLIs).
  No dedicated component library — plain HTML `<select>` elements styled via `.module.css`
  (project convention favors vanilla-extract for *new* CSS, but these existing pickers use
  CSS Modules).

### UX gap ("unclear how to change running sessions")

There is no persistent, discoverable "current program" control on the session list/row — the
program is only shown as static text (`SessionCard.tsx:661-662`, `SessionRow.tsx:264-267`
`title={session.program}`), and the only way to change it is:
- a buried item inside the "⋯" overflow menu (`SessionActionsOverflow`), or
- an inline edit-in-place pencil icon in the full `SessionDetailView` panel.
Neither surfaces any indication (badge, tooltip, disabled state) that changing the program on an
**active** session will kill and relaunch its tmux pane (`instance.Restart(true)`), which is a
non-trivial, potentially disruptive side effect a user should be warned about before confirming.

## Versions & compatibility

| Component | Version (from repo) | Source |
|---|---|---|
| Go | 1.25.0 | `go.mod` |
| `connectrpc.com/connect` | v1.19.0 | `go.mod` |
| `connectrpc.com/otelconnect` | v0.8.0 | `go.mod` |
| `entgo.io/ent` | v0.14.5 | `go.mod` |
| Next.js | 15.3.2 | `web-app/package.json` |
| React | ^19.0.0 | `web-app/package.json` |
| TypeScript | ^5.9.3 | `web-app/package.json` |
| Program-picker UI | native `<select>` + CSS Modules (no third-party dropdown lib) | `SessionActionsOverflow.tsx`, `SessionDetailView.tsx` |

No third-party dropdown/combobox component (e.g. Radix, Headless UI, react-select) is used for
program switching — both pickers are plain controlled `<select>` elements. Any UX fix (autocomplete,
searchable list, richer picker) would either need a new dependency or a hand-rolled combobox
consistent with `.claude/rules/css-architecture.md` (vanilla-extract for new components).

## Regeneration/build implications

- **Proto changes**: `UpdateSessionRequest.program` already exists as `optional string`
  (field 5) — a UX/behavior fix likely does **not** need a new proto field. If the fix adds
  something like a `confirm_restart` flag or a `dry_run` check, that *would* require editing
  `proto/session/v1/session.proto` and running `make proto-gen`, which regenerates both
  `session/gen/session/v1/*.go`/`gen/proto/go/session/v1/*.go` (Go bindings — currently showing
  as modified in the working tree, e.g. `gen/proto/go/session/v1/session.pb.go`) and
  `web-app/src/gen/session/v1/*_pb.ts` (TS bindings, also currently modified).
- **ent schema**: `program` is already a column (`field.String("program").NotEmpty()`) — no
  schema change is needed unless the fix wants to (a) relax `NotEmpty` to allow explicit "system
  default" storage, or (b) change the `Update` lookup key from `Title` to a stable ID/UUID to
  close the race described above. Either change requires regenerating ent code with the
  project's required flag:
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
  ```
  per `.claude/rules/ent-schema-generation.md` — omitting `--feature sql/upsert` silently breaks
  `UpsertRule`-style methods elsewhere in the codebase, so the flag must not be dropped even for
  an unrelated schema edit.
- **tmux/session lifecycle impact**: Changing `program` on an `Active` session is **not** purely
  a config/state persistence operation — `UpdateSession` calls `instance.Restart(true)`
  (`session/instance.go:1368`), which stops the controller, kills the tmux session
  (`KillSession()`), and relaunches it in the (possibly recreated) worktree directory. This
  touches `session/instance_tmux.go` / `session/process_manager.go` territory, not just
  `session/storage.go`/`config/`. Any fix that changes error handling around program-switch
  should account for partial failure states where the DB was updated but the tmux relaunch
  failed (or vice versa) — see `Restart`'s paused-worktree-recreation and Claude
  `--resume`-clearing logic at `session/instance.go:1397-1413`, which already has to handle
  losing the Claude conversation UUID on worktree recreation.
- **Two persistence call paths**: `UpdateSession` (RPC handler) and `UpdateSessionProgram`
  (direct Go method used by auto-transition/provider-limit callers, e.g.
  `server/services/gemini_limits_client.go`, currently modified in this working tree) each
  independently implement "mutate + port history + save + restart + publish event." A fix should
  probably consolidate these into one code path so persistence semantics can't drift between the
  user-initiated and system-initiated program switches.

## Open questions

1. Does `s.storage.SaveInstances` need to start **returning** the underlying `repo.Update`/
   `repo.Create` error instead of only logging it (`session/storage.go:254-259`)? Today a DB
   failure during a program switch is invisible to both the RPC caller and the frontend, which
   is the most direct root-cause candidate for "does not save correctly."
2. Should `EntRepository.Update` look sessions up by UUID instead of `Title`
   (`session/ent_repository.go:331`) to eliminate any theoretical race between a concurrent
   rename and a program switch?
3. Should `UpdateSession`'s "save before restart" pattern (lines 1494-1498) be unified with
   `UpdateSessionProgram`'s save-then-restart pattern (lines 3969-3986), e.g. by having
   `UpdateSession` call `UpdateSessionProgram` internally, so there's one source of truth for
   program-switch persistence/restart/history-porting behavior?
4. What should the UX be for surfacing failures? Both frontend call sites
   (`SessionCard.tsx:841`, `SessionRow.tsx:387`) use `void sessionActions.update(...)`, discarding
   the promise — does the fix need a toast/inline error, and should the "Change Program" dialog
   stay open (instead of unconditionally closing) on failure (`SessionActionsOverflow.tsx:789-793`
   sets `isSavingProgram=false` and `isProgramPickerOpen=false` in a `finally`, regardless of
   whether `onChangeProgram` succeeded)?
5. Does restarting an *active* session's tmux pane need an explicit user confirmation step
   (given it kills the running process), or a non-destructive alternative (e.g. queue the program
   change to apply on next natural restart/resume) to address the "unclear how to change running
   sessions" UX complaint?
