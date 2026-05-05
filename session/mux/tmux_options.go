package mux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// tmux user option keys for claude-mux session metadata.
// User options are @-prefixed and stored natively on the tmux session,
// surviving server restarts and enabling socket-free discovery via
// a single `tmux list-sessions` call.
const (
	tmuxOptSocketPath = "@cs_socket_path"
	tmuxOptCwd        = "@cs_cwd"
	tmuxOptCommand    = "@cs_command"
	tmuxOptPID        = "@cs_pid"
	tmuxOptStartTime  = "@cs_start_time"
)

// WriteSessionUserOptions stores session metadata as tmux user options on the
// named session. This enables the claude-squad server to discover external
// sessions without probing Unix sockets, and the data survives server restarts.
//
// Callers should treat errors as non-fatal — log and continue. The socket-based
// discovery path remains a fallback.
func WriteSessionUserOptions(sessionName, socketPath, cwd, command string, pid int, startTime int64) error {
	opts := []struct{ key, value string }{
		{tmuxOptSocketPath, socketPath},
		{tmuxOptCwd, cwd},
		{tmuxOptCommand, command},
		{tmuxOptPID, strconv.Itoa(pid)},
		{tmuxOptStartTime, strconv.FormatInt(startTime, 10)},
	}
	for _, opt := range opts {
		setCtx, setCancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := safeexec.CommandContext(setCtx, "tmux", "set-option", "-t", sessionName, opt.key, opt.value)
		out, runErr := cmd.CombinedOutput()
		setCancel()
		if runErr != nil {
			return fmt.Errorf("set %s on %s: %w (output: %s)",
				opt.key, sessionName, runErr, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// ScanByUserOptions discovers active claude-mux sessions via a single
// `tmux list-sessions` call, reading user options written by WriteSessionUserOptions.
//
// This is faster than probing N sockets because:
//   - One subprocess call returns all sessions
//   - No network I/O (socket connect/read) per session
//   - Works immediately after a server restart before sockets are re-probed
//
// Returns an empty slice (not an error) when tmux is not running or has no sessions.
// Sessions without @cs_socket_path set are silently skipped (not claude-mux sessions).
func ScanByUserOptions() ([]*DiscoveredSession, error) {
	format := strings.Join([]string{
		"#{session_name}",
		"#{" + tmuxOptSocketPath + "}",
		"#{" + tmuxOptCwd + "}",
		"#{" + tmuxOptCommand + "}",
		"#{" + tmuxOptPID + "}",
		"#{" + tmuxOptStartTime + "}",
	}, "\t")

	scanCtx, scanCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scanCancel()
	cmd := safeexec.CommandContext(scanCtx, "tmux", "list-sessions", "-F", format)
	out, err := cmd.Output()
	if err != nil {
		// tmux exits non-zero when the server is not running or there are no
		// sessions. Both are normal states — return empty, not an error.
		return nil, nil
	}

	var sessions []*DiscoveredSession
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 6)
		if len(fields) < 6 {
			continue
		}

		socketPath := fields[1]
		if socketPath == "" {
			// @cs_socket_path not set — not a claude-mux session.
			continue
		}

		pid, _ := strconv.Atoi(fields[4])
		startTime, _ := strconv.ParseInt(fields[5], 10, 64)

		sessions = append(sessions, &DiscoveredSession{
			SocketPath: socketPath,
			Metadata: &SessionMetadata{
				TmuxSession: fields[0],
				SocketPath:  socketPath,
				Cwd:         fields[2],
				Command:     fields[3],
				PID:         pid,
				StartTime:   startTime,
			},
			LastSeen: time.Now(),
		})
	}
	return sessions, nil
}
