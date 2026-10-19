package github

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/telemetry"
)

const (
	repoSettingsTTL           = 10 * time.Minute
	repoSettingsNegativeCap   = 5 * time.Minute
	repoSettingsErrorBackoff  = 30 * time.Second
	repoSettingsTimeout       = 2 * time.Second
	repoSettingsBreakerFails  = 3
	repoSettingsBreakerOpen   = 60 * time.Second
	repoSettingsMaxEntries    = 1024
	repoSettingsMinRateWindow = time.Second
)

// Lookup outcome labels for github.repo_settings.lookup_total.
const (
	outcomeSuccess      = "success"
	outcomeCached       = "cached"
	outcomeNotModified  = "not_modified"
	outcomeRateLimited  = "rate_limited"
	outcomeNoPermission = "no_permission"
	outcomeNotFound     = "not_found"
	outcomeFailure      = "failure"
	outcomeCircuitOpen  = "circuit_open"
)

// RepoSettings returns the repository's merge settings, served from a
// per-(host, owner, repo) cache with ETag revalidation, singleflight
// coalescing, a negative-cache back-off, a 2s lookup timeout and a per-host
// circuit breaker. It never returns an error: failures are encoded in Status.
func RepoSettings(ctx context.Context, ref RepoRef) RepoSettingsResult {
	return defaultRepoSettings.lookup(ctx, ref)
}

// RepoSettingsLookupCount reports how many default-client lookups ended with
// outcome (success, cached, not_modified, rate_limited, no_permission,
// not_found, failure, circuit_open).
func RepoSettingsLookupCount(outcome string) int64 {
	return defaultRepoSettings.outcomeCount(outcome)
}

var defaultRepoSettings = newRepoSettingsLookup(ghHTTPClient, time.Now)

type repoSettingsEntry struct {
	hasValue    bool
	autoMerge   bool
	squashMerge bool
	etag        string
	fetchedAt   time.Time
	freshUntil  time.Time

	neg      RepoSettingsResult
	negUntil time.Time
}

type hostBreaker struct {
	failures  int
	openUntil time.Time
}

// repoSettingsLookup holds the cache and breaker state. Fields other than
// mu-guarded state are immutable after construction except in tests.
type repoSettingsLookup struct {
	http    *http.Client
	now     func() time.Time
	token   func(ctx context.Context, host string) string
	timeout time.Duration

	sf singleflight.Group

	mu       sync.Mutex
	entries  map[string]*repoSettingsEntry
	breakers map[string]*hostBreaker

	countMu sync.Mutex
	counts  map[string]int64
}

func newRepoSettingsLookup(client *http.Client, now func() time.Time) *repoSettingsLookup {
	return &repoSettingsLookup{
		http: client,
		now:  now,
		token: func(ctx context.Context, host string) string {
			return getGHTokenForAccount(ctx, AccountRef{Host: host})
		},
		timeout:  repoSettingsTimeout,
		entries:  map[string]*repoSettingsEntry{},
		breakers: map[string]*hostBreaker{},
		counts:   map[string]int64{},
	}
}

func (c *repoSettingsLookup) lookup(ctx context.Context, ref RepoRef) RepoSettingsResult {
	if !ref.IsValid() {
		return RepoSettingsResult{Status: RepoSettingsError, Reason: "invalid_ref"}
	}
	key := ref.repoKey()
	v, _, _ := c.sf.Do(key, func() (any, error) { return c.resolve(ctx, ref, key), nil })
	return v.(RepoSettingsResult)
}

// resolve runs inside the singleflight, so it re-checks the cache: a caller
// that missed just before another flight finished must not fetch again.
func (c *repoSettingsLookup) resolve(ctx context.Context, ref RepoRef, key string) RepoSettingsResult {
	host := NormalizeHost(ref.Host())

	c.mu.Lock()
	now := c.now()
	e := c.entries[key]
	if e != nil {
		if res, ok := e.cachedResult(now); ok {
			c.mu.Unlock()
			c.record(ctx, outcomeCached)
			return res
		}
	}
	if !c.admitLocked(host, now) {
		res := e.failureResult(RepoSettingsResult{Status: RepoSettingsError, Reason: "circuit_open", CheckedAt: now})
		c.mu.Unlock()
		c.record(ctx, outcomeCircuitOpen)
		return res
	}
	etag := ""
	if e != nil && e.hasValue {
		etag = e.etag
	}
	c.mu.Unlock()

	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
	f := c.fetchRepoSettings(fctx, ref, etag)
	cancel()

	c.mu.Lock()
	res, outcome := c.applyLocked(ref, f, c.now())
	c.mu.Unlock()
	c.record(ctx, outcome)
	return res
}

// admitLocked reports whether a request to host may go out. Once the breaker
// has tripped it admits a single half-open probe per repoSettingsBreakerOpen.
func (c *repoSettingsLookup) admitLocked(host string, now time.Time) bool {
	b := c.breakers[host]
	if b == nil || b.failures < repoSettingsBreakerFails {
		return true
	}
	if now.Before(b.openUntil) {
		return false
	}
	b.openUntil = now.Add(repoSettingsBreakerOpen)
	return true
}

func (c *repoSettingsLookup) breakerLocked(host string) *hostBreaker {
	b := c.breakers[host]
	if b == nil {
		b = &hostBreaker{}
		c.breakers[host] = b
	}
	return b
}

func (c *repoSettingsLookup) noteHostFailureLocked(host string, now time.Time) {
	b := c.breakerLocked(host)
	b.failures++
	if b.failures >= repoSettingsBreakerFails {
		b.openUntil = now.Add(repoSettingsBreakerOpen)
	}
}

func (c *repoSettingsLookup) applyLocked(ref RepoRef, f repoSettingsFetch, now time.Time) (RepoSettingsResult, string) {
	key, host := ref.repoKey(), NormalizeHost(ref.Host())
	e := c.entries[key]
	if e == nil {
		c.evictLocked(now)
		e = &repoSettingsEntry{}
		c.entries[key] = e
	}
	if f.status == RepoSettingsError {
		c.noteHostFailureLocked(host, now)
	} else {
		c.breakerLocked(host).failures = 0
	}

	if f.status == RepoSettingsOK && f.notModified && !e.hasValue {
		// A 304 with nothing retained cannot be served; drop the validator path.
		f = repoSettingsFetch{status: RepoSettingsError, reason: "unexpected_304"}
	}
	if f.status == RepoSettingsOK {
		e.neg = RepoSettingsResult{}
		e.freshUntil = now.Add(repoSettingsTTL)
		e.fetchedAt = now
		if f.notModified {
			return e.valueResult(), outcomeNotModified
		}
		e.hasValue, e.autoMerge, e.squashMerge, e.etag = true, f.autoMerge, f.squashMerge, f.etag
		return e.valueResult(), outcomeSuccess
	}

	e.neg = RepoSettingsResult{Status: f.status, Reason: f.reason, CheckedAt: now}
	e.negUntil = now.Add(negativeWindow(f))
	return e.failureResult(e.neg), failureOutcome(f.status)
}

func negativeWindow(f repoSettingsFetch) time.Duration {
	switch f.status {
	case RepoSettingsRateLimited:
		return min(max(f.backoff, repoSettingsMinRateWindow), repoSettingsNegativeCap)
	case RepoSettingsError:
		return repoSettingsErrorBackoff
	default:
		return repoSettingsNegativeCap
	}
}

func failureOutcome(s RepoSettingsStatus) string {
	switch s {
	case RepoSettingsRateLimited:
		return outcomeRateLimited
	case RepoSettingsNoPermission:
		return outcomeNoPermission
	case RepoSettingsNotFound:
		return outcomeNotFound
	default:
		return outcomeFailure
	}
}

// evictLocked bounds the cache: drop expired entries first, then one arbitrary
// entry if still full.
func (c *repoSettingsLookup) evictLocked(now time.Time) {
	if len(c.entries) < repoSettingsMaxEntries {
		return
	}
	for k, e := range c.entries {
		if !now.Before(e.freshUntil) && !now.Before(e.negUntil) {
			delete(c.entries, k)
		}
	}
	for k := range c.entries {
		if len(c.entries) < repoSettingsMaxEntries {
			return
		}
		delete(c.entries, k)
	}
}

func (e *repoSettingsEntry) valueResult() RepoSettingsResult {
	return RepoSettingsResult{
		Status: RepoSettingsOK, CheckedAt: e.fetchedAt,
		autoMerge: e.autoMerge, squashMerge: e.squashMerge,
	}
}

// failureResult prefers the last known good value (marked Stale) over a
// failure, so a transient error does not blank a previously-known setting.
// A nil receiver yields res unchanged.
func (e *repoSettingsEntry) failureResult(res RepoSettingsResult) RepoSettingsResult {
	if e == nil || !e.hasValue {
		return res
	}
	v := e.valueResult()
	v.Stale, v.Reason = true, res.Reason
	return v
}

// cachedResult returns the result to serve without a request, if any.
func (e *repoSettingsEntry) cachedResult(now time.Time) (RepoSettingsResult, bool) {
	if e.hasValue && now.Before(e.freshUntil) {
		return e.valueResult(), true
	}
	if e.neg.Status != RepoSettingsUnspecified && now.Before(e.negUntil) {
		return e.failureResult(e.neg), true
	}
	return RepoSettingsResult{}, false
}

var (
	repoSettingsCounterOnce sync.Once
	repoSettingsCounter     metric.Int64Counter
)

func (c *repoSettingsLookup) record(ctx context.Context, outcome string) {
	c.countMu.Lock()
	c.counts[outcome]++
	c.countMu.Unlock()

	repoSettingsCounterOnce.Do(func() {
		var err error
		repoSettingsCounter, err = telemetry.GetMeter().Int64Counter("github.repo_settings.lookup_total",
			metric.WithDescription("Count of repository-settings lookups by outcome"))
		if err != nil {
			log.Error("github: failed to register github.repo_settings.lookup_total", "error", err)
		}
	})
	if repoSettingsCounter != nil {
		repoSettingsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
	}
}

func (c *repoSettingsLookup) outcomeCount(outcome string) int64 {
	c.countMu.Lock()
	defer c.countMu.Unlock()
	return c.counts[outcome]
}
