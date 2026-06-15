package session

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
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

// executorLifecycle holds the running context for the executor.
// Protected by lifecycle; write-locked only during Start/Stop transitions.
type executorLifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// CommandExecutor executes commands by writing to PTY and monitoring responses.
type CommandExecutor struct {
	// Immutable after construction
	sessionName    string
	ptyAccess      *PTYAccess
	responseStream *ResponseStream
	statusDetector detection.TerminalDetector
	queue          *CommandQueue
	subscriberID   string

	// lifecycle guards Start/Stop transitions; write-locked only at transition boundary.
	lifecycle Locked[executorLifecycle]
	// executing is true between Start() and Stop(). Atomic so IsExecuting() never contends.
	executing atomic.Bool

	// currentCommand is set for the duration of each command execution.
	// atomic.Pointer eliminates lock contention on GetCurrentCommand().
	currentCommand atomic.Pointer[Command]

	// optMu protects options and resultCallback (rarely changed).
	optMu          sync.RWMutex
	options        ExecutionOptions
	resultCallback func(*ExecutionResult)

	wg sync.WaitGroup
}

// NewCommandExecutor creates a new command executor for the given session.
func NewCommandExecutor(
	sessionName string,
	ptyAccess *PTYAccess,
	responseStream *ResponseStream,
	statusDetector detection.TerminalDetector,
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
	statusDetector detection.TerminalDetector,
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
	var startErr error
	ce.lifecycle.Write(func(l *executorLifecycle) {
		if l.ctx != nil {
			startErr = fmt.Errorf("command executor already started for session '%s'", ce.sessionName)
			return
		}
		if ce.responseStream == nil {
			startErr = fmt.Errorf("response stream not initialized for session '%s'", ce.sessionName)
			return
		}
		innerCtx, cancel := context.WithCancel(ctx)
		l.ctx = innerCtx
		l.cancel = cancel
		ce.executing.Store(true)
		ce.wg.Add(1)
		go ce.executionLoop(innerCtx)
	})
	if startErr != nil {
		return startErr
	}
	log.Info("command executor started", "session", ce.sessionName)
	return nil
}

// Stop stops the command executor and waits for completion.
func (ce *CommandExecutor) Stop() error {
	var cancelFn context.CancelFunc
	ce.lifecycle.Write(func(l *executorLifecycle) {
		if l.cancel == nil {
			return
		}
		cancelFn = l.cancel
		l.ctx = nil
		l.cancel = nil
	})
	if cancelFn == nil {
		return fmt.Errorf("command executor not started for session '%s'", ce.sessionName)
	}
	cancelFn()
	ce.wg.Wait()
	ce.executing.Store(false)
	ce.currentCommand.Store(nil)
	log.Info("command executor stopped", "session", ce.sessionName)
	return nil
}

// executionLoop is the main execution loop that processes commands from the queue.
func (ce *CommandExecutor) executionLoop(ctx context.Context) {
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
		case <-ctx.Done():
			// Execution cancelled
			return
		default:
			// Try to get next command
			cmd := ce.queue.Dequeue()
			if cmd == nil {
				// No commands available, wait for notification while draining response channel
				// to prevent "channel full" warnings when output arrives during idle periods
				if !ce.waitForCommandOrDrain(ctx, responseCh) {
					// Response channel closed - session is stopping, exit the loop
					return
				}
				continue
			}

			// Execute the command
			result := ce.executeCommand(ctx, cmd, responseCh)

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
			ce.optMu.RLock()
			cb := ce.resultCallback
			ce.optMu.RUnlock()
			if cb != nil {
				cb(result)
			}
		}
	}
}

// waitForCommandOrDrain waits for a new command while draining the response channel
// to prevent buffer overflow when output arrives during idle periods.
// Returns true if the caller should continue (timer fired or command ready),
// false if the response channel closed (caller should exit).
func (ce *CommandExecutor) waitForCommandOrDrain(ctx context.Context, responseCh <-chan ResponseChunk) bool {
	// Create the timer once outside the loop. Using time.After() inside the loop
	// allocates a new channel+timer on every iteration, causing massive GC pressure
	// when response chunks are flowing (thousands of leaked timers per second).
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ce.queue.NotifyChannel():
			// New command available
			return true
		case _, ok := <-responseCh:
			// Drain response chunks while idle - we don't need the data,
			// just preventing channel overflow. Consume all buffered chunks
			// in a tight loop so we re-enter the scheduler only once per burst
			// rather than once per chunk.
			if !ok {
				return false
			}
			for len(responseCh) > 0 {
				if _, ok := <-responseCh; !ok {
					return false
				}
			}
			// Continue draining
		case <-timer.C:
			// Periodic check for context cancellation
			return true
		}
	}
}

// executeCommand executes a single command and returns the result.
func (ce *CommandExecutor) executeCommand(ctx context.Context, cmd *Command, responseCh <-chan ResponseChunk) *ExecutionResult {
	log.Info("executing command", "cmd_id", cmd.ID, "session", ce.sessionName, "text", cmd.Text)

	result := &ExecutionResult{
		Command:       cmd,
		Success:       false,
		StartTime:     time.Now(),
		StatusChanges: make([]StatusChange, 0),
	}

	// Mark command as executing
	ce.currentCommand.Store(cmd)
	defer ce.currentCommand.Store(nil)

	// Snapshot options once at the start of execution so the ticker/timer use
	// consistent values even if SetOptions is called concurrently.
	ce.optMu.RLock()
	opts := ce.options
	ce.optMu.RUnlock()

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
	timeoutTimer := time.NewTimer(opts.Timeout)
	defer timeoutTimer.Stop()

	statusCheckTicker := time.NewTicker(opts.StatusCheckInterval)
	defer statusCheckTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Execution cancelled
			result.Error = fmt.Errorf("execution cancelled")
			result.EndTime = time.Now()
			return result

		case <-timeoutTimer.C:
			// Timeout
			result.Error = fmt.Errorf("command execution timed out after %v", opts.Timeout)
			result.EndTime = time.Now()
			log.Warn("command timed out", "cmd_id", cmd.ID, "timeout", opts.Timeout)
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
			if opts.MaxOutputSize > 0 && len(outputBuffer) > opts.MaxOutputSize {
				// Keep only the last MaxOutputSize bytes
				outputBuffer = outputBuffer[len(outputBuffer)-opts.MaxOutputSize:]
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
					if ce.isTerminalStatus(opts, status) {
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
func (ce *CommandExecutor) isTerminalStatus(opts ExecutionOptions, status detection.DetectedStatus) bool {
	for _, terminalStatus := range opts.TerminalStatuses {
		if status == terminalStatus {
			return true
		}
	}
	return false
}

// IsExecuting returns whether the executor is currently running.
func (ce *CommandExecutor) IsExecuting() bool {
	return ce.executing.Load()
}

// GetCurrentCommand returns the currently executing command, or nil if none.
func (ce *CommandExecutor) GetCurrentCommand() *Command {
	cmd := ce.currentCommand.Load()
	if cmd == nil {
		return nil
	}
	// Return a copy to prevent external modification
	cmdCopy := *cmd
	return &cmdCopy
}

// SetResultCallback sets a callback function to be invoked after each command execution.
func (ce *CommandExecutor) SetResultCallback(callback func(*ExecutionResult)) {
	ce.optMu.Lock()
	defer ce.optMu.Unlock()
	ce.resultCallback = callback
}

// SetOptions updates execution options (only applies to future commands).
func (ce *CommandExecutor) SetOptions(options ExecutionOptions) {
	ce.optMu.Lock()
	defer ce.optMu.Unlock()
	ce.options = options
}

// GetOptions returns the current execution options.
func (ce *CommandExecutor) GetOptions() ExecutionOptions {
	ce.optMu.RLock()
	defer ce.optMu.RUnlock()
	return ce.options
}

// ExecuteImmediate executes a command immediately without using the queue.
// This is useful for interactive commands that need immediate execution.
func (ce *CommandExecutor) ExecuteImmediate(cmd *Command) (*ExecutionResult, error) {
	if !ce.executing.Load() {
		return nil, fmt.Errorf("command executor not started")
	}
	var innerCtx context.Context
	ce.lifecycle.Read(func(l executorLifecycle) {
		innerCtx = l.ctx
	})
	if innerCtx == nil {
		return nil, fmt.Errorf("command executor not started")
	}

	// Subscribe to response stream for this execution
	subscriberID := fmt.Sprintf("immediate_%s_%s", ce.sessionName, cmd.ID)
	responseCh, err := ce.responseStream.Subscribe(subscriberID)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to response stream: %w", err)
	}
	defer ce.responseStream.Unsubscribe(subscriberID)

	result := ce.executeCommand(innerCtx, cmd, responseCh)
	return result, nil
}

// GetSessionName returns the session name for this executor.
func (ce *CommandExecutor) GetSessionName() string {
	return ce.sessionName
}
