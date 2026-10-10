package deliverygate

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

// Gate owns the visibility index, flag cache, counters and logger from
// construction (there is no later Bind). It implements the bus publish filter
// and the one-method consumer-side gates; no consumer receives the whole Gate.
type Gate struct {
	index    *VisibilityIndex
	resolver *Resolver
	flags    *FlagCache
	metrics  *Metrics
	logger   *slog.Logger
	limiter  *rateLimitedLogger
	now      Clock

	filterSeq atomic.Uint64 // counts notification decisions for duration sampling

	tickerMu sync.Mutex
	ticker   *time.Ticker
}

// filterSampleEvery is the duration-histogram sampling period (one in N).
const filterSampleEvery = 32

// Option configures NewGate.
type Option func(*gateConfig)

type gateConfig struct {
	logger     *slog.Logger
	now        Clock
	loader     FlagLoader
	lister     InstanceDataLister
	metrics    *Metrics
	refreshOut func() <-chan time.Time
}

// WithLogger injects the logger (default: the repo log package; never slog.Default).
func WithLogger(l *slog.Logger) Option { return func(c *gateConfig) { c.logger = l } }

// WithClock injects the clock (index TTLs, refresh back-off, log limiter).
func WithClock(now Clock) Option { return func(c *gateConfig) { c.now = now } }

// WithFlagLoader injects the flag source (default: ConfigFlagLoader).
func WithFlagLoader(l FlagLoader) Option { return func(c *gateConfig) { c.loader = l } }

// WithInstanceLister wires the async refresh backstop.
func WithInstanceLister(l InstanceDataLister) Option { return func(c *gateConfig) { c.lister = l } }

// WithMetrics injects the counter mirror (tests read it back).
func WithMetrics(m *Metrics) Option { return func(c *gateConfig) { c.metrics = m } }

// WithRefreshTimeout injects the refresh-abandon timer channel factory (tests).
func WithRefreshTimeout(f func() <-chan time.Time) Option {
	return func(c *gateConfig) { c.refreshOut = f }
}

// NewGate builds a gate with an empty, unseeded index and the flag off.
func NewGate(opts ...Option) *Gate {
	cfg := gateConfig{now: time.Now, loader: ConfigFlagLoader}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.logger == nil {
		cfg.logger = slog.New(repoLogHandler{})
	}
	if cfg.metrics == nil {
		cfg.metrics = NewMetrics()
	}
	index := NewVisibilityIndex(cfg.now)
	res := newResolver(index, cfg.metrics, cfg.now)
	res.timeout = cfg.refreshOut
	if cfg.lister != nil {
		res.SetLister(cfg.lister)
	}
	return &Gate{
		index:    index,
		resolver: res,
		flags:    NewFlagCache(cfg.loader, cfg.logger),
		metrics:  cfg.metrics,
		logger:   cfg.logger,
		limiter:  newRateLimitedLogger(cfg.logger, cfg.now),
		now:      cfg.now,
	}
}

// Flags returns the flag cache (Reload at startup, ticker, FlagObserver).
func (g *Gate) Flags() *FlagCache { return g.flags }

// Metrics returns the in-process counter mirror.
func (g *Gate) Metrics() *Metrics { return g.metrics }

// Index returns the visibility index (feed points, tests).
func (g *Gate) Index() *VisibilityIndex { return g.index }

// Resolver returns the resolver (late wiring of the refresh lister).
func (g *Gate) Resolver() *Resolver { return g.resolver }

// Seeded reports whether the startup seed ran.
func (g *Gate) Seeded() bool { return g.index.Seeded() }

// StartFlagReloader re-reads the flag every interval until ctx ends or Stop is
// called, so a config.json edit takes effect without a restart.
func (g *Gate) StartFlagReloader(ctx context.Context, interval time.Duration) {
	g.tickerMu.Lock()
	defer g.tickerMu.Unlock()
	if g.ticker != nil {
		return
	}
	g.ticker = time.NewTicker(interval)
	g.flags.Start(ctx, g.ticker.C)
}

// Stop stops the flag reloader and joins background work. Idempotent.
func (g *Gate) Stop() {
	g.tickerMu.Lock()
	if g.ticker != nil {
		g.ticker.Stop()
	}
	g.tickerMu.Unlock()
	g.flags.Stop()
	g.resolver.Wait()
}

// SeedFromInstances seeds the index synchronously from already-loaded
// instances, reading each only through Snapshot() (no storage or poller call).
func (g *Gate) SeedFromInstances(instances []*session.Instance) {
	entries := make([]Entry, 0, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		entries = append(entries, EntryFromSnapshot(inst.Snapshot()))
	}
	g.index.Replace(entries)
}

// UpsertInstance feeds one session (create, restore, rename, tag change).
func (g *Gate) UpsertInstance(inst *session.Instance) {
	if inst == nil {
		return
	}
	g.index.Upsert(EntryFromSnapshot(inst.Snapshot()))
}

// RemoveSession tombstones a deleted session given any identity key (hidden
// sessions stay hidden for 24h).
func (g *Gate) RemoveSession(key string) { g.index.Remove(key) }

// PublishFilter is installed on the EventBus at construction. It applies to
// EventNotification only, takes no caller-visible lock, and fails open on a
// panic so it can never break a Publish caller.
func (g *Gate) PublishFilter() func(*events.Event) bool {
	return func(ev *events.Event) (deliver bool) {
		if ev == nil || ev.Type != events.EventNotification {
			return true
		}
		// Time one in filterSampleEvery decisions: a clock read costs more than the
		// rest of the visible-session path (Task 2.3f), so timing every event would
		// dominate the filter's own latency.
		var start time.Time
		sampled := g.filterSeq.Add(1)%filterSampleEvery == 1
		if sampled {
			start = time.Now()
		}
		defer func() {
			if r := recover(); r != nil {
				deliver = true
				g.metrics.Add(CounterFilterPanic)
				g.limiter.log(slog.LevelError, "delivery_gate_filter_panic",
					limiterKey{event: "filter_panic"}, "panic", r)
			}
			if sampled {
				g.metrics.ObserveFilterDuration(time.Since(start))
			}
		}()
		return g.decide(ChannelBus, ev.SessionID, ev.NotificationMetadata, sessionv1.NotificationType(ev.NotificationType))
	}
}

// AllowStatusChange gates the Stopped push (push.SessionDeliveryGate).
func (g *Gate) AllowStatusChange(sessionID string) bool {
	return g.decideFailOpen(ChannelPushStatus, sessionID, sessionv1.NotificationType_NOTIFICATION_TYPE_STATUS_CHANGE)
}

// AllowAutoApprovedRow gates the auto-approved history row. A deny is always
// recorded: it is the only evidence a rule blocked a tool in a background session.
func (g *Gate) AllowAutoApprovedRow(sessionID, decision string) bool {
	if decision == "deny" {
		return true
	}
	return g.decideFailOpen(ChannelAutoApproved, sessionID, sessionv1.NotificationType_NOTIFICATION_TYPE_AUTO_APPROVED)
}

// decideFailOpen is decide for the consumer-side gates, which run on subscriber
// and handler goroutines where an unrecovered panic would take the process down.
func (g *Gate) decideFailOpen(ch Channel, sessionID string, t sessionv1.NotificationType) (deliver bool) {
	defer func() {
		if r := recover(); r != nil {
			deliver = true
			g.metrics.Add(CounterFilterPanic)
			g.limiter.log(slog.LevelError, "delivery_gate_filter_panic",
				limiterKey{event: "filter_panic", typ: string(ch)}, "panic", r)
		}
	}()
	return g.decide(ch, sessionID, nil, t)
}

// typeNames caches NotificationType.String() so the hot path allocates nothing.
var typeNames = func() map[sessionv1.NotificationType]string {
	m := map[sessionv1.NotificationType]string{}
	for v, n := range sessionv1.NotificationType_name {
		m[sessionv1.NotificationType(v)] = n
	}
	return m
}()

func typeName(t sessionv1.NotificationType) string {
	if n, ok := typeNames[t]; ok {
		return n
	}
	return t.String()
}

// decide resolves visibility, applies the policy and the flag, and records
// counters and rate-limited logs. It returns whether to deliver.
func (g *Gate) decide(ch Channel, sessionID string, metadata map[string]string, t sessionv1.NotificationType) bool {
	res := g.resolver.Resolve(sessionID, metadata)
	hint, hintOK := ParseClassHint(metadata)
	if !hintOK {
		if _, ok := g.limiter.permit(limiterKey{event: "unknown_hint", typ: typeName(t)}); ok {
			g.logger.Warn("delivery_class_unknown", "channel", string(ch),
				"delivery_class", metadata[events.MetadataKeyDeliveryClass])
		}
	}
	switch res.Visibility {
	case VisibilityUnresolved:
		class := ClassOf(t).String()
		g.metrics.Add(CounterUnresolved, class)
		if since, ok := g.limiter.permit(limiterKey{event: "unresolved", session: sessionID, typ: typeName(t), reason: string(ReasonUnresolvedFailOpen)}); ok {
			g.logger.Warn("delivery_unresolved_fail_open", "channel", string(ch), "session_id", sessionID,
				"notification_type", typeName(t), "class", class, "suppressed_since_last", since)
		}
		return true
	case VisibilityHidden:
		return g.decideHidden(ch, res, ShouldDeliver(res.Visibility, Facts{Type: t, Hint: hint}), t, sessionID)
	default:
		return true
	}
}

func (g *Gate) decideHidden(ch Channel, res Resolution, d Decision, t sessionv1.NotificationType, sessionID string) bool {
	kind := string(res.Kind)
	name := typeName(t)
	key := limiterKey{session: sessionID, typ: name, reason: string(d.Reason)}
	if d.Outcome == OutcomeDeliver {
		g.metrics.Add(CounterHiddenDelivered, string(ch), d.Class.String(), kind)
		key.event = "hidden_delivery_allowed"
		if since, ok := g.limiter.permit(key); ok {
			g.logger.Info("hidden_delivery_allowed", "channel", string(ch), "session_id", sessionID,
				"class", d.Class.String(), "reason", string(d.Reason), "suppressed_since_last", since)
		}
		return true
	}
	if g.flags.EnabledFor(res.Kind) {
		g.metrics.Add(CounterSuppressed, string(ch), name, string(d.Reason), kind)
		key.event = "delivery_suppressed"
		if since, ok := g.limiter.permit(key); ok {
			g.logger.Info("delivery_suppressed", "channel", string(ch), "session_id", sessionID,
				"notification_type", name, "reason", string(d.Reason), "suppressed_since_last", since)
		}
		return false
	}
	g.metrics.Add(CounterWouldSuppress, string(ch), name, string(d.Reason), kind)
	key.event = "delivery_would_suppress"
	if since, ok := g.limiter.permit(key); ok {
		g.logger.Info("delivery_would_suppress", "channel", string(ch), "session_id", sessionID,
			"notification_type", name, "reason", string(d.Reason), "suppressed_since_last", since)
	}
	return true
}

// CountLegacySuppressed increments the legacy-check counter at one of the
// legacy hidden-session check sites, labelling the class the policy would give
// that event (Task 2.4c). It never changes behavior.
func (g *Gate) CountLegacySuppressed(site string, t sessionv1.NotificationType, hint ClassHint) {
	d := ShouldDeliver(VisibilityHidden, Facts{Type: t, Hint: hint})
	g.metrics.Add(CounterLegacySuppressed, site, typeName(t), d.Class.String())
}

// CountLegacySuppressedType is CountLegacySuppressed for callers that hold the
// raw int32 notification type (the event shape) and no class hint.
func (g *Gate) CountLegacySuppressedType(site string, notificationType int32) {
	g.CountLegacySuppressed(site, sessionv1.NotificationType(notificationType), HintNone)
}

// CountUnversionedRequest counts a SendNotification without ssq_notify_schema
// and logs one WARN per hour.
func (g *Gate) CountUnversionedRequest() {
	g.metrics.Add(CounterRPCUnversioned)
	g.limiter.logEvery(time.Hour, slog.LevelWarn, "legacy_ssq_notify_detected",
		limiterKey{event: "legacy_ssq_notify"})
}
