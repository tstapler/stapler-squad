package session

import (
	"context"
	"errors"
	"strings"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
)

// isClaudeAntigravityFamily reports whether program belongs to the Claude/Antigravity
// family recognized by the history-porting and conversation-UUID heuristics below. Derived
// from the history adapter registry (rather than re-declaring its own claude/agy/antigravity string
// match) so this can't independently drift from which programs actually have a HistoryAdapter
// — the exact drift that caused "gemini" to be treated as portable by AgyAdapter.CanHandle
// but not by this family check.
func isClaudeAntigravityFamily(reg *HistoryAdapterRegistry, program string) bool {
	return reg.Handles(program)
}

// isClaudeAntigravityCrossSwitch reports whether oldProgram and newProgram cross between two
// distinct HistoryAdapter-backed programs (e.g. claude <-> agy <-> opencode) where conversation
// history should be ported rather than discarded.
func isClaudeAntigravityCrossSwitch(reg *HistoryAdapterRegistry, oldProgram, newProgram string) bool {
	return reg.CrossesAdapters(oldProgram, newProgram)
}

// portHistoryFailureIsExpected reports whether portErr is the low-severity ErrNoHistoryAdapter
// sentinel or an unlinked conversation UUID error (no conversation history generated yet)
// so SwitchProgram can pick Warn vs Error.
func portHistoryFailureIsExpected(portErr error) bool {
	if errors.Is(portErr, ErrNoHistoryAdapter) {
		return true
	}
	if portErr != nil && strings.Contains(portErr.Error(), "no conversation UUID found") {
		return true
	}
	return false
}

// SwitchProgram atomically switches this instance's Program to rawProgram (resolving an
// empty string to the configured default), porting Claude<->Antigravity conversation
// history when crossing between those two and clearing stale conversation linkage
// (ClearConversationState) when the switch leaves that family entirely. If persist is
// non-nil it runs after the field mutation but before an Active-session restart, so
// callers can make the new program durable even if the subsequent restart fails.
//
// The whole operation runs under a per-instance lock (restartTriggerMu, shared with
// SetAutoApprove) so a manual program-switch request, an automatic capacity-monitor
// fallback, and a post-creation auto-approve toggle firing near-simultaneously serialize
// instead of double-restarting or double-porting history. This is the single
// implementation shared by the UpdateSession RPC handler and the capacity-monitor
// auto-fallback path (SessionService.UpdateSessionProgram) so the two entry points can't
// drift.
//
// changed reports whether the resolved program actually differed from the current one; a
// no-op skips persist/restart entirely. err is only ever a Restart failure — persist
// failures are logged, not returned, matching the pre-existing best-effort save semantics.
func (i *Instance) SwitchProgram(ctx context.Context, rawProgram string, persist func() error) (changed bool, resolvedProgram string, err error) {
	i.restartTriggerMu.Lock()
	defer i.restartTriggerMu.Unlock()

	resolvedProgram = rawProgram
	if resolvedProgram == "" {
		resolvedProgram = config.LoadConfig().DefaultProgram
	}

	oldProgram := i.Program
	if oldProgram == resolvedProgram {
		return false, resolvedProgram, nil
	}

	i.SetProgram(resolvedProgram)

	switch {
	case isClaudeAntigravityCrossSwitch(i.HistoryRegistry(), oldProgram, resolvedProgram):
		if portErr := portSessionHistory(ctx, oldProgram, resolvedProgram, i); portErr != nil {
			if portHistoryFailureIsExpected(portErr) {
				// Low-severity: the family-gate above and each adapter's CanHandle
				// have drifted out of sync. Best-effort porting still no-ops safely.
				log.Warn("[SwitchProgram] no history adapter resolved for program pair; skipping history port", "session", i.Title, "old", oldProgram, "new", resolvedProgram)
			} else {
				log.Error("[SwitchProgram] failed to port session history during program switch", "session", i.Title, "old", oldProgram, "new", resolvedProgram, "err", portErr)
			}
		}
	case isClaudeAntigravityFamily(i.HistoryRegistry(), oldProgram) && !isClaudeAntigravityFamily(i.HistoryRegistry(), resolvedProgram):
		// Leaving the Claude/Antigravity family entirely: a stale --resume UUID
		// captured under the old program would fail against the new one.
		i.ClearConversationState()
	}

	if persist != nil {
		if saveErr := persist(); saveErr != nil {
			log.Warn("[SwitchProgram] failed to persist program change before restart", "session", i.Title, "err", saveErr)
		}
	}

	if i.Status == domain.Active {
		if restartErr := i.Restart(true, ""); restartErr != nil {
			return true, resolvedProgram, restartErr
		}
	}

	return true, resolvedProgram, nil
}

type HistoryAdapter interface {
	Name() string
	CanHandle(program string) bool

	// Import reads this CLI's native format and returns canonical turns.
	Import(ctx context.Context, inst *Instance) ([]CanonicalTurn, error)

	// Export writes canonical turns into this CLI's native format so it can resume.
	Export(ctx context.Context, turns []CanonicalTurn, inst *Instance) error
}

// HistoryAdapterRegistry resolves a program string to the first adapter whose CanHandle
// claims it, in registration order. Order is the priority for programs more than one
// adapter matches (e.g. "antigravity-claude" resolves to claude).
type HistoryAdapterRegistry struct {
	adapters []HistoryAdapter
}

// NewHistoryAdapterRegistry builds a registry; an empty list is a wiring bug, so it errors.
func NewHistoryAdapterRegistry(adapters ...HistoryAdapter) (*HistoryAdapterRegistry, error) {
	if len(adapters) == 0 {
		return nil, errors.New("history adapter registry needs at least one adapter")
	}
	return &HistoryAdapterRegistry{adapters: append([]HistoryAdapter(nil), adapters...)}, nil
}

// Resolve returns the first adapter that claims program, or nil if none does. It is the
// single resolution point shared by PortSessionHistory and the checkpoint create/fork paths.
func (r *HistoryAdapterRegistry) Resolve(program string) HistoryAdapter {
	for _, a := range r.adapters {
		if a.CanHandle(program) {
			return a
		}
	}
	return nil
}

// Handles reports whether any adapter claims program.
func (r *HistoryAdapterRegistry) Handles(program string) bool {
	return r.Resolve(program) != nil
}

// CrossesAdapters reports whether the two programs resolve to two distinct adapters.
func (r *HistoryAdapterRegistry) CrossesAdapters(oldProgram, newProgram string) bool {
	src, dst := r.Resolve(oldProgram), r.Resolve(newProgram)
	return src != nil && dst != nil && src.Name() != dst.Name()
}

// ErrNoHistoryAdapter indicates PortSessionHistory was asked to port history for a program
// pair where no registered HistoryAdapter claims one (or both) sides — e.g. opencode, aider,
// bash, or gemini (the real Gemini CLI, distinct from Antigravity's own storage format — see
// AgyAdapter.CanHandle), none of which have a canonical history format to port. Callers should
// treat this as an expected, low-severity no-op (log and continue), not a hard failure.
var ErrNoHistoryAdapter = errors.New("no history adapter resolves for this program")

// HistoryPorter translates a session's history from oldProgram's format to newProgram's.
// SwitchProgram calls it through historyPorterHook so Instance code does not name
// history_transfer.go and the concrete adapters it drives, which stay above it. A
// marginal seam: one production implementation (PortSessionHistory), no fake yet.
type HistoryPorter func(ctx context.Context, oldProgram, newProgram string, inst *Instance) error

// historyPorterHook is assigned once, in history_transfer.go's init().
var historyPorterHook HistoryPorter

func portSessionHistory(ctx context.Context, oldProgram, newProgram string, inst *Instance) error {
	if historyPorterHook == nil {
		panic("session: HistoryPorter not registered (history_transfer.go's init must set historyPorterHook)")
	}
	return historyPorterHook(ctx, oldProgram, newProgram, inst)
}

// historyRegistryHook is the registry the Instance resolves history adapters through;
// assigned once, in history_adapter.go's init(), because the concrete adapters stay
// above the Instance file set.
var historyRegistryHook *HistoryAdapterRegistry

// HistoryRegistry returns the adapter registry this instance resolves programs through;
// exported for history_transfer.go, which stays outside the Instance package.
func (i *Instance) HistoryRegistry() *HistoryAdapterRegistry {
	if historyRegistryHook == nil {
		panic("session: history adapter registry not registered (history_adapter.go's init must set historyRegistryHook)")
	}
	return historyRegistryHook
}
