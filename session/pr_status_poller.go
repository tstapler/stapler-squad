package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linkdata/deadlock"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// pollerAuthResult is an immutable snapshot of the auth check state.
// Stored in PRStatusPoller.authState (atomic.Value) so readers are lock-free.
type pollerAuthResult struct {
	ok        bool
	checkedAt time.Time
}

// PRStatusPollerConfig contains configuration for the PR status poller.
type PRStatusPollerConfig struct {
	// PollInterval controls how often all sessions are checked.
	PollInterval time.Duration
	// ConcurrentFetches limits simultaneous gh CLI calls (respects secondary rate limits).
	ConcurrentFetches int
	// CallTimeout is the maximum time for a single gh API call.
	CallTimeout time.Duration
	// AuthCacheDuration controls how long a successful auth check is cached.
	AuthCacheDuration time.Duration
	// NoPRBackoff is how long to wait before re-checking a session after ErrNoPR.
	// Zero disables the backoff (always re-check).
	NoPRBackoff time.Duration
}

// DefaultPRStatusPollerConfig returns sensible defaults.
func DefaultPRStatusPollerConfig() PRStatusPollerConfig {
	return PRStatusPollerConfig{
		PollInterval:      60 * time.Second,
		ConcurrentFetches: 5,
		CallTimeout:       10 * time.Second,
		AuthCacheDuration: 5 * time.Minute,
		NoPRBackoff:       5 * time.Minute,
	}
}

// PRStatusPoller polls GitHub PR status for all sessions at a shared interval.
// Uses a single workspace-level ticker (not per-session goroutines) and an ETag
// cache so unchanged PRs return HTTP 304 and cost zero rate-limit quota.
type PRStatusPoller struct {
	instances []*Instance
	storage   *Storage
	config    PRStatusPollerConfig
	etagCache *github.ETagCache

	// onUpdated is called when a session's PR priority changes.
	// Intended for EventBus notification; injected from the server layer.
	onUpdated func(*Instance)

	// authState stores pollerAuthResult atomically; readers are lock-free.
	authState atomic.Value //nolint:exhaustruct

	// noPRPollAfter tracks the earliest time at which we should re-check a
	// session that had no PR on the previous poll. Keyed by session title.
	// Guarded by mu.
	noPRPollAfter map[string]time.Time

	// listEtags stores ETags for the branch→PR list endpoint, keyed by
	// "owner/repo/branch". Allows 304 responses during PR discovery.
	listEtags sync.Map

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     deadlock.RWMutex
}

// NewPRStatusPoller creates a new poller with default configuration.
func NewPRStatusPoller(storage *Storage) *PRStatusPoller {
	return NewPRStatusPollerWithConfig(storage, DefaultPRStatusPollerConfig())
}

// NewPRStatusPollerWithConfig creates a poller with custom configuration.
func NewPRStatusPollerWithConfig(storage *Storage, config PRStatusPollerConfig) *PRStatusPoller {
	return &PRStatusPoller{
		instances:     make([]*Instance, 0),
		storage:       storage,
		config:        config,
		etagCache:     github.NewETagCache(),
		noPRPollAfter: make(map[string]time.Time),
	}
}

// PollInterval returns the poller's configured poll interval, so other components
// (e.g. ApprovalHandler's CI-status staleness guard) can bound freshness against the
// same live-configured value rather than a disconnected duplicate literal.
func (p *PRStatusPoller) PollInterval() time.Duration {
	return p.config.PollInterval
}

// ETagCache returns the poller's shared *github.ETagCache, so other pollers
// (e.g. WorktreePRPoller) reuse the same conditional-request cache instead of
// each maintaining their own — per ADR-022, a separate cache would double
// GitHub API call volume for repos both pollers hit.
func (p *PRStatusPoller) ETagCache() *github.ETagCache {
	return p.etagCache
}

// SetInstances replaces the full list of monitored instances.
func (p *PRStatusPoller) SetInstances(instances []*Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instances = instances
}

// GetInstances returns a defensive copy of the currently monitored instances.
// Callers must not modify the returned slice elements.
func (p *PRStatusPoller) GetInstances() []*Instance {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Instance, len(p.instances))
	copy(out, p.instances)
	return out
}

// AddInstance adds a single instance to monitor.
func (p *PRStatusPoller) AddInstance(inst *Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instances = append(p.instances, inst)
}

// RemoveInstance removes an instance from monitoring.
func (p *PRStatusPoller) RemoveInstance(title string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	filtered := make([]*Instance, 0, len(p.instances))
	for _, inst := range p.instances {
		if !inst.MatchesID(title) {
			filtered = append(filtered, inst)
		}
	}
	p.instances = filtered
	delete(p.noPRPollAfter, title)
}

// SetOnUpdated registers a callback called when a session's PR priority changes.
// The callback is invoked from a goroutine; it must be concurrency-safe.
func (p *PRStatusPoller) SetOnUpdated(fn func(*Instance)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onUpdated = fn
}

// Start begins the polling loop. Safe to call multiple times; subsequent calls are no-ops.
func (p *PRStatusPoller) Start(ctx context.Context) {
	p.mu.Lock()
	if p.ctx != nil {
		p.mu.Unlock()
		return
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.mu.Unlock()

	p.wg.Add(1)
	go p.pollLoop()
	log.Info("PR status poller started", "interval", p.config.PollInterval, "concurrency", p.config.ConcurrentFetches)
}

// Stop gracefully shuts down the poller and waits for in-flight requests.
func (p *PRStatusPoller) Stop() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.mu.Unlock()
	p.wg.Wait()
	log.Info("PR status poller stopped")
}

// pollLoop runs the main ticker loop.
func (p *PRStatusPoller) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	// Run an immediate check so sessions show real status without waiting a
	// full PollInterval. ETag caching and rate-limit guards still apply.
	p.checkAllSessions()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.checkAllSessions()
		}
	}
}

// checkAllSessions iterates all monitored instances and updates PR status concurrently.
func (p *PRStatusPoller) checkAllSessions() {
	if limited, until := github.DefaultRateLimiter.IsLimited(); limited {
		log.Info("PR status poller: rate limited, skipping tick", "until", until)
		return
	}

	p.mu.RLock()
	instances := make([]*Instance, len(p.instances))
	copy(instances, p.instances)
	p.mu.RUnlock()

	if !p.isAuthOK() {
		return
	}

	sem := make(chan struct{}, p.config.ConcurrentFetches)
	var wg sync.WaitGroup

	now := time.Now()
	p.mu.RLock()
	noPRPollAfter := make(map[string]time.Time, len(p.noPRPollAfter))
	for k, v := range p.noPRPollAfter {
		noPRPollAfter[k] = v
	}
	p.mu.RUnlock()

	for _, inst := range instances {
		// Lock-free snapshot replaces both the unguarded GitHubOwner/GitHubRepo reads and
		// the explicit stateMutex.RLock() for GitHubPRStatusTerminal / GitHubIsFork.
		instSnap := inst.Snapshot()

		if instSnap.GitHub.GitHubOwner == "" || instSnap.GitHub.GitHubRepo == "" {
			continue // no GitHub info for this session
		}

		if instSnap.GitHub.GitHubPRStatusTerminal {
			continue // merged/closed; poller already marked it terminal
		}
		if instSnap.GitHub.GitHubIsFork {
			log.Info("PR status poller: skipping fork session (upstream PR lookup Phase 2)", "session", instSnap.Title)
			continue
		}

		if pollAfter, ok := noPRPollAfter[instSnap.Title]; ok && now.Before(pollAfter) {
			continue // no-PR backoff still in effect
		}

		captured := inst
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			p.fetchAndUpdatePRStatus(captured)
		}()
	}

	wg.Wait()
}

// isAuthOK returns true if gh auth check passes, using a time-based cache.
// Auth state is stored in an atomic.Value so this read path is lock-free.
func (p *PRStatusPoller) isAuthOK() bool {
	// Fast path: cached result still fresh (lock-free read).
	if v := p.authState.Load(); v != nil {
		if r := v.(pollerAuthResult); time.Since(r.checkedAt) < p.config.AuthCacheDuration {
			return r.ok
		}
	}

	if err := github.CheckGHAuth(); err != nil {
		log.Warn("PR status poller: github auth unavailable", "err", err)
		p.authState.Store(pollerAuthResult{ok: false, checkedAt: time.Now()})
		return false
	}

	p.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})
	return true
}

// fetchAndUpdatePRStatus fetches fresh PR status for one instance and applies it.
func (p *PRStatusPoller) fetchAndUpdatePRStatus(inst *Instance) {
	ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)
	defer cancel()

	// Use Snapshot() — actor-based writes (SetGitHubPRNumber etc.) do not hold mu,
	// so mu.RLock would not synchronize with them.
	snap := inst.Snapshot()
	prNumber := snap.GitHub.GitHubPRNumber
	owner := snap.GitHub.GitHubOwner
	repo := snap.GitHub.GitHubRepo

	// Auto-discovery: find PR for branch when PR number not yet known.
	// CurrentBranch() reads live from git for directory sessions (Branch field is empty).
	if prNumber == 0 {
		branch := inst.CurrentBranch()
		if branch == "" {
			return
		}
		listKey := owner + "/" + repo + "/" + branch
		var listEtag string
		if v, ok := p.listEtags.Load(listKey); ok {
			listEtag = v.(string)
		}
		prInfo, newEtag, changed, err := github.GetPRForBranchConditional(ctx, owner, repo, branch, listEtag)
		if newEtag != "" {
			p.listEtags.Store(listKey, newEtag)
		}
		if !changed {
			// 304: branch list unchanged, still no PR — re-arm backoff to avoid constant polling.
			if p.config.NoPRBackoff > 0 {
				p.mu.Lock()
				p.noPRPollAfter[inst.Title] = time.Now().Add(p.config.NoPRBackoff)
				p.mu.Unlock()
			}
			return
		}
		if err != nil {
			if errors.Is(err, github.ErrNoPR) {
				p.applyNoPR(inst)
				return
			}
			if p.handleFetchError(err) {
				return
			}
			log.Warn("PR status poller: PR discovery failed", "session", inst.Title, "owner", owner, "repo", repo, "branch", branch, "err", err)
			return
		}
		// Persist discovered PR number and clear no-PR backoff.
		inst.SetGitHubPRNumber(prInfo.Number)
		p.mu.Lock()
		delete(p.noPRPollAfter, inst.Title)
		p.mu.Unlock()
		if p.storage != nil {
			if err := p.storage.UpdateInstancePRNumber(inst.Title, prInfo.Number); err != nil {
				log.Warn("PR status poller: failed to persist PR number", "session", inst.Title, "err", err)
			}
		}
		p.applyPRUpdate(inst, prInfo)
		return
	}

	// Conditional fetch using ETag cache (304 = no change)
	prInfo, changed, err := github.GetPRInfoConditional(ctx, owner, repo, prNumber, p.etagCache)
	if err != nil {
		if p.handleFetchError(err) {
			return
		}
		log.Warn("PR status poller: failed to fetch PR", "pr", prNumber, "session", inst.Title, "err", err)
		return
	}

	if !changed {
		// 304 Not Modified — PR unchanged; just bump the check timestamp
		inst.SetLastPRStatusCheck(time.Now())
		return
	}

	p.applyPRUpdate(inst, prInfo)
}

// handleFetchError inspects an error for rate limits and auth failures.
// Returns true if the error was handled (caller should not log separately).
// Rate-limit state is managed by github.DefaultRateLimiter (updated by the
// transport); this method only needs to detect the error type and signal auth
// cache invalidation.
func (p *PRStatusPoller) handleFetchError(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "429") {
		log.Warn("PR status poller: github rate limit hit")
		return true
	}
	if strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") {
		log.Warn("PR status poller: github auth error, invalidating auth cache")
		p.authState.Store(pollerAuthResult{ok: false, checkedAt: time.Now()})
		return true
	}
	return false
}

// applyNoPR sets the session to no_pr state (branch has no PR yet) and
// schedules a backoff so the branch is not re-queried every poll tick.
func (p *PRStatusPoller) applyNoPR(inst *Instance) {
	if p.config.NoPRBackoff > 0 {
		p.mu.Lock()
		p.noPRPollAfter[inst.Title] = time.Now().Add(p.config.NoPRBackoff)
		p.mu.Unlock()
	}
	p.applyPRUpdate(inst, nil)
}

// applyPRUpdate derives priority from prInfo (nil = no_pr) and persists the result.
func (p *PRStatusPoller) applyPRUpdate(inst *Instance, prInfo *github.PRInfo) {
	priority := string(github.DerivePRPriority(prInfo))
	terminal := github.IsTerminal(github.PRPriority(priority))

	state, checkConclusion := "", ""
	approvedCount, changesReqCount := 0, 0
	isDraft := false
	if prInfo != nil {
		state = prInfo.State
		checkConclusion = prInfo.CheckConclusion
		approvedCount = prInfo.ApprovedCount
		changesReqCount = prInfo.ChangesRequestedCount
		isDraft = prInfo.IsDraft
	}

	result := inst.UpdatePRStatus(state, priority, checkConclusion, approvedCount, changesReqCount, isDraft, terminal)

	if p.storage != nil {
		if err := p.storage.UpdateInstancePRStatus(inst.Title, state, priority, checkConclusion,
			approvedCount, changesReqCount, isDraft, terminal); err != nil {
			log.Warn("PR status poller: failed to persist PR status", "session", inst.Title, "err", err)
		}
	}

	// Fire on either a priority change or a CI-conclusion-only change (Task 3.2.1a) — a
	// conclusion flip that doesn't cross a priority boundary (e.g. pending -> failure with
	// priority already "blocking") would otherwise leave the diff-viewer badge stale
	// between poll ticks. Neither changed: no event, per the changed-only-publish guard.
	if result.PriorityChanged || result.CheckConclusionChanged {
		p.mu.RLock()
		onUpdated := p.onUpdated
		p.mu.RUnlock()
		if onUpdated != nil {
			onUpdated(inst)
		}
		if result.PriorityChanged {
			log.Info("PR status poller: PR priority changed", "session", inst.Title, "new", priority)
		}
		if result.CheckConclusionChanged {
			log.Info("PR status poller: CI check conclusion changed", "session", inst.Title, "new", checkConclusion)
		}
	}
}
