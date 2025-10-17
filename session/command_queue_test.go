package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewCommandQueue(t *testing.T) {
	cq := NewCommandQueue("test-session")
	if cq == nil {
		t.Fatal("NewCommandQueue() returned nil")
	}

	if cq.sessionName != "test-session" {
		t.Errorf("Session name = %q, expected %q", cq.sessionName, "test-session")
	}

	if cq.Len() != 0 {
		t.Errorf("Initial queue length = %d, expected 0", cq.Len())
	}

	if !cq.IsEmpty() {
		t.Error("New queue should be empty")
	}
}

func TestNewCommandQueueWithPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	cq, err := NewCommandQueueWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandQueueWithPersistence() failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "queue_test-session.json")
	if cq.GetPersistPath() != expectedPath {
		t.Errorf("Persist path = %q, expected %q", cq.GetPersistPath(), expectedPath)
	}
}

func TestCommandQueue_Enqueue(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:        "cmd-1",
		Text:      "test command",
		Priority:  10,
		Timestamp: time.Now(),
	}

	if err := cq.Enqueue(cmd); err != nil {
		t.Fatalf("Enqueue() failed: %v", err)
	}

	if cq.Len() != 1 {
		t.Errorf("Queue length = %d, expected 1", cq.Len())
	}

	if cq.IsEmpty() {
		t.Error("Queue should not be empty after enqueue")
	}
}

func TestCommandQueue_EnqueueDuplicate(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd1 := &Command{
		ID:       "cmd-1",
		Text:     "test command",
		Priority: 10,
	}

	if err := cq.Enqueue(cmd1); err != nil {
		t.Fatalf("First Enqueue() failed: %v", err)
	}

	cmd2 := &Command{
		ID:       "cmd-1",
		Text:     "duplicate command",
		Priority: 20,
	}

	err := cq.Enqueue(cmd2)
	if err == nil {
		t.Error("Enqueue() should fail with duplicate ID")
	}

	if cq.Len() != 1 {
		t.Errorf("Queue length = %d, expected 1 after duplicate attempt", cq.Len())
	}
}

func TestCommandQueue_Dequeue(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:       "cmd-1",
		Text:     "test command",
		Priority: 10,
	}

	cq.Enqueue(cmd)

	dequeued := cq.Dequeue()
	if dequeued == nil {
		t.Fatal("Dequeue() returned nil")
	}

	if dequeued.ID != "cmd-1" {
		t.Errorf("Dequeued command ID = %q, expected %q", dequeued.ID, "cmd-1")
	}

	if cq.Len() != 0 {
		t.Errorf("Queue length = %d, expected 0 after dequeue", cq.Len())
	}
}

func TestCommandQueue_DequeueEmpty(t *testing.T) {
	cq := NewCommandQueue("test-session")

	dequeued := cq.Dequeue()
	if dequeued != nil {
		t.Error("Dequeue() on empty queue should return nil")
	}
}

func TestCommandQueue_PriorityOrdering(t *testing.T) {
	cq := NewCommandQueue("test-session")

	// Add commands with different priorities
	commands := []*Command{
		{ID: "cmd-1", Text: "low priority", Priority: 1, Timestamp: time.Now()},
		{ID: "cmd-2", Text: "high priority", Priority: 100, Timestamp: time.Now()},
		{ID: "cmd-3", Text: "medium priority", Priority: 50, Timestamp: time.Now()},
	}

	for _, cmd := range commands {
		cq.Enqueue(cmd)
	}

	// Dequeue should return highest priority first
	first := cq.Dequeue()
	if first.ID != "cmd-2" {
		t.Errorf("First dequeue ID = %q, expected %q (highest priority)", first.ID, "cmd-2")
	}

	second := cq.Dequeue()
	if second.ID != "cmd-3" {
		t.Errorf("Second dequeue ID = %q, expected %q (medium priority)", second.ID, "cmd-3")
	}

	third := cq.Dequeue()
	if third.ID != "cmd-1" {
		t.Errorf("Third dequeue ID = %q, expected %q (low priority)", third.ID, "cmd-1")
	}
}

func TestCommandQueue_TimestampTieBreaker(t *testing.T) {
	cq := NewCommandQueue("test-session")

	now := time.Now()

	// Add commands with same priority but different timestamps
	cmd1 := &Command{
		ID:        "cmd-1",
		Text:      "older",
		Priority:  10,
		Timestamp: now.Add(-2 * time.Second),
	}
	cmd2 := &Command{
		ID:        "cmd-2",
		Text:      "newer",
		Priority:  10,
		Timestamp: now,
	}

	cq.Enqueue(cmd2)
	cq.Enqueue(cmd1)

	// Should dequeue older timestamp first (FIFO for equal priority)
	first := cq.Dequeue()
	if first.ID != "cmd-1" {
		t.Errorf("First dequeue ID = %q, expected %q (older timestamp)", first.ID, "cmd-1")
	}
}

func TestCommandQueue_Peek(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:       "cmd-1",
		Text:     "test command",
		Priority: 10,
	}

	cq.Enqueue(cmd)

	peeked := cq.Peek()
	if peeked == nil {
		t.Fatal("Peek() returned nil")
	}

	if peeked.ID != "cmd-1" {
		t.Errorf("Peeked command ID = %q, expected %q", peeked.ID, "cmd-1")
	}

	// Queue should still have the command
	if cq.Len() != 1 {
		t.Errorf("Queue length = %d, expected 1 after peek", cq.Len())
	}
}

func TestCommandQueue_PeekEmpty(t *testing.T) {
	cq := NewCommandQueue("test-session")

	peeked := cq.Peek()
	if peeked != nil {
		t.Error("Peek() on empty queue should return nil")
	}
}

func TestCommandQueue_Cancel(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:       "cmd-1",
		Text:     "test command",
		Priority: 10,
		Status:   CommandPending,
	}

	cq.Enqueue(cmd)

	if err := cq.Cancel("cmd-1"); err != nil {
		t.Fatalf("Cancel() failed: %v", err)
	}

	// Command should be removed from queue
	if cq.Len() != 0 {
		t.Errorf("Queue length = %d, expected 0 after cancel", cq.Len())
	}
}

func TestCommandQueue_CancelNonExistent(t *testing.T) {
	cq := NewCommandQueue("test-session")

	err := cq.Cancel("nonexistent")
	if err == nil {
		t.Error("Cancel() should fail with non-existent ID")
	}
}

func TestCommandQueue_CancelExecuting(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:       "cmd-1",
		Text:     "test command",
		Priority: 10,
		Status:   CommandExecuting,
	}

	cq.Enqueue(cmd)

	// Update status to executing
	cmd.Status = CommandExecuting
	cq.Update(cmd)

	err := cq.Cancel("cmd-1")
	if err == nil {
		t.Error("Cancel() should fail for executing command")
	}
}

func TestCommandQueue_Get(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:       "cmd-1",
		Text:     "test command",
		Priority: 10,
	}

	cq.Enqueue(cmd)

	retrieved, err := cq.Get("cmd-1")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if retrieved.ID != "cmd-1" {
		t.Errorf("Retrieved command ID = %q, expected %q", retrieved.ID, "cmd-1")
	}
}

func TestCommandQueue_GetNonExistent(t *testing.T) {
	cq := NewCommandQueue("test-session")

	_, err := cq.Get("nonexistent")
	if err == nil {
		t.Error("Get() should fail with non-existent ID")
	}
}

func TestCommandQueue_Update(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:       "cmd-1",
		Text:     "test command",
		Priority: 10,
		Status:   CommandPending,
	}

	cq.Enqueue(cmd)

	// Update command
	cmd.Status = CommandCompleted
	cmd.Result = "success"

	if err := cq.Update(cmd); err != nil {
		t.Fatalf("Update() failed: %v", err)
	}

	// Verify update
	updated, _ := cq.Get("cmd-1")
	if updated.Status != CommandCompleted {
		t.Errorf("Updated status = %v, expected %v", updated.Status, CommandCompleted)
	}
	if updated.Result != "success" {
		t.Errorf("Updated result = %q, expected %q", updated.Result, "success")
	}
}

func TestCommandQueue_UpdateNonExistent(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd := &Command{
		ID:   "nonexistent",
		Text: "test command",
	}

	err := cq.Update(cmd)
	if err == nil {
		t.Error("Update() should fail with non-existent ID")
	}
}

func TestCommandQueue_List(t *testing.T) {
	cq := NewCommandQueue("test-session")

	// Add multiple commands
	for i := 1; i <= 3; i++ {
		cmd := &Command{
			ID:       string(rune('a' + i - 1)),
			Text:     "command",
			Priority: i * 10,
		}
		cq.Enqueue(cmd)
	}

	list := cq.List()
	if len(list) != 3 {
		t.Errorf("List() length = %d, expected 3", len(list))
	}
}

func TestCommandQueue_ListByStatus(t *testing.T) {
	cq := NewCommandQueue("test-session")

	cmd1 := &Command{ID: "cmd-1", Text: "pending", Status: CommandPending}
	cmd2 := &Command{ID: "cmd-2", Text: "completed", Status: CommandCompleted}
	cmd3 := &Command{ID: "cmd-3", Text: "failed", Status: CommandFailed}

	cq.Enqueue(cmd1)
	cq.Enqueue(cmd2)
	cq.Enqueue(cmd3)

	pending := cq.ListByStatus(CommandPending)
	if len(pending) != 1 {
		t.Errorf("ListByStatus(Pending) length = %d, expected 1", len(pending))
	}
	if pending[0].ID != "cmd-1" {
		t.Errorf("Pending command ID = %q, expected %q", pending[0].ID, "cmd-1")
	}
}

func TestCommandQueue_Clear(t *testing.T) {
	cq := NewCommandQueue("test-session")

	// Add commands
	for i := 1; i <= 3; i++ {
		cmd := &Command{ID: string(rune('a' + i - 1)), Text: "command"}
		cq.Enqueue(cmd)
	}

	if err := cq.Clear(); err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	if cq.Len() != 0 {
		t.Errorf("Queue length = %d, expected 0 after clear", cq.Len())
	}

	if !cq.IsEmpty() {
		t.Error("Queue should be empty after clear")
	}
}

func TestCommandQueue_NotifyChannel(t *testing.T) {
	cq := NewCommandQueue("test-session")

	notifyCh := cq.NotifyChannel()

	// Add command
	cmd := &Command{ID: "cmd-1", Text: "test"}
	cq.Enqueue(cmd)

	// Should receive notification
	select {
	case <-notifyCh:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for notification")
	}
}

func TestCommandQueue_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create queue with persistence
	cq1, err := NewCommandQueueWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandQueueWithPersistence() failed: %v", err)
	}

	// Add commands
	cmd1 := &Command{ID: "cmd-1", Text: "first", Priority: 10}
	cmd2 := &Command{ID: "cmd-2", Text: "second", Priority: 20}

	cq1.Enqueue(cmd1)
	cq1.Enqueue(cmd2)

	// Save
	if err := cq1.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Create new queue and load
	cq2, err := NewCommandQueueWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandQueueWithPersistence() failed: %v", err)
	}

	if cq2.Len() != 2 {
		t.Errorf("Loaded queue length = %d, expected 2", cq2.Len())
	}

	// Verify priority order is maintained
	first := cq2.Dequeue()
	if first.ID != "cmd-2" {
		t.Errorf("First dequeued ID = %q, expected %q (highest priority)", first.ID, "cmd-2")
	}
}

func TestCommandQueue_PersistenceAutoSave(t *testing.T) {
	tmpDir := t.TempDir()

	cq, err := NewCommandQueueWithPersistence("test-session", tmpDir)
	if err != nil {
		t.Fatalf("NewCommandQueueWithPersistence() failed: %v", err)
	}

	cmd := &Command{ID: "cmd-1", Text: "test"}
	cq.Enqueue(cmd)

	// Should auto-save on enqueue
	persistPath := cq.GetPersistPath()
	if _, err := os.Stat(persistPath); os.IsNotExist(err) {
		t.Error("Persist file should be created after enqueue")
	}
}

func TestCommandQueue_LoadNonExistent(t *testing.T) {
	cq := NewCommandQueue("test-session")
	cq.SetPersistPath("/nonexistent/path/queue.json")

	err := cq.Load()
	if err == nil {
		t.Error("Load() should fail with non-existent file")
	}
}

func TestCommandQueue_ConcurrentEnqueue(t *testing.T) {
	cq := NewCommandQueue("test-session")

	var wg sync.WaitGroup
	numGoroutines := 10
	commandsPerGoroutine := 10

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < commandsPerGoroutine; j++ {
				cmd := &Command{
					ID:       string(rune('a'+goroutineID)) + string(rune('0'+j)),
					Text:     "concurrent",
					Priority: goroutineID*10 + j,
				}
				if err := cq.Enqueue(cmd); err != nil {
					t.Errorf("Concurrent Enqueue() failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	expectedLength := numGoroutines * commandsPerGoroutine
	if cq.Len() != expectedLength {
		t.Errorf("Queue length = %d, expected %d after concurrent enqueue", cq.Len(), expectedLength)
	}
}

func TestCommandQueue_ConcurrentDequeue(t *testing.T) {
	cq := NewCommandQueue("test-session")

	// Add commands
	numCommands := 100
	for i := 0; i < numCommands; i++ {
		cmd := &Command{
			ID:       string(rune('a' + i)),
			Text:     "test",
			Priority: i,
		}
		cq.Enqueue(cmd)
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	dequeued := make(chan *Command, numCommands)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for {
				cmd := cq.Dequeue()
				if cmd == nil {
					break
				}
				dequeued <- cmd
			}
		}()
	}

	wg.Wait()
	close(dequeued)

	count := 0
	for range dequeued {
		count++
	}

	if count != numCommands {
		t.Errorf("Dequeued %d commands, expected %d", count, numCommands)
	}

	if cq.Len() != 0 {
		t.Errorf("Queue length = %d, expected 0 after concurrent dequeue", cq.Len())
	}
}

func TestCommandQueue_ConcurrentMixedOperations(t *testing.T) {
	cq := NewCommandQueue("test-session")

	var wg sync.WaitGroup
	numOperations := 100

	// Concurrent enqueue
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			cmd := &Command{
				ID:       "enq-" + string(rune('a'+i)),
				Text:     "test",
				Priority: i,
			}
			cq.Enqueue(cmd)
		}
	}()

	// Concurrent dequeue
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations/2; i++ {
			cq.Dequeue()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Concurrent peek
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			cq.Peek()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()

	// No crashes = success
}

func TestCommandStatus_String(t *testing.T) {
	testCases := []struct {
		status   CommandStatus
		expected string
	}{
		{CommandPending, "Pending"},
		{CommandExecuting, "Executing"},
		{CommandCompleted, "Completed"},
		{CommandFailed, "Failed"},
		{CommandCancelled, "Cancelled"},
	}

	for _, tc := range testCases {
		result := tc.status.String()
		if result != tc.expected {
			t.Errorf("Status %v String() = %q, expected %q", tc.status, result, tc.expected)
		}
	}
}

func Benchmark_CommandQueue_Enqueue(b *testing.B) {
	cq := NewCommandQueue("test-session")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := &Command{
			ID:       string(rune('a' + (i % 26))),
			Text:     "benchmark",
			Priority: i % 100,
		}
		cq.Enqueue(cmd)
	}
}

func Benchmark_CommandQueue_Dequeue(b *testing.B) {
	cq := NewCommandQueue("test-session")

	// Pre-populate queue
	for i := 0; i < 1000; i++ {
		cmd := &Command{
			ID:       string(rune('a' + (i % 26))),
			Text:     "benchmark",
			Priority: i % 100,
		}
		cq.Enqueue(cmd)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cq.Dequeue()
		if cq.IsEmpty() {
			b.StopTimer()
			// Refill
			for j := 0; j < 1000; j++ {
				cmd := &Command{
					ID:       string(rune('a' + (j % 26))),
					Text:     "benchmark",
					Priority: j % 100,
				}
				cq.Enqueue(cmd)
			}
			b.StartTimer()
		}
	}
}

func Benchmark_CommandQueue_ConcurrentOperations(b *testing.B) {
	cq := NewCommandQueue("test-session")

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				cmd := &Command{
					ID:       string(rune('a' + (i % 26))),
					Text:     "benchmark",
					Priority: i % 100,
				}
				cq.Enqueue(cmd)
			} else {
				cq.Dequeue()
			}
			i++
		}
	})
}
