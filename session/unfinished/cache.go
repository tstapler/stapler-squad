package unfinished

import "github.com/linkdata/deadlock"

import (
	"time"
)

// worktreeCache is a single-entry TTL cache for a ScanResult.
type worktreeCache struct {
	mu       deadlock.RWMutex
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

// snapshot returns the entry's current value and scan time for persistence.
// hasValue mirrors Get's notion of "has a result ever been stored here",
// independent of TTL -- staleness is judged by the caller when reloading.
func (c *worktreeCache) snapshot() (ScanResult, time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.result, c.scanTime, c.hasValue
}

// restore seeds the cache with a result carrying its original scan time
// (rather than time.Now(), as Set does), used to hydrate from a persisted
// snapshot. TTL freshness is judged the same way as any other entry --
// restoring an entry older than the TTL just means the next Get treats it as
// expired, same as if it had aged out in memory.
func (c *worktreeCache) restore(result ScanResult, scanTime time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result = result
	c.scanTime = scanTime
	c.hasValue = true
}
