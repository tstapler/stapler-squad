package session

// shell_registry.go — Concurrent shell+handle map that enforces lock-scope discipline.
//
// ShellRegistry wraps xsync.Map[string, shellEntry] so callers cannot hold the map
// lock across I/O. The only mutations are via Compute callbacks that run atomically
// under a single bucket lock and complete in nanoseconds — no subprocess calls, no
// DB writes, no channel operations inside them.
//
// This replaces the shellRegistry embedded struct (shellsMu RWMutex + two plain maps)
// that caused live stalls: the write lock was previously held across DoesSessionExist()
// (subprocess-per-shell) and DB calls during ReconcileShells.

import (
	"sort"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// shellEntry pairs the in-memory Shell with its tmux handle. Stored as a value (not
// pointer) inside the xsync.Map so Compute receives an immutable copy that it can
// mutate and return without aliasing races.
type shellEntry struct {
	shell  *Shell
	handle *tmux.ShellTmuxHandle
}

// ShellRegistry is a concurrent shell+handle store. Zero value is not usable; create
// with newShellRegistry(). The exported API intentionally has no Lock/Unlock methods —
// mutations go through named operations that hold the bucket lock only for fast
// in-memory work.
type ShellRegistry struct {
	m  *xsync.Map[string, shellEntry]
	wg sync.WaitGroup
}

func newShellRegistry() *ShellRegistry {
	return &ShellRegistry{m: xsync.NewMap[string, shellEntry]()}
}

// Add stores a new shell+handle pair. Overwrites any existing entry for the same ID.
func (r *ShellRegistry) Add(sh *Shell, handle *tmux.ShellTmuxHandle) {
	if r == nil {
		return
	}
	r.m.Store(sh.ID, shellEntry{shell: sh, handle: handle})
}

// AddStopped stores a shell that has no live handle (already stopped or error).
func (r *ShellRegistry) AddStopped(sh *Shell) {
	if r == nil {
		return
	}
	r.m.Store(sh.ID, shellEntry{shell: sh})
}

// Remove deletes the entry for shellID. No-op if not present.
func (r *ShellRegistry) Remove(shellID string) {
	if r == nil {
		return
	}
	r.m.Delete(shellID)
}

// Get returns the Shell for shellID, or (nil, false) if absent.
func (r *ShellRegistry) Get(shellID string) (*Shell, bool) {
	if r == nil {
		return nil, false
	}
	e, ok := r.m.Load(shellID)
	if !ok {
		return nil, false
	}
	return e.shell, true
}

// GetHandle returns the ShellTmuxHandle for shellID, or (nil, false) if absent.
func (r *ShellRegistry) GetHandle(shellID string) (*tmux.ShellTmuxHandle, bool) {
	if r == nil {
		return nil, false
	}
	e, ok := r.m.Load(shellID)
	if !ok {
		return nil, false
	}
	return e.handle, true
}

// GetBoth returns both shell and handle in one atomic load.
func (r *ShellRegistry) GetBoth(shellID string) (*Shell, *tmux.ShellTmuxHandle, bool) {
	if r == nil {
		return nil, nil, false
	}
	e, ok := r.m.Load(shellID)
	if !ok {
		return nil, nil, false
	}
	return e.shell, e.handle, true
}

// List returns all shells sorted by OrderIndex.
func (r *ShellRegistry) List() []*Shell {
	if r == nil {
		return nil
	}
	var result []*Shell
	r.m.Range(func(_ string, e shellEntry) bool {
		result = append(result, e.shell)
		return true
	})
	sort.Slice(result, func(a, b int) bool {
		return result[a].OrderIndex < result[b].OrderIndex
	})
	return result
}

// Len returns the number of shells in the registry.
func (r *ShellRegistry) Len() int {
	if r == nil {
		return 0
	}
	return r.m.Size()
}

// UpdateStatus atomically updates Shell.Status and Shell.ExitCode for shellID.
// Returns true if the entry was found and updated.
func (r *ShellRegistry) UpdateStatus(shellID string, status ShellStatus, exitCode *int) bool {
	if r == nil {
		return false
	}
	var updated bool
	r.m.Compute(shellID, func(e shellEntry, loaded bool) (shellEntry, xsync.ComputeOp) {
		if !loaded || e.shell == nil {
			return e, xsync.CancelOp
		}
		// COW: copy the Shell so existing pointers to the old struct remain valid
		// (e.g., callers holding sh.exitCh do not need to re-read).
		newShell := *e.shell
		newShell.Status = status
		if exitCode != nil {
			newShell.ExitCode = *exitCode
		}
		updated = true
		return shellEntry{shell: &newShell, handle: e.handle}, xsync.UpdateOp
	})
	return updated
}

// UpdateForRestart atomically replaces a shell's mutable restart fields with new
// values (Status=Running, ExitCode=0, new TmuxSessionName, new exitCh/watcherDone).
// If the shellID is not found it stores a brand-new entry built from newShell.
func (r *ShellRegistry) UpdateForRestart(shellID string, newHandle *tmux.ShellTmuxHandle, newSessionName string, exitCh, watcherDone chan struct{}) {
	if r == nil {
		return
	}
	r.m.Compute(shellID, func(e shellEntry, loaded bool) (shellEntry, xsync.ComputeOp) {
		var sh *Shell
		if loaded && e.shell != nil {
			copied := *e.shell
			copied.Status = ShellStatusRunning
			copied.ExitCode = 0
			copied.TmuxSessionName = newSessionName
			copied.exitCh = exitCh
			copied.watcherDone = watcherDone
			sh = &copied
		} else {
			sh = &Shell{
				ID:              shellID,
				TmuxSessionName: newSessionName,
				Status:          ShellStatusRunning,
				StartedAt:       time.Now(),
				exitCh:          exitCh,
				watcherDone:     watcherDone,
			}
		}
		return shellEntry{shell: sh, handle: newHandle}, xsync.UpdateOp
	})
}

// SetHandle atomically replaces the handle for shellID without changing the shell.
func (r *ShellRegistry) SetHandle(shellID string, handle *tmux.ShellTmuxHandle) {
	if r == nil {
		return
	}
	r.m.Compute(shellID, func(e shellEntry, loaded bool) (shellEntry, xsync.ComputeOp) {
		if !loaded {
			return e, xsync.CancelOp
		}
		return shellEntry{shell: e.shell, handle: handle}, xsync.UpdateOp
	})
}
