package vcsstatus

import (
	"time"

	"github.com/tstapler/stapler-squad/session/vc"
)

// PeekForTest returns workDir's cache entry, whatever its age.
func (c *Cache) PeekForTest(workDir string) (status *vc.VCSStatus, cachedAt time.Time, ok bool) {
	cached, ok := c.entries.Load(workDir)
	if !ok {
		return nil, time.Time{}, false
	}
	entry := cached.(vcsStatusCacheEntry)
	return entry.status, entry.cachedAt, true
}

// StoreForTest writes a cache entry, so a test can age one past a TTL.
func (c *Cache) StoreForTest(workDir string, status *vc.VCSStatus, cachedAt time.Time) {
	c.entries.Store(workDir, vcsStatusCacheEntry{status: status, cachedAt: cachedAt})
}
