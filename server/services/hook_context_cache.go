package services

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"golang.org/x/sync/singleflight"
)

const (
	defaultHookContextFreshFor = 30 * time.Second
	defaultHookContextMaxStale = 5 * time.Minute
	defaultHookContextEntries  = 512
)

type hookContextEntry struct {
	context    classifier.ClassificationContext
	observedAt time.Time
	refreshing bool
}

type HookContextCache struct {
	build    func(string) classifier.ClassificationContext
	now      func() time.Time
	freshFor time.Duration
	maxStale time.Duration
	maxItems int

	mu      sync.RWMutex
	entries map[string]hookContextEntry
	group   singleflight.Group
}

type HookContextCacheOption func(*HookContextCache)

func WithHookContextClock(now func() time.Time) HookContextCacheOption {
	return func(c *HookContextCache) {
		if now != nil {
			c.now = now
		}
	}
}

func WithHookContextAges(freshFor, maxStale time.Duration) HookContextCacheOption {
	return func(c *HookContextCache) {
		if freshFor > 0 {
			c.freshFor = freshFor
		}
		if maxStale >= c.freshFor {
			c.maxStale = maxStale
		}
	}
}

func NewHookContextCache(build func(string) classifier.ClassificationContext, options ...HookContextCacheOption) *HookContextCache {
	cache := &HookContextCache{
		build:    build,
		now:      time.Now,
		freshFor: defaultHookContextFreshFor,
		maxStale: defaultHookContextMaxStale,
		maxItems: defaultHookContextEntries,
		entries:  make(map[string]hookContextEntry),
	}
	for _, option := range options {
		option(cache)
	}
	return cache
}

// Get returns fresh context, stale context with asynchronous refresh, or one
// singleflight-coalesced cold lookup. Classification itself is never coalesced.
func (c *HookContextCache) Get(cwd string) classifier.ClassificationContext {
	if c == nil || c.build == nil {
		return classifier.ClassificationContext{Cwd: cwd}
	}
	key := canonicalContextKey(cwd)
	now := c.now()
	c.mu.RLock()
	entry, found := c.entries[key]
	c.mu.RUnlock()
	if found {
		age := now.Sub(entry.observedAt)
		if age <= c.freshFor {
			return entry.context
		}
		if age <= c.maxStale {
			c.refreshInBackground(key, cwd)
			return entry.context
		}
	}

	value, _, _ := c.group.Do(key, func() (interface{}, error) {
		// A caller can observe a miss, then arrive after the previous
		// singleflight call completed and was removed. Recheck under the flight
		// so that scheduling lag cannot trigger a second cold lookup.
		flightNow := c.now()
		c.mu.RLock()
		current, exists := c.entries[key]
		c.mu.RUnlock()
		if exists && flightNow.Sub(current.observedAt) <= c.freshFor {
			return current.context, nil
		}
		context := c.build(cwd)
		c.store(key, context, c.now())
		return context, nil
	})
	return value.(classifier.ClassificationContext)
}

func (c *HookContextCache) refreshInBackground(key, cwd string) {
	c.mu.Lock()
	entry, found := c.entries[key]
	if !found || entry.refreshing {
		c.mu.Unlock()
		return
	}
	entry.refreshing = true
	c.entries[key] = entry
	c.mu.Unlock()

	go func() {
		context := c.build(cwd)
		c.store(key, context, c.now())
	}()
}

func (c *HookContextCache) store(key string, context classifier.ClassificationContext, observedAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxItems {
		var oldestKey string
		var oldestTime time.Time
		for candidate, entry := range c.entries {
			if oldestKey == "" || entry.observedAt.Before(oldestTime) {
				oldestKey = candidate
				oldestTime = entry.observedAt
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = hookContextEntry{context: context, observedAt: observedAt}
}

func canonicalContextKey(cwd string) string {
	if cwd == "" {
		return "."
	}
	absolute, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return filepath.Clean(cwd)
	}
	return absolute
}
