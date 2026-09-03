package services

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/protocol"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tmux"

	"github.com/gorilla/websocket"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// createTestWebSocketPair sets up a test WebSocket server and returns the
// server-side connectWebSocketStream and the client-side connection.
func createTestWebSocketPair(t *testing.T) (*connectWebSocketStream, *websocket.Conn, func()) {
	t.Helper()

	streamChan := make(chan *connectWebSocketStream, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server: failed to upgrade WebSocket: %v", err)
			return
		}
		streamChan <- &connectWebSocketStream{conn: conn}
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("failed to connect test client: %v", err)
	}

	serverStream := <-streamChan

	cleanup := func() {
		clientConn.Close()
		serverStream.conn.Close()
		srv.Close()
	}

	return serverStream, clientConn, cleanup
}

// readEnvelopeFromClient reads one binary WebSocket message and parses its envelope.
func readEnvelopeFromClient(t *testing.T, conn *websocket.Conn) *protocol.Envelope {
	t.Helper()
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message from server: %v", err)
	}
	env, _, err := protocol.ParseEnvelope(msg)
	if err != nil {
		t.Fatalf("failed to parse envelope: %v", err)
	}
	return env
}

// TestSendEndStreamSuccess verifies that sendEndStreamSuccess writes a message
// with the EndStream flag set (regression: streamViaControlMode was missing this call).
func TestSendEndStreamSuccess(t *testing.T) {
	t.Parallel()
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	sendEndStreamSuccess(serverStream)

	env := readEnvelopeFromClient(t, clientConn)
	if !env.IsEndStream() {
		t.Errorf("sendEndStreamSuccess: expected EndStream flag (0x%02x), got flags=0x%02x", protocol.EndStreamFlag, env.Flags)
	}
}

// TestSendEndStreamError verifies that sendEndStreamError writes a message
// with the EndStream flag set and an encoded error.
func TestSendEndStreamError(t *testing.T) {
	t.Parallel()
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	testErr := fmt.Errorf("something went wrong")
	sendEndStreamError(serverStream, testErr)

	env := readEnvelopeFromClient(t, clientConn)
	if !env.IsEndStream() {
		t.Errorf("sendEndStreamError: expected EndStream flag (0x%02x), got flags=0x%02x", protocol.EndStreamFlag, env.Flags)
	}

	// The payload should be a ConnectRPC JSON error envelope:
	// {"error":{"code":"internal","message":"..."}}
	var errEnvelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(env.Data, &errEnvelope); err != nil {
		t.Fatalf("sendEndStreamError: failed to unmarshal JSON payload: %v", err)
	}
	if errEnvelope.Error.Code != "internal" {
		t.Errorf("sendEndStreamError: expected error code %q, got %q", "internal", errEnvelope.Error.Code)
	}
	if !strings.Contains(errEnvelope.Error.Message, testErr.Error()) {
		t.Errorf("sendEndStreamError: error message %q does not contain %q", errEnvelope.Error.Message, testErr.Error())
	}
}

// TestSendEndStreamSuccessIsIdempotentFormat verifies the envelope structure
// matches what the ConnectRPC client expects (EndStreamFlag = 0x02).
func TestSendEndStreamSuccessEnvelopeFormat(t *testing.T) {
	t.Parallel()
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	sendEndStreamSuccess(serverStream)

	_, raw, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	// First byte of envelope is the flags field
	if len(raw) < 5 {
		t.Fatalf("envelope too short: %d bytes", len(raw))
	}
	flags := raw[0]
	if flags&protocol.EndStreamFlag == 0 {
		t.Errorf("EndStream flag (0x%02x) not set in first byte; got 0x%02x", protocol.EndStreamFlag, flags)
	}
}

// --- sanitizeInitialContent ---

// TestSanitizeInitialContentStripsPositioningCodes verifies that the ANSI sequences
// that cause garbled rendering on replay are stripped from tmux capture-pane output.
func TestSanitizeInitialContentStripsPositioningCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "absolute cursor home ESC[H",
			input: "\x1b[Hhello",
			want:  "hello",
		},
		{
			name:  "absolute cursor with coordinates ESC[5;10H",
			input: "\x1b[5;10Hhello",
			want:  "hello",
		},
		{
			name:  "absolute cursor f-variant ESC[5;10f",
			input: "\x1b[5;10fhello",
			want:  "hello",
		},
		{
			name:  "screen clear ESC[J (erase to end of screen)",
			input: "\x1b[Jhello",
			want:  "hello",
		},
		{
			name:  "screen clear ESC[2J (full screen)",
			input: "\x1b[2Jhello",
			want:  "hello",
		},
		{
			name:  "screen clear ESC[3J (scrollback)",
			input: "\x1b[3Jhello",
			want:  "hello",
		},
		{
			name:  "alternate screen enter ESC[?1049h",
			input: "\x1b[?1049hhello",
			want:  "hello",
		},
		{
			name:  "cursor hide ESC[?25l",
			input: "\x1b[?25lhello",
			want:  "hello",
		},
		{
			name:  "DEC save cursor ESC7",
			input: "\x1b7hello",
			want:  "hello",
		},
		{
			name:  "DEC restore cursor ESC8",
			input: "\x1b8hello",
			want:  "hello",
		},
		{
			name:  "CSI save cursor ESC[s",
			input: "\x1b[shello",
			want:  "hello",
		},
		{
			name:  "CSI restore cursor ESC[u",
			input: "\x1b[uhello",
			want:  "hello",
		},
		{
			name:  "multiple positioning codes stripped",
			input: "\x1b[H\x1b[2J\x1b[?1049hhello\x1b[H",
			want:  "hello",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeInitialContent(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeInitialContent(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestSanitizeInitialContentPreservesSGRColors verifies that SGR color sequences
// (which are safe for replay) are intentionally NOT stripped.
func TestSanitizeInitialContentPreservesSGRColors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{name: "SGR reset", input: "\x1b[0m"},
		{name: "SGR bold", input: "\x1b[1m"},
		{name: "SGR green fg", input: "\x1b[32m"},
		{name: "SGR bold green", input: "\x1b[1;32m"},
		{name: "SGR 256-color fg", input: "\x1b[38;5;123m"},
		{name: "SGR truecolor", input: "\x1b[38;2;255;128;0m"},
		{name: "SGR bg color", input: "\x1b[41m"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeInitialContent(tc.input)
			if got != tc.input {
				t.Errorf("sanitizeInitialContent(%q) unexpectedly changed SGR code to %q", tc.input, got)
			}
		})
	}
}

// TestSanitizeInitialContentPreservesPlainText verifies that printable text
// and newlines pass through unchanged.
func TestSanitizeInitialContentPreservesPlainText(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"hello world",
		"line1\nline2\r\nline3",
		"  spaces and\ttabs",
		"unicode: 日本語",
	}

	for _, input := range cases {
		got := sanitizeInitialContent(input)
		if got != input {
			t.Errorf("sanitizeInitialContent(%q) = %q; want unchanged", input, got)
		}
	}
}

// TestSanitizeInitialContentRealWorldCapture exercises a realistic tmux capture-pane
// output with mixed SGR colors and cursor positioning codes. The test verifies that
// colors are kept and positioning codes are removed.
func TestSanitizeInitialContentRealWorldCapture(t *testing.T) {
	t.Parallel()
	// Simulate tmux capture-pane -e output: colored prompt with cursor positioning
	input := "\x1b[?1049h\x1b[H\x1b[2J\x1b[1;32m$\x1b[0m \x1b[1mcommand\x1b[0m\x1b[H"
	got := sanitizeInitialContent(input)

	// Positioning codes must be absent
	positioningCodes := []string{"\x1b[?1049h", "\x1b[H", "\x1b[2J"}
	for _, code := range positioningCodes {
		if strings.Contains(got, code) {
			t.Errorf("sanitizeInitialContent: positioning code %q still present in output %q", code, got)
		}
	}

	// SGR codes must be preserved
	sgrCodes := []string{"\x1b[1;32m", "\x1b[0m", "\x1b[1m"}
	for _, code := range sgrCodes {
		if !strings.Contains(got, code) {
			t.Errorf("sanitizeInitialContent: SGR code %q missing from output %q", code, got)
		}
	}
}

// --- waitForQuiescence ---

// TestWaitForQuiescenceReturnsAfterQuietPeriod verifies that waitForQuiescence
// returns once no updates arrive for the quietFor duration.
func TestWaitForQuiescenceReturnsAfterQuietPeriod(t *testing.T) {
	t.Parallel()
	updates := make(chan struct{}, 1)
	start := time.Now()

	// Send one update, then stop; quiescence should be detected after quietFor.
	updates <- struct{}{}

	waitForQuiescence(updates, 200*time.Millisecond, 30*time.Millisecond)

	elapsed := time.Since(start)
	if elapsed < 30*time.Millisecond {
		t.Errorf("waitForQuiescence returned too quickly (%v); expected >= 30ms quiet window", elapsed)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("waitForQuiescence took too long (%v); expected ~30ms quiet period after last update", elapsed)
	}
}

// TestWaitForQuiescenceReturnsOnTimeout verifies that waitForQuiescence returns
// at the timeout even when updates keep arriving continuously.
func TestWaitForQuiescenceReturnsOnTimeout(t *testing.T) {
	t.Parallel()
	updates := make(chan struct{}, 64)

	// Continuously send updates from a goroutine to prevent quiescence.
	// stopSender is closed by the outer function; the goroutine exits when it sees the signal.
	stopSender := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopSender:
				return
			default:
				// Fill the buffer; ignore if full.
				select {
				case updates <- struct{}{}:
				default:
				}
				<-time.After(time.Millisecond)
			}
		}
	}()

	start := time.Now()
	timeout := 60 * time.Millisecond
	waitForQuiescence(updates, timeout, 500*time.Millisecond)
	elapsed := time.Since(start)
	close(stopSender) // signal the sender goroutine to stop

	if elapsed < timeout {
		t.Errorf("waitForQuiescence returned before timeout (%v < %v)", elapsed, timeout)
	}
	// Tolerance is generous (150ms, vs. a 60ms timeout) because this only needs to
	// prove waitForQuiescence returned at the deadline rather than blocking for the
	// full 500ms quietFor window — not that scheduling is sub-50ms precise. A tight
	// 50ms margin flaked under CPU contention from concurrent test/build load on a
	// shared machine (goroutine wasn't scheduled promptly after the deadline fired),
	// even though 5 isolated re-runs of this test alone all passed comfortably.
	if elapsed > timeout+150*time.Millisecond {
		t.Errorf("waitForQuiescence took too long after timeout (%v)", elapsed)
	}
}

// TestWaitForQuiescenceReturnsOnChannelClose verifies that closing the updates
// channel causes waitForQuiescence to return promptly.
func TestWaitForQuiescenceReturnsOnChannelClose(t *testing.T) {
	t.Parallel()
	updates := make(chan struct{})
	close(updates)

	start := time.Now()
	waitForQuiescence(updates, time.Second, time.Second)
	elapsed := time.Since(start)

	if elapsed > 20*time.Millisecond {
		t.Errorf("waitForQuiescence did not return promptly on closed channel (took %v)", elapsed)
	}
}

// TestWaitForQuiescenceResetsTimerOnUpdates verifies that each incoming update
// resets the quiet timer, delaying the return.
func TestWaitForQuiescenceResetsTimerOnUpdates(t *testing.T) {
	t.Parallel()
	updates := make(chan struct{}, 4)
	// Use 100ms intervals / 200ms quiet — 5× larger than the original values so
	// the OS scheduler has real slack. With 20ms/40ms the scheduler jitter alone
	// could delay an update past the quiet window, causing a spurious early return.
	const updateInterval = 100 * time.Millisecond
	const quietFor = 200 * time.Millisecond

	// Send 3 updates 100ms apart; each should reset the 200ms quiet timer.
	go func() {
		for i := 0; i < 3; i++ {
			<-time.After(updateInterval)
			updates <- struct{}{}
		}
	}()

	start := time.Now()
	waitForQuiescence(updates, 5*time.Second, quietFor)
	elapsed := time.Since(start)

	// 3 updates × 100ms + final 200ms quiet = ~500ms minimum.
	// We check > 400ms (80% of theoretical minimum) to leave headroom for
	// scheduler jitter while still catching a broken reset.
	const minExpected = 400 * time.Millisecond
	if elapsed < minExpected {
		t.Errorf("waitForQuiescence returned too early (%v < %v); timer not being reset on updates", elapsed, minExpected)
	}
}

// --- recordControlModeStreamStart ---

// TestRecordControlModeStreamStart_should_AssignIncreasingGenerations_When_CalledRepeatedly
// verifies the generation counter is monotonic and unique per call, which is
// what lets a later log correlation ("generation N started at T, generation
// N+1 started at T+50ms while N was still active") actually work.
func TestRecordControlModeStreamStart_should_AssignIncreasingGenerations_When_CalledRepeatedly(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)

	gen1, done1 := h.recordControlModeStreamStart("sess1", "staplersquad_sess1")
	done1()
	gen2, done2 := h.recordControlModeStreamStart("sess1", "staplersquad_sess1")
	done2()

	if gen2 <= gen1 {
		t.Errorf("gen2 = %d, want > gen1 = %d", gen2, gen1)
	}
}

// TestRecordControlModeStreamStart_should_LeaveEntryRegistered_When_OlderGenerationCleansUpAfterNewerStarts
// is the regression test for the actual race this exists to detect: if
// generation N's stream is still torn down (its deferred cleanup runs) after
// generation N+1 has already started for the same tmux session, N's cleanup
// must NOT clear N+1's still-active entry out of activeControlModeStreams --
// doing so would make a genuinely-overlapping N+2 invocation look like the
// first and only stream, silently defeating the whole detection mechanism.
func TestRecordControlModeStreamStart_should_LeaveEntryRegistered_When_OlderGenerationCleansUpAfterNewerStarts(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	const tmuxName = "staplersquad_sess1"

	genOld, doneOld := h.recordControlModeStreamStart("sess1", tmuxName)
	genNew, _ := h.recordControlModeStreamStart("sess1", tmuxName)

	// The older generation's stream ends (its stream handler returns) after
	// the newer one has already started -- exactly the overlap this
	// mechanism exists to catch.
	doneOld()

	cur, ok := h.activeControlModeStreams.Load(tmuxName)
	if !ok {
		t.Fatalf("activeControlModeStreams entry for %q was removed entirely; want the newer generation's entry to survive", tmuxName)
	}
	if cur.generation != genNew {
		t.Errorf("activeControlModeStreams entry generation = %d, want %d (the newer, still-active generation); got the older one (%d) instead", cur.generation, genNew, genOld)
	}
}

// TestRecordControlModeStreamStart_should_LogOverlapWarning_When_TwoInvocationsOverlapForSameSession
// is Story 3.2.1's other open gap named in this project's plan.md: the
// "420584566" WARN log at recordControlModeStreamStart's overlap-detection
// branch (connectrpc_websocket.go, "[streamViaControlMode] overlapping
// control-mode stream detected for tmux session") is exercised by the two
// TestRecordControlModeStreamStart_* tests above only for its generation
// counter bookkeeping — neither asserts the WARN log line itself actually
// fires. This test drives the same two-overlapping-invocations scenario
// (recordControlModeStreamStart called for a second generation before the
// first generation's done() has run — the exact sequence streamViaControlMode
// produces when a reconnect races a still-in-flight prior connection for one
// tmux session) and asserts the WARN is present, naming both generations, so
// a future refactor that accidentally silences it is caught by a test
// instead of only by an operator noticing missing logs during an incident.
//
// Not run with t.Parallel(): captureLogs swaps the process-global slog
// default logger for the duration of the test, which would race against any
// other t.Parallel() test emitting logs concurrently in this package.
func TestRecordControlModeStreamStart_should_LogOverlapWarning_When_TwoInvocationsOverlapForSameSession(t *testing.T) {
	buf := captureLogs(t)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	const sessionID = "sess-overlap"
	const tmuxName = "staplersquad_sess-overlap"

	genOld, doneOld := h.recordControlModeStreamStart(sessionID, tmuxName)
	defer doneOld()

	// genOld's done() has deliberately not run yet -- this second call is
	// the overlapping invocation the WARN exists to catch.
	genNew, doneNew := h.recordControlModeStreamStart(sessionID, tmuxName)
	defer doneNew()

	logs := buf.String()
	if !strings.Contains(logs, "overlapping control-mode stream detected") {
		t.Fatalf("expected the overlap WARN log line, got logs:\n%s", logs)
	}
	if !strings.Contains(logs, fmt.Sprintf("prior_generation=%d", genOld)) {
		t.Errorf("expected the WARN to name the prior generation (%d), got logs:\n%s", genOld, logs)
	}
	if !strings.Contains(logs, fmt.Sprintf("new_generation=%d", genNew)) {
		t.Errorf("expected the WARN to name the new generation (%d), got logs:\n%s", genNew, logs)
	}
	if !strings.Contains(logs, "level=WARN") {
		t.Errorf("expected the log line to be at WARN level, got logs:\n%s", logs)
	}
}

// --- HubRegistry / PathHubOwned routing (Epic 2.2) ---

// TestUseStreamHub_should_DefaultToTrue_When_EnvVarUnsetAndRehearsalRecorded
// covers the post-rollout default: with STAPLER_SQUAD_USE_STREAM_HUB unset
// (mirrors STAPLER_SQUAD_USE_CONTROL_MODE's "unset means on" convention) and
// a recorded rollback rehearsal, streamTerminal routes to PathHubOwned.
func TestUseStreamHub_should_DefaultToTrue_When_EnvVarUnsetAndRehearsalRecorded(t *testing.T) {
	recordRollbackRehearsalCompletedForTest(t)

	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "")
	require.True(t, useStreamHub())
}

// TestUseStreamHub_should_ReturnFalse_When_EnvVarIsExactlyFalse verifies the
// opt-out: any value other than "" or "true" (e.g. the literal "false", or a
// typo like "1"/"True") stays on the legacy path rather than failing open.
// Requires a recorded rollback rehearsal (Story 3.3.2) since useStreamHub()'s
// gate refuses to enable the global default otherwise (Story 3.3.1's Task
// 3.3.1d).
func TestUseStreamHub_should_ReturnFalse_When_EnvVarIsExactlyFalse(t *testing.T) {
	recordRollbackRehearsalCompletedForTest(t)

	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "true")
	require.True(t, useStreamHub())

	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "false")
	require.False(t, useStreamHub())

	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "1")
	require.False(t, useStreamHub())
}

// recordRollbackRehearsalCompletedForTest records a passing rollback
// rehearsal (Story 3.3.2's Task 3.3.2c) so a test's useStreamHub() call can
// resolve the global default to true, and restores
// RollbackRehearsalCompletedAt to nil via t.Cleanup so this doesn't leak
// into other tests in the same package binary run.
func recordRollbackRehearsalCompletedForTest(t *testing.T) {
	t.Helper()
	require.NoError(t, config.LoadConfig().RecordRollbackRehearsalCompleted())
	t.Cleanup(func() {
		cfg := config.LoadConfig()
		cfg.RollbackRehearsalCompletedAt = nil
		_ = config.SaveConfig(cfg)
	})
}

// TestUseStreamHub_should_ReturnFalse_When_RehearsalNotCompleted is Story
// 3.3.1/3.3.2's integration-level regression test (pre-mortem P1 #4): even
// with STAPLER_SQUAD_USE_STREAM_HUB="true" set, useStreamHub() must return
// false — safely, not by panicking or crashing the connection — when no
// rollback rehearsal has been recorded.
func TestUseStreamHub_should_ReturnFalse_When_RehearsalNotCompleted(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.RollbackRehearsalCompletedAt = nil
	require.NoError(t, config.SaveConfig(cfg))

	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "true")
	require.False(t, useStreamHub(), "expected the global default to stay false without a recorded rollback rehearsal")
}

// TestUseStreamHub_should_PreferGlobalOverride_Over_EnvVar (Story 3.3.4)
// verifies the live, browser-settable config.StreamHubGlobalOverride takes
// precedence over the STAPLER_SQUAD_USE_STREAM_HUB env var in both
// directions, and that clearing it (nil) reverts resolution to the env var.
func TestUseStreamHub_should_PreferGlobalOverride_Over_EnvVar(t *testing.T) {
	recordRollbackRehearsalCompletedForTest(t)
	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "true")

	cfg := config.LoadConfig()
	forceFalse := false
	require.NoError(t, cfg.SetStreamHubGlobalOverride(&forceFalse))
	require.False(t, useStreamHub(), "override=false must win over env var=true")

	forceTrue := true
	require.NoError(t, cfg.SetStreamHubGlobalOverride(&forceTrue))
	require.True(t, useStreamHub())

	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "false")
	require.True(t, useStreamHub(), "override=true must win over env var=false")

	require.NoError(t, cfg.SetStreamHubGlobalOverride(nil))
	require.False(t, useStreamHub(), "clearing the override must revert resolution to the env var")
}

// TestUseStreamHub_should_GateGlobalOverride_On_RollbackRehearsal (Story
// 3.3.4) verifies forcing the override to true is refused, same as the env
// var path, when no rollback rehearsal has been recorded.
func TestUseStreamHub_should_GateGlobalOverride_On_RollbackRehearsal(t *testing.T) {
	cfg := config.LoadConfig()
	cfg.RollbackRehearsalCompletedAt = nil
	require.NoError(t, config.SaveConfig(cfg))

	forceTrue := true
	require.NoError(t, cfg.SetStreamHubGlobalOverride(&forceTrue))
	t.Cleanup(func() { _ = cfg.SetStreamHubGlobalOverride(nil) })

	require.False(t, useStreamHub(), "override=true must still be refused without a recorded rollback rehearsal")
}

// getOrCreateHubForTest wraps hubRegistry.GetOrCreate and registers
// t.Cleanup to tear down the hub it returns (if any) — the single place
// this file's tests get automatic hub teardown from, instead of each test
// remembering its own t.Cleanup(func() { hub.ForceTeardown() }).
//
// This exists because pumpControlModeOutputIntoHub now retries
// SubscribeControlModeUpdates forever until hub.State() reports
// HubTornDown (2026-08-25 fix — a hub whose control-mode subscription
// closes must not be permanently starved of live output just because it
// closed once, the bug that motivated this whole file's changes that day).
// Before that fix, a test that created a hub and never tore it down was
// harmless: the pump just exited once and stayed exited. After it, the
// same test leaves a goroutine resubscribing and logging every 500ms for
// the rest of the test binary's life — concretely, this raced with an
// unrelated test's log-capture helper under go test -race. Routing hub
// creation through this one helper is what makes that mistake structurally
// hard to make again, rather than relying on every future test remembering
// the cleanup on its own.
func getOrCreateHubForTest(t *testing.T, registry *hubRegistry, sessionName string, controller streamhub.SessionController) (*streamhub.StreamHub, error) {
	t.Helper()
	hub, err := registry.GetOrCreate(sessionName, controller)
	if hub != nil {
		t.Cleanup(func() { _ = hub.ForceTeardown() })
	}
	return hub, err
}

// TestHubRegistry_should_CreateExactlyOneHub_When_GetOrCreateCalledConcurrently
// is REQ-1's core wiring assertion from validation.md, scoped to
// HubRegistry.GetOrCreate itself (the unit Story 2.2.2's task list actually
// specifies — see Task 2.2.2b): concurrent callers for the same tmux session
// name must all observe the same *streamhub.StreamHub instance, and the
// controller passed by a losing caller must never be consulted (only the
// winning LoadOrCompute call's controller matters).
func TestHubRegistry_should_CreateExactlyOneHub_When_GetOrCreateCalledConcurrently(t *testing.T) {
	t.Parallel()
	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}
	const sessionName = "hub-registry-concurrent-test"

	const goroutines = 20
	hubs := make([]*streamhub.StreamHub, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			controller := &fakeSessionController{}
			hub, err := getOrCreateHubForTest(t, registry, sessionName, controller)
			require.NoError(t, err)
			hubs[i] = hub
		}(i)
	}
	wg.Wait()

	first := hubs[0]
	require.NotNil(t, first)
	for i, h := range hubs {
		require.Same(t, first, h, "goroutine %d got a different StreamHub for the same session name", i)
	}
}

// TestHubRegistry_should_ReturnDifferentHubs_When_SessionNamesDiffer is the
// unremarkable counterpart proving GetOrCreate is actually keyed per session
// name, not a single global hub.
func TestHubRegistry_should_ReturnDifferentHubs_When_SessionNamesDiffer(t *testing.T) {
	t.Parallel()
	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}

	hubA, errA := getOrCreateHubForTest(t, registry, "session-a", &fakeSessionController{})
	require.NoError(t, errA)
	hubB, errB := getOrCreateHubForTest(t, registry, "session-b", &fakeSessionController{})
	require.NoError(t, errB)

	require.NotSame(t, hubA, hubB)
}

// TestHubRegistry_should_RefuseToCreateHub_When_OwnershipLockAlreadyResolvedLegacy
// is validation.md's REQ-8 scenario applied to the real HubRegistry: if a
// concurrent legacy StartControlMode call already won this session's
// StreamOwnershipLock resolution, GetOrCreate must refuse to create a
// competing hub (ErrOwnershipResolvedToOtherPath) rather than silently
// creating one anyway — the exact "no silent success reinterpreted"
// requirement from Task 3.1.2b.
func TestHubRegistry_should_RefuseToCreateHub_When_OwnershipLockAlreadyResolvedLegacy(t *testing.T) {
	t.Parallel()
	sessionName := "hub-registry-legacy-already-resolved-" + t.Name()

	// Simulate the legacy path winning the race: it resolves the lock with
	// flagValue=false before this connection's hub-creation attempt runs.
	resolved := streamhub.AcquireOwnershipLock(sessionName).Resolve(false)
	require.Equal(t, streamhub.PathLegacyPerConnection, resolved)

	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}
	hub, err := getOrCreateHubForTest(t, registry, sessionName, &fakeSessionController{})

	require.Nil(t, hub, "GetOrCreate must not create a hub once ownership resolved legacy")
	require.ErrorIs(t, err, streamhub.ErrOwnershipResolvedToOtherPath)
}

// TestHubRegistry_should_CreateHub_When_OwnershipLockResolvesHubOwned is the
// happy-path counterpart: when no concurrent legacy call has won the race
// (or this call is itself the first), GetOrCreate's own ownership check
// passes and hub creation proceeds exactly as before Story 3.1.2.
func TestHubRegistry_should_CreateHub_When_OwnershipLockResolvesHubOwned(t *testing.T) {
	t.Parallel()
	sessionName := "hub-registry-hub-owned-" + t.Name()

	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}
	hub, err := getOrCreateHubForTest(t, registry, sessionName, &fakeSessionController{})

	require.NoError(t, err)
	require.NotNil(t, hub)
	require.Equal(t, streamhub.PathHubOwned, streamhub.AcquireOwnershipLock(sessionName).Resolve(false))
}

// TestHubRegistryAndStreamOwnershipLock_should_NeverProduceTwoOwners_When_RacedConcurrently
// is REQ-8's race scenario (plan.md Story 3.1.2 AC2, scoped to the two real
// entry points that live in this package/file rather than
// Instance.StartControlMode, which this story's task explicitly leaves
// untouched — see the commit message / PR description for why). Many
// goroutines race GetOrCreate (the hub-creation intent) against
// AcquireOwnershipLock(...).ResolveExpecting(false, PathLegacyPerConnection)
// (the legacy-start intent, i.e. exactly the check streamViaControlMode now
// performs before calling StartControlMode) for the same session name.
// Exactly one intent may "win" (no error) per session; every loser must
// observe ErrOwnershipResolvedToOtherPath and the single resolved StreamPath
// must be consistent for all racers.
func TestHubRegistryAndStreamOwnershipLock_should_NeverProduceTwoOwners_When_RacedConcurrently(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1000-iteration race test in short mode")
	}

	const iterations = 1000
	const racersPerSide = 8

	for i := 0; i < iterations; i++ {
		sessionName := fmt.Sprintf("hub-vs-legacy-race-%d", i)
		registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}

		var wg sync.WaitGroup
		var hubWins, legacyWins atomic.Int64
		var hubErrs, legacyErrs atomic.Int64
		var createdHub atomic.Pointer[streamhub.StreamHub]

		for g := 0; g < racersPerSide; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if h, err := registry.GetOrCreate(sessionName, &fakeSessionController{}); err != nil {
					hubErrs.Add(1)
				} else {
					hubWins.Add(1)
					createdHub.Store(h)
				}
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := streamhub.AcquireOwnershipLock(sessionName).ResolveExpecting(false, streamhub.PathLegacyPerConnection); err != nil {
					legacyErrs.Add(1)
				} else {
					legacyWins.Add(1)
				}
			}()
		}
		wg.Wait()

		// Every winning GetOrCreate spawns a pumpControlModeOutputIntoHub
		// goroutine that only exits once the hub reports HubTornDown.
		// fakeSessionController always hands back an already-closed update
		// channel, so left un-torn-down these goroutines loop forever,
		// Warn-logging every pumpControlModeResubscribeDelay — across up to
		// `iterations` hubs, that's a mass of leaked goroutines racing every
		// later test's captureLogs buffer (log/slog's default logger is
		// process-global) for the rest of the binary's run, surfacing as a
		// `-race` failure in an unrelated, much-later test.
		if hub := createdHub.Load(); hub != nil {
			_ = hub.ForceTeardown()
		}

		resolved := streamhub.AcquireOwnershipLock(sessionName).Resolve(false)
		switch resolved {
		case streamhub.PathHubOwned:
			require.Equal(t, int64(racersPerSide), hubWins.Load(), "iteration %d: every hub-creation racer should win when ownership resolved PathHubOwned", i)
			require.Equal(t, int64(0), legacyWins.Load(), "iteration %d: no legacy racer should win when ownership resolved PathHubOwned", i)
			require.Equal(t, int64(racersPerSide), legacyErrs.Load())
		case streamhub.PathLegacyPerConnection:
			require.Equal(t, int64(racersPerSide), legacyWins.Load(), "iteration %d: every legacy racer should win when ownership resolved PathLegacyPerConnection", i)
			require.Equal(t, int64(0), hubWins.Load(), "iteration %d: no hub-creation racer should win when ownership resolved PathLegacyPerConnection", i)
			require.Equal(t, int64(racersPerSide), hubErrs.Load())
		}

		// Tear down any hub this iteration created before moving to the next
		// one. pumpControlModeOutputIntoHub now retries SubscribeControlModeUpdates
		// forever until hub.State() reports HubTornDown (2026-08-25 fix — a
		// hub whose control-mode subscription closes must not be
		// permanently starved of live output just because it closed once).
		// Without this, up to 1000 iterations x racersPerSide's worth of
		// hubs each leave a pump goroutine resubscribing every 500ms for
		// the rest of the test binary's life — a real resource leak this
		// test's fixture pattern (a fresh, never-torn-down hubRegistry per
		// iteration) never used to trigger, back when the pump just exited
		// once and stayed exited.
		if hub, ok := registry.hubs.Load(sessionName); ok {
			_ = hub.ForceTeardown()
		}
	}
}

// --- Epic 3.2 / Story 3.2.1: registry consolidation ---

// TestHubOwnedSession_should_NeverTouchActiveControlModeStreams_When_MultipleSubscribersAttach
// is Story 3.2.1's core AC: for PathHubOwned sessions, hub.SubscriberCount()
// is the sole source of truth for connection count — activeControlModeStreams
// (the legacy, per-handler-instance registry) must never be incremented for
// hub-owned sessions, since streamViaHub (unlike streamViaControlMode) never
// calls recordControlModeStreamStart.
func TestHubOwnedSession_should_NeverTouchActiveControlModeStreams_When_MultipleSubscribersAttach(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	sessionName := "hub-owned-connection-count-" + t.Name()

	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}
	hub, err := getOrCreateHubForTest(t, registry, sessionName, &fakeSessionController{})
	require.NoError(t, err)

	hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{})
	hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{})

	require.Equal(t, 2, hub.SubscriberCount(), "hub.SubscriberCount() must be the sole source of truth for PathHubOwned sessions")
	_, loaded := h.activeControlModeStreams.Load(sessionName)
	require.False(t, loaded, "hub-owned sessions must never touch activeControlModeStreams (Story 3.2.1)")
}

// listSessionsFakeExecutor is a minimal executor.Executor whose
// CombinedOutput (the only method session/tmux's listSessionsRaw calls, per
// TmuxSession.DoesSessionExist/DoesSessionExistNoCache) answers "does this
// tmux session exist" deterministically by call count, not by talking to a
// real tmux server. This is what lets
// TestStreamViaHub_should_SendHubStartFailedError_... force streamViaHub's
// own pre-GetOrCreate restore check and streamViaControlMode's later,
// separate restore check to observe *different* answers from the exact same
// *tmux.TmuxSession object without any timing race: the two checks are
// distinguished purely by which call they are, not by when either runs.
type listSessionsFakeExecutor struct {
	mu         sync.Mutex
	calls      int
	existsName string
}

// CombinedOutput reports existsName as present on the first call (so
// streamViaHub's own restore check sees the session as already there and
// skips straight to HubRegistry.GetOrCreate) and absent on every call after
// that (so streamViaControlMode's own restore check — and the retry loop
// inside tmux.RestoreWithWorkDir it then drives — consistently finds no
// session and genuinely falls through to validateWorkDir's fast failure).
func (e *listSessionsFakeExecutor) CombinedOutput(_ *exec.Cmd) ([]byte, error) {
	e.mu.Lock()
	first := e.calls == 0
	e.calls++
	e.mu.Unlock()
	if first {
		return []byte(e.existsName + "\n"), nil
	}
	return nil, fmt.Errorf("no server running on socket (fake: session absent)")
}

func (e *listSessionsFakeExecutor) Run(_ *exec.Cmd) error {
	return fmt.Errorf("listSessionsFakeExecutor: Run is unsupported, only list-sessions (CombinedOutput) is expected on this path")
}

func (e *listSessionsFakeExecutor) Output(_ *exec.Cmd) ([]byte, error) {
	return nil, fmt.Errorf("listSessionsFakeExecutor: Output is unsupported, only list-sessions (CombinedOutput) is expected on this path")
}

// TestStreamViaHub_should_SendHubStartFailedError_When_HubCreationAndLegacyFallbackBothFail
// is design/ux.md Surface 2's server-side half: when streamViaHub's
// HubRegistry.GetOrCreate call fails (here, because the session's
// StreamOwnershipLock is pre-resolved to PathLegacyPerConnection, simulating
// a concurrent legacy StartControlMode having already won the race) AND its
// streamViaControlMode fallback also fails (here, because the instance's
// working directory has been removed, so RestoreWithWorkDir fails fast with
// tmux.ErrWorkDirMissing instead of spawning a real tmux session), the
// connection must receive a TerminalData_Error frame with
// Code: HubStartFailedErrorCode over the WebSocket before streamViaHub
// returns its error.
//
// Root cause of a previous version of this test hanging until the package
// timeout: streamViaHub (connectrpc_websocket.go) has its OWN "session not
// in tmux, restore before control mode" pre-check — structurally identical
// to streamViaControlMode's — that runs *before* HubRegistry.GetOrCreate is
// ever called. Attaching one real-but-never-started *tmux.TmuxSession (via a
// missing work dir, so both checks see "doesn't exist") made THAT earlier
// check fail and return first, so GetOrCreate — and therefore the
// streamViaControlMode fallback and its HubStartFailedErrorCode frame — was
// never reached at all; streamViaHub returned its own error without ever
// writing anything to the client, so the test's readEnvelopeFromClient
// blocked forever. listSessionsFakeExecutor's call-counted answers fix this
// by making streamViaHub's own check see "session exists" (skip its restore,
// proceed to GetOrCreate) while streamViaControlMode's later check
// genuinely sees "doesn't exist" and fails via ErrWorkDirMissing as intended
// — deterministic by construction, not by racing tmux subprocess timing.
func TestStreamViaHub_should_SendHubStartFailedError_When_HubCreationAndLegacyFallbackBothFail(t *testing.T) {
	// A working directory that is never created, rather than a real temp dir
	// removed after the fact: os.Stat on it fails immediately, so
	// tmux.RestoreWithWorkDir's validateWorkDir step returns
	// tmux.ErrWorkDirMissing instead of proceeding to actually spawn a tmux
	// session (which a since-deleted-but-once-real path risked doing, if
	// e.g. symlink resolution made the deletion and the later Stat disagree).
	dir := filepath.Join(t.TempDir(), "never-created")

	inst, err := session.NewInstance(session.InstanceOptions{
		Title: "hub-start-failed-" + t.Name(),
		Path:  dir,
	})
	require.NoError(t, err)

	snap := inst.Snapshot()
	tmuxPrefix := snap.TmuxPrefix
	if tmuxPrefix == "" {
		tmuxPrefix = "staplersquad_"
	}
	tmuxSessionName := tmux.NewSessionName(snap.Title, tmuxPrefix).String()

	// Force Instance.pm() to lazily initialize as a *TmuxBackend (a bare
	// *Instance has no processManager until first use), then attach a
	// *tmux.TmuxSession backed by listSessionsFakeExecutor so GetTmuxSession()
	// returns non-nil below — otherwise streamViaControlMode's "session
	// missing, restore" branch never runs at all (a nil GetTmuxSession()
	// skips straight to a no-op StartControlMode success, per
	// session/tmux_process_manager.go's StartControlMode returning nil when
	// no session is set). NewTmuxSessionWithDeps (rather than
	// NewTmuxSessionWithPrefix) is what makes the fake executor — and thus
	// the call-counted exists/doesn't-exist answers — take effect; it also
	// passes WithRegistry(nil) internally so the real push-based
	// TmuxServerRegistry can never short-circuit DoesSessionExist(NoCache)
	// ahead of the fake.
	_ = inst.GetTmuxSessionName()
	fakeExec := &listSessionsFakeExecutor{existsName: tmuxSessionName}
	inst.SetTmuxSession(tmux.NewTmuxSessionWithDeps(snap.Title, "true", tmux.MakePtyFactory(), fakeExec))

	// Simulate a concurrent legacy StartControlMode call having already won
	// this session's ownership race, so streamViaHub's own GetOrCreate call
	// below fails with ErrOwnershipResolvedToOtherPath.
	_, err = streamhub.AcquireOwnershipLock(tmuxSessionName).ResolveExpecting(false, streamhub.PathLegacyPerConnection)
	require.NoError(t, err)

	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	handshake := &sessionv1.TerminalData{
		Data: &sessionv1.TerminalData_CurrentPaneRequest{
			CurrentPaneRequest: &sessionv1.CurrentPaneRequest{},
		},
	}
	handshakeBytes, err := proto.Marshal(handshake)
	require.NoError(t, err)
	serverStream.requestMsg = handshakeBytes

	h := NewConnectRPCWebSocketHandler(nil, nil, nil)

	// Run off the test goroutine with a hard deadline: if either failure
	// assumption above doesn't hold, streamViaControlMode's fallback can end
	// up actually starting a working (if degenerate) tmux-backed stream that
	// blocks forever instead of returning an error — fail loudly and fast
	// rather than hanging the whole package for the full `go test` timeout.
	streamErrCh := make(chan error, 1)
	go func() { streamErrCh <- h.streamViaHub(serverStream, inst) }()

	var streamErr error
	select {
	case streamErr = <-streamErrCh:
	case <-time.After(15 * time.Second):
		t.Fatal("streamViaHub did not return within 15s — the legacy fallback likely succeeded instead of failing as this test requires")
	}
	require.Error(t, streamErr, "streamViaHub must return the legacy fallback's error once both paths fail")

	env := readEnvelopeFromClient(t, clientConn)
	var terminalData sessionv1.TerminalData
	require.NoError(t, proto.Unmarshal(env.Data, &terminalData))

	errData := terminalData.GetError()
	require.NotNil(t, errData, "expected a TerminalData_Error frame when both the hub path and legacy fallback fail")
	require.Equal(t, HubStartFailedErrorCode, errData.Code)

	// handleTmuxRestoreFailure's whole purpose is to move a session with a
	// missing working directory to PermanentlyFailed (so "Retry now" is
	// reachable) instead of leaving it Active with a terminal that can never
	// reconnect — this scenario is exactly the ErrWorkDirMissing path
	// (streamViaControlMode's fallback check above), so assert on it here
	// rather than only on the unrelated HubStartFailedErrorCode frame.
	require.Equal(t, session.PermanentlyFailed, inst.Snapshot().Status,
		"handleTmuxRestoreFailure must mark the session PermanentlyFailed when its working directory is missing")
}

// TestEndStreamErrorCode_should_UseFailedPrecondition_When_ErrIsErrWorkDirMissing
// and its sibling below cover endStreamErrorCode directly (extracted from
// sendEndStreamError specifically so this selection is testable without a
// real WebSocket stream) — the frontend's isWorktreeMissingError depends on
// "failed_precondition" being emitted only for this specific failure.
func TestEndStreamErrorCode_should_UseFailedPrecondition_When_ErrIsErrWorkDirMissing(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("tmux session missing and restore failed: %w", tmux.ErrWorkDirMissing)

	require.Equal(t, "failed_precondition", endStreamErrorCode(wrapped))
}

func TestEndStreamErrorCode_should_UseInternal_When_ErrIsUnrelated(t *testing.T) {
	t.Parallel()

	require.Equal(t, "internal", endStreamErrorCode(errors.New("boom")))
}

// TestHubRegistry_should_CallSubscribeControlModeUpdatesExactlyOnce_When_MultipleSubscribersAttach
// is Story 3.2.1's Task 3.2.1c/3.2.1d AC: the hub subscribes to the
// underlying TmuxSession's control-mode output exactly once (via
// pumpControlModeOutputIntoHub, started only from GetOrCreate's winning
// LoadOrCompute call), regardless of how many Subscribers later attach to
// the hub itself — i.e. TmuxSession.controlModeSubscribers gets exactly one
// entry for the hub's own subscription, not one per attached hub Subscriber.
func TestHubRegistry_should_CallSubscribeControlModeUpdatesExactlyOnce_When_MultipleSubscribersAttach(t *testing.T) {
	t.Parallel()
	sessionName := "hub-subscribe-once-" + t.Name()
	controller := &fakeSessionController{}
	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}

	hub, err := getOrCreateHubForTest(t, registry, sessionName, controller)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		hub.AttachSubscriber(streamhub.NewMemoryTransport(), streamhub.SubscriberCapability{})
	}

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&controller.subscribeCalls) >= 1
	}, time.Second, 5*time.Millisecond, "expected the pump goroutine to subscribe at least once")
	require.EqualValues(t, 1, atomic.LoadInt32(&controller.subscribeCalls),
		"the hub must subscribe to the underlying TmuxSession's control-mode output exactly once, regardless of how many Subscribers are attached to the hub itself")
}

// pumpTestController is a minimal streamhub.SessionController test double
// that lets a test control exactly what pumpControlModeOutputIntoHub reads,
// unlike fakeSessionController's SubscribeControlModeUpdates (which always
// returns an already-closed channel and so can never deliver a burst).
type pumpTestController struct {
	updates chan []byte

	// subscribeCalls counts SubscribeControlModeUpdates invocations —
	// TestPumpControlModeOutputIntoHub_should_ResubscribeAndKeepDelivering_When_ControlModeRestartsMidSession
	// uses this to hand back a fresh channel starting from the second call,
	// simulating control mode crashing and restarting mid-session.
	subscribeCalls atomic.Int32
	// resubscribeUpdates, when non-nil, is returned from the second
	// SubscribeControlModeUpdates call onward instead of updates.
	resubscribeUpdates chan []byte

	// startControlModeCalls counts StartControlMode invocations —
	// TestPumpControlModeOutputIntoHub_should_RestartControlMode_When_ResubscribingAfterCrash
	// asserts the pump actually attempts to restart the dead process on every
	// (re)subscribe, not just re-register a listener on it.
	startControlModeCalls atomic.Int32
	startControlModeErr   error
}

func (c *pumpTestController) SetWindowSizeContext(context.Context, int, int) error { return nil }
func (c *pumpTestController) ResizePTY(int, int) error                             { return nil }
func (c *pumpTestController) CapturePaneContentRawContext(context.Context) (streamhub.RawPaneContent, error) {
	return "", nil
}
func (c *pumpTestController) GetPaneCursorPosition() (x, y int, err error) {
	return 0, 0, nil
}
func (c *pumpTestController) StartControlMode() error {
	c.startControlModeCalls.Add(1)
	return c.startControlModeErr
}
func (c *pumpTestController) StopControlMode() error               { return nil }
func (c *pumpTestController) UnsubscribeControlModeUpdates(string) {}
func (c *pumpTestController) SubscribeControlModeUpdates() (string, <-chan []byte) {
	n := c.subscribeCalls.Add(1)
	if n > 1 && c.resubscribeUpdates != nil {
		return "pump-test-sub-resubscribed", c.resubscribeUpdates
	}
	return "pump-test-sub", c.updates
}

// TestPumpControlModeOutputIntoHub_should_FlushOpportunistically_When_ChannelMomentarilyDrained
// is the regression test for the architecture gap this fix closes: before it,
// pumpControlModeOutputIntoHub's `for data := range updates { hub.OnRawOutput(data) }`
// loop never called BatchWindow.TryFlush, so every hub-owned burst always paid
// the full MaxBatchWindow (20ms default) ceiling latency, unlike the legacy
// per-connection coalesce loop's `select {...; default: break coalesce}`
// early-flush behavior it is meant to mirror. This test sets the hub's batch
// ceiling to a duration (2s) far longer than any reasonable flush latency, so
// a frame reaching the subscriber quickly can only be explained by the pump's
// drain-then-TryFlush step firing — not by the ceiling timer, which would
// still be more than a second away from expiring.
func TestPumpControlModeOutputIntoHub_should_FlushOpportunistically_When_ChannelMomentarilyDrained(t *testing.T) {
	t.Parallel()

	const ceiling = 2 * time.Second
	controller := &pumpTestController{updates: make(chan []byte, 4)}
	hub := streamhub.NewStreamHub("pump-early-flush-"+t.Name(), controller, streamhub.WithBatchMaxWindow(ceiling))

	transport := streamhub.NewMemoryTransport()
	hub.AttachSubscriber(transport, streamhub.SubscriberCapability{})

	// Buffer every chunk of the burst before the pump goroutine ever reads
	// from the channel, so the drain loop's non-blocking `default` branch is
	// guaranteed to fire on an already-populated-then-momentarily-empty
	// channel, exactly like the legacy coalesce loop's scenario.
	controller.updates <- []byte("frame-1;")
	controller.updates <- []byte("frame-2;")
	controller.updates <- []byte("frame-3;")

	start := time.Now()
	go pumpControlModeOutputIntoHub(hub, controller, "pump-early-flush-"+t.Name())
	// ForceTeardown before closing updates: the pump now resubscribes
	// whenever its channel closes (2026-08-25 fix — control mode can
	// crash/restart mid-session, so a closed channel alone no longer means
	// "stop for good"), and only exits when hub.State() reports
	// HubTornDown. Without this, closing controller.updates alone would
	// leave the pump goroutine spinning a resubscribe retry forever in the
	// background after this test returns.
	t.Cleanup(func() {
		_ = hub.ForceTeardown()
		close(controller.updates)
	})

	require.Eventually(t, func() bool {
		for _, frame := range transport.ReceivedFrames() {
			if bytes.Contains(frame, []byte("frame-3;")) {
				return true
			}
		}
		return false
	}, 500*time.Millisecond, 5*time.Millisecond, "expected the burst to be flushed to the subscriber well before it did")

	elapsed := time.Since(start)
	require.Less(t, elapsed, ceiling/2,
		"burst was flushed in %s — should have been well under half the %s ceiling if TryFlush fired opportunistically instead of waiting on the timer", elapsed, ceiling)
}

// TestPumpControlModeOutputIntoHub_should_ResubscribeAndKeepDelivering_When_ControlModeRestartsMidSession
// is the regression test for the 2026-08-25 bug: control mode's process can
// crash and restart mid-session (StartControlMode is refcounted; a crash
// closes every subscriber's channel — session/tmux/control_mode.go's exit
// handler), which is a normal recoverable event, not a session end. Before
// this fix, pumpControlModeOutputIntoHub treated its subscription channel
// closing as permanent and returned for good, silently starving the hub of
// all live output for the rest of its lifetime even though control mode
// came back seconds later — reported as "no real-time feedback while
// typing, updates only appear after a resize" (resize still worked because
// handleCurrentPaneRequest captures fresh via a direct capture-pane
// subprocess call, bypassing this pump entirely).
func TestPumpControlModeOutputIntoHub_should_ResubscribeAndKeepDelivering_When_ControlModeRestartsMidSession(t *testing.T) {
	t.Parallel()

	controller := &pumpTestController{
		updates:            make(chan []byte, 4),
		resubscribeUpdates: make(chan []byte, 4),
	}
	hub := streamhub.NewStreamHub("pump-resubscribe-"+t.Name(), controller,
		streamhub.WithBatchMaxWindow(2*time.Second))
	t.Cleanup(func() { _ = hub.ForceTeardown() })

	transport := streamhub.NewMemoryTransport()
	hub.AttachSubscriber(transport, streamhub.SubscriberCapability{})

	go pumpControlModeOutputIntoHub(hub, controller, "pump-resubscribe-"+t.Name())

	// First "control mode session": deliver a frame, then simulate a crash
	// by closing its channel — control mode's own exit handler does the same
	// to every subscriber when the process dies.
	controller.updates <- []byte("before-crash;")
	require.Eventually(t, func() bool {
		return bytes.Contains(bytes.Join(transport.ReceivedFrames(), nil), []byte("before-crash;"))
	}, time.Second, 5*time.Millisecond, "expected the pre-crash frame to be delivered")
	close(controller.updates)

	// Control mode "restarts": the pump must resubscribe (picking up
	// resubscribeUpdates, per the fake's second-call behavior) and keep
	// delivering — not have exited for good when updates closed above.
	require.Eventually(t, func() bool {
		return controller.subscribeCalls.Load() >= 2
	}, time.Second, 5*time.Millisecond, "expected the pump to resubscribe after its channel closed instead of exiting for good")

	// 2026-09-01 regression: resubscribing alone is not recovery.
	// SubscribeControlModeUpdates only registers a listener on whatever
	// control-mode process already exists (or, if it crashed, immediately
	// returns a pre-closed channel forever) — it never starts a fresh one.
	// Before this fix, this loop spun on pumpControlModeResubscribeDelay
	// forever after a real crash, permanently starving the hub (see
	// StartControlMode's doc comment on session_controller.go). The pump
	// must call StartControlMode on every (re)subscribe attempt, not just
	// the first.
	require.GreaterOrEqual(t, controller.startControlModeCalls.Load(), int32(2),
		"expected the pump to call StartControlMode on every (re)subscribe attempt, not just the first")

	controller.resubscribeUpdates <- []byte("after-restart;")
	require.Eventually(t, func() bool {
		return bytes.Contains(bytes.Join(transport.ReceivedFrames(), nil), []byte("after-restart;"))
	}, time.Second, 5*time.Millisecond, "expected the post-restart frame to be delivered via the resubscribed channel")
}

// TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown is
// the regression test for the bug AttachSubscriber's doc comment left as
// HubRegistry's policy to close: a hub that has fully torn down (0
// subscribers, grace period expired, StopControlMode returned) has no pump
// goroutine left — pumpControlModeOutputIntoHub already exited for good and
// called MarkPumpExited. GetOrCreate's LoadOrCompute only runs its
// pump-starting constructor on a genuine cache miss, so before
// TryStartPump/MarkPumpExited existed, a reconnect to that same session
// found the hub already in the registry, reactivated its *state* via
// AttachSubscriber, and never restarted the pump — silently losing live
// output for the rest of the process's life. Fails against the pre-fix
// GetOrCreate (which never called TryStartPump on a cache hit); passes with
// it.
func TestHubRegistry_should_RestartPump_When_ReconnectingAfterFullTeardown(t *testing.T) {
	t.Parallel()

	sessionName := "pump-restart-after-teardown-" + t.Name()
	controller := &pumpTestController{
		updates:            make(chan []byte, 4),
		resubscribeUpdates: make(chan []byte, 4),
	}
	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}

	// First connection: create the hub via the real registry path, attach,
	// and confirm live delivery works before tearing anything down.
	hub, err := getOrCreateHubForTest(t, registry, sessionName, controller)
	require.NoError(t, err)

	transport1 := streamhub.NewMemoryTransport()
	subID1 := hub.AttachSubscriber(transport1, streamhub.SubscriberCapability{})

	controller.updates <- []byte("first-connection;")
	require.Eventually(t, func() bool {
		return bytes.Contains(bytes.Join(transport1.ReceivedFrames(), nil), []byte("first-connection;"))
	}, time.Second, 5*time.Millisecond, "expected the first connection's frame to be delivered")

	// Detach and force a full teardown, matching what a real grace-period
	// expiry (onTeardownGraceExpired -> ForceTeardown) does once every
	// viewer has disconnected. Close controller.updates right after so the
	// original pump — still blocked reading it — notices and exits, mirroring
	// the real StopControlMode's side effect of closing the underlying
	// control-mode subscription.
	hub.DetachSubscriber(subID1)
	require.NoError(t, hub.ForceTeardown())
	require.Equal(t, streamhub.HubTornDown, hub.State())
	close(controller.updates)

	// Deterministically wait for the original pump goroutine to have
	// actually returned (MarkPumpExited flips this hub's internal pumpActive
	// back to false) before reconnecting below. Without this, Go's scheduler
	// could let this test's own reconnect reactivate the hub's state before
	// the original pump notices HubTornDown and exits — that stale-but-still-
	// running pump would then coincidentally resubscribe on its own,
	// producing a false pass that doesn't actually exercise GetOrCreate's
	// fix. TryStartPump/MarkPumpExited are the only exported hooks onto that
	// state, so this uses them as a poll-and-release probe: claim, observe
	// success, release immediately so the real reconnect below can claim it
	// for real.
	require.Eventually(t, func() bool {
		if hub.TryStartPump() {
			hub.MarkPumpExited()
			return true
		}
		return false
	}, time.Second, 5*time.Millisecond, "expected the original pump to exit after its subscription channel closed")

	// Reconnect: GetOrCreate must find the same (cached) hub AND restart its
	// pump. Pre-fix, this silently reactivated the hub's state with nothing
	// feeding it, so a frame pushed after this point would never arrive.
	hub2, err := getOrCreateHubForTest(t, registry, sessionName, controller)
	require.NoError(t, err)
	require.Same(t, hub, hub2, "expected GetOrCreate to reuse the same hub instance on reconnect")

	transport2 := streamhub.NewMemoryTransport()
	hub2.AttachSubscriber(transport2, streamhub.SubscriberCapability{})

	controller.resubscribeUpdates <- []byte("after-reconnect;")
	require.Eventually(t, func() bool {
		return bytes.Contains(bytes.Join(transport2.ReceivedFrames(), nil), []byte("after-reconnect;"))
	}, time.Second, 5*time.Millisecond, "expected live output to resume after reconnecting to a fully-torn-down hub")
}

// TestStreamTerminal_should_RouteThroughHubWithNoLegacyResizeCall_When_PathHubOwnedResolved
// is validation.md's REQ-1 test name. Full end-to-end coverage of
// streamTerminal's session resolution (SessionService/storage/tmux) is out of
// this unit test's scope; what's asserted here is the specific claim Story
// 2.2.2's AC makes: when useStreamHub() is true, the PathHubOwned branch
// attaches via HubRegistry/WebSocketTransport and never touches
// streamViaControlMode's legacy per-connection resize/capture code — proven
// by HubRegistry.GetOrCreate on a fake SessionController never having
// SetWindowSize/ResizePTY/CapturePaneContent called by anything in this
// epic's new code path (only AttachSubscriber's bookkeeping runs).
func TestStreamTerminal_should_RouteThroughHubWithNoLegacyResizeCall_When_PathHubOwnedResolved(t *testing.T) {
	// Story 3.3.1/3.3.2: useStreamHub() now refuses to resolve the global
	// default to true unless a rollback rehearsal has been recorded
	// (config.RollbackRehearsalCompletedAt) — record one here so this
	// test's premise (the global default resolving true) still holds.
	recordRollbackRehearsalCompletedForTest(t)
	t.Setenv("STAPLER_SQUAD_USE_STREAM_HUB", "true")
	require.True(t, useStreamHub(), "flag must resolve PathHubOwned for this test's premise to hold")

	registry := &hubRegistry{hubs: xsync.NewMap[string, *streamhub.StreamHub]()}
	controller := &fakeSessionController{}
	hub, err := getOrCreateHubForTest(t, registry, "path-hub-owned-test", controller)
	require.NoError(t, err)

	serverStream, _, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	transport := NewWebSocketTransport(serverStream, "path-hub-owned-test")
	id := hub.AttachSubscriber(transport, streamhub.SubscriberCapability{CanResize: true, CanWrite: true})
	transport.BindSubscriber(hub, id)

	require.Equal(t, 1, hub.SubscriberCount())
	require.Equal(t, 0, controller.stopControlModeCalls, "no teardown should have been triggered by attaching")
}

// --- getOrRefreshSnapshot / markSnapshotDirty ---

// TestGetOrRefreshSnapshotCallsCaptureFnOnMiss verifies that on a cache miss
// captureFn is called and the result is cached.
func TestGetOrRefreshSnapshotCallsCaptureFnOnMiss(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	calls := 0
	captureFn := func() (string, error) {
		calls++
		return "fresh content", nil
	}

	got, err := h.getOrRefreshSnapshot("sess1", captureFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fresh content" {
		t.Errorf("got %q, want %q", got, "fresh content")
	}
	if calls != 1 {
		t.Errorf("captureFn called %d times, want 1", calls)
	}
}

// TestGetOrRefreshSnapshotReturnsCacheOnHit verifies that a second call returns
// the cached result without invoking captureFn again.
func TestGetOrRefreshSnapshotReturnsCacheOnHit(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	calls := 0
	captureFn := func() (string, error) {
		calls++
		return "content", nil
	}

	_, _ = h.getOrRefreshSnapshot("sess1", captureFn)
	got, err := h.getOrRefreshSnapshot("sess1", captureFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "content" {
		t.Errorf("got %q, want cached %q", got, "content")
	}
	if calls != 1 {
		t.Errorf("captureFn called %d times on second hit, want 1", calls)
	}
}

// TestGetOrRefreshSnapshotRefreshesOnDirty verifies that marking a snapshot dirty
// causes the next getOrRefreshSnapshot call to invoke captureFn again.
func TestGetOrRefreshSnapshotRefreshesOnDirty(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	calls := 0
	captureFn := func() (string, error) {
		calls++
		return fmt.Sprintf("content%d", calls), nil
	}

	// Populate cache
	_, _ = h.getOrRefreshSnapshot("sess1", captureFn)

	// Mark dirty
	h.markSnapshotDirty("sess1")

	// Should re-invoke captureFn
	got, err := h.getOrRefreshSnapshot("sess1", captureFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("captureFn called %d times after dirty, want 2", calls)
	}
	if got != "content2" {
		t.Errorf("got %q after refresh, want %q", got, "content2")
	}
}

// TestMarkSnapshotDirtyOnUnknownSessionIsNoOp verifies that marking an absent
// session dirty does not panic or create a cache entry.
func TestMarkSnapshotDirtyOnUnknownSessionIsNoOp(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)

	// Should not panic
	h.markSnapshotDirty("nonexistent-session")

	// Should not create an entry
	if _, ok := h.snapshotCache.Load("nonexistent-session"); ok {
		t.Error("markSnapshotDirty created a cache entry for an unknown session")
	}
}

// TestGetOrRefreshSnapshotPropagatesCaptureFnError verifies that captureFn errors
// are returned to the caller and nothing is cached.
func TestGetOrRefreshSnapshotPropagatesCaptureFnError(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	captureErr := fmt.Errorf("tmux: session not found")
	captureFn := func() (string, error) { return "", captureErr }

	_, err := h.getOrRefreshSnapshot("sess1", captureFn)
	if err == nil {
		t.Fatal("expected error from captureFn, got nil")
	}
	if !strings.Contains(err.Error(), captureErr.Error()) {
		t.Errorf("error %q does not contain %q", err.Error(), captureErr.Error())
	}
	if _, ok := h.snapshotCache.Load("sess1"); ok {
		t.Error("cache entry created despite captureFn error")
	}
}

// TestSnapshotCacheConcurrentAccess verifies that concurrent reads and
// dirty-marking do not cause data races. Run with -race to validate.
func TestSnapshotCacheConcurrentAccess(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		sessionID := fmt.Sprintf("sess%d", i)
		go func(id string) {
			defer wg.Done()
			_, _ = h.getOrRefreshSnapshot(id, func() (string, error) {
				return "content", nil
			})
		}(sessionID)
		go func(id string) {
			defer wg.Done()
			h.markSnapshotDirty(id)
		}(sessionID)
	}
	wg.Wait()
}

// --- isAllowedOrigin ---

func newRequestWithOrigin(origin string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

// TestIsAllowedOriginNoHeader verifies that requests without an Origin header
// (e.g. CLI tools, server-side callers) are allowed unconditionally.
func TestIsAllowedOriginNoHeader(t *testing.T) {
	t.Parallel()
	r := newRequestWithOrigin("")
	if !isAllowedOrigin(r) {
		t.Error("request with no Origin header should be allowed")
	}
}

// TestIsAllowedOriginLocalhostVariants verifies that all localhost forms are accepted.
func TestIsAllowedOriginLocalhostVariants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		origin string
	}{
		{"localhost name", "http://localhost:3000"},
		{"127.0.0.1", "http://127.0.0.1:8543"},
		{"IPv6 loopback", "http://[::1]:8543"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newRequestWithOrigin(tc.origin)
			if !isAllowedOrigin(r) {
				t.Errorf("origin %q should be allowed", tc.origin)
			}
		})
	}
}

// TestIsAllowedOriginHTTPS verifies that any HTTPS origin is accepted
// (auth enforcement is left to middleware).
func TestIsAllowedOriginHTTPS(t *testing.T) {
	t.Parallel()
	cases := []string{
		"https://myapp.example.com",
		"https://company.internal:8443",
		"https://staging.myapp.io",
	}
	for _, origin := range cases {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			r := newRequestWithOrigin(origin)
			if !isAllowedOrigin(r) {
				t.Errorf("HTTPS origin %q should be allowed", origin)
			}
		})
	}
}

// TestIsAllowedOriginHTTPNonLocalhostBlocked verifies that plaintext HTTP origins
// from non-localhost hosts are rejected to prevent CSRF from remote pages.
func TestIsAllowedOriginHTTPNonLocalhostBlocked(t *testing.T) {
	t.Parallel()
	cases := []string{
		"http://attacker.example.com",
		"http://evil.com",
		"http://192.168.1.100:3000",
	}
	for _, origin := range cases {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			r := newRequestWithOrigin(origin)
			if isAllowedOrigin(r) {
				t.Errorf("HTTP non-localhost origin %q should be blocked", origin)
			}
		})
	}
}

// TestIsAllowedOriginMalformed verifies that a malformed Origin header is rejected.
func TestIsAllowedOriginMalformed(t *testing.T) {
	t.Parallel()
	r := newRequestWithOrigin("not-a-url")
	// url.Parse("not-a-url") does not return an error (it parses as a relative URL with no scheme).
	// A relative URL has no scheme → not https → host is "" → not localhost → should be blocked.
	if isAllowedOrigin(r) {
		t.Error("malformed Origin 'not-a-url' should be blocked (no scheme, not localhost)")
	}
}

// --- prepareSnapshotContent ---
//
// Regression tests for the two snapshot rendering bugs:
//
//   Bug A (double display): post-resize snapshot written on top of existing
//   content because the prefix lacked a screen clear → fixed by ansiSnapshotPrefix
//   containing ansiEraseScreen.
//
//   Bug B (stairstepped newlines): tmux capture-pane -p emits rows separated by
//   bare \n (LF). In xterm.js, LF only moves the cursor DOWN — it does not return
//   to column 0 — unless convertEol/LNM is on. LNM is off by default and DECSTR
//   resets it to off, so every row after the first was indented by the previous
//   row's width. Fix: normalize \n → \r\n so rows always start at column 0.

// TestPrepareSnapshotContentNormalizesNewlines is the direct regression test for
// Bug B. It MUST fail against the old sanitizeInitialContent-only implementation
// (which returned bare \n unchanged).
func TestPrepareSnapshotContentNormalizesNewlines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bare LF separators converted to CRLF",
			input: "line1\nline2\nline3",
			want:  "line1\r\nline2\r\nline3",
		},
		{
			name:  "single trailing newline",
			input: "line1\n",
			want:  "line1\r\n",
		},
		{
			name:  "pre-existing CRLF not doubled to CRRLF",
			input: "line1\r\nline2\r\n",
			want:  "line1\r\nline2\r\n",
		},
		{
			name:  "mixed bare LF and CRLF normalised uniformly",
			input: "line1\nline2\r\nline3\n",
			want:  "line1\r\nline2\r\nline3\r\n",
		},
		{
			name:  "empty string unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "no newlines unchanged",
			input: "no newline here",
			want:  "no newline here",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := prepareSnapshotContent(streamhub.RawPaneContent(tc.input))
			if got != tc.want {
				t.Errorf("prepareSnapshotContent(%q) =\n  %q\nwant\n  %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestPrepareSnapshotContentStripsCursorPositioning verifies that sanitization
// (stripping cursor-positioning codes) still runs before newline normalisation.
func TestPrepareSnapshotContentStripsCursorPositioning(t *testing.T) {
	t.Parallel()
	// A realistic capture-pane fragment: cursor home + color + text + newline
	input := "\x1b[H\x1b[1;32mline1\x1b[0m\nline2\n"
	got := prepareSnapshotContent(streamhub.RawPaneContent(input))

	if strings.Contains(got, "\x1b[H") {
		t.Errorf("prepareSnapshotContent: cursor home ESC[H not stripped; got %q", got)
	}
	if !strings.Contains(got, "\r\n") {
		t.Errorf("prepareSnapshotContent: expected \\r\\n line endings; got %q", got)
	}
	if strings.Contains(got, "\x1b[H\r\n") {
		t.Errorf("prepareSnapshotContent: cursor home was converted to \\r\\n instead of stripped; got %q", got)
	}
}

// TestPrepareSnapshotContentPreservesSGR verifies that SGR color sequences are
// preserved (they are safe to replay and must not be lost).
func TestPrepareSnapshotContentPreservesSGR(t *testing.T) {
	t.Parallel()
	input := "\x1b[1;32mhello\x1b[0m\nworld\n"
	got := prepareSnapshotContent(streamhub.RawPaneContent(input))

	for _, sgr := range []string{"\x1b[1;32m", "\x1b[0m"} {
		if !strings.Contains(got, sgr) {
			t.Errorf("prepareSnapshotContent: SGR sequence %q was lost; got %q", sgr, got)
		}
	}
}

// --- runInputReadLoop ---

// TestRunInputReadLoopExitsPromptlyOnConnectionClose verifies the WebSocket
// input-read loop (Story 3.1 / AC4-Go): it processes input while the
// connection is open, exits within a bounded timeout once the connection is
// closed (the same event that unblocks it in production via the outer
// handler's deferred conn.Close()), and does not keep forwarding input
// received after the loop has exited.
func TestRunInputReadLoopExitsPromptlyOnConnectionClose(t *testing.T) {
	t.Parallel()
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	var mu sync.Mutex
	var recordedInput [][]byte
	onInput := func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		cp := append([]byte(nil), data...)
		recordedInput = append(recordedInput, cp)
	}
	onResize := func(cols, rows int) {}
	onScrollbackRequest := func(startLine, endLine string) (string, error) {
		return "", nil
	}
	onCurrentPaneRequest := func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		return &sessionv1.TerminalOutput{}, nil
	}

	doneChan := make(chan struct{})
	errChan := make(chan error, 2)
	done := make(chan struct{})
	var resizeSettling atomic.Bool
	go func() {
		runInputReadLoop(serverStream, doneChan, errChan, "test-session", onInput, onResize, onScrollbackRequest, onCurrentPaneRequest, &resizeSettling)
		close(done)
	}()

	// Write one input envelope from the client side and verify it's recorded.
	sendInput := func(payload []byte) {
		t.Helper()
		td := &sessionv1.TerminalData{
			Data: &sessionv1.TerminalData_Input{
				Input: &sessionv1.TerminalInput{Data: payload},
			},
		}
		dataBytes, err := proto.Marshal(td)
		if err != nil {
			t.Fatalf("failed to marshal TerminalData: %v", err)
		}
		envelope := protocol.CreateEnvelope(0, dataBytes)
		if err := clientConn.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
			t.Fatalf("failed to write input envelope: %v", err)
		}
	}

	sendInput([]byte("hello"))

	// Wait for the loop to record the input (avoid a fixed sleep/race).
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(recordedInput)
		mu.Unlock()
		if n >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for runInputReadLoop to record the first input")
		case <-time.After(5 * time.Millisecond):
		}
	}

	mu.Lock()
	if len(recordedInput) != 1 || string(recordedInput[0]) != "hello" {
		mu.Unlock()
		t.Fatalf("expected recorded input [%q], got %v", "hello", recordedInput)
	}
	mu.Unlock()

	// Close the CLIENT connection, simulating the production shutdown path
	// where the outer handler's deferred conn.Close() runs.
	if err := clientConn.Close(); err != nil {
		t.Fatalf("failed to close client connection: %v", err)
	}

	// The loop must exit within a bounded timeout — this is the core
	// AC4-Go assertion: no unbounded blocking read after the connection dies.
	select {
	case <-done:
		// loop returned promptly, as expected
	case <-time.After(2 * time.Second):
		t.Fatal("runInputReadLoop did not exit within 2s of the client connection closing")
	}

	// The client attempted (and necessarily failed) to write again after
	// closing its own end. Regardless of whether that write itself errors,
	// the loop has already exited, so no further input can have been recorded.
	_ = sendInputIgnoringError(clientConn, []byte("late-input"))

	mu.Lock()
	defer mu.Unlock()
	if len(recordedInput) != 1 {
		t.Fatalf("expected recorded input length to stay at 1 after connection close, got %d: %v", len(recordedInput), recordedInput)
	}
}

// fakePanePTY is a minimal panePTY test double: no tmux subprocess, no
// dimension mismatches by default, so it exercises handleCurrentPaneRequest's
// capture/response-construction logic without the resize/SIGWINCH-workaround
// branch (covered separately via the dimension fields below when needed).
type fakePanePTY struct {
	captureContent            string
	captureErr                error
	cols, rows                int
	dimensionsErr             error
	resizePTYErr              error
	refreshTmuxErr            error
	capturePaneCalled         int
	resizePTYCalled           int
	refreshTmuxCalled         int
	capturePanePriorityCalled int
	refreshTmuxPriorityCalled int

	// fastLaneCtxsSeen records every ctx a fast-lane (*Priority) method received, in call
	// order — lets a test assert handleCurrentPaneRequest actually shares one ctx (and
	// therefore one real, decreasing deadline) across every fast-lane call in a single
	// resize/resync, rather than each call getting its own independent
	// tmux.ResyncFastLaneTimeout allowance (2026-08-25 incident).
	fastLaneCtxsSeen []context.Context
}

// CapturePaneContentRaw is the plain (non-fast-lane) capture path
// handleCurrentPaneRequest now calls — tracks its own call count so tests
// can assert which of the two methods (this or CapturePaneContentRawPriority)
// was invoked, i.e. that ResyncOptions.UseFastLane routed the call correctly.
func (f *fakePanePTY) CapturePaneContentRaw() (streamhub.RawPaneContent, error) {
	f.capturePaneCalled++
	return streamhub.RawPaneContent(f.captureContent), f.captureErr
}

// CapturePaneContentRawPriority mirrors CapturePaneContentRaw for this fake: it shares the
// same content/error fields since the fake has no real exec-gate fast lane to distinguish,
// but tracks its own call count for the same reason CapturePaneContentRaw does. ctx is
// accepted (handleCurrentPaneRequest threads one shared ctx through every fast-lane call in
// a single resync — see tmux.ResyncFastLaneTimeout's doc comment) but this fake has no
// timeout behavior of its own to exercise, so it's unused here.
func (f *fakePanePTY) CapturePaneContentRawPriority(ctx context.Context) (streamhub.RawPaneContent, error) {
	f.capturePanePriorityCalled++
	f.fastLaneCtxsSeen = append(f.fastLaneCtxsSeen, ctx)
	return streamhub.RawPaneContent(f.captureContent), f.captureErr
}

func (f *fakePanePTY) GetPaneDimensions() (int, int, error) {
	return f.cols, f.rows, f.dimensionsErr
}

// GetPaneDimensionsPriority mirrors GetPaneDimensions for this fake — see
// CapturePaneContentRawPriority's doc comment on why ctx is recorded.
func (f *fakePanePTY) GetPaneDimensionsPriority(ctx context.Context) (int, int, error) {
	f.fastLaneCtxsSeen = append(f.fastLaneCtxsSeen, ctx)
	return f.cols, f.rows, f.dimensionsErr
}

func (f *fakePanePTY) ResizePTY(cols, rows int) error {
	f.resizePTYCalled++
	f.cols, f.rows = cols, rows
	return f.resizePTYErr
}

func (f *fakePanePTY) RefreshTmuxClient() error {
	f.refreshTmuxCalled++
	return f.refreshTmuxErr
}

// RefreshTmuxClientPriority mirrors RefreshTmuxClient for this fake, tracking its own call
// count so tests can assert the fast-lane path (ResyncOptions.UseFastLane) was used. See
// CapturePaneContentRawPriority's doc comment on why ctx is unused here.
func (f *fakePanePTY) RefreshTmuxClientPriority(ctx context.Context) error {
	f.refreshTmuxPriorityCalled++
	f.fastLaneCtxsSeen = append(f.fastLaneCtxsSeen, ctx)
	return f.refreshTmuxErr
}

func (f *fakePanePTY) GetPaneCursorPosition() (int, int, error) {
	return 0, 0, nil
}

// TestStreamViaTmuxCapturePane_should_EchoResyncIdOnTerminalOutput_When_RequestCarriesResyncId
// is validation.md's AC2 server-side test for Epic 3.2 / Story 3.2.1 (name per that row,
// prefixed with "Test" so `go test` actually runs it): a CurrentPaneRequest carrying a
// resync_id must produce a TerminalOutput whose ResyncId echoes it verbatim. Exercises
// handleCurrentPaneRequest directly — the shared helper streamViaTmuxCapturePane's
// CurrentPaneRequest branch (and both control-mode call sites) delegate to, so this covers
// all three integration points at once.
func TestStreamViaTmuxCapturePane_should_EchoResyncIdOnTerminalOutput_When_RequestCarriesResyncId(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, true))

	target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{ResyncId: "abc-123"}

	output, err := handleCurrentPaneRequest("test-session", target, req, ResyncOptions{EchoResyncID: true})
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}
	if output.ResyncId != "abc-123" {
		t.Errorf("expected TerminalOutput.ResyncId %q, got %q", "abc-123", output.ResyncId)
	}
	if !strings.Contains(string(output.Data), "hello") {
		t.Errorf("expected captured content %q in output data, got %q", "hello", string(output.Data))
	}
}

// TestHandleCurrentPaneRequest_should_LeaveResyncIdEmpty_When_RequestOmitsIt verifies
// the "never invent an ID server-side" requirement: a request with no resync_id set
// must produce a TerminalOutput with an empty ResyncId, not a generated one.
func TestHandleCurrentPaneRequest_should_LeaveResyncIdEmpty_When_RequestOmitsIt(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, true))

	target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{}

	output, err := handleCurrentPaneRequest("test-session", target, req, ResyncOptions{EchoResyncID: true})
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}
	if output.ResyncId != "" {
		t.Errorf("expected empty ResyncId when request omits one, got %q", output.ResyncId)
	}
}

// TestHandleCurrentPaneRequest_should_NotEchoResyncId_When_CorrelationIdFlagIsOff verifies
// Task 3.2.1.1's gating requirement: the resync_id echo is only live behind
// terminal:resync-correlation-id. With the flag off (its default), a request carrying a
// resync_id must still get back an empty ResyncId, matching pre-project behavior exactly.
func TestHandleCurrentPaneRequest_should_NotEchoResyncId_When_CorrelationIdFlagIsOff(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, false))

	target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{ResyncId: "abc-123"}

	output, err := handleCurrentPaneRequest("test-session", target, req, ResyncOptions{})
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}
	if output.ResyncId != "" {
		t.Errorf("expected empty ResyncId when correlation-id flag is off, got %q", output.ResyncId)
	}
}

// TestHandleCurrentPaneRequest_should_LogDebugWhenResyncIdNotEchoed_When_CorrelationIdFlagIsOff
// covers Task 7.1.1.3 (Epic 7.1 observability) server-side half: the client-side
// notifyResyncOutputReceived mismatch log has no visibility into a request whose resync_id
// was silently dropped because terminal:resync-correlation-id was off on the server (the
// client never receives an ID to compare against at all in that case) — this is the only
// place that specific gap is observable, so it must be logged here.
func TestHandleCurrentPaneRequest_should_LogDebugWhenResyncIdNotEchoed_When_CorrelationIdFlagIsOff(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, false))

	target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{ResyncId: "abc-123"}

	restore := captureInfoLog()
	output, err := handleCurrentPaneRequest("test-session", target, req, ResyncOptions{})
	logOutput := restore()

	require.NoError(t, err)
	require.Equal(t, "", output.ResyncId)
	require.Contains(t, logOutput, "resync_id not echoed")
	require.Contains(t, logOutput, "abc-123")
}

// int32Ptr is a small test helper for building *int32 request fields (TargetCols/TargetRows)
// without a throwaway local variable at every call site.
func int32Ptr(v int32) *int32 { return &v }

// TestHandleCurrentPaneRequest_should_SkipResizeAndSigwinchLoop_When_StaleDimensionsTrueAndFlagOn
// is validation.md's AC3 test for Epic 4.1 / Task 4.1.1.1: when the request flags its own
// target dimensions as stale and terminal:resync-skip-stale-dimension-slowpath is on (via
// opts.SkipStaleDimensionSlowPath, resolved by the real caller from that flag), the entire
// resize-through-verify block must be skipped — ResizePTY and RefreshTmuxClient must each be
// called 0 times — and the response must capture at the pane's pre-existing dimensions even
// though they differ from the request's target_cols/target_rows.
func TestHandleCurrentPaneRequest_should_SkipResizeAndSigwinchLoop_When_StaleDimensionsTrueAndFlagOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		TargetCols:      int32Ptr(120),
		TargetRows:      int32Ptr(40),
		StaleDimensions: true,
	}

	output, err := handleCurrentPaneRequest("test-session", target, req, ResyncOptions{SkipStaleDimensionSlowPath: true})
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}

	if target.resizePTYCalled != 0 {
		t.Errorf("expected ResizePTY to be called 0 times, got %d", target.resizePTYCalled)
	}
	if target.refreshTmuxCalled != 0 {
		t.Errorf("expected RefreshTmuxClient to be called 0 times, got %d", target.refreshTmuxCalled)
	}
	// Dimensions must be untouched (still the pane's pre-existing 80x24), not resized to
	// the request's stale 120x40 target.
	if target.cols != 80 || target.rows != 24 {
		t.Errorf("expected pane dimensions to remain 80x24, got %dx%d", target.cols, target.rows)
	}
	if !strings.Contains(string(output.Data), "hello") {
		t.Errorf("expected captured content %q in output data, got %q", "hello", string(output.Data))
	}
}

// TestHandleCurrentPaneRequest_should_RunFullSlowPath_When_StaleDimensionsFalseOrFlagOff is
// validation.md's AC3 negative-case test for Epic 4.1 / Task 4.1.1.1: the resize+SIGWINCH+verify
// slow path must run unchanged (ResizePTY once, RefreshTmuxClient 3 times) whenever either
// StaleDimensions is false or the skip option is off — the skip must never fire on its own.
func TestHandleCurrentPaneRequest_should_RunFullSlowPath_When_StaleDimensionsFalseOrFlagOff(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	testCases := []struct {
		name            string
		staleDimensions bool
		opts            ResyncOptions
	}{
		{
			name:            "stale dimensions false, flag on",
			staleDimensions: false,
			opts:            ResyncOptions{SkipStaleDimensionSlowPath: true},
		},
		{
			name:            "stale dimensions true, flag off",
			staleDimensions: true,
			opts:            ResyncOptions{SkipStaleDimensionSlowPath: false},
		},
		{
			name:            "stale dimensions false, flag off",
			staleDimensions: false,
			opts:            ResyncOptions{SkipStaleDimensionSlowPath: false},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
			req := &sessionv1.CurrentPaneRequest{
				TargetCols:      int32Ptr(120),
				TargetRows:      int32Ptr(40),
				StaleDimensions: tc.staleDimensions,
			}

			_, err := handleCurrentPaneRequest("test-session", target, req, tc.opts)
			if err != nil {
				t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
			}

			if target.resizePTYCalled != 1 {
				t.Errorf("expected ResizePTY to be called once, got %d", target.resizePTYCalled)
			}
			if target.refreshTmuxCalled != 3 {
				t.Errorf("expected RefreshTmuxClient to be called 3 times, got %d", target.refreshTmuxCalled)
			}
			if target.cols != 120 || target.rows != 40 {
				t.Errorf("expected pane resized to 120x40, got %dx%d", target.cols, target.rows)
			}
		})
	}
}

// TestStreamViaTmuxCapturePane_should_CaptureAtExistingPaneDimensions_When_StaleDimensionsTrueAndFlagOn
// is validation.md's AC3 integration-point test for Epic 4.1: streamViaTmuxCapturePane's
// mid-stream CurrentPaneRequest handling delegates entirely to handleCurrentPaneRequest (see
// TestStreamViaTmuxCapturePane_should_EchoResyncIdOnTerminalOutput_When_RequestCarriesResyncId's
// doc comment for why exercising the shared helper covers all three call sites), so this
// confirms the skip path leaves the pane captured at its existing dimensions rather than the
// request's (stale, per the client) target dimensions.
func TestStreamViaTmuxCapturePane_should_CaptureAtExistingPaneDimensions_When_StaleDimensionsTrueAndFlagOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	target := &fakePanePTY{captureContent: "existing-dimension-content", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		TargetCols:      int32Ptr(200),
		TargetRows:      int32Ptr(50),
		StaleDimensions: true,
	}

	output, err := handleCurrentPaneRequest("test-session", target, req, ResyncOptions{SkipStaleDimensionSlowPath: true})
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}

	if target.resizePTYCalled != 0 {
		t.Errorf("expected ResizePTY to be called 0 times, got %d", target.resizePTYCalled)
	}
	if target.cols != 80 || target.rows != 24 {
		t.Errorf("expected capture at pre-existing 80x24 dimensions, got %dx%d", target.cols, target.rows)
	}
	if !strings.Contains(string(output.Data), "existing-dimension-content") {
		t.Errorf("expected captured content in output data, got %q", string(output.Data))
	}
}

// TestRunInputReadLoop_should_InvokeOnCurrentPaneRequestOnce_When_CurrentPaneRequestFrameArrives
// is the server-side test for Story 3.2.2 / AC2: a CurrentPaneRequest frame arriving
// mid-stream on runInputReadLoop must invoke onCurrentPaneRequest exactly once, and its
// returned TerminalOutput (with ResyncId echoed) must be written back on the stream —
// while other frame types (Input, Resize, ScrollbackRequest — Task 3.2.2.3's explicit
// negative cases) must not trigger it.
func TestRunInputReadLoop_should_InvokeOnCurrentPaneRequestOnce_When_CurrentPaneRequestFrameArrives(t *testing.T) {
	t.Parallel()
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	var mu sync.Mutex
	var invocations int
	var lastReq *sessionv1.CurrentPaneRequest
	onCurrentPaneRequest := func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		mu.Lock()
		defer mu.Unlock()
		invocations++
		lastReq = req
		return &sessionv1.TerminalOutput{ResyncId: req.GetResyncId(), Data: []byte("snapshot")}, nil
	}
	onInput := func(data []byte) {}
	onResize := func(cols, rows int) {}
	onScrollbackRequest := func(startLine, endLine string) (string, error) { return "", nil }

	doneChan := make(chan struct{})
	errChan := make(chan error, 2)
	done := make(chan struct{})
	var resizeSettling atomic.Bool
	go func() {
		runInputReadLoop(serverStream, doneChan, errChan, "test-session", onInput, onResize, onScrollbackRequest, onCurrentPaneRequest, &resizeSettling)
		close(done)
	}()
	defer func() {
		_ = clientConn.Close()
		<-done
	}()

	// A non-CurrentPaneRequest frame (plain input) must not trigger the callback.
	sendFrame := func(td *sessionv1.TerminalData) {
		t.Helper()
		dataBytes, err := proto.Marshal(td)
		if err != nil {
			t.Fatalf("failed to marshal TerminalData: %v", err)
		}
		envelope := protocol.CreateEnvelope(0, dataBytes)
		if err := clientConn.WriteMessage(websocket.BinaryMessage, envelope); err != nil {
			t.Fatalf("failed to write envelope: %v", err)
		}
	}
	sendFrame(&sessionv1.TerminalData{
		Data: &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: []byte("hi")}},
	})

	// A resize frame must not trigger the callback either.
	sendFrame(&sessionv1.TerminalData{
		Data: &sessionv1.TerminalData_Resize{Resize: &sessionv1.TerminalResize{Cols: 80, Rows: 24}},
	})

	// Nor must a ScrollbackRequest frame — it has its own dedicated callback.
	sendFrame(&sessionv1.TerminalData{
		Data: &sessionv1.TerminalData_ScrollbackRequest{ScrollbackRequest: &sessionv1.ScrollbackRequest{}},
	})

	sendFrame(&sessionv1.TerminalData{
		Data: &sessionv1.TerminalData_CurrentPaneRequest{
			CurrentPaneRequest: &sessionv1.CurrentPaneRequest{ResyncId: "xyz"},
		},
	})

	if err := clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}

	// The ScrollbackRequest frame gets its own response first (its own dedicated
	// callback/branch) — drain it before reading the CurrentPaneRequest's response.
	_, scrollbackRespMsg, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read scrollback response from server: %v", err)
	}
	scrollbackEnvelope, _, err := protocol.ParseEnvelope(scrollbackRespMsg)
	if err != nil {
		t.Fatalf("failed to parse scrollback response envelope: %v", err)
	}
	var scrollbackRespData sessionv1.TerminalData
	if err := proto.Unmarshal(scrollbackEnvelope.Data, &scrollbackRespData); err != nil {
		t.Fatalf("failed to unmarshal scrollback response TerminalData: %v", err)
	}
	if scrollbackRespData.GetScrollbackResponse() == nil {
		t.Fatalf("expected a ScrollbackResponse, got %+v", &scrollbackRespData)
	}

	// Read the response the server wrote back for the CurrentPaneRequest frame.
	_, respMsg, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read response from server: %v", err)
	}
	envelope, _, err := protocol.ParseEnvelope(respMsg)
	if err != nil {
		t.Fatalf("failed to parse response envelope: %v", err)
	}
	var respData sessionv1.TerminalData
	if err := proto.Unmarshal(envelope.Data, &respData); err != nil {
		t.Fatalf("failed to unmarshal response TerminalData: %v", err)
	}
	output := respData.GetOutput()
	if output == nil {
		t.Fatalf("expected an Output response, got %+v", &respData)
	}
	if output.ResyncId != "xyz" {
		t.Errorf("expected response ResyncId %q, got %q", "xyz", output.ResyncId)
	}

	mu.Lock()
	defer mu.Unlock()
	if invocations != 1 {
		t.Fatalf("expected onCurrentPaneRequest to be invoked exactly once, got %d", invocations)
	}
	if lastReq == nil || lastReq.GetResyncId() != "xyz" {
		t.Fatalf("expected onCurrentPaneRequest to receive resync_id %q, got %+v", "xyz", lastReq)
	}
}

// sendInputIgnoringError attempts to write an input envelope to a (possibly
// already-closed) client connection, discarding any error. Used to simulate
// the client "attempting" to send more input after it has closed its end.
func sendInputIgnoringError(conn *websocket.Conn, payload []byte) error {
	td := &sessionv1.TerminalData{
		Data: &sessionv1.TerminalData_Input{
			Input: &sessionv1.TerminalInput{Data: payload},
		},
	}
	dataBytes, err := proto.Marshal(td)
	if err != nil {
		return err
	}
	envelope := protocol.CreateEnvelope(0, dataBytes)
	return conn.WriteMessage(websocket.BinaryMessage, envelope)
}

// TestAnsiSnapshotPrefixContainsRequiredSequences verifies the prefix used before
// every snapshot contains DECSTR, ED2, and CUP in that order.
// Regression for Bug A: a prefix without ED2 (screen clear) caused double display.
func TestAnsiSnapshotPrefixContainsRequiredSequences(t *testing.T) {
	t.Parallel()
	decstr := "\x1b[!p"
	ed2 := "\x1b[2J"
	cup := "\x1b[H"

	for _, seq := range []string{decstr, ed2, cup} {
		if !strings.Contains(ansiSnapshotPrefix, seq) {
			t.Errorf("ansiSnapshotPrefix missing required sequence %q; prefix = %q", seq, ansiSnapshotPrefix)
		}
	}

	// Order matters: DECSTR must precede ED2 (so the scroll region is reset before
	// the clear), and ED2 must precede CUP (so the screen is blank before cursor
	// home). A wrong order could still clear only a partial scroll region.
	dIdx := strings.Index(ansiSnapshotPrefix, decstr)
	eIdx := strings.Index(ansiSnapshotPrefix, ed2)
	cIdx := strings.Index(ansiSnapshotPrefix, cup)

	if dIdx >= eIdx || eIdx >= cIdx {
		t.Errorf("ansiSnapshotPrefix sequence order wrong: DECSTR@%d ED2@%d CUP@%d; want DECSTR < ED2 < CUP; prefix = %q",
			dIdx, eIdx, cIdx, ansiSnapshotPrefix)
	}
}

// TestStripAnsiCodesHandlesNonLetterCSITerminators verifies stripAnsiCodes (used by
// detectContentWidth to count visible characters) strips CSI sequences terminated by
// a non-letter final byte. The CSI final-byte range is 0x40-0x7E per ECMA-48, not just
// A-Z/a-z; a letter-only class would leave these bytes in the "visible" count.
func TestStripAnsiCodesHandlesNonLetterCSITerminators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "insert_character_at_sign", input: "\x1b[5@Hello", want: "Hello"},
		{name: "tilde_terminator", input: "\x1b[3~Hello", want: "Hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stripAnsiCodes(tt.input)
			if got != tt.want {
				t.Errorf("stripAnsiCodes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestInvalidateSnapshotForcesRecapture verifies that invalidateSnapshot causes the
// next getOrRefreshSnapshot to re-run captureFn.
//
// Regression guard for garbled-terminal-on-connect: streamViaControlMode performs a
// ±1 resize nudge to force the TUI to repaint, which makes any cached capture-pane
// content stale by construction (it was captured at the pre-nudge dimensions). The
// nudge cannot rely on markSnapshotDirty to invalidate it, because markSnapshotDirty
// is only called from the output-forwarding goroutine, which does not start until
// after the initial snapshot has already been captured and sent.
func TestInvalidateSnapshotForcesRecapture(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	calls := 0
	captureFn := func() (string, error) {
		calls++
		return fmt.Sprintf("content%d", calls), nil
	}

	// Populate the cache (simulates a snapshot captured at the previous dimensions).
	if _, err := h.getOrRefreshSnapshot("sess1", captureFn); err != nil {
		t.Fatalf("unexpected error priming cache: %v", err)
	}

	// A resize nudge just forced a repaint — the cached content is now stale.
	h.invalidateSnapshot("sess1")

	got, err := h.getOrRefreshSnapshot("sess1", captureFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("captureFn called %d times after invalidate, want 2 (stale snapshot was served)", calls)
	}
	if got != "content2" {
		t.Errorf("got %q after invalidate, want %q", got, "content2")
	}
}

// TestInvalidateSnapshotOnUnknownSessionIsNoOp verifies invalidating an absent
// session neither panics nor creates a cache entry.
func TestInvalidateSnapshotOnUnknownSessionIsNoOp(t *testing.T) {
	t.Parallel()
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)

	h.invalidateSnapshot("nonexistent-session")

	if _, ok := h.snapshotCache.Load("nonexistent-session"); ok {
		t.Error("invalidateSnapshot created a cache entry for an unknown session")
	}
}

// TestWaitForQuiescenceReturnsAfterQuietForWhenNoProducer documents that
// waitForQuiescence degenerates to a fixed quietFor sleep when nothing ever signals
// the channel. streamViaControlMode's initial call is in exactly that situation: the
// only producer (the output-forwarding goroutine) starts after the call site, so the
// "wait for the TUI to finish redrawing" is really a short fixed delay and its
// timeout warning can never fire.
//
// This test pins the current semantics so the trap is visible; making the initial
// wait genuinely quiescence-driven requires subscribing a producer before the nudge.
func TestWaitForQuiescenceReturnsAfterQuietForWhenNoProducer(t *testing.T) {
	t.Parallel()
	ch := make(chan struct{}, 16) // no producer, mirroring the initial-nudge call site

	start := time.Now()
	waitForQuiescence(ch, 500*time.Millisecond, 50*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed >= 400*time.Millisecond {
		t.Errorf("waited %v — expected to return after quietFor (~50ms), not the 500ms timeout", elapsed)
	}
	if elapsed < 40*time.Millisecond {
		t.Errorf("returned after %v — expected to wait at least quietFor (~50ms)", elapsed)
	}
}

// fakeCursorPositioner is a cursorPositioner stub for withCursorSync tests.
type fakeCursorPositioner struct {
	x, y int
	err  error
}

func (f fakeCursorPositioner) GetPaneCursorPosition() (int, int, error) {
	return f.x, f.y, f.err
}

// TestWithCursorSyncAppendsOneBasedCUP verifies the trailing CUP escape converts
// tmux's 0-based cursor coords to CUP's 1-based row;col form.
func TestWithCursorSyncAppendsOneBasedCUP(t *testing.T) {
	t.Parallel()
	got := withCursorSync("content", fakeCursorPositioner{x: 4, y: 9})
	want := "content\x1b[10;5H"
	if got != want {
		t.Errorf("withCursorSync = %q, want %q", got, want)
	}
}

// TestWithCursorSyncPassesThroughOnErrorOrNilTarget verifies content is returned
// unchanged when the cursor position is unavailable.
func TestWithCursorSyncPassesThroughOnErrorOrNilTarget(t *testing.T) {
	t.Parallel()
	if got := withCursorSync("content", nil); got != "content" {
		t.Errorf("nil target: got %q, want unchanged", got)
	}
	failing := fakeCursorPositioner{x: 1, y: 1, err: fmt.Errorf("pane gone")}
	if got := withCursorSync("content", failing); got != "content" {
		t.Errorf("error target: got %q, want unchanged", got)
	}
}

// slowCursorPositioner blocks for longer than withCursorSyncTimeout,
// simulating a degraded GetPaneCursorPosition — see withCursorSyncTimeout's
// doc comment for why it has no bound of its own. calledCh, if non-nil, is
// closed once GetPaneCursorPosition actually returns — lets a test wait for
// withCursorSync's deliberately-abandoned goroutine to finish before the
// test itself returns, so it can never bleed into a later test's
// goleak-style leak check.
type slowCursorPositioner struct {
	delay    time.Duration
	calledCh chan struct{}
}

func (s slowCursorPositioner) GetPaneCursorPosition() (int, int, error) {
	time.Sleep(s.delay)
	if s.calledCh != nil {
		close(s.calledCh)
	}
	return 1, 1, nil
}

// TestWithCursorSync_should_ReturnWithinTimeout_When_PositionLookupIsSlow is
// the regression test for the 2026-08-25 latency fix: handleCurrentPaneRequest
// calls withCursorSync on the client-triggered resync path, which the
// frontend force-disconnects if it doesn't respond within 4s
// (useVisibilityResync.ts's stall watchdog) — an unbounded cursor lookup
// could single-handedly blow that budget and trigger a disconnect+reconnect,
// which (StartControlMode/StopControlMode refcounting) tears down and
// restarts control mode.
func TestWithCursorSync_should_ReturnWithinTimeout_When_PositionLookupIsSlow(t *testing.T) {
	calledCh := make(chan struct{})
	// Delay only modestly past the timeout — long enough to prove
	// withCursorSync doesn't wait for it, short enough that this test's own
	// cleanup wait below (for the abandoned goroutine) doesn't slow the
	// suite down.
	slow := slowCursorPositioner{delay: withCursorSyncTimeout + 50*time.Millisecond, calledCh: calledCh}

	start := time.Now()
	got := withCursorSync("content", slow)
	elapsed := time.Since(start)

	if got != "content" {
		t.Errorf("withCursorSync() = %q, want content left unchanged when the lookup times out", got)
	}
	if elapsed >= slow.delay {
		t.Errorf("withCursorSync() took %s, want it bounded by withCursorSyncTimeout (%s), well under the %s lookup delay", elapsed, withCursorSyncTimeout, slow.delay)
	}

	// Let the abandoned goroutine actually finish before this test returns —
	// see the identical comment in streamhub's copy of this test.
	<-calledCh
}

// TestWaitForEvent_should_ReturnTrue_When_MatchingEventArrivesDuringWait proves waitForEvent
// observes an event published mid-wait, not just at subscribe time.
func TestWaitForEvent_should_ReturnTrue_When_MatchingEventArrivesDuringWait(t *testing.T) {
	bus := events.NewEventBus(1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		bus.Publish(&events.Event{Type: events.EventSessionUpdated})
	}()

	start := time.Now()
	got := waitForEvent(bus, 2*time.Second, func(ev *events.Event) bool { return ev.Type == events.EventSessionUpdated })
	elapsed := time.Since(start)

	if !got {
		t.Error("expected true once a matching event is published")
	}
	if elapsed >= 2*time.Second {
		t.Errorf("expected to return well before the 2s timeout, took %v", elapsed)
	}
}

// TestWaitForEvent_should_ReturnFalse_When_NoMatchingEventWithinTimeout proves the bound:
// waitForEvent gives up after timeout instead of blocking forever.
func TestWaitForEvent_should_ReturnFalse_When_NoMatchingEventWithinTimeout(t *testing.T) {
	bus := events.NewEventBus(1)

	start := time.Now()
	got := waitForEvent(bus, 100*time.Millisecond, func(ev *events.Event) bool { return true })
	elapsed := time.Since(start)

	if got {
		t.Error("expected false when no event is ever published")
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected to wait out the full 100ms timeout, took %v", elapsed)
	}
}

// TestWaitForEvent_should_ReturnFalse_When_BusIsNil covers a nil/unwired event bus (e.g. a
// test double) — must not panic or hang.
func TestWaitForEvent_should_ReturnFalse_When_BusIsNil(t *testing.T) {
	if waitForEvent(nil, time.Second, func(ev *events.Event) bool { return true }) {
		t.Error("expected false for a nil bus")
	}
}

// TestWaitForInstanceStartedEvent_should_Ignore_UnrelatedUpdateOnSameInstance is the
// regression test for the false-positive this predicate must avoid: many code paths publish
// EventSessionUpdated for reasons unrelated to Start() completing (title rename, rate-limit
// state, PR URL, ...). An event for the right instance whose Session isn't actually
// Started() must not be treated as the started signal.
func TestWaitForInstanceStartedEvent_should_Ignore_UnrelatedUpdateOnSameInstance(t *testing.T) {
	inst := &session.Instance{UUID: "same-uuid"}
	bus := events.NewEventBus(1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"title"}))
	}()

	got := waitForInstanceStartedEvent(bus, inst, 200*time.Millisecond)

	if got {
		t.Error("expected false: the published event's Session is not actually Started()")
	}
}

// TestWaitForInstanceStartedEvent_should_ReturnFalse_When_BusIsNil covers the fallback path
// for an unwired event bus — falls back to instance.Started() rather than hanging.
func TestWaitForInstanceStartedEvent_should_ReturnFalse_When_BusIsNil(t *testing.T) {
	inst := &session.Instance{UUID: "some-uuid"}

	if waitForInstanceStartedEvent(nil, inst, 50*time.Millisecond) {
		t.Error("expected false: nil bus falls back to instance.Started(), which is false for a never-started Instance")
	}
}

// TestAllSnapshotSendsUseCursorSync guards the invariant that every full-screen
// snapshot send ends with a cursor-sync.
//
// prepareSnapshotContent deliberately strips absolute-cursor codes, so a snapshot
// written without a trailing CUP leaves xterm.js's cursor desynced from the tmux pane
// cursor; relative cursor-up redraws from an Ink TUI then rewind to the wrong row and
// each repaint stacks below the previous one ("billowing"). The post-resize snapshot
// omitted this call, which meant resizing to clear a garbled pane left interactive
// menus stacking their previously highlighted option.
//
// This is a source-level guard because the snapshot sends live inside long-lived
// streaming goroutines with no injectable seam. If those are ever refactored behind a
// single snapshot-composition helper, replace this with a direct test of that helper.
func TestAllSnapshotSendsUseCursorSync(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("connectrpc_websocket.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	for i, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Snapshot frames are composed as <prefix> + prepareSnapshotContent(...).
		if !strings.Contains(trimmed, "prepareSnapshotContent(") {
			continue
		}
		// Skip the helper's own declaration.
		if strings.HasPrefix(trimmed, "func prepareSnapshotContent") {
			continue
		}
		if !strings.Contains(trimmed, "withCursorSync(") {
			t.Errorf("connectrpc_websocket.go:%d composes a snapshot without withCursorSync:\n\t%s", i+1, trimmed)
		}
	}
}

// TestHandleBatchedCurrentPaneRequest_should_DispatchNIndividuallyTaggedResponses_When_BatchingFlagOn
// is validation.md's AC6b happy-path test for Epic 5.2 / Story 5.2.1: a
// BatchedCurrentPaneRequest coalescing N CurrentPaneRequests (as the client's stagger
// coordinator produces when terminal:resync-batching is on) must produce N TerminalOutput
// replies, each still carrying its own request's resync_id — batching must not collapse
// or merge the individual responses.
func TestHandleBatchedCurrentPaneRequest_should_DispatchNIndividuallyTaggedResponses_When_BatchingFlagOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, true))

	target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
	batch := &sessionv1.BatchedCurrentPaneRequest{
		Requests: []*sessionv1.CurrentPaneRequest{
			{ResyncId: "resync-1"},
			{ResyncId: "resync-2"},
			{ResyncId: "resync-3"},
		},
	}

	onCurrentPaneRequest := func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		return handleCurrentPaneRequest("test-session", target, req, ResyncOptions{EchoResyncID: true})
	}

	outputs := handleBatchedCurrentPaneRequest("test-session", batch, onCurrentPaneRequest)

	if len(outputs) != 3 {
		t.Fatalf("expected 3 outputs, got %d", len(outputs))
	}
	for i, wantID := range []string{"resync-1", "resync-2", "resync-3"} {
		if outputs[i].ResyncId != wantID {
			t.Errorf("output[%d]: expected ResyncId %q, got %q", i, wantID, outputs[i].ResyncId)
		}
		if !strings.Contains(string(outputs[i].Data), "hello") {
			t.Errorf("output[%d]: expected captured content %q in output data, got %q", i, "hello", string(outputs[i].Data))
		}
	}
}

// TestHandleBatchedCurrentPaneRequest_should_PreserveCorrelationPerRequest_When_ThreeCoalescedRequestsHaveDistinctResyncIds
// is validation.md's AC6b integration test: three coalesced requests with distinct
// resync_ids must map 1:1 to three outputs in the same order, even when one of the
// coalesced requests' captures fails — the failure must be skipped (logged), not corrupt
// or misattribute another sibling's resync_id.
func TestHandleBatchedCurrentPaneRequest_should_PreserveCorrelationPerRequest_When_ThreeCoalescedRequestsHaveDistinctResyncIds(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, true))

	batch := &sessionv1.BatchedCurrentPaneRequest{
		Requests: []*sessionv1.CurrentPaneRequest{
			{ResyncId: "alpha"},
			{ResyncId: "bravo"},
			{ResyncId: "charlie"},
		},
	}

	// bravo's underlying capture fails; alpha and charlie must still come back correctly
	// correlated to their own resync_id, with bravo simply absent from the results.
	onCurrentPaneRequest := func(req *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		if req.GetResyncId() == "bravo" {
			return nil, fmt.Errorf("simulated capture failure")
		}
		target := &fakePanePTY{captureContent: req.GetResyncId() + "-content", cols: 80, rows: 24}
		return handleCurrentPaneRequest("test-session", target, req, ResyncOptions{EchoResyncID: true})
	}

	outputs := handleBatchedCurrentPaneRequest("test-session", batch, onCurrentPaneRequest)

	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs (bravo skipped), got %d", len(outputs))
	}
	if outputs[0].ResyncId != "alpha" || !strings.Contains(string(outputs[0].Data), "alpha-content") {
		t.Errorf("outputs[0]: expected alpha's tagged content, got ResyncId=%q Data=%q", outputs[0].ResyncId, outputs[0].Data)
	}
	if outputs[1].ResyncId != "charlie" || !strings.Contains(string(outputs[1].Data), "charlie-content") {
		t.Errorf("outputs[1]: expected charlie's tagged content, got ResyncId=%q Data=%q", outputs[1].ResyncId, outputs[1].Data)
	}
}

// allTerminalResyncFlagNames lists every terminal-resync feature flag added across this
// project (Epics 3.2, 4.1, 4.2, 5.1, 5.2, 6.1, 8.3). Epic 8.1's job is to prove all seven
// compose correctly, so the round-trip and spot-check tests below need the complete set in
// one place rather than each hand-rolling its own (partial, driftable) list. All seven have
// named Go constants (see feature_flag_service.go); three of them
// (terminalResyncVisibilityScopeFlagName, terminalResyncStaggerFlagName,
// terminalResyncBatchingFlagName) are pure client-side concerns with no Go production call
// site to share the constant with, but are still named for consistency and so this list and
// knownFeatureFlags can't drift on the string value.
var allTerminalResyncFlagNames = []string{
	terminalResyncVisibilityScopeFlagName,
	terminalResyncCorrelationIDFlagName,
	terminalResyncSkipStaleDimensionSlowpathFlagName,
	terminalResyncExecGateFastLaneFlagName,
	terminalResyncStaggerFlagName,
	terminalResyncCompressionFlagName,
	terminalResyncBatchingFlagName,
}

// setAllTerminalResyncFlags sets every flag in allTerminalResyncFlagNames to on, restoring
// all of them to false via t.Cleanup so a later test in the package doesn't inherit
// leftover true state.
func setAllTerminalResyncFlags(t *testing.T, on bool) {
	t.Helper()
	for _, name := range allTerminalResyncFlagNames {
		require.NoError(t, config.LoadConfig().SetFeatureFlag(name, on))
	}
	t.Cleanup(func() {
		for _, name := range allTerminalResyncFlagNames {
			_ = config.LoadConfig().SetFeatureFlag(name, false)
		}
	})
}

// setOnlyResyncFlag sets flagName on and every other terminal-resync flag off, so a spot
// check can attribute any observed behavior change to that one flag alone. Restores every
// flag to false via t.Cleanup.
func setOnlyResyncFlag(t *testing.T, flagName string) {
	t.Helper()
	for _, name := range allTerminalResyncFlagNames {
		require.NoError(t, config.LoadConfig().SetFeatureFlag(name, name == flagName))
	}
	t.Cleanup(func() {
		for _, name := range allTerminalResyncFlagNames {
			_ = config.LoadConfig().SetFeatureFlag(name, false)
		}
	})
}

// TestFullResyncRoundTrip_should_MatchPreProjectBaseline_When_AllSevenFlagsOff is Epic 8.1 /
// Task 8.1.1.1 (validation.md AC7 row: resyncFlow_should_BeByteForByteIdenticalToBaseline_
// When_AllSevenFlagsOff, Test-prefixed here since Go requires it to actually run). It drives
// a full mid-stream CurrentPaneRequest through handleCurrentPaneRequestFrame — the same
// frame-handling entry point streamViaControlMode's runInputReadLoop uses — over a real
// WebSocket pair, and asserts the wire-level TerminalOutput matches pre-project behavior on
// every axis this project touched: no resync_id echoed, the full resize+SIGWINCH+verify slow
// path always runs (never skipped), the default (non-fast-lane) capture/refresh methods are
// used, and the response envelope carries no compression flag.
func TestFullResyncRoundTrip_should_MatchPreProjectBaseline_When_AllSevenFlagsOff(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setAllTerminalResyncFlags(t, false)

	stream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	target := &fakePanePTY{captureContent: "baseline-content", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		ResyncId:   "should-not-be-echoed",
		TargetCols: int32Ptr(120),
		TargetRows: int32Ptr(40),
	}

	onCurrentPaneRequest := func(r *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		return handleCurrentPaneRequest("test-session", target, r, currentResyncOptions())
	}
	var resizeSettling atomic.Bool
	handleCurrentPaneRequestFrame(stream, "test-session", req, onCurrentPaneRequest, &resizeSettling)

	env := readEnvelopeFromClient(t, clientConn)
	if env.Flags != 0 {
		t.Errorf("expected envelope flags 0x00 (no compression, not end-of-stream), got 0x%02x", env.Flags)
	}

	var terminalData sessionv1.TerminalData
	if err := proto.Unmarshal(env.Data, &terminalData); err != nil {
		t.Fatalf("failed to unmarshal TerminalData: %v", err)
	}
	output := terminalData.GetOutput()

	if output.ResyncId != "" {
		t.Errorf("expected empty ResyncId (correlation-id flag off), got %q", output.ResyncId)
	}
	if target.resizePTYCalled != 1 {
		t.Errorf("expected ResizePTY called once (full slow path), got %d", target.resizePTYCalled)
	}
	if target.refreshTmuxCalled != 3 {
		t.Errorf("expected RefreshTmuxClient called 3 times (full slow path), got %d", target.refreshTmuxCalled)
	}
	if target.capturePaneCalled != 1 {
		t.Errorf("expected default CapturePaneContent called once, got %d", target.capturePaneCalled)
	}
	if target.capturePanePriorityCalled != 0 || target.refreshTmuxPriorityCalled != 0 {
		t.Errorf("expected no fast-lane calls, got capturePriority=%d refreshPriority=%d",
			target.capturePanePriorityCalled, target.refreshTmuxPriorityCalled)
	}
	if !strings.Contains(string(output.Data), "baseline-content") {
		t.Errorf("expected captured content in output data, got %q", output.Data)
	}
}

// TestFullResyncRoundTrip_should_ExhibitAllSevenBehaviors_When_AllSevenFlagsOn is Epic 8.1 /
// Task 8.1.1.2, the "kitchen sink" test: every one of the seven flags on at once, asserting
// each fix's server-observable behavior fires simultaneously and none suppresses another
// (the cross-flag-coupling risk this epic exists to catch).
//
// terminal:resync-visibility-scope and terminal:resync-stagger have no server-side code path
// at all (both are pure client-side concerns — see this project's validation.md, which marks
// their integration-test cells "N/A — client-only"), so there is nothing for a Go test to
// observe for either; their behavior is covered client-side (e.g.
// web-app/src/components/sessions/__tests__/ResyncStaggerQueue.test.ts).
//
// terminal:resync-compression is now wired into writeCurrentPaneResponse
// (connectrpc_websocket.go), which gzip-compresses the marshaled reply and sets the
// envelope's CompressedFlag bit when the flag is on and the payload exceeds
// terminalResyncCompressionThresholdBytes (1024 bytes) — see
// TestHandleCurrentPaneRequest_should_RoundTripCompressedTerminalOutput_When_PayloadExceedsSizeThresholdAndCompressionFlagOn
// for the dedicated round-trip test. This kitchen-sink request's captured content
// ("kitchen-sink-content", well under the threshold) intentionally stays below it, so
// CompressedFlag must still be unset here — this assertion documents the "flag on but
// payload too small" case, not the compression primitive's absence.
func TestFullResyncRoundTrip_should_ExhibitAllSevenBehaviors_When_AllSevenFlagsOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setAllTerminalResyncFlags(t, true)

	stream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	target := &fakePanePTY{captureContent: "kitchen-sink-content", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		ResyncId:        "resync-kitchen-sink",
		TargetCols:      int32Ptr(120),
		TargetRows:      int32Ptr(40),
		StaleDimensions: true,
	}

	onCurrentPaneRequest := func(r *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		return handleCurrentPaneRequest("test-session", target, r, currentResyncOptions())
	}
	var resizeSettling atomic.Bool
	handleCurrentPaneRequestFrame(stream, "test-session", req, onCurrentPaneRequest, &resizeSettling)

	env := readEnvelopeFromClient(t, clientConn)
	var terminalData sessionv1.TerminalData
	if err := proto.Unmarshal(env.Data, &terminalData); err != nil {
		t.Fatalf("failed to unmarshal TerminalData: %v", err)
	}
	output := terminalData.GetOutput()

	// terminal:resync-correlation-id: resync_id echoed verbatim.
	if output.ResyncId != "resync-kitchen-sink" {
		t.Errorf("expected ResyncId echoed, got %q", output.ResyncId)
	}
	// terminal:resync-skip-stale-dimension-slowpath: the resize+SIGWINCH+verify block must
	// not run at all (request set StaleDimensions true).
	if target.resizePTYCalled != 0 || target.refreshTmuxCalled != 0 || target.refreshTmuxPriorityCalled != 0 {
		t.Errorf("expected slow path fully skipped, got resize=%d refresh=%d refreshPriority=%d",
			target.resizePTYCalled, target.refreshTmuxCalled, target.refreshTmuxPriorityCalled)
	}
	// terminal:resync-exec-gate-fast-lane: capture routed through the priority method.
	if target.capturePanePriorityCalled != 1 || target.capturePaneCalled != 0 {
		t.Errorf("expected fast-lane capture used, got priority=%d default=%d",
			target.capturePanePriorityCalled, target.capturePaneCalled)
	}
	if !strings.Contains(string(output.Data), "kitchen-sink-content") {
		t.Errorf("expected captured content in output data, got %q", output.Data)
	}

	// terminal:resync-compression: this kitchen-sink payload stays under
	// terminalResyncCompressionThresholdBytes, so CompressedFlag must not be set even with
	// the flag on — see the doc comment above and
	// TestHandleCurrentPaneRequest_should_RoundTripCompressedTerminalOutput_When_PayloadExceedsSizeThresholdAndCompressionFlagOn
	// for the over-threshold case.
	if env.Flags&protocol.CompressedFlag != 0 {
		t.Errorf("expected CompressedFlag unset for a payload under the compression threshold, got env.Flags=0x%02x", env.Flags)
	}

	// terminal:resync-batching: server-side effect is answering each coalesced request
	// individually and correctly tagged — see
	// TestHandleBatchedCurrentPaneRequest_should_DispatchNIndividuallyTaggedResponses_When_BatchingFlagOn
	// for the dedicated test. Exercised here too so "all seven simultaneously" actually
	// covers it in this same all-flags-on context, not only in isolation.
	batchTarget := &fakePanePTY{captureContent: "batch-content", cols: 80, rows: 24}
	batchOnCurrentPaneRequest := func(r *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		return handleCurrentPaneRequest("test-session", batchTarget, r, currentResyncOptions())
	}
	batch := &sessionv1.BatchedCurrentPaneRequest{
		Requests: []*sessionv1.CurrentPaneRequest{{ResyncId: "batch-1"}, {ResyncId: "batch-2"}},
	}
	outputs := handleBatchedCurrentPaneRequest("test-session", batch, batchOnCurrentPaneRequest)
	if len(outputs) != 2 || outputs[0].ResyncId != "batch-1" || outputs[1].ResyncId != "batch-2" {
		t.Errorf("expected 2 individually-tagged batched outputs, got %+v", outputs)
	}
}

// TestHandleCurrentPaneRequest_should_RoundTripCompressedTerminalOutput_When_PayloadExceedsSizeThresholdAndCompressionFlagOn
// is validation.md's AC6a integration test (Epic 5.1, Task 5.1.1.4): with
// terminal:resync-compression on and a captured pane content large enough to push the
// marshaled TerminalData reply over terminalResyncCompressionThresholdBytes,
// writeCurrentPaneResponse must set the envelope's CompressedFlag bit and gzip-compress the
// payload — decompressing and unmarshaling it back (mirroring the client's
// parseResponseBody/DecompressionStream('gzip') path in websocket-transport.ts) must recover
// the exact same ResyncId/Data the pre-compression TerminalOutput had.
func TestHandleCurrentPaneRequest_should_RoundTripCompressedTerminalOutput_When_PayloadExceedsSizeThresholdAndCompressionFlagOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setOnlyResyncFlag(t, terminalResyncCompressionFlagName)
	// ResyncId is only echoed back when terminal:resync-correlation-id is on (see
	// handleCurrentPaneRequest's doc comment) — enable it alongside compression so this
	// test can assert the full TerminalOutput (ResyncId and Data) round-trips through
	// compression, not just Data.
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, true))
	t.Cleanup(func() {
		_ = config.LoadConfig().SetFeatureFlag(terminalResyncCorrelationIDFlagName, false)
	})

	// Large enough that the marshaled TerminalData exceeds terminalResyncCompressionThresholdBytes.
	largeContent := strings.Repeat("resync-payload-filler-", 100)
	target := &fakePanePTY{captureContent: largeContent, cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{ResyncId: "compress-me"}

	stream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	onCurrentPaneRequest := func(r *sessionv1.CurrentPaneRequest) (*sessionv1.TerminalOutput, error) {
		return handleCurrentPaneRequest("test-session", target, r, currentResyncOptions())
	}
	var resizeSettling atomic.Bool
	handleCurrentPaneRequestFrame(stream, "test-session", req, onCurrentPaneRequest, &resizeSettling)

	env := readEnvelopeFromClient(t, clientConn)
	if env.Flags&protocol.CompressedFlag == 0 {
		t.Fatalf("expected CompressedFlag set for an over-threshold payload, got env.Flags=0x%02x", env.Flags)
	}

	gz, err := gzip.NewReader(bytes.NewReader(env.Data))
	if err != nil {
		t.Fatalf("failed to open gzip reader on compressed envelope payload: %v", err)
	}
	decompressed, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("failed to decompress envelope payload: %v", err)
	}

	var terminalData sessionv1.TerminalData
	if err := proto.Unmarshal(decompressed, &terminalData); err != nil {
		t.Fatalf("failed to unmarshal decompressed TerminalData: %v", err)
	}
	output := terminalData.GetOutput()
	if output.ResyncId != "compress-me" {
		t.Errorf("expected ResyncId %q round-tripped through compression, got %q", "compress-me", output.ResyncId)
	}
	if !strings.Contains(string(output.Data), largeContent) {
		t.Errorf("expected captured content round-tripped through compression, got %d bytes of data", len(output.Data))
	}
}

// TestHandleCurrentPaneRequest_should_LogSkippedSlowPathWithSessionIdAndElapsedMs_When_StaleDimensionSkipFires
// is validation.md's AC8 unit test: when the stale-dimension skip fires (Epic 4.1), the debug
// log line it emits must carry sessionID, targetCols/targetRows, and estimatedTimeSavedMs —
// the fields Epic 7.1's observability plan assigns to this fix — so an operator can attribute
// a skip event to a specific session and quantify the time saved without instrumenting a
// separate metric.
func TestHandleCurrentPaneRequest_should_LogSkippedSlowPathWithSessionIdAndElapsedMs_When_StaleDimensionSkipFires(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	target := &fakePanePTY{captureContent: "hello", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		TargetCols:      int32Ptr(120),
		TargetRows:      int32Ptr(40),
		StaleDimensions: true,
	}

	restore := captureInfoLog()
	_, err := handleCurrentPaneRequest("skip-log-session", target, req, ResyncOptions{SkipStaleDimensionSlowPath: true})
	logOutput := restore()

	require.NoError(t, err)
	require.Contains(t, logOutput, "skipping stale-dimension resize slow path")
	require.Contains(t, logOutput, "sessionID=skip-log-session")
	// targetCols/targetRows are dereferenced via derefOr before logging, so they render as
	// the actual request values (120/40) rather than *int32 pointer addresses.
	require.Contains(t, logOutput, "targetCols=120")
	require.Contains(t, logOutput, "targetRows=40")
	require.Contains(t, logOutput, "estimatedTimeSavedMs=450")
}

// The following three tests are Epic 8.1 / Task 8.1.1.3's spot checks: with every flag
// except one held off, only that one flag's behavior must change. This is the check that
// would catch a cross-flag coupling bug — e.g. the fast-lane flag accidentally also
// suppressing the correlation-id echo, or vice versa.

// TestHandleCurrentPaneRequest_should_OnlyRouteFastLane_When_OnlyExecGateFastLaneFlagOn spot
// checks terminal:resync-exec-gate-fast-lane in isolation.
func TestHandleCurrentPaneRequest_should_OnlyRouteFastLane_When_OnlyExecGateFastLaneFlagOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setOnlyResyncFlag(t, terminalResyncExecGateFastLaneFlagName)

	target := &fakePanePTY{captureContent: "fast-lane-content", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		ResyncId:   "should-not-be-echoed",
		TargetCols: int32Ptr(120),
		TargetRows: int32Ptr(40),
	}

	output, err := handleCurrentPaneRequest("test-session", target, req, currentResyncOptions())
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}

	// Changed: fast-lane capture/refresh used instead of the default methods.
	if target.capturePanePriorityCalled != 1 || target.capturePaneCalled != 0 {
		t.Errorf("expected fast-lane capture used, got priority=%d default=%d",
			target.capturePanePriorityCalled, target.capturePaneCalled)
	}
	if target.refreshTmuxPriorityCalled != 3 || target.refreshTmuxCalled != 0 {
		t.Errorf("expected fast-lane refresh used 3x, got priority=%d default=%d",
			target.refreshTmuxPriorityCalled, target.refreshTmuxCalled)
	}
	// Unchanged: the slow path still runs in full — fast lane only changes which method
	// variant is called, not whether the resize block runs at all.
	if target.resizePTYCalled != 1 {
		t.Errorf("expected ResizePTY still called once, got %d", target.resizePTYCalled)
	}
	// Unchanged: correlation-id flag is off, so resync_id must still not be echoed.
	if output.ResyncId != "" {
		t.Errorf("expected ResyncId to remain unechoed (correlation-id flag off), got %q", output.ResyncId)
	}
}

// TestHandleCurrentPaneRequest_should_ShareOneDeadlineAcrossAllFastLaneCalls_When_ResizeNeeded
// is the regression test for the second half of the 2026-08-25 incident (see
// tmux.ResyncFastLaneTimeout's doc comment): fixing each fast-lane call's individual
// timeout wasn't enough on its own — handleCurrentPaneRequest makes up to 5 fast-lane
// calls in sequence for one resize/resync (a dimension check, up to 3 refresh-client
// calls, a dimension verify, and a final capture), and each one independently minting a
// fresh 3s budget could still let the *total* elapsed time silently blow well past the
// client's stall watchdog. This asserts every fast-lane call the request actually
// triggers received the exact same ctx (by deadline) — a single shared, decreasing
// budget for the whole operation, not N independent ones.
func TestHandleCurrentPaneRequest_should_ShareOneDeadlineAcrossAllFastLaneCalls_When_ResizeNeeded(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setOnlyResyncFlag(t, terminalResyncExecGateFastLaneFlagName)

	target := &fakePanePTY{captureContent: "fast-lane-content", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		TargetCols: int32Ptr(120),
		TargetRows: int32Ptr(40),
	}

	_, err := handleCurrentPaneRequest("test-session", target, req, currentResyncOptions())
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}

	// Dimension check (1) + refresh x3 + dimension verify (1) + final capture (1) = 6.
	if len(target.fastLaneCtxsSeen) != 6 {
		t.Fatalf("expected 6 fast-lane calls in the resize-through-verify path, got %d", len(target.fastLaneCtxsSeen))
	}
	wantDeadline, ok := target.fastLaneCtxsSeen[0].Deadline()
	if !ok {
		t.Fatal("expected the first fast-lane call's ctx to carry a deadline")
	}
	for i, ctx := range target.fastLaneCtxsSeen {
		gotDeadline, ok := ctx.Deadline()
		if !ok {
			t.Errorf("call %d: ctx has no deadline", i)
			continue
		}
		if !gotDeadline.Equal(wantDeadline) {
			t.Errorf("call %d: deadline %v does not match the first call's deadline %v — each fast-lane call must share one budget, not mint its own",
				i, gotDeadline, wantDeadline)
		}
	}
}

// TestHandleCurrentPaneRequest_should_OnlyEchoResyncId_When_OnlyCorrelationIdFlagOn spot
// checks terminal:resync-correlation-id in isolation.
func TestHandleCurrentPaneRequest_should_OnlyEchoResyncId_When_OnlyCorrelationIdFlagOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setOnlyResyncFlag(t, terminalResyncCorrelationIDFlagName)

	target := &fakePanePTY{captureContent: "correlation-content", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		ResyncId:   "abc-123",
		TargetCols: int32Ptr(120),
		TargetRows: int32Ptr(40),
	}

	output, err := handleCurrentPaneRequest("test-session", target, req, currentResyncOptions())
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}

	// Changed: resync_id now echoed.
	if output.ResyncId != "abc-123" {
		t.Errorf("expected ResyncId echoed, got %q", output.ResyncId)
	}
	// Unchanged: default (non-fast-lane) capture/refresh still used.
	if target.capturePaneCalled != 1 || target.capturePanePriorityCalled != 0 {
		t.Errorf("expected default capture still used, got default=%d priority=%d",
			target.capturePaneCalled, target.capturePanePriorityCalled)
	}
	if target.refreshTmuxCalled != 3 || target.refreshTmuxPriorityCalled != 0 {
		t.Errorf("expected default refresh still used 3x, got default=%d priority=%d",
			target.refreshTmuxCalled, target.refreshTmuxPriorityCalled)
	}
	if target.resizePTYCalled != 1 {
		t.Errorf("expected ResizePTY still called once, got %d", target.resizePTYCalled)
	}
}

// TestHandleCurrentPaneRequest_should_OnlySkipSlowPath_When_OnlySkipStaleDimensionFlagOn spot
// checks terminal:resync-skip-stale-dimension-slowpath in isolation.
func TestHandleCurrentPaneRequest_should_OnlySkipSlowPath_When_OnlySkipStaleDimensionFlagOn(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	setOnlyResyncFlag(t, terminalResyncSkipStaleDimensionSlowpathFlagName)

	target := &fakePanePTY{captureContent: "skip-content", cols: 80, rows: 24}
	req := &sessionv1.CurrentPaneRequest{
		ResyncId:        "should-not-be-echoed",
		TargetCols:      int32Ptr(120),
		TargetRows:      int32Ptr(40),
		StaleDimensions: true,
	}

	output, err := handleCurrentPaneRequest("test-session", target, req, currentResyncOptions())
	if err != nil {
		t.Fatalf("handleCurrentPaneRequest returned error: %v", err)
	}

	// Changed: slow path skipped entirely.
	if target.resizePTYCalled != 0 || target.refreshTmuxCalled != 0 {
		t.Errorf("expected slow path skipped, got resize=%d refresh=%d",
			target.resizePTYCalled, target.refreshTmuxCalled)
	}
	// Unchanged: default (non-fast-lane) capture still used.
	if target.capturePaneCalled != 1 || target.capturePanePriorityCalled != 0 {
		t.Errorf("expected default capture still used, got default=%d priority=%d",
			target.capturePaneCalled, target.capturePanePriorityCalled)
	}
	// Unchanged: correlation-id flag off, so resync_id must still not be echoed.
	if output.ResyncId != "" {
		t.Errorf("expected ResyncId to remain unechoed, got %q", output.ResyncId)
	}
}

// TestStreamTerminal_should_PopulateConnectionCount_When_SessionIsPathHubOwned
// is Epic 4.2, Story 4.2.1's AC: a PathHubOwned session's connection reports
// hub.SubscriberCount() via sendConnectionCountUpdates' side-channel
// TerminalOutput messages, and the value tracks attach/detach in real time
// (validation.md REQ-14).
func TestStreamTerminal_should_PopulateConnectionCount_When_SessionIsPathHubOwned(t *testing.T) {
	serverStream, clientConn, cleanup := createTestWebSocketPair(t)
	defer cleanup()

	controller := &fakeSessionController{}
	hub := streamhub.NewStreamHub("conn-count-test", controller, streamhub.WithTeardownGrace(0))

	stop := make(chan struct{})
	defer close(stop)

	transport1 := NewWebSocketTransport(serverStream, "conn-count-test")
	id1 := hub.AttachSubscriber(transport1, streamhub.SubscriberCapability{CanResize: true})
	transport1.BindSubscriber(hub, id1)
	defer hub.DetachSubscriber(id1)

	// Attach before spawning the updater, matching streamViaHub's real call order
	// (AttachSubscriber at line ~1554, sendConnectionCountUpdates at ~1615) — the
	// updater's immediate send() races hub.SubscriberCount() against whatever the
	// caller does next, so spawning it before this attach (as this test used to)
	// let the immediate send observe 0 subscribers instead of 1, intermittently
	// failing the very next assertion.
	go sendConnectionCountUpdates(serverStream, hub, "conn-count-test", stop)

	readConnectionCount := func() int32 {
		t.Helper()
		require.NoError(t, clientConn.SetReadDeadline(time.Now().Add(3*time.Second)))
		env := readEnvelopeFromClient(t, clientConn)
		var msg sessionv1.TerminalData
		require.NoError(t, proto.Unmarshal(env.Data, &msg))
		output := msg.GetOutput()
		require.NotNil(t, output, "expected a TerminalData_Output message")
		require.NotNil(t, output.ConnectionCount, "expected ConnectionCount to be set")
		return output.GetConnectionCount()
	}

	// sendConnectionCountUpdates' immediate send() (before its first ticker
	// tick) delivers the current count without waiting a full poll interval.
	require.Equal(t, int32(1), readConnectionCount())

	transport2 := NewWebSocketTransport(serverStream, "conn-count-test")
	id2 := hub.AttachSubscriber(transport2, streamhub.SubscriberCapability{CanResize: true})
	transport2.BindSubscriber(hub, id2)
	defer hub.DetachSubscriber(id2)

	require.Equal(t, int32(2), readConnectionCount())
}

// TestStreamTerminal_should_OmitConnectionCount_When_SessionIsPathLegacyPerConnection
// is Story 4.2.1's second AC: a legacy-path connection never has
// sendConnectionCountUpdates running against it at all, so no
// connection_count field is ever fabricated from a signal (e.g.
// activeControlModeStreams) that isn't a real live subscriber count. This is
// enforced structurally — streamViaControlMode's send paths
// (marshalProtoEnvelope with TerminalOutput{Data: data}) never set
// ConnectionCount, verified here as a proto-level guarantee: a
// TerminalOutput built the same way streamViaControlMode builds one leaves
// ConnectionCount nil.
func TestStreamTerminal_should_OmitConnectionCount_When_SessionIsPathLegacyPerConnection(t *testing.T) {
	msg := &sessionv1.TerminalOutput{Data: []byte("legacy output")}
	require.Nil(t, msg.ConnectionCount, "legacy-path TerminalOutput must never carry a fabricated connection_count")
}
