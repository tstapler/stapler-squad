package session

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CommandStatus represents the current status of a command in the queue.
type CommandStatus int

const (
	CommandPending CommandStatus = iota
	CommandExecuting
	CommandCompleted
	CommandFailed
	CommandCancelled
)

// String returns a human-readable string for the command status.
func (cs CommandStatus) String() string {
	switch cs {
	case CommandPending:
		return "Pending"
	case CommandExecuting:
		return "Executing"
	case CommandCompleted:
		return "Completed"
	case CommandFailed:
		return "Failed"
	case CommandCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// Command represents a command to be executed in a Claude instance.
type Command struct {
	ID        string        `json:"id"`
	Text      string        `json:"text"`
	Priority  int           `json:"priority"`  // Higher priority = executed first
	Timestamp time.Time     `json:"timestamp"` // When the command was queued
	Status    CommandStatus `json:"status"`
	Result    string        `json:"result,omitempty"`    // Command result/output
	Error     string        `json:"error,omitempty"`     // Error message if failed
	StartTime time.Time     `json:"start_time,omitempty"` // When execution started
	EndTime   time.Time     `json:"end_time,omitempty"`   // When execution finished
}

// priorityQueue implements a priority queue using the heap interface.
type priorityQueue []*Command

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority comes first, if equal then earlier timestamp
	if pq[i].Priority == pq[j].Priority {
		return pq[i].Timestamp.Before(pq[j].Timestamp)
	}
	return pq[i].Priority > pq[j].Priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*Command))
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// CommandQueue manages a priority queue of commands with persistence.
type CommandQueue struct {
	sessionName string
	queue       priorityQueue
	commandMap  map[string]*Command // For fast lookup by ID
	mu          sync.Mutex
	notifyCh    chan struct{} // Notifies when new commands are added
	persistPath string        // Path to persist queue state
}

// NewCommandQueue creates a new command queue for the given session.
func NewCommandQueue(sessionName string) *CommandQueue {
	return &CommandQueue{
		sessionName: sessionName,
		queue:       make(priorityQueue, 0),
		commandMap:  make(map[string]*Command),
		notifyCh:    make(chan struct{}, 1), // Buffered to prevent blocking
	}
}

// NewCommandQueueWithPersistence creates a command queue with persistence enabled.
// The queue state will be saved to the specified directory.
func NewCommandQueueWithPersistence(sessionName string, persistDir string) (*CommandQueue, error) {
	cq := NewCommandQueue(sessionName)
	cq.persistPath = filepath.Join(persistDir, fmt.Sprintf("queue_%s.json", sessionName))

	// Try to load existing queue state
	if err := cq.Load(); err != nil {
		// If file doesn't exist, that's fine - we'll create it on first save
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load queue state: %w", err)
		}
	}

	return cq, nil
}

// Enqueue adds a command to the queue with the specified priority.
// Higher priority commands are executed first.
func (cq *CommandQueue) Enqueue(cmd *Command) error {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	// Check if command with this ID already exists
	if _, exists := cq.commandMap[cmd.ID]; exists {
		return fmt.Errorf("command with ID '%s' already exists in queue", cmd.ID)
	}

	// Set initial status if not set
	if cmd.Status == 0 {
		cmd.Status = CommandPending
	}

	// Add to priority queue
	heap.Push(&cq.queue, cmd)
	cq.commandMap[cmd.ID] = cmd

	// Notify waiting consumers
	select {
	case cq.notifyCh <- struct{}{}:
	default:
		// Channel already has notification, no need to add another
	}

	// Persist queue state if persistence is enabled
	if cq.persistPath != "" {
		if err := cq.saveUnsafe(); err != nil {
			// Log error but don't fail the enqueue operation
			return fmt.Errorf("command enqueued but failed to persist: %w", err)
		}
	}

	return nil
}

// Dequeue removes and returns the highest priority command from the queue.
// Returns nil if the queue is empty.
func (cq *CommandQueue) Dequeue() *Command {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if cq.queue.Len() == 0 {
		return nil
	}

	cmd := heap.Pop(&cq.queue).(*Command)
	delete(cq.commandMap, cmd.ID)

	// Persist queue state if persistence is enabled
	if cq.persistPath != "" {
		if err := cq.saveUnsafe(); err != nil {
			// Log error but don't fail the dequeue operation
			_ = err // TODO: log this error
		}
	}

	return cmd
}

// Peek returns the highest priority command without removing it.
// Returns nil if the queue is empty.
func (cq *CommandQueue) Peek() *Command {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if cq.queue.Len() == 0 {
		return nil
	}

	return cq.queue[0]
}

// Cancel marks a command as cancelled and removes it from the queue.
// Returns an error if the command is not found or is already executing.
func (cq *CommandQueue) Cancel(id string) error {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	cmd, exists := cq.commandMap[id]
	if !exists {
		return fmt.Errorf("command '%s' not found in queue", id)
	}

	if cmd.Status == CommandExecuting {
		return fmt.Errorf("cannot cancel command '%s' that is currently executing", id)
	}

	// Mark as cancelled
	cmd.Status = CommandCancelled
	cmd.EndTime = time.Now()

	// Remove from queue (need to rebuild heap)
	newQueue := make(priorityQueue, 0, cq.queue.Len()-1)
	for _, c := range cq.queue {
		if c.ID != id {
			newQueue = append(newQueue, c)
		}
	}
	cq.queue = newQueue
	heap.Init(&cq.queue)

	delete(cq.commandMap, id)

	// Persist queue state if persistence is enabled
	if cq.persistPath != "" {
		if err := cq.saveUnsafe(); err != nil {
			return fmt.Errorf("command cancelled but failed to persist: %w", err)
		}
	}

	return nil
}

// Get retrieves a command by ID without removing it from the queue.
func (cq *CommandQueue) Get(id string) (*Command, error) {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	cmd, exists := cq.commandMap[id]
	if !exists {
		return nil, fmt.Errorf("command '%s' not found", id)
	}

	return cmd, nil
}

// Update updates the status and metadata of a command.
func (cq *CommandQueue) Update(cmd *Command) error {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	existing, exists := cq.commandMap[cmd.ID]
	if !exists {
		return fmt.Errorf("command '%s' not found", cmd.ID)
	}

	// Update the command in place
	*existing = *cmd

	// Persist queue state if persistence is enabled
	if cq.persistPath != "" {
		if err := cq.saveUnsafe(); err != nil {
			return fmt.Errorf("command updated but failed to persist: %w", err)
		}
	}

	return nil
}

// Len returns the number of commands in the queue.
func (cq *CommandQueue) Len() int {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return cq.queue.Len()
}

// IsEmpty returns true if the queue is empty.
func (cq *CommandQueue) IsEmpty() bool {
	return cq.Len() == 0
}

// List returns all commands currently in the queue.
// The returned slice is a copy to prevent external modification.
func (cq *CommandQueue) List() []*Command {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	commands := make([]*Command, len(cq.queue))
	copy(commands, cq.queue)
	return commands
}

// ListByStatus returns all commands with the specified status.
func (cq *CommandQueue) ListByStatus(status CommandStatus) []*Command {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	var commands []*Command
	for _, cmd := range cq.queue {
		if cmd.Status == status {
			commands = append(commands, cmd)
		}
	}
	return commands
}

// Clear removes all commands from the queue.
func (cq *CommandQueue) Clear() error {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	cq.queue = make(priorityQueue, 0)
	cq.commandMap = make(map[string]*Command)

	// Persist queue state if persistence is enabled
	if cq.persistPath != "" {
		if err := cq.saveUnsafe(); err != nil {
			return fmt.Errorf("queue cleared but failed to persist: %w", err)
		}
	}

	return nil
}

// NotifyChannel returns a channel that receives a notification when commands are added.
// This can be used to wait for new commands without polling.
func (cq *CommandQueue) NotifyChannel() <-chan struct{} {
	return cq.notifyCh
}

// Save persists the queue state to disk.
func (cq *CommandQueue) Save() error {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return cq.saveUnsafe()
}

// saveUnsafe saves the queue without acquiring the lock (internal use only).
func (cq *CommandQueue) saveUnsafe() error {
	if cq.persistPath == "" {
		return fmt.Errorf("persistence not enabled for this queue")
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(cq.persistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create persist directory: %w", err)
	}

	// Convert queue to serializable format
	data := struct {
		SessionName string     `json:"session_name"`
		Commands    []*Command `json:"commands"`
		Timestamp   time.Time  `json:"timestamp"`
	}{
		SessionName: cq.sessionName,
		Commands:    cq.queue,
		Timestamp:   time.Now(),
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal queue data: %w", err)
	}

	// Write to file atomically (write to temp file then rename)
	tempPath := cq.persistPath + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write queue data: %w", err)
	}

	if err := os.Rename(tempPath, cq.persistPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Load restores the queue state from disk.
func (cq *CommandQueue) Load() error {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if cq.persistPath == "" {
		return fmt.Errorf("persistence not enabled for this queue")
	}

	// Read file
	jsonData, err := os.ReadFile(cq.persistPath)
	if err != nil {
		return err
	}

	// Unmarshal JSON
	var data struct {
		SessionName string     `json:"session_name"`
		Commands    []*Command `json:"commands"`
		Timestamp   time.Time  `json:"timestamp"`
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal queue data: %w", err)
	}

	// Rebuild queue and command map
	cq.queue = make(priorityQueue, 0, len(data.Commands))
	cq.commandMap = make(map[string]*Command)

	for _, cmd := range data.Commands {
		heap.Push(&cq.queue, cmd)
		cq.commandMap[cmd.ID] = cmd
	}

	return nil
}

// GetPersistPath returns the path where the queue state is persisted.
func (cq *CommandQueue) GetPersistPath() string {
	return cq.persistPath
}

// SetPersistPath sets the path for queue persistence.
func (cq *CommandQueue) SetPersistPath(path string) {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.persistPath = path
}
