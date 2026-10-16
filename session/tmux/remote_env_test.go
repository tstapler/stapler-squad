package tmux

import (
	"reflect"
	"testing"
)

// TestWrapRemoteCommand verifies wrapRemoteCommand against known inputs:
// the returned program is always "env", and the returned argv unsets $TMUX,
// forces $TERM=xterm-256color, then reproduces the original name/args
// unmodified and in order.
func TestWrapRemoteCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmdName  string
		cmdArgs  []string
		wantName string
		wantArgs []string
	}{
		{
			name:     "tmux has-session",
			cmdName:  "tmux",
			cmdArgs:  []string{"has-session", "-t", "staplersquad_foo"},
			wantName: "env",
			wantArgs: []string{"-u", "TMUX", "TERM=xterm-256color", "tmux", "has-session", "-t", "staplersquad_foo"},
		},
		{
			name:     "tmux new-session with socket flag",
			cmdName:  "tmux",
			cmdArgs:  []string{"-L", "isolated", "new-session", "-A", "-d", "-s", "staplersquad_bar", "-c", "/work/dir", "claude"},
			wantName: "env",
			wantArgs: []string{"-u", "TMUX", "TERM=xterm-256color", "tmux", "-L", "isolated", "new-session", "-A", "-d", "-s", "staplersquad_bar", "-c", "/work/dir", "claude"},
		},
		{
			name:     "no args",
			cmdName:  "tmux",
			cmdArgs:  nil,
			wantName: "env",
			wantArgs: []string{"-u", "TMUX", "TERM=xterm-256color", "tmux"},
		},
		{
			name:     "non-tmux command (e.g. kill -WINCH)",
			cmdName:  "kill",
			cmdArgs:  []string{"-WINCH", "12345"},
			wantName: "env",
			wantArgs: []string{"-u", "TMUX", "TERM=xterm-256color", "kill", "-WINCH", "12345"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotArgs := wrapRemoteCommand(tt.cmdName, tt.cmdArgs)
			if gotName != tt.wantName {
				t.Errorf("wrapRemoteCommand() name = %q, want %q", gotName, tt.wantName)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("wrapRemoteCommand() args = %#v, want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}

// TestWrapRemoteCommand_DoesNotMutateInputSlice guards against a subtle
// aliasing bug: wrapRemoteCommand must build a new backing array rather than
// appending onto (and potentially reallocating into, or corrupting) the
// caller's args slice, since callers may reuse that slice afterward (e.g.
// Socket.Args's own result).
func TestWrapRemoteCommand_DoesNotMutateInputSlice(t *testing.T) {
	original := []string{"has-session", "-t", "sess"}
	inputCopy := append([]string(nil), original...)

	_, gotArgs := wrapRemoteCommand("tmux", original)

	if !reflect.DeepEqual(original, inputCopy) {
		t.Fatalf("wrapRemoteCommand mutated its input slice: got %#v, want %#v", original, inputCopy)
	}
	// Mutating the returned slice must not affect the original either.
	gotArgs[0] = "MUTATED"
	if !reflect.DeepEqual(original, inputCopy) {
		t.Fatalf("mutating wrapRemoteCommand's return value affected the input slice: %#v", original)
	}
}
