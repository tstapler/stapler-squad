package repostate

import (
	"context"
	"runtime"
	"sync/atomic"

	"github.com/go-git/go-git/v5/plumbing"
)

// ParentsSource reads one commit's parents. The adapter (Story 2.1.2) wraps
// the go-git storer; this package never touches a storer itself. A shallow
// boundary commit reports truncated=true (its parent objects are absent) and
// no parents. Implementations must be deterministic per hash.
type ParentsSource interface {
	Parents(ctx context.Context, h plumbing.Hash) (parents []plumbing.Hash, truncated bool, err error)
}

// claimSpinRounds bounds how long a claim loser waits for the holder before it
// decodes locally without publishing (ADR-009 item 3).
const claimSpinRounds = 8

// Fill makes row id readable under stamp ordinal ord and appends its parent
// ids to dst. The result state is Filled (parents appended) or Truncated
// (shallow boundary). The row is filled at most once per epoch by whoever wins
// the Pending->Filling CAS; a loser waits a bounded number of Gosched rounds
// and then decodes locally without publishing, so a descheduled holder delays
// only the spin, never correctness. Lock-free, not wait-free.
func (g *Generation) Fill(ctx context.Context, id uint32, src ParentsSource, ord StampOrd, dst []uint32) ([]uint32, RowState, error) {
	word := g.stateWord(id)
	lost := false
	markLost := func() {
		if !lost {
			lost = true
			g.stats.fillLostCAS.V.Add(1)
		}
	}
	for spins := 0; ; {
		w := word.Load()
		st, o := unpackState(w)
		switch {
		case st == Filled:
			out, _ := g.AppendParents(dst, id)
			return out, Filled, nil
		case st == Truncated && o == ord:
			return dst, Truncated, nil
		case st == Filling:
			markLost()
			if spins == claimSpinRounds {
				return g.decodeLocal(ctx, id, src, dst)
			}
			spins++
			runtime.Gosched()
			continue
		}
		// Pending (including a holder's rollback), or Truncated under a stale ordinal.
		g.yield()
		if !word.CompareAndSwap(w, packState(Filling, 0)) {
			g.stats.casRetries.V.Add(1)
			markLost()
			continue
		}
		if st == Truncated {
			g.stats.reprobe.V.Add(1)
		}
		return g.fillClaimed(ctx, id, word, src, ord, dst)
	}
}

// fillClaimed runs as the claim holder. The deferred rollback returns the row
// to Pending on every exit that did not publish (error, cancellation, panic,
// closed storer); the payload is unpublished then, so no reader saw it.
func (g *Generation) fillClaimed(ctx context.Context, id uint32, word *atomic.Uint32, src ParentsSource, ord StampOrd, dst []uint32) (out []uint32, st RowState, err error) {
	published := false
	defer func() {
		if !published {
			g.yield()
			word.Store(packState(Pending, 0))
			g.stats.rollback.V.Add(1)
		}
	}()

	parents, truncated, err := src.Parents(ctx, g.Hash(id))
	if err != nil {
		return dst, Pending, err
	}
	if truncated {
		g.yield()
		word.Store(packState(Truncated, ord))
		published = true
		return dst, Truncated, nil
	}
	base := len(dst)
	dst, err = g.internAll(dst, parents)
	if err != nil {
		return dst[:base], Pending, err
	}
	if err := g.writePayload(id, dst[base:]); err != nil {
		g.stats.graphFull.V.Add(1)
		return dst[:base], Pending, err
	}
	g.yield()
	word.Store(packState(Filled, 0))
	published = true
	return dst, Filled, nil
}

// decodeLocal is the losing path: read parents without publishing.
func (g *Generation) decodeLocal(ctx context.Context, id uint32, src ParentsSource, dst []uint32) ([]uint32, RowState, error) {
	g.stats.fillLostDecode.V.Add(1)
	parents, truncated, err := src.Parents(ctx, g.Hash(id))
	if err != nil {
		return dst, Pending, err
	}
	if truncated {
		return dst, Truncated, nil
	}
	base := len(dst)
	dst, err = g.internAll(dst, parents)
	if err != nil {
		return dst[:base], Pending, err
	}
	return dst, Filled, nil
}

func (g *Generation) internAll(dst []uint32, hashes []plumbing.Hash) ([]uint32, error) {
	for _, h := range hashes {
		pid, err := g.Intern(h)
		if err != nil {
			return dst, err
		}
		dst = append(dst, pid)
	}
	return dst, nil
}
