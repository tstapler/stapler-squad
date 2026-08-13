package session

import (
	"errors"
	"os"
	"time"
)

// ErrShellStopped is returned when an operation is attempted on a shell that has been stopped.
var ErrShellStopped = errors.New("shell is stopped")

// ShellStatus represents the lifecycle status of a custom shell.
type ShellStatus string

const (
	// ShellStatusRunning means the shell process is alive and the PTY is open.
	ShellStatusRunning ShellStatus = "running"
	// ShellStatusStopped means the shell exited cleanly (via exit command or StopShell).
	ShellStatusStopped ShellStatus = "stopped"
	// ShellStatusError means the shell exited with a non-zero status unexpectedly.
	ShellStatusError ShellStatus = "error"
)

// ShellHandle is the interface for managing a single shell PTY. It is implemented by
// session/tmux.ShellTmuxHandle for real sessions and can be mocked in tests.
type ShellHandle interface {
	// GetPTY returns the PTY file for reading terminal output.
	// Returns ErrShellStopped if the shell has been closed.
	GetPTY() (*os.File, error)
	// Resize updates the PTY dimensions.
	Resize(cols, rows int) error
	// Close stops the shell process and releases resources.
	Close() error
}

// Shell represents a custom shell attached to a session. It is the in-memory projection
// of the ent Shell entity; changes are written back through the repository.
type Shell struct {
	// ID is the stable UUID for this shell. Also the fragment used in the tmux session name.
	ID string
	// Name is the user-visible label for the shell tab.
	Name string
	// Command is the command running in the shell (e.g. "bash", "python").
	Command string
	// WorkingDir is the working directory for the shell process.
	WorkingDir string
	// TmuxSessionName is the full computed tmux session name:
	// "{parentPrefix}_shell_{shellID}"
	TmuxSessionName string
	// Status is the current lifecycle status.
	Status ShellStatus
	// ExitCode is the process exit code (meaningful when Status != ShellStatusRunning).
	ExitCode int
	// OrderIndex controls tab display order.
	OrderIndex int
	// StartedAt is when the shell was spawned.
	StartedAt time.Time

	// exitCh is closed (not sent to) when the shell exits or is stopped.
	// Multiple subscribers can range-select without coordination.
	// Created in SpawnShell; closed exactly once by watchShellExit.
	exitCh chan struct{}

	// watcherDone is closed after shellWg.Done() returns inside watchShellExit.
	// DeleteShell waits on this channel so it tracks only this shell's goroutine,
	// not the global WaitGroup which covers all shells.
	watcherDone chan struct{}
}

// ShellData carries the input fields for creating a new shell.
type ShellData struct {
	// ID is the UUID for the shell. If empty, the caller must populate it.
	ID string
	// Name is the user-visible label.
	Name string
	// Command is the command to run in the shell.
	Command string
	// WorkingDir is the starting directory for the shell process.
	WorkingDir string
	// TmuxSessionName is the full sibling tmux session name.
	TmuxSessionName string
	// OrderIndex is the display order for the tab.
	OrderIndex int
}

// SpawnShellRequest carries the parameters for Instance.SpawnShell.
type SpawnShellRequest struct {
	// Name is the optional user-visible label. Defaults to the command base name.
	Name string
	// Command is the command to run. Defaults to $SHELL or /bin/sh.
	Command string
	// WorkingDir is the starting directory. Defaults to the session's WorkingDir.
	WorkingDir string
}
