package services

import (
	"context"
	"fmt"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/claudehistory"
)

// defaultSearchSyncInterval is how often the background loop re-syncs the
// search index with Claude history on disk.
const defaultSearchSyncInterval = 60 * time.Second

// SearchSyncIntervalEnv sets the background sync interval as a Go duration
// (e.g. "30s", "5m"); unset, invalid or non-positive values keep the default.
const SearchSyncIntervalEnv = "STAPLER_SQUAD_SEARCH_SYNC_INTERVAL"

// SetSyncInterval overrides the background sync interval; call before
// StartSyncLoop. Non-positive values keep the current interval.
func (ss *SearchService) SetSyncInterval(d time.Duration) {
	if d > 0 {
		ss.syncInterval = d
	}
}

// SyncNow reloads history from disk (bypassing the cache TTL) and runs
// IncrementalSync once. Safe to call concurrently with searches.
func (ss *SearchService) SyncNow(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	hist, err := claudehistory.NewClaudeSessionHistoryFromClaudeDir()
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	ss.historySnap.Store(&historySnapshot{cache: hist, at: time.Now()})

	result, err := ss.searchEngine.IncrementalSyncContext(ctx, hist)
	if err != nil {
		return fmt.Errorf("sync search index: %w", err)
	}
	ss.markSynced()
	if result.HasChanges() || result.WasFullRebuild {
		log.Info("search index sync", "result", result.String())
	}
	return nil
}

// StartSyncLoop syncs once immediately and then every sync interval until ctx
// ends. It returns at once; the loop runs in its own goroutine.
func (ss *SearchService) StartSyncLoop(ctx context.Context) {
	if !ss.loopStarted.CompareAndSwap(false, true) {
		return
	}
	interval := ss.syncInterval
	if interval <= 0 {
		interval = searchSyncIntervalFromEnv()
	}
	if interval <= 0 {
		interval = defaultSearchSyncInterval
	}
	done := make(chan struct{})
	ss.loopDone.Store(&done)
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := ss.SyncNow(ctx); err != nil && ctx.Err() == nil {
				log.Warn("background search index sync failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// WaitSyncLoop blocks until the loop goroutine has exited, i.e. after the
// context given to StartSyncLoop ends and any in-flight sync finishes. It
// returns at once if the loop was never started.
func (ss *SearchService) WaitSyncLoop() {
	if done := ss.loopDone.Load(); done != nil {
		<-*done
	}
}

// markSynced records that the index has been synced at least once and wakes
// streaming requests waiting on a cold start.
func (ss *SearchService) markSynced() {
	ss.syncedOnce.Do(func() { close(ss.syncedCh) })
}

// indexSynced reports whether the index has been synced (or loaded from a
// persisted sync) at least once.
func (ss *SearchService) indexSynced() bool {
	select {
	case <-ss.syncedCh:
		return true
	default:
	}
	if ss.searchEngine.GetSyncMetadata() != nil {
		ss.markSynced()
		return true
	}
	return false
}

// unarySyncWait is how long the unary search waits for the first background
// sync before returning CodeUnavailable, so a search right after a restart
// usually succeeds instead of failing.
const unarySyncWait = 5 * time.Second

// requireIndexSynced is the cold-start guard: a search before the first sync
// waits briefly (ss.unarySyncWait) then gets CodeUnavailable instead of
// silently returning nothing.
func (ss *SearchService) requireIndexSynced(ctx context.Context) error {
	if ss.indexSynced() {
		return nil
	}
	if ss.unarySyncWait > 0 {
		timer := time.NewTimer(ss.unarySyncWait)
		defer timer.Stop()
		select {
		case <-ss.syncedCh:
			return nil
		case <-ctx.Done():
		case <-timer.C:
		}
	}
	return connect.NewError(connect.CodeUnavailable,
		fmt.Errorf("search index is warming up (first sync has not completed); retry shortly"))
}

func searchSyncIntervalFromEnv() time.Duration {
	raw := os.Getenv(SearchSyncIntervalEnv)
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Warn("ignoring invalid search sync interval", "env", SearchSyncIntervalEnv, "value", raw)
		return 0
	}
	return d
}
