package services

import (
	"sync"
	"time"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

type gitignoreCacheEntry struct {
	patterns []gitignore.Pattern
	dirMtime time.Time
	cachedAt time.Time
}

// GitignoreCache is a thread-safe TTL cache of gitignore.Pattern slices keyed by
// a composite path string. It mirrors the design of DirCache.
//
// Invalidation policy:
//   - An entry is stale if time.Since(cachedAt) > ttl.
//   - Eviction is LRU by insertion order: when len(entries) >= maxSize, the oldest
//     entry (earliest cachedAt) is removed before storing a new one.
//
// No background goroutines are used; all operations are on-demand.
type GitignoreCache struct {
	mu      sync.RWMutex
	entries map[string]*gitignoreCacheEntry
	maxSize int
	ttl     time.Duration
}

// NewGitignoreCache creates a GitignoreCache with the given capacity and TTL.
func NewGitignoreCache(maxSize int, ttl time.Duration) GitignoreCache {
	return GitignoreCache{
		entries: make(map[string]*gitignoreCacheEntry, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get returns the cached pattern slice for key if the entry is still valid.
// Validity requires time.Since(entry.cachedAt) <= c.ttl.
// Returns (nil, false) on any miss: entry absent or TTL expired.
func (c *GitignoreCache) Get(key string) ([]gitignore.Pattern, bool) {
	// Fast path: read lock.
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	// TTL check — cachedAt is immutable after Put.
	if time.Since(entry.cachedAt) > c.ttl {
		// Evict the stale entry so it doesn't occupy a slot until the next Put.
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.patterns, true
}

// Put stores patterns for key with the provided mtime (stored for future reference).
// If the cache is at capacity, the entry with the oldest cachedAt is evicted first.
func (c *GitignoreCache) Put(key string, patterns []gitignore.Pattern, mtime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &gitignoreCacheEntry{
		patterns: patterns,
		dirMtime: mtime,
		cachedAt: time.Now(),
	}
}

// evictOldest removes the entry with the smallest cachedAt timestamp.
// Caller must hold the write lock.
func (c *GitignoreCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for k, e := range c.entries {
		if oldestKey == "" || e.cachedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.cachedAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
