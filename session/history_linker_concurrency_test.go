package session

import (
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
	entered chan struct{}
	release chan struct{}
}

func (p blockingPanePM) GetPanePID() (int32, error) {
	close(p.entered)
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
	pm := blockingPanePM{entered: make(chan struct{}), release: make(chan struct{})}
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

	close(pm.release)
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

	f.linker.commitLink(a, info, viaPath, false)

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

	f.linker.commitLink(a, info, viaPath, false)

	assert.Equal(t, claimOwnStart, a.GetClaudeSession().ConversationUUID)
}

func TestScanCoalescer_should_RunOnceMoreThanInFlight_When_ManyCallersArriveDuringRun(t *testing.T) {
	t.Parallel()
	var c scanCoalescer
	var runs atomic.Int64
	entered := make(chan struct{}, 8)
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
	var joined sync.WaitGroup
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		joined.Add(1)
		go func() { joined.Done(); defer wg.Done(); c.do(run) }()
	}
	joined.Wait()
	// Waiters have started but may not have enqueued yet; release runs until all finish.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for {
		select {
		case release <- struct{}{}:
		case <-done:
			// Each waiter either joined the single trailing run or, if it arrived
			// after that run finished, started its own; never one run per waiter
			// while runs are in flight is the property under test.
			assert.GreaterOrEqual(t, runs.Load(), int64(2))
			assert.Less(t, runs.Load(), int64(waiters+1))
			return
		case <-time.After(concurrencyTestGuard):
			t.Fatal("coalescer did not finish")
		}
	}
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
	go func() { defer close(waiterDone); c.do(func() {}) }()
	// Let the waiter enqueue: it can only return once the panic releases it.
	close(release)
	<-panicked
	select {
	case <-waiterDone:
	case <-time.After(concurrencyTestGuard):
		t.Fatal("waiter stuck after run panicked")
	}
}
