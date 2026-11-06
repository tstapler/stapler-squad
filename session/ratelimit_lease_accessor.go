package session

import (
	"context"
	"fmt"
	"time"
)

// leaseAcquirer is satisfied by *Instance; a fake InstanceContext without it is
// written to directly (nothing to serialize against).
type leaseAcquirer interface {
	AcquireTerminalWriteLease(ctx context.Context, writer string, maxWait time.Duration) (*HeldLease, error)
}

// leasedSessionAccessor is the ratelimit.SessionAccessor the controller hands
// to ratelimit.NewManager. session imports ratelimit, so *HeldLease cannot
// appear there: this wrapper is the chain's acquirer and the manager's own
// WriteToPTY call is lease-held by construction.
//
// The acquire is bounded rather than a Try because the recovery loop does not
// retry a send error: a recovery typed while a steer holds the pane must wait
// a moment, not report a failed rate-limit recovery.
type leasedSessionAccessor struct {
	InstanceContext
}

func (a leasedSessionAccessor) WriteToPTY(data []byte) (int, error) {
	acq, ok := a.InstanceContext.(leaseAcquirer)
	if !ok {
		return a.InstanceContext.WriteToPTY(data)
	}
	lease, err := acq.AcquireTerminalWriteLease(context.Background(), LeaseWriterRateLimit, RateLimitRecoveryLeaseWait)
	if err != nil {
		return 0, fmt.Errorf("rate-limit recovery input not sent: %w", err)
	}
	defer lease.Release()
	return a.InstanceContext.WriteToPTY(data)
}
