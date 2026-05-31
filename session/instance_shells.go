package session

// instance_shells.go — Shell spawn/stop/restart/reconcile methods on Instance.
//
// Architecture: each shell is an independent sibling tmux session named:
//   {parentSessionName}_shell_{shellUUID}
//
// This is NOT a window inside the parent session (adversarial review Challenge 1 resolution).
// PTY isolation: killing the parent does not affect shells; each shell gets its own PTY via
// a separate attach-session call targeting the sibling session.
//
// Synchronization:
//   - shells is a *ShellRegistry (xsync.Map) — bucket-level lock, never held across I/O.
//   - spawnShellMu (deadlock.Mutex) serializes SpawnShell calls to prevent concurrent spawn races.
//   - Per-shell exitCh is closed (not sent) on exit — fans out to any number of subscribers.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// shellRegistryEmbed holds runtime state for shell management on an Instance.
// It is embedded in Instance. The heavy-lifting concurrent map lives in *ShellRegistry
// (shell_registry.go); only the spawn serialiser and wait-group live here.
type shellRegistryEmbed struct {
	shells *ShellRegistry
	// spawnShellMu serializes concurrent SpawnShell calls.
	spawnShellMu deadlock.Mutex
}

// initShellRegistry initialises the ShellRegistry. Called from NewInstance.
func (i *Instance) initShellRegistry() {
	i.shells = newShellRegistry()
}

// shellWg convenience accessors — nil-safe so instances loaded without initShellRegistry
// can still call these without panicking.
func (i *Instance) shellWgAdd(n int) {
	if i.shells != nil {
		i.shells.wg.Add(n)
	}
}
func (i *Instance) shellWgDone() {
	if i.shells != nil {
		i.shells.wg.Done()
	}
}

// shellTmuxSessionName computes the sibling tmux session name for a given shell.
// Format: "{parentSessionName}_shell_{shellID}"
func (i *Instance) shellTmuxSessionName(shellID string) string {
	parentName := i.GetTmuxSessionName()
	return fmt.Sprintf("%s_shell_%s", parentName, shellID)
}

// SpawnShell creates and starts a new shell as an independent sibling tmux session.
// It persists the shell to the ent repository, registers it in memory, and launches
// the watchShellExit goroutine.
func (i *Instance) SpawnShell(ctx context.Context, req SpawnShellRequest) (*Shell, error) {
	// Serialize spawn calls to prevent concurrent tmux new-session races.
	i.spawnShellMu.Lock()
	defer i.spawnShellMu.Unlock()

	// Resolve working directory.
	workDir := req.WorkingDir
	if workDir == "" {
		workDir = i.WorkingDir
		if workDir == "" {
			workDir = i.Path
		}
	}

	// Validate working dir is within session workspace (SEC-2).
	if workDir != "" {
		absWork, err := filepath.Abs(workDir)
		if err == nil {
			absPath := i.Path
			if absPath != "" && !strings.HasPrefix(absWork, absPath) {
				log.Warn("shell working_dir outside session workspace; using session path",
					"session", i.Title, "requested", workDir, "workspace", absPath)
				workDir = absPath
			}
		}
	}

	// Resolve command.
	command := req.Command
	if command == "" {
		command = os.Getenv("SHELL")
		if command == "" {
			command = "/bin/sh"
		}
	}

	// Resolve name.
	name := req.Name
	if name == "" {
		name = filepath.Base(command)
	}

	// Assign shell ID.
	shellID := uuid.New().String()
	sessionName := i.shellTmuxSessionName(shellID)

	orderIndex := i.shells.Len()

	// Build the ShellTmuxHandle.
	cmdExec := executor.Exec{}
	handle := tmux.NewShellTmuxHandle(sessionName, i.TmuxServerSocket, tmux.MakePtyFactory(), cmdExec)

	// Spawn the sibling tmux session.
	if err := handle.Spawn(workDir, command); err != nil {
		return nil, fmt.Errorf("SpawnShell: %w", err)
	}

	// Build the in-memory Shell.
	shell := &Shell{
		ID:              shellID,
		Name:            name,
		Command:         command,
		WorkingDir:      workDir,
		TmuxSessionName: sessionName,
		Status:          ShellStatusRunning,
		OrderIndex:      orderIndex,
		StartedAt:       time.Now(),
		exitCh:          make(chan struct{}),
		watcherDone:     make(chan struct{}),
	}

	i.shells.Add(shell, handle)

	// Persist to database (best-effort; non-fatal on error).
	if i.shellRepo != nil {
		data := ShellData{
			ID:              shellID,
			Name:            name,
			Command:         command,
			WorkingDir:      workDir,
			TmuxSessionName: sessionName,
			OrderIndex:      orderIndex,
		}
		if _, err := i.shellRepo.CreateShell(ctx, i.Title, data); err != nil {
			log.Warn("SpawnShell: failed to persist shell", "session", i.Title, "shell", shellID, "err", err)
		}
	}

	// Watch for exit in background.
	i.shellWgAdd(1)
	go i.watchShellExit(ctx, shellID, handle, shell.exitCh, shell.watcherDone)

	log.Info("shell spawned", "session", i.Title, "shell", shellID, "name", name, "command", command)
	return shell, nil
}

// watchShellExit blocks reading from the shell's PTY until EOF or context cancellation.
// On exit it updates shell status, persists to ent, and closes exitCh to notify subscribers.
// watcherDone is closed after shellWg.Done() so DeleteShell can wait on this specific goroutine.
func (i *Instance) watchShellExit(ctx context.Context, shellID string, handle *tmux.ShellTmuxHandle, exitCh chan struct{}, watcherDone chan struct{}) {
	defer close(watcherDone) // registered first → runs last (after Done)
	defer i.shellWgDone()
	defer func() {
		defer func() { recover() }() //nolint:errcheck
		close(exitCh)
	}()

	// Attach the PTY so we can detect EOF.
	if err := handle.Attach(); err != nil {
		log.Warn("watchShellExit: attach failed", "session", i.Title, "shell", shellID, "err", err)
		i.setShellStatus(ctx, shellID, ShellStatusError, nil)
		return
	}

	pty, err := handle.GetPTY()
	if err != nil {
		log.Warn("watchShellExit: GetPTY failed", "session", i.Title, "shell", shellID, "err", err)
		i.setShellStatus(ctx, shellID, ShellStatusError, nil)
		return
	}

	// Drain PTY until EOF or context cancellation.
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			log.Info("watchShellExit: context cancelled", "session", i.Title, "shell", shellID)
			return
		default:
		}
		_, readErr := pty.Read(buf)
		if readErr != nil {
			break
		}
	}

	// Check if shell was intentionally stopped (stop-while-streaming guard, adversarial Challenge 3).
	sh, exists := i.shells.Get(shellID)
	if exists && sh.Status == ShellStatusStopped {
		return
	}

	// Determine exit code and final status.
	exitCode, ok := handle.ExitCode()
	var finalStatus ShellStatus
	if ok && exitCode == 0 {
		finalStatus = ShellStatusStopped
	} else if ok && exitCode != 0 {
		finalStatus = ShellStatusError
	} else {
		finalStatus = ShellStatusStopped
	}

	log.Info("shell exited", "session", i.Title, "shell", shellID, "exitCode", exitCode, "status", finalStatus)
	var exitCodePtr *int
	if ok {
		code := exitCode
		exitCodePtr = &code
	}
	i.setShellStatus(ctx, shellID, finalStatus, exitCodePtr)
}

// setShellStatus updates in-memory Shell.Status and persists to ent.
func (i *Instance) setShellStatus(ctx context.Context, shellID string, status ShellStatus, exitCode *int) {
	i.shells.UpdateStatus(shellID, status, exitCode)

	if i.shellRepo != nil {
		if err := i.shellRepo.UpdateShellStatus(ctx, shellID, string(status), exitCode); err != nil {
			log.Warn("setShellStatus: failed to persist", "shell", shellID, "err", err)
		}
	}
}

// StopShell stops a running shell by setting status first (stop-while-streaming guard),
// then closing the handle.
func (i *Instance) StopShell(ctx context.Context, shellID string) error {
	// Atomically mark stopped before closing PTY so watchShellExit treats the
	// read error as a clean stop (not a crash).
	updated := i.shells.UpdateStatus(shellID, ShellStatusStopped, nil)
	if !updated {
		return fmt.Errorf("StopShell: shell %q not found", shellID)
	}

	_, handle, _ := i.shells.GetBoth(shellID)
	if handle != nil {
		if err := handle.Close(); err != nil {
			log.Warn("StopShell: handle.Close error", "shell", shellID, "err", err)
		}
	}

	if i.shellRepo != nil {
		if err := i.shellRepo.UpdateShellStatus(ctx, shellID, string(ShellStatusStopped), nil); err != nil {
			log.Warn("StopShell: failed to persist status", "shell", shellID, "err", err)
		}
	}
	return nil
}

// RestartShell stops a shell (if running) and relaunches it with the same command and workdir.
func (i *Instance) RestartShell(ctx context.Context, shellID string) error {
	sh, ok := i.shells.Get(shellID)
	if !ok {
		return fmt.Errorf("RestartShell: shell %q not found", shellID)
	}
	command := sh.Command
	workDir := sh.WorkingDir
	exitCh := make(chan struct{})
	watcherDone := make(chan struct{})

	// Stop if currently running.
	if err := i.StopShell(ctx, shellID); err != nil {
		log.Warn("RestartShell: StopShell failed (may already be stopped)", "shell", shellID, "err", err)
	}

	// Build a new sibling session name (reuse same shell ID so ent FK survives).
	newSessionName := i.shellTmuxSessionName(shellID)
	cmdExec := executor.Exec{}
	handle := tmux.NewShellTmuxHandle(newSessionName, i.TmuxServerSocket, tmux.MakePtyFactory(), cmdExec)
	if err := handle.Spawn(workDir, command); err != nil {
		return fmt.Errorf("RestartShell: spawn failed: %w", err)
	}

	i.shells.UpdateForRestart(shellID, handle, newSessionName, exitCh, watcherDone)

	// Persist restart.
	if i.shellRepo != nil {
		if err := i.shellRepo.UpdateShellStatus(ctx, shellID, string(ShellStatusRunning), nil); err != nil {
			log.Warn("RestartShell: failed to persist running status", "shell", shellID, "err", err)
		}
	}

	// Launch new exit watcher.
	i.shellWgAdd(1)
	go i.watchShellExit(ctx, shellID, handle, exitCh, watcherDone)

	return nil
}

// GetShellPTYReader returns the PTY for streaming shell output.
// Lazily attaches if not yet attached (ADR-3: lazy PTY attach).
func (i *Instance) GetShellPTYReader(shellID string) (*os.File, error) {
	sh, handle, ok := i.shells.GetBoth(shellID)
	if !ok {
		return nil, fmt.Errorf("GetShellPTYReader: shell %q not found", shellID)
	}
	if sh.Status == ShellStatusStopped || sh.Status == ShellStatusError {
		return nil, ErrShellStopped
	}
	if handle == nil {
		return nil, fmt.Errorf("GetShellPTYReader: no handle for shell %q", shellID)
	}

	// Lazily attach if not yet attached.
	if err := handle.Attach(); err != nil {
		return nil, fmt.Errorf("GetShellPTYReader: attach failed: %w", err)
	}
	return handle.GetPTY()
}

// GetShellExitCh returns a channel that is closed when the shell exits.
// Multiple callers can select on it without coordination (closed-channel fan-out).
func (i *Instance) GetShellExitCh(shellID string) (<-chan struct{}, bool) {
	sh, ok := i.shells.Get(shellID)
	if !ok {
		return nil, false
	}
	return sh.exitCh, true
}

// AddShellInMemory registers a pre-built Shell directly into the in-memory registry without
// spawning a tmux process. Used by ReconcileShells and by tests that need to inject shells
// into an Instance without going through the full SpawnShell / tmux path.
func (i *Instance) AddShellInMemory(sh *Shell) {
	if sh == nil {
		return
	}
	if i.shells == nil {
		i.shells = newShellRegistry()
	}
	i.shells.AddStopped(sh)
}

// ListShellsInMemory returns in-memory shells sorted by OrderIndex.
func (i *Instance) ListShellsInMemory() []*Shell {
	return i.shells.List()
}

// DeleteShell stops a shell (if running), waits for active handlers to drain, then
// removes it from memory and the database.
func (i *Instance) DeleteShell(ctx context.Context, shellID string) error {
	// Snapshot watcherDone before stopping so we can wait on this shell's goroutine only.
	sh, _ := i.shells.Get(shellID)

	// Stop first to trigger PTY EOF and allow watchShellExit to return.
	_ = i.StopShell(ctx, shellID)

	// Wait for this shell's watcher goroutine to finish before removing from the map.
	if sh != nil && sh.watcherDone != nil {
		select {
		case <-sh.watcherDone:
		case <-time.After(6 * time.Second):
			log.Warn("DeleteShell: timeout waiting for shell watcher to drain", "shell", shellID)
		}
	}

	i.shells.Remove(shellID)

	if i.shellRepo != nil {
		if err := i.shellRepo.DeleteShell(ctx, shellID); err != nil {
			log.Warn("DeleteShell: failed to delete from db", "shell", shellID, "err", err)
		}
	}
	return nil
}

// ReconcileShells is called after an Instance is loaded from ent on startup.
// It queries ent for shells marked "running" and checks whether their sibling tmux
// sessions still exist. Live sessions are rebuilt in memory (without PTY attach — lazy).
// Dead sessions are marked stopped in ent.
//
// The map lock is never held during I/O: all subprocess calls and DB writes happen
// outside any map operation; each final map insert is an independent Store call that
// holds only the bucket lock for nanoseconds.
func (i *Instance) ReconcileShells(ctx context.Context) {
	if i.shellRepo == nil {
		return
	}
	dbShells, err := i.shellRepo.ListShells(ctx, i.Title)
	if err != nil {
		log.Warn("ReconcileShells: failed to list shells", "session", i.Title, "err", err)
		return
	}
	if len(dbShells) == 0 {
		return
	}

	cmdExec := executor.Exec{}

	// preparedShell holds all pre-computed state for one reconcile entry.
	type preparedShell struct {
		dbShell     *ent.Shell
		handle      *tmux.ShellTmuxHandle
		alive       bool // DoesSessionExist result; only meaningful when status=Running
		exitCh      chan struct{}
		watcherDone chan struct{}
	}

	// Phase 1 (no lock): create handles and probe subprocess existence for running shells.
	prepared := make([]preparedShell, len(dbShells))
	for idx, dbShell := range dbShells {
		handle := tmux.NewShellTmuxHandle(dbShell.TmuxSessionName, i.TmuxServerSocket, tmux.MakePtyFactory(), cmdExec)
		exitCh := make(chan struct{})
		watcherDone := make(chan struct{})
		var alive bool
		if dbShell.Status == string(ShellStatusRunning) {
			alive = handle.DoesSessionExist()
		}
		prepared[idx] = preparedShell{dbShell, handle, alive, exitCh, watcherDone}
	}

	// Phase 2 (no lock): persist stopped status for running shells whose tmux session vanished.
	for _, e := range prepared {
		if e.dbShell.Status == string(ShellStatusRunning) && !e.alive {
			log.Info("ReconcileShells: shell tmux session missing, marking stopped", "session", i.Title, "shell", e.dbShell.ID)
			close(e.exitCh)
			if updateErr := i.shellRepo.UpdateShellStatus(ctx, e.dbShell.ID, string(ShellStatusStopped), nil); updateErr != nil {
				log.Warn("ReconcileShells: failed to update status", "shell", e.dbShell.ID, "err", updateErr)
			}
		}
	}

	// Phase 3: register shells in the concurrent map. Each Store holds only a single
	// bucket lock for nanoseconds — no I/O, no goroutines launched yet.
	type watcherLaunch struct {
		shellID     string
		handle      *tmux.ShellTmuxHandle
		exitCh      chan struct{}
		watcherDone chan struct{}
	}
	var watchers []watcherLaunch

	for _, e := range prepared {
		if e.dbShell.Status == string(ShellStatusRunning) {
			if e.alive {
				sh := &Shell{
					ID:              e.dbShell.ID,
					Name:            e.dbShell.Name,
					Command:         e.dbShell.Command,
					WorkingDir:      e.dbShell.WorkingDir,
					TmuxSessionName: e.dbShell.TmuxSessionName,
					Status:          ShellStatusRunning,
					OrderIndex:      e.dbShell.OrderIndex,
					StartedAt:       e.dbShell.StartedAt,
					exitCh:          e.exitCh,
					watcherDone:     e.watcherDone,
				}
				i.shells.Add(sh, e.handle)
				watchers = append(watchers, watcherLaunch{e.dbShell.ID, e.handle, e.exitCh, e.watcherDone})
				log.Info("ReconcileShells: restored running shell", "session", i.Title, "shell", e.dbShell.ID)
			}
			// dead case: exitCh already closed and DB already updated in Phase 2; no map entry.
		} else {
			// Already stopped/error: rebuild read-only in-memory entry with closed exitCh.
			close(e.exitCh)
			exitCode := 0
			if e.dbShell.ExitCode != nil {
				exitCode = *e.dbShell.ExitCode
			}
			sh := &Shell{
				ID:              e.dbShell.ID,
				Name:            e.dbShell.Name,
				Command:         e.dbShell.Command,
				WorkingDir:      e.dbShell.WorkingDir,
				TmuxSessionName: e.dbShell.TmuxSessionName,
				Status:          ShellStatus(e.dbShell.Status),
				ExitCode:        exitCode,
				OrderIndex:      e.dbShell.OrderIndex,
				StartedAt:       e.dbShell.StartedAt,
				exitCh:          e.exitCh,
			}
			i.shells.AddStopped(sh)
		}
	}

	// Phase 4 (post-map): launch watcher goroutines for restored running shells.
	for _, w := range watchers {
		i.shellWgAdd(1)
		go i.watchShellExit(ctx, w.shellID, w.handle, w.exitCh, w.watcherDone)
	}
}
