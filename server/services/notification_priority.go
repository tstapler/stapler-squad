package services

import sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"

// derivePriority maps the Eisenhower-style (urgent, important) axes now used at every
// notification call site onto the stored NotificationPriority enum, so nothing
// downstream of "priority" (ent schema, proto, the /notifications page,
// get_notification_history) needs to change. Push delivery then gates on
// priority == URGENT (server/push/subscriber.go's shouldNotify) — the only quadrant
// where both axes are true.
func derivePriority(urgent, important bool) int32 {
	switch {
	case urgent && important:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT)
	case important:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)
	case urgent:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM)
	default:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW)
	}
}
