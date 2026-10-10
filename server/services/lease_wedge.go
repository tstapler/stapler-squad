package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// LeaseWedgeScanInterval is how often wedged write leases are scanned.
const LeaseWedgeScanInterval = 10 * time.Second

const (
	leaseWedgeTitle = "A write to this session is stuck"
	leaseWedgeBody  = "Typing into this session's terminal is blocked, so the driver's prompt and answer keys and any " +
		"steer, nudge or reply wait until it clears. Delete the session or restart stapler-squad to clear it; " +
		"Pause frees the pane but a resumed session stays blocked until the service restarts."
)

// ScanLeaseWedges raises one tray-level WARNING per wedge episode (one
// acquisition that outlived session.LeaseWedgeWarnAfter) and returns how many
// it raised. The event is type WARNING at medium priority, so the push
// subscriber's shouldNotify does not push it, and it is stamped
// delivery_class=failure so the delivery gate lets it through for a hidden
// session. It never releases a lease.
func (s *SessionService) ScanLeaseWedges(now time.Time) int {
	return s.scanLeaseWedges(now, nil)
}

// scanLeaseWedges is ScanLeaseWedges restricted to the instances include
// accepts (nil accepts all); tests use it so parallel tests' leases are not
// swept up.
func (s *SessionService) scanLeaseWedges(now time.Time, include func(*session.Instance) bool) int {
	wedges := session.ScanWedgedLeases(now, session.LeaseWedgeWarnAfter, include)
	for _, w := range wedges {
		snap := w.Instance.Snapshot()
		meta := events.SessionScopedMetadata(
			map[string]string{pkgevents.MetadataKeyDeliveryClass: pkgevents.DeliveryClassFailure},
			s.rateLimitLinkedItemID(w.Instance))
		s.eventBus.Publish(events.NewNotificationEvent(
			snap.UUID, snap.Title, fmt.Sprintf("lease-wedge-%s-%d", snap.UUID, w.AcquisitionID),
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
			derivePriority(true, false), // medium: not urgent, so never pushed
			leaseWedgeTitle, leaseWedgeBody, meta,
		))
		if s.deliveryGate != nil {
			s.deliveryGate.Metrics().Add(deliverygate.CounterLeaseWedgeNotified)
		}
		log.Warn("terminal_write_lease_wedge_notified", "session", snap.Title, "writer", w.Writer, "held", w.Held.Round(time.Second))
	}
	return len(wedges)
}

// StartLeaseWedgeWatcher scans on every tick until ctx ends; the goroutine is
// registered with wg so shutdown joins it.
func (s *SessionService) StartLeaseWedgeWatcher(ctx context.Context, tick <-chan time.Time, wg *sync.WaitGroup) {
	s.startLeaseWedgeWatcher(ctx, tick, wg, func(now time.Time) { s.ScanLeaseWedges(now) })
}

func (s *SessionService) startLeaseWedgeWatcher(ctx context.Context, tick <-chan time.Time, wg *sync.WaitGroup, scan func(time.Time)) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick:
				scan(now)
			}
		}
	}()
}
