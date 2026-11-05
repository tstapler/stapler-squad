package services

import (
	"context"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// insightsSummaryTTL bounds staleness of a cached GetInsightsSummary response.
// The token store changes on every transcript write, so there is no cheap exact
// invalidation seam; a few seconds collapses reload fan-out (three identical
// calls at 3-5 s each) without hiding new usage for long.
const insightsSummaryTTL = 5 * time.Second

// insightsSummaryMaxEntries bounds the cache; filter combinations are few, so
// hitting the cap just means the map is reset.
const insightsSummaryMaxEntries = 32

type summaryCacheEntry struct {
	resp    *sessionv1.GetInsightsSummaryResponse
	expires time.Time
}

// summaryCache is a short-TTL cache plus singleflight keyed on the request.
// The zero value is ready to use. Cached responses are shared between callers
// and must be treated as read-only.
type summaryCache struct {
	mu      sync.Mutex
	entries map[string]summaryCacheEntry
	gen     uint64 // bumped by invalidate so an in-flight compute cannot store a stale result
	flight  singleflight.Group

	now func() time.Time // test seam; nil means time.Now

	afterLookupMiss func() // test seam: runs between get's cache miss and joining a flight
}

func (c *summaryCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *summaryCache) invalidate() {
	c.mu.Lock()
	c.gen++
	c.entries = nil
	c.mu.Unlock()
}

func (c *summaryCache) lookup(key string) (*sessionv1.GetInsightsSummaryResponse, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok && c.clock().Before(e.expires) {
		return e.resp, c.gen, true
	}
	return nil, c.gen, false
}

func (c *summaryCache) store(key string, gen uint64, resp *sessionv1.GetInsightsSummaryResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
	if c.entries == nil || len(c.entries) >= insightsSummaryMaxEntries {
		c.entries = make(map[string]summaryCacheEntry)
	}
	c.entries[key] = summaryCacheEntry{resp: resp, expires: c.clock().Add(insightsSummaryTTL)}
}

// get returns the cached response for req, or runs compute once for all
// concurrent callers with the same request. Partial results are never cached:
// while isLoading reports true the token store is still being walked.
// compute runs detached from the caller's cancellation so one aborted caller
// cannot fail the callers sharing its flight; each caller still returns as soon
// as its own ctx is done.
func (c *summaryCache) get(
	ctx context.Context,
	req *sessionv1.GetInsightsSummaryRequest,
	isLoading func() bool,
	compute func(context.Context) (*sessionv1.GetInsightsSummaryResponse, error),
) (*sessionv1.GetInsightsSummaryResponse, error) {
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(req)
	if err != nil {
		return compute(ctx)
	}
	key := string(raw)
	cached, gen, ok := c.lookup(key)
	if ok {
		return cached, nil
	}
	if c.afterLookupMiss != nil {
		c.afterLookupMiss()
	}
	// The generation is part of the flight key so a caller arriving after an
	// invalidation starts a fresh compute instead of joining a pre-invalidation one.
	ch := c.flight.DoChan(strconv.FormatUint(gen, 10)+":"+key, func() (any, error) {
		// A flight that finished (and stored its result) after our lookup miss but
		// before we joined has left the group, so we would start a second compute.
		if hit, _, ok := c.lookup(key); ok {
			return hit, nil
		}
		resp, err := compute(context.WithoutCancel(ctx))
		if err != nil {
			return nil, err
		}
		if !resp.GetIsLoading() && !isLoading() {
			c.store(key, gen, resp)
		}
		return resp, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(*sessionv1.GetInsightsSummaryResponse), nil
	}
}
