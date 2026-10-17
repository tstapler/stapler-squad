package unfinished

// Task 4.3.2b tests: DirtyGen gates the parsed-index cache and the untracked
// verdict cache, never the tracked-file stat pass; the scanner feeds it from
// watcher events. Deterministic: injected clock, fake watcher, no sleeps.

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dirtyGenReader is a scan_skip reader with the DirtyGen sub-feature on and a
// manual clock that starts a minute ahead of the file system.
func dirtyGenReader() (*GoGitVCSReader, *atomic.Int64) {
	var offset atomic.Int64
	offset.Store(int64(time.Minute))
	g := &GoGitVCSReader{}
	g.scanSkip.flags = &Flags{ScanSkip: true, WatchWorktreeGitdirs: true, ScanSkipDirtyGen: true}
	g.scanSkip.clock = func() time.Time { return time.Now().Add(time.Duration(offset.Load())) }
	return g, &offset
}

func statCounting(g *GoGitVCSReader) *atomic.Int64 {
	var n atomic.Int64
	g.statFn = func(string) { n.Add(1) }
	return &n
}

func TestDirtyGenIntegration_ShouldRunStatPassEveryScan_WhenTierAArmedAndCachesHit(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n", "d/b.txt": "b\n", "d/c.txt": "c\n"})
	g, _ := dirtyGenReader()
	stats := statCounting(g)
	g.ArmDirty(dir)
	require.False(t, rescan(t, g, dir)) // first scan after arming: full walk

	for i := range 3 {
		before := stats.Load()
		require.False(t, rescan(t, g, dir))
		assert.EqualValues(t, 3, stats.Load()-before, "scan %d: the stat pass ran over every tracked file", i)
	}
	s := g.ScanSkipStats()
	assert.Greater(t, s.IndexHits, int64(0), "engagement: the index cache served the armed scans")
	assert.False(t, g.dirtyGenFor(dir).ShouldRunStatPass() == false)

	// An in-place edit is invisible to every watcher; the stat pass finds it.
	require.NoError(t, writeFile(dir, "a.txt", "edited in place\n"))
	assert.True(t, rescan(t, g, dir))
}

func TestDirtyGenIntegration_ShouldHitCaches_WhenScansSpacedAtScanInterval(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n", "d/b.txt": "b\n"})
	g, offset := dirtyGenReader()
	g.ArmDirty(dir)
	require.False(t, rescan(t, g, dir)) // first scan after arming: full walk

	for i := 1; i <= 2; i++ {
		offset.Add(int64(defaultScanTickInterval))
		before := g.ScanSkipStats()
		require.False(t, rescan(t, g, dir))
		after := g.ScanSkipStats()
		assert.Greater(t, after.IndexHits, before.IndexHits, "scan %d at exactly the scan interval reached the index cache", i)
		assert.Greater(t, after.DirsSkipped, before.DirsSkipped, "scan %d reached the untracked-dir verdict cache", i)
	}
}

func TestDirtyGenIntegration_ShouldFullWalk_WhenWatcherUnarmed(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n"})
	g, _ := dirtyGenReader()
	before := DirtyFullWalkTotal(ReasonUnarmed)

	rescan(t, g, dir)
	rescan(t, g, dir)

	assert.Zero(t, g.ScanSkipStats().IndexHits, "unarmed: clean needs proof, so every scan re-parses")
	assert.EqualValues(t, 2, g.ScanSkipStats().IndexDecodes)
	assert.GreaterOrEqual(t, DirtyFullWalkTotal(ReasonUnarmed)-before, int64(2))
}

func TestDirtyGenIntegration_ShouldReparseOnce_WhenIndexEventHintArrives(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n"})
	g, _ := dirtyGenReader()
	g.ArmDirty(dir)
	rescan(t, g, dir)
	rescan(t, g, dir)
	hits := g.ScanSkipStats().IndexHits
	decodes := g.ScanSkipStats().IndexDecodes
	require.Greater(t, hits, int64(0))

	g.NoteDirty(dir, EventTrackedPath)
	assert.False(t, rescan(t, g, dir), "a hint never reports dirty: HEAD/index events also fire for clean checkouts")
	assert.Equal(t, decodes+1, g.ScanSkipStats().IndexDecodes, "the hint forced one re-parse")
	rescan(t, g, dir)
	assert.Equal(t, decodes+1, g.ScanSkipStats().IndexDecodes, "the hint was acknowledged; the cache serves again")
}

func TestDirtyGenIntegration_ShouldFullWalkAndCount_WhenOverflowOrAgeElapses(t *testing.T) {
	t.Parallel()
	_, dir := trackedRepo(t, map[string]string{"a.txt": "a\n"})
	g, offset := dirtyGenReader()
	g.ArmDirty(dir)
	rescan(t, g, dir)
	rescan(t, g, dir)
	decodes := g.ScanSkipStats().IndexDecodes

	overflow := DirtyFullWalkTotal(ReasonOverflow)
	g.NoteDirty(dir, EventOverflow)
	rescan(t, g, dir)
	assert.Equal(t, decodes+1, g.ScanSkipStats().IndexDecodes)
	assert.GreaterOrEqual(t, DirtyFullWalkTotal(ReasonOverflow)-overflow, int64(1))
	rescan(t, g, dir)
	assert.Equal(t, decodes+1, g.ScanSkipStats().IndexDecodes, "the completed full walk cleared the overflow")

	age := DirtyFullWalkTotal(ReasonAge)
	offset.Add(int64(fullWalkMaxAge))
	rescan(t, g, dir)
	assert.Equal(t, decodes+2, g.ScanSkipStats().IndexDecodes, "fullWalkMaxAge forces a full walk regardless of events")
	assert.GreaterOrEqual(t, DirtyFullWalkTotal(ReasonAge)-age, int64(1))
}

func TestScannerWatch_ShouldArmAndFeedDirtyGen_WhenTierAWatchesWorktrees(t *testing.T) {
	t.Parallel()
	f := newLinkedFixture(t)
	g, _ := dirtyGenReader()
	s, fw := newSeamScanner(true)
	s.reader = g
	s.repoSet.Store(f.mainDir, true)
	s.indexRepoWorktree(f.mainDir, f.linked1)

	s.watchRepo(f.mainDir)
	for _, p := range []string{f.mainDir, f.linked1, f.linked2} {
		dec := g.dirtyGenFor(p).Peek()
		assert.Equal(t, ReasonUnarmed, dec.Reason, "armed but never walked: first scan is a full walk (%s)", p)
		assert.True(t, g.dirtyGenFor(p).armed.Load(), p)
		d := g.dirtyGenFor(p)
		d.CompleteFullWalk(d.Decide()) // pretend the first full walk happened
		assert.False(t, d.Peek().FullWalk, p)
	}

	runLoop(t, s)
	send(t, fw, fsnotify.Event{Name: filepath.Join(f.adminDir1, "index"), Op: fsnotify.Write})
	nextTask(t, s)
	assert.True(t, g.dirtyGenFor(f.linked1).Peek().DirtyHint, "index event on wt1's admin dir hints wt1")
	assert.False(t, g.dirtyGenFor(f.linked2).Peek().DirtyHint, "and only wt1")

	s.inFlight.Delete(f.mainDir)
	select {
	case fw.errs <- fsnotify.ErrEventOverflow:
	case <-time.After(10 * time.Second):
		t.Fatal("event loop not receiving")
	}
	nextTask(t, s)
	assert.Equal(t, ReasonOverflow, g.dirtyGenFor(f.mainDir).Peek().Reason)
	assert.Equal(t, ReasonOverflow, g.dirtyGenFor(f.linked1).Peek().Reason)

	s.unwatchRepo(f.mainDir)
	assert.False(t, g.dirtyGenFor(f.linked1).armed.Load(), "unwatching disarms")
}
