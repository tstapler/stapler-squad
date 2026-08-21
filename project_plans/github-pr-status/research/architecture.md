# Architecture Research: Background PR Status Poller

## Executive Summary

Stapler Squad uses a **workspace-level shared-ticker polling architecture** for background work (review queue, history watching). The PR status poller should follow this established pattern: a single goroutine runs a shared `time.Ticker`, iterates all sessions, and polls GitHub via the existing `github.client.GetPRInfo()` wrapper. Session updates are persisted to storage and pushed to web clients through the ConnectRPC event streaming system.

---

## 1. Existing Polling Patterns (review_queue_poller.go Analysis)

### Architecture: Workspace-Level Shared Ticker

**File:** `session/review_queue_poller.go`

```go
// Single shared ticker per ReviewQueuePoller instance (one per workspace)
func (rqp *ReviewQueuePoller) pollLoop() {
	defer rqp.wg.Done()
	ticker := time.NewTicker(rqp.config.PollInterval)  // Lines 205-206
	defer ticker.Stop()
	
	for {
		select {
		case <-rqp.ctx.Done():
			return
		case <-ticker.C:
			rqp.checkSessions()  // Iterates ALL instances per tick
		}
	}
}
```

**Characteristics:**
- **Single goroutine** for the entire workspace (not per-session)
- **Shared ticker** fires every 2 seconds (configurable via `PollInterval`)
- **Iteration model:** Each tick checks ALL monitored sessions sequentially (`checkSessions()` → `checkSession()` for each instance)
- **Instance list:** Maintained in memory (`rqp.instances`), populated at startup and updated via `SetInstances()`, `AddInstance()`, `RemoveInstance()`
- **Lifecycle:** Started/stopped explicitly via `Start(ctx)` / `Stop()`

### Key Benefits of Shared-Ticker Pattern

1. **Resource efficiency:** One goroutine + one ticker for N sessions (not N goroutines/tickers)
2. **Coordinated timing:** All sessions checked at predictable intervals, prevents thundering herd
3. **Graceful shutdown:** Single cancel context propagates to entire poll loop
4. **Observable state:** Poller holds live in-memory instances; no restart-on-read side effects

### Config & Intervals

```go
// Lines 21-29
type ReviewQueuePollerConfig struct {
	PollInterval       time.Duration  // How often to check sessions (2 sec default)
	IdleThreshold      time.Duration  // Duration before considering session idle (5 sec)
	InputWaitDuration  time.Duration  // Flag if waiting for input > threshold (3 sec)
	StalenessThreshold time.Duration  // No meaningful output for threshold (2 min)
}
```

**Default:** Poll every 2 seconds. This is aggressive for GitHub API usage; PR status should use a longer interval (60-300 seconds recommended).

---

## 2. Background Worker Patterns Found

### history_watcher.go: File-Watching Pattern

**File:** `session/history_watcher.go`

Uses `fsnotify.Watcher` for event-driven file monitoring (not polling):

```go
// Lines 37-66: Watch ~/.claude/projects/ for JSONL file creation
func (w *HistoryFileWatcher) Start(ctx context.Context) error {
	// Create watcher with fsnotify
	watcher, err := fsnotify.NewWatcher()
	
	// Watch directory and subdirectories
	_ = watcher.Add(w.watchDir)
	_ = filepath.WalkDir(w.watchDir, func(path string, d os.DirEntry, ...) error {
		if d.IsDir() && path != w.watchDir {
			_ = watcher.Add(path)  // Recursive directory watching
		}
	})
	
	go w.run(ctx)  // Background goroutine listening for events
}

// Event handling (lines 93-115)
func (w *HistoryFileWatcher) handleEvent(event fsnotify.Event) {
	if event.Op&(fsnotify.Create|fsnotify.Rename) == 0 {
		return
	}
	if !strings.HasSuffix(event.Name, ".jsonl") || strings.HasPrefix(base, "agent-") {
		return  // Skip non-JSONL and agent files
	}
	w.callback(path)  // Fire callback
}
```

**Pattern Type:** Event-driven (not polling). Suitable for file system but **not for GitHub API** (requires explicit HTTP calls).

---

## 3. Server Streaming Architecture (How Updates Reach Web UI)

### Event Flow: Poller → Storage → EventBus → ConnectRPC → WebSocket → Browser

**Three layers work together:**

#### Layer 1: In-Memory State & Storage

**Files:**
- `session/review_queue_poller.go`: Holds live instances, checks state every 2 sec
- `session/storage.go`: Persists to Ent database via `UpdateInstanceProcessingGrace()`, `UpdateInstanceLastAddedToQueue()`, etc.

Key methods for PR status:
```go
// Storage provides partial-field update methods to avoid restarting sessions
func (s *Storage) UpdateInstanceLastAddedToQueue(title string, t time.Time) error
func (s *Storage) UpdateInstanceProcessingGrace(title string, t time.Time) error
func (s *Storage) updateFieldInRepo(title string, fn func(*InstanceData)) error
```

**Design principle:** Update only necessary fields; avoid full instance restart.

#### Layer 2: EventBus for Pub/Sub Broadcasting

**File:** `server/events/bus.go`

```go
// Pub/Sub pattern via Go channels
type EventBus struct {
	subscribers map[string]chan *Event  // Multiple subscribers, one channel each
	bufferSize  int                      // Default 100 events per subscriber
}

func (eb *EventBus) Publish(event *Event) {
	for _, ch := range eb.subscribers {
		select {
		case ch <- event:
			// Sent
		default:
			// Drop if subscriber is slow (non-blocking)
		}
	}
}
```

**Event types** (`server/events/types.go`):
- `EventSessionUpdated` - General session property change
- `EventSessionStatusChanged` - Status transition
- `EventNotification` - Session notification
- Custom events can be added

#### Layer 3: ConnectRPC WebSocket Streaming

**File:** `server/services/connectrpc_websocket.go`

Streams session data directly to web clients:

```go
// Lines 280-300: streamTerminal entry point
func (h *ConnectRPCWebSocketHandler) streamTerminal(stream *connectWebSocketStream) error {
	sessionID := terminalData.SessionId
	
	// Resolve session with priority:
	// 1. ReviewQueuePoller (live in-memory)
	// 2. ExternalDiscovery (mux-based external sessions)
	// 3. Storage (last resort)
	instance, _ := h.resolveSession(sessionID)
	
	// Stream terminal content via WebSocket
	// Updates flow: Poller state → storage → web UI (via streaming handler)
}
```

**Streaming modes:**
- `raw-compressed`: Terminal output delta compression
- `state`: Full state snapshots
- `hybrid`: Combined approach

---

## 4. Recommended Poller Design for PR Status

### Architecture: Single Workspace-Level Poller with Long Interval

Based on analysis, **do NOT create per-session goroutines**. Instead:

```
┌─────────────────────────────────────────────────────┐
│ PR Status Poller (workspace-level)                  │
│ • Single goroutine                                  │
│ • Shared ticker (60–300 sec interval)               │
│ • Iterates all sessions with GitHub PR metadata     │
└──────────────┬──────────────────────────────────────┘
               │
        Per tick cycle:
        1. For each session with PRNumber set:
           - Call github.GetPRInfo(owner, repo, prNumber)
           - Check PR status changed (draft, merged, closed, etc.)
           - Emit event if status changed
           - Persist PR metadata to Instance via storage.Update()
        2. Update LastPRStatusCheck timestamp
               │
               ▼
        ┌──────────────────────────┐
        │ EventBus.Publish()       │
        │ (if PR status changed)   │
        └──────────┬───────────────┘
                   │
                   ▼
        ┌──────────────────────────────┐
        │ ConnectRPC Streaming         │
        │ → Browser WebSocket clients  │
        └──────────────────────────────┘
```

### Implementation Skeleton

```go
// session/pr_status_poller.go (new file)

package session

import (
	"context"
	"sync"
	"time"
	
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

type PRStatusPollerConfig struct {
	PollInterval time.Duration  // Recommended: 60-300 seconds (not 2!)
	Timeout      time.Duration  // Per-GitHub API call timeout
}

type PRStatusPoller struct {
	storage        *Storage
	instances      []*Instance
	config         PRStatusPollerConfig
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

func NewPRStatusPoller(storage *Storage) *PRStatusPoller {
	return &PRStatusPoller{
		storage:   storage,
		instances: make([]*Instance, 0),
		config: PRStatusPollerConfig{
			PollInterval: 300 * time.Second,  // 5 minutes
			Timeout:      10 * time.Second,
		},
	}
}

func (p *PRStatusPoller) SetInstances(instances []*Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instances = instances
}

func (p *PRStatusPoller) Start(ctx context.Context) {
	p.mu.Lock()
	if p.ctx != nil {
		p.mu.Unlock()
		return  // Already running
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.mu.Unlock()
	
	p.wg.Add(1)
	go p.pollLoop()
}

func (p *PRStatusPoller) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.checkAllSessions()
		}
	}
}

func (p *PRStatusPoller) checkAllSessions() {
	p.mu.RLock()
	instances := make([]*Instance, len(p.instances))
	copy(instances, p.instances)
	p.mu.RUnlock()
	
	for _, inst := range instances {
		p.checkSession(inst)
	}
}

func (p *PRStatusPoller) checkSession(inst *Instance) {
	// Skip if no PR metadata
	if inst.GitHubPRNumber == 0 || inst.GitHubOwner == "" || inst.GitHubRepo == "" {
		return
	}
	
	// Call GitHub API (with timeout)
	ctx, cancel := context.WithTimeout(p.ctx, p.config.Timeout)
	defer cancel()
	
	prInfo, err := github.GetPRInfo(inst.GitHubOwner, inst.GitHubRepo, inst.GitHubPRNumber)
	if err != nil {
		log.ErrorLog.Printf("Failed to fetch PR status for %s/%s#%d: %v",
			inst.GitHubOwner, inst.GitHubRepo, inst.GitHubPRNumber, err)
		return
	}
	
	// Check if status changed
	oldState := inst.GitHubPRState  // New field needed in Instance
	if prInfo.State != oldState {
		log.InfoLog.Printf("PR #%d status changed: %s → %s",
			inst.GitHubPRNumber, oldState, prInfo.State)
		
		// Update instance
		inst.GitHubPRState = prInfo.State
		inst.LastPRStatusCheck = time.Now()
		
		// Persist to storage
		if err := p.storage.UpdateInstance(inst); err != nil {
			log.ErrorLog.Printf("Failed to persist PR status for %s: %v", inst.Title, err)
		}
		
		// Emit event (will be broadcast via EventBus)
		// TODO: Wire this through session service to emit events
	}
}

func (p *PRStatusPoller) Stop() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Unlock()
	p.wg.Wait()
}
```

---

## 5. Integration Points (Files to Create/Modify)

### New Files

1. **`session/pr_status_poller.go`** (sketch above)
   - Implements shared-ticker polling
   - Calls `github.GetPRInfo()` per session
   - Updates storage & emits events

2. **`server/services/pr_status_service.go`** (new RPC handler)
   - Exposes RPC methods: `GetPRStatus()`, `WatchPRStatus()` (streaming)
   - Wires poller to ReviewQueueService or SessionService

### Modified Files

1. **`session/instance.go`**
   - Add fields: `GitHubPRState`, `LastPRStatusCheck` (if not already present)
   - These already exist in `InstanceData` (`GitHubPRNumber`, `GitHubOwner`, `GitHubRepo`)

2. **`session/storage.go`**
   - Add helper: `UpdateInstancePRStatus(title, state, checkTime)` if needed
   - Use existing `updateFieldInRepo()` for partial updates

3. **`server/server.go`** (app entry point)
   - Wire `PRStatusPoller` startup/shutdown alongside `ReviewQueuePoller`
   - Call `poller.SetInstances()` when instances load
   - Call `poller.Start(ctx)` in startup

4. **`server/services/session_service.go`** or **`github_service.go`**
   - Wire `GitHubService` (or new `PRStatusService`) to expose RPC methods
   - Handle streaming via ConnectRPC

5. **`proto/session/v1/session.proto`** (protobuf definitions)
   - Add `GetPRStatusRequest`, `GetPRStatusResponse`, `WatchPRStatusRequest`, `PRStatusEvent`
   - Define which PR status fields the web UI needs

### Existing Strong Points to Leverage

- **`github.client.GetPRInfo(owner, repo, prNumber)`** - Already fetches PR metadata (state, title, updates, etc.)
- **`ReviewQueuePoller` pattern** - Proven shared-ticker architecture (reuse as template)
- **`Storage.updateFieldInRepo()`** - Designed for partial updates without instance restart
- **`EventBus`** - Ready to broadcast PR status changes
- **ConnectRPC streaming** - Web clients already subscribe to session updates

---

## 6. GitHub API Integration Points

### Existing github/client.go Methods

Already available in `github/client.go`:

```go
// Line 96-148: Fetches full PR metadata
func GetPRInfo(owner, repo string, prNumber int) (*PRInfo, error)
	// Returns: Number, Title, Body, HeadRef, BaseRef, State, Author, Labels, etc.

// Line 150-191: Fetches PR comments
func GetPRComments(owner, repo string, prNumber int) ([]PRComment, error)

// Line 193-212: Gets diff
func GetPRDiff(owner, repo string, prNumber int) (string, error)
```

**Relevant fields for poller** (from `PRInfo` struct, lines 13-30):
```go
State        string      // open, closed, merged (changed check here)
UpdatedAt    time.Time   // Use to avoid re-polling if no change
HeadRef      string      // PR branch name
BaseRef      string      // Target branch
IsDraft      bool        // Draft status
Mergeable    string      // Mergeable state
Additions    int         // File changes
Deletions    int
ChangedFiles int
```

### Rate Limiting Consideration

GitHub API limits: 5,000 requests/hour (authenticated). With N sessions:
- Poll interval: 300 sec (5 min)
- Sessions per poll: N
- Requests/hour: `(60/5) * N = 12 * N`
- Safe for: **~416 sessions** (5000/12)

**Recommendation:** Use longer intervals (300+ sec) and cache `UpdatedAt` to skip unchanged PRs.

---

## 7. Comparison: Polling Strategy Options

| Approach | Pros | Cons | Recommendation |
|----------|------|------|-----------------|
| **Per-Session Goroutines** | Individual control | N goroutines, N timers, GC overhead, uncoordinated | ❌ NO |
| **Shared Ticker (Current)** | Efficient, coordinated, single cleanup path | Must iterate all sessions per tick | ✅ **RECOMMENDED** |
| **Event-Driven (GitHub Webhooks)** | Instant updates, zero polling | External webhook URL, requires ngrok/public IP, GitHub integration setup | ⚠️ Future enhancement |
| **Reactive on User View** | Minimal API calls | High latency, no proactive updates | ❌ NO (for monitoring use case) |

---

## 8. Verification Before Code

Key questions to answer before implementation:

1. **Instance lifecycle:** Are PR-enabled sessions added/removed dynamically at runtime? (Yes → track with `AddInstance()` / `RemoveInstance()`)
2. **Event wire-up:** Does `session_service.go` subscribe to events for broadcasting? (Via `ReactiveQueueManager` pattern)
3. **Storage layer:** Can we use existing `UpdateInstance()` without full restart? (Yes, `updateFieldInRepo()` pattern)
4. **Protobuf schema:** What PR status fields does the web UI need to display? (Affects `proto/session/v1/session.proto`)

---

## Summary

**Poller Design Decision: Workspace-level shared-ticker architecture**

- **Single goroutine** per workspace (not per-session)
- **Polling interval:** 300 seconds (5 minutes), not 2 seconds like review queue
- **Per-tick work:** Iterate monitored sessions, call `github.GetPRInfo()`, emit events on status change
- **Event flow:** Poller → Storage → EventBus → ConnectRPC → Browser
- **Code organization:** `session/pr_status_poller.go` (new) + wiring in `server.go`
- **Leverage:** Existing `github.client.GetPRInfo()`, `Storage.updateFieldInRepo()`, `EventBus`

This approach is **proven by ReviewQueuePoller**, scales to hundreds of sessions, and integrates cleanly with the existing event streaming architecture.
