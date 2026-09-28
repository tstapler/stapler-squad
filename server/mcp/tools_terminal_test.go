package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
	"github.com/tstapler/stapler-squad/session/sendkeysguard"
)

// TestMCPPackage_NoDirectSendKeysPlusEnterConcatenation is the server/mcp
// instance of the BUG-031 structural guard (see sendkeysguard's doc comment
// and session/pane_submit_test.go's sibling test) — the single-write pattern
// this guards against was originally reintroduced here, in writeToSession,
// runCommand, and steerSession, not in the session package.
func TestMCPPackage_NoDirectSendKeysPlusEnterConcatenation(t *testing.T) {
	t.Parallel()
	sendkeysguard.CheckNoSingleWriteEnterConcatenation(t, ".")
}

// TestSubmitErrResult_MapsErrSubmitNotConfirmedToDistinctErrorCode is the
// direct AC-5 test: a swallowed submit (session.ErrSubmitNotConfirmed) must
// surface as an explicit, distinguishable error — not the generic internal
// error every other SendKeys failure gets, and not success.
func TestSubmitErrResult_MapsErrSubmitNotConfirmedToDistinctErrorCode(t *testing.T) {
	res := submitErrResult(session.ErrSubmitNotConfirmed, "message")
	m := parseResult(t, res)

	if success, _ := m["success"].(bool); success {
		t.Fatal("expected success=false for a swallowed submit")
	}
	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	if code, _ := errObj["code"].(string); code != "SUBMIT_NOT_CONFIRMED" {
		t.Errorf("expected code SUBMIT_NOT_CONFIRMED, got %q", code)
	}
}

// TestSubmitErrResult_MapsDeadlineExceededToTimeoutCode proves the timeout
// path is distinguishable from both a swallowed submit and a generic failure.
func TestSubmitErrResult_MapsDeadlineExceededToTimeoutCode(t *testing.T) {
	res := submitErrResult(context.DeadlineExceeded, "command")
	m := parseResult(t, res)

	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	if code, _ := errObj["code"].(string); code != "PTY_WRITE_TIMEOUT" {
		t.Errorf("expected code PTY_WRITE_TIMEOUT, got %q", code)
	}
}

// TestSubmitErrResult_MapsOtherErrorsToInternalError verifies the fallback
// case doesn't silently masquerade as one of the two distinct error codes.
func TestSubmitErrResult_MapsOtherErrorsToInternalError(t *testing.T) {
	res := submitErrResult(fmt.Errorf("pty closed"), "input")
	m := parseResult(t, res)

	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	if code, _ := errObj["code"].(string); code != ErrInternalError {
		t.Errorf("expected code %q, got %q", ErrInternalError, code)
	}
}

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

// TestReadOutputSessionNotReady verifies readSessionOutput returns
// SESSION_NOT_READY (not an empty-output success) when the session exists but
// its scrollback sequence hasn't advanced since creation -- i.e. no bytes
// have ever arrived from the PTY/tmux stream, distinct from a command that
// legitimately printed nothing (see TestReadOutputSucceeds_When_ReadyWithNoNewBytes).
func TestReadOutputSessionNotReady(t *testing.T) {
	sessionID := "not-ready-session"
	store := &stubStore{instances: []*session.Instance{{Title: sessionID}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t), // no AppendOutput call -- sequence stays 0
		writeLim:   newTokenBucket(10, 10),
	}

	req := makeToolReq(map[string]interface{}{"session_id": sessionID})
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
	if code, _ := errObj["code"].(string); code != ErrSessionNotReady {
		t.Errorf("expected error code %q, got %q", ErrSessionNotReady, code)
	}
}

// TestReadOutputSucceeds_When_ReadyWithNoNewBytes verifies a session whose
// scrollback sequence has already advanced (has emitted at least one byte
// since creation) is never flagged SESSION_NOT_READY, even when the most
// recent read finds no output -- e.g. a command like `true` that legitimately
// prints nothing.
func TestReadOutputSucceeds_When_ReadyWithNoNewBytes(t *testing.T) {
	mgr := makeScrollbackMgr(t)
	sessionID := "ready-session"
	if err := mgr.AppendOutput(sessionID, []byte("$ ")); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}

	store := &stubStore{instances: []*session.Instance{{Title: sessionID}}}
	th := &terminalHandlers{
		store:      store,
		scrollback: mgr,
		writeLim:   newTokenBucket(10, 10),
	}

	req := makeToolReq(map[string]interface{}{"session_id": sessionID})
	result, err := th.readSessionOutput(context.Background(), req)
	if err != nil {
		t.Fatalf("readSessionOutput returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)
	if success, _ := m["success"].(bool); !success {
		t.Fatalf("expected success=true, got false; result=%v", m)
	}
}

// TestSessionNotReadyResult verifies the readiness check shared by
// readSessionOutput and runCommand (sessionNotReadyResult) directly, so the
// two call sites cannot silently diverge from each other.
func TestSessionNotReadyResult(t *testing.T) {
	mgr := makeScrollbackMgr(t)
	th := &terminalHandlers{scrollback: mgr}

	if res := th.sessionNotReadyResult("never-appended"); res == nil {
		t.Fatal("expected non-nil SESSION_NOT_READY result for a session with no scrollback yet")
	} else if errObj, _ := parseResult(t, res)["error"].(map[string]interface{}); errObj == nil || errObj["code"] != ErrSessionNotReady {
		t.Errorf("expected error code %q, got %v", ErrSessionNotReady, errObj)
	}

	if err := mgr.AppendOutput("has-output", []byte("$ ")); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}
	if res := th.sessionNotReadyResult("has-output"); res != nil {
		t.Errorf("expected nil (ready) once scrollback has advanced, got %v", res)
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

// --- BUG-035: findInstance must prefer the live instance over LoadInstances() ---

// fakeLiveInstanceFinder is a test double implementing liveInstanceFinder,
// returning a scripted instance (or nil) per session ID.
type fakeLiveInstanceFinder struct {
	instances map[string]*session.Instance
}

func (f *fakeLiveInstanceFinder) FindLiveInstance(id string) *session.Instance {
	return f.instances[id]
}

// TestFindInstance_should_returnLiveInstance_When_LiveFinderHasIt is the
// direct regression test for BUG-035: every MCP tool that mutates a
// session's terminal (write_to_session, send_control, run_command,
// steer_session) called Storage.LoadInstances() on every single invocation
// via findInstance — but LoadInstances() always defers Start() ("so a bulk
// load (server startup) doesn't block"), so the Instance handed back was
// never the live one and SendKeys deterministically failed with "instance
// has not been started", regardless of how long the server had actually
// been running. findInstance must now prefer th.live (the real live
// instance) over the LoadInstances()-reconstructed one when both exist —
// proven here by returning two distinguishable instances (different
// WorkingDir) for the same session ID from each source and asserting the
// live one wins.
func TestFindInstance_should_returnLiveInstance_When_LiveFinderHasIt(t *testing.T) {
	liveInst := &session.Instance{Title: "s1", WorkingDir: "/live/path"}
	staleInst := &session.Instance{Title: "s1", WorkingDir: "/stale/reconstructed/path"}

	th := &terminalHandlers{
		store: &stubStore{instances: []*session.Instance{staleInst}},
		live:  &fakeLiveInstanceFinder{instances: map[string]*session.Instance{"s1": liveInst}},
	}

	got, errRes := th.findInstance("s1")
	if errRes != nil {
		t.Fatalf("findInstance returned an error result: %+v", errRes)
	}
	if got != liveInst {
		t.Errorf("expected the live instance (WorkingDir=%q) to be returned, got WorkingDir=%q", liveInst.WorkingDir, got.WorkingDir)
	}
}

// TestFindInstance_should_fallBackToStore_When_LiveFinderDoesNotHaveIt
// verifies the fallback path: a session th.live doesn't track (e.g. outside
// the review queue poller's scope) must still be found via the pre-fix
// LoadInstances() path rather than returning SESSION_NOT_FOUND.
func TestFindInstance_should_fallBackToStore_When_LiveFinderDoesNotHaveIt(t *testing.T) {
	staleInst := &session.Instance{Title: "s1"}
	th := &terminalHandlers{
		store: &stubStore{instances: []*session.Instance{staleInst}},
		live:  &fakeLiveInstanceFinder{instances: map[string]*session.Instance{}}, // wired, but doesn't have "s1"
	}

	got, errRes := th.findInstance("s1")
	if errRes != nil {
		t.Fatalf("findInstance returned an error result: %+v", errRes)
	}
	if got != staleInst {
		t.Error("expected fallback to the LoadInstances()-sourced instance when the live finder doesn't have it")
	}
}

// TestFindInstance_should_fallBackToStore_When_LiveFinderNil verifies the
// nil-safety guard — th.live unset (as in every pre-existing test in this
// file) must behave exactly as before this fix, not panic.
func TestFindInstance_should_fallBackToStore_When_LiveFinderNil(t *testing.T) {
	staleInst := &session.Instance{Title: "s1"}
	th := &terminalHandlers{store: &stubStore{instances: []*session.Instance{staleInst}}}

	got, errRes := th.findInstance("s1")
	if errRes != nil {
		t.Fatalf("findInstance returned an error result: %+v", errRes)
	}
	if got != staleInst {
		t.Error("expected the LoadInstances()-sourced instance when th.live is nil")
	}
}

// --- Epic 4.1: NudgeGate insertion in steerSession/writeToSession ---

// newGatedTerminalHandlers builds a terminalHandlers wired to store and gate,
// with the scrollback/rate-limiter dependencies every Epic 4.1 test below
// needs but doesn't otherwise vary.
func newGatedTerminalHandlers(t *testing.T, store session.InstanceStore, gate nudgeGateEvaluator) *terminalHandlers {
	t.Helper()
	return &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
		nudgeGate:  gate,
	}
}

// TestSteerSession_ShouldAbortWriteAndReturnSafetyGateReason_WhenNudgeExecutionFlagDisabled
// is REQ-5's named integration test (validation.md): steerSession wired to a
// REAL *diagnose.NudgeGate (not a fake) with the flag off must abort before
// any write is attempted, naming the reason literally.
func TestSteerSession_ShouldAbortWriteAndReturnSafetyGateReason_WhenNudgeExecutionFlagDisabled(t *testing.T) {
	inst := newGatedTestInstance(t, "flag-off-session", "claude")
	store := &stubStore{instances: []*session.Instance{inst}}
	cfgFn := func() *config.Config { return &config.Config{} } // flag defaults false
	th := &terminalHandlers{
		store:      store,
		scrollback: makeScrollbackMgr(t),
		writeLim:   newTokenBucket(10, 10),
		nudgeGate:  NewDiagnoseNudgeGate(cfgFn, nil, newIdleGateRegistry()),
	}

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"message":    "focus on the authentication module",
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		t.Fatal("expected success=false while the nudge-execution flag is disabled")
	}
	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	if code, _ := errObj["code"].(string); code != string(diagnose.SafetyGateReasonNudgeExecutionDisabled) {
		t.Errorf("expected error code %q, got %q", diagnose.SafetyGateReasonNudgeExecutionDisabled, code)
	}
}

// TestWriteToSession_ShouldAbortWriteAndReturnSafetyGateReason_WhenNudgeExecutionFlagDisabled
// mirrors the steerSession test above for write_to_session (Story 4.1.2).
func TestWriteToSession_ShouldAbortWriteAndReturnSafetyGateReason_WhenNudgeExecutionFlagDisabled(t *testing.T) {
	inst := newGatedTestInstance(t, "flag-off-write-session", "claude")
	store := &stubStore{instances: []*session.Instance{inst}}
	cfgFn := func() *config.Config { return &config.Config{} }
	th := newGatedTerminalHandlers(t, store, NewDiagnoseNudgeGate(cfgFn, nil, newIdleGateRegistry()))

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"input":      "echo hello",
	})
	result, err := th.writeToSession(context.Background(), req)
	if err != nil {
		t.Fatalf("writeToSession returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		t.Fatal("expected success=false while the nudge-execution flag is disabled")
	}
	errObj, _ := m["error"].(map[string]interface{})
	code, _ := errObj["code"].(string)
	if code != string(diagnose.SafetyGateReasonNudgeExecutionDisabled) {
		t.Errorf("expected error code %q, got %q", diagnose.SafetyGateReasonNudgeExecutionDisabled, code)
	}
}

// TestSteerSession_ShouldAbortWriteAndReturnSafetyGateReason_WhenPipelineReportsNotIdle
// uses a scripted nudgeGateEvaluator to test the handler's plumbing for a
// non-flag gate failure (idle-fail abort) without standing up real idle
// infrastructure -- the real IdleGate/NudgeGate wiring is covered directly
// in diagnose_gate_wiring_test.go.
func TestSteerSession_ShouldAbortWriteAndReturnSafetyGateReason_WhenPipelineReportsNotIdle(t *testing.T) {
	inst := newGatedTestInstance(t, "idle-fail-session", "claude")
	store := &stubStore{instances: []*session.Instance{inst}}
	th := newGatedTerminalHandlers(t, store, fakeNudgeGateEvaluator{ok: false, reason: diagnose.SafetyGateReasonNotIdle})

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"message":    "focus on the authentication module",
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)
	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	if code, _ := errObj["code"].(string); code != string(diagnose.SafetyGateReasonNotIdle) {
		t.Errorf("expected error code %q, got %q", diagnose.SafetyGateReasonNotIdle, code)
	}
}

// TestSteerSession_ShouldAbortWrite_WhenFinalIdentityRecheckFindsTmuxMarkerMismatch
// exercises the REAL tmux.VerifyIdentityImmediatelyBeforeWrite facade (via a
// fake TMUX_BIN backend) for the final pre-write re-check: the pipeline
// itself passes (scripted true), but the pane's actual owner marker
// disagrees with the session's own UUID.
func TestSteerSession_ShouldAbortWrite_WhenFinalIdentityRecheckFindsTmuxMarkerMismatch(t *testing.T) {
	inst := newGatedTestInstance(t, "identity-mismatch-session", "claude")
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=someone-else", 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	store := &stubStore{instances: []*session.Instance{inst}}
	th := newGatedTerminalHandlers(t, store, fakeNudgeGateEvaluator{ok: true})

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"message":    "focus on the authentication module",
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)
	errObj, _ := m["error"].(map[string]interface{})
	if errObj == nil {
		t.Fatal("expected error object in result")
	}
	if code, _ := errObj["code"].(string); code != string(diagnose.SafetyGateReasonIdentityMismatchTmuxMarker) {
		t.Errorf("expected error code %q, got %q", diagnose.SafetyGateReasonIdentityMismatchTmuxMarker, code)
	}
}

// TestSteerSession_ShouldReachSendKeys_WhenGateAndFinalIdentityRecheckBothPass
// is the "all-pass" scenario: pipeline passes (scripted true) and the real
// final identity re-check's fake tmux backend reports a matching marker, so
// the handler must proceed past the gate to the real SendKeys/
// SubmitContentWithEnter call -- proven the same way
// TestSteerSessionMCP_passesValidationAndReachesSendKeys proves it (no live
// PTY in this unit test, so the write itself fails, but with an
// INTERNAL_ERROR/PTY_WRITE_TIMEOUT shape, never a gate-reason code).
func TestSteerSession_ShouldReachSendKeys_WhenGateAndFinalIdentityRecheckBothPass(t *testing.T) {
	inst := newGatedTestInstance(t, "all-pass-session", "claude")
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	store := &stubStore{instances: []*session.Instance{inst}}
	th := newGatedTerminalHandlers(t, store, fakeNudgeGateEvaluator{ok: true})

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"message":    "focus on the authentication module",
	})
	result, err := th.steerSession(context.Background(), req)
	if err != nil {
		t.Fatalf("steerSession returned unexpected Go error: %v", err)
	}

	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		return // a real PTY would make this the true happy path
	}
	errObj, _ := m["error"].(map[string]interface{})
	code, _ := errObj["code"].(string)
	for _, gateCode := range []string{
		string(diagnose.SafetyGateReasonNudgeExecutionDisabled),
		string(diagnose.SafetyGateReasonNotIdle),
		string(diagnose.SafetyGateReasonIdentityMismatchInstance),
		string(diagnose.SafetyGateReasonIdentityMismatchTmuxMarker),
		string(diagnose.SafetyGateReasonNudgeCapReached),
	} {
		if code == gateCode {
			t.Fatalf("steerSession returned gate-failure code %q after an all-pass gate; expected SendKeys to be reached", code)
		}
	}
}

// --- Story 4.1.4: ambiguous write-outcome classification ---

// TestIsConnectionOrTimeoutShapedWriteError covers the classifier all three
// NudgeGate-gated handlers (steerSession, writeToSession, resumeSession)
// share: a table across the exact error shapes Task 4.1.4a's AC names
// (deadline/timeout, connection-refused, no-such-socket) plus a generic,
// unrelated error that must NOT be misclassified as ambiguous.
func TestIsConnectionOrTimeoutShapedWriteError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"context.DeadlineExceeded directly", context.DeadlineExceeded, true},
		{"context.DeadlineExceeded wrapped", fmt.Errorf("submit cancelled: %w", context.DeadlineExceeded), true},
		{"connection refused text", fmt.Errorf("dial unix /tmp/tmux.sock: connect: connection refused"), true},
		{"no such file or directory text", fmt.Errorf("fork/exec /nonexistent/tmux: no such file or directory"), true},
		{"no such socket text", fmt.Errorf("tmux: no such socket"), true},
		{"unrelated error", fmt.Errorf("cannot send keys to instance that has not been started or is paused"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isConnectionOrTimeoutShapedWriteError(tc.err))
		})
	}
}

// TestSteerSession_ShouldReturnWriteOutcomeUnknown_WhenUnderlyingWriteTimesOutAfterGatePasses
// covers Task 4.1.4a/4.1.4d for steerSession's PTY fallback branch: an
// already-expired context makes SubmitContentWithEnter's internal timeout
// fire immediately (context.DeadlineExceeded), which must classify as
// write_outcome_unknown, never a bare/generic error. th.nudgeGate is nil here
// (skips evaluateNudgeGate/verifyNudgeIdentity) so this test isolates the
// post-gate write-outcome classification -- gate plumbing itself is covered
// by the "all-pass" tests above.
func TestSteerSession_ShouldReturnWriteOutcomeUnknown_WhenUnderlyingWriteTimesOutAfterGatePasses(t *testing.T) {
	inst := newGatedTestInstance(t, "steer-timeout-session", "claude")
	store := &stubStore{instances: []*session.Instance{inst}}
	th := newGatedTerminalHandlers(t, store, nil)

	expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"message":    "focus on the authentication module",
	})
	result, err := th.steerSession(expiredCtx, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.NotNil(t, errObj)
	require.Equal(t, writeOutcomeUnknownMarker, errObj["code"],
		"a timeout-shaped write error must classify as write_outcome_unknown, not a bare/generic error")
}

// TestWriteToSession_ShouldReturnWriteOutcomeUnknown_WhenUnderlyingWriteTimesOutAfterGatePasses
// mirrors the steerSession test above for write_to_session.
func TestWriteToSession_ShouldReturnWriteOutcomeUnknown_WhenUnderlyingWriteTimesOutAfterGatePasses(t *testing.T) {
	inst := newGatedTestInstance(t, "write-timeout-session", "claude")
	store := &stubStore{instances: []*session.Instance{inst}}
	th := newGatedTerminalHandlers(t, store, nil)

	expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"input":      "echo hello",
	})
	result, err := th.writeToSession(expiredCtx, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.NotNil(t, errObj)
	require.Equal(t, writeOutcomeUnknownMarker, errObj["code"])
}

// No internal retry is a structural property (each handler calls its write
// function exactly once, no loop) rather than something the two tests above
// runtime-assert -- same class of invariant as verifyBeforeWrite's own doc
// comment ("Go cannot enforce ... automatically"), checked at code review.

// TestSteerSession_ShouldLeaveCapSlotConsumed_NotRefunded_WhenUnderlyingWriteReturnsWriteOutcomeUnknown
// covers Task 4.1.4d's cap-non-refund AC. Scope, stated explicitly: this
// proves no code path in steerSession touches NudgeCapStore based on the
// write's outcome (a cap slot reserved directly via NudgeCapStore before the
// call is still there, unchanged, after a write_outcome_unknown result) --
// it does not additionally re-prove that the real NudgeGate pipeline's own
// cap-check-and-reserve step works, which
// TestNewDiagnoseNudgeGate_ShouldPassAllFourChecks_WhenFlagOnIdleSustainedIdentityMatchesAndCapAvailable
// (diagnose_gate_wiring_test.go) already covers. th.nudgeGate is nil here for
// the same isolation reason as the classification tests above.
func TestSteerSession_ShouldLeaveCapSlotConsumed_NotRefunded_WhenUnderlyingWriteReturnsWriteOutcomeUnknown(t *testing.T) {
	inst := newGatedTestInstance(t, "cap-not-refunded-session", "claude")
	store := &stubStore{instances: []*session.Instance{inst}}
	th := newGatedTerminalHandlers(t, store, nil)

	storage := newTestBacklogStorage(t)
	capStore := services.NewNudgeCapStore(storage)
	itemID := inst.Snapshot().UUID
	ok, err := capStore.CheckAndReserve(context.Background(), itemID, 2, 0)
	require.NoError(t, err)
	require.True(t, ok)

	expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	req := makeToolReq(map[string]interface{}{
		"session_id": inst.Title,
		"message":    "focus on the authentication module",
	})
	result, err := th.steerSession(expiredCtx, req)
	require.NoError(t, err)
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.Equal(t, writeOutcomeUnknownMarker, errObj["code"])

	rec, err := capStore.Get(context.Background(), itemID)
	require.NoError(t, err)
	require.Equal(t, 1, rec.NudgeCount, "a write_outcome_unknown result must not refund the already-reserved cap slot")
}

// --- Story 4.1.4g/h: dispatch-level duplicate-write guard ---

// newDispatchGuardTestSetup builds a terminalHandlers wired to a real
// NudgeCapStore + DiagnoseDispatchStore backed by fresh in-memory storage,
// shared by Story 4.1.4g/h's dispatch-level-guard integration tests below.
// th.nudgeGate is scripted {ok: true} deliberately: these tests isolate the
// dispatch guard's own behavior (it runs independently of, and does not
// consult, the cap -- see checkDuplicateWriteGuard's doc comment); the real
// gate's own cap enforcement is covered separately in
// diagnose_gate_wiring_test.go.
func newDispatchGuardTestSetup(t *testing.T, inst *session.Instance) (*terminalHandlers, services.NudgeCapStore, services.DiagnoseDispatchStore) {
	t.Helper()
	store := &stubStore{instances: []*session.Instance{inst}}
	storage := newTestBacklogStorage(t)
	dispatchStore := services.NewDiagnoseDispatchStore(storage)
	th := newGatedTerminalHandlers(t, store, fakeNudgeGateEvaluator{ok: true})
	th.dispatchWriteGuard = dispatchStore
	return th, services.NewNudgeCapStore(storage), dispatchStore
}

// seedDispatchGuardPrecondition reserves one nudge-cap slot for itemID
// (leaving headroom under a cap of 2) and records a DiagnoseDispatch row
// whose diagnostic session UUID is callerUUID -- the "cap headroom remains,
// but a write was already attempted for this dispatch" precondition the
// guard rejection test below needs.
func seedDispatchGuardPrecondition(t *testing.T, capStore services.NudgeCapStore, dispatchStore services.DiagnoseDispatchStore, itemID, callerUUID string) {
	t.Helper()
	ok, err := capStore.CheckAndReserve(context.Background(), itemID, 2, 0)
	require.NoError(t, err)
	require.True(t, ok, "precondition: NudgeCount=1 < cap=2, i.e. cap headroom remains")

	_, err = dispatchStore.Record(context.Background(), services.DiagnoseDispatchRequest{
		ItemID: itemID, TargetSessionUUID: itemID, DiagnosticSessionUUID: callerUUID,
	})
	require.NoError(t, err)
}

// TestSteerSession_ShouldRejectSecondWriteAttempt_WhenDispatchAlreadyRecordedAWriteAttempt_EvenWithCapHeadroomRemaining
// is the validation.md-named integration test: the specific scenario the
// adversarial re-review flagged as the residual gap -- cap headroom alone
// (NudgeCount=1 < cap=2) would permit a second write, but the dispatch's
// WriteAttemptedAt guard rejects it anyway.
func TestSteerSession_ShouldRejectSecondWriteAttempt_WhenDispatchAlreadyRecordedAWriteAttempt_EvenWithCapHeadroomRemaining(t *testing.T) {
	inst := newGatedTestInstance(t, "dispatch-guard-session", "claude")
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", fakeTmux)
	itemID := inst.Snapshot().UUID
	callerUUID := "diagnostic-session-uuid-for-guard-test"

	th, capStore, dispatchStore := newDispatchGuardTestSetup(t, inst)
	seedDispatchGuardPrecondition(t, capStore, dispatchStore, itemID, callerUUID)

	ctx := WithSessionUUID(context.Background(), callerUUID)
	req := makeToolReq(map[string]interface{}{"session_id": inst.Title, "message": "focus on the authentication module"})

	// First attempt: guard sees no prior WriteAttemptedAt, records one, and
	// proceeds past the guard (the write itself may still fail for unrelated
	// reasons -- no live PTY in this unit test -- which is not what this test
	// asserts).
	first, err := th.steerSession(ctx, req)
	require.NoError(t, err)
	if errObj, _ := parseResult(t, first)["error"].(map[string]interface{}); errObj != nil {
		require.NotEqual(t, string(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch), errObj["code"])
	}

	// Second attempt for the SAME dispatch (same callerUUID) must be rejected,
	// even though cap headroom remains.
	second, err := th.steerSession(ctx, req)
	require.NoError(t, err)
	secondResult := parseResult(t, second)
	require.False(t, secondResult["success"].(bool))
	errObj, _ := secondResult["error"].(map[string]interface{})
	require.NotNil(t, errObj)
	require.Equal(t, string(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch), errObj["code"])

	rec, err := capStore.Get(context.Background(), itemID)
	require.NoError(t, err)
	require.Equal(t, 1, rec.NudgeCount, "the dispatch-level guard's rejection must not consult or consume the nudge cap")
}

// TestSteerSession_ShouldNotApplyDuplicateWriteGuard_WhenCallerHasNoMatchingDiagnoseDispatchRow
// is the validation.md-named integration test: a manual (non-diagnose)
// steer_session call -- STAPLER_SESSION_UUID set, but no DiagnoseDispatch row
// has that UUID as its DiagnosticSessionUUID -- must skip the guard entirely,
// per Task 4.1.4g's explicit carve-out.
func TestSteerSession_ShouldNotApplyDuplicateWriteGuard_WhenCallerHasNoMatchingDiagnoseDispatchRow(t *testing.T) {
	inst := newGatedTestInstance(t, "no-dispatch-row-session", "claude")
	fakeTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", fakeTmux)

	th, _, _ := newDispatchGuardTestSetup(t, inst) // no dispatch rows recorded

	ctx := WithSessionUUID(context.Background(), "manual-caller-with-no-dispatch-row")
	req := makeToolReq(map[string]interface{}{"session_id": inst.Title, "message": "focus on the authentication module"})

	result, err := th.steerSession(ctx, req)
	require.NoError(t, err)

	m := parseResult(t, result)
	if success, _ := m["success"].(bool); success {
		return // a real environment where the write succeeds is fine too
	}
	errObj, _ := m["error"].(map[string]interface{})
	code, _ := errObj["code"].(string)
	require.NotEqual(t, string(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch), code,
		"the guard must be skipped entirely when the caller has no matching DiagnoseDispatch row")
}

// TestSteerSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck is a
// regression test: checkDuplicateWriteGuard's DB round-trips must run BEFORE
// verifyNudgeIdentity's final pre-write identity re-check, so the identity
// re-check is the literal last statement before the write (see
// verifyBeforeWrite's doc comment contract in diagnose_gate_wiring.go). The
// FIRST call uses a matching tmux identity so it passes both checks and marks
// the dispatch's write attempted (the underlying write itself may still fail
// -- no real PTY in this unit test -- which is not what this test asserts).
// The SECOND call switches to a MISMATCHED tmux identity: if the guard
// genuinely runs first, it rejects before the identity re-check is ever
// reached, so the mismatch is never observed. If the buggy ordering ever
// regresses (identity re-check running first), this test would instead
// observe an identity-mismatch code.
func TestSteerSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck(t *testing.T) {
	inst := newGatedTestInstance(t, "guard-before-identity-session", "claude")
	itemID := inst.Snapshot().UUID
	callerUUID := "diagnostic-session-uuid-for-order-test"
	th, capStore, dispatchStore := newDispatchGuardTestSetup(t, inst)
	seedDispatchGuardPrecondition(t, capStore, dispatchStore, itemID, callerUUID)

	ctx := WithSessionUUID(context.Background(), callerUUID)
	req := makeToolReq(map[string]interface{}{"session_id": inst.Title, "message": "focus on the authentication module"})

	matchingTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", matchingTmux)
	_, err := th.steerSession(ctx, req)
	require.NoError(t, err)

	mismatchedTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=someone-else", 0)
	t.Setenv("TMUX_BIN", mismatchedTmux)
	result, err := th.steerSession(ctx, req)
	require.NoError(t, err)
	assertDuplicateWriteGuardRejected(t, result)
}

// TestWriteToSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck
// mirrors TestSteerSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck
// for write_to_session.
func TestWriteToSession_ShouldRunDuplicateWriteGuardBeforeFinalIdentityRecheck(t *testing.T) {
	inst := newGatedTestInstance(t, "write-guard-before-identity-session", "claude")
	itemID := inst.Snapshot().UUID
	callerUUID := "diagnostic-session-uuid-for-write-order-test"
	th, capStore, dispatchStore := newDispatchGuardTestSetup(t, inst)
	seedDispatchGuardPrecondition(t, capStore, dispatchStore, itemID, callerUUID)

	ctx := WithSessionUUID(context.Background(), callerUUID)
	req := makeToolReq(map[string]interface{}{"session_id": inst.Title, "input": "echo hello"})

	matchingTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID="+inst.Snapshot().UUID, 0)
	t.Setenv("TMUX_BIN", matchingTmux)
	_, err := th.writeToSession(ctx, req)
	require.NoError(t, err)

	mismatchedTmux := newFakeTmuxShowEnvironment(t, "STAPLER_SESSION_UUID=someone-else", 0)
	t.Setenv("TMUX_BIN", mismatchedTmux)
	result, err := th.writeToSession(ctx, req)
	require.NoError(t, err)
	assertDuplicateWriteGuardRejected(t, result)
}
