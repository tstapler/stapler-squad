package repostate

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"

	"golang.org/x/sync/singleflight"
)

// A follower whose own ctx ended must report its own ctx error, not the
// leader's, even when the leader's result arrives in the same instant.
func TestFlightCtx_ShouldReturnOwnCtxError_WhenFollowerAndLeaderBothEnded(t *testing.T) {
	for i := 0; i < 50; i++ {
		var sf singleflight.Group
		var joined atomic.Int64
		started, release := make(chan struct{}), make(chan struct{})
		fctx, fcancel := context.WithCancel(context.Background())
		defer fcancel()

		leaderDone := make(chan struct{})
		go func() {
			defer close(leaderDone)
			_, _, _ = FlightCtx(context.Background(), &sf, "k", &joined, func() (int, error) {
				close(started)
				<-release
				fcancel()
				return 0, context.DeadlineExceeded
			})
		}()
		<-started
		var ferr error
		followerDone := make(chan struct{})
		go func() {
			defer close(followerDone)
			_, _, ferr = FlightCtx(fctx, &sf, "k", &joined, func() (int, error) { return 1, nil })
		}()
		for joined.Load() < 2 {
			runtime.Gosched()
		}
		close(release)
		<-followerDone
		<-leaderDone
		if !errors.Is(ferr, context.Canceled) {
			t.Fatalf("iteration %d: follower err = %v, want its own context.Canceled", i, ferr)
		}
	}
}
