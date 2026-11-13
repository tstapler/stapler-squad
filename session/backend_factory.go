package session

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tmux"
	"github.com/tstapler/stapler-squad/session/tymux"
)

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
func NewProcessManager(_ context.Context, defaultBackend domain.ProcessManagerBackend, opts ProcessManagerOptions) (ProcessManager, error) {
	backend := opts.Backend
	if backend == "" {
		backend = getSelectedBackend()
	}
	if backend == "" {
		backend = defaultBackend
	}
	if backend == "" {
		backend = domain.BackendTmux
	}

	return buildBackend(backend, opts)
}

// newTymuxBackendFromOpts constructs a TymuxBackend wired to a real, live
// tymuxd over gRPC (tymux.NewRealTransport) — Epic 2.3's standing Attach
// stream is the first real live-RPC usage in this implementation, so a
// stub transport can no longer stand in here the way it did through Epic
// 2.2 (see tymux.NewRealTransport's doc comment for why that gap existed
// and closing it landed in this epic rather than 2.1/2.2 as originally
// expected).
//
// The address comes from tymux.ResolveDaemonConfig().Addr -- NOT a bare ""
// (which would fall back to NewRealTransport's own tymuxdAddr(), the
// non-instance-aware resolver) -- so that a named STAPLER_SQUAD_INSTANCE's
// RPC traffic actually reaches the same instance-derived port supervision
// (EnsureDaemonRunning, session/tymux/daemon_config.go's ResolveDaemonConfig)
// spawns tymuxd at. Using tymuxdAddr() here instead would silently point
// every tymux-backed session's real traffic at the shared/default
// 127.0.0.1:7419 regardless of instance, even though its own supervision
// correctly started (or found) a daemon at a different, instance-scoped
// address -- a real found-in-review gap (not hypothetical: supervision and
// RPC traffic would simply never agree on which daemon to talk to for any
// named instance) fixed here rather than filed away, since both call sites
// ultimately need the identical DaemonConfig.Addr to agree.
//
// opts.Program, if set, is applied via SetCommand here (mirrors newTmuxBackendFromOpts below).
func newTymuxBackendFromOpts(opts ProcessManagerOptions) *TymuxBackend {
	b := NewTymuxBackend(newTymuxSessionFn())
	if cmd := opts.launchCommand(); cmd != nil {
		b.TymuxManager().SetCommand(cmd)
	}
	b.TymuxManager().SetSessionLabel(opts.SessionName)
	b.TymuxManager().SetStoredSessionID(opts.TymuxSessionID)
	return b
}

// newTymuxSessionFn is the injection seam for constructing a real tymux
// manager, mirroring backend_tymux.go's ensureDaemonRunningFn.
var newTymuxSessionFn = func() tymux.TymuxManager {
	return tymux.NewTymuxGRPCSession(tymux.NewRealTransport(tymux.ResolveDaemonConfig().Addr))
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

// SetTymuxManagerFactoryForTest makes every tymux-backed Instance built until tb ends use
// the manager fn returns, so tests outside this package need no live tymuxd. Not safe
// alongside parallel tests that build tymux Instances.
func SetTymuxManagerFactoryForTest(tb testing.TB, fn func() tymux.TymuxManager) {
	tb.Helper()
	orig := newTymuxSessionFn
	newTymuxSessionFn = fn
	tb.Cleanup(func() { newTymuxSessionFn = orig })
}

func init() { processManagerFactoryHook = NewProcessManager }
