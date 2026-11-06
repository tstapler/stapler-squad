package repostate

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

const longChain = 5000

func TestWalk_ShouldStopPromptly_WhenCtxCancelledMidWalk(t *testing.T) {
	d, head, base := chain(longChain, 0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const cancelAt = 100
	src := &mapSource{d: d}
	src.onParents = func(context.Context, plumbing.Hash) error {
		if src.calls.Load() == cancelAt {
			cancel()
		}
		return nil
	}
	rs := NewRepoState(Options{}, 1<<20)
	w := rs.NewWalk(ctx, src, 0, 100000)
	defer w.Close()

	_, err := w.FindMergeBase(head, base)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if errors.Is(err, ErrFallback) {
		t.Fatalf("err = %v wraps ErrFallback: a cancelled walk must not fall back to the legacy walk", err)
	}
	if got := src.calls.Load(); got > cancelAt+ctxCheckEvery {
		t.Errorf("walk decoded %d commits after cancel at %d, want <= %d more", got, cancelAt, ctxCheckEvery)
	}
}

func TestWalk_ShouldStopPromptly_WhenCtxCancelledOverFilledRows(t *testing.T) {
	d, head, base := chain(longChain, 0, 1)
	rs := NewRepoState(Options{}, 1<<20)
	warm := rs.NewWalk(context.Background(), &mapSource{d: d}, 0, 100000)
	if _, err := warm.FindMergeBase(head, base); err != nil {
		t.Fatal(err)
	}
	warm.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := rs.NewWalk(ctx, &mapSource{d: d}, 0, 100000)
	defer w.Close()
	_, err := w.FindMergeBase(head, base)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if w.hits > ctxCheckEvery {
		t.Errorf("walk read %d filled rows after cancel, want <= %d", w.hits, ctxCheckEvery)
	}
}

func TestComputeMiss_ShouldNotFailFollower_WhenLeaderIsCancelled(t *testing.T) {
	c := newByteCache(1<<20, 0, Options{})
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	started := make(chan struct{})
	leaderDone := make(chan error, 1)
	go func() {
		_, err := c.computeMiss(leaderCtx, "k", func() ([]byte, error) {
			close(started)
			<-leaderCtx.Done()
			return nil, leaderCtx.Err()
		})
		leaderDone <- err
	}()
	<-started

	followerRuns := 0
	followerDone := make(chan []byte, 1)
	go func() {
		v, err := c.computeMiss(context.Background(), "k", func() ([]byte, error) {
			followerRuns++
			return []byte("ok"), nil
		})
		if err != nil {
			t.Errorf("follower err = %v, want nil", err)
		}
		followerDone <- v
	}()
	waitJoined(t, c, 2) // the follower is in the leader's flight
	cancelLeader()

	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Errorf("leader err = %v, want context.Canceled", err)
	}
	if v := <-followerDone; string(v) != "ok" || followerRuns != 1 {
		t.Errorf("follower v=%q runs=%d, want ok 1", v, followerRuns)
	}
	if v, ok := c.Get("k"); !ok || string(v) != "ok" {
		t.Errorf("cache = %q, %v; want the follower's value, not the cancelled flight's", v, ok)
	}
}

func TestComputeMiss_ShouldReturnCtxErrAtOnce_WhenWaiterIsCancelled(t *testing.T) {
	c := newByteCache(1<<20, 0, Options{})
	release := make(chan struct{})
	started := make(chan struct{})
	leaderDone := make(chan []byte, 1)
	go func() {
		v, _ := c.computeMiss(context.Background(), "k", func() ([]byte, error) {
			close(started)
			<-release
			return []byte("v"), nil
		})
		leaderDone <- v
	}()
	<-started

	waiterCtx, cancelWaiter := context.WithCancel(context.Background())
	waiterDone := make(chan error, 1)
	go func() {
		_, err := c.computeMiss(waiterCtx, "k", func() ([]byte, error) { t.Error("waiter ran fn"); return nil, nil })
		waiterDone <- err
	}()
	waitJoined(t, c, 2)
	cancelWaiter()

	if err := <-waiterDone; !errors.Is(err, context.Canceled) {
		t.Errorf("waiter err = %v, want context.Canceled while the leader is still blocked", err)
	}
	close(release)
	if v := <-leaderDone; string(v) != "v" {
		t.Errorf("leader v = %q, want v: a cancelled waiter must not fail the leader", v)
	}
}

// The leader's fn uses resources the caller frees on return (a Walk's scratch),
// so the leader must outlive its own fn even after cancel.
func TestComputeMiss_ShouldNotReturnToLeader_UntilItsFnHasReturned(t *testing.T) {
	c := newByteCache(1<<20, 0, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	var returned atomic.Bool
	done := make(chan error, 1)
	go func() {
		_, err := c.computeMiss(ctx, "k", func() ([]byte, error) {
			close(started)
			<-ctx.Done()
			<-release // fn is still using the caller's resources
			return nil, ctx.Err()
		})
		returned.Store(true)
		done <- err
	}()
	<-started
	cancel()
	for i := 0; i < 10000 && !returned.Load(); i++ {
		runtime.Gosched()
	}
	if returned.Load() {
		t.Fatal("computeMiss returned to the leader while its fn was still running")
	}
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func waitJoined(t *testing.T, c *RepoCache[string, []byte], n int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for c.joined.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("joined = %d, want %d", c.joined.Load(), n)
		}
		runtime.Gosched()
	}
}
