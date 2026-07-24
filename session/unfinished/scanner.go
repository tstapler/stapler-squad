// Package unfinished provides background scanning for git worktrees that have
// uncommitted changes, commits ahead of the default branch, or commits behind.
package unfinished

import "github.com/linkdata/deadlock"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
)

// ScanResultStatus describes the quality of a scan result.
type ScanResultStatus int

const (
	ScanResultStatusOK         ScanResultStatus = 0
	ScanResultStatusTimeout    ScanResultStatus = 1
	ScanResultStatusPermission ScanResultStatus = 2
	ScanResultStatusError      ScanResultStatus = 3
)

// ScanResult holds the complete unfinished-work state for a single git worktree.
type ScanResult struct {
	RepoPath     string
	Branch       string
	WorktreePath string
	RepoName     string
	DisplayPath  string

	HasUncommitted bool
	AheadCount     int
	BehindCount    int
	DefaultBranch  string

	ChangedFiles  int
	LinesAdded    int
	LinesRemoved  int
	AheadMessages []string

	LastModified time.Time
	ScanTime     time.Time

	Status   ScanResultStatus
	ErrorMsg string

	// SessionIDs holds the UUIDs of all active stapler-squad sessions whose Path
	// matches this worktree. Multiple sessions can target the same worktree.
	SessionIDs []string
}

// IsUnfinished returns true when at least one unfinished-work criterion is met.
func (r ScanResult) IsUnfinished() bool {
	return r.HasUncommitted || r.AheadCount > 0 || r.BehindCount > 0
}

// SortByLastModified sorts a slice of ScanResult descending by LastModified.
// Equal times are broken by RepoPath+Branch for stability.
func SortByLastModified(results []ScanResult) {
	sort.Slice(results, func(i, j int) bool {
		ti, tj := results[i].LastModified, results[j].LastModified
		if ti.Equal(tj) {
			ki := results[i].RepoPath + "|" + results[i].Branch
			kj := results[j].RepoPath + "|" + results[j].Branch
			return ki < kj
		}
		return ti.After(tj)
	})
}

// WorktreeInfo is parsed from `git worktree list --porcelain`.
type WorktreeInfo struct {
	Path       string
	HEAD       string
	Branch     string
	IsBare     bool
	IsDetached bool
	IsPrunable bool
	IsLocked   bool
}

// ParseAllWorktrees parses `git worktree list --porcelain` output into WorktreeInfo slices.
// It does NOT filter—the caller decides what to skip.
func ParseAllWorktrees(output string) []WorktreeInfo {
	var results []WorktreeInfo
	var current *WorktreeInfo

	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			if current != nil {
				results = append(results, *current)
				current = nil
			}
			continue
		}
		if current == nil {
			current = &WorktreeInfo{}
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			// ref is like "refs/heads/feature-auth" — strip prefix
			if strings.HasPrefix(ref, "refs/heads/") {
				current.Branch = strings.TrimPrefix(ref, "refs/heads/")
			} else {
				current.Branch = ref
			}
		case line == "bare":
			current.IsBare = true
		case line == "detached":
			current.IsDetached = true
		case strings.HasPrefix(line, "prunable"):
			current.IsPrunable = true
		case strings.HasPrefix(line, "locked"):
			current.IsLocked = true
		}
	}
	if current != nil {
		results = append(results, *current)
	}
	return results
}

// scanTask is an item to process in the worker pool.
type scanTask struct {
	repoPath string
}

// Scanner is the central coordinator for the unfinished-work background scan.
type Scanner struct {
	reader       VCSReader
	scanQueue    chan scanTask
	resultStore  sync.Map // map[string]ScanResult  (key = repoPath+"|"+branch)
	repoSet      sync.Map // map[string]bool (tracked repo paths, from any source)
	cacheStore   sync.Map // map[string]*worktreeCache (key = worktreePath)
	breakerStore sync.Map // map[string]*circuitBreaker (key = repoPath)

	eventBus   *pkgevents.EventBus
	stateStore *StateStore

	triggerCh  chan struct{}  // signals coordinator to run a full scan now
	scanDoneCh chan time.Time // emits timestamp after each full scan completes

	tickInterval time.Duration // default 30s, overridable in tests

	// sessionRepos tracks repos discovered via auto-spider (session paths),
	// keyed by session UUID — not Title — so a later EventSessionDeleted
	// (which only carries SessionID, the UUID; Session is nil for delete
	// events) can look up which repo the deleted session owned (BUG-034).
	sessionRepos sync.Map // map[string]string  sessionUUID -> repoPath

	// autoSpiderEnabled controls whether SessionCreated/Updated events trigger scans.
	autoSpiderEnabled atomic.Bool

	// fsWatcher watches every tracked repo's .git dir so a real change
	// triggers an immediate targeted rescan instead of waiting for the
	// coordinator's backstop tick. nil when fsnotify is unavailable on this
	// platform — AddRepo/RemoveRepo/watchRepo are all nil-safe no-ops in that
	// case, degrading to relying solely on the (still-present) ticker.
	fsWatcher *fsnotify.Watcher

	// severePressureWarned rate-limits the "skipping scan under memory
	// pressure" log line to once per pressure episode rather than once per
	// skipped repo, so the warning itself can't contribute to log-driven
	// allocation pressure during a real incident.
	severePressureWarned atomic.Bool

	mu deadlock.RWMutex
}

// NewScanner constructs a Scanner. Call Start(ctx) to begin background processing.
func NewScanner(eventBus *pkgevents.EventBus, stateStore *StateStore) *Scanner {
	return NewScannerWithReader(eventBus, stateStore, &GoGitVCSReader{})
}

// NewScannerWithReader constructs a Scanner with an explicit VCSReader.
// Used in tests to inject a fake or alternative implementation.
func NewScannerWithReader(eventBus *pkgevents.EventBus, stateStore *StateStore, reader VCSReader) *Scanner {
	// Register the real reader for debug introspection (see currentReader's
	// doc comment) — a no-op for fake/test readers that don't match the type.
	if gg, ok := reader.(*GoGitVCSReader); ok {
		currentReader.Store(gg)
	}
	s := &Scanner{
		reader:     reader,
		scanQueue:  make(chan scanTask, 50),
		eventBus:   eventBus,
		stateStore: stateStore,
		triggerCh:  make(chan struct{}, 1),
		scanDoneCh: make(chan time.Time, 4),
		// fsnotify (wired in Start/AddRepo) is now the primary scan trigger —
		// this ticker is a backstop for anything fsnotify misses (e.g. a
		// worktree mtime change with no .git write), so it no longer needs
		// to run every 30s. Tests override via SetTickInterval.
		tickInterval: 5 * time.Minute,
	}
	s.autoSpiderEnabled.Store(true)
	return s
}

// SetTickInterval overrides the default 30-second scan tick (for tests).
func (s *Scanner) SetTickInterval(d time.Duration) {
	s.mu.Lock()
	s.tickInterval = d
	s.mu.Unlock()
}

// ScanDone returns a channel that receives the completion time of each full scan.
func (s *Scanner) ScanDone() <-chan time.Time {
	return s.scanDoneCh
}

// Start launches the coordinator goroutine, 4 worker goroutines, and (when
// available) the fsnotify watch loop that makes scanning event-driven rather
// than purely tick-driven. All goroutines exit cleanly when ctx is cancelled.
func (s *Scanner) Start(ctx context.Context) {
	const numWorkers = 4
	for i := 0; i < numWorkers; i++ {
		go s.worker(ctx)
	}
	go s.coordinator(ctx)
	go s.subscribeToSessionEvents(ctx)
	go s.pruneMissingRepos(ctx)

	if w, err := fsnotify.NewWatcher(); err != nil {
		log.Warn("fsnotify unavailable, scanner falling back to tick-only polling", "err", err)
	} else {
		s.mu.Lock()
		s.fsWatcher = w
		s.mu.Unlock()
		go s.fsnotifyLoop(ctx)
	}

	if r, ok := s.reader.(*GoGitVCSReader); ok {
		// Proactive, budget-respecting prune: runs far more often than the
		// old 5-minute full ClearCache (every 1 minute, since a budget-based
		// prune is cheap — no full teardown, just eviction of cold/over-budget
		// entries) so cache pressure never has a chance to build up between
		// polls. Under SEVERE pressure this escalates to the old full
		// ClearCache as an emergency valve — evicting hot repos too, but only
		// when the gentler path alone isn't enough; this should be rare, and
		// firing often is itself a signal something else is wrong.
		go func() {
			tick := time.NewTicker(1 * time.Minute)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					if r.UnderSeverePressure() {
						log.Warn("severe memory pressure — clearing entire repo cache as an emergency valve")
						r.ClearCache()
					} else {
						r.PruneToMemoryBudget()
					}
					// gogitstoreRegistry's SharedObjectStores are reference-counted
					// separately from repoCache (see gogit_vcs_reader.go's
					// releaseGogitstoreRef) — a store only becomes evictable once
					// every cachedRepo that referenced it has itself been evicted
					// above, so this Prune runs every tick regardless of which
					// branch fired, mirroring the two-cache relationship rather
					// than duplicating pressure-tier logic here.
					r.gogitstoreRegistry().Prune()
				}
			}
		}()
	}
}

// coordinator goroutine: ticks every 30s and handles trigger signals.
func (s *Scanner) coordinator(ctx context.Context) {
	s.mu.RLock()
	tick := time.NewTicker(s.tickInterval)
	s.mu.RUnlock()
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.enqueueAll()
		case <-s.triggerCh:
			s.enqueueAll()
		}
	}
}

// TriggerScan signals the coordinator to run a full scan immediately.
func (s *Scanner) TriggerScan() {
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

// watchRepo best-effort registers repoPath's .git dir with the fsnotify
// watcher so a real change triggers an immediate targeted rescan. No-op if
// fsnotify is unavailable on this platform (s.fsWatcher == nil) — that's the
// graceful-degradation path back to relying solely on the ticker. Errors are
// logged at Debug, not Warn: many repo paths won't have a .git dir yet at
// the moment they're first tracked (e.g. auto-spidered from a session whose
// worktree is still being created), which is a normal transient condition,
// not a fault.
func (s *Scanner) watchRepo(repoPath string) {
	s.mu.RLock()
	w := s.fsWatcher
	s.mu.RUnlock()
	if w == nil {
		return
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := w.Add(gitDir); err != nil {
		log.Debug("could not watch git dir", "dir", gitDir, "err", err)
	}
}

// unwatchRepo best-effort removes repoPath's .git dir from the fsnotify
// watcher, avoiding a slow leak of watch descriptors as repos come and go.
// No-op if fsnotify is unavailable.
func (s *Scanner) unwatchRepo(repoPath string) {
	s.mu.RLock()
	w := s.fsWatcher
	s.mu.RUnlock()
	if w == nil {
		return
	}
	_ = w.Remove(filepath.Join(repoPath, ".git"))
}

// fsnotifyLoop handles fsnotify events on every watched repo's .git dir and
// enqueues a targeted rescan of just that repo — this is the "only run when
// things change" trigger; the coordinator's tick is now a pure backstop.
// Exits cleanly when ctx is cancelled, per this package's goroutine convention.
func (s *Scanner) fsnotifyLoop(ctx context.Context) {
	s.mu.RLock()
	w := s.fsWatcher
	s.mu.RUnlock()
	if w == nil {
		return
	}
	defer w.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			// Derive the repo root by walking up from the event path to the
			// nearest ".git" component (mirrors WatchDirWatcher.fsnotifyLoop's
			// event-handling shape in watcher.go).
			dir := event.Name
			for {
				if filepath.Base(dir) == ".git" {
					repoRoot := filepath.Dir(dir)
					s.InvalidateCache(repoRoot)
					s.EnqueueRepo(repoRoot)
					break
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Warn("scanner fsnotify error", "err", err)
		}
	}
}

// enqueueAll sends all known repos to the scan queue.
func (s *Scanner) enqueueAll() {
	s.repoSet.Range(func(key, _ any) bool {
		repoPath, _ := key.(string)
		s.EnqueueRepo(repoPath)
		return true
	})
}

// EnqueueRepo queues a repo for scanning if it's not cached recently.
func (s *Scanner) EnqueueRepo(repoPath string) {
	// Check circuit breaker first.
	if !s.shouldScan(repoPath) {
		return
	}

	// Graceful degradation under memory pressure: skip this cycle rather than
	// pile on more allocation while the process is already under severe
	// pressure. The repo's last-known-good cached result stays in place —
	// this is a deliberate stale-over-failed trade-off, matching this
	// codebase's existing "degrade gracefully" conventions elsewhere. Rate-
	// limited to one WARN per pressure episode (not per skipped repo) so the
	// warning itself can't contribute to log-driven allocation during a real
	// incident.
	if r, ok := s.reader.(*GoGitVCSReader); ok && r.UnderSeverePressure() {
		if s.severePressureWarned.CompareAndSwap(false, true) {
			log.Warn("skipping scans under severe memory pressure — results may go stale until pressure subsides")
		}
		return
	}
	s.severePressureWarned.Store(false)

	// Check worktree-level TTL cache: if all worktrees for this repo are fresh, skip.
	// We do a lightweight check by looking for any result stored recently.
	recent := false
	s.cacheStore.Range(func(k, v any) bool {
		c, ok := v.(*worktreeCache)
		if !ok {
			return true
		}
		// Check if this cache entry belongs to the given repo (by path prefix—approximate).
		if strings.HasPrefix(k.(string), repoPath+"/") || k == repoPath {
			if _, ok := c.Get(); ok {
				recent = true
				return false // stop iteration
			}
		}
		return true
	})
	if recent {
		return
	}

	select {
	case s.scanQueue <- scanTask{repoPath: repoPath}:
	default:
		log.Warn("scan queue full, dropping repo", "repo", repoPath)
	}
}

// worker goroutine: consumes tasks from scanQueue.
func (s *Scanner) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-s.scanQueue:
			if !ok {
				return
			}
			results := s.scanRepo(task.repoPath)
			s.publishResults(results)
		}
	}
}

// scanRepo enumerates all worktrees in the given repo root and scans each one.
func (s *Scanner) scanRepo(repoPath string) []ScanResult {
	worktrees, err := s.reader.ListWorktrees(repoPath)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.recordTimeout(repoPath)
			log.Warn("worktree list timed out", "repo", repoPath)
		} else {
			log.Debug("worktree list error", "repo", repoPath, "err", err)
		}
		return nil
	}
	if len(worktrees) == 0 {
		return nil
	}

	// Resolve default branch once per repo.
	defaultBranch := s.reader.ResolveDefaultBranch(repoPath)

	var results []ScanResult
	for _, wt := range worktrees {
		if wt.IsBare || wt.IsDetached || wt.IsPrunable {
			continue
		}
		if wt.Branch == "" {
			continue
		}
		result := s.scanWorktree(wt, defaultBranch, repoPath)
		if result.Status == ScanResultStatusOK && !result.IsUnfinished() {
			// Worktree went clean (e.g. the only uncommitted file was deleted) —
			// remove any stale entry left in resultStore. Mirrors the CleanWorktree
			// removal pattern in review_queue_poller.go, which drops a stale
			// UncommittedChanges queue entry once the determiner reports the
			// worktree clean.
			s.removeStaleResult(repoPath, wt.Branch)
			continue // clean worktree — skip
		}
		results = append(results, result)
	}

	s.resetBreaker(repoPath)
	return results
}

// scanWorktree produces a ScanResult for a single git worktree.
func (s *Scanner) scanWorktree(wt WorktreeInfo, defaultBranch, repoPath string) ScanResult {
	result := ScanResult{
		RepoPath:      repoPath,
		Branch:        wt.Branch,
		WorktreePath:  wt.Path,
		RepoName:      filepath.Base(repoPath),
		DefaultBranch: defaultBranch,
		ScanTime:      time.Now(),
	}

	// Display path with ~ substitution.
	if home, err := os.UserHomeDir(); err == nil {
		if strings.HasPrefix(wt.Path, home) {
			result.DisplayPath = "~" + wt.Path[len(home):]
		} else {
			result.DisplayPath = wt.Path
		}
	} else {
		result.DisplayPath = wt.Path
	}

	// Last modified: mtime of the worktree dir.
	if fi, err := os.Stat(wt.Path); err == nil {
		result.LastModified = fi.ModTime()
	}

	// Check cache.
	cache := s.getOrCreateCache(wt.Path)
	if cached, ok := cache.Get(); ok {
		return cached
	}

	uncommitted, err := s.reader.HasUncommitted(wt.Path)
	if err != nil {
		if strings.Contains(err.Error(), "timed out") {
			result.Status = ScanResultStatusTimeout
			result.ErrorMsg = fmt.Sprintf("HasUncommitted timed out for %s", wt.Path)
			s.recordTimeout(repoPath)
			cache.Set(result)
			return result
		}
		result.Status = ScanResultStatusError
		result.ErrorMsg = err.Error()
		cache.Set(result)
		return result
	}
	result.HasUncommitted = uncommitted

	if defaultBranch != "" {
		ahead, behind, aErr := s.reader.AheadBehind(wt.Path, defaultBranch)
		if aErr == nil {
			result.AheadCount = ahead
			result.BehindCount = behind
		}
		if result.AheadCount > 0 {
			msgs, mErr := s.reader.CommitMessages(wt.Path, defaultBranch, 5)
			if mErr == nil {
				result.AheadMessages = msgs
			}
		}
	}

	if d, dErr := s.reader.DiffShortstat(wt.Path); dErr == nil {
		result.ChangedFiles = d.Files
		result.LinesAdded = d.Insertions
		result.LinesRemoved = d.Deletions
	}

	result.Status = ScanResultStatusOK
	cache.Set(result)
	return result
}

// parseDiffShortstat parses "3 files changed, 142 insertions(+), 28 deletions(-)" into a DiffStat.
func parseDiffShortstat(s string) DiffStat {
	var d DiffStat
	if s == "" {
		return d
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		kw := strings.ToLower(fields[1])
		switch {
		case strings.HasPrefix(kw, "file"):
			d.Files = n
		case strings.HasPrefix(kw, "insertion"):
			d.Insertions = n
		case strings.HasPrefix(kw, "deletion"):
			d.Deletions = n
		}
	}
	return d
}

// publishResults emits UnfinishedWorkUpdated events for changed scan results.
func (s *Scanner) publishResults(results []ScanResult) {
	for _, result := range results {
		key := result.RepoPath + "|" + result.Branch

		// Check dismiss/snooze state.
		if s.stateStore != nil {
			if s.stateStore.IsDismissed(result.RepoPath, result.Branch) {
				continue
			}
			if s.stateStore.IsSnoozed(result.RepoPath, result.Branch, result.WorktreePath) {
				continue
			}
		}

		// Only emit if changed from stored state.
		prevRaw, loaded := s.resultStore.Load(key)
		changed := !loaded
		if !changed {
			prev, _ := prevRaw.(ScanResult)
			// Simple change detection: compare key fields.
			changed = prev.HasUncommitted != result.HasUncommitted ||
				prev.AheadCount != result.AheadCount ||
				prev.BehindCount != result.BehindCount ||
				prev.Status != result.Status
		}

		s.resultStore.Store(key, result)

		if changed && s.eventBus != nil {
			evt := newUnfinishedWorkUpdatedEvent(result)
			s.eventBus.Publish(evt)
		}
	}

	// After scan batch, emit ScanCompleted.
	if s.eventBus != nil {
		s.eventBus.Publish(newScanCompletedEvent())
	}
	select {
	case s.scanDoneCh <- time.Now():
	default:
	}
}

// ResolveDefaultBranch delegates to the underlying VCSReader.
func (s *Scanner) ResolveDefaultBranch(repoPath string) string {
	return s.reader.ResolveDefaultBranch(repoPath)
}

// GetAllResults returns a snapshot of all stored scan results (excluding dismissed/snoozed).
func (s *Scanner) GetAllResults() []ScanResult {
	var results []ScanResult
	s.resultStore.Range(func(_, v any) bool {
		r, _ := v.(ScanResult)
		if s.stateStore != nil {
			if s.stateStore.IsDismissed(r.RepoPath, r.Branch) {
				return true
			}
			if s.stateStore.IsSnoozed(r.RepoPath, r.Branch, r.WorktreePath) {
				return true
			}
		}
		results = append(results, r)
		return true
	})
	SortByLastModified(results)
	return results
}

// GetResultByKey returns a single stored result by (repoPath, branch).
func (s *Scanner) GetResultByKey(repoPath, branch string) (ScanResult, bool) {
	key := repoPath + "|" + branch
	v, ok := s.resultStore.Load(key)
	if !ok {
		return ScanResult{}, false
	}
	r, _ := v.(ScanResult)
	return r, true
}

// RemoveResult removes a result from the store (called after dismiss/snooze).
func (s *Scanner) RemoveResult(repoPath, branch string) {
	key := repoPath + "|" + branch
	s.resultStore.Delete(key)
}

// removeStaleResult removes a previously stored result for a worktree that is
// now clean and publishes EventUnfinishedWorkRemoved so subscribers drop it
// immediately, rather than waiting for it to silently age out. No-op (and no
// event) if nothing was stored for this key.
func (s *Scanner) removeStaleResult(repoPath, branch string) {
	key := repoPath + "|" + branch
	if _, loaded := s.resultStore.LoadAndDelete(key); loaded && s.eventBus != nil {
		s.eventBus.Publish(&pkgevents.Event{
			Type:      EventUnfinishedWorkRemoved,
			Timestamp: time.Now(),
			Context:   key,
		})
	}
}

// AddRepo adds a repo path to the scan set, registers it with the fsnotify
// watcher (the sole choke point every repo-discovery path — pinned, watch-dir
// walk, or session auto-spider — funnels through, so every tracked repo gets
// event-driven scanning regardless of how it was discovered), and immediately
// enqueues it for an initial scan.
func (s *Scanner) AddRepo(repoPath string) {
	s.repoSet.Store(repoPath, true)
	s.watchRepo(repoPath)
	s.EnqueueRepo(repoPath)
}

// RemoveRepo removes a repo from the scan set, purges its results, and
// unregisters its fsnotify watch.
func (s *Scanner) RemoveRepo(repoPath string) {
	s.repoSet.Delete(repoPath)
	s.unwatchRepo(repoPath)
	// Purge cached results for this repo.
	s.resultStore.Range(func(k, _ any) bool {
		key, _ := k.(string)
		if strings.HasPrefix(key, repoPath+"|") {
			s.resultStore.Delete(k)
		}
		return true
	})
}

// pruneRepoInterval is a var (not a const) so tests can shrink it instead of
// waiting out the real 5-minute cadence.
var pruneRepoInterval = 5 * time.Minute

// pruneMissingRepos is a backstop, independent of the explicit RemoveRepo
// call sites (BacklogService.cleanupItemWorktreesExcept,
// subscribeToSessionEvents's EventSessionDeleted handling): periodically
// checks every currently-registered repo path still exists on disk and
// removes any that don't. Catches any worktree/repo removal path — present
// or future — that doesn't explicitly call RemoveRepo, so this class of bug
// (BUG-034: a repo watched forever after the session/worktree that owned it
// is long gone) can't silently reappear the next time a new cleanup path is
// added without remembering to wire this in. Cheap: one os.Stat per
// registered repo, once per tick, no scan/allocation work.
func (s *Scanner) pruneMissingRepos(ctx context.Context) {
	tick := time.NewTicker(pruneRepoInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			var stale []string
			s.repoSet.Range(func(key, _ any) bool {
				repoPath, _ := key.(string)
				if repoPath == "" {
					return true
				}
				if _, err := os.Stat(repoPath); os.IsNotExist(err) {
					stale = append(stale, repoPath)
				}
				return true
			})
			for _, repoPath := range stale {
				log.Info("pruning repo removed from disk", "path", repoPath)
				s.RemoveRepo(repoPath)
			}
		}
	}
}

// AddPinnedRepo validates that path is a git repo, then adds it.
func (s *Scanner) AddPinnedRepo(repoPath string) error {
	if _, err := os.Stat(repoPath); err != nil {
		return fmt.Errorf("path does not exist: %w", err)
	}
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return fmt.Errorf("path is not a git repository (no .git dir): %s", repoPath)
	}
	s.AddRepo(repoPath)
	return nil
}

// RemovePinnedRepo removes a pinned repo from scanning.
func (s *Scanner) RemovePinnedRepo(repoPath string) {
	s.RemoveRepo(repoPath)
}

// SetAutoSpider enables or disables auto-spider of session paths.
func (s *Scanner) SetAutoSpider(enabled bool) {
	s.autoSpiderEnabled.Store(enabled)
}

// subscribeToSessionEvents listens for SessionCreated/Updated events and
// auto-spiders their repo into the scan set, and for SessionDeleted events to
// remove it again (BUG-034) — without this half of the symmetry, every
// auto-spidered repo stayed watched forever, even long after its session
// (and the worktree it pointed at) was gone. A repo is only actually removed
// once no other tracked session still references it (sessionRepos is checked
// for any other entry pointing at the same repoRoot first), so two sessions
// sharing one repo root (e.g. two non-worktree sessions in the same project)
// don't have their scanning cut out from under the surviving one.
func (s *Scanner) subscribeToSessionEvents(ctx context.Context) {
	if s.eventBus == nil {
		return
	}
	ch, id := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(id)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			switch evt.Type {
			case pkgevents.EventSessionCreated, pkgevents.EventSessionUpdated:
				if !s.autoSpiderEnabled.Load() {
					continue
				}
				if evt.Session == nil || evt.Session.Path == "" {
					continue
				}
				repoRoot := findGitRepoRootSimple(evt.Session.Path)
				if repoRoot == "" {
					continue
				}
				s.sessionRepos.Store(evt.Session.UUID, repoRoot)
				s.AddRepo(repoRoot)
			case pkgevents.EventSessionDeleted:
				s.forgetSessionRepo(evt.SessionID)
			}
		}
	}
}

// forgetSessionRepo drops sessionUUID's entry from sessionRepos and, if no
// other tracked session still references the same repo root, tells the
// scanner to stop watching it (BUG-034). No-op if sessionUUID was never
// auto-spidered (e.g. a pinned repo, or auto-spider was disabled).
func (s *Scanner) forgetSessionRepo(sessionUUID string) {
	if sessionUUID == "" {
		return
	}
	v, loaded := s.sessionRepos.LoadAndDelete(sessionUUID)
	if !loaded {
		return
	}
	repoRoot, _ := v.(string)
	if repoRoot == "" {
		return
	}
	stillReferenced := false
	s.sessionRepos.Range(func(_, other any) bool {
		if otherRepo, _ := other.(string); otherRepo == repoRoot {
			stillReferenced = true
			return false
		}
		return true
	})
	if !stillReferenced {
		s.RemoveRepo(repoRoot)
	}
}

// findGitRepoRootSimple walks up from path to find the first directory containing .git.
func findGitRepoRootSimple(path string) string {
	cur := path
	for {
		gitDir := filepath.Join(cur, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// getOrCreateCache returns the worktreeCache for a given path, creating if absent.
func (s *Scanner) getOrCreateCache(worktreePath string) *worktreeCache {
	v, _ := s.cacheStore.LoadOrStore(worktreePath, &worktreeCache{ttl: 30 * time.Second})
	c, _ := v.(*worktreeCache)
	return c
}

// InvalidateCache invalidates the cache for a given worktree path.
func (s *Scanner) InvalidateCache(worktreePath string) {
	if v, ok := s.cacheStore.Load(worktreePath); ok {
		if c, ok := v.(*worktreeCache); ok {
			c.Invalidate()
		}
	}
}

// --- Circuit breaker ---

type circuitBreaker struct {
	mu                  deadlock.Mutex
	consecutiveTimeouts int
	backoffUntil        time.Time
}

func (s *Scanner) getBreakerFor(repoPath string) *circuitBreaker {
	v, _ := s.breakerStore.LoadOrStore(repoPath, &circuitBreaker{})
	b, _ := v.(*circuitBreaker)
	return b
}

func (s *Scanner) shouldScan(repoPath string) bool {
	b := s.getBreakerFor(repoPath)
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.backoffUntil.IsZero() && time.Now().Before(b.backoffUntil) {
		return false
	}
	return true
}

func (s *Scanner) recordTimeout(repoPath string) {
	b := s.getBreakerFor(repoPath)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveTimeouts++
	if b.consecutiveTimeouts >= 3 {
		b.backoffUntil = time.Now().Add(5 * time.Minute)
		log.Warn("circuit breaker triggered, backing off", "repo", repoPath, "backoff", "5m")
	}
}

func (s *Scanner) resetBreaker(repoPath string) {
	b := s.getBreakerFor(repoPath)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.consecutiveTimeouts = 0
	b.backoffUntil = time.Time{}
}
