package session

import (
	"context"
	"errors"
	"strings"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// isClaudeAntigravityFamily reports whether program belongs to the Claude/Antigravity
// family recognized by the history-porting and conversation-UUID heuristics below.
func isClaudeAntigravityFamily(program string) bool {
	return strings.Contains(program, "claude") || strings.Contains(program, "agy") || strings.Contains(program, "antigravity")
}

// isClaudeAntigravityCrossSwitch reports whether oldProgram and newProgram sit on opposite
// sides of the Claude/Antigravity family (e.g. claude -> antigravity or vice versa) — the
// case where conversation history should be ported rather than discarded.
func isClaudeAntigravityCrossSwitch(oldProgram, newProgram string) bool {
	return (strings.Contains(oldProgram, "claude") && (strings.Contains(newProgram, "agy") || strings.Contains(newProgram, "antigravity"))) ||
		((strings.Contains(oldProgram, "agy") || strings.Contains(oldProgram, "antigravity")) && strings.Contains(newProgram, "claude"))
}

// SwitchProgram atomically switches this instance's Program to rawProgram (resolving an
// empty string to the configured default), porting Claude<->Antigravity conversation
// history when crossing between those two and clearing stale conversation linkage
// (ClearConversationState) when the switch leaves that family entirely. If persist is
// non-nil it runs after the field mutation but before an Active-session restart, so
// callers can make the new program durable even if the subsequent restart fails.
//
// The whole operation runs under a per-instance lock (programSwitchMu) so a manual
// program-switch request and an automatic capacity-monitor fallback firing near-
// simultaneously serialize instead of double-restarting or double-porting history. This
// is the single implementation shared by the UpdateSession RPC handler and the
// capacity-monitor auto-fallback path (SessionService.UpdateSessionProgram) so the two
// entry points can't drift.
//
// changed reports whether the resolved program actually differed from the current one; a
// no-op skips persist/restart entirely. err is only ever a Restart failure — persist
// failures are logged, not returned, matching the pre-existing best-effort save semantics.
func (i *Instance) SwitchProgram(ctx context.Context, rawProgram string, persist func() error) (changed bool, resolvedProgram string, err error) {
	i.programSwitchMu.Lock()
	defer i.programSwitchMu.Unlock()

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
	case isClaudeAntigravityCrossSwitch(oldProgram, resolvedProgram):
		if portErr := PortSessionHistory(ctx, oldProgram, resolvedProgram, i); portErr != nil {
			if errors.Is(portErr, ErrNoHistoryAdapter) {
				// Low-severity: the family-gate above and each adapter's CanHandle
				// have drifted out of sync. Best-effort porting still no-ops safely.
				log.Warn("[SwitchProgram] no history adapter resolved for program pair; skipping history port", "session", i.Title, "old", oldProgram, "new", resolvedProgram)
			} else {
				log.Error("[SwitchProgram] failed to port session history during program switch", "session", i.Title, "old", oldProgram, "new", resolvedProgram, "err", portErr)
			}
		}
	case isClaudeAntigravityFamily(oldProgram) && !isClaudeAntigravityFamily(resolvedProgram):
		// Leaving the Claude/Antigravity family entirely: a stale --resume UUID
		// captured under the old program would fail against the new one.
		i.ClearConversationState()
	}

	if persist != nil {
		if saveErr := persist(); saveErr != nil {
			log.Warn("[SwitchProgram] failed to persist program change before restart", "session", i.Title, "err", saveErr)
		}
	}

	if i.Status == Active {
		if restartErr := i.Restart(true); restartErr != nil {
			return true, resolvedProgram, restartErr
		}
	}

	return true, resolvedProgram, nil
}
