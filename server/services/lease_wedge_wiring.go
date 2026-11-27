package services

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/services/autonomysvc"
)

// ScanLeaseWedges raises the once-per-episode warning for every wedged terminal
// write lease; see autonomysvc.ScanLeaseWedges.
func (s *SessionService) ScanLeaseWedges(now time.Time) int {
	return autonomysvc.ScanLeaseWedges(now, nil, s.leaseWedgePorts())
}

// leaseWedgePorts is rebuilt per scan, so a gate wired after construction is seen.
func (s *SessionService) leaseWedgePorts() autonomysvc.LeaseWedgePorts {
	return autonomysvc.LeaseWedgePorts{Bus: s.eventBus, LinkedItemID: s.rateLimitLinkedItemID, Gate: s.deliveryGate}
}

// StartLeaseWedgeWatcher scans on every tick until ctx ends; the goroutine is
// registered with wg so shutdown joins it.
func (s *SessionService) StartLeaseWedgeWatcher(ctx context.Context, tick <-chan time.Time, wg *sync.WaitGroup) {
	autonomysvc.StartLeaseWedgeWatcher(ctx, tick, wg, func(now time.Time) { s.ScanLeaseWedges(now) })
}
