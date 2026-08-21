package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// ShellTmuxHandle manages a single shell as an independent sibling tmux session.
//
// Architecture (per adversarial review Challenge 1):
// Each shell is its own tmux session named "{parentName}_shell_{shellUUID}".
// This is NOT a window inside the parent session. Using a sibling session means:
//   - attach-session -t {shellSessionName} gives a fully isolated PTY
//   - killing the parent session does not affect shell sessions
//   - PTY output cannot bleed between Claude terminal and shell terminals
type ShellTmuxHandle struct {
	// sessionName is the full sibling tmux session name: "{parentName}_shell_{shellID}"
	sessionName string
	// serverSocket is the tmux server socket (-L flag). Empty = default server.
	serverSocket string
	// ptyFactory creates PTYs for attach-session.
	ptyFactory PtyFactory
	// cmdExec runs tmux subprocesses.
	cmdExec executor.Executor

	// spawnMu serializes Spawn calls; guards ptmx and attachCmd.
	spawnMu sync.Mutex
	// ptmx is the PTY file returned by attach-session. Nil until Attach() is called.
	ptmx *os.File
	// attachCmd is the tmux attach-session process that owns the PTY.
	attachCmd *exec.Cmd

	// lastKnownCols/Rows hold the most recent PTY dimensions.
	lastKnownCols atomic.Int32
	lastKnownRows atomic.Int32
}

// NewShellTmuxHandle creates a ShellTmuxHandle for the given sibling session name.
// sessionName should be the full computed name: "{parentPrefix}_shell_{shellUUID}".
func NewShellTmuxHandle(sessionName, serverSocket string, ptyFactory PtyFactory, cmdExec executor.Executor) *ShellTmuxHandle {
	return &ShellTmuxHandle{
		sessionName:  sessionName,
		serverSocket: serverSocket,
		ptyFactory:   ptyFactory,
		cmdExec:      cmdExec,
	}
}

// buildCmd creates a tmux command with optional -L server socket isolation.
func (h *ShellTmuxHandle) buildCmd(args ...string) *exec.Cmd {
	var cmdArgs []string
	if h.serverSocket != "" {
		cmdArgs = append(cmdArgs, "-L", h.serverSocket)
	}
	cmdArgs = append(cmdArgs, args...)
	return safeexec.CommandContext(context.Background(), Binary(), cmdArgs...)
}

// Spawn creates a new independent sibling tmux session running the given command in workDir.
// It does NOT attach a PTY; call Attach() for streaming I/O.
//
// Equivalent of: tmux new-session -d -s {sessionName} -c {workDir} -- /bin/sh -c {command}
func (h *ShellTmuxHandle) Spawn(workDir, command string) error {
	h.spawnMu.Lock()
	defer h.spawnMu.Unlock()

	if workDir == "" {
		workDir = "."
	}
	if command == "" {
		command = "/bin/sh"
	}

	// Build: tmux [-L socket] new-session -d -s {sessionName} -c {workDir} -- /bin/sh -c {command}
	// Using "-- /bin/sh -c {command}" avoids shell interpolation of the command string in the
	// tmux argument (SEC-4: no exec.Command("sh", "-c", fmt.Sprintf("... %s ...", command)) pattern).
	cmd := h.buildCmd("new-session", "-d", "-s", h.sessionName, "-c", workDir, "--", "/bin/sh", "-c", command)
	if err := h.cmdExec.Run(cmd); err != nil {
		return fmt.Errorf("ShellTmuxHandle.Spawn: tmux new-session failed for %q: %w", h.sessionName, err)
	}
	log.Info("shell tmux session spawned", "session", h.sessionName, "workDir", workDir, "command", command)
	return nil
}

// Attach creates a PTY connection to the shell's sibling tmux session.
// Idempotent: returns nil if already attached.
// This must be called before GetPTY() returns a usable file.
func (h *ShellTmuxHandle) Attach() error {
	h.spawnMu.Lock()
	defer h.spawnMu.Unlock()

	if h.ptmx != nil {
		return nil // already attached
	}

	attachCmd := h.buildCmd("attach-session", "-t", h.sessionName)
	ptmx, cmd, err := h.ptyFactory.Start(attachCmd)
	if err != nil {
		return fmt.Errorf("ShellTmuxHandle.Attach: pty.Start for %q failed: %w", h.sessionName, err)
	}
	h.ptmx = ptmx
	h.attachCmd = cmd
	log.Info("shell PTY attached", "session", h.sessionName)
	return nil
}

// GetPTY returns the PTY file for reading terminal output from the shell.
// Returns an error if Attach() has not been called or the handle is closed.
func (h *ShellTmuxHandle) GetPTY() (*os.File, error) {
	h.spawnMu.Lock()
	defer h.spawnMu.Unlock()

	if h.ptmx == nil {
		return nil, fmt.Errorf("ShellTmuxHandle.GetPTY: PTY not attached for session %q; call Attach() first", h.sessionName)
	}
	return h.ptmx, nil
}

// Resize updates the PTY window dimensions by running tmux resize-window.
func (h *ShellTmuxHandle) Resize(cols, rows int) error {
	cmd := h.buildCmd("resize-window", "-t", h.sessionName, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows))
	if err := h.cmdExec.Run(cmd); err != nil {
		return fmt.Errorf("ShellTmuxHandle.Resize: %w", err)
	}
	h.lastKnownCols.Store(int32(cols))
	h.lastKnownRows.Store(int32(rows))
	return nil
}

// Close stops the shell, kills the sibling tmux session, closes the PTY, and reaps
// the attach process to prevent zombies (matches TmuxSession.Close() pattern).
func (h *ShellTmuxHandle) Close() error {
	h.spawnMu.Lock()
	defer h.spawnMu.Unlock()

	var errs []string

	// Close PTY file descriptor first to unblock any ongoing reads.
	if h.ptmx != nil {
		if err := h.ptmx.Close(); err != nil {
			if !errors.Is(err, os.ErrClosed) {
				errs = append(errs, fmt.Sprintf("close PTY: %v", err))
			}
		}
		h.ptmx = nil
	}

	// Reap the attach process with a 5-second timeout to avoid zombies.
	if h.attachCmd != nil {
		done := make(chan struct{})
		go func() {
			_ = h.attachCmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Warn("timeout waiting for shell attach process to exit; killing", "session", h.sessionName)
			if h.attachCmd.Process != nil {
				_ = h.attachCmd.Process.Kill()
			}
			<-done
		}
		h.attachCmd = nil
	}

	// Kill the sibling tmux session.
	killCmd := h.buildCmd("kill-session", "-t", h.sessionName)
	if err := h.cmdExec.Run(killCmd); err != nil {
		// Ignore "no such session" errors — already gone.
		if !isTmuxSessionGone(err) {
			errs = append(errs, fmt.Sprintf("kill-session: %v", err))
		}
	} else {
		log.Info("shell tmux session killed", "session", h.sessionName)
	}

	if len(errs) > 0 {
		return fmt.Errorf("ShellTmuxHandle.Close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// ExitCode queries the exit status of the shell process after it exits.
// Uses tmux display-message to read #{pane_dead_status}.
// Returns (exitCode, true) if the shell has exited, (0, false) if still running or session gone.
func (h *ShellTmuxHandle) ExitCode() (int, bool) {
	// Use #{pane_dead_status} which is empty when the pane is alive.
	cmd := h.buildCmd("display-message", "-t", h.sessionName, "-p", "#{pane_dead_status}")
	out, err := h.cmdExec.Output(cmd)
	if err != nil {
		// Session/window may have vanished after exit.
		return 0, false
	}
	statusStr := strings.TrimSpace(string(out))
	if statusStr == "" {
		// Pane is still alive.
		return 0, false
	}
	code, err := strconv.Atoi(statusStr)
	if err != nil {
		return 0, false
	}
	return code, true
}

// DoesSessionExist checks if the sibling tmux session is still present.
func (h *ShellTmuxHandle) DoesSessionExist() bool {
	cmd := h.buildCmd("has-session", "-t", h.sessionName)
	err := h.cmdExec.Run(cmd)
	return err == nil
}

func isTmuxSessionGone(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such") || strings.Contains(msg, "not found") || strings.Contains(msg, "can't find")
}
