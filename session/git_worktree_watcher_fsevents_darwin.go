//go:build darwin

package session

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/fsevents"
	"github.com/tstapler/stapler-squad/log"
)

// treeWatchDebounce trailing-coalesces working-tree event bursts (a build can
// emit thousands) into one fire(); the periodic fingerprint stays authoritative.
const treeWatchDebounce = 250 * time.Millisecond

// startTreeWatch adds an FSEvents stream over the worktree so plain edits that
// never touch .git invalidate caches immediately instead of at the next
// periodic fingerprint tick. Best-effort: on any failure the periodic loop
// carries on alone, as before.
func (d *WorktreeChangeDetector) startTreeWatch(ctx context.Context, wg *sync.WaitGroup) {
	real, err := filepath.EvalSymlinks(d.worktreePath)
	if err != nil {
		log.Debug("worktree change detector: FSEvents watch skipped", "worktreePath", d.worktreePath, "err", err)
		return
	}
	gitDir := filepath.Join(real, ".git")
	deb := &trailingDebouncer{ctx: ctx, wait: treeWatchDebounce, fn: d.fire}
	stream, err := fsevents.NewStream(fsevents.Config{
		Paths:     []string{real},
		SinceWhen: fsevents.SinceNow,
		OnEvents: func(evs []fsevents.Event) {
			if treeEventsRelevant(gitDir, evs) {
				deb.poke()
			}
		},
	})
	if err == nil {
		err = stream.Start()
	}
	if err != nil {
		log.Warn("worktree change detector: FSEvents watch failed, periodic-check-only for working tree", "worktreePath", d.worktreePath, "err", err)
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		stream.Stop()
	}()
}
