package flow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// SectionFailure names a section that failed and why.
type SectionFailure struct {
	Section SectionName
	Err     error
}

// ComputeStats is what an Observer sees after each uncached compute. It
// carries counts and timings only, never item titles.
type ComputeStats struct {
	Window   WindowSpec
	Rows     int
	QueryMs  int64
	Cache    string
	Failures []SectionFailure
}

// Observer receives ComputeStats after every compute (cache hits do not fire it).
type Observer func(ComputeStats)

// CaptureSinceFunc reports the earliest trigger-capture time (zero when none),
// so comparisons can refuse metrics that straddle it.
type CaptureSinceFunc func(ctx context.Context) (time.Time, error)

// AggregatorOption configures an Aggregator.
type AggregatorOption func(*Aggregator)

// WithObserver sets the compute-stats callback.
func WithObserver(o Observer) AggregatorOption { return func(a *Aggregator) { a.observer = o } }

// WithLogger sets the logger for the debug compute line and section warnings.
func WithLogger(l *slog.Logger) AggregatorOption { return func(a *Aggregator) { a.logger = l } }

// WithCacheOptions overrides the summary cache TTL and size.
func WithCacheOptions(o CacheOptions) AggregatorOption {
	return func(a *Aggregator) { a.cacheOpts = o }
}

// WithReaderLimits overrides the row ceiling, chunk size and batch timeout;
// zero fields keep the defaults.
func WithReaderLimits(l ReaderLimits) AggregatorOption {
	return func(a *Aggregator) { a.limits = l }
}

// WithCaptureSince supplies the trigger-capture date for FlowMeta and compare.
func WithCaptureSince(f CaptureSinceFunc) AggregatorOption {
	return func(a *Aggregator) { a.captureSince = f }
}

// Aggregator assembles FlowSummary, comparisons, drill-downs and timelines
// from the four role-interface sources. Summary is cached and coalesced; one
// instance serves both the RPC handler and the MCP tools. Safe for concurrent use.
type Aggregator struct {
	src          Sources
	clock        Clock
	observer     Observer
	logger       *slog.Logger
	captureSince CaptureSinceFunc
	cacheOpts    CacheOptions
	limits       ReaderLimits
	cache        *Cache[*FlowSummary]
	drillCache   *Cache[*DrilldownPage]
	// timelineCache never stores (ItemIDs makes queries uncacheable); it only
	// coalesces and routes timelines through the shared compute slot.
	timelineCache *Cache[*ItemTimeline]
}

// NewAggregator builds an Aggregator reading time only from clock.
func NewAggregator(src Sources, clock Clock, opts ...AggregatorOption) *Aggregator {
	a := &Aggregator{src: src, clock: clock, logger: slog.New(slog.DiscardHandler)}
	for _, o := range opts {
		o(a)
	}
	a.limits = a.limits.withDefaults()
	a.cacheOpts.DefaultConfig = DefaultConfig()
	a.cacheOpts.slot = make(chan struct{}, 1)
	a.cache = NewCache[*FlowSummary](clock, a.cacheOpts)
	a.cache.storable = func(s *FlowSummary) bool { return !s.Meta.degraded }
	a.drillCache = NewCache[*DrilldownPage](clock, a.cacheOpts)
	a.timelineCache = NewCache[*ItemTimeline](clock, a.cacheOpts)
	return a
}

// allSections lists the section names in response order.
var allSections = []SectionName{
	SectionTiles, SectionFunnel, SectionCauses, SectionReview, SectionPR,
	SectionStuck, SectionAnomalies, SectionTrends, SectionNotes,
}

// AllSections returns every valid section name in response order.
func AllSections() []SectionName { return slices.Clone(allSections) }

func sectionNames() string {
	names := make([]string, len(allSections))
	for i, s := range allSections {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

// selectSections validates names and returns the selected set (all when empty).
func selectSections(names []SectionName) (map[SectionName]bool, error) {
	sel := make(map[SectionName]bool, len(allSections))
	if len(names) == 0 {
		for _, s := range allSections {
			sel[s] = true
		}
		return sel, nil
	}
	for _, n := range names {
		if !slices.Contains(allSections, n) {
			return nil, fmt.Errorf("unknown section %q; valid sections: %s", n, sectionNames())
		}
		sel[n] = true
	}
	return sel, nil
}

// effectiveConfig returns the query's Config, or the defaults when unset.
func effectiveConfig(c Config) Config {
	if c == (Config{}) {
		return DefaultConfig()
	}
	return c
}

// Summary returns the sectioned summary for q. Source failures never fail the
// call: the affected sections carry Status=Failed with the error text. Only an
// invalid query (window or section name) or a busy/cancelled wait returns an error.
func (a *Aggregator) Summary(ctx context.Context, q FlowQuery) (*FlowSummary, error) {
	sel, err := selectSections(q.Sections)
	if err != nil {
		return nil, err
	}
	w, err := q.Window.Window(a.clock)
	if err != nil {
		return nil, err
	}
	return a.cachedSummary(ctx, q, w, sel)
}

// cachedSummary serves q through the cache, coalescing and compute slot; w must
// be the resolution of q.Window.
func (a *Aggregator) cachedSummary(ctx context.Context, q FlowQuery, w FlowWindow, sel map[SectionName]bool) (*FlowSummary, error) {
	computed := false
	s, err := a.cache.Do(ctx, q, func(ctx context.Context) (*FlowSummary, error) {
		computed = true
		return a.computeSummary(ctx, q, w, sel)
	})
	if err != nil {
		return nil, err
	}
	if !computed {
		a.logger.Debug("flow summary served", "cache", "hit", "rows", s.Meta.RowCounts.total(), "query_ms", s.Meta.QueryMs)
	}
	return s, nil
}

func (rc RowCounts) total() int { return rc.Events + rc.Verdicts + rc.StuckIntervals + rc.Items }

// computeSummary runs every selected section against window w, uncached.
// It fails when the window is too large to read (ErrWindowTooLarge) or ctx ends;
// every other source failure is reported per section.
func (a *Aggregator) computeSummary(ctx context.Context, q FlowQuery, w FlowWindow, sel map[SectionName]bool) (*FlowSummary, error) {
	started := a.clock.Now()
	r := newRun(ctx, a, q, w)
	if needsEvents(sel) {
		if _, err := r.loadEvents(); errors.Is(err, ErrWindowTooLarge) {
			return nil, err
		}
	}
	s := r.buildSections(sel)
	// A cancelled compute is not a result: surfacing it as per-section failures
	// would hand coalesced live followers a summary built from the leader's ctx.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("flow summary: %w", err)
	}
	s.Meta = r.meta(sel)
	s.Meta.QueryMs = a.clock.Now().Sub(started).Milliseconds()
	s.Meta.degraded = r.failedSections > 0
	stats := ComputeStats{Window: q.Window, Rows: s.Meta.RowCounts.total(), QueryMs: s.Meta.QueryMs, Cache: "miss", Failures: r.fails}
	a.logger.Debug("flow summary computed", "cache", stats.Cache, "rows", stats.Rows, "query_ms", stats.QueryMs)
	for _, f := range r.fails {
		a.logger.Warn("flow section failed", "section", string(f.Section), "error", f.Err.Error())
	}
	if a.observer != nil {
		a.observer(stats)
	}
	return s, nil
}

// needsEvents reports whether any selected section reads the event table.
func needsEvents(sel map[SectionName]bool) bool {
	for name, on := range sel {
		if on && name != SectionStuck {
			return true
		}
	}
	return false
}
