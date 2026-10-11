package deliverygate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestFlagCache_ShouldDoNoIO_WhenReadingTenThousandTimesAndPickUpEditWithinTick(t *testing.T) {
	t.Parallel()
	lg, _ := newRecLogger()
	f := &staticFlags{}
	c := NewFlagCache(f.load, lg)
	c.Reload()
	base := f.readCount()
	for i := 0; i < 10_000; i++ {
		_ = c.Enabled()
	}
	if got := f.readCount(); got != base {
		t.Fatalf("Enabled() performed I/O: %d loader reads after construction", got-base)
	}

	// An external edit is picked up by the next tick (driven by a fake channel).
	f.set(true)
	if c.Enabled() {
		t.Fatal("flag changed before any reload")
	}
	tick := make(chan time.Time)
	c.Start(context.Background(), tick)
	tick <- time.Time{}
	tick <- time.Time{} // second send returns only after the first Reload finished
	c.Stop()
	if !c.Enabled() {
		t.Fatal("ticker reload did not pick up the edit")
	}
}

func TestFlagCache_ShouldReflectOverrideImmediately_WhenUpdateFeatureFlagCalled(t *testing.T) {
	t.Parallel()
	lg, _ := newRecLogger()
	f := &staticFlags{}
	c := NewFlagCache(f.load, lg)
	f.set(false) // the registry default is on, so the observable change is a flip to off
	c.OnFlagChanged("some_other_flag")
	if !c.Enabled() {
		t.Fatal("unrelated flag change must not reload")
	}
	c.OnFlagChanged("hidden_session_gate")
	if c.Enabled() {
		t.Fatal("OnFlagChanged did not reload immediately")
	}
}

func TestEnabledFor_ShouldApplyKindOverrideThenGlobalThenDefault(t *testing.T) {
	t.Parallel()
	lg, _ := newRecLogger()
	c := NewFlagCache(func() (FlagSettings, error) {
		return FlagSettings{Global: false, KindOverrides: map[HiddenKind]bool{KindReview: true}}, nil
	}, lg)
	if !c.EnabledFor(KindReview) || !c.EnabledFor(KindOther) {
		t.Fatal("before reload everything is the default (on)")
	}
	c.Reload()
	if !c.EnabledFor(KindReview) {
		t.Error("review override on must win over global off")
	}
	if c.EnabledFor(KindOther) {
		t.Error("kind without override follows the loaded global (off)")
	}
}

// T-FL-17: a ticker reload parked between its file read and its swap must not
// publish a value older than a reload that ran meanwhile.
func TestFlagCache_ShouldNeverLetAStaleTickerReloadOverwriteUpdateFeatureFlag_WhenReloadsRace(t *testing.T) {
	t.Parallel()
	lg, _ := newRecLogger()
	f := &staticFlags{}
	c := NewFlagCache(f.load, lg)

	parked := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	c.betweenLoadAndSwap = func() {
		once.Do(func() {
			close(parked)
			<-release
		})
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // the ticker's reload: reads (off), parks before swapping
		defer wg.Done()
		c.Reload()
	}()
	<-parked

	updateDone := make(chan struct{})
	go func() { // operator flips on and notifies; it must serialize behind reloadMu
		defer close(updateDone)
		f.set(true)
		c.OnFlagChanged("hidden_session_gate")
	}()
	select {
	case <-updateDone:
		t.Fatal("OnFlagChanged ran while a reload held reloadMu between load and swap")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	<-updateDone
	if !c.Enabled() {
		t.Fatal("stale ticker reload overwrote the operator's change")
	}
}

func TestFlagCache_ShouldKeepLastGoodValue_WhenReloadFails(t *testing.T) {
	t.Parallel()
	lg, recs := newRecLogger()
	f := &staticFlags{}
	f.set(true)
	c := NewFlagCache(f.load, lg)
	c.Reload()
	f.mu.Lock()
	f.err = errors.New("read failed")
	f.s = FlagSettings{}
	f.mu.Unlock()
	c.Reload()
	if !c.Enabled() {
		t.Fatal("a read error flipped the gate")
	}
	if countMsg(recs(), "hidden_session_gate flag reload failed; keeping last value") != 1 {
		t.Fatal("reload failure not logged")
	}
}

// T-FT-02: the ticker goroutine is joined by Stop and leaks nothing.
func TestBackgroundWorkers_ShouldJoinOnStopAndLeakNoGoroutines_WhenTheFlagCacheTickerRunsOnAFakeClockAndIsStopped(t *testing.T) {
	defer goleak.VerifyNone(t)
	lg, _ := newRecLogger()
	c := NewFlagCache((&staticFlags{}).load, lg)
	c.Start(context.Background(), make(chan time.Time))
	c.Stop()
	c.Stop() // idempotent
}
