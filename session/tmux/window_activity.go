package tmux

import (
	"bytes"
	"strconv"
	"time"
)

// CapturePaneCacheTTL is the TTL for the CapturePaneContent result cache.
// Avoids spawning a tmux subprocess on every poll tick.
const CapturePaneCacheTTL = time.Second

// BatchWindowActivity returns the newest window-activity timestamp for each session
// by running a single `tmux list-panes -a` call. The tmux format #{window_activity}
// is a Unix timestamp updated whenever the window produces output (#{pane_last_activity}
// is not a tmux format variable and expands to empty). Use it to detect whether
// a session has produced new output since the last capture, without spawning capture-pane.
// Empty or non-numeric values are treated as unknown (session omitted).
// Returns nil when tmux is unavailable.
func BatchWindowActivity(socket string) map[string]time.Time {
	output, ok := ListAllPanes(socket, "#{session_name} #{window_activity}")
	if !ok {
		return nil
	}
	// Parse bytes directly: no string(output) copy, no strings.Split allocation.
	result := make(map[string]time.Time)
	rem := output
	for len(rem) > 0 {
		var line []byte
		if idx := bytes.IndexByte(rem, '\n'); idx >= 0 {
			line = bytes.TrimSpace(rem[:idx])
			rem = rem[idx+1:]
		} else {
			line = bytes.TrimSpace(rem)
			rem = rem[:0]
		}
		spaceIdx := bytes.IndexByte(line, ' ')
		if spaceIdx <= 0 || spaceIdx >= len(line)-1 {
			continue
		}
		sessionName := string(line[:spaceIdx])
		ts, err := strconv.ParseInt(string(line[spaceIdx+1:]), 10, 64)
		if err != nil {
			continue
		}
		// Newest window wins: a multi-window session's active window may not be listed first.
		if t := time.Unix(ts, 0); t.After(result[sessionName]) {
			result[sessionName] = t
		}
	}
	return result
}
