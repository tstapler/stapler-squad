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
	started        bool
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

// Initialize sets up all components and prepares the controller for use.
func (cc *ClaudeController) Initialize() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.started {
		return fmt.Errorf("controller already initialized for session '%s'", cc.sessionName)
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

	cc.started = true
	log.InfoLog.Printf("Claude controller initialized for session '%s'", cc.sessionName)
	return nil
}

// Start begins all background operations (streaming, command execution).
func (cc *ClaudeController) Start(ctx context.Context) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if !cc.started {
		return fmt.Errorf("controller not initialized, call Initialize() first")
	}

	if cc.ctx != nil {
		return fmt.Errorf("controller already started")
	}

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

	if cc.ptyAccess == nil {
		return StatusUnknown, "PTY access not initialized"
	}

	output := cc.ptyAccess.GetBuffer()
	return cc.statusDetector.DetectWithContext(output)
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
// This uses pattern-based detection on recent PTY output.
func (cc *ClaudeController) IsIdle() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.idleDetector == nil {
		return false
	}

	return cc.idleDetector.IsIdle()
}

// IsActive returns whether the Claude instance is actively processing commands.
func (cc *ClaudeController) IsActive() bool {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.idleDetector == nil {
		return false
	}

	return cc.idleDetector.IsActive()
}

// GetIdleState returns the current idle state with timing information.
// Returns the state and the timestamp of last activity.
func (cc *ClaudeController) GetIdleState() (IdleState, time.Time) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.idleDetector == nil {
		return IdleStateUnknown, time.Time{}
	}

	state := cc.idleDetector.DetectState()
	lastActivity := cc.idleDetector.GetLastActivity()

	return state, lastActivity
}

// GetIdleStateInfo returns comprehensive idle state information.
func (cc *ClaudeController) GetIdleStateInfo() IdleStateInfo {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.idleDetector == nil {
		return IdleStateInfo{
			State:        IdleStateUnknown,
			SessionName:  cc.sessionName,
			LastActivity: time.Now(),
		}
	}

	return cc.idleDetector.GetStateInfo()
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
