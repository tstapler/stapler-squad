package unfinished

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"
)

func waitJoinedCount(t *testing.T, joined *atomic.Int64, n int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for joined.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("joined = %d, want %d", joined.Load(), n)
		}
		runtime.Gosched()
	}
}

func TestSfDoCtx_ShouldNotFailFollower_WhenLeaderIsCancelled(t *testing.T) {
	var (
		sf      singleflight.Group //nolint:exhaustruct
		joined  atomic.Int64
		started = make(chan struct{})
	)
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	leaderErr := make(chan error, 1)
	go func() {
		_, err := sfDoCtx(leaderCtx, &sf, "k", &joined, func() (int, error) {
			close(started)
			<-leaderCtx.Done()
			return 0, leaderCtx.Err()
		})
		leaderErr <- err
	}()
	<-started

	var followerRuns atomic.Int64
	type result struct {
		v   int
		err error
	}
	followerRes := make(chan result, 1)
	go func() {
		v, err := sfDoCtx(context.Background(), &sf, "k", &joined, func() (int, error) {
			followerRuns.Add(1)
			return 42, nil
		})
		followerRes <- result{v, err}
	}()
	waitJoinedCount(t, &joined, 2) // the follower is registered in the leader's flight
	cancelLeader()

	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Errorf("leader err = %v, want context.Canceled", err)
	}
	if r := <-followerRes; r.err != nil || r.v != 42 || followerRuns.Load() != 1 {
		t.Errorf("follower = %+v runs=%d, want 42 <nil> 1", r, followerRuns.Load())
	}
}

func TestSfDoCtx_ShouldReturnAtOnce_WhenWaiterIsCancelledAndLeaderStillRuns(t *testing.T) {
	var (
		sf      singleflight.Group //nolint:exhaustruct
		joined  atomic.Int64
		started = make(chan struct{})
		release = make(chan struct{})
	)
	leaderVal := make(chan int, 1)
	go func() {
		v, _ := sfDoCtx(context.Background(), &sf, "k", &joined, func() (int, error) {
			close(started)
			<-release
			return 7, nil
		})
		leaderVal <- v
	}()
	<-started

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterErr := make(chan error, 1)
	go func() {
		_, err := sfDoCtx(waiterCtx, &sf, "k", &joined, func() (int, error) { t.Error("waiter ran fn"); return 0, nil })
		waiterErr <- err
	}()
	waitJoinedCount(t, &joined, 2)
	cancelWaiter()

	if err := <-waiterErr; !errors.Is(err, context.Canceled) {
		t.Errorf("waiter err = %v, want context.Canceled while the leader is still blocked", err)
	}
	close(release)
	if v := <-leaderVal; v != 7 {
		t.Errorf("leader v = %d, want 7: a cancelled waiter must not fail the leader", v)
	}
}

func TestAheadBehind_ShouldReturnCtxErrAndCacheNothing_WhenCtxCancelled(t *testing.T) {
	_, dir, _ := divergedClocked(t, 50, 3, 2)
	r := NewCountingReader(Flags{SharedCommitState: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := r.AheadBehind(ctx, dir, "base"); !errors.Is(err, context.Canceled) {
		t.Fatalf("AheadBehind err = %v, want context.Canceled", err)
	}
	if _, err := r.CommitMessages(ctx, dir, "base", 5); !errors.Is(err, context.Canceled) {
		t.Fatalf("CommitMessages err = %v, want context.Canceled", err)
	}
	if _, ok := r.aheadBehindCache.Load(aheadBehindKey{worktree: dir, base: "base"}); ok {
		t.Error("a cancelled AheadBehind left a cache entry")
	}
	ahead, behind, err := r.AheadBehind(context.Background(), dir, "base")
	if err != nil || ahead != 3 || behind != 2 {
		t.Errorf("after the cancel = (%d, %d, %v), want (3, 2, nil)", ahead, behind, err)
	}
}

// cancelOnAheadBehind cancels the scan's ctx from inside AheadBehind, the way a
// shutdown lands mid-walk.
type cancelOnAheadBehind struct {
	fakeVCSReaderForCacheTest
	cancel context.CancelFunc
	calls  atomic.Int64
}

func (c *cancelOnAheadBehind) HasUncommitted(string) (bool, error) { return true, nil }

func (c *cancelOnAheadBehind) AheadBehind(ctx context.Context, _, _ string) (int, int, error) {
	c.calls.Add(1)
	if c.cancel != nil {
		c.cancel()
		return 0, 0, ctx.Err()
	}
	return 1, 0, nil
}

func TestScanRepo_ShouldPublishAndCacheNothing_WhenCtxCancelledMidScan(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelOnAheadBehind{cancel: cancel}
	reader.worktrees = []WorktreeInfo{{Path: "/tmp/wt-ctx", Branch: "feat"}}
	s := NewScannerWithReader(nil, nil, reader)

	if got := s.scanRepo(ctx, "/tmp/repo-ctx", false); got != nil {
		t.Fatalf("cancelled scan published %d results, want none", len(got))
	}

	reader.cancel = nil
	got := s.scanRepo(context.Background(), "/tmp/repo-ctx", false)
	if len(got) != 1 || got[0].AheadCount != 1 {
		t.Fatalf("rescan = %+v, want one result with AheadCount 1 (the cancelled zero counts must not be served from cache)", got)
	}
	if n := reader.calls.Load(); n != 2 {
		t.Errorf("AheadBehind calls = %d, want 2", n)
	}
}
