package detection

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/tstapler/stapler-squad/log"
)

// pluginReloadDebounce is how long PluginWatcher waits, after the most
// recent qualifying fsnotify event, before rebuilding the detector snapshot.
// Editors commonly emit several events for one logical save (write-temp,
// rename, chmod, ...); debouncing collapses a burst into a single reload.
const pluginReloadDebounce = 200 * time.Millisecond

// pluginRescanInterval is the periodic safety-net rescan interval, run
// independent of fsnotify. It covers environments where fsnotify doesn't
// fire reliably (some network mounts/containers) and is the only reload
// mechanism when fsnotify is unavailable entirely.
const pluginRescanInterval = 60 * time.Second

// PluginWatcher watches a detector plugin directory for changes and
// hot-reloads the active detector snapshot via rebuildSnapshot. It always
// watches the directory itself, never individual plugin files: editors
// typically save via write-temp-then-rename, which silently stops firing
// events on a per-file fsnotify watch after the first save — a well-known
// fsnotify caveat (see session/unfinished/watcher.go for the same directory-
// level pattern applied to .git dirs).
type PluginWatcher struct {
	dir     string
	watcher *fsnotify.Watcher // nil when fsnotify is unavailable (periodic-rescan-only mode)
	stopped chan struct{}
}

// Stopped returns a channel that is closed once the watcher's goroutine has
// exited (context cancellation or a closed Events channel).
func (w *PluginWatcher) Stopped() <-chan struct{} {
	return w.stopped
}

// pluginDirFingerprint returns a cheap, comparable fingerprint of dir's
// *.toml entries (name, size, modification time) without opening or reading
// any file's content. The periodic safety-net rescan (pluginRescanInterval)
// uses this to skip a full rebuildSnapshot — and therefore every plugin
// file's regex recompilation — when nothing has changed since the last
// rebuild: fsnotify already handles the fast path, so the ticker only needs
// to catch genuinely missed events, not unconditionally reload on every tick
// (project_plans/detector-plugins/research/pitfalls.md's "diff directory
// listing against loaded plugin set" recommendation).
//
// A single os.Stat on the directory's own mtime would miss an in-place edit
// to an existing file's content (directory mtime only changes when entries
// are added/removed/renamed, not when a file already inside it is
// overwritten) — this is exactly the case a missed fsnotify Write event
// needs the safety net for, so this fingerprint reads the full entry list
// instead. Still just one os.ReadDir syscall, no file content is read and no
// regex is compiled, so it costs nothing close to a rebuild even at
// maxPluginFiles.
func pluginDirFingerprint(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".toml" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			// Vanished between ReadDir and Info (e.g. deleted mid-scan) — skip
			// it for fingerprint purposes; LoadPluginDir's own Lstat/Stat calls
			// handle this same race during the real load.
			continue
		}
		fmt.Fprintf(&sb, "%s:%d:%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return sb.String(), nil
}

// StartPluginWatcher begins watching dir for detector plugin changes and
// returns immediately; the watch loop runs in its own goroutine and stops
// when ctx is cancelled. fsnotify being unavailable (NewWatcher or Add
// failing) is never fatal — StartPluginWatcher falls back to the periodic
// rescan alone and logs a warning, mirroring the fallback pattern in
// session/unfinished/watcher.go's NewWatchDirWatcher. The error return
// exists for API stability/future-proofing; every path today returns nil.
func StartPluginWatcher(ctx context.Context, dir string) (*PluginWatcher, error) {
	w := &PluginWatcher{
		dir:     dir,
		stopped: make(chan struct{}),
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn("detector plugin watcher: fsnotify unavailable, falling back to periodic rescan only", "err", err)
	} else if err := fw.Add(dir); err != nil {
		log.Warn("detector plugin watcher: failed to watch directory, falling back to periodic rescan only", "dir", dir, "err", err)
		_ = fw.Close()
	} else {
		w.watcher = fw
	}

	go w.watchLoop(ctx)

	return w, nil
}

// watchLoop is the single goroutine that owns w.watcher's Events/Errors
// channels plus the debounce and rescan timers — nothing else may read from
// w.watcher concurrently. It exits, closing w.stopped, when ctx is cancelled
// or (if a real fsnotify watcher is in use) its Events channel closes.
func (w *PluginWatcher) watchLoop(ctx context.Context) {
	defer close(w.stopped)
	defer func() {
		if w.watcher != nil {
			_ = w.watcher.Close()
		}
	}()

	// Start the debounce timer stopped-and-drained; it is armed via Reset
	// only once a qualifying event arrives. This is the standard idiom for
	// "create a timer that doesn't fire until first use" — a nil timer would
	// need its own nil-guard in the select below, same as w.watcher's
	// channels do via the nil `events`/`errs` variables.
	debounce := time.NewTimer(pluginReloadDebounce)
	if !debounce.Stop() {
		<-debounce.C
	}
	defer debounce.Stop()

	ticker := time.NewTicker(pluginRescanInterval)
	defer ticker.Stop()

	// events/errs stay nil (never fire in the select) when w.watcher is nil,
	// i.e. the fsnotify-unavailable fallback: the watch loop then runs on
	// pluginRescanInterval alone.
	var events <-chan fsnotify.Event
	var errs <-chan error
	if w.watcher != nil {
		events = w.watcher.Events
		errs = w.watcher.Errors
	}

	// lastFingerprint tracks the plugin directory's *.toml listing as of the
	// most recent rebuild, so the periodic safety-net tick (below) can skip a
	// full rebuildSnapshot when nothing has changed. A read error here just
	// means the first tick always attempts a rebuild — the same behavior as
	// before this fingerprint check existed — and rebuildSnapshot's own error
	// handling takes it from there.
	lastFingerprint, _ := pluginDirFingerprint(w.dir)

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if !event.Has(fsnotify.Write | fsnotify.Create | fsnotify.Remove | fsnotify.Rename | fsnotify.Chmod) {
				continue
			}
			if filepath.Ext(event.Name) != ".toml" {
				continue
			}
			if !debounce.Stop() {
				select {
				case <-debounce.C:
				default:
				}
			}
			debounce.Reset(pluginReloadDebounce)

		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			log.Warn("detector plugin watcher: fsnotify error", "err", err)

		case <-debounce.C:
			_ = rebuildSnapshot(ctx, w.dir)
			lastFingerprint, _ = pluginDirFingerprint(w.dir)

		case <-ticker.C:
			// Safety-net tick: only pay for a full rebuild (and every plugin
			// file's regex recompilation) when the directory listing actually
			// changed since the last rebuild — fsnotify already handles the
			// fast path, this ticker exists purely to catch missed events.
			if fp, err := pluginDirFingerprint(w.dir); err == nil && fp == lastFingerprint {
				continue
			}
			_ = rebuildSnapshot(ctx, w.dir)
			lastFingerprint, _ = pluginDirFingerprint(w.dir)
		}
	}
}
