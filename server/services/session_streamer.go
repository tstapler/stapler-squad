package services

// SessionStreamer is the interface the WebSocket streaming handler requires from a session.
// Defined in the consumer package (server/services) to prevent import cycles and to keep
// the interface minimal — only what this package legitimately needs for terminal streaming.
//
// *session.Instance satisfies this interface via delegation methods.
type SessionStreamer interface {
	StartControlMode() error
	StopControlMode() error
	SubscribeControlModeUpdates() (string, <-chan []byte)
	UnsubscribeControlModeUpdates(id string)
}
