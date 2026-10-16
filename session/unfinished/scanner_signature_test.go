package unfinished

// Story 4.1.2: ScanSignature skip. The fake reader scripts HEAD/base/shallow
// and counts every call, so each assertion is a call count, never a timing.

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// missingFromCoverage lists the fields of typ that table does not declare, and
// the table entries naming no field.
func missingFromCoverage(typ reflect.Type, table map[string]fieldClass) (missing, extra []string) {
	seen := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		seen[name] = true
		if _, ok := table[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range table {
		if !seen[name] {
			extra = append(extra, name)
		}
	}
	return missing, extra
}

func TestScanResultCoverageTable_ShouldFail_WhenNewFieldMissingFromTable(t *testing.T) {
	t.Parallel()
	missing, extra := missingFromCoverage(reflect.TypeOf(ScanResult{}), scanResultCoverage)
	assert.Empty(t, missing, "declare a dependency class in scanResultCoverage for every new ScanResult field")
	assert.Empty(t, extra, "scanResultCoverage names a field ScanResult no longer has")

	// The check itself must be able to fail.
	type grown struct {
		AheadCount int
		NewField   string
	}
	miss, _ := missingFromCoverage(reflect.TypeOf(grown{}), scanResultCoverage)
	assert.Equal(t, []string{"NewField"}, miss)
}

func TestScanSignature_ShouldReuseCommittedFieldsAndStillRunHasUncommitted_WhenSiblingCommits(t *testing.T) {
	t.Parallel()
	p := newPreludeRepo(t, 2)
	r := newPreludeReader()
	hasUncommitted := &countingHasUncommitted{preludeReader: r}
	s := NewScannerWithReader(nil, nil, hasUncommitted)
	w1, w2 := p.wts[0], p.wts[1]
	for i, wt := range []string{w1, w2} {
		testrepo.CommitAll(t, testrepo.OpenLinked(t, wt), map[string]string{"f.txt": "x\n"}, "ahead", testrepo.Epoch.Add(time.Duration(i+1)*time.Hour))
	}
	s.scanRepo(p.mainDir, true)
	resolved0, uncommitted0 := r.resolved.Load(), hasUncommitted.calls.Load()

	// A commit in W1 only: W2's HEAD, base and shallow stamp are unchanged.
	testrepo.CommitAll(t, testrepo.OpenLinked(t, w1), map[string]string{"g.txt": "y\n"}, "second", testrepo.Epoch.Add(5*time.Hour))
	results := s.scanRepo(p.mainDir, true)

	assert.EqualValues(t, 1, r.resolved.Load()-resolved0, "AheadBehind recomputed for W1 only; main and W2 are unchanged")
	assert.EqualValues(t, 3, hasUncommitted.calls.Load()-uncommitted0, "HasUncommitted runs for every worktree regardless")
	var w2Result ScanResult
	for _, res := range results {
		if res.WorktreePath == w2 {
			w2Result = res
		}
	}
	assert.Equal(t, 1, w2Result.AheadCount, "W2's reused committed state is still reported")
	assert.Equal(t, []string{"ahead"}, trimHashes(w2Result.AheadMessages), "reused messages")
	st := s.ScanStateStats()
	assert.EqualValues(t, 2, st.SignatureSkips, "main and W2 skipped")
	assert.EqualValues(t, 6, st.SignatureChecks)
}

// countingHasUncommitted counts HasUncommitted calls (the stat pass entry).
type countingHasUncommitted struct {
	*preludeReader
	calls atomic.Int64
}

func (c *countingHasUncommitted) HasUncommittedPass(wt string) (bool, *statPass, error) {
	c.calls.Add(1)
	return c.preludeReader.HasUncommittedPass(wt)
}

// trimHashes strips the "<short-hash> " prefix CommitMessages puts on each line.
func trimHashes(msgs []string) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		if len(m) > 8 {
			m = m[8:]
		}
		out[i] = m
	}
	return out
}

// sigFake scripts the refSnapshotReader capability with no go-git at all.
type sigFake struct {
	mu        sync.Mutex
	wts       []WorktreeInfo
	heads     map[string]plumbing.Hash
	base      plumbing.Hash
	shallow   repostate.ShallowStamp
	index     map[string]statSig
	order     []string // HasUncommitted call order
	resolved  atomic.Int64
	inFlight  atomic.Int64
	maxFlight atomic.Int64
}

func newSigFake(paths ...string) *sigFake {
	f := &sigFake{heads: map[string]plumbing.Hash{}, index: map[string]statSig{}, base: hashOf(1)}
	for i, p := range paths {
		f.wts = append(f.wts, WorktreeInfo{Path: p, Branch: "b" + p})
		f.heads[p] = hashOf(byte(i + 2))
	}
	return f
}

func hashOf(b byte) plumbing.Hash { var h plumbing.Hash; h[0] = b; return h }

func (f *sigFake) effectiveFlags() Flags { return preludeFlags }
func (f *sigFake) ListWorktrees(string) ([]WorktreeInfo, error) {
	return f.wts, nil
}
func (f *sigFake) ResolveDefaultBranch(string) string { return "main" }
func (f *sigFake) HasUncommitted(wt string) (bool, error) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		m := f.maxFlight.Load()
		if n <= m || f.maxFlight.CompareAndSwap(m, n) {
			break
		}
	}
	f.mu.Lock()
	f.order = append(f.order, wt)
	f.mu.Unlock()
	return false, nil
}
func (f *sigFake) AheadBehind(string, string) (int, int, error)         { return 0, 0, nil }
func (f *sigFake) CommitMessages(string, string, int) ([]string, error) { return nil, nil }
func (f *sigFake) DiffShortstat(string) (DiffStat, error)               { return DiffStat{}, nil }

func (f *sigFake) ResolveRefSnapshot(_, base string, paths []string) *RefSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	snap := &RefSnapshot{Base: base, BaseHash: f.base, Shallow: f.shallow, Worktrees: map[string]WorktreeRefs{}}
	for _, p := range paths {
		snap.Worktrees[p] = WorktreeRefs{Head: f.heads[p], Index: f.index[p]}
	}
	return snap
}

// AheadBehindResolved derives counts from the hash bytes so a stale reuse is visible.
func (f *sigFake) AheadBehindResolved(_, _ string, head, base plumbing.Hash) (int, int, error) {
	f.resolved.Add(1)
	return int(head[0]), int(base[0]), nil
}

func (f *sigFake) set(mutate func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mutate()
}

func scanFake(s *Scanner) []ScanResult { return s.scanRepo("/repo", true) }

func TestScanSignature_ShouldRecompute_WhenShallowStampOrBaseHashChanges(t *testing.T) {
	t.Parallel()
	f := newSigFake("/a", "/b")
	s := NewScannerWithReader(nil, nil, f)
	scanFake(s)
	require.EqualValues(t, 2, f.resolved.Load())

	scanFake(s)
	assert.EqualValues(t, 2, f.resolved.Load(), "equal signatures: committed state reused")

	f.set(func() { f.shallow = repostate.ShallowStamp{Exists: true, Size: 40, MtimeNs: 9} })
	results := scanFake(s)
	assert.EqualValues(t, 4, f.resolved.Load(), "a ShallowStamp change recomputes every worktree")
	assert.Equal(t, 1, results[0].BehindCount)

	f.set(func() { f.base = hashOf(7) })
	results = scanFake(s)
	assert.EqualValues(t, 6, f.resolved.Load(), "a base hash change recomputes every worktree")
	for _, res := range results {
		assert.Equal(t, 7, res.BehindCount, "reported behind follows the new base, not the stored one")
	}
}

func TestScanSignature_ShouldExpire_WhenClockAdvancesPastOneTick(t *testing.T) {
	t.Parallel()
	f := newSigFake("/a")
	s := NewScannerWithReader(nil, nil, f)
	s.SetTickInterval(5 * time.Minute)
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	s.clock = clk.Now

	scanFake(s)
	clk.t = clk.t.Add(time.Minute)
	scanFake(s)
	require.EqualValues(t, 1, f.resolved.Load(), "within a tick the stored state is reused")

	clk.t = clk.t.Add(5 * time.Minute)
	scanFake(s)
	assert.EqualValues(t, 2, f.resolved.Load(), "past one tick the stored signature is expired and recomputed")
}

func TestScanOrder_ShouldProcessChangedWorktreesFirst_WhenW1W3ChangedW2Not(t *testing.T) {
	t.Parallel()
	f := newSigFake("/w1", "/w2", "/w3")
	s := NewScannerWithReader(nil, nil, f)
	scanFake(s)

	f.set(func() {
		f.heads["/w1"] = hashOf(20)
		f.index["/w3"] = statSig{mtimeNs: 5, size: 1} // an index rewrite alone also counts as changed
		f.order = nil
	})
	scanFake(s)

	assert.Equal(t, []string{"/w1", "/w3", "/w2"}, f.order)
}

func TestEnqueueRepo_ShouldNotSkipRepo_WhenSiblingSignatureChanged(t *testing.T) {
	t.Parallel()
	f := newSigFake("/w1", "/w2")
	s := NewScannerWithReader(nil, nil, f)
	repo := "/repo"
	s.scanRepo(repo, false) // leaves fresh caches for both worktrees

	s.enqueueRepo(repo, false)
	assert.Empty(t, s.scanQueue, "nothing moved: the fresh caches still skip the repo")

	f.set(func() { f.heads["/w1"] = hashOf(30) })
	s.enqueueRepo(repo, false)
	assert.Len(t, s.scanQueue, 1, "W1's HEAD moved while W2's cache is fresh: the repo must be scanned")
}

func TestScanPool_ShouldRunSequentially_WhenPoolOptionUnset(t *testing.T) {
	t.Parallel()
	f := newSigFake("/a", "/b", "/c", "/d")
	s := NewScannerWithReader(nil, nil, f)

	scanFake(s)

	assert.EqualValues(t, 1, f.maxFlight.Load(), "worktrees of one repo are scanned one at a time")
}

// SG8 on a scripted fixture: four worktrees, one HEAD moves per scan. After the
// cold scan, three of four committed-state lookups are served from the record.
func TestScanSignature_ShouldEngageForUnchangedSiblings_OnScriptedFixture(t *testing.T) {
	t.Parallel()
	paths := []string{"/a", "/b", "/c", "/d"}
	f := newSigFake(paths...)
	s := NewScannerWithReader(nil, nil, f)
	scanFake(s)
	const rounds = 20
	for i := 0; i < rounds; i++ {
		f.set(func() { f.heads[paths[i%len(paths)]] = hashOf(byte(100 + i)) })
		scanFake(s)
	}
	st := s.ScanStateStats()
	assert.EqualValues(t, (rounds+1)*len(paths), st.SignatureChecks)
	assert.EqualValues(t, rounds*(len(paths)-1), st.SignatureSkips, "engagement = 60/84 = 71.4 percent of lookups")
}
