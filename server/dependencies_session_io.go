package server

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/external"
	"github.com/tstapler/stapler-squad/session/importer"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// pausedTmuxReapInterval is how often the safety-net reaper runs.
const pausedTmuxReapInterval = 30 * time.Minute

// sessionIOWiring is the output of buildSessionIO: the session-I/O helpers that
// need no inputs beyond the loaded instance count (logging only).
type sessionIOWiring struct {
	historyLinker       *importer.HistoryLinker
	scrollbackManager   *scrollback.ScrollbackManager
	tmuxStreamerManager *external.ExternalTmuxStreamerManager
}

// buildSessionIO builds the HistoryLinker (links Claude JSONL conversation UUIDs
// to sessions so cold restore can use --resume), the ScrollbackManager and the
// external tmux streamer manager. No goroutines, hooks or globals.
func buildSessionIO(instanceCount int) sessionIOWiring {
	historyLinker := importer.NewHistoryLinkerFromRealInspector()
	log.Info("HistoryLinker initialized", "instances", instanceCount)

	homeDir, _ := os.UserHomeDir()
	scrollbackPath := filepath.Join(homeDir, ".stapler-squad", "sessions")
	scrollbackConfig := scrollback.DefaultScrollbackConfig()
	scrollbackConfig.StoragePath = scrollbackPath
	scrollbackManager := scrollback.NewScrollbackManager(scrollbackConfig)
	log.Info("initialized ScrollbackManager", "path", scrollbackPath, "compression", scrollbackConfig.StoragePath, "maxLines", scrollbackConfig.MaxLines)

	return sessionIOWiring{
		historyLinker:       historyLinker,
		scrollbackManager:   scrollbackManager,
		tmuxStreamerManager: external.NewExternalTmuxStreamerManager(),
	}
}

// backfillStuckStates seeds durable BacklogStuckState rows (notified_at pre-set)
// for items already stuck, so the first reconcile tick does not re-notify for
// conditions known before this restart. It must run before the reconcile ticker
// starts, and is gated on the backlog feature flag directly because the
// BacklogController does not exist yet at this point.
func backfillStuckStates(backlogEnabled bool, backfill func(context.Context)) {
	if backlogEnabled {
		backfill(context.Background())
	}
}

// startPausedTmuxReaper starts a process-lifetime goroutine that calls reap every
// interval: a safety net for sessions paused before kill-on-pause existed, or
// where the initial kill fell back to detach.
func startPausedTmuxReaper(interval time.Duration, reap func()) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			reap()
		}
	}()
}
