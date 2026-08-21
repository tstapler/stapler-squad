package push

// Notification priority levels (mirror sessionv1.NotificationPriority values).
const (
	priorityLow    = int32(1)
	priorityMedium = int32(2)
	priorityHigh   = int32(3)
	priorityUrgent = int32(4)
)

// Notification type values (mirror sessionv1.NotificationType values).
const (
	typeUnspecified = int32(0)
	typeApproval    = int32(1) // NOTIFICATION_TYPE_APPROVAL_NEEDED
)
