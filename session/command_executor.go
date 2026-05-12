package session

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"sync"
	"time"
)

// ExecutionResult represents the result of a command execution.
type ExecutionResult struct {
	Command       *Command
	Success       bool
	Output        string
	Error         error
	StartTime     time.Time
	EndTime       time.Time
	FinalStatus   detection.DetectedStatus
	StatusChanges []StatusChange
}

// StatusChange represents a change in detected status during execution.
type StatusChange struct {
	Timestamp time.Time
	Status    detection.DetectedStatus
	Context   string
}

// ExecutionOptions configures command execution behavior.
type ExecutionOptions struct {
	// Timeout for command execution (0 = no timeout)
	Timeout time.Duration
	// MaxOutputSize limits captured output (0 = unlimited)
	MaxOutputSize int
	// StatusCheckInterval for polling status detector
	StatusCheckInterval time.Duration
	// TerminalStatuses are statuses that indicate command completion
	TerminalStatuses []detection.DetectedStatus
}

// DefaultExecutionOptions returns sensible defaults for command execution.
func DefaultExecutionOptions() ExecutionOptions {
	return ExecutionOptions{
		Timeout:             5 * time.Minute,
		MaxOutputSize:       1024 * 1024, // 1MB
		StatusCheckInterval: 100 * time.Millisecond,
		TerminalStatuses: []detection.DetectedStatus{
			detection.StatusReady,
			detection.StatusError,
		},
	}
}

// CommandExecutor executes commands by writing to PTY and monitoring responses.
type CommandExecutor struct {
	sessionName    string
	ptyAccess      *PTYAccess
	responseStream *ResponseStream
	statusDetector *detection.StatusDetector
	queue          *CommandQueue
	options        ExecutionOptions
	mu             sync.RWMutex
	executing      bool
	currentCommand *Command
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	resultCallback func(*ExecutionResult)
	subscriberID   string
}

// NewCommandExecutor creates a new command executor for the given session.
func NewCommandExecutor(
	sessionName string,
	ptyAccess *PTYAccess,
	responseStream *ResponseStream,
	statusDetector *detection.StatusDetector,
	queue *CommandQueue,
) *CommandExecutor {
	return &CommandExecutor{
		sessionName:    sessionName,
		ptyAccess:      ptyAccess,
		responseStream: responseStream,
		statusDetector: statusDetector,
		queue:          queue,
		options:        DefaultExecutionOptions(),
		subscriberID:   fmt.Sprintf("executor_%s", sessionName),
	}
}

// NewCommandExecutorWithOptions creates a command executor with custom options.
func NewCommandExecutorWithOptions(
	sessionName string,
	ptyAccess *PTYAccess,
	responseStream *ResponseStream,
	statusDetector *detection.StatusDetector,
	queue *CommandQueue,
	options ExecutionOptions,
) *CommandExecutor {
	return &CommandExecutor{
		sessionName:    sessionName,
		ptyAccess:      ptyAccess,
		responseStream: responseStream,
		statusDetector: statusDetector,
		queue:          queue,
		options:        options,
		subscriberID:   fmt.Sprintf("executor_%s", sessionName),
	}
}

// Start begins processing commands from the queue.
func (ce *CommandExecutor) Start(ctx context.Context) error {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if ce.executing {
		return fmt.Errorf("command executor already started for session '%s'", ce.sessionName)
	}

	if ce.responseStream == nil {
		return fmt.Errorf("response stream not initialized for session '%s'", ce.sessionName)
	}

	ce.ctx, ce.cancel = context.WithCancel(ctx)
	ce.executing = true

	// Start the execution loop
	ce.wg.Add(1)
	go ce.executionLoop()

	log.Info("command executor started", "session", ce.sessionName)
	return nil
}

// executionLoop is the main execution loop that processes commands from the queue.
func (ce *CommandExecutor) executionLoop() {
	defer ce.wg.Done()
	defer log.Info("command executor stopped", "session", ce.sessionName)

	// Subscribe to response stream
	responseCh, err := ce.responseStream.Subscribe(ce.subscriberID)
	if err != nil {
		log.Error("failed to subscribe to response stream", "session", ce.sessionName, "err", err)
		return
	}
	defer ce.responseStream.Unsubscribe(ce.subscriberID)

	for {
		select {
		case <-ce.ctx.Done():
			// Execution cancelled
			return
		default:
			// Try to get next command
			cmd := ce.queue.Dequeue()
			if cmd == nil {
				// No commands available, wait for notification while draining response channel
				// to prevent "channel full" warnings when output arrives during idle periods
				if !ce.waitForCommandOrDrain(responseCh) {
					// Response channel closed - session is stopping, exit the loop
					return
				}
				continue
			}

			// Execute the command
			result := ce.executeCommand(cmd, responseCh)

			// Update command in queue
			cmd.Status = CommandCompleted
			if !result.Success {
				cmd.Status = CommandFailed
			}
			cmd.Result = result.Output
			if result.Error != nil {
				cmd.Error = result.Error.Error()
			}
			cmd.StartTime = result.StartTime
			cmd.EndTime = result.EndTime

			if err := ce.queue.Update(cmd); err != nil {
				log.Error("failed to update command in queue", "cmd_id", cmd.ID, "err", err)
			}

			// Invoke callback if set
			if ce.resultCallback != nil {
				ce.resultCallback(result)
			}
		}
	}
}

// waitForCommandOrDrain waits for a new command while draining the response channel
// to prevent buffer overflow when output arrives during idle periods.
// Returns true if the caller should continue (timer fired or command ready),
// false if the response channel closed (caller should exit).
func (ce *CommandExecutor) waitForCommandOrDrain(responseCh <-chan ResponseChunk) bool {
	// Create the timer once outside the loop. Using time.After() inside the loop
	// allocates a new channel+timer on every iteration, causing massive GC pressure
	// when response chunks are flowing (thousands of leaked timers per second).
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ce.ctx.Done():
			return false
		case <-ce.queue.NotifyChannel():
			// New command available
			return true
		case _, ok := <-responseCh:
			// Drain response chunks while idle - we don't need the data,
			// just preventing channel overflow
			if !ok {
				// Response channel closed - session is stopping
				return false
			}
			// Continue draining
		case <-timer.C:
			// Periodic check for context cancellation
			return true
		}
	}
}

// executeCommand executes a single command and returns the result.
func (ce *CommandExecutor) executeCommand(cmd *Command, responseCh <-chan ResponseChunk) *ExecutionResult {
	log.Info("executing command", "cmd_id", cmd.ID, "session", ce.sessionName, "text", cmd.Text)

	result := &ExecutionResult{
		Command:       cmd,
		Success:       false,
		StartTime:     time.Now(),
		StatusChanges: make([]StatusChange, 0),
	}

	// Mark command as executing
	ce.mu.Lock()
	ce.currentCommand = cmd
	ce.mu.Unlock()

	// Update command status in queue
	cmd.Status = CommandExecuting
	cmd.StartTime = result.StartTime
	if err := ce.queue.Update(cmd); err != nil {
		log.Error("failed to update command status to executing", "err", err)
	}

	// Write command to PTY
	commandText := cmd.Text + "\n"
	if _, err := ce.ptyAccess.Write([]byte(commandText)); err != nil {
		result.Error = fmt.Errorf("failed to write command to PTY: %w", err)
		result.EndTime = time.Now()
		log.Error("failed to write command to PTY", "cmd_id", cmd.ID, "err", err)
		return result
	}

	// Monitor response and detect status changes
	var outputBuffer []byte
	lastStatus := detection.StatusUnknown
	timeoutTimer := time.NewTimer(ce.options.Timeout)
	defer timeoutTimer.Stop()

	statusCheckTicker := time.NewTicker(ce.options.StatusCheckInterval)
	defer statusCheckTicker.Stop()

	for {
		select {
		case <-ce.ctx.Done():
			// Execution cancelled
			result.Error = fmt.Errorf("execution cancelled")
			result.EndTime = time.Now()
			return result

		case <-timeoutTimer.C:
			// Timeout
			result.Error = fmt.Errorf("command execution timed out after %v", ce.options.Timeout)
			result.EndTime = time.Now()
			log.Warn("command timed out", "cmd_id", cmd.ID, "timeout", ce.options.Timeout)
			return result

		case chunk, ok := <-responseCh:
			if !ok {
				// Channel closed
				result.EndTime = time.Now()
				result.Output = string(outputBuffer)
				result.FinalStatus = lastStatus
				return result
			}

			if chunk.Error != nil {
				result.Error = chunk.Error
				result.EndTime = time.Now()
				result.Output = string(outputBuffer)
				result.FinalStatus = lastStatus
				return result
			}

			// Append to output buffer
			outputBuffer = append(outputBuffer, chunk.Data...)

			// Check output size limit
			if ce.options.MaxOutputSize > 0 && len(outputBuffer) > ce.options.MaxOutputSize {
				// Keep only the last MaxOutputSize bytes
				outputBuffer = outputBuffer[len(outputBuffer)-ce.options.MaxOutputSize:]
			}

		case <-statusCheckTicker.C:
			// Check status periodically
			if len(outputBuffer) > 0 {
				status, context := ce.statusDetector.DetectWithContext(outputBuffer)
				if status != lastStatus {
					// Status changed
					change := StatusChange{
						Timestamp: time.Now(),
						Status:    status,
						Context:   context,
					}
					result.StatusChanges = append(result.StatusChanges, change)
					lastStatus = status

					// Check if terminal status reached
					if ce.isTerminalStatus(status) {
						result.Success = (status == detection.StatusReady)
						result.EndTime = time.Now()
						result.Output = string(outputBuffer)
						result.FinalStatus = status
						return result
					}
				}
			}
		}
	}
}

// isTerminalStatus checks if a status indicates command completion.
func (ce *CommandExecutor) isTerminalStatus(status detection.DetectedStatus) bool {
	for _, terminalStatus := range ce.options.TerminalStatuses {
		if status == terminalStatus {
			return true
		}
	}
	return false
}

// Stop stops the command executor and waits for completion.
func (ce *CommandExecutor) Stop() error {
	ce.mu.Lock()
	if !ce.executing {
		ce.mu.Unlock()
		return fmt.Errorf("command executor not started for session '%s'", ce.sessionName)
	}
	ce.mu.Unlock()

	// Cancel context
	if ce.cancel != nil {
		ce.cancel()
	}

	// Wait for execution loop to finish
	ce.wg.Wait()

	ce.mu.Lock()
	ce.executing = false
	ce.currentCommand = nil
	ce.mu.Unlock()

	log.Info("command executor stopped", "session", ce.sessionName)
	return nil
}

// IsExecuting returns whether the executor is currently running.
func (ce *CommandExecutor) IsExecuting() bool {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.executing
}

// GetCurrentCommand returns the currently executing command, or nil if none.
func (ce *CommandExecutor) GetCurrentCommand() *Command {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	if ce.currentCommand == nil {
		return nil
	}
	// Return a copy to prevent external modification
	cmdCopy := *ce.currentCommand
	return &cmdCopy
}

// SetResultCallback sets a callback function to be invoked after each command execution.
func (ce *CommandExecutor) SetResultCallback(callback func(*ExecutionResult)) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.resultCallback = callback
}

// SetOptions updates execution options (only applies to future commands).
func (ce *CommandExecutor) SetOptions(options ExecutionOptions) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.options = options
}

// GetOptions returns the current execution options.
func (ce *CommandExecutor) GetOptions() ExecutionOptions {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.options
}

// ExecuteImmediate executes a command immediately without using the queue.
// This is useful for interactive commands that need immediate execution.
func (ce *CommandExecutor) ExecuteImmediate(cmd *Command) (*ExecutionResult, error) {
	ce.mu.RLock()
	if !ce.executing {
		ce.mu.RUnlock()
		return nil, fmt.Errorf("command executor not started")
	}
	ce.mu.RUnlock()

	// Subscribe to response stream for this execution
	subscriberID := fmt.Sprintf("immediate_%s_%s", ce.sessionName, cmd.ID)
	responseCh, err := ce.responseStream.Subscribe(subscriberID)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to response stream: %w", err)
	}
	defer ce.responseStream.Unsubscribe(subscriberID)

	// Execute the command
	result := ce.executeCommand(cmd, responseCh)
	return result, nil
}

// GetSessionName returns the session name for this executor.
func (ce *CommandExecutor) GetSessionName() string {
	return ce.sessionName
}
