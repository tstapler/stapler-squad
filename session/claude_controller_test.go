package session

import (
	"claude-squad/session/tmux"
	"testing"
	"time"
)

// Mock instance for testing
func createMockInstance(t *testing.T) *Instance {
	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("Failed to create mock PTY: %v", err)
	}

	// Create a basic instance
	instance := &Instance{
		Title:   "test-session",
		Status:  Running,
		started: true,
	}

	// Create tmux session mock
	tmuxSession := &tmux.TmuxSession{}
	instance.SetTmuxSession(tmuxSession)

	// Store PTY for later access
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})

	return instance
}

func TestNewClaudeController(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	if controller == nil {
		t.Fatal("NewClaudeController() returned nil")
	}

	if controller.sessionName != "test-session" {
		t.Errorf("Session name = %q, expected %q", controller.sessionName, "test-session")
	}
}

func TestNewClaudeController_NilInstance(t *testing.T) {
	_, err := NewClaudeController(nil)
	if err == nil {
		t.Error("NewClaudeController(nil) should fail")
	}
}

func TestNewClaudeController_EmptyTitle(t *testing.T) {
	instance := &Instance{
		Title: "",
	}

	_, err := NewClaudeController(instance)
	if err == nil {
		t.Error("NewClaudeController() with empty title should fail")
	}
}

func TestClaudeController_Initialize(t *testing.T) {
	// Skip this test as it requires a fully initialized instance with PTY
	// This would be tested in integration tests
	t.Skip("Requires full instance initialization")
}

func TestClaudeController_IsStarted(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	if controller.IsStarted() {
		t.Error("Controller should not be started initially")
	}
}

func TestClaudeController_GetSessionName(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	if controller.GetSessionName() != "test-session" {
		t.Errorf("GetSessionName() = %q, expected %q", controller.GetSessionName(), "test-session")
	}
}

func TestClaudeController_GetInstance(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	if controller.GetInstance() != instance {
		t.Error("GetInstance() returned different instance")
	}
}

func TestClaudeController_StopWithoutStart(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	err = controller.Stop()
	if err == nil {
		t.Error("Stop() without Start() should fail")
	}
}

func TestClaudeController_SendCommandWithoutStart(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	_, err = controller.SendCommand("test", 10)
	if err == nil {
		t.Error("SendCommand() without Start() should fail")
	}
}

func TestClaudeController_SendCommandImmediateWithoutStart(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	_, err = controller.SendCommandImmediate("test")
	if err == nil {
		t.Error("SendCommandImmediate() without Start() should fail")
	}
}

func TestClaudeController_GetExecutionOptions(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should return defaults when not initialized
	opts := controller.GetExecutionOptions()
	if opts.Timeout <= 0 {
		t.Error("Default timeout should be > 0")
	}
}

func TestClaudeController_SetExecutionOptions(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	newOpts := ExecutionOptions{
		Timeout:             30 * time.Second,
		MaxOutputSize:       4096,
		StatusCheckInterval: 500 * time.Millisecond,
		TerminalStatuses:    []DetectedStatus{StatusReady},
	}

	controller.SetExecutionOptions(newOpts)

	// Options should be set even if executor is nil
	// Will be applied when executor is created
}

func TestClaudeController_ClearHistoryWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	err = controller.ClearHistory()
	if err == nil {
		t.Error("ClearHistory() without initialization should fail")
	}
}

func TestClaudeController_ClearQueueWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	err = controller.ClearQueue()
	if err == nil {
		t.Error("ClearQueue() without initialization should fail")
	}
}

func TestClaudeController_GetRecentOutputWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	output := controller.GetRecentOutput(100)
	if output != nil {
		t.Error("GetRecentOutput() without initialization should return nil")
	}
}

func TestClaudeController_GetCurrentStatusWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	status, context := controller.GetCurrentStatus()
	if status != StatusUnknown {
		t.Errorf("Status = %v, expected StatusUnknown", status)
	}

	if context == "" {
		t.Error("Context should not be empty")
	}
}

func TestClaudeController_SubscribeWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	_, err = controller.Subscribe("test-subscriber")
	if err == nil {
		t.Error("Subscribe() without initialization should fail")
	}
}

func TestClaudeController_UnsubscribeWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	err = controller.Unsubscribe("test-subscriber")
	if err == nil {
		t.Error("Unsubscribe() without initialization should fail")
	}
}

func TestClaudeController_GetCommandStatusNoCommand(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should fail gracefully when no command exists
	_, err = controller.GetCommandStatus("nonexistent")
	if err == nil {
		t.Error("GetCommandStatus() for nonexistent command should fail")
	}
}

func TestClaudeController_CancelCommandWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should handle nil queue gracefully
	err = controller.CancelCommand("test-cmd")
	// May panic or return error depending on implementation
}

func TestClaudeController_GetCurrentCommandWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should handle nil executor gracefully
	cmd := controller.GetCurrentCommand()
	// May panic or return nil depending on implementation
	_ = cmd
}

func TestClaudeController_GetQueuedCommandsWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should handle nil queue gracefully
	cmds := controller.GetQueuedCommands()
	// May panic or return nil depending on implementation
	_ = cmds
}

func TestClaudeController_GetCommandHistoryWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should handle nil history gracefully
	history := controller.GetCommandHistory(10)
	// May panic or return nil depending on implementation
	_ = history
}

func TestClaudeController_SearchHistoryWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should handle nil history gracefully
	results := controller.SearchHistory("test")
	// May panic or return nil depending on implementation
	_ = results
}

func TestClaudeController_GetHistoryStatisticsWithoutInit(t *testing.T) {
	instance := &Instance{
		Title: "test-session",
	}

	controller, err := NewClaudeController(instance)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}

	// Should handle nil history gracefully
	stats := controller.GetHistoryStatistics()
	// May panic or return zero stats depending on implementation
	_ = stats
}

func TestGenerateCommandID(t *testing.T) {
	id1 := generateCommandID()
	if id1 == "" {
		t.Error("generateCommandID() returned empty string")
	}

	// Wait a bit to ensure different timestamp
	time.Sleep(1 * time.Millisecond)

	id2 := generateCommandID()
	if id1 == id2 {
		t.Error("generateCommandID() should generate unique IDs")
	}
}

func TestGetPersistDir(t *testing.T) {
	dir := getPersistDir()
	if dir == "" {
		t.Error("getPersistDir() returned empty string")
	}
}

func TestGetQueuePersistDir(t *testing.T) {
	dir := getQueuePersistDir()
	if dir == "" {
		t.Error("getQueuePersistDir() returned empty string")
	}
}

func TestGetHistoryPersistDir(t *testing.T) {
	dir := getHistoryPersistDir()
	if dir == "" {
		t.Error("getHistoryPersistDir() returned empty string")
	}
}

// Integration test - requires full setup
func TestClaudeController_FullLifecycle(t *testing.T) {
	t.Skip("Integration test - requires full instance with PTY")

	// This test would verify:
	// 1. Initialize()
	// 2. Start()
	// 3. SendCommand()
	// 4. GetCommandStatus()
	// 5. Subscribe()
	// 6. GetRecentOutput()
	// 7. GetCurrentStatus()
	// 8. Stop()
}

// Benchmark tests
func Benchmark_ClaudeController_Creation(b *testing.B) {
	instance := &Instance{
		Title: "test-session",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewClaudeController(instance)
	}
}

func Benchmark_GenerateCommandID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateCommandID()
	}
}
