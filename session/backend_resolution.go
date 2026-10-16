package session

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ResolveSessionBackend resolves the effective ProcessManagerBackend for a
// new session, in precedence order: an explicit per-request override, this
// session's config-backed override (in either direction — see
// research/features.md (b).5's resolveLocked bug this deliberately does not
// replicate), the already-gated process-wide default, then BackendTmux.
//
// sessionName must be the sanitized tmux session name (see tmux.NewSessionName),
// not the raw request title — that's the key TymuxSessionOverrides is keyed by.
func ResolveSessionBackend(cfg *config.Config, sessionName string, requestOverride ProcessManagerBackend) ProcessManagerBackend {
	if requestOverride != "" {
		return requestOverride
	}
	if forceTymux, ok := cfg.GetTymuxSessionOverride(sessionName); ok {
		if forceTymux {
			return BackendTymux
		}
		return BackendTmux
	}
	if backend := getSelectedBackend(); backend != "" {
		return backend
	}
	return BackendTmux
}

// ResolveSessionBackendForTitle derives the sanitized tmux session key from
// title via tmux.NewSessionName and resolves it through ResolveSessionBackend.
// This is the call every InstanceOptions{} construction site should use
// instead of calling ResolveSessionBackend directly with a hand-derived key:
// ResolveSessionBackend's own doc comment requires the sanitized name, not
// the raw title, and repeating that derivation at each call site (Phase 4,
// Epics 4.3/4.4) is a leaky-abstraction risk a future call site could get
// wrong silently (it would compile, and would just never match an existing
// TymuxSessionOverrides entry). Centralizing it here closes that gap for
// every current and future caller.
func ResolveSessionBackendForTitle(cfg *config.Config, title string, requestOverride ProcessManagerBackend) ProcessManagerBackend {
	return ResolveSessionBackend(cfg, tmux.NewSessionName(title, tmux.TmuxPrefix).String(), requestOverride)
}
