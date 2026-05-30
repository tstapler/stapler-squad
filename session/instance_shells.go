package session

// instance_shells.go — Shell registry, spawn/stop/restart/reconcile methods on Instance.
//
// Architecture: each shell is an independent sibling tmux session named:
//   {parentSessionName}_shell_{shellUUID}
//
// This is NOT a window inside the parent session (adversarial review Challenge 1 resolution).
// PTY isolation: killing the parent does not affect shells; each shell gets its own PTY via
// a separate attach-session call targeting the sibling session.
//
// Synchronization:
//   - shellsMu (deadlock.RWMutex) guards all reads/writes to shells and shellHandles maps.
//   - spawnShellMu (deadlock.Mutex) serializes SpawnShell calls to prevent concurrent spawn races.
//   - Per-shell exitCh is closed (not sent) on exit — fans out to any number of subscribers.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// shellRegistry holds runtime state for shell management on an Instance.
// It is embedded in Instance.
type shellRegistry struct {
	// shellsMu guards shells and shellHandles maps.
	// Use RLock for reads; Lock for writes.
	shellsMu deadlock.RWMutex
	// shells maps shellID → in-memory Shell.
	shells map[string]*Shell
	// shellHandles maps shellID → ShellTmuxHandle.
	shellHandles map[string]*tmux.ShellTmuxHandle
	// spawnShellMu serializes concurrent SpawnShell calls.
	// Acquired before any tmux new-session call to prevent races on tmux window state.
	spawnShellMu deadlock.Mutex
	// shellWg tracks active watchShellExit goroutines and StreamTerminal handlers.
	// Waited by DeleteShell to ensure no handler holds a dangling reference after map removal.
	shellWg sync.WaitGroup
}

// initShellRegistry initializes the shell maps. Called from NewInstance.
func (i *Instance) initShellRegistry() {
	i.shells = make(map[string]*Shell)
	i.shellHandles = make(map[string]*tmux.ShellTmuxHandle)
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
			// Allow any absolute path — sec review: we restrict to session path prefix.
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

	// Determine the order index (append to end).
	i.shellsMu.RLock()
	orderIndex := len(i.shells)
	i.shellsMu.RUnlock()

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

	// Register in memory.
	i.shellsMu.Lock()
	i.shells[shellID] = shell
	i.shellHandles[shellID] = handle
	i.shellsMu.Unlock()

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
	i.shellWg.Add(1)
	go i.watchShellExit(ctx, shellID, handle, shell.exitCh, shell.watcherDone)

	log.Info("shell spawned", "session", i.Title, "shell", shellID, "name", name, "command", command)
	return shell, nil
}

// watchShellExit blocks reading from the shell's PTY until EOF or context cancellation.
// On exit it updates shell status, persists to ent, and closes exitCh to notify subscribers.
// watcherDone is closed after shellWg.Done() so DeleteShell can wait on this specific goroutine.
func (i *Instance) watchShellExit(ctx context.Context, shellID string, handle *tmux.ShellTmuxHandle, exitCh chan struct{}, watcherDone chan struct{}) {
	defer close(watcherDone) // registered first → runs last (after Done)
	defer i.shellWg.Done()
	defer func() {
		// Safely close exitCh exactly once; using recover in case another path already closed it.
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
	i.shellsMu.RLock()
	sh, exists := i.shells[shellID]
	var alreadyStopped bool
	if exists {
		alreadyStopped = sh.Status == ShellStatusStopped
	}
	i.shellsMu.RUnlock()

	if alreadyStopped {
		// StopShell already updated status; nothing to do here.
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
	i.shellsMu.Lock()
	sh, ok := i.shells[shellID]
	if ok {
		sh.Status = status
		if exitCode != nil {
			sh.ExitCode = *exitCode
		}
	}
	i.shellsMu.Unlock()

	if i.shellRepo != nil {
		var exitCodePtr *int
		if exitCode != nil {
			exitCodePtr = exitCode
		}
		if err := i.shellRepo.UpdateShellStatus(ctx, shellID, string(status), exitCodePtr); err != nil {
			log.Warn("setShellStatus: failed to persist", "shell", shellID, "err", err)
		}
	}
}

// StopShell stops a running shell by setting status first (stop-while-streaming guard),
// then closing the handle.
func (i *Instance) StopShell(ctx context.Context, shellID string) error {
	i.shellsMu.Lock()
	sh, ok := i.shells[shellID]
	if !ok {
		i.shellsMu.Unlock()
		return fmt.Errorf("StopShell: shell %q not found", shellID)
	}
	// Set status before closing PTY so watchShellExit won't treat the read error as a crash.
	sh.Status = ShellStatusStopped
	handle := i.shellHandles[shellID]
	i.shellsMu.Unlock()

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
	i.shellsMu.RLock()
	sh, ok := i.shells[shellID]
	if !ok {
		i.shellsMu.RUnlock()
		return fmt.Errorf("RestartShell: shell %q not found", shellID)
	}
	command := sh.Command
	workDir := sh.WorkingDir
	name := sh.Name
	orderIndex := sh.OrderIndex
	exitCh := make(chan struct{})
	watcherDone := make(chan struct{})
	i.shellsMu.RUnlock()

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

	// Update in-memory state.
	i.shellsMu.Lock()
	if existing, exists := i.shells[shellID]; exists {
		existing.Status = ShellStatusRunning
		existing.ExitCode = 0
		existing.TmuxSessionName = newSessionName
		existing.exitCh = exitCh
		existing.watcherDone = watcherDone
	} else {
		i.shells[shellID] = &Shell{
			ID:              shellID,
			Name:            name,
			Command:         command,
			WorkingDir:      workDir,
			TmuxSessionName: newSessionName,
			Status:          ShellStatusRunning,
			OrderIndex:      orderIndex,
			StartedAt:       time.Now(),
			exitCh:          exitCh,
			watcherDone:     watcherDone,
		}
	}
	i.shellHandles[shellID] = handle
	i.shellsMu.Unlock()

	// Persist restart.
	if i.shellRepo != nil {
		if err := i.shellRepo.UpdateShellStatus(ctx, shellID, string(ShellStatusRunning), nil); err != nil {
			log.Warn("RestartShell: failed to persist running status", "shell", shellID, "err", err)
		}
	}

	// Launch new exit watcher.
	i.shellWg.Add(1)
	go i.watchShellExit(ctx, shellID, handle, exitCh, watcherDone)

	return nil
}

// GetShellPTYReader returns the PTY for streaming shell output.
// Lazily attaches if not yet attached (ADR-3: lazy PTY attach).
func (i *Instance) GetShellPTYReader(shellID string) (*os.File, error) {
	i.shellsMu.RLock()
	sh, ok := i.shells[shellID]
	handle := i.shellHandles[shellID]
	i.shellsMu.RUnlock()

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
	i.shellsMu.RLock()
	defer i.shellsMu.RUnlock()
	sh, ok := i.shells[shellID]
	if !ok {
		return nil, false
	}
	return sh.exitCh, true
}

// AddShellInMemory registers a pre-built Shell directly into the in-memory registry without
// spawning a tmux process. Used by ReconcileShells and by tests that need to inject shells
// into an Instance without going through the full SpawnShell / tmux path.
// The caller is responsible for setting all fields on sh (including exitCh / watcherDone
// if DeleteShell must drain the watcher goroutine; pass closed channels for stopped shells).
// Safe to call on instances loaded from storage that have not yet called initShellRegistry.
func (i *Instance) AddShellInMemory(sh *Shell) {
	if sh == nil {
		return
	}
	i.shellsMu.Lock()
	if i.shells == nil {
		i.shells = make(map[string]*Shell)
	}
	if i.shellHandles == nil {
		i.shellHandles = make(map[string]*tmux.ShellTmuxHandle)
	}
	i.shells[sh.ID] = sh
	i.shellsMu.Unlock()
}

// ListShellsInMemory returns in-memory shells sorted by OrderIndex.
func (i *Instance) ListShellsInMemory() []*Shell {
	i.shellsMu.RLock()
	defer i.shellsMu.RUnlock()

	result := make([]*Shell, 0, len(i.shells))
	for _, sh := range i.shells {
		result = append(result, sh)
	}
	sort.Slice(result, func(a, b int) bool {
		return result[a].OrderIndex < result[b].OrderIndex
	})
	return result
}

// DeleteShell stops a shell (if running), waits for active handlers to drain, then
// removes it from memory and the database.
func (i *Instance) DeleteShell(ctx context.Context, shellID string) error {
	// Snapshot watcherDone before stopping so we can wait on this shell's goroutine only.
	i.shellsMu.RLock()
	sh := i.shells[shellID]
	i.shellsMu.RUnlock()

	// Stop first to trigger PTY EOF and allow watchShellExit to return.
	_ = i.StopShell(ctx, shellID)

	// Wait for this shell's watcher goroutine to finish before removing from the map.
	// Using sh.watcherDone (per-shell) instead of the global shellWg so concurrent deletes
	// of independent shells do not block each other.
	if sh != nil && sh.watcherDone != nil {
		select {
		case <-sh.watcherDone:
		case <-time.After(6 * time.Second):
			log.Warn("DeleteShell: timeout waiting for shell watcher to drain", "shell", shellID)
		}
	}

	i.shellsMu.Lock()
	delete(i.shells, shellID)
	delete(i.shellHandles, shellID)
	i.shellsMu.Unlock()

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
func (i *Instance) ReconcileShells(ctx context.Context) {
	if i.shellRepo == nil {
		return
	}
	dbShells, err := i.shellRepo.ListShells(ctx, i.Title)
	if err != nil {
		log.Warn("ReconcileShells: failed to list shells", "session", i.Title, "err", err)
		return
	}

	cmdExec := executor.Exec{}

	i.shellsMu.Lock()
	defer i.shellsMu.Unlock()

	for _, dbShell := range dbShells {
		handle := tmux.NewShellTmuxHandle(dbShell.TmuxSessionName, i.TmuxServerSocket, tmux.MakePtyFactory(), cmdExec)
		exitCh := make(chan struct{})
		watcherDone := make(chan struct{})

		if dbShell.Status == string(ShellStatusRunning) {
			if handle.DoesSessionExist() {
				// Rebuild in-memory Shell with lazy attach (ADR-3).
				sh := &Shell{
					ID:              dbShell.ID,
					Name:            dbShell.Name,
					Command:         dbShell.Command,
					WorkingDir:      dbShell.WorkingDir,
					TmuxSessionName: dbShell.TmuxSessionName,
					Status:          ShellStatusRunning,
					OrderIndex:      dbShell.OrderIndex,
					StartedAt:       dbShell.StartedAt,
					exitCh:          exitCh,
					watcherDone:     watcherDone,
				}
				i.shells[dbShell.ID] = sh
				i.shellHandles[dbShell.ID] = handle
				i.shellWg.Add(1)
				go i.watchShellExit(ctx, dbShell.ID, handle, exitCh, watcherDone)
				log.Info("ReconcileShells: restored running shell", "session", i.Title, "shell", dbShell.ID)
			} else {
				// Session gone; mark as stopped.
				log.Info("ReconcileShells: shell tmux session missing, marking stopped", "session", i.Title, "shell", dbShell.ID)
				close(exitCh)
				if updateErr := i.shellRepo.UpdateShellStatus(ctx, dbShell.ID, string(ShellStatusStopped), nil); updateErr != nil {
					log.Warn("ReconcileShells: failed to update status", "shell", dbShell.ID, "err", updateErr)
				}
			}
		} else {
			// Already stopped/error: rebuild read-only in-memory entry with closed exitCh.
			close(exitCh)
			exitCode := 0
			if dbShell.ExitCode != nil {
				exitCode = *dbShell.ExitCode
			}
			sh := &Shell{
				ID:              dbShell.ID,
				Name:            dbShell.Name,
				Command:         dbShell.Command,
				WorkingDir:      dbShell.WorkingDir,
				TmuxSessionName: dbShell.TmuxSessionName,
				Status:          ShellStatus(dbShell.Status),
				ExitCode:        exitCode,
				OrderIndex:      dbShell.OrderIndex,
				StartedAt:       dbShell.StartedAt,
				exitCh:          exitCh,
			}
			i.shells[dbShell.ID] = sh
		}
	}
}
