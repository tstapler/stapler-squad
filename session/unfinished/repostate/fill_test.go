package repostate

import (
	"context"
	"github.com/go-git/go-git/v5/plumbing"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Losers of a row's claim wait for the holder instead of decoding the commit
// again (ADR-009 item 3 as amended): one decode serves every waiter.
func TestFill_ShouldDecodeOnce_WhenLosersWaitForSlowHolder(t *testing.T) {
	const losers = 4
	d := newDAG(7, 3)
	h := d.hashes[2]
	g := NewGraph(Options{ClaimWait: time.Minute}).Load() // never the timeout: the wake must end the wait
	id := mustIntern(t, g, h)

	inside, release := make(chan struct{}), make(chan struct{})
	src := &mapSource{d: d, onParents: func(context.Context, plumbing.Hash) error {
		close(inside) // only the holder reaches here: losers never decode
		<-release
		return nil
	}}
	var wg sync.WaitGroup
	fill := func() {
		defer wg.Done()
		if _, st, err := g.Fill(context.Background(), id, src, 0, nil); err != nil || st != Filled {
			t.Errorf("Fill = %v, %v; want Filled", st, err)
		}
	}
	wg.Add(1)
	go fill()
	<-inside
	for range losers {
		wg.Add(1)
		go fill()
	}
	for g.stats.fillLostCAS.V.Load() < losers { // every loser has seen Filling and is waiting
		runtime.Gosched()
	}
	close(release)
	wg.Wait()

	if calls := src.calls.Load(); calls != 1 {
		t.Errorf("source read %d times, want 1 (losers must wait, not decode)", calls)
	}
	if lost := g.stats.fillLostDecode.V.Load(); lost != 0 {
		t.Errorf("fillLostDecode = %d, want 0", lost)
	}
}

func TestFill_ShouldDecodeLocally_WhenHolderOutlastsClaimWait(t *testing.T) {
	d := newDAG(8, 3)
	h := d.hashes[2]
	g := NewGraph(Options{ClaimWait: time.Millisecond}).Load()
	id := mustIntern(t, g, h)

	inside, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	holder := &mapSource{d: d, onParents: func(context.Context, plumbing.Hash) error {
		once.Do(func() { close(inside) })
		<-release
		return nil
	}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = g.Fill(context.Background(), id, holder, 0, nil)
	}()
	<-inside

	loser := &mapSource{d: d}
	if _, st, err := g.Fill(context.Background(), id, loser, 0, nil); err != nil || st != Filled {
		t.Fatalf("loser Fill = %v, %v; want Filled by local decode", st, err)
	}
	if loser.calls.Load() != 1 || g.stats.fillLostDecode.V.Load() != 1 {
		t.Errorf("loser decodes = %d, fillLostDecode = %d; want 1 and 1", loser.calls.Load(), g.stats.fillLostDecode.V.Load())
	}
	close(release)
	<-done
}
