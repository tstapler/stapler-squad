package session

// CallbackDispatcher fires an outbound HTTP callback for a named lifecycle event
// (e.g. "session_complete", "session_stale", "queue_item_created" — see
// server/services.CallbackDispatcher's Dispatch doc comment for the full event-type
// list and payload shapes). Implemented outside this package
// (server/services.CallbackDispatcher) since this package cannot import
// server/services — server/services imports session, so the reverse import would be
// a cycle. Mirrors the ItemChangePublisher cross-package adapter pattern (see
// ItemChangePublisher, backlog_item_change.go).
//
// Dispatch must never block the caller (FR8) and must never panic into the caller —
// implementations bound concurrent in-flight dispatch goroutines and drop+log beyond
// that cap (AC10) rather than queuing unboundedly.
type CallbackDispatcher interface {
	Dispatch(eventType string, payload any)
}
