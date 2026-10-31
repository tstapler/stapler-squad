package session

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const concurrencyTestGuard = 10 * time.Second // failure guard only; never reached when passing

// blockingPanePM blocks GetPanePID until release is closed, signalling entered.
type blockingPanePM struct {
	ProcessManager
	enter   *sync.Once
	calls   *atomic.Int64
	entered chan struct{}
	release chan struct{}
}

func (p blockingPanePM) GetPanePID() (int32, error) {
	if p.calls != nil {
		p.calls.Add(1)
	}
	p.enter.Do(func() { close(p.entered) })
	<-p.release
	return 0, assert.AnError
}

// Slow detection I/O for one session must not stop another session correlating.
// Fails on the old code, where one global mutex was held across GetPanePID.
func TestHistoryLinker_Correlate_should_NotBlockOtherSessions_When_OneDetectionIsSlow(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/slowio")
	f.write(t, claimOther, "{}", time.Second)

	slow := &Instance{Title: "slow", Path: t.TempDir(), Status: Running}
	pm := blockingPanePM{enter: new(sync.Once), entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	releasePane := func() { releaseOnce.Do(func() { close(pm.release) }) }
	t.Cleanup(releasePane) // unblock slow even if an assertion fails first
	slow.processManager = pm
	fast := &Instance{Title: "fast", Path: "/repo/slowio", Status: Running}
	fast.processManager = panePIDCountingPM{calls: new(atomic.Int64)}
	f.linker.SetInstances([]*Instance{slow, fast})

	slowDone := make(chan struct{})
	go func() { defer close(slowDone); f.linker.correlateSession(slow, false) }()
	<-pm.entered // slow is now inside GetPanePID

	fastDone := make(chan struct{})
	go func() { defer close(fastDone); f.linker.correlateSession(fast, false) }()
	select {
	case <-fastDone:
	case <-time.After(concurrencyTestGuard):
		t.Fatal("fast session blocked behind another session's slow detection")
	}
	assert.Equal(t, claimOther, fast.GetClaudeSession().ConversationUUID)

	releasePane()
	<-slowDone
}

// Two unlinked sessions in one directory, one unclaimed conversation: exactly one
// may take it however the goroutines interleave.
func TestHistoryLinker_Correlate_should_LinkConversationToExactlyOneSession_When_RacingSiblings(t *testing.T) {
	t.Parallel()
	for round := 0; round < 50; round++ {
		f := newClaimFixture(t, "/repo/race")
		f.write(t, claimOther, "{}", time.Second)
		const n = 8
		insts := make([]*Instance, n)
		for i := range insts {
			insts[i] = &Instance{Title: "racer-" + string(rune('a'+i)), Path: "/repo/race", Status: Running}
			insts[i].processManager = panePIDCountingPM{calls: new(atomic.Int64)}
		}
		f.linker.SetInstances(insts)

		start := make(chan struct{})
		var wg sync.WaitGroup
		for _, inst := range insts {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; f.linker.correlateSession(inst, false) }()
		}
		close(start)
		wg.Wait()

		linked := 0
		for _, inst := range insts {
			if inst.HasClaudeSession() {
				linked++
			}
		}
		require.Equal(t, 1, linked, "round %d: conversation linked to %d sessions", round, linked)
	}
}

// A sibling claiming the chosen conversation between detection and commit must
// make the commit re-select instead of double-linking.
func TestHistoryLinker_Commit_should_NotDoubleLink_When_SiblingClaimsBetweenDetectAndCommit(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/commit")
	path := f.write(t, claimOther, "{}", time.Second)
	a := &Instance{Title: "a", Path: "/repo/commit", Status: Running}
	a.processManager = panePIDCountingPM{calls: new(atomic.Int64)}
	b := &Instance{Title: "b", Path: "/repo/commit", Status: Running}
	f.linker.SetInstances([]*Instance{a, b})

	info, viaPath := f.linker.detectLink(a, false)
	require.NotNil(t, info)
	require.True(t, viaPath)
	b.SetHistoryInfo(claimOther, path) // sibling wins the race

	committed := f.linker.commitLink(a, info, viaPath, false)

	assert.Nil(t, committed)
	assert.False(t, a.HasClaudeSession(), "a must not take a conversation b already owns")
	assert.Equal(t, claimOther, b.GetClaudeSession().ConversationUUID)
}

// When the claimed pick is stale but another unclaimed file exists, commit
// falls through to that one.
func TestHistoryLinker_Commit_should_Reselect_When_ChosenConversationWasClaimed(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/reselect")
	f.write(t, claimOwnStart, "{}", 5*time.Second)
	newest := f.write(t, claimOther, "{}", time.Second)
	a := &Instance{Title: "a", Path: "/repo/reselect", Status: Running}
	a.processManager = panePIDCountingPM{calls: new(atomic.Int64)}
	b := &Instance{Title: "b", Path: "/repo/reselect", Status: Running}
	f.linker.SetInstances([]*Instance{a, b})

	info, viaPath := f.linker.detectLink(a, false)
	require.Equal(t, claimOther, info.ConversationUUID)
	b.SetHistoryInfo(claimOther, newest)

	committed := f.linker.commitLink(a, info, viaPath, false)

	require.NotNil(t, committed)
	assert.Equal(t, claimOwnStart, committed.ConversationUUID)
	assert.Equal(t, claimOwnStart, a.GetClaudeSession().ConversationUUID)
}

// A session linked by a concurrent correlation while this one was still
// detecting must keep the newer link rather than be overwritten by the stale result.
func TestHistoryLinker_Commit_should_KeepNewerLink_When_SessionLinkedMeanwhile(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/stale")
	f.write(t, claimOwnStart, "{}", 5*time.Second)
	a := &Instance{Title: "a", Path: "/repo/stale", Status: Running}
	f.linker.SetInstances([]*Instance{a})

	stale := &HistoryFileInfo{ConversationUUID: claimOwnStart}
	a.SetHistoryInfo(claimOther, "/p/"+claimOther+".jsonl") // newer link landed first

	assert.Nil(t, f.linker.commitLink(a, stale, false, false))
	assert.Equal(t, claimOther, a.GetClaudeSession().ConversationUUID)
}

// A session that ends up unlinked because a sibling owns the only candidate
// must count as a miss, so exponential backoff and parking still apply. The
// outcome is the same whether the sibling's claim lands before detection or
// between detection and commit (the gate is held to widen the second window).
func TestHistoryLinker_Correlate_should_RecordMiss_When_SiblingOwnsOnlyCandidate(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/miss")
	path := f.write(t, claimOther, "{}", time.Second)
	a := &Instance{Title: "a", Path: "/repo/miss", Status: Running}
	a.processManager = panePIDCountingPM{calls: new(atomic.Int64)}
	sibling := &Instance{Title: "b", Path: "/repo/miss", Status: Running}
	f.linker.SetInstances([]*Instance{a, sibling})

	f.linker.claimMu.Lock()
	done := make(chan struct{})
	go func() { defer close(done); f.linker.correlateSession(a, false) }()
	sibling.SetHistoryInfo(claimOther, path)
	f.linker.claimMu.Unlock()
	<-done

	assert.False(t, a.HasClaudeSession())
	f.linker.mu.RLock()
	bo := f.linker.backoffs["a"]
	f.linker.mu.RUnlock()
	require.NotNil(t, bo, "no miss recorded for the unlinked session")
	assert.Equal(t, 1, bo.consecutiveMisses)
}

// awaitJoined blocks until n callers are queued on the coalescer. Spinning with
// Gosched is a scheduler yield, not a timed sleep.
func awaitJoined(t *testing.T, c *scanCoalescer, n int) {
	t.Helper()
	deadline := time.Now().Add(concurrencyTestGuard)
	for {
		c.mu.Lock()
		got := len(c.joined)
		c.mu.Unlock()
		if got >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d callers joined", got, n)
		}
		runtime.Gosched()
	}
}

func TestScanCoalescer_should_RunExactlyTwice_When_ManyCallersJoinDuringOneRun(t *testing.T) {
	t.Parallel()
	var c scanCoalescer
	var runs atomic.Int64
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	run := func() {
		runs.Add(1)
		entered <- struct{}{}
		<-release
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); c.do(run) }()
	<-entered // first run in flight

	const waiters = 20
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.do(run) }()
	}
	awaitJoined(t, &c, waiters)

	release <- struct{}{} // finish run 1
	<-entered             // the single trailing run starts
	close(release)        // finish it (and unblock any further run)
	wg.Wait()

	assert.Equal(t, int64(2), runs.Load(), "%d joiners must share one trailing run", waiters)
}

func TestHistoryLinker_ScanAll_should_CoalesceConcurrentCalls(t *testing.T) {
	t.Parallel()
	f := newClaimFixture(t, "/repo/coalesce")
	inst := &Instance{Title: "slow", Path: t.TempDir(), Status: Running}
	scans := new(atomic.Int64)
	pm := blockingPanePM{enter: new(sync.Once), calls: scans, entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	releasePane := func() { releaseOnce.Do(func() { close(pm.release) }) }
	t.Cleanup(releasePane)
	inst.processManager = pm
	f.linker.SetInstances([]*Instance{inst})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); f.linker.ScanAll() }()
	<-pm.entered // first scan blocked inside GetPanePID

	const callers = 10
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.linker.ScanAll() }()
	}
	awaitJoined(t, &f.linker.scanFlight, callers)
	releasePane()
	wg.Wait()

	assert.Equal(t, int64(2), scans.Load(), "%d concurrent ScanAll calls must share one trailing scan", callers)
}

func TestScanCoalescer_should_ReleaseWaiters_When_RunPanics(t *testing.T) {
	t.Parallel()
	var c scanCoalescer
	entered := make(chan struct{})
	release := make(chan struct{})
	panicked := make(chan struct{})
	go func() {
		defer func() { _ = recover(); close(panicked) }()
		c.do(func() { close(entered); <-release; panic("boom") })
	}()
	<-entered
	waiterDone := make(chan struct{})
	go func() {
		defer close(waiterDone)
		c.do(func() { t.Error("waiter must be released by the panic, not run") })
	}()
	awaitJoined(t, &c, 1) // waiter is queued behind the doomed run
	close(release)
	<-panicked
	select {
	case <-waiterDone:
	case <-time.After(concurrencyTestGuard):
		t.Fatal("waiter stuck after run panicked")
	}
}
