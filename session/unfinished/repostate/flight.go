package repostate

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"golang.org/x/sync/singleflight"
)

// Flight states of one FlightCtx call's closure.
const (
	flightPending   int32 = iota // closure not started: this call is a follower, or the flight goroutine is yet to run
	flightRunning                // closure started: this call is the leader
	flightAbandoned              // caller gave up before the closure started; it must not run fn
)

// FlightCtx is a singleflight call whose callers each honour their own ctx.
//
//   - A follower stops waiting when its ctx ends.
//   - The leader runs fn under its own ctx and, once fn has started, waits for it:
//     fn uses the leader's resources (a Walk, scratch) that the caller releases on
//     return, so it must not outlive the call. A ctx-aware fn returns promptly.
//   - fn runs under the leader's ctx, so a cancelled leader fails with an error that
//     is not the followers'. A follower whose ctx is live retries (and may lead)
//     instead of inheriting it. fn must not cache anything on error.
//
// led reports whether this caller's fn ran to produce the result. joined, if
// non-nil, counts callers once registered in a flight.
func FlightCtx[T any](ctx context.Context, sf *singleflight.Group, key string, joined *atomic.Int64, fn func() (T, error)) (v T, led bool, err error) {
	var zero T
	for {
		if cerr := ctx.Err(); cerr != nil {
			return zero, false, cerr
		}
		var state atomic.Int32
		ch := sf.DoChan(key, func() (out any, doErr error) {
			if !state.CompareAndSwap(flightPending, flightRunning) {
				return nil, ctx.Err() // abandoned before it started: fn must not touch the caller's resources
			}
			defer func() {
				if r := recover(); r != nil {
					doErr = fmt.Errorf("flight panic: %v", r)
				}
			}()
			return fn()
		})
		if joined != nil {
			joined.Add(1)
		}
		var res singleflight.Result
		select {
		case res = <-ch:
		case <-ctx.Done():
			if state.CompareAndSwap(flightPending, flightAbandoned) {
				return zero, false, ctx.Err()
			}
			res = <-ch // fn is running for this caller: wait for it to see the cancel
		}
		ledHere := state.Load() == flightRunning
		if res.Err != nil {
			if !ledHere && ctx.Err() == nil && isContextErr(res.Err) {
				continue
			}
			return zero, ledHere, res.Err
		}
		return res.Val.(T), ledHere, nil
	}
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
