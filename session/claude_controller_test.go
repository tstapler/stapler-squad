package session

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/analytics"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/dtypes"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestNewClaudeController(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	_, err := NewClaudeController(nil)
	if err == nil {
		t.Error("NewClaudeController(nil) should fail")
	}
}

func TestNewClaudeController_EmptyTitle(t *testing.T) {
	t.Parallel()
	instance := &Instance{
		Title: "",
	}

	_, err := NewClaudeController(instance)
	if err == nil {
		t.Error("NewClaudeController() with empty title should fail")
	}
}

func TestClaudeController_Initialize(t *testing.T) {
	t.Parallel()
	// Skip this test as it requires a fully initialized instance with PTY
	// This would be tested in integration tests
	t.Skip("Requires full instance initialization")
}

func TestClaudeController_IsStarted(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	dir := getPersistDir()
	if dir == "" {
		t.Error("getPersistDir() returned empty string")
	}
}

func TestGetQueuePersistDir(t *testing.T) {
	t.Parallel()
	dir := getQueuePersistDir()
	if dir == "" {
		t.Error("getQueuePersistDir() returned empty string")
	}
}

func TestGetHistoryPersistDir(t *testing.T) {
	t.Parallel()
	dir := getHistoryPersistDir()
	if dir == "" {
		t.Error("getHistoryPersistDir() returned empty string")
	}
}

// Integration test - requires full setup
func TestClaudeController_FullLifecycle(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	s := "hello\nworld\n"
	got := tailContent(s, 4096)
	if got != s {
		t.Errorf("expected unchanged string, got %q", got)
	}
}

func TestTailContent_LongerThanWindow(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	s := strings.Repeat("x", statusDetectionTailBytes)
	got := tailContent(s, statusDetectionTailBytes)
	if got != s {
		t.Errorf("expected unchanged string for exact-size input")
	}
}

func TestTailContent_NoNewlineInTail(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	h1 := hashString("hello")
	h2 := hashString("hello")
	if h1 != h2 {
		t.Error("identical inputs must produce identical hashes")
	}
}

func TestHashString_DifferentInputDifferentHash(t *testing.T) {
	t.Parallel()
	if hashString("hello") == hashString("world") {
		t.Error("different inputs must (almost certainly) produce different hashes")
	}
}

func TestHashString_EmptyString(t *testing.T) {
	t.Parallel()
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
	stableID   string
	ptyReader  *os.File // nil = GetPTYReader() errors, matching the old always-error behavior
	preview    string
	previewErr error
	program    string // "" (the zero value used by every test that doesn't care) matches no registered detector
}

func (m *mockInstance) GetTitle() string { return m.title }

// GetStableID deliberately does NOT fall back to title when stableID is unset — a fallback
// to title would make GetStableID() == title == sessionName by coincidence for any test that
// forgets to set stableID, which would let a future regression to BUG-025 (using the tmux
// name instead of the stable UUID) silently pass any test asserting on this value.
func (m *mockInstance) GetStableID() string {
	if m.stableID != "" {
		return m.stableID
	}
	return "UNSET-STABLE-ID"
}
func (m *mockInstance) GetPTYReader() (*os.File, error) {
	if m.ptyReader != nil {
		return m.ptyReader, nil
	}
	return nil, fmt.Errorf("no PTY in mock")
}
func (m *mockInstance) Preview() (string, error)            { return m.preview, m.previewErr }
func (m *mockInstance) LastMeaningfulOutputTime() time.Time { return time.Time{} }
func (m *mockInstance) GetCreatedAt() time.Time             { return time.Time{} }
func (m *mockInstance) SetLastMeaningfulOutput(_ time.Time) {}
func (m *mockInstance) GetStatus() int                      { return 0 }
func (m *mockInstance) WriteToPTY(_ []byte) (int, error)    { return 0, nil }
func (m *mockInstance) GetProgram() string                  { return m.program }

func newControllerWithMock(content string) (*ClaudeController, *mockInstance) {
	inst := &mockInstance{title: "test", preview: content}
	cc := &ClaudeController{
		sessionName: "test",
		instance:    inst,
	}
	cc.statusDetector.Store(detection.NewStatusDetector())
	cc.idleDetector.Store(detection.NewIdleDetector("test", nil))
	buf := NewCircularBuffer(256 * 1024)
	if content != "" {
		_, _ = buf.Write([]byte(content))
	}
	cc.ptyAccess.Store(NewPTYAccess("test", nil, buf))
	return cc, inst
}

// TestClaudeController_IsIdle_should_returnFalse_When_BatchedToolCallSummaryDisplayed is the
// most direct regression test for the "no-op nudge into actively-working sessions" bug: it
// exercises the exact mechanism the bug lived in, not a proxy for it.
//
// cc.IsIdle()/GetIdleState() reads pane content through detection.IdleDetector.DetectStateFromContent,
// which runs the SAME StatusDetector this PR's pattern fix changes, then maps the result via
// mapStatusToIdleState (session/detection/idle.go) — critically, StatusUnknown maps to
// IdleStateWaiting (session/detection/idle.go's comment: "Unknown status - don't maintain
// Unknown, default to Waiting"), i.e. treated as IDLE. Before the pattern widening, the reported
// batched-summary pane text classified as StatusUnknown, so cc.IsIdle() incorrectly returned true
// for a session that was actively working — this is the real site AutonomousDriver.run()'s
// waitForIdle reads to decide whether a turn is warranted (both its settleWindow=0 startup path
// and its settleWindow>0 post-turn path call cc.IsIdle() once to seed their decision). After the
// fix, the same text classifies as StatusExecuting, which maps to IdleStateActive.
func TestClaudeController_IsIdle_should_returnFalse_When_BatchedToolCallSummaryDisplayed(t *testing.T) {
	t.Parallel()
	batchedSummary := "✻ Searching for 9 patterns, reading 2 files, running 7 shell commands…"
	cc, _ := newControllerWithMock(batchedSummary)

	if got := cc.IsIdle(); got {
		t.Error("IsIdle() = true for a batched multi-tool-call summary pane — the session is " +
			"actively working; this would let waitForIdle seed idleSince immediately and " +
			"eventually fire a spurious no-op nudge")
	}
	if state, _ := cc.GetIdleState(); state != detection.IdleStateActive {
		t.Errorf("GetIdleState() = %v, want IdleStateActive for a batched multi-tool-call summary pane", state)
	}
}

// TestClaudeController_Start_TagsEscapeAnalyticsWithStableID is a regression test for
// BUG-025 at its actual assembly point. TestResponseStream_SetStableSessionID (in
// response_stream_test.go) proves ResponseStream.SetStableSessionID wiring works, but it
// calls that method directly — it never goes through ClaudeController.Start(), which is
// where cc.instance.GetStableID() is actually supplied in production
// (claude_controller.go: `rs.SetStableSessionID(cc.instance.GetStableID())`). Every other
// test in this file uses mockInstance.GetPTYReader()'s default always-error behavior, so
// Start() returns before reaching that line — this test is the only one that gives
// mockInstance a real (pipe-backed) PTY so Start() can proceed past it. Without this test,
// a future regression at that exact line (e.g. reverting to `rs.SetStableSessionID(cc.sessionName)`)
// would pass the entire existing suite.
func TestClaudeController_Start_TagsEscapeAnalyticsWithStableID(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	spy := &escapeEventSpy{}
	prev := analytics.GetGlobalEscapeWriter()
	analytics.SetGlobalEscapeWriter(spy)
	defer analytics.SetGlobalEscapeWriter(prev)

	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	inst := &mockInstance{
		title:     "claude-controller-wiring-test-tmux-name",
		stableID:  "claude-controller-wiring-test-stable-uuid",
		ptyReader: reader,
	}
	cc, err := NewClaudeController(inst)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}
	if err := cc.Start(context.Background()); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer cc.Stop()

	if _, err := writer.Write([]byte("\x1b[31m")); err != nil {
		t.Fatalf("failed to write test data: %v", err)
	}

	cfg := wait.DefaultWaitConfig()
	cfg.Description = "escape event captured via ClaudeController.Start()"
	if err := wait.WaitForCondition(func() bool {
		return len(spy.snapshot()) > 0
	}, cfg); err != nil {
		t.Fatalf("no escape event captured: %v", err)
	}

	for _, ev := range spy.snapshot() {
		if ev.SessionID == inst.title {
			t.Fatalf("escape event used the tmux name (%q) instead of the stable ID — BUG-025 regressed", ev.SessionID)
		}
		if ev.SessionID != inst.stableID {
			t.Errorf("event SessionID = %q, want %q", ev.SessionID, inst.stableID)
		}
	}
}

// TestClaudeController_Start_UsesPerProgramDetector_WhenRegistered covers
// Story 2.4.1 AC1: when Instance.Program names a program with a registered
// detector, Start() must build its StatusDetector from that program's
// pattern set, not the hardcoded claude patterns getDefaultPatterns() /
// NewStatusDetector() produce.
//
// This uses the built-in "gemini" detector (session/detection/binaries/gemini.go)
// rather than injecting a real plugin file: detection.activeSnapshot is
// unexported and package-private to session/detection, so there is no way to
// mutate it from this package's tests and reliably restore it afterward
// (detection.InitPlugins is the only exported registration path, and it is
// guarded by a process-wide sync.Once with no reset hook — a bad fit for a
// test that must clean up after itself). A built-in override exercises the
// exact same code path (detection.ResolveDetectorForProgram resolving cc.instance.GetProgram()
// against the live snapshot) with zero global state to leak into other tests.
func TestClaudeController_Start_UsesPerProgramDetector_WhenRegistered(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	inst := &mockInstance{
		title:     "gemini-session",
		ptyReader: reader,
		program:   "gemini",
	}
	cc, err := NewClaudeController(inst)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}
	if err := cc.Start(context.Background()); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer cc.Stop()

	sd := cc.statusDetector.Load()
	if sd == nil {
		t.Fatal("statusDetector not set after Start()")
	}

	// "esc to interrupt" only matches claude's built-in Active pattern
	// (`esc\s+(to\s+)?(interrupt|cancel)`, session/detection/binaries/claude.go).
	// gemini's built-in Active category is empty (session/detection/binaries/gemini.go),
	// so this text must NOT resolve to StatusExecuting if Start() correctly
	// wired up the gemini detector instead of falling back to claude's.
	if status := sd.Detect([]byte("esc to interrupt")); status == detection.StatusExecuting {
		t.Errorf(`Detect("esc to interrupt") = %v, want != %v — Start() used claude's default patterns instead of the registered "gemini" detector`, status, detection.StatusExecuting)
	}

	// Positive check: gemini's own pattern does resolve correctly.
	if status := sd.Detect([]byte("✦ Working")); status != detection.StatusProcessing {
		t.Errorf(`Detect("✦ Working") = %v, want %v (gemini_working pattern)`, status, detection.StatusProcessing)
	}
}

// TestClaudeController_Start_FallsBackToDefaultDetector_WhenProgramUnregistered
// is the regression-critical check for Story 2.4.1 AC2: a program with no
// built-in or plugin detector registered (a bare shell name, or any test
// mockInstance that leaves program unset) must construct exactly the same
// StatusDetector Start() always has — NewStatusDetector()'s claude default
// patterns — so every session that isn't running a program with a registered
// detector sees no behavior change from this story.
func TestClaudeController_Start_FallsBackToDefaultDetector_WhenProgramUnregistered(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	reader, writer, err := mockPTY()
	if err != nil {
		t.Fatalf("failed to create mock PTY: %v", err)
	}
	defer reader.Close()
	defer writer.Close()

	inst := &mockInstance{
		title:     "bash-session",
		ptyReader: reader,
		program:   "bash", // no built-in or plugin detector registered for "bash"
	}
	cc, err := NewClaudeController(inst)
	if err != nil {
		t.Fatalf("NewClaudeController() failed: %v", err)
	}
	if err := cc.Start(context.Background()); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer cc.Stop()

	sd := cc.statusDetector.Load()
	if sd == nil {
		t.Fatal("statusDetector not set after Start()")
	}

	if status := sd.Detect([]byte("esc to interrupt")); status != detection.StatusExecuting {
		t.Errorf(`Detect("esc to interrupt") = %v, want %v — regression: Start() must fall back to NewStatusDetector()'s claude default patterns when no detector is registered for the program`, status, detection.StatusExecuting)
	}
}

func TestGetCurrentStatus_CacheHit(t *testing.T) {
	t.Parallel()
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
	if sc := cc.statusCache.Load(); sc == nil || sc.tailHash == 0 {
		t.Error("statusCache.tailHash should be non-zero after first call")
	}
}

func TestGetCurrentStatus_ThenGetStatusAndIdleInfo_should_shareSubagentCount_When_sameTailHash(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("✻ Waiting for 2 background agents to finish")

	// GetCurrentStatus populates the shared statusCache first.
	cc.GetCurrentStatus()
	if sc := cc.statusCache.Load(); sc == nil || sc.subagentCount != 2 {
		t.Fatalf("statusCache.subagentCount = %v, want 2 after GetCurrentStatus", sc)
	}

	// GetStatusAndIdleInfo, called immediately after with the same unchanged tail, must
	// read the same count back off the cache (cache-hit path) — the two methods must
	// never disagree. See ADR-001.
	status, _, _, count := cc.GetStatusAndIdleInfo()
	if status != detection.StatusWaitingForAgent {
		t.Fatalf("status = %v, want StatusWaitingForAgent", status)
	}
	if count != 2 {
		t.Errorf("GetStatusAndIdleInfo count = %d, want 2 (must match GetCurrentStatus's cached write)", count)
	}
	if sc := cc.statusCache.Load(); sc == nil || sc.subagentCount != 2 {
		t.Errorf("statusCache.subagentCount = %v, want 2 to remain after GetStatusAndIdleInfo", sc)
	}
}

func TestGetStatusAndIdleInfo_should_returnZeroCount_When_statusIsNotWaitingForAgent(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("✻ Baked for 3s")

	status, _, _, count := cc.GetStatusAndIdleInfo()
	if status == detection.StatusWaitingForAgent {
		t.Fatalf("test fixture unexpectedly matched StatusWaitingForAgent")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestGetCurrentStatus_CacheMissOnChange(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock(tmuxOutputSmall)
	_, _ = cc.GetCurrentStatus()
	var firstHash uint64
	if sc := cc.statusCache.Load(); sc != nil {
		firstHash = sc.tailHash
	}

	// Update the PTY buffer directly — inst.preview is no longer read by GetCurrentStatus.
	if pa := cc.ptyAccess.Load(); pa != nil {
		pa.buffer.Clear()
		_, _ = pa.buffer.Write([]byte(tmuxOutputSmall + "\n  New line that changes the tail\n"))
	}
	_, _ = cc.GetCurrentStatus()
	var secondHash uint64
	if sc := cc.statusCache.Load(); sc != nil {
		secondHash = sc.tailHash
	}

	if firstHash == secondHash {
		t.Error("hash should change when content changes")
	}
}

func TestGetCurrentStatus_EmptyContent(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("")
	status, _ := cc.GetCurrentStatus()
	if status != detection.StatusUnknown {
		t.Errorf("empty content should yield StatusUnknown, got %v", status)
	}
}

func TestGetCurrentStatus_NilInstance(t *testing.T) {
	t.Parallel()
	cc := &ClaudeController{sessionName: "test"}
	cc.statusDetector.Store(detection.NewStatusDetector())
	status, msg := cc.GetCurrentStatus()
	if status != detection.StatusUnknown {
		t.Errorf("nil instance should yield StatusUnknown, got %v", status)
	}
	if msg == "" {
		t.Error("should return a non-empty message for nil instance")
	}
}

func TestGetCurrentStatus_TailOnlyProcessed(t *testing.T) {
	t.Parallel()
	// Build content where the tail contains "esc to interrupt" (Active) but the
	// body only has "Thinking" (Processing).  We expect Active to win, proving
	// that the tail — not the full buffer — is what the detector sees.
	body := strings.Repeat("  Thinking...\n", 300) // would match Processing
	tail := "  esc to interrupt\n"
	cc, _ := newControllerWithMock(body + tail)

	status, _ := cc.GetCurrentStatus()
	if status != detection.StatusExecuting {
		t.Errorf("expected StatusExecuting from tail, got %v", status)
	}
}

// ---------------------------------------------------------------------------
// Unit tests — idle cache (GetIdleState)
// ---------------------------------------------------------------------------

func TestGetIdleState_CacheHit(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock(tmuxOutputSmall)

	state1, _ := cc.GetIdleState()
	state2, _ := cc.GetIdleState()

	if state1 != state2 {
		t.Errorf("idle cache hit should return same state: %v vs %v", state1, state2)
	}
	if ic := cc.idleCache.Load(); ic == nil || ic.tailHash == 0 {
		t.Error("idleCache.tailHash should be non-zero after first call")
	}
}

func TestGetIdleState_CacheMissOnChange(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock(tmuxOutputSmall)
	_, _ = cc.GetIdleState()
	var firstHash uint64
	if ic := cc.idleCache.Load(); ic != nil {
		firstHash = ic.tailHash
	}

	// Update the PTY buffer directly — inst.preview is no longer read by GetIdleState.
	if pa := cc.ptyAccess.Load(); pa != nil {
		pa.buffer.Clear()
		_, _ = pa.buffer.Write([]byte(tmuxOutputSmall + "\n  changed\n"))
	}
	_, _ = cc.GetIdleState()
	var secondHash uint64
	if ic := cc.idleCache.Load(); ic != nil {
		secondHash = ic.tailHash
	}

	if firstHash == secondHash {
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

// ---------------------------------------------------------------------------
// Unit tests — StatusChangeListener
// ---------------------------------------------------------------------------

// newControllerWithMockAndChannel returns a ClaudeController with an initialized
// statusCheckCh, suitable for StatusChangeListener tests that manually drive the
// runStatusChangeLoop goroutine.
func newControllerWithMockAndChannel(preview string) (*ClaudeController, *mockInstance, context.Context, context.CancelFunc) {
	inst := &mockInstance{title: "test", preview: preview}
	ctx, cancel := context.WithCancel(context.Background())
	cc := &ClaudeController{
		sessionName:   "test",
		instance:      inst,
		statusCheckCh: make(chan struct{}, 1),
	}
	cc.statusDetector.Store(detection.NewStatusDetector())
	cc.idleDetector.Store(detection.NewIdleDetector("test", nil))
	buf := NewCircularBuffer(256 * 1024)
	if preview != "" {
		_, _ = buf.Write([]byte(preview))
	}
	cc.ptyAccess.Store(NewPTYAccess("test", nil, buf))
	// Wire the lifecycle so runStatusChangeLoop receives the correct ctx.
	cc.lifecycle.Write(func(l *controllerLifecycle) {
		l.ctx = ctx
		l.cancel = cancel
	})
	return cc, inst, ctx, cancel
}

// TestClaudeController_StatusChangeListener_FiresOnStatusChange verifies that
// the listener is invoked when a status transition is detected after an output signal.
func TestClaudeController_StatusChangeListener_FiresOnStatusChange(t *testing.T) {
	t.Parallel()
	// Use content that produces a known status (StatusExecuting via "esc to interrupt").
	preview := tmuxOutputSmall // contains "esc to interrupt" → StatusExecuting
	cc, _, ctx, cancel := newControllerWithMockAndChannel(preview)
	defer cancel()

	fired := make(chan detection.DetectedStatus, 1)
	cc.AddStatusChangeListener(func(newStatus detection.DetectedStatus, _ string) {
		select {
		case fired <- newStatus:
		default:
		}
	})

	// Start the background goroutine.
	go cc.runStatusChangeLoop(ctx, make(chan struct{}))

	// Signal an output event.
	cc.statusCheckCh <- struct{}{}

	select {
	case got := <-fired:
		if got == detection.StatusUnknown {
			t.Errorf("expected a non-Unknown status, got %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for StatusChangeListener to fire")
	}
}

// TestClaudeController_StatusChangeListener_SuppressedOnNoChange verifies that
// the listener fires only once when the status doesn't change across two signals.
func TestClaudeController_StatusChangeListener_SuppressedOnNoChange(t *testing.T) {
	t.Parallel()
	preview := tmuxOutputSmall
	cc, _, ctx, cancel := newControllerWithMockAndChannel(preview)
	defer cancel()

	callCount := make(chan struct{}, 10)
	cc.AddStatusChangeListener(func(_ detection.DetectedStatus, _ string) {
		callCount <- struct{}{}
	})

	go cc.runStatusChangeLoop(ctx, make(chan struct{}))

	// Send two signals with the same preview content (same status both times).
	cc.statusCheckCh <- struct{}{}
	// Wait for first call to be processed before sending second signal.
	select {
	case <-callCount:
		// First call received — good.
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first StatusChangeListener call")
	}

	// Now send a second signal; status hasn't changed so listener must NOT fire again.
	cc.statusCheckCh <- struct{}{}

	// Wait for the goroutine to consume the second signal from the channel (without sleeping a
	// fixed duration). Once the channel is empty the goroutine has processed the signal and
	// decided — correctly — not to call the listener again.
	deadline := time.After(2 * time.Second)
	for len(cc.statusCheckCh) > 0 {
		select {
		case <-deadline:
			// Timed out waiting for channel to drain — fall through to the assertion below.
			goto checkResult
		default:
			time.Sleep(1 * time.Millisecond)
		}
	}
	// Give the goroutine a brief window (10ms) to potentially call the listener after draining.
	time.Sleep(10 * time.Millisecond)

checkResult:
	select {
	case <-callCount:
		t.Error("StatusChangeListener fired a second time for the same status")
	default:
		// Expected: no second call.
	}
}

// TestClaudeController_StatusChangeListener_NotCalledAfterStop verifies that
// the listener is not called after the context is cancelled (Stop).
func TestClaudeController_StatusChangeListener_NotCalledAfterStop(t *testing.T) {
	t.Parallel()
	preview := tmuxOutputSmall
	cc, _, ctx, cancel := newControllerWithMockAndChannel(preview)

	called := make(chan struct{}, 1)
	cc.AddStatusChangeListener(func(_ detection.DetectedStatus, _ string) {
		select {
		case called <- struct{}{}:
		default:
		}
	})

	loopDone := make(chan struct{})
	go cc.runStatusChangeLoop(ctx, loopDone)

	// Cancel the context (simulating Stop()).
	cancel()

	// Wait for the goroutine to actually observe ctx.Done() and return, rather
	// than sleeping a fixed duration that can be too short under load — a
	// short sleep here would let the still-running goroutine consume the
	// post-stop signal below and spuriously fire the listener.
	select {
	case <-loopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("runStatusChangeLoop did not exit within 2s after ctx cancellation")
	}

	// Send a signal after stop — listener must not be called.
	select {
	case cc.statusCheckCh <- struct{}{}:
	default:
	}

	// Allow time for any spurious delivery.
	select {
	case <-called:
		t.Error("StatusChangeListener called after Stop()")
	case <-time.After(200 * time.Millisecond):
		// Expected: silence after stop.
	}
}

// ---------------------------------------------------------------------------
// OSC title status override (osc-status-signals)
// ---------------------------------------------------------------------------

func TestGetCurrentStatus_OSCSpinnerOverridesFalseIdle(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("$ \x1b]0;⠋ working\x07")
	cc.started.Store(true)

	status, desc := cc.GetCurrentStatus()
	if status != detection.StatusExecuting {
		t.Errorf("GetCurrentStatus() = (%v, %q), want StatusExecuting (OSC spinner title over a bare prompt)", status, desc)
	}
}

func TestGetCurrentStatus_OSCIdleMarker_PromotesReadyOnlyText(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("$ \x1b]0;✳\x07")
	cc.started.Store(true)

	status, desc := cc.GetCurrentStatus()
	if status != detection.StatusIdle {
		t.Errorf("GetCurrentStatus() = (%v, %q), want StatusIdle (OSC ✳ marker over Ready/Unknown text)", status, desc)
	}
}

func TestGetCurrentStatus_OSCIdle_DoesNotOverrideActiveText(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("esc to interrupt\x1b]0;✳\x07")
	cc.started.Store(true)

	status, desc := cc.GetCurrentStatus()
	if status != detection.StatusExecuting {
		t.Errorf("GetCurrentStatus() = (%v, %q), want StatusExecuting (a stale/nested ✳ OSC title must not downgrade active text)", status, desc)
	}
}

func TestGetCurrentStatus_NoOSCTitle_FallsBackToTextPattern(t *testing.T) {
	t.Parallel()
	ccWithOSC, _ := newControllerWithMock(tmuxOutputSmall)
	ccWithOSC.started.Store(true)
	statusStarted, descStarted := ccWithOSC.GetCurrentStatus()

	ccNotStarted, _ := newControllerWithMock(tmuxOutputSmall)
	statusNotStarted, descNotStarted := ccNotStarted.GetCurrentStatus()

	// tmuxOutputSmall contains no OSC sequence, so classifyOSC never matches
	// regardless of cc.started — the result must be identical either way,
	// proving AC7 (no behavior change when no OSC title is present).
	if statusStarted != statusNotStarted || descStarted != descNotStarted {
		t.Errorf("result differs with no OSC title present: started=(%v,%q) not-started=(%v,%q)",
			statusStarted, descStarted, statusNotStarted, descNotStarted)
	}
}

func TestGetStatusAndIdleInfo_OSCPromotesIdleState(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("$ \x1b]0;⠋ working\x07")
	cc.started.Store(true)

	status, _, idleInfo, _ := cc.GetStatusAndIdleInfo()
	if status != detection.StatusExecuting {
		t.Errorf("GetStatusAndIdleInfo() status = %v, want StatusExecuting", status)
	}
	if idleInfo.State != detection.IdleStateActive {
		t.Errorf("GetStatusAndIdleInfo() idleInfo.State = %v, want IdleStateActive", idleInfo.State)
	}
}

func TestGetIdleState_OSCSpinnerMatchesGetStatusAndIdleInfo(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("$ \x1b]0;⠋ working\x07")
	cc.started.Store(true)

	state, _ := cc.GetIdleState()
	if state != detection.IdleStateActive {
		t.Errorf("GetIdleState() = %v, want IdleStateActive (consistent with GetStatusAndIdleInfo)", state)
	}
}

func TestClassifyOSC_StaleActivity_FallsBackToNone(t *testing.T) {
	t.Parallel()
	cc, _ := newControllerWithMock("$ \x1b]0;⠋ working\x07")
	cc.started.Store(true)

	id := cc.idleDetector.Load()
	id.InitializeFromTimestamp(time.Now().Add(-oscStaleThreshold - time.Second))

	if osc, ok := cc.classifyOSC("$ \x1b]0;⠋ working\x07"); ok {
		t.Errorf("classifyOSC() with stale activity = (%v, true), want (_, false)", osc)
	}

	// Recent activity: the spinner title is still classified normally — proves
	// the guard doesn't fire on legitimate in-progress work.
	id.InitializeFromTimestamp(time.Now())
	if _, ok := cc.classifyOSC("$ \x1b]0;⠋ working\x07"); !ok {
		t.Error("classifyOSC() with recent activity should still classify the OSC title")
	}
}

func TestApplyOSCStatusOverride_FullMatrix(t *testing.T) {
	t.Parallel()
	allStatuses := []detection.DetectedStatus{
		detection.StatusUnknown, detection.StatusReady, detection.StatusProcessing,
		detection.StatusNeedsApproval, detection.StatusInputRequired, detection.StatusError,
		detection.StatusTestsFailing, detection.StatusIdle, detection.StatusExecuting,
		detection.StatusSuccess, detection.StatusWaitingForAgent, detection.StatusCompacting,
	}
	executingPromotable := map[detection.DetectedStatus]bool{
		detection.StatusReady: true, detection.StatusUnknown: true,
		detection.StatusIdle: true, detection.StatusProcessing: true,
	}
	idlePromotable := map[detection.DetectedStatus]bool{
		detection.StatusReady: true, detection.StatusUnknown: true,
	}

	for _, s := range allStatuses {
		t.Run(s.String()+"/OSCStatusExecuting", func(t *testing.T) {
			got, _ := applyOSCStatusOverride(s, "orig", dtypes.OSCStatusExecuting)
			wantPromoted := executingPromotable[s]
			if wantPromoted && got != detection.StatusExecuting {
				t.Errorf("applyOSCStatusOverride(%v, OSCStatusExecuting) = %v, want StatusExecuting (promotable)", s, got)
			}
			if !wantPromoted && got != s {
				t.Errorf("applyOSCStatusOverride(%v, OSCStatusExecuting) = %v, want %v (never demoted/changed)", s, got, s)
			}
		})
		t.Run(s.String()+"/OSCStatusIdle", func(t *testing.T) {
			got, _ := applyOSCStatusOverride(s, "orig", dtypes.OSCStatusIdle)
			wantPromoted := idlePromotable[s]
			if wantPromoted && got != detection.StatusIdle {
				t.Errorf("applyOSCStatusOverride(%v, OSCStatusIdle) = %v, want StatusIdle (promotable)", s, got)
			}
			if !wantPromoted && got != s {
				t.Errorf("applyOSCStatusOverride(%v, OSCStatusIdle) = %v, want %v (never demoted/changed)", s, got, s)
			}
		})
	}

	t.Run("OSCStatusNone is always a no-op", func(t *testing.T) {
		for _, s := range allStatuses {
			got, gotDesc := applyOSCStatusOverride(s, "orig", dtypes.OSCStatusNone)
			if got != s || gotDesc != "orig" {
				t.Errorf("applyOSCStatusOverride(%v, OSCStatusNone) = (%v, %q), want (%v, %q)", s, got, gotDesc, s, "orig")
			}
		}
	})
}
