package session

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/domain"
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
func ResolveSessionBackend(cfg *config.Config, sessionName string, requestOverride domain.ProcessManagerBackend) domain.ProcessManagerBackend {
	RecordBackendInfo(cfg)
	if requestOverride != "" {
		return requestOverride
	}
	if forceTymux, ok := cfg.GetTymuxSessionOverride(sessionName); ok {
		if forceTymux {
			return domain.BackendTymux
		}
		return domain.BackendTmux
	}
	if backend := getSelectedBackend(); backend != "" {
		return backend
	}
	return domain.BackendTmux
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
func ResolveSessionBackendForTitle(cfg *config.Config, title string, requestOverride domain.ProcessManagerBackend) domain.ProcessManagerBackend {
	return ResolveSessionBackend(cfg, tmux.NewSessionName(title, tmux.TmuxPrefix).String(), requestOverride)
}

// pinResolvedBackend records the backend the process manager actually resolved
// when no per-session pin exists, so a later change of the process-wide default
// cannot silently move this Instance to another backend. Used for new Instances
// only: restored unpinned records keep following the live default. Call before
// the Instance is shared.
func (i *Instance) pinResolvedBackend() {
	if i.Backend != "" {
		return
	}
	i.Backend = processManagerBackendKey(i.processManager)
}

// getSelectedBackend resolves the process-wide default backend live, on
// every call — no process restart required to change it. cfg.ProcessManagerBackend
// (hand-editable in config.json) or the "tymux" feature flag
// (config.EffectiveTymuxEnabled, live-settable via SetTymuxGlobalOverride)
// each force BackendTymux; otherwise cfg.ProcessManagerBackend passes
// through as-is (so "native" still works), and "" falls through to
// NewProcessManager's defaultBackend/BackendTmux.
func getSelectedBackend() domain.ProcessManagerBackend {
	return selectedBackendFromConfig(config.LoadConfig())
}

// selectedBackendFromConfig is getSelectedBackend's pure core over an
// already-loaded config.
func selectedBackendFromConfig(cfg *config.Config) domain.ProcessManagerBackend {
	backend := domain.ProcessManagerBackend(cfg.ProcessManagerBackend)
	if backend == domain.BackendTymux || config.EffectiveTymuxEnabled(cfg) {
		return domain.BackendTymux
	}
	return backend
}
