package session

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/claudectl"
)

var _ claudectl.RateLimitWriter = (*Instance)(nil)

// WriteRateLimitRecovery types a rate-limit recovery input under the terminal
// write lease. claudectl cannot name *HeldLease (session imports it), so the
// controller's accessor delegates here.
//
// The acquire is bounded rather than a Try because the recovery loop does not
// retry a send error: a recovery typed while a steer holds the pane must wait
// a moment, not report a failed rate-limit recovery.
func (i *Instance) WriteRateLimitRecovery(data []byte) (int, error) {
	return writeUnderRateLimitLease(i.AcquireTerminalWriteLease, i.WriteToPTY, data)
}

func writeUnderRateLimitLease(
	acquire func(ctx context.Context, writer string, maxWait time.Duration) (*HeldLease, error),
	write func([]byte) (int, error),
	data []byte,
) (int, error) {
	lease, err := acquire(context.Background(), LeaseWriterRateLimit, RateLimitRecoveryLeaseWait)
	if err != nil {
		return 0, fmt.Errorf("rate-limit recovery input not sent: %w", err)
	}
	defer lease.Release()
	return write(data)
}
