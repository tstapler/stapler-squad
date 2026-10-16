package unfinished

// Story 4.1.1: the per-Burst RefSnapshot. Every test is deterministic: no
// sleeps, no watcher goroutines; events are delivered by calling handleEvent.

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// preludeFlags is the scan_skip prelude on (scan_skip needs shared_commit_state
// for it). repoState reads the process-wide flag, which stays off here, so
// AheadBehindResolved takes the legacy phase 2.
var preludeFlags = Flags{ScanSkip: true, SharedCommitState: true, ScanSkipPrelude: true}

// preludeReader counts the phase-1 (AheadBehind) and hashes-in
// (AheadBehindResolved) entry points on a real go-git reader.
type preludeReader struct {
	*GoGitVCSReader
	full, resolved atomic.Int64
}

func newPreludeReader() *preludeReader {
	g := &GoGitVCSReader{}
	f := preludeFlags
	g.scanSkip.flags = &f
	return &preludeReader{GoGitVCSReader: g}
}

func (r *preludeReader) AheadBehind(wt, base string) (int, int, error) {
	r.full.Add(1)
	return r.GoGitVCSReader.AheadBehind(wt, base)
}

func (r *preludeReader) AheadBehindResolved(wt, base string, head, baseHash plumbing.Hash) (int, int, error) {
	r.resolved.Add(1)
	return r.GoGitVCSReader.AheadBehindResolved(wt, base, head, baseHash)
}

// preludeRepo is a main repo (branch main, 3 commits) plus linked worktrees.
type preludeRepo struct {
	mainDir string
	wts     []string // linked worktrees only
}

func newPreludeRepo(t *testing.T, linked int) preludeRepo {
	t.Helper()
	repo, dir := testrepo.NewRepo(t)
	testrepo.Linear(t, repo, 3)
	return preludeRepo{mainDir: dir, wts: testrepo.LinkedWorktrees(t, dir, linked)}
}

func (p preludeRepo) all() []string { return append([]string{p.mainDir}, p.wts...) }

func TestBurst_ShouldResolveBaseOnceAndEachHeadOnce_WhenFourWorktrees(t *testing.T) {
	t.Parallel()
	p := newPreludeRepo(t, 3)
	r := newPreludeReader()
	var shallowStats atomic.Int64
	r.statFn = func(rel string) {
		if rel == "shallow" {
			shallowStats.Add(1)
		}
	}
	s := NewScannerWithReader(nil, nil, r)

	s.scanRepo(p.mainDir, true)

	got := r.RefResolveStats()
	assert.Equal(t, RefResolveStats{Snapshots: 1, BaseResolves: 1, HeadResolves: 4, ShallowStats: 1}, got,
		"one snapshot: base once, each of the 4 HEADs once, shallow stat'ed once")
	assert.EqualValues(t, 1, shallowStats.Load(), "the statFn seam saw exactly one shallow stat")
	assert.EqualValues(t, 4, r.resolved.Load(), "phase 2 ran from the snapshot's hashes for every worktree")
	assert.Zero(t, r.full.Load(), "phase 1 is skipped for the Burst")
	st := s.ScanStateStats()
	assert.EqualValues(t, 1, st.SnapshotsBuilt)
	assert.Zero(t, st.ReusedAcrossBursts)
	assert.Nil(t, s.burstSnapshot(p.mainDir), "the snapshot is discarded when the Burst ends")
}

func TestBurst_ShouldNotBuildSnapshot_WhenPreludeFlagOff(t *testing.T) {
	t.Parallel()
	p := newPreludeRepo(t, 2)
	r := newPreludeReader()
	off := Flags{ScanSkip: true} // scan_skip without shared_commit_state: prelude normalized off
	r.scanSkip.flags = &off
	s := NewScannerWithReader(nil, nil, r)

	s.scanRepo(p.mainDir, true)

	assert.Zero(t, r.RefResolveStats().Snapshots)
	assert.EqualValues(t, 3, r.full.Load(), "legacy phase 1 per worktree")
	assert.Zero(t, r.resolved.Load())
}

func TestRefSnapshot_ShouldRebuild_WhenGitEventArrivesBetweenBursts(t *testing.T) {
	t.Parallel()
	p := newPreludeRepo(t, 1)
	r := newPreludeReader()
	s := NewScannerWithReader(nil, nil, r)
	s.gitwatch.watchWorktreeGitdirs = true
	paths := p.all()

	s.beginBurst(p.mainDir, "main", paths)
	require.NotNil(t, s.burstSnapshot(p.mainDir))

	// A .git event mid-Burst drops the snapshot: the rest of the Burst takes the
	// plain path and the next Burst builds a new one.
	ev := fsnotify.Event{Name: filepath.Join(p.mainDir, ".git", "HEAD"), Op: fsnotify.Write}
	s.handleEvent(context.Background(), ev)
	assert.Nil(t, s.burstSnapshot(p.mainDir), "event drops the snapshot")
	assert.EqualValues(t, 1, s.ScanStateStats().SnapshotsDropped)

	_, _, err := s.aheadBehind(s.burstSnapshot(p.mainDir), p.mainDir, "main")
	require.NoError(t, err)
	assert.EqualValues(t, 1, r.full.Load(), "no snapshot: the reader's full call resolves refs itself")

	s.endBurst(p.mainDir)
	s.beginBurst(p.mainDir, "main", paths)
	assert.NotNil(t, s.burstSnapshot(p.mainDir), "next Burst builds a fresh snapshot")
	assert.EqualValues(t, 2, s.ScanStateStats().SnapshotsBuilt)
	assert.Zero(t, s.ScanStateStats().ReusedAcrossBursts)
	s.endBurst(p.mainDir)
}

func TestRefSnapshot_ShouldReportNewAheadBehind_WhenBaseMovedThroughStorerWithoutEvent(t *testing.T) {
	t.Parallel()
	p := newPreludeRepo(t, 1)
	r := newPreludeReader()
	s := NewScannerWithReader(nil, nil, r)
	wt := p.wts[0]

	// One commit ahead of main in the linked worktree.
	wtRepo := testrepo.OpenLinked(t, wt)
	testrepo.CommitAll(t, wtRepo, map[string]string{"a.txt": "a\n"}, "ahead", testrepo.Epoch.Add(time.Hour))

	first := scanResultFor(t, s, p.mainDir, wt)
	require.Equal(t, 1, first.AheadCount)
	require.Equal(t, 0, first.BehindCount)

	// Move main through the storer only: no fsnotify event is ever delivered.
	mainRepo := openRepo(t, p.mainDir)
	head, err := mainRepo.Head()
	require.NoError(t, err)
	headCommit, err := mainRepo.CommitObject(head.Hash())
	require.NoError(t, err)
	next := testrepo.RawCommit(t, mainRepo, headCommit.TreeHash, []plumbing.Hash{head.Hash()}, "main moved", testrepo.Epoch.Add(2*time.Hour))
	testrepo.SetBranch(t, mainRepo, "main", next)

	second := scanResultFor(t, s, p.mainDir, wt)
	assert.Equal(t, 1, second.AheadCount)
	assert.Equal(t, 1, second.BehindCount, "the next Burst re-resolves refs, so the base move is seen")
	assert.Zero(t, s.ScanStateStats().ReusedAcrossBursts, "refsnapshot_reused_across_bursts stays 0")
	assert.EqualValues(t, 2, s.ScanStateStats().SnapshotsBuilt)
}

func TestBurst_ShouldNotFillGraph_WhenSeverePressureOrAtStart(t *testing.T) {
	t.Parallel()
	t.Run("severe pressure builds no snapshot", func(t *testing.T) {
		t.Parallel()
		p := newPreludeRepo(t, 1)
		r := &pressuredReader{preludeReader: newPreludeReader()}
		s := NewScannerWithReader(nil, nil, r)

		s.beginBurst(p.mainDir, "main", p.all())

		assert.Nil(t, s.burstSnapshot(p.mainDir))
		assert.Zero(t, r.RefResolveStats().Snapshots, "no ref work under severe pressure; callers take the plain path")
	})
	// A Burst reads refs and stats only: no commit is decoded at its start, so
	// no graph row can be filled by the prelude.
	t.Run("start reads no commit", func(t *testing.T) {
		t.Parallel()
		p := newPreludeRepo(t, 2)
		c := NewCountingReader()
		f := preludeFlags
		c.scanSkip.flags = &f
		for _, wt := range p.all() {
			c.Track(t, wt)
		}
		s := NewScannerWithReader(nil, nil, c)

		s.beginBurst(p.mainDir, "main", p.all())

		require.NotNil(t, s.burstSnapshot(p.mainDir))
		assert.Zero(t, c.DecodeCount(), "the prelude never reads a commit object")
		s.endBurst(p.mainDir)
	})
}

type pressuredReader struct{ *preludeReader }

func (*pressuredReader) UnderSeverePressure() bool { return true }

// scanResultFor force-scans repoPath and returns worktree wt's result.
func scanResultFor(t *testing.T, s *Scanner, repoPath, wt string) ScanResult {
	t.Helper()
	for _, res := range s.scanRepo(repoPath, true) {
		if res.WorktreePath == wt {
			return res
		}
	}
	t.Fatalf("no unfinished result for %s", wt)
	return ScanResult{}
}

func openRepo(t *testing.T, dir string) *git.Repository {
	t.Helper()
	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	return repo
}
