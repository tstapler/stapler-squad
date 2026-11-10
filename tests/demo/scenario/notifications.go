package scenario

import "time"

// SendNotification stores the row and pushes it, so the history page is seeded over RPC.
func notificationsBundle(_ time.Time) Bundle {
	return Bundle{Notifications: []Notification{
		{ID: "n-1", Type: "NOTIFICATION_TYPE_APPROVAL_NEEDED", Title: "Approval needed", Message: "checkout-retry-logic wants to run git push."},
		{ID: "n-2", Type: "NOTIFICATION_TYPE_TASK_COMPLETE", Title: "Session finished", Message: "search-index-rebuild finished with 3 files changed."},
		{ID: "n-3", Type: "NOTIFICATION_TYPE_INPUT_REQUIRED", Title: "Waiting for input", Message: "autoscaling-tuning has been idle for 15 minutes."},
	}}
}
