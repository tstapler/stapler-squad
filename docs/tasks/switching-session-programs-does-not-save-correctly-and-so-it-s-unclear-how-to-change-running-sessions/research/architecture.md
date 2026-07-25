# Architecture: Switching Session Programs Does Not Save Correctly

> **Status note (2026-07-01):** This backlog item (`c35902a2-8027-4910-a8bd-2c6d0fd564fc`) was already
> researched and largely fixed in commit `914138ec` ("fix(session): program switching now saves
> correctly for all cases", 2026-06-29), which also produced the plan/research artifacts already
> present in this directory (`plan.md`, `validation.md`, `research/{features,pitfalls,stack}.md`).
> This document re-verifies that fix against the current `main` tree (2026-07-01) line-by-line and
> reports what is actually shipped vs. what the original plan called for but never landed. Treat the
> "Root cause hypothesis" section below as historical — it is confirmed fixed — and treat "Proposed
> architecture (to-be)" as the *remaining* gap list, not a from-scratch redesign.

## Current data flow (as-is)

### 1. Session creation (program set once, at creation time)

- `session/instance.go:423` — `Instance.Program string` field, doc: *"Program is the program to run
  in the instance (e.g. 'claude', 'aider --model ollama_chat/gemma3:1b')"*.
- `server/services/session_service.go:1106-1212` (`CreateSession`) — resolves `program` from the
  request, falling back through directory-rule/profile/global defaults (`resolved.Program`), and
  passes it into `session.NewInstance(...)` via `Program: program` (line 1212).
- Historically `program` was **create-time only** — there was no RPC to change it after creation.
  That gap has since been closed (see below).

### 2. Post-creation mutation — `UpdateSession` RPC (the fix)

- Proto: `proto/session/v1/session.proto:576-614`, `message UpdateSessionRequest` field
  `optional string program = 5;` (line 590). `optional` (proto3 field presence) is what lets the
  server distinguish "field not sent" (`nil`) from "explicitly cleared to empty string" (`Some("")`)
  — this distinction is the crux of the historical bug (see Root cause hypothesis).
- Backend handler: `server/services/session_service.go`, `UpdateSession`, program block at
  **lines 1472-1505**:
  ```go
  if req.Msg.Program != nil {
      oldProgram := instance.Program
      newProgram := *req.Msg.Program
      if newProgram == "" {
          newProgram = config.LoadConfig().DefaultProgram
      }
      if instance.Program != newProgram {
          instance.SetProgram(newProgram)
          updatedFields = append(updatedFields, "program")
          // ports claude<->antigravity history if applicable
          // pre-saves + restarts if instance.Status == session.Active
      }
  }
  ```
  Two things to note vs. the historical bug: (a) the guard is `req.Msg.Program != nil` only — the
  erroneous `&& *req.Msg.Program != ""` is **gone**; (b) an empty string is resolved to
  `config.LoadConfig().DefaultProgram` **before** the `instance.Program != newProgram` comparison and
  before persistence, which satisfies the ent `NotEmpty()` constraint on the `program` column
  (`session/ent/schema/session.go:53-54`, `field.String("program").NotEmpty()`).
- Unconditional save: regardless of which fields changed, `UpdateSession` always calls
  `s.storage.SaveInstances(instances)` at **line 1596** before returning. For the Active-session case
  there is *also* a pre-save at **line 1494-1498**, specifically so the program change is durable even
  if the subsequent `instance.Restart(true)` call fails (this is the "RC4 ordering race" fix called
  out in `plan.md`).
- A second, narrower entry point exists for the same operation:
  `SessionService.UpdateSessionProgram(ctx, sessionID, newProgram)` at
  `server/services/session_service.go:3957-3991`. This is not wired to any RPC handler directly (no
  `+api:` marker, not referenced from `session.proto`); grep shows no callers in `server/` or
  `web-app/` — it appears to be a helper for an "auto-transition" code path (its log lines say
  "...in auto-transition") that duplicates the `UpdateSession` program logic (history porting,
  `SaveInstances`, conditional `Restart`, event publish). **This is drift risk**: any future change to
  the program-switch semantics in `UpdateSession` must be mirrored here or the two paths diverge.

### 3. Actor-level mutation and snapshotting

- `session/instance_actor_setters.go:193-204`:
  ```go
  func setProgramLocked(s *instanceState, program string) {
      s.inst.Program = program
      s.inst.snapshot.Store(buildSnapshot(s.inst))
  }
  func (i *Instance) SetProgram(program string) {
      _ = i.sendSyncErr(func(s *instanceState) error {
          setProgramLocked(s, program)
          return nil
      })
  }
  ```
  `SetProgram` routes through the instance's single-writer actor (`sendSyncErr`), so by the time
  `UpdateSession` proceeds to `Restart()`, `i.Program` and the atomic `snapshot` are guaranteed
  consistent — no separate lock-ordering bug here (this file's mutation pattern is the one referenced
  by `.claude/rules/go-double-checked-locking.md`, and `IsDirty` in `session/git/worktree_git.go` is
  the canonical example — `SetProgram` follows the same actor-serialized-write discipline, just via a
  message-passing actor rather than a mutex).

### 4. Persistence layer

- `session/storage.go:238-240` (`Storage.SaveInstances`) → `saveInstancesToRepo` → ent repository
  upsert. `session/ent_repository.go:139` and `:344` both call `.SetProgram(data.Program)` when
  building the ent mutation (create and update paths respectively) — persistence is unconditional
  given a non-empty string, matching the `NotEmpty()` schema constraint.
- Schema: `session/ent/schema/session.go:53-54` — `field.String("program").NotEmpty()`. This is why
  the handler must resolve `""` to `config.LoadConfig().DefaultProgram` before calling `SetProgram`;
  passing a literal empty string to the ent mutation would either fail validation or (worse, if
  validation were ever loosened) silently write an empty value that breaks `buildLaunchCommand`.

### 5. Runtime effect — restart with new program

- `session/instance.go:1368` (`Instance.Restart`) — on Active sessions, `UpdateSession` calls
  `instance.Restart(true)` (preserve terminal output) after `SetProgram` has already run. `Restart`
  kills the current tmux session (`KillSession()`), then calls
  `program := i.buildLaunchCommand(claudeSessionID)` (line 1432) — this reads `i.Program` directly,
  so the *live* PTY command is rebuilt from the newly-set value, not a stale cached command.
  `buildLaunchCommand` (`session/instance_tmux.go:74-88`) classifies `i.Program` via
  `classifyProgram(...)` into `claudeProgram` (adds `--resume <uuid>`, MCP flags, etc.) or
  `plainProgram` (uses the raw command string, e.g. `aider ...`) and appends `i.CLIFlags`.
- History continuity: if the switch is specifically claude↔antigravity (`agy`), `UpdateSession`
  calls `session.PortSessionHistory(ctx, oldProgram, newProgram, instance)`
  (`server/services/session_service.go:1487`, impl at `session/history_transfer.go:41-71`), which
  imports turns from the old adapter and exports them to the new adapter's format so conversation
  continuity survives the switch. For any other pair of programs (e.g. claude→aider→claude, or
  claude→anything-not-agy), no porting happens and no history transfer error is possible — but see
  the residual gap noted below.

### 6. Frontend UI (discoverability + wiring)

- `web-app/src/components/sessions/SessionActionsOverflow.tsx`:
  - Optional prop `onChangeProgram?: (sessionId: string, program: string) => Promise<void> | void`
    (line 65).
  - Menu item "⚙️ Change Program" (lines 613-620) opens an inline dialog
    (`isProgramPickerOpen`, lines 746-802) with a `<select>` populated from
    `useAvailablePrograms()` plus a `"System default"` (`value=""`) option, and warns
    `"The session will restart."` when `session.status === 3 /* ACTIVE */` (line 763).
  - Save handler (line 788-794) calls `onChangeProgram?.(session.id, programPickerValue)`.
- Wiring: `SessionCard.tsx:841` and `SessionRow.tsx:387` both pass
  `onChangeProgram={(_id, program) => { void sessionActions.update({ program }); }}` —
  `sessionActions.update` is the `useSessionActions(sessionId).update` adapter
  (`web-app/src/lib/hooks/useSessionActions.ts:32-33`) which forwards to
  `updateSession(sessionId, updates)`.
- `useSessionService.ts:280-316` (`updateSession`) builds the `UpdateSessionRequest` and includes
  `program: updates.program` (line 295) unconditionally passed through — so `program: ""` (System
  default) is sent as an explicit empty string, matching proto `optional` semantics, not omitted.
- A second, older editing surface exists in `SessionDetailView.tsx`: an inline "Info" tab editor
  (`isEditingProgram` state, `programValue` state, lines ~175-181, 355-358, 948-976) using the shared
  `makeStringFieldEditor` helper, calling `actions.update({ program: v })` on save. Per the fix commit,
  a `useEffect` was added (visible at lines 180-181: `if (!isEditingProgram) setProgramValue(session.program || "")`,
  dependency array `[session.program, isEditingProgram]`) so this local state re-syncs whenever
  `WatchSessions` pushes an updated `session.program` — this closes the "stale local state after
  external update" gap (RC2 in `plan.md`).

## Root cause hypothesis

**Historical (confirmed fixed).** The bug had two parts, both addressed by commit `914138ec`:

1. **RC1 — backend guard silently dropped "System default" saves.** The original guard was
   `req.Msg.Program != nil && *req.Msg.Program != "" && instance.Program != *req.Msg.Program`. Since
   the UI's "System default" option has `value: ""` (`web-app/src/lib/constants/programs.ts`), any
   attempt to clear a session's program to the default silently no-opped: the RPC returned HTTP 200
   with unchanged data, and the frontend had no way to detect the mismatch. **Verified fixed**: the
   current guard at `session_service.go:1474` is `if req.Msg.Program != nil { ... }` with the empty
   string resolved to `config.LoadConfig().DefaultProgram` before the equality check — the `!= ""`
   short-circuit is gone.
2. **RC2 — stale React state.** `SessionDetailView.tsx`'s `programValue` had no re-sync effect,
   so after a successful save (or an external `WatchSessions` push) the input could show a value that
   no longer matched `session.program`. **Verified fixed**: the `useEffect` at lines 180-181 now
   re-syncs whenever `!isEditingProgram`.
3. **RC3 — discoverability.** Prior to the fix, the only way to change a session's program was to
   scroll to the "Info" tab of the detail view. **Verified fixed**: `SessionActionsOverflow.tsx` now
   surfaces "⚙️ Change Program" as a first-class menu item on both the card and row views.
4. **RC4 — persist-before-restart ordering race.** If `Restart()` were called before
   `SaveInstances`, a restart failure would leave the program change neither running nor persisted.
   **Verified fixed**: `session_service.go:1494-1498` explicitly pre-saves before calling
   `instance.Restart(true)`, with a comment: *"Save before restarting so the new program is persisted
   even if Restart fails."*

**Residual (not covered by the shipped fix — see "Proposed architecture" below):**

5. **Conversation continuity when leaving the claude/antigravity pair.** `PortSessionHistory` only
   acts when *both* old and new programs are recognized by the claude or antigravity adapters
   (`session/history_transfer.go:47-57`); for any other transition (e.g. claude → aider, or
   aider → claude) it returns `nil` immediately (line 59-61) without clearing
   `i.claudeSession.ConversationUUID` or `i.HistoryFilePath`. Restart's `claudeSessionID` capture
   (`session/instance.go:1387-1390`) reads `i.claudeSession.ConversationUUID` directly. If a user
   switches claude → aider → claude, the stale claude `ConversationUUID` from *before* the first
   switch is still present and will be passed as `--resume <uuid>` on the second switch back — this
   was flagged as task 3 in the original `plan.md` ("Clear `claudeSession.ConversationUUID` when
   switching program away from claude family") and is **not implemented** in the current tree.
6. **No "pending on next resume" indicator** for non-Active sessions (plan.md task 7) — not found in
   `SessionActionsOverflow.tsx`, `SessionCard.tsx`, or `SessionDetailView.tsx`.
7. **No automated test coverage** for the program-switch paths specifically:
   - Go: no `TestUpdateSession_Program*` function exists in
     `server/services/session_service_test.go` (only tag/title/status tests are present around
     lines 490-661). Plan.md task 8 ("Go tests: `UpdateSession` program — Active (restart occurs),
     Stopped (save only), empty-string clear, restart error path") is unimplemented.
   - Frontend: no `SessionActionsOverflow.test.tsx` file exists at all, so the program picker dialog
     (open, select, save, cancel, "System default") has zero Jest coverage. Plan.md task 9 is
     unimplemented.
8. **`UpdateSessionProgram` helper duplication** (`session_service.go:3957-3991`) reimplements the
   same program-switch logic as the `UpdateSession` RPC handler but is reached by no discoverable
   caller in the current tree — a maintenance hazard if the two diverge (see data-flow §2 above).

## Proposed architecture (to-be)

The core save/notify/restart pipeline does not need a redesign — it is sound and already shipped.
The remaining work is closing the gaps above without introducing new proto surface:

1. **Clear stale conversation linkage on any program-family exit (closes gap 5).** In the
   `UpdateSession` program block (`session_service.go:1480-1490`), when `oldProgram` was
   claude-or-antigravity and `newProgram` is neither (i.e. leaving the ported-history family
   entirely), explicitly clear `instance.claudeSession.ConversationUUID` and `instance.HistoryFilePath`
   the same way `Restart()` already does for the paused-worktree-recreation case
   (`session/instance.go:1415-1419`). This should live in the service layer (or as a new
   `Instance.ClearConversationLinkage()` actor-routed setter in `instance_actor_setters.go`, mirroring
   `SetProgram`'s `sendSyncErr` pattern) — not in `Restart()`, since the linkage should be cleared at
   switch time regardless of whether the session is Active (and thus restarts) or Paused/Stopped
   (where `Restart()` never runs).
2. **"Pending on next resume" indicator (closes gap 6).** Purely additive frontend work: when
   `UpdateSession` changes `program` on a non-Active session, the response already carries the new
   `session.program` — `SessionCard.tsx` / `SessionRow.tsx` can diff `session.program` against
   `session.launchCommand`-derived program (or simply render a badge whenever `status !== ACTIVE`
   after a program edit) with no new RPC or proto field required. `Session.launchCommand` already
   exists on the wire (`i.LaunchCommand`, set in `Restart()` at `session/instance.go:1442`) as the
   source of truth for "what actually launched last time" vs. "what will launch next time."
3. **Test coverage (closes gap 7).** No architecture change; this is pure test-writing debt.
   Suggested Go cases in `session_service_test.go`: Active session program change triggers
   `Restart` and persists even if `Restart` errors; Stopped/Paused session program change persists
   without restart; `program: ""` resolves to `config.LoadConfig().DefaultProgram` and persists (not
   dropped); no-op when `newProgram == instance.Program`. Suggested Jest cases in a new
   `SessionActionsOverflow.test.tsx`: opening the picker pre-fills `session.program`; selecting
   "System default" and saving calls `onChangeProgram` with `""`; the "session will restart" warning
   only renders when `session.status === ACTIVE`.
4. **Collapse `UpdateSessionProgram` duplication (closes gap 8).** Either delete
   `SessionService.UpdateSessionProgram` if it truly has no caller (confirm via a repo-wide
   `grep -rn "UpdateSessionProgram"` before removing — this research pass found only the definition
   and its own log lines), or refactor the `UpdateSession` RPC handler's program block to call it
   instead of duplicating the logic inline, so there is a single source of truth for
   "port history → save → conditionally restart → publish event."

No proto changes, no ent schema changes, and no new RPC are required for any of the above — all four
items are implementable within the existing `UpdateSession` RPC and existing `Instance` actor methods.
This also means the **Session Creation Mode Registry's 7-touchpoint pattern does not apply** here:
that pattern governs *new session creation modes* (new `SessionType` enum values), and program
switching is a metadata mutation on an existing session, not a creation mode.

## Component boundaries / touchpoints

| Layer | File | Role | Status |
|---|---|---|---|
| Proto | `proto/session/v1/session.proto:590` | `optional string program` on `UpdateSessionRequest` | done, correct |
| ORM schema | `session/ent/schema/session.go:53-54` | `field.String("program").NotEmpty()` | done, drives empty→default resolution |
| Backend RPC handler | `server/services/session_service.go:1472-1505` | Guard, resolve empty→default, `SetProgram`, history port, pre-save, conditional `Restart` | done |
| Backend RPC handler | `server/services/session_service.go:3957-3991` (`UpdateSessionProgram`) | Duplicate helper, no known caller | **drift risk — recommend consolidating (item 4 above)** |
| Actor mutation | `session/instance_actor_setters.go:193-204` (`SetProgram`) | Single-writer update of `Program` + snapshot | done |
| History porting | `session/history_transfer.go:41-71` (`PortSessionHistory`) | claude↔antigravity turn conversion | done for claude/agy pair; **gap for other pairs (item 1 above)** |
| Runtime launch | `session/instance.go:1368-1477` (`Restart`), `session/instance_tmux.go:74-88` (`buildLaunchCommand`) | Kill tmux, rebuild launch command from live `i.Program`, restart controller | done |
| Persistence | `session/storage.go:238-240` → `session/ent_repository.go:139,344` | Unconditional upsert of `Program` | done |
| Frontend action surface | `web-app/src/components/sessions/SessionActionsOverflow.tsx:613-802` | "Change Program" menu item + inline picker dialog | done |
| Frontend action surface | `web-app/src/components/sessions/SessionDetailView.tsx:175-181,948-976` | Inline "Info" tab editor with re-sync `useEffect` | done |
| Frontend wiring | `SessionCard.tsx:841`, `SessionRow.tsx:387` | `onChangeProgram` → `sessionActions.update({ program })` | done |
| Frontend hook | `web-app/src/lib/hooks/useSessionService.ts:280-316` | Builds `UpdateSessionRequest`, passes `program` through | done |
| Test coverage | `server/services/session_service_test.go`, (missing) `SessionActionsOverflow.test.tsx` | Program-switch specific cases | **missing (item 3 above)** |

## Runtime semantics (what happens to the live tmux session)

- **Active session:** `UpdateSession` pre-saves the new `program` to storage, then calls
  `instance.Restart(true)` — this is a **kill-and-respawn**, not a live in-place substitution: the
  existing tmux pane/process is killed (`KillSession()`), a new tmux session is created with a
  freshly built launch command (`buildLaunchCommand` reading the new `i.Program`), and (when
  preserving output) the old terminal scrollback is echoed back into the new pane as a visual marker
  before the new program's own output begins. The `StatusManager`/detection controller is also
  restarted (`StartController()`). If `Restart` fails, the RPC returns `CodeInternal` but the program
  metadata is already durably saved (pre-save at line 1494-1498), so a subsequent manual restart or
  page reload will pick up the correct (new) program.
- **Paused/Stopped/Hibernated session:** the metadata write (`SetProgram` + `SaveInstances`) happens,
  but no tmux/process action occurs — `instance.Status == session.Active` is false, so the `Restart`
  branch is skipped entirely. The new program takes effect the next time the session is resumed
  (`ResumeHibernatedSession` / resume-from-paused flow), which builds its own launch command from the
  now-updated `i.Program`. This is the scenario gap 6 ("pending on next resume" indicator) is meant to
  surface to the user — today there is no UI cue that a Stopped session's program was changed but
  won't actually run until the next resume.
- **History/context continuity:** only the claude↔antigravity pair gets active conversation-turn
  porting (`PortSessionHistory`); any other program transition switches the underlying agent CLI with
  no attempt to translate conversation state — the new program simply starts fresh (or, if it happens
  to be claude again after a detour through a non-ported program, may incorrectly `--resume` a stale
  UUID per residual gap 5 above).
