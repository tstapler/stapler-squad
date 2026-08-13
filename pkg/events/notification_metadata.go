package events

// Metadata keys stamped onto session-scoped notifications (see
// SessionScopedMetadata). Shared here so producers (server/review_queue_manager.go)
// and consumers (e.g. server/notifications) agree on the exact key strings.
const (
	// MetadataKeySessionScoped marks a notification as originating from a
	// specific session (as opposed to a backlog-item-level or global
	// notification with no associated session).
	MetadataKeySessionScoped = "session_scoped"
	// MetadataKeyItemID carries the backlog item ID a session-scoped
	// notification's session is linked to, when known.
	MetadataKeyItemID = "item_id"
)

// SessionScopedMetadata builds a fresh metadata map for a session-scoped
// notification, copying any entries from base (never mutating base — base may
// be a *ReviewItem.Metadata map stored unlocked and concurrently read by other
// goroutines, e.g. WatchReviewQueue's ReviewItemToProto) and adding the
// session_scoped marker plus item_id when non-empty.
func SessionScopedMetadata(base map[string]string, itemID string) map[string]string {
	m := make(map[string]string, len(base)+2)
	for k, v := range base {
		m[k] = v
	}
	m[MetadataKeySessionScoped] = "true"
	if itemID != "" {
		m[MetadataKeyItemID] = itemID
	}
	return m
}
