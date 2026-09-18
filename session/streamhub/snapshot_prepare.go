package streamhub

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// rePositionCodes, the ansi* constants, and prepareSnapshotContent mirror
// server/services/connectrpc_websocket.go's identically-named helpers
// exactly. Duplicated rather than imported: server/services imports
// session/streamhub, so the reverse import would cycle (same tradeoff as
// instance_tmux.go's terminalResyncExecGateFastLaneFlagName) — keep both
// copies in sync if either changes.
var rePositionCodes = regexp.MustCompile(
	`\x1b\[\d*;?\d*[Hf]` + // Absolute cursor: ESC[H, ESC[n;mH, ESC[n;mf
		`|\x1b\[\d*J` + // Screen clear: ESC[J, ESC[1J, ESC[2J, ESC[3J
		`|\x1b\[\?\d+[hl]` + // Private mode: ESC[?1049h (alt screen), ESC[?25l, etc.
		`|\x1b[78]` + // DEC save/restore cursor: ESC7, ESC8
		`|\x1b\[[su]`, // CSI save/restore cursor: ESC[s, ESC[u
)

const (
	ansiDECSTR         = "\x1b[!p"
	ansiEraseScreen    = "\x1b[2J"
	ansiCursorHome     = "\x1b[H"
	ansiSnapshotPrefix = ansiDECSTR + ansiEraseScreen + ansiCursorHome
)

// RawPaneContent is tmux `capture-pane -p -e` output captured WITHOUT -J:
// one output line per visual row of the pane, exactly matching what a
// terminal emulator displayed, with cursor-positioning/SGR escape codes
// intact. This is the only form safe to feed into prepareSnapshotContent and
// replay into a live terminal emulator (xterm.js) — see
// SessionController.CapturePaneContentRaw's doc comment.
//
// It is deliberately NOT the type of tmux's -J ("joined") capture variant,
// which merges soft-wrapped continuation rows back into their original
// logical line for plain-text uses (search, logging, debug dumps) — safe to
// display as text, but replaying -J output into a terminal emulator
// destroys the visual grid structure and drops cursor codes the -J join
// strips as a side effect. Keeping that variant as an untyped string (rather
// than a matching JoinedPaneContent) is deliberate: prepareSnapshotContent
// requiring RawPaneContent is what makes passing the wrong variant a compile
// error, which is the actual safety property this type exists for — every
// -J consumer in this codebase treats its result as display/search text,
// never as renderer input, so there is no equivalent mistake to guard
// against on that side.
type RawPaneContent string

// prepareSnapshotContent normalizes raw tmux `capture-pane -e -t` output
// (session.Instance.CapturePaneContentRaw) for replay as a full-screen
// xterm.js snapshot. Two fixes, both required:
//
//  1. Strips cursor-positioning/screen-control escapes that would conflict
//     with the leading ansiSnapshotPrefix erase+home this package prefixes
//     every snapshot with.
//  2. Converts every bare LF row separator to CRLF. capture-pane emits bare
//     LF; xterm.js only returns the cursor to column 0 on CR, not LF alone.
//     Without this, each row after the first starts one column further
//     right than tmux intended, staircasing the new snapshot diagonally
//     across whatever was already on screen — the reflow-scrambling bug
//     this function exists to fix (streamhub's resize/catch-up snapshot
//     pipeline sent raw, unprepared capture-pane output directly to
//     subscribers, unlike the legacy control-mode path in
//     connectrpc_websocket.go, which always ran it through this same
//     transform before broadcasting).
func prepareSnapshotContent(content RawPaneContent) string {
	sanitized := rePositionCodes.ReplaceAllString(string(content), "")
	sanitized = strings.ReplaceAll(sanitized, "\r\n", "\n")
	return strings.ReplaceAll(sanitized, "\n", "\r\n")
}

// cursorPositioner mirrors connectrpc_websocket.go's identically-named
// interface — see this file's package doc comment on why it's duplicated
// rather than imported. SessionController satisfies it structurally.
type cursorPositioner interface {
	GetPaneCursorPosition() (x, y int, err error)
}

// withCursorSyncTimeout mirrors connectrpc_websocket.go's identically-named
// constant — see its doc comment for why GetPaneCursorPosition needs an
// external bound. It matters even more here: applyNegotiatedSize
// (hub.go) calls this while h.resizing is true, which suppresses live
// output for every attached subscriber, not just one connection's resync —
// a slow cursor lookup here stalls everyone watching the session, not just
// the one who triggered the resize.
const withCursorSyncTimeout = 300 * time.Millisecond

// withCursorSync appends a CUP escape so the xterm.js cursor lands at the
// same position as the tmux pane cursor after the snapshot renders. Without
// it, the client cursor is left wherever the last snapshot byte placed it,
// while tmux's cursor sits at the process's actual working position —
// causing an interactive TUI's relative cursor-up redraws to rewind to the
// wrong row and stack each frame below the last instead of overwriting it.
// A failed position lookup, or one that doesn't return within
// withCursorSyncTimeout, leaves content unchanged rather than blocking the
// whole snapshot over a best-effort cosmetic fix.
func withCursorSync(content string, target cursorPositioner) string {
	type cursorResult struct {
		x, y int
		err  error
	}
	resultCh := make(chan cursorResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		x, y, err := target.GetPaneCursorPosition()
		resultCh <- cursorResult{x, y, err}
	}()
	select {
	case res := <-resultCh:
		// The goroutine's defer runs immediately after the send above, but
		// waiting on done here (already closed or about to be by the time we
		// reach it) guarantees it has fully exited before this function
		// returns — not just handed off its result — so a test's
		// goleak.VerifyNone() immediately afterward never catches it
		// mid-teardown. Only the timeout branch below intentionally leaves
		// the goroutine to finish (and get GC'd) on its own.
		<-done
		if res.err != nil {
			return content
		}
		// CUP is 1-based; tmux cursor coords are 0-based.
		return content + fmt.Sprintf("\x1b[%d;%dH", res.y+1, res.x+1)
	case <-time.After(withCursorSyncTimeout):
		log.Warn("streamhub: GetPaneCursorPosition exceeded timeout, skipping cursor sync", "timeout", withCursorSyncTimeout)
		return content
	}
}
