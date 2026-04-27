package unfinished

import (
	"sync"
	"time"
)

// worktreeCache is a single-entry TTL cache for a ScanResult.
type worktreeCache struct {
	mu       sync.RWMutex
	result   ScanResult
	scanTime time.Time
	ttl      time.Duration
	hasValue bool
}

// Get returns the cached result and true if the entry is still fresh.
func (c *worktreeCache) Get() (ScanResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.hasValue {
		return ScanResult{}, false
	}
	if time.Since(c.scanTime) > c.ttl {
		return ScanResult{}, false
	}
	return c.result, true
}

// Set stores a new scan result and resets the TTL clock.
func (c *worktreeCache) Set(result ScanResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result = result
	c.scanTime = time.Now()
	c.hasValue = true
}

// Invalidate clears the cache entry, forcing the next Get to return false.
func (c *worktreeCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hasValue = false
	c.scanTime = time.Time{}
}
