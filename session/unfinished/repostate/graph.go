package repostate

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

// ParentsSource reads one commit's parents and committer time (unix seconds,
// which orders the paint-down merge-base queue). The adapter wraps the go-git
// storer; this package never touches a storer itself. truncated=true marks a
// commit whose object is absent (a shallow boundary): no parents, no time.
// Implementations must be deterministic per hash.
type ParentsSource interface {
	Parents(ctx context.Context, h plumbing.Hash) (parents []plumbing.Hash, when int64, truncated bool, err error)
}

const (
	// defaultClaimWait is how long a claim loser waits for the holder before
	// decoding locally (ADR-009 item 3, amended: a bounded spin of 8 Gosched
	// rounds measured 0.99 lost-decode per lost-claim on racing walks).
	defaultClaimWait = 200 * time.Millisecond
	waiterStripes    = 64
)

// Fill makes row id readable under stamp ordinal ord and appends its parent
// ids to dst. The result state is Filled (parents appended) or Truncated
// (shallow boundary). The row is filled at most once per epoch by whoever wins
// the Pending->Filling CAS; a loser waits a bounded number of Gosched rounds
// and then decodes locally without publishing, so a descheduled holder delays
// only the spin, never correctness. Lock-free, not wait-free.
func (g *Generation) Fill(ctx context.Context, id uint32, src ParentsSource, ord StampOrd, dst []uint32) ([]uint32, RowState, error) {
	out, _, st, err := g.fill(ctx, id, src, ord, dst)
	return out, st, err
}

// fill is Fill that also returns the row's committer time (0 for Truncated).
func (g *Generation) fill(ctx context.Context, id uint32, src ParentsSource, ord StampOrd, dst []uint32) ([]uint32, int64, RowState, error) {
	word := g.stateWord(id)
	lost := false
	markLost := func() {
		if !lost {
			lost = true
			g.stats.fillLostCAS.V.Add(1)
		}
	}
	for waited := false; ; {
		w := word.Load()
		st, o := unpackState(w)
		switch {
		case st == Filled:
			out, _ := g.AppendParents(dst, id)
			return out, g.When(id), Filled, nil
		case st == Truncated && o == ord:
			return dst, 0, Truncated, nil
		case st == Filling:
			markLost()
			if waited {
				return g.decodeLocal(ctx, id, src, dst)
			}
			waited = !g.awaitPublish(ctx, id, word)
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
func (g *Generation) fillClaimed(ctx context.Context, id uint32, word *atomic.Uint32, src ParentsSource, ord StampOrd, dst []uint32) (out []uint32, when int64, st RowState, err error) {
	published := false
	defer func() {
		if !published {
			g.yield()
			word.Store(packState(Pending, 0))
			g.wake(id)
			g.stats.rollback.V.Add(1)
		}
	}()

	parents, when, truncated, err := src.Parents(ctx, g.Hash(id))
	if err != nil {
		return dst, 0, Pending, err
	}
	if truncated {
		g.yield()
		word.Store(packState(Truncated, ord))
		published = true
		g.wake(id)
		return dst, 0, Truncated, nil
	}
	base := len(dst)
	dst, err = g.internAll(dst, parents)
	if err != nil {
		return dst[:base], 0, Pending, err
	}
	if err := g.writePayload(id, dst[base:], when); err != nil {
		g.noteFull()
		return dst[:base], 0, Pending, err
	}
	g.yield()
	word.Store(packState(Filled, 0))
	published = true
	g.wake(id)
	return dst, when, Filled, nil
}

// awaitPublish blocks a claim loser until the row leaves Filling (reporting
// true) or ClaimWait elapses or ctx ends (false). The holder stores the new
// state and then closes the stripe's channel; the loser registers the channel
// and then re-checks the state, so one of the two orders always wins and no
// wakeup is missed. A wake of a shared stripe is spurious for other rows, which
// re-check and wait again.
func (g *Generation) awaitPublish(ctx context.Context, id uint32, word *atomic.Uint32) bool {
	stripe := &g.waiters[id%waiterStripes]
	timer := time.NewTimer(g.hooks.ClaimWait)
	defer timer.Stop()
	for {
		if st, _ := unpackState(word.Load()); st != Filling {
			return true
		}
		cp := stripe.Load()
		if cp == nil {
			c := make(chan struct{})
			cp = &c
			if !stripe.CompareAndSwap(nil, cp) {
				continue
			}
		}
		if st, _ := unpackState(word.Load()); st != Filling {
			return true
		}
		select {
		case <-*cp:
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// wake releases the losers waiting on id's stripe. Called after the publishing
// store; the cheap Load keeps the uncontended publish free of an atomic swap.
func (g *Generation) wake(id uint32) {
	stripe := &g.waiters[id%waiterStripes]
	if stripe.Load() == nil {
		return
	}
	if cp := stripe.Swap(nil); cp != nil {
		close(*cp)
	}
}

// decodeLocal is the losing path: read parents without publishing.
func (g *Generation) decodeLocal(ctx context.Context, id uint32, src ParentsSource, dst []uint32) ([]uint32, int64, RowState, error) {
	g.stats.fillLostDecode.V.Add(1)
	parents, when, truncated, err := src.Parents(ctx, g.Hash(id))
	if err != nil {
		return dst, 0, Pending, err
	}
	if truncated {
		return dst, 0, Truncated, nil
	}
	base := len(dst)
	dst, err = g.internAll(dst, parents)
	if err != nil {
		return dst[:base], 0, Pending, err
	}
	return dst, when, Filled, nil
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
