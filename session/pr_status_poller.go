package session

import "github.com/linkdata/deadlock"

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

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

	// Cached auth check state.
	authOK        bool
	authCheckedAt time.Time

	// Pause polling when rate limited.
	rateLimitedUntil time.Time

	// noPRPollAfter tracks the earliest time at which we should re-check a
	// session that had no PR on the previous poll. Keyed by session title.
	// Guarded by mu.
	noPRPollAfter map[string]time.Time

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

// SetInstances replaces the full list of monitored instances.
func (p *PRStatusPoller) SetInstances(instances []*Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instances = instances
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
	p.mu.RLock()
	instances := make([]*Instance, len(p.instances))
	copy(instances, p.instances)
	rateLimitedUntil := p.rateLimitedUntil
	p.mu.RUnlock()

	if time.Now().Before(rateLimitedUntil) {
		log.Info("PR status poller: rate limited, skipping tick", "until", rateLimitedUntil)
		return
	}

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
		if inst.GitHubOwner == "" || inst.GitHubRepo == "" {
			continue // no GitHub info for this session
		}

		inst.stateMutex.RLock()
		isTerminal := inst.GitHubPRStatusTerminal
		isFork := inst.GitHubIsFork
		inst.stateMutex.RUnlock()

		if isTerminal {
			continue // merged/closed; poller already marked it terminal
		}
		if isFork {
			log.Info("PR status poller: skipping fork session (upstream PR lookup Phase 2)", "session", inst.Title)
			continue
		}

		if pollAfter, ok := noPRPollAfter[inst.Title]; ok && now.Before(pollAfter) {
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
func (p *PRStatusPoller) isAuthOK() bool {
	p.mu.RLock()
	cached := p.authOK && time.Since(p.authCheckedAt) < p.config.AuthCacheDuration
	p.mu.RUnlock()
	if cached {
		return true
	}

	if err := github.CheckGHAuth(); err != nil {
		log.Warn("PR status poller: github auth unavailable", "err", err)
		p.mu.Lock()
		p.authOK = false
		p.authCheckedAt = time.Now()
		p.mu.Unlock()
		return false
	}

	p.mu.Lock()
	p.authOK = true
	p.authCheckedAt = time.Now()
	p.mu.Unlock()
	return true
}

// fetchAndUpdatePRStatus fetches fresh PR status for one instance and applies it.
func (p *PRStatusPoller) fetchAndUpdatePRStatus(inst *Instance) {
	ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)
	defer cancel()

	inst.stateMutex.RLock()
	prNumber := inst.GitHubPRNumber
	branch := inst.Branch
	owner := inst.GitHubOwner
	repo := inst.GitHubRepo
	inst.stateMutex.RUnlock()

	// Auto-discovery: find PR for branch when PR number not yet known
	if prNumber == 0 {
		if branch == "" {
			return
		}
		prInfo, err := github.GetPRForBranch(ctx, owner, repo, branch)
		if err != nil {
			if errors.Is(err, github.ErrNoPR) {
				// No PR exists yet for this branch
				p.applyNoPR(inst)
				return
			}
			if p.handleFetchError(err) {
				return // rate limit or auth error handled
			}
			log.Warn("PR status poller: PR discovery failed", "session", inst.Title, "owner", owner, "repo", repo, "branch", branch, "err", err)
			return
		}
		// Persist discovered PR number and clear no-PR backoff.
		inst.stateMutex.Lock()
		inst.GitHubPRNumber = prInfo.Number
		inst.stateMutex.Unlock()
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
		inst.stateMutex.Lock()
		inst.LastPRStatusCheck = time.Now()
		inst.stateMutex.Unlock()
		return
	}

	p.applyPRUpdate(inst, prInfo)
}

// handleFetchError inspects an error and updates poller state for rate limits / auth failures.
// Returns true if the error requires aborting the current session fetch.
func (p *PRStatusPoller) handleFetchError(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "429") {
		log.Warn("PR status poller: github rate limit hit, pausing for 60s")
		p.mu.Lock()
		p.rateLimitedUntil = time.Now().Add(60 * time.Second)
		p.mu.Unlock()
		return true
	}
	if strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") {
		log.Warn("PR status poller: github auth error, invalidating auth cache")
		p.mu.Lock()
		p.authOK = false
		p.authCheckedAt = time.Now()
		p.mu.Unlock()
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

	// Check whether priority actually changed before notifying
	inst.stateMutex.RLock()
	oldPriority := inst.GitHubPRPriority
	inst.stateMutex.RUnlock()

	inst.UpdatePRStatus(state, priority, checkConclusion, approvedCount, changesReqCount, isDraft, terminal)

	if p.storage != nil {
		if err := p.storage.UpdateInstancePRStatus(inst.Title, state, priority, checkConclusion,
			approvedCount, changesReqCount, isDraft, terminal); err != nil {
			log.Warn("PR status poller: failed to persist PR status", "session", inst.Title, "err", err)
		}
	}

	if priority != oldPriority {
		p.mu.RLock()
		onUpdated := p.onUpdated
		p.mu.RUnlock()
		if onUpdated != nil {
			onUpdated(inst)
		}
		log.Info("PR status poller: PR priority changed", "session", inst.Title, "old", oldPriority, "new", priority)
	}
}
