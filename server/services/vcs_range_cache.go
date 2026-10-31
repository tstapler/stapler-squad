package services

// vcs_range_cache.go memoizes the two per-range git stages of GetVCSStatus
// (shipped commits, aggregate diff stat). A range is identified by immutable
// SHAs, so a result never goes stale: when a session's HEAD hasn't moved, the
// 15s status-cache miss no longer repeats the walk and diff.

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/session/git"
)

// Stage labels for vcs.status.stage.duration and vcs.status.range_cache. Bounded set.
const (
	vcsStageGitStatus      = "git_status"
	vcsStageShippedCommits = "shipped_commits"
	vcsStageDiffStat       = "diff_stat"
)

// Results for vcs.status.range_cache. Bounded set.
const (
	rangeCacheHit      = "hit"
	rangeCacheMiss     = "miss"
	rangeCacheNegative = "negative_hit"
)

const (
	vcsRangeCacheSize = 256
	// vcsRangeFailureTTL is how long a stage that blew its budget is not retried for the same range.
	vcsRangeFailureTTL = time.Minute
)

// Per-stage budgets inside the 30s shared computation; variables so tests can shrink them.
var (
	vcsShippedCommitsBudget = 10 * time.Second //nolint:gochecknoglobals
	vcsDiffStatBudget       = 10 * time.Second //nolint:gochecknoglobals
)

var errVCSStageBudget = errors.New("vcs status stage exceeded its time budget")

type vcsRangeKey struct{ workDir, baseSHA, headSHA string }

type rangeCacheEntry[V any] struct {
	key         vcsRangeKey
	val         V
	failedUntil time.Time // non-zero: remembers a budget failure instead of a value
}

// rangeCache is a size-bounded LRU keyed by (workDir, base, head). The zero value is not usable; use newRangeCache.
type rangeCache[V any] struct {
	mu  sync.Mutex
	max int
	now func() time.Time
	ll  *list.List
	m   map[vcsRangeKey]*list.Element
}

func newRangeCache[V any](max int, now func() time.Time) *rangeCache[V] {
	return &rangeCache[V]{max: max, now: now, ll: list.New(), m: make(map[vcsRangeKey]*list.Element)}
}

// lookup returns the cached value (hit), a remembered failure (negative), or neither (miss).
func (c *rangeCache[V]) lookup(k vcsRangeKey) (val V, result string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.m[k]
	if !ok {
		return val, rangeCacheMiss
	}
	e := el.Value.(*rangeCacheEntry[V])
	if !e.failedUntil.IsZero() {
		if c.now().Before(e.failedUntil) {
			return val, rangeCacheNegative
		}
		c.ll.Remove(el)
		delete(c.m, k)
		return val, rangeCacheMiss
	}
	c.ll.MoveToFront(el)
	return e.val, rangeCacheHit
}

func (c *rangeCache[V]) store(k vcsRangeKey, val V, failedUntil time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.m[k]; ok {
		*el.Value.(*rangeCacheEntry[V]) = rangeCacheEntry[V]{key: k, val: val, failedUntil: failedUntil}
		c.ll.MoveToFront(el)
		return
	}
	c.m[k] = c.ll.PushFront(&rangeCacheEntry[V]{key: k, val: val, failedUntil: failedUntil})
	for c.ll.Len() > c.max {
		last := c.ll.Back()
		c.ll.Remove(last)
		delete(c.m, last.Value.(*rangeCacheEntry[V]).key)
	}
}

type shippedCommitsResult struct {
	commits   []git.ShippedCommit
	truncated bool
}

// vcsRangeStages holds the caches and the git operations they front. Operations are fields so tests
// can count calls and block on ctx without a real repo.
type vcsRangeStages struct {
	once      sync.Once
	now       func() time.Time // nil: time.Now
	listing   *rangeCache[shippedCommitsResult]
	diffStats *rangeCache[git.AggregateDiffStat]

	listShipped func(ctx context.Context, repoPath, baseSHA, headSHA string) ([]git.ShippedCommit, bool, error)
	diffStat    func(ctx context.Context, repoPath, baseSHA, headSHA string) (git.AggregateDiffStat, error)
}

func (s *vcsRangeStages) init() {
	s.once.Do(func() {
		if s.now == nil {
			s.now = time.Now
		}
		s.listing = newRangeCache[shippedCommitsResult](vcsRangeCacheSize, s.now)
		s.diffStats = newRangeCache[git.AggregateDiffStat](vcsRangeCacheSize, s.now)
		if s.listShipped == nil {
			s.listShipped = git.ListShippedCommits
		}
		if s.diffStat == nil {
			s.diffStat = git.DiffStatBetween
		}
	})
}

// shippedCommits returns the commit list for the range, from cache when HEAD hasn't moved.
func (s *vcsRangeStages) shippedCommits(ctx context.Context, k vcsRangeKey) (shippedCommitsResult, error) {
	s.init()
	return runRangeStage(ctx, s, vcsStageShippedCommits, s.listing, k, vcsShippedCommitsBudget,
		func(ctx context.Context) (shippedCommitsResult, error) {
			commits, truncated, err := s.listShipped(ctx, k.workDir, k.baseSHA, k.headSHA)
			return shippedCommitsResult{commits: commits, truncated: truncated}, err
		})
}

// aggregateDiffStat returns the diff stat for the range, from cache when HEAD hasn't moved.
func (s *vcsRangeStages) aggregateDiffStat(ctx context.Context, k vcsRangeKey) (git.AggregateDiffStat, error) {
	s.init()
	return runRangeStage(ctx, s, vcsStageDiffStat, s.diffStats, k, vcsDiffStatBudget,
		func(ctx context.Context) (git.AggregateDiffStat, error) {
			return s.diffStat(ctx, k.workDir, k.baseSHA, k.headSHA)
		})
}

// runRangeStage serves k from cache, else runs compute under budget. Successes are cached; a stage
// that exhausts its own budget (caller still waiting) is remembered for vcsRangeFailureTTL so a
// pathological range doesn't burn the budget on every cache miss. Caller cancellation is never cached.
func runRangeStage[V any](ctx context.Context, s *vcsRangeStages, stage string, cache *rangeCache[V], k vcsRangeKey, budget time.Duration, compute func(context.Context) (V, error)) (V, error) {
	m := vcsMetrics()
	attr := attribute.String("stage", stage)
	val, result := cache.lookup(k)
	m.rangeCache.Add(ctx, 1, metric.WithAttributes(attr, attribute.String("result", result)))
	switch result {
	case rangeCacheHit:
		return val, nil
	case rangeCacheNegative:
		var zero V
		return zero, errVCSStageBudget
	}

	start := s.now()
	stageCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	val, err := compute(stageCtx)
	outcome := "ok"
	switch {
	case err == nil:
		cache.store(k, val, time.Time{})
	case ctx.Err() != nil:
		outcome = vcsOutcomeCanceled
	case stageCtx.Err() != nil:
		outcome = "budget_exceeded"
		cache.store(k, val, s.now().Add(vcsRangeFailureTTL))
		err = errVCSStageBudget
	default:
		outcome = vcsOutcomeError
	}
	recordVCSStage(ctx, stage, outcome, s.now().Sub(start))
	return val, err
}

func recordVCSStage(ctx context.Context, stage, outcome string, d time.Duration) {
	vcsMetrics().stage.Record(context.WithoutCancel(ctx), float64(d.Microseconds())/1000,
		metric.WithAttributes(attribute.String("stage", stage), attribute.String("result", outcome)))
}

func stageResult(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, context.Canceled):
		return vcsOutcomeCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return "budget_exceeded"
	}
	return vcsOutcomeError
}
