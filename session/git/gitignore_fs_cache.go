package git

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
)

// gitignoreCacheTTL bounds how long a cached directory listing or ignore-file read
// (from gitignoreFSCache) may be reused. It matches IsDirtyCacheTTL: the outer
// isDirtyCache already tolerates a dirty/clean result up to that old, so serving the
// underlying filesystem reads from a cache of the same age doesn't relax the freshness
// contract IsDirtyWithHint documents, and InvalidateDirtyCache clears both together —
// a commit/push still forces a fully fresh read on the next check.
const gitignoreCacheTTL = IsDirtyCacheTTL

// readDirEntry is a cached result of one gitignoreFSCache.dirs entry.
type readDirEntry struct {
	infos  []os.FileInfo
	err    error
	expiry time.Time
}

// openEntry is a cached result of one gitignoreFSCache.files entry. Only small files
// (.gitignore, .git/info/exclude) are ever read through this path, so buffering the
// full content is safe.
type openEntry struct {
	content []byte
	err     error
	expiry  time.Time
}

// gitignoreFSCache memoizes the ReadDir/Open calls go-git's
// gitignore.ReadPatterns issues while recursively walking a worktree, scoped to a
// single GitWorktree instance and cleared by InvalidateDirtyCache.
//
// go-git's Worktree.Status() unconditionally re-reads and re-parses every
// .gitignore/.git/info/exclude file in the tree on every call — it has no caching of
// its own for this. Profiling a live instance under load showed this recursive walk
// accounting for ~31% of process CPU and ~20% of allocations (session/git's
// worktreeIsDirty, called from IsDirtyWithHint's cache-miss path). The zero value is
// ready to use (sync.Map requires no init), so this is embedded by value in
// GitWorktree rather than lazily constructed.
type gitignoreFSCache struct {
	dirs  sync.Map // path -> readDirEntry
	files sync.Map // path -> openEntry
}

// reset clears every cached entry. Called from InvalidateDirtyCache so a commit/push
// (which can add/remove/edit .gitignore files or the files they cover) is reflected on
// the very next status check, matching the freshness guarantee already documented on
// IsDirtyCacheTTL/IsDirtyCleanCacheTTL.
func (c *gitignoreFSCache) reset() {
	c.dirs.Range(func(k, _ interface{}) bool {
		c.dirs.Delete(k)
		return true
	})
	c.files.Range(func(k, _ interface{}) bool {
		c.files.Delete(k)
		return true
	})
}

// cachedFilesystem wraps a billy.Filesystem, serving ReadDir and Open — the only two
// calls gitignore.ReadPatterns' recursive walk issues — from cache for
// gitignoreCacheTTL. Every other billy.Filesystem method delegates unchanged via the
// embedded interface.
type cachedFilesystem struct {
	billy.Filesystem
	cache *gitignoreFSCache
}

// newCachedFilesystem returns fs wrapped with cache. Passing a nil cache disables
// caching (ReadDir/Open pass straight through).
func newCachedFilesystem(fs billy.Filesystem, cache *gitignoreFSCache) billy.Filesystem {
	if cache == nil {
		return fs
	}
	return &cachedFilesystem{Filesystem: fs, cache: cache}
}

func (c *cachedFilesystem) ReadDir(path string) ([]os.FileInfo, error) {
	if v, ok := c.cache.dirs.Load(path); ok {
		if e := v.(readDirEntry); time.Now().Before(e.expiry) {
			return e.infos, e.err
		}
	}
	infos, err := c.Filesystem.ReadDir(path)
	c.cache.dirs.Store(path, readDirEntry{infos: infos, err: err, expiry: time.Now().Add(gitignoreCacheTTL)})
	return infos, err
}

func (c *cachedFilesystem) Open(filename string) (billy.File, error) {
	if v, ok := c.cache.files.Load(filename); ok {
		if e := v.(openEntry); time.Now().Before(e.expiry) {
			if e.err != nil {
				return nil, e.err
			}
			return &cachedFile{name: filename, Reader: bytes.NewReader(e.content)}, nil
		}
	}

	f, err := c.Filesystem.Open(filename)
	if err != nil {
		// Only cache the common "no .gitignore here" case — every other error
		// (permission denied, transient I/O failure) passes through uncached so
		// it surfaces on the very next call instead of being masked for
		// gitignoreCacheTTL.
		if os.IsNotExist(err) {
			c.cache.files.Store(filename, openEntry{err: err, expiry: time.Now().Add(gitignoreCacheTTL)})
		}
		return nil, err
	}
	defer f.Close()

	content, readErr := io.ReadAll(f)
	if readErr != nil {
		return nil, readErr
	}
	c.cache.files.Store(filename, openEntry{content: content, expiry: time.Now().Add(gitignoreCacheTTL)})
	return &cachedFile{name: filename, Reader: bytes.NewReader(content)}, nil
}

// cachedFile is a read-only billy.File backed by an in-memory buffer, returned by
// cachedFilesystem.Open on both cache hits and (freshly-buffered) misses.
// gitignore.readIgnoreFile only ever calls Read/Close on the file it opens, so Write
// and Truncate are intentionally unsupported.
type cachedFile struct {
	name string
	*bytes.Reader
}

func (f *cachedFile) Name() string  { return f.name }
func (f *cachedFile) Close() error  { return nil }
func (f *cachedFile) Lock() error   { return nil }
func (f *cachedFile) Unlock() error { return nil }
func (f *cachedFile) Write([]byte) (int, error) {
	return 0, errors.New("cachedFile: write not supported")
}
func (f *cachedFile) Truncate(int64) error { return errors.New("cachedFile: truncate not supported") }
