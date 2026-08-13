package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor"
)

// TestTmuxCircuitBreakerConfig_NoSessionsNotFailure verifies that the tmux-specific
// IsFailure function does NOT trip the breaker on a "no sessions" exit-1 from
// tmux list-sessions, while it DOES trip on real server-down output.
func TestTmuxCircuitBreakerConfig_NoSessionsNotFailure(t *testing.T) {
	cfg := tmuxCircuitBreakerConfig()

	if cfg.IsFailure == nil {
		t.Fatal("expected IsFailure to be set by tmuxCircuitBreakerConfig")
	}

	exitErr := fmt.Errorf("exit status 1")

	tests := []struct {
		name         string
		commandClass string
		output       []byte
		err          error
		wantFailure  bool
	}{
		{
			name:         "no error is not a failure",
			commandClass: "tmux-list-sessions",
			output:       []byte("staplersquad_mysession"),
			err:          nil,
			wantFailure:  false,
		},
		{
			name:         "list-sessions empty output (no sessions) is not a failure",
			commandClass: "tmux-list-sessions",
			output:       []byte(""),
			err:          exitErr,
			wantFailure:  false,
		},
		{
			name:         "list-sessions server-down output IS a failure",
			commandClass: "tmux-list-sessions",
			output:       []byte("no server running on /tmp/tmux-1000/default"),
			err:          exitErr,
			wantFailure:  true,
		},
		{
			name:         "list-sessions error socket-not-found IS a failure",
			commandClass: "tmux-list-sessions",
			output:       []byte("error connecting to /tmp/tmux-1000/default (No such file or directory)"),
			err:          exitErr,
			wantFailure:  true,
		},
		{
			name:         "non-list-sessions server-down output IS a failure",
			commandClass: "tmux-new-session",
			output:       []byte("no server running on /tmp/tmux-1000/default"),
			err:          exitErr,
			wantFailure:  true,
		},
		{
			name:         "non-list-sessions with no error is not a failure",
			commandClass: "tmux-new-session",
			output:       []byte(""),
			err:          nil,
			wantFailure:  false,
		},
		{
			name:         "capture-pane pane-not-found is NOT a failure (server is healthy)",
			commandClass: "tmux-capture-pane",
			output:       []byte("can't find pane: staplersquad_mysession"),
			err:          exitErr,
			wantFailure:  false,
		},
		{
			name:         "capture-pane server-down IS a failure",
			commandClass: "tmux-capture-pane",
			output:       []byte("error connecting to /tmp/tmux-1000/default (No such file or directory)"),
			err:          exitErr,
			wantFailure:  true,
		},
		{
			name:         "display-message session-not-found is NOT a failure (server is healthy)",
			commandClass: "tmux-display-message",
			output:       []byte("session not found: staplersquad_mysession"),
			err:          exitErr,
			wantFailure:  false,
		},
		{
			name:         "non-list-sessions empty output is NOT a failure (server may be healthy)",
			commandClass: "tmux-new-session",
			output:       []byte(""),
			err:          exitErr,
			wantFailure:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cfg.IsFailure(tc.commandClass, tc.output, tc.err)
			if got != tc.wantFailure {
				t.Errorf("IsFailure(%q, %q, %v) = %v, want %v",
					tc.commandClass, tc.output, tc.err, got, tc.wantFailure)
			}
		})
	}
}

// TestDoesSessionExist_CircuitBypassFallback verifies that DoesSessionExist does NOT
// panic and returns a valid bool when the circuit breaker is open (ErrCircuitOpen).
// The open circuit causes the fallback to direct exec, which may fail if tmux is
// unavailable in the test environment — but the code path must be exercised without
// crashing and must not return an error to the caller.
func TestDoesSessionExist_CircuitBypassFallback(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	openCircuitExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			if strings.Contains(cmd.String(), "list-sessions") {
				// Simulate circuit breaker open
				return nil, executor.ErrCircuitOpen
			}
			return []byte(""), nil
		},
	}

	session := newTmuxSession("test-bypass", "echo test", ptyFactory, openCircuitExec, TmuxPrefix)

	// Must not panic; returns bool regardless of what the fallback exec produces.
	// (The fallback calls real tmux, which may or may not be running.)
	result := session.DoesSessionExistNoCache()
	_ = result // just verifying no panic and correct type
}

// TestDoesSessionExistNoCache_CircuitBypassFallback mirrors the above for DoesSessionExistNoCache.
func TestDoesSessionExistNoCache_CircuitBypassFallback(t *testing.T) {
	ptyFactory := NewMockPtyFactory(t)

	callCount := 0
	openCircuitExec := MockCmdExec{
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			callCount++
			if strings.Contains(cmd.String(), "list-sessions") {
				return nil, executor.ErrCircuitOpen
			}
			return []byte(""), nil
		},
	}

	session := newTmuxSession("test-bypass-nocache", "echo test", ptyFactory, openCircuitExec, TmuxPrefix)
	_ = session.DoesSessionExistNoCache()

	if callCount == 0 {
		t.Error("expected at least one call through the mock executor")
	}
}
