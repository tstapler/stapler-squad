package mux

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// hasTmux reports whether tmux is available in PATH.
// Tests that require a live tmux session are skipped when it's absent.
func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// TestScanByUserOptions_NoSessions verifies that ScanByUserOptions returns
// an empty slice (not an error) when tmux is unavailable or has no sessions.
func TestScanByUserOptions_NoSessions(t *testing.T) {
	sessions, err := ScanByUserOptions()
	if err != nil {
		t.Fatalf("ScanByUserOptions: unexpected error: %v", err)
	}
	// Result may be empty (no sessions) or contain sessions from other tests.
	// Either way, no panic and no error is the key assertion.
	_ = sessions
}

// TestScanByUserOptions_ParsesFields verifies the tab-separated parsing
// logic using the same field splitting used in ScanByUserOptions.
func TestScanByUserOptions_ParsesFields(t *testing.T) {
	// Simulate the output of `tmux list-sessions -F ...` for two sessions:
	// one claude-mux session and one without options.
	lines := []string{
		"claudesquad_ext_myproject_claude_1234\t/tmp/claude-mux-1234.sock\t/home/user/myproject\tclaude\t1234\t1700000000",
		"some_other_session\t\t\t\t\t", // no @cs_socket_path — should be filtered
	}
	output := strings.Join(lines, "\n")

	// Replicate ScanByUserOptions parsing inline so we can test it
	// without invoking tmux.
	var sessions []*DiscoveredSession
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 6)
		if len(fields) < 6 {
			continue
		}
		socketPath := fields[1]
		if socketPath == "" {
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

	if len(sessions) != 1 {
		t.Fatalf("expected 1 parsed session (non-mux line filtered), got %d", len(sessions))
	}

	s := sessions[0]
	if s.Metadata.TmuxSession != "claudesquad_ext_myproject_claude_1234" {
		t.Errorf("TmuxSession: got %q", s.Metadata.TmuxSession)
	}
	if s.SocketPath != "/tmp/claude-mux-1234.sock" {
		t.Errorf("SocketPath: got %q", s.SocketPath)
	}
	if s.Metadata.Cwd != "/home/user/myproject" {
		t.Errorf("Cwd: got %q", s.Metadata.Cwd)
	}
	if s.Metadata.Command != "claude" {
		t.Errorf("Command: got %q", s.Metadata.Command)
	}
	if s.Metadata.PID != 1234 {
		t.Errorf("PID: got %d", s.Metadata.PID)
	}
	if s.Metadata.StartTime != 1700000000 {
		t.Errorf("StartTime: got %d", s.Metadata.StartTime)
	}
}

// TestWriteReadUserOptions verifies that WriteSessionUserOptions writes all
// five option keys and that they are readable via `tmux show-options`.
// Skipped when tmux is unavailable.
func TestWriteReadUserOptions(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	sessionName := "cs-test-useropts"

	// Create a throw-away tmux session.
	create := safeexec.CommandContext(context.Background(), "tmux", "new-session", "-d", "-s", sessionName, "sleep", "60")
	if err := create.Run(); err != nil {
		t.Fatalf("create tmux session: %v", err)
	}
	t.Cleanup(func() {
		killCtx, killCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer killCancel()
		_ = safeexec.CommandContext(killCtx, "tmux", "kill-session", "-t", sessionName).Run()
	})

	socketPath := "/tmp/claude-mux-test-99999.sock"
	cwd := "/tmp/test-cwd"
	command := "claude"
	pid := 99999
	startTime := int64(1700000000)

	if err := WriteSessionUserOptions(sessionName, socketPath, cwd, command, pid, startTime); err != nil {
		t.Fatalf("WriteSessionUserOptions: %v", err)
	}

	// Verify each option via tmux show-options.
	cases := []struct {
		key      string
		expected string
	}{
		{tmuxOptSocketPath, socketPath},
		{tmuxOptCwd, cwd},
		{tmuxOptCommand, command},
		{tmuxOptPID, strconv.Itoa(pid)},
		{tmuxOptStartTime, strconv.FormatInt(startTime, 10)},
	}
	for _, tc := range cases {
		showCtx, showCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer showCancel()
		out, err := safeexec.CommandContext(showCtx, "tmux", "show-options", "-t", sessionName, tc.key).Output()
		if err != nil {
			t.Errorf("show-options %s: %v", tc.key, err)
			continue
		}
		// Output format: "@key value\n"
		got := strings.TrimSpace(string(out))
		expected := tc.key + " " + tc.expected
		if got != expected {
			t.Errorf("option %s: got %q, want %q", tc.key, got, expected)
		}
	}
}

// TestScanFromUserOptions_RegistersSession verifies that ScanFromUserOptions
// correctly registers a session returned by ScanByUserOptions into d.sessions
// and fires the new-session callback. Skipped when tmux is unavailable.
func TestScanFromUserOptions_RegistersSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	sessionName := "cs-test-scanfromopts"
	create := safeexec.CommandContext(context.Background(), "tmux", "new-session", "-d", "-s", sessionName, "sleep", "60")
	if err := create.Run(); err != nil {
		t.Fatalf("create tmux session: %v", err)
	}
	t.Cleanup(func() {
		killCtx2, killCancel2 := context.WithTimeout(context.Background(), 10*time.Second)
		defer killCancel2()
		_ = safeexec.CommandContext(killCtx2, "tmux", "kill-session", "-t", sessionName).Run()
	})

	socketPath := "/tmp/claude-mux-test-88888.sock"
	if err := WriteSessionUserOptions(sessionName, socketPath, "/tmp", "claude", 88888, time.Now().Unix()); err != nil {
		t.Fatalf("WriteSessionUserOptions: %v", err)
	}

	d := NewDiscovery()

	var callbackSessions []*DiscoveredSession
	d.OnSessionChange(func(s *DiscoveredSession, isNew bool) {
		if isNew {
			callbackSessions = append(callbackSessions, s)
		}
	})

	discovered, err := d.ScanFromUserOptions()
	if err != nil {
		t.Fatalf("ScanFromUserOptions: %v", err)
	}

	// Find our test session in the results.
	var found *DiscoveredSession
	for _, s := range discovered {
		if s.Metadata != nil && s.Metadata.TmuxSession == sessionName {
			found = s
			break
		}
	}
	if found == nil {
		t.Fatalf("test session %q not found in ScanFromUserOptions results", sessionName)
	}

	// Verify callback was fired.
	var callbackFired bool
	for _, s := range callbackSessions {
		if s.Metadata != nil && s.Metadata.TmuxSession == sessionName {
			callbackFired = true
			break
		}
	}
	if !callbackFired {
		t.Error("new-session callback was not fired for test session")
	}

	// Verify session is registered in d.sessions.
	registered := d.GetSessions()
	var inSessions bool
	for _, s := range registered {
		if s.Metadata != nil && s.Metadata.TmuxSession == sessionName {
			inSessions = true
			break
		}
	}
	if !inSessions {
		t.Error("test session not registered in d.sessions after ScanFromUserOptions")
	}
}
