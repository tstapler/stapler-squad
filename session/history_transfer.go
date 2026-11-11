package session

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/log"
)

// ErrNoHistoryAdapter indicates PortSessionHistory was asked to port history for a program
// pair where no registered HistoryAdapter claims one (or both) sides — e.g. opencode, aider,
// bash, or gemini (the real Gemini CLI, distinct from Antigravity's own storage format — see
// AgyAdapter.CanHandle), none of which have a canonical history format to port. Callers should
// treat this as an expected, low-severity no-op (log and continue), not a hard failure.
var ErrNoHistoryAdapter = errors.New("no history adapter resolves for this program")

// PortSessionHistory translates and syncs history between Claude Code and Antigravity CLI.
func PortSessionHistory(ctx context.Context, oldProgram, newProgram string, i *Instance) error {
	srcAdapter := i.historyRegistry().Resolve(oldProgram)
	dstAdapter := i.historyRegistry().Resolve(newProgram)

	if srcAdapter == nil || dstAdapter == nil {
		return ErrNoHistoryAdapter
	}
	if srcAdapter.Name() == dstAdapter.Name() {
		return nil
	}

	// 1. Import turns to canonical format from source adapter
	turns, err := srcAdapter.Import(ctx, i)
	if err != nil {
		return fmt.Errorf("failed to import session history from %s: %w", srcAdapter.Name(), err)
	}

	// 2. Apply Stapler Squad Inhibition Engine to redact secrets
	inhibition := NewInhibitionEngine()
	sanitizedTurns := inhibition.SanitizeTurns(turns)

	// 3. Export sanitized turns in target adapter format
	if err := dstAdapter.Export(ctx, sanitizedTurns, i); err != nil {
		return fmt.Errorf("failed to export session history to %s: %w", dstAdapter.Name(), err)
	}

	// 4. Run the target adapter's post-switch bookkeeping (history.jsonl, session data)
	if _, err := os.UserHomeDir(); err != nil {
		return err
	}
	if pe, ok := dstAdapter.(PostExporter); ok {
		if err := pe.AfterExport(ctx, i); err != nil {
			return err
		}
	}

	log.Info("PortSessionHistory: ported session history using canonical formats", "from", srcAdapter.Name(), "to", dstAdapter.Name(), "session", i.Title)
	return nil
}

// Shim functions to keep backward compatibility with existing tests and server code.
func portClaudeToAgy(i *Instance) error {
	return PortSessionHistory(context.Background(), "claude", "agy", i)
}
