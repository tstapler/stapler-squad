package repostate

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
)

// The memos below cache results that are pure functions of commit hashes, so
// no TTL guards them: a changed ref changes the key. Every key carries the walk
// cap and the ShallowStamp (deepening a shallow repository in place changes
// what a walk can reach), and no key holds a CommitID, so a result survives a
// graph epoch change. Each is a byte-budgeted RepoCache registered in
// TierProducts, the first tier ADR-007 drops.

// ABKey identifies one ahead/behind answer. Cap is the walk's per-side visit cap.
type ABKey struct {
	Repo    RepoKey
	Head    plumbing.Hash
	Base    plumbing.Hash
	Cap     int
	Shallow ShallowStamp
}

// ABResult is an ahead/behind answer.
type ABResult struct{ Ahead, Behind int }

// BehindKey identifies the commits-behind count of base against a merge base,
// shared by every head with that merge base.
type BehindKey struct {
	Repo      RepoKey
	Base      plumbing.Hash
	MergeBase plumbing.Hash
	Cap       int
	Shallow   ShallowStamp
}

// MessageKey identifies one CommitMessages list. Cap is the reach-set cap.
type MessageKey struct {
	Repo    RepoKey
	Head    plumbing.Hash
	Base    plumbing.Hash
	Max     int
	Cap     int
	Shallow ShallowStamp
}

// LineKey identifies one commit's first message line. It is hash-keyed alone:
// a commit's message never changes.
type LineKey struct {
	Repo RepoKey
	Hash plumbing.Hash
}

// Default per-memo budgets when Options.ProductBudgetBytes is zero.
const (
	defaultABBudgetBytes      = 256 << 10
	defaultBehindBudgetBytes  = 128 << 10
	defaultMessageBudgetBytes = 1 << 20
	defaultLineBudgetBytes    = 1 << 20

	abEntryBytes     = 160
	behindEntryBytes = 144
	stringHeader     = 16
	messageOverhead  = 192
	lineOverhead     = 128
)

type productCaches struct {
	ab       *RepoCache[ABKey, ABResult]
	behind   *RepoCache[BehindKey, int]
	messages *RepoCache[MessageKey, []string]
	lines    *RepoCache[LineKey, string]
}

func newProductCaches(opts Options) *productCaches {
	budget := func(def int64) int64 {
		if opts.ProductBudgetBytes > 0 {
			return opts.ProductBudgetBytes
		}
		return def
	}
	return &productCaches{
		ab: NewRepoCache[ABKey, ABResult](budget(defaultABBudgetBytes), 0, func(ABResult) int { return abEntryBytes }, opts),
		behind: NewRepoCache[BehindKey, int](budget(defaultBehindBudgetBytes), 0,
			func(int) int { return behindEntryBytes }, opts),
		messages: NewRepoCache[MessageKey, []string](budget(defaultMessageBudgetBytes), 0, sizeOfLines, opts),
		lines: NewRepoCache[LineKey, string](budget(defaultLineBudgetBytes), 0,
			func(s string) int { return lineOverhead + len(s) }, opts),
	}
}

func sizeOfLines(lines []string) int {
	n := messageOverhead
	for _, l := range lines {
		n += stringHeader + len(l)
	}
	return n
}

// ProductStats are the memo counters of one RepoState.
type ProductStats struct {
	AheadBehind, Behind, Messages, Lines CacheStats
}

// memos returns the repository's memos, creating and registering them in
// TierProducts on first use.
func (s *RepoState) memos() *productCaches {
	if p := s.memo.Load(); p != nil {
		return p
	}
	fresh := newProductCaches(s.opts)
	if !s.memo.CompareAndSwap(nil, fresh) {
		return s.memo.Load()
	}
	for _, p := range []Product{fresh.ab, fresh.behind, fresh.messages, fresh.lines} {
		s.Register(TierProducts, p)
	}
	return fresh
}

// ProductStats snapshots the memo counters (zero before first use).
func (s *RepoState) ProductStats() ProductStats {
	p := s.memo.Load()
	if p == nil {
		return ProductStats{}
	}
	return ProductStats{AheadBehind: p.ab.Stats(), Behind: p.behind.Stats(), Messages: p.messages.Stats(), Lines: p.lines.Stats()}
}

// AheadBehind returns (ahead, behind) of want.Head against want.Base, memoized
// on the complete hash key; the behind count is memoized separately on
// (base, merge base) so heads sharing a merge base walk it once. The caller
// fills want.Repo, Head, Base and Cap; Shallow is set here. Walk failures keep
// the walk's own errors (ErrFallback means rerun the plain walk); errors are
// never stored.
func (s *RepoState) AheadBehind(ctx context.Context, src ParentsSource, ord StampOrd, want ABKey) (ABResult, error) {
	m := s.memos()
	key := want
	key.Shallow = s.ShallowStamp()
	if r, ok := m.ab.Get(key); ok {
		return r, nil
	}
	return m.ab.computeMiss(key, func() (ABResult, error) { return s.computeAheadBehind(ctx, m, src, ord, key) })
}

func (s *RepoState) computeAheadBehind(ctx context.Context, m *productCaches, src ParentsSource, ord StampOrd, key ABKey) (ABResult, error) {
	w := s.NewWalk(ctx, src, ord, key.Cap)
	defer w.Close()
	mb, err := w.FindMergeBase(key.Head, key.Base)
	if err != nil {
		if errors.Is(err, ErrFallback) {
			return ABResult{}, err
		}
		return ABResult{}, fmt.Errorf("merge base: %w", err)
	}
	ahead, err := w.CountCommitsTo(key.Head, mb)
	if err != nil {
		return ABResult{}, err
	}
	bk := BehindKey{Repo: key.Repo, Base: key.Base, MergeBase: mb, Cap: key.Cap, Shallow: key.Shallow}
	behind, ok := m.behind.Get(bk)
	if !ok {
		behind, err = m.behind.computeMiss(bk, func() (int, error) { return w.CountCommitsTo(key.Base, mb) })
		if err != nil {
			return ABResult{}, err
		}
	}
	return ABResult{Ahead: ahead, Behind: behind}, nil
}

// LineFunc reads one commit's first message line from the object store. It is
// called at most once per commit hash while the line memo retains the result.
type LineFunc func(h plumbing.Hash) (string, error)

// shortHashLen is the abbreviation CommitMessages prints, like `git log --oneline`.
const shortHashLen = 7

// MessageInputs bundles what CommitMessages reads: the parents source and stamp
// ordinal of the walk, the base's reach set and the first-line reader.
type MessageInputs struct {
	Src   ParentsSource
	Ord   StampOrd
	Reach *ReachBitset
	Line  LineFunc
}

// CommitMessages lists up to want.Max "short-hash subject" lines from want.Head
// back to the first commit already in in.Reach, memoized on the complete key.
// in.Reach must have been built under the same graph epoch; otherwise, as for
// any walk failure, the error is an ErrFallback and the caller runs the legacy
// walk. The traversal is go-git's default Log order (pre-order DFS, first
// parent first) ending at the first commit in reach, exactly as the legacy loop
// does. The returned slice is shared by every caller and must not be modified.
func (s *RepoState) CommitMessages(ctx context.Context, in MessageInputs, want MessageKey) ([]string, error) {
	m := s.memos()
	key := want
	key.Shallow = s.ShallowStamp()
	if v, ok := m.messages.Get(key); ok {
		return v, nil
	}
	return m.messages.computeMiss(key, func() ([]string, error) { return s.computeMessages(ctx, m, in, key) })
}

func (s *RepoState) computeMessages(ctx context.Context, m *productCaches, in MessageInputs, key MessageKey) ([]string, error) {
	w := s.NewWalk(ctx, in.Src, in.Ord, key.Cap)
	defer w.Close()
	if in.Reach.gen != w.gen {
		return nil, fallback(errors.New("reach set and walk are from different graph epochs"))
	}
	head, err := w.intern(key.Head)
	if err != nil {
		return nil, err
	}
	sc := w.s
	defer sc.seen.reset()
	sc.stack = append(sc.stack[:0], head)
	var out []string
	for len(sc.stack) > 0 {
		id := sc.stack[len(sc.stack)-1]
		sc.stack = sc.stack[:len(sc.stack)-1]
		if sc.seen.get(id) != 0 {
			continue
		}
		parents, missing, err := w.reachRead(id)
		if err != nil || missing {
			return nil, w.reachFail(err)
		}
		sc.seen.or(id, 1)
		if in.Reach.has(id) {
			break
		}
		if len(out) < key.Max {
			line, err := m.messageLine(w.gen.Hash(id), key.Repo, in.Line)
			if err != nil {
				return nil, fallback(err)
			}
			out = append(out, line)
		}
		pushUnseenReversed(sc, parents)
	}
	return out, nil
}

// pushUnseenReversed pushes parents so the first parent is popped first.
func pushUnseenReversed(sc *Scratch, parents []uint32) {
	for i := len(parents) - 1; i >= 0; i-- {
		if sc.seen.get(parents[i]) == 0 {
			sc.stack = append(sc.stack, parents[i])
		}
	}
}

// messageLine formats h's "short-hash subject" line, reading the subject once
// per commit hash through the memo.
func (m *productCaches) messageLine(h plumbing.Hash, repo RepoKey, line LineFunc) (string, error) {
	k := LineKey{Repo: repo, Hash: h}
	subject, ok := m.lines.Get(k)
	if !ok {
		var err error
		if subject, err = m.lines.computeMiss(k, func() (string, error) { return line(h) }); err != nil {
			return "", err
		}
	}
	return h.String()[:shortHashLen] + " " + subject, nil
}
