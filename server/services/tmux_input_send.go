package services

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// sendInputToTmux sends input bytes to a tmux session using tmux send-keys.
// Each byte is sent individually using -H (hex) format to handle special characters properly.
// serverSocket must be the same socket the target session's tmux server is bound to
// (e.g. instance.TmuxServerSocket) — an empty string targets the default socket.
// Without routing through ResolveSocket/Args here, send-keys unconditionally hits the
// default tmux server, silently missing any session running on an isolated socket.
func sendInputToTmux(serverSocket, tmuxSessionName string, data []byte) error {
	// Build send-keys command with hex-encoded bytes
	// Using -H flag to send hex bytes, which handles all special characters correctly
	baseArgs := make([]string, 0, 4+len(data))
	baseArgs = append(baseArgs, "send-keys", "-t", tmuxSessionName, "-H")
	for _, b := range data {
		baseArgs = append(baseArgs, fmt.Sprintf("%02x", b))
	}
	args := tmux.ResolveSocket(serverSocket).Args(baseArgs...)

	gateCtx, gateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer gateCancel()
	err := runTmuxInputGatedErr(gateCtx, serverSocket, func() error {
		// Use a fresh context for the command itself, not gateCtx. Gate
		// acquisition can consume most of gateCtx's budget under contention,
		// leaving the exec below racing an already-expiring deadline: if
		// tmux send-keys reaches the server just before cancellation kills
		// the client process, the caller sees an error and retries
		// (sendInputToTmuxWithRetry), re-sending keystrokes that already
		// landed — producing duplicated input in the pane. A fresh timeout
		// here guarantees the command always gets its full run budget.
		cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cmdCancel()
		return safeexec.CommandContext(cmdCtx, tmux.Binary(), args...).Run()
	})
	if err != nil {
		return fmt.Errorf("tmux send-keys failed: %w", err)
	}
	return nil
}

// sendInputToTmuxInputRetries bounds how many times sendInputToTmux is retried
// on failure before the input is given up on. Failures here are almost always
// transient exec-gate contention (the 5s acquire timeout in sendInputToTmux
// expiring under concurrent tmux subprocess load) rather than a real tmux
// error, so a short bounded retry recovers keystrokes that would otherwise be
// silently dropped with no client-visible signal.
const sendInputToTmuxInputRetries = 2

// sendInputToTmuxRetryBackoff is the delay between retries of sendInputToTmux.
const sendInputToTmuxRetryBackoff = 100 * time.Millisecond

// sendInputToTmuxWithRetry calls sendInputToTmux, retrying a bounded number of
// times on failure. See sendInputToTmuxInputRetries for why this exists.
func sendInputToTmuxWithRetry(serverSocket, tmuxSessionName string, data []byte) error {
	var err error
	for attempt := 0; attempt <= sendInputToTmuxInputRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(sendInputToTmuxRetryBackoff)
		}
		if err = sendInputToTmux(serverSocket, tmuxSessionName, data); err == nil {
			return nil
		}
	}
	return err
}

// runTmuxInputGatedErr acquires a tmux input-fast-lane exec-gate slot for
// serverSocket (bounded by ctx), runs fn, then releases the slot. Used
// specifically for keystroke input traffic (the legacy per-keystroke
// send-keys path, sendInputToTmux): it draws from tmux.AcquireInputExecSlot's
// pool, separate from the shared default pool runTmuxGatedErr uses, so user
// input never queues behind a background poller's capture-pane calls on the
// same tmux server.
func runTmuxInputGatedErr(ctx context.Context, serverSocket string, fn func() error) error {
	release, err := tmux.AcquireInputExecSlot(ctx, serverSocket)
	if err != nil {
		return fmt.Errorf("input exec gate: %w", err)
	}
	defer release()
	return fn()
}
