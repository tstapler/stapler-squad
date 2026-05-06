// Package a contains test fixtures for the norawexec analyzer.
package a

import (
	"context"
	"os/exec"
	"time"
)

// BAD1: exec.Command with no context — blocks forever, no WaitDelay.
func bad1() {
	_ = exec.Command("tmux", "list-sessions") // want `direct call to os/exec.Command`
}

// BAD2: exec.CommandContext with no WaitDelay — Wait() can block on grandchild pipe.
func bad2() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "list-sessions") // want `direct call to os/exec.CommandContext`
	_, _ = cmd.Output()
}

// GOOD1: nolint comment on the same line — long-running cmd.Start() that needs the raw API.
func good1() {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "tmux", "-C", "attach-session", "-t", "foo") //nolint:norawexec long-running control-mode process; cmd.Start() lifecycle managed by caller
	_ = cmd.Start()
}

// GOOD2: nolint comment on the preceding line — same justification pattern.
func good2() {
	ctx := context.Background()
	//nolint:norawexec keepalive session creation at startup; executes once, not in a polling loop
	_ = exec.CommandContext(ctx, "tmux", "new-session", "-d", "-s", "keepalive")
}
