package session

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

// pollTrigger says why a poll pass started. Scanner completions arrive every
// few seconds under fsnotify and must not each cost a GitHub request per PR;
// the PollInterval ticker is the guaranteed fallback and always polls.
type pollTrigger int

const (
	pollTriggerScan pollTrigger = iota
	pollTriggerTick
)

// polledState remembers when a worktree key was last dispatched and its
// fingerprint at that time.
type polledState struct {
	at          time.Time
	fingerprint string
	failures    int // consecutive fetch errors; drives exponential retry delay
}

// failureBackoffBase is the first retry delay after a fetch error (shorter than
// a scan interval, so a one-off failure is retried on the next scan). It
// doubles per consecutive failure up to PollInterval, which bounds the request
// rate for permanently failing worktrees (404/403, deleted repo, SSO block).
const failureBackoffBase = 5 * time.Second

var (
	pollOutcomeCounterOnce sync.Once
	pollOutcomeCounter     metric.Int64Counter
)

// recordPollOutcome counts poll decisions; outcome is one of polled,
// skipped_min_interval, skipped_duplicate (low cardinality).
func recordPollOutcome(ctx context.Context, outcome string) {
	pollOutcomeCounterOnce.Do(func() {
		var err error
		pollOutcomeCounter, err = telemetry.GetMeter().Int64Counter("worktree_pr_poller.poll_total",
			metric.WithDescription("Worktree PR poller per-worktree decisions by outcome"))
		if err != nil {
			log.Error("worktree PR poller: failed to register poll_total", "error", err)
		}
	})
	if pollOutcomeCounter != nil {
		pollOutcomeCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
	}
}

// WorktreeScanItem is the minimal worktree info the poller needs from the
// unfinished-work scanner. Using a local struct avoids an import cycle:
//
//	session → session/unfinished → pkg/events → session
//
// The server layer bridges the two packages via WorktreeSource (adapter pattern).
type WorktreeScanItem struct {
	RepoPath     string
	Branch       string
	WorktreePath string
	// Fingerprint changes whenever the worktree's HEAD/commit state changes
	// (empty if the source cannot tell). A changed fingerprint bypasses the
	// per-PR minimum interval so new commits are polled promptly.
	Fingerprint string
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
	// NoPRBackoff is how long to skip a branch after its PR list returns empty.
	// Zero disables the backoff.
	NoPRBackoff time.Duration
}

// DefaultWorktreePRPollerConfig returns sensible defaults matching PRStatusPoller.
func DefaultWorktreePRPollerConfig() WorktreePRPollerConfig {
	return WorktreePRPollerConfig{
		PollInterval:      60 * time.Second,
		CallTimeout:       10 * time.Second,
		AuthCacheDuration: 5 * time.Minute,
		NoPRBackoff:       5 * time.Minute,
	}
}

// listCacheEntry records the ETag and last result for a branch→PR list API call.
type listCacheEntry struct {
	etag string
	noPR bool // true if the last list response contained no PRs
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
//   - rateLimitedUntil and noPRPollAfter are guarded by mu
type WorktreePRPoller struct {
	source       WorktreeSource
	prPoller     *PRStatusPoller // for session-backed-path exclusion
	etagCache    *github.ETagCache
	config       WorktreePRPollerConfig
	data         sync.Map     // key: worktreeCacheKey(repoPath, branch), value: *github.PRInfo
	listEtags    sync.Map     // key: worktreeCacheKey, value: listCacheEntry
	authState    atomic.Value //nolint:exhaustruct // stores pollerAuthResult
	onUpdatedVal atomic.Value //nolint:exhaustruct // stores worktreeOnUpdatedFn

	mu            sync.Mutex
	noPRPollAfter map[string]time.Time // key: worktreeCacheKey
	lastPolled    map[string]polledState
	keyRepo       map[string]string // worktree key -> lowercase owner/repo, for targeted InvalidateCache

	// now and fetch are test seams (fake clock, fake GitHub fetch).
	now   func() time.Time
	fetch func(ctx context.Context, item WorktreeScanItem) error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorktreePRPoller creates a WorktreePRPoller with default configuration.
// source may be nil at construction time; call SetSource before Start.
func NewWorktreePRPoller(etagCache *github.ETagCache, prPoller *PRStatusPoller) *WorktreePRPoller {
	return NewWorktreePRPollerWithConfig(etagCache, prPoller, DefaultWorktreePRPollerConfig())
}

// NewWorktreePRPollerWithConfig creates a WorktreePRPoller with custom configuration.
func NewWorktreePRPollerWithConfig(etagCache *github.ETagCache, prPoller *PRStatusPoller, cfg WorktreePRPollerConfig) *WorktreePRPoller {
	p := &WorktreePRPoller{
		etagCache:     etagCache,
		prPoller:      prPoller,
		config:        cfg,
		noPRPollAfter: make(map[string]time.Time),
		lastPolled:    make(map[string]polledState),
		keyRepo:       make(map[string]string),
		now:           time.Now,
	}
	p.fetch = p.fetchAndStore
	return p
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

// Wait blocks until the poll loop launched by Start has exited. Unlike Stop it
// does not cancel anything; call it after cancelling Start's ctx.
func (p *WorktreePRPoller) Wait() {
	p.wg.Wait()
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

// InvalidateCache clears the shared ETag cache entry for a PR so the next
// scheduled tick (ticker or ScanDone()) performs a full fetch instead of a
// 304. It does not dispatch an immediate refetch: WorktreePRPoller has no
// persistent per-worktree index to target one, so up to one PollInterval of
// staleness is an accepted trade-off for worktrees with no active session.
func (p *WorktreePRPoller) InvalidateCache(owner, repo string, prNumber int) {
	// Host unknown at this call site (webhook's repoFullName carries no host) —
	// github.com is the safe default; see poller_invalidation_adapter.go.
	ref, err := github.NewRepoRef(owner, repo)
	if err != nil {
		return
	}
	if p.etagCache != nil {
		p.etagCache.Invalidate(ref, prNumber)
	}

	// Re-arm only worktrees of this repo (host-agnostic: the webhook carries
	// none) so the next scan re-polls them; other repos stay gated.
	want := repoID(owner, repo)
	p.mu.Lock()
	for key, id := range p.keyRepo {
		if id != want {
			continue
		}
		delete(p.noPRPollAfter, key) // a webhook may mean a PR was just opened
		// A failing key keeps its retry schedule so a webhook stream cannot
		// defeat the failure backoff.
		if st, ok := p.lastPolled[key]; ok && st.failures == 0 {
			delete(p.lastPolled, key)
		}
	}
	p.mu.Unlock()
}

func repoID(owner, repo string) string {
	return strings.ToLower(owner + "/" + repo)
}

// noteRepo records which GitHub repo a worktree key belongs to.
func (p *WorktreePRPoller) noteRepo(key, owner, repo string) {
	p.mu.Lock()
	p.keyRepo[key] = repoID(owner, repo)
	p.mu.Unlock()
}

// pollLoop drives the poller: react to scanner completions and a fallback ticker.
func (p *WorktreePRPoller) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	// Seed immediately if source is already available.
	if p.source != nil {
		p.pollWorktrees(p.source.GetWorktrees(), pollTriggerTick)
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
			p.pollWorktrees(p.source.GetWorktrees(), pollTriggerScan)
		case <-ticker.C:
			p.pollWorktrees(p.source.GetWorktrees(), pollTriggerTick)
		}
	}
}

// pollWorktrees iterates scan results, skips session-backed worktrees, and
// fetches GitHub PR data for the remainder. Scan-triggered passes poll a
// worktree at most once per PollInterval unless its fingerprint changed;
// ticker passes always poll.
func (p *WorktreePRPoller) pollWorktrees(items []WorktreeScanItem, trigger pollTrigger) {
	if limited, until := github.DefaultRateLimiter.IsLimited(); limited {
		log.Info("worktree PR poller: rate limited, skipping tick", "until", until)
		return
	}

	p.mu.Lock()
	noPRPollAfter := make(map[string]time.Time, len(p.noPRPollAfter))
	for k, v := range p.noPRPollAfter {
		noPRPollAfter[k] = v
	}
	p.mu.Unlock()

	if !p.isAuthOK() {
		return
	}

	sessionPaths := p.sessionBackedPaths()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // match PRStatusPoller default concurrency

	now := p.now()
	seen := make(map[string]struct{}, len(items))
	eligible := make([]WorktreeScanItem, 0, len(items))
	for _, item := range items {
		if _, owned := sessionPaths[item.WorktreePath]; owned {
			continue // PRStatusPoller covers this one
		}
		if item.Branch == "" || item.RepoPath == "" {
			continue
		}
		eligible = append(eligible, item)
	}
	fingerprints := combinedFingerprints(eligible)
	for _, item := range eligible {

		key := worktreeCacheKey(item.RepoPath, item.Branch)
		if pollAfter, ok := noPRPollAfter[key]; ok && now.Before(pollAfter) {
			seen[key] = struct{}{} // keep its lastPolled/keyRepo so InvalidateCache can lift the backoff
			continue               // no-PR backoff still in effect
		}
		if _, dup := seen[key]; dup {
			recordPollOutcome(p.ctx, "skipped_duplicate") // same repo+branch from another worktree
			continue
		}
		seen[key] = struct{}{}
		if !p.claimPoll(key, fingerprints[key], now, trigger) {
			recordPollOutcome(p.ctx, "skipped_min_interval")
			continue
		}
		recordPollOutcome(p.ctx, "polled")

		captured := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)
			defer cancel()
			ctx = github.WithGitHubCallOrigin(ctx, github.OriginWorktreePRPoller)
			p.recordResult(key, p.fetch(ctx, captured))
		}()
	}
	p.pruneLastPolled(seen)
	wg.Wait()
}

// combinedFingerprints merges the fingerprints of all worktrees sharing a
// repo+branch key (order-independent), so a change in any of them counts.
func combinedFingerprints(items []WorktreeScanItem) map[string]string {
	byKey := make(map[string][]string, len(items))
	for _, it := range items {
		k := worktreeCacheKey(it.RepoPath, it.Branch)
		byKey[k] = append(byKey[k], it.Fingerprint)
	}
	out := make(map[string]string, len(byKey))
	for k, fps := range byKey {
		sort.Strings(fps)
		out[k] = strings.Join(fps, "\x00")
	}
	return out
}

// recordResult resets key's failure streak on success, or schedules an
// exponentially growing retry (capped at PollInterval) on error by backdating
// the poll record so the interval gate reopens after the delay.
func (p *WorktreePRPoller) recordResult(key string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.lastPolled[key]
	if !ok {
		return // invalidated or pruned meanwhile; next scan polls anyway
	}
	if err == nil {
		st.failures = 0
		p.lastPolled[key] = st
		return
	}
	st.failures++
	delay := failureBackoffBase
	for i := 1; i < st.failures && delay < p.config.PollInterval; i++ {
		delay *= 2
	}
	delay = min(delay, p.config.PollInterval)
	st.at = p.now().Add(delay - p.config.PollInterval)
	p.lastPolled[key] = st
}

// claimPoll reports whether key should be polled now and, if so, records it.
func (p *WorktreePRPoller) claimPoll(key, fingerprint string, now time.Time, trigger pollTrigger) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if prev, ok := p.lastPolled[key]; ok && trigger == pollTriggerScan &&
		prev.fingerprint == fingerprint && now.Sub(prev.at) < p.config.PollInterval {
		return false
	}
	p.lastPolled[key] = polledState{at: now, fingerprint: fingerprint, failures: p.lastPolled[key].failures}
	return true
}

// pruneLastPolled drops entries for worktrees no longer reported.
func (p *WorktreePRPoller) pruneLastPolled(live map[string]struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k := range p.lastPolled {
		if _, ok := live[k]; !ok {
			delete(p.lastPolled, k)
		}
	}
	for k := range p.keyRepo {
		if _, ok := live[k]; !ok {
			delete(p.keyRepo, k)
		}
	}
}

// fetchAndStore fetches PR info for one worktree and stores it in the cache.
func (p *WorktreePRPoller) fetchAndStore(ctx context.Context, item WorktreeScanItem) error {
	repoRef, err := github.GetOwnerRepoFromRemote(item.RepoPath, enterpriseHostsForRemoteParsing())
	if err != nil {
		log.Warn("worktree PR poller: could not read remote URL", "path", item.RepoPath, "err", err)
		return nil // also covers transient git exec/lock errors: they wait a full PollInterval, which paces the warning
	}
	if !repoRef.IsValid() {
		return nil // not a GitHub remote
	}

	key := worktreeCacheKey(item.RepoPath, item.Branch)
	p.noteRepo(key, repoRef.Owner(), repoRef.Repo())

	// Use ETag conditional fetch when we already know the PR number.
	if existing := p.GetPRData(item.RepoPath, item.Branch); existing != nil && existing.Number > 0 {
		info, changed, fetchErr := github.GetPRInfoConditional(ctx, repoRef, existing.Number, p.etagCache)
		if fetchErr != nil {
			if !p.handleFetchError(fetchErr) {
				log.Warn("worktree PR poller: failed to fetch PR status", "branch", item.Branch, "err", fetchErr)
			}
			return fetchErr
		}
		if changed && info != nil {
			p.data.Store(key, info)
			p.fireOnUpdated(item.RepoPath, item.Branch, info)
		}
		return nil
	}

	// Discovery: find PR by branch name, using ETag to avoid redundant list calls.
	var listEtag string
	if v, ok := p.listEtags.Load(key); ok {
		listEtag = v.(listCacheEntry).etag
	}

	info, newEtag, changed, fetchErr := github.GetPRForBranchConditional(ctx, repoRef, item.Branch, listEtag)
	if changed && newEtag != "" {
		p.listEtags.Store(key, listCacheEntry{etag: newEtag, noPR: errors.Is(fetchErr, github.ErrNoPR)})
	}
	if fetchErr != nil {
		if errors.Is(fetchErr, github.ErrNoPR) {
			p.setNoPRBackoff(key)
			return nil
		}
		if !p.handleFetchError(fetchErr) {
			log.Warn("worktree PR poller: PR discovery failed", "branch", item.Branch, "err", fetchErr)
		}
		return fetchErr
	}
	if !changed {
		// 304 Not Modified: re-apply no-PR backoff if the cached result had no PR.
		if v, ok := p.listEtags.Load(key); ok && v.(listCacheEntry).noPR {
			p.setNoPRBackoff(key)
		}
		return nil
	}
	p.data.Store(key, info)
	p.fireOnUpdated(item.RepoPath, item.Branch, info)
	return nil
}

// setNoPRBackoff records that a branch has no PR and skips it for NoPRBackoff duration.
func (p *WorktreePRPoller) setNoPRBackoff(key string) {
	if p.config.NoPRBackoff <= 0 {
		return
	}
	p.mu.Lock()
	p.noPRPollAfter[key] = p.now().Add(p.config.NoPRBackoff)
	p.mu.Unlock()
}

// isAuthOK returns true if GitHub auth passes, using a time-based atomic cache.
func (p *WorktreePRPoller) isAuthOK() bool {
	if v := p.authState.Load(); v != nil {
		if r := v.(pollerAuthResult); time.Since(r.checkedAt) < p.config.AuthCacheDuration {
			return r.ok
		}
	}
	if err := github.CheckGHAuth(p.ctx); err != nil {
		log.Warn("worktree PR poller: github auth unavailable", "err", err)
		p.authState.Store(pollerAuthResult{ok: false, checkedAt: time.Now()})
		return false
	}
	p.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})
	return true
}

// handleFetchError inspects an error for rate limits and auth failures.
// Returns true if the error was handled (caller should not log separately).
// Rate-limit state is managed by github.DefaultRateLimiter (updated by the
// transport); this method only needs to detect the error type.
func (p *WorktreePRPoller) handleFetchError(err error) bool {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "429"):
		log.Warn("worktree PR poller: rate limit hit")
		return true
	case strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized"):
		log.Warn("worktree PR poller: auth error, invalidating cache")
		p.authState.Store(pollerAuthResult{ok: false, checkedAt: time.Now()})
		return true
	default:
		return false
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
		if p := inst.GetPath(); p != "" {
			paths[p] = struct{}{}
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
