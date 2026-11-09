package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/internal/dirwatch"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/scanqueue"
)

// HistoryFileWatcher watches ~/.claude/projects/ for new JSONL files.
//
// The event loop only classifies events and enqueues paths; callback runs on a
// single scanqueue worker, so a slow callback (a HistoryLinker pass) never stops
// fsnotify events from draining. One worker keeps callbacks serialized, as they
// were when run inline. Events for a path already queued or running coalesce
// into one (newest wins), and callers re-read the file, so nothing is lost.
type HistoryFileWatcher struct {
	watchDir string
	callback func(filePath string)
	watcher  dirwatch.Watcher
	stopped  chan struct{}
	queue    *scanqueue.Queue
	cancel   context.CancelFunc

	// mu guards the callback accounting that lets shutdown wait for an
	// in-flight callback and refuse to start another one after it.
	mu           sync.Mutex
	closed       bool
	running      int
	drained      chan struct{} // closed once closed && running == 0
	drainOnce    sync.Once
	drainTimeout time.Duration
}

// defaultHistoryDrainTimeout bounds how long shutdown waits for a running callback.
const defaultHistoryDrainTimeout = 30 * time.Second

// NewHistoryFileWatcher creates a watcher for the given directory.
// If watchDir is empty, defaults to ~/.claude/projects/.
func NewHistoryFileWatcher(watchDir string, callback func(filePath string)) *HistoryFileWatcher {
	if watchDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			watchDir = filepath.Join(homeDir, ".claude", "projects")
		}
	}
	w := &HistoryFileWatcher{
		watchDir: watchDir,
		callback: callback,
		stopped:  make(chan struct{}),
		drained:  make(chan struct{}),

		drainTimeout: defaultHistoryDrainTimeout,
	}
	w.queue = scanqueue.New(scanqueue.Config{Workers: 1}, w.runCallback)
	return w
}

// runCallback invokes the callback unless shutdown has begun, and tracks it so
// run can wait for it. A callback is never interrupted (it takes no ctx).
func (w *HistoryFileWatcher) runCallback(path string) {
	w.mu.Lock()
	if w.closed || w.callback == nil {
		w.mu.Unlock()
		return
	}
	w.running++
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.running--
		done := w.closed && w.running == 0
		w.mu.Unlock()
		if done {
			w.drainOnce.Do(func() { close(w.drained) })
		}
	}()
	w.callback(path)
}

// closeAndDrain stops new callbacks, then waits up to drainTimeout for a running
// one, so HistoryLinker.Wait returns only after its writes are done.
func (w *HistoryFileWatcher) closeAndDrain() {
	w.mu.Lock()
	w.closed = true
	idle := w.running == 0
	w.mu.Unlock()
	if idle {
		w.drainOnce.Do(func() { close(w.drained) })
		return
	}
	t := time.NewTimer(w.drainTimeout)
	defer t.Stop()
	select {
	case <-w.drained:
	case <-t.C:
		log.Warn("history file watcher: callback still running at shutdown", "timeout", w.drainTimeout)
	}
}

// Start begins watching the directory. It returns without error even if the
// directory does not exist (degraded mode — polling fallback still works).
func (w *HistoryFileWatcher) Start(ctx context.Context) error {
	if _, err := os.Stat(w.watchDir); os.IsNotExist(err) {
		log.Warn("history file watcher: watch directory does not exist", "path", w.watchDir)
		close(w.stopped)
		return nil
	}

	watcher, err := dirwatch.New(dirwatch.WithName("history"), dirwatch.RootAt(w.watchDir))
	if err != nil {
		close(w.stopped)
		return err
	}
	w.watcher = watcher

	// RootAt: one FSEvents stream covers every project dir (a stream per dir would
	// exhaust the ~512 per-process limit at a few hundred projects).
	if err := watcher.Add(w.watchDir); err != nil {
		_ = watcher.Close()
		close(w.stopped)
		return err
	}

	// Also watch any existing subdirectories so we catch files created
	// inside project-specific subdirectories.
	_ = filepath.WalkDir(w.watchDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() || path == w.watchDir {
			return nil
		}
		if w.skipHistoryDir(path) {
			return filepath.SkipDir
		}
		_ = watcher.Add(path)
		return nil
	})

	ctx, w.cancel = context.WithCancel(ctx)
	w.queue.Start(ctx)
	go w.run(ctx)
	startedAt := time.Now()
	// An FSEvents stream can deliver late or lose a write right after starting, with no
	// error: re-enqueue transcripts modified since the watch started, once, shortly after.
	time.AfterFunc(historyCatchUpDelay, func() {
		if ctx.Err() == nil {
			w.enqueueModifiedSince(startedAt.Add(-time.Minute))
		}
	})
	return nil
}

// Stopped returns a channel that is closed when the watcher goroutine has exited
// and any in-flight callback has finished (or drainTimeout elapsed).
func (w *HistoryFileWatcher) Stopped() <-chan struct{} {
	return w.stopped
}

func (w *HistoryFileWatcher) run(ctx context.Context) {
	defer close(w.stopped) // runs last: Stopped() means no callback is running
	defer w.closeAndDrain()
	defer w.cancel() // stops the queue workers
	defer func() {
		if w.watcher != nil {
			_ = w.watcher.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events():
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors():
			if !ok {
				return
			}
			log.Warn("history file watcher error", "err", err)
		}
	}
}

func (w *HistoryFileWatcher) handleEvent(event fsnotify.Event) {
	// A newly created subdirectory (e.g. a fresh git worktree's own
	// ~/.claude/projects/<encoded-path>/ directory) is not recursed into by
	// fsnotify automatically — watch it explicitly or every .jsonl written
	// inside it goes unobserved for the life of the process.
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			w.watchNewDir(event.Name)
			return
		}
	}

	// Care about CREATE, RENAME, and WRITE events (WRITE fires as JSONL is appended).
	if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Write) == 0 {
		return
	}

	path := event.Name

	// Must end in .jsonl
	if !strings.HasSuffix(path, ".jsonl") {
		return
	}

	// Skip agent files
	base := filepath.Base(path)
	if strings.HasPrefix(base, "agent-") {
		return
	}

	w.queue.Enqueue(path)
}

// watchNewDir adds a newly created subdirectory to the fsnotify watch list
// and enqueues any .jsonl files already present in it. The directory can
// arrive non-empty if Claude Code writes its transcript before this handler
// gets to process the CREATE event for the directory itself.
func (w *HistoryFileWatcher) watchNewDir(dir string) {
	if w.skipHistoryDir(dir) {
		return
	}
	if err := w.watcher.Add(dir); err != nil {
		log.Warn("history file watcher: failed to watch new directory", "path", dir, "err", err)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") || strings.HasPrefix(name, "agent-") {
			continue
		}
		w.queue.Enqueue(filepath.Join(dir, name))
	}
}

// historyCatchUpDelay is the one-shot safety-net rescan after the watch starts.
const historyCatchUpDelay = 15 * time.Second

// enqueueModifiedSince enqueues every project-level transcript written after since.
func (w *HistoryFileWatcher) enqueueModifiedSince(since time.Time) {
	projects, err := os.ReadDir(w.watchDir)
	if err != nil {
		return
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(w.watchDir, p.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if f.IsDir() || !strings.HasSuffix(name, ".jsonl") || strings.HasPrefix(name, "agent-") {
				continue
			}
			if info, err := f.Info(); err == nil && info.ModTime().After(since) {
				w.queue.Enqueue(filepath.Join(dir, name))
			}
		}
	}
}

// skipHistoryDir reports whether dir holds only files this watcher ignores. Claude Code
// writes transcripts as <project>/<session>.jsonl; everything below a project dir
// (<session>/subagents, <session>/tool-results, ...) is never consumed. On kqueue each
// file in a watched dir costs an fd: subagents alone cost ~8,000 and tool-results
// ~1,200 on a long-lived instance, so only the project dirs themselves are watched.
func (w *HistoryFileWatcher) skipHistoryDir(dir string) bool {
	return filepath.Dir(dir) != filepath.Clean(w.watchDir)
}

// Stop closes the watcher.
func (w *HistoryFileWatcher) Stop() {
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
	if w.cancel != nil {
		w.cancel()
	}
}
