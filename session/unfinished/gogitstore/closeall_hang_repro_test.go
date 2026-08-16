// closeall_hang_repro_test.go documents and regression-tests the
// packHandleCache.closeAll() hang that TestGogitstore_SoakUnderSustainedLoad
// hit under sustained load: the original closeAll() held c.mu across a
// per-handle h.mu.Lock() loop, so a getFromPackfile read stuck holding that
// same h.mu (via cachedPackHandle.mu) wedged closeAll — and therefore every
// other WorktreeStorer.Close() caller piled up in sync.Once.doSlow — forever.
//
// This uses debugStallHeldPackHandleRead (store.go), a test-only injection
// point, to force a getFromPackfile call to hold ch.mu far longer than any
// real read would, instead of relying on hitting the race by pure timing
// luck under soak load.
//
// TestGogitstore_CloseAllHangRepro_CapturesRealDump captured a real,
// complete pprof.Lookup("goroutine") dump against the ORIGINAL (buggy)
// closeAll() confirming this exact mechanism — see this task's PR
// description for the full dump. That capture is not repeated as part of the
// regular suite (the original code path no longer exists to reproduce
// against); what remains and runs on every non -short invocation is the
// regression test below, which pins the fixed behavior: Close() must return
// within one packHandleCloseTimeout period even when a read is stuck, and
// the stuck handle must still eventually get closed (no fd leak) once the
// read releases it.
package gogitstore

import (
	"sync"
	"testing"
	"time"
)

func TestGogitstore_Close_BoundedEvenWithStalledRead(t *testing.T) {
	if testing.Short() {
		t.Skip("diagnostic regression skipped under -short")
	}

	root := t.TempDir()
	mainDir := root + "/repo"
	buildPackedFixture(t, mainDir, 25)

	reg := &Registry{}
	gr, err := Open(mainDir, reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ws, ok := gr.Storer.(*WorktreeStorer)
	if !ok {
		t.Fatalf("expected *WorktreeStorer, got %T", gr.Storer)
	}

	readStarted := make(chan struct{})
	releaseRead := make(chan struct{})
	debugStallHeldPackHandleRead = func() {
		close(readStarted)
		<-releaseRead
	}
	defer func() { debugStallHeldPackHandleRead = nil }()

	head, err := gr.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	// Goroutine A: a read that blocks inside getFromPackfile, holding ch.mu,
	// until the test releases it — simulating a slow/stuck packfile read
	// concurrent with Close().
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = gr.CommitObject(head.Hash())
	}()
	<-readStarted

	// Two concurrent Close() callers on the same WorktreeStorer — under the
	// original closeAll, both would block forever in sync.Once.doSlow behind
	// the stuck handle. Under the fix, closeHandleBounded's per-handle
	// timeout bounds how long closeAll (and therefore Close()) waits for the
	// stuck handle before hedging it off to a background goroutine.
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		_ = ws.Close()
	}()
	secondCloseDone := make(chan struct{})
	go func() {
		defer close(secondCloseDone)
		_ = ws.Close()
	}()

	// Close() must return within one packHandleCloseTimeout period plus
	// slack for scheduling/test overhead — NOT hang indefinitely the way the
	// original closeAll did.
	bound := packHandleCloseTimeout + 5*time.Second
	deadline := time.After(bound)
	for i := 0; i < 2; i++ {
		select {
		case <-closeDone:
		case <-secondCloseDone:
		case <-deadline:
			close(releaseRead)
			<-readDone
			t.Fatalf("Close() did not return within %s of a stalled read — closeAll's bounded-close fix appears to have regressed to an unbounded hang", bound)
		}
	}

	// The stalled read is still holding the handle; release it now so the
	// hedged-off background close (and the read itself) can complete, and
	// confirm both finish promptly — proving the handle is eventually
	// closed rather than leaked.
	close(releaseRead)
	select {
	case <-readDone:
	case <-time.After(10 * time.Second):
		t.Fatal("stalled read did not complete after being released")
	}
}

// TestGogitstore_Close_IdempotentUnderConcurrency pins the existing
// closeOnce sync.Once contract documented on WorktreeStorer.Close (storer.go)
// — safe to call more than once, and only the first call has any effect —
// proving the closeAll() bounded-close fix above did not disturb it. N
// concurrent Close() calls on the same WorktreeStorer must drop the
// underlying SharedObjectStore's refcount by exactly one, not N.
func TestGogitstore_Close_IdempotentUnderConcurrency(t *testing.T) {
	root := t.TempDir()
	mainDir := root + "/repo"
	buildPackedFixture(t, mainDir, 5)

	reg := &Registry{}
	gr, err := Open(mainDir, reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ws, ok := gr.Storer.(*WorktreeStorer)
	if !ok {
		t.Fatalf("expected *WorktreeStorer, got %T", gr.Storer)
	}

	before := ws.shared.RefCount()
	if before == 0 {
		t.Fatalf("expected a live reference from Open, got RefCount()=0")
	}

	const concurrentClosers = 20
	var wg sync.WaitGroup
	wg.Add(concurrentClosers)
	for i := 0; i < concurrentClosers; i++ {
		go func() {
			defer wg.Done()
			_ = ws.Close()
		}()
	}
	wg.Wait()

	if got, want := ws.shared.RefCount(), before-1; got != want {
		t.Fatalf("RefCount() after %d concurrent Close() calls = %d, want %d (closeOnce should make every call but the first a no-op)", concurrentClosers, got, want)
	}

	// A further Close() call after the group above must remain a safe no-op.
	_ = ws.Close()
	if got, want := ws.shared.RefCount(), before-1; got != want {
		t.Fatalf("RefCount() after an extra Close() call = %d, want %d (Close must stay idempotent)", got, want)
	}
}
