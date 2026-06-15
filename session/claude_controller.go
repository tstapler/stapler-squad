package session

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/ratelimit"
)

// StatusChangeListener is called when the controller detects a terminal status transition.
// Always invoked from the controller's own background goroutine, outside any lock.
type StatusChangeListener func(newStatus detection.DetectedStatus, sessionName string)

// InstanceContext is the narrow interface ClaudeController needs from its owning Instance.
// Using an interface breaks the bidirectional Instance ↔ ClaudeController dependency.
type InstanceContext interface {
	GetTitle() string
	GetPTYReader() (*os.File, error)
	Preview() (string, error)
	LastMeaningfulOutputTime() time.Time
	GetCreatedAt() time.Time
	SetLastMeaningfulOutput(t time.Time)
	GetStatus() int
	WriteToPTY(data []byte) (int, error)
}

// statusCacheEntry holds the result of the last successful status detection
// along with the FNV hash of the tail content that produced it.
type statusCacheEntry struct {
	tailHash uint64
	status   detection.DetectedStatus
	desc     string
}

// idleCacheEntry holds the result of the last successful idle state detection
// along with the FNV hash of the tail content that produced it.
type idleCacheEntry struct {
	tailHash uint64
	state    detection.IdleState
}

// statusDetectionTailBytes is the number of bytes taken from the tail of the
// terminal content before line-based detection. This bounds the scope passed to
// filterTmuxMetadata and the line splitter.
const statusDetectionTailBytes = detection.StatusDetectionTailBytes

// statusDetectionLinesWindow is the number of trailing lines examined by
// DetectWithContextFromLines. Status indicators (◇ Ready, esc to interrupt,
// Thinking…, ? for shortcuts) always appear within the last few lines of the
// terminal, so restricting the window prevents stale scrollback content — e.g.
// an "esc to interrupt" from a previous turn — from overriding a fresh idle
// prompt on the last line.
const statusDetectionLinesWindow = 15

// controllerLifecycle holds the running context for the controller.
// Protected by lifecycle; write-locked only during Start/Stop transitions.
type controllerLifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// cacheState groups the status and idle tail-hash caches.
// Protected by cache; updated by GetCurrentStatus, GetIdleState, and runStatusChangeLoop.
type cacheState struct {
	status statusCacheEntry
	idle   idleCacheEntry
}

// ClaudeController provides a high-level API for controlling Claude instances.
// It orchestrates all the underlying components (queue, executor, history, streams).
//
// Locking discipline:
//   - lifecycle (Locked[controllerLifecycle]): write-locked briefly at the boundary of
//     Start/Stop transitions. Slow cleanup in Stop() runs OUTSIDE this lock so that
//     status reads are never blocked by goroutine joins or disk I/O.
//   - Sub-components (atomic.Pointer[T]): set once in Start(), cleared in Stop().
//     Readers call .Load() — a nil result means not yet initialized. Atomic access
//     means GetCurrentStatus, GetRecentOutput, Subscribe, etc. never contend with Stop().
//   - listeners (Locked[[]StatusChangeListener]): fan-out callbacks.
//   - cache (Locked[cacheState]): tail-hash result cache for status/idle detection.
//
// Cache-line layout: lifecycle.mu (a sync.RWMutex) and the atomic.Pointer fields are
// separated by [64]byte padding so that write operations on the mutex do not invalidate
// the cache line read by atomic.Load() calls (Go issue #67764).
type ClaudeController struct {
	// Immutable after construction.
	sessionName   string
	instance      InstanceContext
	onEOFCallback func()
	statusCheckCh chan struct{}

	// lifecycle guards Start/Stop transitions. Write lock is held only for the
	// brief check-and-set at the beginning of each transition; all slow cleanup
	// in Stop() (goroutine joins, disk I/O) runs outside the lock.
	lifecycle Locked[controllerLifecycle]
	_         [64]byte // cache-line padding: prevents lifecycle.mu invalidating adjacent atomic slots (Go #67764)

	// Sub-components initialized atomically in Start(), cleared atomically in Stop().
	// Callers load via .Load(); nil means the controller is not running.
	// Using atomic.Pointer lets read-only operations (GetCurrentStatus, GetRecentOutput,
	// Subscribe, GetIdleState, …) proceed without holding any lifecycle lock, so they
	// are never blocked when Stop() runs its slow cleanup.
	ptyAccess        atomic.Pointer[PTYAccess]
	responseStream   atomic.Pointer[ResponseStream]
	rateLimitHandler atomic.Pointer[ratelimit.PTYConsumer]
	statusDetector   atomic.Pointer[detection.StatusDetector]
	idleDetector     atomic.Pointer[detection.IdleDetector]
	queue            atomic.Pointer[CommandQueue]
	executor         atomic.Pointer[CommandExecutor]
	history          atomic.Pointer[CommandHistory]

	_ [64]byte // cache-line padding: separates hot atomic slots from Locked fields below (Go #67764)

	// listeners is the fan-out set of status-change callbacks.
	listeners Locked[[]StatusChangeListener]

	// lastEmittedStatus tracks the status most recently broadcast to listeners.
	// Written only by the single runStatusChangeLoop goroutine; atomic so no lock needed.
	lastEmittedStatus atomic.Int64

	// cache holds the tail-hash and result of the last status/idle detection run.
	cache Locked[cacheState]
}

// SetOnEOFCallback registers a function called when the PTY backing this controller
// exits unexpectedly (program exit, not an explicit Stop() call).
// Must be called before Start().
func (cc *ClaudeController) SetOnEOFCallback(fn func()) {
	cc.onEOFCallback = fn
}

// AddStatusChangeListener appends fn to the fan-out set of status-change listeners.
// All registered listeners fire on every status transition.
// Safe to call before or after Start().
func (cc *ClaudeController) AddStatusChangeListener(fn StatusChangeListener) {
	cc.listeners.Write(func(ls *[]StatusChangeListener) {
		*ls = append(*ls, fn)
	})
}

// SetStatusChangeListener registers fn as the sole status-change listener, replacing any
// previously registered listeners. Kept for backward compatibility; prefer AddStatusChangeListener.
func (cc *ClaudeController) SetStatusChangeListener(fn StatusChangeListener) {
	cc.listeners.Write(func(ls *[]StatusChangeListener) {
		*ls = []StatusChangeListener{fn}
	})
}

// NewClaudeController creates a new controller for the given instance.
func NewClaudeController(instance InstanceContext) (*ClaudeController, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}

	sessionName := instance.GetTitle()
	if sessionName == "" {
		return nil, fmt.Errorf("instance title cannot be empty")
	}

	return &ClaudeController{
		sessionName:   sessionName,
		instance:      instance,
		statusCheckCh: make(chan struct{}, 1),
	}, nil
}

// Start initializes all components and begins background operations (streaming, command execution).
// This is the single entry point for starting the controller — no separate Initialize() call needed.
//
// The lifecycle write lock is held for the duration of initialization to prevent concurrent
// Start() calls. Read-only operations (GetCurrentStatus, etc.) do not use this lock and
// are therefore unblocked — they simply see nil atomic pointers until initialization completes.
func (cc *ClaudeController) Start(ctx context.Context) error {
	var startErr error
	cc.lifecycle.Write(func(l *controllerLifecycle) {
		if l.ctx != nil {
			startErr = fmt.Errorf("controller already started for session '%s'", cc.sessionName)
			return
		}

		// Get PTY reader from instance
		ptyReader, err := cc.instance.GetPTYReader()
		if err != nil {
			startErr = fmt.Errorf("failed to get PTY reader: %w", err)
			return
		}

		// Create circular buffer for PTY output
		buffer := NewCircularBuffer(256 * 1024) // 256KB: detection needs ≤4KB; scrollback owns long-term history
		pa := NewPTYAccess(cc.sessionName, ptyReader, buffer)

		// Create rate limit detection handler
		rateLimitManager := ratelimit.NewManager(cc.sessionName, cc.instance)
		rlh := ratelimit.NewPTYConsumer(pa, rateLimitManager)

		// Create response stream
		rs := NewResponseStream(cc.sessionName, pa)

		// Create status detector and tag it with the session name for detection event attribution.
		sd := detection.NewStatusDetector()
		sd.SetSessionID(cc.sessionName)

		// Create idle detector — inject sd so both components share one ring buffer.
		id := detection.NewIdleDetectorWithDetector(cc.sessionName, pa, detection.DefaultIdleDetectorConfig(), sd)

		// CRITICAL FIX: Restore idle detector state from persisted timestamps
		// This prevents false "timeout" detection after server restarts by maintaining
		// temporal continuity between historical activity and idle detection.
		//
		// We use LastMeaningfulOutput as the source of truth because:
		// 1. It excludes tmux status banners (more accurate activity signal)
		// 2. It's already used by review queue for staleness detection
		// 3. It's persisted to storage and restored on startup
		//
		// This restoration happens BEFORE the detector starts analyzing PTY output,
		// so the first DetectState() call will have accurate historical context.
		if cc.instance != nil && !cc.instance.LastMeaningfulOutputTime().IsZero() {
			id.InitializeFromTimestamp(cc.instance.LastMeaningfulOutputTime())
		}

		// MIGRATION: Handle old sessions without LastMeaningfulOutput timestamp.
		// These sessions were created before this tracking was implemented and show
		// "20412d ago" (epoch: 1970-01-01) in the review queue.
		//
		// Migration strategy (in order of preference):
		// 1. Use CreatedAt if available (best approximation of session age)
		// 2. Use time.Now() as last resort (for transient tmux-only sessions)
		//
		// This timestamp will be persisted the next time the session state is saved,
		// completing the migration.
		if cc.instance != nil && cc.instance.LastMeaningfulOutputTime().IsZero() {
			var migrationTime time.Time
			var migrationSource string

			if !cc.instance.GetCreatedAt().IsZero() {
				// Prefer CreatedAt: gives accurate age for persistent sessions
				migrationTime = cc.instance.GetCreatedAt()
				migrationSource = "CreatedAt"
			} else {
				// Fallback for transient sessions: use current time
				// Better to show "idle for 0s" than "20412d ago"
				migrationTime = time.Now()
				migrationSource = "time.Now()"
			}

			log.Info("migrating old session: initializing lastmeaningfuloutput", "session", cc.sessionName, "from", migrationSource, "time", migrationTime)
			cc.instance.SetLastMeaningfulOutput(migrationTime)
			id.InitializeFromTimestamp(migrationTime)
		}

		// Create command queue with persistence
		q, err := NewCommandQueueWithPersistence(cc.sessionName, getQueuePersistDir())
		if err != nil {
			startErr = fmt.Errorf("failed to create command queue: %w", err)
			return
		}

		// Create command history with persistence
		h, err := NewCommandHistoryWithPersistence(cc.sessionName, getHistoryPersistDir())
		if err != nil {
			startErr = fmt.Errorf("failed to create command history: %w", err)
			return
		}

		// Create command executor
		exec := NewCommandExecutor(cc.sessionName, pa, rs, sd, q)

		// Set up result callback to automatically add to history.
		// Captures h directly — safe since h is never replaced after Start().
		exec.SetResultCallback(func(result *ExecutionResult) {
			if err := h.AddFromResult(result); err != nil {
				log.Error("failed to add execution result to history", "err", err)
			}
		})

		// Set up context for lifecycle management
		innerCtx, cancel := context.WithCancel(ctx)

		// Drive idle detector from PTY events so lastActivity reflects actual output time.
		// This replaces polling-only updates: lastActivity now resets on every PTY read,
		// giving accurate idle duration even when active patterns persist in old scrollback.
		// Also notify the rate limit handler so it processes output immediately rather than
		// waiting for the 500ms polling interval.
		//
		// The closures load from atomic pointers so they always see the current handler
		// even if wired before cc.rateLimitHandler is stored.
		rs.SetOnOutput(func() {
			if detector := cc.idleDetector.Load(); detector != nil {
				detector.RecordActivity()
			}
			if handler := cc.rateLimitHandler.Load(); handler != nil {
				handler.NotifyOutput()
			}
			// Signal status-check goroutine; non-blocking drop if already pending.
			select {
			case cc.statusCheckCh <- struct{}{}:
			default:
			}
		})

		// Wire PTY-EOF callback so the owning Instance can transition state when the
		// program exits unexpectedly (not via an explicit Stop() call).
		if cc.onEOFCallback != nil {
			rs.OnEOF = cc.onEOFCallback
		}

		// Start response stream
		if err := rs.Start(innerCtx); err != nil {
			cancel()
			startErr = fmt.Errorf("failed to start response stream: %w", err)
			return
		}

		// Publish all sub-components atomically so read paths see a consistent view.
		cc.ptyAccess.Store(pa)
		cc.responseStream.Store(rs)
		cc.rateLimitHandler.Store(rlh)
		cc.statusDetector.Store(sd)
		cc.idleDetector.Store(id)
		cc.queue.Store(q)
		cc.executor.Store(exec)
		cc.history.Store(h)

		// Start status-change background goroutine (exits via ctx cancellation).
		// Always start unconditionally: for sessions loaded from the database,
		// wireStatusChangeCallback is called AFTER Start(), so the listener may be
		// nil here but will be wired later. runStatusChangeLoop handles nil listeners.
		go cc.runStatusChangeLoop(innerCtx)

		// Start command executor
		if err := exec.Start(innerCtx); err != nil {
			cancel()
			rs.Stop()
			startErr = fmt.Errorf("failed to start command executor: %w", err)
			return
		}

		// Start rate limit detection
		rlh.Start()

		l.ctx = innerCtx
		l.cancel = cancel
	})

	if startErr != nil {
		return startErr
	}

	log.Info("claude controller started", "session", cc.sessionName)
	return nil
}

// Stop stops all background operations and cleans up resources.
//
// The lifecycle write lock is held only to cancel the context and clear the lifecycle
// fields. All slow cleanup (goroutine joins via executor.Stop/responseStream.Stop, disk
// I/O via queue.Save/history.Save) runs OUTSIDE the lock, so concurrent callers of
// GetCurrentStatus, GetRecentOutput, Subscribe, etc. are never blocked.
func (cc *ClaudeController) Stop() error {
	// Phase 1: grab the cancel function and mark as stopped (brief write lock).
	var cancelFn context.CancelFunc
	cc.lifecycle.Write(func(l *controllerLifecycle) {
		if l.cancel == nil {
			return
		}
		cancelFn = l.cancel
		l.ctx = nil
		l.cancel = nil
	})
	if cancelFn == nil {
		return fmt.Errorf("controller not started")
	}
	cancelFn() // Signal all background goroutines to stop.

	// Phase 2: swap out sub-components atomically. New callers see nil immediately;
	// in-flight callers already hold local references and finish normally.
	exec := cc.executor.Swap(nil)
	rs := cc.responseStream.Swap(nil)
	rlh := cc.rateLimitHandler.Swap(nil)
	q := cc.queue.Swap(nil)
	h := cc.history.Swap(nil)
	cc.ptyAccess.Store(nil)
	cc.statusDetector.Store(nil)
	cc.idleDetector.Store(nil)

	// Phase 3: slow cleanup — outside any lock.
	var errs []error

	if exec != nil {
		if err := exec.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("executor stop error: %w", err))
		}
	}

	if rs != nil {
		if err := rs.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("response stream stop error: %w", err))
		}
	}

	if rlh != nil {
		rlh.Stop()
	}

	if q != nil {
		if err := q.Save(); err != nil {
			errs = append(errs, fmt.Errorf("queue save error: %w", err))
		}
	}

	if h != nil {
		if err := h.Save(); err != nil {
			errs = append(errs, fmt.Errorf("history save error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}

	log.Info("claude controller stopped", "session", cc.sessionName)
	return nil
}

// SendCommand sends a command to the Claude instance (queued execution).
func (cc *ClaudeController) SendCommand(text string, priority int) (string, error) {
	var running bool
	cc.lifecycle.Read(func(l controllerLifecycle) {
		running = l.ctx != nil
	})
	if !running {
		return "", fmt.Errorf("controller not started")
	}

	q := cc.queue.Load()
	if q == nil {
		return "", fmt.Errorf("controller not started")
	}

	cmd := &Command{
		ID:        generateCommandID(),
		Text:      text,
		Priority:  priority,
		Timestamp: time.Now(),
		Status:    CommandPending,
	}

	if err := q.Enqueue(cmd); err != nil {
		return "", fmt.Errorf("failed to enqueue command: %w", err)
	}

	log.Info("command queued", "session", cc.sessionName, "text", text, "id", cmd.ID, "priority", priority)

	return cmd.ID, nil
}

// SendCommandImmediate sends a command for immediate execution (bypasses queue).
func (cc *ClaudeController) SendCommandImmediate(text string) (*ExecutionResult, error) {
	var running bool
	cc.lifecycle.Read(func(l controllerLifecycle) {
		running = l.ctx != nil
	})
	if !running {
		return nil, fmt.Errorf("controller not started")
	}

	exec := cc.executor.Load()
	if exec == nil {
		return nil, fmt.Errorf("controller not started")
	}

	cmd := &Command{
		ID:        generateCommandID(),
		Text:      text,
		Priority:  100, // High priority for immediate commands
		Timestamp: time.Now(),
		Status:    CommandPending,
	}

	result, err := exec.ExecuteImmediate(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to execute immediate command: %w", err)
	}

	if h := cc.history.Load(); h != nil {
		if err := h.AddFromResult(result); err != nil {
			log.Error("failed to add immediate execution to history", "err", err)
		}
	}

	log.Info("immediate command executed", "session", cc.sessionName, "text", text, "id", cmd.ID)

	return result, nil
}

// GetCommandStatus retrieves the current status of a command.
func (cc *ClaudeController) GetCommandStatus(commandID string) (*Command, error) {
	if exec := cc.executor.Load(); exec != nil {
		if currentCmd := exec.GetCurrentCommand(); currentCmd != nil && currentCmd.ID == commandID {
			return currentCmd, nil
		}
	}

	if q := cc.queue.Load(); q != nil {
		if cmd, err := q.Get(commandID); err == nil {
			return cmd, nil
		}
	}

	if h := cc.history.Load(); h != nil {
		if entries := h.GetByCommandID(commandID); len(entries) > 0 {
			return &entries[0].Command, nil
		}
	}

	return nil, fmt.Errorf("command '%s' not found", commandID)
}

// CancelCommand cancels a pending command in the queue.
func (cc *ClaudeController) CancelCommand(commandID string) error {
	q := cc.queue.Load()
	if q == nil {
		return fmt.Errorf("queue not initialized")
	}
	return q.Cancel(commandID)
}

// GetCurrentCommand returns the currently executing command, if any.
func (cc *ClaudeController) GetCurrentCommand() *Command {
	if exec := cc.executor.Load(); exec != nil {
		return exec.GetCurrentCommand()
	}
	return nil
}

// GetQueuedCommands returns all commands currently in the queue.
func (cc *ClaudeController) GetQueuedCommands() []*Command {
	if q := cc.queue.Load(); q != nil {
		return q.List()
	}
	return nil
}

// GetCommandHistory returns recent command history.
func (cc *ClaudeController) GetCommandHistory(limit int) []*HistoryEntry {
	h := cc.history.Load()
	if h == nil {
		return nil
	}
	if limit <= 0 {
		return h.GetAll()
	}
	return h.GetRecent(limit)
}

// SearchHistory searches command history by text.
func (cc *ClaudeController) SearchHistory(query string) []*HistoryEntry {
	if h := cc.history.Load(); h != nil {
		return h.Search(query)
	}
	return nil
}

// GetHistoryStatistics returns statistics about command execution.
func (cc *ClaudeController) GetHistoryStatistics() HistoryStatistics {
	if h := cc.history.Load(); h != nil {
		return h.GetStatistics()
	}
	return HistoryStatistics{}
}

// Subscribe creates a new subscription to the response stream.
func (cc *ClaudeController) Subscribe(subscriberID string) (<-chan ResponseChunk, error) {
	rs := cc.responseStream.Load()
	if rs == nil {
		return nil, fmt.Errorf("response stream not initialized")
	}
	return rs.Subscribe(subscriberID)
}

// Unsubscribe removes a subscription from the response stream.
func (cc *ClaudeController) Unsubscribe(subscriberID string) error {
	rs := cc.responseStream.Load()
	if rs == nil {
		return fmt.Errorf("response stream not initialized")
	}
	return rs.Unsubscribe(subscriberID)
}

// GetCurrentStatus detects the current status of the Claude instance.
//
// Two optimisations are applied on every call:
//  1. Tail slicing — only the last statusDetectionTailBytes bytes of the
//     terminal content are examined. Status indicators (◇ Ready, Thinking…,
//     esc to interrupt) always appear near the current cursor position, so
//     scanning the full scrollback is unnecessary.
//  2. Content hash cache — a FNV-64a hash of the tail is compared against the
//     previous call. If the tail is unchanged the cached result is returned
//     immediately with zero allocations.
//
// This function holds no lifecycle lock — it reads ptyAccess and statusDetector
// via atomic.Pointer, and the cache via Locked[cacheState]. It therefore never
// blocks when Stop() is running its slow cleanup.
func (cc *ClaudeController) GetCurrentStatus() (detection.DetectedStatus, string) {
	pa := cc.ptyAccess.Load()
	if pa == nil {
		return detection.StatusUnknown, "PTY not initialized"
	}

	// pa.GetBuffer() uses its own p.mu — safe without any lifecycle lock.
	raw := pa.GetBuffer()
	if len(raw) == 0 {
		return detection.StatusUnknown, "No terminal content"
	}
	content := string(raw)

	tail := tailContent(content, statusDetectionTailBytes)
	h := hashString(tail)

	var hit bool
	var cachedStatus detection.DetectedStatus
	var cachedDesc string
	cc.cache.Read(func(c cacheState) {
		if h == c.status.tailHash {
			hit = true
			cachedStatus = c.status.status
			cachedDesc = c.status.desc
		}
	})
	if hit {
		return cachedStatus, cachedDesc
	}

	filtered, _ := filterTmuxMetadata(tail)

	// Line-based reverse scan: process from the most recent line backwards.
	// This ensures a fresh idle prompt on the last line ("? for shortcuts",
	// "> ") beats a stale "esc to interrupt" that is still within the window
	// from an earlier turn.
	lines := lastNLines(filtered, statusDetectionLinesWindow)

	sd := cc.statusDetector.Load()
	if sd == nil {
		return detection.StatusUnknown, "status detector not initialized"
	}
	status, desc := sd.DetectWithContextFromLines(lines)

	cc.cache.Write(func(c *cacheState) {
		c.status = statusCacheEntry{tailHash: h, status: status, desc: desc}
	})
	return status, desc
}

// filterTmuxMetadata removes common tmux UI elements from terminal output.
// This prevents false positive status detections from metadata like session names
// appearing in window titles, status bars, or shell prompts.
func filterTmuxMetadata(content string) (string, int) {
	var sb strings.Builder
	sb.Grow(len(content))
	removedCount := 0
	first := true
	remaining := content

	for len(remaining) > 0 {
		// Slice off one line at a time without allocating a []string.
		var line string
		if idx := strings.IndexByte(remaining, '\n'); idx >= 0 {
			line = remaining[:idx+1] // includes the trailing \n
			remaining = remaining[idx+1:]
		} else {
			line = remaining
			remaining = ""
		}

		// Skip lines that look like tmux status bars:
		// - "[staplersquad_session-name]" window title format
		// - Lines starting with "[" followed by timestamp or session info
		trimmed := strings.TrimRight(strings.TrimLeft(line, " \t"), " \t\r\n")
		if len(trimmed) > 0 && trimmed[0] == '[' {
			removedCount++
			continue
		}

		if !first {
			sb.WriteByte('\n')
		}
		first = false
		sb.WriteString(line)
	}

	return sb.String(), removedCount
}

// lastNLines returns the last n lines of s as a slice.
// If s has fewer than n lines, all lines are returned.
func lastNLines(s string, n int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// tailContent returns the last n bytes of s, snapped forward to the next
// newline boundary so we never split a line mid-way through.
// If s is shorter than n bytes, s itself is returned unchanged.
func tailContent(s string, n int) string {
	if len(s) <= n {
		return s
	}
	tail := s[len(s)-n:]
	// Advance to the next newline so we start on a clean line boundary.
	if nl := strings.IndexByte(tail, '\n'); nl >= 0 {
		tail = tail[nl+1:]
	}
	return tail
}

// hashString returns a fast FNV-64a hash of s.
func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// GetRecentOutput returns recent output from the PTY buffer.
// Holds no lifecycle lock; returns nil if the controller is not started.
func (cc *ClaudeController) GetRecentOutput(bytes int) []byte {
	pa := cc.ptyAccess.Load()
	if pa == nil {
		return nil
	}

	if bytes <= 0 {
		return pa.GetBuffer()
	}

	buffer := pa.GetBuffer()
	if len(buffer) <= bytes {
		return buffer
	}

	return buffer[len(buffer)-bytes:]
}

// IsStarted returns whether the controller is currently started.
func (cc *ClaudeController) IsStarted() bool {
	var started bool
	cc.lifecycle.Read(func(l controllerLifecycle) {
		started = l.ctx != nil
	})
	return started
}

// GetSessionName returns the session name for this controller.
func (cc *ClaudeController) GetSessionName() string {
	return cc.sessionName
}

// GetInstance returns the InstanceContext backing this controller.
func (cc *ClaudeController) GetInstance() InstanceContext {
	return cc.instance
}

// SetExecutionOptions updates command execution options.
func (cc *ClaudeController) SetExecutionOptions(options ExecutionOptions) {
	if exec := cc.executor.Load(); exec != nil {
		exec.SetOptions(options)
	}
}

// GetExecutionOptions returns current execution options.
func (cc *ClaudeController) GetExecutionOptions() ExecutionOptions {
	if exec := cc.executor.Load(); exec != nil {
		return exec.GetOptions()
	}
	return DefaultExecutionOptions()
}

// ClearHistory removes all command history entries.
func (cc *ClaudeController) ClearHistory() error {
	h := cc.history.Load()
	if h == nil {
		return fmt.Errorf("history not initialized")
	}
	return h.Clear()
}

// ClearQueue removes all pending commands from the queue.
func (cc *ClaudeController) ClearQueue() error {
	q := cc.queue.Load()
	if q == nil {
		return fmt.Errorf("queue not initialized")
	}
	return q.Clear()
}

// IsIdle returns whether the Claude instance is currently idle (waiting for input).
// This uses pattern-based detection on terminal content.
func (cc *ClaudeController) IsIdle() bool {
	state, _ := cc.GetIdleState()
	return state == detection.IdleStateWaiting || state == detection.IdleStateTimeout
}

// IsActive returns whether the Claude instance is actively processing commands.
func (cc *ClaudeController) IsActive() bool {
	state, _ := cc.GetIdleState()
	return state == detection.IdleStateActive
}

// GetIdleState returns the current idle state with timing information.
// Returns the state and the timestamp of last activity.
//
// Applies the same tail-slice + hash-cache optimisations as GetCurrentStatus
// so that polling the idle state on an unchanged terminal is essentially free.
//
// Holds no lifecycle lock — reads ptyAccess and idleDetector via atomic.Pointer.
// This also fixes the re-entrant RWMutex bug that existed when calling
// cc.instance.Preview() → GetRecentOutput() → cc.mu.RLock() while already
// holding cc.mu.RLock(); with atomic pointers there is no lock to re-enter.
func (cc *ClaudeController) GetIdleState() (detection.IdleState, time.Time) {
	id := cc.idleDetector.Load()
	if id == nil {
		return detection.IdleStateUnknown, time.Time{}
	}

	pa := cc.ptyAccess.Load()

	var state detection.IdleState
	if pa != nil {
		raw := pa.GetBuffer()
		if len(raw) > 0 {
			content := string(raw)
			tail := tailContent(content, statusDetectionTailBytes)
			h := hashString(tail)

			var hit bool
			cc.cache.Read(func(c cacheState) {
				if h == c.idle.tailHash {
					state = c.idle.state
					hit = true
				}
			})
			if !hit {
				filtered, _ := filterTmuxMetadata(tail)
				state = id.DetectStateFromContent(filtered)
				cc.cache.Write(func(c *cacheState) {
					c.idle = idleCacheEntry{tailHash: h, state: state}
				})
			}
		} else {
			state = id.GetState()
		}
	} else {
		// No PTY access available — return the last cached state without
		// triggering the deprecated PTY-buffer detection path.
		state = id.GetState()
	}

	lastActivity := id.GetLastActivity()
	return state, lastActivity
}

// GetIdleStateInfo returns comprehensive idle state information.
func (cc *ClaudeController) GetIdleStateInfo() detection.IdleStateInfo {
	state, lastActivity := cc.GetIdleState()

	id := cc.idleDetector.Load()
	if id == nil {
		return detection.IdleStateInfo{
			State:        detection.IdleStateUnknown,
			SessionName:  cc.sessionName,
			LastActivity: time.Now(),
		}
	}

	return detection.IdleStateInfo{
		State:           state,
		LastActivity:    lastActivity,
		IdleDuration:    time.Since(lastActivity),
		LastStateChange: id.GetStateInfo().LastStateChange,
		SessionName:     cc.sessionName,
	}
}

// GetIdleDuration returns how long the session has been idle.
func (cc *ClaudeController) GetIdleDuration() time.Duration {
	if id := cc.idleDetector.Load(); id != nil {
		return id.GetIdleDuration()
	}
	return 0
}

// GetRateLimitState returns the current rate limit detection state.
func (cc *ClaudeController) GetRateLimitState() ratelimit.RateLimitState {
	if rlh := cc.rateLimitHandler.Load(); rlh != nil {
		return rlh.GetRateLimitState()
	}
	return ratelimit.StateNone
}

// GetExitContent returns the last bytes captured before the PTY exited.
// Returns nil if the controller has no response stream or no exit content was recorded.
func (cc *ClaudeController) GetExitContent() []byte {
	if rs := cc.responseStream.Load(); rs != nil {
		return rs.GetExitTail()
	}
	return nil
}

// GetEscapeParser returns the escape code parser from the response stream.
// Returns nil if the controller is not started or has no response stream.
func (cc *ClaudeController) GetEscapeParser() *analytics.EscapeCodeParser {
	if rs := cc.responseStream.Load(); rs != nil {
		return rs.GetEscapeParser()
	}
	return nil
}

// GetTotalBytesWritten returns the monotonic PTY byte offset from the response
// stream's circular buffer. Returns 0 if the controller is not started or has
// no response stream.
func (cc *ClaudeController) GetTotalBytesWritten() int64 {
	if rs := cc.responseStream.Load(); rs != nil {
		return rs.GetTotalBytesWritten()
	}
	return 0
}

// GetRateLimitResetTime returns the reset time from the rate limit handler.
// Returns zero time if no handler is active or no reset time is known.
func (cc *ClaudeController) GetRateLimitResetTime() time.Time {
	if rlh := cc.rateLimitHandler.Load(); rlh != nil {
		return rlh.GetResetTime()
	}
	return time.Time{}
}

// GetRateLimitHandler returns the rate limit PTY consumer (for callback wiring).
// Returns nil if the controller has not been started yet.
func (cc *ClaudeController) GetRateLimitHandler() *ratelimit.PTYConsumer {
	return cc.rateLimitHandler.Load()
}

// SetRateLimitEnabled enables or disables rate limit detection.
func (cc *ClaudeController) SetRateLimitEnabled(enabled bool) {
	if rlh := cc.rateLimitHandler.Load(); rlh != nil {
		rlh.SetEnabled(enabled)
	}
}

// IsRateLimitEnabled returns whether rate limit detection is enabled.
func (cc *ClaudeController) IsRateLimitEnabled() bool {
	if rlh := cc.rateLimitHandler.Load(); rlh != nil {
		return rlh.IsEnabled()
	}
	return false
}

// GetStatusDetector returns the status detector used by this controller.
// Used by GetDetectionEvents RPC to retrieve recent detection events for debugging.
func (cc *ClaudeController) GetStatusDetector() detection.TerminalDetector {
	return cc.statusDetector.Load()
}

// runStatusChangeLoop waits for output signals on statusCheckCh, checks the current
// status, and calls registered listeners whenever the status transitions to a new value.
// Exits when ctx is cancelled (i.e., when Stop() calls cancel()).
func (cc *ClaudeController) runStatusChangeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-cc.statusCheckCh:
			newStatus, _ := cc.GetCurrentStatus()

			last := detection.DetectedStatus(cc.lastEmittedStatus.Load())
			if newStatus == last {
				continue
			}
			cc.lastEmittedStatus.Store(int64(newStatus))

			var ls []StatusChangeListener
			cc.listeners.Read(func(listeners []StatusChangeListener) {
				ls = make([]StatusChangeListener, len(listeners))
				copy(ls, listeners)
			})

			for _, fn := range ls {
				fn(newStatus, cc.sessionName)
			}
		}
	}
}

func generateCommandID() string {
	return fmt.Sprintf("cmd_%d", time.Now().UnixNano())
}

func getQueuePersistDir() string {
	return getPersistDir()
}

func getHistoryPersistDir() string {
	return getPersistDir()
}

func getPersistDir() string {
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/tmp"
	}
	return homeDir + "/.stapler-squad"
}
