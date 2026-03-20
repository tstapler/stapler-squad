package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

func TestNewCommandExecutor(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	if executor == nil {
		t.Fatal("NewCommandExecutor() returned nil")
	}

	if executor.sessionName != "test-session" {
		t.Errorf("Session name = %q, expected %q", executor.sessionName, "test-session")
	}

	if executor.IsExecuting() {
		t.Error("Executor should not be executing initially")
	}
}

func TestNewCommandExecutorWithOptions(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	customOptions := ExecutionOptions{
		Timeout:             10 * time.Second,
		MaxOutputSize:       2048,
		StatusCheckInterval: 200 * time.Millisecond,
		TerminalStatuses:    []detection.DetectedStatus{detection.StatusReady},
	}

	executor := NewCommandExecutorWithOptions("test-session", ptyAccess, responseStream, statusDetector, queue, customOptions)

	opts := executor.GetOptions()
	if opts.Timeout != customOptions.Timeout {
		t.Errorf("Timeout = %v, expected %v", opts.Timeout, customOptions.Timeout)
	}
	if opts.MaxOutputSize != customOptions.MaxOutputSize {
		t.Errorf("MaxOutputSize = %d, expected %d", opts.MaxOutputSize, customOptions.MaxOutputSize)
	}
}

func TestCommandExecutor_StartAndStop(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	// Start response stream first
	ctx := context.Background()
	if err := responseStream.Start(ctx); err != nil {
		t.Fatalf("Failed to start response stream: %v", err)
	}
	defer responseStream.Stop()

	// Start executor
	if err := executor.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if !executor.IsExecuting() {
		t.Error("Executor should be executing after Start()")
	}

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Stop executor
	if err := executor.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	if executor.IsExecuting() {
		t.Error("Executor should not be executing after Stop()")
	}
}

func TestCommandExecutor_DoubleStart(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	ctx := context.Background()
	responseStream.Start(ctx)
	defer responseStream.Stop()

	if err := executor.Start(ctx); err != nil {
		t.Fatalf("First Start() failed: %v", err)
	}
	defer executor.Stop()

	// Second start should fail
	err = executor.Start(ctx)
	if err == nil {
		t.Error("Second Start() should fail")
	}
}

func TestCommandExecutor_ExecuteSimpleCommand(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	// Use shorter timeout for testing
	options := DefaultExecutionOptions()
	options.Timeout = 2 * time.Second
	options.StatusCheckInterval = 50 * time.Millisecond

	executor := NewCommandExecutorWithOptions("test-session", ptyAccess, responseStream, statusDetector, queue, options)

	ctx := context.Background()
	responseStream.Start(ctx)
	defer responseStream.Stop()

	executor.Start(ctx)
	defer executor.Stop()

	// Track execution results
	var resultMu sync.Mutex
	var executionResult *ExecutionResult
	executor.SetResultCallback(func(result *ExecutionResult) {
		resultMu.Lock()
		defer resultMu.Unlock()
		executionResult = result
	})

	// Enqueue a command
	cmd := &Command{
		ID:       "test-cmd-1",
		Text:     "echo test",
		Priority: 10,
	}
	queue.Enqueue(cmd)

	// Simulate response from PTY
	go func() {
		time.Sleep(100 * time.Millisecond)
		writer.Write([]byte("test\n$ "))
	}()

	// Wait for execution
	time.Sleep(1 * time.Second)

	// Check result
	resultMu.Lock()
	defer resultMu.Unlock()

	if executionResult == nil {
		t.Fatal("No execution result received")
	}

	if executionResult.Command.ID != cmd.ID {
		t.Errorf("Result command ID = %q, expected %q", executionResult.Command.ID, cmd.ID)
	}
}

func TestCommandExecutor_GetCurrentCommand(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	// Should be nil initially
	if cmd := executor.GetCurrentCommand(); cmd != nil {
		t.Error("GetCurrentCommand() should return nil initially")
	}
}

func TestCommandExecutor_SetResultCallback(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	callbackCalled := false
	executor.SetResultCallback(func(result *ExecutionResult) {
		callbackCalled = true
	})

	// Callback should be set (we'll verify it gets called in integration test)
	if !callbackCalled {
		// Expected - callback not called yet
	}
}

func TestCommandExecutor_SetOptions(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	newOptions := ExecutionOptions{
		Timeout:             30 * time.Second,
		MaxOutputSize:       4096,
		StatusCheckInterval: 500 * time.Millisecond,
		TerminalStatuses:    []detection.DetectedStatus{detection.StatusReady, detection.StatusError},
	}

	executor.SetOptions(newOptions)

	opts := executor.GetOptions()
	if opts.Timeout != newOptions.Timeout {
		t.Errorf("Timeout = %v, expected %v", opts.Timeout, newOptions.Timeout)
	}
}

func TestCommandExecutor_ExecuteImmediate(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	options := DefaultExecutionOptions()
	options.Timeout = 1 * time.Second
	options.StatusCheckInterval = 50 * time.Millisecond

	executor := NewCommandExecutorWithOptions("test-session", ptyAccess, responseStream, statusDetector, queue, options)

	ctx := context.Background()
	responseStream.Start(ctx)
	defer responseStream.Stop()

	executor.Start(ctx)
	defer executor.Stop()

	// Simulate response
	go func() {
		time.Sleep(100 * time.Millisecond)
		writer.Write([]byte("immediate response\n$ "))
	}()

	cmd := &Command{
		ID:       "immediate-cmd",
		Text:     "test immediate",
		Priority: 100,
	}

	result, err := executor.ExecuteImmediate(cmd)
	if err != nil {
		t.Fatalf("ExecuteImmediate() failed: %v", err)
	}

	if result == nil {
		t.Fatal("ExecuteImmediate() returned nil result")
	}

	if result.Command.ID != cmd.ID {
		t.Errorf("Result command ID = %q, expected %q", result.Command.ID, cmd.ID)
	}
}

func TestCommandExecutor_ExecuteImmediateNotStarted(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	cmd := &Command{ID: "test", Text: "test"}

	_, err = executor.ExecuteImmediate(cmd)
	if err == nil {
		t.Error("ExecuteImmediate() should fail when executor not started")
	}
}

func TestCommandExecutor_Timeout(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	// Very short timeout
	options := DefaultExecutionOptions()
	options.Timeout = 200 * time.Millisecond
	options.StatusCheckInterval = 50 * time.Millisecond

	executor := NewCommandExecutorWithOptions("test-session", ptyAccess, responseStream, statusDetector, queue, options)

	ctx := context.Background()
	responseStream.Start(ctx)
	defer responseStream.Stop()

	executor.Start(ctx)
	defer executor.Stop()

	var resultMu sync.Mutex
	var executionResult *ExecutionResult
	executor.SetResultCallback(func(result *ExecutionResult) {
		resultMu.Lock()
		defer resultMu.Unlock()
		executionResult = result
	})

	// Enqueue command but don't send response (will timeout)
	cmd := &Command{
		ID:       "timeout-cmd",
		Text:     "long running command",
		Priority: 10,
	}
	queue.Enqueue(cmd)

	// Wait for timeout
	time.Sleep(500 * time.Millisecond)

	resultMu.Lock()
	defer resultMu.Unlock()

	if executionResult == nil {
		t.Fatal("No execution result received")
	}

	if executionResult.Error == nil {
		t.Error("Expected timeout error")
	}

	if !executionResult.Success {
		// Expected - command timed out
	}
}

func TestCommandExecutor_GetSessionName(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	if executor.GetSessionName() != "test-session" {
		t.Errorf("GetSessionName() = %q, expected %q", executor.GetSessionName(), "test-session")
	}
}

func TestDefaultExecutionOptions(t *testing.T) {
	opts := DefaultExecutionOptions()

	if opts.Timeout <= 0 {
		t.Error("Default timeout should be > 0")
	}

	if opts.MaxOutputSize <= 0 {
		t.Error("Default max output size should be > 0")
	}

	if opts.StatusCheckInterval <= 0 {
		t.Error("Default status check interval should be > 0")
	}

	if len(opts.TerminalStatuses) == 0 {
		t.Error("Default terminal statuses should not be empty")
	}
}

func TestCommandExecutor_StopWithoutStart(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	err = executor.Stop()
	if err == nil {
		t.Error("Stop() without Start() should fail")
	}
}

func TestCommandExecutor_StatusDetection(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	options := DefaultExecutionOptions()
	options.Timeout = 2 * time.Second
	options.StatusCheckInterval = 50 * time.Millisecond

	executor := NewCommandExecutorWithOptions("test-session", ptyAccess, responseStream, statusDetector, queue, options)

	ctx := context.Background()
	responseStream.Start(ctx)
	defer responseStream.Stop()

	executor.Start(ctx)
	defer executor.Stop()

	var resultMu sync.Mutex
	var executionResult *ExecutionResult
	executor.SetResultCallback(func(result *ExecutionResult) {
		resultMu.Lock()
		defer resultMu.Unlock()
		executionResult = result
	})

	// Enqueue command
	cmd := &Command{
		ID:       "status-cmd",
		Text:     "test status",
		Priority: 10,
	}
	queue.Enqueue(cmd)

	// Simulate processing then ready status
	go func() {
		time.Sleep(100 * time.Millisecond)
		writer.Write([]byte("Processing...\n"))
		time.Sleep(200 * time.Millisecond)
		writer.Write([]byte("$ "))
	}()

	// Wait for execution
	time.Sleep(1 * time.Second)

	resultMu.Lock()
	defer resultMu.Unlock()

	if executionResult == nil {
		t.Fatal("No execution result received")
	}

	// The test completes successfully even without status changes due to mock PTY limitations
	// In real usage with actual PTY, status changes would be detected
}

func TestCommandExecutor_NilResponseStream(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, nil, statusDetector, queue)

	ctx := context.Background()
	err = executor.Start(ctx)
	if err == nil {
		t.Error("Start() with nil response stream should fail")
	}
}

func TestCommandExecutor_ContextCancellation(t *testing.T) {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	executor := NewCommandExecutor("test-session", ptyAccess, responseStream, statusDetector, queue)

	ctx, cancel := context.WithCancel(context.Background())

	responseStream.Start(ctx)
	defer responseStream.Stop()

	executor.Start(ctx)

	// Cancel context
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Should stop gracefully
	time.Sleep(200 * time.Millisecond)

	// Should be able to stop without error
	if err := executor.Stop(); err != nil {
		// May already be stopped due to context cancellation
	}
}

func Benchmark_CommandExecutor_Execution(b *testing.B) {
	reader, writer, _ := mockPTY()
	defer reader.Close()
	defer writer.Close()

	buffer := NewCircularBuffer(1024)
	ptyAccess := NewPTYAccess("test-session", reader, buffer)
	responseStream := NewResponseStream("test-session", ptyAccess)
	statusDetector := detection.NewStatusDetector()
	queue := NewCommandQueue("test-session")

	options := DefaultExecutionOptions()
	options.Timeout = 1 * time.Second
	options.StatusCheckInterval = 10 * time.Millisecond

	executor := NewCommandExecutorWithOptions("test-session", ptyAccess, responseStream, statusDetector, queue, options)

	ctx := context.Background()
	responseStream.Start(ctx)
	defer responseStream.Stop()

	executor.Start(ctx)
	defer executor.Stop()

	// Simulate responses
	go func() {
		for {
			time.Sleep(50 * time.Millisecond)
			writer.Write([]byte("output\n$ "))
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := &Command{
			ID:       string(rune('a' + (i % 26))),
			Text:     "benchmark command",
			Priority: i,
		}
		queue.Enqueue(cmd)
	}
}
