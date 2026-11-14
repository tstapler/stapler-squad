package session

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/instance"
)

// PortSessionHistory translates and syncs history between Claude Code and Antigravity CLI.
func PortSessionHistory(ctx context.Context, oldProgram, newProgram string, i *instance.Instance) error {
	srcAdapter := i.HistoryRegistry().Resolve(oldProgram)
	dstAdapter := i.HistoryRegistry().Resolve(newProgram)

	if srcAdapter == nil || dstAdapter == nil {
		return instance.ErrNoHistoryAdapter
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
	if pe, ok := dstAdapter.(PostExporter); ok {
		if err := pe.AfterExport(ctx, i); err != nil {
			return err
		}
	}

	log.Info("PortSessionHistory: ported session history using canonical formats", "from", srcAdapter.Name(), "to", dstAdapter.Name(), "session", i.Title)
	return nil
}

// Shim functions to keep backward compatibility with existing tests and server code.
func portClaudeToAgy(i *instance.Instance) error {
	return PortSessionHistory(context.Background(), "claude", "agy", i)
}

func init() { instance.HistoryPorterHook = PortSessionHistory }
