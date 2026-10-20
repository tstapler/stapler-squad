package unfinished

// Story 5.1.1: the shadow-verify sampler. The RNG is injected, so which scans
// are sampled is a function of the seed, never of timing.

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// oracleFake is a sigFake whose new path reports (head[0], base[0]) and whose
// oracle answers are scripted, so a deliberately wrong new path is a one-line change.
type oracleFake struct {
	*sigFake
	mu            sync.Mutex
	oracleAhead   func(call int) (int, int)
	oracleDirty   bool
	oracleCalls   atomic.Int64
	pressure      bool
	aheadCallsSeq int
}

func newOracleFake(paths ...string) *oracleFake {
	f := &oracleFake{sigFake: newSigFake(paths...)}
	f.oracleAhead = func(int) (int, int) { return 0, 0 }
	return f
}

func (f *oracleFake) UnderSeverePressure() bool { return f.pressure }

func (f *oracleFake) OracleAheadBehind(wt, _ string) (int, int, error) {
	f.oracleCalls.Add(1)
	f.mu.Lock()
	f.aheadCallsSeq++
	n := f.aheadCallsSeq
	f.mu.Unlock()
	a, b := f.oracleAhead(n)
	return a, b, nil
}
func (f *oracleFake) OracleCommitMessages(string, string, int) ([]string, error) { return nil, nil }
func (f *oracleFake) OracleHasUncommitted(string) (bool, error) {
	f.oracleCalls.Add(1)
	return f.oracleDirty, nil
}
func (f *oracleFake) OracleDiffShortstat(string) (DiffStat, error) { return DiffStat{}, nil }
func (f *oracleFake) OracleBegin()                                 {}
func (f *oracleFake) OracleDone()                                  {}

// newShadowScanner returns a scanner that samples every scan and records debug lines.
func newShadowScanner(f *oracleFake) (*Scanner, *[]string) {
	s := NewScannerWithReader(nil, nil, f)
	s.shadow.intn = func(int) int { return 0 }
	var mu sync.Mutex
	var lines []string
	s.debugLog = func(msg string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		line := msg
		for i := 0; i+1 < len(args); i += 2 {
			line += fmt.Sprintf(" %v=%v", args[i], args[i+1])
		}
		lines = append(lines, line)
	}
	return s, &lines
}

func TestShadowVerify_ShouldCountMismatchAndLogBothValues_WhenNewPathDisagreesWithOracle(t *testing.T) {
	t.Parallel()
	f := newOracleFake("/a")
	// New path reports ahead = head[0] = 2, behind = base[0] = 1; the oracle says ahead 5.
	f.oracleAhead = func(int) (int, int) { return 5, 1 }
	s, lines := newShadowScanner(f)

	scanFake(s)

	st := s.ParityStats()
	assert.EqualValues(t, 1, st.Mismatch[parityAhead], "the deliberately wrong new path moves the counter")
	assert.Zero(t, st.Mismatch[parityBehind])
	assert.EqualValues(t, 1, st.Sampled)
	require.Len(t, *lines, 1)
	assert.Contains(t, (*lines)[0], "field=ahead")
	assert.Contains(t, (*lines)[0], "new=2")
	assert.Contains(t, (*lines)[0], "oracle=5", "the debug line carries both values")
}

func TestShadowVerify_ShouldCountDirtyMismatch_WhenOracleSeesUncommittedChanges(t *testing.T) {
	t.Parallel()
	f := newOracleFake("/a")
	f.oracleAhead = func(int) (int, int) { return 2, 1 } // agrees with the new path
	f.oracleDirty = true                                 // new path reported clean
	s, _ := newShadowScanner(f)

	scanFake(s)

	st := s.ParityStats()
	assert.EqualValues(t, 1, st.Mismatch[parityDirty])
	assert.Zero(t, st.Mismatch[parityAhead]+st.Mismatch[parityBehind])
}

func TestShadowVerify_ShouldStayQuiet_WhenNewPathAgreesWithOracle(t *testing.T) {
	t.Parallel()
	f := newOracleFake("/a")
	f.oracleAhead = func(int) (int, int) { return 2, 1 }
	s, lines := newShadowScanner(f)

	scanFake(s)

	st := s.ParityStats()
	assert.EqualValues(t, 1, st.Sampled)
	assert.Equal(t, [numParityFields]int64{}, st.Mismatch)
	assert.Equal(t, [numParityFields]int64{}, st.Transient)
	assert.Empty(t, *lines)
}

func TestShadowVerify_ShouldDiscardTransient_WhenOracleChangesBetweenReads(t *testing.T) {
	t.Parallel()
	f := newOracleFake("/a")
	// A ref moves between the two oracle reads: first 5, then 6.
	f.oracleAhead = func(call int) (int, int) { return 4 + call, 1 }
	s, lines := newShadowScanner(f)

	scanFake(s)

	st := s.ParityStats()
	assert.Zero(t, st.Mismatch[parityAhead], "an unstable oracle is not a parity bug")
	assert.EqualValues(t, 1, st.Transient[parityAhead])
	assert.Empty(t, *lines)
}

func TestShadowVerify_ShouldNeverSample_WhenSeverePressure(t *testing.T) {
	t.Parallel()
	f := newOracleFake("/a", "/b")
	f.pressure = true
	s, _ := newShadowScanner(f)

	scanFake(s)

	st := s.ParityStats()
	assert.Zero(t, st.Sampled)
	assert.EqualValues(t, 2, st.SkippedPressure)
	assert.Zero(t, f.oracleCalls.Load(), "no oracle work under severe pressure")
}

func TestShadowVerify_ShouldStayOff_WhenEveryRollbackUnitIsOff(t *testing.T) {
	t.Parallel()
	f := newOracleFake("/a")
	f.flags = &Flags{}
	s, _ := newShadowScanner(f)

	scanFake(s)

	assert.Zero(t, s.ParityStats().Sampled, "with the legacy path as the only path there is nothing to compare")
	assert.Zero(t, f.oracleCalls.Load())
}

func TestShadowVerify_ShouldSampleAtDefaultRate_WhenSeededRNG(t *testing.T) {
	t.Parallel()
	const scans = 2000
	seed := func() *rand.Rand { return rand.New(rand.NewPCG(7, 11)) }
	want := 0
	replay := seed()
	for range scans {
		if replay.IntN(defaultShadowVerifyEvery) == 0 {
			want++
		}
	}
	require.Positive(t, want, "seed must sample at least once")

	f := newOracleFake("/a")
	f.oracleAhead = func(int) (int, int) { return 2, 1 }
	s, _ := newShadowScanner(f)
	rng := seed()
	s.shadow.intn = rng.IntN
	for range scans {
		scanFake(s)
	}

	got := s.ParityStats().Sampled
	assert.EqualValues(t, want, got, "sampled-scan counter equals the seeded draw count")
	assert.Less(t, got, int64(scans/10), "added CPU is bounded by the sample rate")
}

// The oracle code paths agree with the new paths on real go-git history and a
// dirty worktree: zero mismatches, and the sampler really ran.
func TestShadowVerify_ShouldReportNoMismatch_WhenRealNewPathsMatchLegacy(t *testing.T) {
	t.Parallel()
	p := newPreludeRepo(t, 1)
	wt := p.wts[0]
	wtRepo := testrepo.OpenLinked(t, wt)
	testrepo.CommitAll(t, wtRepo, map[string]string{"f.txt": "one\ntwo\n"}, "ahead", testrepo.Epoch.Add(time.Hour))
	testrepo.WriteFiles(t, wt, map[string]string{"f.txt": "one\nTWO\nthree\n", "new.txt": "x\n"})

	r := newPreludeReader()
	s := NewScannerWithReader(nil, nil, r)
	s.shadow.intn = func(int) int { return 0 }

	results := s.scanRepo(p.mainDir, true)

	var got ScanResult
	for _, res := range results {
		if res.WorktreePath == wt {
			got = res
		}
	}
	require.True(t, got.HasUncommitted, "precondition: the fixture is dirty")
	require.Equal(t, 1, got.AheadCount)
	st := s.ParityStats()
	assert.GreaterOrEqual(t, st.Sampled, int64(2), "both worktrees sampled")
	assert.Equal(t, [numParityFields]int64{}, st.Mismatch)
	assert.Equal(t, [numParityFields]int64{}, st.Transient)
	assert.Zero(t, st.OracleErrors, "every oracle read answered: the comparison was not vacuous")
}

// Two overlapping sampled runs share one oracle: the first Done must not close
// the repo handles the second still reads; only the last Done clears.
func TestShadowOracle_ShouldClearOnlyOnLastRelease_WhenRunsOverlap(t *testing.T) {
	t.Parallel()
	p := newPreludeRepo(t, 1)
	g := newPreludeReader()

	g.OracleBegin()
	g.OracleBegin()
	_, err := g.OracleHasUncommitted(p.wts[0])
	require.NoError(t, err)
	oracle := g.scanSkip.oracle.Load()
	require.NotNil(t, oracle)
	require.EqualValues(t, 1, atomic.LoadInt64(&oracle.repoCacheSize), "precondition: oracle holds a handle")

	g.OracleDone()
	assert.EqualValues(t, 1, atomic.LoadInt64(&oracle.repoCacheSize), "first release must keep the handle the other run reads")
	_, err = g.OracleHasUncommitted(p.wts[0])
	assert.NoError(t, err, "the surviving run still gets answers")

	g.OracleDone()
	assert.Zero(t, atomic.LoadInt64(&oracle.repoCacheSize), "last release clears")
	g.OracleDone() // unbalanced: no-op, no panic
	assert.Zero(t, g.scanSkip.oracleUsers.Load())
}
