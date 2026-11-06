package session

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/fsevents"
)

// treeEventsRelevant reports whether any event warrants invalidation. .git
// paths are skipped (gitWatchLoop already handles them) unless the event is a
// rescan flag, which means events were lost and anything may have changed.
func treeEventsRelevant(gitDir string, evs []fsevents.Event) bool {
	prefix := gitDir + string(filepath.Separator)
	for _, e := range evs {
		if e.Flags&fsevents.HistoryDone != 0 && !e.Flags.NeedsRescan() {
			continue
		}
		if e.Flags.NeedsRescan() || !strings.HasPrefix(e.Path+string(filepath.Separator), prefix) {
			return true
		}
	}
	return false
}

// trailingDebouncer runs fn once, wait after the first poke of a burst.
// Pokes after ctx is done are dropped.
type trailingDebouncer struct {
	ctx  context.Context
	wait time.Duration
	fn   func()

	mu    sync.Mutex
	armed bool
}

func (t *trailingDebouncer) poke() {
	t.mu.Lock()
	if t.armed || t.ctx.Err() != nil {
		t.mu.Unlock()
		return
	}
	t.armed = true
	t.mu.Unlock()
	time.AfterFunc(t.wait, func() {
		t.mu.Lock()
		t.armed = false
		t.mu.Unlock()
		if t.ctx.Err() == nil {
			t.fn()
		}
	})
}
