package session

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	// CallTimeout is the maximum time for a single batch classification call.
	CallTimeout time.Duration
	// MaxBatchSessions caps how many changed sessions a single tick classifies in its one
	// batch LLM call. Surplus sessions defer to later ticks — natural backpressure that
	// bounds spend to one batch call per PollInterval no matter how many sessions churn.
	MaxBatchSessions int
	// MinReclassifyInterval is the retry backoff for degraded (failed) classifications: a
	// session whose last classification degraded is not retried until this elapses, so a
	// persistently-failing setup (bad key, proxy down) cannot burn a batch slot every tick.
	// Successfully applied classifications are never retried automatically (classify-once) —
	// re-running one is an explicit user action (ClassifyNow / ReclassifySessionTags RPC).
	MinReclassifyInterval time.Duration
	// Model is the primary LLM model for batch classification calls (passed as --model to
	// the headless CLI). Empty means "haiku" (historical default).
	Model string
	// FallbackModels is the ordered fallback hierarchy tried after Model fails: the first
	// model whose call succeeds and parses wins. This is the free-proxy story — e.g.
	// Model "sonnet" with FallbackModels ["proxy-free-model"] keeps classifying when the
	// paid tier is unreachable, at zero marginal cost when the proxy reports 0.
	FallbackModels []string
}

// DefaultSessionTagPollerConfig returns sensible defaults: at most one batch call (ten
// sessions) every 2 minutes — 30 calls/hour worst case — via haiku, with failed
// classifications retried no more often than every 30 minutes and successful ones never
// retried automatically.
func DefaultSessionTagPollerConfig() SessionTagPollerConfig {
	return SessionTagPollerConfig{
		PollInterval:          2 * time.Minute,
		CallTimeout:           60 * time.Second,
		MaxBatchSessions:      10,
		MinReclassifyInterval: 30 * time.Minute,
		Model:                 "haiku",
	}
}

// modelHierarchy returns the ordered model list for batch calls: primary first, fallbacks
// after, defaulting a blank primary to haiku so zero-value configs behave like before.
func (p *SessionTagClassificationPoller) modelHierarchy() []string {
	p.mu.RLock()
	model := p.config.Model
	fallbacks := p.config.FallbackModels
	p.mu.RUnlock()
	if model == "" {
		model = "haiku"
	}
	return append([]string{model}, fallbacks...)
}

// SetModelConfig hot-swaps the model hierarchy without a poller restart — the Update RPC
// path for the settings UI. Next tick's batch call uses the new models.
func (p *SessionTagClassificationPoller) SetModelConfig(model string, fallbackModels []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.Model = model
	p.config.FallbackModels = slices.Clone(fallbackModels)
}

// cachedTagResult is the per-session cache entry. applied entries are never retried
// automatically (classify-once); degraded ones retry after MinReclassifyInterval.
// tags is informational: reclassification always goes through ClassifyNow.
type cachedTagResult struct {
	tags         []string
	classifiedAt time.Time
	applied      bool
}

// SessionTagClassificationPoller polls all monitored sessions at a shared interval and, for
// any session not yet classified (or degraded and past its retry backoff), batches it into one
// headless.GenerateSessionTagsBatch call. Mirrors PRStatusPoller's
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

// Instances returns a snapshot copy of the currently monitored instances, mirroring
// HistoryLinker.Instances().
func (p *SessionTagClassificationPoller) Instances() []*Instance {
	p.mu.RLock()
	defer p.mu.RUnlock()
	snap := make([]*Instance, len(p.instances))
	copy(snap, p.instances)
	return snap
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
	loopCtx := p.ctx
	p.wg.Add(1)
	p.mu.Unlock()

	// loopCtx is passed directly rather than having pollLoop re-read p.ctx itself: a re-read
	// would still race a fast Stop() that nils p.ctx out before this goroutine gets scheduled.
	go p.pollLoop(loopCtx)
	log.Info("session tag classification poller started", "interval", p.config.PollInterval)
}

// Running reports whether the poll loop is currently active — true between a Start() call and
// the matching Stop(). Exposed so callers (and tests) can assert the poller's actual state
// rather than scraping log output for a "started" message.
func (p *SessionTagClassificationPoller) Running() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ctx != nil
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

// pollLoop runs the ticker loop. ctx is passed in because reading p.ctx would race a Stop()
// that nils it (Stop makes the poller restartable).
func (p *SessionTagClassificationPoller) pollLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	p.pollOnce()

	for {
		select {
		case <-ctx.Done():
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

// pendingClassification is one session that cleared the classify-once gate and awaits the
// tick's single batch call.
type pendingClassification struct {
	inst *Instance
	meta classifier.SessionTaggingContext
}

// pollOnce runs exactly one classification pass over every monitored instance. Extracted from
// pollLoop's ticker wrapper so tests can invoke a single tick synchronously with no real sleep
// (deterministic-fast-tests). At most one LLM call is issued per tick — a batch covering up to
// MaxBatchSessions changed sessions, surplus deferred to later ticks — so spend is bounded to
// one haiku call per PollInterval regardless of fleet size or churn.
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
	now := time.Now()

	var pending []pendingClassification
	var alreadyClassified, cooldownSkips, suspended int
	for _, inst := range instances {
		pc, skip := p.classificationNeeded(inst, now)
		switch skip {
		case "suspended":
			suspended++
		case "already_classified":
			alreadyClassified++
		case "cooldown":
			cooldownSkips++
		default:
			pending = append(pending, *pc)
		}
	}

	if len(pending) == 0 {
		// Deliberately Debug, not Info: the old per-session cache_hit INFO line is what
		// wallpapered the Logs tab with a line per session per tick while spending zero
		// tokens. Real LLM spend still logs per session at Info in classifyBatch.
		log.Debug("session tag poller: tick complete, nothing to classify",
			"sessions", len(instances), "already_classified", alreadyClassified, "cooldown_skips", cooldownSkips, "suspended", suspended)
		return
	}

	batchSize := len(pending)
	if batchSize > p.config.MaxBatchSessions && p.config.MaxBatchSessions > 0 {
		batchSize = p.config.MaxBatchSessions
	}
	p.classifyBatch(ctx, pending[:batchSize], vocabulary, now)
	log.Info("session tag poller: tick complete",
		"sessions", len(instances), "classified", batchSize, "deferred", len(pending)-batchSize,
		"already_classified", alreadyClassified, "cooldown_skips", cooldownSkips, "suspended", suspended)
}

// metaForSession builds the classifier input for one instance's current snapshot. Shared by
// classificationNeeded and ClassifyNow so the manual path classifies exactly what the
// automatic path would have.
func metaForSession(snap *InstanceSnapshot) classifier.SessionTaggingContext {
	return classifier.SessionTaggingContext{
		Name:    snap.Title,
		Branch:  snap.Branch,
		Path:    snap.Path,
		Program: snap.Program,
		Tags:    snap.Tags,
	}
}

// classificationNeeded enforces classify-once: an applied cache entry is never reclassified
// automatically. It returns the pending classification (skip == "") or the skip reason:
// "suspended", "already_classified", or "cooldown" (degraded entry inside the retry backoff).
func (p *SessionTagClassificationPoller) classificationNeeded(inst *Instance, now time.Time) (*pendingClassification, string) {
	snap := inst.Snapshot()
	if snap.Status.IsSuspended() {
		return nil, "suspended"
	}

	cached, ok := p.cache.Load(snap.Title)
	if ok && cached.applied {
		return nil, "already_classified" // classified once: left alone until the user re-runs it
	}
	if ok && p.config.MinReclassifyInterval > 0 && now.Sub(cached.classifiedAt) < p.config.MinReclassifyInterval {
		return nil, "cooldown" // last attempt degraded; back off before retrying
	}
	return &pendingClassification{inst: inst, meta: metaForSession(snap)}, ""
}

// ClassifyNow synchronously (re)classifies one monitored session, bypassing the classify-once
// gate — the programmatic back end of the user's manual "re-run classification" action. It
// runs a single-session batch call bounded by CallTimeout and cancelled with ctx or the
// poller. It errors when no session matches title or the classification degraded.
func (p *SessionTagClassificationPoller) ClassifyNow(ctx context.Context, title string) error {
	p.mu.RLock()
	instances := make([]*Instance, len(p.instances))
	copy(instances, p.instances)
	pollerCtx := p.ctx
	p.mu.RUnlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if pollerCtx != nil {
		defer context.AfterFunc(pollerCtx, cancel)()
	}

	for _, inst := range instances {
		if !inst.MatchesID(title) {
			continue
		}
		snap := inst.Snapshot()
		if p.classifyBatch(ctx, []pendingClassification{{inst: inst, meta: metaForSession(snap)}}, currentVocabulary(p.engine), time.Now()) > 0 {
			return ErrClassificationDegraded
		}
		return nil
	}
	return fmt.Errorf("tag poller: no monitored session matches %q", title)
}

// ErrClassificationDegraded means every model in the hierarchy failed, so no tags were applied.
var ErrClassificationDegraded = errors.New("tag classification degraded: all models failed")

// classifyBatch classifies the batch in one call over the model hierarchy and caches each
// result: applied entries are final, degraded ones retry after MinReclassifyInterval and
// leave existing tags untouched. A model choosing Unclassified is still "applied". Returns
// the number of degraded sessions.
func (p *SessionTagClassificationPoller) classifyBatch(ctx context.Context, batch []pendingClassification, vocabulary []string, now time.Time) int {
	models := p.modelHierarchy()
	// Outer bound is one CallTimeout per model; each attempt is also capped at CallTimeout.
	callCtx, cancel := context.WithTimeout(ctx, p.config.CallTimeout*time.Duration(len(models)))
	defer cancel()

	metas := make([]classifier.SessionTaggingContext, len(batch))
	for i, b := range batch {
		metas[i] = b.meta
	}

	start := time.Now()
	results, cost := headless.GenerateSessionTagsBatchWithTimeout(callCtx, p.pool, metas, vocabulary, models, p.config.CallTimeout)
	latency := time.Since(start)

	log.Info("session tag poller: batch classified", "sessions", len(batch), "batch_cost_usd", cost, "latency_ms", latency.Milliseconds())

	degraded := 0
	for _, b := range batch {
		res := results[b.meta.Name]
		if res.Degraded {
			degraded++
			log.Warn("session tag classification poller: batch classification degraded", "session", b.meta.Name)
			log.Info("session tag poller: LLM classification", "session", b.meta.Name, "outcome", "failed_unclassified")
			p.cache.Store(b.meta.Name, cachedTagResult{classifiedAt: now, applied: false})
			continue
		}
		log.Info("session tag poller: LLM classification", "session", b.meta.Name, "outcome", "applied", "tags", res.Tags)
		b.inst.ApplyLLMTagResult(res.Tags, llmSentinelRuleID)
		p.cache.Store(b.meta.Name, cachedTagResult{tags: res.Tags, classifiedAt: now, applied: true})
	}
	return degraded
}
