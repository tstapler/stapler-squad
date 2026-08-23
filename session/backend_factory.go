package session

import (
	"context"
	"fmt"
	"sync"

	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// ErrUnrecognizedBackend is returned by NewProcessManager when the resolved
// ProcessManagerBackend value (from opts.Backend, the process-wide global, or
// defaultBackend) matches none of the known backend constants
// (BackendTmux/BackendNative/BackendTymux). Story 2.1.3's acceptance
// criteria (UX-9.2) require this to fail loudly at construction, not
// silently fall back to BackendTmux and not panic — an unrecognized value is
// most likely a typo'd constant or corrupted persisted data, and a silent
// tmux fallback would mask that.
var ErrUnrecognizedBackend = fmt.Errorf("session: unrecognized ProcessManagerBackend")

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

// NewProcessManager returns the ProcessManager implementation selected by, in order
// of precedence: opts.Backend (an explicit per-session override), the registered
// process-wide backend (RegisterBackendProvider), then defaultBackend. Falls back to
// TmuxBackend for the empty value (no override anywhere in the chain); an explicit
// but unrecognized value returns ErrUnrecognizedBackend rather than silently
// constructing a TmuxBackend (Story 2.1.3 AC / UX-9.2: an illegal backend constant
// must fail loudly at construction, not silently downgrade to a working-but-wrong
// backend or panic).
//
// opts.Backend was added ahead of the process-wide global specifically so a caller
// can request BackendTymux for one session without affecting every other concurrent
// session — passing defaultBackend alone can never win here, since the process-wide
// global (set at package init to BackendTmux and never empty in production) is always
// checked first. See plan.md Epic 2.1 Story 2.1.3 for why defaultBackend's precedence
// position is otherwise unchanged.
func NewProcessManager(_ context.Context, defaultBackend ProcessManagerBackend, opts ProcessManagerOptions) (ProcessManager, error) {
	backend := opts.Backend
	if backend == "" {
		backend = getSelectedBackend()
	}
	if backend == "" {
		backend = defaultBackend
	}
	if backend == "" {
		backend = BackendTmux
	}

	switch backend {
	case BackendTmux:
		return newTmuxBackendFromOpts(opts), nil
	case BackendNative:
		return NewNativeProcessManager(opts), nil
	case BackendTymux:
		return newTymuxBackendFromOpts(opts), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnrecognizedBackend, backend)
	}
}

// newTymuxBackendFromOpts constructs a TymuxBackend wired to a real, live
// tymuxd over gRPC (tymux.NewRealTransport) — Epic 2.3's standing Attach
// stream is the first real live-RPC usage in this implementation, so a
// stub transport can no longer stand in here the way it did through Epic
// 2.2 (see tymux.NewRealTransport's doc comment for why that gap existed
// and closing it landed in this epic rather than 2.1/2.2 as originally
// expected). The address defaults to tymuxd's documented loopback
// address, overridable via the TYMUXD_ADDR environment variable.
func newTymuxBackendFromOpts(opts ProcessManagerOptions) *TymuxBackend {
	return NewTymuxBackend(tymux.NewTymuxGRPCSession(tymux.NewRealTransport("")))
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
