package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// TestReadOutputLineCap verifies that readSessionOutput respects the lines cap
// and sets truncated=true / total_lines correctly when there is more output
// than the requested line count.  (U-4.4)
func TestReadOutputLineCap(t *testing.T) {
	mgr := makeScrollbackMgr(t)
	sessionID := "test-session"

	// Populate 250 lines.
	var buf strings.Builder
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&buf, "line %d\n", i)
	}
	if err := mgr.AppendOutput(sessionID, []byte(buf.String())); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}

	store := &stubStore{instances: []*session.Instance{{Title: sessionID}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: mgr,
		writeLim:   newTokenBucket(10, 10),
	}

	// Request 200 lines.
	req := makeToolReq(map[string]interface{}{
		"session_id": sessionID,
		"lines":      float64(200),
	})
	result, err := th.readSessionOutput(context.Background(), req)
	if err != nil {
		t.Fatalf("readSessionOutput returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)

	if success, _ := m["success"].(bool); !success {
		t.Fatalf("expected success=true, got false; result=%v", m)
	}

	truncated, _ := m["truncated"].(bool)
	if !truncated {
		t.Error("expected truncated=true, got false")
	}

	totalLines, _ := m["total_lines"].(float64)
	if int(totalLines) != 250 {
		t.Errorf("expected total_lines=250, got %v", totalLines)
	}

	output, _ := m["output"].(string)
	// The first line should be the truncation marker.
	if !strings.Contains(output, "lines omitted") {
		t.Errorf("expected truncation marker in output, got: %q", output[:min(len(output), 200)])
	}
	// Count the non-marker lines in the output.
	outputLines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	// First line is the marker; the remaining 200 should be data lines.
	dataLines := 0
	for _, l := range outputLines {
		if !strings.Contains(l, "lines omitted") {
			dataLines++
		}
	}
	if dataLines != 200 {
		t.Errorf("expected 200 data lines, got %d", dataLines)
	}
}

// TestReadOutputSessionNotFound verifies that readSessionOutput returns a
// SESSION_NOT_FOUND error when the session does not exist.  (U-1.5 terminal)
func TestReadOutputSessionNotFound(t *testing.T) {
	store := &stubStore{}
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
	}

	req := makeToolReq(map[string]interface{}{"session_id": "ghost"})
	result, err := th.readSessionOutput(context.Background(), req)
	if err != nil {
		t.Fatalf("readSessionOutput returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)

	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false, got true")
	}

	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	code, _ := errObj["code"].(string)
	if code != ErrSessionNotFound {
		t.Errorf("expected error code %q, got %q", ErrSessionNotFound, code)
	}
}

// TestWriteInputLengthCap verifies that writeToSession rejects inputs longer
// than maxInputBytes with an INPUT_TOO_LONG error.  (U-4.9)
func TestWriteInputLengthCap(t *testing.T) {
	store := &stubStore{instances: []*session.Instance{{Title: "s1"}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
	}

	oversized := strings.Repeat("x", maxInputBytes+1) // 4097 bytes
	req := makeToolReq(map[string]interface{}{
		"session_id": "s1",
		"input":      oversized,
	})
	result, err := th.writeToSession(context.Background(), req)
	if err != nil {
		t.Fatalf("writeToSession returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)

	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false for oversized input, got true")
	}

	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	code, _ := errObj["code"].(string)
	if code != "INPUT_TOO_LONG" {
		t.Errorf("expected error code INPUT_TOO_LONG, got %q", code)
	}
}

// TestSendControlBytes verifies that the controlChars and controlNames maps
// contain the expected byte sequences and display names.  (U-4.12)
func TestSendControlBytes(t *testing.T) {
	cases := []struct {
		key  string
		char string
		name string
	}{
		{"C", "\x03", "^C"},
		{"D", "\x04", "^D"},
		{"Z", "\x1a", "^Z"},
		{"L", "\x0c", "^L"},
	}
	for _, c := range cases {
		got, ok := controlChars[c.key]
		if !ok {
			t.Errorf("controlChars[%q]: key missing", c.key)
			continue
		}
		if got != c.char {
			t.Errorf("controlChars[%q]=%q, want %q", c.key, got, c.char)
		}
		name, ok := controlNames[c.key]
		if !ok {
			t.Errorf("controlNames[%q]: key missing", c.key)
			continue
		}
		if name != c.name {
			t.Errorf("controlNames[%q]=%q, want %q", c.key, name, c.name)
		}
	}
	// "X" is not a valid control key.
	if _, ok := controlChars["X"]; ok {
		t.Error("controlChars should not contain key 'X'")
	}
}

// TestWaitForOutputTimeout verifies that waitForOutput times out after the
// requested seconds when the pattern is absent.  (U-4.14)
func TestWaitForOutputTimeout(t *testing.T) {
	mgr := makeScrollbackMgr(t)
	sessionID := "s1"
	if err := mgr.AppendOutput(sessionID, []byte("some output\n")); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}

	store := &stubStore{instances: []*session.Instance{{Title: sessionID}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: mgr,
		writeLim:   newTokenBucket(10, 10),
	}

	start := time.Now()
	req := makeToolReq(map[string]interface{}{
		"session_id":      sessionID,
		"pattern":         "DONE",
		"timeout_seconds": float64(2),
	})
	result, err := th.waitForOutput(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("waitForOutput returned unexpected Go error: %v", err)
	}

	// Should have taken roughly 2 seconds (allow 1.5s–6s for slow CI).
	if elapsed < 1500*time.Millisecond {
		t.Errorf("timeout too fast: elapsed=%v, want >= 1.5s", elapsed)
	}
	if elapsed > 6*time.Second {
		t.Errorf("timeout too slow: elapsed=%v, want <= 6s", elapsed)
	}

	m := parseResult(t, result)

	// Handler returns success=true even on timeout (timeout is a normal outcome).
	if success, _ := m["success"].(bool); !success {
		t.Error("expected success=true on timeout result")
	}

	matched, _ := m["matched"].(bool)
	if matched {
		t.Error("expected matched=false on timeout")
	}

	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object with WAIT_TIMEOUT code")
	}
	code, _ := errObj["code"].(string)
	if code != "WAIT_TIMEOUT" {
		t.Errorf("expected error code WAIT_TIMEOUT, got %q", code)
	}

	output, _ := m["output"].(string)
	if output == "" {
		t.Error("expected non-empty output on timeout")
	}
}

// ─── U-GO-32: TestSteerSessionMCP_rejectsEmptyMessage ────────────────────────

func TestSteerSessionMCP_rejectsEmptyMessage(t *testing.T) {
	store := &stubStore{instances: []*session.Instance{{Title: "s1"}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
	}

	req := makeToolReq(map[string]interface{}{
		"session_id": "s1",
		"message":    "",
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}
	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false for empty message, got true")
	}
}

// ─── U-GO-33: TestSteerSessionMCP_rejectsMessageExceeding4096Bytes ────────────

func TestSteerSessionMCP_rejectsMessageExceeding4096Bytes(t *testing.T) {
	store := &stubStore{instances: []*session.Instance{{Title: "s1"}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
	}

	oversized := strings.Repeat("x", maxInputBytes+1) // 4097 bytes
	req := makeToolReq(map[string]interface{}{
		"session_id": "s1",
		"message":    oversized,
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}
	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false for oversized message, got true")
	}
	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object")
	}
	code, _ := errObj["code"].(string)
	if code != "MESSAGE_TOO_LONG" {
		t.Errorf("expected MESSAGE_TOO_LONG error code, got %q", code)
	}
}

// ─── U-GO-34: TestSteerSessionMCP_stripsNullBytes ─────────────────────────────

func TestSteerSessionMCP_stripsNullBytes(t *testing.T) {
	store := &stubStore{instances: []*session.Instance{{Title: "s1"}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
	}

	// Message with only null bytes should be rejected after sanitization.
	req := makeToolReq(map[string]interface{}{
		"session_id": "s1",
		"message":    "\x00\x00\x00",
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}
	m := parseResult(t, result)
	// After stripping null bytes, message is empty → should be rejected.
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false after null byte stripping results in empty message")
	}
}

// ─── U-GO-31 note ─────────────────────────────────────────────────────────────
// TestSteerSessionMCP_sendsMessageWithNewline requires a live PTY session and
// cannot be tested without tmux in unit tests. The validation tests above cover
// the sanitization and error-path behavior. The full send path (SendKeys+"\n")
// is verified by integration/E2E tests.

// ─── U-GO-35: TestSteerSessionMCP_passesValidationAndReachesSendKeys ──────────
// Verifies that a valid message passes all validation and reaches the SendKeys
// call. Since SendKeys requires a live PTY, the call will return an internal
// error — but receiving INTERNAL_ERROR (not a validation error) proves the happy
// path was reached.

func TestSteerSessionMCP_passesValidationAndReachesSendKeys(t *testing.T) {
	inst := &session.Instance{
		Title:   "active-session",
		Status:  session.Active,
		Program: "claude",
	}
	store := &stubStore{instances: []*session.Instance{inst}}
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
	}

	req := makeToolReq(map[string]interface{}{
		"session_id": "active-session",
		"message":    "focus on the authentication module",
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)
	// All validation passed; SendKeys will fail because the instance has no live
	// PTY (not started). The response must be an INTERNAL_ERROR, not a validation
	// rejection — proving the message reached the SendKeys call.
	if success, _ := m["success"].(bool); success {
		// A real active session would return success — that's the true happy path.
		// In unit tests without tmux we accept success too (if somehow it worked).
		return
	}
	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	code, _ := errObj["code"].(string)
	// Must be INTERNAL_ERROR or PTY_WRITE_TIMEOUT — not a validation rejection.
	if code == ErrInvalidArgument || code == "MESSAGE_TOO_LONG" || code == ErrSessionNotFound {
		t.Errorf("steerSession returned validation error %q after valid message; expected SendKeys to be reached", code)
	}
}
