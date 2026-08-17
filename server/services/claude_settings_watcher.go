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
	// onReload's notify parameter is false only for Start()'s initial priming reload — see
	// that method's doc comment for why a notification there would be redundant or spurious.
	onReload func(rules []classifier.Rule, origin string, notify bool)

	// mu guards lastGood. Reload is reachable concurrently from the fsnotify debounce-timer
	// goroutine (Start's loop) and from the ReloadClaudeSettingsRules RPC handler — without
	// this lock, concurrent map reads/writes on lastGood are a `fatal error: concurrent map
	// read and map write` crash, not just a logic bug. Held for Reload's entire body.
	mu       sync.Mutex
	lastGood map[string][]classifier.Rule

	watcher *fsnotify.Watcher
	stopped chan struct{}

	// watchedFilenames restricts run()'s event handling to the settings files themselves,
	// ignoring other files fsnotify reports in the same watched directory (parent-dir
	// watching is per-directory, not per-file — see Start). Set once in Start before the
	// run goroutine starts; read-only afterward, so no separate lock is needed.
	watchedFilenames map[string]bool
}

// NewClaudeSettingsWatcher creates a watcher for projectDir's claude-settings paths (see
// settingsPaths). onReload is invoked after every reload — auto (fsnotify) or manual
// (Reload called directly) — with the merged rule set, an origin tag ("global", "project",
// or "mixed"; see computeReloadOrigin), and whether this reload is notification-worthy.
func NewClaudeSettingsWatcher(projectDir string, onReload func(rules []classifier.Rule, origin string, notify bool)) *ClaudeSettingsWatcher {
	return &ClaudeSettingsWatcher{
		projectDir: projectDir,
		onReload:   onReload,
		lastGood:   make(map[string][]classifier.Rule),
		stopped:    make(chan struct{}),
	}
}

// Reload re-parses every claude-settings path, preserving last-known-good rules for any path
// with a transient parse error. Safe for concurrent callers (fsnotify loop + RPC handler) —
// see mu. Always notification-worthy; Start's own priming reload uses reloadLocked directly
// to suppress that.
func (w *ClaudeSettingsWatcher) Reload(ctx context.Context) (ruleCount int, failedPaths []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reloadLocked(ctx, true)
}

// reloadLocked is Reload's implementation, callable with an explicit notify flag. Callers
// must hold w.mu.
func (w *ClaudeSettingsWatcher) reloadLocked(ctx context.Context, notify bool) (ruleCount int, failedPaths []string) {
	results := LoadClaudeSettingsRulesDetailed(w.projectDir)

	changedLabels := make(map[string]bool)
	var merged []classifier.Rule
	for _, result := range results {
		if result.Err != nil {
			if result.Err == ErrSettingsNotFound {
				// The file legitimately doesn't exist (never created, or just deleted) — it
				// contributes zero rules. Forget any rules previously loaded from it so a
				// deletion actually revokes them, instead of resurrecting stale rules from
				// lastGood forever. If it previously had rules, that's a real change.
				if len(w.lastGood[result.Path]) > 0 {
					changedLabels[result.Label] = true
				}
				delete(w.lastGood, result.Path)
				continue
			}
			// A genuine parse error (e.g. an editor's mid-autosave truncated write) — unlike a
			// missing file, this must not wipe rules that were already working.
			failedPaths = append(failedPaths, result.Path)
			log.Warn("[ClaudeSettingsWatcher] path failed to parse, using last-known-good", "path", result.Path, "err", result.Err)
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
		w.onReload(merged, origin, notify)
	}
	return len(merged), failedPaths
}

// rulesEqual reports whether two rule slices from the same settings path represent the same
// content. Compares ToolName/CommandPattern/Decision rather than Rule.ID: claudeAllowsToRules
// assigns IDs positionally ("claude-settings-<label>-<index>"), so editing a pattern in place
// (same array index, same slice length) would otherwise go undetected — which would silently
// break computeReloadOrigin's security-visibility tagging for exactly the case it exists to
// catch (a project-level settings edit that changes what gets auto-allowed).
func rulesEqual(a, b []classifier.Rule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		aPattern, bPattern := "", ""
		if a[i].CommandPattern != nil {
			aPattern = a[i].CommandPattern.String()
		}
		if b[i].CommandPattern != nil {
			bPattern = b[i].CommandPattern.String()
		}
		if a[i].ToolName != b[i].ToolName || aPattern != bPattern || a[i].Decision != b[i].Decision {
			return false
		}
	}
	return true
}

// computeReloadOrigin tags a reload "global", "project", or "mixed" from which settingsPaths
// labels changed, so a project-level change (e.g. an unreviewed PR branch) is distinguishable
// from an operator's own global edit.
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

// Start performs a synchronous, non-notifying priming reload (see notify param on
// reloadLocked), then debounces fsnotify events into notification-worthy reloads. Degrades
// gracefully — logs and returns without starting a goroutine if fsnotify is unavailable.
// Exits when ctx is cancelled.
func (w *ClaudeSettingsWatcher) Start(ctx context.Context) {
	w.mu.Lock()
	w.reloadLocked(ctx, false)
	w.mu.Unlock()

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn("[ClaudeSettingsWatcher] fsnotify unavailable, watcher disabled", "err", err)
		close(w.stopped)
		return
	}
	w.watcher = fsw

	// Watch each settings file's parent directory (not the file itself) — an editor's
	// atomic-rename save changes the file's inode, which would silently stop a
	// file-level fsnotify watch from firing on the next save. fsnotify reports every event
	// in the watched directory, not just our files, so run() filters by watchedFilenames.
	watchedDirs := make(map[string]bool)
	w.watchedFilenames = make(map[string]bool)
	for _, p := range settingsPaths(w.projectDir) {
		dir := filepath.Dir(p.path)
		w.watchedFilenames[filepath.Base(p.path)] = true
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
			if !w.watchedFilenames[filepath.Base(event.Name)] {
				continue
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
