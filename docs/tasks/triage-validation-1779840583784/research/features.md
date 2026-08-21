# Triage Pipeline — Feature Behavior Reference

This document maps the complete triage pipeline as it exists in the codebase, covering every step from the TriggerTriage RPC through to result storage and retrieval. All file:line references are current as of the research date (2026-06-18).

---

## 1. Full Flow: TriggerTriage RPC → Session Creation → Claude Runs Prompt → submit_triage_result → Result Stored

### Step 1: TriggerTriage RPC (`server/services/backlog_service.go:1065`)

Entry point: `BacklogService.TriggerTriage(ctx, req)`.

**Precondition guards (in order):**
1. Storage nil check (line 1069)
2. Item loaded via `storage.GetBacklogItem` (line 1074)
3. Status guard: item must be `idea` or `ready` (line 1083)
4. `repo_path` must be non-empty (line 1090)
5. Per-item TOCTOU lock: `triggerInProgress.LoadOrStore(itemID)` (line 1097). Returns `CodeAlreadyExists` if a concurrent trigger is already in flight for the same item.
6. Orphan-aware guard: iterates existing open triage ItemSessions (line 1110) and tombstones each if orphaned; returns `CodeAlreadyExists` only if a session is genuinely live.
7. Re-trigger path: if item is `ready`, transitions back to `idea` (line 1139).

**Session title:** `"triage:" + slugify(item.Title)` (line 1182)

**Artifact directory created:** `<repo_path>/docs/tasks/<slug>` (lines 1148–1169)

**Stale tmux session killed:** `KillTmuxSessionByTitle("triage:<slug>")` (line 1160) — ensures a fresh tmux session so prompt injection fires correctly on the new process.

**Session spawn:**
```go
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, !useAutonomous /*oneShot*/, true /*hidden*/)
```
(line 1184)

- `oneShot = true` when AutonomousDriver is NOT available (pure `-p` mode)
- `oneShot = false` when AutonomousDriver IS available (interactive + driver)
- `hidden = true` always (not shown in default UI list)

**AutonomousDriver start (when wired):**
```go
s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)
```
(line 1190) — startup timeout of 5 minutes.

**ItemSession record created** with `SessionRole = "triage"` (line 1194).

### Step 2: Session Creation — Prompt Injection Path

`CreateDirectorySession` in `server/services/session_service.go:588` builds `InstanceOptions`:
```go
opts := session.InstanceOptions{
    ...
    Prompt: prompt,   // triage prompt → positional CLI arg
    OneShot: oneShot,
    MCPServerURL: s.mcpServerURL,
    ...
}
```
(line 597)

**CRITICAL: prompt goes into `Prompt` (positional CLI arg `-p`), NOT `AppendSystemPrompt`.**

`buildClaudeCommand` in `session/instance_tmux.go:88` assembles the final command:
```go
if i.OneShot {
    parts = append(parts, "-p", "--output-format", "json")
}
if i.Prompt != "" && (claudeSessionID == "" || i.OneShot) {
    parts = append(parts, fmt.Sprintf("%q", i.Prompt))
}
```
(lines 108–113)

Result: `claude --dangerously-skip-permissions --mcp-config '...' -p --output-format json "<triage prompt>"`

`AppendSystemPrompt` is a separate Instance field (`session/instance.go:227`) that generates `--append-system-prompt <text>`. It is NOT used by the triage pipeline. Using it instead of `Prompt` was a historical bug (prompt delivered as system context only; Claude then idled waiting for a user message).

### Step 3: Claude Runs the Injected Prompt

Claude receives the triage prompt as its `-p` positional argument. It must:
1. Spawn 4 parallel subagents writing `research/{stack,features,architecture,pitfalls}.md`
2. Synthesize `plan.md` and `validation.md` under `<artifactAbsPath>/`
3. Call `submit_triage_result` MCP tool

The MCP server URL is injected via `--mcp-config` (line 93 of `instance_tmux.go`), and the session UUID is injected as the `X-Stapler-Session-UUID` HTTP header. This is how Claude authenticates its MCP calls back to the server.

### Step 4: submit_triage_result MCP Tool Called

See Section 4 for full handler spec. On success the handler:
1. Persists `plan_artifacts_path` on the `BacklogItem` (line 518 of `tools_backlog.go`)
2. Persists the triage JSON payload on the `ItemSession.triage_result` column (line 524)
3. Publishes a `NOTIFICATION_TYPE_INPUT_REQUIRED` event on the EventBus (line 531)

### Step 5: Result Stored

`UpdateItemSessionTriageResult` in `session/storage_backlog.go:227` writes the JSON blob to `item_session.triage_result`.

---

## 2. Prompt Injected Into Claude

The full prompt is built by `buildTriagePrompt(item, artifactAbsPath, slug)` at `server/services/backlog_service.go:1213`.

**Prompt structure:**

```
You are a senior software architect performing pre-implementation triage.

# Backlog Item: <title>

item_id (pass this as item_id to submit_triage_result): <uuid>

## Description
<item.Description>

## Acceptance Criteria
<numbered list from ParseAcCriteria>

## Your Task

### Step 1 — Research (run 4 subagents in parallel)
Each subagent writes one file:
- <artifactAbsPath>/research/stack.md
- <artifactAbsPath>/research/features.md
- <artifactAbsPath>/research/architecture.md
- <artifactAbsPath>/research/pitfalls.md

### Step 2 — Synthesis
Write <artifactAbsPath>/plan.md ...

### Step 3 — Validation
Write <artifactAbsPath>/validation.md ...

### Step 4 — Submit
After all files are written, call the submit_triage_result MCP tool with:
- item_id: <uuid>
- plan_artifact_path: "<artifactAbsPath>"
- summary: ...
- suggestions: [...]
- tasks: [...] (max 12)

### Step 5 — Clarifying Questions (optional)
If ambiguous, include up to 3 questions as suggestions with rationale="question".
Do not pause or wait for user input.

Do not modify any source code. Only write planning documents.
```

**Key details:**
- `item_id` is explicitly included in the prompt text (line 1218) so Claude can pass it to the MCP tool.
- `plan_artifact_path` is the **absolute** path (line 1260) so `os.Stat` can verify it in `ApprovePlan`.
- Research files are written to `<artifactAbsPath>/research/` (line 1233).
- Claude is instructed NOT to modify source code.

---

## 3. Structured Output Claude Is Expected To Produce

### Canonical types (`session/backlog.go:163–183`)

```go
type TriageSuggestion struct {
    Text      string `json:"text"`
    Rationale string `json:"rationale"`
}

type TriageTask struct {
    Text     string `json:"text"`
    Estimate string `json:"estimate"`
    Category string `json:"category"` // one of: backend|frontend|test|infra|docs
}

type TriageResultPayload struct {
    Summary             string             `json:"summary"`
    Suggestions         []TriageSuggestion `json:"suggestions"`
    ClarifyingQuestions []string           `json:"clarifying_questions,omitempty"`
    Tasks               []TriageTask       `json:"tasks,omitempty"`
}
```

**Fields:**
- `summary` — required, 2–3 sentence executive summary
- `suggestions` — optional array, each needs `text` + `rationale`. Suggestions with `rationale="question"` are treated as clarifying questions by convention.
- `tasks` — optional array, capped at 12 in the MCP handler (`tools_backlog.go:489`), each needs `text`, `estimate`, `category`
- `clarifying_questions` — legacy field in the struct; the current pattern uses suggestions with `rationale="question"` instead

**Proto representation** (`backlog_service.go:238–244`):
```go
p.TriageResult = &sessionv1.TriageResult{
    Summary:             tr.Summary,
    Suggestions:         suggs,
    ClarifyingQuestions: tr.ClarifyingQuestions,
    Tasks:               tasks,
}
```

---

## 4. MCP submit_triage_result Tool — What It Accepts

Handler: `backlogHandlers.submitTriageResult` at `server/mcp/tools_backlog.go:423`.

### Tool registration (`tools_backlog.go:640–678`)

Parameters:
| Parameter | Type | Required | Description |
|---|---|---|---|
| `item_id` | string (UUID) | Yes | Backlog item UUID |
| `summary` | string | Yes | Executive summary |
| `suggestions` | array | No | `[{text, rationale}]` |
| `tasks` | array | No | `[{text, estimate, category}]`, max 12 |
| `plan_artifact_path` | string | No | Absolute path to artifact dir |

### Handler validation (lines 423–527)

1. **Caller UUID required**: reads `STAPLER_SESSION_UUID` from context (`callerSessionUUID`, line 424). Returns `PERMISSION_DENIED` if absent.
2. **item_id required + UUID format**: validated with `validateUUID` (line 431).
3. **summary required**: returns `ErrInvalidArgument` if empty.
4. **Session-item link verified**: `GetItemSessionBySessionAndItem(callerUUID, itemID)` (line 445). Returns `PERMISSION_DENIED` if not linked.
5. **Role check**: `itemSession.SessionRole` must be `"triage"` (line 452). Returns `PERMISSION_DENIED` for any other role.
6. **Suggestions parsed**: iterated from `[]interface{}`, each JSON-marshaled into `session.TriageSuggestion` (lines 457–470).
7. **Tasks parsed**: iterated from `[]interface{}`, each JSON-marshaled into `session.TriageTask`, then capped at 12 (lines 474–494).

### Persistence (lines 497–549)

1. Build `session.TriageResultPayload` and marshal to JSON (line 498).
2. If `plan_artifact_path` provided: `UpdateBacklogItem(itemID, {PlanArtifactsPath: &pap})` (line 518).
3. Persist triage result: `UpdateItemSessionTriageResult(itemSession.ID, payloadJSON)` (line 524).
4. Publish EventBus notification: `NOTIFICATION_TYPE_INPUT_REQUIRED`, priority MEDIUM, title "Triage complete" (line 531).

### Success response (line 551)
```
"Triage result submitted for item <id>. <N> suggestion(s) recorded.\n\nSummary: <summary>"
```

---

## 5. Triage Results — Storage and Retrieval

### Storage location

- **`item_session.triage_result`** column (`session/ent/schema/item_session.go:36`): optional string field, stores JSON `TriageResultPayload`.
- **`backlog_item.plan_artifacts_path`** column (`session/ent/schema/backlog_item.go:48`): absolute path to the artifact directory written by the triage session.

### Write path

`storage.UpdateItemSessionTriageResult(ctx, id, triageResult)` at `session/storage_backlog.go:227`:
```go
r.client.ItemSession.UpdateOneID(parsedID).SetTriageResult(triageResult).Save(ctx)
```

### Read path

`GetBacklogItem` → `ListItemSessions` → `itemSessionToProto`.

`itemSessionToProto` in `backlog_service.go:225` deserializes:
```go
if is.TriageResult != "" {
    var tr session.TriageResultPayload
    json.Unmarshal([]byte(is.TriageResult), &tr)
    p.TriageResult = &sessionv1.TriageResult{...}
}
```
(lines 225–244)

`GetBacklogItem` (line 425) eagerly loads item sessions via `ListItemSessions` (line 442), which queries `WithReviewVerdict()` edges. Triage results are returned inline inside the `ItemSession` proto embedded in the `BacklogItem` response.

---

## 6. Edge Cases

### 6a. Duplicate Trigger Prevention — sync.Map

`triggerInProgress sync.Map` on `BacklogService` (`backlog_service.go:78`) prevents a TOCTOU race:

```go
if _, loaded := s.triggerInProgress.LoadOrStore(req.Msg.ItemId, struct{}{}); loaded {
    return nil, connect.NewError(connect.CodeAlreadyExists, ...)
}
defer s.triggerInProgress.Delete(req.Msg.ItemId)
```
(lines 1097–1101)

This is **in-process only**. Multi-process deployments do not get cross-process protection.

### 6b. Hung Session Detection (maxTriageDuration)

`maxTriageDuration = 2 * time.Hour` (`backlog_service.go:60`).

In the orphan-aware guard loop (lines 1115–1135):
```go
timedOut := is.StartedAt != nil && time.Since(*is.StartedAt) > maxTriageDuration
if notLive || statusAdvanced || timedOut {
    now := time.Now()
    _ = s.storage.UpdateItemSessionEnded(ctx, is.ID.String(), now)
    if s.sessionStopper != nil {
        _ = s.sessionStopper.StopSessionByUUID(ctx, is.SessionUUID)
    }
    continue
}
```

A session is tombstoned when ANY of:
- `started_at` is NULL (never confirmed running) → `neverStarted`
- Session UUID not in live in-memory poller → `notLive`
- Item status has advanced past `idea` (triage already completed) → `statusAdvanced`
- Session has been running > 2 hours → `timedOut`

Only sessions that pass all orphan checks block with `CodeAlreadyExists`.

### 6c. Stale tmux Session Kill (pre-spawn)

Before spawning, `KillTmuxSessionByTitle("triage:<slug>")` is called (line 1160). This is required because the tmux session name is deterministic and reused across retriggers. Without this kill, `TmuxSession.Start()` would reattach to the old session, silently skipping the new prompt injection.

### 6d. Session Not Wired (SessionCreator nil)

If `s.sessionCreator == nil` (test/degraded environment), TriggerTriage returns `CodeUnimplemented` (line 1174). This is the expected degradation contract for tests.

### 6e. ItemSession Role Enforcement in MCP

The MCP handler explicitly rejects non-triage sessions calling `submit_triage_result` (line 452):
```go
if itemSession.SessionRole != "triage" {
    return errResult(ErrPermissionDenied, "session role is ... — only 'triage' role may submit triage results", ...)
}
```

This prevents work or review sessions from accidentally writing triage results.

### 6f. Re-triage on Ready Item

If the item is already `ready`, TriggerTriage moves it back to `idea` (line 1139) so the UI reflects that evaluation is in progress. This is non-fatal: if the transition fails, triage still spawns.

### 6g. AutonomousDriver Startup Timeout

When AutonomousDriver is wired, it is started with `StartAutonomousDriverWithTimeout(inst, 5*time.Minute)` (line 1190). If the session does not reach idle state within 5 minutes, the driver fires `fireCompletion(Stuck=true, Reason: "startup timeout")` (`session/autonomous_driver.go:183`).

---

## Source File Index

| File | Lines of interest |
|---|---|
| `server/services/backlog_service.go` | TriggerTriage: 1065–1209; buildTriagePrompt: 1213–1284; maxTriageDuration const: 60; triggerInProgress: 78 |
| `server/mcp/tools_backlog.go` | submitTriageResult handler: 423–555; registration: 640–678 |
| `session/backlog.go` | TriageSuggestion: 165; TriageTask: 171; TriageResultPayload: 178; status constants: 12–19; SessionRole constants: 23–26 |
| `session/storage_backlog.go` | UpdateItemSessionTriageResult: 227; CreateItemSession: 42; ListItemSessions: 82; GetItemSessionBySessionAndItem: 119 |
| `session/ent/schema/item_session.go` | triage_result field: 36 |
| `session/ent/schema/backlog_item.go` | plan_artifacts_path field: 48 |
| `session/instance.go` | Prompt field: 124; AppendSystemPrompt field: 227; InstanceOptions.Prompt: 423; NewInstance Prompt copy: 538 |
| `session/instance_tmux.go` | buildClaudeCommand: 88; Prompt → `-p` positional arg: 111 |
| `server/services/session_service.go` | CreateDirectorySession: 588; opts.Prompt mapping: 597 |
| `session/autonomous_driver.go` | AutonomousDriver.Start: 110; startup timeout path: 183 |
