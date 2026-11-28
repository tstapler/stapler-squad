package sessionstream

// SessionStreamer is the interface the WebSocket streaming handler requires from a session.
// server/services/wsconn is its consumer; it is declared beside the stream RPCs and kept
// to what terminal streaming needs.
//
// *session.Instance satisfies this interface via delegation methods.
type SessionStreamer interface {
	StartControlMode() error
	StopControlMode() error
	SubscribeControlModeUpdates() (string, <-chan []byte)
	UnsubscribeControlModeUpdates(id string)
}
