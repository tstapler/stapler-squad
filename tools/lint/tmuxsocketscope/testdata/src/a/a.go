// Package a contains test fixtures for the tmuxsocketscope analyzer.
package a

import (
	"context"
	"os/exec"

	"executor/safeexec"
	"session/tmux"
)

// BAD1: raw literal assigned to a variable with no resolver call, then
// spread into the exec call. Mirrors the original server_registry.go
// keepalive bug: the args never touch ResolveSocket/prependSocket/Args.
func bad1() {
	ctx := context.Background()
	createArgs := []string{"new-session", "-d", "-s", "keepalive"}
	_ = safeexec.CommandContext(ctx, tmux.Binary(), createArgs...) // want `tmux command built without routing through ResolveSocket`
}

// BAD2: raw string literals passed directly at the call site, no variable,
// no resolver call anywhere. Mirrors the original orphan-sweep bug.
func bad2() {
	ctx := context.Background()
	_, _ = exec.CommandContext(ctx, tmux.Binary(), "list-sessions", "-F", "#{session_name}").Output() // want `tmux command built without routing through ResolveSocket`
}

// GOOD1: args built directly via Socket.Args.
func good1() {
	ctx := context.Background()
	socket := tmux.ResolveSocket("")
	args := socket.Args("list-sessions", "-F", "#{session_name}")
	_, _ = exec.CommandContext(ctx, tmux.Binary(), args...).Output()
}

// GOOD2: args built by referencing an already-resolved variable inside an
// append -- safety propagates through the identifier reference, matching
// the batchPaneActivity pattern in session/pty_discovery.go.
func good2() {
	ctx := context.Background()
	socket := tmux.ResolveSocket("")
	args := []string{"list-panes", "-a"}
	if socket != "" {
		args = append([]string{"-L", string(socket)}, args...)
	}
	_, _ = exec.CommandContext(ctx, tmux.Binary(), args...).Output()
}

// GOOD3: nolint comment on the same line -- a reviewed, intentionally
// unscoped targeted single-session call.
func good3() {
	ctx := context.Background()
	_, _ = exec.CommandContext(ctx, tmux.Binary(), "kill-session", "-t", "specific-session").Output() //nolint:tmuxsocketscope targeted single-session call, reviewed
}

// GOOD4: an exec call for a non-tmux binary is never flagged, regardless of
// how its args are built.
func good4() {
	ctx := context.Background()
	_, _ = exec.CommandContext(ctx, "git", "status").Output()
}

// GOOD5: a flat variadic call with an explicit, literal "-L" flag -- mirrors
// a dedicated isolated-test-server helper that always spells out its own
// socket name directly. None of "list-sessions" et al. can individually
// prove safety; the "-L" literal itself is the evidence, and it only needs
// to appear once among the trailing args.
func good5(socketName string) {
	ctx := context.Background()
	_, _ = exec.CommandContext(ctx, tmux.Binary(), "-L", socketName, "list-sessions", "-F", "#{session_name}").Output()
}
