package unfinished

import (
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/puzpuzpuz/xsync/v4"
)

// FileDiffMemoKey identifies one working-tree file's content against one HEAD
// blob. Size, mtime, ctime and inode together stand in for the content: a
// timestamp-preserving rewrite restores mtime but not ctime, and a replacing
// rename changes the inode.
type FileDiffMemoKey struct {
	Path             string
	Size             int64
	MtimeNs, CtimeNs int64
	Inode            uint64
	HeadBlob         plumbing.Hash
}

// fileStamp is the stat identity memoKey is built from.
type fileStamp struct {
	Size             int64
	MtimeNs, CtimeNs int64
	Inode            uint64
}

const (
	// diffMemoRacyWindow mirrors git's racy-index rule: a file whose mtime or
	// ctime is within this window of the store time could be rewritten in the
	// same timestamp tick and still look unchanged, so it is never memoized.
	diffMemoRacyWindow = 2 * time.Second
	// diffMemoDefaultBudget bounds the memo (about 30,000 entries).
	diffMemoDefaultBudget = 4 << 20
	// diffMemoEntryOverhead approximates the per-entry cost beyond the path.
	diffMemoEntryOverhead = 112
)

// DiffMemoCounters are the memo's engagement counters.
type DiffMemoCounters struct {
	Hits, Misses, RacySkips, Evictions, Diffs int64
}

// fileDiffMemo caches per-file EditCounts. Reads and writes are lock-free on
// the caller side (xsync.Map plus atomics); only entries that pass the racy
// guard at store time are kept, so every stored entry is trusted.
type fileDiffMemo struct {
	m      *xsync.Map[FileDiffMemoKey, EditCounts]
	bytes  atomic.Int64
	budget int64
	now    func() time.Time // injectable for tests
	// stat is the platform stat seam; ok=false disables memoization.
	stat func(path string) (fileStamp, bool)

	hits, misses, racy, evictions, diffs atomic.Int64
}

func newFileDiffMemo(budget int64, now func() time.Time) *fileDiffMemo {
	if budget <= 0 {
		budget = diffMemoDefaultBudget
	}
	if now == nil {
		now = time.Now
	}
	return &fileDiffMemo{m: xsync.NewMap[FileDiffMemoKey, EditCounts](), budget: budget, now: now, stat: statFileStamp}
}

// activeDiffMemo is the process-wide memo. Keys carry absolute paths, inode and
// HEAD blob hash, so sharing one memo across readers cannot mix entries up.
// Tests swap it through setDiffMemoForTest.
var activeDiffMemo atomic.Pointer[fileDiffMemo]

func diffMemoInstance() *fileDiffMemo {
	if m := activeDiffMemo.Load(); m != nil {
		return m
	}
	activeDiffMemo.CompareAndSwap(nil, newFileDiffMemo(0, nil))
	return activeDiffMemo.Load()
}

// DiffMemoStats returns the process-wide memo's counters.
func DiffMemoStats() DiffMemoCounters { return diffMemoInstance().counters() }

func (f *fileDiffMemo) counters() DiffMemoCounters {
	return DiffMemoCounters{
		Hits: f.hits.Load(), Misses: f.misses.Load(), RacySkips: f.racy.Load(),
		Evictions: f.evictions.Load(), Diffs: f.diffs.Load(),
	}
}

// keyFor stats path and builds its key; ok=false means the file cannot be
// memoized (not a regular file, stat failed, or ctime is unavailable).
func (f *fileDiffMemo) keyFor(path string, head plumbing.Hash) (FileDiffMemoKey, bool) {
	st, ok := f.stat(path)
	if !ok {
		return FileDiffMemoKey{}, false
	}
	return FileDiffMemoKey{Path: path, Size: st.Size, MtimeNs: st.MtimeNs, CtimeNs: st.CtimeNs, Inode: st.Inode, HeadBlob: head}, true
}

func (f *fileDiffMemo) lookup(k FileDiffMemoKey) (EditCounts, bool) {
	c, ok := f.m.Load(k)
	if ok {
		f.hits.Add(1)
	} else {
		f.misses.Add(1)
	}
	return c, ok
}

// store records c under k unless the file is inside the racy window.
func (f *fileDiffMemo) store(k FileDiffMemoKey, c EditCounts) {
	newest := max(k.MtimeNs, k.CtimeNs)
	if f.now().UnixNano()-newest < int64(diffMemoRacyWindow) {
		f.racy.Add(1)
		return
	}
	if _, loaded := f.m.LoadOrStore(k, c); loaded {
		return
	}
	f.bytes.Add(entryBytes(k))
	if f.bytes.Load() > f.budget {
		f.evict()
	}
}

func entryBytes(k FileDiffMemoKey) int64 { return int64(len(k.Path)) + diffMemoEntryOverhead }

// evict deletes arbitrary entries until the memo fits its budget again.
func (f *fileDiffMemo) evict() {
	f.m.Range(func(k FileDiffMemoKey, _ EditCounts) bool {
		if _, loaded := f.m.LoadAndDelete(k); loaded {
			f.bytes.Add(-entryBytes(k))
			f.evictions.Add(1)
		}
		return f.bytes.Load() > f.budget
	})
}
