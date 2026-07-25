# Feature Research: Triage Autonomous Migration

## 1. SpawnSessionFromItem — Autonomous Mode Flow

**Location:** `server/services/backlog_service.go:858–978`

`SpawnSessionFromItem` is a ConnectRPC handler (`+api: backlog:spawn-session`) that:

1. Loads the backlog item and validates it is in `ready` status.
2. Enforces the planning gate: `SkipPlanning || PlanApproved` must be true.
3. Requires `item.RepoPath` to be set.
4. Builds an agent prompt via `session.BuildTokenBudgetedPrompt`.
5. Calls `s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, prompt, []string{"backlog:work"}, false, false)` — **oneShot=false, hidden=false** for work sessions.
6. **Autonomous path (lines 940–942):**

```go
if req.Msg.Autonomous && s.autonomousStarter != nil {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

7. Creates the `ItemSession` DB record with `SessionRole = SessionRoleWork`.
8. Writes slash commands and a context file to the worktree under `worktreeMu`.
9. Transitions item to `in_progress`.

### AutonomousDriverStarter Interface

```go
type AutonomousDriverStarter interface {
    StartAutonomousDriverForInstance(inst *session.Instance)
}
```

Satisfied by `*SessionService` (`server/services/session_service.go:671`).

### StartAutonomousDriverForInstance Call Chain

`session_service.go:673–703`:

```go
func (s *SessionService) StartAutonomousDriverForInstance(inst *session.Instance) {
    driver := session.NewAutonomousDriver(inst, s.headlessPool, inst.Prompt, 0)
    driver.RegisterCompletionCallback(s.onAutonomousDriverComplete)
    driver.RegisterTurnCallback(func(turn, maxTurns int, prompt string) { ... })
    driver.Start(s.driverCtx())
    s.registerDriver(inst.Title, driver)
}
```

- Requires `s.headlessPool` — if nil, logs a warning and returns early.
- Uses `inst.Prompt` (the full backlog work prompt) as the autonomous driver prompt.
- Publishes `NewSessionUpdatedEvent` on each turn, incrementing `AutonomousTurn`/`AutonomousMaxTurns`.

**Wiring:** `BacklogService.SetAutonomousDriverStarter(starter)` is called from `server.go` after both services are constructed (`server/dependencies.go`).

---

## 2. Hidden Sessions

### What `Hidden` Means

`Instance.Hidden bool` (defined in `session/instance.go:203`) with doc comment:

> Hidden excludes this session from the default session list and review queue. Set true for system/background sessions (triage, validation) that should not appear in the user-facing session viewer.

Stored in the DB via `InstanceData.Hidden` (`session/storage.go:111`, `json:"hidden,omitempty"`).

### Where `Hidden` Is Set

**In `InstanceOptions`** (`session/instance.go:448`):
```go
// Hidden excludes the session from the default session list and review queue.
Hidden bool
```

Passed through in `NewInstance` (`instance.go:557`):
```go
Hidden: opts.Hidden,
```

**In `CreateDirectorySession`** (`session_service.go:577–619`), the `hidden` parameter is passed directly to `InstanceOptions.Hidden`. Call sites:

| Caller | hidden value | oneShot | Tags |
|---|---|---|---|
| `SpawnSessionFromItem` (work session) | `false` | `false` | `["backlog:work"]` |
| `TriggerTriage` | `true` | `true` | `["backlog:triage"]` |
| `TriggerReReview` | `true` | `true` | `["backlog:review"]` |

### How ListSessions Filters Hidden Sessions

`session_service.go:797–800`:
```go
// Exclude hidden (system/background) sessions unless explicitly requested
if inst.Hidden && !req.Msg.IncludeHidden {
    continue
}
```

Same pattern for external sessions (`session_service.go:837–840`). Controlled by the `ListSessionsRequest.IncludeHidden` proto field.

### How ReviewQueuePoller Handles Hidden Sessions

`session/review_queue_poller.go:590–596`:
```go
// shouldSkipSession returns true for sessions the poller should not evaluate.
// Hidden (system/background) sessions are never shown in the review queue.
func (rqp *ReviewQueuePoller) shouldSkipSession(inst *Instance) bool {
    return inst.Hidden || inst.Status == Stopped || inst.Paused() || !inst.Started()
}
```

Hidden sessions are excluded from all review queue evaluation — they never trigger attention or appear in the review queue UI.

### WatchSessions and Hidden Sessions — Current Gap

**`WatchSessions` does NOT filter hidden sessions.** Current implementation (`session_service.go:1620–1703`):

- **Initial snapshot** (lines 1643–1669): iterates `reviewQueuePoller.GetInstances()` and sends them with only `CategoryFilter` and `StatusFilter` applied — **no `Hidden` filter**.
- **Real-time events** (lines 1673–1703): applies only `CategoryFilter` and `StatusFilter` — **no `Hidden` filter**.

This means hidden triage/review sessions will appear in the WatchSessions stream that drives the frontend session viewer, unless the frontend explicitly filters them. `ListSessions` correctly excludes them (opt-in via `IncludeHidden`), but `WatchSessions` does not.

---

## 3. BacklogLifecycleListener

**Location:** `session/backlog_lifecycle.go`

### What It Is

A component that drives backlog item state transitions in response to session lifecycle events (`EventStarted`, `EventExited`). It is registered on each session `Instance` via `WireToInstance`.

### Events and Handlers

**`EventStarted` → `onSessionStarted(sessionUUID string)`** (line 143):
- Looks up the `ItemSession` by `sessionUUID` via storage.
- Calls `storage.UpdateItemSessionStarted(ctx, is.ID.String(), time.Now())` to record `started_at`.
- **All session roles** (triage, work, review).

**`EventExited` → `onSessionExited(sessionUUID string)`** (line 159):
- Calls `storage.UpdateItemSessionEnded(ctx, is.ID.String(), now)` for **all session roles**.
- For `SessionRoleWork` only, drives the in_progress → review/done transition:
  - If `item.SkipReviewGate` → transitions to `BacklogStatusDone`.
  - Otherwise → transitions to `BacklogStatusReview`.
  - Transition uses an optimistic-lock precondition (`ExpectedStatus=in_progress`, `ExpectedUpdatedAt`).
- If transitioning to `review` and a review mechanism is configured → spawns `spawnReviewGate`.

### Interaction with Sessions

`BacklogLifecycleListener` is wired via:
1. `CreateDirectorySession` in `session_service.go:616–618`:
   ```go
   if s.backlogLifecycleListener != nil {
       s.backlogLifecycleListener.WireToInstance(instance)
   }
   ```
2. `WireToInstance(inst)` registers an `instanceBacklogListener` shim on the `Instance`.

The listener is **non-blocking**: `OnLifecycleEvent` dispatches all work to goroutines.

### Review Gate (`spawnReviewGate`)

Called when a work session exits and item transitions to `BacklogStatusReview`:

1. **Security check:** `RunPreGateSecurityCheck(diff)` — blocks if secrets detected, records a FAIL verdict.
2. **Headless path** (preferred): calls `headless.Pool.CallBlocking` with a structured JSON prompt, parses the LLM result, creates an `ItemSession` + `ReviewVerdict` atomically via `CreateItemSessionWithVerdict`.
3. **Legacy tmux path**: calls `sessionCreator.SpawnReviewSession` to create a one-shot review session.

Concurrency is bounded by `reviewSem` (capacity: `maxConcurrentReviewGates = 8`).

### Triage and Review Session Roles

`onSessionExited` explicitly exits early for non-work sessions:
```go
if is.SessionRole != SessionRoleWork {
    return
}
```

So triage (`SessionRoleTriage`) and review (`SessionRoleReview`) sessions only get their `ended_at` recorded — they do not trigger any item status transition. The status transition on triage complete is driven by the `submit_triage_result` MCP tool (via `storage.TransitionBacklogItemStatus` called from the triage flow, indirectly triggered when the agent reads the prompt and calls the tool).

---

## 4. session_driver.go — `isOneShot()` Tag-Based No-Retry Logic

**Location:** `session/session_driver.go:488–493`

```go
// isOneShot returns true for sessions that should NOT be auto-retried.
// Triage and review sessions run exactly once; retrying them could corrupt
// backlog state by re-triggering lifecycle transitions.
func isOneShot(inst *Instance) bool {
    return inst.HasTag("backlog:triage") || inst.HasTag("backlog:review")
}
```

### Where This Is Used (in the driver loop)

**Before initial prompt is sent:**
```go
if isOneShot(inst) || retried.Load() {
    return
}
```
Exit before first prompt → no retry for triage/review sessions.

**After initial prompt was sent, session Stopped:**
```go
if inst.OneShot {
    tryExtractClaudeSessionID(inst)
}
if isOneShot(inst) || retried.Load() {
    // One-shot sessions: BacklogLifecycleListener handles this; driver exits cleanly.
    return
}
```
Clean exit; no retry.

**After minimum runtime:**
```go
if time.Since(initialPromptSentAt) > driverMinRuntimeBeforeRetry {
    // ... treat as completion
    if inst.OneShot {
        tryExtractClaudeSessionID(inst)
    }
    return
}
```

### Relationship Between `inst.OneShot` and `isOneShot()`

- `inst.OneShot` is set in `InstanceOptions` — it runs claude in `-p` (print) mode, exiting after task completion.
- `isOneShot(inst)` is a tag-based predicate. Both triage and review sessions have `inst.OneShot = true` AND carry the `backlog:triage` / `backlog:review` tags. The tag check acts as an additional semantic guard against retry.

The driver explicitly hands off to `BacklogLifecycleListener` for triage/review exit handling:
```
// One-shot sessions: BacklogLifecycleListener handles this; driver exits cleanly.
```

---

## 5. TriggerTriage and TriggerReReview — Full Implementation

### TriggerTriage (`backlog_service.go:1075–1201`)

**Purpose:** Kick off a one-shot planning session for a backlog item.

**Preconditions:**
- Item in `idea` or `ready` status.
- `item.RepoPath` must be non-empty.

**Orphan-aware re-trigger guard (lines 1111–1131):**
- Iterates existing ItemSessions with `role=triage` and `ended_at=nil`.
- Orphan conditions: `started_at=nil` OR session not live in memory OR item advanced past `idea`.
- Orphaned sessions are tombstoned (`UpdateItemSessionEnded`) and the old tmux session is stopped.
- A genuinely running triage session returns `CodeAlreadyExists`.

**Re-trigger on `ready` item (lines 1135–1140):**
- Moves item back to `idea` status so UI reflects ongoing evaluation.

**Artifact path:** `filepath.Join(item.RepoPath, "docs/tasks/<slug>")` — absolute path.

**Stale tmux cleanup (lines 1155–1159):**
```go
s.sessionStopper.KillTmuxSessionByTitle(ctx, "triage:"+slug)
```
Kills any stale tmux session with the deterministic name to force a clean session (so `--append-system-prompt` is injected fresh).

**Session creation:**
```go
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, true, true)
//                                       oneShot=true, hidden=true
```

**ItemSession record:** `SessionRole = SessionRoleTriage`.

**Triage prompt (`buildTriagePrompt`):**
1. Provides item ID, title, description, acceptance criteria.
2. Instructs parallel subagent research: `stack.md`, `features.md`, `architecture.md`, `pitfalls.md` → all written to `<artifactAbsPath>/research/`.
3. Instructs synthesis: `plan.md` and `validation.md` in `<artifactAbsPath>`.
4. Instructs calling `submit_triage_result` MCP tool with `item_id`, `plan_artifact_path`, `summary`, `suggestions`, `tasks` (max 12).
5. Allows up to 3 clarifying questions via `suggestions` with `rationale="question"`.

### TriggerReReview (`backlog_service.go:1396–1555`)

**Purpose:** Re-run the review gate for an item already in `review` status.

**Preconditions:**
- Item must be in `review` status.
- `item.RepoPath` must be non-empty.

**Steps:**
1. Loads most recent `review` and `work` ItemSessions for the item.
2. Gets git diff from work session's `LastCommitSha` (falls back to `HEAD~1`).
3. Deserializes AC snapshot from work session or item.
4. Builds re-review prompt including: prior verdict summary (if any), AC criteria, git diff.
5. **Session creation:**
   ```go
   inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
       []string{"backlog:review"}, true, true)
   //                                       oneShot=true, hidden=true
   ```
6. Creates `ItemSession` with `SessionRole = SessionRoleReview`.

**Re-review prompt instructs the agent to:**
- Assess each AC criterion against the diff.
- Call `submit_review_verdict` with `item_id`, `summary`, and per-criterion verdicts (`criterion_index`, `outcome` PASS/FAIL/PARTIAL, `evidence`).
- Not modify code.

**Degraded mode (no SessionCreator):** Returns a placeholder `ItemSession` with `SessionRole="re-review-triggered"` so the caller knows the request was acknowledged.

---

## 6. `submit_triage_result` MCP Tool

**Location:** `server/mcp/tools_backlog.go:402–536`

### Registration

```go
mcpgo.NewTool("submit_triage_result",
    mcpgo.WithDescription("Record completed triage analysis for a backlog item. Role: triage only. Call this LAST — after all research/*.md, plan.md, and validation.md files are written. ..."),
    // Parameters: item_id (required), suggestions (array), tasks (array, max 12), plan_artifact_path, summary (required)
)
```

### Parameters

| Parameter | Type | Required | Description |
|---|---|---|---|
| `item_id` | string | yes | UUID of the backlog item |
| `summary` | string | yes | Executive triage summary |
| `suggestions` | array | no | `[{text, rationale}]` — AC gaps, questions |
| `tasks` | array | no | `[{text, estimate, category}]` — impl checklist, capped at 12 |
| `plan_artifact_path` | string | no | Absolute path to `docs/tasks/<slug>` dir |

### What It Does

1. **Validates caller:** `callerSessionUUID` from context — requires `STAPLER_SESSION_UUID` env var.
2. **Validates item_id UUID format.**
3. **Links caller to item:** `storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)` — must find a link.
4. **Role check:** `itemSession.SessionRole != "triage"` → `ErrPermissionDenied`.
5. **Parses suggestions** and **tasks** (capped at 12).
6. **Serializes triage result** to JSON (`triageResultPayload`):
   ```json
   {"summary":"...","suggestions":[...],"tasks":[...]}
   ```
7. **Updates `plan_artifacts_path`** on BacklogItem via `storage.UpdateBacklogItem` (if provided).
8. **Persists triage result JSON** on ItemSession via `storage.UpdateItemSessionTriageResult`.
9. **Publishes notification** via EventBus (`NOTIFICATION_TYPE_INPUT_REQUIRED`, `PRIORITY_MEDIUM`):
   - Title: `"Triage complete"`
   - Message: `"<item title> — <N> suggestion(s). Click to review."`
   - Metadata: `{"item_id": "<uuid>"}`

### What It Does NOT Do

- Does **not** transition the backlog item status. The item remains in whatever state it was in (typically `idea` during triage). Status transitions (`idea` → `ready`) are a separate operator action.
- Does **not** approve or reject the plan. Approval is via the `ApprovePlan` RPC after operator review.

### JSON Schema (shared with backlog_service.go)

`triageResultJSON` struct in `backlog_service.go:242–261` must stay in sync with `triageResultPayload` in `tools_backlog.go`. Both use the same field names for JSON round-tripping. `itemSessionToProto` deserializes the stored JSON back into `sessionv1.TriageResult` proto.

---

## 7. WatchSessions — Current Implementation

**Location:** `server/services/session_service.go:1620–1703`

```go
func (s *SessionService) WatchSessions(
    ctx context.Context,
    req *connect.Request[sessionv1.WatchSessionsRequest],
    stream *connect.ServerStream[sessionv1.SessionEvent],
) error {
    // Subscribe before snapshot to avoid race between subscribe and snapshot phases.
    eventCh, subID := s.eventBus.Subscribe(ctx)
    defer s.eventBus.Unsubscribe(subID)
```

### Reconnecting client path (`req.Msg.AfterSeq > 0`)

Replays all events from the EventBus since `AfterSeq`:
```go
for _, event := range s.eventBus.EventsSince(req.Msg.AfterSeq) {
    stream.Send(convertEventToProto(event))
}
```

### Fresh connection path

Sends initial snapshot from poller:
```go
instances = s.reviewQueuePoller.GetInstances()
for _, inst := range instances {
    // Filters applied:
    if req.Msg.CategoryFilter != nil && *req.Msg.CategoryFilter != "" {
        if inst.Category != *req.Msg.CategoryFilter { continue }
    }
    if req.Msg.StatusFilter != nil && *req.Msg.StatusFilter != ... {
        if adapters.StatusToProto(inst.Status) != *req.Msg.StatusFilter { continue }
    }
    // NO Hidden filter here
    stream.Send(createInitialSnapshotEvent(inst))
}
```

### Real-time streaming loop

```go
for {
    select {
    case <-ctx.Done(): return nil
    case event, ok := <-eventCh:
        // Filters applied to real-time events:
        if req.Msg.CategoryFilter != nil && ... { continue }
        if req.Msg.StatusFilter != nil && ... { continue }
        // NO Hidden filter here
        protoEvent := convertEventToProto(event)
        stream.Send(protoEvent)
    }
}
```

### Summary of Filters

| Filter | ListSessions | WatchSessions (snapshot) | WatchSessions (real-time) |
|---|---|---|---|
| `hidden` | ✅ Excluded by default, `IncludeHidden` to opt-in | ❌ Not filtered | ❌ Not filtered |
| `archived` | ✅ Excluded by default, `IncludeArchived` to opt-in | ❌ Not filtered | ❌ Not filtered |
| `category` | ✅ `Category` filter | ✅ `CategoryFilter` | ✅ `CategoryFilter` |
| `status` | ✅ `Status` filter | ✅ `StatusFilter` | ✅ `StatusFilter` |
| `workflow_id` | ✅ `WorkflowId` filter | ❌ Not filtered | ❌ Not filtered |

### Proto request fields

`WatchSessionsRequest` currently has:
- `after_seq` (uint64) — for reconnection replay
- `category_filter` (optional string)
- `status_filter` (optional SessionStatus)

`WatchSessionsRequest` does **not** have `include_hidden`, `include_archived`, or `workflow_id` fields.

### Events published to the EventBus

Events that can appear in the WatchSessions stream (from `pkg/events/`):
- `NewSessionCreatedEvent(instance)` — published in `CreateDirectorySession` and `CreateSession`
- `NewSessionUpdatedEvent(instance, changedFields)` — published on status changes, autonomous turns, etc.
- `NewSessionDeletedEvent(sessionID)` — published on delete

Hidden sessions (triage, review) are added to the poller via `reviewQueuePoller.AddInstance(instance)` and events are published to the EventBus — they will appear in WatchSessions streams.
