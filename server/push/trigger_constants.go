package push

import "time"

// Notification priority levels (mirror sessionv1.NotificationPriority values).
const (
	priorityLow    = int32(1)
	priorityMedium = int32(2)
	priorityHigh   = int32(3)
	priorityUrgent = int32(4)
)

// urgentTTL bounds how long a URGENT-priority notification stays push-eligible after it
// fires — urgency is time-bound even when importance isn't, so a notification evaluated
// more than urgentTTL after it was created no longer pushes on priority alone (see
// shouldNotify). Named so it's easy to retune later, not a magic literal inline. Mirrors
// server/notifications/subscriber.go's UrgentTTL, which independently demotes the stored
// record once it ages past the same threshold; duplicated here (not a shared import)
// following this file's existing pattern of mirroring small proto-adjacent constants
// locally rather than pulling in another package for them.
const urgentTTL = 1 * time.Hour

// Notification type values (mirror sessionv1.NotificationType values).
const (
	typeUnspecified = int32(0)
	typeApproval    = int32(1) // NOTIFICATION_TYPE_APPROVAL_NEEDED
)
