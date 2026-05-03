package session

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

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
		TerminalStatuses:    []detection.DetectedStatus{detection.StatusReady},
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
	if status != detection.StatusUnknown {
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
	_ = controller.CancelCommand("test-cmd")
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

	var id2 string
	if err := wait.WaitForCondition(func() bool {
		id2 = generateCommandID()
		return id2 != id1
	}, wait.FastWaitConfig()); err != nil {
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

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

// tmuxOutputSmall is a realistic small terminal pane: ~15 lines, some tmux bars.
var tmuxOutputSmall = func() string {
	lines := []string{
		"[staplersquad_my-session] 10:32:01",
		"",
		"  ✓ Compiled successfully",
		"  Reading file.go",
		"  Writing output.go",
		"[staplersquad_my-session] 10:32:02",
		"  > Running tests...",
		"  ok  github.com/tstapler/stapler-squad/session  0.123s",
		"  Thinking...",
		"[staplersquad_my-session] 10:32:03",
		"  Processing request",
		"  Tool use: Read ./main.go",
		"  ◇ Ready",
		"",
		"  esc to interrupt",
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	return sb.String()
}()

// tmuxOutputLarge is a realistic large terminal pane: ~500 lines of mixed content.
var tmuxOutputLarge = func() string {
	var sb strings.Builder
	for i := 0; i < 33; i++ {
		sb.WriteString(tmuxOutputSmall)
	}
	return sb.String()
}()

// ---------------------------------------------------------------------------
// Unit tests — tailContent
// ---------------------------------------------------------------------------

func TestTailContent_ShorterThanWindow(t *testing.T) {
	s := "hello\nworld\n"
	got := tailContent(s, 4096)
	if got != s {
		t.Errorf("expected unchanged string, got %q", got)
	}
}

func TestTailContent_LongerThanWindow(t *testing.T) {
	// Build a string with 10 lines; keep only the last 3.
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	// Window large enough to capture last ~3 lines but not all.
	got := tailContent(content, 20)
	// Must not start mid-line.
	if len(got) == 0 || got[0] == '\n' {
		t.Errorf("tail starts at bad position: %q", got)
	}
	// The last line of content must be present.
	if !strings.Contains(got, "line10") {
		t.Errorf("tail missing last line, got: %q", got)
	}
}

func TestTailContent_ExactlyWindowSize(t *testing.T) {
	s := strings.Repeat("x", statusDetectionTailBytes)
	got := tailContent(s, statusDetectionTailBytes)
	if got != s {
		t.Errorf("expected unchanged string for exact-size input")
	}
}

func TestTailContent_NoNewlineInTail(t *testing.T) {
	// Content that after slicing has no newline — entire tail is one line.
	prefix := strings.Repeat("a\n", 200) // lots of short lines
	suffix := strings.Repeat("b", 100)   // no newline, fits in window
	content := prefix + suffix
	got := tailContent(content, 200)
	// Should contain the suffix (no newline, so tail starts wherever the slice lands)
	if !strings.Contains(got, suffix) {
		t.Errorf("expected tail to include no-newline suffix, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Unit tests — hashString
// ---------------------------------------------------------------------------

func TestHashString_SameInputSameHash(t *testing.T) {
	h1 := hashString("hello")
	h2 := hashString("hello")
	if h1 != h2 {
		t.Error("identical inputs must produce identical hashes")
	}
}

func TestHashString_DifferentInputDifferentHash(t *testing.T) {
	if hashString("hello") == hashString("world") {
		t.Error("different inputs must (almost certainly) produce different hashes")
	}
}

func TestHashString_EmptyString(t *testing.T) {
	// Should not panic and should return a consistent value.
	h1 := hashString("")
	h2 := hashString("")
	if h1 != h2 {
		t.Error("empty string hash must be deterministic")
	}
}

// ---------------------------------------------------------------------------
// Unit tests — status cache (GetCurrentStatus)
// ---------------------------------------------------------------------------

// mockInstance is a minimal InstanceContext that returns a controllable Preview.
type mockInstance struct {
	title      string
	preview    string
	previewErr error
}

func (m *mockInstance) GetTitle() string                    { return m.title }
func (m *mockInstance) GetPTYReader() (*os.File, error)     { return nil, fmt.Errorf("no PTY in mock") }
func (m *mockInstance) Preview() (string, error)            { return m.preview, m.previewErr }
func (m *mockInstance) LastMeaningfulOutputTime() time.Time { return time.Time{} }
func (m *mockInstance) GetCreatedAt() time.Time             { return time.Time{} }
func (m *mockInstance) SetLastMeaningfulOutput(_ time.Time) {}
func (m *mockInstance) GetStatus() int                      { return 0 }
func (m *mockInstance) WriteToPTY(_ []byte) (int, error)    { return 0, nil }

func newControllerWithMock(preview string) (*ClaudeController, *mockInstance) {
	inst := &mockInstance{title: "test", preview: preview}
	cc := &ClaudeController{
		sessionName:    "test",
		instance:       inst,
		statusDetector: detection.NewStatusDetector(),
		idleDetector:   detection.NewIdleDetector("test", nil),
	}
	return cc, inst
}

func TestGetCurrentStatus_CacheHit(t *testing.T) {
	cc, inst := newControllerWithMock(tmuxOutputSmall)

	status1, desc1 := cc.GetCurrentStatus()
	// Change the mock so a real call would return something different — but the
	// tail hash must still match the cached entry.
	inst.preview = tmuxOutputSmall // same content
	status2, desc2 := cc.GetCurrentStatus()

	if status1 != status2 || desc1 != desc2 {
		t.Errorf("cache hit should return same result: (%v,%q) vs (%v,%q)", status1, desc1, status2, desc2)
	}
	// Verify the cache entry was actually populated.
	if cc.statusCache.tailHash == 0 {
		t.Error("statusCache.tailHash should be non-zero after first call")
	}
}

func TestGetCurrentStatus_CacheMissOnChange(t *testing.T) {
	cc, inst := newControllerWithMock(tmuxOutputSmall)
	_, _ = cc.GetCurrentStatus()
	firstHash := cc.statusCache.tailHash

	// Change content — cache must be invalidated.
	inst.preview = tmuxOutputSmall + "\n  New line that changes the tail\n"
	_, _ = cc.GetCurrentStatus()
	secondHash := cc.statusCache.tailHash

	if firstHash == secondHash {
		t.Error("hash should change when content changes")
	}
}

func TestGetCurrentStatus_EmptyContent(t *testing.T) {
	cc, _ := newControllerWithMock("")
	status, _ := cc.GetCurrentStatus()
	if status != detection.StatusUnknown {
		t.Errorf("empty content should yield StatusUnknown, got %v", status)
	}
}

func TestGetCurrentStatus_NilInstance(t *testing.T) {
	cc := &ClaudeController{
		sessionName:    "test",
		statusDetector: detection.NewStatusDetector(),
	}
	status, msg := cc.GetCurrentStatus()
	if status != detection.StatusUnknown {
		t.Errorf("nil instance should yield StatusUnknown, got %v", status)
	}
	if msg == "" {
		t.Error("should return a non-empty message for nil instance")
	}
}

func TestGetCurrentStatus_TailOnlyProcessed(t *testing.T) {
	// Build content where the tail contains "esc to interrupt" (Active) but the
	// body only has "Thinking" (Processing).  We expect Active to win, proving
	// that the tail — not the full buffer — is what the detector sees.
	body := strings.Repeat("  Thinking...\n", 300) // would match Processing
	tail := "  esc to interrupt\n"
	cc, _ := newControllerWithMock(body + tail)

	status, _ := cc.GetCurrentStatus()
	if status != detection.StatusActive {
		t.Errorf("expected StatusActive from tail, got %v", status)
	}
}

// ---------------------------------------------------------------------------
// Unit tests — idle cache (GetIdleState)
// ---------------------------------------------------------------------------

func TestGetIdleState_CacheHit(t *testing.T) {
	cc, inst := newControllerWithMock(tmuxOutputSmall)

	state1, _ := cc.GetIdleState()
	inst.preview = tmuxOutputSmall
	state2, _ := cc.GetIdleState()

	if state1 != state2 {
		t.Errorf("idle cache hit should return same state: %v vs %v", state1, state2)
	}
	if cc.idleCache.tailHash == 0 {
		t.Error("idleCache.tailHash should be non-zero after first call")
	}
}

func TestGetIdleState_CacheMissOnChange(t *testing.T) {
	cc, inst := newControllerWithMock(tmuxOutputSmall)
	_, _ = cc.GetIdleState()
	firstHash := cc.idleCache.tailHash

	inst.preview = tmuxOutputSmall + "\n  changed\n"
	_, _ = cc.GetIdleState()

	if firstHash == cc.idleCache.tailHash {
		t.Error("hash should change when content changes")
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func Benchmark_filterTmuxMetadata_Small(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = filterTmuxMetadata(tmuxOutputSmall)
	}
}

func Benchmark_filterTmuxMetadata_Large(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = filterTmuxMetadata(tmuxOutputLarge)
	}
}

func Benchmark_tailContent_Large(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tailContent(tmuxOutputLarge, statusDetectionTailBytes)
	}
}

func Benchmark_hashString_4KB(b *testing.B) {
	s := tailContent(tmuxOutputLarge, statusDetectionTailBytes)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashString(s)
	}
}

// Benchmark_GetCurrentStatus_CacheHit measures the hot path: content unchanged.
func Benchmark_GetCurrentStatus_CacheHit(b *testing.B) {
	cc, _ := newControllerWithMock(tmuxOutputLarge)
	// Warm the cache.
	_, _ = cc.GetCurrentStatus()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cc.GetCurrentStatus()
	}
}

// Benchmark_GetCurrentStatus_CacheMiss measures the cold path: content changed
// every call (worst case — forces full filter + detect on every tick).
func Benchmark_GetCurrentStatus_CacheMiss(b *testing.B) {
	cc, inst := newControllerWithMock(tmuxOutputLarge)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Toggle a trailing character so the hash always misses.
		if i%2 == 0 {
			inst.preview = tmuxOutputLarge + "a"
		} else {
			inst.preview = tmuxOutputLarge + "b"
		}
		_, _ = cc.GetCurrentStatus()
	}
}
