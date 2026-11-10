package scenario

import "time"

// SendNotification is a live push, not a stored row (Q5), so history is a
// route fixture.
func notificationsBundle(ref time.Time) Bundle {
	n := func(id, kind, title, detail string, age time.Duration) Row {
		return Row{ID: id, Kind: kind, Title: title, Detail: detail, At: ago(ref, age)}
	}
	return Bundle{Routes: []RouteFixture{{
		ID: "notification-history", Route: "/api/session.v1.SessionService/GetNotificationHistory", Seeded: SeededRoute,
		Rows: []Row{
			n("n-1", "approval", "Approval needed", "checkout-retry-logic wants to run git push.", 2*mn),
			n("n-2", "completed", "Session finished", "search-index-rebuild finished with 3 files changed.", 11*mn),
			n("n-3", "idle", "Waiting for input", "autoscaling-tuning has been idle for 15 minutes.", 24*mn),
		},
	}}}
}
