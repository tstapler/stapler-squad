package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/protocol"

	"github.com/gorilla/websocket"
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

// --- Streaming mode validation ---

// TestNewHandlerAcceptsValidStreamingModes verifies that each documented
// transport mode is stored on the handler without being silently replaced.
func TestNewHandlerAcceptsValidStreamingModes(t *testing.T) {
	validModes := []string{"raw", "raw-compressed", "state", "hybrid"}
	for _, mode := range validModes {
		t.Run(mode, func(t *testing.T) {
			h := NewConnectRPCWebSocketHandler(nil, nil, nil, mode)
			if h.streamingMode != mode {
				t.Errorf("mode %q: expected handler.streamingMode=%q, got %q", mode, mode, h.streamingMode)
			}
		})
	}
}

// TestNewHandlerDefaultsInvalidModeToRawCompressed verifies that an unrecognised
// mode (including the empty string) is replaced with "raw-compressed", matching
// the documented default transport.
func TestNewHandlerDefaultsInvalidModeToRawCompressed(t *testing.T) {
	invalidModes := []string{"", "unknown", "ssp", "HYBRID", "Raw"}
	for _, mode := range invalidModes {
		t.Run(mode, func(t *testing.T) {
			h := NewConnectRPCWebSocketHandler(nil, nil, nil, mode)
			if h.streamingMode != "raw-compressed" {
				t.Errorf("invalid mode %q: expected default %q, got %q", mode, "raw-compressed", h.streamingMode)
			}
		})
	}
}

// TestSendEndStreamSuccess verifies that sendEndStreamSuccess writes a message
// with the EndStream flag set (regression: streamViaControlMode was missing this call).
func TestSendEndStreamSuccess(t *testing.T) {
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
	if elapsed > timeout+50*time.Millisecond {
		t.Errorf("waitForQuiescence took too long after timeout (%v)", elapsed)
	}
}

// TestWaitForQuiescenceReturnsOnChannelClose verifies that closing the updates
// channel causes waitForQuiescence to return promptly.
func TestWaitForQuiescenceReturnsOnChannelClose(t *testing.T) {
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

// --- getOrRefreshSnapshot / markSnapshotDirty ---

// TestGetOrRefreshSnapshotCallsCaptureFnOnMiss verifies that on a cache miss
// captureFn is called and the result is cached.
func TestGetOrRefreshSnapshotCallsCaptureFnOnMiss(t *testing.T) {
	h := NewConnectRPCWebSocketHandler(nil, nil, nil, "raw")
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
	h := NewConnectRPCWebSocketHandler(nil, nil, nil, "raw")
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
	h := NewConnectRPCWebSocketHandler(nil, nil, nil, "raw")
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
	h := NewConnectRPCWebSocketHandler(nil, nil, nil, "raw")

	// Should not panic
	h.markSnapshotDirty("nonexistent-session")

	// Should not create an entry
	if _, ok := h.snapshotCache["nonexistent-session"]; ok {
		t.Error("markSnapshotDirty created a cache entry for an unknown session")
	}
}

// TestGetOrRefreshSnapshotPropagatesCaptureFnError verifies that captureFn errors
// are returned to the caller and nothing is cached.
func TestGetOrRefreshSnapshotPropagatesCaptureFnError(t *testing.T) {
	h := NewConnectRPCWebSocketHandler(nil, nil, nil, "raw")
	captureErr := fmt.Errorf("tmux: session not found")
	captureFn := func() (string, error) { return "", captureErr }

	_, err := h.getOrRefreshSnapshot("sess1", captureFn)
	if err == nil {
		t.Fatal("expected error from captureFn, got nil")
	}
	if !strings.Contains(err.Error(), captureErr.Error()) {
		t.Errorf("error %q does not contain %q", err.Error(), captureErr.Error())
	}
	if _, ok := h.snapshotCache["sess1"]; ok {
		t.Error("cache entry created despite captureFn error")
	}
}

// TestSnapshotCacheConcurrentAccess verifies that concurrent reads and
// dirty-marking do not cause data races. Run with -race to validate.
func TestSnapshotCacheConcurrentAccess(t *testing.T) {
	h := NewConnectRPCWebSocketHandler(nil, nil, nil, "raw")

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
	r := newRequestWithOrigin("")
	if !isAllowedOrigin(r) {
		t.Error("request with no Origin header should be allowed")
	}
}

// TestIsAllowedOriginLocalhostVariants verifies that all localhost forms are accepted.
func TestIsAllowedOriginLocalhostVariants(t *testing.T) {
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
	cases := []string{
		"https://myapp.example.com",
		"https://company.internal:8443",
		"https://staging.myapp.io",
	}
	for _, origin := range cases {
		t.Run(origin, func(t *testing.T) {
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
	cases := []string{
		"http://attacker.example.com",
		"http://evil.com",
		"http://192.168.1.100:3000",
	}
	for _, origin := range cases {
		t.Run(origin, func(t *testing.T) {
			r := newRequestWithOrigin(origin)
			if isAllowedOrigin(r) {
				t.Errorf("HTTP non-localhost origin %q should be blocked", origin)
			}
		})
	}
}

// TestIsAllowedOriginMalformed verifies that a malformed Origin header is rejected.
func TestIsAllowedOriginMalformed(t *testing.T) {
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
			got := prepareSnapshotContent(tc.input)
			if got != tc.want {
				t.Errorf("prepareSnapshotContent(%q) =\n  %q\nwant\n  %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestPrepareSnapshotContentStripsCursorPositioning verifies that sanitization
// (stripping cursor-positioning codes) still runs before newline normalisation.
func TestPrepareSnapshotContentStripsCursorPositioning(t *testing.T) {
	// A realistic capture-pane fragment: cursor home + color + text + newline
	input := "\x1b[H\x1b[1;32mline1\x1b[0m\nline2\n"
	got := prepareSnapshotContent(input)

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
	input := "\x1b[1;32mhello\x1b[0m\nworld\n"
	got := prepareSnapshotContent(input)

	for _, sgr := range []string{"\x1b[1;32m", "\x1b[0m"} {
		if !strings.Contains(got, sgr) {
			t.Errorf("prepareSnapshotContent: SGR sequence %q was lost; got %q", sgr, got)
		}
	}
}

// TestAnsiSnapshotPrefixContainsRequiredSequences verifies the prefix used before
// every snapshot contains DECSTR, ED2, and CUP in that order.
// Regression for Bug A: a prefix without ED2 (screen clear) caused double display.
func TestAnsiSnapshotPrefixContainsRequiredSequences(t *testing.T) {
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
