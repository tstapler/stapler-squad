package instance

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
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

// SpawnShellRequest carries the parameters for Instance.SpawnShell.
type SpawnShellRequest struct {
	// Name is the optional user-visible label. Defaults to the command base name.
	Name string
	// Command is the command to run. Defaults to $SHELL or /bin/sh.
	Command string
	// WorkingDir is the starting directory. Defaults to the session's WorkingDir.
	WorkingDir string
}

// ShellRepository is the minimal persistence interface for per-session shell management.
// It is implemented by EntRepository; pass nil to disable persistence (e.g., tests).
type ShellRepository interface {
	// CreateShell persists a new shell record under the given session title.
	CreateShell(ctx context.Context, sessionTitle string, data domain.ShellData) error
	// ListShells returns all shell records for the given session title, ordered by order_index.
	ListShells(ctx context.Context, sessionTitle string) ([]domain.ShellData, error)
	// UpdateShellStatus sets the status (and optionally exit code) for the shell with the given ID.
	UpdateShellStatus(ctx context.Context, shellID, status string, exitCode *int) error
	// DeleteShell removes the shell record with the given ID.
	DeleteShell(ctx context.Context, shellID string) error
}

// InitialPromptRepository is the minimal persistence interface for recording when
// Instance.InitialPrompt was actually sent. It is implemented by EntRepository;
// pass nil to disable persistence (e.g., tests).
type InitialPromptRepository interface {
	// UpdateInitialPromptSentAt sets the initial_prompt_sent_at field for a session.
	UpdateInitialPromptSentAt(ctx context.Context, title string, t time.Time) error
}
