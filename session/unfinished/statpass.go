package unfinished

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// Per-file lstat outcomes recorded by a collecting HasUncommitted pass.
const (
	statOK uint8 = iota
	statMissing
	statFailed
)

// statRes is one tracked file's lstat result. Both consumers apply their own
// predicate to it (HasUncommitted compares to the nanosecond, DiffShortstat to
// the second plus its racy-content check), so the raw facts are shared, never
// a verdict. It implements os.FileInfo through pointer receivers so handing
// one out allocates nothing.
type statRes struct {
	size    int64
	mtimeNs int64
	code    uint8
}

func (r *statRes) Name() string       { return "" }
func (r *statRes) Size() int64        { return r.size }
func (r *statRes) Mode() fs.FileMode  { return 0 }
func (r *statRes) ModTime() time.Time { return time.Unix(0, r.mtimeNs) }
func (r *statRes) IsDir() bool        { return false }
func (r *statRes) Sys() any           { return nil }

// statPass is the tracked-file lstat results of one HasUncommitted scan,
// handed to DiffShortstat within the same scanWorktree call. It is a value
// passed down the call chain, never stored in a cache, so it cannot go stale
// beyond that scan; a file edited between the two calls is judged by the
// earlier stat and the next scan sees the edit.
type statPass struct {
	tracked []trackedFile // index order, shared and immutable
	res     []statRes     // parallel to tracked
}

// maxCursorLookahead bounds the forward search when the diff's index read
// differs from the pass's; past it the file is stat'ed directly.
const maxCursorLookahead = 64

// passCursor walks a statPass in index order alongside a consumer that
// visits a subsequence of the same entries.
type passCursor struct {
	p *statPass
	i int
}

// cursor returns a cursor over p; a nil p yields a cursor that never matches.
func (p *statPass) cursor() passCursor { return passCursor{p: p} }

// next returns the recorded result for name, or nil when the pass has none.
func (c *passCursor) next(name string) *statRes {
	if c.p == nil || c.p.res == nil {
		return nil
	}
	end := min(c.i+maxCursorLookahead, len(c.p.tracked))
	for j := c.i; j < end; j++ {
		if c.p.tracked[j].name == name {
			c.i = j + 1
			return &c.p.res[j]
		}
	}
	return nil
}

var errStatFailed = errors.New("lstat failed")

// lstatTracked returns the lstat result of one tracked file: the pass's when
// it has one, otherwise a fresh os.Lstat (counted by the statFn seam).
func (g *GoGitVCSReader) lstatTracked(c *passCursor, name, wtPath string) (os.FileInfo, error) {
	if r := c.next(name); r != nil {
		switch r.code {
		case statMissing:
			return nil, fs.ErrNotExist
		case statFailed:
			return nil, errStatFailed
		}
		return r, nil
	}
	if g.statFn != nil {
		g.statFn(name)
	}
	return os.Lstat(wtPath)
}

// HasUncommittedPass is HasUncommitted that also returns the lstat results of
// a dirty scan for DiffShortstatPass (Story 4.2.4). The pass is nil when scan_skip
// is off, the verdict came from the result cache, or nothing needs diffing.
func (g *GoGitVCSReader) HasUncommittedPass(worktreePath string) (bool, *statPass, error) {
	if !g.effectiveFlags().ScanSkip {
		dirty, err := g.HasUncommitted(worktreePath)
		return dirty, nil, err
	}
	if v, ok := g.hasUncommittedCache.Load(worktreePath); ok {
		if e := v.(hasUncommittedEntry); time.Now().Before(e.expiry) {
			return e.result, nil, nil
		}
	}
	defer g.guardInvalidation(worktreePath)()
	type outcome struct {
		dirty bool
		pass  *statPass
	}
	res, err := sfDo(&g.scanSkip.passSF, worktreePath, func() (outcome, error) {
		entry, err := g.openRepoEntry(worktreePath)
		if err != nil {
			return outcome{}, err
		}
		dirty, pass, err := g.hasUncommittedSkipPass(entry, worktreePath, true)
		return outcome{dirty, pass}, err
	})
	if err != nil {
		return false, nil, err
	}
	return res.dirty, res.pass, nil
}

// DiffShortstatPass is DiffShortstat reusing pass's lstat results (nil: none).
func (g *GoGitVCSReader) DiffShortstatPass(worktreePath string, pass *statPass) (DiffStat, error) {
	if pass == nil {
		return g.DiffShortstat(worktreePath)
	}
	if v, ok := g.diffStatCache.Load(worktreePath); ok {
		if e := v.(diffStatEntry); time.Now().Before(e.expiry) {
			return e.result, nil
		}
	}
	defer g.guardInvalidation(worktreePath)()
	return sfDo(&g.diffStatSF, worktreePath, func() (DiffStat, error) {
		stat, err := g.diffShortstatUncachedPass(worktreePath, pass)
		if err != nil {
			return DiffStat{}, err
		}
		g.diffStatCache.Store(worktreePath, diffStatEntry{result: stat, expiry: time.Now().Add(diffStatCacheTTL)})
		return stat, nil
	})
}
