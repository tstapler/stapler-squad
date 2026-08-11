package services

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/testutil"
)

// newReqCtx returns a background context for use in test RPC calls.
func newReqCtx(_ *testing.T) context.Context {
	return context.Background()
}

// entryNames extracts the Name field from a slice of PathEntry protos.
func entryNames(entries []*sessionv1.PathEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// TestDirCache_Hit verifies that a Put followed by an immediate Get returns
// the same entries without performing a second disk read.
func TestDirCache_Hit(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, filepath.Join(dir, "a.txt"))
	makeFile(t, filepath.Join(dir, "b.txt"))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	info, err := os.Stat(dir)
	require.NoError(t, err)

	cache := NewDirCache(16, 60*time.Second)
	cache.Put(dir, entries, info.ModTime())

	got, ok := cache.Get(dir)
	assert.True(t, ok, "expected cache hit")
	assert.Equal(t, entries, got, "expected same slice from cache")
}

// TestDirCache_MissOnMtimeChange verifies that after a directory is modified
// (which advances its mtime), Get returns a miss and removes the stale entry.
func TestDirCache_MissOnMtimeChange(t *testing.T) {
	dir := t.TempDir()

	info, err := os.Stat(dir)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	cache := NewDirCache(16, 60*time.Second)
	cache.Put(dir, entries, info.ModTime())

	// Modify the directory and explicitly advance its mtime by 1 s so the
	// test is not sensitive to filesystem mtime resolution (e.g. tmpfs on fast
	// machines can return the same timestamp if everything runs within one tick).
	makeFile(t, filepath.Join(dir, "newfile.txt"))
	future := info.ModTime().Add(time.Second)
	require.NoError(t, os.Chtimes(dir, future, future))

	got, ok := cache.Get(dir)
	assert.False(t, ok, "expected cache miss after directory was modified")
	assert.Nil(t, got)

	// Verify the stale entry was evicted.
	cache.mu.RLock()
	_, stillPresent := cache.entries[dir]
	cache.mu.RUnlock()
	assert.False(t, stillPresent, "stale entry should have been removed from cache")
}

// TestDirCache_MissOnTTLExpiry verifies that an entry is not returned after the TTL
// has elapsed, even if the directory has not changed.
func TestDirCache_MissOnTTLExpiry(t *testing.T) {
	dir := t.TempDir()

	info, err := os.Stat(dir)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	// 1 ms TTL so it expires almost immediately.
	cache := NewDirCache(16, 1*time.Millisecond)
	cache.Put(dir, entries, info.ModTime())

	// Poll until the TTL has expired and Get returns a miss.
	require.NoError(t, testutil.WaitForCondition(func() bool {
		_, ok := cache.Get(dir)
		return !ok
	}, testutil.FastWaitConfig()))

	got, ok := cache.Get(dir)
	assert.False(t, ok, "expected cache miss after TTL expiry")
	assert.Nil(t, got)
}

// TestDirCache_NoEvictionBelowMax verifies that no eviction occurs when the cache
// has fewer entries than maxSize.
func TestDirCache_NoEvictionBelowMax(t *testing.T) {
	const maxSize = 4
	cache := NewDirCache(maxSize, 60*time.Second)

	root := t.TempDir()
	for i := 0; i < maxSize-1; i++ {
		dir := filepath.Join(root, string(rune('a'+i)))
		makeDir(t, dir)

		info, err := os.Stat(dir)
		require.NoError(t, err)
		cache.Put(dir, nil, info.ModTime())
	}

	cache.mu.RLock()
	n := len(cache.entries)
	cache.mu.RUnlock()

	assert.Equal(t, maxSize-1, n, "no eviction expected below maxSize")
}

// TestDirCache_EvictsOldestAtMax verifies that adding an entry when the cache is at
// capacity evicts the entry with the oldest cachedAt, keeping len(entries) == maxSize.
func TestDirCache_EvictsOldestAtMax(t *testing.T) {
	const maxSize = 3
	cache := NewDirCache(maxSize, 60*time.Second)

	root := t.TempDir()

	// Create maxSize directories and insert them in order, each with a distinct
	// cachedAt. We manipulate cachedAt directly so the test is deterministic.
	dirs := make([]string, maxSize)
	for i := 0; i < maxSize; i++ {
		d := filepath.Join(root, string(rune('a'+i)))
		makeDir(t, d)
		dirs[i] = d

		info, err := os.Stat(d)
		require.NoError(t, err)

		cache.mu.Lock()
		if len(cache.entries) >= cache.maxSize {
			cache.evictOldest()
		}
		cache.entries[d] = &dirCacheEntry{
			entries:  nil,
			dirMtime: info.ModTime(),
			// stagger cachedAt so "a" is oldest, "b" is middle, "c" is newest
			cachedAt: time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		cache.mu.Unlock()
	}

	// Cache is now at maxSize. Adding one more entry should evict "a" (oldest).
	extraDir := filepath.Join(root, "extra")
	makeDir(t, extraDir)

	extraInfo, err := os.Stat(extraDir)
	require.NoError(t, err)
	cache.Put(extraDir, nil, extraInfo.ModTime())

	cache.mu.RLock()
	_, aPresent := cache.entries[dirs[0]] // "a" — oldest, should be evicted
	totalLen := len(cache.entries)
	cache.mu.RUnlock()

	assert.Equal(t, maxSize, totalLen, "cache size should remain at maxSize after eviction")
	assert.False(t, aPresent, "oldest entry should have been evicted")
}

// TestDirCache_ConcurrentReads verifies that many simultaneous Get calls after a Put
// do not cause data races. Run with: go test -race ./server/services/...
func TestDirCache_ConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, filepath.Join(dir, "file.txt"))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	info, err := os.Stat(dir)
	require.NoError(t, err)

	cache := NewDirCache(16, 60*time.Second)
	cache.Put(dir, entries, info.ModTime())

	const workers = 100
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			got, ok := cache.Get(dir)
			// Either a hit or a miss is fine; no panic or race is the requirement.
			if ok {
				assert.NotNil(t, got)
			}
		}()
	}

	wg.Wait()
}

// TestDirCache_NonexistentDir verifies that Get for a path that does not exist returns
// a miss and does not store anything in the cache.
func TestDirCache_NonexistentDir(t *testing.T) {
	cache := NewDirCache(16, 60*time.Second)

	got, ok := cache.Get("/does/not/exist/anywhere")
	assert.False(t, ok)
	assert.Nil(t, got)

	cache.mu.RLock()
	n := len(cache.entries)
	cache.mu.RUnlock()
	assert.Equal(t, 0, n, "Get on nonexistent path should not populate cache")
}

// TestListPathCompletions_CacheHit verifies that calling ListPathCompletions twice
// for the same directory returns identical entries and exercises the cache hit path
// (indirectly: the second call completes without error and returns consistent results).
func TestListPathCompletions_CacheHit(t *testing.T) {
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "alpha"))
	makeFile(t, filepath.Join(dir, "beta.txt"))

	svc := NewPathCompletionService()

	resp1, err := svc.ListPathCompletions(newReqCtx(t), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)
	require.True(t, resp1.Msg.BaseDirExists)

	names1 := entryNames(resp1.Msg.Entries)

	svc.cache.mu.RLock()
	entry1, inCache := svc.cache.entries[dir]
	svc.cache.mu.RUnlock()
	require.True(t, inCache, "entry should be in cache after first call")
	cachedAtFirst := entry1.cachedAt

	resp2, err := svc.ListPathCompletions(newReqCtx(t), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)
	require.True(t, resp2.Msg.BaseDirExists)

	names2 := entryNames(resp2.Msg.Entries)

	svc.cache.mu.RLock()
	entry2, stillInCache := svc.cache.entries[dir]
	svc.cache.mu.RUnlock()
	require.True(t, stillInCache, "entry should still be in cache after second call")
	assert.Equal(t, cachedAtFirst, entry2.cachedAt, "second call must use cache hit - cachedAt must not change")

	assert.Equal(t, names1, names2, "second call should return same entries as first (cache hit)")
}

// TestListPathCompletions_CacheMissAfterMtimeChange verifies that if the directory
// changes between two ListPathCompletions calls, the second call returns the updated
// contents (cache miss due to mtime change).
func TestListPathCompletions_CacheMissAfterMtimeChange(t *testing.T) {
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "alpha"))

	svc := NewPathCompletionService()

	resp1, err := svc.ListPathCompletions(newReqCtx(t), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)
	names1 := entryNames(resp1.Msg.Entries)
	assert.Contains(t, names1, "alpha")
	assert.NotContains(t, names1, "beta")

	// Modify the directory after the first call, then advance its mtime by 1 s
	// so the cache invalidation check is not sensitive to filesystem mtime resolution.
	makeDir(t, filepath.Join(dir, "beta"))
	future := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(dir, future, future))

	resp2, err := svc.ListPathCompletions(newReqCtx(t), newReq(&sessionv1.ListPathCompletionsRequest{
		PathPrefix: dir + "/",
	}))
	require.NoError(t, err)
	names2 := entryNames(resp2.Msg.Entries)

	assert.Contains(t, names2, "alpha", "original entry should still be present")
	assert.Contains(t, names2, "beta", "new entry should appear after cache miss")
}
