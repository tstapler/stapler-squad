package session

import (
	"context"
	"sync"
	"time"

	"github.com/linkdata/deadlock"
	"github.com/puzpuzpuz/xsync/v4"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session/headless"
)

// SessionTagPollerConfig contains configuration for SessionTagClassificationPoller,
// mirroring PRStatusPollerConfig's field shape (session/pr_status_poller.go).
type SessionTagPollerConfig struct {
	// PollInterval controls how often all sessions are checked.
	PollInterval time.Duration
	// ConcurrentCalls limits simultaneous headless LLM calls.
	ConcurrentCalls int
	// CallTimeout is the maximum time for a single GenerateSessionTags call.
	CallTimeout time.Duration
}

// DefaultSessionTagPollerConfig returns sensible defaults.
func DefaultSessionTagPollerConfig() SessionTagPollerConfig {
	return SessionTagPollerConfig{
		PollInterval:    2 * time.Minute,
		ConcurrentCalls: 3,
		CallTimeout:     60 * time.Second,
	}
}

// cachedTagResult is the poller's per-session cache entry: the content hash the tags were
// derived from, plus the tags themselves (informational only — the poller re-derives the
// vocabulary and re-applies to the live Instance on every hash change, never replaying tags
// straight from the cache).
type cachedTagResult struct {
	hash string
	tags []string
}

// SessionTagClassificationPoller polls all monitored sessions at a shared interval and, for
// any session whose classifier.TagContentHash has changed since the last successful or failed
// classification, calls headless.GenerateSessionTags to (re)classify it. Mirrors PRStatusPoller's
// shape (build-vs-buy.md 4a) rather than forking a new poller idiom.
type SessionTagClassificationPoller struct {
	instances []*Instance
	config    SessionTagPollerConfig
	pool      headless.PoolClient
	// engine is stored (not a derived vocabulary field) so the vocabulary is recomputed from
	// the live rule set on every tick — a CRUD-added OutputTag must reach the LLM call without
	// a poller restart (pre-mortem.md Failure #3, P2).
	engine *classifier.TaggingEngine
	// cache is keyed by session title; ponytail: xsync.Map for lock-free reads across sessions.
	cache *xsync.Map[string, cachedTagResult]

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     deadlock.RWMutex
}

// NewSessionTagClassificationPoller creates a new poller with the given headless pool and
// TaggingEngine, using DefaultSessionTagPollerConfig().
func NewSessionTagClassificationPoller(pool headless.PoolClient, engine *classifier.TaggingEngine) *SessionTagClassificationPoller {
	return NewSessionTagClassificationPollerWithConfig(pool, engine, DefaultSessionTagPollerConfig())
}

// NewSessionTagClassificationPollerWithConfig creates a poller with custom configuration.
func NewSessionTagClassificationPollerWithConfig(pool headless.PoolClient, engine *classifier.TaggingEngine, config SessionTagPollerConfig) *SessionTagClassificationPoller {
	return &SessionTagClassificationPoller{
		instances: make([]*Instance, 0),
		config:    config,
		pool:      pool,
		engine:    engine,
		cache:     xsync.NewMap[string, cachedTagResult](),
	}
}

// SetInstances replaces the full list of monitored instances.
func (p *SessionTagClassificationPoller) SetInstances(instances []*Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instances = instances
}

// AddInstance adds a single instance to monitor.
func (p *SessionTagClassificationPoller) AddInstance(inst *Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.instances = append(p.instances, inst)
}

// RemoveInstance removes an instance from monitoring and evicts its cache entry, mirroring
// pollerContentProvider.EvictInstance (session/review_queue_poller.go).
func (p *SessionTagClassificationPoller) RemoveInstance(title string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	filtered := make([]*Instance, 0, len(p.instances))
	for _, inst := range p.instances {
		if !inst.MatchesID(title) {
			filtered = append(filtered, inst)
		}
	}
	p.instances = filtered
	p.cache.Delete(title)
}

// Start begins the polling loop. Safe to call multiple times; subsequent calls are no-ops.
func (p *SessionTagClassificationPoller) Start(ctx context.Context) {
	p.mu.Lock()
	if p.ctx != nil {
		p.mu.Unlock()
		return
	}
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.wg.Add(1)
	p.mu.Unlock()

	go p.pollLoop()
	log.Info("session tag classification poller started", "interval", p.config.PollInterval, "concurrency", p.config.ConcurrentCalls)
}

// Stop gracefully shuts down the poller and waits for in-flight calls.
func (p *SessionTagClassificationPoller) Stop() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.ctx, p.cancel = nil, nil
	p.mu.Unlock()
	p.wg.Wait()
	log.Info("session tag classification poller stopped")
}

// pollLoop runs the main ticker loop.
func (p *SessionTagClassificationPoller) pollLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	p.pollOnce()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce()
		}
	}
}

// currentVocabulary derives the accepted tag vocabulary fresh from the live engine's rule set
// on every call — never cached as a poller field — so a CRUD-added OutputTag (Story 2.3.2)
// reaches the LLM call on the very next tick, with no poller restart.
func currentVocabulary(engine *classifier.TaggingEngine) []string {
	seen := make(map[string]bool)
	vocabulary := make([]string, 0)
	for _, rule := range engine.Rules() {
		if rule.OutputTag == "" || seen[rule.OutputTag] {
			continue
		}
		seen[rule.OutputTag] = true
		vocabulary = append(vocabulary, rule.OutputTag)
	}
	if !seen[UnclassifiedTag] {
		vocabulary = append(vocabulary, UnclassifiedTag)
	}
	return vocabulary
}

// nonLLMOwnedTags returns tags with every entry whose RuleTagProvenance is llmSentinelRuleID
// removed, preserving order. See dispatchIfChanged's hash-gating comment for why: the content
// hash must be invariant to this poller's own prior output, or every successful classification
// would immediately invalidate its own cache entry.
func nonLLMOwnedTags(tags []string, provenance map[string]string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if provenance[tag] == llmSentinelRuleID {
			continue
		}
		out = append(out, tag)
	}
	return out
}

// pollOnce runs exactly one classification pass over every monitored instance. Extracted from
// pollLoop's ticker wrapper so tests can invoke a single tick synchronously with no real sleep
// (deterministic-fast-tests).
func (p *SessionTagClassificationPoller) pollOnce() {
	p.mu.RLock()
	instances := make([]*Instance, len(p.instances))
	copy(instances, p.instances)
	ctx := p.ctx
	p.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}

	vocabulary := currentVocabulary(p.engine)

	sem := make(chan struct{}, p.config.ConcurrentCalls)
	var wg sync.WaitGroup

	for _, inst := range instances {
		p.dispatchIfChanged(ctx, inst, vocabulary, sem, &wg)
	}

	wg.Wait()
}

// dispatchIfChanged checks a single instance's content hash against the cache and, if unseen
// or changed, launches classifyOne on wg/sem — the per-instance body extracted out of pollOnce
// to keep that method under the file's line-count lint budget.
func (p *SessionTagClassificationPoller) dispatchIfChanged(ctx context.Context, inst *Instance, vocabulary []string, sem chan struct{}, wg *sync.WaitGroup) {
	snap := inst.Snapshot()
	if snap.Status.IsSuspended() {
		return // paused/hibernated/stopped/crashed/permanently-failed: nothing to classify
	}

	meta := classifier.SessionTaggingContext{
		Name:    snap.Title,
		Branch:  snap.Branch,
		Path:    snap.Path,
		Program: snap.Program,
		Tags:    snap.Tags,
	}
	// The hash used for cache-gating excludes tags this poller itself previously applied
	// (RuleTagProvenance == llmSentinelRuleID) — otherwise every successful classification
	// would change Tags, which would change the hash, which would force a re-classification
	// on the very next tick, forever. Sync-rule-owned and plain user tags still count as real
	// content that can legitimately warrant re-classification.
	hashCtx := meta
	hashCtx.Tags = nonLLMOwnedTags(snap.Tags, snap.RuleTagProvenance)
	hash := classifier.TagContentHash(hashCtx)

	if cached, ok := p.cache.Load(snap.Title); ok && cached.hash == hash {
		log.Info("session tag poller: LLM classification", "session", snap.Title, "outcome", "cache_hit", "tags", cached.tags)
		return // unchanged since last classification (Success Metric requirement)
	}

	wg.Add(1)
	sem <- struct{}{}
	go func() {
		defer wg.Done()
		defer func() { <-sem }()
		p.classifyOne(ctx, inst, meta, hash, vocabulary)
	}()
}

// classifyOne calls GenerateSessionTags for a single instance, applies the result (or
// UnclassifiedTag on failure) via Instance.ApplyLLMTagResult, and updates the cache entry
// regardless of success/failure — a failed call must not be retried every tick.
func (p *SessionTagClassificationPoller) classifyOne(ctx context.Context, inst *Instance, meta classifier.SessionTaggingContext, hash string, vocabulary []string) {
	callCtx, cancel := context.WithTimeout(ctx, p.config.CallTimeout)
	defer cancel()

	start := time.Now()
	tags, cost, err := headless.GenerateSessionTags(callCtx, p.pool, meta, vocabulary)
	latency := time.Since(start)
	outcome := "applied"
	if err != nil {
		log.Warn("session tag classification poller: GenerateSessionTags failed", "session", meta.Name, "err", err)
		tags = []string{UnclassifiedTag}
		outcome = "failed_unclassified"
	}
	log.Info("session tag poller: LLM classification", "session", meta.Name, "outcome", outcome, "tags", tags, "cost_usd", cost, "latency_ms", latency.Milliseconds())

	inst.ApplyLLMTagResult(tags, llmSentinelRuleID)
	p.cache.Store(meta.Name, cachedTagResult{hash: hash, tags: tags})
}
