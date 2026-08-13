package session

import (
	"context"
	"sync"

	"github.com/tstapler/stapler-squad/session/tmux"
)

var (
	selectedBackendMu    sync.RWMutex
	selectedBackendValue ProcessManagerBackend = BackendTmux
)

// RegisterBackendProvider sets the backend used by NewProcessManager.
// Call once at startup, before any session is created.
func RegisterBackendProvider(backend ProcessManagerBackend) {
	selectedBackendMu.Lock()
	selectedBackendValue = backend
	selectedBackendMu.Unlock()
}

func getSelectedBackend() ProcessManagerBackend {
	selectedBackendMu.RLock()
	v := selectedBackendValue
	selectedBackendMu.RUnlock()
	return v
}

// NewProcessManager returns the ProcessManager implementation selected by the
// registered backend. Falls back to TmuxBackend for unknown values.
func NewProcessManager(_ context.Context, defaultBackend ProcessManagerBackend, opts ProcessManagerOptions) ProcessManager {
	backend := getSelectedBackend()
	if backend == "" {
		backend = defaultBackend
	}
	if backend == "" {
		backend = BackendTmux
	}

	switch backend {
	case BackendTmux:
		return newTmuxBackendFromOpts(opts)
	case BackendNative:
		return NewNativeProcessManager(opts)
	default:
		return newTmuxBackendFromOpts(opts)
	}
}

// newTmuxBackendFromOpts constructs a TmuxProcessManager from ProcessManagerOptions
// and wraps it in a TmuxBackend. When no program/opts are provided, returns an
// empty-backed TmuxBackend that callers populate via SetSession() (initTmuxSession).
func newTmuxBackendFromOpts(opts ProcessManagerOptions) *TmuxBackend {
	mgr := &TmuxProcessManager{}
	if opts.SessionName != "" && opts.Program != "" {
		var sess *tmux.TmuxSession
		if opts.ServerSocket != "" {
			sess = tmux.NewTmuxSessionWithServerSocket(opts.SessionName, opts.Program, opts.Prefix, opts.ServerSocket, tmux.WithRegistry(nil))
		} else {
			prefix := opts.Prefix
			if prefix == "" {
				prefix = "staplersquad_"
			}
			sess = tmux.NewTmuxSessionWithPrefix(opts.SessionName, opts.Program, prefix)
		}
		mgr.SetSession(sess)
	}
	return NewTmuxBackend(mgr)
}
