package unfinished

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/puzpuzpuz/xsync/v4"
)

// maxCachedDirs bounds one worktree's verdict map; past it new directories are
// simply not stored (they re-walk), so memory stays proportional to the cap.
const maxCachedDirs = 50_000

// untrackedCache is the per-worktree untracked-directory verdict cache
// (Story 4.2.3). The current generation is an immutable header plus a
// concurrent map; a header change swaps in an empty generation, which is the
// "re-walk the affected scope" rule for every key component except a single
// directory's own signature.
type untrackedCache struct {
	accessedAtNs atomic.Int64
	gen          atomic.Pointer[untrackedGen]
	// omitAncestorChain is a mutation seam for the model test only: it drops
	// the ancestor .gitignore markers from every key so the test can prove it
	// would notice their absence. Never set in production.
	omitAncestorChain bool
}

// untrackedGenKey is everything a directory verdict depends on besides the
// directory itself and its ancestor .gitignore chain.
type untrackedGenKey struct {
	// matcher identity: a rebuilt matcher may classify differently even when
	// no .gitignore stat changed (it was built from an older read).
	matcher gitignore.Matcher
	// excludes fingerprints .git/info/exclude and the global excludes file.
	// The current matcher does not read them; the fingerprint keeps the cache
	// correct if it ever does.
	excludes uint64
	// tracked-set generation: the index signature observed before the index
	// was read, plus its entry count.
	indexSig statSig
	nIndexed int
}

type untrackedGen struct {
	key  untrackedGenKey
	dirs *xsync.Map[string, *dirVerdict]

	omitAncestorChain bool // copied from untrackedCache (test seam)
}

// dirVerdict is one directory's classified entries, in ReadDir order: the
// untracked files to report and the non-ignored subdirectories to descend
// into. Immutable once stored.
type dirVerdict struct {
	sig      statSig   // the directory's own signature when listed
	chain    uint64    // ancestor .gitignore markers including this directory's own
	storedAt time.Time // clock reading before the listing (racy and age checks)
	hasGI    bool      // the directory held a .gitignore when listed
	items    []untrackedItem
}

type untrackedItem struct {
	name  string
	isDir bool
}

// gitignoreMarker folds one directory's .gitignore existence and signature
// into the running chain hash (FNV-style, 64-bit). A collision needs a 2^-64
// coincidence of every stat field, accepted as negligible.
func gitignoreMarker(parent uint64, sig statSig, present bool) uint64 {
	h := parent
	mix := func(v uint64) { h = (h ^ v) * 0x100000001b3 }
	if !present {
		mix(0)
		return h
	}
	mix(1)
	mix(uint64(sig.mtimeNs)) //nolint:gosec // hashing, wraparound is fine
	mix(uint64(sig.ctimeNs)) //nolint:gosec // hashing, wraparound is fine
	mix(uint64(sig.size))    //nolint:gosec // hashing, wraparound is fine
	mix(sig.ino)
	return h
}

const chainSeed uint64 = 0xcbf29ce484222325

// excludesFingerprint hashes the stat signatures of the per-repo and global
// exclude files.
func excludesFingerprint(worktreePath string) uint64 {
	h := chainSeed
	for _, p := range excludeFilePaths(worktreePath) {
		sig, ok := statPathSig(p)
		h = gitignoreMarker(h, sig, ok)
	}
	return h
}

func excludeFilePaths(worktreePath string) []string {
	paths := []string{filepath.Join(gitCommonDir(worktreePath), "info", "exclude")}
	if dir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(dir, "git", "ignore"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "git", "ignore"))
	}
	return paths
}

// untrackedGenFor returns the generation to walk with, or nil when the walk
// must not use the cache (flag off, no trustworthy tracked-set signature).
// fresh keeps the generation but makes every directory re-list (a forced full
// walk refreshing the cache).
func (g *GoGitVCSReader) untrackedGenFor(worktreePath string, indexed map[string]struct{}, matcher gitignore.Matcher, tg trackedGen) *untrackedGen {
	if !g.effectiveFlags().ScanSkip || !tg.ok || !tg.sig.trustedAt(tg.observedAt) {
		if g.effectiveFlags().ScanSkip && tg.ok {
			g.scanSkip.stats.dirsRacyRefused.Add(1)
		}
		return nil
	}
	key := untrackedGenKey{matcher: matcher, excludes: excludesFingerprint(worktreePath), indexSig: tg.sig, nIndexed: len(indexed)}
	c, _ := g.scanSkip.untracked.Load(worktreePath)
	if c == nil {
		fresh := &untrackedCache{}
		g.scanSkip.untracked.Store(worktreePath, fresh) // last writer wins; both are valid empty caches
		c, _ = g.scanSkip.untracked.Load(worktreePath)
	}
	c.accessedAtNs.Store(g.scanSkip.now().UnixNano())
	cur := c.gen.Load()
	if cur != nil && cur.key == key {
		return cur
	}
	next := &untrackedGen{key: key, dirs: xsync.NewMap[string, *dirVerdict](), omitAncestorChain: c.omitAncestorChain}
	if c.gen.CompareAndSwap(cur, next) {
		return next
	}
	if cur = c.gen.Load(); cur != nil && cur.key == key {
		return cur
	}
	return next // a concurrent walker moved the generation; use a private one for this walk
}

// cachedWalker is the verdict-cache walk shared by hasUntrackedSkip and
// walkUntrackedSkip. It threads one parts slice and one byte buffer through
// the walk like untrackedWalker; single-goroutine, depth-first.
type cachedWalker struct {
	g       *GoGitVCSReader
	gen     *untrackedGen
	indexed map[string]struct{}
	matcher gitignore.Matcher
	fresh   bool
	parts   []string
	buf     []byte
	onFile  func(abs string) error
}

func (w *cachedWalker) enter(name string) (prevBuf int) {
	w.parts = append(w.parts, name)
	prevBuf = len(w.buf)
	if prevBuf > 0 {
		w.buf = append(w.buf, '/')
	}
	w.buf = append(w.buf, name...)
	return prevBuf
}

func (w *cachedWalker) leave(prevBuf int) {
	w.parts = w.parts[:len(w.parts)-1]
	w.buf = w.buf[:prevBuf]
}

// lookup returns the stored verdict for abs when its directory signature,
// own .gitignore marker, ancestor chain and age all still match.
func (w *cachedWalker) lookup(abs string, dsig statSig, parentChain uint64) (*dirVerdict, bool) {
	if w.fresh {
		return nil, false
	}
	v, ok := w.gen.dirs.Load(abs)
	if !ok || v.sig != dsig || w.g.scanSkip.now().Sub(v.storedAt) >= scanSkipMaxAge {
		return nil, false
	}
	// An unchanged directory signature means an unchanged entry set, so
	// .gitignore existence is unchanged; only an in-place edit needs a stat.
	if chain, _ := gitignoreChain(abs, parentChain, v.hasGI); chain != v.chain {
		return nil, false
	}
	return v, true
}

// gitignoreChain folds abs's own .gitignore (when present) into parentChain
// and reports that file's newest timestamp for the racy check.
func gitignoreChain(abs string, parentChain uint64, present bool) (chain uint64, newest int64) {
	if !present {
		return gitignoreMarker(parentChain, statSig{}, false), 0
	}
	sig, ok := lstatPathSig(filepath.Join(abs, ".gitignore"))
	return gitignoreMarker(parentChain, sig, ok), sig.newest()
}

// walkDir visits abs: a verdict-cache hit costs one directory stat (plus one
// .gitignore stat when the directory has one), a miss lists and classifies it.
// chainNewest carries the newest timestamp among ancestor .gitignore files.
func (w *cachedWalker) walkDir(abs string, parentChain uint64, chainNewest int64) error {
	if w.gen.omitAncestorChain {
		parentChain, chainNewest = chainSeed, 0
	}
	dsig, sigOK := statPathSig(abs)
	stats := &w.g.scanSkip.stats
	if sigOK {
		if v, hit := w.lookup(abs, dsig, parentChain); hit {
			stats.dirsSkipped.Add(1)
			return w.visit(abs, v.items, v.chain, max(chainNewest, ownNewest(abs, v.hasGI)))
		}
	}
	startedAt := w.g.scanSkip.now()
	entries, err := os.ReadDir(abs)
	if err != nil {
		return err
	}
	stats.dirsWalked.Add(1)
	hasGI := false
	for _, de := range entries {
		if de.Name() == ".gitignore" && !de.IsDir() {
			hasGI = true
			break
		}
	}
	chain, giNewest := gitignoreChain(abs, parentChain, hasGI)
	newest := max(chainNewest, giNewest)
	items := w.classify(entries)
	if sigOK {
		w.store(abs, &dirVerdict{sig: dsig, chain: chain, storedAt: startedAt, hasGI: hasGI, items: items}, max(newest, dsig.newest()))
	}
	return w.visit(abs, items, chain, newest)
}

func ownNewest(abs string, present bool) int64 {
	if !present {
		return 0
	}
	_, n := gitignoreChain(abs, 0, true)
	return n
}

// store keeps v unless its inputs are inside the racy window or the map is full.
func (w *cachedWalker) store(abs string, v *dirVerdict, newest int64) {
	if v.storedAt.UnixNano()-newest <= int64(racyWindow) {
		w.g.scanSkip.stats.dirsRacyRefused.Add(1)
		return
	}
	if w.gen.dirs.Size() >= maxCachedDirs {
		return
	}
	w.gen.dirs.Store(abs, v)
}

// classify applies the same rules as untrackedWalker to one directory's
// entries: ignored entries vanish, directories are kept for descent, and
// untracked files are kept for reporting.
func (w *cachedWalker) classify(entries []os.DirEntry) []untrackedItem {
	var items []untrackedItem
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		prev := w.enter(name)
		isDir := de.IsDir()
		switch {
		case w.matcher != nil && w.matcher.Match(w.parts, isDir):
		case isDir:
			items = append(items, untrackedItem{name: name, isDir: true})
		default:
			if _, tracked := w.indexed[string(w.buf)]; !tracked {
				items = append(items, untrackedItem{name: name})
			}
		}
		w.leave(prev)
	}
	return items
}

// visit reports each untracked file and descends into each subdirectory, in
// the order ReadDir produced them.
func (w *cachedWalker) visit(abs string, items []untrackedItem, chain uint64, chainNewest int64) error {
	for _, it := range items {
		prev := w.enter(it.name)
		child := filepath.Join(abs, it.name)
		var err error
		if it.isDir {
			err = w.walkDir(child, chain, chainNewest)
		} else {
			err = w.onFile(child)
		}
		w.leave(prev)
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *GoGitVCSReader) newCachedWalker(gen *untrackedGen, indexed map[string]struct{}, matcher gitignore.Matcher, fresh bool, onFile func(string) error) *cachedWalker {
	return &cachedWalker{g: g, gen: gen, indexed: indexed, matcher: matcher, fresh: fresh, buf: make([]byte, 0, 256), onFile: onFile}
}

// errFoundUntracked ends a has-untracked walk at the first untracked file.
var errFoundUntracked = errors.New("untracked file found")

// hasUntrackedSkip is hasUntrackedFiles served from the verdict cache when a
// trustworthy tracked-set signature is available.
func (g *GoGitVCSReader) hasUntrackedSkip(worktreePath string, indexed map[string]struct{}, matcher gitignore.Matcher, tg trackedGen, fresh bool) (bool, error) {
	gen := g.untrackedGenFor(worktreePath, indexed, matcher, tg)
	if gen == nil {
		return hasUntrackedFiles(worktreePath, indexed, matcher)
	}
	w := g.newCachedWalker(gen, indexed, matcher, fresh, func(string) error { return errFoundUntracked })
	err := w.walkDir(worktreePath, chainSeed, 0)
	if errors.Is(err, errFoundUntracked) {
		return true, nil
	}
	return false, err
}

// walkUntrackedSkip is walkUntracked served from the verdict cache. Files are
// still reported (and by the caller read) every call; only the directory
// listing and classification are skipped.
func (g *GoGitVCSReader) walkUntrackedSkip(worktreePath string, indexed map[string]struct{}, matcher gitignore.Matcher, tg trackedGen, fn func(absPath string)) error {
	gen := g.untrackedGenFor(worktreePath, indexed, matcher, tg)
	if gen == nil {
		return walkUntracked(worktreePath, indexed, matcher, fn)
	}
	count := 0
	w := g.newCachedWalker(gen, indexed, matcher, g.scanSkipPeekForce(worktreePath), func(abs string) error {
		count++
		if count > maxUntrackedFiles {
			return errStopWalk
		}
		fn(abs)
		return nil
	})
	if err := w.walkDir(worktreePath, chainSeed, 0); err != nil && !errors.Is(err, errStopWalk) {
		return err
	}
	return nil
}
