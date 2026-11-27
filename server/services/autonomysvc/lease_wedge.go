package autonomysvc

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
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/session/instance"
)

// LeaseWedgeScanInterval is how often wedged write leases are scanned.
const LeaseWedgeScanInterval = 10 * time.Second

const (
	leaseWedgeTitle = "A write to this session is stuck"
	leaseWedgeBody  = "Typing into this session's terminal is blocked, so the driver's prompt and answer keys and any " +
		"steer, nudge or reply wait until it clears. Delete the session or restart stapler-squad to clear it; " +
		"Pause frees the pane but a resumed session stays blocked until the service restarts."
)

// LeaseWedgePorts are the hub values one scan reads.
type LeaseWedgePorts struct {
	Bus *events.EventBus
	// LinkedItemID returns the backlog item linked to the session, or "".
	LinkedItemID func(*instance.Instance) string
	// Gate counts raised warnings; nil counts nothing.
	Gate *deliverygate.Gate
}

// ScanLeaseWedges raises one tray-level WARNING per wedge episode (one
// acquisition that outlived session.LeaseWedgeWarnAfter) among the instances
// include accepts (nil accepts all) and returns how many it raised. The event is
// type WARNING at medium priority, so the push subscriber's shouldNotify does
// not push it, and it is stamped delivery_class=failure so the delivery gate
// lets it through for a hidden session. It never releases a lease.
func ScanLeaseWedges(now time.Time, include func(*instance.Instance) bool, p LeaseWedgePorts) int {
	wedges := instance.ScanWedgedLeases(now, instance.LeaseWedgeWarnAfter, include)
	for _, w := range wedges {
		snap := w.Instance.Snapshot()
		meta := events.SessionScopedMetadata(
			map[string]string{pkgevents.MetadataKeyDeliveryClass: pkgevents.DeliveryClassFailure},
			p.LinkedItemID(w.Instance))
		p.Bus.Publish(events.NewNotificationEvent(
			snap.UUID, snap.Title, fmt.Sprintf("lease-wedge-%s-%d", snap.UUID, w.AcquisitionID),
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
			notify.DerivePriority(true, false), // medium: not urgent, so never pushed
			leaseWedgeTitle, leaseWedgeBody, meta,
		))
		if p.Gate != nil {
			p.Gate.Metrics().Add(deliverygate.CounterLeaseWedgeNotified)
		}
		log.Warn("terminal_write_lease_wedge_notified", "session", snap.Title, "writer", w.Writer, "held", w.Held.Round(time.Second))
	}
	return len(wedges)
}

// StartLeaseWedgeWatcher calls scan on every tick until ctx ends; the goroutine
// is registered with wg so shutdown joins it.
func StartLeaseWedgeWatcher(ctx context.Context, tick <-chan time.Time, wg *sync.WaitGroup, scan func(time.Time)) {
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
