# Architecture Research: Integration Points

## 1. Headless Pool (`session/headless/`)

Location: `session/headless/pool.go`, `session/headless/runner.go`, `session/headless/features.go`

The Pool manages named LLM feature sessions for non-interactive AI calls:
```go
type Pool struct {
    cfg       PoolConfig
    sessions  map[FeatureKey]*sessionState
    keyMu     map[FeatureKey]*sync.Mutex
}

type FeatureKey string  // e.g., headless.FeatureKeyCustom
```

Wired into `SessionService` via:
```go
// session_service.go:
headlessPool *headless.Pool
func (s *SessionService) SetHeadlessPool(pool *headless.Pool)
```

Used in `RunOneShot` RPC (session_service.go line 2923):
```go
outputStr, callErr = s.headlessPool.CallBlockingWithOptions(runCtx, headless.FeatureKeyCustom, "", req.Msg.Prompt, headless.CallOptions{WorkDir: workDir})
```

The headless pool is the right mechanism for **background AI analysis** (e.g., interpreting CI failure output, summarizing what needs fixing) without creating a full tmux session. It is **not** the right mechanism for the interactive fix session itself.

---

## 2. Approval Handler Flow (`server/services/approval_handler.go`)

The full approval pipeline:

```
HTTP POST /api/hooks/permission-request
    │
    ├── 1. Secret scan (ScanForSecrets on Bash commands) → auto-deny
    │
    ├── 2. Domain age check (IsNewlyRegistered) → escalate
    │
    ├── 3. AskUserQuestion tool → notify + defer (no blocking)
    │
    ├── 4. Classifier.Classify(payload, context)
    │   ├── AutoAllow → writeDecision("allow") return
    │   ├── AutoDeny  → writeDecision("deny", msg) return
    │   └── Escalate  → fall through to createApproval
    │
    └── 5. createApproval:
        │   Create PendingApproval{ID, SessionID, ToolName, ExpiresAt=now+4m}
        │   Store in ApprovalStore
        │   Broadcast notification to web UI
        │   Trigger ReviewQueue check
        │
        └── BLOCK on approval.decisionCh (4m timeout)
            ├── User responds → writeDecision(behavior, message)
            ├── Timeout       → HTTP 200 empty (Claude native dialog fallback)
            └── Context done  → allow silently
```

### Auto-approval pathways for AutonomousDriver

Two mechanisms already exist to bypass the manual approval flow:

**Option A: `--permission-mode bypassPermissions` CLI flag**
```go
// instance_tmux.go line 59:
if i.PermissionMode != "" && strings.Contains(program, "claude") {
    program = fmt.Sprintf("%s --permission-mode %q", program, i.PermissionMode)
}
```
`PermissionMode: "bypassPermissions"` makes Claude Code skip its own permission dialogs — the HTTP hook is never called. Best for fully-trusted autonomous sessions.

**Option B: `--allowedTools` CLI flag**
```go
// session/instance.go line 219:
AllowedTools string // "--allowedTools Bash,Read,Edit" or pattern-style
```
Pre-approves specific tools. The hook is still called but the Classifier's `AutoAllow` rules fire.

**Option C: Classifier rules**
`ApprovalHandler.SetClassifier(c classifier.Classifier)` — inject a custom classifier that `AutoAllow`s all tool calls for sessions with `AutonomousMode == true`. This requires knowing the session ID from the HTTP header (`X-CS-Session-ID`).

---

## 3. Proto RPCs relevant to AutonomousDriver (`proto/session/v1/session.proto`)

### Existing RPCs (lines 260-316)
```protobuf
rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse) {}
rpc RunOneShot(RunOneShotRequest) returns (RunOneShotResponse) {}
rpc WriteToSession(WriteToSessionRequest) returns (WriteToSessionResponse) {}
rpc HibernateSession(HibernateSessionRequest) returns (HibernateSessionResponse) {}
rpc ResumeHibernatedSession(...) returns (...) {}
```

### CreateSessionRequest fields (lines 405-457)
```protobuf
message CreateSessionRequest {
    string title = 1;
    string path = 2;
    string program = 3;
    string prompt = 4;
    string branch = 5;
    // ...
    SessionType session_type = 13;       // DIRECTORY, NEW_WORKTREE, EXISTING_WORKTREE
    bool one_off = 14;
    string initial_prompt = 15;
    bool one_shot = 16;
    // ...
    string allowed_tools = 21;           // --allowedTools passthrough
    string permission_mode = 22;         // --permission-mode passthrough
}
```

A new RPC `TriggerAutonomousFix` could be added, or the AutonomousDriver could be activated by updating `AutonomousMode` on an existing session (needs a new `UpdateSession` field or a dedicated RPC).

### PendingApproval message (types.proto lines 935-961)
```protobuf
message PendingApproval {
    string id = 1;
    string session_id = 2;
    string tool_name = 3;
    map<string, string> tool_input = 4;
    string cwd = 5;
    string permission_mode = 6;
    // ...
}
```

---

## 4. PermissionMode in types.proto (lines 940-960)

`permission_mode` is a string field on `PendingApproval` (not an enum). Valid values per `instance.go`:
- `"default"` — normal interactive approval
- `"acceptEdits"` — accept file edits without prompting
- `"bypassPermissions"` — bypass all permission checks
- `"auto"` — auto-approve based on context

---

## 5. Instance Options + Constructor (`session/instance.go` lines 420-560)

`InstanceOptions` relevant fields for AutonomousDriver:
```go
type InstanceOptions struct {
    Title           string
    Path            string
    Program         string
    AutoYes         bool    // -y flag on claude
    SessionType     SessionType
    Prompt          string
    Tags            []string
    OneShot         bool
    Hidden          bool
    MCPServerURL    string
    AppendSystemPrompt string
    AllowedTools    string  // --allowedTools
    PermissionMode  string  // --permission-mode
    CreateIfMissing bool
    GitHubPRNumber  int     // for GitHub-session correlation
    GitHubPRURL     string
    GitHubOwner     string
    GitHubRepo      string
}
```

`NewInstance(opts)` sets all fields, auto-detects worktree info if `GitHubOwner` is empty, and initializes `ReviewState.LastMeaningfulOutput = time.Now()`.

---

## 6. LifecycleListener Interface

```go
// session/instance.go (inferred from instance_controller.go usage):
type LifecycleEvent int
const (
    EventStarted LifecycleEvent = iota
    EventExited
)

type LifecycleListener interface {
    OnLifecycleEvent(event LifecycleEvent, reason string)
}

// Register via:
func (i *Instance) RegisterLifecycleListener(l LifecycleListener)
```

`fireLifecycleEvent` copies the listeners slice before iterating (avoids holding the lock during callbacks). An `AutonomousDriver` implementing `LifecycleListener` can be registered at session-creation time to start/stop the driver goroutine.

---

## 7. Session Creation Call Chain for Autonomous Sessions

```
SpawnSessionFromItem / CreateDirectorySession
    │
    └── session.NewInstance(opts)           // creates Instance with AutonomousMode, PermissionMode, AllowedTools
    └── instance.Start(true)               // starts tmux session
    └── session.StartSessionDriver(inst)   // existing startup driver
    └── s.wireStatusChangeCallback(inst)   // wires StatusChangeListener
    └── s.storage.AddInstance(inst)        // persists
    └── s.reviewQueuePoller.AddInstance    // adds to poller
    └── s.backlogLifecycleListener.WireToInstance(inst)  // backlog callbacks
    // TODO: wire AutonomousDriver here
```

The correct insertion point for an AutonomousDriver is **after** `wireStatusChangeCallback` but **before** `AddInstance` persistence — so the driver is active before any status events could be missed.

---

## 8. StatusChangeListener Multiplexing Problem

`Instance` only stores **one** `onStatusChange func`:
```go
// instance_controller.go:
func (i *Instance) SetStatusChangeCallback(fn func(detection.DetectedStatus, string))
```

`wireStatusChangeCallback` overwrites any previously set callback. There is no multi-listener support at the `Instance` level (though `ClaudeController` itself also only stores one `statusChangeListener`).

**Solutions:**
1. Store the existing callback and chain a new one — fragile
2. Add a `RegisterStatusChangeListener(fn)` that appends to a slice — cleaner, requires a small refactor
3. Drive the AutonomousDriver from polling `cc.GetIdleState()` on a ticker — avoids the multiplexing problem entirely but adds latency

The existing `BacklogLifecycleListener` uses option 3 implicitly — it responds to `LifecycleEvent` (session started/exited), not to status changes. The AutonomousDriver likely needs status-change events (to know when Claude is done thinking), so option 2 or 3 is needed.
