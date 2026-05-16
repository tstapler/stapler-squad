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

	// sessionRepos tracks repos discovered via auto-spider (session paths).
	sessionRepos sync.Map // map[string]string  sessionID -> repoPath

	// autoSpiderEnabled controls whether SessionCreated/Updated events trigger scans.
	autoSpiderEnabled atomic.Bool

	mu deadlock.RWMutex
}

// NewScanner constructs a Scanner. Call Start(ctx) to begin background processing.
func NewScanner(eventBus *pkgevents.EventBus, stateStore *StateStore) *Scanner {
	return NewScannerWithReader(eventBus, stateStore, &GoGitVCSReader{})
}

// NewScannerWithReader constructs a Scanner with an explicit VCSReader.
// Used in tests to inject a fake or alternative implementation.
func NewScannerWithReader(eventBus *pkgevents.EventBus, stateStore *StateStore, reader VCSReader) *Scanner {
	s := &Scanner{
		reader:       reader,
		scanQueue:    make(chan scanTask, 50),
		eventBus:     eventBus,
		stateStore:   stateStore,
		triggerCh:    make(chan struct{}, 1),
		scanDoneCh:   make(chan time.Time, 4),
		tickInterval: 30 * time.Second,
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

// Start launches the coordinator goroutine and 4 worker goroutines.
// All goroutines exit cleanly when ctx is cancelled.
func (s *Scanner) Start(ctx context.Context) {
	const numWorkers = 4
	for i := 0; i < numWorkers; i++ {
		go s.worker(ctx)
	}
	go s.coordinator(ctx)
	go s.subscribeToSessionEvents(ctx)
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

// AddRepo adds a repo path to the scan set and immediately enqueues it.
func (s *Scanner) AddRepo(repoPath string) {
	s.repoSet.Store(repoPath, true)
	s.EnqueueRepo(repoPath)
}

// RemoveRepo removes a repo from the scan set and purges its results.
func (s *Scanner) RemoveRepo(repoPath string) {
	s.repoSet.Delete(repoPath)
	// Purge cached results for this repo.
	s.resultStore.Range(func(k, _ any) bool {
		key, _ := k.(string)
		if strings.HasPrefix(key, repoPath+"|") {
			s.resultStore.Delete(k)
		}
		return true
	})
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

// subscribeToSessionEvents listens for SessionCreated/Updated events and enqueues repos.
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
			if !s.autoSpiderEnabled.Load() {
				continue
			}
			if evt.Type != pkgevents.EventSessionCreated && evt.Type != pkgevents.EventSessionUpdated {
				continue
			}
			if evt.Session == nil || evt.Session.Path == "" {
				continue
			}
			repoRoot := findGitRepoRootSimple(evt.Session.Path)
			if repoRoot == "" {
				continue
			}
			sessionID := evt.Session.Title
			s.sessionRepos.Store(sessionID, repoRoot)
			s.AddRepo(repoRoot)
		}
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
