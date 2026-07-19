// mmapwatch.go wires SharedObjectStore.refreshIndexes (store.go) to an
// fsnotify watch on a commondir's objects/pack directory, so a repack is
// picked up promptly instead of only ever being noticed the next time
// Registry.Prune recycles the whole store (see design doc §5.3's staleness-
// detection requirement).
//
// This watcher is deliberately self-contained inside gogitstore rather than
// wired through session/unfinished/scanner.go's existing fsnotifyLoop (which
// watches worktree .git dirs non-recursively — objects/pack is a nested
// subdirectory fsnotify would not report events for without its own watch —
// and drives the Scanner's own, unrelated cache-invalidation concern). Per
// this stage's scope, that's an acceptable trade — one extra fsnotify
// watcher per mmap-enabled commondir — in exchange for keeping stage 2 code
// entirely inside this package rather than reaching back into scanner.go's
// production logic.
package gogitstore

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// packWatchDebounce coalesces bursts of pack-dir events — a single repack
// typically touches several files in quick succession (temp pack, temp idx,
// rename each into place, unlink the old pair) — into one refreshIndexes
// call instead of one per raw event.
const packWatchDebounce = 200 * time.Millisecond

// startPackWatchLocked starts (at most once per store) a background
// goroutine watching this store's objects/pack directory, calling
// refreshIndexes on activity. Callers MUST hold s.mu (called from
// ensureIndex, which already does). Only meaningful, and only ever called,
// when s.useMmapIndex is true.
//
// Best-effort: if fsnotify.NewWatcher or Add fails (platform without
// inotify/kqueue support, or the objects/pack dir doesn't exist yet — e.g. a
// brand new repo with no packs), this silently no-ops and staleness falls
// back to Registry.Prune's TTL-driven store recreation, exactly like the
// copy-based loader's existing (coarser) staleness story.
func (s *SharedObjectStore) startPackWatchLocked() {
	if s.packWatchStarted {
		return
	}
	s.packWatchStarted = true

	packDir := filepath.Join(s.commonDirAbs, "objects", "pack")
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn("gogitstore: fsnotify unavailable, mmap index staleness will only be caught by Registry TTL eviction", "err", err)
		return
	}
	if err := w.Add(packDir); err != nil {
		log.Debug("gogitstore: could not watch pack dir (may not exist yet)", "dir", packDir, "err", err)
		_ = w.Close()
		return
	}

	s.packWatchStop = make(chan struct{})
	go s.packWatchLoop(w, s.packWatchStop)
}

// packWatchLoop debounces bursts of pack-dir fsnotify events into a single
// refreshIndexes call per quiet period, exiting cleanly when stop is closed
// (mirrors session/unfinished/scanner.go's fsnotifyLoop goroutine-exit
// convention, though this loop is otherwise independent of it — see package
// doc above).
func (s *SharedObjectStore) packWatchLoop(w *fsnotify.Watcher, stop chan struct{}) {
	defer func() { _ = w.Close() }()

	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-stop:
			return
		case _, ok := <-w.Events:
			if !ok {
				return
			}
			if timer == nil {
				timer = time.NewTimer(packWatchDebounce)
			} else {
				timer.Reset(packWatchDebounce)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			if err := s.refreshIndexes(); err != nil {
				log.Warn("gogitstore: refreshIndexes failed", "commonDir", s.commonDirAbs, "err", err)
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// stopPackWatch stops this store's pack-dir watcher (if one was started)
// and force-releases every remaining mmap handle's mapping, regardless of
// whether it is currently marked retiring. Called exactly once, from
// Registry's eviction path (registry.go's pruneLocked), only after the
// store's refCount has already been confirmed zero (see Prune's doc
// comment) — meaning no live WorktreeStorer should be able to produce new
// pins on this store from this point on.
//
// A handle whose pins is still > 0 at this point would mean some caller
// obtained an Entries() iterator (index.go's pinnedEntryIter) and never
// called Close() on it before its WorktreeStorer was released — a caller
// bug this package cannot fully protect against without blocking eviction
// indefinitely. Rather than either (a) unmapping anyway, risking a genuine
// use-after-munmap for that leaked iterator's next Next() call, or (b)
// blocking Prune (defeating the whole "be responsive to memory pressure"
// point of this eviction path), this logs a warning and skips unmapping
// ONLY that one handle — a bounded, rare leak instead of a crash. This
// mirrors the same "never evict/unmap while something might still be
// reading" principle Registry.Prune already applies at the whole-store
// granularity (never evicting a store with RefCount() > 0), just applied
// one level deeper, at per-pack-mapping granularity.
func (s *SharedObjectStore) stopPackWatch() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.packWatchStop != nil {
		close(s.packWatchStop)
		s.packWatchStop = nil
	}

	for h, li := range s.index {
		if li.handle == nil {
			continue
		}
		if li.handle.pins != 0 {
			log.Warn("gogitstore: leaking mmap index mapping at store eviction — pins still held (leaked Entries() iterator?)", "pack", h.String(), "pins", li.handle.pins)
			continue
		}
		li.handle.retiring = true
		li.handle.maybeUnmapLocked()
	}
}
