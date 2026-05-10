package executor

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// fakeHook is a test AuditHook that captures all received entries.
type fakeHook struct {
	entries []AuditEntry
}

func (f *fakeHook) OnExec(entry AuditEntry) {
	f.entries = append(f.entries, entry)
}

// testSlogHandler captures slog log records for assertion in tests.
type testSlogHandler struct {
	records []slog.Record
}

func (h *testSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *testSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *testSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *testSlogHandler) WithGroup(_ string) slog.Handler      { return h }

// T-UNIT-021: WithAuditHook_receivesEntryOnCompletion
func TestWithAuditHook_receivesEntryOnCompletion(t *testing.T) {
	t.Parallel()

	hook := &fakeHook{}
	ctx := WithAuditHook(context.Background(), hook)

	cmd := New(ctx, helperBin, []string{"--exit-code", "0"})
	err := cmd.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hook.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(hook.entries))
	}
	e := hook.entries[0]
	if e.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", e.ExitCode)
	}
	if len(e.Command) == 0 || e.Command[0] != helperBin {
		t.Errorf("expected Command[0] == helperBin, got %v", e.Command)
	}
	if e.Duration <= 0 {
		t.Errorf("expected Duration > 0, got %v", e.Duration)
	}
	if e.StartTime.IsZero() {
		t.Error("expected non-zero StartTime")
	}
	if e.KilledByCtx {
		t.Error("expected KilledByCtx == false")
	}
}

// T-UNIT-022: WithAuditHook_noHook_noPanic
func TestWithAuditHook_noHook_noPanic(t *testing.T) {
	t.Parallel()

	// Context without audit hook — should not panic.
	cmd := New(context.Background(), helperBin, []string{"--exit-code", "0"})
	err := cmd.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// T-UNIT-023: LoggingAuditHook_emitsAtDebugOnSuccess
func TestLoggingAuditHook_emitsAtDebugOnSuccess(t *testing.T) {
	t.Parallel()

	handler := &testSlogHandler{}
	logger := slog.New(handler)
	hook := &LoggingAuditHook{Logger: logger}

	hook.OnExec(AuditEntry{
		ExitCode:    0,
		KilledByCtx: false,
		Command:     []string{"echo"},
		StartTime:   time.Now(),
		Duration:    time.Millisecond,
	})

	if len(handler.records) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(handler.records))
	}
	if handler.records[0].Level != slog.LevelDebug {
		t.Errorf("expected Debug level, got %v", handler.records[0].Level)
	}
}

// T-UNIT-024: LoggingAuditHook_escalatesToInfoOnNonZeroExit
func TestLoggingAuditHook_escalatesToInfoOnNonZeroExit(t *testing.T) {
	t.Parallel()

	handler := &testSlogHandler{}
	logger := slog.New(handler)
	hook := &LoggingAuditHook{Logger: logger}

	hook.OnExec(AuditEntry{
		ExitCode:    1,
		KilledByCtx: false,
		Command:     []string{"false"},
		StartTime:   time.Now(),
		Duration:    time.Millisecond,
	})

	if len(handler.records) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(handler.records))
	}
	if handler.records[0].Level != slog.LevelInfo {
		t.Errorf("expected Info level for non-zero exit, got %v", handler.records[0].Level)
	}
}

// TestLoggingAuditHook_escalatesToInfoOnKill ensures kills also escalate.
func TestLoggingAuditHook_escalatesToInfoOnKill(t *testing.T) {
	t.Parallel()

	handler := &testSlogHandler{}
	hook := &LoggingAuditHook{Logger: slog.New(handler)}

	hook.OnExec(AuditEntry{
		ExitCode:    0,
		KilledByCtx: true,
		Command:     []string{"sleep", "10"},
		StartTime:   time.Now(),
		Duration:    time.Millisecond,
	})

	if len(handler.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(handler.records))
	}
	if handler.records[0].Level != slog.LevelInfo {
		t.Errorf("expected Info for killed process, got %v", handler.records[0].Level)
	}
}

// TestLoggingAuditHook_nilLogger uses slog.Default() without panic.
func TestLoggingAuditHook_nilLogger(t *testing.T) {
	t.Parallel()

	hook := &LoggingAuditHook{Logger: nil}
	// Should not panic; falls back to slog.Default().
	hook.OnExec(AuditEntry{Command: []string{"echo"}, StartTime: time.Now()})
}

// TestEmitAudit_noOpWithoutHook verifies emitAudit is a no-op when no hook.
func TestEmitAudit_noOpWithoutHook(t *testing.T) {
	t.Parallel()
	// Should not panic.
	emitAudit(context.Background(), AuditEntry{Command: []string{"echo"}})
}

// TestRedactArgs verifies secret scrubbing logic.
func TestRedactArgs(t *testing.T) {
	t.Parallel()

	argv := []string{"myprogram", "--token", "s3cr3t", "--user", "alice"}
	result := redactArgs(argv, []int{2})

	if result[2] != "<redacted>" {
		t.Errorf("expected <redacted> at index 2, got %q", result[2])
	}
	// Other positions unchanged.
	if result[0] != "myprogram" || result[1] != "--token" {
		t.Errorf("non-redacted positions changed: %v", result)
	}
	// Original slice not mutated.
	if argv[2] != "s3cr3t" {
		t.Error("redactArgs mutated the input slice")
	}
}

// TestRedactArgs_emptyIndices verifies no-op on empty indices.
func TestRedactArgs_emptyIndices(t *testing.T) {
	t.Parallel()

	argv := []string{"echo", "hello"}
	result := redactArgs(argv, nil)
	// It's OK if the same slice is returned for efficiency (no copy needed).
	if result[1] != "hello" {
		t.Errorf("expected 'hello', got %q", result[1])
	}
}

// TestRedactArgs_outOfRangeIndex verifies no panic on out-of-range indices.
func TestRedactArgs_outOfRangeIndex(t *testing.T) {
	t.Parallel()

	argv := []string{"echo", "hello"}
	// Should not panic on out-of-range index.
	result := redactArgs(argv, []int{99, -1})
	if len(result) != 2 {
		t.Errorf("expected len 2, got %d", len(result))
	}
}

// T-UNIT-027: BenchmarkAuditEmit — overhead measurement
func BenchmarkAuditEmit(b *testing.B) {
	// No-hook fast path: emitAudit should be essentially free.
	ctx := context.Background()
	entry := AuditEntry{
		Command:   []string{"echo", "hi"},
		StartTime: time.Now(),
		Duration:  time.Millisecond,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		emitAudit(ctx, entry)
	}
}

// BenchmarkAuditEmit_withHook measures overhead with a real hook attached.
func BenchmarkAuditEmit_withHook(b *testing.B) {
	hook := &fakeHook{}
	ctx := WithAuditHook(context.Background(), hook)
	entry := AuditEntry{
		Command:   []string{"echo", "hi"},
		StartTime: time.Now(),
		Duration:  time.Millisecond,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		emitAudit(ctx, entry)
	}
}

// TestAuditHookFromCtx verifies round-trip extraction.
func TestAuditHookFromCtx(t *testing.T) {
	t.Parallel()

	hook := &fakeHook{}
	ctx := WithAuditHook(context.Background(), hook)

	extracted := AuditHookFromCtx(ctx)
	if extracted != hook {
		t.Error("extracted hook is not the same as the one stored")
	}

	// Context without hook returns nil.
	noHook := AuditHookFromCtx(context.Background())
	if noHook != nil {
		t.Errorf("expected nil hook from plain context, got %v", noHook)
	}
}
