package session

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// WorktreeScanItem is the minimal worktree info the poller needs from the
// unfinished-work scanner. Using a local struct avoids an import cycle:
//   session → session/unfinished → pkg/events → session
//
// The server layer bridges the two packages via WorktreeSource (adapter pattern).
type WorktreeScanItem struct {
	RepoPath     string
	Branch       string
	WorktreePath string
}

// WorktreeSource provides the set of currently-known worktrees.
// Implement this interface by wrapping an *unfinished.Scanner in the server layer.
type WorktreeSource interface {
	// ScanDone returns a channel that receives the scan time after every scan pass.
	ScanDone() <-chan time.Time
	// GetWorktrees returns a snapshot of all currently-known worktrees.
	GetWorktrees() []WorktreeScanItem
}

// WorktreePRPollerConfig controls polling cadence and auth caching.
type WorktreePRPollerConfig struct {
	PollInterval      time.Duration
	CallTimeout       time.Duration
	AuthCacheDuration time.Duration
}

// DefaultWorktreePRPollerConfig returns sensible defaults matching PRStatusPoller.
func DefaultWorktreePRPollerConfig() WorktreePRPollerConfig {
	return WorktreePRPollerConfig{
		PollInterval:      60 * time.Second,
		CallTimeout:       10 * time.Second,
		AuthCacheDuration: 5 * time.Minute,
	}
}

// worktreeOnUpdatedFn is a function value stored in an atomic.Value.
// Wrapping in a struct is required — atomic.Value panics if nil is stored.
type worktreeOnUpdatedFn struct {
	fn func(repoPath, branch string, info *github.PRInfo)
}

// WorktreePRPoller polls GitHub PR status for worktrees that have no active
// session. It is the counterpart to PRStatusPoller (which covers session-backed
// worktrees). The two pollers divide the worktree space: PRStatusPoller owns
// worktrees with a running session; WorktreePRPoller owns the rest.
//
// Concurrency design:
//   - data cache is a sync.Map — lock-free reads in the steady state
//   - auth state is an atomic.Value (pollerAuthResult) — same pattern as PRStatusPoller
//   - onUpdated callback is an atomic.Value — writers Store, readers Load, no lock
type WorktreePRPoller struct {
	source       WorktreeSource
	prPoller     *PRStatusPoller // for session-backed-path exclusion
	etagCache    *github.ETagCache
	config       WorktreePRPollerConfig
	data         sync.Map     // key: worktreeCacheKey(repoPath, branch), value: *github.PRInfo
	authState    atomic.Value //nolint:exhaustruct // stores pollerAuthResult
	onUpdatedVal atomic.Value //nolint:exhaustruct // stores worktreeOnUpdatedFn
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewWorktreePRPoller creates a WorktreePRPoller with default configuration.
// source may be nil at construction time; call SetSource before Start.
func NewWorktreePRPoller(etagCache *github.ETagCache, prPoller *PRStatusPoller) *WorktreePRPoller {
	return NewWorktreePRPollerWithConfig(etagCache, prPoller, DefaultWorktreePRPollerConfig())
}

// NewWorktreePRPollerWithConfig creates a WorktreePRPoller with custom configuration.
func NewWorktreePRPollerWithConfig(etagCache *github.ETagCache, prPoller *PRStatusPoller, cfg WorktreePRPollerConfig) *WorktreePRPoller {
	return &WorktreePRPoller{
		etagCache: etagCache,
		prPoller:  prPoller,
		config:    cfg,
	}
}

// SetSource sets the worktree data source. Safe to call before Start.
func (p *WorktreePRPoller) SetSource(src WorktreeSource) {
	p.source = src
}

// SetOnUpdated registers a callback invoked whenever cached PR data changes.
// Safe to call before or after Start; the callback is replaced atomically.
func (p *WorktreePRPoller) SetOnUpdated(fn func(repoPath, branch string, info *github.PRInfo)) {
	p.onUpdatedVal.Store(worktreeOnUpdatedFn{fn: fn})
}

// Start begins the polling loop. It is a no-op if already started.
func (p *WorktreePRPoller) Start(ctx context.Context) {
	if p.ctx != nil {
		return
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	go p.pollLoop()
	log.Info("worktree PR poller started", "interval", p.config.PollInterval)
}

// Stop gracefully shuts down the poller and waits for in-flight requests.
func (p *WorktreePRPoller) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
	log.Info("worktree PR poller stopped")
}

// GetPRData returns cached PR info for a worktree, or nil if not yet known.
func (p *WorktreePRPoller) GetPRData(repoPath, branch string) *github.PRInfo {
	v, ok := p.data.Load(worktreeCacheKey(repoPath, branch))
	if !ok {
		return nil
	}
	return v.(*github.PRInfo)
}

// pollLoop drives the poller: react to scanner completions and a fallback ticker.
func (p *WorktreePRPoller) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	// Seed immediately if source is already available.
	if p.source != nil {
		p.pollWorktrees(p.source.GetWorktrees())
	}

	for {
		if p.source == nil {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
			}
			continue
		}

		select {
		case <-p.ctx.Done():
			return
		case <-p.source.ScanDone():
			p.pollWorktrees(p.source.GetWorktrees())
		case <-ticker.C:
			p.pollWorktrees(p.source.GetWorktrees())
		}
	}
}

// pollWorktrees iterates scan results, skips session-backed worktrees, and
// fetches GitHub PR data for the remainder.
func (p *WorktreePRPoller) pollWorktrees(items []WorktreeScanItem) {
	if !p.isAuthOK() {
		return
	}

	sessionPaths := p.sessionBackedPaths()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // match PRStatusPoller default concurrency

	for _, item := range items {
		if _, owned := sessionPaths[item.WorktreePath]; owned {
			continue // PRStatusPoller covers this one
		}
		if item.Branch == "" || item.RepoPath == "" {
			continue
		}

		captured := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			p.fetchAndStore(captured)
		}()
	}
	wg.Wait()
}

// fetchAndStore fetches PR info for one worktree and stores it in the cache.
func (p *WorktreePRPoller) fetchAndStore(item WorktreeScanItem) {
	owner, repo, err := github.GetOwnerRepoFromRemote(item.RepoPath)
	if err != nil {
		log.Warn("worktree PR poller: could not read remote URL", "path", item.RepoPath, "err", err)
		return
	}
	if owner == "" || repo == "" {
		return // not a GitHub remote
	}

	ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)
	defer cancel()

	key := worktreeCacheKey(item.RepoPath, item.Branch)

	// Use ETag conditional fetch when we already know the PR number.
	if existing := p.GetPRData(item.RepoPath, item.Branch); existing != nil && existing.Number > 0 {
		info, changed, fetchErr := github.GetPRInfoConditional(ctx, owner, repo, existing.Number, p.etagCache)
		if fetchErr != nil {
			p.handleFetchError(fetchErr)
			return
		}
		if changed && info != nil {
			p.data.Store(key, info)
			p.fireOnUpdated(item.RepoPath, item.Branch, info)
		}
		return
	}

	// Discovery: find PR by branch name.
	info, err := github.GetPRForBranch(ctx, owner, repo, item.Branch)
	if err != nil {
		if strings.Contains(err.Error(), "no open PR") {
			return // expected when branch has no PR yet
		}
		p.handleFetchError(err)
		return
	}
	p.data.Store(key, info)
	p.fireOnUpdated(item.RepoPath, item.Branch, info)
}

// isAuthOK returns true if GitHub auth passes, using a time-based atomic cache.
func (p *WorktreePRPoller) isAuthOK() bool {
	if v := p.authState.Load(); v != nil {
		if r := v.(pollerAuthResult); time.Since(r.checkedAt) < p.config.AuthCacheDuration {
			return r.ok
		}
	}
	if err := github.CheckGHAuth(); err != nil {
		log.Warn("worktree PR poller: github auth unavailable", "err", err)
		p.authState.Store(pollerAuthResult{ok: false, checkedAt: time.Now()})
		return false
	}
	p.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})
	return true
}

// handleFetchError logs rate-limit and auth errors, invalidating auth cache on 401.
func (p *WorktreePRPoller) handleFetchError(err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "429"):
		log.Warn("worktree PR poller: rate limit hit")
	case strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized"):
		log.Warn("worktree PR poller: auth error, invalidating cache")
		p.authState.Store(pollerAuthResult{ok: false, checkedAt: time.Now()})
	default:
		log.Warn("worktree PR poller: fetch error", "err", err)
	}
}

// sessionBackedPaths returns the set of worktree paths currently covered by
// PRStatusPoller (i.e. have an active session). O(n) snapshot under read lock.
func (p *WorktreePRPoller) sessionBackedPaths() map[string]struct{} {
	if p.prPoller == nil {
		return nil
	}
	p.prPoller.mu.RLock()
	insts := make([]*Instance, len(p.prPoller.instances))
	copy(insts, p.prPoller.instances)
	p.prPoller.mu.RUnlock()

	paths := make(map[string]struct{}, len(insts))
	for _, inst := range insts {
		if inst.Path != "" {
			paths[inst.Path] = struct{}{}
		}
	}
	return paths
}

// fireOnUpdated invokes the registered callback if one is set.
func (p *WorktreePRPoller) fireOnUpdated(repoPath, branch string, info *github.PRInfo) {
	if v := p.onUpdatedVal.Load(); v != nil {
		if w := v.(worktreeOnUpdatedFn); w.fn != nil {
			w.fn(repoPath, branch, info)
		}
	}
}

// worktreeCacheKey returns the sync.Map key for a (repoPath, branch) pair.
func worktreeCacheKey(repoPath, branch string) string {
	return repoPath + "|" + branch
}
