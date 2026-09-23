package log

import "sync/atomic"

// DropLogInterval is how often a rate-limited "dropped" warning is logged:
// only every Nth drop, to avoid flooding logs under sustained backpressure.
const DropLogInterval = 100

// DropCounter rate-limits repeated "dropped due to backpressure" warnings:
// only every Nth drop is logged, with the running total attached. Shared by
// session/tokens and session/artifacts, whose worker-pool queues both need
// this exact pattern for queue-full drops.
type DropCounter struct {
	count atomic.Uint64
}

// Hit records one drop and reports the new running total plus whether this
// particular drop should be logged (every DropLogInterval-th one).
func (d *DropCounter) Hit() (total uint64, shouldLog bool) {
	n := d.count.Add(1)
	return n, n%DropLogInterval == 1
}
