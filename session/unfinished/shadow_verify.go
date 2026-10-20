package unfinished

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
)

// Shadow verification (Story 5.1.1, pre-mortem P1 #4). The corpus-bound oracles
// cannot see real-history shapes, octopus merges, deepen-in-place or a storer
// closed mid-walk, so a sampled fraction of live scans also runs the legacy
// implementation beside the new one and counts any disagreement.

// defaultShadowVerifyEvery samples one scan in 200. 1 in 50 made the oracle 43%
// of the scanner's allocations in the S2 A/B; STAPLER_SQUAD_SHADOW_VERIFY_EVERY
// raises the rate for a parity soak.
const defaultShadowVerifyEvery = 200

// parityField names a ScanResult value the sampler compares.
type parityField uint8

const (
	parityAhead parityField = iota
	parityBehind
	parityMessages
	parityDirty
	parityShortstat
	numParityFields
)

func (f parityField) String() string {
	switch f {
	case parityAhead:
		return "ahead"
	case parityBehind:
		return "behind"
	case parityMessages:
		return "messages"
	case parityDirty:
		return "dirty"
	case parityShortstat:
		return "shortstat"
	}
	return "unknown"
}

// parityCounters back parity_mismatch_total{field} and its siblings.
type parityCounters struct {
	mismatch  [numParityFields]atomic.Int64
	transient [numParityFields]atomic.Int64
	// sampled counts scans the sampler ran an oracle for: the added CPU is
	// bounded by sampled/scans, i.e. the sample rate.
	sampled         atomic.Int64
	skippedPressure atomic.Int64
	// oracleErrors counts oracle reads that failed: each one is a comparison
	// that did not happen, so a non-zero value means coverage is lower than the
	// sample rate suggests.
	oracleErrors atomic.Int64
}

// ParityStats is a snapshot of parityCounters.
type ParityStats struct {
	Mismatch, Transient [numParityFields]int64
	Sampled             int64
	SkippedPressure     int64
	OracleErrors        int64
}

func (c *parityCounters) snapshot() ParityStats {
	var s ParityStats
	for i := range numParityFields {
		s.Mismatch[i] = c.mismatch[i].Load()
		s.Transient[i] = c.transient[i].Load()
	}
	s.Sampled = c.sampled.Load()
	s.SkippedPressure = c.skippedPressure.Load()
	s.OracleErrors = c.oracleErrors.Load()
	return s
}

// shadowVerifier is the sampler's state. The zero value samples at the default
// rate with the process RNG; tests inject intn.
type shadowVerifier struct {
	// intn returns a uniform value in [0, n); nil means math/rand/v2.IntN.
	intn func(n int) int
	// override replaces the configured rate when non-zero; negative is off.
	override atomic.Int64
	counters parityCounters
}

func (v *shadowVerifier) draw(n int) int {
	if v.intn != nil {
		return v.intn(n)
	}
	return rand.IntN(n) //nolint:gosec // sampling, not security
}

// SetShadowVerifyEvery overrides the sampling rate: every n-th scan on average
// (n <= 0 turns it off).
func (s *Scanner) SetShadowVerifyEvery(n int) {
	if n <= 0 {
		n = -1
	}
	s.shadow.override.Store(int64(n))
}

// ParityStats returns the parity counters.
func (s *Scanner) ParityStats() ParityStats { return s.shadow.counters.snapshot() }

// shadowEvery is the effective sampling denominator; 0 means off. With every
// rollback unit off the new path is the legacy path, so there is nothing to
// compare and the sampler stays off.
func (s *Scanner) shadowEvery() int {
	f := s.readerFlags()
	if !f.SharedCommitState && !f.DiffCounter && !f.ScanSkip {
		return 0
	}
	switch n := s.shadow.override.Load(); {
	case n < 0:
		return 0
	case n > 0:
		return int(n)
	}
	return defaultShadowVerifyEvery
}

// readerFlags is the flag view this scanner's reader uses.
func (s *Scanner) readerFlags() Flags {
	if v, ok := s.reader.(flagView); ok {
		return v.effectiveFlags()
	}
	return EffectiveFlags()
}

// oracleReader is the optional VCSReader capability that runs the legacy
// implementations (plainWalk for commit data, the flags-off HasUncommitted and
// DiffShortstat for working-tree data), uncached and independent of the new
// path's state.
type oracleReader interface {
	OracleAheadBehind(worktreePath, base string) (ahead, behind int, err error)
	OracleCommitMessages(worktreePath, base string, limit int) ([]string, error)
	OracleHasUncommitted(worktreePath string) (bool, error)
	OracleDiffShortstat(worktreePath string) (DiffStat, error)
	// OracleBegin registers a sampled run; each must be matched by one OracleDone.
	OracleBegin()
	// OracleDone releases one run; the last one releases the oracle's memory.
	OracleDone()
}

// shadowRun is one sampled scan.
type shadowRun struct {
	s      *Scanner
	oracle oracleReader
	repo   string
	done   atomic.Bool
}

// beginShadow decides whether this scan is sampled. A sampled scan evicts the
// reader's TTL-cached results first, so the new path is computed fresh and the
// comparison is not against a value up to 30 s old. It returns nil (and does
// nothing) when the scan is not sampled or the process is under severe pressure.
func (s *Scanner) beginShadow(repoPath, worktreePath string) *shadowRun {
	oracle, ok := s.reader.(oracleReader)
	if !ok {
		return nil
	}
	every := s.shadowEvery()
	if every <= 0 || s.shadow.draw(every) != 0 {
		return nil
	}
	if p, ok := s.reader.(pressureReader); ok && p.UnderSeverePressure() {
		s.shadow.counters.skippedPressure.Add(1)
		return nil
	}
	s.shadow.counters.sampled.Add(1)
	if inv, ok := s.reader.(worktreeInvalidator); ok {
		inv.InvalidateWorktree(worktreePath)
	}
	oracle.OracleBegin()
	return &shadowRun{s: s, oracle: oracle, repo: repoPath}
}

// release gives the oracle back once; Nil-safe and idempotent so scanWorktree
// can defer it on every exit path (finish releases earlier on success).
func (r *shadowRun) release() {
	if r != nil && r.done.CompareAndSwap(false, true) {
		r.oracle.OracleDone()
	}
}

// verdict is the outcome of comparing the new path with the oracle.
type verdict uint8

const (
	verdictAgree verdict = iota
	// verdictMismatch: the oracle repeated itself and disagrees with the new path.
	verdictMismatch
	// verdictTransient: the oracle disagreed with itself between two reads, so
	// the worktree or a ref moved mid-check; not counted as a mismatch.
	verdictTransient
	// verdictOracleError: the oracle could not answer; nothing to compare.
	verdictOracleError
)

// compareStable runs oracle and, when it disagrees with got, runs it once more:
// a disagreement counts only if the oracle repeats itself, because an edit or a
// ref move between the two reads would otherwise look like a bug.
func compareStable[T comparable](got T, oracle func() (T, error)) (want T, v verdict) {
	want, err := oracle()
	if err != nil {
		return want, verdictOracleError
	}
	if want == got {
		return want, verdictAgree
	}
	again, err := oracle()
	if err != nil || again != want {
		return want, verdictTransient
	}
	return want, verdictMismatch
}

// finish compares result with the oracle path. Nil-safe.
func (r *shadowRun) finish(result *ScanResult, base string) {
	if r == nil {
		return
	}
	defer r.release()
	wt := result.WorktreePath
	if base != "" {
		r.verifyAheadBehind(result, wt, base)
	}
	want, v := compareStable(result.HasUncommitted, func() (bool, error) { return r.oracle.OracleHasUncommitted(wt) })
	r.record(v, wt, result.HasUncommitted, want, parityDirty)
	if result.HasUncommitted {
		got := DiffStat{Files: result.ChangedFiles, Insertions: result.LinesAdded, Deletions: result.LinesRemoved}
		wantStat, v := compareStable(got, func() (DiffStat, error) { return r.oracle.OracleDiffShortstat(wt) })
		r.record(v, wt, got, wantStat, parityShortstat)
	}
}

func (r *shadowRun) verifyAheadBehind(result *ScanResult, wt, base string) {
	type pair [2]int
	got := pair{result.AheadCount, result.BehindCount}
	want, v := compareStable(got, func() (pair, error) {
		a, b, err := r.oracle.OracleAheadBehind(wt, base)
		return pair{a, b}, err
	})
	if got[0] != want[0] {
		r.record(v, wt, got[0], want[0], parityAhead)
	}
	if got[1] != want[1] {
		r.record(v, wt, got[1], want[1], parityBehind)
	}
	if want[0] == 0 || v == verdictOracleError {
		return
	}
	gotMsgs := strings.Join(result.AheadMessages, "\n")
	wantMsgs, v := compareStable(gotMsgs, func() (string, error) {
		m, err := r.oracle.OracleCommitMessages(wt, base, 5)
		return strings.Join(m, "\n"), err
	})
	r.record(v, wt, gotMsgs, wantMsgs, parityMessages)
}

// record counts one comparison outcome for field f.
func (r *shadowRun) record(v verdict, worktree string, got, want any, f parityField) {
	switch v {
	case verdictMismatch:
		r.mismatch(f, worktree, got, want)
	case verdictTransient:
		r.s.shadow.counters.transient[f].Add(1)
	case verdictOracleError:
		r.s.shadow.counters.oracleErrors.Add(1)
	case verdictAgree:
	}
}

func (r *shadowRun) mismatch(f parityField, worktree string, got, want any) {
	r.s.shadow.counters.mismatch[f].Add(1)
	r.s.logDebug("shadow verify: parity mismatch",
		"field", f.String(), "repo", r.repo, "worktree", worktree,
		"new", fmt.Sprint(got), "oracle", fmt.Sprint(want))
}

// logDebug and debugEnabled are the scanner's debug seam (default: the process
// logger, gated by its runtime level) so a test can assert on emitted lines.
func (s *Scanner) logDebug(msg string, args ...any) {
	if s.debugLog != nil {
		s.debugLog(msg, args...)
		return
	}
	log.Debug(msg, args...)
}

func (s *Scanner) debugEnabled() bool {
	if s.debugOn != nil {
		return s.debugOn()
	}
	return log.IsDebugEnabled()
}
