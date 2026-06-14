package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// NotificationClearHistory describes the clear-notification-history RPC.
var NotificationClearHistory = featureregistry.Feature{
	ID:          "notification-clear-history",
	Title:       "Clear Notification History",
	Description: "Clears the notification history for the current user.",
	RPCIDs:      []string{"notification:clear-history"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// NotificationGetHistory describes the get-notification-history RPC.
var NotificationGetHistory = featureregistry.Feature{
	ID:          "notification-get-history",
	Title:       "Get Notification History",
	Description: "Retrieves the notification history for the current user.",
	RPCIDs:      []string{"notification:get-history"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// NotificationMarkRead describes the mark-notification-read RPC.
var NotificationMarkRead = featureregistry.Feature{
	ID:          "notification-mark-read",
	Title:       "Mark Notification Read",
	Description: "Marks one or more notifications as read by their IDs.",
	RPCIDs:      []string{"notification:mark-read"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// NotificationSend describes the send-notification RPC.
var NotificationSend = featureregistry.Feature{
	ID:          "notification-send",
	Title:       "Send Notification",
	Description: "Sends a push notification to subscribed clients and manages subscription lifecycle.",
	RPCIDs:      []string{"notification:send"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(NotificationClearHistory)
	featureregistry.Register(NotificationGetHistory)
	featureregistry.Register(NotificationMarkRead)
	featureregistry.Register(NotificationSend)
}
