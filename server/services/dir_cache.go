// Package services provides the server-side service implementations.
package services

import (
	"os"
	"sync"
	"time"
)

type dirCacheEntry struct {
	entries  []os.DirEntry
	dirMtime time.Time
	cachedAt time.Time
}

// DirCache is a thread-safe, mtime+TTL-invalidated cache of os.DirEntry slices
// keyed by directory path. It is intended to reduce repeated os.ReadDir calls for
// the same directory within a short window (e.g., repeated Omnibar opens).
//
// Invalidation policy:
//   - An entry is stale if time.Since(cachedAt) > ttl.
//   - An entry is stale if the directory's mtime has advanced since the entry was stored.
//   - Eviction is LRU by insertion order: when len(entries) >= maxSize, the oldest
//     entry (earliest cachedAt) is removed before storing a new one.
//
// No background goroutines are used; all operations are on-demand.
type DirCache struct {
	mu      sync.RWMutex
	entries map[string]*dirCacheEntry
	maxSize int
	ttl     time.Duration
}

// NewDirCache creates a DirCache with the given capacity and TTL.
func NewDirCache(maxSize int, ttl time.Duration) *DirCache {
	return &DirCache{
		entries: make(map[string]*dirCacheEntry, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get returns the cached DirEntry slice for path if the entry is still valid.
//
// Validity requires both:
//  1. time.Since(entry.cachedAt) <= c.ttl  (TTL not expired)
//  2. os.Stat(path).ModTime() == entry.dirMtime  (directory unchanged)
//
// Returns (nil, false) on any miss: entry absent, TTL expired, or mtime changed.
// A stale-mtime entry is removed from the cache under a write lock so the next
// Put does not exceed maxSize unnecessarily.
func (c *DirCache) Get(path string) ([]os.DirEntry, bool) {
	// Fast path: read lock.
	c.mu.RLock()
	entry, ok := c.entries[path]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	// TTL check — no lock needed; cachedAt is immutable after Put.
	if time.Since(entry.cachedAt) > c.ttl {
		// Evict the stale entry so it doesn't occupy a slot until the next Put.
		c.mu.Lock()
		delete(c.entries, path)
		c.mu.Unlock()
		return nil, false
	}

	// Mtime check: one stat call per Get, cheap on local filesystems.
	info, err := os.Stat(path)
	if err != nil {
		// Path no longer accessible; treat as miss.
		return nil, false
	}
	if info.ModTime().After(entry.dirMtime) {
		// Directory has changed; evict the stale entry.
		c.mu.Lock()
		delete(c.entries, path)
		c.mu.Unlock()
		return nil, false
	}

	return entry.entries, true
}

// Put stores entries for path with the provided dirMtime.
// If the cache is at capacity, the entry with the oldest cachedAt is evicted first.
func (c *DirCache) Put(path string, entries []os.DirEntry, dirMtime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[path] = &dirCacheEntry{
		entries:  entries,
		dirMtime: dirMtime,
		cachedAt: time.Now(),
	}
}

// evictOldest removes the entry with the smallest cachedAt timestamp.
// Caller must hold the write lock.
// O(n) scan is acceptable for the small cache sizes used in practice (n ≤ 256).
func (c *DirCache) evictOldest() {
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
