# Architecture Research: import-external-session

## 1. Applicable patterns

- **Anti-corruption layer, already partially built.** `session.HistoryAdapter`
  (`session/history_adapter.go`) is exactly this pattern: `ClaudeAdapter` /
  `AgyAdapter` translate a foreign CLI's on-disk format into `[]CanonicalTurn`
  and back. Import should reuse this layer as-is for the "read the external
  program's history" step rather than inventing a parallel translator.
- **Two-tier instance model already exists — this is a promotion, not a new
  concept.** `session.InstanceType` (`session/types.go:11-21`) already
  distinguishes `InstanceTypeManaged` (persisted, full lifecycle) from
  `InstanceTypeExternal` (ephemeral, discovered, limited). Critically,
  **`InstanceType` is never written to the ent schema** — `ent_repository.go`
  persists `SessionType` (directory/new_worktree/...) but has no
  `InstanceType` column at all (confirmed by grep: only `SessionType` appears
  in `Create`/`Update`/`sessionToInstanceData`). External instances today live
  only in `ExternalSessionDiscovery.sessions` (an in-memory map), never in the
  DB. "Import" is therefore a **promotion / lifecycle state machine**
  transition: `ephemeral-external → durable-managed`, not a variant of
  instance creation. No such promotion path exists in the codebase yet — it
  is net-new.
- **Command-then-confirm two-phase action**, already the shape used by
  destructive-action flows elsewhere in the repo (per project safety
  posture). Import naturally decomposes into `PrepareImport` (side-effect-free
  discovery/verification) → `CommitImport` (create managed Instance, copy
  history) → `ConfirmKill` (separate, reversible-until-confirmed gate). This
  maps cleanly onto the requirement "verify first, kill on confirmation."
- **Existing resume mechanism is the reuse point, not a new command.**
  `CreateSessionRequest.ResumeId` / `ForkSourceId` already exist and are
  handled in `server/services/session_service.go` (~line 1133 onward): a
  client-supplied `resume_id` is validated (`resumeIDRe`) and threaded through
  to `claude --resume <uuid>` via `ClaudeCommandBuilder`
  (`session/claude_command_builder.go`). Import's "manual/JSONL" path is
  almost entirely: find the UUID → call the *existing* `CreateSession` RPC
  with `SessionType=DIRECTORY`, `Path=<external cwd>`, `ResumeId=<uuid>`.

## 2. Integration points (concrete)

| File | What's there today | What import needs from it |
|---|---|---|
| `session/instance.go` | `Instance` struct (`InstanceType` field, line 309); `SessionType = config.SessionType` alias (line 427); `finishInstanceConstruction` (line 706); `Destroy()` (line 1221) | No new `SessionType` needed (see §4.1). Import's "commit" step is a normal `Instance` construction via the existing `NewInstance`/`opts.SessionType = SessionTypeDirectory` path (~line 576-606), just with `Path` = the external session's cwd and a `ResumeId` sourced from correlation instead of user input. |
| `session/ent_repository.go` | `Create`/`Update` persist `SessionType` string (lines 164-165, 373-374) but never `InstanceType` — external instances are never rows in the DB | Import's commit step is a plain `EntRepository.Create` call for a **new managed row**; nothing to migrate from an "external row" because none exists. The only DB novelty is that `HistoryFilePath`/`ConversationUUID` must be pre-populated from correlation rather than discovered later by `HistoryLinker`. |
| `server/services/session_service.go` | `CreateSession` RPC (line 1104), path-required guard (line 1112), `ResumeId`/`ForkSourceId` handling (lines 1133-1160), `SessionType` switch (lines 1478-1486) | **No new switch case required.** Reuse `SessionType_SESSION_TYPE_DIRECTORY` unconditionally for imports — the external session already has a real, existing working directory; import never needs `new_worktree`/`new_project` semantics. Add a *new* RPC (e.g. `ImportExternalSession`) that is a thin orchestrator calling into discovery/correlation, then delegating to the same code paths `CreateSession` uses (or calling `CreateSession` internally) — not a new field on `CreateSessionRequest`, since the 7-touchpoint registry is scoped to session *creation modes* with distinct workdir semantics, which this isn't. |
| `session/mux/discovery.go` | `Discovery.Scan()`/`GetSessions()` returns `[]*DiscoveredSession{SocketPath, Metadata, LastSeen}` (lines 14-19, 122-131) | Source of the "already discovered" list for the ssq-mux import path. `Metadata.Cwd`, `.Command`, `.TmuxSession`, `.PID` map directly onto `ExternalInstanceMetadata` fields already used in `external_discovery.go:137-162`. |
| `session/external_discovery.go` | `ExternalSessionDiscovery.handleNewSession` (line 120) already builds an `Instance{InstanceType: InstanceTypeExternal, ...}`, attaches tmux (line 170), calls `finishInstanceConstruction` (line 182); `KillExternalSession()` (in `instance_tmux.go`) already kills the tmux session for an `InstanceTypeExternal` instance | This is the **existing promotion candidate for the ssq-mux path**: the `Instance` object for a `DiscoveredSession` already exists in memory with correct `Path`/`Program`/`ExternalMetadata`. Promotion = (a) correlate its `ConversationUUID` via `HistoryLinker`/`HistoryFileDetector`, (b) construct a *new* `InstanceTypeManaged` `Instance` (fresh tmux session, not reusing the external one — see §3), (c) persist it via `EntRepository.Create`, (d) call `KillExternalSession()` on confirm. |
| `session/history_linker.go` | `HistoryLinker.correlateSession` (line 221): PID-based `detector.Detect(pid)` fast path via open-files inspection, falls back to `detector.DetectByPath(effectivePath)` — "most recently modified JSONL in that project dir" heuristic (lines 254-273); explicit warning at lines 258-263 that the path-fallback heuristic is **wrong when multiple sessions share a directory** | This is exactly the mechanism flagged as unproven for *unmanaged* processes in the requirements' Feasibility Risks. `Detect(pid)` needs a real PID, which a plain-tmux pane has (via `tmux list-panes -F '#{pane_pid}'`) even without ssq-mux — so PID-based detection should work for manual import too, in principle, without new correlation logic. `DetectByPath` is the fallback and is the disambiguation weak point (see §4.3). |
| `session/history_transfer.go` | `PortSessionHistory(ctx, oldProgram, newProgram, i)` (line 41): `srcAdapter.Import` → `dstAdapter.Export` → post-switch `history.jsonl` bookkeeping; explicitly built for **switching programs on an already-managed `Instance`** (mutates `i.claudeSession` in place, lines 144-160) | Multi-program import (`AgyAdapter` etc.) can reuse `Import`/`Export` directly, but `PortSessionHistory` itself assumes `i` is already a live, managed `Instance` with `i.claudeSessionMu` etc. initialized. For import, the natural call is `srcAdapter.Import(ctx, externalInstance)` to get canonical turns, then `dstAdapter.Export(ctx, turns, newManagedInstance)` — i.e. call the two adapter methods directly rather than `PortSessionHistory`, since there is no single already-managed `Instance` to mutate in place until the new one is created. This is the "refactoring, not just calling" risk called out in the requirements' Feasibility Risks — confirmed here. |
| `session/history_adapter.go` | `HistoryAdapter` interface: `Name()`, `CanHandle(program)`, `Import(ctx, inst) ([]CanonicalTurn, error)`, `Export(ctx, turns, inst) error` | Both `Import` and `Export` take `*Instance`, not a raw PID/path — so even the *source* side of an import needs a constructed `Instance` (managed or external) before adapters can run. This confirms the external `Instance` (already built by `ExternalSessionDiscovery` for ssq-mux, or a lightweight synthetic one for manual import) must exist before history transfer, reinforcing the "construct external Instance first, then promote" flow over "resume blind and hope." |

## 3. Data flow / consistency walk-throughs

### 3a. ssq-mux import (single session)

1. User sees a `DiscoveredSession` (already an in-memory `InstanceTypeExternal`
   `Instance` per `ExternalSessionDiscovery.sessions`) in the UI and clicks Import.
2. **Prepare**: `HistoryLinker`/`HistoryFileDetector` correlate the external
   instance's PID (`ExternalMetadata.OriginalPID`) to a JSONL → `ConversationUUID`.
   *(No mutation yet — read-only.)*
3. **Verify**: adapter (`ClaudeAdapter.Import`) reads canonical turns from that
   JSONL; UI/RPC shows a preview (turn count, last message) so the user can
   confirm this is the right conversation before anything destructive happens.
4. **Commit**: construct a *new* `InstanceTypeManaged` `Instance` with
   `SessionType=Directory`, `Path=<external cwd>`, and either `ResumeId=<uuid>`
   (Claude case — start with `--resume`) or, for non-Claude source programs,
   `dstAdapter.Export(turns, newInstance)` to seed the target program's native
   format before first start. Call `EntRepository.Create` to persist the row.
   Start the new tmux session (`Instance.Start()`), which cold-restores via
   `--resume`.
5. **Confirm-kill gate**: only after step 4's new session is observed to have
   actually resumed the right conversation (e.g. `HistoryLinker` reports the
   new `Instance`'s linked UUID matches step 2's UUID) does the UI offer "kill
   original." On confirm, call `KillExternalSession()` on the *old*
   `InstanceTypeExternal` instance (already implemented, `instance_tmux.go`).
6. Remove the old instance from `ExternalSessionDiscovery.sessions` (already
   happens automatically next `Scan()` once the mux socket disappears, via
   `handleRemovedSession`).

**Where state can desync:**
- Step 4 succeeds (DB row + tmux started) but step 5's kill fails (IDE terminal
  refuses signal, tmux session gone already, etc.) → **two live processes**,
  both potentially writing to the same JSONL file / git worktree. This is the
  most dangerous failure mode; needs surfacing as a distinct, non-silent error
  state (not just "kill failed, ignore").
- Step 4's `EntRepository.Create` succeeds but the tmux `Start()`/`--resume`
  fails afterward (e.g. race with the external process still holding a lock,
  or `claude --resume` fails because the external process hasn't released the
  session file yet) → an orphaned **managed DB row with no live process** and
  the *original* external process untouched. Needs a compensating delete of
  the just-created row, or a "failed/incomplete" status the user can retry
  or discard.
- Kill happens (step 5) but is not observed to complete before the user closes
  the confirm dialog → no undo per requirements, so kill must be
  fire-and-confirm synchronously (await process exit / tmux session gone)
  before reporting success, not fire-and-forget.

### 3b. Manual / plain-tmux (no ssq-mux) import

1. User points at a project directory (or pastes a UUID). No `Instance` object
   exists yet — this is genuinely new territory; `ExternalSessionDiscovery`
   never saw this process.
2. **Prepare**: find the tmux pane's PID via `tmux list-panes` (or user-supplied
   PID/UUID), then use the *same* `HistoryFileDetector.Detect(pid)` /
   `DetectByPath(path)` used by `HistoryLinker` — this validates Feasibility
   Risk #1 architecturally (the detector doesn't care whether the `Instance`
   is registered; it only needs a PID or a path), but the multi-process
   disambiguation problem (§4.3) is real for `DetectByPath`.
3. **Verify**: same as 3a step 3 — read canonical turns, show preview.
4. **Commit**: same as 3a step 4. Crucially — **per open question 2, no live
   PTY attach to the old pane is required for this.** The old pane is only
   ever the source of (a) a PID for correlation and (b) a kill target; the
   conversation is resumed fresh via `claude --resume <uuid>` in a brand-new
   stapler-squad tmux session, not by reattaching to the old PTY. This matches
   how cold-restore already works for managed sessions today (`instance.go`
   lines ~863, 1046-1049 — "cold restoring with --resume" is the existing,
   proven pattern; import is just cold-restore against a *foreign* origin
   instead of a stapler-squad-paused one).
5. **Confirm-kill**: no `ExternalMetadata`/`KillExternalSession()` available
   (that method requires `InstanceType == InstanceTypeExternal`, which this
   session never had). Needs a new primitive — kill-by-PID or
   `tmux kill-session -t <name>` against the plain tmux session name the user
   pointed at — deliberately *not* reusing `KillExternalSession()`, which is
   ssq-mux-specific by construction (checks `ExternalMetadata.TmuxSessionName`).

**Where state can desync:** same two hazards as 3a (double-live-process on
kill failure; orphaned DB row on start failure), plus a manual-path-specific
one: the user-identified PID could refer to a *different* process by the time
kill executes (PID reuse after the original process exited) — kill-by-PID
without a liveness/identity recheck immediately before signaling is unsafe.

## 4. Direct answers to the open questions

### 4.1 New `SessionType` vs. lighter action?

**Lighter action — do not add a `SessionType`.** `SessionType` (`config/types.go:167-190`)
exists purely to describe *how a working directory comes into being*
(`directory`, `new_worktree`, `existing_worktree`, `new_project`, `one_off`).
Every import scenario in scope already has a real, existing working directory
(the external process's cwd) — there is no new workdir-provisioning semantic
to add. The 7-touchpoint registry (`.claude/rules/session-creation-registry.md`)
exists to keep that specific fan-out (proto enum, Go switch, frontend union,
etc.) in sync; forcing import through it would mean inventing a fake
`SESSION_TYPE_IMPORT` that behaves identically to `SESSION_TYPE_DIRECTORY` in
6 of 7 touchpoints and only differs in "where does `ResumeId` come from,"
which is exactly the kind of speculative branching this repo's own
interface-pollution checklist warns against for Go/proto abstractions.
Recommendation: reuse `SessionType_SESSION_TYPE_DIRECTORY` for the commit
step, and put all new behavior behind a **separate, new RPC**
(`ImportExternalSession` / `BatchImportExternalSessions`) that internally
constructs the same `CreateSessionRequest` shape (`Path`, `ResumeId`,
`SessionType=DIRECTORY`) that `CreateSession` already knows how to handle,
plus the discovery/correlation/kill orchestration that `CreateSession` has no
reason to know about.

### 4.2 Does manual (no ssq-mux) import need a live PTY attach?

**No — it's a metadata + history operation, per the codebase's own existing
cold-restore pattern.** `instance.go`'s cold-restore path (`--resume` at lines
863 and 1046-1049) already proves stapler-squad can resume a Claude
conversation in a brand-new tmux session purely from a `ConversationUUID`,
with zero dependency on the *previous* process's PTY. Import should follow
exactly this: correlate PID/path → UUID (read-only), then `claude --resume
<uuid>` in a **new** managed tmux session. The old pane's PTY is touched
exactly once, at the very end, to send a kill signal — never read from or
attached to. This also sidesteps a real hazard: attaching to/streaming from a
tmux pane the user didn't create through stapler-squad risks racing with
whatever the user is still typing into it.

### 4.3 What identifies "the same session" for batch-import disambiguation?

The strongest identifier is the **(PID, cwd) pair combined with the JSONL's
own embedded session UUID**, not cwd alone. `HistoryFileDetector.Detect(pid)`
(used by `HistoryLinker.correlateSession`, `history_linker.go:246-252`) is
PID-scoped and therefore inherently disambiguates same-directory processes
correctly — each process has its own PID and (per Claude Code's own file
layout) its own JSONL. The failure mode flagged explicitly in this repo's own
comments (`history_linker.go:258-263`) is the **path-only fallback**
(`DetectByPath`, "most recently modified JSONL in that project dir"), which
is documented there as *already known to be wrong* when multiple sessions
share a directory — that comment is about paused/hibernated managed
sessions, but the same ambiguity is worse for batch-importing multiple
*unmanaged* processes in the same directory, since there's no "most
recent within a known instance" scoping at all.
Recommendation for batch import: require PID-based detection
(`Detect(pid)`, via `tmux list-panes -F '#{pane_pid}'` per candidate pane) as
the primary and only mechanically-reliable disambiguator; treat
`DetectByPath` results as ambiguous/needs-user-confirmation whenever more than
one candidate process shares an effective root dir, rather than silently
picking "most recent."

## 5. Failure modes for the cross-cutting/high-stakes aspect

- **Partial failure ordering is the central risk, not any single step.** The
  three operations — (a) persist new managed `Instance` row, (b) start new
  tmux session / resume conversation, (c) kill original external process —
  are not transactional and touch three different subsystems (ent/SQLite,
  tmux, the external process's OS-level lifecycle). Any 2-of-3 completing
  while the third fails leaves an inconsistent, user-visible state.
- **Recommended ordering to minimize blast radius:** always do the
  *reversible* operations first and the *irreversible* one (kill) strictly
  last, gated on an explicit "the new session actually resumed correctly"
  check — this is already implied by the requirements ("verify first, kill on
  confirmation") but the architecture must enforce it mechanically: kill must
  be a separate RPC call the client can only invoke after receiving a success
  response from commit, not a flag on the commit request.
- **Rollback story for (a) succeeds / (b) fails:** delete the DB row (or mark
  it `failed`/`incomplete` status rather than hard-deleting, so the user can
  see what was attempted) and leave the original external process untouched
  — this is the safe failure because nothing destructive happened yet.
- **Rollback story for (a)+(b) succeed / (c) [kill] fails:** no rollback of
  (a)/(b) is desirable or safe (the user now has a working, correctly-resumed
  managed session) — the correct response is a *visible, actionable* error
  state ("import succeeded, kill failed — the original session titled X is
  still running at PID Y, terminate it manually or retry kill") rather than
  auto-retrying or silently swallowing the failure. Two live processes writing
  the same JSONL/worktree is the one state that must never be silent.
- **Batch import all-or-nothing vs. partial:** given the ordering above, batch
  import should be modeled as N independent (commit, then confirm-kill) state
  machines, not one transaction — a failure on session 3 of 5 has no reason to
  roll back 1-2 (already-succeeded, safe, irreversible-in-a-good-way) or block
  4-5 (independent). The per-session Observability Requirement (structured log
  per import attempt) doubles as the audit trail needed to reconcile a
  partially-failed batch after the fact.
- **PTY-fan-out infra note:** `project_plans/pty-multiplexer/research/architecture.md`
  covers `CircularBuffer`/`PTYAccess`/`mux.Multiplexer` for concurrent-reader
  streaming race conditions — not directly reusable here since import
  deliberately avoids attaching to the old PTY at all (§4.2), but confirms
  that *if* a future iteration wanted a "live preview of the external pane
  before import" feature, that infra would be the right place to look.

## Event-Command-Policy table (EventStorming)

| Domain Event (what happened) | Policy trigger (whenever X, then…) | Command (intent to change state) | Actor / System |
|---|---|---|---|
| `ExternalSessionDiscovered` (ssq-mux socket found) | whenever a new mux socket is probed successfully | `RegisterDiscoveredSession` | `mux.Discovery` / `ExternalSessionDiscovery` |
| `ImportRequested` (user selects one or more external sessions) | whenever user clicks Import | `PrepareImport(sessionRef)` | User → new Import RPC |
| `ConversationCorrelated` (UUID found for PID/path) | whenever `PrepareImport` runs | `DetectHistoryFile(pid \| path)` | `HistoryFileDetector` / `HistoryLinker` |
| `ConversationCorrelationAmbiguous` (multiple candidate JSONLs) | whenever more than one process shares an effective root dir | `RequestUserDisambiguation` | Import orchestrator → User |
| `HistoryPreviewReady` (turns loaded, ready for user to eyeball) | whenever correlation succeeds | `ImportCanonicalTurns` | `HistoryAdapter.Import` |
| `ImportVerified` (user confirms preview looks correct) | whenever user approves preview | `CommitImport` | User → Import orchestrator |
| `ManagedInstanceCreated` (new DB row + tmux session started, resumed) | whenever `CommitImport` succeeds | `PersistInstance` + `StartSession(resumeId)` | `EntRepository` / `Instance.Start` |
| `ManagedInstanceCreationFailed` (start or persist failed) | whenever `CommitImport` fails after partial work | `RollbackFailedImport` | Import orchestrator |
| `ImportedSessionConfirmedHealthy` (new instance's linked UUID matches source) | whenever `ManagedInstanceCreated` fires | `OfferKillConfirmation` | `HistoryLinker` → UI |
| `KillConfirmed` (user explicitly confirms kill, no undo) | whenever user confirms | `KillExternalProcess(pid \| tmuxSession)` | User → `KillExternalSession` / new by-PID kill primitive |
| `ExternalProcessKilled` | whenever kill succeeds | `MarkImportComplete` | Import orchestrator |
| `ExternalProcessKillFailed` | whenever kill fails after `ManagedInstanceCreated` | `SurfaceDualLiveProcessWarning` | Import orchestrator → User (non-silent, actionable) |
| `BatchImportItemFailed` (one item in a batch fails) | whenever any single item's `CommitImport` or kill fails | `ContinueRemainingBatchItems` (never roll back siblings) | Import orchestrator |
