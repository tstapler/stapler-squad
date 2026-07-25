# Implementation Plan: backlog-triage-autonomous

**Feature**: Replace broken AutonomousDriver triage path with proven headless pool triage
**Date**: 2026-06-22
**Status**: Ready for implementation
**ADRs**: ADR-022-headless-triage-over-autonomous-driver.md

---

## Creative Pass: Approaches Considered

Three approaches were evaluated before committing to this plan.

### Approach A — Fix AutonomousDriver in-place (band-aid)
Set `InitialPrompt = triagePrompt` in `CreateDirectorySession` call, extend the startup timeout
in `StartAutonomousDriverWithTimeout` to 15 minutes, and add a `TriageCompletionSignaler`
parallel to `ReviewCompletionSignaler` so `submit_triage_result` stops the driver.

**Strength**: Smallest diff; leaves the tmux session visible in the UI for operator inspection.
**Weakness**: Still depends on all three fragile pieces: idle-detection timing, headlessPool
availability, and orchestrator LLM making correct NEXT_MESSAGE decisions for a 15-minute
parallel-subagent run. Requires raising the per-turn timeout far above the 5-minute limit,
which is a global change that risks all autonomous sessions.

### Approach B — Headless pool triage (no tmux session) — CHOSEN
Model the triage call exactly like `spawnReviewGate`: call `pool.CallBlockingWithOptions`
with `WorkDir = item.RepoPath`, a JSON-output prompt, and a bounded semaphore in
`BacklogService`. No tmux session. No AutonomousDriver. No idle-detection. `TriggerTriage`
returns a synthetic `ItemSession` immediately; the goroutine drives the real work.

**Strength**: Eliminates all three failure modes simultaneously. Uses a pattern already
proven in production (`spawnReviewGate`). Single failure mode: LLM error or timeout.
**Weakness**: No tmux pane to inspect mid-triage. Operator cannot watch research happen.
UI visibility is deferred to a follow-up enhancement.

### Approach C — Hybrid: headless for research subagents, tmux for final synthesis
Run research via headless pool (4 parallel `CallBlocking` calls), then spawn a tmux
session only for synthesis + `submit_triage_result`.

**Strength**: Gives operator visibility into the synthesis step.
**Weakness**: Significantly more complex; still requires the broken AutonomousDriver for
the second phase; introduces a handoff race between research completion and session spawn.
Not worth the complexity at this stage.

**Decision**: Approach B is chosen. It converges cleanly with existing proven patterns and
eliminates all known failure modes. Tmux visibility can be added later as a UI enhancement
once the core flow is reliable.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `BacklogItem` | A unit of planned work stored in the ent ORM with title, description, AC, status. | Identified by UUID string. |
| `BacklogStatus` | Typed string enum: `idea`, `in_progress`, `review`, `done`, `ready`. | Triage promotes `idea` → `ready`. |
| `ItemSession` | DB record linking a `BacklogItem` to a session UUID with a role (triage/work/review). | Row in `item_sessions` table. |
| `SessionRole` | The function a session plays: `triage`, `work`, or `review`. | Constant strings in `session/` package. |
| `TriggerTriage` | RPC handler that initiates autonomous triage for a backlog item. | Currently spawns tmux + AutonomousDriver. |
| `HeadlessTriageCall` | A single `pool.CallBlockingWithOptions` call that runs the full triage prompt in a subprocess. | Analogous to headless review gate. |
| `HeadlessTriageResult` | Parsed output from the headless triage call: summary, suggestions, tasks, planArtifactPath. | JSON struct analogous to `verdictResult`. |
| `FeatureKeyTriage` | Headless pool feature key constant `"triage"` that identifies triage LLM sessions. | Added to `AllowedFeatureKeys`. |
| `BuildHeadlessTriagePrompt` | Function that constructs the JSON-output triage prompt from a `BacklogItemData`. | Analogous to `BuildHeadlessReviewPrompt`. |
| `ParseHeadlessTriageResult` | Function that unmarshals the LLM JSON response into `HeadlessTriageResult`. | Analogous to `ParseHeadlessVerdictResult`. |
| `headlessTriageSystemPrompt` | Stable system prompt instructing the LLM to perform triage and output JSON. | Prefix-cached across calls. |
| `triageSem` | Bounded semaphore (channel of `struct{}`) capping concurrent headless triage calls at 8. | Lives in `BacklogService`. |
| `TriageCompletionEvent` | Event published on `EventBus` when triage completes (success or failure). | Used for UI notification and status transition. |
| `AutonomousDriverStarter` | Interface satisfied by `SessionService` for starting AutonomousDriver on a session. | Retained; not changed by this plan. |
| `oneShot` | Flag on `Instance` that causes `session_driver` to inject the prompt via `-p`. | Was used as fallback; no longer used for triage. |
| `artifactAbsPath` | Absolute filesystem path to `docs/tasks/<slug>/` where triage output files are written. | Computed from `item.RepoPath + "docs/tasks/" + slug`. |
| `slug` | URL-safe kebab-case identifier derived from the item title. | Used in dir names, tmux session names, and ItemSession links. |
| `HeadlessTriageRecord` | Synthetic `ItemSession` row created immediately when triage is triggered. | UUID prefixed `"headless-triage-"`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Triage execution | Transaction Script (headless pool goroutine) | PoEAA §9 | Domain Model (AutonomousDriver loop) | Triage is a single-step LLM call with one I/O boundary; Domain Model adds orchestration complexity with no benefit |
| Concurrency cap | Semaphore (buffered channel cap 8) | Go concurrency idioms | Worker pool | Semaphore is already the pattern in `spawnReviewGate`; no queue needed; goroutines are cheap |
| Result parsing | Value Object (`HeadlessTriageResult` struct) | DDD / type-driven design | ad-hoc string parsing | Prevents schema drift between prompt and consumer; makes `ParseHeadlessTriageResult` testable independently |
| Status transition | Preconditioned DB update (optimistic lock on `expectedStatus`) | Existing `TransitionBacklogItemStatus` | In-memory state machine | DB is the source of truth; concurrent re-triggers must not double-transition |
| Prompt construction | Pure function `BuildHeadlessTriagePrompt` | PoEAA §11 (Gateway) | Embedded string in handler | Isolates prompt from handler; enables independent unit testing; mirrors review gate |
| Feature key | Typed string newtype `FeatureKey` | type-driven design | raw string literal | Already the project convention; compiler rejects typos |
| Driver stop | `TriageCompletionSignaler` interface (mirroring `ReviewCompletionSignaler`) | GoF Interface Segregation | Calling `StopDriverForSession` directly | Keeps MCP tools decoupled from `SessionService`; mirrors existing pattern |
| UI progress | `TriageLoadingIndicator` (existing component, already wired to `BacklogItemDetail`) | Existing frontend | New spinner component | `TriageLoadingIndicator` already handles in-progress state; only needs the start/stop event wiring |

---

## Observability Plan

- **Logs**: structured `log.Info`/`log.Warn`/`log.Error` at:
  - `[TriggerTriage] headless triage started` (item, slug, artifactAbsPath)
  - `[TriggerTriage] headless triage complete` (item, outcome, elapsed)
  - `[TriggerTriage] headless triage failed` (item, error)
  - `[TriggerTriage] status transition idea→ready` (item, ok/err)
  - `[TriggerTriage] submit_triage_result saved` (item, suggestions, tasks)
- **Metrics**: none added in this iteration (headless pool already tracks cost via `CostUSD`)
- **Alerts**: UI push notification (existing `EventBus`) on both success and failure

## Risk Control

- **Feature flag**: not gated — the previous tmux+AutonomousDriver path is removed, not toggled
- **Rollback procedure**: `git revert` of the PR; all changes are in `BacklogService.TriggerTriage` and two new functions in `session/headless/features.go`
- **Staged rollout**: full rollout on merge; concurrent call cap (8) limits blast radius

## Unresolved Questions

- **LLM model for headless triage**: the headless pool uses its `DefaultModel` unless overridden. Triage calls are expensive (parallel subagents). Consider whether `opus-4-5` should be the model override in `CallOptions`. Leave as pool default for now; operator can adjust `DefaultModel` in config.
- **Subagent parallelism in headless mode**: the triage prompt instructs Claude to spawn 4 parallel subagents. In headless `-p` mode, Claude Code can still dispatch subagents; they inherit the working directory but may need the MCP server for `submit_triage_result`. Verify the MCP URL is passed via `--mcp-server-url` in the headless pool's argv. If it is not, `submit_triage_result` must be removed from the prompt and replaced with JSON output parsing.
- **`submit_triage_result` in headless mode**: the current `submit_triage_result` MCP tool requires `STAPLER_SESSION_UUID`. Headless pool calls do not inject this env var. Two options: (a) parse JSON output from the LLM instead of calling the tool (matches the review gate pattern exactly), or (b) inject a synthetic UUID env var into the headless call. Option (a) is chosen in this plan (matches proven pattern).

---

## Dependency Visualization

```
Phase 1 (Plumbing — types, prompt, parser)
├── Task 1.1.1a  Add FeatureKeyTriage constant (NOT in AllowedFeatureKeys)
├── Task 1.1.1b  Add headlessTriageSystemPrompt + HeadlessTriageSystemPrompt()
├── Task 1.1.2a  Create session/backlog_triage.go: shared types + BuildHeadlessTriagePrompt
├── Task 1.1.2b  Update tools_backlog.go + backlog_service.go to use shared types
└── Task 1.1.2c  ParseHeadlessTriageResult (with fence-stripping + task cap)
│
Phase 2 (TriggerTriage rewrite)  [depends on Phase 1]
├── Task 2.1.0a  Add headlessPool + shutdownCtx + SetHeadlessPool + Shutdown() to BacklogService
├── Task 2.1.0b  Wire pool in dependencies.go + update guard in TriggerTriage
├── Task 2.1.0c  Define HeadlessPoolClient interface; use it as headlessPool field type
├── Task 2.1.0d  Fix CreateBacklogItem auto-triage guard (sessionCreator → headlessPool)
├── Task 2.1.1a  Add triageSem (cap 8) to BacklogService
├── Task 2.1.1b  Create synthetic ItemSession synchronously in TriggerTriage
├── Task 2.1.1c  Goroutine body (shutdownCtx select + 30-min timeout + headless call + persist + transition)
└── Task 2.1.2a  Remove useAutonomous, KillTmuxSessionByTitle, CreateDirectorySession from TriggerTriage
│
Phase 3 (MCP cleanup)  [independent of Phase 2]
└── Task 3.1.1a  Audit headlessTriageSystemPrompt: no submit_triage_result reference
│
Phase 4 (Tests)  [depends on Phase 1 + 2]
├── Task 4.1.1a  Unit test ParseHeadlessTriageResult (valid, fenced, invalid, cap)
├── Task 4.1.1b  Unit test BuildHeadlessTriagePrompt (contains ID, path, schema; no submit_triage_result)
├── Task 4.1.2a  Integration test TriggerTriage success path (mock pool, poll status → ready)
├── Task 4.1.2b  Integration test TriggerTriage failure path (mock pool error, item stays idea)
└── Task 4.1.2c  Integration test Shutdown cancellation (semaphore full, Shutdown unblocks)
│
Phase 5 (UI wiring)  [independent]
├── Task 5.1.1a  Wire TriageLoadingIndicator compact into BacklogItemCard list row
└── Task 5.1.2a  Replace bare error div with InlineError + retry button in BacklogItemDetail
```

---

## Phase 1: Headless Triage Plumbing

### Epic 1.1: Headless prompt + parser functions
**Goal**: Add the two pure functions and constants that `TriggerTriage` will call. These have
no dependencies on handler state and can be written and tested in isolation before touching
any handler code.

#### Story 1.1.1: Add FeatureKeyTriage and headlessTriageSystemPrompt
**As a** `BacklogService`, **I want** a stable feature key and system prompt for headless
triage calls, **so that** the headless pool can cache the system prompt prefix and
triage calls are isolated from other feature keys.

**Acceptance Criteria**:
- `FeatureKeyTriage` constant is declared in `session/headless/features.go`. It is NOT added to `AllowedFeatureKeys` — triage is an internal server-initiated call, not an external caller endpoint (same pattern as `FeatureKeyAutonomousFix`).
- `headlessTriageSystemPrompt` constant instructs the LLM to output a JSON object matching `HeadlessTriageResult`.
  - *Given* the `headlessTriageSystemPrompt` constant, *When* inspected, *Then* it contains the JSON schema `{"summary":"...","suggestions":[...],"tasks":[...]}` and does NOT mention `submit_triage_result`.

**Files**:
- `session/headless/features.go`

##### Task 1.1.1a: Add FeatureKeyTriage constant (~2 min)
- In `session/headless/features.go`, add `FeatureKeyTriage FeatureKey = "triage"` alongside the other constants.
- Do NOT add it to `AllowedFeatureKeys` (internal-only key, not externally callable).
- Files: `session/headless/features.go`

##### Task 1.1.1b: Add headlessTriageSystemPrompt constant and accessor (~4 min)
- In `session/headless/features.go`, add a `headlessTriageSystemPrompt` constant (unexported, like `headlessReviewSystemPrompt`).
- The prompt must instruct the LLM to: perform pre-implementation triage, write research/*.md files, write plan.md + validation.md, then output ONLY a JSON object — no other text — matching:
  ```json
  {"summary":"...","suggestions":[{"text":"...","rationale":"..."}],"tasks":[{"text":"...","estimate":"...","category":"..."}]}
  ```
- Add exported accessor `HeadlessTriageSystemPrompt() string` (mirrors `HeadlessReviewSystemPrompt`).
- Files: `session/headless/features.go`

---

#### Story 1.1.2: HeadlessTriageResult, BuildHeadlessTriagePrompt, ParseHeadlessTriageResult

**PLACEMENT NOTE**: These types and functions go in `session/backlog_triage.go` (NOT in `session/headless/`). The `session/headless` package is imported by `session/` — placing `BuildHeadlessTriagePrompt` in `headless/` would create a circular import. This mirrors how `BuildReviewPrompt` and `ParseHeadlessVerdictResult` live in `session/backlog_review.go`, not in `session/headless/`.

**As a** `BacklogService`, **I want** a prompt builder and a result parser for headless
triage, **so that** the handler can call the headless pool and convert its output into
typed Go structs without coupling the JSON schema to the handler.

**Acceptance Criteria**:
- `BuildHeadlessTriagePrompt(item *BacklogItemData, artifactAbsPath, slug string) string` returns a non-empty prompt that includes `item.ID`, `item.Title`, `artifactAbsPath`, and instructions to output JSON.
  - *Given* a `BacklogItemData` with `ID="abc-123"`, `Title="Add dark mode"`, *When* `BuildHeadlessTriagePrompt` is called with `artifactAbsPath="/repo/docs/tasks/add-dark-mode"`, *Then* the returned string contains `"abc-123"` and `/repo/docs/tasks/add-dark-mode`.
- `ParseHeadlessTriageResult(raw string) (HeadlessTriageResult, error)` correctly parses valid JSON and returns a descriptive error for invalid JSON.
  - *Given* `raw = '{"summary":"looks good","suggestions":[],"tasks":[]}'`, *When* `ParseHeadlessTriageResult(raw)` is called, *Then* result.Summary equals `"looks good"` and no error is returned.
- `ParseHeadlessTriageResult` strips markdown code fences before parsing (LLM often wraps JSON in triple-backtick blocks).
  - *Given* `raw = "` + "```json\n{\"summary\":\"ok\"}\n```" + `"`, *When* parsed, *Then* result.Summary equals `"ok"`.

**Files**:
- `session/backlog_triage.go` (new file — mirrors `session/backlog_review.go`)

##### Task 1.1.2a: Define shared triage types and HeadlessTriageResult (~3 min)
- Create `session/backlog_triage.go` with:
  ```go
  // TriageSuggestion and TriageTask are canonical shared types used by
  // headless triage parsing and the submit_triage_result MCP tool.
  type TriageSuggestion struct {
      Text      string `json:"text"`
      Rationale string `json:"rationale"`
  }
  type TriageTask struct {
      Text     string `json:"text"`
      Estimate string `json:"estimate"`
      Category string `json:"category"`
  }
  type HeadlessTriageResult struct {
      Summary     string            `json:"summary"`
      Suggestions []TriageSuggestion `json:"suggestions"`
      Tasks       []TriageTask       `json:"tasks"`
  }
  ```
- Update `server/mcp/tools_backlog.go` to import and use `session.TriageSuggestion` / `session.TriageTask` instead of its local copies. Update `server/services/backlog_service.go` similarly for `triageSuggestionJSON` / `triageTaskJSON`.
- Files: `session/backlog_triage.go`, `server/mcp/tools_backlog.go`, `server/services/backlog_service.go`

##### Task 1.1.2b: Write BuildHeadlessTriagePrompt (~4 min)
- Add `BuildHeadlessTriagePrompt(item *BacklogItemData, artifactAbsPath, slug string) string` to `session/backlog_triage.go`.
- Pure function — embeds item title, ID, description, AC text, and `artifactAbsPath` into a structured prompt ending with the JSON output instruction. Does NOT instruct Claude to call `submit_triage_result`.
- Files: `session/backlog_triage.go`

##### Task 1.1.2c: Write ParseHeadlessTriageResult (~3 min)
- Add `ParseHeadlessTriageResult(raw string) (HeadlessTriageResult, error)` to `session/backlog_triage.go`.
- Strip markdown fences using same approach as `ParseHeadlessVerdictResult` in `session/backlog_review.go`.
- `json.Unmarshal` into `HeadlessTriageResult`; return descriptive error if it fails.
- Cap tasks at 12 (matching `submit_triage_result` behavior).
- Files: `session/backlog_triage.go`

---

## Phase 2: TriggerTriage Handler Rewrite

### Epic 2.1: Replace tmux+AutonomousDriver path with headless pool call
**Goal**: `TriggerTriage` returns immediately after creating an `ItemSession`; a bounded
goroutine drives the full triage flow headlessly and fires a completion event.

#### Story 2.1.0: Wire headlessPool into BacklogService
**As a** `BacklogService`, **I want** access to the headless pool, **so that** `TriggerTriage` can call it without going through the session service.

**Acceptance Criteria**:
- `BacklogService` has a `headlessPool *headless.Pool` field.
- `SetHeadlessPool(pool *headless.Pool)` setter is added to `BacklogService`.
- `dependencies.go` calls `backlogSvc.SetHeadlessPool(headlessPool)` after the headless pool is initialized (after line 444 in `server/dependencies.go`).
- `TriggerTriage` returns `CodeUnimplemented` if `s.headlessPool == nil` (analogous to the `sessionCreator == nil` guard already present).
  - *Given* `BacklogService` with no headless pool set, *When* `TriggerTriage` is called, *Then* RPC returns `connect.CodeUnimplemented` with message "headless pool not available".
- `BacklogService` also gets a `shutdownCtx context.Context` and `shutdownCancel context.CancelFunc` initialized in `NewBacklogService`, and a `Shutdown()` method that calls `shutdownCancel()`.
  - *Given* `BacklogService.Shutdown()` is called, *When* a triage goroutine is blocked waiting on `triageSem`, *Then* the goroutine unblocks and exits via `shutdownCtx.Done()`.

**Files**:
- `server/services/backlog_service.go`
- `server/dependencies.go`

##### Task 2.1.0a: Add headlessPool field + SetHeadlessPool + shutdownCtx (~4 min)
- Add to `BacklogService` struct: `headlessPool *headless.Pool`, `shutdownCtx context.Context`, `shutdownCancel context.CancelFunc`.
- In `NewBacklogService`, initialize: `ctx, cancel := context.WithCancel(context.Background())`, assign to fields.
- Add `SetHeadlessPool(pool *headless.Pool)` setter.
- Add `Shutdown()` method calling `s.shutdownCancel()`.
- Files: `server/services/backlog_service.go`

##### Task 2.1.0b: Wire pool in dependencies.go and guard in TriggerTriage (~3 min)
- In `server/dependencies.go`, after headlessPool is initialized (line ~444), add `backlogSvc.SetHeadlessPool(headlessPool)`.
- In `TriggerTriage`, replace the `s.sessionCreator == nil` guard with two guards: one for `s.headlessPool == nil` (returns CodeUnimplemented) and one for `s.storage == nil` (already present).
- Files: `server/dependencies.go`, `server/services/backlog_service.go`

##### Task 2.1.0c: Define HeadlessPoolClient interface; back headlessPool field with it (~3 min)
- Define `HeadlessPoolClient` interface in `session/headless/` (or `server/services/`) with a single method matching `Pool.CallBlockingWithOptions`:
  ```go
  type HeadlessPoolClient interface {
      CallBlockingWithOptions(ctx context.Context, key FeatureKey, systemPrompt, userPrompt string, opts CallOptions) (string, error)
  }
  ```
- Verify `*Pool` satisfies `HeadlessPoolClient` (compile-time check: `var _ HeadlessPoolClient = (*Pool)(nil)`).
- Change `BacklogService.headlessPool` field type from `*headless.Pool` to `headless.HeadlessPoolClient` (or the interface's qualified name).
- Update `SetHeadlessPool` parameter type accordingly.
- This allows tests to inject a `struct{ fn func(...) (string, error) }` stub without needing `FakeRunner` WorkDir support.
- Files: `session/headless/pool.go` or new `session/headless/client.go`, `server/services/backlog_service.go`

##### Task 2.1.0d: Fix CreateBacklogItem auto-triage guard (~2 min)
- In `server/services/backlog_service.go`, locate `CreateBacklogItem` (around line 413).
- Find the guard that checks `s.sessionCreator != nil` to decide whether to auto-trigger triage.
- Replace with `s.headlessPool != nil` (or a `s.canTriage()` helper that checks the pool).
- Rationale: after the rewrite, `sessionCreator` is no longer involved in triage; a headless-only environment has `headlessPool != nil` but may have `sessionCreator == nil`, causing auto-triage to silently never fire.
- Files: `server/services/backlog_service.go`

---

#### Story 2.1.1: Add triageSem and headless triage goroutine
**As a** `BacklogService`, **I want** a semaphore-bounded headless triage goroutine,
**so that** up to 8 triage calls can run concurrently without unbounded goroutine fan-out.

**Acceptance Criteria**:
- `BacklogService` has a `triageSem chan struct{}` field initialized with capacity 8.
- `TriggerTriage` no longer calls `s.sessionCreator.CreateDirectorySession` or `s.autonomousStarter.StartAutonomousDriverWithTimeout`.
- `TriggerTriage` creates a synthetic `ItemSession` with `SessionRole = SessionRoleTriage` and UUID prefixed `"headless-triage-"` **synchronously before spawning the goroutine** (prevents TOCTOU race where concurrent re-trigger passes orphan check before the row is written).
- The goroutine acquires `triageSem` via a select that also watches `shutdownCtx.Done()`, so server shutdown unblocks waiting goroutines.
  - *Given* `BacklogService.triageSem` is a channel of capacity 8, *When* 9 concurrent `TriggerTriage` calls are made, *Then* exactly 8 goroutines enter the headless pool call and the 9th blocks on the semaphore select until one finishes.
- `TriggerTriage` returns `TriggerTriageResponse{ItemSession: itemSessionToProto(is)}` synchronously; the headless work is asynchronous.
  - *Given* a valid item in `idea` status, *When* `TriggerTriage` is called, *Then* the RPC returns within 2 seconds (before the LLM finishes).

**Files**:
- `server/services/backlog_service.go`

##### Task 2.1.1a: Add triageSem field and initialize in BacklogService constructor (~2 min)
- Add `triageSem chan struct{}` to `BacklogService` struct.
- In `NewBacklogService`, initialize: `triageSem: make(chan struct{}, 8)`.
- Files: `server/services/backlog_service.go`

##### Task 2.1.1b: Create synthetic ItemSession synchronously before goroutine spawn (~4 min)
- In `TriggerTriage`, after the precondition checks and `BuildHeadlessTriagePrompt` call:
  1. Generate a synthetic UUID: `triageSessionUUID := "headless-triage-" + uuid.New().String()`
  2. Call `s.storage.CreateItemSession` with `SessionRole = session.SessionRoleTriage`, `SessionUUID = triageSessionUUID`.
  3. Immediately after step 2, spawn the goroutine (Task 2.1.1c) with `go func(is *ItemSession) {...}(is)`.
  4. Return `TriggerTriageResponse{ItemSession: itemSessionToProto(is)}` synchronously.
- Files: `server/services/backlog_service.go`

##### Task 2.1.1c: Implement headless triage goroutine body (~5 min)
- Spawn a goroutine that:
  1. Acquires `triageSem` via `select { case s.triageSem <- struct{}{}: case <-s.shutdownCtx.Done(): return }`, defers release.
  2. Creates a context derived from `s.shutdownCtx` with 30-minute timeout: `ctx, cancel := context.WithTimeout(s.shutdownCtx, 30*time.Minute); defer cancel()`.
  3. Calls `s.headlessPool.CallBlockingWithOptions(ctx, headless.FeatureKeyTriage, headless.HeadlessTriageSystemPrompt(), prompt, headless.CallOptions{WorkDir: item.RepoPath})`.
  4. On error: logs `[TriggerTriage] headless triage failed`, calls `s.storage.UpdateItemSessionEnded(ctx, is.ID, now)`, publishes failure `NotificationEvent`, returns.
  5. On success: calls `session.ParseHeadlessTriageResult(raw)`.
  6. Persists result: calls `s.storage.UpdateItemSessionTriageResult(ctx, is.ID, payloadJSON)`.
  7. If `result.PlanArtifactsPath` non-empty: calls `s.storage.UpdateBacklogItemPlanArtifacts(ctx, itemID, pap)`.
  8. Transitions status: calls `s.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusReady, &BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusIdea)})`.
  9. Calls `s.storage.UpdateItemSessionEnded(ctx, is.ID, now)`.
  10. Publishes success `NotificationEvent` ("Triage complete").
- Files: `server/services/backlog_service.go`

---

#### Story 2.1.2: Remove dead AutonomousDriver triage path
**As a** developer, **I want** the old tmux+AutonomousDriver triage code removed, **so that**
there is one clear code path and no dead code creates confusion.

**Acceptance Criteria**:
- `TriggerTriage` no longer references `s.sessionCreator`, `s.autonomousStarter`, or `CreateDirectorySession` for the triage flow.
- The `autonomousStarter != nil` branch and the `useAutonomous` variable are removed from `TriggerTriage`.
- `KillTmuxSessionByTitle` call is removed (no tmux session is created for headless triage).
  - *Given* the old `TriggerTriage` code, *When* the rewrite is complete, *Then* `git grep -n "useAutonomous\|KillTmuxSessionByTitle\|oneShot.*triage\|CreateDirectorySession.*triage"` returns no hits in `backlog_service.go`.

**Files**:
- `server/services/backlog_service.go`

##### Task 2.1.2a: Delete AutonomousDriver and tmux session code from TriggerTriage (~3 min)
- Remove: steps 4.5 (`KillTmuxSessionByTitle`), step 8 (tmux session + AutonomousDriver spawn) from `TriggerTriage`.
- Remove: `useAutonomous`, `inst`, and all references to `s.autonomousStarter` within `TriggerTriage`.
- Keep: slug, artifactRelPath, artifactAbsPath computation (still needed for artifact dir + prompt).
- Keep: `os.MkdirAll(artifactAbsPath)` (still needed for headless Claude to write files).
- Files: `server/services/backlog_service.go`

---

## Phase 3: MCP Tool Cleanup

### Epic 3.1: TriageCompletionSignaler — not needed; verify and document
**Goal**: Confirm `submit_triage_result` does not need a `TriageCompletionSignaler` after
the headless rewrite, and that the handler remains correct.

#### Story 3.1.1: Verify submit_triage_result works headlessly and document
**As a** developer, **I want** to confirm `submit_triage_result` is NOT called in headless
mode (JSON output replaces it), **so that** the MCP tool's `STAPLER_SESSION_UUID` requirement
does not break headless triage.

**Acceptance Criteria**:
- The headless triage prompt in `headlessTriageSystemPrompt` does NOT instruct Claude to call `submit_triage_result`. It instructs Claude to output JSON only.
- The headless triage prompt instructs Claude to write artifact files to `artifactAbsPath` (still needed — LLM writes files directly during the `-p` call).
- `submit_triage_result` remains intact for any future tmux-based triage sessions; no changes to `tools_backlog.go`.
  - *Given* `headlessTriageSystemPrompt` constant, *When* inspected, *Then* it contains no mention of `submit_triage_result`.

**Files**:
- `session/headless/features.go` (prompt content only)

##### Task 3.1.1a: Audit headlessTriageSystemPrompt for MCP tool references (~2 min)
- Read the `headlessTriageSystemPrompt` written in Task 1.1.1b.
- Confirm it does not instruct Claude to call `submit_triage_result`. If it does, remove that instruction — the result is parsed from JSON output, not from the MCP call.
- Confirm it includes the JSON output schema so Claude knows what format to produce.
- Files: `session/headless/features.go`

---

## Phase 4: Tests

### Epic 4.1: Unit and integration tests
**Goal**: Every new pure function has a unit test; TriggerTriage goroutine path has an
integration test using `FakeRunner`.

#### Story 4.1.1: Unit tests for headless triage functions
**As a** developer, **I want** unit tests for `ParseHeadlessTriageResult` and
`BuildHeadlessTriagePrompt`, **so that** prompt/parser regressions are caught without
spinning up a real LLM.

**Acceptance Criteria**:
- `ParseHeadlessTriageResult` test cases: valid JSON, markdown-fenced JSON, invalid JSON, task cap at 12.
  - *Given* raw JSON `{"summary":"S","suggestions":[{"text":"T","rationale":"R"}],"tasks":[]}`, *When* `ParseHeadlessTriageResult` is called, *Then* result.Summary equals `"S"` and result.Suggestions has length 1.
- `BuildHeadlessTriagePrompt` test cases: output contains item.ID, artifactAbsPath, and the JSON schema marker.
  - *Given* item with `ID="id-999"` and `artifactAbsPath="/r/docs/tasks/foo"`, *When* `BuildHeadlessTriagePrompt` is called, *Then* returned string contains `"id-999"` and `"/r/docs/tasks/foo"`.

**Files**:
- `session/headless/features_test.go`

##### Task 4.1.1a: Write ParseHeadlessTriageResult unit tests (~4 min)
- Add test cases for: valid JSON, triple-backtick fenced JSON, totally invalid input, task list longer than 12 entries (verify cap).
- Files: `session/backlog_triage_test.go`

##### Task 4.1.1b: Write BuildHeadlessTriagePrompt unit tests (~3 min)
- Assert prompt contains item.ID, item.Title, artifactAbsPath.
- Assert prompt contains the JSON output schema string.
- Assert prompt does NOT contain "submit_triage_result".
- Files: `session/backlog_triage_test.go`

---

#### Story 4.1.2: Integration test for TriggerTriage headless path
**As a** developer, **I want** an integration test that exercises the full
`TriggerTriage` → headless pool → persist + transition path, **so that** the goroutine
body is covered and future regressions are caught.

**Acceptance Criteria**:
- A test uses `headless.FakeRunner` (already exists in `session/headless/fake_runner.go`) to simulate the LLM returning a valid JSON triage result.
- The test verifies that after `TriggerTriage` returns and the goroutine completes: the `ItemSession` has a non-empty `TriageResult`, and the `BacklogItem` status transitioned to `ready`.
  - *Given* a `BacklogService` wired with `FakeRunner` that returns `{"summary":"ok","suggestions":[],"tasks":[]}`, *When* `TriggerTriage` is called for an `idea` item, *Then* after waiting for goroutine completion (poll storage), item status is `ready` and itemSession.TriageResult is non-empty.
- A test verifies that when the `FakeRunner` returns an error, the item stays at `idea` and a failure notification is published.
  - *Given* a `BacklogService` wired with `FakeRunner` that returns an error, *When* `TriggerTriage` is called, *Then* item status remains `idea`.

**Files**:
- `server/services/backlog_service_test.go` (or nearest existing test file)

##### Task 4.1.2a: Write integration test for TriggerTriage success path (~5 min)
- Use the `HeadlessPoolClient` interface introduced in Task 2.1.0c. Create a `fakeHeadlessPool` stub in the test file:
  ```go
  type fakeHeadlessPool struct{ result string; err error }
  func (f *fakeHeadlessPool) CallBlockingWithOptions(_ context.Context, _ headless.FeatureKey, _, _ string, _ headless.CallOptions) (string, error) {
      return f.result, f.err
  }
  ```
  This avoids `FakeRunner` WorkDir incompatibility entirely.
- Wire into a minimal `BacklogService` with in-memory `Storage`, using `svc.SetHeadlessPool(&fakeHeadlessPool{result: `{"summary":"ok","suggestions":[],"tasks":[]}`})`.
- Call `TriggerTriage`, poll storage (up to 2 seconds with a 50ms tick) for status change to `ready`.
- Assert: item status is `ready`, itemSession.TriageResult is non-empty, `UpdateItemSessionEnded` was called.
- Files: `server/services/backlog_service_test.go`

##### Task 4.1.2b: Write integration test for TriggerTriage failure path (~4 min)
- Use a mock/fake pool configured to return an error.
- Assert: item stays at `idea` after goroutine completes (poll up to 2s), `UpdateItemSessionEnded` was called.
- Files: `server/services/backlog_service_test.go`

##### Task 4.1.2c: Write integration test for shutdown cancellation (~3 min)
- Fill `triageSem` to cap (8 goroutines), call `TriggerTriage` to get a 9th blocked on the semaphore.
- Call `Shutdown()` on the service.
- Assert the 9th goroutine unblocks and the ItemSession has `ended_at` set (not stuck open).
- Files: `server/services/backlog_service_test.go`

---

## Phase 5: UI Wiring

### Epic 5.1: Triage progress and completion visibility
**Goal**: Operator sees triage in-progress state in the item list and gets a clear error
message with retry button on failure. Both fix UX gaps identified in ux.md.

#### Story 5.1.1: Wire TriageLoadingIndicator into list rows
**As an** operator, **I want** to see a triage-in-progress indicator on list rows,
**so that** I know which items are being triaged without opening the detail pane.

**Acceptance Criteria**:
- `BacklogItemCard.tsx` renders `<TriageLoadingIndicator compact />` when `item.status === "idea"` and an open triage `ItemSession` exists.
- The compact indicator is visible in the board/table view without requiring pane expansion.
  - *Given* item with status `"idea"` and an open triage ItemSession with `ended_at: null`, *When* the list renders, *Then* a triage spinner is visible on the row.

**Files**:
- `web-app/src/components/backlog/BacklogItemCard.tsx`

##### Task 5.1.1a: Add triage-in-progress prop to BacklogItemCard (~4 min)
- Add `hasActiveTriage?: boolean` prop to `BacklogItemCard`.
- When `hasActiveTriage && item.status === "idea"`, render `<TriageLoadingIndicator compact />` (import from existing component).
- Wire `hasActiveTriage` from the parent page by checking if any `ItemSession` for the item has `role === "triage"` and `ended_at === null`.
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx`, `web-app/src/app/backlog/page.tsx`

#### Story 5.1.2: Fix failure state in BacklogItemDetail
**As an** operator, **I want** a retry button and clear error message when triage fails,
**so that** I can re-trigger without manually navigating.

**Acceptance Criteria**:
- When the triage session has `ended_at` set and `triage_result` is empty, `BacklogItemDetail.tsx` renders `<InlineError>` with a "Retry ↺" button.
- Clicking "Retry ↺" calls `TriggerTriage` and updates the UI optimistically.
  - *Given* a `BacklogItem` in `idea` status with an `ItemSession` that has `ended_at` non-null and empty `triage_result`, *When* `BacklogItemDetail` renders, *Then* an `InlineError` with a "Retry ↺" button is visible.

**Files**:
- `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 5.1.2a: Replace bare div alert with InlineError + retry button (~4 min)
- In `BacklogItemDetail.tsx`, find the `<div role="alert">` for triage failure.
- Replace with `<InlineError type="triage-failed" onRetry={handleRetriggerTriage} />`.
- Implement `handleRetriggerTriage` to call the `TriggerTriage` RPC and invalidate item query.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

## Validation Checklist

Before marking this plan "implemented":

- [ ] `make build && make test` passes
- [ ] `go test ./server/services/... -run TestTriggerTriage` green
- [ ] `go test ./session/headless/... -run TestParse` green
- [ ] Manual test: trigger triage on a real `idea` item, verify item transitions to `ready` within 15 minutes
- [ ] Manual test: item `plan_artifacts_path` is set after triage completes
- [ ] Manual test: triage failure (disconnect headless pool) shows retry button in UI
- [ ] `make lint` passes (no new warnings)
