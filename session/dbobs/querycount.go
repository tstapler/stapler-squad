package dbobs

import (
	"context"
	"sync/atomic"
)

// QueryCounter counts the statements issued under one context, so a request
// (or a test) can report or assert how many round trips it cost.
type QueryCounter struct{ n atomic.Int64 }

// Count returns the statements observed so far.
func (c *QueryCounter) Count() int64 { return c.n.Load() }

type queryCounterKey struct{}

// WithQueryCounter returns a context whose instrumented statements, including
// BEGIN/COMMIT, are counted by the returned counter.
func WithQueryCounter(ctx context.Context) (context.Context, *QueryCounter) {
	c := &QueryCounter{}
	return context.WithValue(ctx, queryCounterKey{}, c), c
}

func countQuery(ctx context.Context) {
	if c, ok := ctx.Value(queryCounterKey{}).(*QueryCounter); ok {
		c.n.Add(1)
	}
}
