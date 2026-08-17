package services

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// claudeSettingsWatchDebounce coalesces bursts of fsnotify events one editor save produces
// (temp file write, rename into place, etc.) into a single reload, mirroring
// session/unfinished/gogitstore/mmapwatch.go's packWatchDebounce.
const claudeSettingsWatchDebounce = 250 * time.Millisecond

// ClaudeSettingsWatcher watches Claude settings files (global + project-level) for changes
// and reloads their derived rules on edit, preserving last-known-good rules per path when a
// reload finds a malformed file. Constructed once per server process inside NewSessionService
// (see settingsPaths for which paths are watched); callback-driven rather than DI-injected,
// mirroring session.HistoryFileWatcher's shape.
type ClaudeSettingsWatcher struct {
	projectDir string
	onReload   func(rules []classifier.Rule, origin string)

	// mu guards lastGood. Reload is reachable concurrently from the fsnotify debounce-timer
	// goroutine (Start's loop) and from the ReloadClaudeSettingsRules RPC handler — without
	// this lock, concurrent map reads/writes on lastGood are a `fatal error: concurrent map
	// read and map write` crash, not just a logic bug. Held for Reload's entire body.
	mu       sync.Mutex
	lastGood map[string][]classifier.Rule

	watcher *fsnotify.Watcher
	stopped chan struct{}
}

// NewClaudeSettingsWatcher creates a watcher for projectDir's claude-settings paths (see
// settingsPaths). onReload is invoked after every reload — auto (fsnotify) or manual
// (Reload called directly) — with the merged rule set and an origin tag ("global",
// "project", or "mixed"; see computeReloadOrigin).
func NewClaudeSettingsWatcher(projectDir string, onReload func(rules []classifier.Rule, origin string)) *ClaudeSettingsWatcher {
	return &ClaudeSettingsWatcher{
		projectDir: projectDir,
		onReload:   onReload,
		lastGood:   make(map[string][]classifier.Rule),
		stopped:    make(chan struct{}),
	}
}

// Reload re-parses every claude-settings path and merges the results, preserving
// last-known-good rules for any path that fails to parse (a transient parse error — e.g. an
// editor's mid-autosave truncated write — must never wipe rules that were already working).
// Safe to call concurrently: the whole body runs under w.mu, so a debounced fsnotify-driven
// reload and a manual RPC-driven reload can never interleave their reads/writes of lastGood.
func (w *ClaudeSettingsWatcher) Reload(ctx context.Context) (ruleCount int, failedPaths []string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	results := LoadClaudeSettingsRulesDetailed(w.projectDir)

	changedLabels := make(map[string]bool)
	var merged []classifier.Rule
	for _, result := range results {
		if result.Err != nil {
			// A missing file (most machines have no settings.local.json) is normal, not a
			// failure — only a genuine parse error counts toward failedPaths/last-known-good.
			if result.Err != ErrSettingsNotFound {
				failedPaths = append(failedPaths, result.Path)
				log.Warn("[ClaudeSettingsWatcher] path failed to parse, using last-known-good", "path", result.Path, "err", result.Err)
			}
			merged = append(merged, w.lastGood[result.Path]...)
			continue
		}
		if !rulesEqual(w.lastGood[result.Path], result.Rules) {
			changedLabels[result.Label] = true
		}
		w.lastGood[result.Path] = result.Rules
		merged = append(merged, result.Rules...)
	}

	origin := computeReloadOrigin(changedLabels)
	if w.onReload != nil {
		w.onReload(merged, origin)
	}
	return len(merged), failedPaths
}

// rulesEqual reports whether two rule slices came from an identical settings source, using
// each rule's ID (unique per pattern within a path — see claudeAllowsToRules) plus its
// Decision, since a changed permission on the same pattern must still count as a change.
func rulesEqual(a, b []classifier.Rule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Decision != b[i].Decision {
			return false
		}
	}
	return true
}

// computeReloadOrigin tags a reload as "global", "project", or "mixed" based on which
// settings-path labels actually changed, for security visibility: a project-level settings
// change (e.g. from checking out an unreviewed PR branch) is distinguishable from the
// operator's own global edit. Labels are those settingsPaths assigns ("global",
// "global-local", "project", "project-local"); anything containing "project" counts as a
// project-origin change.
func computeReloadOrigin(changedLabels map[string]bool) string {
	sawGlobal, sawProject := false, false
	for label := range changedLabels {
		if strings.Contains(label, "project") {
			sawProject = true
		} else {
			sawGlobal = true
		}
	}
	switch {
	case sawGlobal && sawProject:
		return "mixed"
	case sawProject:
		return "project"
	default:
		return "global"
	}
}

// Start begins watching this watcher's claude-settings paths for changes. It performs an
// initial synchronous Reload before entering the event loop, then debounces bursts of
// fsnotify events into single reloads. Graceful degradation: if fsnotify is unavailable on
// this platform, it logs a warning and returns without starting a goroutine — never blocks
// startup. Exits when ctx is cancelled.
func (w *ClaudeSettingsWatcher) Start(ctx context.Context) {
	w.Reload(ctx)

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn("[ClaudeSettingsWatcher] fsnotify unavailable, watcher disabled", "err", err)
		close(w.stopped)
		return
	}
	w.watcher = fsw

	// Watch each settings file's parent directory (not the file itself) — an editor's
	// atomic-rename save changes the file's inode, which would silently stop a
	// file-level fsnotify watch from firing on the next save.
	watchedDirs := make(map[string]bool)
	for _, p := range settingsPaths(w.projectDir) {
		dir := filepath.Dir(p.path)
		if watchedDirs[dir] {
			continue
		}
		if err := fsw.Add(dir); err != nil {
			log.Debug("[ClaudeSettingsWatcher] could not watch settings dir (may not exist yet)", "dir", dir, "err", err)
			continue
		}
		watchedDirs[dir] = true
	}

	go w.run(ctx)
}

func (w *ClaudeSettingsWatcher) run(ctx context.Context) {
	defer close(w.stopped)
	defer func() { _ = w.watcher.Close() }()

	var timer *time.Timer
	var timerC <-chan time.Time
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(claudeSettingsWatchDebounce)
			} else {
				timer.Reset(claudeSettingsWatchDebounce)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			w.Reload(ctx)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Warn("[ClaudeSettingsWatcher] fsnotify error", "err", err)
		}
	}
}

// Stopped returns a channel that is closed when the watcher goroutine has exited (or
// immediately, if Start degraded gracefully without starting one). Matches
// session.HistoryFileWatcher.Stopped's convention.
func (w *ClaudeSettingsWatcher) Stopped() <-chan struct{} {
	return w.stopped
}
