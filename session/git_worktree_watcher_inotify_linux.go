//go:build linux

package session

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/fsevents"
	"github.com/tstapler/stapler-squad/internal/treewatch"
	"github.com/tstapler/stapler-squad/log"
)

// treeWatchDebounce trailing-coalesces working-tree event bursts into one
// fire(); the periodic fingerprint stays authoritative.
const treeWatchDebounce = 250 * time.Millisecond

// startTreeWatch adds a recursive inotify watch over the worktree so plain
// edits that never touch .git invalidate caches immediately. Best-effort: if
// the tree exceeds the inotify limit or the watch fails, the periodic loop
// carries on alone, as before.
func (d *WorktreeChangeDetector) startTreeWatch(ctx context.Context, wg *sync.WaitGroup) {
	gitDir := filepath.Join(d.worktreePath, ".git")
	deb := &trailingDebouncer{ctx: ctx, wait: treeWatchDebounce, fn: d.fire}
	tw, err := treewatch.Start(treewatch.Config{
		Root: d.worktreePath,
		OnEvents: func(evs []fsevents.Event) {
			if treeEventsRelevant(gitDir, evs) {
				deb.poke()
			}
		},
	})
	if err != nil {
		log.Warn("worktree change detector: inotify tree watch failed, periodic-check-only for working tree", "worktreePath", d.worktreePath, "err", err)
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		tw.Stop()
	}()
}
