package streamhub

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestPrepareSnapshotContent is the direct unit test for the transform at the
// heart of the 2026-08-25 reflow bug: streamhub broadcast raw capture-pane
// output straight to subscribers with no preparation, and bare LF row
// separators (not CRLF) staircased every resize snapshot diagonally across
// whatever was already on screen. Level 1 (the RawPaneContent type) already
// makes it a compile error to feed the wrong capture variant in here — this
// test instead verifies the transform ITSELF is correct on the content that
// does get in, which the type system has no way to check.
func TestPrepareSnapshotContent(t *testing.T) {
	cases := []struct {
		name  string
		input RawPaneContent
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

// TestPrepareSnapshotContentStripsCursorPositioning verifies sanitization
// (stripping cursor-positioning/screen-control codes) runs before newline
// normalization — those codes would otherwise conflict with the leading
// ansiSnapshotPrefix erase+home every snapshot gets.
func TestPrepareSnapshotContentStripsCursorPositioning(t *testing.T) {
	input := RawPaneContent("\x1b[H\x1b[1;32mline1\x1b[0m\nline2\n")
	got := prepareSnapshotContent(input)

	if strings.Contains(got, "\x1b[H") {
		t.Errorf("prepareSnapshotContent: cursor home ESC[H not stripped; got %q", got)
	}
	if !strings.Contains(got, "\r\n") {
		t.Errorf("prepareSnapshotContent: expected \\r\\n line endings; got %q", got)
	}
}

// TestPrepareSnapshotContentPreservesSGR verifies SGR color sequences
// survive — they're safe to replay and must not be lost.
func TestPrepareSnapshotContentPreservesSGR(t *testing.T) {
	input := RawPaneContent("\x1b[1;32mhello\x1b[0m\nworld\n")
	got := prepareSnapshotContent(input)

	for _, sgr := range []string{"\x1b[1;32m", "\x1b[0m"} {
		if !strings.Contains(got, sgr) {
			t.Errorf("prepareSnapshotContent: SGR sequence %q was lost; got %q", sgr, got)
		}
	}
}

// fakeCursorPositioner is a minimal cursorPositioner test double.
type fakeCursorPositioner struct {
	x, y int
	err  error
}

func (f fakeCursorPositioner) GetPaneCursorPosition() (x, y int, err error) {
	return f.x, f.y, f.err
}

// TestWithCursorSync_should_AppendCUPEscape_When_PositionLookupSucceeds
// verifies the CUP escape appended matches the 1-based conversion of tmux's
// 0-based cursor coordinates — get this backwards and every post-snapshot
// cursor lands one row/column off, which is exactly the kind of off-by-one
// a compile-time type can't catch.
func TestWithCursorSync_should_AppendCUPEscape_When_PositionLookupSucceeds(t *testing.T) {
	got := withCursorSync("content", fakeCursorPositioner{x: 4, y: 9})
	want := "content\x1b[10;5H"
	if got != want {
		t.Errorf("withCursorSync() = %q, want %q", got, want)
	}
}

// TestWithCursorSync_should_LeaveContentUnchanged_When_PositionLookupFails
// covers the graceful-degradation path: a failed cursor lookup must not
// corrupt or drop the snapshot, just skip the cosmetic cursor-sync suffix.
func TestWithCursorSync_should_LeaveContentUnchanged_When_PositionLookupFails(t *testing.T) {
	wantErr := errors.New("cursor lookup failed")
	got := withCursorSync("content", fakeCursorPositioner{err: wantErr})
	if got != "content" {
		t.Errorf("withCursorSync() = %q, want content left unchanged on lookup error", got)
	}
}

// slowCursorPositioner blocks for longer than withCursorSyncTimeout before
// returning, simulating a degraded GetPaneCursorPosition (its own control-mode
// path has no timeout shorter than 3s, and its subprocess fallback has none
// at all beyond a 5s exec-gate wait — see withCursorSyncTimeout's doc comment).
// calledCh, if non-nil, is closed once GetPaneCursorPosition actually
// returns — lets a test wait for withCursorSync's deliberately-abandoned
// goroutine to finish before the test itself returns, so it can never bleed
// into a later test's goleak.VerifyNone() check.
type slowCursorPositioner struct {
	delay    time.Duration
	calledCh chan struct{}
}

func (s slowCursorPositioner) GetPaneCursorPosition() (x, y int, err error) {
	time.Sleep(s.delay)
	if s.calledCh != nil {
		close(s.calledCh)
	}
	return 1, 1, nil
}

// TestWithCursorSync_should_ReturnWithinTimeout_When_PositionLookupIsSlow is
// the regression test for the 2026-08-25 latency fix: applyNegotiatedSize
// (hub.go) calls withCursorSync while h.resizing suppresses live output for
// every subscriber, and connectrpc_websocket.go's handleCurrentPaneRequest
// calls its own copy on the client-triggered resync path, which the
// frontend force-disconnects if it doesn't respond within 4s
// (useVisibilityResync.ts's stall watchdog) — a slow, unbounded cursor
// lookup could single-handedly blow both budgets. Content must come back
// unchanged (matching the lookup-error path) rather than carrying a cursor
// escape for a position that arrived too late to be worth appending.
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
	// withCursorSync's timeout branch deliberately doesn't wait for it (that's
	// the behavior under test), but leaving it running past this test's own
	// return risks it still being alive when a later test's
	// goleak.VerifyNone() checks.
	<-calledCh
}
