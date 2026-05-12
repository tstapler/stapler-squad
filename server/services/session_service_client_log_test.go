package services

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"connectrpc.com/connect"
)

// newLogClientEventsRequest wraps entries into a ConnectRPC request.
func newLogClientEventsRequest(entries ...*sessionv1.ClientLogEntry) *connect.Request[sessionv1.LogClientEventsRequest] {
	return connect.NewRequest(&sessionv1.LogClientEventsRequest{
		Entries: entries,
	})
}

// captureInfoLog temporarily redirects the slog default logger to a buffer
// and returns a function that restores it and returns the captured output.
func captureInfoLog() func() string {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := slog.Default()
	slog.SetDefault(slog.New(h))
	return func() string {
		slog.SetDefault(original)
		return buf.String()
	}
}

// captureErrorLog captures slog output (same handler; error-level calls are filtered in).
func captureErrorLog() func() string {
	return captureInfoLog()
}

// minimalService returns a SessionService that is usable for logClientEntry tests
// (it only needs the LogClientEvents handler, which has no struct-field dependencies).
func minimalService() *SessionService {
	return &SessionService{}
}

// UT-B-01
func TestBrowserLog_ValidSingleEntry(t *testing.T) {
	svc := minimalService()
	req := newLogClientEventsRequest(&sessionv1.ClientLogEntry{
		Level:   "log",
		Message: "hello world",
	})
	resp, err := svc.LogClientEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// UT-B-02
func TestBrowserLog_ValidBatch(t *testing.T) {
	svc := minimalService()
	entries := make([]*sessionv1.ClientLogEntry, 50)
	for i := range 50 {
		entries[i] = &sessionv1.ClientLogEntry{
			Level:   "warn",
			Message: "entry",
		}
	}
	req := connect.NewRequest(&sessionv1.LogClientEventsRequest{Entries: entries})
	_, err := svc.LogClientEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// UT-B-03
func TestBrowserLog_EmptyEntries(t *testing.T) {
	svc := minimalService()
	req := newLogClientEventsRequest()
	_, err := svc.LogClientEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// UT-B-09: message with newline is sanitized
func TestBrowserLog_LogInjection_Newline(t *testing.T) {
	restore := captureInfoLog()
	logClientEntry(&sessionv1.ClientLogEntry{
		Level:   "log",
		Message: "line1\nline2",
	})
	got := restore()
	if strings.Contains(got, "\n[client-log]") {
		t.Errorf("log injection: got unescaped newline in output: %q", got)
	}
	if !strings.Contains(got, `\n`) {
		t.Errorf("expected escaped newline in output, got: %q", got)
	}
}

// UT-B-10: message with carriage return is sanitized
func TestBrowserLog_LogInjection_CarriageReturn(t *testing.T) {
	restore := captureInfoLog()
	logClientEntry(&sessionv1.ClientLogEntry{
		Level:   "log",
		Message: "line1\rline2",
	})
	got := restore()
	if strings.Contains(got, "\r") {
		t.Errorf("log injection: got unescaped carriage return in output: %q", got)
	}
	if !strings.Contains(got, `\r`) {
		t.Errorf("expected escaped carriage return in output, got: %q", got)
	}
}

// UT-B-11: message > 200 chars is truncated with ellipsis
func TestBrowserLog_MessageTruncation(t *testing.T) {
	restore := captureInfoLog()
	msg := strings.Repeat("a", 300)
	logClientEntry(&sessionv1.ClientLogEntry{
		Level:   "log",
		Message: msg,
	})
	got := restore()
	// truncated result is 200 runes + "…" (3 UTF-8 bytes)
	// the logged message in the output should not contain the 201st char
	if strings.Contains(got, strings.Repeat("a", 201)) {
		t.Errorf("message was not truncated: output contains 201 'a' chars")
	}
	if !strings.Contains(got, "…") {
		t.Errorf("expected ellipsis in truncated output, got: %q", got)
	}
}

// UT-B-12: userAgent > 80 chars is truncated
func TestBrowserLog_UAShortened(t *testing.T) {
	restore := captureInfoLog()
	ua := strings.Repeat("u", 200)
	logClientEntry(&sessionv1.ClientLogEntry{
		Level:     "log",
		Message:   "test",
		UserAgent: ua,
	})
	got := restore()
	if strings.Contains(got, strings.Repeat("u", 81)) {
		t.Errorf("user agent was not truncated: output contains 81+ 'u' chars")
	}
}

// UT-B-13: level "error" routes to ErrorLog
func TestBrowserLog_ErrorLevelRoutesToErrorLog(t *testing.T) {
	restore := captureErrorLog()
	logClientEntry(&sessionv1.ClientLogEntry{
		Level:   "error",
		Message: "something broke",
	})
	got := restore()
	if !strings.Contains(got, "[client-log]") {
		t.Errorf("expected [client-log] prefix in error log, got: %q", got)
	}
	if !strings.Contains(got, "error") {
		t.Errorf("expected level 'error' in log output, got: %q", got)
	}
}

// UT-B-14: level "log" routes to InfoLog
func TestBrowserLog_OtherLevelRoutesToInfoLog(t *testing.T) {
	restore := captureInfoLog()
	logClientEntry(&sessionv1.ClientLogEntry{
		Level:   "log",
		Message: "info message",
	})
	got := restore()
	if !strings.Contains(got, "[client-log]") {
		t.Errorf("expected [client-log] prefix in info log, got: %q", got)
	}
}

// UT-B-15: entry without sessionId/url does not panic
func TestBrowserLog_MissingOptionalFields(t *testing.T) {
	restore := captureInfoLog()
	defer restore()
	// Should not panic
	logClientEntry(&sessionv1.ClientLogEntry{
		Level:   "debug",
		Message: "minimal entry",
	})
}

// sanitizeClientLogField unit tests

func TestSanitizeClientLogField_TruncatesLongString(t *testing.T) {
	s := strings.Repeat("x", 300)
	result := sanitizeClientLogField(s, 200)
	runes := []rune(result)
	// 200 chars + "…" (1 rune)
	if len(runes) != 201 {
		t.Errorf("want 201 runes, got %d", len(runes))
	}
	if !strings.HasSuffix(result, "…") {
		t.Errorf("want ellipsis suffix, got %q", result)
	}
}

func TestSanitizeClientLogField_ExactLength(t *testing.T) {
	s := strings.Repeat("x", 200)
	result := sanitizeClientLogField(s, 200)
	if result != s {
		t.Errorf("want unchanged string at exact length, got different result")
	}
}

func TestSanitizeClientLogField_EscapesNewlines(t *testing.T) {
	result := sanitizeClientLogField("a\nb", 100)
	if strings.Contains(result, "\n") {
		t.Errorf("newline not escaped: %q", result)
	}
}

func TestSanitizeClientLogField_EscapesCarriageReturn(t *testing.T) {
	result := sanitizeClientLogField("a\rb", 100)
	if strings.Contains(result, "\r") {
		t.Errorf("carriage return not escaped: %q", result)
	}
}

func TestSanitizeClientLogField_EscapesTabs(t *testing.T) {
	result := sanitizeClientLogField("a\tb", 100)
	if strings.Contains(result, "\t") {
		t.Errorf("tab not escaped: %q", result)
	}
}
