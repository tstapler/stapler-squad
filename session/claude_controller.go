package session

import (
	"claude-squad/log"
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// ClaudeController provides a high-level API for controlling Claude instances.
// It orchestrates all the underlying components (queue, executor, history, streams).
type ClaudeController struct {
	sessionName    string
	instance       *Instance
	ptyAccess      *PTYAccess
	responseStream *ResponseStream
	statusDetector *StatusDetector
	idleDetector   *IdleDetector // NEW: Idle state detection
	queue          *CommandQueue
	executor       *CommandExecutor
	history        *CommandHistory
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewClaudeController creates a new controller for the given instance.
func NewClaudeController(instance *Instance) (*ClaudeController, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}

	sessionName := instance.Title
	if sessionName == "" {
		return nil, fmt.Errorf("instance title cannot be empty")
	}

	return &ClaudeController{
		sessionName: sessionName,
		instance:    instance,
	}, nil
}

// Start initializes all components and begins background operations (streaming, command execution).
// This is the single entry point for starting the controller - no separate Initialize() call needed.
func (cc *ClaudeController) Start(ctx context.Context) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.ctx != nil {
		return fmt.Errorf("controller already started for session '%s'", cc.sessionName)
	}

	// Get PTY reader from instance
	ptyReader, err := cc.instance.GetPTYReader()
	if err != nil {
		return fmt.Errorf("failed to get PTY reader: %w", err)
	}

	// Create circular buffer for PTY output
	buffer := NewCircularBuffer(10 * 1024 * 1024) // 10MB buffer
	cc.ptyAccess = NewPTYAccess(cc.sessionName, ptyReader, buffer)

	// Create response stream
	cc.responseStream = NewResponseStream(cc.sessionName, cc.ptyAccess)

	// Create status detector
	cc.statusDetector = NewStatusDetector()

	// Create idle detector
	cc.idleDetector = NewIdleDetector(cc.sessionName, cc.ptyAccess)

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
	if cc.instance != nil && !cc.instance.LastMeaningfulOutput.IsZero() {
		cc.idleDetector.InitializeFromTimestamp(cc.instance.LastMeaningfulOutput)
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
	if cc.instance != nil && cc.instance.LastMeaningfulOutput.IsZero() {
		var migrationTime time.Time
		var migrationSource string

		if !cc.instance.CreatedAt.IsZero() {
			// Prefer CreatedAt: gives accurate age for persistent sessions
			migrationTime = cc.instance.CreatedAt
			migrationSource = "CreatedAt"
		} else {
			// Fallback for transient sessions: use current time
			// Better to show "idle for 0s" than "20412d ago"
			migrationTime = time.Now()
			migrationSource = "time.Now()"
		}

		log.InfoLog.Printf("[ClaudeController] Migrating old session '%s': initializing LastMeaningfulOutput from %s (%v)",
			cc.sessionName, migrationSource, migrationTime)
		cc.instance.LastMeaningfulOutput = migrationTime
		cc.idleDetector.InitializeFromTimestamp(migrationTime)
	}

	// Create command queue with persistence
	queue, err := NewCommandQueueWithPersistence(cc.sessionName, getQueuePersistDir())
	if err != nil {
		return fmt.Errorf("failed to create command queue: %w", err)
	}
	cc.queue = queue

	// Create command history with persistence
	history, err := NewCommandHistoryWithPersistence(cc.sessionName, getHistoryPersistDir())
	if err != nil {
		return fmt.Errorf("failed to create command history: %w", err)
	}
	cc.history = history

	// Create command executor
	cc.executor = NewCommandExecutor(
		cc.sessionName,
		cc.ptyAccess,
		cc.responseStream,
		cc.statusDetector,
		cc.queue,
	)

	// Set up result callback to automatically add to history
	cc.executor.SetResultCallback(func(result *ExecutionResult) {
		if err := cc.history.AddFromResult(result); err != nil {
			log.ErrorLog.Printf("Failed to add execution result to history: %v", err)
		}
	})

	// Set up context for lifecycle management
	cc.ctx, cc.cancel = context.WithCancel(ctx)

	// Start response stream
	if err := cc.responseStream.Start(cc.ctx); err != nil {
		return fmt.Errorf("failed to start response stream: %w", err)
	}

	// Start command executor
	if err := cc.executor.Start(cc.ctx); err != nil {
		cc.responseStream.Stop()
		return fmt.Errorf("failed to start command executor: %w", err)
	}

	log.InfoLog.Printf("Claude controller started for session '%s'", cc.sessionName)
	return nil
}

// Stop stops all background operations and cleans up resources.
func (cc *ClaudeController) Stop() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.ctx == nil {
		return fmt.Errorf("controller not started")
	}

	// Cancel context
	if cc.cancel != nil {
		cc.cancel()
	}

	var errs []error

	// Stop executor
	if err := cc.executor.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("executor stop error: %w", err))
	}

	// Stop response stream
	if err := cc.responseStream.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("response stream stop error: %w", err))
	}

	// Save queue state
	if err := cc.queue.Save(); err != nil {
		errs = append(errs, fmt.Errorf("queue save error: %w", err))
	}

	// Save history
	if err := cc.history.Save(); err != nil {
		errs = append(errs, fmt.Errorf("history save error: %w", err))
	}

	cc.ctx = nil
	cc.cancel = nil

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}

	log.InfoLog.Printf("Claude controller stopped for session '%s'", cc.sessionName)
	return nil
}

// SendCommand sends a command to the Claude instance (queued execution).
func (cc *ClaudeController) SendCommand(text string, priority int) (string, error) {
	cc.mu.RLock()
	if cc.ctx == nil {
		cc.mu.RUnlock()
		return "", fmt.Errorf("controller not started")
	}
	cc.mu.RUnlock()

	cmd := &Command{
		ID:        generateCommandID(),
		Text:      text,
		Priority:  priority,
		Timestamp: time.Now(),
		Status:    CommandPending,
	}

	if err := cc.queue.Enqueue(cmd); err != nil {
		return "", fmt.Errorf("failed to enqueue command: %w", err)
	}

	log.InfoLog.Printf("Command queued for session '%s': %s (ID: %s, priority: %d)",
		cc.sessionName, text, cmd.ID, priority)

	return cmd.ID, nil
}

// SendCommandImmediate sends a command for immediate execution (bypasses queue).
func (cc *ClaudeController) SendCommandImmediate(text string) (*ExecutionResult, error) {
	cc.mu.RLock()
	if cc.ctx == nil {
		cc.mu.RUnlock()
		return nil, fmt.Errorf("controller not started")
	}
	cc.mu.RUnlock()

	cmd := &Command{
		ID:        generateCommandID(),
		Text:      text,
		Priority:  100, // High priority for immediate commands
		Timestamp: time.Now(),
		Status:    CommandPending,
	}

	result, err := cc.executor.ExecuteImmediate(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to execute immediate command: %w", err)
	}

	// Add to history
	if err := cc.history.AddFromResult(result); err != nil {
		log.ErrorLog.Printf("Failed to add immediate execution to history: %v", err)
	}

	log.InfoLog.Printf("Immediate command executed for session '%s': %s (ID: %s)",
		cc.sessionName, text, cmd.ID)

	return result, nil
}

// GetCommandStatus retrieves the current status of a command.
func (cc *ClaudeController) GetCommandStatus(commandID string) (*Command, error) {
	// Check if currently executing
	if cc.executor != nil {
		currentCmd := cc.executor.GetCurrentCommand()
		if currentCmd != nil && currentCmd.ID == commandID {
			return currentCmd, nil
		}
	}

	// Check queue
	if cc.queue != nil {
		cmd, err := cc.queue.Get(commandID)
		if err == nil {
			return cmd, nil
		}
	}

	// Check history
	if cc.history != nil {
		entries := cc.history.GetByCommandID(commandID)
		if len(entries) > 0 {
			return &entries[0].Command, nil
		}
	}

	return nil, fmt.Errorf("command '%s' not found", commandID)
}

// CancelCommand cancels a pending command in the queue.
func (cc *ClaudeController) CancelCommand(commandID string) error {
	if cc.queue == nil {
		return fmt.Errorf("queue not initialized")
	}
	return cc.queue.Cancel(commandID)
}

// GetCurrentCommand returns the currently executing command, if any.
func (cc *ClaudeController) GetCurrentCommand() *Command {
	if cc.executor == nil {
		return nil
	}
	return cc.executor.GetCurrentCommand()
}

// GetQueuedCommands returns all commands currently in the queue.
func (cc *ClaudeController) GetQueuedCommands() []*Command {
	if cc.queue == nil {
		return nil
	}
	return cc.queue.List()
}

// GetCommandHistory returns recent command history.
func (cc *ClaudeController) GetCommandHistory(limit int) []*HistoryEntry {
	if cc.history == nil {
		return nil
	}
	if limit <= 0 {
		return cc.history.GetAll()
	}
	return cc.history.GetRecent(limit)
}

// SearchHistory searches command history by text.
func (cc *ClaudeController) SearchHistory(query string) []*HistoryEntry {
	if cc.history == nil {
		return nil
	}
	return cc.history.Search(query)
}

// GetHistoryStatistics returns statistics about command execution.
func (cc *ClaudeController) GetHistoryStatistics() HistoryStatistics {
	if cc.history == nil {
		return HistoryStatistics{}
	}
	return cc.history.GetStatistics()
}

// Subscribe creates a new subscription to the response stream.
func (cc *ClaudeController) Subscribe(subscriberID string) (<-chan ResponseChunk, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.responseStream == nil {
		return nil, fmt.Errorf("response stream not initialized")
	}

	return cc.responseStream.Subscribe(subscriberID)
}

// Unsubscribe removes a subscription from the response stream.
func (cc *ClaudeController) Unsubscribe(subscriberID string) error {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.responseStream == nil {
		return fmt.Errorf("response stream not initialized")
	}

	return cc.responseStream.Unsubscribe(subscriberID)
}

// GetCurrentStatus detects the current status of the Claude instance.
func (cc *ClaudeController) GetCurrentStatus() (DetectedStatus, string) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	// Use instance.Preview() to get actual terminal content instead of PTY buffer.
	// The PTY buffer may contain incomplete data or just status bar updates,
	// while Preview() captures the current visible terminal pane content.
	if cc.instance == nil {
		return StatusUnknown, "Instance not initialized"
	}

	content, err := cc.instance.Preview()
	if err != nil {
		log.DebugLog.Printf("[GetCurrentStatus] Session '%s': Preview() error: %v", cc.sessionName, err)
		return StatusUnknown, "Failed to get terminal content"
	}

	if content == "" {
		return StatusUnknown, "No terminal content"
	}

	// Filter tmux status bars before pattern matching to prevent false positives
	// from session names containing keywords like "error", "fail", etc.
	filtered, _ := filterTmuxMetadata(content)

	return cc.statusDetector.DetectWithContext([]byte(filtered))
}

// filterTmuxMetadata removes common tmux UI elements from terminal output.
// This prevents false positive status detections from metadata like session names
// appearing in window titles, status bars, or shell prompts.
func filterTmuxMetadata(content string) (string, int) {
	lines := []string{}
	removedCount := 0

	for _, line := range splitLines(content) {
		// Skip lines that look like tmux status bars
		// Common patterns:
		// - "[claudesquad_session-name]" window title format
		// - Lines with only whitespace and status indicators
		// - Lines starting with "[" followed by timestamp or session info
		trimmed := []byte(line)
		// Trim leading/trailing whitespace
		start := 0
		end := len(trimmed)
		for start < end && (trimmed[start] == ' ' || trimmed[start] == '\t') {
			start++
		}
		for end > start && (trimmed[end-1] == ' ' || trimmed[end-1] == '\t' || trimmed[end-1] == '\r' || trimmed[end-1] == '\n') {
			end--
		}
		trimmed = trimmed[start:end]

		// Check if this looks like a tmux status bar
		isStatusBar := false
		if len(trimmed) > 0 && trimmed[0] == '[' {
			// Likely a tmux window title or status indicator
			isStatusBar = true
		}

		if isStatusBar {
			removedCount++
			continue
		}

		lines = append(lines, line)
	}

	result := ""
	for i, line := range lines {
		result += line
		if i < len(lines)-1 {
			result += "\n"
		}
	}

	return result, removedCount
}

// splitLines splits content into lines while preserving line endings.
func splitLines(content string) []string {
	lines := []string{}
	currentLine := ""

	for _, ch := range content {
		currentLine += string(ch)
		if ch == '\n' {
			lines = append(lines, currentLine)
			currentLine = ""
		}
	}

	// Add remaining content if any
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}

	return lines
}

// GetRecentOutput returns recent output from the PTY buffer.
func (cc *ClaudeController) GetRecentOutput(bytes int) []byte {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.ptyAccess == nil {
		return nil
	}

	if bytes <= 0 {
		return cc.ptyAccess.GetBuffer()
	}

	buffer := cc.ptyAccess.GetBuffer()
	if len(buffer) <= bytes {
		return buffer
	}

	return buffer[len(buffer)-bytes:]
}

// IsStarted returns whether the controller is currently started.
func (cc *ClaudeController) IsStarted() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.ctx != nil
}

// GetSessionName returns the session name for this controller.
func (cc *ClaudeController) GetSessionName() string {
	return cc.sessionName
}

// GetInstance returns the underlying Instance.
func (cc *ClaudeController) GetInstance() *Instance {
	return cc.instance
}

// SetExecutionOptions updates command execution options.
func (cc *ClaudeController) SetExecutionOptions(options ExecutionOptions) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.executor != nil {
		cc.executor.SetOptions(options)
	}
}

// GetExecutionOptions returns current execution options.
func (cc *ClaudeController) GetExecutionOptions() ExecutionOptions {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.executor == nil {
		return DefaultExecutionOptions()
	}

	return cc.executor.GetOptions()
}

// ClearHistory removes all command history entries.
func (cc *ClaudeController) ClearHistory() error {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.history == nil {
		return fmt.Errorf("history not initialized")
	}

	return cc.history.Clear()
}

// ClearQueue removes all pending commands from the queue.
func (cc *ClaudeController) ClearQueue() error {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.queue == nil {
		return fmt.Errorf("queue not initialized")
	}

	return cc.queue.Clear()
}

// Helper functions

// IsIdle returns whether the Claude instance is currently idle (waiting for input).
// This uses pattern-based detection on terminal content.
func (cc *ClaudeController) IsIdle() bool {
	state, _ := cc.GetIdleState()
	return state == IdleStateWaiting || state == IdleStateTimeout
}

// IsActive returns whether the Claude instance is actively processing commands.
func (cc *ClaudeController) IsActive() bool {
	state, _ := cc.GetIdleState()
	return state == IdleStateActive
}

// GetIdleState returns the current idle state with timing information.
// Returns the state and the timestamp of last activity.
func (cc *ClaudeController) GetIdleState() (IdleState, time.Time) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.idleDetector == nil {
		return IdleStateUnknown, time.Time{}
	}

	// Use instance.Preview() to get actual terminal content instead of PTY buffer.
	// The PTY buffer may contain incomplete data or just status bar updates,
	// while Preview() captures the current visible terminal pane content.
	var state IdleState
	if cc.instance != nil {
		content, err := cc.instance.Preview()
		if err != nil {
			log.DebugLog.Printf("[GetIdleState] Session '%s': Preview() error: %v, using fallback", cc.sessionName, err)
			// Fallback to old method if Preview() fails
			state = cc.idleDetector.DetectState()
		} else if content != "" {
			// Filter tmux status bars before pattern matching
			filtered, _ := filterTmuxMetadata(content)
			state = cc.idleDetector.DetectStateFromContent(filtered)
		} else {
			// Empty content - keep current state
			state = cc.idleDetector.GetState()
		}
	} else {
		// No instance - fallback to old method
		state = cc.idleDetector.DetectState()
	}

	lastActivity := cc.idleDetector.GetLastActivity()

	return state, lastActivity
}

// GetIdleStateInfo returns comprehensive idle state information.
func (cc *ClaudeController) GetIdleStateInfo() IdleStateInfo {
	// Get the current state using the reliable detection method
	state, lastActivity := cc.GetIdleState()

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.idleDetector == nil {
		return IdleStateInfo{
			State:        IdleStateUnknown,
			SessionName:  cc.sessionName,
			LastActivity: time.Now(),
		}
	}

	// Build the info using the reliably-detected state
	return IdleStateInfo{
		State:           state,
		LastActivity:    lastActivity,
		IdleDuration:    time.Since(lastActivity),
		LastStateChange: cc.idleDetector.GetStateInfo().LastStateChange,
		SessionName:     cc.sessionName,
	}
}

// GetIdleDuration returns how long the session has been idle.
func (cc *ClaudeController) GetIdleDuration() time.Duration {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.idleDetector == nil {
		return 0
	}

	return cc.idleDetector.GetIdleDuration()
}

func generateCommandID() string {
	return fmt.Sprintf("cmd_%d", time.Now().UnixNano())
}

func getQueuePersistDir() string {
	// Use the same directory structure as the main app
	return getPersistDir()
}

func getHistoryPersistDir() string {
	// Use the same directory structure as the main app
	return getPersistDir()
}

func getPersistDir() string {
	// This should ideally be configurable, but for now use a default
	// In production, this would be injected or configured
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/tmp"
	}
	return homeDir + "/.claude-squad"
}
