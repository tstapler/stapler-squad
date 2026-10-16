package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/scrollback"
)

// newTestScrollbackManager creates a *scrollback.ScrollbackManager backed by a temp
// storage directory, suitable for isolated tests.
func newTestScrollbackManager(t *testing.T) *scrollback.ScrollbackManager {
	t.Helper()
	cfg := scrollback.DefaultScrollbackConfig()
	cfg.StoragePath = t.TempDir()
	sm := scrollback.NewScrollbackManager(cfg)
	t.Cleanup(func() { _ = sm.Close() })
	return sm
}

func TestStripANSI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no escape sequences",
			in:   "plain text with no escapes\n",
			want: "plain text with no escapes\n",
		},
		{
			name: "CSI color codes",
			in:   "\x1b[31mred text\x1b[0m normal\n",
			want: "red text normal\n",
		},
		{
			name: "CSI cursor movement",
			in:   "line1\x1b[2K\x1b[1Aline2\n",
			want: "line1line2\n",
		},
		{
			name: "OSC window title (BEL terminated)",
			in:   "before\x1b]0;my title\x07after\n",
			want: "beforeafter\n",
		},
		{
			name: "G0 charset designation",
			in:   "t\x1b(Bh\x1b(Bi\x1b(Bn\x1b(Bk\x1b(Bi\x1b(Bn\x1b(Bg",
			want: "thinking",
		},
		{
			name: "mixed CSI + OSC + charset",
			in:   "\x1b]0;title\x07\x1b[1;32mgreen\x1b[0m \x1b(Btext\n",
			want: "green text\n",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stripANSI([]byte(tt.in))
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWriteReviewTranscriptFile_WritesStrippedContent(t *testing.T) {
	t.Parallel()
	sm := newTestScrollbackManager(t)
	workDir := t.TempDir()
	sessionUUID := "sess-abc-123"

	raw := "\x1b[31mhello\x1b[0m world\n\x1b]0;title\x07line two\n"
	if err := sm.AppendOutput(sessionUUID, []byte(raw)); err != nil {
		t.Fatalf("AppendOutput() error = %v", err)
	}

	relPath, cleanup, err := WriteReviewTranscriptFile(sm, sessionUUID, workDir, DefaultReviewTranscriptMaxBytes)
	if err != nil {
		t.Fatalf("WriteReviewTranscriptFile() error = %v", err)
	}
	defer cleanup()

	if relPath == "" {
		t.Fatal("expected non-empty relPath when scrollback is available")
	}
	if strings.Contains(relPath, string(os.PathSeparator)) {
		t.Errorf("relPath %q should be a bare file name relative to workDir, not contain a separator", relPath)
	}

	absPath := filepath.Join(workDir, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read written transcript file: %v", err)
	}

	got := string(data)
	if strings.Contains(got, "\x1b") {
		t.Errorf("written transcript still contains raw ESC bytes: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("written transcript missing expected content, got: %q", got)
	}
	if !strings.Contains(got, "line two") {
		t.Errorf("written transcript missing expected content, got: %q", got)
	}

	// cleanup() should remove the file.
	cleanup()
	if _, statErr := os.Stat(absPath); !os.IsNotExist(statErr) {
		t.Errorf("expected transcript file to be removed after cleanup(), stat err = %v", statErr)
	}
}

func TestWriteReviewTranscriptFile_TruncatesToMaxBytesKeepingTail(t *testing.T) {
	t.Parallel()
	sm := newTestScrollbackManager(t)
	workDir := t.TempDir()
	sessionUUID := "sess-long-999"

	// Build content clearly longer than maxBytes once ANSI-stripped.
	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		sb.WriteString("line-")
		sb.WriteString(strings.Repeat("x", 10))
		sb.WriteString("\n")
	}
	raw := sb.String()
	if err := sm.AppendOutput(sessionUUID, []byte(raw)); err != nil {
		t.Fatalf("AppendOutput() error = %v", err)
	}

	const maxBytes = 500
	relPath, cleanup, err := WriteReviewTranscriptFile(sm, sessionUUID, workDir, maxBytes)
	if err != nil {
		t.Fatalf("WriteReviewTranscriptFile() error = %v", err)
	}
	defer cleanup()

	if relPath == "" {
		t.Fatal("expected non-empty relPath")
	}

	data, err := os.ReadFile(filepath.Join(workDir, relPath))
	if err != nil {
		t.Fatalf("failed to read written transcript file: %v", err)
	}

	if int64(len(data)) > maxBytes+int64(len(reviewTranscriptTruncationMarker)) {
		t.Errorf("written transcript is larger than maxBytes+marker: got %d bytes, maxBytes=%d", len(data), maxBytes)
	}
	if !strings.HasPrefix(string(data), reviewTranscriptTruncationMarker) {
		t.Errorf("expected truncation marker prefix, got: %q", string(data)[:min(60, len(data))])
	}
	// The tail (most recent activity) should be preserved -- the very last line written.
	if !strings.Contains(string(data), "line-xxxxxxxxxx") {
		t.Errorf("expected truncated transcript to retain tail content, got: %q", string(data))
	}
}

func TestWriteReviewTranscriptFile_NoScrollbackReturnsEmpty(t *testing.T) {
	t.Parallel()
	sm := newTestScrollbackManager(t)
	workDir := t.TempDir()

	relPath, cleanup, err := WriteReviewTranscriptFile(sm, "session-that-never-ran", workDir, DefaultReviewTranscriptMaxBytes)
	if err != nil {
		t.Fatalf("WriteReviewTranscriptFile() error = %v, want nil (enrichment-only, should not fail)", err)
	}
	if relPath != "" {
		t.Errorf("relPath = %q, want empty string when no scrollback exists", relPath)
	}
	if cleanup == nil {
		t.Fatal("cleanup func should never be nil")
	}
	// Should be safe to call even though nothing was written.
	cleanup()

	entries, readErr := os.ReadDir(workDir)
	if readErr != nil {
		t.Fatalf("failed to read workDir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files written to workDir, found %d entries", len(entries))
	}
}

func TestWriteReviewTranscriptFile_NilManagerReturnsEmpty(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	relPath, cleanup, err := WriteReviewTranscriptFile(nil, "some-session", workDir, DefaultReviewTranscriptMaxBytes)
	if err != nil {
		t.Fatalf("WriteReviewTranscriptFile() error = %v, want nil", err)
	}
	if relPath != "" {
		t.Errorf("relPath = %q, want empty string when scrollback manager is nil", relPath)
	}
	cleanup() // must not panic
}

func TestWriteReviewTranscriptFile_EmptyArgsReturnEmpty(t *testing.T) {
	t.Parallel()
	sm := newTestScrollbackManager(t)
	workDir := t.TempDir()

	tests := []struct {
		name        string
		sessionUUID string
		workDir     string
	}{
		{name: "empty session UUID", sessionUUID: "", workDir: workDir},
		{name: "empty work dir", sessionUUID: "some-session", workDir: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			relPath, cleanup, err := WriteReviewTranscriptFile(sm, tt.sessionUUID, tt.workDir, DefaultReviewTranscriptMaxBytes)
			if err != nil {
				t.Fatalf("WriteReviewTranscriptFile() error = %v, want nil", err)
			}
			if relPath != "" {
				t.Errorf("relPath = %q, want empty string", relPath)
			}
			cleanup()
		})
	}
}
